#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# Refuses a Bash `cd` that takes the main session out of the worktree a running build-change
# workflow holds. One script, registered for two events and keyed by the payload's hook_event_name:
#   PostToolUse (Workflow)  records the hold: a build-change run started from the session's cwd
#   PreToolUse (Bash)       refuses the main session's `cd` (or `pushd`) out of a held worktree
#
# WHY (OPS-82). A workflow agent takes the session's cwd when it STARTS, not when the run started
# (build-change.js, "EVERY LATER AGENT IS PINNED TO THIS ROOT"). The session's cwd moved out of the
# build's worktree three times on 2026-10-17, the fifth such slip in all (docs/insights-log.md, "cd
# moves the session"), and the next group's builders and reviewers ran in the wrong checkout. The
# workflow's whenToUse already says to stay put: an instruction is not a control (AGENTS.md rule 4).
#
# WHAT IS A BUILD-CHANGE RUN. A Workflow call whose tool_input names it: `name` "build-change", a
# `scriptPath` ending build-change.js, or an inline `script` whose meta says name: 'build-change'.
# The held root is `git rev-parse --show-toplevel` of the payload's cwd as the call returns: the
# worktree build-change's preflight then pins every agent to.
#
# WHAT IS A CD OUT. lib/gate-reader.awk (`-v cds=1`) lists each `cd` that moves THIS shell, in
# order: one inside `( … )`, `$( … )` or `sh -c` moves a child and is not counted. Each target is
# resolved from the payload's cwd (then from the last cd's), physically where it exists and
# lexically where it does not yet (`mkdir x && cd x`). Refused when the command starts inside a held
# root and any target lands outside it. A target that cannot be resolved here (`$VAR`, `$(…)`, a
# glob, `cd -`) is refused too while a hold stands: a refused command can always be rewritten.
#
# WHEN A HOLD ENDS. Nothing tells a hook that a workflow finished, so a hold ends when the first of
# these is seen: its task's output file (<dirname scratchpad_dir>/tasks/<id>.output, as
# no-silent-wait reads) ends "[exited with code N]" or "[killed]"; its worktree is gone; or it is
# older than CD_GUARD_MAX_HOURS (default 24). Every refusal names the hold file to remove by hand.
#
# THE RECORD lives in the git common dir (every worktree shares it), per session:
# <common dir>/clauductor-cd-guard/<session_id>/holds/<task id>, holding the root; outside a git
# repository, under TMPDIR. Another session's records untouched for two days are pruned.
#
# KNOWN LIMITS.
#   - Unprobed: what Claude Code sends on a Workflow's PostToolUse (the task id's key, whether a
#     workflow's task writes an output file with the end line). Without an id the hold is keyed by
#     time and ends only by its worktree or its age.
#   - Only the main session (no agent_id) is held: build-change's own agents `cd` into the root
#     before every command, and whether a subagent's `cd` moves the session's cwd is unprobed.
#   - A `cd` through `eval`, an alias or a function is not seen (lib/gate-reader.awk).
#
# WITHOUT jq, for what it polices (as no-blind-source-rewrite): a Bash call whose raw text holds a
# `cd` or `pushd` word is refused while any hold is recorded in this repository; anything else is
# allowed. Recording needs jq too.
#
# Protocol: exit 2 = block (stderr reaches Claude); exit 0 = allow. Checked by .claude/checks/hooks.sh.

payload=$(cat)

block() { echo "BLOCKED by cd-guard: $1" >&2; exit 2; }
FIX="Stay where you are and reach other paths without moving: absolute paths, git -C <dir>, or a subshell, ( cd <dir> && … ), which moves only the child shell."

common_dir() { git -C "${CLAUDE_PROJECT_DIR:-.}" rev-parse --path-format=absolute --git-common-dir 2>/dev/null || printf '%s\n' "${TMPDIR:-/tmp}"; }

if ! command -v jq >/dev/null 2>&1 || ! printf '%s' "$payload" | jq -e . >/dev/null 2>&1; then
  printf '%s' "$payload" | grep -Eq '"tool_name"[[:space:]]*:[[:space:]]*"Bash"' || exit 0
  printf '%s' "$payload" | grep -Eq '(^|[^A-Za-z0-9_.-])(cd|pushd)([^A-Za-z0-9_.-]|$)' || exit 0
  held=""
  for held in "$(common_dir)"/clauductor-cd-guard/*/holds/*; do [ -f "$held" ] && break; held=""; done
  [ -n "$held" ] || exit 0
  block "jq could not read the hook payload (jq missing, or a malformed payload), this command holds a cd, and a build-change run holds a worktree ($held), so it is refused rather than risk moving the session out of it. Install jq (macOS: brew install jq; Debian/Ubuntu: apt-get install jq). $FIX"
fi

safe() { printf '%s' "$1" | tr -cd 'A-Za-z0-9_.-' | sed 's/^\.*//'; }
fields=$(printf '%s' "$payload" | jq -r '[.hook_event_name, .tool_name, .agent_id, .session_id, .cwd, .scratchpad_dir,
  (.tool_response | objects | .taskId // .task_id // .backgroundTaskId // .runId // .run_id),
  (.tool_input | objects | (.name == "build-change")
    or ((.scriptPath // "") | tostring | test("(^|/)build-change\\.js$"))
    or ((.script // "") | tostring | test("name:[[:space:]]*[\u0027\"]build-change[\u0027\"]")))]
  | map(. // "" | tostring | gsub("[\n\u001f]"; " ")) | join("\u001f")')
IFS="$(printf '\037')" read -r event tool agent sid cwd scratch task_id is_build <<EOF
$fields
EOF
sid=$(safe "$sid"); [ -n "$sid" ] || sid=unknown
task_id=$(safe "$task_id")
[ -n "$cwd" ] || cwd=$(pwd)

if [ "$event" = PostToolUse ]; then
  { [ "$tool" = Workflow ] && [ "$is_build" = true ]; } || exit 0
  top=$(git -C "$cwd" rev-parse --show-toplevel 2>/dev/null) || exit 0
  BASE="$(common_dir)/clauductor-cd-guard"
  mkdir -p "$BASE/$sid/holds" 2>/dev/null || exit 0
  printf '%s\n' "$top" > "$BASE/$sid/holds/${task_id:-run-$(date +%s)}"
  touch "$BASE/$sid"
  find "$BASE" -mindepth 1 -maxdepth 1 -type d ! -name "$sid" -mtime +2 -exec rm -rf {} + 2>/dev/null
  exit 0
fi

{ [ "$event" = PreToolUse ] && [ "$tool" = Bash ] && [ -z "$agent" ]; } || exit 0
cmd=$(printf '%s' "$payload" | jq -r '.tool_input.command // "" | tostring')
# No cd or pushd anywhere in the text: the common case, left before any git call.
case $cmd in *cd*|*pushd*) ;; *) exit 0 ;; esac
HOLDS="$(common_dir)/clauductor-cd-guard/$sid/holds"
[ -d "$HOLDS" ] || exit 0

# phys DIR: DIR's physical path where it exists; else its lexical form, `.` and `..` resolved.
phys() {
  if [ -d "$1" ]; then (cd "$1" 2>/dev/null && pwd -P) && return; fi
  printf '%s\n' "$1" | awk -F/ '{ n = 0; for (i = 2; i <= NF; i++) { if ($i == "" || $i == ".") continue; if ($i == "..") { if (n) n--; continue } p[++n] = $i } o = ""; for (i = 1; i <= n; i++) o = o "/" p[i]; print (o == "" ? "/" : o) }'
}
inside() { case $1 in "$2"|"$2"/*) return 0 ;; esac; return 1; }

tasks_dir=""; [ -n "$scratch" ] && tasks_dir="$(dirname "$scratch")/tasks"
MAXH=${CD_GUARD_MAX_HOURS:-24}
case $MAXH in ''|*[!0-9]*) MAXH=24 ;; esac
start=$(phys "$cwd")
held=""; hold=""
for h in "$HOLDS"/*; do
  [ -f "$h" ] || continue
  r=$(head -n 1 "$h")
  f="$tasks_dir/$(basename "$h").output"
  if [ ! -d "$r" ] || [ -n "$(find "$h" -mmin +$((MAXH * 60)) 2>/dev/null)" ] \
    || { [ -n "$tasks_dir" ] && [ -f "$f" ] && [ ! -L "$f" ] && tail -n 1 "$f" | grep -Eq '^\[(exited with code -?[0-9]+|killed)\]$'; }; then
    rm -f "$h"; continue
  fi
  r=$(phys "$r")
  inside "$start" "$r" && { held=$r; hold=$h; break; }
done
[ -n "$held" ] || exit 0

targets=$(printf '%s' "$cmd" | LC_ALL=C awk -v gates="" -v cds=1 -f "$(dirname "$0")/lib/gate-reader.awk" 2>/dev/null) \
  || block "the command reader (lib/gate-reader.awk) could not read this command, and a build-change run holds $held. $FIX"
WHY="A build-change run holds $held, and every agent it starts from now on takes the session's cwd, so moving the session sends its next builder, gate and commit into the wrong checkout."
END="If that run has ended, remove the hold and retry: rm $hold"
cur=$start
# A heredoc, not a pipe: the loop runs in this shell, so its block() exits the hook.
while IFS= read -r t; do
  [ -n "$t" ] || continue
  case $t in
    '~') t=$HOME ;;
    '~/'*) t="$HOME/${t#\~/}" ;;
    *'$'*|*'`'*|*'*'*|*'?'*|*'['*|-*|'~'*) block "this command runs 'cd $t', whose target cannot be resolved here. $WHY $FIX $END" ;;
  esac
  case $t in /*) ;; *) t="$cur/$t" ;; esac
  t=$(phys "$t")
  inside "$t" "$held" || block "this command moves the session to $t, outside $held. $WHY $FIX $END"
  cur=$t
done <<EOF
$(printf '%s\n' "$targets" | sed -n 's/^CD	//p')
EOF
exit 0
