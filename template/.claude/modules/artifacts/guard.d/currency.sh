#!/bin/sh
# The artifacts module's merge-guard rule (Standing Tee's rule 8): a session CLOSE cannot land while
# a core artifact is BEHIND its sources. BLOCKING. Run by pr-merge-guard.sh's extension loop with
# GUARD_HEAD, GUARD_BRANCH and ROOT in the environment (.claude/local/README.md has the contract).
#
# WHY THE CLOSE AND NOT EVERY PR. A change PR moves an authority and the page follows at the close,
# the one PR per session whose job is to leave the shared pages true. Blocking every PR would force
# a page review into the middle of a build. The price: a session that never closes leaves a page
# behind, and the next session-start prints it. A close branch is `<BRANCH_OPS>session-<N>-close`
# (session-close step 7), matched as `session-[0-9]*-close*`, so `-close-2` and `-close-addendum`
# count, and session TOOLING branches (`ops/session-start-x`) do not.
#
# READ AT THE PR'S HEAD COMMIT (registry, stamps and authorities all from there), never the local
# tree: a local file can be stale in either direction. A head with no registry is skipped, saying
# so. That the head contains origin/main (so the head's tree is what the squash lands) is the core
# guard's to require; this rule judges the head it is given.
#
# A stamp clears a line: "refreshed: <what>" or "reviewed, no change: <why>" (currency.sh --stamp).
# BOUNDED, AND FAILS CLOSED: the check runs under ARTIFACT_RULE_SECONDS (default 40, inside the
# extension loop's own GUARD_RULE_TIMEOUT), and a check that is stopped, or cannot run, blocks.
ROOT=$(cd "$(dirname "$0")/../../../.." 2>/dev/null && pwd)
# shellcheck disable=SC1091
. "$ROOT/.claude/lib/conf.sh"
# shellcheck disable=SC1091
. "$ROOT/.claude/lib/modules.sh"
TOP=$ROOT
# How a person runs the tool: the template's path, or the plugin's command.
self=".claude/modules/artifacts/bin/currency.sh"
case ${CLAUDUCTOR_FW:-} in '' | */.claude) run="sh $self" ;; *) run="clauductor-model modules/artifacts/bin/currency.sh" ;; esac

case ${GUARD_BRANCH:-} in
  "${BRANCH_OPS:-ops/}"session-[0-9]*-close*) ;;
  *) exit 0 ;;
esac
hd=${GUARD_HEAD:?the guard passes GUARD_HEAD}
reg=${ARTIFACT_REGISTRY:-docs/artifacts.json}
cur="$ROOT/.claude/modules/artifacts/bin/currency.sh"

git -C "$TOP" cat-file -e "$hd^{commit}" 2>/dev/null \
  || { echo "cannot find the head $hd locally, so the core-artifact currency rule cannot be evaluated. Run: git fetch origin pull/${GUARD_PR:-<n>}/head" >&2; exit 2; }
if ! git -C "$TOP" cat-file -e "$hd:$reg" 2>/dev/null; then
  # Deleting the registry must not be the way past this rule.
  if git -C "$TOP" cat-file -e "origin/${MAIN_BRANCH:-main}:$reg" 2>/dev/null; then
    echo "PR #${GUARD_PR:-?} (${GUARD_BRANCH}) is a session close whose head has no $reg, which origin/${MAIN_BRANCH:-main} has: the core artifacts cannot be judged. Restore it, or turn the artifacts module off in .claude/project.conf (MODULES) in its own PR." >&2
    exit 2
  fi
  echo "no $reg at the head: no core artifacts to hold current."
  exit 0
fi
[ -f "$cur" ] || { echo "cannot find $cur, so the core-artifact currency rule cannot be evaluated. Restore it." >&2; exit 2; }
command -v jq >/dev/null 2>&1 || { echo "jq is not installed, so the core-artifact currency rule cannot be evaluated." >&2; exit 2; }

out=$(mktemp "${TMPDIR:-/tmp}/artifacts-guard.XXXXXX") || { echo "mktemp failed, so the core-artifact currency rule cannot be evaluated." >&2; exit 2; }
secs=${ARTIFACT_RULE_SECONDS:-40}
with_timeout "$secs" sh "$cur" --root "$TOP" --ref "$hd" --check > "$out" 2>&1
rc=$?
verdict=$(cat "$out"); rm -f "$out"
case $rc in
  0) echo "every core artifact is current with its sources at the head ($(printf '%s' "$hd" | cut -c1-9))." ; exit 0 ;;
  1) printf '%s\n' "PR #${GUARD_PR:-?} (${GUARD_BRANCH}) is a session close, and not every core artifact is current with its sources:" \
       "$verdict" \
       "For each line that is not OK: refresh the artifact and stamp it, or stamp it 'reviewed, no change: <why>' ($run --stamp <key> --note \"...\"). Commit, re-run the gate, then merge." >&2
     exit 2 ;;
  143 | 137) echo "the core-artifact currency check did not finish within $secs s and was stopped, so PR #${GUARD_PR:-?} cannot be judged (it fails closed). Run: $run --ref $hd --check, and look at what is slow." >&2; exit 2 ;;
  *) printf 'the core-artifact currency check could not run (exit %s), so PR #%s cannot be judged: %s\n' "$rc" "${GUARD_PR:-?}" "$(printf '%s' "$verdict" | tail -n 1)" >&2; exit 2 ;;
esac
