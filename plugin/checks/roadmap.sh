#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# The roadmap queue parser (.claude/roadmap-queue.sh) accepts the grammar docs/roadmap.md states,
# REFUSES every shape it cannot classify (so no row can vanish silently), derives the current
# phase, and inherits owners section-over-phase. Then: the project's own roadmap parses.
. "$(dirname "$0")/lib.sh"

P="$CLAUDUCTOR_FW/roadmap-queue.sh"
d=$(scratch)
# The grammar tests run the template's own parser, whatever ROADMAP_PARSER this project sets.
rq() { ROADMAP_BUILTIN=1 sh "$P" "$@"; }

cat > "$d/good.md" <<'EOF'
# Roadmap
## Phase 1 — Done already
**Owner:** Ana
**Stage 1 — the dry run. Owner: #116. ✅ MET 2026-09-08, on the second run.**
**Owners decide the order** of the rows below.
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
out=$(rq --check "$d/good.md"); rc=$?
expect_rc 0 "$rc" "a well-formed roadmap passes --check"
case "$out" in *"7 row(s) in 3 phase(s); current phase 2"*) ok "counts rows and derives the current phase (2: phase 1 is all merged/cancelled)" ;; *) fail "wrong summary: $out" ;; esac
tsv=$(rq --tsv "$d/good.md")
case "$(printf '%s\n' "$tsv" | awk -F'\t' '$4=="2.1"{print $5, $6, $7, $8, $9}')" in "add-c change inflight 7 Ben") ok "row 2.1: id, kind, state, PR, section owner overrides phase owner" ;; *) fail "row 2.1 parsed as: $(printf '%s\n' "$tsv" | grep '	2.1	')" ;; esac
case "$(printf '%s\n' "$tsv" | awk -F'\t' '$4=="2.3"{print $5, $9}')" in "add-e Ana") ok "row 2.3: strikethrough id read, owner falls back to the phase" ;; *) fail "row 2.3 parsed wrong" ;; esac
case "$(printf '%s\n' "$tsv" | awk -F'\t' '$4=="2.2"{print $6}')" in fix) ok "a fix/ id is the fix lane" ;; *) fail "fix/ kind wrong" ;; esac
text=$(rq --text "$d/good.md")
case "$text" in *"2.2"*NEXT*) ok "--text marks the first queued row of the current phase NEXT" ;; *) fail "--text: $text" ;; esac
case "$text" in *3.1*) fail "--text leaked a later phase's row" ;; *) ok "--text shows only the current phase" ;; esac
[ "$(rq --queued ops "$d/good.md")" = "ops/tidy" ] && ok "--queued ops lists only queued ops rows" || fail "--queued ops wrong"

bad() {  # bad LABEL ROW-OR-LINES
  printf '## Phase 1 — x\n| # | Change | Scope | Deps | Status |\n|---|---|---|---|---|\n%s\n' "$2" > "$d/bad.md"
  out=$(rq --check "$d/bad.md"); rc=$?
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
rq --check "$d/near.md" >/dev/null 2>&1 && fail "accepted a near-miss queue header" || ok "refuses a near-miss queue header"
rq --check "$d/absent.md" >/dev/null 2>&1 && fail "a missing roadmap passed" || ok "a missing roadmap is an ERROR, not an empty queue"

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
tsv=$(rq --tsv "$d/dated.md")
[ "$(printf '%s\n' "$tsv" | awk -F'\t' '$4=="1.1"{print $11}')" = 40 ] && ok "reads 'Budget: \$40' as the row's budget (tsv column 11)" || fail "budget not read: $(printf '%s\n' "$tsv" | grep '	1.1	')"
[ "$(printf '%s\n' "$tsv" | awk -F'\t' '$4=="o.1"{print $12}')" = 2026-01-10 ] && ok "reads '(due YYYY-MM-DD)' as the row's date (tsv column 12)" || fail "due date not read"
text=$(ROADMAP_TODAY=2026-02-01 rq --text "$d/dated.md")
case "$text" in *"DUE    o.1"*) ok "--text lists an outcome check that is due, outside every phase" ;; *) fail "--text did not list the due outcome check: $text" ;; esac
case "$text" in *o.2*) fail "--text listed an outcome check that is not due yet" ;; *) ok "--text leaves out an outcome check not yet due" ;; esac
bad "a budget that is not in dollars" '| 1.1 | `add-a` — a | s · Budget: 40 | — | ⬜ queued |'
bad "a malformed due date" '| 1.1 | `add-a` — a (due soon) | s | — | ⬜ queued |'

# Gates (P2.15): a section's started line and open rows in their own word. Ported from the
# meta-tests of the parser this grammar was upstreamed from; each refusal, with its guard removed,
# parsed as prose and left the gate "not started" with exit 0.
cat > "$d/gates.md" <<'EOF'
## Phase 1 — Done
| # | Change | Scope | Deps | Status |
|---|---|---|---|---|
| 1.1 | `add-a` — a | s | — | ✅ merged (#3) |
## Phase 2 — Now
**Owner:** Ana
### Gate 2C-L — the league track
**Gate 2C-L started:** 2026-09-24 — the first commits of #319
| # | Change | Scope | Deps | Status |
|---|---|---|---|---|
| 2C.9 | `add-league` — league | s | — | ⬜ planned — walkthrough ready, not started |
| 2C.10 | `add-other` — other | s | 2C.9 | ⬜ queued (after ✅ 2C.8) |
### Gate 2D — the app
| # | Change | Scope | Deps | Status |
|---|---|---|---|---|
| 2D.1 | `add-app` — app | s | — | ⬜ placeholder — owner: designer, scope TBD |
### Gate 2E — later
| # | Change | Scope | Deps | Status |
|---|---|---|---|---|
| 2E.1 | `add-trip` — trip | s | — | ✅ merged (#9) |
**Gate 2E started:** 2026-08-01
## Phase 3 — Later
| # | Change | Scope | Deps | Status |
|---|---|---|---|---|
| 3.1 | `add-g` — g | s | — | ⬜ deferred — until launch |
EOF
out=$(rq --check "$d/gates.md"); expect_rc 0 $? "a roadmap with started lines and open rows in their own word passes --check ($out)"
tsv=$(rq --tsv "$d/gates.md")
[ "$(printf '%s\n' "$tsv" | awk -F'\t' '$4=="2C.9"{print $7, $13}')" = "open 2026-09-24" ] && ok "a started line with a reason dates its section's rows (tsv column 13); ⬜ planned is state open" || fail "row 2C.9 read as: $(printf '%s\n' "$tsv" | grep '	2C.9	')"
[ "$(printf '%s\n' "$tsv" | awk -F'\t' '$4=="2D.1"{print $7 "|" $13 "|"}')" = "open||" ] && ok "a section with no started line has none (an empty column, not a borrowed date)" || fail "row 2D.1 read as: $(printf '%s\n' "$tsv" | grep '	2D.1	')"
[ "$(printf '%s\n' "$tsv" | awk -F'\t' '$4=="2E.1"{print $13}')" = 2026-08-01 ] && ok "a started line after its section's table still dates the rows above it" || fail "row 2E.1 read as: $(printf '%s\n' "$tsv" | grep '	2E.1	')"
[ "$(printf '%s\n' "$tsv" | awk -F'\t' '$4=="2C.10"{print $7}')" = queued ] && ok "an open row whose status merely mentions a tick stays queued" || fail "row 2C.10 read wrong"
case "$(rq --check "$d/gates.md")" in *"current phase 2"*) ok "open rows keep their phase current" ;; *) fail "current phase with only open rows: $(rq --check "$d/gates.md")" ;; esac
printf '## Phase 1 — x\n| # | Change | Scope | Deps | Status |\n|---|---|---|---|---|\n| 1.1 | `add-a` — a | s | — | ⬜ planned |\n## Phase 2 — y\n| # | Change | Scope | Deps | Status |\n|---|---|---|---|---|\n| 2.1 | `add-b` — b | s | — | ⬜ queued |\n' > "$d/only-open.md"
case "$(rq --check "$d/only-open.md")" in *"current phase 1"*) ok "a phase whose only open row is in its own word is still the current phase" ;; *) fail "a phase of open rows was skipped: $(rq --check "$d/only-open.md")" ;; esac
text=$(ROADMAP_TODAY=2026-10-01 rq --text "$d/gates.md")
case "$text" in *"2C.10"*NEXT*) ok "--text: NEXT is the first QUEUED row; an open row in its own word is never NEXT" ;; *) fail "--text NEXT: $text" ;; esac
case "$text" in *"2C.9"*"[planned]"*"(started 2026-09-24, 7d)"*) ok "--text shows an open row under its own word, and its section's start with the days since" ;; *) fail "--text open row: $text" ;; esac
case "$text" in *"2D.1"*"[placeholder]"*) ok "--text lists every open row of the current phase" ;; *) fail "--text dropped an open row: $text" ;; esac
case "$(rq --queued "$d/gates.md")" in add-other) ok "--queued lists only queued rows, never one waiting in its own word" ;; *) fail "--queued with open rows: $(rq --queued "$d/gates.md")" ;; esac
gate_bad() {  # gate_bad LABEL LINES: lines after a ### Gate 2D section heading and its row
  printf '## Phase 1 — x\n### Gate 2D — the app\n%s\n| # | Change | Scope | Deps | Status |\n|---|---|---|---|---|\n| 2D.1 | `add-a` — a | s | — | ⬜ queued |\n' "$2" > "$d/gbad.md"
  out=$(rq --check "$d/gbad.md"); rc=$?
  if [ "$rc" -ne 0 ] && printf '%s' "$out" | grep -q "$3"; then ok "refuses $1"; else fail "accepted $1 (exit $rc): $out"; fi
}
gate_bad "a near-miss started line (Started)" '**Gate 2D Started:** 2026-09-24' 'looks like a started line'
gate_bad "a near-miss started line (start)" '**Gate 2D start:** 2026-09-24' 'looks like a started line'
gate_bad "a started line whose date is not YYYY-MM-DD" '**Gate 2D started:** 24 Sept' 'must read'
gate_bad "a started line with an impossible date" '**Gate 2D started:** 2026-02-30' 'impossible date'
gate_bad "a started line that names another section" '**Gate 2U started:** 2026-09-24' 'not its own'
gate_bad "a started line for a prefix of the section's name" '**Gate 2 started:** 2026-09-24' 'not its own'
gate_bad "a second started line for one section" '**Gate 2D started:** 2026-09-24
**Gate 2D started:** 2026-09-25' 'a second started line'
printf '## Phase 1 — x\n**Gate 2D started:** 2026-09-24\n| # | Change | Scope | Deps | Status |\n|---|---|---|---|---|\n| 1.1 | `add-a` — a | s | — | ⬜ queued |\n' > "$d/gout.md"
rq --check "$d/gout.md" | grep -q 'outside any ### section' && ok "refuses a started line outside any ### section" || fail "a started line outside a section: $(rq --check "$d/gout.md")"
printf '## Phase 1 — x\n**Restart:** after the freeze\n**Start here:** the README\n' > "$d/gprose.md"
rq --check "$d/gprose.md" >/dev/null && ok "bold labels that only contain or begin with \"start\" are prose, not near-miss started lines" || fail "bold prose refused as a started line: $(rq --check "$d/gprose.md")"
bad "an open status that is not one word" '| 1.1 | `add-a` — a | s | — | ⬜ Planned |'
bad "an open status glued to its text" '| 1.1 | `add-a` — a | s | — | ⬜ planned2 |'
bad "\"⬜ in\" that is not in flight" '| 1.1 | `add-a` — a | s | — | ⬜ in progress |'
bad "a bare ⬜" '| 1.1 | `add-a` — a | s | — | ⬜ |'
bad "a misspelled queued (⬜ queud), not an open word" '| 1.1 | `add-a` — a | s | — | ⬜ queud |'
bad "in flight spelled as one word (⬜ inflight (#12))" '| 1.1 | `add-a` — a | s | — | ⬜ inflight (#12) |'
bad "a merged row with the open glyph (⬜ merged (#3))" '| 1.1 | `add-a` — a | s | — | ⬜ merged (#3) |'
bad "a cancelled row with the open glyph" '| 1.1 | `add-a` — a | s | — | ⬜ cancelled — dropped |'
bad "a synonym of merged (⬜ shipped (#3))" '| 1.1 | `add-a` — a | s | — | ⬜ shipped (#3) |'
printf '#!/bin/sh\nprintf "1\\t1\\t\\t1.1\\tadd-a\\tchange\\tqueued\\t\\tAna\\tsummary\\t\\t\\t24 Sept\\n"\n' > "$d/started13.sh"
(cd "$d" && printf '%s\n' "$(sh started13.sh)" | roadmap_tsv_errors) >/dev/null && fail "a 13th column that is not a date passed the contract" || ok "a --tsv started column that is not YYYY-MM-DD breaks the contract"

bad "a near-miss owner line written as a gate label" '**Gate 2U owner:** Ben'
printf '## Phase 1 — x\n**Stage 1 — the dry run. Owner: #116. ✅ MET.**\n' > "$d/prose.md"
rq --check "$d/prose.md" >/dev/null 2>&1 && ok "bold prose that mentions an owner is not a near-miss owner line" || fail "bold prose naming an owner was refused as an owner line: $(rq --check "$d/prose.md")"

# Deps (OPS-38): the row ids a row waits on, or #N outside the roadmap, as --tsv column 15. A Deps
# cell the queue cannot follow is an error in every mode, never a row that silently waits on nothing.
cat > "$d/deps.md" <<'EOF'
## Phase 1 — Done
| # | Change | Scope | Deps | Status |
|---|---|---|---|---|
| 1.1 | `add-a` — a | s | — | ✅ merged (#3) |
| 1.2 | `add-b` — b | s | — | ❌ cancelled — not needed |
| 1.3 | `add-c` — c | s | 1.2 | ❌ cancelled — 1.2 was |
| 1.4 | `add-f` — f | s | 1.2 | ✅ merged (#6) |
## Phase 2 — Now
| # | Change | Scope | Deps | Status |
|---|---|---|---|---|
| 2.1 | `add-d` — d | s | 1.1 | ⬜ queued |
| 2.2 | `add-e` — e | s | 2.1,  #45 ,1.1 | ⬜ planned |
EOF
out=$(rq --check "$d/deps.md"); expect_rc 0 $? "a roadmap whose Deps name rows and #N, and whose merged and cancelled rows waited on a cancelled one (history), passes --check ($out)"
tsv=$(rq --tsv "$d/deps.md")
[ "$(printf '%s\n' "$tsv" | awk -F'\t' '$4=="2.2"{print NF "|" $15}')" = "15|2.1,#45,1.1" ] && ok "Deps are --tsv column 15: ids and #N, comma-separated, no spaces" || fail "row 2.2's Deps read as: $(printf '%s\n' "$tsv" | grep '	2.2	' | cut -f4,14-)"
[ "$(printf '%s\n' "$tsv" | awk -F'\t' '$4=="1.1"{print NF "|" $15 "|"}')" = "15||" ] && ok "a Deps cell of — is an empty column 15" || fail "row 1.1's Deps read as: $(printf '%s\n' "$tsv" | grep '	1.1	' | cut -f4,14-)"
(printf '%s\n' "$tsv" | roadmap_tsv_errors) >/dev/null && ok "the template parser's 15 columns meet the --tsv contract" || fail "the template parser's rows broke the contract: $(printf '%s\n' "$tsv" | roadmap_tsv_errors)"
deps_bad() {  # deps_bad LABEL ROWS ERROR-TEXT: refused by --check, --text and --tsv alike
  printf '## Phase 1 — x\n| # | Change | Scope | Deps | Status |\n|---|---|---|---|---|\n%s\n' "$2" > "$d/dbad.md"
  for m in --check --text --tsv; do
    out=$(rq $m "$d/dbad.md"); rc=$?
    if [ "$rc" -eq 0 ] || ! printf '%s' "$out" | grep -q "^ERROR" || ! printf '%s' "$out" | grep -qF -- "$3"; then
      fail "accepted $1 under $m (exit $rc, looked for '$3'): $out"; return
    fi
  done
  ok "refuses $1 (--check, --text and --tsv): $3"
}
deps_bad "Deps naming no row" '| 1.1 | `add-a` — a | s | 9.9 | ⬜ queued |' 'line 4 (1.1): Deps name 9.9, which is no row'
deps_bad "a freeform Deps cell" '| 1.1 | `add-a` — a | s | — | ✅ merged (#2) |
| 1.2 | `add-b` — b | s | 1.1 (the way back only) | ⬜ queued |' '"1.1 (the way back only)" is not a row id or #N'
deps_bad "a Deps token that is not #N" '| 1.1 | `add-a` — a | s | #4x | ⬜ queued |' '"#4x" is not a row id or #N'
deps_bad "a row that waits on itself" '| 1.1 | `add-a` — a | s | 1.1 | ⬜ queued |' 'Deps cycle: 1.1 → 1.1'
deps_bad "a Deps cycle across rows" '| 1.1 | `add-a` — a | s | 1.3 | ⬜ queued |
| 1.2 | `add-b` — b | s | 1.1 | ⬜ queued |
| 1.3 | `add-c` — c | s | 1.2 | ✅ merged (#4) |' 'Deps cycle: 1.1 → 1.3 → 1.2 → 1.1'
deps_bad "an open row waiting on a cancelled row" '| 1.1 | `add-a` — a | s | — | ❌ cancelled — dropped |
| 1.2 | `add-b` — b | s | 1.1 | ⬜ in flight (#5) |' 'Deps name 1.1, which is cancelled'
[ "$(rq --check "$d/dbad.md" | grep -c 'cycle')" = 0 ] && ok "...and a cancelled dependency is not also read as a cycle" || fail "a cancelled dependency also read as a cycle: $(rq --check "$d/dbad.md")"
# The contract holds a plugged parser's column 15 the same way (roadmap_tsv_errors).
tsv15() { printf '1\t1\t\t1.1\tadd-a\tchange\tqueued\t\tAna\ta\t\t\t\t\t%s\n2\t1\t\t1.2\tadd-b\tchange\tcancelled\t\tAna\tb\t\t\t\t\t\n' "$1" | roadmap_tsv_errors; }
tsv15 '1.2' | grep -q 'which is cancelled' && ok "the --tsv contract refuses a parser's open row waiting on a cancelled one" || fail "the contract accepted a wait on a cancelled row: $(tsv15 '1.2')"
tsv15 '1.2, #3' | grep -q '"1.2, #3" is not a row id or #N' && ok "the --tsv contract refuses a column 15 that is not comma-separated ids" || fail "the contract accepted a spaced column 15: $(tsv15 '1.2, #3')"
tsv15 '7.7' | grep -q 'Deps name 7.7, which is no row' && ok "the --tsv contract refuses a parser's Deps naming no row" || fail "the contract accepted an unknown dependency: $(tsv15 '7.7')"
tsv15 '#3' >/dev/null && ok "the --tsv contract accepts #N, a dependency outside the roadmap" || fail "the contract refused #N: $(tsv15 '#3')"

# ── ROADMAP_PARSER: a project's own parser, plugged in through the contract ──────────────────────
# A project shaped like one whose roadmap has its own grammar (phases with gates, owner lines per
# gate, status words of its own): the template's parser refuses it; the project's parser, named
# by ROADMAP_PARSER, reads it, and every consumer follows the key (the panel's suggestions here).
need jq
F="$d/own"; mkdir -p "$F/.claude/lib" "$F/docs" "$F/changes" "$F/infra"
cp "$CLAUDUCTOR_FW/lib/conf.sh" "$CLAUDUCTOR_FW/lib/change.sh" "$CLAUDUCTOR_FW/lib/fragments.sh" "$F/.claude/lib/"
cp "$CLAUDUCTOR_FW/roadmap-queue.sh" "$CLAUDUCTOR_FW/roadmap-plan.sh" "$CLAUDUCTOR_FW/panel-suggest.sh" "$F/.claude/"
cat > "$F/docs/roadmap.md" <<'EOF'
# Roadmap

## Phase 1 — Foundations  *(✅ done — closed 2026-01-14)*
**Phase 1 owner:** Ana

| # | Change | Scope | Deps | Status |
|---|--------|-------|------|--------|
| 1 | **`add-accounts`** — people can sign in | auth | — | ✅ merged 2026-01-10 (#12) |
| 2 | **`add-teams`** — teams of people | teams | 1 | ✅ merged |

## Phase 2 — Domain core  *(the real build begins)*
**Phase 2 owner:** Ana — every row below belongs to this owner unless its gate names another.

#### Gate 2A — "the model is correct" *(no UI)*
**Gate 2A started:** 2026-02-01 — the first commits of #20.

**Stage 1 — the dry run. Owner: #16. ✅ MET 2026-02-08, on the second run.**

| # | Change | Scope | Deps | Status |
|---|--------|-------|------|--------|
| 2A.1 | **`add-season-standings`** — standings across a season | engine | — | ⬜ in flight (#31) |
| 2A.2 | ~~**`add-game-types`**~~ — cancelled | — | — | ❌ retired 2026-02-03 |
| 2A.3 | **`add-handicaps`** — compute an index | engine · Budget: $30 | 2A.1 | ⬜ planned — after 2A.1 |

#### Gate 2U — "design & brand"
**Gate 2U owner:** Ben

| # | Change | Scope | Deps | Status |
|---|--------|-------|------|--------|
| 2U.1 | **`ops/brand-kit`** — the brand kit | design | — | ⬜ queued |
EOF
# The project's own parser (a stand-in for one in another language): its own state words.
cat > "$F/infra/roadmap-queue.sh" <<'EOF'
#!/bin/sh
mode=${1:---text}; [ $# -gt 0 ] && shift
f=${1:-$ROADMAP}; [ -f "$f" ] || { echo "ERROR: $f is missing"; exit 1; }
awk -v mode="$mode" '
  function trim(s) { sub(/^[ \t]+/, "", s); sub(/[ \t]+$/, "", s); return s }
  /^## Phase [0-9]+/ { split($0, a, " "); ph = a[3]; po = ""; go = ""; gate = ""; next }
  /^## / { ph = "-"; next }
  /^#### Gate / { split($0, a, " "); gate = "Gate " a[3]; go = ""; next }
  /^\*\*Phase [0-9]+ owner:\*\*/ { po = $4; next }
  /^\*\*Gate [^ ]+ owner:\*\*/ { go = $4; next }
  /^\| [0-9A-Za-z.]+ \|/ {
    n = split($0, c, "|"); id = trim(c[2]); if (id == "#") next
    match(c[3], /`[^`]+`/); cid = substr(c[3], RSTART + 1, RLENGTH - 2)
    sm = substr(c[3], RSTART + RLENGTH); gsub(/\*\*|~~/, "", sm); sub(/^[ \t—-]+/, "", sm); sm = trim(sm)
    st = trim(c[6]); pr = ""; if (match(st, /\(#[0-9]+\)/)) pr = substr(st, RSTART + 2, RLENGTH - 3)
    s = st; sub(/^[^ ]+ /, "", s); split(s, w, " "); state = w[1]; if (state == "in") state = "inflight"
    kind = cid ~ /^ops\// ? "ops" : (cid ~ /^fix\// ? "fix" : "change")
    b = ""; if (match($0, /Budget: \$[0-9]+/)) b = substr($0, RSTART + 9, RLENGTH - 9)
    rows++; printf "%d\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t\n", NR, ph, gate, id, cid, kind, state, pr, (go != "" ? go : po), sm, b
  }' "$f" > "${TMPDIR:-/tmp}/own-rq.$$"
case $mode in
  --tsv) cat "${TMPDIR:-/tmp}/own-rq.$$" ;;
  --check) echo "roadmap ok: $(grep -c . "${TMPDIR:-/tmp}/own-rq.$$") row(s)" ;;
  --text) awk -F'\t' '$7 == "queued" || $7 == "planned" || $7 == "inflight" { print "  " $4 "  " $5 "  [" $7 "]" }' "${TMPDIR:-/tmp}/own-rq.$$" ;;
esac
rm -f "${TMPDIR:-/tmp}/own-rq.$$"
EOF
# The adapter: the project's state words to the contract's four.
printf '#!/bin/sh\nawk -F"\\t" -v OFS="\\t" '"'"'$7 == "planned" { $7 = "queued" } $7 == "retired" { $7 = "cancelled" } { print }'"'"'\n' > "$F/infra/normalize.sh"
conf() { printf 'ROADMAP_PARSER="%s"\nROADMAP_NORMALIZER="%s"\n' "$1" "$2" > "$F/.claude/project.conf"; }
fq() { (cd "$F" && sh .claude/roadmap-queue.sh "$@") 2>&1; }

conf "" ""
out=$(fq --check); rc=$?
[ "$rc" -ne 0 ] && ok "the template's parser refuses a roadmap in another grammar (so it needs ROADMAP_PARSER)" || fail "the template's parser accepted a foreign grammar: $out"
case "$out" in *"dry run"*|*":17:"*) fail "the template's parser flagged bold prose naming an owner (line 17) as an owner line: $out" ;; *) ok "...and does not flag the bold prose that names an owner (the near-miss regex)" ;; esac

conf "sh infra/roadmap-queue.sh" "sh infra/normalize.sh"
out=$(fq --check); expect_rc 0 $? "ROADMAP_PARSER: the front door hands --check to the project's parser ($out)"
tsv=$(fq --tsv); rc=$?
expect_rc 0 "$rc" "ROADMAP_PARSER: --tsv through the normalizer meets the contract"
[ "$(printf '%s\n' "$tsv" | awk -F'\t' '$4=="2A.3"{print $5, $7, $11}')" = "add-handicaps queued 30" ] && ok "...a row in the project's own words reads as queued, budget in column 11" || fail "row 2A.3 read as: $(printf '%s\n' "$tsv" | grep '	2A.3	')"
[ "$(printf '%s\n' "$tsv" | awk -F'\t' '$4=="2U.1"{print $6, $9}')" = "ops Ben" ] && ok "...a gate's owner and an ops row come through" || fail "row 2U.1 read as: $(printf '%s\n' "$tsv" | grep '	2U.1	')"
[ "$(fq --queued | tr '\n' ' ')" = "add-handicaps ops/brand-kit " ] && ok "ROADMAP_PARSER: --queued is derived from --tsv (the parser need not implement it)" || fail "--queued through the key: $(fq --queued)"
[ "$(fq --queued ops)" = "ops/brand-kit" ] && ok "ROADMAP_PARSER: --queued ops filters by kind" || fail "--queued ops through the key: $(fq --queued ops)"
case "$(fq --text)" in *"add-handicaps"*) ok "ROADMAP_PARSER: --text is the project's parser's own" ;; *) fail "--text through the key: $(fq --text)" ;; esac
[ "$(cd "$F" && sh .claude/panel-suggest.sh ops | jq -r '.[].name')" = "brand-kit" ] && ok "a consumer (panel-suggest) follows ROADMAP_PARSER" || fail "panel-suggest ignored ROADMAP_PARSER: $(cd "$F" && sh .claude/panel-suggest.sh ops 2>&1)"

conf "sh infra/roadmap-queue.sh" ""
out=$(fq --tsv); rc=$?
if [ "$rc" -ne 0 ] && printf '%s' "$out" | grep -q 'column 7 (state) "planned"'; then ok "without its normalizer, a state outside the contract is an ERROR, not a dropped row"; else fail "a non-contract state passed (exit $rc): $out"; fi
(cd "$F" && sh .claude/panel-suggest.sh ops >/dev/null 2>&1) && fail "panel-suggest offered rows from a queue that broke the contract" || ok "...and a consumer reads that queue as UNKNOWN"
conf "false" ""
fq --text >/dev/null && fail "a parser that fails read as a queue" || ok "a parser that exits non-zero is an UNKNOWN queue"
printf '#!/bin/sh\nprintf "1\\t1\\t\\t1.1\\tadd-a\\tchange\\tqueued\\t\\tAna\\tsummary\\t20\\n"\n' > "$F/infra/short.sh"
conf "sh infra/short.sh" ""
out=$(fq --tsv); rc=$?
if [ "$rc" -ne 0 ] && printf '%s' "$out" | grep -q 'needs 12'; then ok "a --tsv row with 11 columns breaks the contract"; else fail "an 11-column row passed (exit $rc): $out"; fi
printf '#!/bin/sh\nprintf "1\\t1\\t\\t1.1\\tadd-a\\tchange\\tqueued\\t\\tAna\\tsummary\\tforty\\t\\n"\n' > "$F/infra/budget.sh"
conf "sh infra/budget.sh" ""
fq --tsv | grep -q 'column 11 (budget)' && ok "a budget that is not dollars breaks the contract" || fail "a non-dollar budget passed the contract"

# ── Every consumer reads the queue through roadmap_queue (lib/conf.sh) ──────────────────────────
# bypass DIR: each line in DIR's scripts that runs the parser by path, reads the roadmap file
# itself, or matches its phase headings (a second parser, as currency.sh's was before OPS-38,
# whatever path it read the file by). Allowed: the helper, the parser, this check (its grammar tests run the parser),
# checks/panel-contract.sh (it holds the panel card's front-door call to the helper's output), and
# a cp that copies the parser into a scratch project.
# The roadmap path is caught however it is quoted or braced ($ROOT/$ROADMAP, $ROOT/"$ROADMAP",
# "${ROOT}"/"${ROADMAP}"), and as a bare "$ROADMAP" on a line that cd's to $ROOT. Not caught: a cd
# on one line and the read on a later one; a bare "$ROADMAP" is also a git path or a pattern
# elsewhere (hooks/lib/change-guard.sh), so it cannot be refused alone.
# Every allowance is a path in the case below, so allowing a reader is an edit to this guard, and
# the green line names each file it skipped. No marker in a scanned file allows anything.
# bypass DIR [SKIPS]: prints each bypassing line; appends each allowed file it skipped to SKIPS.
bypass() {
  find "$1" \( -name '*.sh' -o -name '*.js' \) -type f | sort | while read -r f; do
    case $f in
      */lib/conf.sh|*/roadmap-queue.sh|*/checks/roadmap.sh|*/checks/panel-contract.sh) echo "${f#"$1"/}" >> "${2:-/dev/null}"; continue ;;
      # Clauductor's roadmap tracker (OPS-83): it reads only the roadmap's `## Session plan`
      # section, which is not a queue table; its rows come from roadmap_queue.
      */local/roadmap-tracker.sh) echo "${f#"$1"/}" >> "${2:-/dev/null}"; continue ;;
      # ...and its check, which finds that section's line range and greps for a committed address.
      */local/checks/roadmap-tracker.sh) echo "${f#"$1"/}" >> "${2:-/dev/null}"; continue ;;
      # OPS-108: the daily close's fold writes each folded row's Status cell (no parser writes), at
      # the line roadmap_queue --tsv names, and re-parses the result with roadmap_queue --check;
      # review-cap.sh reads a row's Risk: from the line roadmap_queue names, because the --tsv
      # summary column may be shortened by a project's own parser.
      */skills/session-close/fold.sh|"$1"/review-cap.sh) echo "${f#"$1"/}" >> "${2:-/dev/null}"; continue ;;
    esac
    # OPS-125: the marker reads a row's session from the `## Session plan` through its one parser,
    # roadmap-plan.sh (no queue table); the row itself comes from roadmap_queue --tsv. Only that
    # call is allowed (OPS-126): the first such call on each line is cut before the scan, so a raw
    # queue read added to the marker, even on that same line, is still flagged.
    strip='s/^$//'
    # The plugin's build spells the call "$CLAUDUCTOR_FW/roadmap-plan.sh"; both forms are cut.
    case $f in */skills/session-start/marker.sh) strip='s#sh "\$(ROOT/\.claude|CLAUDUCTOR_FW)/roadmap-plan\.sh" "\$ROOT/\$ROADMAP"##' ;; esac
    sed -E "$strip" "$f" | grep -nE 'roadmap-queue\.sh|\$\{?ROOT\}?"?/"?\$\{?ROADMAP([^A-Z_]|$)|cd[[:space:]]+"?\$\{?ROOT\}?"?[[:space:]].*"\$\{?ROADMAP\}?"|\^#*## Phase' \
      | grep -vE '^[0-9]+:[[:space:]]*#|^[0-9]+:[[:space:]]*cp |ROADMAP_BUILTIN=1|cp "\$ROOT/\.claude/roadmap-queue\.sh"' \
      | sed "s|^|${f#"$1"/}:|"
  done
}
fw=$(cd "$(dirname "$0")/.." && pwd)
mkdir -p "$d/plant/skills/x"
printf '#!/bin/sh\ntsv=$(sh .claude/%s --tsv)\n' roadmap-queue.sh > "$d/plant/skills/x/context.sh"
printf '#!/bin/sh\nawk 1 "$ROOT/$ROADMAP"\n' > "$d/plant/second-parser.sh"
printf '#!/bin/sh\nread_src | jq -R -s %s\n' "'split(\"\\\\n\")[] | select(test(\"^## Phase \\\\\\\\d+\"))'" > "$d/plant/phase-reader.sh"
[ "$(bypass "$d/plant" | wc -l | tr -d ' ')" = 3 ] && ok "the bypass scan finds a script that runs the parser by path, one that reads the roadmap itself, and one that matches its phase headings" || fail "the bypass scan missed a planted bypass: $(bypass "$d/plant")"
# The other spellings of the roadmap's path; a comment on the line allows nothing, a path in the
# guard's own case does.
mkdir -p "$d/plant2/local"
printf '#!/bin/sh\nf=$ROOT/$ROADMAP\nawk 1 "$f"\n' > "$d/plant2/unquoted.sh"
printf '#!/bin/sh\nawk 1 $ROOT/"$ROADMAP"\n' > "$d/plant2/mixed.sh"
printf '#!/bin/sh\nawk 1 "${ROOT}"/"${ROADMAP}"\n' > "$d/plant2/braced.sh"
printf '#!/bin/sh\ncd "$ROOT" && awk 1 "$ROADMAP"\n' > "$d/plant2/cd-then-read.sh"
printf '#!/bin/sh\nf=$ROOT/$ROADMAP # roadmap-bypass-ok: reads only its own section\n' > "$d/plant2/marked.sh"
printf '#!/bin/sh\nf=$ROOT/$ROADMAP\n' > "$d/plant2/local/roadmap-tracker.sh"
printf '#!/bin/sh\nexport ROADMAP_PARSER; grep -vxF -e "$ROADMAP" list\n' > "$d/plant2/not-a-read.sh"
: > "$d/skips2"
p2=$(bypass "$d/plant2" "$d/skips2" | cut -d: -f1 | tr '\n' ' ')
[ "$p2" = "braced.sh cd-then-read.sh marked.sh mixed.sh unquoted.sh " ] \
  && ok "the bypass scan finds the roadmap's path unquoted, half-quoted, braced, after a cd and behind a comment marker; it passes a bare \"\$ROADMAP\" pattern" \
  || fail "the bypass scan flagged [$p2], want [braced.sh cd-then-read.sh marked.sh mixed.sh unquoted.sh ]"
mkdir -p "$d/plant3/skills/session-start"
printf '#!/bin/sh\nsn=$(sh "$CLAUDUCTOR_FW/roadmap-plan.sh" "$ROOT/$ROADMAP" 2>/dev/null)\nawk 1 "$ROOT/$ROADMAP"\nx=$(sh "$CLAUDUCTOR_FW/roadmap-plan.sh" "$ROOT/$ROADMAP"; awk 1 "$ROOT/$ROADMAP")\n' > "$d/plant3/skills/session-start/marker.sh"
p3=$(bypass "$d/plant3" | cut -d: -f1,2 | tr '\n' ' ')
[ "$p3" = "skills/session-start/marker.sh:3 skills/session-start/marker.sh:4 " ] && ok "the marker's exemption covers only its roadmap-plan.sh call: a raw read added to it is flagged, on its own line or the call's" || fail "the marker plant flagged [$p3], want [skills/session-start/marker.sh:3 skills/session-start/marker.sh:4 ]"
[ "$(cat "$d/skips2")" = "local/roadmap-tracker.sh" ] && ok "...and skips a path its own case allows, naming it" || fail "the allowlisted plant was not skipped and named: [$(cat "$d/skips2")]"
: > "$d/skips"
by=$(bypass "$fw" "$d/skips")
nskip=$(wc -l < "$d/skips" | tr -d ' ')
[ -z "$by" ] && ok "no consumer bypasses roadmap_queue (every reader follows ROADMAP_PARSER); $nskip allowed file(s) skipped: $(tr '\n' ' ' < "$d/skips")" || fail "these read the queue without roadmap_queue (lib/conf.sh), so ROADMAP_PARSER cannot reach them: $by"
nuse=$(grep -rlE --include='*.sh' '(^|[^_])roadmap_queue --' "$fw" | grep -vc '/lib/conf.sh$\|/checks/')
[ "$nuse" -ge 5 ] && ok "$nuse consumer script(s) call roadmap_queue (the scan saw the real consumers)" || fail "only $nuse script(s) call roadmap_queue; the bypass scan is reading the wrong directory ($fw)"

# ── The project's own roadmap, through whatever parser it configured ───────────────────────────
out=$(roadmap_queue --check "$ROOT/$ROADMAP"); rc=$?
expect_rc 0 "$rc" "the project's roadmap ($ROADMAP) parses: $(printf '%s' "$out" | head -1)"
[ "$rc" -eq 0 ] || printf '%s\n' "$out" | sed 's/^/     /'
out=$(roadmap_queue --tsv "$ROOT/$ROADMAP"); rc=$?
expect_rc 0 "$rc" "the project's roadmap meets the --tsv contract through ${ROADMAP_PARSER:-.claude/roadmap-queue.sh}"
[ "$rc" -eq 0 ] || printf '%s\n' "$out" | sed 's/^/     /'
finish
