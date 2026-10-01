#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# An ADR number is never taken twice, every ADR file has a row in the index (docs/adr/README.md),
# and every index row links a file that exists. The set is the directory (the authority), not the
# index: an ADR the index forgot is the failure this exists to catch.
. "$(dirname "$0")/lib.sh"

dir="$ROOT/$ADR_DIR"
[ -d "$dir" ] || { fail "ADR_DIR $ADR_DIR does not exist"; finish; }
idx="$dir/README.md"
[ -f "$idx" ] || { fail "no index at $ADR_DIR/README.md"; finish; }
[ -f "$dir/TEMPLATE.md" ] && ok "TEMPLATE.md exists" || fail "no $ADR_DIR/TEMPLATE.md for new-adr to copy"

files=$(ls "$dir" | grep -E '^[0-9]{4}-.*\.md$')
dups=$(printf '%s\n' "$files" | grep . | cut -c1-4 | sort | uniq -d)
if [ -z "$dups" ]; then ok "no ADR number is used twice ($(printf '%s\n' "$files" | grep -c .) ADRs)"; else fail "ADR numbers used twice: $(echo $dups) (the branch that merged second takes the next free number)"; fi

for f in $files; do
  if grep -qF "($f)" "$idx"; then :; else fail "$f has no row in $ADR_DIR/README.md's index"; fi
  head -1 "$dir/$f" | grep -qE "^# ADR $(printf '%s' "$f" | cut -c1-4)\b" || fail "$f: first line must be '# ADR $(printf '%s' "$f" | cut -c1-4): <title>'"
  grep -q '^## Enforcement' "$dir/$f" || fail "$f has no '## Enforcement' section (an ADR without one is a wish)"
done
for l in $(grep -oE '\]\([0-9]{4}-[^)]*\.md\)' "$idx" | sed 's/^](//; s/)$//'); do
  [ -f "$dir/$l" ] || fail "the index links $l, which does not exist"
done
[ "$_fails" -eq 0 ] && ok "every ADR is indexed, titled and has an Enforcement section"
finish
