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
| 1.22 | `ops/nolink` — x [see](Risk: high) | s | — | ⬜ queued |
| 1.23 | `ops/reflabel` — x [see][Risk: high] | s | — | ⬜ queued |
| 1.24 | `ops/named` — Risk&ratio; high | s | — | ⬜ queued |
| 1.25 | `ops/runs` — x. `Risk: high`` | s | — | ⬜ queued |
| 1.26 | `ops/esclink` — [\*\*Risk\*\*](x): high | s | — | ⬜ queued |
| 1.27 | `ops/nestlink` — [[Risk]](x): high | s | — | ⬜ queued |
| 1.28 | `ops/parenlink` — [Risk](https://e.org/Tiers_(review)): high | s | — | ⬜ queued |
| 1.29 | `ops/twospan` — a row may say ``Risk: high`` | s | — | ⬜ queued |
| 1.30 | `ops/twospanin` — Risk: low. A row may say ``a` Risk: high`` instead | s | — | ⬜ queued |
| 1.31 | `ops/anglelink` — [\*Risk](<a (b>): high | s | — | ⬜ queued |
| 1.32 | `ops/anglenest` — [[Risk]](<a(b>): high | s | — | ⬜ queued |
| 1.33 | `ops/titlelink` — [\*Risk](x "a (b"): high | s | — | ⬜ queued |
| 1.34 | `ops/escopen` — [Risk](https://e.org/a\(b): high | s | — | ⬜ queued |
| 1.35 | `ops/escclose` — [Risk](https://e.org/a\)b): high | s | — | ⬜ queued |
| 1.36 | `ops/angleclose` — [\*Risk](<a )b>): high | s | — | ⬜ queued |
| 1.37 | `ops/titleclose` — [\*Risk](x "a )b"): high | s | — | ⬜ queued |
| 1.38 | `ops/balanced` — [Risk](https://e.org/a_(b)c): high | s | — | ⬜ queued |
| 1.39 | `ops/oldread` — [Risk](see the note): high | s | — | ⬜ queued |
| 1.40 | `ops/opencell` — x [a](b|c) Risk: high | — | ⬜ queued |
| 1.41 | `ops/openangle` — x [a](<b|c>) Risk: high | — | ⬜ queued |
| 1.42 | `ops/opentitle` — x [a](b "c|d") Risk: high | — | ⬜ queued |
| 1.43 | `ops/escangle` — [\*Risk](<a\>)b>): high | s | — | ⬜ queued |
| 1.44 | `ops/esctitle` — [\*Risk](x "a\")b"): high | s | — | ⬜ queued |
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
is "4 — risk high, from row 1.6's Change cell; REVIEW_AT_CAP stop" "--why names the source and REVIEW_AT_CAP" 1.6 --why
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
is 4 "a Risk in a destination with a space, which is no link" ops/nolink
is 4 "a Risk in a reference label, which renders where a definition names it" ops/reflabel
is 4 "a Risk read as rendered (a named entity not listed, as a colon)" ops/named
is 4 "a Risk after a lone backtick, which no run of one length closes" ops/runs
# A link CommonMark makes that lk does not match (an escape in its text, a bracket in it, a paren in
# its destination) still reads as its text: every destination is also read dropped.
is 4 "a Risk read as rendered (an escaped emphasis in a link's text)" ops/esclink
is 4 "a Risk read as rendered (a bracket in a link's text)" ops/nestlink
refuses "a Risk that is not" "a paren in a link's destination reads as the reader before OPS-131 read it (Risk):)" ops/parenlink
is 3 "a Risk in a span of two backticks is a mention (normal)" ops/twospan
is 2 "a Risk in a span of two backticks holding one is a mention (the row's low)" ops/twospanin
# A destination is read by its grammar: a <…> one, a bare one whose parens balance, a title, an
# escaped paren that counts for nothing. Each would hide the Risk if read to its first `)`.
is 4 "a Risk read as rendered (an open paren in a <…> destination)" ops/anglelink
is 4 "a Risk read as rendered (a bracket in the text, a paren in a <…> destination)" ops/anglenest
is 4 "a Risk read as rendered (an open paren in a link's title)" ops/titlelink
is 4 "a Risk read as rendered (an escaped open paren in a destination)" ops/escopen
is 4 "a Risk read as rendered (an escaped close paren in a destination)" ops/escclose
is 4 "a Risk read as rendered (a close paren in a <…> destination)" ops/angleclose
is 4 "a Risk read as rendered (a close paren in a link's title)" ops/titleclose
is 4 "a Risk read as rendered (balanced parens mid-destination)" ops/balanced
is 4 "a Risk read as rendered (an escaped > in a <…> destination)" ops/escangle
is 4 "a Risk read as rendered (an escaped quote in a link's title)" ops/esctitle
is 4 "a destination no link closes is still dropped, as the reader before OPS-131 dropped it" ops/oldread
# A destination open at the end of the Change cell never reaches into the Scope, whose Risk is not
# the row's: a cell's `|` ends it in every reading.
is 3 "a bare destination open at the Change cell's end does not take in the Scope" ops/opencell
is 3 "a <…> destination open at the Change cell's end does not take in the Scope" ops/openangle
is 3 "a title open at the Change cell's end does not take in the Scope" ops/opentitle
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
printf 'REVIEW_CAPS="low:2 normal:3 high:4"\nREVIEW_AT_CAP="bogus"\n' > "$F/.claude/project.conf"
refuses "not stop or known-limits" "an unreadable REVIEW_AT_CAP refuses, never a cap that lets merge through" 1.6
printf 'REVIEW_CAPS="low:2 normal:3 high:4"\nREVIEW_AT_CAP="known-limits"\n' > "$F/.claude/project.conf"
is "4 — risk high, from row 1.6's Change cell; REVIEW_AT_CAP known-limits" "REVIEW_AT_CAP known-limits appears on --why" 1.6 --why

# The callers: a capability no real process asks is not shipped (rule 3).
grep -q 'review-cap\.sh' "$CLAUDUCTOR_FW/workflows/build-change.js" 2>/dev/null \
  && ok "build-change.js takes its maxRounds default from review-cap.sh" || fail "build-change.js does not ask review-cap.sh for its maxRounds default"
grep -q 'REVIEW_AT_CAP' "$CLAUDUCTOR_FW/workflows/build-change.js" 2>/dev/null \
  && ok "build-change.js reads REVIEW_AT_CAP from review-cap.sh --why" || fail "build-change.js does not read REVIEW_AT_CAP"
grep -q 'review-cap\.sh' "$CLAUDUCTOR_FW/skills/merge-pr/SKILL.md" 2>/dev/null \
  && ok "merge-pr step 3 stops at review-cap.sh's cap" || fail "merge-pr's SKILL.md does not name review-cap.sh"
grep -q 'REVIEW_AT_CAP stop' "$CLAUDUCTOR_FW/skills/merge-pr/SKILL.md" 2>/dev/null \
  && ok "merge-pr step 3 stops at cap with medium or worse when REVIEW_AT_CAP is stop" || fail "merge-pr does not stop on medium+ at cap under stop"
grep -qF 'do not merge' "$CLAUDUCTOR_FW/skills/merge-pr/SKILL.md" 2>/dev/null \
  && ok "merge-pr step 3 refuses to merge past medium+ at cap under stop" || fail "merge-pr may still merge past medium+ at cap under stop"
grep -q 'known-limits' "$CLAUDUCTOR_FW/skills/merge-pr/SKILL.md" 2>/dev/null \
  && ok "merge-pr step 3 keeps known-limits behavior for projects that want it" || fail "merge-pr does not name known-limits"
finish
