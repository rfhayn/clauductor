#!/bin/sh
# The roadmap queue parser (.claude/roadmap-queue.sh) accepts the grammar docs/roadmap.md states,
# REFUSES every shape it cannot classify (so no row can vanish silently), derives the current
# phase, and inherits owners section-over-phase. Then: the project's own roadmap parses.
. "$(dirname "$0")/lib.sh"

P="$ROOT/.claude/roadmap-queue.sh"
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

bad "a near-miss owner line written as a gate label" '**Gate 2U owner:** Ben'
printf '## Phase 1 — x\n**Stage 1 — the dry run. Owner: #116. ✅ MET.**\n' > "$d/prose.md"
rq --check "$d/prose.md" >/dev/null 2>&1 && ok "bold prose that mentions an owner is not a near-miss owner line" || fail "bold prose naming an owner was refused as an owner line: $(rq --check "$d/prose.md")"

# ── ROADMAP_PARSER: a project's own parser, plugged in through the contract ──────────────────────
# A project shaped like one whose roadmap has its own grammar (phases with gates, owner lines per
# gate, status words of its own): the template's parser refuses it; the project's parser, named
# by ROADMAP_PARSER, reads it, and every consumer follows the key (the panel's suggestions here).
need jq
F="$d/own"; mkdir -p "$F/.claude/lib" "$F/docs" "$F/changes" "$F/infra"
cp "$ROOT/.claude/lib/conf.sh" "$F/.claude/lib/"
cp "$ROOT/.claude/roadmap-queue.sh" "$ROOT/.claude/panel-suggest.sh" "$F/.claude/"
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
# bypass DIR: each line in DIR's scripts that runs the parser by path or reads the roadmap file
# itself. Allowed: the helper, the parser, this check (its grammar tests run the parser),
# checks/panel-contract.sh (it holds the panel card's front-door call to the helper's output), and
# a cp that copies the parser into a scratch project.
bypass() {
  find "$1" \( -name '*.sh' -o -name '*.js' \) -type f | sort | while read -r f; do
    case $f in */lib/conf.sh|*/roadmap-queue.sh|*/checks/roadmap.sh|*/checks/panel-contract.sh) continue ;; esac
    grep -nE 'roadmap-queue\.sh|"\$ROOT/\$ROADMAP"|\$\{ROOT\}/\$\{?ROADMAP' "$f" \
      | grep -vE '^[0-9]+:[[:space:]]*#|^[0-9]+:[[:space:]]*cp |ROADMAP_BUILTIN=1|cp "\$ROOT/\.claude/roadmap-queue\.sh"' \
      | sed "s|^|${f#"$1"/}:|"
  done
}
fw=$(cd "$(dirname "$0")/.." && pwd)
mkdir -p "$d/plant/skills/x"
printf '#!/bin/sh\ntsv=$(sh .claude/%s --tsv)\n' roadmap-queue.sh > "$d/plant/skills/x/context.sh"
printf '#!/bin/sh\nawk 1 "$ROOT/$ROADMAP"\n' > "$d/plant/second-parser.sh"
[ "$(bypass "$d/plant" | wc -l | tr -d ' ')" = 2 ] && ok "the bypass scan finds a script that runs the parser by path, and one that reads the roadmap itself" || fail "the bypass scan missed a planted bypass: $(bypass "$d/plant")"
by=$(bypass "$fw")
[ -z "$by" ] && ok "no consumer bypasses roadmap_queue (every reader follows ROADMAP_PARSER)" || fail "these read the queue without roadmap_queue (lib/conf.sh), so ROADMAP_PARSER cannot reach them: $by"
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
