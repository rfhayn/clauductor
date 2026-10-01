#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# The ci-status module's parts, whether it is on here or not, in a throwaway repo with a stub gh
# that logs every call (no network, no GitHub):
#   - publish-status.sh posts exactly one status per verdict, to the commit named, with the
#     configured context and description: local pass|fail, and github <job>=<result>... (success
#     only when every job succeeded; skipped is not success); descriptions cut at 140;
#   - it refuses a context missing from GATE_DISPLAY_CONTEXTS, and never fails its caller nor goes
#     quiet: no gh, no repo, an unpushed commit, a failed POST, a bad mode each print and exit 0;
#   - the model's runner (GATE_RUN) posts success after a full clean pass, failure after a failed
#     full run, both to the commit it TESTED (a commit made during the run gets neither), and
#     nothing for --quick, --dirty, a tree that changed, or while the module is off.
# That the merge guard ignores these statuses both ways is checks/merge-guard.sh's.
. "$(dirname "$0")/lib.sh"
need git bash

PS="$CLAUDUCTOR_FW/modules/ci-status/scripts/publish-status.sh"
d=$(scratch)
mkdir -p "$d/bin" "$d/nogh"
# gh: every call's argv on one line in $GH_LOG; a status POST also as "<sha> <state> <context>|<description>"
# in $GH_POSTS. GH_FAIL names a step to fail: repo, commit or post.
cat > "$d/bin/gh" <<'EOF'
#!/bin/sh
echo "$*" >> "$GH_LOG"
case "$1 $2" in
  "repo view") [ "$GH_FAIL" = repo ] && exit 1; echo acme/app ;;
  "api "*)
    case "$2" in
      */statuses/*)
        [ "$GH_FAIL" = post ] && { echo "HTTP 403"; exit 1; }
        st="" cx="" ds=""
        for a in "$@"; do case $a in state=*) st=${a#state=} ;; context=*) cx=${a#context=} ;; description=*) ds=${a#description=} ;; esac; done
        echo "${2##*/} $st $cx|$ds" >> "$GH_POSTS"; echo "$cx" ;;
      */commits/*) [ "$GH_FAIL" = commit ] && exit 1; echo "${2##*/}" ;;
    esac ;;
esac
exit 0
EOF
chmod +x "$d/bin/gh"
# PATH without clauductor, gitleaks or a real gh: the stub goes first where it is wanted.
np=""; IFS_OLD=$IFS; IFS=:
for p in $PATH; do [ -x "$p/clauductor" ] || [ -x "$p/gitleaks" ] || [ -x "$p/gh" ] && continue; np="$np${np:+:}$p"; done
IFS=$IFS_OLD

# ── The publisher ───────────────────────────────────────────────────────────────────────
P="$d/pub"; new_repo "$P"
mkdir -p "$P/.claude/lib"
cp "$CLAUDUCTOR_FW/lib/conf.sh" "$CLAUDUCTOR_FW/lib/modules.sh" "$P/.claude/lib/"
cp -R "$CLAUDUCTOR_FW/modules" "$P/.claude/modules"
printf 'MODULES="ci-status"\nGATE_REMOTE_WORKFLOW="ci.yml"\n' > "$P/.claude/project.conf"
git -C "$P" add -A && git -C "$P" commit -qm init
SHA=$(git -C "$P" rev-parse HEAD)
pub() {  # pub [VAR=value...] -- ARGS: run the module's publisher in $P; posts in $d/posts, stderr in $d/err
  : > "$d/posts"; : > "$d/log"
  envs=""; while [ "$1" != -- ]; do envs="$envs $1"; shift; done; shift
  # shellcheck disable=SC2086
  (cd "$P" && env PATH="$d/bin:$np" GH_LOG="$d/log" GH_POSTS="$d/posts" GH_FAIL= $envs sh .claude/modules/ci-status/scripts/publish-status.sh "$@") 2>"$d/err"
}
posts() { cat "$d/posts"; }
is() {  # is WANT GOT LABEL
  [ "$1" = "$2" ] && ok "$3" || fail "$3: got '$2', want '$1' (stderr: $(cat "$d/err"))"
}

pub -- local pass; rc=$?
expect_rc 0 "$rc" "local pass exits 0"
is "$SHA success ci/local|scripts/ci/run-local.sh passed in full on a clean tree, in the author's clone" "$(posts)" "local pass posts ci/local=success on HEAD, saying where the result came from"
grep -q "posted ci/local=success on $(printf %.9s "$SHA")" "$d/err" && ok "...and prints what it posted" || fail "no 'posted' line: $(cat "$d/err")"
pub SHA=0123456789abcdef0123456789abcdef01234567 CI_STATUS_LOCAL_FAIL=boom -- local fail
is "0123456789abcdef0123456789abcdef01234567 failure ci/local|boom" "$(posts)" "local fail posts failure on the SHA it is given, in the configured words"
# The exact calls, in order: the repo, the commit is on the remote, then the one POST.
cat > "$d/want" <<EOF
repo view --json nameWithOwner --jq .nameWithOwner
api repos/acme/app/commits/$SHA --jq .sha
api repos/acme/app/statuses/$SHA -X POST -f state=success -f context=ci/local -f description=scripts/ci/run-local.sh passed in full on a clean tree, in the author's clone --jq .context
EOF
pub -- local pass
cmp -s "$d/want" "$d/log" && ok "the payload is three gh calls: the repo, the commit, one status POST" || { fail "gh calls differ:"; diff "$d/want" "$d/log" | sed 's/^/     /'; }
pub -- github verify=success e2e=success
is "$SHA success ci/github|ci.yml passed on GitHub: verify and e2e" "$(posts)" "github: every job success posts ci/github=success, naming the workflow and jobs"
pub -- github verify=success e2e=failure
is "$SHA failure ci/github|ci.yml did not pass on GitHub — verify=success, e2e=failure" "$(posts)" "github: one failed job posts failure, naming each result"
pub -- github verify=skipped e2e=success
is "failure" "$(posts | cut -d' ' -f2)" "github: a skipped job is not success"
long=$(printf 'x%.0s' $(seq 1 200))
pub CI_STATUS_LOCAL_PASS="$long" -- local pass
is 140 "$(posts | sed 's/^[^|]*|//' | tr -d '\n' | wc -c | tr -d ' ')" "a description is cut at 140 characters, as GitHub would"

# Refusals: each prints why, exits 0 and posts nothing.
refuse() {  # refuse LABEL WANT-IN-STDERR [VAR=value...] -- ARGS
  label=$1 want=$2; shift 2
  pub "$@"; rc=$?
  if [ "$rc" -eq 0 ] && [ ! -s "$d/posts" ] && grep -q -- "$want" "$d/err"; then ok "$label: exits 0, posts nothing, says why"
  else fail "$label: rc $rc, posts '$(posts)', stderr '$(cat "$d/err")' (want '$want')"; fi
}
cp "$P/.claude/project.conf" "$d/conf.keep"
echo 'GATE_DISPLAY_CONTEXTS="ci/other"' >> "$P/.claude/project.conf"
refuse "a context not in GATE_DISPLAY_CONTEXTS" "is not in GATE_DISPLAY_CONTEXTS" -- local pass
cp "$d/conf.keep" "$P/.claude/project.conf"
refuse "a context the project renamed but did not list" "is not in GATE_DISPLAY_CONTEXTS" CI_STATUS_LOCAL_CONTEXT=ci/mine -- local pass
mv "$d/bin/gh" "$d/gh.off"; (cd "$P" && env PATH="$np" sh .claude/modules/ci-status/scripts/publish-status.sh local pass) 2>"$d/err"; rc=$?; mv "$d/gh.off" "$d/bin/gh"
[ "$rc" -eq 0 ] && grep -q 'gh not installed' "$d/err" && ok "without gh on PATH: exits 0 and says so" || fail "without gh: rc $rc, $(cat "$d/err")"
refuse "gh cannot name the repo" "could not identify the repo" GH_FAIL=repo -- local pass
refuse "a commit not on the remote" "is not on the remote yet" GH_FAIL=commit -- local pass
pub GH_FAIL=post -- local pass; rc=$?
[ "$rc" -eq 0 ] && grep -q 'FAILED to post ci/local=success' "$d/err" && ok "a failed POST: exits 0 and prints FAILED with gh's answer" || fail "failed POST: rc $rc, $(cat "$d/err")"
refuse "no mode" "no mode given" --
refuse "a bad local verdict" "needs pass|fail" -- local maybe
refuse "a github argument without =" "is not <job>=<result>" -- github success
refuse "an unknown mode" "unknown mode" -- remote pass

# ── The runner ──────────────────────────────────────────────────────────────────────────
RUN_REL=$GATE_RUN; RUN_DIR=$(dirname "$RUN_REL")
if ! grep -q '^# run-local.sh: THE full gate' "$ROOT/$RUN_REL" 2>/dev/null; then
  ok "GATE_RUN=$RUN_REL is the project's own runner: ci-status:display checks that it calls the publisher"
  finish
fi
R="$d/repo"; new_repo "$R"
mkdir -p "$R/$RUN_DIR/lib" "$R/.claude/lib"
cp "$ROOT/$RUN_REL" "$R/$RUN_REL"
cp "$ROOT/$RUN_DIR/lease.sh" "$R/$RUN_DIR/lease.sh"
cp "$ROOT/$RUN_DIR/lib/steps.sh" "$R/$RUN_DIR/lib/steps.sh"
cp "$CLAUDUCTOR_FW/lib/conf.sh" "$CLAUDUCTOR_FW/lib/change.sh" "$CLAUDUCTOR_FW/lib/modules.sh" "$R/.claude/lib/"
cp "$CLAUDUCTOR_FW/scenario-trace.sh" "$R/.claude/"
cp -R "$CLAUDUCTOR_FW/modules" "$R/.claude/modules"
printf 'MODULES="ci-status"\nGATE_RUN="%s"\nGATE_STEPS="ci-steps.sh"\n' "$RUN_REL" > "$R/.claude/project.conf"
# The steps: one that fails on FAIL_UNIT, one that commits mid-run on COMMIT_MID_RUN, one that takes
# the lease away (another holder's nonce) on STEAL_LEASE.
cat > "$R/ci-steps.sh" <<'EOF'
gate_steps() {
  if [ -f COMMIT_MID_RUN ]; then
    echo "later $$" >> later.txt; git add later.txt; git commit -qm "committed while the gate ran"
  fi
  step "unit" sh -c '[ ! -f FAIL_UNIT ]' || return 1
  if [ -f STEAL_LEASE ]; then
    sed 's/"nonce":"[0-9a-f]*"/"nonce":"0123456789abcdef"/' "$CLAUDUCTOR_LOCK_HELD/owner.json" > "$CLAUDUCTOR_LOCK_HELD/x"
    mv "$CLAUDUCTOR_LOCK_HELD/x" "$CLAUDUCTOR_LOCK_HELD/owner.json"
  fi
  return 0
}
EOF
printf 'FAIL_UNIT\nCOMMIT_MID_RUN\nSTEAL_LEASE\n' > "$R/.gitignore"
git -C "$R" add -A && git -C "$R" commit -qm init
# The publisher's "is the commit on the remote" call is the stub's: it answers yes for any SHA.
gate() {  # gate [ARGS]: the full gate in $R; status posts in $d/posts
  : > "$d/posts"; : > "$d/log"
  (cd "$R" && PATH="$d/bin:$np" GH_LOG="$d/log" GH_POSTS="$d/posts" GH_FAIL= CI= CLAUDUCTOR_LANE=check bash "$RUN_REL" "$@") > "$d/out" 2>&1
}
st() { cut -d' ' -f1,2 "$d/posts"; }
T=$(git -C "$R" rev-parse HEAD)

gate; rc=$?
expect_rc 0 "$rc" "a full clean run passes"
is "$T success" "$(st)" "a full clean pass posts ci/local=success on the tested commit"
gate --quick
is "" "$(st)" "a --quick run posts nothing"
touch "$R/FAIL_UNIT"; gate; rc=$?
expect_rc 1 "$rc" "a failing full run fails"
is "$T failure" "$(st)" "a failing full run posts failure on the tested commit (the green is retracted with the receipt)"
gate --quick
is "" "$(st)" "a failing --quick run posts nothing"
gate --dirty
is "" "$(st)" "a failing --dirty run posts nothing (it tested the working tree, not the commit)"
rm -f "$R/FAIL_UNIT"
gate --dirty
is "" "$(st)" "a passing --dirty run posts nothing"
echo "x" >> "$R/ci-steps.sh"; gate; git -C "$R" checkout -q -- ci-steps.sh
is "" "$(st)" "a pass over an uncommitted change posts nothing (its receipt says dirty)"
touch "$R/COMMIT_MID_RUN"; gate; rc=$?; rm -f "$R/COMMIT_MID_RUN"
[ "$(git -C "$R" rev-parse HEAD)" != "$T" ] && ok "(the step really moved HEAD)" || fail "the mid-run commit did not happen"
is "" "$(st)" "a pass during which HEAD moved posts no green, to either commit"
T=$(git -C "$R" rev-parse HEAD)
touch "$R/COMMIT_MID_RUN" "$R/FAIL_UNIT"; gate; rm -f "$R/COMMIT_MID_RUN" "$R/FAIL_UNIT"
[ "$(git -C "$R" rev-parse HEAD)" != "$T" ] && ok "(the failing run's step really moved HEAD too)" || fail "the failing run's mid-run commit did not happen"
is "$T failure" "$(st)" "a failing run during which a commit lands posts its red to the TESTED commit, not the new one"
git -C "$R" reset -q --hard "$T"
touch "$R/STEAL_LEASE"; gate; rc=$?; rm -f "$R/STEAL_LEASE"
expect_rc 70 "$rc" "a run that lost its gate lease fails"
is "$T failure" "$(st)" "...and posts failure, never the green its steps earned"
printf 'MODULES=""\nGATE_RUN="%s"\nGATE_STEPS="ci-steps.sh"\n' "$RUN_REL" > "$R/.claude/project.conf"
git -C "$R" commit -qam "module off"
gate; rc=$?
expect_rc 0 "$rc" "with the module off, a full clean run passes"
is "" "$(st)" "...and posts nothing"
[ ! -s "$d/log" ] || [ "$(grep -c . "$d/log")" -eq 0 ] && ok "...and never calls gh" || fail "gh was called with the module off: $(cat "$d/log")"
grep -q 'publish-status' "$d/out" && fail "the runner ran the publisher with the module off: $(grep publish-status "$d/out")" || ok "...and never runs the publisher at all"
finish
