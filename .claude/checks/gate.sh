#!/bin/sh
# The gate honours its receipt contract (scripts/ci/README.md): a full clean pass writes
# `<sha> full clean all`; a dirty tree's pass writes `dirty`; a --quick run neither writes nor
# deletes; a failed full run deletes. It holds the lease while the steps run and releases it after.
# gate.sh keeps its exit code and prints the markers and the log path. Runs in a throwaway repo,
# through lease.sh: clauductor is removed from PATH, so the check never starts the real binary.
# The operating model's own steps run in every gate: the scenario trace fails the gate on an uncited
# scenario, and the secret scan fails it on a finding, SKIPS (saying so) without gitleaks, and fails
# without gitleaks under CI. gitleaks here is a stub, so the check never depends on the real one.
. "$(dirname "$0")/lib.sh"
need git bash

d=$(scratch)
R="$d/repo"; new_repo "$R"
mkdir -p "$R/scripts/ci" "$R/.claude/lib" "$d/bin"
cp "$ROOT"/scripts/ci/run-local.sh "$ROOT"/scripts/ci/gate.sh "$ROOT"/scripts/ci/lease.sh "$R/scripts/ci/"
cp "$ROOT/.claude/lib/conf.sh" "$ROOT/.claude/lib/change.sh" "$R/.claude/lib/"
cp "$ROOT/.claude/scenario-trace.sh" "$R/.claude/"
# A gitleaks stub: finds a "secret" only while FAIL_SECRET exists in the tree it scans.
cat > "$d/bin/gitleaks" <<'EOF'
#!/bin/sh
case "$*" in *--help*) exit 0 ;; esac
for a in "$@"; do last=$a; done
[ -n "$(find "$last" -name FAIL_SECRET 2>/dev/null)" ] && { echo "leaks found: 1"; exit 1; }
exit 0
EOF
chmod +x "$d/bin/gitleaks"
cat > "$R/scripts/ci/steps.sh" <<'EOF'
gate_steps() {
  step "lease held" sh -c '[ -n "$CLAUDUCTOR_LOCK_HELD" ] && [ -d "$CLAUDUCTOR_LOCK_HELD" ]' || return 1
  step "unit" sh -c '[ ! -f FAIL_UNIT ]' || return 1
  [ "$1" = full ] || return 0
  step "slow" sh -c '[ ! -f FAIL_SLOW ]' || return 1
}
EOF
printf 'FAIL_UNIT\nFAIL_SLOW\nlocal/\n' > "$R/.gitignore"
git -C "$R" add -A && git -C "$R" commit -qm init
SHA=$(git -C "$R" rev-parse HEAD)
REC="$R/.git/ci-receipt"
LOCK="$R/.git/clauductor/gate.lock"

# PATH without any directory holding a clauductor or a gitleaks binary; the stub goes first.
np=""; IFS_OLD=$IFS; IFS=:
for p in $PATH; do [ -x "$p/clauductor" ] || [ -x "$p/gitleaks" ] && continue; np="$np${np:+:}$p"; done
IFS=$IFS_OLD
run() { (cd "$R" && PATH="$d/bin:$np" CI= CLAUDUCTOR_LANE=check bash scripts/ci/run-local.sh "$@") >"$d/out" 2>&1; }

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

# The operating model's own steps.
grep -q '^==> scenario trace' "$d/out" && grep -q '^==> secrets' "$d/out" && ok "every gate runs the scenario trace and the secret scan" || fail "the model's own steps did not run: $(grep '^==>' "$d/out" | tr '\n' ' ')"
printf 'secret\n' > "$R/FAIL_SECRET"; git -C "$R" add FAIL_SECRET && git -C "$R" commit -qm "a leak"
run; rc=$?
expect_rc 1 "$rc" "a secret the scan finds fails the full gate"
grep -q '^==> FAIL: secrets' "$d/out" && [ ! -f "$REC" ] && ok "...names the secrets step and writes no receipt" || fail "secret finding: $(grep '^==>' "$d/out" | tr '\n' ' '); receipt $(cat "$REC" 2>/dev/null || echo none)"
git -C "$R" rm -q FAIL_SECRET && git -C "$R" commit -qm "no leak"
mkdir -p "$R/local"; printf 'secret\n' > "$R/local/FAIL_SECRET"
run; rc=$?
expect_rc 0 "$rc" "a secret in an IGNORED file (a local .env) does not fail the gate: only tracked files are scanned"
rm -rf "$R/local"
nogl() { (cd "$R" && PATH="$np" CI="$1" CLAUDUCTOR_LANE=check bash scripts/ci/run-local.sh) >"$d/out" 2>&1; }
nogl ""; rc=$?
expect_rc 0 "$rc" "without gitleaks, locally, the gate passes"
grep -q 'secrets: SKIPPED — gitleaks is not installed' "$d/out" && ok "...and says the secret scan was SKIPPED, and why" || fail "no SKIPPED line: $(grep secrets "$d/out")"
nogl true; rc=$?
expect_rc 1 "$rc" "without gitleaks under CI=true, the gate fails"
[ ! -f "$REC" ] && ok "...and writes no receipt" || fail "a receipt was written without a secret scan under CI"
mkdir -p "$R/specs/cap"
printf '# Cap\n\n## Purpose\nx\n\n## Requirements\n\n### Requirement: R\nThe system SHALL r.\n\n#### Scenario: [CAP-1-S1] r\n- **THEN** r\n' > "$R/specs/cap/spec.md"
git -C "$R" add -A && git -C "$R" commit -qm "an uncited scenario"
run; rc=$?
expect_rc 1 "$rc" "an uncited scenario fails the gate"
grep -q '^==> FAIL: scenario trace' "$d/out" && ok "...at the scenario trace step" || fail "scenario trace: $(grep '^==>' "$d/out" | tr '\n' ' ')"
git -C "$R" rm -rq specs && git -C "$R" commit -qm "no spec"
run; SHA=$(git -C "$R" rev-parse HEAD)

out=$(cd "$R" && PATH="$d/bin:$np" CI= CLAUDUCTOR_LANE=check bash scripts/ci/gate.sh --quick 2>&1); rc=$?
expect_rc 0 "$rc" "gate.sh keeps run-local's exit code"
case "$out" in *"==> unit"*"full log"*) ok "gate.sh prints the step markers and the log path" ;; *) fail "gate.sh output: $out" ;; esac
touch "$R/FAIL_UNIT"
(cd "$R" && PATH="$d/bin:$np" CI= bash scripts/ci/gate.sh --quick >/dev/null 2>&1); rc=$?
expect_rc 1 "$rc" "gate.sh passes a failure's exit code through"
rm -f "$R/FAIL_UNIT"

# The vendored lease.sh must be the block clauductor documents (the framework's Go tests compare it
# with docs/panel.md; here: it still defines what run-local.sh calls).
grep -q '^lease_run() {' "$ROOT/scripts/ci/lease.sh" && ok "lease.sh defines lease_run" || fail "lease.sh has no lease_run"
finish
