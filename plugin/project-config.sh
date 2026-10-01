#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# project-config.sh: the project's settings that the model's skills and its build-change workflow
# need at run time, read from .claude/project.conf (over .claude/lib/conf.sh's defaults) and
# .claude/model-roles.json. A workflow script cannot read a file, and a skill's prose cannot hold a
# value a project changes; both ask this script instead, so no project edits a framework file to
# change a branch prefix or turn attribution off (OPS-8 rehearsal: build-change.js held both, and
# `clauductor update` then flagged it forever).
#
#   sh .claude/project-config.sh --json            {"branch": {"change","fix","ops","main"}, "changesDir",
#                                                   "specsDir", "gate", "attribution", "provenance"}
#   sh .claude/project-config.sh branch <kind> <name>   the branch for a lane: kind change|fix|ops,
#                                                   e.g. `branch change add-x` prints <BRANCH_CHANGE>add-x
#   sh .claude/project-config.sh [prefixes]        BRANCH_CHANGE=… BRANCH_FIX=… BRANCH_OPS=… MAIN_BRANCH=…
#                                                   (a skill's context line: the skills name branches
#                                                   by these keys, never by a literal prefix)
#
# attribution is the trailer every commit Claude writes ends with ("" when model-roles.json's
# attribution.enabled is false); provenance is model-roles.json's provenance.enabled. Exit 2 when
# it cannot read them (no jq, or model-roles.json missing or invalid): a caller stops rather than
# guess.
ROOT=$(case $CLAUDUCTOR_FW in (*/.claude) dirname "$CLAUDUCTOR_FW" ;; (*) [ -n "${ROOT:-}" ] && echo "$ROOT" || git rev-parse --show-toplevel 2>/dev/null || pwd ;; esac)
. "$CLAUDUCTOR_FW/lib/conf.sh"

case "${1:-}" in
  prefixes | '')
    printf 'BRANCH_CHANGE=%s BRANCH_FIX=%s BRANCH_OPS=%s MAIN_BRANCH=%s\n' "$BRANCH_CHANGE" "$BRANCH_FIX" "$BRANCH_OPS" "$MAIN_BRANCH"
    ;;
  branch)
    kind=${2:-}; name=${3:-}
    case "$kind" in
      change) p=$BRANCH_CHANGE ;;
      fix) p=$BRANCH_FIX ;;
      ops) p=$BRANCH_OPS ;;
      *) echo "usage: project-config.sh branch change|fix|ops <name>" >&2; exit 64 ;;
    esac
    printf '%s%s\n' "$p" "$name"
    ;;
  --json)
    command -v jq >/dev/null 2>&1 || { echo "project-config.sh: jq is not installed" >&2; exit 2; }
    roles="$ROOT/.claude/model-roles.json"
    jq -e 'type == "object"' "$roles" >/dev/null 2>&1 || { echo "project-config.sh: $roles is missing or not JSON" >&2; exit 2; }
    jq -c --arg change "$BRANCH_CHANGE" --arg fix "$BRANCH_FIX" --arg ops "$BRANCH_OPS" --arg main "$MAIN_BRANCH" \
      --arg changes "$CHANGES_DIR" --arg specs "$SPECS_DIR" --arg gate "$GATE" '
      { branch: {change: $change, fix: $fix, ops: $ops, main: $main},
        changesDir: $changes, specsDir: $specs, gate: $gate,
        attribution: (if (.attribution | type) == "object" then (if .attribution.enabled == true then (.attribution.trailer // "") else "" end)
                      elif (.attribution | type) == "string" then .attribution else "" end),
        provenance: (.provenance.enabled == true) }' "$roles"
    ;;
  *)
    echo "usage: project-config.sh --json | prefixes | branch change|fix|ops <name>" >&2
    exit 64
    ;;
esac
