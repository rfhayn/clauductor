#!/bin/sh
# batch.sh: scripts/ci/batch.sh runs PRAC-0013's batch steps (OPS-176). `merge` merges the lane
# branches --no-ff in the order given and regenerates what BATCH_REGENERATE names; a conflict stops
# it, naming the branch, with the earlier merges kept and no merge left half done. `sync` merges
# origin/<main> right before the gate, and a conflict there is caught before the gate, not by GitHub.
# `record` writes the batch PR into each row's fragment and names a row with none under the first
# fragment's **Also closes**, so the fold marks every row merged by the batch PR.
# Also: the playbook's batch steps name the script (the caller, rule 3).
. "$(dirname "$0")/lib.sh"

S=$(scratch)
B="$ROOT/scripts/ci/batch.sh"
g() { git -C "$F" "$@"; }
setup() {  # an origin, a clone on a batch branch, two lane branches, one that conflicts, one fragment
  rm -rf "$S/o" "$S/w"; F="$S/w"
  git init -q --bare -b main "$S/o"
  git clone -q "$S/o" "$F" 2>/dev/null
  g config user.name "Ann Example"; g config user.email a@example.com
  mkdir -p "$F/.claude/lib" "$F/docs/journal.d"
  cp "$ROOT/.claude/lib/conf.sh" "$ROOT/.claude/lib/fragments.sh" "$F/.claude/lib/"
  printf 'BATCH_REGENERATE="sh gen.sh"\n' > "$F/.claude/project.conf"
  printf 'cat a b 2>/dev/null > gen.out\n' > "$F/gen.sh"
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

# sync: origin/main moved with a commit that conflicts with the batch: caught here, before the gate.
setup; run merge ops/a >/dev/null
g checkout -q main; printf 'm\n' > "$F/a"; g add -A; g commit -qm m; g push -q origin main 2>/dev/null; g checkout -q ops/batch
out=$(run sync); rc=$?
[ $rc -ne 0 ] && printf '%s' "$out" | grep -q "origin/main" && [ ! -f "$F/.git/MERGE_HEAD" ] && [ -z "$(g status --porcelain)" ] \
  && ok "batch sync: a conflict with origin/main is caught before the gate, the merge aborted" || fail "batch sync: conflict: exit $rc: $out"
setup; run merge ops/a >/dev/null
g checkout -q main; printf 'm\n' > "$F/m"; g add -A; g commit -qm m; g push -q origin main 2>/dev/null; g reset -q --hard HEAD~1; g checkout -q ops/batch
out=$(run sync); rc=$?
[ $rc -eq 0 ] && g merge-base --is-ancestor origin/main HEAD \
  && ok "batch sync: fetches and merges origin/main" || fail "batch sync: exit $rc: $out"

# record: the batch PR into every fragment; a row with none goes under the first one's Also closes.
setup; frag A-1 11; frag A-3 13 'X-9'; g add -A; g commit -qm frags
out=$(run record 40 A-1 A-2 A-3); rc=$?
[ $rc -eq 0 ] && grep -qx -- '- \*\*PR\*\*: #40' "$F/docs/journal.d/A-1.md" && grep -qx -- '- \*\*PR\*\*: #40' "$F/docs/journal.d/A-3.md" \
  && grep -qx -- '- \*\*Also closes\*\*: A-2' "$F/docs/journal.d/A-1.md" && grep -qx -- '- \*\*Also closes\*\*: X-9' "$F/docs/journal.d/A-3.md" \
  && [ -z "$(g status --porcelain)" ] && g log -1 --format=%s | grep -q '^A-1: Record batch PR #40' \
  && ok "batch record: each fragment names the batch PR, a row with none is under Also closes, committed" \
  || fail "batch record: exit $rc: $out; $(cat "$F/docs/journal.d/A-1.md" | head -9 | tr '\n' '~')"
(ROOT=$F; . "$F/.claude/lib/conf.sh"; . "$F/.claude/lib/fragments.sh"
 for f in "$F"/docs/journal.d/*.md; do fragment_problems "$f" >/dev/null || exit 1; done) \
  && ok "batch record: the fragments keep the shape the fold reads" || fail "batch record: a fragment lost its shape"
out=$(run record 41 A-2); [ $? -ne 0 ] && printf '%s' "$out" | grep -q 'no fragment' \
  && ok "batch record: no row with a fragment is refused" || fail "batch record: no fragment: $out"
out=$(run record x A-1); [ $? -ne 0 ] && printf '%s' "$out" | grep -q 'usage' \
  && ok "batch record: a PR that is not a number is refused" || fail "batch record: bad PR: $out"

# The caller: a script no step runs is not shipped (rule 3).
grep -q 'scripts/ci/batch\.sh' "$ROOT/docs/playbook.md" 2>/dev/null \
  && ok "the playbook's batch steps run scripts/ci/batch.sh" || fail "docs/playbook.md does not name scripts/ci/batch.sh"
finish
