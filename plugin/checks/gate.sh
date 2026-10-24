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
for f in lib/gate-state.sh gate-wait.sh; do [ -f "$ROOT/$RUN_DIR/$f" ] && cp "$ROOT/$RUN_DIR/$f" "$R/$RUN_DIR/$f"; done
if grep -q '^# gate.sh: run-local.sh for an AGENT' "$ROOT/$WRAP_REL" 2>/dev/null; then cp "$ROOT/$WRAP_REL" "$R/$WRAP_REL"; WRAP=yes; else WRAP=""; fi
cp "$CLAUDUCTOR_FW/lib/conf.sh" "$CLAUDUCTOR_FW/lib/change.sh" "$R/.claude/lib/"
[ -f "$CLAUDUCTOR_FW/lib/docs-gate.sh" ] && cp "$CLAUDUCTOR_FW/lib/docs-gate.sh" "$R/.claude/lib/"
cp "$CLAUDUCTOR_FW/scenario-trace.sh" "$R/.claude/"
printf 'GATE_RUN="%s"\nGATE="%s"\nGATE_STEPS="%s"\n' "$RUN_REL" "$WRAP_REL" "$STEPS_REL" > "$R/.claude/project.conf"
cat > "$R/$STEPS_REL" <<'EOF'
gate_steps() {
  # A full run holds the lease; a --quick run takes none (OPS-103).
  if [ "$1" = full ]; then
    step "lease held" sh -c '[ -n "$CLAUDUCTOR_LOCK_HELD" ] && [ -d "$CLAUDUCTOR_LOCK_HELD" ]' || return 1
  else
    step "no lease" sh -c '[ -z "${CLAUDUCTOR_LOCK_HELD:-}" ]' || return 1
  fi
  # Holds the run while local/HOLD (ignored, so the tree under test does not change) exists.
  step "hold" sh -c 'while [ -f local/HOLD ]; do sleep 1; done' || return 1
  # Fails if the caller's CHECKS_SKIP reached the steps (a checks/run.sh here would leave checks out).
  step "unit" sh -c '[ ! -f FAIL_UNIT ] && [ -z "${CHECKS_SKIP+x}" ]' || return 1
  # Leaves a directory the clean room's cleanup cannot remove (a step that chmods its own output).
  if [ -f LOCK_CLEANUP ]; then mkdir -p locked/x && touch locked/x/f && chmod 555 locked/x; fi
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
(cd "$R" && PATH="$d/bin:$np" CI= CLAUDUCTOR_LANE=check CHECKS_SKIP="gate hooks" bash "$RUN_REL" --quick) >"$d/out" 2>&1; rc=$?
expect_rc 0 "$rc" "a CHECKS_SKIP in the caller's environment never reaches the gate's steps (no check is left out)"

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

  # ── The summary contract, driven with GATE_RUNNER (ported from Standing Tee's ci-gate-summary):
  # every failure line survives a 5,000-line log, colour codes and a `pnpm -r` prefix included; a
  # green 10,000-line run stays short; GATE_TAIL_EXCLUDE drops the JSON lines an e2e server logs
  # after the verdict (and only it does); the runner's exit code, not the filter's, is returned.
  S="$d/summary"; new_repo "$S"; mkdir -p "$S/$(dirname "$WRAP_REL")" "$S/.claude/lib"
  cp "$ROOT/$WRAP_REL" "$S/$WRAP_REL"; cp "$CLAUDUCTOR_FW/lib/conf.sh" "$S/.claude/lib/"
  # Standing Tee's own GATE_FAIL_PATTERN (its infra/ci/gate.sh), set the way a project sets it.
  cat > "$S/.claude/project.conf" <<'EOF'
GATE_FAIL_PATTERN='^gate-lock: |^==> |error TS[0-9]+|Found [0-9]+ errors?|(^|: ) *(FAIL|×|✗|✘) |AssertionError|Error: |(Test Files|Tests|Errors) +[0-9]+ |^ +[0-9]+\) \[|^ +[0-9]+ (passed|failed)|[0-9]+ failed|REFUSING|!!|lint/[a-zA-Z]+/|CI-STEPS-COMPLETE'
GATE_TAIL_EXCLUDE='^\{"'
EOF
  noise='for i in $(seq 1 5000); do echo "  ✓ passing test $i (3ms)"; done'
  { echo 'echo "==> lint"'; echo "$noise"; echo 'echo "apps/web/src/x.ts:3:1 lint/suspicious/noExplicitAny ━━━━━━"'
    echo 'echo "==> typecheck"'; echo "echo \"src/a.ts(4,7): error TS2322: Type 'string' is not assignable to type 'number'.\""
    echo 'echo "==> test (TZ=UTC)"'
    echo "printf 'packages/db test:  \\033[41m FAIL \\033[49m packages/db/test/foo.test.ts > foo > bar\\n'"
    echo "printf 'packages/db test: AssertionError: expected 1 to be 2\\n'"
    echo "printf 'packages/db test: \\033[2m      Tests \\033[22m \\033[1m\\033[31m1 failed\\033[39m\\033[22m | 4999 passed (5000)\\n'"
    echo "printf 'packages/service test: \\033[2m     Errors \\033[22m \\033[1m\\033[31m1 error\\033[39m\\033[22m\\n'"
    echo "printf '  1) [chromium] \\342\\200\\272 e2e/csp.spec.ts:132:5 \\342\\200\\272 signed out \\n'"
    echo "$noise"; echo 'echo "==> FAIL (exit 1)"'; echo 'exit 1'; } > "$d/red.sh"
  { echo "$noise"; echo "printf 'packages/db test: \\033[2m Test Files \\033[22m \\033[1m\\033[32m98 passed\\033[39m (99)\\n'"
    echo "$noise"; echo 'echo "==> PASS"'
    echo "for i in \$(seq 1 40); do echo '{\"route\":\"/leagues/:leagueId\",\"result_code\":\"ok\"}'; done"; } > "$d/green.sh"
  summ() { (cd "$S" && GATE_RUNNER="$1" bash "$WRAP_REL" --quick) > "$d/summ.out" 2>&1; }
  summ "$d/red.sh"; rc=$?
  expect_rc 1 "$rc" "gate.sh summary: a red run keeps its exit code"
  for want in "lint/suspicious/noExplicitAny" "error TS2322" "FAIL  packages/db/test/foo.test.ts > foo > bar" "AssertionError: expected 1 to be 2" "Tests  1 failed" "Errors  1 error" "1) [chromium] › e2e/csp.spec.ts:132:5" "==> FAIL (exit 1)"; do
    grep -qF -- "$want" "$d/summ.out" && ok "gate.sh summary: shows '$want' from a 10,000-line log, colour stripped" || fail "gate.sh summary: '$want' missing from the red summary"
  done
  log=$(sed -n 's/^--- full log ([0-9]* lines): //p' "$d/summ.out")
  [ -f "$log" ] && [ "$(wc -l < "$log")" -gt 10000 ] && ok "gate.sh summary: the whole log stays on disk ($(wc -l < "$log" | tr -d ' ') lines)" || fail "gate.sh summary: no full log at '$log'"
  summ "$d/green.sh"; rc=$?
  expect_rc 0 "$rc" "gate.sh summary: a green run keeps its exit code"
  if grep -qF "Test Files  98 passed" "$d/summ.out" && grep -qF "==> PASS" "$d/summ.out" && ! grep -qF '{"route"' "$d/summ.out" && [ "$(wc -l < "$d/summ.out")" -lt 60 ]; then
    ok "gate.sh summary: a green run is short, shows the counts and the verdict, and GATE_TAIL_EXCLUDE drops the JSON lines after it"
  else fail "gate.sh summary: green output ($(wc -l < "$d/summ.out" | tr -d ' ') lines): $(head -c 600 "$d/summ.out" | tr '\n' '|')"; fi
  grep -v '^GATE_TAIL_EXCLUDE' "$S/.claude/project.conf" > "$d/conf" && cp "$d/conf" "$S/.claude/project.conf"
  summ "$d/green.sh"
  grep -qF '{"route"' "$d/summ.out" && ok "gate.sh summary: ...and without GATE_TAIL_EXCLUDE the tail keeps them (the setting is what drops them)" || fail "gate.sh summary: JSON lines dropped with GATE_TAIL_EXCLUDE unset"
  printf "GATE_TAIL_EXCLUDE='^==> PASS'\n" >> "$S/.claude/project.conf"
  summ "$d/green.sh"
  sed -n '/^--- last/,$p' "$d/summ.out" > "$d/tail"
  grep -qF '{"route"' "$d/tail" && ! grep -qF '==> PASS' "$d/tail" && ok "gate.sh summary: ...and another GATE_TAIL_EXCLUDE drops what IT matches, not JSON (the pattern is the project's)" || fail "gate.sh summary: GATE_TAIL_EXCLUDE='^==> PASS' tail: $(tr '\n' '|' < "$d/tail" | head -c 300)"
  printf '#!/bin/sh\necho boom; exit 3\n' > "$d/exit3.sh"
  summ "$d/exit3.sh"; rc=$?
  expect_rc 3 "$rc" "gate.sh summary: returns the runner's exit code (3), not the filter's"
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
out=$(bash -c '. "$0"; mkdir -p "$1"; printf "{\"v\":1,\"nonce\":\"00112233aabbccdd\",\"pid\":%s,\"pstart\":\"\",\"host\":\"h\",\"lane\":\"l\",\"cmd\":\"c\",\"started\":1,\"renewed\":1,\"ttl\":600}\n" "$$" > "$1/owner.json"; sh -c ". \"\$0\"; lease_verify \"\$1\" \$PPID && echo VERIFIED || echo LOST" "$0" "$1"; :' "$LS" "$KL" 2>&1)
# (the trailing `:` keeps bash from exec-ing its last command, which would make the holder the asker itself)
[ "$out" = VERIFIED ] && ok "lease_verify: with no nonce (lock-run), the holder's pid as an ancestor verifies" || fail "lease_verify by pid: $out"
rm -rf "$KL"

# Where processes cannot be inspected (a sandbox: ps errors, kill -0 answers EPERM for every pid),
# the lease never hangs in silence. ps and kill are stubbed; each run is bounded here, so a hang is
# a FAIL, not a stuck check.
U="$d/uninspectable"; mkdir -p "$U/bin"; UL="$U/gate.lock"
printf '#!/bin/sh\necho "ps: operation not permitted" >&2\nexit 1\n' > "$U/bin/ps"; chmod +x "$U/bin/ps"
holder() {  # holder RENEWED TTL: a holder record on this host, for a pid that cannot be inspected
  rm -rf "$UL" "$UL.waiters"; mkdir -p "$UL"
  printf '{"v":1,"nonce":"0123456789abcdef","pid":999999,"pstart":"","host":"%s","lane":"ghost","cmd":"gate","started":%s,"renewed":%s,"ttl":%s}\n' "$(hostname)" "$1" "$1" "$2" > "$UL/owner.json"
}
blind() {  # blind MAXWAIT: lease_run with ps erroring and kill -0 denied; killed after 25 s
  ( PATH="$U/bin:$PATH" CLAUDUCTOR_LEASE_MAX_WAIT=$1 bash -c 'kill() { case "$1" in -0) echo "kill: Operation not permitted" >&2; return 1 ;; esac; builtin kill "$@"; }
    . "$0"; lease_run "$1" blind sh -c "echo RAN"' "$LS" "$UL" > "$U/out" 2>&1; echo "$?" > "$U/rc" ) &
  bp=$!; n=0
  while kill -0 "$bp" 2>/dev/null && [ "$n" -lt 25 ]; do sleep 1; n=$((n + 1)); done
  if kill -0 "$bp" 2>/dev/null; then kill "$bp" 2>/dev/null; echo hung > "$U/rc"; fi
  wait "$bp" 2>/dev/null
}
holder $(( $(date +%s) - 3600 )) 60; blind 0
[ "$(cat "$U/rc")" = 0 ] && grep -q '^RAN$' "$U/out" && ok "uninspectable: a holder whose heartbeat expired is reclaimed, and the waiter (never judging itself) runs" || fail "uninspectable, expired heartbeat: rc $(cat "$U/rc"): $(tr '\n' ' ' < "$U/out")"
holder "$(date +%s)" 0; blind 3
[ "$(cat "$U/rc")" = 124 ] && ok "uninspectable: a holder it cannot judge is waited for at most CLAUDUCTOR_LEASE_MAX_WAIT, exit 124" || fail "uninspectable, live-looking holder: rc $(cat "$U/rc") (want 124, not a hang): $(tr '\n' ' ' < "$U/out")"
grep -q 'gave up after .*held by pid 999999' "$U/out" && grep -q "rm -rf '$UL'" "$U/out" && ok "...and names the holder and how to remove the lock" || fail "give-up message: $(tr '\n' ' ' < "$U/out")"
rm -rf "$UL" "$UL.waiters"

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

# --quick takes no lease (OPS-103): nothing it runs binds a fixed port, socket or database, or
# writes the receipt, so it never queues behind a full gate. A full run still queues. The other
# holder is a live process on this host; each run is bounded by CLAUDUCTOR_LEASE_MAX_WAIT, so a
# quick run that queued shows as 124, not a hang.
sleep 60 & other=$!
mkdir -p "$LOCK"
printf '{"v":1,"nonce":"0123456789abcdef","pid":%s,"pstart":"","host":"%s","lane":"other","cmd":"gate","started":%s,"renewed":%s,"ttl":600}\n' \
  "$other" "$(hostname)" "$(date +%s)" "$(date +%s)" > "$LOCK/owner.json"
(cd "$R" && PATH="$d/bin:$np" CI= CLAUDUCTOR_LANE=check CLAUDUCTOR_LEASE_MAX_WAIT=3 bash "$RUN_REL" --quick) >"$d/out" 2>&1; rc=$?
expect_rc 0 "$rc" "a --quick run passes while another gate holds the lease (it takes none)"
grep -q '"lane":"other"' "$LOCK/owner.json" 2>/dev/null && ok "...and leaves the holder's lease alone" || fail "the quick run changed the holder's lease: $(cat "$LOCK/owner.json" 2>/dev/null || echo gone)"
(cd "$R" && PATH="$d/bin:$np" CI= CLAUDUCTOR_LANE=check CLAUDUCTOR_LEASE_MAX_WAIT=3 bash "$RUN_REL") >"$d/out" 2>&1; rc=$?
expect_rc 124 "$rc" "a full run still queues behind the holder (gave up after CLAUDUCTOR_LEASE_MAX_WAIT)"
grep -q '^==> ' "$d/out" && fail "...but a full run ran steps without the lease: $(grep '^==>' "$d/out" | tr '\n' ' ')" || ok "...and ran no step without it"
kill "$other" 2>/dev/null; wait "$other" 2>/dev/null
# A quick run started inside another gate's lease (a fixture's gate, run by a step) neither takes
# that lease as its own nor verifies it at the end: it holds none.
(cd "$R" && PATH="$d/bin:$np" CI= CLAUDUCTOR_LANE=check CLAUDUCTOR_LOCK_HELD="$d/someone-elses.lock" bash "$RUN_REL" --quick) >"$d/out" 2>&1; rc=$?
expect_rc 0 "$rc" "a --quick run under a caller's CLAUDUCTOR_LOCK_HELD runs unleased and is not failed by that lease"
rm -rf "$LOCK" "$LOCK.waiters"

# ── The docs gate (OPS-89) ──────────────────────────────────────────────────────────────
# `--docs` vouches only for a diff that is prose against origin/main (.claude/lib/docs-gate.sh): it
# runs the model's steps and `gate_steps docs`, never the full-only ones, and writes a receipt whose
# stage is `docs`, which the merge guard accepts only for a PR whose own diff is prose.
git init -q --bare "$d/origin.git"; git -C "$R" remote add origin "$d/origin.git"
git -C "$R" push -q origin HEAD:main && git -C "$R" fetch -q origin
git -C "$R" checkout -q -b docs-x
mkdir -p "$R/docs"; printf '# A note\n' > "$R/docs/note.md"; git -C "$R" add -A && git -C "$R" commit -qm "a note"
DOCS=$(git -C "$R" rev-parse HEAD)
run --docs; rc=$?
expect_rc 1 "$rc" "the docs gate is off until GATE_DOCS says the project's docs mode guards its own tests"
grep -q 'docs gate: off in this project' "$d/out" && [ ! -f "$REC" ] && ! grep -q '^==> secrets' "$d/out" && ok "...refused before any step, naming GATE_DOCS" || fail "docs gate off: $(grep -v '^ ' "$d/out" | tail -3 | tr '\n' ' ')"
# On, on main (undone below): on the branch, project.conf would be the diff's code.
git -C "$R" checkout -q main; printf 'GATE_DOCS="1"\n' >> "$R/.claude/project.conf"; git -C "$R" commit -qam "docs gate on"
git -C "$R" push -q origin HEAD:main && git -C "$R" fetch -q origin
git -C "$R" checkout -q docs-x && git -C "$R" rebase -q main; DOCS=$(git -C "$R" rev-parse HEAD)
touch "$R/FAIL_SLOW"; run --docs; rc=$?
expect_rc 0 "$rc" "a docs gate over a prose-only diff passes (and runs no full-only step)"
[ "$(cat "$REC" 2>/dev/null)" = "$(printf '%s\tdocs\tclean\tdocs' "$DOCS")" ] && ok "...and writes '<sha> docs clean docs', never stage all" || fail "docs receipt: $(cat "$REC" 2>/dev/null || echo none); $(grep '^==>' "$d/out" | tr '\n' ' ')"
grep -q '^==> unit' "$d/out" && ! grep -q '^==> slow' "$d/out" && grep -q '^==> secrets' "$d/out" && ok "...running the model's steps and gate_steps docs" || fail "docs gate steps: $(grep '^==>' "$d/out" | tr '\n' ' ')"
rm -f "$R/FAIL_SLOW"
touch "$R/FAIL_UNIT"; run --docs; rc=$?; rm -f "$R/FAIL_UNIT"
expect_rc 1 "$rc" "a failing docs gate fails"
[ ! -f "$REC" ] && ok "...and deletes the receipt (a stale pass must not outlive a fail)" || fail "a failed docs gate left $(cat "$REC")"
# The fixture's own skill path, in pieces: the plugin build rewrites a literal framework path.
cl=.claude
for p in x.sh "$cl/skills/x/SKILL.md" AGENTS.md docs/sub/.claude/a.md specs/cap/spec.md docs/Claude.md CLAUDE.local.md skills/y/SKILL.md agents/a.md; do
  mkdir -p "$(dirname "$R/$p")"; echo x > "$R/$p"; git -C "$R" add -A && git -C "$R" commit -qm "$p"
  run --docs; rc=$?
  expect_rc 1 "$rc" "the docs gate refuses a diff with $p in it"
  grep -q "not a docs-only diff.*$p" "$d/out" && [ ! -f "$REC" ] && ! grep -q '^==> secrets' "$d/out" && ok "...before any step, naming $p, with no receipt" || fail "docs refusal for $p: $(grep -v '^ ' "$d/out" | tail -3 | tr '\n' ' ')"
  git -C "$R" reset -q --hard HEAD~1
done
# A rename lists both ends (--no-renames): `git mv x.sh x.md` deletes code, and must not read as prose.
git -C "$R" checkout -q main; echo x > "$R/x.sh"; git -C "$R" add x.sh && git -C "$R" commit -qm x.sh
git -C "$R" push -q origin HEAD:main && git -C "$R" fetch -q origin
git -C "$R" checkout -q docs-x && git -C "$R" rebase -q main && git -C "$R" mv x.sh x.md && git -C "$R" commit -qm "x.sh to x.md"
run --docs; rc=$?
expect_rc 1 "$rc" "the docs gate refuses a rename of code to Markdown"
grep -q 'not a docs-only diff.*x\.sh' "$d/out" && ok "...naming the code it deletes" || fail "docs rename: $(grep -v '^ ' "$d/out" | tail -3 | tr '\n' ' ')"
git -C "$R" reset -q --hard HEAD~1
# The working tree counts too: an untracked script is code the run would test.
echo x > "$R/new.sh"; run --docs; rc=$?; rm -f "$R/new.sh"
expect_rc 1 "$rc" "the docs gate refuses an untracked code file"
grep -q 'not a docs-only diff.*new\.sh' "$d/out" && [ ! -f "$REC" ] && ok "...naming it, before any step" || fail "docs untracked: $(grep -v '^ ' "$d/out" | tail -3 | tr '\n' ' ')"
# Config never narrows the list: diff.relative from a subdirectory, and a gitlink bump that
# diff.ignoreSubmodules=all would hide.
echo x > "$R/x2.sh"; git -C "$R" add x2.sh
git -C "$R" update-index --add --cacheinfo "160000,$(git -C "$R" rev-parse HEAD),sub" && git -C "$R" commit -qm "code and a gitlink"
got=$(cd "$R/docs" && git config diff.relative true && git config diff.ignoreSubmodules all && . "$R/.claude/lib/docs-gate.sh" && docs_gate_paths HEAD | tr '\n' ' ')
git -C "$R" config --unset diff.relative; git -C "$R" config --unset diff.ignoreSubmodules
case " $got " in *" x2.sh "*) has_x=1 ;; *) has_x=0 ;; esac
case " $got " in *" sub "*) has_s=1 ;; *) has_s=0 ;; esac
[ "$has_x$has_s" = 11 ] && ok "docs_gate_paths lists from the root and every gitlink, whatever diff.relative and diff.ignoreSubmodules say" || fail "docs_gate_paths under diff.relative and ignoreSubmodules=all: '$got', want x2.sh and sub"
git -C "$R" reset -q --hard HEAD~1
git -C "$R" checkout -q main; git -C "$R" reset -q --hard HEAD~2; git -C "$R" branch -q -D docs-x; git -C "$R" remote remove origin

# ── The run state, the run log and gate-wait (OPS-41) ───────────────────────────────────
# Every run writes the worktree's state file and one run-log line; gate-wait reads the state and says
# PASS, FAIL, STILL RUNNING or NO RESULT for the tree in front of it, and never waits past its bound.
WAIT_REL="$RUN_DIR/gate-wait.sh"; STATE="$R/.git/ci-gate.state"; RUNS="$R/.git/clauductor/gate-runs.log"
if [ ! -f "$R/$WAIT_REL" ] || [ ! -f "$R/$RUN_DIR/lib/gate-state.sh" ]; then
  fail "gate-wait.sh or lib/gate-state.sh is missing beside GATE_RUN ($RUN_DIR): no gate can be waited for, and no run is recorded"
  finish
fi
gw() {  # gw [SECONDS] [PATH]: gate-wait in $R, its output in $d/gw
  (cd "$R" && PATH="${2:-$d/bin:$np}" GATE_WAIT_POLL=1 GATE_WAIT_SECONDS="${1:-0}" sh "$WAIT_REL") > "$d/gw" 2>&1
}
lines() { if [ -f "$1" ]; then wc -l < "$1" | tr -d ' '; else echo 0; fi; }
says() { grep -q "$1" "$d/gw" && ok "$2" || fail "$2: gate-wait said: $(tr '\n' ' ' < "$d/gw")"; }
# A process that cannot be inspected: ps fails, as in a sandbox.
mkdir -p "$d/nops"; printf '#!/bin/sh\necho "ps: operation not permitted" >&2\nexit 1\n' > "$d/nops/ps"; chmod +x "$d/nops/ps"

rm -f "$STATE" "$REC"
gw; rc=$?
expect_rc 4 "$rc" "gate-wait: no gate has run here and no receipt names HEAD: NO RESULT (4)"
says 'no gate has run in this checkout' "...and it says no gate has run, and names the foreground gate"

n0=$(lines "$RUNS")
run; rc=$?; SHA=$(git -C "$R" rev-parse HEAD)
expect_rc 0 "$rc" "fixture: a full clean run passes"
[ "$(lines "$RUNS")" = $((n0 + 1)) ] && ok "the run log gains one line per run ($RUNS)" || fail "run log: $(lines "$RUNS") lines after one run, $n0 before"
last=$(tail -n 1 "$RUNS" 2>/dev/null)
if printf '%s\n' "$last" | awk -F'\t' -v sha="$SHA" 'NF == 10 && $1 ~ /^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9:]{8}Z$/ && $2 == "check" && $3 == "main" && $4 == sha && $5 == "full" && $6 == "pass" && $7 == "0" && $8 ~ /^[0-9]+$/ && $9 ~ /^[0-9]+$/ { ok = 1 } END { exit !ok }'; then
  ok "...a line of time, lane, branch, sha, mode, pass, exit code, run and queue seconds, and the steps"
else fail "run log line: '$last'"; fi
# Each step's duration (OPS-89): every step that ran, in order, the model's two first.
steps_re='^scenario trace=[0-9]+;secrets=[0-9]+;lease held=[0-9]+;hold=[0-9]+;unit=[0-9]+;slow=[0-9]+$'
printf '%s\n' "$last" | awk -F'\t' -v re="$steps_re" '$10 ~ re { ok = 1 } END { exit !ok }' \
  && ok "...the steps field names each step that ran, in order, with its seconds" || fail "run log steps field: '$(printf '%s' "$last" | cut -f10)'"
grep -q "^started pid=[0-9]* at=[0-9]* mode=full subject=worktree head=$SHA tree=$SHA script=run-local.sh" "$STATE" && grep -q '^exit pid=[0-9]* code=0 ' "$STATE" \
  && ok "the state file has the run's started line (a clean tree's id is HEAD) and its exit line" || fail "state: $(tr '\n' '|' < "$STATE" 2>/dev/null)"
gw; rc=$?
expect_rc 0 "$rc" "gate-wait after a passing run: PASS (0)"
says "PASS for HEAD $(printf %.9s "$SHA"). Receipt" "...naming HEAD and the receipt"

touch "$R/FAIL_UNIT"; run --quick; rm -f "$R/FAIL_UNIT"
tail -n 1 "$RUNS" | awk -F'\t' '$5 == "quick" && $6 == "fail" && $7 == "1" && $10 ~ /;hold=[0-9]+;unit=[0-9]+$/ { ok = 1 } END { exit !ok }' \
  && ok "a failed --quick run is logged as quick, fail, exit 1, its steps ending at the one that failed" || fail "run log after a failed --quick: '$(tail -n 1 "$RUNS")'"
gw; rc=$?
expect_rc 1 "$rc" "gate-wait after a failed run: FAIL (1), even with a passing receipt for HEAD on disk"
says 'FAIL for HEAD .*exited 1' "...and says the gate exited 1"

echo "edit" > "$R/new-file"
gw; rc=$?
expect_rc 4 "$rc" "gate-wait after an edit since the last run: NO RESULT (4), never that run's verdict"
says 'tested an earlier state of this tree' "...and says the run tested an earlier state of the tree"
run --dirty --quick; gw; rc=$?
expect_rc 0 "$rc" "...and a run of the edited tree is its verdict: PASS (0)"
says 'PASS (--quick) for HEAD .* + uncommitted changes' "...for HEAD plus the uncommitted changes, and says a quick run writes no receipt"
rm -f "$R/new-file"

# Running: a gate held in its hold step. gate-wait returns within its bound (3), then follows it to PASS.
mkdir -p "$R/local"; touch "$R/local/HOLD"
(cd "$R" && PATH="$d/bin:$np" CI= CLAUDUCTOR_LANE=check exec bash "$RUN_REL") > "$d/bg" 2>&1 &
bg=$!
n=0; while ! grep -q '^==> hold' "$d/bg" 2>/dev/null && [ "$n" -lt 30 ]; do sleep 1; n=$((n + 1)); done
s=$(date +%s); gw 1; rc=$?; took=$(( $(date +%s) - s ))
expect_rc 3 "$rc" "gate-wait while a gate runs: STILL RUNNING (3), call again"
[ "$took" -lt 10 ] && ok "...within its bound (GATE_WAIT_SECONDS=1: ${took}s), never a hang" || fail "gate-wait took ${took}s with GATE_WAIT_SECONDS=1"
gw 1 "$d/nops:$d/bin:$np"; rc=$?
expect_rc 3 "$rc" "...and where ps cannot inspect it (a sandbox), kill -0 still finds it running (3)"
rm -f "$R/local/HOLD"
gw 60; rc=$?
expect_rc 0 "$rc" "gate-wait follows a running gate to its end: PASS (0)"
wait "$bg" 2>/dev/null
# The step held for the gate-wait calls above (seconds) is logged with what it took, not a constant.
held=$(tail -n 1 "$RUNS" | cut -f10 | tr ';' '\n' | sed -n 's/^hold=//p')
[ "${held:-0}" -ge 1 ] 2>/dev/null && ok "a step's logged duration is what it took (the held step: ${held}s)" || fail "the held step's logged duration: '${held:-none}' in '$(tail -n 1 "$RUNS" | cut -f10)'"

# Died: a started line whose process is gone, with or without ps.
dead=$(sh -c 'echo $$')
printf 'started pid=%s at=1 mode=full subject=worktree head=%s tree=%s script=run-local.sh\n' "$dead" "$SHA" "$SHA" > "$STATE"
rm -f "$REC"
gw; rc=$?
expect_rc 4 "$rc" "gate-wait on a gate that died without a verdict: NO RESULT (4), not STILL RUNNING"
says 'no longer running and wrote no verdict' "...and says it was killed or crashed"
gw 0 "$d/nops:$d/bin:$np"; rc=$?
expect_rc 4 "$rc" "...and the same where ps cannot run (kill -0: no such process)"
printf 'started pid=%s at=1 mode=full subject=worktree head=%s tree=%s script=run-local.sh\n' "$$" "$SHA" "$SHA" > "$STATE"
if ps -p $$ -o command= >/dev/null 2>&1; then
  gw; rc=$?
  expect_rc 4 "$rc" "...and a live pid that is not a gate (a reused pid) is not a running gate"
else fail "cannot check that a reused pid is not read as a running gate: ps does not run here (a sandbox?); run the checks outside it"; fi

# Interrupted: a signal mid-run is recorded as `signal` (a fail), never as the pass bash's $? = 0 implies.
# The gate runs alone here (CLAUDUCTOR_LOCK_HELD names a lease made by hand), so its pid is the run's.
mkdir -p "$LOCK" "$R/local"; touch "$R/local/HOLD"
(cd "$R" && PATH="$d/bin:$np" CI= CLAUDUCTOR_LANE=check CLAUDUCTOR_LOCK_HELD="$LOCK" exec bash "$RUN_REL") > "$d/bg" 2>&1 &
bg=$!
n=0; while ! grep -q '^==> hold' "$d/bg" 2>/dev/null && [ "$n" -lt 30 ]; do sleep 1; n=$((n + 1)); done
kill -TERM "$bg" 2>/dev/null; wait "$bg" 2>/dev/null
rm -f "$R/local/HOLD"; rm -rf "$LOCK"
grep -q '^exit pid=[0-9]* code=signal ' "$STATE" && tail -n 1 "$RUNS" | awk -F'\t' '$6 == "fail" && $7 == "signal" { ok = 1 } END { exit !ok }' \
  && ok "a gate killed mid-run is recorded as interrupted (code=signal, fail), not as a pass" || fail "after TERM: state $(tr '\n' '|' < "$STATE"); log '$(tail -n 1 "$RUNS")'"
gw; rc=$?
expect_rc 1 "$rc" "gate-wait on an interrupted gate: FAIL (1)"

# A run that ends before any step (no GATE_STEPS) logs its steps field as `-`, never empty.
mv "$R/$STEPS_REL" "$d/steps.away"; run --quick; mv "$d/steps.away" "$R/$STEPS_REL"
tail -n 1 "$RUNS" | awk -F'\t' 'NF == 10 && $6 == "fail" && $10 == "-" { ok = 1 } END { exit !ok }' \
  && ok "a run that ran no step logs its steps as '-'" || fail "run log with no step: '$(tail -n 1 "$RUNS")'"

# Bounded: past GATE_RUNS_MAX_BYTES the log becomes gate-runs.log.1 and a new one starts.
# Only a full run rotates, under the lease; a --quick run, unleased (OPS-103), only appends.
rm -f "$RUNS.1"; before=$(lines "$RUNS")
(cd "$R" && PATH="$d/bin:$np" CI= CLAUDUCTOR_LANE=check GATE_RUNS_MAX_BYTES=1 bash "$RUN_REL" --quick) >/dev/null 2>&1
[ ! -e "$RUNS.1" ] && [ "$(lines "$RUNS")" = $((before + 1)) ] && ok "a --quick run past GATE_RUNS_MAX_BYTES appends and never rotates (it holds no lease)" || fail "quick rotation: log $(lines "$RUNS") lines (was $before), .1 $(lines "$RUNS.1") lines"
(cd "$R" && PATH="$d/bin:$np" CI= CLAUDUCTOR_LANE=check GATE_RUNS_MAX_BYTES=1 bash "$RUN_REL" --bogus --quick) >/dev/null 2>&1
[ ! -e "$RUNS.1" ] && tail -n 1 "$RUNS" | cut -f5 | grep -qx quick && ok "...nor does one that ends before its flags are read (logged as quick)" || fail "early-exit quick run: .1 $(lines "$RUNS.1") lines, logged as '$(tail -n 1 "$RUNS" | cut -f5)'"
(cd "$R" && PATH="$d/bin:$np" CI= CLAUDUCTOR_LANE=check GATE_RUNS_MAX_BYTES=1 bash "$RUN_REL") >/dev/null 2>&1
[ -s "$RUNS.1" ] && [ "$(lines "$RUNS")" = 1 ] && ok "past GATE_RUNS_MAX_BYTES a full run rotates the log to gate-runs.log.1 and starts again" || fail "rotation: log $(lines "$RUNS") lines, .1 $(lines "$RUNS.1") lines"

# A project.conf that names a variable the run state does not set ($ROOT, read before conf.sh runs)
# still gates: recording the run must never stop the gate.
printf 'PROJECT_NOTES="$ROOT/docs/notes"\n' >> "$R/.claude/project.conf"
# ROOT unset, as in a real gate: checks/run.sh exports it, which would hide the failure.
rm -f "$STATE"; (unset ROOT; run --quick); rc=$?
git -C "$R" checkout -q -- .claude/project.conf
expect_rc 0 "$rc" "a project.conf naming \$ROOT does not stop the gate (the run state reads it without nounset)"
grep -q '^started .* mode=quick ' "$STATE" && ok "...and the run is still recorded" || fail "no started line after a project.conf naming \$ROOT: $(tr '\n' '|' < "$STATE" 2>/dev/null)"

# gate-wait takes whole seconds only, and says so: anything else would never compare as elapsed.
run --quick
out=$(cd "$R" && PATH="$d/bin:$np" GATE_WAIT_SECONDS=abc GATE_WAIT_POLL=x sh "$WAIT_REL" 2>&1); rc=$?
expect_rc 0 "$rc" "gate-wait with a non-numeric GATE_WAIT_SECONDS and GATE_WAIT_POLL still answers (PASS after the run above)"
case "$out" in *"GATE_WAIT_SECONDS=abc is not whole seconds"*"GATE_WAIT_POLL=x is not whole seconds"*) ok "...and says which values it ignored" ;; *) fail "gate-wait did not name the ignored values: $out" ;; esac

# A signal AFTER the verdict keeps it: TERM lands while a passing full run publishes its status (the
# ci-status module, here a stub that waits), and the run is recorded as the pass it was.
mkdir -p "$R/.claude/modules/ci-status/scripts" "$R/local"
printf '#!/bin/sh\necho "$PPID" > local/gate.pid\nwhile [ -f local/PUBHOLD ]; do sleep 1; done\n' > "$R/.claude/modules/ci-status/scripts/publish-status.sh"
printf 'MODULES="ci-status"\n' >> "$R/.claude/project.conf"
git -C "$R" add -A && git -C "$R" commit -qm "a ci-status stub"
rm -f "$R/local/gate.pid"; touch "$R/local/PUBHOLD"
(cd "$R" && PATH="$d/bin:$np" CI= CLAUDUCTOR_LANE=check exec bash "$RUN_REL") > "$d/bg" 2>&1 &
bg=$!
n=0; while [ ! -s "$R/local/gate.pid" ] && [ "$n" -lt 60 ]; do sleep 1; n=$((n + 1)); done
[ -s "$R/local/gate.pid" ] && kill -TERM "$(cat "$R/local/gate.pid")" 2>/dev/null
rm -f "$R/local/PUBHOLD"; wait "$bg" 2>/dev/null
grep -q '^exit pid=[0-9]* code=0 ' "$STATE" && tail -n 1 "$RUNS" | awk -F'\t' '$6 == "pass" && $7 == "0" { ok = 1 } END { exit !ok }' \
  && ok "a signal after the verdict (while publishing) keeps the pass: code=0, not signal" || fail "signal after the verdict: state $(tr '\n' '|' < "$STATE"); log '$(tail -n 1 "$RUNS")'; $(grep '^==>' "$d/bg" | tr '\n' ' ')"

# A cleanup that fails (a clean room the cleanup cannot remove) still writes the exit line.
printf 'GATE_CLEAN_ROOM="archive"\nMODULES=""\n' >> "$R/.claude/project.conf"
touch "$R/LOCK_CLEANUP"; git -C "$R" add -A && git -C "$R" commit -qm "a step that locks its output"
mkdir -p "$d/tmp"; rm -f "$STATE"
(cd "$R" && PATH="$d/bin:$np" CI= CLAUDUCTOR_LANE=check TMPDIR="$d/tmp" bash "$RUN_REL") > "$d/out" 2>&1
chmod -R u+w "$d/tmp" 2>/dev/null; rm -rf "$d/tmp"
grep -q '^exit pid=[0-9]* code=0 ' "$STATE" && ok "a cleanup that fails still leaves the run's exit line" || fail "failing cleanup: state $(tr '\n' '|' < "$STATE" 2>/dev/null); $(grep '^==>' "$d/out" | tr '\n' ' ')"

# Every place that tells an agent to run the gate says how to follow one the tool backgrounds. Read
# from the model's own copy: the project's, or the plugin's (CLAUDUCTOR_FW).
for f in workflows/build-change.js skills/apply-change/SKILL.md; do
  p="${CLAUDUCTOR_FW:-$ROOT/.claude}/$f"
  if [ ! -f "$p" ]; then fail "$p is missing: nothing tells its agents how to run the gate"
  elif grep -q 'gate-wait.sh' "$p" && grep -q 'exit 3 means call it again' "$p"; then ok "$f tells its agent to follow a backgrounded gate with gate-wait.sh, and to stop on any exit but 3"
  else fail "$f does not name gate-wait.sh with 'exit 3 means call it again': an agent whose gate the tool backgrounds is told nothing, or waits forever on a 4"; fi
done
finish
