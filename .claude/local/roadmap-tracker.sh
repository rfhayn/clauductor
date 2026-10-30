#!/bin/sh
# roadmap-tracker.sh: the data of the owner's roadmap tracker, a private dashboard, built from the
# roadmap so the tracker stays current by mechanism, not by someone remembering to edit it
# (OPS-83). POSIX sh, git and jq; deterministic and offline (local git only). The tracker's address
# is never committed (the repository is public): it lives per machine in
# <git common dir>/stridal/tracker-url, which `--url` prints (exit 1 when there is none).
#
#   sh .claude/local/roadmap-tracker.sh DIR [SINCE]   write DIR/roadmap.json and DIR/sessions.json
#                                                    (SINCE: YYYY-MM-DD, default today)
#   sh .claude/local/roadmap-tracker.sh --mark DIR    after a successful upload of DIR: stamp the
#                                                    roadmap blob DIR was built from
#   sh .claude/local/roadmap-tracker.sh --url         this machine's tracker address, or exit 1
#                                                    "no tracker configured on this machine"
#
# Rows and their states come from roadmap_queue (lib/conf.sh), never a second parser of the queue
# tables. The sessions, their order and the owner's decision per row come from the roadmap's
# "## Session plan" table through .claude/roadmap-plan.sh, its one parser, which the panel's Up
# next reads too: its header is not the queue header, so the queue parser skips it. A row's track is its ### section (or "Phase N"),
# and its phase is the queue's phase number (0 outside every phase), which the tracker's page groups
# the sessions by (OPS-148).
#
# State mapping (roadmap -> tracker):
#   merged -> merged, kept only when its squash commit "(#N)" on HEAD is dated SINCE or later
#   in flight with a PR -> merging; in flight without one (a plugged parser) -> running
#   queued, or in flight without a PR, whose <BRANCH_CHANGE><id> branch is checked out in a
#     worktree whose <CHANGES_DIR>/<id>/tasks.md has a ticked task -> running (a build under way)
#   queued otherwise -> approved when <CHANGES_DIR>/<id>/proposal.md or design.md has an
#     **Approved:** line, else queued
#   an open word (parked, deferred, ...) -> parked;  cancelled -> cancelled (only when planned)
# pr is the queue's PR field, kept only for a row in flight or merged: the template's parser fills
# it from any (#N) in the status, which on an open row is often an issue or a proposal PR.
# Rows: every row the plan names, except a merged one the filter drops and none it names that is
# no roadmap row (warned); every open row (queued, in flight, parked) it does not name, as session
# "Unplanned" (or "Parked"); and every merged row the filter keeps.
# A row not merged that .claude/local/cloud-lanes.tsv lists (<id> TAB <routine name>) is noted
# "cloud: <routine name>", so the owner sees which work runs in the cloud.
#
# The stamp, <git common dir>/stridal/tracker-refreshed, holds the roadmap blob of the last
# upload; context.d/session-start/roadmap-tracker.sh compares it with today's roadmap.
ROOT=$(cd "$(dirname "$0")/../.." 2>/dev/null && pwd)
# shellcheck disable=SC1091
. "$ROOT/.claude/lib/conf.sh"

die() { echo "roadmap-tracker: $*" >&2; exit 1; }
command -v jq >/dev/null 2>&1 || die "jq is not installed"

stamp_path() {
  _c=$(git -C "$ROOT" rev-parse --path-format=absolute --git-common-dir 2>/dev/null) || return 1
  printf '%s/stridal/tracker-refreshed' "$_c"
}

if [ "${1:-}" = --url ]; then
  s=$(stamp_path) || die "not in a git repository: $ROOT"
  u=$(head -n 1 "$(dirname "$s")/tracker-url" 2>/dev/null)
  [ -n "$u" ] || die "no tracker configured on this machine (tracker-url)"
  printf '%s\n' "$u"
  exit 0
fi

if [ "${1:-}" = --mark ]; then
  [ -n "${2:-}" ] || die "usage: --mark DIR"
  b=$(cat "$2/roadmap.blob" 2>/dev/null) || die "$2 holds no roadmap.blob: build it first (roadmap-tracker.sh $2)"
  [ -n "$b" ] || die "$2/roadmap.blob is empty"
  s=$(stamp_path) || die "not in a git repository: $ROOT"
  mkdir -p "$(dirname "$s")" && printf '%s\n' "$b" > "$s" || die "cannot write $s"
  echo "roadmap-tracker: stamped $s with ${b%"${b#???????}"}"
  exit 0
fi

out=${1:-}
[ -n "$out" ] || die "usage: roadmap-tracker.sh DIR [SINCE] | --mark DIR"
since=${2:-$(date +%Y-%m-%d)}
case $since in [0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]) ;; *) die "SINCE must be YYYY-MM-DD, not '$since'" ;; esac

# A blob left from an earlier build must never be marked for this one.
rm -f "$out/roadmap.blob"
# Read here for its Session plan section only; the rows come from roadmap_queue. checks/roadmap.sh's
# bypass scan allows this file by path, in its own case list.
rmfile=$ROOT/$ROADMAP
[ -f "$rmfile" ] || die "$ROADMAP is missing"
blob=$(git -C "$ROOT" hash-object -- "$ROADMAP") || die "cannot hash $ROADMAP"

tsv=$(roadmap_queue --tsv "$rmfile") || die "the change queue is UNKNOWN: $tsv"

# The plan, as tab-separated records:  S session order when focus owner  |  R id session order pos
# kind lead  |  D id decision.  Any shape it cannot read is an error, never a shorter plan.
plan=$(sh "$ROOT/.claude/roadmap-plan.sh" "$rmfile") || die "the Session plan cannot be read (above)"
# The template's parser treats the section as optional; the tracker has nothing to show without it.
printf '%s\n' "$plan" | grep -q '^S	' || die "no \"## Session plan\" section in $ROADMAP"

# The approved queued changes: an **Approved:** line in their proposal or design.
approved=$(printf '%s\n' "$tsv" | awk -F'\t' '$7 == "queued" { print $5 }' | while read -r cid; do
  for f in proposal.md design.md; do
    if grep -q '^\*\*Approved:\*\*' "$ROOT/$CHANGES_DIR/$cid/$f" 2>/dev/null; then echo "$cid"; break; fi
  done
done)

# The changes being built: <BRANCH_CHANGE><id> checked out in a worktree whose tasks.md there has
# a ticked task. A branch that only exists, or a worktree with nothing ticked, is not a build.
building=$(git -C "$ROOT" worktree list --porcelain 2>/dev/null | awk -v pre="refs/heads/$BRANCH_CHANGE" '
  /^worktree / { wt = substr($0, 10) }
  /^branch / && index(substr($0, 8), pre) == 1 { print substr($0, 8 + length(pre)) "\t" wt }' |
  while IFS="$(printf '\t')" read -r cid wt; do
    grep -q '^[[:space:]]*- \[[xX]\]' "$wt/$CHANGES_DIR/$cid/tasks.md" 2>/dev/null && echo "$cid"
  done)

# Merge dates: the squash commits on HEAD, whose subjects end "(#N)".
merges=$(git -C "$ROOT" log --format='%cs %s' HEAD 2>/dev/null | sed -n 's/^\([0-9-]*\) .*(#\([0-9][0-9]*\))$/\2 \1/p')

mkdir -p "$out" || die "cannot create $out"
printf '%s\n' "$tsv" > "$out/.tsv"
printf '%s\n' "$plan" > "$out/.plan"
printf '%s\n' "$approved" > "$out/.approved"
printf '%s\n' "$building" > "$out/.building"
printf '%s\n' "$merges" > "$out/.merges"
# The cloud lanes: comments and blank lines are skipped; any other line that is not exactly an id
# and a routine name, TAB-separated, is warned about and left out, never dropped silently.
: > "$out/.cloud"
if [ -f "$ROOT/.claude/local/cloud-lanes.tsv" ]; then
  awk -F'\t' '/^#/ || /^[[:space:]]*$/ { next }
    NF == 2 && $1 != "" && $2 != "" { print $1 "\t" $2; next }
    { printf "roadmap-tracker: warning: cloud-lanes.tsv line %d is not <id> TAB <routine name>; left out\n", NR > "/dev/stderr" }' \
    "$ROOT/.claude/local/cloud-lanes.tsv" > "$out/.cloud"
fi
jq -n --arg since "$since" --rawfile cloud "$out/.cloud" \
  --rawfile tsv "$out/.tsv" --rawfile plan "$out/.plan" --rawfile approved "$out/.approved" \
  --rawfile building "$out/.building" --rawfile merges "$out/.merges" '
  def lines($s): $s | split("\n") | map(select(length > 0) | split("\t"));
  # A title is the summary up to its first clause break, when that head has three words or more,
  # without a parenthesis the cut left open, at most 110 characters, capitalised when it starts
  # with a plain word (never "v0.1.0" or a file name).
  def cut($d): split($d) as $p | if ($p | length) > 1 and ($p[0] | test(" .* ")) then $p[0] else . end;
  def short: gsub("`"; "") | cut(": ") | cut("; ") | cut(" — ") | cut(". ") | sub(" \\([^)]*$"; "")
    | (if length > 110 then (.[0:107] | sub(" [^ ]*$"; "")) + "…" else . end)
    | (if test("^[a-z]+ ") then (.[0:1] | ascii_upcase) + .[1:] else . end);
  (lines($plan)) as $p
  | ([$p[] | select(.[0] == "R") | {key: .[1], value: {session: .[2], order: (.[3] | tonumber), pos: (.[4] | tonumber)}}] | from_entries) as $where
  | ([$p[] | select(.[0] == "D") | {key: .[1], value: .[2]}] | from_entries) as $owner
  | ([lines($cloud)[] | {key: .[0], value: .[1]}] | from_entries) as $cloudlane
  | ($approved | split("\n") | map(select(length > 0))) as $ok
  | ($building | split("\n") | map(select(length > 0))) as $built
  | ([$merges | split("\n")[] | select(length > 0) | split(" ") | {key: .[0], value: .[1]}] | reverse | from_entries) as $merged
  | [lines($tsv)[] | {line: (.[0] | tonumber), phase: .[1], section: .[2], id: .[3], cid: .[4], kind: .[5],
      st: .[6], pr: .[7], summary: .[9], budget: .[10], due: .[11], deps: (.[14] // "")}
    | . as $r
    | ($where[.id]) as $w
    | (if .st == "merged" then "merged"
       elif .st == "inflight" and .pr != "" then "merging"
       elif .st == "inflight" then "running"
       elif .st == "queued" and ($built | any(. == $r.cid)) then "running"
       elif .st == "queued" then (if ($ok | any(. == $r.cid)) then "approved" else "queued" end)
       elif .st == "open" then "parked"
       elif .st == "cancelled" then "cancelled"
       else error("row \(.id): unknown state \(.st)") end) as $state
    | ($merged[.pr] // "") as $md
    | select(if $state == "merged" then ($md != "" and $md >= $since)
             elif $state == "cancelled" then $w != null
             else true end)
    | {id, title: (.summary | short),
       phase: (if .phase == "-" then 0 else ($r.phase | tonumber? // error("row \($r.id): phase \($r.phase) is not a number; the tracker groups by phase number")) end),
       track: (if .section != "" then (.section | sub(" \\(.*$"; "")) elif .phase != "-" then "Phase \(.phase)" else "Outside the phases" end),
       kind,
       session: (if $w then $w.session elif $state == "parked" then "Parked" else "Unplanned" end),
       order: (if $w then $w.order elif $state == "parked" then 99 else 98 end),
       state: $state, pr: (if .pr != "" and (.st == "inflight" or .st == "merged") then "#\(.pr)" else "" end),
       owner: ($owner[.id] // "" | (.[0:1] | ascii_upcase) + .[1:]),
       note: ([(if .deps != "" and ($state != "merged") then "after " + (.deps | gsub(","; ", ")) else empty end),
               (if .budget != "" then "budget $\(.budget)" else empty end),
               (if .due != "" then "due \(.due)" else empty end),
               (if $state == "merged" then "merged \($md)" else empty end),
               (if $state != "merged" and $cloudlane[.id] then "cloud: \($cloudlane[.id])" else empty end)] | join("; ")),
       _pos: ($w.pos // 0), _line: .line}]
  | sort_by(.order, ._pos, ._line) | map(del(._pos, ._line))' > "$out/roadmap.json" || die "cannot build roadmap.json"

jq -n --rawfile plan "$out/.plan" '
  [$plan | split("\n")[] | select(length > 0) | split("\t") | select(.[0] == "S")
   | {session: .[1], order: (.[2] | tonumber), when: .[3], focus: .[4], owner: .[5]}]' > "$out/sessions.json" || die "cannot build sessions.json"

# A planned id that is no roadmap row, or a decision about one, is left out and said so: a row
# another PR adds is the usual cause, a typo the other.
printf '%s\n' "$plan" | awk -F'\t' '$1 == "R" || $1 == "D" { print $2 }' | sort -u | while read -r id; do
  printf '%s\n' "$tsv" | awk -F'\t' -v id="$id" '$4 == id { f = 1 } END { exit !f }' \
    || echo "roadmap-tracker: warning: the Session plan names $id, which is no roadmap row; left out" >&2
done
cut -f1 "$out/.cloud" | while read -r id; do
  printf '%s\n' "$tsv" | awk -F'\t' -v id="$id" '$4 == id { f = 1 } END { exit !f }' \
    || echo "roadmap-tracker: warning: cloud-lanes.tsv names $id, which is no roadmap row; left out" >&2
done
rm -f "$out/.tsv" "$out/.plan" "$out/.approved" "$out/.building" "$out/.merges" "$out/.cloud"
printf '%s\n' "$blob" > "$out/roadmap.blob"
echo "roadmap-tracker: $(jq length "$out/roadmap.json") row(s), $(jq length "$out/sessions.json") session(s) in $out (merged since $since; roadmap ${blob%"${blob#???????}"})"
