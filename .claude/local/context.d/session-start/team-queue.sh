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
#   NEEDS ROW   no single `Row: <ID>` line, or one roadmap_queue does not know (or reads cancelled)
#   DONE        the row reads merged, or a commit body on origin/main has a line starting
#               `Delivers team-queue #N` (the delivering PR's body line; never `Closes #N`, which
#               closes the issue at merge, and never session close's `Team queue: #N → …` outcome
#               line, which also records open and blocked items)
#   STALE       its ```diff (or ```patch) block does not apply to origin/main
#   APPLIES     none of the above: no patch, or a patch that applies to origin/main
# Approved means: both labels; exactly one `Content-SHA256: <hex>` line equal to the SHA-256 of the
# body without it (the canon below; `--hash FILE` prints it); and the last `approved` label event
# later than the body's last edit (lastEditedAt) and the last title change (RenamedTitleEvent).
# The bots sign in as the owner, so no author, actor or "Approved by" text is evidence; until the
# owner's test issue (docs/owner-queue.md) shows bot writes can be told apart, every line also
# says the label's source is unverifiable.
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

cannot() { echo "Team queue: CANNOT CHECK ($1). Learned NOTHING; treat the queue as unknown."; exit 0; }

# The body the hash covers: CRs dropped, every line starting `Content-SHA256:` removed, the rest
# joined by LF, trailing whitespace dropped. The template (.github/ISSUE_TEMPLATE/team-queue.md)
# states the same rule.
CANON='gsub("\r"; "") | split("\n") | map(select(startswith("Content-SHA256:") | not)) | join("\n") | sub("\\s+\\z"; "")'

if [ "${1:-}" = --hash ]; then
  [ -f "${2:-}" ] || { echo "usage: team-queue.sh --hash FILE" >&2; exit 2; }
  jq -Rsj "$CANON" "$2" | sha256_hex
  exit 0
fi

[ "${CONTEXT_OFFLINE:-}" = 1 ] && cannot offline
command -v gh >/dev/null 2>&1 || cannot "gh is not installed"
command -v jq >/dev/null 2>&1 || cannot "jq is not installed"
cd "$ROOT" || cannot "cannot enter the project root"
[ "$(printf x | sha256_hex)" = "2d711642b726b04401627ca9fbac32f5c8530fb1903cc4db02258717921a4881" ] || cannot "no SHA-256 tool"
t=$(mktemp -d "${TMPDIR:-/tmp}/team-queue.XXXXXX") || cannot "no temporary directory"
trap 'rm -rf "$t"' EXIT

# The one call. {owner} and {repo} are gh's placeholders for this checkout's repository.
Q='query($owner: String!, $name: String!) { repository(owner: $owner, name: $name) {
  issues(first: 50, states: OPEN, labels: ["team-queue"], orderBy: {field: CREATED_AT, direction: ASC}) {
    totalCount
    nodes { number title body lastEditedAt
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
jq -e '(.errors // [] | length == 0) and (.data.repository.issues.nodes | type == "array")' "$t/resp" >/dev/null 2>&1 \
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
if [ "$n" -eq 0 ]; then echo "Team queue: no open team-queue issues."; exit 0; fi
# Lines collect in a file and print only after the loop, so a failure part-way prints the one
# CANNOT CHECK line and no facts beside it.
echo "Team queue: $total open team-queue issue(s); facts only, handled by the team-queue step." > "$t/out"
used=200; shown=0
i=0
while [ "$i" -lt "$n" ]; do
  jq -c ".data.repository.issues.nodes[$i]" "$t/resp" > "$t/issue"
  i=$((i + 1))
  # One line of facts, fields split by US (\037; a TAB would collapse empty fields): the number,
  # the approved label and its last event (else ""), why the dates refuse it (else ""), the count
  # of Content-SHA256 lines, the claimed hash, the count of Row lines and the row.
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
        ([$l[] | select(startswith("Row:")) | sub("^Row:[ \t]*"; "") | sub("[ \t]+$"; "")] | .[0] // "") ]
    | map(gsub("[\\p{Cc}]"; " ")) | join("\u001f")' "$t/issue") || cannot "an issue does not parse"
  # Read from a file, not a here-document: a sandbox may refuse the shell its here-doc temp file.
  printf '%s\n' "$facts" > "$t/facts"
  IFS=$(printf '\037') read -r num lab datewhy nh claimed nr row < "$t/facts"
  case $num in '' | *[!0-9]*) cannot "an issue has no number" ;; esac
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
       [ "$nr" = 1 ] && rstate=$(printf '%s\n' "$tsv" | awk -F'\t' -v id="$row" '$4 == id { print $7; exit }') ;;
  esac

  if [ "$why" != "$ok" ]; then state=UNAPPROVED
  elif [ -z "$rstate" ] || [ "$rstate" = cancelled ]; then state="NEEDS ROW"
  elif [ "$rstate" = merged ] || grep -qx "$num" "$t/closed"; then state=DONE
  else
    jq -j '.body // "" | gsub("\r"; "")' "$t/issue" \
      | awk 'p == 0 && /^```(diff|patch)[[:space:]]*$/ { p = 1; next } p == 1 && /^```[[:space:]]*$/ { exit } p == 1' > "$t/patch"
    if [ -s "$t/patch" ] && ! GIT_INDEX_FILE="$t/index" git apply --cached --check "$t/patch" >/dev/null 2>&1; then state=STALE
    else state=APPLIES; fi
  fi

  line="#$num $shown_row $state approval: source unverifiable ($why) \"$title\""
  len=$(printf '%s' "$line" | wc -m | tr -d ' ')
  if [ $((used + len + 1)) -gt 1900 ]; then break; fi
  used=$((used + len + 1)); shown=$((shown + 1))
  printf '%s\n' "$line" >> "$t/out"
done
[ "$shown" -lt "$total" ] && echo "... and $((total - shown)) more not shown (the listing stops near 2,000 characters; at most 50 are fetched)." >> "$t/out"
cat "$t/out"
exit 0
