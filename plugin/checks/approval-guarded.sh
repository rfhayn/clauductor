#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# The guarded list follows AGENTS.md's table (OPS-130). A delegated approval (OPS-127) may never
# cover a change that touches what runs a control, and .claude/lib/approval-guarded.txt is that
# list, written by hand. OPS-127's review found a control runner missing from it in every round, so
# the table is the authority and the list is what was handed (*Enumerate the authority*): every
# path the "What executes each rule" table names in bold (**`...`**) must be guarded by the
# framework list or the project's (.claude/local/approval-guarded.txt), on whole path segments as
# change-approval.sh matches them. A bare name (`pr-merge-guard.sh`, `checks/gate.sh`, `merge-pr`,
# `local:team-queue`) is resolved to the first file it names under .claude/ and the usual control
# directories, a shell function (`roadmap_queue`) to the .claude/lib file that defines it, and an
# agent's `tools:` line to .claude/agents. A name that resolves to nothing fails: a delegation
# target is a named thing, and a typo or a rename must not pass as guarded. Bold in that column
# marks what executes a control, so a command or a key (`gitleaks`, `APPROVAL_DELEGATED`) is named
# by the file that runs it, or left unbolded.
#
# Under the plugin, the model's own files live in the plugin's root, not the project's .claude/
# (the table names them `${CLAUDE_PLUGIN_ROOT}/...`): a name found only there is guarded, since that
# root is the plugin cache, which no branch of the project can change.
#
# It holds with APPROVAL_DELEGATED off too: the list must be whole before the key turns on, and the
# key is turned on in a change of its own that this check would not otherwise see again.
. "$(dirname "$0")/lib.sh"

# guarded_list LIST DIR: the framework LIST's entries and DIR's own, as change-approval.sh reads them.
guarded_list() {
  cat "$1" "$2/.claude/local/approval-guarded.txt" 2>/dev/null \
    | sed 's/[[:space:]]*$//; s|/$||' | grep -v '^#' | grep .
}

# resolve DIR NAME [PLUGIN]: every path NAME stands for in DIR (a skill's framework copy and the
# project's fragment are both the control, so the first match does not hide the rest),
# `plugin:<path>` for one only in the plugin's root PLUGIN, or nothing.
resolve() {
  hits=$(resolve_each "$@")
  [ -n "$hits" ] || return 0
  printf '%s\n' "$hits" | grep -v '^plugin:' || printf '%s\n' "$hits" | head -1
}

resolve_each() {
  n=$2
  case $n in
    '${CLAUDE_PLUGIN_ROOT}/'*) n=".claude/${n#'${CLAUDE_PLUGIN_ROOT}/'}" ;;
    tools:) n=.claude/agents ;;
    local:*) n=".claude/local/checks/${n#local:}.sh" ;;
  esac
  for c in "$n" ".claude/$n" ".claude/skills/$n" ".claude/local/skills/$n" ".claude/hooks/$n" ".claude/checks/$n" \
           ".claude/lib/$n" ".claude/workflows/$n" ".github/workflows/$n" "scripts/ci/$n"; do
    # A glob (`.claude/health/*.sh`) stands for its directory.
    [ -e "$1/$c" ] || [ -e "$1/${c%/\**}" ] && { printf '%s\n' "$c"; continue; }
    [ -n "${3:-}" ] || continue
    case $c in .claude/local/*) ;; .claude/*)
      f=${c#.claude/}; [ -e "$3/$f" ] || [ -e "$3/${f%/\**}" ] && printf 'plugin:%s\n' "$f" ;;
    esac
  done | grep . && return
  case $n in *[!A-Za-z0-9_]*) return ;; esac
  for f in "$1"/.claude/lib/*.sh ${3:+"$3"/lib/*.sh}; do
    grep -Eq "^$n[[:space:]]*\(\)" "$f" 2>/dev/null || continue
    case $f in "$1"/*) printf '%s\n' "${f#"$1"/}" ;; *) printf 'plugin:%s\n' "${f#"$3"/}" ;; esac
    return
  done
}

# names DIR: every name the table's executor column puts in bold.
names() {
  grep -E '^\|' "$1/AGENTS.md" 2>/dev/null | awk -F'|' '{ print $3 }' \
    | grep -oE '\*\*`[^`]+`\*\*' | sed 's/^\*\*`//; s/`\*\*$//' | sort -u
}

# problems DIR LIST [PLUGIN]: one line for each bold table name the lists do not guard or that
# names no file.
problems() {
  g=$(guarded_list "$2" "$1")
  [ -n "$g" ] || { echo "the framework's approval-guarded.txt is missing or lists no path"; return; }
  names "$1" | while IFS= read -r n; do
    ps=$(resolve "$1" "$n" "${3:-}")
    [ -n "$ps" ] || { echo "\`$n\` names no file: name the file that runs it, or unbold it"; continue; }
    printf '%s\n' "$ps" | while IFS= read -r p; do
      case $p in plugin:*) continue ;; esac
      hit=$(printf '%s\n' "$g" | while IFS= read -r e; do case "/$p/" in (*"/$e/"*) echo y; break ;; esac; done)
      [ -n "$hit" ] || echo "\`$n\` ($p) is on neither approval-guarded.txt"
    done
  done
}

# Self-test: an unguarded path, a resolved bare name, an unguarded skill (the framework's or the
# project's), a missing file and a name that resolves nowhere are found; a guarded path, a guarded
# skill, a `local:` check, a lib function and the agents' `tools:` line are not.
d=$(scratch)
mkdir -p "$d/t/.claude/lib" "$d/t/.claude/hooks" "$d/t/.claude/skills/merge-pr" "$d/t/.claude/skills/start-project" \
  "$d/t/.claude/local/skills/deploy-guard" "$d/t/.claude/local/skills/merge-pr" "$d/t/.claude/local/checks" "$d/t/.claude/health" "$d/t/.claude/agents"
: > "$d/t/.claude/hooks/guard.sh"; : > "$d/t/.claude/loose.sh"; : > "$d/t/.claude/bare.sh"; : > "$d/t/.claude/local/checks/q.sh"
printf 'roadmap_queue() {\n  :\n}\n' > "$d/t/.claude/lib/conf.sh"
printf '# a comment\n.claude/hooks/\n.claude/skills/merge-pr\n.claude/health\n.claude/lib\n.claude/agents\n.claude/local/checks\n' > "$d/t/.claude/lib/approval-guarded.txt"
cat > "$d/t/AGENTS.md" <<'EOF'
| Rule | What executes it |
|---|---|
| a | **`.claude/hooks/guard.sh`**; **`guard.sh`**; **`merge-pr`** step 3; **`roadmap_queue`**; **`local:q`**; each agent's **`tools:`** line |
| b | **`.claude/loose.sh`**; **`bare.sh`**; **`.claude/health/*.sh`**; **`start-project`**; **`deploy-guard`** step 2 |
| c | **`.claude/gone.sh`**; **`merge-prr`** |
| **`.claude/loose.sh`** in the rule column | not the executor |
EOF
L="$d/t/.claude/lib/approval-guarded.txt"
# The resolved paths are compared without their .claude/ prefix, so the plugin's path test
# (TestNoProjectPathsLeft) does not read the expected lines as the model reaching into a project.
got=$(problems "$d/t" "$L" | sed 's|(\.claude/|(|')
want='`.claude/gone.sh` names no file: name the file that runs it, or unbold it
`.claude/loose.sh` (loose.sh) is on neither approval-guarded.txt
`bare.sh` (bare.sh) is on neither approval-guarded.txt
`deploy-guard` (local/skills/deploy-guard) is on neither approval-guarded.txt
`merge-pr` (local/skills/merge-pr) is on neither approval-guarded.txt
`merge-prr` names no file: name the file that runs it, or unbold it
`start-project` (skills/start-project) is on neither approval-guarded.txt'
[ "$got" = "$want" ] && ok "self-test: unguarded paths, bare names and skills, a project fragment behind a guarded skill, a missing file and a misspelt name are found; guarded ones, a local: check, a lib function and tools: are not" \
  || fail "self-test: found [$got]"
printf '.claude/loose.sh\n.claude/bare.sh\n.claude/skills/start-project\n.claude/local/skills/deploy-guard\n.claude/local/skills/merge-pr\n' > "$d/t/.claude/local/approval-guarded.txt"
want='`.claude/gone.sh` names no file: name the file that runs it, or unbold it
`merge-prr` names no file: name the file that runs it, or unbold it'
[ "$(problems "$d/t" "$L")" = "$want" ] && ok "self-test: the project's list guards what the framework's leaves out" \
  || fail "self-test: with the project list, found [$(problems "$d/t" "$L")]"

# A command or a key in bold fails; the same control named by its file, or unbolded, passes.
mkdir -p "$d/k/.claude/lib" "$d/k/scripts/ci"; : > "$d/k/scripts/ci/run-local.sh"; printf 'scripts/ci\n' > "$d/k/.claude/lib/approval-guarded.txt"
printf '| a | b |\n|---|---|\n| s | **`gitleaks`** |\n' > "$d/k/AGENTS.md"
k1=$(problems "$d/k" "$d/k/.claude/lib/approval-guarded.txt")
printf '| a | b |\n|---|---|\n| s | **`run-local.sh`** secrets step (`gitleaks`) |\n' > "$d/k/AGENTS.md"
k2=$(problems "$d/k" "$d/k/.claude/lib/approval-guarded.txt")
[ "$k1" = '`gitleaks` names no file: name the file that runs it, or unbold it' ] && [ -z "$k2" ] \
  && ok "self-test: a command in bold fails, and naming the file that runs it passes" \
  || fail "self-test: a command in bold gave [$k1], its file gave [$k2]"

# Under the plugin, the framework's list and the model's files are in the plugin root, and the
# table names them `${CLAUDE_PLUGIN_ROOT}/...` or bare; the project's own skill still needs a list.
mkdir -p "$d/p/fw/lib" "$d/p/fw/hooks" "$d/p/fw/skills/merge-pr" "$d/p/proj/.claude/local/skills/deploy-guard"
: > "$d/p/fw/hooks/guard.sh"; printf 'roadmap_queue() {\n  :\n}\n' > "$d/p/fw/lib/conf.sh"; printf '.claude/hooks\n' > "$d/p/fw/lib/approval-guarded.txt"
cat > "$d/p/proj/AGENTS.md" <<'EOF'
| Rule | What executes it |
|---|---|
| a | **`${CLAUDE_PLUGIN_ROOT}/hooks/guard.sh`**; **`guard.sh`**; **`merge-pr`**; **`roadmap_queue`**; **`deploy-guard`**; **`${CLAUDE_PLUGIN_ROOT}/gone.sh`** |
EOF
got=$(problems "$d/p/proj" "$d/p/fw/lib/approval-guarded.txt" "$d/p/fw")
want='`${CLAUDE_PLUGIN_ROOT}/gone.sh` names no file: name the file that runs it, or unbold it
`deploy-guard` (.claude/local/skills/deploy-guard) is on neither approval-guarded.txt'
[ "$got" = "$want" ] && ok "self-test: under the plugin, its root's files pass and the project's unguarded skill and a missing file do not" \
  || fail "self-test: under the plugin, found [$got]"

[ -f "$ROOT/AGENTS.md" ] || { fail "no AGENTS.md in $ROOT, so the table cannot be read"; finish; }
# Under the plugin the rewriter points this path at the plugin root's list, and CLAUDUCTOR_FW is set.
found=$(problems "$ROOT" "$CLAUDUCTOR_FW/lib/approval-guarded.txt" "${CLAUDUCTOR_FW:-}")
if [ -n "$found" ]; then
  fail "AGENTS.md's table names a control a delegated approval could change, or a name that is no file; guard it in .claude/lib/approval-guarded.txt (the framework's) or .claude/local/approval-guarded.txt (yours), with APPROVAL_DELEGATED off too, so the list is whole before the key turns on:"
  printf '%s\n' "$found" | sed 's/^/       /'
else
  ok "every name AGENTS.md's table puts in bold ($(names "$ROOT" | grep -c .)) is a file guarded from a delegated approval"
fi
finish
