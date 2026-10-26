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
#                                                          APPROVAL_DELEGATED="1", the change
#                                                          touching no guarded path, ADR or practice
#                                                          record, no **Approval:** line in its
#                                                          proposal and no Approval: owner in its
#                                                          row (the one naming it), its Risk not
#                                                          below its row's (OPS-131), and a proposal
#                                                          never approved before, by its Status
#                                                          line, its history or this clone's
#                                                          approval log (OPS-128, OPS-131)
#   sh .claude/change-approval.sh <id> --revoke            back to `**Status:** awaiting approval
#                                                          (revoked; was approved by <who>)`
#   sh .claude/change-approval.sh --start-log "<owner>"    start this clone's approval log, on the
#                                                          owner's word: --delegate refuses until a
#                                                          start or record line is in it
#
# A revoke is final for delegation (OPS-128, the owner's ruling): once a change has been approved
# and revoked, whoever approved it, only the owner approves it again. --revoke keeps who approved in
# the Status line, --delegate refuses while that marker or any Approved line stands, or stood in the
# proposal's history, and only --record clears it; the check fails a delegated line whose history
# shows an Approved line removed. So an edit after any approval, which voids it, goes to the owner.
# The history is followed across a move of the change's whole directory only, so another change's
# history is never read as this one's. It is firm on origin/<MAIN_BRANCH>; the branch's own can be
# rewritten, so --record, --delegate and --revoke also add a line to the approval log,
# <git common dir>/clauductor/approvals.log, which no branch carries, and --delegate refuses a change
# the log names. A revoke the log cannot take still revokes, and exits 1 to say so. The log keys on
# the change's id, so a change filed again under a new id is new to it. The log guards the act, not the merge: the check never reads it, since a CI runner
# has none. It is per clone (every worktree of a clone shares it): another machine, or a fresh clone,
# starts its own, so a revoke made only on a branch elsewhere is not seen here (#272).
#
# The Approved line reads `**Approved:** YYYY-MM-DD by <owner> · design <hash>`, or for a delegated
# one `by Claude (delegated by <owner>)`. A delegated approval holds only while the key is on and the
# change touches no path in lib/approval-guarded.txt or local/approval-guarded.txt: none named in a
# task line of tasks.md and, on the change's own branch, none in its diff. The key and the lists are
# read from origin/<MAIN_BRANCH>, so a branch cannot loosen them for itself; only a repository with
# no remote reads them from disk, and where neither can be read the approval does not hold. The check
# re-tests all of it every run: turning the key off or touching a guarded path voids the approval
# like a design edit. The hash is over
# design.md and the proposal's Risk line (lib/change.sh design_hash), so an edit to either after
# approval makes checks/changes.sh fail until the owner approves again: the gate cannot write a
# receipt and the merge guard refuses the merge. Recording is a skill step taken on the owner's word
# (propose step 5); this script only writes what the owner said, it cannot tell that they said it.
ROOT=$(case $CLAUDUCTOR_FW in (*/.claude) dirname "$CLAUDUCTOR_FW" ;; (*) [ -n "${ROOT:-}" ] && echo "$ROOT" || git rev-parse --show-toplevel 2>/dev/null || pwd ;; esac)
. "$CLAUDUCTOR_FW/lib/conf.sh"
. "$CLAUDUCTOR_FW/lib/change.sh"

# ── The approval log (OPS-131, #272) ───────────────────────────────────────────────────────────
# A line per approval or revoke, in the repository's common git dir: outside every worktree and
# every branch's history, so no rewrite of a branch reaches it. Only a repository's root has one.
approval_log=""
if [ "$(git -C "$ROOT" rev-parse --show-toplevel 2>/dev/null)" = "$(cd "$ROOT" && pwd -P)" ]; then
  common=$(cd "$ROOT" && cd "$(git rev-parse --git-common-dir)" 2>/dev/null && pwd -P) \
    && approval_log="$common/clauductor/approvals.log"
fi
# log_append VERB WHO: add a line for this change, creating the log; fails when it cannot. A log is
# started only by a start or record line, the owner's word: --delegate refuses until one is there.
log_append() {
  [ -n "$approval_log" ] || return 0
  mkdir -p "${approval_log%/*}" 2>/dev/null \
    && printf '%s\t%s\t%s\t%s\n' "${APPROVAL_DATE:-$(date +%Y-%m-%d)}" "${id:--}" "$1" "$2" 2>/dev/null >> "$approval_log"
}

if [ "${1:-}" = --start-log ]; then
  owner=${2:-}; [ -n "$owner" ] || { echo "change-approval: --start-log needs the owner's name" >&2; exit 64; }
  [ -z "$OWNER_NAME" ] || [ "$owner" = "$OWNER_NAME" ] || { echo "change-approval: only OWNER_NAME ($OWNER_NAME) starts the approval log, not $owner" >&2; exit 64; }
  [ -n "$approval_log" ] || { echo "change-approval: $ROOT is not a git repository's root, so it has no approval log" >&2; exit 1; }
  ! awk -F'\t' '$3 == "start" { found = 1 } END { exit !found }' "$approval_log" 2>/dev/null \
    || { echo "change-approval: the approval log is started already: $approval_log" >&2; exit 1; }
  id=""; log_append start "$owner" || { echo "change-approval: cannot write the approval log $approval_log" >&2; exit 1; }
  echo "started: the approval log $approval_log, by $owner"
  exit 0
fi

id=${1:-}; [ -n "$id" ] || { echo "usage: change-approval.sh <id> [--record <owner> | --delegate <owner> | --revoke] | --start-log <owner>" >&2; exit 64; }
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
# claude_name NAME: true when NAME is Claude's (its first word, any case) and not OWNER_NAME's.
claude_name() { printf '%s\n' "$1" | grep -Eiq '^[[:space:]]*claude([^[:alnum:]_]|$)' && [ "$1" != "$OWNER_NAME" ]; }
# The authority is origin/<main> in a clone of a remote, and the disk only in a repository with no
# remote. Anywhere else delegation cannot be judged, so it does not hold (fail closed): outside a
# repository (a GATE_CLEAN_ROOM="archive" copy), or a remote with no origin/<main> (a shallow CI
# checkout; fetch it).
main_ref="" gap=""
if [ "$(git -C "$ROOT" rev-parse --show-toplevel 2>/dev/null)" != "$(cd "$ROOT" && pwd -P)" ]; then
  gap="$ROOT is not a git repository's root (a GATE_CLEAN_ROOM=\"archive\" copy?), so origin/$MAIN_BRANCH cannot be read"
elif git -C "$ROOT" rev-parse -q --verify "refs/remotes/origin/$MAIN_BRANCH" >/dev/null 2>&1; then
  main_ref="origin/$MAIN_BRANCH"
elif [ -n "$(git -C "$ROOT" remote 2>/dev/null)" ]; then
  gap="this clone has a remote but no origin/$MAIN_BRANCH to read the key and the lists from (fetch it; a CI checkout needs fetch-depth: 0)"
fi

# authority FILE: FILE as origin/<main> has it, when FILE is in this repository and that ref exists
# (nothing when main lacks it); else as it is on disk: the plugin's own copy, which no branch can
# edit, or a repository with no remote.
authority() {
  case $1 in "$ROOT"/*) [ -z "$main_ref" ] || { git -C "$ROOT" show "$main_ref:${1#"$ROOT"/}" 2>/dev/null; return 0; } ;; esac
  cat "$1" 2>/dev/null
}

# on_branch: true when a HEAD that main does not contain is the change's work by its history or its
# content (OPS-130): at or past a tip of the change's branch, local or origin's, that main does not
# already contain (a fresh branch still at main identifies nothing, so a branch cut from main is
# not charged to it), or with commits since main that touch the change's own directory, as every
# build's do when it ticks tasks.md. So a commit made on a detached HEAD past the tip, or an amend
# or rebase of it there, and pushed to the branch, has its diff counted. A HEAD main contains is
# never the branch's, so a merged change, or the main checkout, is judged by its task lines alone.
# The content signal reaches any branch that edits changes/<id>/, so one that does and also touches
# a guarded path refuses that change's delegation: a refusal sends the approval to the owner, which
# is the safe side to err on.
on_branch() {
  m=${main_ref:-$MAIN_BRANCH}
  git -C "$ROOT" merge-base --is-ancestor HEAD "$m" 2>/dev/null && return 1
  for t in $(git -C "$ROOT" for-each-ref --format='%(objectname)' "refs/heads/$BRANCH_CHANGE$id" "refs/remotes/origin/$BRANCH_CHANGE$id" 2>/dev/null); do
    git -C "$ROOT" merge-base --is-ancestor "$t" "$m" 2>/dev/null && continue
    git -C "$ROOT" merge-base --is-ancestor "$t" HEAD 2>/dev/null && return 0
  done
  mb=$(git -C "$ROOT" merge-base "$m" HEAD 2>/dev/null) || return 1
  [ -n "$(git -C "$ROOT" diff --name-only "$mb" HEAD -- "$CHANGES_DIR/$id/" 2>/dev/null)" ]
}

risk_rank() { case $1 in low) echo 1 ;; normal) echo 2 ;; high) echo 3 ;; *) echo 0 ;; esac; }

# kept: say why the owner keeps this change though no guarded path names it (OPS-131), or nothing:
# a proposal line `**Approval:** …` (propose step 3a writes `owner — <why>` for a change that is
# money, legal or business, outward, irreversible or needs the owner at the computer; any other
# value is read as that too), its roadmap row's `Approval: owner`, or a `**Risk:**` below the row's
# (a row with no Risk: is normal), which would lower the review cap the row set. The row is the
# authority, read as origin/<main> has it like the key, and found by the change id it names, never
# by the proposal's say: a proposal naming another row would choose its own floor. One row must name
# the change, and it must be the proposal's `**Roadmap row:**`; anything else refuses, as does a
# roadmap whose table is not `| <id> | Change | … |`.
kept() {
  prop="$dir/proposal.md"
  mark=$(grep -E '^\*\*Approval:\*\*' "$prop" | head -1)
  [ -z "$mark" ] || { echo "proposal.md says $mark"; return; }
  rid=$(proposal_field "$prop" "Roadmap row")
  [ -n "$rid" ] || { echo "proposal.md names no **Roadmap row:**, so its row's Risk and Approval cannot be read"; return; }
  # The row as main has it, found through roadmap_queue so a project's ROADMAP_PARSER decides what a
  # row is; its Change cell is read from the line the parser names, as review-cap.sh reads it.
  rmf=$(mktemp "${TMPDIR:-/tmp}/approval-roadmap.XXXXXX") || { echo "no temporary file for $ROADMAP, so row $rid cannot be read"; return; }
  authority "$ROOT/$ROADMAP" > "$rmf"
  if tsv=$(roadmap_queue --tsv "$rmf"); then
    rows=$(printf '%s\n' "$tsv" | awk -F'\t' -v c="$id" '$5 == c { print $1 "\t" $4 }')
    n=$(printf '%s\n' "$rows" | grep -c .)
    line=""; [ "$n" != 1 ] || line=$(sed -n "$(printf '%s\n' "$rows" | cut -f1)p" "$rmf")
    rm -f "$rmf"
  else
    rm -f "$rmf"; echo "$ROADMAP${main_ref:+ on $main_ref} did not parse, so row $rid cannot be read"; return
  fi
  [ "$n" -le 1 ] || { echo "$n rows of $ROADMAP${main_ref:+ on $main_ref} name the change $id ($(printf '%s\n' "$rows" | cut -f2 | tr '\n' ' ')), so its Risk and Approval cannot be read"; return; }
  [ -n "$line" ] || { echo "no row of $ROADMAP${main_ref:+ on $main_ref} names the change $id, so its Risk and Approval cannot be read"; return; }
  row=$(printf '%s\n' "$rows" | cut -f2)
  [ "$row" = "$rid" ] || { echo "proposal.md names row $rid, but row $row${main_ref:+ on $main_ref} is the one that names $id"; return; }
  printf '%s\n' "$line" | row_change_cell | grep -Eiq '(^|[^[:alnum:]])Approval: owner([^[:alnum:]]|$)' \
    && { echo "row $rid${main_ref:+ on $main_ref} says Approval: owner"; return; }
  rr=$(printf '%s\n' "$line" | row_risks)
  case $(printf '%s\n' "$rr" | grep -c .) in
    0) rr=normal ;;
    1) ;;
    *) echo "row $rid's Change cell says more than one risk ($(echo $rr)), so its floor cannot be read"; return ;;
  esac
  pr=$(proposal_field "$prop" Risk)
  [ "$(risk_rank "$pr")" -ge "$(risk_rank "$rr")" ] \
    || echo "its **Risk:** ${pr:-(none)} is below row $rid's $rr (a row with no Risk: is normal), which would lower the review cap the row set"
}

# delegable: say why a delegated approval cannot hold for this change, or nothing when it can. It
# iterates the guarded lists (the authority) against what the change touches: the paths its task
# lines name and, on its own branch, its diff. An empty framework list refuses: a list that failed
# to load must not read as "nothing guarded".
delegable() {
  [ -z "$gap" ] || { echo "$gap"; return; }
  key=$APPROVAL_DELEGATED
  [ -z "$main_ref" ] || key=$(authority "$ROOT/.claude/project.conf" | sed -n 's/^APPROVAL_DELEGATED=["'\'']*\([^"'\''#[:space:]]*\).*/\1/p' | tail -1)
  [ "$key" = 1 ] || { echo "APPROVAL_DELEGATED is not \"1\" in .claude/project.conf${main_ref:+ on $main_ref}, so only the $OWNER_ROLE approves"; return; }
  fw=$(authority "$CLAUDUCTOR_FW/lib/approval-guarded.txt" | sed 's/[[:space:]]*$//; s|/$||' | grep -v '^#' | grep .)
  [ -n "$fw" ] || { echo ".claude/lib/approval-guarded.txt${main_ref:+ on $main_ref} is missing or lists no path, so no approval can be delegated"; return; }
  kept=$(kept); [ -z "$kept" ] || { echo "$kept; the $OWNER_ROLE approves it"; return; }
  # ADRs and practice records are the owner's (Who decides): their directories are guarded too.
  # project.conf, which names them, is itself guarded, so a branch cannot move them aside.
  own=$(authority "$ROOT/.claude/local/approval-guarded.txt" | sed 's/[[:space:]]*$//; s|/$||' | grep -v '^#' | grep .)
  own=$(printf '%s\n%s\n%s\n' "$own" "$ADR_DIR" "$PRACTICES_DIR" | sed 's|/$||' | grep .)
  # A token's `:line` or `#anchor` is not part of its path.
  touched=$(grep -E '^[[:space:]]*- \[[ xX]\]' "$dir/tasks.md" 2>/dev/null | grep -oE '`[^` ]+`' | tr -d '`' | sed 's|^\./||; s/[:#].*//')
  # The change's own branch: checked out by name, a CI pull request's head, or any HEAD at or past
  # its tip (on_branch). Its diff names both sides of a rename (--no-renames), so a guarded file
  # cannot move out.
  b=$(git -C "$ROOT" symbolic-ref --short -q HEAD 2>/dev/null) || b=${GITHUB_HEAD_REF:-}
  if [ "$b" = "$BRANCH_CHANGE$id" ] || on_branch; then
    base=$(git -C "$ROOT" merge-base "${main_ref:-$MAIN_BRANCH}" HEAD 2>/dev/null) \
      || { echo "the change's branch shares no commit with ${main_ref:-$MAIN_BRANCH} here, so its diff cannot be read"; return; }
    touched="$touched
$(git -C "$ROOT" diff --name-only --no-renames "$base" 2>/dev/null)
$(git -C "$ROOT" ls-files -o --exclude-standard 2>/dev/null)"
  fi
  # A guarded entry matches whole path segments anywhere in a path.
  hits=$(printf '%s\n%s\n' "$fw" "$own" | grep . | while IFS= read -r g; do
    printf '%s\n' "$touched" | while IFS= read -r p; do
      case "/$p/" in (*"/$g/"*) printf '%s\n' "$p" ;; esac
    done
  done | sort -u | tr '\n' ' ')
  [ -z "$hits" ] || echo "it touches a guarded path (approval-guarded.txt, ADR_DIR, PRACTICES_DIR): $hits; the $OWNER_ROLE approves it"
}

# change_root PATH: the change directory a path under CHANGES_DIR belongs to (an archived one is one
# level deeper); nothing for a file directly in CHANGES_DIR.
change_root() {
  rel=${1#"$CHANGES_DIR"/}; case $rel in */*) ;; *) return ;; esac
  seg=${rel%%/*}
  if [ "$seg" = archive ]; then rest=${rel#archive/}; case $rest in */*) seg="archive/${rest%%/*}" ;; *) return ;; esac; fi
  echo "$CHANGES_DIR/$seg"
}

# renamed_from COMMIT PATH: the proposal.md COMMIT renamed to PATH, when COMMIT moved its change's
# whole directory: every file of the old directory to the same place in a directory that did not
# exist before, and no other change directory added. A split (one directory out, two in), a
# partial move or a merge into another change starts a new lineage, so its history is not this
# change's (OPS-131, #272). Prints nothing then; fails when git cannot answer.
renamed_from() {
  # Paths split on newlines only and never glob: a name with a space is one path. It always runs in
  # a command substitution, so neither setting reaches the caller.
  set -f; IFS='
'
  c=$1 p=$2 new=${2%/*}
  git -C "$ROOT" rev-parse -q --verify "$c^" >/dev/null 2>&1 || return 0
  diff=$(git -C "$ROOT" diff-tree -r -M --name-status "$c^" "$c" -- "$CHANGES_DIR/") || return 1
  old=$(printf '%s\n' "$diff" | awk -F'\t' -v p="$p" '$1 ~ /^R/ && $3 == p { print $2; exit }')
  [ -n "$old" ] || return 0
  olddir=${old%/*}
  before=$(git -C "$ROOT" ls-tree -r --name-only "$c^" -- "$new/") || return 1
  [ -z "$before" ] || return 0
  files=$(git -C "$ROOT" ls-tree -r --name-only "$c^" -- "$olddir/") || return 1
  for f in $files; do
    printf '%s\n' "$diff" | awk -F'\t' -v o="$f" -v n="$new/${f#"$olddir"/}" '$1 ~ /^R/ && $2 == o && $3 == n { found = 1 } END { exit !found }' || return 0
  done
  for d in $(printf '%s\n' "$diff" | awk -F'\t' '$1 ~ /^[ARC]/ { print $NF }'); do
    r=$(change_root "$d"); [ -n "$r" ] && [ "$r" != "$new" ] || continue
    there=$(git -C "$ROOT" ls-tree --name-only "$c^" -- "$r") || return 1
    [ -n "$there" ] || return 0
  done
  echo "$old"
}

# history_holds PATTERN WHAT: say where proposal.md's history, on origin/<main> or this branch, has a
# diff line matching PATTERN (WHAT names it), or that it cannot be read; nothing when neither. The
# history crosses a move of the change's whole directory (renamed_from), not git's --follow, which
# pairs the proposal with any similar one deleted in the same commit (OPS-131, #272). An unborn HEAD
# (a repository with no commit yet) has no history to hold one.
history_holds() {
  for ref in $main_ref HEAD; do
    where="this branch"; [ "$ref" = HEAD ] || where=$ref
    [ "$ref" != HEAD ] || git -C "$ROOT" rev-parse -q --verify HEAD >/dev/null 2>&1 || continue
    todo="$ref $CHANGES_DIR/$id/proposal.md"; n=0
    while [ -n "$todo" ]; do
      r=${todo%%"
"*}; [ "$r" = "$todo" ] && todo="" || todo=${todo#*"
"}
      p=${r#* }; r=${r%% *}; n=$((n + 1))
      [ "$n" -le 50 ] || { echo "its history on $where cannot be read: more than 50 moves of its directory"; return; }
      # log.follow would turn this single-path log back into --follow; --text and --no-textconv keep
      # a -diff, binary or textconv attribute from hiding the lines.
      log=$(git -C "$ROOT" -c log.follow=false log --no-renames --no-color --no-ext-diff --text --no-textconv --format= -p "$r" -- "$p" 2>&1) \
        || { echo "its history on $where cannot be read: $log"; return; }
      ! printf '%s\n' "$log" | grep -Eq "$1" || { echo "its history on $where holds $2"; return; }
      adds=$(git -C "$ROOT" -c log.follow=false log --no-renames --format=%H --diff-filter=A "$r" -- "$p" 2>&1) \
        || { echo "its history on $where cannot be read: $adds"; return; }
      for c in $adds; do
        src=$(renamed_from "$c" "$p") || { echo "its history on $where cannot be read: a move at $c"; return; }
        [ -z "$src" ] || todo="${todo:+$todo
}$c^ $src"
      done
    done
  done
}

case "${1:-}" in
  --delegate)
    owner=${2:-}; [ -n "$owner" ] || { echo "change-approval: --delegate needs the delegating owner's name" >&2; exit 64; }
    [ -z "$OWNER_NAME" ] || [ "$owner" = "$OWNER_NAME" ] || { echo "change-approval: only OWNER_NAME ($OWNER_NAME) delegates, not $owner" >&2; exit 64; }
    # A proposal approved before, standing or revoked since, is the owner's to approve again (OPS-128).
    prior=$(grep -E '^\*\*(Approved:\*\*|Status:\*\* awaiting approval \(revoked)' "$dir/proposal.md" | head -1)
    [ -z "$prior" ] || { echo "change-approval: cannot delegate $CHANGES_DIR/$id: it was approved before ($prior), so only the $OWNER_ROLE approves it again: 'sh "$CLAUDUCTOR_FW"/change-approval.sh $id --record <owner>' on their word" >&2; exit 1; }
    why=$(delegable); [ -z "$why" ] || { echo "change-approval: cannot delegate $CHANGES_DIR/$id: $why" >&2; exit 1; }
    # --revoke's marker is a line a hand edit of the Status line removes; the history is not
    # (OPS-131). An Approved line ever committed to proposal.md is an approval before.
    why=$(history_holds '^[+-]\*\*Approved:\*\* ' "an Approved line"); [ -z "$why" ] \
      || { echo "change-approval: cannot delegate $CHANGES_DIR/$id: $why, so it was approved before (or an earlier change used the id $id) and only the $OWNER_ROLE approves it again: 'sh "$CLAUDUCTOR_FW"/change-approval.sh $id --record <owner>' on their word" >&2; exit 1; }
    # The log holds what a rewrite of the branch can take out of its history. A log that is missing
    # or cannot be read is not "never approved": it refuses, and the owner starts it (OPS-131, #272).
    [ -n "$approval_log" ] || { echo "change-approval: cannot delegate $CHANGES_DIR/$id: the repository's common git dir cannot be found, so its approval log cannot be read" >&2; exit 1; }
    [ -e "$approval_log" ] || { echo "change-approval: cannot delegate $CHANGES_DIR/$id: no approval log at $approval_log, so this clone cannot tell whether it was approved before; the $OWNER_ROLE starts it on their word: 'sh "$CLAUDUCTOR_FW"/change-approval.sh --start-log <owner>' (or approves this change: --record)" >&2; exit 1; }
    seen=$(awk -F'\t' -v id="$id" '$3 == "start" || $3 == "record" { started = 1 } $2 == id && $3 != "start" { print $1 " " $3 " by " $4 } END { if (!started) print "unstarted" }' "$approval_log" 2>/dev/null) && [ -r "$approval_log" ] \
      || { echo "change-approval: cannot delegate $CHANGES_DIR/$id: the approval log $approval_log cannot be read" >&2; exit 1; }
    # A revoke adds to a log the owner has not started: that log cannot say "never approved" either.
    [ "$(printf '%s\n' "$seen" | tail -1)" != unstarted ] || { echo "change-approval: cannot delegate $CHANGES_DIR/$id: the approval log $approval_log was never started by the $OWNER_ROLE, so this clone cannot tell whether it was approved before: 'sh "$CLAUDUCTOR_FW"/change-approval.sh --start-log <owner>' on their word (or approves this change: --record)" >&2; exit 1; }
    [ -z "$seen" ] || { echo "change-approval: cannot delegate $CHANGES_DIR/$id: this clone's approval log $approval_log shows it before ($(printf '%s\n' "$seen" | tail -1)), so only the $OWNER_ROLE approves it again: 'sh "$CLAUDUCTOR_FW"/change-approval.sh $id --record <owner>' on their word" >&2; exit 1; }
    log_append delegate "Claude (delegated by $owner)" || { echo "change-approval: cannot delegate $CHANGES_DIR/$id: the approval log $approval_log cannot be written" >&2; exit 1; }
    rewrite "**Approved:** ${APPROVAL_DATE:-$(date +%Y-%m-%d)} by Claude (delegated by $owner) · design $hash"
    echo "recorded: $CHANGES_DIR/$id approved by Claude, delegated by $owner, design $hash"
    ;;
  --record)
    owner=${2:-}; [ -n "$owner" ] || { echo "change-approval: --record needs the owner's name" >&2; exit 64; }
    claude_name "$owner" && { echo "change-approval: Claude's approval is recorded with --delegate, which checks the delegation (an owner named Claude sets OWNER_NAME)" >&2; exit 64; }
    log_append record "$owner" || { echo "change-approval: cannot record $CHANGES_DIR/$id: the approval log $approval_log cannot be written" >&2; exit 1; }
    rewrite "**Approved:** ${APPROVAL_DATE:-$(date +%Y-%m-%d)} by $owner · design $hash"
    echo "recorded: $CHANGES_DIR/$id approved by $owner, design $hash"
    ;;
  --revoke)
    line=$(grep -E '^\*\*Approved:\*\*' "$dir/proposal.md" | head -1)
    [ -n "$line" ] || { echo "revoked: $CHANGES_DIR/$id has no Approved line; it is awaiting approval already"; exit 0; }
    who=$(printf '%s\n' "$line" | sed -nE 's/^\*\*Approved:\*\* [0-9-]+ by (.+) · design .*/\1/p')
    # A revoke takes the approval away even when the log cannot record it: failing toward the owner
    # means the approval does not stand. It says so, since a rewrite of the branch could then undo it.
    log_append revoke "${who:-an unreadable line}"; logged=$?
    rewrite "**Status:** awaiting approval (revoked; was approved by ${who:-an unreadable line})"
    echo "revoked: $CHANGES_DIR/$id is awaiting approval again; only the $OWNER_ROLE approves it now (--record)"
    [ "$logged" = 0 ] || { echo "change-approval: but the approval log $approval_log cannot be written, so this revoke is only on the branch: fix the log, then commit the revoke before anything rewrites the branch" >&2; exit 1; }
    ;;
  '')
    line=$(grep -E '^\*\*Approved:\*\*' "$dir/proposal.md" | head -1)
    if [ -z "$line" ]; then echo "design $hash: not approved yet"; exit 1; fi
    printf '%s\n' "$line" | grep -Eq "$approved" || { echo "design $hash: the Approved line does not end '· design <12 hex>': $line"; exit 1; }
    rec=${line##* }
    by=""
    if printf '%s\n' "$line" | grep -Eq "$delegated"; then
      why=$(delegable); [ -z "$why" ] || { echo "design $hash: approved by Claude on delegation, which cannot hold: $why"; exit 1; }
      # --delegate writes over no Approved line, so an Approved line its history shows removed (a
      # revoke, or one rewritten) means this one was put back by hand, past --delegate (OPS-131).
      why=$(history_holds '^-\*\*Approved:\*\* ' "a removed Approved line"); [ -z "$why" ] \
        || { echo "design $hash: approved by Claude on delegation, which cannot hold: $why, so it was revoked or rewritten and only the $OWNER_ROLE approves it again"; exit 1; }
      by=", by Claude on delegation"
      who=$(printf '%s\n' "$line" | sed -E "s/$delegated.*/\\1/")
      [ -z "$OWNER_NAME" ] || [ "$who" = "$OWNER_NAME" ] || { echo "design $hash: delegated by $who, but only OWNER_NAME ($OWNER_NAME) delegates"; exit 1; }
    elif claude_name "$(printf '%s\n' "$line" | sed -E 's/^\*\*Approved:\*\* [0-9-]+ by (.+) · design [0-9a-f]{12}$/\1/')"; then
      echo "design $hash: an approval by Claude that is not '(delegated by <owner>)', the one form a delegation is checked in: $line"; exit 1
    fi
    if [ "$rec" = "$hash" ]; then echo "design $hash: approved as written$by"; exit 0; fi
    echo "design $hash: CHANGED since the owner approved design $rec; the approval no longer covers it"
    exit 1
    ;;
  *) echo "change-approval: unknown option $1" >&2; exit 64 ;;
esac
