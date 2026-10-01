#!/bin/sh
# The people module's own check, run by checks/run.sh as people:registry only while the module is
# on: this project's people registry (PEOPLE) is sound (a non-empty "people" array, each person
# with a name, a GitHub login and a role, no name or login twice, lanes well formed), and every
# roadmap owner is a person in it (roadmap_queue --check, which runs the module's owner rule).
# The module's materials themselves are tested whether it is on or not, by checks/people.sh.
# shellcheck disable=SC1090
. "${CHECKS_LIB:?run this through checks/run.sh}"
need jq

out=$(sh "$ROOT/.claude/modules/people/people.sh" --check 2>&1); rc=$?
printf '%s\n' "$out"
if [ "$rc" -ne 0 ]; then
  fail "the people registry ($PEOPLE) is not sound (lines above); who-is-on-what and the owner rule cannot read it"
fi
if q=$(roadmap_queue --check 2>&1); then
  ok "every roadmap owner is a person in $PEOPLE ($(printf '%s' "$q" | head -1))"
else
  fail "the roadmap does not pass with the people module on:"; printf '%s\n' "$q" | sed 's/^/     /' | head -20
fi
finish
