#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# review-cap.sh prints a row's review-round cap from the one place its risk is written: a change's
# proposal (`**Risk:**`, an archived one included), else an ops or fix row's Change cell (`Risk:`
# outside backticks), else normal; REVIEW_CAPS turns the risk into rounds. It refuses, rather than
# guess, a change row with no proposal, an unreadable Risk, a Change cell naming two risks, and a
# row line its parser names that is not a line of the queue table.
# Also: build-change's maxRounds and merge-pr step 3 ask it (the callers, rule 3).
. "$(dirname "$0")/lib.sh"

F="$(scratch)/proj"; mkdir -p "$F/.claude/lib" "$F/docs" "$F/changes/add-low" "$F/changes/add-bad" "$F/changes/archive/2026-01-02-add-old"
cp "$CLAUDUCTOR_FW/lib/conf.sh" "$CLAUDUCTOR_FW/lib/change.sh" "$F/.claude/lib/" 2>/dev/null
cp "$CLAUDUCTOR_FW/roadmap-queue.sh" "$CLAUDUCTOR_FW/review-cap.sh" "$F/.claude/" 2>/dev/null
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
| 1.8 | `ops/cell` — the row, Risk: high | the Scope says Risk: low | — | ⬜ queued |
| 1.9 | `ops/pipe` — a \| b. Risk: high | s | — | ⬜ queued |
| 1.10 | `ops/tick` — trims 5` of padding. Risk: high | `s` | — | ⬜ queued |
| 1.11 | `ops/keyspan` — `Risk:` high | s | — | ⬜ queued |
| 1.12 | `ops/odd` — Risk: very high | s | — | ⬜ queued |
| 1.13 | `ops/caps` — Risk: High | s | — | ⬜ queued |
| 1.14 | `ops/entity` — Risk&#58; high | s | — | ⬜ queued |
| 1.15 | `ops/letter` — Ri&#115;k: high | s | — | ⬜ queued |
| 1.16 | `ops/oddent` — Ri&#115;k: low-ish | s | — | ⬜ queued |
| 1.17 | `ops/link` — [Risk](#risk): high | s | — | ⬜ queued |
| 1.18 | `ops/lt` — p95 < 50ms. Risk: high. Touches > 3 files | s | — | ⬜ queued |
| 1.19 | `ops/cgj` — Ri&#847;sk: high | s | — | ⬜ queued |
| 1.20 | `ops/lookalike` — Riskː high | s | — | ⬜ queued |
| 1.21 | `ops/colonval` — Risk: high：today | s | — | ⬜ queued |
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
is 4 "only the Change cell's Risk counts, not the Scope's" ops/cell
is 4 "an escaped pipe does not end the Change cell" ops/pipe
is 4 "a stray backtick does not pair with the Scope's" ops/tick
is 4 "a span holding only the key does not hide the Risk" ops/keyspan
refuses "a Risk that is not" "a Risk word that is not a tier, never normal" ops/odd
refuses "a Risk that is not" "a Risk tier in another case, never normal" ops/caps
is 4 "a Risk read as rendered (an entity colon)" ops/entity
is 4 "a Risk read as rendered (an entity letter)" ops/letter
refuses "a Risk that is not" "an odd Risk spelt with an entity letter" ops/oddent
is 4 "a Risk read as rendered (a link around the key)" ops/link
is 4 "a Risk between a lone < and a later > (no tag)" ops/lt
is 4 "a Risk read as rendered (a combining joiner in the key)" ops/cgj
is 4 "a Risk read as rendered (a look-alike colon)" ops/lookalike
is 4 "a look-alike colon ends a Risk's value, as a colon does" ops/colonval
printf 'REVIEW_CAPS="low:1 normal:4 high:5"\n' > "$F/.claude/project.conf"
is 1 "REVIEW_CAPS sets the rounds (low:1)" add-low
is 4 "REVIEW_CAPS sets the rounds (normal:4)" 1.5
printf 'REVIEW_CAPS="normal:3"\n' > "$F/.claude/project.conf"
refuses "has no 'low:<rounds>'" "a risk REVIEW_CAPS has no rounds for" add-low
# A project's own parser (ROADMAP_PARSER) may name a line of another shape, whose Change cell cannot
# be found: its Risk is unknown, never normal.
printf -- '- 2.1 `ops/listed` — a list row. Risk: high\n' >> "$F/docs/roadmap.md"
ln=$(wc -l < "$F/docs/roadmap.md" | tr -d ' '); lt=$(grep -n '^| 1.9 ' "$F/docs/roadmap.md" | cut -d: -f1)
printf '#!/bin/sh\nprintf "%s\\t1\\t\\t1.9\\tops/pipe\\tops\\tqueued\\t\\t\\tp\\t\\t\\t\\t\\t\\n%s\\t1\\t\\t2.1\\tops/listed\\tops\\tqueued\\t\\t\\tl\\t\\t\\t\\t\\t\\n"\n' "$lt" "$ln" > "$(dirname "$F")/own.sh"
printf 'REVIEW_CAPS="low:2 normal:3 high:4"\nROADMAP_PARSER="sh %s/own.sh"\n' "$(dirname "$F")" > "$F/.claude/project.conf"
is 4 "a table row a project's own parser names" 1.9
refuses "is not a line of the" "a row a project's own parser names in another shape" 2.1

# The callers: a capability no real process asks is not shipped (rule 3).
grep -q 'review-cap\.sh' "$CLAUDUCTOR_FW/workflows/build-change.js" 2>/dev/null \
  && ok "build-change.js takes its maxRounds default from review-cap.sh" || fail "build-change.js does not ask review-cap.sh for its maxRounds default"
grep -q 'review-cap\.sh' "$CLAUDUCTOR_FW/skills/merge-pr/SKILL.md" 2>/dev/null \
  && ok "merge-pr step 3 stops at review-cap.sh's cap" || fail "merge-pr's SKILL.md does not name review-cap.sh"
finish
