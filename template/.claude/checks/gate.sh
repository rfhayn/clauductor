#!/bin/sh
# The gate honours its receipt contract (scripts/ci/README.md): a full clean pass writes
# `<sha> full clean all`; a dirty tree's pass writes `dirty`; a --quick run neither writes nor
# deletes; a failed full run deletes. It holds the lease while the steps run and releases it after.
# gate.sh keeps its exit code and prints the markers and the log path. Runs in a throwaway repo,
# through lease.sh: clauductor is removed from PATH, so the check never starts the real binary.
. "$(dirname "$0")/lib.sh"
need git bash

d=$(scratch)
R="$d/repo"; new_repo "$R"
mkdir -p "$R/scripts/ci" "$R/.claude/lib"
cp "$ROOT"/scripts/ci/run-local.sh "$ROOT"/scripts/ci/gate.sh "$ROOT"/scripts/ci/lease.sh "$R/scripts/ci/"
cp "$ROOT/.claude/lib/conf.sh" "$R/.claude/lib/"
cat > "$R/scripts/ci/steps.sh" <<'EOF'
gate_steps() {
  step "lease held" sh -c '[ -n "$CLAUDUCTOR_LOCK_HELD" ] && [ -d "$CLAUDUCTOR_LOCK_HELD" ]' || return 1
  step "unit" sh -c '[ ! -f FAIL_UNIT ]' || return 1
  [ "$1" = full ] || return 0
  step "slow" sh -c '[ ! -f FAIL_SLOW ]' || return 1
}
EOF
echo 'FAIL_*' > "$R/.gitignore"
git -C "$R" add -A && git -C "$R" commit -qm init
SHA=$(git -C "$R" rev-parse HEAD)
REC="$R/.git/ci-receipt"
LOCK="$R/.git/clauductor/gate.lock"

# PATH without any directory holding a clauductor binary.
np=""; IFS_OLD=$IFS; IFS=:
for p in $PATH; do [ -x "$p/clauductor" ] && continue; np="$np${np:+:}$p"; done
IFS=$IFS_OLD
run() { (cd "$R" && PATH="$np" CLAUDUCTOR_LANE=check bash scripts/ci/run-local.sh "$@") >"$d/out" 2>&1; }

run; rc=$?
expect_rc 0 "$rc" "a full clean run passes (steps saw the lease held)"
[ "$(cat "$REC" 2>/dev/null)" = "$(printf '%s\tfull\tclean\tall' "$SHA")" ] && ok "it wrote '<sha> full clean all'" || fail "receipt after a clean run: $(cat "$REC" 2>/dev/null || echo none)"
[ ! -e "$LOCK" ] && ok "the lease is released after the run" || fail "the lease directory is left behind: $LOCK"

touch "$R/FAIL_SLOW"; run --quick; rc=$?
expect_rc 0 "$rc" "--quick skips the full-only steps"
[ "$(cut -f1 "$REC" 2>/dev/null)" = "$SHA" ] && ok "--quick leaves the full receipt alone" || fail "--quick touched the receipt"
touch "$R/FAIL_UNIT"; run --quick; rm -f "$R/FAIL_UNIT"
[ -f "$REC" ] && ok "a failed --quick run does not delete the full receipt" || fail "a failed --quick run deleted the receipt"

run; rc=$?
expect_rc 1 "$rc" "a failing full run fails"
[ ! -f "$REC" ] && ok "a failed full run deletes the receipt" || fail "a failed full run left a receipt"
grep -q '^==> FAIL: slow' "$d/out" && ok "it names the failed step" || fail "no '==> FAIL: slow' line"
rm -f "$R/FAIL_SLOW"

echo "change" >> "$R/.gitignore"
run; rc=$?
expect_rc 0 "$rc" "a run over an uncommitted change passes"
[ "$(cut -f3 "$REC" 2>/dev/null)" = dirty ] && ok "...and its receipt says dirty, which the guard refuses" || fail "an uncommitted tree got receipt state '$(cut -f3 "$REC" 2>/dev/null)'"
git -C "$R" checkout -q -- .gitignore

out=$(cd "$R" && PATH="$np" CLAUDUCTOR_LANE=check bash scripts/ci/gate.sh --quick 2>&1); rc=$?
expect_rc 0 "$rc" "gate.sh keeps run-local's exit code"
case "$out" in *"==> unit"*"full log"*) ok "gate.sh prints the step markers and the log path" ;; *) fail "gate.sh output: $out" ;; esac
touch "$R/FAIL_UNIT"
(cd "$R" && PATH="$np" bash scripts/ci/gate.sh --quick >/dev/null 2>&1); rc=$?
expect_rc 1 "$rc" "gate.sh passes a failure's exit code through"
rm -f "$R/FAIL_UNIT"

# The vendored lease.sh must be the block clauductor documents (the framework's Go tests compare it
# with docs/panel.md; here: it still defines what run-local.sh calls).
grep -q '^lease_run() {' "$ROOT/scripts/ci/lease.sh" && ok "lease.sh defines lease_run" || fail "lease.sh has no lease_run"
finish
