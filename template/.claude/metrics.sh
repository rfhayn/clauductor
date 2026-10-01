#!/bin/sh
# metrics.sh: flow, DORA, cost, quality and outcomes, as the panel's metrics JSON (OPS-9).
#
#   sh .claude/metrics.sh                     the JSON for 7d, 30d and 90d (the panel's metrics.command)
#   sh .claude/metrics.sh --window 30d        one window (repeatable)
#   sh .claude/metrics.sh --line              one health line from the 30d window (health/flow.sh, session-close)
#
# THE CONTRACT is the panel's (docs/panel.md in the clauductor repository, *Metrics*, version 1),
# read strictly and drawn whole or not at all: an unknown key, a negative number, a percent over
# 100 or a count that is not an integer refuses the WHOLE payload. So this script prints only the
# keys the contract names, rounds every figure, and never prints anything else on stdout.
# checks/metrics.sh holds it to the contract with a fake repository, a fake gh and fake transcripts.
#
# EVERYTHING COMES FROM DATA THAT ALREADY EXISTS; nothing new is recorded to feed it:
#   merged PRs (gh)      lead time (first commit to merge), cycle time (PR opened to merge), merge
#                        frequency, change-fail rate (a PR later reverted, or fixed by a `fix/` PR
#                        that names it: its #number, its change id or its title), escaped defects
#   change records       approval wait (the proposal's first commit to its Approved line), review
#                        rounds (tasks.md `## Progress`: "converged in N round(s)"), outcomes (the
#                        proposal's `## How we'll know`, due and checked from its roadmap row)
#   git refs             aging work in progress: open changes, and change/fix/ops branches not merged
#   transcripts          cost by role, model, change and project (.claude/lib/usage.sh, the reading
#                        usage-report.sh and change-cost.sh's list prices share)
#
# IT FAILS SOFT, PER FIGURE. A source it cannot read (no gh, offline, no transcripts) gives its
# figures `"value": null` with a `note` saying why, and every other figure still draws; a figure
# with nothing in the window says so the same way, never a zero. It exits 0 and prints valid JSON
# whatever happens: even without jq, even if its own aggregation fails. Its stderr is discarded
# (the panel's command may fold it into stdout); METRICS_DEBUG=1 keeps it.
#
# Units are the contract's: hours (median), merges per week, percent, US dollars, days.
# Test seams: METRICS_NOW (unix seconds), CLAUDE_PROJECTS_DIR (transcripts), PATH (gh).
ROOT=$(cd "$(dirname "$0")/.." 2>/dev/null && pwd)
[ "${METRICS_DEBUG:-}" = 1 ] || exec 2>/dev/null
# shellcheck disable=SC1091
. "$ROOT/.claude/lib/conf.sh"
# shellcheck disable=SC1091
. "$ROOT/.claude/lib/change.sh"

now=${METRICS_NOW:-$(date +%s)}
windows="" line=""
while [ $# -gt 0 ]; do
  case "$1" in
    --window) case "${2:-}" in 7d|30d|90d) windows="$windows $2" ;; esac; shift 2 ;;
    --line) line=1; shift ;;
    *) shift ;;
  esac
done
[ -n "$line" ] && [ -z "$windows" ] && windows=30d
[ -n "$windows" ] || windows="7d 30d 90d"

# Without jq nothing can be computed or even escaped safely: one figure that says so.
if ! command -v jq >/dev/null 2>&1; then
  if [ -n "$line" ]; then echo "CANNOT CHECK — jq is not installed"; exit 0; fi
  printf '{"version":1,"generated_at":%s,"windows":{"30d":{"flow":{"cycle_time":{"value":null,"note":"metrics.sh needs jq, which is not installed"}}}}}\n' "$now"
  exit 0
fi

tmp=$(mktemp -d "${TMPDIR:-/tmp}/metrics.XXXXXX") || tmp=""
if [ -z "$tmp" ]; then
  printf '{"version":1,"generated_at":%s,"windows":{"30d":{"flow":{"cycle_time":{"value":null,"note":"metrics.sh could not make a temp directory"}}}}}\n' "$now"
  exit 0
fi
trap 'rm -rf "$tmp"' EXIT

maxdays=0
for w in $windows; do d=${w%d}; [ "$d" -gt "$maxdays" ] && maxdays=$d; done
since_epoch=$((now - maxdays * 86400))
since_iso=$(jq -rn --argjson t "$since_epoch" '$t | todate')
git_() { git -C "$ROOT" "$@"; }

# ── Pull requests (gh) ────────────────────────────────────────────────────────────────────
prnote=""
echo '[]' > "$tmp/open.json"
if [ "${CONTEXT_OFFLINE:-}" = 1 ]; then
  prnote="offline (CONTEXT_OFFLINE=1): no gh call, so no pull request was read"
elif ! command -v gh >/dev/null 2>&1; then
  prnote="gh is not installed, so no pull request was read"
# GraphQL search, not `gh pr list --json commits`: the list asks for every commit's authors, and past
# ~50 PRs that query exceeds GitHub's node limit. The first commit is all lead time needs. The search
# API returns at most 1000 results.
elif ! gh api graphql --paginate -F q="repo:{owner}/{repo} is:pr is:merged merged:>=$(printf '%s' "$since_iso" | cut -c1-10)" \
       -f query='query($q: String!, $endCursor: String) { search(query: $q, type: ISSUE, first: 100, after: $endCursor) {
         pageInfo { hasNextPage endCursor }
         nodes { ... on PullRequest { number title body headRefName createdAt mergedAt
                 commits(first: 1) { nodes { commit { authoredDate } } } } } } }' > "$tmp/search.json" \
     || ! jq -s '[ .[].data.search.nodes[] | select(.number != null)
                  | {number, title, body, headRefName, createdAt, mergedAt,
                     commits: [ .commits.nodes[]?.commit | {authoredDate} ]} ]' "$tmp/search.json" > "$tmp/merged.json" \
     || ! jq -e 'type == "array"' "$tmp/merged.json" >/dev/null; then
  prnote="gh could not list the merged pull requests (not signed in, no network, or no GitHub remote)"
else
  [ "$(jq length "$tmp/merged.json")" -ge 1000 ] && prlimit=1
  gh pr list --state open --limit 200 --json number,title,headRefName,createdAt > "$tmp/open.json" \
    && jq -e 'type == "array"' "$tmp/open.json" >/dev/null || echo '[]' > "$tmp/open.json"
fi
[ -n "$prnote" ] && echo 'null' > "$tmp/merged.json"

# ── Change records ────────────────────────────────────────────────────────────────────────
# One JSON object per change, open or archived. Git gives the times: the proposal's first commit
# on any ref, and the first commit carrying its Approved line.
: > "$tmp/changes.jsonl"
for d in "$ROOT/$CHANGES_DIR"/*/ "$ROOT/$CHANGES_DIR"/archive/*/; do
  p="${d}proposal.md"
  [ -f "$p" ] || continue
  name=$(basename "$d")
  case "$d" in
    */archive/*/) archived=$(printf '%s' "$name" | cut -c1-10); id=$(printf '%s' "$name" | cut -c12-) ;;
    *) archived=""; id=$name ;;
  esac
  rel="$CHANGES_DIR/$id/proposal.md"
  created=$(git_ log --all --diff-filter=A --format=%ct -- "$rel" | tail -1)
  apc=$(git_ log --all -G'^\*\*Approved:\*\*' --format=%ct -- "$rel" | tail -1)
  t="${d}tasks.md"
  rounds=$(awk '/^## /{on = ($0 ~ /^## Progress/)} on && match($0, /converged in [0-9]+ round/) {
      n = substr($0, RSTART + 13, RLENGTH - 19); dt = ""
      if (match($0, /[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]/)) dt = substr($0, RSTART, 10)
      print dt "\t" n }' "$t" 2>/dev/null)
  know=$(awk '/^## /{on = ($0 ~ /^## How we.ll know/); next} on' "$p")
  jq -cn --arg id "$id" --arg archived "$archived" \
    --arg approved "$(proposal_field "$p" Approved | cut -c1-10)" \
    --arg created "$created" --arg apc "$apc" \
    --arg budget "$(proposal_field "$p" Budget | tr -d '$ ')" \
    --arg signal "$(printf '%s\n' "$know" | sed -n 's/^[-* ]*\*\*Signal:\*\*[[:space:]]*//p' | head -1)" \
    --arg on "$(printf '%s\n' "$know" | sed -n 's/^.*\*\*Check on:\*\*[[:space:]]*\([0-9-]\{10\}\).*/\1/p' | head -1)" \
    --arg after "$(printf '%s\n' "$know" | sed -n 's/^.*\*\*Check after:\*\*[[:space:]]*\([0-9][0-9]*\).*/\1/p' | head -1)" \
    --arg rounds "$rounds" --arg open "$(open_tasks "$t")" '
    { id: $id, archived: $archived,
      approved: (if $approved | test("^[0-9]{4}-[0-9]{2}-[0-9]{2}$") then $approved else "" end),
      created: ($created | tonumber? // null), approved_commit: ($apc | tonumber? // null),
      budget: ($budget | tonumber? // null), signal: $signal, check_on: $on,
      check_after: ($after | tonumber? // null),
      rounds: [ $rounds | split("\n")[] | select(. != "") | split("\t") | {date: .[0], n: (.[1] | tonumber)} ],
      open_tasks: ($open | tonumber? // 0) }' >> "$tmp/changes.jsonl"
done

# The roadmap's rows (one parser: roadmap-queue.sh), for titles and outcome checks.
sh "$ROOT/.claude/roadmap-queue.sh" --tsv 2>/dev/null \
  | jq -R 'split("\t") | select(length >= 12) | {id: .[3], change: .[4], state: .[6], summary: .[9], due: .[11]}' \
  | jq -s . > "$tmp/roadmap.json" || echo '[]' > "$tmp/roadmap.json"
jq -e 'type == "array"' "$tmp/roadmap.json" >/dev/null || echo '[]' > "$tmp/roadmap.json"

# ── Branches in flight ────────────────────────────────────────────────────────────────────
base="origin/$MAIN_BRANCH"
git_ rev-parse -q --verify "$base" >/dev/null || base=$MAIN_BRANCH
: > "$tmp/branches.jsonl"
if git_ rev-parse -q --verify "$base" >/dev/null; then
  git_ for-each-ref --format='%(refname)' \
      "refs/heads/$BRANCH_CHANGE" "refs/heads/$BRANCH_FIX" "refs/heads/$BRANCH_OPS" \
      "refs/remotes/origin/$BRANCH_CHANGE" "refs/remotes/origin/$BRANCH_FIX" "refs/remotes/origin/$BRANCH_OPS" \
    | while IFS= read -r ref; do
        short=${ref#refs/heads/}; short=${short#refs/remotes/origin/}
        first=$(git_ rev-list --reverse "$base..$ref" | head -1)
        [ -n "$first" ] || continue
        jq -cn --arg name "$short" --arg first "$(git_ log -1 --format=%ct "$first")" --arg tip "$(git_ log -1 --format=%ct "$ref")" \
          '{name: $name, first: ($first | tonumber), tip: ($tip | tonumber)}'
      done > "$tmp/branches.jsonl"
fi

# ── Cost (transcripts) ────────────────────────────────────────────────────────────────────
usagenote=""
if USAGE_NOW=$now sh "$ROOT/.claude/usage-report.sh" --days "$maxdays" --records > "$tmp/usage.jsonl" 2> "$tmp/usage.err"; then :
else
  usagenote=$(sed -n 's/^CANNOT CHECK — //p' "$tmp/usage.jsonl" | head -1)
  [ -n "$usagenote" ] || usagenote="usage-report.sh failed"
  usagenote="no cost: $usagenote"
  : > "$tmp/usage.jsonl"
fi
jq -s . "$tmp/usage.jsonl" > "$tmp/usage.json" 2>/dev/null || { echo '[]' > "$tmp/usage.json"; usagenote="no cost: the usage records could not be read"; }

# ── The payload ───────────────────────────────────────────────────────────────────────────
jq -s . "$tmp/changes.jsonl" > "$tmp/changes.json"
jq -s . "$tmp/branches.jsonl" > "$tmp/branches.json"
jq -n --argjson now "$now" --arg windows "$windows" \
    --arg prnote "$prnote" --arg prlimit "${prlimit:-}" --arg usagenote "$usagenote" \
    --arg change "$BRANCH_CHANGE" --arg fix "$BRANCH_FIX" --arg ops "$BRANCH_OPS" --arg project "$PROJECT_NAME" \
    --slurpfile merged "$tmp/merged.json" --slurpfile open "$tmp/open.json" \
    --slurpfile changes "$tmp/changes.json" --slurpfile roadmap "$tmp/roadmap.json" \
    --slurpfile branches "$tmp/branches.json" --slurpfile usage "$tmp/usage.json" '
  def r1: . * 10 | round / 10;
  def r2: . * 100 | round / 100;
  def median: sort | length as $n | if $n == 0 then null elif $n % 2 == 1 then .[($n - 1) / 2]
              else (.[$n / 2 - 1] + .[$n / 2]) / 2 end;
  def ts: if type == "string" then (sub("\\.[0-9]+"; "") | fromdateiso8601? // null) else null end;
  def day: (. + "T00:00:00Z") | fromdateiso8601;
  # Text the contract accepts: no control or format character (Go unicode.IsControl and Cf), at
  # most 300 characters.
  def clip: tostring | explode
            | map(if . < 32 or (. >= 127 and . < 160) or . == 173 or (. >= 1536 and . <= 1541) or . == 1564
                     or . == 1757 or . == 1807 or . == 2274 or . == 6158 or (. >= 8203 and . <= 8207)
                     or (. >= 8234 and . <= 8238) or (. >= 8288 and . <= 8292) or (. >= 8294 and . <= 8303)
                     or . == 65279 or (. >= 65529 and . <= 65531) or . == 69821 or . == 69837
                     or (. >= 78896 and . <= 78911) or (. >= 113824 and . <= 113827) or (. >= 119155 and . <= 119162)
                     or . == 917505 or (. >= 917536 and . <= 917631) then 32 else . end)
            | implode | if length > 300 then .[:299] + "…" else . end;
  def esc: gsub("[.*+?()\\[\\]{}|^$\\\\]"; "\\\\\\(.)");
  def nb($days): if $days <= 7 then 7 elif $days <= 30 then 10 else 15 end;
  # The same figure over equal buckets of the window, oldest first; f turns a bucket of items into a
  # number or null. The last bucket closes at now.
  def series($items; $start; $days; f): nb($days) as $n | ($days * 86400 / $n) as $w
    | [ range(0; $n) as $i
        | [ $items[] | select(.ts >= $start + $i * $w and (.ts < $start + ($i + 1) * $w or ($i == $n - 1 and .ts <= $now))) ]
        | f ];
  def stat($vals; $note): if ($vals | length) == 0 then {value: null, note: $note}
                          else {value: ($vals | median | r1), n: ($vals | length)} end;

  ($merged[0]) as $raw
  | ($changes[0]) as $chs
  | ($roadmap[0]) as $rows
  | [ ($raw // [])[] | {number, title: (.title // ""), body: (.body // ""), head: (.headRefName // ""),
        created: (.createdAt | ts), merged: (.mergedAt | ts),
        first: ([.commits[]? | (.authoredDate // .committedDate) | ts | select(. != null)] | min)}
      | select(.merged != null) ] as $prs
  # A remediation: a revert, or a fix/ PR. It links to the PRs it names: by #number, by the change id
  # of a change/ PR, by a revert of its title.
  | [ $prs[] | . as $p
      | select(($p.title | startswith("Revert \"")) or ($p.head | startswith($fix)))
      | ($p.title + "\n" + $p.body) as $text
      | { number: $p.number, merged: $p.merged,
          targets: [ $prs[] | select(.number != $p.number and .merged <= $p.merged)
                     | (.number | tostring) as $num
                     | select( ($text | test("#" + $num + "(?![0-9])"))
                            or ((.head | startswith($change))
                                and ((.head | ltrimstr($change)) as $cid | ($cid | length) > 2
                                     and ($text | test("(^|[^A-Za-z0-9_-])" + ($cid | esc) + "($|[^A-Za-z0-9_-])"))))
                            or ($p.title == ("Revert \"" + .title + "\"")) )
                     | .number ] } ] as $rem
  | ([ $rem[] | select(.targets | length > 0) | .number ]) as $remnums
  | ([ $rem[] | .targets[] ] | unique) as $failed
  | ([ $prs[] | select(.head | startswith($change)) | {id: (.head | ltrimstr($change)), first} ]) as $chprs
  | ([ ($open[0] // [])[] | .headRefName ]) as $openheads
  | ($rows | map(select(.change | startswith("ops/check-outcome-")) | {key: (.change | ltrimstr("ops/check-outcome-")), value: .}) | from_entries) as $checks
  | ($rows | map(select(.change != "") | {key: .change, value: .summary}) | from_entries) as $titles
  | (if $prlimit == "" then "" else "; GitHub search returned its limit of 1000, so the oldest part of the window may lack merges" end) as $cap

  # Approval wait: first commit of the proposal (or of a PR on its branch) to the Approved line. Where
  # one commit carries both (a squash), the Approved line is dated to the day.
  | [ $chs[] | select(.approved != "") | . as $c
      | ([ $c.created, ($chprs[] | select(.id == $c.id) | .first) ] | map(select(. != null)) | min) as $from
      | select($from != null)
      | (if $c.approved_commit != null and $c.approved_commit > $from then $c.approved_commit
         else ([$from, ($c.approved | day)] | max) end) as $to
      | {ts: $to, hours: (($to - $from) / 3600)} ] as $approvals

  # Aging work in progress: every open change, then every unmerged change/fix/ops branch not already
  # listed and not merged by a PR since its last commit.
  | [ $chs[] | select(.archived == "") | . as $c
      | ([ $c.created, ($branches[0][] | select(.name == $change + $c.id) | .first) ] | map(select(. != null)) | min) as $from
      | { id: ($c.id | clip), title: (($titles[$c.id] // "") | clip),
          age_days: (if $from == null then 0 else ((($now - $from) / 86400) | if . < 0 then 0 else . end | r1) end),
          stage: (if $c.approved == "" then "approval"
                  elif ($openheads | index($change + $c.id)) != null then "review"
                  elif $c.open_tasks > 0 then "build" else "review" end) } ] as $wipc
  | ([ $wipc[] | $change + .id ]) as $listed
  | ($wipc + [ $branches[0] | unique_by(.name)[] | . as $b
      | select(($listed | index($b.name)) == null)
      | select([ $prs[] | select(.head == $b.name and .merged >= $b.tip) ] | length == 0)
      | { id: ($b.name | clip), title: (($titles[$b.name] // "") | clip),
          age_days: ((($now - $b.first) / 86400) | if . < 0 then 0 else . end | r1),
          stage: (if ($openheads | index($b.name)) != null then "review" else "build" end) } ]
    | map(if .title == "" then del(.title) else . end) | sort_by(-.age_days) | .[:500]) as $wip

  # Outcomes: each change with a "How we will know" signal. Due from its outcome-check roadmap row,
  # else Check on, else the archive date plus Check after.
  | [ $chs[] | select(.signal != "") | . as $c | $checks[$c.id] as $row
      | { change: ($c.id | clip), hypothesis: ($c.signal | clip),
          due: (if ($row.due // "") != "" then $row.due elif $c.check_on != "" then $c.check_on
                elif $c.archived != "" and $c.check_after != null then (($c.archived | day) + $c.check_after * 86400 | todate | .[:10])
                else null end),
          checked: (($row.state // "") == "merged") }
      | if .due == null then del(.due) else . end ] as $hyps

  | ($usage[0] // []) as $recs
  | ([ $recs[] | select(.usd == null) | .model ] | unique) as $unpriced
  | ($chs | map(select(.budget != null) | {key: .id, value: .budget}) | from_entries) as $budgets

  | { version: 1, generated_at: $now,
      windows: ( [ $windows | splits(" +") | select(. != "") ] | map(. as $w | ($w | rtrimstr("d") | tonumber) as $days
        | ($now - $days * 86400) as $start
        | [ $prs[] | select(.merged >= $start and .merged <= $now) | . + {ts: .merged} ] as $in
        | ($in | map(select((.number as $n | $remnums | index($n)) == null))) as $deploys
        | ($deploys | map(select(.created != null) | . + {v: ((.merged - .created) / 3600)})) as $cyc
        | ($deploys | map(select(.first != null) | . + {v: ((.merged - .first) / 3600 | if . < 0 then 0 else . end)})) as $lead
        | ("No pull request was merged in the last " + $w) as $none
        | [ $approvals[] | select(.ts >= $start and .ts <= $now) ] as $appr
        | [ $chs[] | . as $c | [ $c.rounds[] | select(.date != "" and (.date | day) >= $start - 86400 and (.date | day) <= $now) ]
            | select(length > 0) | (map(.n) | add / length) ] as $rounds
        | [ $recs[] | select(.ts >= $start and .ts <= $now) ] as $cost
        | ($cost | map(.usd // 0) | add // 0) as $total
        | { key: $w, value: ({
            flow: (( if $prnote != "" then
                      { lead_time: {value: null, note: $prnote}, cycle_time: {value: null, note: $prnote},
                        merge_frequency: {value: null, note: $prnote}, change_fail_rate: {value: null, note: $prnote} }
                    else
                      { lead_time: (stat([$lead[].v]; $none) + (if ($lead | length) > 0 then
                          {series: series($lead; $start; $days; if length == 0 then null else (map(.v) | median | r1) end)} else {} end)),
                        cycle_time: (stat([$cyc[].v]; $none) + (if ($cyc | length) > 0 then
                          {series: series($cyc; $start; $days; if length == 0 then null else (map(.v) | median | r1) end)} else {} end)),
                        merge_frequency: ({ value: (($in | length) * 7 / $days | r1),
                          series: series($in; $start; $days; (length * 7 / ($days / nb($days))) | r1), n: ($in | length) }
                          + (if $cap != "" then {note: ("merges per week" + $cap)} else {} end)),
                        change_fail_rate: (if ($deploys | length) == 0 then {value: null, note: $none}
                          else { value: ([ $deploys[] | select((.number as $n | $failed | index($n)) != null) ] | length
                                         | . * 100 / ($deploys | length) | r1),
                                 n: ($deploys | length),
                                 note: "PRs later reverted, or fixed by a fix/ PR that names them" } end) } end )
              + { approval_wait: (if ($appr | length) == 0
                    then {value: null, note: ("No change was approved in the last " + $w + " (proposal to Approved line)")}
                    else {value: ([$appr[].hours] | median | r1), n: ($appr | length),
                          note: "proposal written to Approved line; day precision where one commit carries both"} end) }
              + (if ($wip | length) > 0 then {aging_wip: $wip} else {} end)),
            cost: ( if $usagenote != "" then {per_week: {value: null, note: $usagenote}}
                    elif ($cost | length) == 0 then {per_week: {value: null, note: ("No Claude Code message in this repository in the last " + $w + " on this machine")}}
                    else
                      { total_usd: ($total | r2),
                        per_week: ({ value: ($total * 7 / $days | r2),
                                     series: series($cost; $start; $days; (map(.usd // 0) | add // 0) * 7 / ($days / nb($days)) | r2) }
                                   + (if ($unpriced | length) > 0 then {note: ("excludes unpriced models: " + ($unpriced | join(", ")) | clip)} else {} end)),
                        by_role: ($cost | group_by(.role) | map({name: (.[0].role | clip), usd: (map(.usd // 0) | add | r2)}) | sort_by(-.usd) | .[:500]),
                        by_model: ($cost | group_by(.model) | map({name: (.[0].model | ltrimstr("claude-") | sub("-[0-9]{8}$"; "") | clip), usd: (map(.usd // 0) | add | r2)}) | sort_by(-.usd) | .[:500]),
                        by_change: ($cost | map(select(.branch | startswith($change))) | group_by(.branch)
                                    | map((.[0].branch | ltrimstr($change)) as $id
                                          | {name: ($id | clip), usd: (map(.usd // 0) | add | r2)}
                                          + (if $budgets[$id] != null then {budget_usd: $budgets[$id]} else {} end))
                                    | map(select(.name != "")) | sort_by(-.usd) | .[:500]),
                        by_project: [ {name: ($project | clip), usd: ($total | r2)} ] }
                      | if (.by_change | length) == 0 then del(.by_change) else . end end ),
            quality: ( { review_rounds: (if ($rounds | length) == 0
                           then {value: null, note: ("No change recorded a review round (tasks.md Progress) in the last " + $w)}
                           else {value: ($rounds | median | r1), n: ($rounds | length)} end),
                         escaped_defects: (if $prnote != "" then {value: null, note: $prnote}
                           else ([ $rem[] | select(.merged >= $start and .merged <= $now and (.targets | length) > 0) ] | length) as $e
                             | {value: $e, n: ($deploys | length), note: "reverts and fix/ PRs that name a merged PR"} end) } ),
            outcomes: (if ($hyps | length) == 0 then null
                       else {hypotheses: ([ $hyps[] | select((.checked | not) or ((.due // "") != "" and (.due | day) >= $start)) ] | .[:500])} end)
          } | if .outcomes == null or (.outcomes.hypotheses | length) == 0 then del(.outcomes) else . end) } )
        | from_entries ) }' > "$tmp/out.json" 2> "$tmp/jq.err" || : > "$tmp/out.json"

if [ ! -s "$tmp/out.json" ]; then
  why=$(head -1 "$tmp/jq.err" | cut -c1-250)
  jq -cn --argjson now "$now" --arg why "metrics.sh failed: ${why:-unknown error}" \
    '{version: 1, generated_at: $now, windows: {"30d": {flow: {cycle_time: {value: null, note: $why}}}}}'
  [ -n "$line" ] && echo "CANNOT CHECK — ${why:-metrics.sh failed}"
  exit 0
fi

if [ -n "$line" ]; then
  w=$(printf '%s' "$windows" | awk '{print $1}')
  jq -r --arg w "$w" --arg day "$(jq -rn --argjson t "$now" '$t | todate | .[:10]')" '
    .windows[$w] as $x
    | def v($f; $unit): if $f == null or $f.value == null then "—" else "\($f.value)\($unit)" end;
    ($x.flow // {}) as $f | ($x.cost // {}) as $c
    | [ $f.cycle_time.note, $c.per_week.note ] | map(select(. != null)) as $notes
    | (if ($f.cycle_time.value == null and $f.merge_frequency.value == null and $c.per_week.value == null)
       then "CANNOT CHECK — " + ($notes | first // "no figure")
       else "OK flow \($w) to \($day): cycle \(v($f.cycle_time; "h")) median, lead \(v($f.lead_time; "h")), \(v($f.merge_frequency; "")) merges/wk, change-fail \(v($f.change_fail_rate; "%")), approval wait \(v($f.approval_wait; "h")), review rounds \(v($x.quality.review_rounds; "")), spend $\(v($c.per_week; ""))/wk, \(($f.aging_wip // []) | length) in flight"
            + (if ($notes | length) > 0 then " (" + ($notes | join("; ")) + ")" else "" end) end)' "$tmp/out.json"
  exit 0
fi
cat "$tmp/out.json"
