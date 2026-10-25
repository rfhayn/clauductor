#!/bin/sh
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
#                                                          touching no guarded path
#   sh .claude/change-approval.sh <id> --revoke            back to `**Status:** awaiting approval`
#
# The Approved line reads `**Approved:** YYYY-MM-DD by <owner> · design <hash>`, or for a delegated
# one `by Claude (delegated by <owner>)`. A delegated approval holds only while the key is on and the
# change touches no path in lib/approval-guarded.txt or local/approval-guarded.txt: none named in a
# task line of tasks.md and, on the change's own branch, none in its diff. The key and the lists are
# read from origin/<MAIN_BRANCH> when it exists, so a branch cannot loosen them for itself. The check
# re-tests all of it every run: turning the key off or touching a guarded path voids the approval
# like a design edit. The hash is over
# design.md and the proposal's Risk line (lib/change.sh design_hash), so an edit to either after
# approval makes checks/changes.sh fail until the owner approves again: the gate cannot write a
# receipt and the merge guard refuses the merge. Recording is a skill step taken on the owner's word
# (propose step 5); this script only writes what the owner said, it cannot tell that they said it.
ROOT=$(cd "$(dirname "$0")/.." 2>/dev/null && pwd)
. "$ROOT/.claude/lib/conf.sh"
. "$ROOT/.claude/lib/change.sh"

id=${1:-}; [ -n "$id" ] || { echo "usage: change-approval.sh <id> [--record <owner> | --delegate <owner> | --revoke]" >&2; exit 64; }
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

# ── Delegation (OPS-127) ───────────────────────────────────────────────────────────────────────
delegated='^\*\*Approved:\*\* [0-9-]+ by Claude \(delegated by (.+)\) · '
main_ref=""
git -C "$ROOT" rev-parse -q --verify "refs/remotes/origin/$MAIN_BRANCH" >/dev/null 2>&1 && main_ref="origin/$MAIN_BRANCH"

# authority FILE: FILE as origin/<main> has it, when FILE is in this repository and that ref exists
# (nothing when main lacks it); else as it is on disk: the plugin's own copy, which no branch can
# edit, or a repository with no remote.
authority() {
  case $1 in "$ROOT"/*) [ -z "$main_ref" ] || { git -C "$ROOT" show "$main_ref:${1#"$ROOT"/}" 2>/dev/null; return 0; } ;; esac
  cat "$1" 2>/dev/null
}

# delegable: say why a delegated approval cannot hold for this change, or nothing when it can. It
# iterates the guarded lists (the authority) against what the change touches: the paths its task
# lines name and, on its own branch, its diff. An empty framework list refuses: a list that failed
# to load must not read as "nothing guarded".
delegable() {
  key=$APPROVAL_DELEGATED
  [ -z "$main_ref" ] || key=$(authority "$ROOT/.claude/project.conf" | sed -n 's/^APPROVAL_DELEGATED=["'\'']*\([^"'\''#[:space:]]*\).*/\1/p' | tail -1)
  [ "$key" = 1 ] || { echo "APPROVAL_DELEGATED is not \"1\" in .claude/project.conf${main_ref:+ on $main_ref}, so only the $OWNER_ROLE approves"; return; }
  fw=$(authority "$ROOT/.claude/lib/approval-guarded.txt" | sed 's/[[:space:]]*$//; s|/$||' | grep -v '^#' | grep .)
  [ -n "$fw" ] || { echo ".claude/lib/approval-guarded.txt${main_ref:+ on $main_ref} is missing or lists no path, so no approval can be delegated"; return; }
  own=$(authority "$ROOT/.claude/local/approval-guarded.txt" | sed 's/[[:space:]]*$//; s|/$||' | grep -v '^#' | grep .)
  touched=$(grep -E '^[[:space:]]*- \[[ xX]\]' "$dir/tasks.md" 2>/dev/null | grep -oE '`[^` ]+`' | tr -d '`' | sed 's|^\./||')
  if [ "$(git -C "$ROOT" symbolic-ref --short -q HEAD 2>/dev/null)" = "$BRANCH_CHANGE$id" ] \
     && base=$(git -C "$ROOT" merge-base "${main_ref:-$MAIN_BRANCH}" HEAD 2>/dev/null); then
    touched="$touched
$(git -C "$ROOT" diff --name-only "$base" 2>/dev/null)
$(git -C "$ROOT" ls-files -o --exclude-standard 2>/dev/null)"
  fi
  # A guarded entry matches whole path segments anywhere in a path.
  hits=$(printf '%s\n%s\n' "$fw" "$own" | grep . | while IFS= read -r g; do
    printf '%s\n' "$touched" | while IFS= read -r p; do
      case "/$p/" in (*"/$g/"*) printf '%s\n' "$p" ;; esac
    done
  done | sort -u | tr '\n' ' ')
  [ -z "$hits" ] || echo "it touches a guarded path (approval-guarded.txt): $hits; the $OWNER_ROLE approves it"
}

case "${1:-}" in
  --delegate)
    owner=${2:-}; [ -n "$owner" ] || { echo "change-approval: --delegate needs the delegating owner's name" >&2; exit 64; }
    [ -z "$OWNER_NAME" ] || [ "$owner" = "$OWNER_NAME" ] || { echo "change-approval: only OWNER_NAME ($OWNER_NAME) delegates, not $owner" >&2; exit 64; }
    why=$(delegable); [ -z "$why" ] || { echo "change-approval: cannot delegate $CHANGES_DIR/$id: $why" >&2; exit 1; }
    rewrite "**Approved:** ${APPROVAL_DATE:-$(date +%Y-%m-%d)} by Claude (delegated by $owner) · design $hash"
    echo "recorded: $CHANGES_DIR/$id approved by Claude, delegated by $owner, design $hash"
    ;;
  --record)
    owner=${2:-}; [ -n "$owner" ] || { echo "change-approval: --record needs the owner's name" >&2; exit 64; }
    printf '%s\n' "$owner" | grep -Eiq '^[[:space:]]*claude' && { echo "change-approval: Claude's approval is recorded with --delegate, which checks the delegation" >&2; exit 64; }
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
      who=$(printf '%s\n' "$line" | sed -E "s/$delegated.*/\\1/")
      [ -z "$OWNER_NAME" ] || [ "$who" = "$OWNER_NAME" ] || { echo "design $hash: delegated by $who, but only OWNER_NAME ($OWNER_NAME) delegates"; exit 1; }
    elif printf '%s\n' "$line" | grep -Eiq '^\*\*Approved:\*\* [0-9-]+ by[[:space:]]+claude'; then
      echo "design $hash: an approval by Claude that is not '(delegated by <owner>)', the one form a delegation is checked in: $line"; exit 1
    fi
    if [ "$rec" = "$hash" ]; then echo "design $hash: approved as written$by"; exit 0; fi
    echo "design $hash: CHANGED since the owner approved design $rec; the approval no longer covers it"
    exit 1
    ;;
  *) echo "change-approval: unknown option $1" >&2; exit 64 ;;
esac
