#!/bin/sh
# review-cap.sh prints a row's review-round cap from the one place its risk is written: a change's
# proposal (`**Risk:**`, an archived one included), else an ops or fix row's Change cell (`Risk:`
# outside backticks), else normal; REVIEW_CAPS turns the risk into rounds. It refuses, rather than
# guess, a change row with no proposal, an unreadable Risk, and a Change cell naming two risks.
# Also: build-change's maxRounds and merge-pr step 3 ask it (the callers, rule 3).
. "$(dirname "$0")/lib.sh"

F="$(scratch)/proj"; mkdir -p "$F/.claude/lib" "$F/docs" "$F/changes/add-low" "$F/changes/add-bad" "$F/changes/archive/2026-01-02-add-old"
cp "$ROOT/.claude/lib/conf.sh" "$ROOT/.claude/lib/change.sh" "$F/.claude/lib/" 2>/dev/null
cp "$ROOT/.claude/roadmap-queue.sh" "$ROOT/.claude/review-cap.sh" "$F/.claude/" 2>/dev/null
cat > "$F/docs/roadmap.md" <<'EOF'
## Phase 1 — Now
| # | Change | Scope | Deps | Status |
|---|---|---|---|---|
| 1.1 | `add-old` — merged and archived | s | — | ✅ merged (#1) |
| 1.2 | `add-low` — proposed | s | — | ⬜ queued |
| 1.3 | `add-none` — not proposed yet | s | — | ⬜ queued |
| 1.4 | `add-bad` — a proposal with a bad risk | s | — | ⬜ queued |
| 1.5 | `ops/plain` — no risk named | s | — | ⬜ queued |
| 1.6 | `ops/risky` — says how `Risk: low` would read, then Risk: high | s | — | ⬜ queued |
| 1.7 | `fix/7-two` — Risk: low, then Risk: high | s | — | ⬜ queued |
EOF
printf '# add-low\n\n**Risk:** low\n' > "$F/changes/add-low/proposal.md"
printf '# add-bad\n\n**Risk:** medium\n' > "$F/changes/add-bad/proposal.md"
printf '# add-old\n\n**Risk:** high\n' > "$F/changes/archive/2026-01-02-add-old/proposal.md"
cap() { (cd "$F" && sh .claude/review-cap.sh "$@" 2>&1); }
is() {  # is WANT LABEL ARGS…: the cap printed, exit 0
  w=$1; l=$2; shift 2
  if got=$(cap "$@") && [ "$got" = "$w" ]; then ok "review-cap: $l → $w"; else fail "review-cap: $l: want $w, got '$got'"; fi
}
refuses() {  # refuses WORDS LABEL ARGS…: exit 1, saying WORDS
  w=$1; l=$2; shift 2
  if got=$(cap "$@"); then fail "review-cap: $l printed '$got', want a refusal"
  elif printf '%s' "$got" | grep -qF -- "$w"; then ok "review-cap: $l refused ($w)"
  else fail "review-cap: $l refused, but not for '$w': $got"; fi
}
is 2 "a change's proposal, Risk low, by row id" 1.2
# From here high and normal differ, so a test cannot pass by reading the wrong one.
printf 'REVIEW_CAPS="low:2 normal:3 high:4"\n' > "$F/.claude/project.conf"
is 2 "the same, by its branch" change/add-low
is 4 "an archived change's proposal, Risk high" add-old
is 3 "an ops row naming no risk (normal)" 1.5
is 4 "an ops row's Risk: high, a backticked Risk: low ignored" ops/risky
is "4 — risk high, from row 1.6's Change cell" "--why names the source" 1.6 --why
is 3 "a fix branch with no row (normal)" fix/99-from-an-issue
refuses "is not here" "a change row with no proposal" add-none
refuses "not low, normal or high" "a proposal whose Risk is not a tier" add-bad
refuses "more than one risk" "a Change cell naming two risks" fix/7-two
printf 'REVIEW_CAPS="low:1 normal:4 high:5"\n' > "$F/.claude/project.conf"
is 1 "REVIEW_CAPS sets the rounds (low:1)" add-low
is 4 "REVIEW_CAPS sets the rounds (normal:4)" 1.5
printf 'REVIEW_CAPS="normal:3"\n' > "$F/.claude/project.conf"
refuses "has no 'low:<rounds>'" "a risk REVIEW_CAPS has no rounds for" add-low

# The callers: a capability no real process asks is not shipped (rule 3).
grep -q 'review-cap\.sh' "$ROOT/.claude/workflows/build-change.js" 2>/dev/null \
  && ok "build-change.js takes its maxRounds default from review-cap.sh" || fail "build-change.js does not ask review-cap.sh for its maxRounds default"
grep -q 'review-cap\.sh' "$ROOT/.claude/skills/merge-pr/SKILL.md" 2>/dev/null \
  && ok "merge-pr step 3 stops at review-cap.sh's cap" || fail "merge-pr's SKILL.md does not name review-cap.sh"
finish
