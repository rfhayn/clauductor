#!/bin/sh
# stridal-model.sh SCRIPT [ARGS]: run one of the stridal plugin's scripts from outside a
# Claude Code session: the gate (steps.sh runs the process checks this way), the panel's lanes, CI.
#
# This repository runs the operating model from the stridal PLUGIN, so the checks live in the
# plugin, not here. Where the plugin is, in order:
#   1. $STRIDAL_PLUGIN_ROOT        a plugin directory you name
#   2. `stridal-model root`        on the PATH inside a Claude Code session
#   3. the path the plugin recorded at its last session start, in its data directory
#   4. in CI ($CI set, as GitHub Actions and most CI services do) or with STRIDAL_FETCH=1:
#      the plugin at its pinned version, cloned once into a cache directory:
#        STRIDAL_REF       the tag or branch to clone      (default v@VERSION@, the version
#                                                              /stridal:init scaffolded from)
#        STRIDAL_REPO_URL  where from                      (default github.com/rfhayn/stridal)
#        STRIDAL_CACHE     where to keep it                (default ~/.cache/stridal; cache
#                                                              it between CI runs to skip the clone)
# None found is a FAILURE, not a skip: a check that did not run must not read as one that passed.
#
# Scaffolded by /stridal:init; the repository owns it from then on (bump STRIDAL_REF's
# default when you update the plugin).
root=${STRIDAL_PLUGIN_ROOT:-}
if [ -z "$root" ] && command -v stridal-model >/dev/null 2>&1; then
  root=$(stridal-model root 2>/dev/null)
fi
if [ -z "$root" ]; then
  rec="${CLAUDE_CODE_PLUGIN_CACHE_DIR:-${CLAUDE_CONFIG_DIR:-$HOME/.claude}/plugins}/data/stridal-stridal/root"
  [ -f "$rec" ] && root=$(cat "$rec")
fi
if [ -z "$root" ] && { [ -n "${CI:-}" ] || [ "${STRIDAL_FETCH:-}" = 1 ]; }; then
  ref=${STRIDAL_REF:-v@VERSION@}
  url=${STRIDAL_REPO_URL:-https://github.com/rfhayn/stridal.git}
  dir="${STRIDAL_CACHE:-${XDG_CACHE_HOME:-$HOME/.cache}/stridal}/$(printf '%s' "$ref" | tr '/' '-')"
  if [ ! -f "$dir/plugin/.claude-plugin/plugin.json" ]; then
    rm -rf "$dir.tmp" && mkdir -p "$(dirname "$dir")" &&
      git clone -q --depth 1 --branch "$ref" "$url" "$dir.tmp" >&2 &&
      rm -rf "$dir" && mv "$dir.tmp" "$dir" ||
      echo "stridal-model.sh: could not clone $url at $ref" >&2
  fi
  [ -f "$dir/plugin/.claude-plugin/plugin.json" ] && root="$dir/plugin"
fi
if [ -z "$root" ] || [ ! -f "$root/${1:-}" ]; then
  echo "FAIL stridal-model.sh: cannot find the stridal plugin's ${1:-<script>} (tried \$STRIDAL_PLUGIN_ROOT, stridal-model on PATH, the plugin's recorded root$( { [ -n "${CI:-}" ] || [ "${STRIDAL_FETCH:-}" = 1 ]; } && echo ", a clone at ${STRIDAL_REF:-v@VERSION@}")). In CI it clones github.com/rfhayn/stridal at STRIDAL_REF; locally, start one Claude Code session with the plugin enabled, or set STRIDAL_FETCH=1." >&2
  exit 1
fi
s=$1; shift
exec sh "$root/$s" "$@"
