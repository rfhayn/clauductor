#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/../../.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# Health: does every docs/*.html page's claude.ai copy match origin/MAIN_BRANCH (the publishing half,
# ARTIFACT_PUBLISH="claude.ai")? One verdict line per page that needs action, or one OK.
#
# A DETECTOR, never the executor: publishing needs the Artifact tool, which no shell reaches. The
# executors are skill steps (the module's session-start and merge-pr fragments publish; session-close
# records). With ARTIFACT_PUBLISH=off there is nothing to check, and the line says so. Never degrades
# to silence: Node missing, no origin ref, or an unreadable registry prints CANNOT CHECK. Exit 0.
ROOT=$(case $CLAUDUCTOR_FW in (*/.claude) dirname "$CLAUDUCTOR_FW" ;; (*) [ -n "${ROOT:-}" ] && echo "$ROOT" || git rev-parse --show-toplevel 2>/dev/null || pwd ;; esac)
# shellcheck disable=SC1091
. "$CLAUDUCTOR_FW/lib/conf.sh"
pub="$CLAUDUCTOR_FW/modules/artifacts/bin/publish.sh"
case ${ARTIFACT_PUBLISH:-off} in
  off) echo "OK — not applicable: ARTIFACT_PUBLISH=off, so no claude.ai copy is published or checked"; exit 0 ;;
esac
out=$(sh "$pub" --root "$ROOT" --status --ref "origin/${MAIN_BRANCH:-main}" 2>&1); rc=$?
case $rc in
  0) ;;
  3) printf '%s\n' "$out" | tail -n 1; exit 0 ;;
  *) echo "CANNOT CHECK — shared copies: $(printf '%s' "$out" | tail -n 1 | sed 's/^prep-artifact: //; s/^artifacts: //'). UNKNOWN, not current."; exit 0 ;;
esac
subject=$(printf '%s\n' "$out" | head -n 1 | sed -n 's/^Shared copies vs \(.*\); registry.*/\1/p')
printf '%s\n' "$out" | awk -v subj="$subject" '
  /^STALE / { sub(/^STALE /, ""); print "STALE — " $0 " (republish it: the module'"'"'s session-start step)"; bad++; next }
  /^UNREGISTERED / { sub(/^UNREGISTERED /, ""); print "STALE — " $0; bad++; next }
  /^MISSING / { sub(/^MISSING /, ""); print "STALE — " $0; bad++; next }
  /^WARN / { sub(/^WARN +/, ""); print "STALE — the copy will carry a dead link: " $0; bad++; next }
  /^OK / { n++ }
  END { if (!bad) print "OK — all " n " shared copies match " subj }'
exit 0
