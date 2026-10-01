#!/bin/sh
# open.sh: open every shared claude.ai artifact the registry (ARTIFACT_REGISTRY) holds, in the
# browser. Run by session-start (ARTIFACT_PUBLISH=claude.ai), after it has republished any STALE
# page copy, so what opens is the current copy.
#
# ITERATES THE AUTHORITY. The list is every entry of every section (pages, walkthroughs, and any
# section added later), read at run time, so an artifact registered later opens with no edit here
# or in a skill. Keys starting with $ are comments. An entry with no well-formed url is an ERROR,
# never a skip: a skipped entry would shrink the list to a smaller plausible one.
#
#   open.sh               open each url with `open` (macOS) or `xdg-open`
#   open.sh --print       print the urls only, open nothing
#   --opener <cmd>        open with <cmd> instead (the checks pass a recorder)
#   --root <dir>          read <dir>/<registry>
#
# One line per artifact, `<section>/<key> → <url>`, so the session can repeat the links. Exit 0 when
# every url opened; 1 when an opener call failed (the others are still tried); 2 when the registry
# cannot be read or an entry is malformed. Standing Tee's infra/open-artifacts.mjs, in sh and jq.
ROOT=$(cd "$(dirname "$0")/../../../.." 2>/dev/null && pwd)
# shellcheck disable=SC1091
. "$ROOT/.claude/lib/conf.sh"
REG=${ARTIFACT_REGISTRY:-docs/artifacts.json}
die() { echo "open-artifacts: $*" >&2; exit 2; }
top=$ROOT; opener=""; print=""
while [ $# -gt 0 ]; do
  case $1 in
    --root | --opener)
      [ $# -ge 2 ] || die "$1 needs a value"
      case $2 in --*) die "$1 needs a value" ;; esac
      if [ "$1" = --root ]; then top=$2; else opener=$2; fi
      shift 2; continue ;;
    --print) print=1 ;;
    *) die "unknown argument: $1" ;;
  esac
  shift
done
if [ -z "$opener" ]; then
  if [ "$(uname -s)" = Darwin ]; then opener=open; else opener=xdg-open; fi
fi
command -v jq >/dev/null 2>&1 || die "jq is not installed"
[ -f "$top/$REG" ] || die "cannot read $REG: no such file"
jq -e 'type == "object"' "$top/$REG" >/dev/null 2>&1 || die "$REG is not an object (or not JSON)"

# The one url form: two spellings of one artifact would walk past a duplicate check.
targets=$(jq -r '
  def isobj: type == "object";
  to_entries[] | select(.key | startswith("$") | not) | .key as $s
  | if (.value | isobj | not) then "ERR\tsection \"\($s)\" is not an object of entries"
    else .value | to_entries[] | select(.key | startswith("$") | not)
      | (if (.value | isobj) then .value.url else null end) as $u
      | if ($u | type) == "string" and ($u | test("\\Ahttps://claude\\.ai/artifact/[A-Za-z0-9]{10,40}\\z"))
        then "\($s)/\(.key)\t\($u)"
        else "ERR\t\($s)/\(.key) has no well-formed url: \($u // "undefined")" end
    end' "$top/$REG") || die "cannot read $REG"
bad=$(printf '%s\n' "$targets" | grep '^ERR' | head -n 1)
[ -z "$bad" ] || die "${bad#ERR	}"
[ -n "$targets" ] || die "$REG registers no artifacts"

failed=0; n=0
TAB=$(printf '\t')
while IFS="$TAB" read -r name url; do
  n=$((n + 1))
  if [ -n "$print" ]; then echo "$name → $url"; continue; fi
  if why=$("$opener" "$url" 2>&1 >/dev/null); then
    echo "$name → $url"
  else
    rc=$?; failed=$((failed + 1))
    echo "$name → $url  NOT OPENED ($opener: ${why:-exit $rc})"
  fi
done <<EOF
$targets
EOF
if [ "$failed" -gt 0 ]; then echo "open-artifacts: $failed of $n did not open" >&2; exit 1; fi
exit 0
