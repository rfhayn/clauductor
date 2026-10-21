#!/bin/sh
# Refuses a Bash `cd` that takes the main session out of the worktree a running build-change
# workflow holds. One script, registered for three events and keyed by the payload's hook_event_name:
#   PostToolUse (Workflow)  records the hold: a build-change run started from the session's cwd
#   PreToolUse (Bash)       refuses the main session's `cd` (or `pushd`) out of a held worktree
#   Stop                    drops each hold whose task the session no longer lists as running
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
# WHEN A HOLD ENDS. No hook event marks a workflow's end (TaskCompleted never fires for one), so a
# hold ends when the first of these is seen:
#   - its run's state file says "status": "completed", "failed" or "killed". Claude Code writes
#     <transcript_path minus .jsonl>/workflows/<runId>.json as a run ends, and not before. Any other
#     status, or a file jq cannot read, keeps the hold: a guard fails safe;
#   - the session's Stop lists its running tasks in background_tasks, and no longer lists the
#     hold's task (a Stop with no background_tasks at all drops nothing);
#   - its worktree is gone;
#   - it is older than CD_GUARD_MAX_HOURS (default 24): the backstop for a claude killed mid-run,
#     which writes no state file and sends no Stop.
# Every refusal names the hold file to remove by hand.
#
# WHAT WAS PROBED (OPS-82, Claude Code 2.1.306, macOS, headless and interactive, 2026-10-21): the
# PostToolUse payload carries tool_response.taskId and .runId, and every payload transcript_path
# (scratchpad_dir only interactively, so no path is built from it); the state file is absent while
# a run goes, then holds completed, failed or killed (a TaskStop); a Stop lists a running workflow
# in background_tasks and drops it once it ends; a SIGKILLed claude leaves no state file. A task's
# output file is no end signal for a workflow: 0 bytes on a failure or a kill, and the
# "[exited with code N]" and "[killed]" lines belong to shell tasks only.
#
# THE RECORD lives in the git common dir (every worktree shares it), per session:
# <common dir>/clauductor-cd-guard/<session_id>/holds/<task id>: the root on its first line, then
# "task <task id>" and "state <state file>" when the payload names them; outside a git repository,
# under TMPDIR. Another session's records untouched for two days are pruned.
#
# KNOWN LIMITS.
#   - Unprobed: a run resumed with resumeFromRunId (a new runId, so maybe a state file the hold does
#     not name), a stop from the /workflows UI rather than TaskStop, and Linux. Each falls back to
#     Stop, the worktree and the age.
#   - Without a task id the hold is keyed by time, and Stop cannot match it; without a runId or a
#     transcript_path it has no state file. Either ends only by the rest of the list above.
#   - Only the main session (no agent_id) is held: build-change's own agents `cd` into the root
#     before every command, and a workflow agent's `cd` was probed (headless) not to move the
#     session's cwd.
#   - A `cd` through `eval`, an alias or a function is not seen (lib/gate-reader.awk).
#
# WITHOUT jq, for what it polices (as no-blind-source-rewrite): a Bash call whose raw text holds a
# `cd` or `pushd` word is refused while any hold is recorded in this repository; anything else is
# allowed. Recording needs jq too.
#
# Protocol: exit 2 = block (stderr reaches Claude); exit 0 = allow, and always on Stop, where exit 2
# would keep the session from stopping. Checked by .claude/checks/hooks.sh.

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
fields=$(printf '%s' "$payload" | jq -r '[.hook_event_name, .tool_name, .agent_id, .session_id, .cwd, .transcript_path,
  (.tool_response | objects | .taskId // .task_id // .backgroundTaskId),
  (.tool_response | objects | .runId // .run_id),
  (.tool_input | objects | (.name == "build-change")
    or ((.scriptPath // "") | tostring | test("(^|/)build-change\\.js$"))
    or ((.script // "") | tostring | test("name:[[:space:]]*[\u0027\"]build-change[\u0027\"]")))]
  | map(. // "" | tostring | gsub("[\n\u001f]"; " ")) | join("\u001f")')
IFS="$(printf '\037')" read -r event tool agent sid cwd transcript task_id run_id is_build <<EOF
$fields
EOF
sid=$(safe "$sid"); [ -n "$sid" ] || sid=unknown
task_id=$(safe "$task_id"); run_id=$(safe "$run_id")
[ -n "$cwd" ] || cwd=$(pwd)
BASE="$(common_dir)/clauductor-cd-guard"
HOLDS="$BASE/$sid/holds"

if [ "$event" = PostToolUse ]; then
  { [ "$tool" = Workflow ] && [ "$is_build" = true ]; } || exit 0
  top=$(git -C "$cwd" rev-parse --show-toplevel 2>/dev/null) || exit 0
  mkdir -p "$HOLDS" 2>/dev/null || exit 0
  state=""
  case $transcript in /*.jsonl) [ -z "$run_id" ] || state="${transcript%.jsonl}/workflows/$run_id.json" ;; esac
  { printf '%s\n' "$top"
    [ -z "$task_id" ] || printf 'task %s\n' "$task_id"
    [ -z "$state" ] || printf 'state %s\n' "$state"; } > "$HOLDS/${task_id:-run-$(date +%s)}"
  touch "$BASE/$sid"
  find "$BASE" -mindepth 1 -maxdepth 1 -type d ! -name "$sid" -mtime +2 -exec rm -rf {} + 2>/dev/null
  exit 0
fi

# Stop: a hold whose task the session's background_tasks no longer lists has ended. Only a list
# that is there counts: a payload without one drops nothing. Never exit 2 here.
if [ "$event" = Stop ]; then
  [ -d "$HOLDS" ] || exit 0
  running=$(printf '%s' "$payload" | jq -er '.background_tasks | arrays | map(objects | .id // empty | tostring) | join("\n")' 2>/dev/null) || exit 0
  for h in "$HOLDS"/*; do
    [ -f "$h" ] || continue
    t=$(sed -n 's/^task //p' "$h" | head -n 1)
    [ -n "$t" ] || continue
    printf '%s\n' "$running" | grep -qxF -- "$t" || rm -f "$h"
  done
  exit 0
fi

{ [ "$event" = PreToolUse ] && [ "$tool" = Bash ] && [ -z "$agent" ]; } || exit 0
cmd=$(printf '%s' "$payload" | jq -r '.tool_input.command // "" | tostring')
# No cd or pushd anywhere in the text: the common case, left before any git call.
case $cmd in *cd*|*pushd*) ;; *) exit 0 ;; esac
[ -d "$HOLDS" ] || exit 0

# phys DIR: DIR's physical path where it exists; else its lexical form, `.` and `..` resolved.
phys() {
  if [ -d "$1" ]; then (cd "$1" 2>/dev/null && pwd -P) && return; fi
  printf '%s\n' "$1" | awk -F/ '{ n = 0; for (i = 2; i <= NF; i++) { if ($i == "" || $i == ".") continue; if ($i == "..") { if (n) n--; continue } p[++n] = $i } o = ""; for (i = 1; i <= n; i++) o = o "/" p[i]; print (o == "" ? "/" : o) }'
}
inside() { case $1 in "$2"|"$2"/*) return 0 ;; esac; return 1; }
# ended HOLD: the hold's state file says its run ended. No file (the run is going), another status,
# or a file jq cannot read all say no.
ended() {
  _s=$(sed -n 's/^state //p' "$1" | head -n 1)
  [ -n "$_s" ] && [ -f "$_s" ] || return 1
  _st=$(jq -r '.status // empty | strings' "$_s" 2>/dev/null) || return 1
  case $_st in completed|failed|killed) return 0 ;; esac
  return 1
}

MAXH=${CD_GUARD_MAX_HOURS:-24}
case $MAXH in ''|*[!0-9]*) MAXH=24 ;; esac
start=$(phys "$cwd")
held=""; hold=""
for h in "$HOLDS"/*; do
  [ -f "$h" ] || continue
  r=$(head -n 1 "$h")
  if [ ! -d "$r" ] || [ -n "$(find "$h" -mmin +$((MAXH * 60)) 2>/dev/null)" ] || ended "$h"; then
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
