#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# The gate honours its receipt contract (scripts/ci/README.md): a full clean pass writes
# `<sha> full clean all`; a dirty tree's pass writes `dirty`; a --quick run neither writes nor
# deletes; a failed full run deletes. It holds the lease while the steps run and releases it after.
# gate.sh keeps its exit code and prints the markers and the log path. Runs in a throwaway repo,
# through lease.sh: clauductor is removed from PATH, so the check never starts the real binary.
# The operating model's own steps run in every gate: the scenario trace fails the gate on an uncited
# scenario, and the secret scan fails it on a finding, SKIPS (saying so) without gitleaks, and fails
# without gitleaks under CI. gitleaks here is a stub, so the check never depends on the real one.
#
# The files are found where .claude/project.conf puts them (GATE_RUN, GATE, GATE_STEPS; lease.sh
# and lib/steps.sh beside GATE_RUN), never at a typed scripts/ci path (B10). The model's steps
# library is tested on its own, as a project's OWN runner calls it (P1.6); when GATE_RUN is such a
# runner (not the model's run-local.sh), this checks that it calls the library, and stops there.
# The lease (B15): the holder writes a real ttl and renews it while the command runs, the command
# can verify it still holds, and a gate that lost its lease writes no receipt.
. "$(dirname "$0")/lib.sh"
need git bash

RUN_REL=$GATE_RUN WRAP_REL=$GATE STEPS_REL=$GATE_STEPS
RUN_DIR=$(dirname "$RUN_REL")
LIB="$ROOT/$RUN_DIR/lib/steps.sh"
[ -f "$LIB" ] || LIB="$ROOT/scripts/ci/lib/steps.sh"

d=$(scratch)
mkdir -p "$d/bin"
# A gitleaks stub: finds a "secret" only while FAIL_SECRET exists in the tree it scans.
cat > "$d/bin/gitleaks" <<'EOF'
#!/bin/sh
case "$*" in *--help*) exit 0 ;; esac
for a in "$@"; do last=$a; done
[ -n "$(find "$last" -name FAIL_SECRET 2>/dev/null)" ] && { echo "leaks found: 1"; exit 1; }
exit 0
EOF
chmod +x "$d/bin/gitleaks"
# PATH without any directory holding a clauductor or a gitleaks binary; the stub goes first.
np=""; IFS_OLD=$IFS; IFS=:
for p in $PATH; do [ -x "$p/clauductor" ] || [ -x "$p/gitleaks" ] && continue; np="$np${np:+:}$p"; done
IFS=$IFS_OLD
uncited() {  # uncited REPO: commit a living spec whose one scenario no test cites
  mkdir -p "$1/specs/cap"
  printf '# Cap\n\n## Purpose\nx\n\n## Requirements\n\n### Requirement: R\nThe system SHALL r.\n\n#### Scenario: [CAP-1-S1] r\n- **THEN** r\n' > "$1/specs/cap/spec.md"
  git -C "$1" add -A && git -C "$1" commit -qm "an uncited scenario"
}

# ── The model's steps as a library, called by a project's own runner ─────────────────────
if [ ! -f "$LIB" ]; then
  fail "the model's gate steps library is missing ($RUN_DIR/lib/steps.sh, or scripts/ci/lib/steps.sh)"
else
  L="$d/own"; new_repo "$L"
  mkdir -p "$L/infra/lib" "$L/.claude/lib"
  cp "$LIB" "$L/infra/lib/steps.sh"
  cp "$CLAUDUCTOR_FW/lib/conf.sh" "$CLAUDUCTOR_FW/lib/change.sh" "$L/.claude/lib/"
  cp "$CLAUDUCTOR_FW/scenario-trace.sh" "$L/.claude/"
  cat > "$L/infra/mine.sh" <<'EOF'
#!/bin/sh
# A project's own runner: its own step function, then the model's steps from the library.
ROOT=$(git rev-parse --show-toplevel); cd "$ROOT" || exit 1
. ./.claude/lib/conf.sh
step() { n=$1; shift; echo "==> $n (mine)"; "$@" || { echo "==> FAIL: $n"; return 1; }; }
. infra/lib/steps.sh
model_steps && echo "own runner: PASS"
EOF
  git -C "$L" add -A && git -C "$L" commit -qm init
  own() { (cd "$L" && PATH="$d/bin:$np" CI= sh infra/mine.sh) >"$d/own.out" 2>&1; }
  own; rc=$?
  expect_rc 0 "$rc" "a project's own runner runs the model's steps from the library"
  grep -q '^==> scenario trace (mine)' "$d/own.out" && grep -q '^==> secrets (mine)' "$d/own.out" \
    && ok "...both steps, through the runner's own step function" || fail "own runner output: $(tr '\n' ' ' < "$d/own.out")"
  uncited "$L"
  own; rc=$?
  expect_rc 1 "$rc" "...and an uncited scenario fails it at the scenario trace"
  git -C "$L" rm -rq specs && git -C "$L" commit -qm "no spec"
  printf 'secret\n' > "$L/FAIL_SECRET"; git -C "$L" add FAIL_SECRET && git -C "$L" commit -qm "a leak"
  own; rc=$?
  expect_rc 1 "$rc" "...and a secret fails it at the secrets step"
fi

# ── The lease conformance kit (P1.12): shipped as clauductor tests it ────────────────────
KIT="$ROOT/$RUN_DIR/lease-conformance"; [ -f "$KIT/run.sh" ] || KIT="$ROOT/scripts/ci/lease-conformance"
if [ ! -f "$KIT/run.sh" ]; then
  fail "the lease conformance kit is missing ($RUN_DIR/lease-conformance or scripts/ci/lease-conformance)"
elif out=$(sh "$KIT/run.sh" --sha 2>&1); then
  ok "the lease conformance kit is the suite its VERSION names (sha256 $(printf %.12s "$out")…)"
else
  fail "$out"
fi

# ── A project whose GATE_RUN is its own runner ──────────────────────────────────────────
if ! grep -q '^# run-local.sh: THE full gate' "$ROOT/$RUN_REL" 2>/dev/null; then
  if [ ! -f "$ROOT/$RUN_REL" ]; then
    fail "GATE_RUN=$RUN_REL does not exist"
  elif grep -q 'lib/steps.sh' "$ROOT/$RUN_REL" && grep -qE 'model_step' "$ROOT/$RUN_REL"; then
    ok "GATE_RUN=$RUN_REL is the project's own runner, and it runs the model's steps (lib/steps.sh)"
  else
    fail "GATE_RUN=$RUN_REL is the project's own runner and does not run the model's steps: source scripts/ci/lib/steps.sh and call model_steps (the scenario trace and the secret scan)"
  fi
  finish
fi

# ── The model's runner ──────────────────────────────────────────────────────────────────
R="$d/repo"; new_repo "$R"
mkdir -p "$R/$RUN_DIR/lib" "$R/$(dirname "$WRAP_REL")" "$R/$(dirname "$STEPS_REL")" "$R/.claude/lib"
cp "$ROOT/$RUN_REL" "$R/$RUN_REL"
cp "$ROOT/$RUN_DIR/lease.sh" "$R/$RUN_DIR/lease.sh"
cp "$LIB" "$R/$RUN_DIR/lib/steps.sh"
if grep -q '^# gate.sh: run-local.sh for an AGENT' "$ROOT/$WRAP_REL" 2>/dev/null; then cp "$ROOT/$WRAP_REL" "$R/$WRAP_REL"; WRAP=yes; else WRAP=""; fi
cp "$CLAUDUCTOR_FW/lib/conf.sh" "$CLAUDUCTOR_FW/lib/change.sh" "$R/.claude/lib/"
cp "$CLAUDUCTOR_FW/scenario-trace.sh" "$R/.claude/"
printf 'GATE_RUN="%s"\nGATE="%s"\nGATE_STEPS="%s"\n' "$RUN_REL" "$WRAP_REL" "$STEPS_REL" > "$R/.claude/project.conf"
cat > "$R/$STEPS_REL" <<'EOF'
gate_steps() {
  step "lease held" sh -c '[ -n "$CLAUDUCTOR_LOCK_HELD" ] && [ -d "$CLAUDUCTOR_LOCK_HELD" ]' || return 1
  step "unit" sh -c '[ ! -f FAIL_UNIT ]' || return 1
  # Takes the lease away mid-run (another holder's nonce), as a reclaim by mistake would.
  if [ -f STEAL_LEASE ]; then
    sed 's/"nonce":"[0-9a-f]*"/"nonce":"0123456789abcdef"/' "$CLAUDUCTOR_LOCK_HELD/owner.json" > "$CLAUDUCTOR_LOCK_HELD/x"
    mv "$CLAUDUCTOR_LOCK_HELD/x" "$CLAUDUCTOR_LOCK_HELD/owner.json"
  fi
  [ "$1" = full ] || return 0
  step "slow" sh -c '[ ! -f FAIL_SLOW ]' || return 1
}
EOF
printf 'FAIL_UNIT\nFAIL_SLOW\nSTEAL_LEASE\nlocal/\n' > "$R/.gitignore"
git -C "$R" add -A && git -C "$R" commit -qm init
SHA=$(git -C "$R" rev-parse HEAD)
REC="$R/.git/ci-receipt"
LOCK="$R/.git/clauductor/gate.lock"
run() { (cd "$R" && PATH="$d/bin:$np" CI= CLAUDUCTOR_LANE=check bash "$RUN_REL" "$@") >"$d/out" 2>&1; }

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
nogl() { (cd "$R" && PATH="$np" CI="$1" CLAUDUCTOR_LANE=check bash "$RUN_REL") >"$d/out" 2>&1; }
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

if [ -n "$WRAP" ]; then
  out=$(cd "$R" && PATH="$d/bin:$np" CI= CLAUDUCTOR_LANE=check bash "$WRAP_REL" --quick 2>&1); rc=$?
  expect_rc 0 "$rc" "gate.sh keeps run-local's exit code"
  case "$out" in *"==> unit"*"full log"*) ok "gate.sh prints the step markers and the log path" ;; *) fail "gate.sh output: $out" ;; esac
  touch "$R/FAIL_UNIT"
  (cd "$R" && PATH="$d/bin:$np" CI= bash "$WRAP_REL" --quick >/dev/null 2>&1); rc=$?
  expect_rc 1 "$rc" "gate.sh passes a failure's exit code through"
  rm -f "$R/FAIL_UNIT"
else
  ok "GATE=$WRAP_REL is the project's own wrapper, not the model's gate.sh: its output contract is its own"
fi

# The vendored lease.sh must be the block clauductor documents (the framework's Go tests compare it
# with docs/panel.md; here: it still defines what run-local.sh calls).
grep -q '^lease_run() {' "$ROOT/$RUN_DIR/lease.sh" && ok "lease.sh defines lease_run" || fail "lease.sh has no lease_run"
grep -q '^lease_verify() {' "$ROOT/$RUN_DIR/lease.sh" && ok "lease.sh defines lease_verify" || fail "lease.sh has no lease_verify"

# ── The lease is renewed and verified (B15) ─────────────────────────────────────────────
LS="$R/$RUN_DIR/lease.sh"; K="$d/lk"; mkdir -p "$K"; KL="$K/gate.lock"
# lease CMD...: lease_run on $KL with a 3 s ttl (renewed each second), from bash as run-local does.
lease() { CLAUDUCTOR_LEASE_TTL=3 bash -c '. "$0"; lease_run "$1" check "${@:2}"' "$LS" "$KL" "$@"; }
lease sh -c 'cat "$CLAUDUCTOR_LOCK_HELD/owner.json" > "$0/o1"; sleep 3; cat "$CLAUDUCTOR_LOCK_HELD/owner.json" > "$0/o2"' "$d" 2>/dev/null
grep -q '"ttl":3[,}]' "$d/o1" 2>/dev/null && ok "the holder writes a real ttl (CLAUDUCTOR_LEASE_TTL), not ttl 0" || fail "owner.json ttl: $(cat "$d/o1" 2>/dev/null)"
r1=$(sed -n 's/.*"renewed":\([0-9]*\).*/\1/p' "$d/o1" 2>/dev/null); r2=$(sed -n 's/.*"renewed":\([0-9]*\).*/\1/p' "$d/o2" 2>/dev/null)
[ -n "$r1" ] && [ -n "$r2" ] && [ "$r2" -gt "$r1" ] && ok "the holder renews 'renewed' while the command runs ($r1 -> $r2)" || fail "renewed did not move: '$r1' -> '$r2'"
[ ! -e "$KL" ] && ok "the lease is released after the command" || fail "the lease is left behind: $(cat "$KL/owner.json" 2>/dev/null)"
out=$(lease bash -c '. "$0"; lease_verify "$CLAUDUCTOR_LOCK_HELD" "$PPID" && echo VERIFIED || echo LOST' "$LS" 2>&1)
[ "$out" = VERIFIED ] && ok "lease_verify: the command holding the lease is told it holds it" || fail "lease_verify while held: $out"
out=$(lease bash -c 'sed "s/\"nonce\":\"[0-9a-f]*\"/\"nonce\":\"0123456789abcdef\"/" "$CLAUDUCTOR_LOCK_HELD/owner.json" > "$CLAUDUCTOR_LOCK_HELD/x"; mv "$CLAUDUCTOR_LOCK_HELD/x" "$CLAUDUCTOR_LOCK_HELD/owner.json"; . "$0"; lease_verify "$CLAUDUCTOR_LOCK_HELD" "$PPID" && echo VERIFIED || echo LOST' "$LS" 2>/dev/null); rc=$?
[ "$out" = LOST ] && ok "lease_verify: a command whose lease was taken away is told it lost it" || fail "lease_verify after the lease was taken: $out"
expect_rc 70 "$rc" "lease_run exits 70 when the lease was taken away while the command ran"
[ -e "$KL" ] && ok "...and leaves the new holder's lease alone" || fail "lease_run removed a lease that was no longer its own"
rm -rf "$KL" "$KL.waiters"
# Under lock-run no nonce is exported: verified by the holder's pid among the asker's ancestors.
out=$(bash -c '. "$0"; mkdir -p "$1"; printf "{\"v\":1,\"nonce\":\"00112233aabbccdd\",\"pid\":%s,\"pstart\":\"\",\"host\":\"h\",\"lane\":\"l\",\"cmd\":\"c\",\"started\":1,\"renewed\":1,\"ttl\":600}\n" "$$" > "$1/owner.json"; sh -c ". \"\$0\"; lease_verify \"\$1\" \$PPID && echo VERIFIED || echo LOST" "$0" "$1"' "$LS" "$KL" 2>&1)
[ "$out" = VERIFIED ] && ok "lease_verify: with no nonce (lock-run), the holder's pid as an ancestor verifies" || fail "lease_verify by pid: $out"
rm -rf "$KL"

# The kit runs against this lease.sh (two quick cases; clauductor's own tests run all of them).
if [ -f "$KIT/run.sh" ]; then
  out=$(CASES="dead-pid owner-record" CONFORMANCE_WAIT=1 sh "$KIT/run.sh" 2>&1)
  [ "$(printf '%s\n' "$out" | grep -c '^ok ')" = 2 ] && ok "the conformance kit passes lease.sh (dead-pid, owner-record)" || fail "conformance kit on lease.sh: $(printf '%s' "$out" | grep -v '^ok' | head -5)"
fi

# The gate itself: a run that lost its lease writes no receipt.
run; [ -f "$REC" ] || fail "fixture: no receipt before the stolen-lease run: $(grep '^==>' "$d/out" | tr '\n' ' ')"
touch "$R/STEAL_LEASE"; run; rc=$?; rm -f "$R/STEAL_LEASE"
expect_rc 70 "$rc" "a gate whose lease was taken away mid-run fails (70)"
grep -q '^==> FAIL: lease' "$d/out" && [ ! -f "$REC" ] && ok "...names the lost lease and writes no receipt (deletes the old one)" || fail "stolen lease: $(grep '^==>' "$d/out" | tr '\n' ' '); receipt $(cat "$REC" 2>/dev/null || echo none)"
rm -rf "$LOCK" "$LOCK.waiters"
finish
