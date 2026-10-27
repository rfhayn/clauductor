#!/bin/sh
# The roadmap's history rule (OPS-147): roadmap.d/history.sh refuses a roadmap row whose id
# docs/roadmap-history.md holds, reads only the history's queue tables (never its session plan),
# and lets a project with no history file through. Run on a scratch project, the rule fed --tsv
# rows by hand, as roadmap_queue feeds it.
. "${CHECKS_LIB:?}"

F="$(scratch)/proj"
mkdir -p "$F/docs"
rule="$ROOT/.claude/local/roadmap.d/history.sh"
tab=$(printf '\t')
rows() { for id in "$@"; do printf '7%sPhase 1%s%s%s\n' "$tab$tab" "$tab" "$id" "$tab"; done; }
run() { rows "$@" | ROOT="$F" sh "$rule"; }

rows OPS-1 | ROOT="$F" sh "$rule" >/dev/null 2>&1 \
  && ok "no history file: every id passes" || fail "the rule refused a project with no docs/roadmap-history.md"

cat > "$F/docs/roadmap-history.md" <<'EOF'
# Roadmap history

## Phase 1 — Old

| # | Change | Scope | Deps | Status |
|---|--------|-------|------|--------|
| OPS-8 | `ops/ops-8-x` — merged | x | — | ✅ merged (#27) |
| o.1 | `ops/check-outcome-x` — cancelled | x | — | ❌ cancelled — no |

## Session plan

| Session | When | Focus | Rows | The owner decides |
|---|---|---|---|---|
| S4 | (done) | x | OPS-8 | — |
EOF

out=$(run OPS-9 o.2 S4 2>&1) && ok "ids the history does not hold pass (a session name in its plan is not an id)" \
  || fail "the rule refused ids the history does not hold: $out"
out=$(run OPS-9 OPS-8 2>&1); rc=$?
[ "$rc" -ne 0 ] && printf '%s' "$out" | grep -q "OPS-8 is an id docs/roadmap-history.md already holds" \
  && ok "a merged id the history holds is refused, by name" || fail "OPS-8 reused passed (exit $rc): $out"
out=$(run o.1 2>&1) && fail "a cancelled outcome id the history holds passed" \
  || ok "a cancelled id the history holds is refused too"
out=$(run '#' 2>&1) && ok "the table header's # is not an id" || fail "the header cell read as an id: $out"
# This repository's own roadmap against its history is the roadmap check's: roadmap_queue runs the
# rule on every read, so a reused id fails `roadmap-queue.sh --check` and every reader.
finish
