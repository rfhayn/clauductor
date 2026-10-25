#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# The guarded list follows AGENTS.md's table (OPS-130). A delegated approval (OPS-127) may never
# cover a change that touches what runs a control, and .claude/lib/approval-guarded.txt is that
# list, written by hand. OPS-127's review found a control runner missing from it in every round, so
# the table is the authority and the list is what was handed (*Enumerate the authority*): every
# path the "What executes each rule" table names in bold (**`...`**) must be guarded by the
# framework list or the project's (.claude/local/approval-guarded.txt), on whole path segments as
# change-approval.sh matches them. A bare name (`pr-merge-guard.sh`, `checks/gate.sh`, `merge-pr`)
# is resolved to the first file it names under .claude/ and the usual control directories; a name
# with a `.` or `/` that resolves to nothing fails (a delegation target is a named thing), and one
# with neither (`roadmap_queue`, `tools:`) is a function or a key, not a path, and is skipped.
. "$(dirname "$0")/lib.sh"

# guarded_list DIR: both lists' entries, as change-approval.sh reads them.
guarded_list() {
  cat "$1/.claude/lib/approval-guarded.txt" "$1/.claude/local/approval-guarded.txt" 2>/dev/null \
    | sed 's/[[:space:]]*$//; s|/$||' | grep -v '^#' | grep .
}

# resolve DIR NAME: the path NAME stands for in DIR, or nothing.
resolve() {
  for c in "$2" ".claude/$2" ".claude/skills/$2" ".claude/hooks/$2" ".claude/checks/$2" ".claude/lib/$2" \
           ".claude/workflows/$2" ".github/workflows/$2" "scripts/ci/$2"; do
    # A glob (`.claude/health/*.sh`) stands for its directory.
    [ -e "$1/$c" ] || [ -e "$1/${c%/\**}" ] && { printf '%s\n' "$c"; return; }
  done
}

# problems DIR: one line for each bold table path the lists do not guard or that names no file.
problems() {
  g=$(guarded_list "$1")
  [ -n "$g" ] || { echo ".claude/lib/approval-guarded.txt is missing or lists no path"; return; }
  grep -E '^\|' "$1/AGENTS.md" 2>/dev/null | awk -F'|' '{ print $3 }' \
    | grep -oE '\*\*`[^`]+`\*\*' | sed 's/^\*\*`//; s/`\*\*$//' | sort -u | while IFS= read -r n; do
    case $n in *.* | */*) ;; *) [ -n "$(resolve "$1" "$n")" ] || continue ;; esac
    p=$(resolve "$1" "$n")
    [ -n "$p" ] || { echo "\`$n\` names no file"; continue; }
    hit=$(printf '%s\n' "$g" | while IFS= read -r e; do case "/$p/" in (*"/$e/"*) echo y; break ;; esac; done)
    [ -n "$hit" ] || echo "\`$n\` ($p) is on neither approval-guarded.txt"
  done
}

# Self-test: an unguarded path, a missing one and a resolved bare name are found; a guarded path,
# a key and a skill on the list are not.
d=$(scratch)
mkdir -p "$d/t/.claude/lib" "$d/t/.claude/hooks" "$d/t/.claude/skills/merge-pr" "$d/t/.claude/health"
: > "$d/t/.claude/hooks/guard.sh"; : > "$d/t/.claude/loose.sh"; : > "$d/t/.claude/bare.sh"
printf '# a comment\n.claude/hooks/\n.claude/skills/merge-pr\n.claude/health\n' > "$d/t/.claude/lib/approval-guarded.txt"
cat > "$d/t/AGENTS.md" <<'EOF'
| Rule | What executes it |
|---|---|
| a | **`.claude/hooks/guard.sh`**; **`guard.sh`**; **`merge-pr`** step 3; **`roadmap_queue`** |
| b | **`.claude/loose.sh`**; **`bare.sh`**; **`.claude/health/*.sh`** |
| c | **`.claude/gone.sh`** |
| **`.claude/loose.sh`** in the rule column | not the executor |
EOF
got=$(problems "$d/t")
want='`.claude/gone.sh` names no file
`.claude/loose.sh` (.claude/loose.sh) is on neither approval-guarded.txt
`bare.sh` (.claude/bare.sh) is on neither approval-guarded.txt'
[ "$got" = "$want" ] && ok "self-test: an unguarded path, a resolved bare name and a missing file are found; guarded paths, a skill and a key are not" \
  || fail "self-test: found [$got]"
printf '.claude/loose.sh\n.claude/bare.sh\n' > "$d/t/.claude/local.txt"; mkdir -p "$d/t/.claude/local"; mv "$d/t/.claude/local.txt" "$d/t/.claude/local/approval-guarded.txt"
[ "$(problems "$d/t")" = '`.claude/gone.sh` names no file' ] && ok "self-test: the project's list guards what the framework's leaves out" \
  || fail "self-test: with the project list, found [$(problems "$d/t")]"

[ -f "$ROOT/AGENTS.md" ] || { fail "no AGENTS.md in $ROOT, so the table cannot be read"; finish; }
found=$(problems "$ROOT")
if [ -n "$found" ]; then
  fail "AGENTS.md's table names a control a delegated approval could change; add it to .claude/lib/approval-guarded.txt (the framework's) or .claude/local/approval-guarded.txt (yours):"
  printf '%s\n' "$found" | sed 's/^/       /'
else
  ok "every path AGENTS.md's table names in bold ($(grep -E '^\|' "$ROOT/AGENTS.md" | awk -F'|' '{ print $3 }' | grep -oE '\*\*`[^`]+`\*\*' | sort -u | grep -c .) names) is guarded from a delegated approval"
fi
finish
