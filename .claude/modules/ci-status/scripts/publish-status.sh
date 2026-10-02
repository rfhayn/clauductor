#!/bin/sh
# publish-status.sh: DRAW a gate's result on its pull request as a commit status (the ci-status
# module). When the gate runs locally, a PR shows no checks at all: the evidence (the receipt in
# .git/ci-receipt) is real, and nothing renders it. A commit status (POST /repos/{o}/{r}/statuses/
# {sha}) works with gh's ordinary token, costs no Actions minutes, and shows in the PR's checks.
#
# ─────────────────────────────────────────────────────────────────────────────────────────────
# THIS IS DISPLAY. IT IS NOT EVIDENCE.
#
# pr-merge-guard.sh rule 2(b) decides a merge by asking two questions BY NAME: did the remote
# workflow (GATE_REMOTE_WORKFLOW) succeed on this exact SHA, or does a receipt in this clone name
# it? A status answers neither, deliberately: anyone with write access can POST one, by hand, so a
# guard that accepted it would be satisfiable with one curl. Rule 2(a) also ignores every context
# in GATE_DISPLAY_CONTEXTS in BOTH directions, so a red status cannot block either. This script
# posts only contexts that list names (it refuses any other, so a status can never start gating),
# and checks/ci-status.sh holds the guard to ignoring them.
#
# The residual risk is a HUMAN: someone who sees green in a browser and merges on it, backed by a
# receipt in a clone they do not have. Every description says where the result came from for that
# reason; set the wording with the CI_STATUS_* keys (module.conf), never to just "passed".
# ─────────────────────────────────────────────────────────────────────────────────────────────
#
# Usage:
#   publish-status.sh local pass|fail              the gate runner, after a FULL run, on the commit
#                                                  it tested (lib/steps.sh model_publish_status)
#   publish-status.sh github <job>=<result>...     a remote workflow's report job, with each job's
#                                                  result (success only when every one is success)
# SHA in the environment names the commit (default HEAD); the caller knows the tested one.
#
# NEVER FAILS ITS CALLER. A cosmetic reporter that can turn a passing gate red is worse than the
# blank PR it exists to fix, so every path exits 0, and every path PRINTS (to stderr), because a
# reporter that fails silently is indistinguishable from one that worked.
ROOT=$(cd "$(dirname "$0")/../../../.." && pwd)
# Defined before any call: a function used before its definition prints `note: not found` and
# nothing else, which is how a reporter goes silent.
note() { echo "publish-status: $1" >&2; }
# shellcheck disable=SC1091
. "$ROOT/.claude/lib/conf.sh" 2>/dev/null || { note "cannot read .claude/lib/conf.sh; nothing posted"; exit 0; }

MODE=${1:-}
[ -n "$MODE" ] || { note "no mode given (expected 'local' or 'github'); nothing posted"; exit 0; }

# The SHA is the commit under test, never "the branch": a status on the wrong commit is the same
# lie as a stale green check, and the caller always knows the right one.
SHA=${SHA:-$(git -C "$ROOT" rev-parse HEAD 2>/dev/null)}
[ -n "$SHA" ] || { note "not in a git repo — nothing posted"; exit 0; }
short=$(printf %.9s "$SHA")

# ── Build the status ──────────────────────────────────────────────────────────────────────────
case $MODE in
  local)
    CONTEXT=${CI_STATUS_LOCAL_CONTEXT:-ci/local}
    case ${2:-} in
      pass) STATE=success; DESC=${CI_STATUS_LOCAL_PASS:-"$GATE_RUN passed in full on a clean tree, in the author's clone"} ;;
      fail) STATE=failure; DESC=${CI_STATUS_LOCAL_FAIL:-"$GATE_RUN failed on this commit, in the author's clone"} ;;
      *) note "mode 'local' needs pass|fail, got '${2:-}'; nothing posted"; exit 0 ;;
    esac
    ;;
  github)
    shift
    CONTEXT=${CI_STATUS_REMOTE_CONTEXT:-ci/github}
    wf=$GATE_REMOTE_WORKFLOW; [ -n "$wf" ] || wf="the remote workflow"
    [ $# -gt 0 ] || { note "mode 'github' needs <job>=<result> pairs; nothing posted"; exit 0; }
    # success only when EVERY job is success. `skipped` and `cancelled` are not success: that
    # conflation is how an opt-in gate reads as satisfied (gh buckets a skipped job as `skipping`).
    allok=1 jobs="" results=""
    for a in "$@"; do
      case $a in *=*) ;; *) note "'$a' is not <job>=<result>; nothing posted"; exit 0 ;; esac
      j=${a%%=*} r=${a#*=}
      [ "$r" = success ] || allok=0
      jobs="$jobs${jobs:+ and }$j"
      results="$results${results:+, }$j=${r:-?}"
    done
    if [ "$allok" = 1 ]; then
      STATE=success; DESC=${CI_STATUS_REMOTE_PASS:-"$wf passed on GitHub: $jobs"}
    else
      STATE=failure; DESC="$wf did not pass on GitHub — $results"
    fi
    ;;
  *) note "unknown mode '$MODE' (expected 'local' or 'github'); nothing posted"; exit 0 ;;
esac

# Only a context the guard ignores: anything else posted here would gate merges one way.
case " $GATE_DISPLAY_CONTEXTS " in
  *" $CONTEXT "*) ;;
  *) note "$CONTEXT is not in GATE_DISPLAY_CONTEXTS (\"$GATE_DISPLAY_CONTEXTS\"), so the merge guard would read it as a check; nothing posted. Add it there."; exit 0 ;;
esac

# GitHub truncates a description over 140 characters; do it here so what posts is what was meant.
DESC=$(printf '%.140s' "$DESC")

command -v gh >/dev/null 2>&1 || { note "gh not installed — no status posted for $short"; exit 0; }
NWO=$(cd "$ROOT" && gh repo view --json nameWithOwner --jq .nameWithOwner 2>/dev/null)
[ -n "$NWO" ] || { note "gh could not identify the repo (not authenticated?) — no status posted for $short"; exit 0; }

# ── Post it ───────────────────────────────────────────────────────────────────────────────────
# A commit not on the remote yet gets a 422 that reads like an auth problem: say the true thing.
if ! gh api "repos/$NWO/commits/$SHA" --jq .sha >/dev/null 2>&1; then
  note "commit $short is not on the remote yet — push the branch, then re-run. Nothing posted."
  exit 0
fi
if out=$(gh api "repos/$NWO/statuses/$SHA" -X POST \
           -f state="$STATE" -f context="$CONTEXT" -f description="$DESC" \
           --jq .context 2>&1); then
  note "posted $CONTEXT=$STATE on $short — $DESC"
else
  note "FAILED to post $CONTEXT=$STATE on $short: $out"
fi
exit 0
