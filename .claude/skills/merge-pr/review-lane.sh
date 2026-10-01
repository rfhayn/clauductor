#!/bin/sh
# Which review a PR gets, decided from its changed paths (merge-pr, the review step).
#
#   git diff --name-only --no-renames origin/main...HEAD | sh .claude/skills/merge-pr/review-lane.sh
#
# `--no-renames` is required: a rename lists only its DESTINATION, so moving a skill out of
# `.claude/` into `docs/` would show one prose path and hide the deletion.
#
# Prints `review-lane: docs` when EVERY path is prose (the `reviewer-docs` agent reviews it), else
# `review-lane: full` plus the paths that made it full. Full is the default: an empty diff, or any
# path not positively recognised as prose, is full. Prose is `*.md` and nothing else, EXCEPT:
#   - spec deltas and living specs (SPECS_DIR/**, CHANGES_DIR/*/specs/**, openspec/**/specs/**):
#     they change what the system is said to do;
#   - anything under `.claude/`: a SKILL.md or agent file is executable config (frontmatter sets
#     model, effort and tools; a skill's `!` lines run shell);
#   - AGENTS.md and CLAUDE.md: every agent loads them.
# Checked by .claude/checks/review-lane.sh.
set -eu
ROOT=$(cd "$(dirname "$0")/../../.." && pwd)
# shellcheck disable=SC1091
. "$ROOT/.claude/lib/conf.sh"
n=0
full=""
# `|| [ -n "$p" ]`: a last line with no trailing newline is still a path; without it `read` drops
# it, and a one-code-file PR piped from anything but git classifies as docs.
while IFS= read -r p || [ -n "$p" ]; do
  [ -n "$p" ] || continue
  n=$((n + 1))
  case "$p" in
    "$SPECS_DIR"/* | "$CHANGES_DIR"/*/specs/* | openspec/specs/* | openspec/changes/*/specs/* | openspec/changes/archive/*/specs/*) full="$full $p" ;;
    .claude/* | AGENTS.md | CLAUDE.md) full="$full $p" ;;
    *.md) ;;
    *) full="$full $p" ;;
  esac
done
if [ "$n" -eq 0 ]; then
  echo "review-lane: full (no changed paths read; an empty diff is not evidence of a docs-only PR)"
elif [ -z "$full" ]; then
  echo "review-lane: docs ($n path(s), all prose): spawn the reviewer-docs agent"
else
  echo "review-lane: full; non-prose path(s):$full"
fi
