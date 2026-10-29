#!/bin/sh
# session-start's team-queue section (OPS-95): one line of facts per open GitHub issue labelled
# `team-queue`, the owner's bot team's inbox. An issue is an INPUT to a roadmap row, never a row:
# docs/roadmap.md and its Session plan stay the only order. The handling rules are the local
# skill fragments (.claude/local/skills/session-start/team-queue.md, session-close/team-queue.md).
#
#   #N <row> <STATE> approval: source unverifiable (<why>) "<title>"
#
# STATE, the first that holds:
#   UNAPPROVED  not approved as the owner's rule defines it (below); <why> says which part failed
#   PROPOSES ROW  (OPS-100) a `new-row` item whose row is not on the roadmap yet: no `Row:` line,
#               no row whose Change cell carries `(team-queue #N)`, and one `Proposed-Lane:` that
#               is an existing id prefix (^[A-Z]{2,8}$); the row column reads `<LANE>-?`. The
#               owner's session adds the row with `--next-id`; the listing never prints the proposed
#               text, `--row N` prints it once the approval holds again in that same run
#   NEEDS ROW   no single `Row: <ID>` line, or one roadmap_queue does not know (or reads cancelled);
#               for a `new-row` item: a `Row:` line too, or two rows claiming it (row `conflict`),
#               or no claiming row and a lane that is malformed or no id's prefix (row `?`)
#   DONE        the row reads merged, or a commit body on origin/main has a line starting
#               `Delivers team-queue #N` (the delivering PR's body line; never `Closes #N`, which
#               closes the issue at merge, and never session close's `Team queue: #N → …` outcome
#               line, which also records open and blocked items)
#   STALE       its ```diff (or ```patch) block does not apply to origin/main
#   APPLIES     none of the above: no patch, or a patch that applies to origin/main
# A `new-row` item with exactly one roadmap row carrying `(team-queue #N)` in its Change cell is
# read as naming that row, and goes on to DONE, STALE or APPLIES. The `new-row` label grants
# nothing: approval is one rule for every item, and without the label `Proposed-*` lines mean
# nothing (`Row: NEW` is an unknown row).
# Approved means: both labels; exactly one `Content-SHA256: <hex>` line equal to the SHA-256 of the
# body without it (the canon below; `--hash FILE` prints it); and the last `approved` label event
# later than the body's last edit (lastEditedAt) and the last title change (RenamedTitleEvent).
# The bots sign in as the owner, so no author, actor or "Approved by" text is evidence; until the
# owner's test issue (docs/owner-queue.md) shows bot writes can be told apart, every line also
# says the label's source is unverifiable.
# An author is a GATE, though, not evidence (OPS-166): an issue whose authorAssociation is not
# OWNER, MEMBER or COLLABORATOR (a stranger's, once the repository is public; or one with no
# association at all) is skipped, nothing of it read but its number. Since OPS-155 the issue
# templates apply no label, so a stranger's issue reaches this query only when someone with
# triage rights labels it `team-queue`; this skip closes that path. Left open: a triager who
# labels many strangers' issues fills the 50 fetched (100 for --next-id), and writers' items past
# that window go unread (the listing says how many are not shown, `--row` finds no such item,
# `--next-id` refuses). The listing counts the skipped in one line and never prints their titles;
# `--row` refuses such an item, and `--next-id` reads no Row line from one. MEMBER and
# COLLABORATOR are GitHub's associations, not permissions: on an organization's repository either
# may hold read access only; on a personal repository there are no members, and a collaborator
# can write.
#
# WHY so narrow: the text is written by bots and lands in the session's context. So it prints no
# body, no patch and no comment (comments are never read), cleans titles of control and format
# characters and cuts them to 60, keeps the whole listing near 2,000 characters, and states facts
# without asking anything (session start runs hands-off). One GraphQL call with a 10 s deadline;
# on any failure exactly one CANNOT CHECK line and exit 0. Rows come only from roadmap_queue
# (checks/roadmap.sh fails a second parser); a patch is checked in a temporary index read from
# origin/main, never the working tree. The check is checks/team-queue.sh (local).
ROOT=$(cd "$(dirname "$0")/../../../.." 2>/dev/null && pwd)
# shellcheck disable=SC1091
. "$ROOT/.claude/lib/conf.sh"
# shellcheck disable=SC1091
. "$ROOT/.claude/lib/modules.sh"   # with_timeout
# shellcheck disable=SC1091
. "$ROOT/.claude/lib/change.sh"    # sha256_hex

cannot() {
  [ -z "${NEXT_ID:-}" ] || { echo "Next id: CANNOT CHECK ($1). Learned NOTHING; take no id."; exit 1; }
  [ -z "${ROW_N:-}" ] || { echo "Row #$ROW_N: CANNOT CHECK ($1). Learned NOTHING; add no row."; exit 1; }
  echo "Team queue: CANNOT CHECK ($1). Learned NOTHING; treat the queue as unknown."; exit 0
}

# The body the hash covers: CRs dropped, every line starting `Content-SHA256:` removed, the rest
# joined by LF, trailing whitespace dropped. The template (.github/ISSUE_TEMPLATE/team-queue.md)
# states the same rule.
CANON='gsub("\r"; "") | split("\n") | map(select(startswith("Content-SHA256:") | not)) | join("\n") | sub("\\s+\\z"; "")'
# An issue whose author GitHub associates as OWNER, MEMBER or COLLABORATOR (on a personal
# repository, those who can write; OPS-166). A missing association is none of them.
WRITER='(.authorAssociation == "OWNER" or .authorAssociation == "MEMBER" or .authorAssociation == "COLLABORATOR")'

if [ "${1:-}" = --hash ]; then
  [ -f "${2:-}" ] || { echo "usage: team-queue.sh --hash FILE" >&2; exit 2; }
  jq -Rsj "$CANON" "$2" | sha256_hex
  exit 0
fi
# --next-id PREFIX (OPS-100): the id a PROPOSES ROW item's row takes, PREFIX-<max+1>. The max is
# over every id taken or claimed: the roadmap's rows (the working tree's, origin/main's and each
# open PR's own), the history's (docs/roadmap-history.md, OPS-147), the `Row:` lines of the open team-queue issues a writer opened (OPS-166) and the leading `PREFIX-N:` of
# open PR titles. A PR's roadmap counts because a PR adds rows its title does not name (#203,
# titled OPS-105, added OPS-120 and OPS-122), and two PRs taking one id both pass their gates.
# Only ids that are exactly PREFIX-<1 to 9 digits> count (OPS-92a is skipped, never read as 92,
# and an issue's `Row: OPS-99999999999999999999` cannot overflow the sum); a reserved placeholder
# row still counts, so its id is never handed out, and a gap is never filled. Unlike the
# listing, any failure is one CANNOT CHECK line and exit 1: a guessed id is worse than none.
NEXT_ID=""
if [ "${1:-}" = --next-id ]; then
  NEXT_ID=$(printf '%s' "${2:-}" | grep -xE '[A-Z][A-Z0-9]{0,15}') \
    || { echo "usage: team-queue.sh --next-id PREFIX (capital letters and digits, e.g. OPS)" >&2; exit 2; }
fi
# --row N (OPS-100): the Proposed-* lines of item #N, printed only if it reads PROPOSES ROW in
# this same run, from the same fetched body the approval was checked against. The row is copied
# from this output, never from a later `gh issue view`: a body edited after the listing would
# otherwise become a roadmap row unapproved. Each of the six Proposed-* lines must appear once.
# Any other state or failure is one CANNOT CHECK line and exit 1.
ROW_N=""
if [ "${1:-}" = --row ]; then
  ROW_N=$(printf '%s' "${2:-}" | grep -xE '[1-9][0-9]{0,8}') \
    || { echo "usage: team-queue.sh --row N (an issue number)" >&2; exit 2; }
fi

[ "${CONTEXT_OFFLINE:-}" = 1 ] && cannot offline
command -v gh >/dev/null 2>&1 || cannot "gh is not installed"
command -v jq >/dev/null 2>&1 || cannot "jq is not installed"
cd "$ROOT" || cannot "cannot enter the project root"
[ "$(printf x | sha256_hex)" = "2d711642b726b04401627ca9fbac32f5c8530fb1903cc4db02258717921a4881" ] || cannot "no SHA-256 tool"
t=$(mktemp -d "${TMPDIR:-/tmp}/team-queue.XXXXXX") || cannot "no temporary directory"
trap 'rm -rf "$t"' EXIT

if [ -n "$NEXT_ID" ]; then
  # One call, under the same deadline, for both lists; more than one page of either is refused,
  # not read short (a missed id is a duplicate id later).
  NQ='query($owner: String!, $name: String!) { repository(owner: $owner, name: $name) {
    issues(first: 100, states: OPEN, labels: ["team-queue"]) { totalCount nodes { body authorAssociation } }
    pullRequests(first: 100, states: OPEN) { totalCount nodes { number title headRefOid } } } }'
  with_timeout "${TEAM_QUEUE_TIMEOUT:-10}" gh api graphql -f query="$NQ" -F owner='{owner}' -F name='{repo}' </dev/null >"$t/resp" 2>/dev/null
  rc=$?
  case $rc in
    0) ;;
    4) cannot "gh is not logged in" ;;
    137 | 143) cannot "gh did not answer within ${TEAM_QUEUE_TIMEOUT:-10} s" ;;
    *) cannot "gh exited $rc" ;;
  esac
  jq -e '(.errors // [] | length == 0) and (.data.repository.issues.nodes | type == "array")
    and (.data.repository.issues.nodes | all(type == "object"))
    and (.data.repository.pullRequests.nodes | type == "array")' "$t/resp" >/dev/null 2>&1 \
    || cannot "GitHub's answer is an error or does not parse"
  jq -e '.data.repository | (.issues.totalCount <= (.issues.nodes | length)) and (.pullRequests.totalCount <= (.pullRequests.nodes | length))' "$t/resp" >/dev/null 2>&1 \
    || cannot "more than 100 open team-queue issues or PRs"
  roadmap_queue --tsv > "$t/tsv" 2>/dev/null || cannot "the change queue is UNKNOWN"
  with_timeout "${TEAM_QUEUE_TIMEOUT:-10}" git fetch -q origin "$MAIN_BRANCH" </dev/null >/dev/null 2>&1 \
    && git show "origin/$MAIN_BRANCH:$ROADMAP" > "$t/main.md" 2>/dev/null || cannot "origin/$MAIN_BRANCH's roadmap could not be read"
  roadmap_queue --tsv "$t/main.md" >> "$t/tsv" 2>/dev/null || cannot "origin/$MAIN_BRANCH's change queue is UNKNOWN"
  # Each open PR's own roadmap, at the head GitHub named: one fetch of every open PR's head, and a
  # head that cannot be read is CANNOT CHECK. A head with no roadmap at all adds no ids.
  jq -r '.data.repository.pullRequests.nodes[] | "\(.number) \(.headRefOid)"' "$t/resp" > "$t/prs" 2>/dev/null \
    || cannot "the open PRs could not be read"
  if [ -s "$t/prs" ]; then
    grep -vxE '[1-9][0-9]{0,8} [0-9a-f]{40}' "$t/prs" >/dev/null && cannot "an open PR's number or head does not parse"
    # shellcheck disable=SC2046 # the refspecs are digits and fixed words, one per PR
    with_timeout "${TEAM_QUEUE_TIMEOUT:-10}" git fetch -q origin $(awk '{ print "refs/pull/" $1 "/head" }' "$t/prs") </dev/null >/dev/null 2>&1 \
      || cannot "the open PRs' heads could not be fetched"
    while read -r n oid; do
      git cat-file -e "$oid^{commit}" 2>/dev/null || cannot "#$n's head could not be read"
      git cat-file -e "$oid:$ROADMAP" 2>/dev/null || continue
      { git show "$oid:$ROADMAP" > "$t/pr.md" && roadmap_queue --tsv "$t/pr.md" >> "$t/tsv"; } 2>/dev/null \
        || cannot "#$n's change queue is UNKNOWN"
    done < "$t/prs"
  fi
  # The history's ids are taken too (OPS-147): the reset moved finished rows out of the roadmap,
  # and REL-2 and o.10 were above everything the roadmap still held.
  sh "$ROOT/.claude/local/roadmap.d/history.sh" --ids > "$t/hist" || cannot "docs/roadmap-history.md's ids could not be read"
  # Each source with its own status: in a group or a pipe only the last command's would count, and
  # a source read short is a duplicate id later.
  { awk -F'\t' '{ print $4 }' "$t/tsv" && cat "$t/hist"; } > "$t/ids" || cannot "the ids could not be read"
  jq -r ".data.repository.issues.nodes[] | select($WRITER)"' | .body // "" | gsub("\r"; "") | split("\n")[]
    | select(startswith("Row:")) | sub("^Row:[ \t]*"; "") | sub("[ \t]+$"; "")' "$t/resp" >> "$t/ids" 2>/dev/null \
    || cannot "the open issues' Row lines could not be read"
  jq -r '.data.repository.pullRequests.nodes[].title // ""' "$t/resp" > "$t/titles" 2>/dev/null \
    || cannot "the open PRs' titles could not be read"
  sed -n "s/^\($NEXT_ID-[0-9][0-9]*\):.*/\1/p" "$t/titles" >> "$t/ids" || cannot "the ids could not be read"
  max=$(grep -xE "$NEXT_ID-[0-9]{1,9}" "$t/ids" | sed "s/^$NEXT_ID-//" | awk '$1 + 0 > m { m = $1 + 0 } END { print m + 0 }')
  [ "${max:-0}" -gt 0 ] || cannot "no id is $NEXT_ID-<number>"
  echo "$NEXT_ID-$((max + 1))"
  exit 0
fi

# The one call. {owner} and {repo} are gh's placeholders for this checkout's repository.
Q='query($owner: String!, $name: String!) { repository(owner: $owner, name: $name) {
  issues(first: 50, states: OPEN, labels: ["team-queue"], orderBy: {field: CREATED_AT, direction: ASC}) {
    totalCount
    nodes { number title body lastEditedAt authorAssociation
      labels(first: 30) { nodes { name } }
      timelineItems(last: 100, itemTypes: [LABELED_EVENT, RENAMED_TITLE_EVENT]) { nodes { __typename
        ... on LabeledEvent { createdAt label { name } }
        ... on RenamedTitleEvent { createdAt } } } } } } }'
with_timeout "${TEAM_QUEUE_TIMEOUT:-10}" gh api graphql -f query="$Q" -F owner='{owner}' -F name='{repo}' </dev/null >"$t/resp" 2>/dev/null
rc=$?
case $rc in
  0) ;;
  4) cannot "gh is not logged in" ;;
  137 | 143) cannot "gh did not answer within ${TEAM_QUEUE_TIMEOUT:-10} s" ;;
  *) cannot "gh exited $rc" ;;
esac
# A node that is not an object is refused here, never counted below as a stranger's issue.
jq -e '(.errors // [] | length == 0) and (.data.repository.issues.nodes | type == "array")
  and (.data.repository.issues.nodes | all(type == "object"))' "$t/resp" >/dev/null 2>&1 \
  || cannot "GitHub's answer is an error or does not parse"
# fetch_main's fetch, under the same deadline (a slow network must not hold session start).
with_timeout "${TEAM_QUEUE_TIMEOUT:-10}" git fetch -q origin "$MAIN_BRANCH" </dev/null >/dev/null 2>&1 \
  && git rev-parse -q --verify "origin/$MAIN_BRANCH" >/dev/null 2>&1 || cannot "origin/$MAIN_BRANCH could not be fetched"
tsv=$(roadmap_queue --tsv 2>/dev/null) || cannot "the change queue is UNKNOWN"
GIT_INDEX_FILE="$t/index" git read-tree "origin/$MAIN_BRANCH" 2>/dev/null || cannot "cannot read origin/$MAIN_BRANCH into a temporary index"
# Issue numbers a commit body on origin/main names as delivered: a line starting
# `Delivers team-queue #N`. A bare #N is not enough (bodies name open issues in passing: #143, the
# flake, in many); a closing keyword closes the issue at merge, so it never reaches this listing;
# and the journal's `Team queue: #N → <row> <outcome>` line also records open and blocked items.
git log --format=%b "origin/$MAIN_BRANCH" 2>/dev/null \
  | grep -oE '^Delivers team-queue #[0-9]+' | grep -oE '[0-9]+$' | sort -u > "$t/closed"

total=$(jq -r '.data.repository.issues.totalCount // 0' "$t/resp")
n=$(jq -r '.data.repository.issues.nodes | length' "$t/resp")
if [ "$n" -eq 0 ]; then [ -z "$ROW_N" ] || cannot "no open team-queue issues"; echo "Team queue: no open team-queue issues."; exit 0; fi
# Lines collect in a file and print only after the loop, so a failure part-way prints the one
# CANNOT CHECK line and no facts beside it.
echo "Team queue: $total open team-queue issue(s); facts only, handled by the team-queue step." > "$t/out"
# The fetched issues whose author cannot write (OPS-166), counted over all of them, so the count
# does not depend on where the size cut falls.
skipped=$(jq "[.data.repository.issues.nodes[] | select($WRITER | not)] | length" "$t/resp") \
  || cannot "the issues' authors could not be read"
used=200; shown=0
[ "$skipped" -gt 0 ] && used=270   # room for the skipped line
i=0
while [ "$i" -lt "$n" ]; do
  jq -c ".data.repository.issues.nodes[$i]" "$t/resp" > "$t/issue"
  i=$((i + 1))
  # A stranger's issue: nothing of it is read but its number, which --row needs for its refusal.
  if ! jq -e "$WRITER" "$t/issue" >/dev/null 2>&1; then
    [ -n "$ROW_N" ] && jq -e --argjson n "$ROW_N" '.number == $n' "$t/issue" >/dev/null 2>&1 \
      && cannot "#$ROW_N's author has no write access"
    continue
  fi
  # One line of facts, fields split by US (\037; a TAB would collapse empty fields): the number,
  # the approved label and its last event (else ""), why the dates refuse it (else ""), the count
  # of Content-SHA256 lines, the claimed hash, the count of Row lines and the row; then the
  # new-row label (1 or 0), the count of Proposed-Lane lines and the lane.
  facts=$(jq -r '
    (.body // "" | gsub("\r"; "") | split("\n")) as $l
    | ([.timelineItems.nodes[] | select(.__typename == "LabeledEvent" and .label.name == "approved") | .createdAt] | max // "") as $at
    | ([.timelineItems.nodes[] | select(.__typename == "RenamedTitleEvent") | .createdAt] | max // "") as $ren
    | [ (.number | tostring),
        (if ([.labels.nodes[].name] | index("approved")) and $at != "" then "1" else "0" end),
        (if (.lastEditedAt // "") != "" and ($at > .lastEditedAt | not) then "body edited after the label"
         elif $ren != "" and ($at > $ren | not) then "title renamed after the label" else "" end),
        ([$l[] | select(startswith("Content-SHA256:"))] | length | tostring),
        ([$l[] | select(startswith("Content-SHA256:")) | sub("^Content-SHA256:[ \t]*"; "") | sub("[ \t]+$"; "") | ascii_downcase] | .[0] // ""),
        ([$l[] | select(startswith("Row:"))] | length | tostring),
        ([$l[] | select(startswith("Row:")) | sub("^Row:[ \t]*"; "") | sub("[ \t]+$"; "")] | .[0] // ""),
        (if [.labels.nodes[].name] | index("new-row") then "1" else "0" end),
        ([$l[] | select(startswith("Proposed-Lane:"))] | length | tostring),
        ([$l[] | select(startswith("Proposed-Lane:")) | sub("^Proposed-Lane:[ \t]*"; "") | sub("[ \t]+$"; "")] | .[0] // "") ]
    | map(gsub("[\\p{Cc}]"; " ")) | join("\u001f")' "$t/issue") || cannot "an issue does not parse"
  # Read from a file, not a here-document: a sandbox may refuse the shell its here-doc temp file.
  printf '%s\n' "$facts" > "$t/facts"
  IFS=$(printf '\037') read -r num lab datewhy nh claimed nr row newrow nl lane < "$t/facts"
  case $num in '' | *[!0-9]*) cannot "an issue has no number" ;; esac
  [ -n "$ROW_N" ] && [ "$num" != "$ROW_N" ] && continue
  # Control, format and line/paragraph separators (U+2028, U+2029) all become spaces.
  title=$(jq -r '.title // "" | gsub("[\\p{Cc}\\p{Cf}\\p{Zl}\\p{Zp}]"; " ") | gsub("\""; "'"'"'") | gsub(" +"; " ") | sub("^ "; "") | .[0:60]' "$t/issue")

  # Approval, bound to the content.
  ok="label, hash and dates hold"
  why=$ok
  if [ "$lab" != 1 ]; then why="no approved label"
  elif [ "$nh" != 1 ]; then why="needs one Content-SHA256 line"
  elif [ "$(jq -j '.body // ""' "$t/issue" | jq -Rsj "$CANON" | sha256_hex)" != "$claimed" ]; then why="hash mismatch"
  elif [ -n "$datewhy" ]; then why=$datewhy
  fi

  # The row, only through roadmap_queue. An id outside a plain charset is never echoed.
  rstate=""
  case $row in
    '') shown_row=- ;;
    *[!A-Za-z0-9._-]*) shown_row='?' ;;
    *) shown_row=$(printf '%.24s' "$row")
       [ "$nr" = 1 ] && rstate=$(printf '%s\n' "$tsv" | awk -F'\t' -v id="$row" '!got && $4 == id { print $7; got = 1 }') ;;
  esac
  # A new-row item (OPS-100) names no row: its row is the one whose Change cell carries
  # `(team-queue #N)`, once the owner's session has added it. Until then it proposes one, in a
  # lane that must already be an id prefix; its proposed text is never read here.
  proposes=""
  if [ "$newrow" = 1 ]; then
    rstate=""
    if [ "$nr" != 0 ]; then shown_row=conflict
    else
      printf '%s\n' "$tsv" | awk -F'\t' -v tag="(team-queue #$num)" 'index($10, tag) { print $4; print $7 }' > "$t/claims"
      nc=$(($(wc -l < "$t/claims") / 2))
      if [ "$nc" -ge 2 ]; then shown_row=conflict
      elif [ "$nc" = 1 ]; then shown_row=$(sed -n '1s/^\(.\{0,24\}\).*/\1/p' "$t/claims"); rstate=$(sed -n 2p "$t/claims")
      elif [ "$nl" = 1 ] && printf '%s\n' "$lane" | grep -qxE '[A-Z]{2,8}' \
        && printf '%s\n' "$tsv" | awk -F'\t' -v p="$lane-" 'index($4, p) == 1 { f = 1 } END { exit !f }'; then
        proposes=1; shown_row="$lane-?"
      else shown_row='?'; fi
    fi
  fi

  if [ "$why" != "$ok" ]; then state=UNAPPROVED
  elif [ -n "$proposes" ]; then state="PROPOSES ROW"
  elif [ -z "$rstate" ] || [ "$rstate" = cancelled ]; then state="NEEDS ROW"
  elif [ "$rstate" = merged ] || grep -qx "$num" "$t/closed"; then state=DONE
  else
    jq -j '.body // "" | gsub("\r"; "")' "$t/issue" \
      | awk 'p == 0 && /^```(diff|patch)[[:space:]]*$/ { p = 1; next } p == 1 && /^```[[:space:]]*$/ { exit } p == 1' > "$t/patch"
    if [ -s "$t/patch" ] && ! GIT_INDEX_FILE="$t/index" git apply --cached --check "$t/patch" >/dev/null 2>&1; then state=STALE
    else state=APPLIES; fi
  fi

  if [ -n "$ROW_N" ]; then
    [ "$state" = "PROPOSES ROW" ] || cannot "#$num reads $state ($why), not PROPOSES ROW"
    jq -r '.body // "" | gsub("\r"; "") | split("\n") | map(select(test("^Proposed-(Lane|Change|Scope|Deps|Section|Slot):")))
      | map(gsub("[\\p{Cc}\\p{Cf}\\p{Zl}\\p{Zp}]"; " ")) | .[]' "$t/issue" > "$t/proposed" || cannot "#$num's body does not parse"
    for k in Lane Change Scope Deps Section Slot; do
      [ "$(grep -c "^Proposed-$k:" "$t/proposed")" = 1 ] || cannot "#$num needs one Proposed-$k line"
    done
    # The row's Change cell gets ` (team-queue #<num>)` appended; one already in the text would
    # let this item's row claim another item.
    grep -q '^Proposed-Change:.*(team-queue #' "$t/proposed" && cannot "#$num's Proposed-Change already names a team-queue item"
    cat "$t/proposed"
    exit 0
  fi

  line="#$num $shown_row $state approval: source unverifiable ($why) \"$title\""
  len=$(printf '%s' "$line" | wc -m | tr -d ' ')
  if [ $((used + len + 1)) -gt 1900 ]; then break; fi
  used=$((used + len + 1)); shown=$((shown + 1))
  printf '%s\n' "$line" >> "$t/out"
done
[ -z "$ROW_N" ] || cannot "#$ROW_N is not among the open team-queue issues read"
[ "$skipped" -gt 0 ] && echo "$skipped team-queue issue(s) from authors without write access skipped" >> "$t/out"
[ $((shown + skipped)) -lt "$total" ] && echo "... and $((total - shown - skipped)) more not shown (the listing stops near 2,000 characters; at most 50 are fetched)." >> "$t/out"
cat "$t/out"
exit 0
