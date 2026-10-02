#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/../../.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# living.sh: the living pages, held true. A LIVING PAGE is an artifact in the registry
# (ARTIFACT_REGISTRY, the artifacts module) whose entry names how it is refreshed:
#
#   "docs/roadmap.html": { "url": …, "authorities": […], "reviewedAt": …, "reviewNote": …,
#     "refresh": "update-roadmap",                         a skill in .claude/skills, or "edit"
#     "generated": { "queue": "sh scripts/queue-html.sh" },   block NAME = this command's stdout
#     "claims": { "migration-range": "sh scripts/claims/migration-range.sh",
#                 "owner-gate-*": "sh scripts/claims/gate-owner.sh" } }   value = command's stdout
#
# WHY. The artifacts module says WHEN a page's sources moved (currency.sh, BEHIND). It cannot say
# whether what the page STATES is still true, and the refresh skills fire on a structural event (a
# migration, a route), never on a stated fact going false because the thing it counts grew somewhere
# else. Three mechanisms make a page's facts checkable, each a port of Standing Tee's
# living-visuals.test.ts, and each refused rather than skipped when it cannot be read:
#   - GENERATED BLOCKS: `<!-- generated:NAME:begin … -->` … `<!-- generated:NAME:end -->` holds
#     exactly what the declared command prints. A derived copy cannot drift, only go stale, and this
#     turns stale into red. An optional `"outside"` command lists strings (open change ids, the
#     block's own markup) that must not appear in the page OUTSIDE the block: the hand-mirror leak.
#   - CLAIMS: `data-claim="NAME">VALUE<` marks a number or name as CURRENT STATE (not history), and
#     VALUE must equal what the claim's declared command prints. A claim with no command, and a
#     declared claim the page no longer carries, both fail: a check over nothing is decoration.
#   - DURATIONS: `data-days-since="YYYY-MM-DD"` is counted by the page's own script on load; a typed
#     elapsed count ("65 days so far") is wrong the next morning, so LIVING_TYPED_DURATION matching
#     the page's text (scripts, styles and comments aside) fails.
# And the page must be whole and run: read twice and identical, an HTML page ending `</html>`, and
# every inline script parsing (`node --check`, the one Node use; LIVING_SCRIPT_CHECK="off" says so
# instead).
#
#   living.sh --check                       every living page, every rule: ok/FAIL lines; exit 1 on a FAIL
#   living.sh --list [--ref <rev> | --worktree]
#                                           each living page's currency (currency.sh) and how to refresh it
#   living.sh --regen [<page>...]           rewrite every generated block (of the named pages) in place
#   ... [--root <dir>]                      the project (default: this checkout)
#
# The registry's commands run with `sh -c` from the project root, each bounded by
# LIVING_COMMAND_SECONDS: they are the project's own, as GATE_RUN is.
# Exit: 0 fine; 1 --check found a FAIL (or --list a page not OK); 2 an error, its reason on stderr.
ROOT=$(case $CLAUDUCTOR_FW in (*/.claude) dirname "$CLAUDUCTOR_FW" ;; (*) [ -n "${ROOT:-}" ] && echo "$ROOT" || git rev-parse --show-toplevel 2>/dev/null || pwd ;; esac)
# shellcheck disable=SC1091
. "$CLAUDUCTOR_FW/lib/conf.sh"
die() { printf 'living-visuals: %s\n' "$*" >&2; exit 2; }
command -v jq >/dev/null 2>&1 || die "jq is not installed"
TAB=$(printf '\t')
# The artifacts module's currency tool, beside this module wherever it is installed.
CUR="$CLAUDUCTOR_FW/modules/artifacts/bin/currency.sh"

MODE="" REF="" WT="" PAGES=""
while [ $# -gt 0 ]; do
  case $1 in
    --check | --list | --regen) [ -z "$MODE" ] || die "one of --check, --list, --regen"; MODE=${1#--}; shift ;;
    --ref) [ $# -ge 2 ] || die "--ref needs a revision"; REF=$2; shift 2 ;;
    --worktree) WT=1; shift ;;
    --root) [ $# -ge 2 ] || die "--root needs a directory"; ROOT=$(cd "$2" 2>/dev/null && pwd) || die "--root $2: no such directory"; shift 2 ;;
    --*) die "unknown option $1" ;;
    *) PAGES="$PAGES$1$TAB"; shift ;;
  esac
done
[ -n "$MODE" ] || die "usage: living.sh --check | --list [--ref <rev> | --worktree] | --regen [<page>...] [--root <dir>]"
[ "$MODE" = list ] || [ -z "$REF$WT" ] || die "--ref and --worktree go with --list"
[ "$MODE" = regen ] || [ -z "$PAGES" ] || die "pages are named only with --regen"
REG=${ARTIFACT_REGISTRY:-docs/artifacts.json}
SECS=${LIVING_COMMAND_SECONDS:-60}
[ -f "$ROOT/$REG" ] || die "$REG does not exist: the living pages are its entries (the artifacts module's README)"
W=$(mktemp -d "${TMPDIR:-/tmp}/living.XXXXXX") || die "mktemp failed"
trap 'rm -rf "$W"' EXIT

# JavaScript's whitespace, spelt out (Oniguruma's \s differs at U+0085 and U+FEFF): the reference
# trimmed with String.prototype.trim.
JQ_DEFS='
  def ws: "[\t\n\u000b\f\r    -     　﻿]";
  def jstrim: sub("\\A" + ws + "+"; "") | sub(ws + "+\\z"; "");
'
# ── the living pages: every registry entry (any section; $-keys are comments) with a `refresh` ──
jq -c '
  def obj: type == "object";
  [to_entries[] | select(.key | startswith("$") | not) | .key as $s
   | (.value | if obj then to_entries[] else empty end) | select(.key | startswith("$") | not)
   | select((.value | obj) and (.value | has("refresh")))
   | .key as $k | .value as $e
   | {key: $k, refresh: $e.refresh,
      generated: [($e.generated // {}) | if obj then to_entries[] else {key: "(not an object)", value: null} end
                  | {name: .key, run: (if (.value | type) == "string" then .value elif (.value | obj) then .value.run else null end),
                     outside: (if (.value | obj) then .value.outside else null end)}],
      claims: [($e.claims // {}) | if obj then to_entries[] else {key: "(not an object)", value: null} end
               | {key, run: (if (.value | obj) then .value.run else .value end), each: (if (.value | obj) then .value.each else null end)}],
      bad: [(if ($e.generated // {}) | obj | not then "`generated` is not an object" else empty end),
            (if ($e.claims // {}) | obj | not then "`claims` is not an object" else empty end)]}][]' \
  "$ROOT/$REG" > "$W/pages" 2> "$W/err" || die "$REG cannot be read: $(head -n 1 "$W/err")"

# run_cmd CMD OUT [ARG]: CMD's stdout into OUT, from the project root, bounded, ARG as its $1. The
# status is also left in RC, for cmd_fail (an `if !` resets $? before the message reads it).
run_cmd() {
  _rc_cmd=$1 _rc_out=$2; shift 2
  (cd "$ROOT" && bounded "$SECS" sh -c "$_rc_cmd" living "$@" > "$_rc_out" 2> "$_rc_out.err" </dev/null)
  RC=$?; return "$RC"
}
# bounded SECONDS COMMAND [ARGS]: with_timeout's contract (TERM after SECONDS, KILL 2 s later;
# 143/137 when stopped), with a watcher that leaves the moment the command does. with_timeout's
# watcher polls in 1 s sleeps and is waited for, so each call costs a second; a page with a few
# dozen claims made the check take a minute. Here the watcher sleeps once and its sleep is killed.
bounded() {
  _bs=$1; shift
  "$@" &
  _bp=$!
  (
    _bz=""
    trap '[ -z "$_bz" ] || kill "$_bz" 2>/dev/null; exit 0' TERM
    sleep "$_bs" & _bz=$!
    wait "$_bz" 2>/dev/null || exit 0
    kill -TERM "$_bp" 2>/dev/null; sleep 2; kill -KILL "$_bp" 2>/dev/null
  ) </dev/null >/dev/null 2>&1 &
  _bw=$!
  wait "$_bp"; _brc=$?
  kill "$_bw" 2>/dev/null; wait "$_bw" 2>/dev/null
  return "$_brc"
}
cmd_fail() {  # cmd_fail OUT: why the last run_cmd failed, in one line
  case $RC in 143 | 137) echo "did not finish within ${SECS}s (LIVING_COMMAND_SECONDS)" ;;
    *) echo "exited $RC: $(tail -n 3 "$1.err" | tr '\n' ' ' | cut -c1-300)" ;; esac
}

# ── --list ─────────────────────────────────────────────────────────────────────────────────────
if [ "$MODE" = list ]; then
  [ -s "$W/pages" ] || { echo "CANNOT CHECK — no entry in $REG declares \`refresh\`, so there is no living page"; exit 1; }
  if [ -n "$WT" ]; then set -- --worktree; elif [ -n "$REF" ]; then set -- --ref "$REF"; else set --; fi
  [ -f "$CUR" ] || die "cannot find $CUR (the living-visuals module requires the artifacts module)"
  sh "$CUR" --root "$ROOT" "$@" > "$W/cur" 2> "$W/cur.err" || die "currency.sh could not run: $(tail -n 1 "$W/cur.err")"
  head -n 1 "$W/cur" | sed 's/^Core artifacts/Living pages/'
  bad=0
  while IFS= read -r p; do
    k=$(printf '%s' "$p" | jq -r .key); r=$(printf '%s' "$p" | jq -r '.refresh | if type == "string" then . else tojson end')
    how="refresh with /$r"; [ "$r" = edit ] && how="refresh by editing the page"
    l=$(awk -v k="$k" '{ line = $0; sub(/^(OK|BEHIND|CANNOT CHECK) +/, "") } index($0, k " ") == 1 { print line; exit }' "$W/cur")
    case $l in
      OK*) printf '%s\n' "$l" ;;
      '') printf 'CANNOT CHECK %s — currency.sh printed no line for it\n' "$k"; bad=1 ;;
      *) printf '%s\n    → %s, then stamp it (the artifacts module'"'"'s close step)\n' "$l" "$how"; bad=1 ;;
    esac
  done < "$W/pages"
  exit "$bad"
fi

# ── --regen ────────────────────────────────────────────────────────────────────────────────────
if [ "$MODE" = regen ]; then
  [ -s "$W/pages" ] || die "no entry in $REG declares \`refresh\`, so there is no living page"
  if [ -n "$PAGES" ]; then
    printf '%s' "$PAGES" | tr '\t' '\n' | while IFS= read -r want; do
      jq -e --arg k "$want" 'select(.key == $k)' "$W/pages" >/dev/null || { echo "$want" >> "$W/unknown"; }
    done
    [ ! -s "$W/unknown" ] || die "not a living page in $REG: $(tr '\n' ' ' < "$W/unknown")"
  fi
  rc=0; blocks=0
  while IFS= read -r p; do
    k=$(printf '%s' "$p" | jq -r .key)
    if [ -n "$PAGES" ]; then case "$TAB$PAGES" in *"$TAB$k$TAB"*) ;; *) continue ;; esac; fi
    printf '%s' "$p" > "$W/p.json"
    ng=$(jq '.generated | length' "$W/p.json")
    [ "$ng" -gt 0 ] || { [ -z "$PAGES" ] || echo "$k: no generated block declared"; continue; }
    [ -f "$ROOT/$k" ] || { echo "$k: does not exist"; rc=1; continue; }
    # The page is rewritten through jq, which reads text: a byte that is not UTF-8 would come back
    # as U+FFFD, outside the block as much as in it. A page jq cannot hand back byte for byte is
    # refused, never re-encoded.
    jq -Rsj . "$ROOT/$k" > "$W/round" 2>/dev/null
    if ! cmp -s "$W/round" "$ROOT/$k"; then echo "$k: is not valid UTF-8, so regenerating its blocks would re-encode the rest of it; left as it was (fix its encoding first)"; rc=1; continue; fi
    gi=0; blocks=$((blocks + ng))
    while [ "$gi" -lt "$ng" ]; do
      gi=$((gi + 1))
      n=$(jq -r --argjson i "$((gi - 1))" '.generated[$i].name' "$W/p.json")
      c=$(jq -r --argjson i "$((gi - 1))" '.generated[$i].run // "" | if type == "string" then . else "" end' "$W/p.json")
      case $n in *[!A-Za-z0-9_-]* | '') echo "$k: '$n' is not a block name ([A-Za-z0-9_-])"; rc=1; continue ;; esac
      [ -n "$c" ] || { echo "$k: block $n declares no command"; rc=1; continue; }
      if ! run_cmd "$c" "$W/out"; then echo "$k: block $n's command $(cmd_fail "$W/out"); left as it was"; rc=1; continue; fi
      if ! grep -q '[^[:space:]]' "$W/out"; then echo "$k: block $n's command printed nothing; left as it was"; rc=1; continue; fi
      jq -Rsj --arg n "$n" --rawfile out "$W/out" '
        ($out | if endswith("\n") then . else . + "\n" end) as $o
        | ([match("<!-- generated:" + $n + ":begin"; "g")] | length) as $b
        | ([match("<!-- generated:" + $n + ":end -->"; "g")] | length) as $e
        | if $b != 1 or $e != 1 then error("has \($b) `generated:\($n):begin` and \($e) `generated:\($n):end` markers, not one of each; left as it was")
          elif test("<!-- generated:" + $n + ":begin[\\s\\S]*?-->\n[\\s\\S]*?<!-- generated:" + $n + ":end -->") | not
          then error("block \($n): its begin marker is not a comment followed by a newline, before its end marker; left as it was")
          else sub("(?<pre><!-- generated:" + $n + ":begin[\\s\\S]*?-->\n)[\\s\\S]*?(?<post><!-- generated:" + $n + ":end -->)"; "\(.pre)\($o)\(.post)") end' \
        "$ROOT/$k" > "$W/new" 2> "$W/err" || { echo "$k: $(sed 's/^jq: error[^:]*: //' "$W/err" | head -n 1)"; rc=1; continue; }
      # Only the block may differ: the bytes before its begin marker and after its end marker must
      # come back exactly as they were.
      cp "$ROOT/$k" "$W/old"
      for f in old new; do
        awk -v m="<!-- generated:$n:begin" '{ i = index($0, m); if (i) { printf "%s", substr($0, 1, i - 1); exit } print }' "$W/$f" > "$W/$f.head"
        awk -v m="<!-- generated:$n:end -->" 'seen { print; next } { i = index($0, m); if (i) { seen = 1; print substr($0, i) } }' "$W/$f" > "$W/$f.tail"
      done
      if ! cmp -s "$W/old.head" "$W/new.head" || ! cmp -s "$W/old.tail" "$W/new.tail"; then
        echo "$k: block $n: regenerating it would change bytes outside the block; left as it was"; rc=1; continue
      fi
      if cmp -s "$W/new" "$ROOT/$k"; then echo "$k: block $n already current"
      else
        # Through a temporary file beside the page, renamed over it: a reader never sees half a page.
        tmp="$ROOT/$k.living-regen.$$"
        if cat "$W/new" > "$tmp" && mv "$tmp" "$ROOT/$k"; then echo "$k: block $n regenerated"
        else rm -f "$tmp"; echo "$k: block $n could not be written; left as it was"; rc=1; fi
      fi
    done
  done < "$W/pages"
  [ "$blocks" -gt 0 ] || [ -n "$PAGES" ] || echo "no living page in $REG declares a generated block: nothing to regenerate"
  exit "$rc"
fi

# ── --check ────────────────────────────────────────────────────────────────────────────────────
fails=0
ok() { echo "ok   $*"; }
fail() { echo "FAIL $*"; fails=$((fails + 1)); }
if [ ! -s "$W/pages" ]; then
  fail "the living-visuals module is on, but no entry in $REG declares \`refresh\`: there is no living page to hold true"
  exit 1
fi
SCRIPT_CHECK=${LIVING_SCRIPT_CHECK:-node}
case $SCRIPT_CHECK in
  node) command -v node >/dev/null 2>&1 || fail "LIVING_SCRIPT_CHECK=node, but node is not installed: no page's script can be shown to parse (install Node, or set LIVING_SCRIPT_CHECK=\"off\" to say so)" ;;
  off) ;;
  *) fail "LIVING_SCRIPT_CHECK=\"$SCRIPT_CHECK\" is not node or off" ;;
esac
TYPED=${LIVING_TYPED_DURATION-'\b[0-9]+ (days?|weeks?|months?|years?) (so far|and counting|elapsed|ago)\b'}
TODAY=$(date +%Y-%m-%d)

while IFS= read -r p; do
  k=$(printf '%s' "$p" | jq -r .key)
  printf '%s' "$p" > "$W/p.json"
  jq -r '.bad[]' "$W/p.json" | while IFS= read -r b; do echo "FAIL $k: $b"; done
  fails=$((fails + $(jq '.bad | length' "$W/p.json")))
  # The refresh is a NAMED THING: a skill that is not there is an instruction nobody can follow.
  r=$(jq -r '.refresh | if type == "string" then . else "" end' "$W/p.json")
  case $r in
    edit) ok "$k: refreshed by editing the page" ;;
    '' | *[!a-z0-9-]*) fail "$k: \`refresh\` is $(jq -c .refresh "$W/p.json"), not a skill name or \"edit\"" ;;
    *) if [ -f "$ROOT/.claude/skills/$r/SKILL.md" ]; then ok "$k: refreshed by /$r"
       else fail "$k: \`refresh\` names /$r, which is not a skill here (.claude/skills/$r/SKILL.md)"; fi ;;
  esac
  [ -f "$ROOT/$k" ] || { fail "$k: is a living page in $REG, but there is no such file"; continue; }

  # READ TWICE: a page caught mid-rewrite is short, and every rule below would report the
  # difference as drift. Not a retry: a page that stays short is still checked, and fails.
  cat "$ROOT/$k" > "$W/a"; cat "$ROOT/$k" > "$W/page"
  cmp -s "$W/a" "$W/page" || { fail "$k CHANGED WHILE THIS CHECK READ IT: not a drift finding; something rewrote it mid-run. Re-run."; continue; }

  jq -Rs "$JQ_DEFS"'
    def blank: gsub("(?i)<!--[\\s\\S]*?-->|<script\\b[\\s\\S]*?</script\\s*>|<style\\b[\\s\\S]*?</style\\s*>"; " ");
    {html: test("(?i)<html\\b"),
     complete: (sub(ws + "+\\z"; "") | endswith("</html>")),
     opens: ([match("(?i)<script\\b(?![^>]*\\bsrc=)"; "g")] | length),
     scripts: [match("(?i)<script\\b(?![^>]*\\bsrc=)[^>]*>([\\s\\S]*?)</script\\s*>"; "g") | .captures[0].string],
     attrs: ([match("data-claim\\s*="; "gi")] | length),
     claims: [match("data-claim=[\"\\x27]([A-Za-z0-9_-]+)[\"\\x27][^>]*>([^<]+)<"; "g") | [.captures[0].string, (.captures[1].string | jstrim)]],
     days: [match("data-days-since=\"([^\"]*)\""; "g") | .captures[0].string],
     begins: [match("<!-- generated:([A-Za-z0-9_.:-]+?):begin"; "g") | .captures[0].string],
     ends: [match("<!-- generated:([A-Za-z0-9_.:-]+?):end -->"; "g") | .captures[0].string],
     text: blank}' "$W/page" > "$W/facts" 2> "$W/err" || { fail "$k: could not be read as text: $(head -n 1 "$W/err")"; continue; }

  # Whole: an HTML page ends with its </html>; a fragment (no <html>) is not held to it.
  if [ "$(jq .html "$W/facts")" = true ]; then
    if [ "$(jq .complete "$W/facts")" = true ]; then ok "$k: read whole (ends </html>)"
    else fail "$k: read INCOMPLETE (no closing </html>): not a drift finding. Re-run; if it persists, the committed file is truncated."; continue; fi
  fi

  # Runs: every inline script parses. The extractor's count is checked against a plain scan for
  # the opening tag, so a block it fails to extract is red, never a silently shorter list.
  o=$(jq .opens "$W/facts"); n=$(jq '.scripts | length' "$W/facts")
  if [ "$o" != "$n" ]; then fail "$k: opens $o inline <script> tags but $n were extracted: an unextracted script is one nothing parses"
  elif [ "$SCRIPT_CHECK" = node ] && command -v node >/dev/null 2>&1; then
    i=0; bad=""
    while [ "$i" -lt "$n" ]; do
      jq -j --argjson i "$i" '.scripts[$i]' "$W/facts" > "$W/s$i.js"
      if ! node --check "$W/s$i.js" > "$W/s.err" 2>&1 </dev/null; then
        bad="$bad; script $((i + 1)): $(grep -m1 -E 'Error' "$W/s.err" | cut -c1-200)"
      fi
      i=$((i + 1))
    done
    if [ -z "$bad" ]; then ok "$k: all $n inline scripts parse"
    else fail "$k: an inline script does not parse, so the page renders blank (a raw quote inside a quoted string is the usual cause)$bad"; fi
  elif [ "$SCRIPT_CHECK" = off ]; then ok "$k: $n inline scripts NOT parsed (LIVING_SCRIPT_CHECK=off)"
  fi

  # Generated blocks. Fields are read one at a time by index, never through a delimited format a
  # command's own tab or backslash could split.
  jq -r '.begins[]' "$W/facts" | LC_ALL=C sort -u > "$W/onpage"
  jq -r '.generated[].name' "$W/p.json" | LC_ALL=C sort -u > "$W/declared"
  for u in $(comm -23 "$W/onpage" "$W/declared"); do
    fail "$k: carries a generated:$u block that its registry entry does not declare: nothing regenerates it, so it is hand-kept text that claims to be generated"
  done
  ng=$(jq '.generated | length' "$W/p.json"); gi=0
  while [ "$gi" -lt "$ng" ]; do
    gi=$((gi + 1))
    field() { jq -r --argjson i "$((gi - 1))" ".generated[\$i].$1 // \"\" | if type == \"string\" then . else \"\" end" "$W/p.json"; }
    gn=$(field name); gc=$(field run); go=$(field outside)
    case $gn in *[!A-Za-z0-9_-]* | '') fail "$k: '$gn' is not a generated block name ([A-Za-z0-9_-])"; continue ;; esac
    [ -n "$gc" ] || { fail "$k: generated block $gn declares no command (a string, or {\"run\": …, \"outside\": …})"; continue; }
    b=$(jq --arg n "$gn" '[.begins[] | select(. == $n)] | length' "$W/facts"); e=$(jq --arg n "$gn" '[.ends[] | select(. == $n)] | length' "$W/facts")
    if [ "$b" != 1 ] || [ "$e" != 1 ]; then
      fail "$k: has $b generated:$gn:begin and $e generated:$gn:end markers, not one of each. Without them nothing checks the block, and the page is free to drift."
      continue
    fi
    if ! run_cmd "$gc" "$W/gen"; then fail "$k: block $gn's command ($gc) $(cmd_fail "$W/gen")"; continue; fi
    # An empty block that equals an empty output passes forever: a generator says what it found,
    # even when it found nothing ("None.").
    if ! grep -q '[^[:space:]]' "$W/gen"; then fail "$k: block $gn's command ($gc) printed nothing: an empty block would match it forever. Make it print what it found, \"None.\" included"; continue; fi
    v=$(jq -Rsr --arg n "$gn" --rawfile out "$W/gen" "$JQ_DEFS"'
      (capture("<!-- generated:" + $n + ":begin[\\s\\S]*?-->\n(?<r>[\\s\\S]*?)<!-- generated:" + $n + ":end -->") | .r) as $r
      | if $r == null then "NOREGION" elif ($r | jstrim) == ($out | jstrim) then "SAME" else "DIFF" end' "$W/page" 2>/dev/null)
    case $v in
      SAME) ok "$k: generated block $gn matches its command" ;;
      DIFF) fail "$k: generated block $gn no longer matches \`$gc\`. Its source is the authority; do NOT hand-edit the page: sh "$CLAUDUCTOR_FW"/modules/living-visuals/bin/living.sh --regen $k" ;;
      *) fail "$k: generated:$gn's begin marker is not a comment followed by a newline, so the block cannot be read" ;;
    esac
    [ -n "$go" ] || continue
    if ! run_cmd "$go" "$W/toks"; then fail "$k: block $gn's outside command ($go) $(cmd_fail "$W/toks")"; continue; fi
    lk=$(jq -Rsr --arg n "$gn" --rawfile toks "$W/toks" "$JQ_DEFS"'
      sub("<!-- generated:" + $n + ":begin[\\s\\S]*?<!-- generated:" + $n + ":end -->"; "") as $rest
      | [$toks | split("\n")[] | jstrim | select(. != "")] as $t
      | if ($t | length) == 0 then "EMPTY" else [$t[] | select(. as $x | $rest | contains($x))] | unique | join(", ") end' "$W/page")
    case $lk in
      EMPTY) fail "$k: block $gn's outside command printed nothing, so the leak check searches the page for nothing and cannot fail" ;;
      '') ok "$k: nothing block $gn carries is hand-mirrored outside it ($(grep -c . "$W/toks") strings)" ;;
      *) fail "$k: names outside its generated:$gn block what only the block may carry: $lk. A second hand-kept copy is the half that drifts." ;;
    esac
  done

  # Claims: every annotated value equals its declared command's output. A key is a claim name, or
  # a family with one `*` (`owner-gate-*`), whose command gets the part the `*` matched as $1.
  jq -r '.claims[].key' "$W/p.json" > "$W/auth"
  while IFS= read -r a; do
    printf '%s\n' "$a" | grep -Eqx '[A-Za-z0-9_-]+|[A-Za-z0-9_-]*\*[A-Za-z0-9_-]*' \
      || fail "$k: claim key '$a' is not a claim name ([A-Za-z0-9_-]) or a family with one *"
  done < "$W/auth"
  : > "$W/used"; : > "$W/members"
  nc=$(jq '.claims | length' "$W/facts"); ci=0
  # Every annotation is read, or the check says so: the reader takes data-claim="name">value< only,
  # so one in any other shape (markup in its value, a name with a dot, no quotes, spaces round
  # the =) would be skipped silently. Counted against a plain scan, as the scripts are.
  na=$(jq .attrs "$W/facts")
  [ "$na" = "$nc" ] || fail "$k: carries $na data-claim attributes, but $nc were read. The reader takes data-claim=\"<name>\">value< (a name of letters, digits, _ and -, quoted; the value plain text, no markup inside): put each annotation in that shape, or nothing checks it"
  while [ "$ci" -lt "$nc" ]; do
    ci=$((ci + 1))
    cn=$(jq -r --argjson i "$((ci - 1))" '.claims[$i][0]' "$W/facts"); cv=$(jq -r --argjson i "$((ci - 1))" '.claims[$i][1]' "$W/facts")
    hit=$(jq -r --arg c "$cn" 'first(.claims[].key | select(. == $c)) // ""' "$W/p.json")
    arg=""
    if [ -z "$hit" ]; then
      hit=$(jq -r --arg c "$cn" 'first(.claims[].key | select(test("\\A[A-Za-z0-9_-]*\\*[A-Za-z0-9_-]*\\z"))
        | select(split("*") as [$p, $s] | ($c | startswith($p) and endswith($s) and (length > ($p + $s | length))))) // ""' "$W/p.json")
      [ -n "$hit" ] && arg=$(jq -rn --arg h "$hit" --arg c "$cn" '($h | split("*")) as [$p, $s] | $c | .[($p | length):(length - ($s | length))]')
    fi
    if [ -z "$hit" ]; then fail "$k: data-claim=\"$cn\" (\"$cv\") has no command in its registry entry's \`claims\`: an annotation nothing checks is decoration"; continue; fi
    echo "$hit" >> "$W/used"
    [ "$hit" = "$cn" ] || printf '%s\t%s\n' "$hit" "$arg" >> "$W/members"
    cc=$(jq -r --arg h "$hit" 'first(.claims[] | select(.key == $h) | .run) // "" | if type == "string" then . else "" end' "$W/p.json")
    [ -n "$cc" ] || { fail "$k: claim $hit declares no command"; continue; }
    CLAIM=$cn; export CLAIM
    if ! run_cmd "$cc" "$W/cv" "$arg"; then
      fail "$k: claim $cn's command ($cc) $(cmd_fail "$W/cv")"; continue
    fi
    want=$(jq -Rsr "$JQ_DEFS"' jstrim' "$W/cv")
    # Empty on either side is not agreement: a blank annotation states nothing, and a command that
    # prints nothing has said nothing.
    if [ -z "$cv" ]; then fail "$k: claim $cn claims nothing (its value is empty or only whitespace)"; continue; fi
    if [ -z "$want" ]; then fail "$k: claim $cn's command ($cc) printed nothing, so it cannot vouch for \"$cv\""; continue; fi
    # Equal as text, or as numbers: a count reads "six" in prose and 6 from a command, either case.
    same=$(jq -rn --arg a "$cv" --arg b "$want" '
      def num: ascii_downcase as $l
        | {"zero":0,"one":1,"two":2,"three":3,"four":4,"five":5,"six":6,"seven":7,"eight":8,"nine":9,"ten":10,"eleven":11,"twelve":12}[$l]
          // (if test("\\A[0-9]+\\z") then tonumber else null end);
      $a == $b or (($a | num) != null and ($a | num) == ($b | num))')
    if [ "$same" = true ]; then ok "$k: claim $cn = \"$cv\", as its command says"
    else fail "$k: claim $cn says \"$cv\", but its command ($cc) says \"$want\". The page states something its authority does not."; fi
  done
  cut -f1 "$W/auth" | while IFS= read -r a; do
    [ -n "$a" ] || continue
    grep -qxF "$a" "$W/used" || echo "FAIL $k: declares claim $a, but the page carries no data-claim it matches: did the annotation get renamed or dropped?"
  done > "$W/unused"
  [ ! -s "$W/unused" ] || { cat "$W/unused"; fails=$((fails + $(grep -c . "$W/unused"))); }
  # A family with an `each` command is held in BOTH directions: the page carries a claim for every
  # member the authority defines (a new phase with no owner shown fails), and none it does not.
  nf=$(jq '[.claims[] | select(.each != null)] | length' "$W/p.json"); fi_=0
  while [ "$fi_" -lt "$nf" ]; do
    fi_=$((fi_ + 1))
    fk=$(jq -r --argjson i "$((fi_ - 1))" '[.claims[] | select(.each != null)][$i].key' "$W/p.json")
    fe=$(jq -r --argjson i "$((fi_ - 1))" '[.claims[] | select(.each != null)][$i].each | if type == "string" then . else "" end' "$W/p.json")
    case $fk in *'*'*) ;; *) fail "$k: claim $fk declares \`each\`, which only a family (a key with *) can have"; continue ;; esac
    [ -n "$fe" ] || { fail "$k: claim family $fk's \`each\` is not a command"; continue; }
    if ! run_cmd "$fe" "$W/each"; then fail "$k: claim family $fk's each command ($fe) $(cmd_fail "$W/each")"; continue; fi
    sed 's/^[[:space:]]*//; s/[[:space:]]*$//' "$W/each" | grep -v '^$' | LC_ALL=C sort -u > "$W/want"
    awk -F"$TAB" -v f="$fk" '$1 == f { print $2 }' "$W/members" | LC_ALL=C sort -u > "$W/have"
    if [ ! -s "$W/want" ]; then fail "$k: claim family $fk's each command printed no member, so the family is checked against nothing"; continue; fi
    miss=$(comm -23 "$W/want" "$W/have" | tr '\n' ' '); extra=$(comm -13 "$W/want" "$W/have" | tr '\n' ' ')
    if [ -z "$miss$extra" ]; then ok "$k: claim family $fk covers exactly the $(grep -c . "$W/want") members its authority defines"
    else fail "$k: claim family $fk disagrees with its authority's members (the part * matches):${miss:+ no claim on the page for: $miss.}${extra:+ claimed, but not a member the authority defines: $extra.}"; fi
  done

  # Durations: counted by the page, never typed.
  nd=$(jq '.days | length' "$W/facts")
  if [ "$nd" -gt 0 ]; then
    # A real day survives the round trip (2026-02-30 comes back 2026-03-02); none after today.
    bad=$(jq -r --arg today "$TODAY" '[.days[] | select(
        (test("\\A[0-9]{4}-[0-9]{2}-[0-9]{2}\\z") | not)
        or (((try (strptime("%Y-%m-%d") | mktime | strftime("%Y-%m-%d")) catch "") as $r | $r) != .)
        or (. > $today))] | join(", ")' "$W/facts")
    [ -z "$bad" ] && ok "$k: $nd data-days-since dates are real days, none in the future" || fail "$k: data-days-since values that are not a past YYYY-MM-DD: $bad"
    if jq -e '[.scripts[] | select(contains("data-days-since"))] | length > 0' "$W/facts" >/dev/null; then ok "$k: a script of its own fills the day counters"
    else fail "$k: carries data-days-since counters, but none of its scripts mentions data-days-since: nothing fills them on load"; fi
  fi
  if [ -n "$TYPED" ]; then
    typed=$(jq -r --arg re "$TYPED" '[.text | match($re; "gi") | .string] | unique | join("; ")' "$W/facts" 2> "$W/err") \
      || { fail "LIVING_TYPED_DURATION is not a regular expression jq reads: $(head -n 1 "$W/err")"; typed=""; }
    [ -z "$typed" ] && ok "$k: no typed elapsed duration" || fail "$k: a typed elapsed duration is wrong the next morning: \"$typed\". Count it on load: <span data-days-since=\"YYYY-MM-DD\" data-suffix=\" days so far\">"
  fi
done < "$W/pages"
[ "$fails" -eq 0 ]
