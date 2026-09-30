#!/bin/sh
# clauductor-model.sh SCRIPT [ARGS]: run one of the clauductor plugin's scripts from outside a
# Claude Code session: the gate (steps.sh runs the process checks this way), the panel's gate
# lane, CI.
#
# This repository runs the operating model from the clauductor PLUGIN, so the checks live in the
# plugin, not here. Where the plugin is, in order:
#   1. $CLAUDUCTOR_PLUGIN_ROOT        set it in CI: clone github.com/rfhayn/clauductor at the
#                                     release you use and point it at <clone>/plugin
#   2. `clauductor-model root`        on the PATH inside a Claude Code session
#   3. the path the plugin recorded at its last session start, in its data directory
# None found is a FAILURE, not a skip: a check that did not run must not read as one that passed.
#
# Scaffolded by /clauductor:init; the repository owns it from then on.
root=${CLAUDUCTOR_PLUGIN_ROOT:-}
if [ -z "$root" ] && command -v clauductor-model >/dev/null 2>&1; then
  root=$(clauductor-model root 2>/dev/null)
fi
if [ -z "$root" ]; then
  rec="${CLAUDE_CODE_PLUGIN_CACHE_DIR:-${CLAUDE_CONFIG_DIR:-$HOME/.claude}/plugins}/data/clauductor-clauductor/root"
  [ -f "$rec" ] && root=$(cat "$rec")
fi
if [ -z "$root" ] || [ ! -f "$root/${1:-}" ]; then
  echo "FAIL clauductor-model.sh: cannot find the clauductor plugin's ${1:-<script>} (tried \$CLAUDUCTOR_PLUGIN_ROOT, clauductor-model on PATH, the plugin's recorded root). In CI, clone github.com/rfhayn/clauductor and set CLAUDUCTOR_PLUGIN_ROOT=<clone>/plugin; locally, start one Claude Code session with the plugin enabled." >&2
  exit 1
fi
s=$1; shift
exec sh "$root/$s" "$@"
