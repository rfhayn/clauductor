#!/bin/sh
# The living-visuals module's check of THIS project, run by checks/run.sh as living-visuals:pages
# only while the module is on, so in every gate: each living page (a registry entry with `refresh`)
# is whole, its scripts parse, its generated blocks equal their commands and nothing they carry is
# hand-mirrored outside them, its claims equal their commands (both directions, for a family with
# `each`), its day counters are real dates its own script fills, it carries no typed elapsed
# duration, and its refresh skill exists. bin/living.sh --check does the work; this relays it.
# The module's materials themselves are tested whether it is on or not, by
# .claude/checks/living-visuals.sh.
# shellcheck disable=SC1090
. "${CHECKS_LIB:?run this through checks/run.sh}"
need jq
out=$(sh "$ROOT/.claude/modules/living-visuals/bin/living.sh" --root "$ROOT" --check 2>&1); rc=$?
printf '%s\n' "$out" | grep -v '^FAIL' | grep '^ok' || true
bad=$(printf '%s\n' "$out" | grep -v '^ok')
if [ -n "$bad" ]; then
  printf '%s\n' "$bad" | while IFS= read -r l; do case $l in FAIL*) echo "$l" ;; *) echo "FAIL $l" ;; esac; done
  _fails=$((_fails + 1))
elif [ "$rc" -ne 0 ]; then fail "living.sh --check exited $rc and said nothing that failed: this learned nothing"; fi
finish
