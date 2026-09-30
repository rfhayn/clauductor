#!/bin/sh
# Every open change in CHANGES_DIR has the shape changes/README.md gives, so build-change and the
# merge guard can read it: proposal.md with an approval or status line (and the review-page line
# when REVIEW_PAGE=artifact), design.md, tasks.md with `## N.` groups, open tasks as `- [ ]`, and a
# Slice line; spec deltas whose requirements each carry a scenario. Also the living specs' own
# format. The set is the directory, so a new change is checked the moment it exists.
. "$(dirname "$0")/lib.sh"

# Self-test first: this check run against a well-formed change must pass, and against each broken
# shape must fail. (Skipped inside the self-test's own runs.)
if [ -z "${CHANGES_SELFTEST:-}" ]; then
  me="$ROOT/.claude/checks/changes.sh"
  mk() {  # mk NAME: a fixture project with one change, returned in $F
    F="$(scratch)/$1"; mkdir -p "$F/.claude/lib" "$F/.claude/checks" "$F/changes/add-x/specs/auth" "$F/specs"
    cp "$ROOT/.claude/lib/conf.sh" "$F/.claude/lib/"; cp "$ROOT/.claude/checks/lib.sh" "$F/.claude/checks/"
    printf '**Approved:** 2026-01-02 by Ana\n\n## Why\nx\n' > "$F/changes/add-x/proposal.md"
    printf '# Design\n' > "$F/changes/add-x/design.md"
    printf '## 1. Do it\n- [ ] 1.1 thing\n\n- [ ] Slice: a user can x at /x\n' > "$F/changes/add-x/tasks.md"
    printf '## ADDED Requirements\n### Requirement: X\nThe system SHALL x.\n#### Scenario: y\n- **WHEN** a\n- **THEN** b\n' > "$F/changes/add-x/specs/auth/spec.md"
  }
  run_on() { (ROOT="$F" CHANGES_SELFTEST=1 sh "$me" >/dev/null 2>&1); }
  mk good; run_on && ok "self-test: a well-formed change passes" || fail "self-test: a well-formed change FAILED"
  mk noslice; printf '## 1. Do it\n- [ ] 1.1 thing\n' > "$F/changes/add-x/tasks.md"
  run_on && fail "self-test: a change with no Slice line passed" || ok "self-test: no Slice line fails"
  mk nogroups; printf -- '- [ ] thing\n- [ ] Slice: x\n' > "$F/changes/add-x/tasks.md"
  run_on && fail "self-test: tasks.md with no group heading passed" || ok "self-test: no task group fails"
  mk noapproval; printf '## Why\nx\n' > "$F/changes/add-x/proposal.md"
  run_on && fail "self-test: a proposal with no approval line passed" || ok "self-test: no approval line fails"
  mk awaiting; printf '**Status:** awaiting approval\n\n## Why\nx\n' > "$F/changes/add-x/proposal.md"
  run_on && fail "self-test: a proposal awaiting approval passed (it could merge unapproved)" || ok "self-test: a proposal awaiting approval fails"
  mk noscenario; printf '## ADDED Requirements\n### Requirement: X\nThe system SHALL x.\n### Requirement: Y\n#### Scenario: z\n- **THEN** b\n' > "$F/changes/add-x/specs/auth/spec.md"
  run_on && fail "self-test: a requirement with no scenario passed" || ok "self-test: a requirement with no scenario fails"
fi

[ "$PROPOSALS" = openspec ] && { ok "PROPOSALS=openspec: the openspec CLI validates changes (openspec validate --strict)"; finish; }
dir="$ROOT/$CHANGES_DIR"
[ -d "$dir" ] || { fail "CHANGES_DIR $CHANGES_DIR does not exist"; finish; }

# spec_ok FILE: every "### Requirement:" is followed by a "#### Scenario:" before the next one.
spec_ok() {
  awk '/^### Requirement:/{ if (open) { bad = 1; print "requirement without a scenario: " name } open = 1; name = $0; next }
       /^#### Scenario:/{ open = 0 }
       /^## /{ if (open) { bad = 1; print "requirement without a scenario: " name } open = 0 }
       END { if (open) { bad = 1; print "requirement without a scenario: " name } exit bad }' "$1"
}

n=0
for c in "$dir"/*/; do
  [ -d "$c" ] || continue
  id=$(basename "$c"); [ "$id" = archive ] && continue
  n=$((n + 1))
  for f in proposal.md design.md tasks.md; do [ -f "$c/$f" ] || fail "$id: no $f"; done
  if [ -f "$c/proposal.md" ]; then
    if [ "$REVIEW_PAGE" = artifact ]; then
      head -1 "$c/proposal.md" | grep -qE '^\*\*Review page:\*\* https://claude\.ai/artifact/[A-Za-z0-9_-]+$' \
        && ok "$id: proposal.md opens with its review page" || fail "$id: proposal.md line 1 must be '**Review page:** https://claude.ai/artifact/<id>' (REVIEW_PAGE=artifact)"
    fi
    # An unapproved proposal FAILS: this check is a gate step, and the merge guard wants a gate
    # receipt, so a proposal cannot reach main before the owner approves it.
    if grep -qE '^\*\*Approved:\*\* [0-9]{4}-[0-9]{2}-[0-9]{2} by .+' "$c/proposal.md"; then ok "$id: proposal records the owner's approval"
    elif grep -qE '^\*\*Status:\*\* awaiting approval' "$c/proposal.md"; then fail "$id: awaiting the $OWNER_ROLE's approval; it cannot merge until proposal.md reads '**Approved:** YYYY-MM-DD by <name>'"
    else fail "$id: proposal.md needs '**Approved:** YYYY-MM-DD by <owner>' (or '**Status:** awaiting approval' while it waits)"; fi
  fi
  if [ -f "$c/tasks.md" ]; then
    grep -qE '^## [0-9]+\. ' "$c/tasks.md" && ok "$id: tasks.md has task groups" || fail "$id: tasks.md has no '## <n>. <title>' group heading"
    grep -qE '^[[:space:]]*(-[[:space:]]*)?(\[[ xX]\][[:space:]]*)?([0-9]+(\.[0-9]+)*[[:space:]]+)?(\*\*)?Slice:' "$c/tasks.md" \
      && ok "$id: tasks.md states its slice" || fail "$id: tasks.md has no Slice: line (pr-merge-guard rule 3 will refuse the PR)"
  fi
  for s in "$c"/specs/*/spec.md; do
    [ -f "$s" ] || continue
    out=$(spec_ok "$s") && ok "$id: ${s#"$c"} every requirement has a scenario" || fail "$id: ${s#"$c"}: $out"
  done
done
[ "$n" -le 1 ] && ok "$n change(s) open (at most one proposed ahead of the one building)" || ok "NOTE $n changes open: more than one proposed ahead is a finding for session-start"
for s in "$ROOT/$SPECS_DIR"/*/spec.md; do
  [ -f "$s" ] || continue
  out=$(spec_ok "$s") && ok "spec ${s#"$ROOT"/} every requirement has a scenario" || fail "spec ${s#"$ROOT"/}: $out"
  grep -q '^## Purpose' "$s" || fail "spec ${s#"$ROOT"/} has no ## Purpose"
done
[ "$n" -eq 0 ] && ok "no open changes in $CHANGES_DIR"
finish
