#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# change-approval.sh: the owner's approval covers the design AS WRITTEN (D8 of the change process).
#
#   sh .claude/change-approval.sh <id>                     the design hash, and whether the recorded
#                                                          approval still matches it (exit 1 if not)
#   sh .claude/change-approval.sh <id> --record "<owner>"  record the owner's approval, AFTER they gave
#                                                          it: line 1 (or 2, under a review page)
#                                                          becomes the Approved line with today's hash
#   sh .claude/change-approval.sh <id> --delegate "<owner>"  record Claude's approval on the owner's
#                                                          delegation (OPS-127): only with
#                                                          APPROVAL_DELEGATED="1" and the change
#                                                          naming no guarded path
#   sh .claude/change-approval.sh <id> --revoke            back to `**Status:** awaiting approval`
#
# The Approved line reads `**Approved:** YYYY-MM-DD by <owner> · design <hash>`, or for a delegated
# one `by Claude (delegated by <owner>)`. A delegated approval holds only while the key is on and
# the change's records name no path in .claude/approval-guarded.txt: the check re-reads both every
# time, so turning the key off or adding a guarded path voids it like a design edit. The hash is over
# design.md and the proposal's Risk line (lib/change.sh design_hash), so an edit to either after
# approval makes checks/changes.sh fail until the owner approves again: the gate cannot write a
# receipt and the merge guard refuses the merge. Recording is a skill step taken on the owner's word
# (propose step 5); this script only writes what the owner said, it cannot tell that they said it.
ROOT=$(case $CLAUDUCTOR_FW in (*/.claude) dirname "$CLAUDUCTOR_FW" ;; (*) [ -n "${ROOT:-}" ] && echo "$ROOT" || git rev-parse --show-toplevel 2>/dev/null || pwd ;; esac)
. "$CLAUDUCTOR_FW/lib/conf.sh"
. "$CLAUDUCTOR_FW/lib/change.sh"

id=${1:-}; [ -n "$id" ] || { echo "usage: change-approval.sh <id> [--record <owner> | --revoke]" >&2; exit 64; }
shift
dir="$ROOT/$CHANGES_DIR/$id"
[ -f "$dir/proposal.md" ] && [ -f "$dir/design.md" ] || { echo "change-approval: $CHANGES_DIR/$id has no proposal.md and design.md" >&2; exit 1; }
hash=$(design_hash "$dir")
approved='^\*\*Approved:\*\* [0-9]{4}-[0-9]{2}-[0-9]{2} by .+ · design [0-9a-f]{12}$'

# rewrite LINE: replace the Approved or Status line with LINE, keeping every other line.
rewrite() {
  awk -v new="$1" '!done && (/^\*\*Approved:\*\*/ || /^\*\*Status:\*\*/) { print new; done = 1; next } { print } END { if (!done) exit 3 }' "$dir/proposal.md" > "$dir/proposal.md.tmp" \
    && mv "$dir/proposal.md.tmp" "$dir/proposal.md" \
    || { rm -f "$dir/proposal.md.tmp"; echo "change-approval: proposal.md has no **Approved:** or **Status:** line to replace" >&2; exit 1; }
}

# delegable: say why a delegated approval cannot hold for this change, or nothing when it can. It
# iterates the guarded list (the authority), not the paths the change claims to touch, and an empty
# or missing list refuses: a list that failed to load must not read as "nothing guarded".
delegated='^\*\*Approved:\*\* [0-9-]+ by Claude \(delegated by .+\) · '
guarded_list="$ROOT/.claude/approval-guarded.txt"
delegable() {
  [ "$APPROVAL_DELEGATED" = 1 ] || { echo "APPROVAL_DELEGATED is not \"1\" in .claude/project.conf, so only the $OWNER_ROLE approves"; return; }
  [ -f "$guarded_list" ] || { echo ".claude/approval-guarded.txt is missing, so no approval can be delegated"; return; }
  n=0; hits=""
  while IFS= read -r g; do
    case $g in ''|'#'*) continue ;; esac
    n=$((n + 1))
    grep -rqF -- "$g" "$dir" && hits="$hits $g"
  done < "$guarded_list"
  [ "$n" -gt 0 ] || { echo ".claude/approval-guarded.txt lists no path, so no approval can be delegated"; return; }
  [ -z "$hits" ] || echo "it names a guarded path (.claude/approval-guarded.txt):$hits; the $OWNER_ROLE approves it"
}

case "${1:-}" in
  --delegate)
    owner=${2:-}; [ -n "$owner" ] || { echo "change-approval: --delegate needs the delegating owner's name" >&2; exit 64; }
    why=$(delegable); [ -z "$why" ] || { echo "change-approval: cannot delegate $CHANGES_DIR/$id: $why" >&2; exit 1; }
    rewrite "**Approved:** ${APPROVAL_DATE:-$(date +%Y-%m-%d)} by Claude (delegated by $owner) · design $hash"
    echo "recorded: $CHANGES_DIR/$id approved by Claude, delegated by $owner, design $hash"
    ;;
  --record)
    owner=${2:-}; [ -n "$owner" ] || { echo "change-approval: --record needs the owner's name" >&2; exit 64; }
    case $owner in Claude*) echo "change-approval: Claude's approval is recorded with --delegate, which checks the delegation" >&2; exit 64 ;; esac
    rewrite "**Approved:** ${APPROVAL_DATE:-$(date +%Y-%m-%d)} by $owner · design $hash"
    echo "recorded: $CHANGES_DIR/$id approved by $owner, design $hash"
    ;;
  --revoke)
    rewrite "**Status:** awaiting approval"
    echo "revoked: $CHANGES_DIR/$id is awaiting approval again"
    ;;
  '')
    line=$(grep -E '^\*\*Approved:\*\*' "$dir/proposal.md" | head -1)
    if [ -z "$line" ]; then echo "design $hash: not approved yet"; exit 1; fi
    printf '%s\n' "$line" | grep -Eq "$approved" || { echo "design $hash: the Approved line does not end '· design <12 hex>': $line"; exit 1; }
    rec=${line##* }
    by=""
    if printf '%s\n' "$line" | grep -Eq "$delegated"; then
      why=$(delegable); [ -z "$why" ] || { echo "design $hash: approved by Claude on delegation, which cannot hold: $why"; exit 1; }
      by=", by Claude on delegation"
    elif printf '%s\n' "$line" | grep -Eq '^\*\*Approved:\*\* [0-9-]+ by Claude'; then
      echo "design $hash: an approval by Claude that is not '(delegated by <owner>)', the one form a delegation is checked in: $line"; exit 1
    fi
    if [ "$rec" = "$hash" ]; then echo "design $hash: approved as written$by"; exit 0; fi
    echo "design $hash: CHANGED since the owner approved design $rec; the approval no longer covers it"
    exit 1
    ;;
  *) echo "change-approval: unknown option $1" >&2; exit 64 ;;
esac
