#!/bin/sh
# Scenario-to-test traceability (.claude/scenario-trace.sh), falsified both ways in throwaway repos:
# a complete change whose scenarios are cited passes; one uncited scenario fails; a (manual: …)
# line escapes it and an escape with no reason does not; AUTH-1-S10 does not cite AUTH-1-S1; a
# citation in a non-test file or in the spec itself does not count; each default TEST_GLOBS layout
# (Go, JS/TS, Python, a tests/ directory) is found; a change still being built is PENDING, not red;
# after archive, deleting the test that cited a living scenario fails; --rev reads a commit, not
# the working tree. Then the project's own trace must pass.
. "$(dirname "$0")/lib.sh"
need git

d=$(scratch)
EX="$ROOT/.claude/examples"
TRACE="$ROOT/.claude/scenario-trace.sh"

# mkrepo NAME: a repo holding the example change (every task ticked: a finished build) and the
# living spec it modifies, with the scripts the trace needs. Returns it in $R.
mkrepo() {
  R="$d/$1"; new_repo "$R"
  mkdir -p "$R/.claude/lib" "$R/changes" "$R/specs"
  cp "$ROOT/.claude/lib/conf.sh" "$ROOT/.claude/lib/change.sh" "$R/.claude/lib/"
  cp "$TRACE" "$R/.claude/"
  cp -R "$EX/changes/add-greeting-name" "$R/changes/"
  cp -R "$EX/specs/greeting" "$R/specs/"
  sed -i.bak 's/- \[ \]/- [x]/' "$R/changes/add-greeting-name/tasks.md" && rm -f "$R/changes/add-greeting-name/tasks.md.bak"
}
trace() { (cd "$R" && sh .claude/scenario-trace.sh --check "$@") > "$d/out" 2>&1; }
expect() {  # expect WANT LABEL [trace args...]
  want=$1 label=$2; shift 2
  trace "$@"; rc=$?
  expect_rc "$want" "$rc" "scenario-trace $label"
  [ "$rc" = "$want" ] || sed 's/^/       /' "$d/out" | tail -8
}

# ── The shapes of a citation ───────────────────────────────────────────────────────────────────
mkrepo cited
mkdir -p "$R/tests"; cp "$EX/test/greeting.sh" "$R/tests/"
expect 0 "passes a finished change whose scenarios a tests/ file cites (and one (manual: …) line)"
grep -q '^MANUAL   FAREWELL-1-S2' "$d/out" && ok "the (manual: reason) line is reported as MANUAL, with its reason" || fail "no MANUAL line for FAREWELL-1-S2: $(cat "$d/out")"
grep -q '^CITED    GREETING-1-S1 .*tests/greeting.sh' "$d/out" && ok "a CITED line names the citing test file" || fail "CITED line does not name tests/greeting.sh"

sed -i.bak '/FAREWELL-1-S1/d' "$R/tests/greeting.sh" && rm -f "$R/tests/greeting.sh.bak"
expect 1 "fails when one added scenario (FAREWELL-1-S1) is cited by no test"
grep -q '^MISSING  FAREWELL-1-S1' "$d/out" && ok "it names the missing ID" || fail "MISSING line absent: $(cat "$d/out")"

echo '# FAREWELL-1-S1 is described in the README, not tested' > "$R/README.md"
expect 1 "does not count a citation in a file that is not a test"
printf '# FAREWELL-1-S10: another scenario\n' >> "$R/tests/greeting.sh"
expect 1 "does not read FAREWELL-1-S10 as citing FAREWELL-1-S1"
printf '# XFAREWELL-1-S1: another capability\n' >> "$R/tests/greeting.sh"
expect 1 "does not read XFAREWELL-1-S1 as citing FAREWELL-1-S1"

sed -i.bak 's/- \[x\] 2.1 Show the farewell/- [x] 2.1 [FAREWELL-1-S1] (untestable: )  Show the farewell/' "$R/changes/add-greeting-name/tasks.md" && rm -f "$R/changes/add-greeting-name/tasks.md.bak"
expect 1 "does not accept an escape with a blank reason"
sed -i.bak 's/(untestable: )/(untestable: rendered by the identity provider)/' "$R/changes/add-greeting-name/tasks.md" && rm -f "$R/changes/add-greeting-name/tasks.md.bak"
expect 0 "accepts an (untestable: <reason>) line naming the ID"

# ── Each default layout ────────────────────────────────────────────────────────────────────────
layout() {  # layout LABEL PATH
  mkrepo "layout-$(printf '%s' "$1" | tr -c 'a-z0-9' '-')"
  mkdir -p "$R/$(dirname "$2")"
  printf '// GREETING-1-S1 GREETING-1-S2 FAREWELL-1-S1\n' > "$R/$2"
  expect 0 "finds a $1 test ($2) with the default TEST_GLOBS"
}
layout "Go" "internal/home/greeting_test.go"
layout "TypeScript" "src/home/greeting.test.ts"
layout "JS spec" "web/greeting.spec.js"
layout "Python" "pkg/test_greeting.py"
layout "Python suffix" "pkg/greeting_test.py"
layout "__tests__ directory" "src/__tests__/greeting.js"
mkrepo notests
mkdir -p "$R/src"; printf '// GREETING-1-S1 GREETING-1-S2 FAREWELL-1-S1\n' > "$R/src/greeting.ts"
expect 1 "does not count src/greeting.ts (no test glob matches it)"
printf 'TEST_GLOBS="src/*.ts"\n' > "$R/.claude/project.conf"
expect 0 "reads TEST_GLOBS from project.conf (a path glob)"
printf 'TEST_GLOBS=""\n' > "$R/.claude/project.conf"
expect 1 "with TEST_GLOBS empty nothing is a test, so an enforced scenario is MISSING"

# ── A change still being built, the living specs, and --rev ────────────────────────────────────
mkrepo pending
mkdir -p "$R/tests"; printf '# GREETING-1-S1\n' > "$R/tests/living.sh"
sed -i.bak 's/- \[x\] 2.1/- [ ] 2.1/' "$R/changes/add-greeting-name/tasks.md" && rm -f "$R/changes/add-greeting-name/tasks.md.bak"
expect 0 "does not enforce a change with open tasks (a proposal, or a build in progress)"
grep -q '^PENDING  change add-greeting-name: 1 task' "$d/out" && ok "...and says it is PENDING" || fail "no PENDING line: $(cat "$d/out")"

mkrepo living
mkdir -p "$R/tests"; cp "$EX/test/greeting.sh" "$R/tests/"
rm -rf "$R/changes/add-greeting-name"
cp "$R/specs/greeting/spec.md" "$R/specs/greeting/spec.md.orig"
cp "$EX/changes/add-greeting-name/specs/greeting/spec.md" "$d/delta"
{ sed -n '1,/^## Requirements/p' "$R/specs/greeting/spec.md.orig"; echo; sed -n '/^### Requirement/,$p' "$d/delta"; } > "$R/specs/greeting/spec.md"
rm -f "$R/specs/greeting/spec.md.orig"
git -C "$R" add -A && git -C "$R" commit -qm "archived"
expect 0 "passes the living specs while their tests cite them"
sed -i.bak '/GREETING-1-S2/d' "$R/tests/greeting.sh" && rm -f "$R/tests/greeting.sh.bak"
expect 1 "fails the living specs once the test that cited GREETING-1-S2 is deleted"
expect 0 "--rev reads the committed tree, where the citation still exists" --rev "$(git -C "$R" rev-parse HEAD)"
git -C "$R" commit -qam "drop the test"
expect 1 "--rev on the commit that deleted the citation fails" --rev "$(git -C "$R" rev-parse HEAD)"
mkdir -p "$R/changes/archive/2026-01-01-x"
printf -- '- [x] 1.1 [GREETING-1-S2] checked by hand (manual: needs a real member)\n' > "$R/changes/archive/2026-01-01-x/tasks.md"
expect 0 "a living scenario's escape is read from an archived tasks.md"

# ── The project's own trace ────────────────────────────────────────────────────────────────────
out=$(sh "$TRACE" --check 2>&1); rc=$?
expect_rc 0 "$rc" "this project's scenario trace: $(printf '%s\n' "$out" | tail -1)"
[ "$rc" -eq 0 ] || printf '%s\n' "$out" | grep -v '^CITED' | sed 's/^/     /'
finish
