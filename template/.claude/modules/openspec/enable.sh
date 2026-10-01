#!/bin/sh
# enable.sh: switch the OpenSpec module on (D2 of the change process). Nothing is migrated: the
# records stay in CHANGES_DIR and SPECS_DIR, and OpenSpec's own layout points at them.
#
#   sh .claude/modules/openspec/enable.sh          create openspec/changes and openspec/specs as
#                                                  symlinks, and openspec/config.yaml from this module
#   sh .claude/modules/openspec/enable.sh --check  only report: the links, and the CLI's version
#
# The CLI must be 1.13 or later: 1.2.0 passes a MODIFIED block that drops a scenario and then
# deletes that scenario on archive, and ignores `skip_specs`. An older CLI is refused, by name.
# Then set PROPOSALS="openspec" in .claude/project.conf; .claude/checks/openspec.sh runs
# `openspec validate --all --strict` in every gate from then on.
ROOT=$(cd "$(dirname "$0")/../../.." 2>/dev/null && pwd)
. "$ROOT/.claude/lib/conf.sh"
MIN=1.13

# ver_ok VERSION: 0 when VERSION (x.y[.z]) is at least MIN.
ver_ok() {
  printf '%s\n' "$1" | awk -F. -v min="$MIN" '{ split(min, m, "."); if ($1 + 0 > m[1] + 0 || ($1 + 0 == m[1] + 0 && $2 + 0 >= m[2] + 0)) exit 0; exit 1 }'
}
bin=${OPENSPEC_BIN:-openspec}
if command -v "$bin" >/dev/null 2>&1; then
  v=$(DO_NOT_TRACK=1 OPENSPEC_TELEMETRY=0 "$bin" --version 2>/dev/null | grep -oE '[0-9]+\.[0-9]+(\.[0-9]+)?' | head -1)
  if [ -n "$v" ] && ver_ok "$v"; then echo "ok   openspec $v (at least $MIN)"
  else
    echo "FAIL openspec is ${v:-of unknown version}; the module needs $MIN or later (1.2.0 deletes scenarios a MODIFIED block leaves out, and ignores skip_specs). Upgrade: brew upgrade openspec, or npm i -g @fission-ai/openspec@latest"
    exit 1
  fi
else
  echo "NOTE the openspec CLI is not installed: the links still work for it later (npm i -g @fission-ai/openspec@latest, $MIN or later)"
fi
[ "${1:-}" = --check ] || mkdir -p "$ROOT/openspec"
link() {  # link NAME TARGET-DIR
  l="$ROOT/openspec/$1"; want="../$2"
  if [ -L "$l" ] && [ "$(readlink "$l")" = "$want" ]; then echo "ok   openspec/$1 -> $want"; return; fi
  if [ "${2#openspec/}" != "$2" ]; then echo "ok   $2 is already under openspec/ (no link needed)"; return; fi
  [ "${3:-}" = --check ] && { echo "FAIL openspec/$1 is not a link to $want"; return 1; }
  [ -e "$l" ] && ! [ -L "$l" ] && { echo "FAIL openspec/$1 exists and is not a link; move its contents to $2 first"; return 1; }
  rm -f "$l" && ln -s "$want" "$l" && echo "made openspec/$1 -> $want"
}
link changes "$CHANGES_DIR" "${1:-}" || exit 1
link specs "$SPECS_DIR" "${1:-}" || exit 1
# The explore skill: shipped here, installed as .claude/skills/explore so Claude Code finds it. A copy,
# not a link (a link into a plugin's cache breaks when the plugin moves); a copy that differs from the
# module's after an update is reported here and by checks/run.sh openspec:project, and refreshed by
# running this again.
sk_src="$ROOT/.claude/modules/openspec/skills/explore/SKILL.md"
# Spelt with the quote before /skills on purpose: the destination is the PROJECT's .claude/, which
# the plugin build must not rewrite to the plugin's own copy.
sk_dst="$ROOT/.claude"/skills/explore/SKILL.md
if [ -f "$sk_dst" ] && cmp -s "$sk_src" "$sk_dst"; then echo "ok   .claude/skills/explore is the module's explore skill"
elif [ "${1:-}" = --check ]; then
  echo "FAIL .claude/skills/explore $( [ -f "$sk_dst" ] && echo 'differs from' || echo 'is not installed from') .claude/modules/openspec/skills/explore (run sh .claude/modules/openspec/enable.sh)"; exit 1
else
  mkdir -p "$(dirname "$sk_dst")" && cp "$sk_src" "$sk_dst" && echo "made .claude/skills/explore (map it in .claude/model-roles.json: \"explore\": \"thinker\", and add its row to the playbook's skills table)"
fi
if [ "${1:-}" != --check ] && [ ! -f "$ROOT/openspec/config.yaml" ]; then
  cp "$ROOT/.claude/modules/openspec/config.yaml" "$ROOT/openspec/config.yaml" && echo "made openspec/config.yaml (adjust its rules)"
fi
[ "${1:-}" = --check ] || echo "next: set PROPOSALS=\"openspec\" in .claude/project.conf"
