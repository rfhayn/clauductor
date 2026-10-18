#!/bin/sh
# session-start's roadmap-tracker section (OPS-83): one line saying whether the owner's tracker
# (a private dashboard) was last refreshed from today's roadmap: NOT CONFIGURED on a machine with
# no tracker-url (its address is never committed), else NEVER RAN, OK or STALE. It compares the stamp
# roadmap-tracker.sh --mark writes after an upload with the roadmap's blob now. Local git only, so
# it reads the same with CONTEXT_OFFLINE=1: it never fetches and never calls gh.
ROOT=$(cd "$(dirname "$0")/../../../.." 2>/dev/null && pwd)
# shellcheck disable=SC1091
. "$ROOT/.claude/lib/conf.sh"
cd "$ROOT" || { echo "tracker: CANNOT CHECK — cannot enter $ROOT"; exit 0; }
if [ ! -f "$ROADMAP" ]; then
  echo "tracker: CANNOT CHECK — $ROADMAP is missing"
  exit 0
fi
now=$(git -C "$ROOT" hash-object -- "$ROADMAP" 2>/dev/null) || { echo "tracker: CANNOT CHECK — cannot hash $ROADMAP"; exit 0; }
common=$(git -C "$ROOT" rev-parse --path-format=absolute --git-common-dir 2>/dev/null) || { echo "tracker: CANNOT CHECK — not in a git repository"; exit 0; }
stamp="$common/clauductor/tracker-refreshed"
if [ ! -s "$common/clauductor/tracker-url" ]; then
  echo "tracker: NOT CONFIGURED on this machine (no tracker-url)"
  exit 0
fi
if [ ! -s "$stamp" ]; then
  echo "tracker: NEVER RAN, no refresh stamp; refresh it (session-start step)"
  exit 0
fi
was=$(head -n 1 "$stamp")
if [ "$was" = "$now" ]; then
  echo "tracker: OK, current with $ROADMAP"
else
  echo "tracker: STALE, built from $(printf '%.7s' "$was"), $(basename "$ROADMAP") is now $(printf '%.7s' "$now"); refresh it (session-start step)"
fi
exit 0
