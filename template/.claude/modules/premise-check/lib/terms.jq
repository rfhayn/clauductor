# terms.jq: the text half of premise-check.sh, ported from Standing Tee's infra/premise-check.mjs
# (extractTerms, asPath, variants). jq's regexes are Oniguruma's: the same syntax as the original's
# JavaScript ones for everything used here, with (?m) for JavaScript's `m` flag.
#
#   jq -r --arg mode terms    -f terms.jq  < issue.json    one record per term: term, path ("" if
#                                                          not a path), identifier (1/0), the term
#                                                          cut to 40 and to 50 characters, US-separated
#   jq -r --arg mode count    -f terms.jq  < issue.json    how many terms there are in all
#   jq -r --arg mode fences   -f terms.jq  < issue.json    fenced blocks skipped, rounded down
#   jq -rn --arg mode variants --arg t TERM -f terms.jq    every spelling to search for, one per line
#   jq -rR --arg mode hit     -f terms.jq  < hits          `path:line:content` -> path US line US
#                                                          content trimmed and cut to 140
#   jq -rR --arg mode hist --argjson filed EPOCH -f terms.jq  < rows   `h TAB ct TAB cI TAB subject`
#                                                          -> the history line the report prints
#
# Issue JSON: {number, title, createdAt, body, comments: [{body}]} (gh issue view --json ...).

def strip: sub("^\\s+"; "") | sub("\\s+$"; "");

# The prose the terms come from: the title, then the body and every comment. A comment is often
# where the premise changed.
def text: [.body // "", ((.comments // [])[] | .body // "")] | join("\n\n");
def fulltext: "\(.title // "")\n\n\(text)";

# Fenced blocks (logs, stack traces) are skipped: long, noisy, and rarely a claim about code.
def nofences: gsub("(?m)^\\s*(```|~~~)[\\s\\S]*?^\\s*\\1\\s*$"; "");

# Every named thing: inline code spans, then quoted strings (straight or curly) found OUTSIDE a span
# that itself holds a quote, so `setStatus(x, "closed")` does not yield a second term `"closed"`.
def terms:
  nofences as $t
  | [$t | match("`([^`\n]+)`"; "g") | .captures[0].string] as $spans
  | ($t | gsub("`(?<inner>[^`\n]+)`"; if (.inner | contains("\"")) then " " else "`" + .inner + "`" end)) as $prose
  | [$prose | match("\"([^\"\n]{6,160})\"|“([^”\n]{6,160})”"; "g") | (.captures[0].string // .captures[1].string)] as $quotes
  | reduce ($spans + $quotes)[] as $x ({seen: {}, out: []};
      ($x | strip | sub("[.,;:!?]+$"; "")) as $u
      | if ($u | length) < 4 then .
        elif ($u | test("^#?[0-9]+$")) then .                 # an issue/PR number or a bare value
        elif ($u | test("^[0-9a-f]{7,40}$")) then .           # a sha: history, not a claim
        elif .seen[$u] then .
        else .seen[$u] = true | .out += [$u] end)
  | .out;

# A term that names a file: `a/b.ts`, `harness.ts:56`. A dotted identifier (`Date.now`,
# `sequence.shuffle.files`) is not one: a path needs a slash or a known source extension.
def code_ext: "\\.(?:[cm]?[jt]sx?|sql|md|sh|json|ya?ml|css|html|mjs|toml|conf|txt|go|py|rb|rs|java|kt|swift|c|h|cc|cpp|hpp|php|lua|ex|exs|tf)$";
def aspath:
  ((capture("^(?<p>[A-Za-z0-9_.@\\-\\[\\]()/]+?)(?::[0-9]+(?:-[0-9]+)?)?$") | .p) // null)
  | if . == null then ""
    elif (test("\\.[A-Za-z]{1,5}$") | not) then ""
    elif (contains("/") or test(code_ext)) then .
    else "" end;

# A real identifier (`handicap_snapshots`, `startTestDb`) is never "too common".
def identifier: test("^[A-Za-z_$][A-Za-z0-9_$]*(?:\\(\\))?$") and test("[_$]|[a-z][A-Z]");

# Source writes apostrophes and quotes typographically or as entities; an issue types them straight.
def variants:
  . as $t
  | [ $t,
      (if contains("`") then gsub("`"; "") else empty end),
      (if contains("'") then ("’", "&rsquo;", "&apos;", "&#39;") as $a | $t | gsub("'"; $a) else empty end),
      (if contains("’") then gsub("’"; "'") else empty end),
      (if contains("\"") then (gsub("\""; "“"), gsub("\""; "”"), gsub("\""; "&quot;")) else empty end) ]
  | reduce .[] as $v ([]; if any(.[]; . == $v) then . else . + [$v] end)
  | .[];

def days($d): if $d == 0 then "same day as filing" elif $d > 0 then "\($d)d AFTER filing" else "\(-$d)d before filing" end;

($ARGS.named.mode) as $mode | if $mode == "terms" then
  fulltext | terms | .[:60][] | [., aspath, (if identifier then "1" else "0" end), .[:40], .[:50]] | join("\u001f")
elif $mode == "count" then
  fulltext | terms | length
elif $mode == "fences" then
  ([text | match("(?m)^\\s*(```|~~~)"; "g")] | length) / 2 | floor
elif $mode == "variants" then
  $ARGS.named.t | variants
elif $mode == "hit" then
  (split(":")) as $f | [$f[0], $f[1], ($f[2:] | join(":") | strip | .[:140])] | join("\u001f")
elif $mode == "hist" then
  split("\t") as $f | $f[0] as $h | $f[1] as $ct | $f[2] as $iso | ($f[3:] | join("\t")) as $s
  | "\($h) \($iso[:10]) (\(days((((($ct | tonumber) - ($ARGS.named.filed | tonumber)) / 86400) + 0.5) | floor))) \($s[:90])"
else
  error("terms.jq: unknown mode \($mode)")
end
