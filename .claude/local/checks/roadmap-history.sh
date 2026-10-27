#!/bin/sh
# The roadmap's history rule (OPS-147): roadmap.d/history.sh refuses a roadmap row whose id
# docs/roadmap-history.md holds for another change id, passes that row itself read from a roadmap
# that predates its move (the same change id), reads the history's queue tables before AND after
# its session plan but never the plan, and lets a project with no history file through. `--ids`
# lists the history's ids for the code that hands ids out. Run on a scratch project, the rule fed
# --tsv rows by hand, as roadmap_queue feeds it.
. "${CHECKS_LIB:?}"

F="$(scratch)/proj"
mkdir -p "$F/docs"
rule="$ROOT/.claude/local/roadmap.d/history.sh"
tab=$(printf '\t')
# run ID[:CHANGE]...: the rule over one --tsv row per argument (column 4 the id, 5 the change id).
run() {
  for a in "$@"; do
    id=${a%%:*}; cid=""; case $a in *:*) cid=${a#*:} ;; esac
    printf '7%sPhase 1%s%s%s%s%s\n' "$tab$tab" "$tab" "$id" "$tab" "$cid" "$tab"
  done | ROOT="$F" sh "$rule"
}

out=$(run OPS-1 2>&1) && ok "no history file: every id passes" || fail "the rule refused a project with no docs/roadmap-history.md: $out"

cat > "$F/docs/roadmap-history.md" <<'EOF'
# Roadmap history

## Phase 1 — Old

| # | Change | Scope | Deps | Status |
|---|--------|-------|------|--------|
| OPS-8 | `ops/ops-8-x` — merged | x | — | ✅ merged (#27) |

## Session plan

| Session | When | Focus | Rows | The owner decides |
|---|---|---|---|---|
| S4 | (done) | x | OPS-8 | — |

## Outcome checks

| # | Change | Scope | Deps | Status |
|---|--------|-------|------|--------|
| o.1 | `ops/check-outcome-x` — cancelled | x | — | ❌ cancelled — no |
EOF

out=$(run OPS-9:ops/ops-9-y o.2:ops/check-outcome-z S4:ops/s4 2>&1) \
  && ok "ids the history does not hold pass (a session name in its plan is not an id)" \
  || fail "the rule refused ids the history does not hold: $out"
out=$(run OPS-9:ops/ops-9-y OPS-8:ops/ops-8-new 2>&1); rc=$?
[ "$rc" -ne 0 ] && printf '%s' "$out" | grep -q "OPS-8 is an id docs/roadmap-history.md already holds" \
  && ok "an id the history holds, on another change, is refused by name" || fail "OPS-8 reused passed (exit $rc): $out"
out=$(run o.1:ops/check-outcome-new 2>&1) && fail "an id in a table AFTER the history's session plan passed" \
  || ok "an id in a table after the history's session plan is refused too (the plan's skip ends at the next ##)"
out=$(run OPS-8:ops/ops-8-x o.1:ops/check-outcome-x 2>&1) \
  && ok "the moved rows themselves pass (a roadmap from before the move: same id, same change id)" \
  || fail "a pre-move roadmap's own rows were refused: $out"
out=$(run '#' 2>&1) && ok "the table header's # is not an id" || fail "the header cell read as an id: $out"
ids=$(ROOT="$F" sh "$rule" --ids | tr '\n' ' ')
[ "$ids" = "OPS-8 o.1 " ] && ok "--ids lists the history's ids, before and after its plan, and no session" \
  || fail "--ids printed [$ids], want [OPS-8 o.1 ]"
# This repository's own roadmap against its history is the roadmap check's: roadmap_queue runs the
# rule on every read, so a reused id fails `roadmap-queue.sh --check` and every reader.
finish
