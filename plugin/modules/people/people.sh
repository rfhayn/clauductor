#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/../.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# people.sh: the people module's one reader of the people registry (PEOPLE, docs/people.json by
# default), and the who-is-on-what block built from it. Nothing else parses the registry: the
# context sections, the roadmap owner rule and the checks all call this.
#
#   sh people.sh --check   [--people F]     validate the registry; exit 1 naming each defect
#   sh people.sh --names   [--people F]     each person's name, one per line
#   sh people.sh --logins  [--people F]     each person's GitHub login, one per line
#   sh people.sh --lanes   [--people F]     the lane table (Markdown), from the registry's "lanes"
#   sh people.sh --owners  [--people F]     stdin: the roadmap's --tsv rows; exit 1 naming each row
#                                           whose owner is not a person in the registry
#   sh people.sh --activity --state DIR [--people F] [--rows TSV] [--roadmap FILE]
#                                           who is on what: per person Last, Now and Next
#
# EVERY LINE IS DERIVED, NOTHING IS KEPT. The people come from the registry, their work from
# GitHub, and what they own from the roadmap's `**Owner:**` lines through roadmap_queue --tsv (the
# one parser, or the project's ROADMAP_PARSER). A hand-kept "who is doing what" file would be right
# the day it is written and wrong the first day nobody edits it, with nothing to fail.
#
# --activity MAKES NO NETWORK CALL. context.d/session-start/who.sh reads GitHub and writes what it
# read into DIR; a MISSING FILE THERE MEANS THE READ FAILED, and prints CANNOT CHECK, never "none":
#   open-prs.json        gh pr list --state open --json number,title,headRefName,author
#   branches.tsv         branch<TAB>login<TAB>date per remote branch with no open PR; login is
#                        GitHub's account for the last commit, `?` when the lookup failed
#   merged-<login>.json  gh pr list --state merged --author <login> --search "sort:updated-desc"
#                        --json number,title,headRefName,mergedAt, one file per person
# --rows takes the roadmap's rows as a --tsv file (12 contract columns; the optional 14th, when a
# project's parser emits one, is the row's raw status, and a status reading "⬜ deferred" is never
# anyone's Next: a deferred row waits on an event, not a person. Column 13, `started`, is not
# read here).
#
# THE REGISTRY FAILS LOUDLY rather than reading as a shorter list: a person dropped here would
# vanish from who-is-on-what and every row they own would refuse as an unknown owner, so the loud
# failure is the one that points at the cause. Needs jq (as every check does).
ROOT=$(case $CLAUDUCTOR_FW in (*/.claude) dirname "$CLAUDUCTOR_FW" ;; (*) [ -n "${ROOT:-}" ] && echo "$ROOT" || git rev-parse --show-toplevel 2>/dev/null || pwd ;; esac)
# shellcheck disable=SC1091
. "$CLAUDUCTOR_FW/lib/conf.sh"
PEOPLE=${PEOPLE:-docs/people.json}

mode=""; people=""; state=""; rows=""; roadmap=""
while [ $# -gt 0 ]; do
  case $1 in
    --check | --names | --logins | --lanes | --owners | --activity) mode=$1; shift ;;
    --people | --state | --rows | --roadmap)
      # A flag with no value, or a flag where the value belongs, is refused: read as absent it
      # would fall back to the DEFAULT file and silently ignore the one named.
      case ${2:-} in '' | --*) echo "CANNOT CHECK — $1 needs a value after it"; exit 1 ;; esac
      case $1 in --people) people=$2 ;; --state) state=$2 ;; --rows) rows=$2 ;; --roadmap) roadmap=$2 ;; esac
      shift 2 ;;
    *) echo "CANNOT CHECK — people.sh: unknown argument '$1' (see its header)"; exit 1 ;;
  esac
done
[ -n "$mode" ] || { sed -n '2,13p' "$0" | sed 's/^# \{0,1\}//'; exit 2; }
if [ -z "$people" ]; then
  case $PEOPLE in /*) people=$PEOPLE ;; *) people="$ROOT/$PEOPLE" ;; esac
  label=$PEOPLE
else
  label=$people
fi
command -v jq >/dev/null 2>&1 || { echo "CANNOT CHECK — jq is not installed, so $label cannot be read"; exit 1; }

# registry_errors: one line per defect of the registry; empty when it is sound.
registry_errors() {
  [ -f "$people" ] || { echo "$label is missing (the people module reads its people from it: PEOPLE in .claude/project.conf)"; return; }
  jq -e . "$people" >/dev/null 2>&1 || { echo "$label is not valid JSON"; return; }
  jq -r --arg f "$label" '
    def blank: (type != "string") or (gsub("^\\s+|\\s+$"; "") == "");
    if type != "object" then "\($f): expected an object with a \"people\" array"
    elif (.people | type) != "array" or (.people | length) == 0 then "\($f): expected a non-empty \"people\" array"
    else
      (.people[] | . as $p | ("name", "github", "role") | select(($p | type) != "object" or ($p[.] | blank))
        | "\($f): a person has no \"\(.)\" (\($p | tojson))"),
      ([.people[] | objects | .name | strings] | group_by(.) | map(select(length > 1))[] | "\($f): name:\(.[0]) is listed twice"),
      ([.people[] | objects | .github | strings | ascii_downcase] | group_by(.) | map(select(length > 1))[] | "\($f): github:\(.[0]) is listed twice"),
      (if has("lanes") then
         if (.lanes | type) != "array" then "\($f): \"lanes\" must be an array of {\"lane\", \"owns\"}"
         else
           (.lanes[] | . as $l | ("lane", "owns") | select(($l | type) != "object" or ($l[.] | blank))
             | "\($f): a lane has no \"\(.)\" (\($l | tojson))"),
           ([.lanes[] | objects | .lane | strings] | group_by(.) | map(select(length > 1))[] | "\($f): lane \"\(.[0])\" is listed twice")
         end
       else empty end)
    end' "$people"
}

errs=$(registry_errors)
if [ -n "$errs" ]; then
  if [ "$mode" = --check ]; then printf '%s\n' "$errs" | sed 's/^/FAIL /'; else printf '%s\n' "$errs" | sed 's/^/CANNOT CHECK — /'; fi
  exit 1
fi

case $mode in
  --check)
    jq -r --arg f "$label" '"ok   \($f): \(.people | length) person(s), \((.lanes // []) | length) lane(s)"' "$people"
    exit 0 ;;
  --names) jq -r '.people[].name' "$people"; exit 0 ;;
  --logins) jq -r '.people[].github' "$people"; exit 0 ;;
  --lanes)
    jq -r --arg f "$label" '
      if ((.lanes // []) | length) == 0 then "none: \($f) lists no lanes (a \"lanes\" array of {\"lane\", \"owns\"} draws them)"
      else "| lane | owns |", "|---|---|", (.lanes[] | "| **\(.lane | gsub("\\|"; "\\|"))** | \(.owns | gsub("\\|"; "\\|")) |") end' "$people"
    exit 0 ;;
  --owners)
    # The roadmap's owner rule: a row's owner (column 9) must be a person here, or the queue is
    # UNKNOWN. An owner that parsed as nobody would drop that person's rows from Next and print a
    # confident, smaller answer.
    out=$(jq -Rr --slurpfile p "$people" --arg f "$label" '
      ($p[0].people | map(.name)) as $names
      | split("\t") | select(length >= 9 and .[8] != "")
      | select(.[8] as $o | $names | index($o) | not)
      | "row \(.[3]) (line \(.[0])): owner \"\(.[8])\" is not in \($f) (\($names | join(", "))). Add the person there, or correct the name."' 2>&1)
    rc=$?
    [ "$rc" -eq 0 ] || { echo "CANNOT CHECK — the roadmap rows could not be read: $out"; exit 1; }
    [ -z "$out" ] || { printf '%s\n' "$out"; exit 1; }
    exit 0 ;;
esac

# --activity
[ -n "$state" ] || { echo "CANNOT CHECK — usage: --activity --state <dir>"; exit 1; }
[ -d "$state" ] || { echo "CANNOT CHECK — the state directory $state is missing"; exit 1; }
w=$(mktemp -d "${TMPDIR:-/tmp}/people.XXXXXX" 2>/dev/null) || { echo "CANNOT CHECK — mktemp failed"; exit 1; }
trap 'rm -rf "$w"' EXIT
if [ -n "$rows" ]; then
  [ -f "$rows" ] || { echo "CANNOT CHECK — the rows file $rows is missing"; exit 1; }
  rowsf=$rows
else
  rowsf="$w/rows.tsv"
  # shellcheck disable=SC2086
  if ! roadmap_queue --tsv ${roadmap:+"$roadmap"} > "$rowsf" 2>&1; then
    echo "CANNOT CHECK — the roadmap could not be read, so nobody's Next is known:"
    sed 's/^/  /' "$rowsf"
    exit 1
  fi
fi

# Each read that succeeded is a file; a missing one is null below, and prints CANNOT CHECK.
merged="$w/merged.json"
jq -r '.people[].github' "$people" | while IFS= read -r login; do
  f="$state/merged-$login.json"
  if [ -f "$f" ]; then jq -c --arg l "$login" '{($l): .}' "$f" || { echo "BAD $f"; exit 1; }
  else jq -nc --arg l "$login" '{($l): null}'; fi
done > "$merged.lines" || { echo "CANNOT CHECK — a merged-PR file in $state is not valid JSON"; exit 1; }
jq -s 'add // {}' "$merged.lines" > "$merged" || { echo "CANNOT CHECK — the merged-PR files could not be combined"; exit 1; }
hasopen=false; openf=/dev/null
if [ -f "$state/open-prs.json" ]; then
  jq -e 'type == "array"' "$state/open-prs.json" >/dev/null 2>&1 || { echo "CANNOT CHECK — $state/open-prs.json is not a JSON array"; exit 1; }
  hasopen=true; openf="$state/open-prs.json"
fi
hasbr=false; brf=/dev/null
[ -f "$state/branches.tsv" ] && { hasbr=true; brf="$state/branches.tsv"; }

jq -nr --slurpfile pp "$people" --slurpfile mm "$merged" --slurpfile oo "$openf" \
  --rawfile br "$brf" --rawfile rr "$rowsf" --argjson hasopen "$hasopen" --argjson hasbr "$hasbr" \
  --arg f "$label" '
  def clip($n): if length > $n then .[0:$n - 1] + "…" else . end;
  def esc: gsub("(?<c>[.*+?^${}()|\\[\\]\\\\/])"; "\\\(.c)");
  def isbot: (.is_bot // false) or ((.login // "") | test("^app/")) or ((.login // "") | test("\\[bot\\]$"));
  ($pp[0].people) as $people
  | ($mm[0]) as $merged
  | (if $hasopen then $oo[0] else null end) as $open
  | (if $hasbr then ($br | split("\n") | map(select(test("^\\s*$") | not) | split("\t")
        | {name: .[0], login: (.[1] // ""), date: (.[2] // "")})) else null end) as $branches
  | ($rr | split("\n") | map(select(. != "") | split("\t")
      | {line: .[0], phase: .[1], section: .[2], id: .[3], change: .[4], state: .[6], owner: .[8],
         summary: (.[9] // ""), status: (.[13] // "")})) as $rows
  # A row id is matched as a whole token of a branch name or title; an all-digit id ("3") is not,
  # since "3" in a title is far more often a count than a row. The change id still matches it.
  | (reduce ($rows[] | select(.id | test("^[0-9]+$") | not)) as $r ({}; .[$r.id] = $r)) as $ids
  # The roadmap row a branch name or PR title names: its change id first, else its row id.
  | def named($texts):
      ( first($rows[] | select(.change != "") as $r
          | select(any($texts[]; (. // "") | test("(^|[^a-z0-9-])" + ($r.change | esc) + "($|[^a-z0-9-])"))) )
        // first($texts[] | (. // "") | splits("[^A-Za-z0-9_.]+") | select(. != "" and $ids[.] != null) | $ids[.])
        // null );
    def rowlabel: "\(.id) \(.change)";
    def where: if .section != "" then .section else "Phase \(.phase)" end;
    def isopen: .state == "queued" or .state == "inflight";
    # IN FLIGHT FOR ANYONE: every row an open PR or a PR-less branch names, whoever has it (bots
    # and unmapped logins included). Next skips these, so it never offers a row somebody else
    # already has a branch for. GitHub is the authority for "under way", not the status column.
    ([($open // [])[] | named([.headRefName, .title]) | select(. != null) | .id]
     + [($branches // [])[] | named([.name]) | select(. != null) | .id]) as $inflight
    | ($people | map(.github | ascii_downcase)) as $known
    | ( $people[] | . as $p | ($p.github | ascii_downcase) as $login
        | "\($p.name) (\($p.github) · \($p.role))",
          # LAST: their most recently MERGED PR, by mergedAt (gh lists by creation or update).
          ( $merged[$p.github] as $m
            | if $m == null then "  Last: CANNOT CHECK — the merged-PR list could not be read from GitHub"
              elif ($m | length) == 0 then "  Last: nothing merged yet"
              else ($m | map(.mergedAt // "") | max) as $top
                | first($m[] | select((.mergedAt // "") == $top)) as $last
                | named([$last.headRefName, $last.title]) as $r
                | "  Last: #\($last.number), merged \(($last.mergedAt // "")[0:10]) — \(($last.title // "") | clip(90))\(if $r then " → \($r | rowlabel)" else " (names no roadmap row)" end)"
              end ),
          # NOW: their open PRs and their PR-less remote branches, each with the row it names.
          ( ( (if $open == null then ["CANNOT CHECK — the open-PR list could not be read from GitHub"]
               else [$open[] | select(((.author.login // "") | ascii_downcase) == $login)
                     | named([.headRefName, .title]) as $r
                     | "#\(.number) \(.headRefName) — \((.title // "") | clip(70))\(if $r then " → \($r | rowlabel)" else "" end)"] end)
            + (if $branches == null then ["CANNOT CHECK — the remote branches with no PR could not be read"]
               else [$branches[] | select((.login | ascii_downcase) == $login)
                     | named([.name]) as $r
                     | "branch \(.name) (no PR, last commit \(if .date == "" then "unknown" else .date end))\(if $r then " → \($r | rowlabel)" else "" end)"] end) )
            | if length == 0 then ["nothing open"] else . end
            | to_entries[] | "  \(if .key == 0 then "Now: " else "     " end) \(.value)" ),
          # NEXT: the first unfinished row they own, in roadmap order, not under way for anyone and
          # not deferred behind a trigger.
          ( ( first($rows[] | select(isopen and .owner == $p.name and (.id as $i | $inflight | index($i) | not)
                and (.status | test("^⬜\\s*deferred\\b"; "i") | not))) // null ) as $next
            | (if $open == null or $branches == null then " — Now could not be read, so it may be under way" else "" end) as $partial
            | if $next then "  Next: \($next | rowlabel) — \($next.summary | clip(70)) (\($next | where))\($partial)"
              else "  Next: no unfinished roadmap row names them as owner" end )
      ),
      # NOTHING DROPS OUT SILENTLY: work by a login no person maps to is named, not omitted, since
      # an omission would read as "nobody else is working".
      ( [ ( ($open // [])[] | select((.author | isbot | not) and (((.author.login // "") | ascii_downcase) as $l | $known | index($l) | not))
            | "#\(.number) \(.headRefName) by \(if (.author.login // "") == "" then "unknown" else .author.login end)" ),
          ( ($branches // [])[]
            | if .login == "" or .login == "?" then "branch \(.name) (\(if .login == "?" then "account lookup failed" else "no GitHub account" end))"
              elif ((.login | ascii_downcase) as $l | $known | index($l) | not) and (.login | test("\\[bot\\]$") | not) then "branch \(.name) by \(.login)"
              else empty end ) ]
        | if length > 0 then "Not mapped to anyone in \($f):", (.[] | "  \(.)") else empty end )
  '
