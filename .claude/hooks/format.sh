#!/bin/sh
# PostToolUse hook (matcher: Write|Edit). Formats the file Claude just wrote with the project's
# formatter, so formatting never reaches review as a finding and never needs a sweeping
# reformat commit.
#
# Configured in .claude/project.conf: FORMAT_CMD (the formatter, given the file as its last
# argument, e.g. `gofmt -w`, `npx --no-install prettier --write`, `ruff format`) and FORMAT_EXT
# (space-separated extensions it owns). With FORMAT_CMD empty this does nothing.
#
# Never blocks and never prints: a formatter failure must not stop an edit, and a PostToolUse
# hook's output on exit 0 reaches only the debug log anyway. The gate's lint step is what
# catches a file the formatter could not handle.

ROOT=$(cd "$(dirname "$0")/../.." 2>/dev/null && pwd)
# shellcheck disable=SC1091
. "$ROOT/.claude/lib/conf.sh"
[ -n "${FORMAT_CMD:-}" ] || exit 0
command -v jq >/dev/null 2>&1 || exit 0

f=$(jq -r '.tool_input.file_path // empty' 2>/dev/null) || exit 0
[ -n "$f" ] && [ -f "$f" ] || exit 0

ext=${f##*.}
case " ${FORMAT_EXT:-} " in *" $ext "*) ;; *) exit 0 ;; esac

# Run from the file's own checkout, so a formatter resolved relative to the repo (a local
# node_modules/.bin, a venv) is the one that checkout carries.
dir=$(git -C "$(dirname "$f")" rev-parse --show-toplevel 2>/dev/null) || dir=$ROOT
# FORMAT_CMD unquoted on purpose: it is a command and its flags.
# shellcheck disable=SC2086
(cd "$dir" && $FORMAT_CMD "$f") >/dev/null 2>&1 || true
exit 0
