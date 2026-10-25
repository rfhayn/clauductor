#!/bin/sh
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
# target is a named thing, and a typo or a rename must not pass as guarded.
#
# It holds with APPROVAL_DELEGATED off too: the list must be whole before the key turns on, and the
# key is turned on in a change of its own that this check would not otherwise see again.
. "$(dirname "$0")/lib.sh"

# guarded_list DIR: both lists' entries, as change-approval.sh reads them.
guarded_list() {
  cat "$1/.claude/lib/approval-guarded.txt" "$1/.claude/local/approval-guarded.txt" 2>/dev/null \
    | sed 's/[[:space:]]*$//; s|/$||' | grep -v '^#' | grep .
}

# resolve DIR NAME: the path NAME stands for in DIR, or nothing.
resolve() {
  case $2 in
    tools:) [ -d "$1/.claude/agents" ] && echo .claude/agents; return ;;
    local:*) set -- "$1" ".claude/local/checks/${2#local:}.sh" ;;
  esac
  for c in "$2" ".claude/$2" ".claude/skills/$2" ".claude/local/skills/$2" ".claude/hooks/$2" ".claude/checks/$2" \
           ".claude/lib/$2" ".claude/workflows/$2" ".github/workflows/$2" "scripts/ci/$2"; do
    # A glob (`.claude/health/*.sh`) stands for its directory.
    [ -e "$1/$c" ] || [ -e "$1/${c%/\**}" ] && { printf '%s\n' "$c"; return; }
  done
  case $2 in *[!A-Za-z0-9_]*) return ;; esac
  for f in "$1"/.claude/lib/*.sh; do
    grep -Eq "^$2[[:space:]]*\(\)" "$f" 2>/dev/null && { printf '%s\n' "${f#"$1"/}"; return; }
  done
}

# names DIR: every name the table's executor column puts in bold.
names() {
  grep -E '^\|' "$1/AGENTS.md" 2>/dev/null | awk -F'|' '{ print $3 }' \
    | grep -oE '\*\*`[^`]+`\*\*' | sed 's/^\*\*`//; s/`\*\*$//' | sort -u
}

# problems DIR: one line for each bold table name the lists do not guard or that names no file.
problems() {
  g=$(guarded_list "$1")
  [ -n "$g" ] || { echo ".claude/lib/approval-guarded.txt is missing or lists no path"; return; }
  names "$1" | while IFS= read -r n; do
    p=$(resolve "$1" "$n")
    [ -n "$p" ] || { echo "\`$n\` names no file"; continue; }
    hit=$(printf '%s\n' "$g" | while IFS= read -r e; do case "/$p/" in (*"/$e/"*) echo y; break ;; esac; done)
    [ -n "$hit" ] || echo "\`$n\` ($p) is on neither approval-guarded.txt"
  done
}

# Self-test: an unguarded path, a resolved bare name, an unguarded skill (the framework's or the
# project's), a missing file and a name that resolves nowhere are found; a guarded path, a guarded
# skill, a `local:` check, a lib function and the agents' `tools:` line are not.
d=$(scratch)
mkdir -p "$d/t/.claude/lib" "$d/t/.claude/hooks" "$d/t/.claude/skills/merge-pr" "$d/t/.claude/skills/start-project" \
  "$d/t/.claude/local/skills/deploy-guard" "$d/t/.claude/local/checks" "$d/t/.claude/health" "$d/t/.claude/agents"
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
got=$(problems "$d/t")
want='`.claude/gone.sh` names no file
`.claude/loose.sh` (.claude/loose.sh) is on neither approval-guarded.txt
`bare.sh` (.claude/bare.sh) is on neither approval-guarded.txt
`deploy-guard` (.claude/local/skills/deploy-guard) is on neither approval-guarded.txt
`merge-prr` names no file
`start-project` (.claude/skills/start-project) is on neither approval-guarded.txt'
[ "$got" = "$want" ] && ok "self-test: unguarded paths, bare names and skills, a missing file and a misspelt name are found; guarded ones, a local: check, a lib function and tools: are not" \
  || fail "self-test: found [$got]"
printf '.claude/loose.sh\n.claude/bare.sh\n.claude/skills/start-project\n.claude/local/skills/deploy-guard\n' > "$d/t/.claude/local/approval-guarded.txt"
want='`.claude/gone.sh` names no file
`merge-prr` names no file'
[ "$(problems "$d/t")" = "$want" ] && ok "self-test: the project's list guards what the framework's leaves out" \
  || fail "self-test: with the project list, found [$(problems "$d/t")]"

[ -f "$ROOT/AGENTS.md" ] || { fail "no AGENTS.md in $ROOT, so the table cannot be read"; finish; }
found=$(problems "$ROOT")
if [ -n "$found" ]; then
  fail "AGENTS.md's table names a control a delegated approval could change, or a name that is no file; guard it in .claude/lib/approval-guarded.txt (the framework's) or .claude/local/approval-guarded.txt (yours), with APPROVAL_DELEGATED off too, so the list is whole before the key turns on:"
  printf '%s\n' "$found" | sed 's/^/       /'
else
  ok "every name AGENTS.md's table puts in bold ($(names "$ROOT" | grep -c .)) is a file guarded from a delegated approval"
fi
finish
