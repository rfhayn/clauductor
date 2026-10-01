#!/bin/sh
# owner-queue.sh: print every OPEN item in the owner queue (OWNER_QUEUE in .claude/project.conf,
# default docs/owner-queue.md). session-start prints it first, and the panel pins it as a card.
#
# WHY. Work that needs the owner at the computer (an interactive login, a production deploy, a
# decision that needs a screen) cannot run in a hands-off session, and a deferral that lives only
# in a journal is read once and forgotten (AGENTS.md rule 1: a deferral needs an owner that is
# SEEN). This puts every open item in front of the owner at the start of every session until it
# is ticked.
#
# An open item is a line starting `- [ ]`, in any section. A missing file is reported, never read
# as "nothing queued": absence must not read as healthy.
#
# Usage: sh .claude/owner-queue.sh [path]      Checked by .claude/checks/owner-queue.sh.

ROOT=$(cd "$(dirname "$0")/.." 2>/dev/null && pwd)
# shellcheck disable=SC1091
. "$ROOT/.claude/lib/conf.sh"

f="${1:-$ROOT/$OWNER_QUEUE}"
if [ ! -f "$f" ]; then
  echo "CANNOT CHECK: $f is missing, so the $OWNER_ROLE queue is unknown (not empty)."
  exit 0
fi
open=$(grep -E '^[[:space:]]*- \[ \]' "$f" | sed -E 's/^[[:space:]]*- \[ \][[:space:]]*/• /')
if [ -z "$open" ]; then
  echo "nothing queued for the $OWNER_ROLE ($f)."
else
  n=$(printf '%s\n' "$open" | wc -l | tr -d ' ')
  echo "$n item(s) need the $OWNER_ROLE at the computer; say each one in the summary ($f):"
  printf '%s\n' "$open"
fi
