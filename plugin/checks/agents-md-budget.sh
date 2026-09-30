#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# AGENTS.md is a budget, not an archive: every session and subagent loads it at startup, so each
# byte is paid for on every agent start. This holds the file to a byte ceiling and each row of
# its "What executes each rule" table to a length ceiling, so a lesson that belongs in the
# insights log cannot quietly accrete here.
#
# Ceilings: AGENTS_MD_MAX_BYTES (default 16000) and AGENTS_MD_MAX_ROW (default 320 characters)
# in .claude/project.conf. Raising a ceiling is a decision; make it in a PR that says why.
# Also checks CLAUDE.md imports AGENTS.md, so the two cannot diverge.
. "$(dirname "$0")/lib.sh"

f="$ROOT/AGENTS.md"
[ -f "$f" ] || { fail "AGENTS.md is missing at $f"; finish; }

max=${AGENTS_MD_MAX_BYTES:-16000}
bytes=$(wc -c < "$f" | tr -d ' ')
if [ "$bytes" -le "$max" ]; then ok "AGENTS.md is $bytes bytes (ceiling $max)"; else fail "AGENTS.md is $bytes bytes, over the $max ceiling: move detail to docs/conventions.md and lessons to the insights log"; fi

rowmax=${AGENTS_MD_MAX_ROW:-320}
# Rows of the table under "What executes each rule", header and separator excluded. Counted in
# characters, not bytes, where the locale allows: the table uses typographic symbols.
rows=$(awk '/^### What executes each rule/{p=1; next} p && /^#/{p=0} p && /^\|/' "$f" | sed '1,2d')
[ -n "$rows" ] || fail "no 'What executes each rule' table found in AGENTS.md (the check reads the table under that heading)"
long=$(printf '%s\n' "$rows" | awk -v m="$rowmax" 'length($0) > m { print length($0) ": " substr($0, 1, 60) "…" }')
if [ -z "$long" ]; then ok "every what-executes row is at most $rowmax characters"; else fail "rows over $rowmax characters (one line per row; put the reasoning in docs/conventions.md):"; printf '%s\n' "$long" | sed 's/^/     /'; fi

# The last row must remain the honest one: what nothing executes.
last=$(printf '%s\n' "$rows" | tail -1)
case "$last" in *"Nothing. You."*) ok "the table ends with its 'Nothing. You.' row" ;; *) fail "the table's last row must be the 'Everything else … Nothing. You.' row" ;; esac

if grep -qx '@AGENTS.md' "$ROOT/CLAUDE.md" 2>/dev/null; then ok "CLAUDE.md imports @AGENTS.md"; else fail "CLAUDE.md does not import @AGENTS.md on a line of its own"; fi
finish
