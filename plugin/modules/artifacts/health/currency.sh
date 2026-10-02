#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/../../.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# Health: is each core artifact (ARTIFACT_REGISTRY) current with the sources it declares, at
# origin/MAIN_BRANCH? One verdict line per artifact that is not, or one OK naming the commit.
#
# A DETECTOR. The control is the module's guard rule, which runs the same check on a session-close
# PR's head and refuses the merge while any artifact is BEHIND; session-close step 3's fragment is
# where a session refreshes or stamps each one. Said here so nobody mistakes this line for the
# control. Never degrades to silence: a check that cannot run prints CANNOT CHECK. Always exits 0.
# Reads local refs only (session-start's context script fetches first), so CONTEXT_OFFLINE changes
# nothing here.
ROOT=$(case $CLAUDUCTOR_FW in (*/.claude) dirname "$CLAUDUCTOR_FW" ;; (*) [ -n "${ROOT:-}" ] && echo "$ROOT" || git rev-parse --show-toplevel 2>/dev/null || pwd ;; esac)
# shellcheck disable=SC1091
. "$CLAUDUCTOR_FW/lib/conf.sh"
cur="$CLAUDUCTOR_FW/modules/artifacts/bin/currency.sh"
reg=${ARTIFACT_REGISTRY:-docs/artifacts.json}

out=$(sh "$cur" --root "$ROOT" 2>&1); rc=$?
if [ "$rc" -ne 0 ]; then
  echo "CANNOT CHECK — core-artifact currency: $(printf '%s' "$out" | tail -n 1 | sed 's/^artifacts: //'). UNKNOWN, not current."
  exit 0
fi
subject=$(printf '%s\n' "$out" | head -n 1 | sed -n 's/^Core artifacts vs \(.*\); authorities.*/\1/p')
printf '%s\n' "$out" | awk -v reg="$reg" -v subj="$subject" '
  /^BEHIND / { k = $2; sub(/^BEHIND +[^ ]+ — /, ""); print "STALE — " k " is BEHIND its authorities in " reg ": " $0; bad++; next }
  /^CANNOT CHECK — / { print; bad++; next }
  /^CANNOT CHECK / { sub(/^CANNOT CHECK /, ""); print "CANNOT CHECK — " $0; bad++; next }
  /^OK / { n++ }
  END { if (!bad) print "OK — all " n + 0 " core artifacts in " reg " are current with their authorities at " subj }'
exit 0
