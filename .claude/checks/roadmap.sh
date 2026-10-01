#!/bin/sh
# The roadmap queue parser (.claude/roadmap-queue.sh) accepts the grammar docs/roadmap.md states,
# REFUSES every shape it cannot classify (so no row can vanish silently), derives the current
# phase, and inherits owners section-over-phase. Then: the project's own roadmap parses.
. "$(dirname "$0")/lib.sh"

P="$ROOT/.claude/roadmap-queue.sh"
d=$(scratch)

cat > "$d/good.md" <<'EOF'
# Roadmap
## Phase 1 — Done already
**Owner:** Ana
| # | Change | Scope | Deps | Status |
|---|---|---|---|---|
| 1.1 | `add-a` — a | s | — | ✅ merged (#3) |
| 1.2 | `add-b` — b | s | — | ❌ cancelled — not needed |
## Phase 2 — Now
**Owner:** Ana
### Track X
**Owner:** Ben
| # | Change | Scope | Deps | Status |
|---|---|---|---|---|
| 2.1 | `add-c` — c \| with a pipe | s | 1.1 | ⬜ in flight (#7) |
| 2.2 | `fix/12-crash` — d | s | — | ⬜ queued |
### Track Y
| # | Change | Scope | Deps | Status |
|---|---|---|---|---|
| 2.3 | ~~`add-e`~~ — e | s | — | ⬜ queued — after 2.2 |
| 2.4 | `ops/tidy` — f | s | — | ⬜ queued |
## Phase 3 — Later
| # | Change | Scope | Deps | Status |
|---|---|---|---|---|
| 3.1 | `add-g` — g | s | — | ⬜ queued |
## Notes
| Other | table |
|---|---|
| is | ignored |
EOF
out=$(sh "$P" --check "$d/good.md"); rc=$?
expect_rc 0 "$rc" "a well-formed roadmap passes --check"
case "$out" in *"7 row(s) in 3 phase(s); current phase 2"*) ok "counts rows and derives the current phase (2: phase 1 is all merged/cancelled)" ;; *) fail "wrong summary: $out" ;; esac
tsv=$(sh "$P" --tsv "$d/good.md")
case "$(printf '%s\n' "$tsv" | awk -F'\t' '$4=="2.1"{print $5, $6, $7, $8, $9}')" in "add-c change inflight 7 Ben") ok "row 2.1: id, kind, state, PR, section owner overrides phase owner" ;; *) fail "row 2.1 parsed as: $(printf '%s\n' "$tsv" | grep '	2.1	')" ;; esac
case "$(printf '%s\n' "$tsv" | awk -F'\t' '$4=="2.3"{print $5, $9}')" in "add-e Ana") ok "row 2.3: strikethrough id read, owner falls back to the phase" ;; *) fail "row 2.3 parsed wrong" ;; esac
case "$(printf '%s\n' "$tsv" | awk -F'\t' '$4=="2.2"{print $6}')" in fix) ok "a fix/ id is the fix lane" ;; *) fail "fix/ kind wrong" ;; esac
text=$(sh "$P" --text "$d/good.md")
case "$text" in *"2.2"*NEXT*) ok "--text marks the first queued row of the current phase NEXT" ;; *) fail "--text: $text" ;; esac
case "$text" in *3.1*) fail "--text leaked a later phase's row" ;; *) ok "--text shows only the current phase" ;; esac
[ "$(sh "$P" --queued ops "$d/good.md")" = "ops/tidy" ] && ok "--queued ops lists only queued ops rows" || fail "--queued ops wrong"

bad() {  # bad LABEL ROW-OR-LINES
  printf '## Phase 1 — x\n| # | Change | Scope | Deps | Status |\n|---|---|---|---|---|\n%s\n' "$2" > "$d/bad.md"
  out=$(sh "$P" --check "$d/bad.md"); rc=$?
  if [ "$rc" -ne 0 ] && printf '%s' "$out" | grep -q '^ERROR'; then ok "refuses $1"; else fail "accepted $1 (exit $rc): $out"; fi
}
bad "a row with 4 cells" '| 1.1 | `add-a` — a | s | ⬜ queued |'
bad "a row with 6 cells" '| 1.1 | `add-a` — a | s | — | x | ⬜ queued |'
bad "a row with no change id" '| 1.1 | add-a — a | s | — | ⬜ queued |'
bad "an unknown status" '| 1.1 | `add-a` — a | s | — | 🔨 in flight |'
bad "a status without its symbol" '| 1.1 | `add-a` — a | s | — | queued |'
bad "in flight without a PR" '| 1.1 | `add-a` — a | s | — | ⬜ in flight |'
bad "merged without a PR" '| 1.1 | `add-a` — a | s | — | ✅ merged |'
bad "a duplicate row id" '| 1.1 | `add-a` — a | s | — | ⬜ queued |
| 1.1 | `add-b` — b | s | — | ⬜ queued |'
bad "an empty row id" '|  | `add-a` — a | s | — | ⬜ queued |'
bad "a near-miss owner line" '**Phase 1 owner:** Ana'
printf '## Phase 1 — x\n| # | Change | Scope | Deps | State |\n|---|---|---|---|---|\n' > "$d/near.md"
sh "$P" --check "$d/near.md" >/dev/null 2>&1 && fail "accepted a near-miss queue header" || ok "refuses a near-miss queue header"
sh "$P" --check "$d/absent.md" >/dev/null 2>&1 && fail "a missing roadmap passed" || ok "a missing roadmap is an ERROR, not an empty queue"

# Budgets and dated rows (the change process, items 12 and 13).
cat > "$d/dated.md" <<'EOF'
## Phase 1 — Now
| # | Change | Scope | Deps | Status |
|---|---|---|---|---|
| 1.1 | `add-a` — a | s · Budget: $40 | — | ⬜ queued |
| 1.2 | `add-b` — b | s | — | ⬜ queued |
## Outcome checks
| # | Change | Scope | Deps | Status |
|---|---|---|---|---|
| o.1 | `ops/check-outcome-add-z` — check the outcome of add-z (due 2026-01-10) | the signal | — | ⬜ queued |
| o.2 | `ops/check-outcome-add-y` — check the outcome of add-y (due 2026-03-01) | the signal | — | ⬜ queued |
EOF
tsv=$(sh "$P" --tsv "$d/dated.md")
[ "$(printf '%s\n' "$tsv" | awk -F'\t' '$4=="1.1"{print $11}')" = 40 ] && ok "reads 'Budget: \$40' as the row's budget (tsv column 11)" || fail "budget not read: $(printf '%s\n' "$tsv" | grep '	1.1	')"
[ "$(printf '%s\n' "$tsv" | awk -F'\t' '$4=="o.1"{print $12}')" = 2026-01-10 ] && ok "reads '(due YYYY-MM-DD)' as the row's date (tsv column 12)" || fail "due date not read"
text=$(ROADMAP_TODAY=2026-02-01 sh "$P" --text "$d/dated.md")
case "$text" in *"DUE    o.1"*) ok "--text lists an outcome check that is due, outside every phase" ;; *) fail "--text did not list the due outcome check: $text" ;; esac
case "$text" in *o.2*) fail "--text listed an outcome check that is not due yet" ;; *) ok "--text leaves out an outcome check not yet due" ;; esac
bad "a budget that is not in dollars" '| 1.1 | `add-a` — a | s · Budget: 40 | — | ⬜ queued |'
bad "a malformed due date" '| 1.1 | `add-a` — a (due soon) | s | — | ⬜ queued |'

out=$(sh "$P" --check "$ROOT/$ROADMAP"); rc=$?
expect_rc 0 "$rc" "the project's roadmap ($ROADMAP) parses: $(printf '%s' "$out" | head -1)"
[ "$rc" -eq 0 ] || printf '%s\n' "$out" | sed 's/^/     /'
finish
