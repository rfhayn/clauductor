#!/bin/sh
# publish.sh: the artifacts module's publishing half, the claude.ai copies of the docs/*.html pages.
# On only with ARTIFACT_PUBLISH="claude.ai"; with "off" (the default) every mode says so and exits 0.
#
#   publish.sh --status [--ref <rev> | --worktree]   OK / STALE / UNREGISTERED / MISSING per page
#   publish.sh --record <page>...                    record the working-tree copy's hash (the close)
#   publish.sh <page> [--ref <rev>] [--out <path>]    write the publish-ready copy; print its path
#   publish.sh --recorded-in <commit>                 "<page> <url>" for each page whose published
#                                                     hash the commit changed (merge-pr's list)
#
# --recorded-in is plain sh and jq: merge-pr derives what to publish from the merge commit, never
# from a session's memory, and that needs no Node. Every other mode runs prep-artifact.mjs (Node),
# because the copy's bytes are its record (prep-artifact.mjs says why). Without Node they say
# CANNOT CHECK and exit 3, never a quiet nothing.
ROOT=$(cd "$(dirname "$0")/../../../.." 2>/dev/null && pwd)
# shellcheck disable=SC1091
. "$ROOT/.claude/lib/conf.sh"
prep="$ROOT/.claude/modules/artifacts/bin/prep-artifact.mjs"
REG=${ARTIFACT_REGISTRY:-docs/artifacts.json}
export ARTIFACT_REGISTRY="$REG"
top=$ROOT
if [ "${1:-}" = --root ]; then top=${2:?--root needs a directory}; shift 2; fi

case ${ARTIFACT_PUBLISH:-off} in
  claude.ai) ;;
  off) echo "publishing is off (ARTIFACT_PUBLISH=off): no claude.ai copies are prepared, recorded or published."; exit 0 ;;
  *) echo "artifacts: ARTIFACT_PUBLISH is \"$ARTIFACT_PUBLISH\"; it must be claude.ai or off" >&2; exit 2 ;;
esac

if [ "${1:-}" = --recorded-in ]; then
  [ $# -eq 2 ] || { echo "artifacts: --recorded-in takes a commit and nothing else" >&2; exit 2; }
  c=$2
  git -C "$top" rev-parse --verify --quiet "$c^{commit}" >/dev/null || { echo "artifacts: cannot resolve $c in $top" >&2; exit 2; }
  # No registry before this commit: it introduced the registry, and its entries are SEEDS describing
  # copies already live, not records awaiting a publish.
  if ! git -C "$top" rev-parse --verify --quiet "$c^" >/dev/null || ! git -C "$top" cat-file -e "$c^:$REG" 2>/dev/null; then
    echo "$REG does not exist before $c: its entries are seeds, nothing to publish" >&2
    exit 0
  fi
  before=$(mktemp "${TMPDIR:-/tmp}/artifacts-before.XXXXXX") || { echo "artifacts: mktemp failed" >&2; exit 2; }
  trap 'rm -f "$before"' EXIT
  git -C "$top" show "$c^:$REG" > "$before" 2>/dev/null || { echo "artifacts: cannot read $REG at $c^" >&2; exit 2; }
  git -C "$top" show "$c:$REG" 2>/dev/null | jq -r --slurpfile b "$before" '
      ($b[0].pages // {}) as $old
      | (.pages // error("no \"pages\" object")) | to_entries | sort_by(.key)[]
      | select(($old[.key].published // null) != .value.published) | "\(.key) \(.value.url)"'
  rc=$?
  [ "$rc" -eq 0 ] || { echo "artifacts: $REG at $c or its parent is not a readable registry" >&2; exit 2; }
  exit 0
fi

command -v node >/dev/null 2>&1 || { echo "CANNOT CHECK — node is not on PATH, and ARTIFACT_PUBLISH=claude.ai prepares the copies with it (prep-artifact.mjs). UNKNOWN, not current."; exit 3; }
exec node "$prep" --root "$top" "$@"
