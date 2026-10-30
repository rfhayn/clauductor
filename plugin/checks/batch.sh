#!/bin/sh
STRIDAL_FW=$(cd "$(dirname "$0")/.." && pwd) # stridal plugin: the plugin root (framework/internal/plugin)
# batch.sh: scripts/ci/batch.sh runs PRAC-0013's batch steps (OPS-176). `merge` merges the lane
# branches --no-ff in the order given and regenerates what BATCH_REGENERATE names; a conflict stops
# it, naming the branch, with the earlier merges kept and no merge left half done. `sync` merges
# origin/<main> right before the gate, and a conflict there is caught before the gate, not by GitHub.
# `record` writes the batch PR into each row's fragment and names a row with none under the first
# fragment's **Also closes**, so the fold marks every row merged by the batch PR.
# Also: the playbook's batch steps name the script (the caller, rule 3).
. "$(dirname "$0")/lib.sh"

S=$(scratch)
# Beside GATE_RUN, where install puts it (B10: never a typed scripts/ci path).
B="$ROOT/$(gate_file_path batch.sh)"
g() { git -C "$F" "$@"; }
setup() {  # an origin, a clone on a batch branch, two lane branches, one that conflicts, one fragment
  rm -rf "$S/o" "$S/w"; F="$S/w"
  git init -q --bare -b main "$S/o"
  git clone -q "$S/o" "$F" 2>/dev/null
  g config user.name "Ann Example"; g config user.email a@example.com
  mkdir -p "$F/.claude/lib" "$F/docs/journal.d"
  cp "$STRIDAL_FW/lib/conf.sh" "$STRIDAL_FW/lib/fragments.sh" "$F/.claude/lib/"
  printf 'BATCH_REGENERATE="sh gen.sh"\n' > "$F/.claude/project.conf"
  printf 'cat a b 2>/dev/null > gen.out; exit 0\n' > "$F/gen.sh"
  printf 'base\n' > "$F/shared"
  g add -A; g commit -qm init; g push -q origin main 2>/dev/null
  for x in a b; do g checkout -qb "ops/$x" main; printf '%s\n' "$x" > "$F/$x"; [ "$x" = b ] || printf 'a\n' > "$F/shared"; g add -A; g commit -qm "$x"; done
  g checkout -qb ops/c main; printf 'c\n' > "$F/shared"; g add -A; g commit -qm c
  g checkout -qb ops/batch main
}
frag() {  # frag ID PR [Also closes]
  { printf '# %s — the work\n\n- **PR**: #%s\n- **Session**: S5, started 2026-01-02 09:00 ET\n' "$1" "$2"
    printf -- '- **Review**: 1 round, peak low, cap 3\n- **Gate**: 7 min\n- **Cost**: $1\n- **Session start**: 1 tokens, 1 min\n'
    [ -z "${3:-}" ] || printf -- '- **Also closes**: %s\n' "$3"
    printf '\n## What happened\n\nX.\n\n## Decisions\n\nX.\n\n## Surprises\n\nNone.\n\n## Insights\n\nNone.\n'
  } > "$F/docs/journal.d/$1.md"
}
run() { (cd "$F" && sh "$B" "$@" 2>&1); }

setup
out=$(run merge ops/a ops/b); rc=$?
[ $rc -eq 0 ] && [ "$(g log --merges --format=%s main..HEAD | sed -n '2p;1p' | tr '\n' ' ')" = "Merge branch 'ops/b' into ops/batch Merge branch 'ops/a' into ops/batch " ] \
  && ok "batch merge: the lanes merge --no-ff, in the order given" || fail "batch merge: exit $rc: $out; $(g log --format=%s main..HEAD | tr '\n' '~')"
[ "$(cat "$F/gen.out" 2>/dev/null | tr '\n' ' ')" = "a b " ] && [ -z "$(g status --porcelain)" ] \
  && g log -1 --format=%s | grep -q 'Regenerate' \
  && ok "batch merge: BATCH_REGENERATE runs after the merges and its output is committed" \
  || fail "batch merge: regenerate: $(cat "$F/gen.out" 2>/dev/null); status: $(g status --porcelain)"

setup
out=$(run merge ops/a ops/c ops/b); rc=$?
[ $rc -ne 0 ] && printf '%s' "$out" | grep -q 'ops/c' && [ "$(g log --merges --format=%s main..HEAD | wc -l | tr -d ' ')" = 1 ] \
  && [ ! -f "$F/.git/MERGE_HEAD" ] && [ -z "$(g status --porcelain)" ] && [ ! -f "$F/b" ] \
  && ok "batch merge: a conflict stops it naming the branch, the earlier merge kept, none half done" \
  || fail "batch merge: conflict: exit $rc: $out; status: $(g status --porcelain)"

setup; printf 'dirty\n' >> "$F/shared"
out=$(run merge ops/a); [ $? -ne 0 ] && printf '%s' "$out" | grep -q 'clean' && [ -z "$(g log --merges --format=%s main..HEAD)" ] \
  && ok "batch merge: a dirty tree is refused, nothing merged" || fail "batch merge: dirty tree: $out"

setup; out=$(cd "$F" && git checkout -q main && sh "$B" merge ops/a 2>&1); rc=$?
[ $rc -ne 0 ] && printf '%s' "$out" | grep -q 'integration branch' && [ -z "$(g log --merges --format=%s)" ] \
  && ok "batch: on <main> it refuses, nothing merged" || fail "batch: on main: exit $rc: $out"

# sync: main moves from ANOTHER clone, so only the script's own fetch can see it.
moved() {  # moved FILE: a commit on origin/main this clone has not fetched
  rm -rf "$S/o2"; git clone -q "$S/o" "$S/o2" 2>/dev/null
  printf 'm\n' > "$S/o2/$1"; git -C "$S/o2" add -A
  git -C "$S/o2" -c user.name=M -c user.email=m@example.com commit -qm m; git -C "$S/o2" push -q origin main 2>/dev/null
}
setup; run merge ops/a >/dev/null; moved a
out=$(run sync); rc=$?
[ $rc -ne 0 ] && printf '%s' "$out" | grep -q "origin/main" && [ ! -f "$F/.git/MERGE_HEAD" ] && [ -z "$(g status --porcelain)" ] \
  && ok "batch sync: a conflict with origin/main is caught before the gate, the merge aborted" || fail "batch sync: conflict: exit $rc: $out"
setup; run merge ops/a >/dev/null; moved m
out=$(run sync); rc=$?
[ $rc -eq 0 ] && [ -f "$F/m" ] && g merge-base --is-ancestor origin/main HEAD \
  && ok "batch sync: fetches origin/main and merges it" || fail "batch sync: exit $rc: $out"

# record: the batch PR into every fragment; a row with none joins the first one's Also closes.
setup; frag A-1 11 'X-8'; frag A-3 13 'X-9'; g add -A; g commit -qm frags
out=$(run record 40 A-1 A-2 A-3); rc=$?
[ $rc -eq 0 ] && grep -qx -- '- \*\*PR\*\*: #40' "$F/docs/journal.d/A-1.md" && grep -qx -- '- \*\*PR\*\*: #40' "$F/docs/journal.d/A-3.md" \
  && grep -qx -- '- \*\*Also closes\*\*: X-8, A-2' "$F/docs/journal.d/A-1.md" && grep -qx -- '- \*\*Also closes\*\*: X-9' "$F/docs/journal.d/A-3.md" \
  && [ -z "$(g status --porcelain)" ] && g log -1 --format=%s | grep -q '^A-1: Record batch PR #40' \
  && ok "batch record: each fragment names the batch PR, a row with none joins the first's Also closes, committed" \
  || fail "batch record: exit $rc: $out; $(head -9 "$F/docs/journal.d/A-1.md" | tr '\n' '~')"
[ $rc -eq 0 ] && (ROOT=$F; . "$F/.claude/lib/conf.sh"; . "$F/.claude/lib/fragments.sh"
 for f in "$F"/docs/journal.d/*.md; do fragment_problems "$f" >/dev/null || exit 1; done) \
  && ok "batch record: the rewritten fragments keep the shape the fold reads" || fail "batch record: a fragment lost its shape"
head=$(g rev-parse HEAD); out=$(run record 40 A-1 A-2 A-3 X-9); rc=$?
[ $rc -eq 0 ] && [ "$(g rev-parse HEAD)" = "$head" ] && grep -qx -- '- \*\*Also closes\*\*: X-8, A-2' "$F/docs/journal.d/A-1.md" \
  && ok "batch record: a rerun (a row another fragment closes included) changes nothing" \
  || fail "batch record: rerun: exit $rc: $out; $(grep Also "$F"/docs/journal.d/*.md)"
# A row that leaves the batch after two record runs: a run with the rest takes it out again.
run record 41 A-1 A-2 A-3 >/dev/null; out=$(run record 41 A-1 A-3); rc=$?
[ $rc -eq 0 ] && grep -qx -- '- \*\*Also closes\*\*: X-8' "$F/docs/journal.d/A-1.md" && grep -qx -- '- \*\*Also closes\*\*: X-9' "$F/docs/journal.d/A-3.md" \
  && ok "batch record: a row that left the batch comes out of Also closes, the authors' rows stay" \
  || fail "batch record: a row that left: exit $rc: $out; $(grep Also "$F"/docs/journal.d/*.md | tr '\n' '~')"

# A fragment that fails the shape stops record, and the fragments rewritten before it are restored.
setup; frag A-1 11; frag A-3 13; grep -v 'Gate' "$F/docs/journal.d/A-3.md" > "$S/a3"; cat "$S/a3" > "$F/docs/journal.d/A-3.md"
g add -A; g commit -qm frags
out=$(run record 50 A-1 A-3); rc=$?
[ $rc -eq 1 ] && printf '%s' "$out" | grep -q 'lost its shape' && [ -z "$(g status --porcelain)" ] && grep -qx -- '- \*\*PR\*\*: #11' "$F/docs/journal.d/A-1.md" \
  && ok "batch record: a stop on a malformed fragment leaves every fragment as it was" || fail "batch record: undo: exit $rc: $out; $(g status --porcelain)"

# Where the check finds batch.sh is where install puts it (conf.go's Mapping).
[ "$(GATE_RUN=tools/ci/run-local.sh gate_file_path batch.sh)" = tools/ci/batch.sh ] \
  && [ "$(GATE_RUN=tools/gate.sh gate_file_path batch.sh)" = scripts/ci/batch.sh ] \
  && [ "$(GATE_RUN=run-local.sh gate_file_path batch.sh)" = ./batch.sh ] \
  && ok "batch: found beside a moved run-local.sh, at scripts/ci beside a project's own runner" || fail "batch: gate_file_path does not follow install"

out=$(run record 41 A-2); [ $? -ne 0 ] && printf '%s' "$out" | grep -q 'no fragment' \
  && ok "batch record: no row with a fragment is refused" || fail "batch record: no fragment: $out"
out=$(run record x A-1); [ $? -ne 0 ] && printf '%s' "$out" | grep -q 'usage' \
  && ok "batch record: a PR that is not a number is refused" || fail "batch record: bad PR: $out"
out=$(run record 42 A-1 'OPS 9'); rc=$?
[ $rc -eq 2 ] && [ -z "$(g status --porcelain)" ] \
  && ok "batch record: a row id that is not one is refused before any write" || fail "batch record: bad id: exit $rc: $out; $(g status --porcelain)"

# The caller: a script no step runs is not shipped (rule 3). The playbook names it at scripts/ci,
# as it names gate.sh, even where install moved both beside GATE_RUN.
grep -q 'batch\.sh merge' "$ROOT/docs/playbook.md" 2>/dev/null \
  && ok "the playbook's batch steps run batch.sh" || fail "docs/playbook.md does not run batch.sh merge"
finish
