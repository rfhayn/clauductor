#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/../../.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# currency.sh: is each core artifact CURRENT with the sources it describes? One line per artifact in
# the registry (ARTIFACT_REGISTRY, default docs/artifacts.json): OK, or BEHIND naming the authority
# that changed and the commit that changed it. Plain sh, git and jq; no Node.
#
# WHY. Shared pages (a roadmap page, an ERD, a playbook, a walkthrough) go stale a different way
# each, and every refresh trigger written into a skill is a judgment call that fires on something
# being BUILT, never on a source moving under a page. This asks the question mechanically, per
# artifact, against the files it describes; the artifacts module's guard rule refuses a
# session-close PR while any artifact is not OK, so the answer is acted on once per session.
#
# HOW. Each registry entry declares `authorities`: path globs (`*` within a directory, `**` across),
# `row:<id>` or `gate:<id>` for rows of the roadmap (ROADMAP; `gate:<id>` is every row under a
# `## Gate <id>` heading, two to four #), and `registry:entries` for the registry's own list of
# artifacts. `reviewedAt` holds a hash of those authorities' CONTENT at the last review, and
# `reviewNote` says what the review was ("YYYY-MM-DD: <one line>"). BEHIND means the content now
# hashes differently. An artifact clears it by being refreshed and stamped, or by being stamped
# "reviewed, no change: <why>":
#
#   sh .claude/modules/artifacts/bin/currency.sh --stamp docs/erd.html --note "refreshed for 0056"
#
# WHY A CONTENT HASH AND NOT A COMMIT. A stamp is written on a branch, and a branch lands as a
# SQUASH: the branch commit is never on main, and a commit cannot contain its own hash. The content
# a review saw survives the squash unchanged, so that is what is recorded. To say WHICH commit
# changed an authority, the history of the authorities' paths is walked back to the commit whose
# content matches the stamp (bounded: 100 commits, and 4 s under --check, 12 s otherwise).
#
#   currency.sh [--ref <rev> | --worktree] [--check] [--root <dir>]
#       Status lines. Default subject: origin/MAIN_BRANCH, what the shared copies should describe.
#       --worktree reads the files on disk (the close, before its PR): TRACKED paths (git ls-files),
#       their content from disk; an untracked match is named, never hashed, because a commit would
#       not carry it. --check exits 1 when any artifact is not OK (the guard rule runs it).
#   currency.sh --stamp <key>... --note "<one line>" [--at <rev>]
#       Record the authorities' current content (or at <rev>) as reviewed, in the working tree's
#       registry. <key> is an entry's key in any section (a page path, a walkthrough name).
#   currency.sh --lint
#       Declarations only, on disk: authorities declared and matching, stamps well formed.
#
# THE SAME HASH AS Standing Tee's infra/artifact-currency.mjs: sha1 over the sorted "key value"
# lines (file:<path> <git blob>, row:<id> <sha1 of the row's cells as JSON>, gate:<id> <ids>,
# registry:entries <sha1 of the sorted "section/key url" lines>), so a registry stamped by either
# reads the same here. The module README lists where the two deliberately differ.
#
# NEVER DEGRADES TO SILENCE. An authority matching no file, a row not in the roadmap, an unreadable
# registry or a missing stamp is a CANNOT CHECK line, never OK: a typo'd glob would otherwise match
# nothing, hash the same forever, and report the page current for good.
#
# Exit: 0 read (and, with --check, all OK); 1 --check found one not OK (or --lint a problem); 2 an
# error (the registry or the revision cannot be read, a bad option), its reason on stderr.
ROOT=$(case $CLAUDUCTOR_FW in (*/.claude) dirname "$CLAUDUCTOR_FW" ;; (*) [ -n "${ROOT:-}" ] && echo "$ROOT" || git rev-parse --show-toplevel 2>/dev/null || pwd ;; esac)
# shellcheck disable=SC1091
. "$CLAUDUCTOR_FW/lib/conf.sh"

TAB=$(printf '\t')
NL='
'
die() { printf 'artifacts: %s\n' "$*" >&2; exit 2; }
command -v jq >/dev/null 2>&1 || die "jq is not installed"

# ── arguments: each flag once, values never starting with -- (as the reference refuses) ─────────
root_arg=""; ref_arg=""; at_arg=""; note_arg=""; note_set=""; at_set=""; ref_set=""
worktree=""; check=""; stamp=""; lint=""; keys=""; nkeys=0
while [ $# -gt 0 ]; do
  case $1 in
    --root | --ref | --at | --note)
      [ $# -ge 2 ] || die "$1 needs a value"
      case $2 in --*) die "$1 needs a value" ;; esac
      case $1 in
        --root) root_arg=$2 ;;
        --ref) ref_arg=$2; ref_set=1 ;;
        --at) at_arg=$2; at_set=1 ;;
        --note) note_arg=$2; note_set=1 ;;
      esac
      shift 2; continue ;;
    --worktree) worktree=1 ;;
    --check) check=1 ;;
    --stamp) stamp=1 ;;
    --lint) lint=1 ;;
    --*) die "unknown option $1" ;;
    *) keys="$keys$1$NL"; nkeys=$((nkeys + 1)) ;;
  esac
  shift
done
[ -n "$worktree" ] && [ -n "$ref_set" ] && die "--worktree and --ref are exclusive"
[ -n "$stamp" ] && [ -n "$lint" ] && die "--stamp and --lint are exclusive"
if { [ -n "$stamp" ] || [ -n "$lint" ]; } && { [ -n "$ref_set" ] || [ -n "$worktree" ] || [ -n "$check" ]; }; then
  die "--stamp and --lint read the working tree; they take no --ref, --worktree or --check"
fi
[ -z "$stamp" ] && { [ -n "$note_set" ] || [ -n "$at_set" ]; } && die "--note and --at go with --stamp"
[ -z "$stamp" ] && [ "$nkeys" -gt 0 ] && die "unexpected argument $(printf '%s' "$keys" | tr '\n' ' ')"

TOP=${root_arg:-$ROOT}
[ -d "$TOP" ] || die "no directory $TOP"
TOP=$(cd "$TOP" && pwd)
# Paths are repository-relative everywhere (ls-tree --full-tree and ls-files must agree), so the
# root must be the repository's top, not a directory inside it.
if _pfx=$(git -C "$TOP" rev-parse --show-prefix 2>/dev/null) && [ -n "$_pfx" ]; then
  die "$TOP is inside a repository ($_pfx), not its top: run from the repository root"
fi
REG=${ARTIFACT_REGISTRY:-docs/artifacts.json}
RM=${ROADMAP:-docs/roadmap.md}
self=".claude/modules/artifacts/bin/currency.sh"
case ${CLAUDUCTOR_FW:-} in '' | */.claude) STAMP_CMD="sh $self" ;; *) STAMP_CMD="clauductor-model modules/artifacts/bin/currency.sh" ;; esac
WALK_LIMIT=100
if [ -n "$check" ]; then WALK_BUDGET=4; else WALK_BUDGET=12; fi
start_s=$(date +%s)

W=$(mktemp -d "${TMPDIR:-/tmp}/artifacts.XXXXXX") || die "mktemp failed"
trap 'rm -rf "$W"' EXIT
# A watchdog's TERM keeps its meaning (143: stopped, not "could not run").
trap 'rm -rf "$W"; exit 130' INT
trap 'rm -rf "$W"; exit 143' TERM

g() { git -C "$TOP" "$@"; }
has_git() { g rev-parse --git-dir >/dev/null 2>&1; }

# ── hashing ──────────────────────────────────────────────────────────────────────────────────
if command -v sha1sum >/dev/null 2>&1; then SHA1="sha1sum"
elif command -v shasum >/dev/null 2>&1; then SHA1="shasum -a 1"
elif command -v openssl >/dev/null 2>&1; then SHA1="openssl dgst -sha1 -r"
else die "no sha1 tool (sha1sum, shasum or openssl)"; fi
# sha1_in: the sha1 of stdin's bytes.
sha1_in() { $SHA1 | awk '{ print $1; exit }'; }
# sha1_files FILE...: one hash per line, in argument order.
sha1_files() { [ $# -gt 0 ] || return 0; $SHA1 "$@" | awk '{ print $1 }'; }
# blob_of FILE: git's blob id for FILE's bytes (no filters), with or without git.
blob_of() {
  if command -v git >/dev/null 2>&1; then git hash-object --no-filters -- "$1"
  else { printf 'blob %s\000' "$(wc -c < "$1" | tr -d ' ')"; cat "$1"; } | sha1_in; fi
}

# ── authorities ──────────────────────────────────────────────────────────────────────────────
# prep: stdin is an entry's JSON; stdout one line per authority, decoded once and reused for every
# state the walk computes: "<kind>\t<authority>\t<ERE>\t<base>". Kinds: G a path glob (its ERE,
# anchored, over repo-relative paths, and the directory a listing starts from: the segments before
# the first wildcard), E registry:entries, Q row:/gate:, U an unknown kind, B not a non-empty
# string (the authority is then its JSON).
prep() {
  jq -r '.authorities[] | if type == "string" and (sub("^\\s+"; "") | sub("\\s+$"; "")) != "" then
      (if . == "registry:entries" then "E\t" + . elif test("\\A(row|gate):") then "Q\t" + .
       elif test("\\A[a-z]+:") then "U\t" + . else "G\t" + . end)
    else "B\t" + tojson end' | awk -F"$TAB" -v T="$TAB" '
    $1 != "G" { print $0 T T; next }
    {
      g = $2; n = split(g, parts, "/"); fw = 0
      for (i = 1; i <= n; i++) if (parts[i] ~ /[*?[]/) { fw = i; break }
      if (fw == 0) base = g; else { base = ""; for (i = 1; i < fw; i++) base = base (i > 1 ? "/" : "") parts[i] }
      re = ""; L = length(g)
      for (i = 1; i <= L; i++) {
        c = substr(g, i, 1)
        if (c == "*" && substr(g, i + 1, 1) == "*") {
          if (substr(g, i + 2, 1) == "/") { re = re "(.*/)?"; i += 2 } else { re = re ".*"; i += 1 }
        } else if (c == "*") re = re "[^/]*"
        else if (c == "?") re = re "[^/]"
        else if (index(".+^${}()|[]\\", c) > 0) re = re "\\" c
        else re = re c
      }
      print "G" T g T "^" re "$" T base
    }'
}
# joinl SEP: stdin's lines joined by SEP.
joinl() { awk -v s="$1" 'NR > 1 { printf "%s", s } { printf "%s", $0 } END { if (NR) printf "\n" }'; }
# pfx TEXT: stdin's lines, each after TEXT (taken literally).
pfx() { PFX=$1 awk '{ print ENVIRON["PFX"] $0 }'; }

# ── sources: a commit (its sha) or DISK ──────────────────────────────────────────────────────
# src_resolve REV: the commit's sha, or exit 1.
src_resolve() { g rev-parse --verify --quiet "$1^{commit}" 2>/dev/null; }

# tree_of SHA: "<path>\t<blob>" for every blob of the commit, cached.
tree_of() {
  if [ ! -f "$W/tree.$1" ]; then
    g ls-tree -r --full-tree -z "$1" 2>/dev/null | tr '\0' '\n' \
      | awk -v T="$TAB" '{ sp = index($0, T); split(substr($0, 1, sp - 1), m, " "); if (m[2] == "blob") print substr($0, sp + 1) T m[3] }' > "$W/tree.$1"
  fi
  printf '%s' "$W/tree.$1"
}

# disk_listed BASE: tracked paths under BASE (index, intent-to-add included), cached; with no git,
# a raw walk that skips dependencies, build output, git data and other agents' worktrees.
disk_listed() {
  _k=$(printf '%s' "$1" | sha1_in)
  if [ ! -f "$W/ls.$_k" ]; then
    if has_git; then
      if [ -z "$1" ]; then g ls-files -c -z; else g ls-files -c -z -- "$1"; fi 2>/dev/null | tr '\0' '\n' > "$W/ls.$_k"
    else
      ( cd "$TOP" && find "$( [ -n "$1" ] && printf './%s' "$1" || printf . )" \( -name node_modules -o -name .git -o -name .DS_Store -o -name .next -o -name dist \
          -o -name coverage -o -name .turbo -o -path ./.claude/worktrees -o -path ./.artifact-publish \) -prune \
          -o -type f -print 2>/dev/null ) | sed 's|^\./||; s|^/||' > "$W/ls.$_k"
    fi
  fi
  printf '%s' "$W/ls.$_k"
}
disk_untracked() {
  _k=$(printf 'u%s' "$1" | sha1_in)
  if [ ! -f "$W/ls.$_k" ]; then
    if has_git; then
      if [ -z "$1" ]; then g ls-files -o --exclude-standard -z; else g ls-files -o --exclude-standard -z -- "$1"; fi 2>/dev/null | tr '\0' '\n' > "$W/ls.$_k"
    else : > "$W/ls.$_k"; fi
  fi
  printf '%s' "$W/ls.$_k"
}

# disk_files RE BASE: "<path>\t<blob>" for each tracked regular file on disk the glob matches (a
# path deleted from the tree but still in the index is absent, as it would be after a commit).
disk_files() {
  grep -E -- "$1" "$(disk_listed "$2")" 2>/dev/null | while IFS= read -r p; do
    [ -f "$TOP/$p" ] && [ ! -L "$TOP/$p" ] && printf '%s\n' "$p"
  done > "$W/df.paths"
  [ -s "$W/df.paths" ] || return 0
  # One hash-object for every path (a process per file is what makes a wide glob slow).
  if command -v git >/dev/null 2>&1; then
    (cd "$TOP" && git hash-object --no-filters --stdin-paths < "$W/df.paths") > "$W/df.blobs" || return 1
  else
    while IFS= read -r p; do blob_of "$TOP/$p"; done < "$W/df.paths" > "$W/df.blobs"
  fi
  paste "$W/df.paths" "$W/df.blobs"
}

# read_src SRC PATH: the file's content, or exit 1.
read_src() {
  if [ "$1" = DISK ]; then cat "$TOP/$2" 2>/dev/null; else g show "$1:$2" 2>/dev/null; fi
}
# blob_at SRC PATH: the file's blob id (a cache key), or exit 1.
blob_at() {
  if [ "$1" = DISK ]; then [ -f "$TOP/$2" ] && blob_of "$TOP/$2"
  else g rev-parse --verify --quiet "$1:$2" 2>/dev/null; fi
}
src_subject() {
  if [ "$1" = DISK ]; then
    _h=$(g rev-parse --short=9 HEAD 2>/dev/null) || _h=""
    if [ -n "$_h" ]; then echo "the working tree (HEAD $_h, uncommitted edits included)"; else echo "the working tree"; fi
  else echo "$2 @ $(printf '%s' "$1" | cut -c1-9)"; fi
}

# ── the registry ─────────────────────────────────────────────────────────────────────────────
# The artifacts: every object entry with a truthy url, in every section; keys starting with $ are
# comments. One JSON object per line, in registry order, carrying the facts each mode needs.
JQ_ARTS='
  def entries_of: if type == "object" then to_entries
    elif type == "array" then [range(length) as $i | {key: ($i | tostring), value: .[$i]}]
    else [] end;
  def truthy: . != null and . != false and . != 0 and . != "";
  def u16len: [explode[] | if . > 65535 then 1, 1 else 1 end] | length;
  to_entries[] | select((.key | startswith("$")) | not) | .key as $s
  | (.value | if . == null then [] else entries_of end)[]
  | select((.key | startswith("$")) | not)
  | select((.value | type) == "object" and ((.value.url // null) | truthy))
  | .key as $k | .value as $e
  | {section: $s, key: $k, url: ($e.url | tostring), authorities: $e.authorities,
     decl: ([
       (if ($e.authorities | type) != "array" or ($e.authorities | length) == 0 then "declares no `authorities`" else empty end),
       (if ($e.reviewedAt | type) != "string" or ($e.reviewedAt | test("^[0-9a-f]{40}$") | not) then "has no 40-hex `reviewedAt`" else empty end),
       (if ($e.reviewNote | type) != "string" or ($e.reviewNote | test("^[0-9]{4}-[0-9]{2}-[0-9]{2}: \\S") | not)
          then "has no `reviewNote` of the form `YYYY-MM-DD: <what the review was>`"
        elif ($e.reviewNote | test("\n")) then "has a `reviewNote` longer than one line" else empty end)
     ] | join("; ")),
     reviewedAt: ($e.reviewedAt // ""),
     reviewed: (if ($e.reviewNote | type) == "string" then
        ($e.reviewNote | .[12:]) as $t
        | ($e.reviewNote | .[0:10]) + " (" + (if ($t | u16len) > 48 then ($t | .[0:47]) + "…" else $t end) + ")"
      else "" end)}
  | @json'

# reg_load SRC FILE: the registry's artifacts (JQ_ARTS lines) into FILE, or print why not and exit 1.
reg_load() {
  if ! read_src "$1" "$REG" > "$W/reg.raw"; then echo "$REG not found in $(src_subject "$1" "${3:-}")"; return 1; fi
  if ! _e=$(jq -e 'type == "object" or type == "array"' "$W/reg.raw" 2>&1 >/dev/null); then
    if jq empty "$W/reg.raw" >/dev/null 2>&1; then echo "$REG is not an object"; else echo "$REG is not valid JSON: $(printf '%s' "$_e" | head -n 1)"; fi
    return 1
  fi
  jq -r "$JQ_ARTS" "$W/reg.raw" > "$2" 2>/dev/null || { echo "$REG cannot be read as a registry"; return 1; }
}

# entries_hash SRC: the registry:entries value (sha1 of the sorted "section/key url" lines), or
# UNREADABLE.
entries_hash() {
  _b=$(blob_at "$1" "$REG") || { echo UNREADABLE; return 1; }
  if [ ! -f "$W/ent.$_b" ]; then
    if read_src "$1" "$REG" | jq -e 'type == "object" or type == "array"' >/dev/null 2>&1; then
      read_src "$1" "$REG" | jq -r "$JQ_ARTS" | jq -r '"\(.section)/\(.key) \(.url)"' | LC_ALL=C sort \
        | awk 'NR > 1 { printf "\n" } { printf "%s", $0 }' | sha1_in > "$W/ent.$_b"
    else echo UNREADABLE > "$W/ent.$_b"; fi
  fi
  cat "$W/ent.$_b"
  [ "$(cat "$W/ent.$_b")" != UNREADABLE ]
}

# ── the roadmap's rows ───────────────────────────────────────────────────────────────────────
# One line per queue row: "<id>\t<gate>\t<cells as JSON>", or one "ERR\t<why>" line. The grammar is
# docs/roadmap.md's: `## Phase N` sections, `| id | change | scope | deps | status |` tables in
# them, a `## Gate <id>` heading (2-4 #) naming the gate of the rows under it. A row's value is its
# change id, summary, scope, deps and status, as plain text (emphasis, code and strikes removed).
JQ_ROWS='
  def trim: sub("^\\s+"; "") | sub("\\s+$"; "");
  def plain: gsub("~~(?<x>[^~]*)~~"; .x) | gsub("\\*\\*"; "") | gsub("`"; "") | gsub("\\*"; "") | trim;
  reduce (split("\n")[]) as $l ({fenced: false, inphase: false, gate: null, rows: [], err: null};
    if .err != null then .
    elif ($l | test("^\\s*(```|~~~)")) then .fenced = (.fenced | not)
    elif .fenced then .
    elif ($l | test("^## Phase \\d+\\b")) then .inphase = true | .gate = null
    elif ($l | test("^#{2,4} Gate [0-9A-Za-z]+(?:-[0-9A-Za-z]+)*\\b")) then
      .gate = ($l | capture("^#{2,4} Gate (?<g>[0-9A-Za-z]+(?:-[0-9A-Za-z]+)*)\\b").g)
    elif ($l | test("^## ")) then .inphase = false | .gate = null
    elif (.inphase | not) or ($l | startswith("|") | not) then .
    else
      ([$l | splits("(?<!\\\\)\\|")] | map(trim)) as $cells
      | ($cells | length) as $n
      | [$cells | to_entries[] | select(.value != "" or (.key != 0 and .key != ($n - 1))) | .value] as $cols
      | (($cols[0] // "")) as $c0
      | if $c0 == "#" or ($c0 | test("^:?-+:?$")) then .
        else ($c0 | plain) as $id
        | (if $id == "" then "(blank id)" else $id end) as $nm
        | if ($id | test("^[A-Za-z0-9._-]+$") | not) then
            .err = "queue row \"\($nm)\" has an id that is not [A-Za-z0-9._-]"
          elif ($cols | length) != 5 then
            .err = "queue row \"\($nm)\" has \($cols | length) columns, expected exactly 5"
          elif ($cols[4] | plain | test("^(✅|❌|⬜)") | not) then
            .err = "queue row \"\($nm)\" has a status that does not start with ✅, ❌ or ⬜"
          else
            ($cols[1]) as $c
            | ((($c | capture("^(?:~~)?\\*\\*`(?<x>[^`]+)`\\*\\*") | .x)
                // ($c | capture("\\b(?<x>add-[a-z0-9-]+)") | .x)
                // ($c | plain | .[0:40]))) as $change
            | ($c | sub("^\\*\\*`?[^`*]+`?\\*\\*\\s*"; "") | sub("^\\*\\([^)]*\\)\\*\\s*"; "") | sub("^—\\s*"; "") | plain) as $summary
            | .rows += [{id: $id, gate: .gate,
                v: ({change: $change, summary: $summary, scope: ($cols[2] | plain), deps: ($cols[3] | plain), status: ($cols[4] | plain)}
                    | tojson | gsub("\\\\u007f"; "\u007f"))}]
          end
        end
    end)
  | if .err != null then "ERR\t\(.err)"
    elif .fenced then "ERR\ta fenced code block is never closed"
    else .rows[] | "\(.id)\t\(.gate // "")\t\(.v)" end'

# rows_at SRC: the rows file ("<id>\t<gate>\t<value>"), or exit 1 when the roadmap is missing or
# does not parse.
rows_at() {
  _rb=$(blob_at "$1" "$RM") || return 1
  if [ ! -f "$W/rows.$_rb" ]; then
    read_src "$1" "$RM" | jq -R -s -r "$JQ_ROWS" > "$W/rows.$_rb.tmp" 2>/dev/null || echo "ERR	jq" > "$W/rows.$_rb.tmp"
    if grep -q '^ERR' "$W/rows.$_rb.tmp"; then
      : > "$W/rows.$_rb.bad"; : > "$W/rows.$_rb"
    else
      mkdir -p "$W/rv.$_rb"; _i=0
      # Whole lines, cut by expansion: a TAB is IFS whitespace, so `read a b c` would merge the two
      # TABs around an EMPTY gate (a row under no Gate heading) and lose the value.
      while IFS= read -r _ln; do
        _i=$((_i + 1)); _v=${_ln#*"$TAB"}; _v=${_v#*"$TAB"}
        printf '%s' "$_v" > "$W/rv.$_rb/$(printf '%06d' "$_i")"
      done < "$W/rows.$_rb.tmp"
      # One sha1 call for every row: the values file's line N is row N's value hash.
      if [ "$_i" -gt 0 ]; then sha1_files "$W/rv.$_rb"/* > "$W/rows.$_rb.h"; else : > "$W/rows.$_rb.h"; fi
      cut -f1,2 "$W/rows.$_rb.tmp" | paste - "$W/rows.$_rb.h" > "$W/rows.$_rb"
    fi
  fi
  [ ! -f "$W/rows.$_rb.bad" ] && printf '%s' "$W/rows.$_rb"
}

# ── the state of an artifact's authorities at a source ───────────────────────────────────────
# state SRC PREP OUT: writes OUT.items (sorted "key\tvalue"), OUT.problems (in authority order) and
# OUT.untracked; prints the hash. PREP is the artifact's prep output. Problems only mean something
# for the CURRENT state: a historical state that cannot be read is simply one the walk cannot match.
state() {
  _src=$1; _pf=$2; _o=$3
  : > "$_o.raw"; : > "$_o.problems"; : > "$_o.untracked"; : > "$_o.zero"
  # Every glob against a commit in ONE pass over its tree: the items, and the globs matching nothing.
  if [ "$_src" != DISK ] && grep -q '^G' "$_pf"; then
    awk -F"$TAB" -v T="$TAB" -v P="$_pf" -v Z="$_o.zero" '
      BEGIN { while ((getline l < P) > 0) { split(l, f, "\t"); if (f[1] == "G") { ng++; re[ng] = f[3]; au[ng] = f[2] } } }
      { for (i = 1; i <= ng; i++) if ($1 ~ re[i]) { hit[i]++; print "file:" $1 T $2 } }
      END { for (i = 1; i <= ng; i++) if (!hit[i]) print au[i] > Z }' "$(tree_of "$_src")" >> "$_o.raw"
  fi
  while IFS="$TAB" read -r _k _a _re _base; do
    case $_k in
      B) echo "an authority is not a non-empty string: $_a" >> "$_o.problems" ;;
      E)
        _v=$(entries_hash "$_src") || echo "$_a: $REG unreadable" >> "$_o.problems"
        printf '%s\t%s\n' "$_a" "$_v" >> "$_o.raw" ;;
      Q)
        _kind=${_a%%:*}; _id=${_a#*:}; _id=${_id%%:*}
        if ! _rf=$(rows_at "$_src"); then
          echo "$_a: $RM does not parse" >> "$_o.problems"; printf '%s\tUNPARSEABLE\n' "$_a" >> "$_o.raw"; continue
        fi
        if [ "$_kind" = row ]; then _col=1; else _col=2; fi
        # As strings: awk compares number-shaped fields numerically, and 1.1 == 1.10.
        awk -F"$TAB" -v c="$_col" -v id="$_id" '($c "") == (id "")' "$_rf" > "$_o.hit"
        [ -s "$_o.hit" ] || echo "$_a names no row in $RM" >> "$_o.problems"
        [ "$_kind" = gate ] && printf '%s\t%s\n' "$_a" "$(cut -f1 "$_o.hit" | joinl ,)" >> "$_o.raw"
        awk -F"$TAB" -v T="$TAB" '{ print "row:" $1 T $3 }' "$_o.hit" >> "$_o.raw" ;;
      U) echo "$_a is not an authority kind this script knows (glob, row:, gate:, registry:entries)" >> "$_o.problems" ;;
      G)
        if [ "$_src" = DISK ]; then
          disk_files "$_re" "$_base" > "$_o.files"
          if [ -s "$_o.files" ]; then awk -F"$TAB" -v T="$TAB" '{ print "file:" $1 T $2 }' "$_o.files" >> "$_o.raw"
          else echo "$_a matches no file" >> "$_o.problems"; fi
          grep -E -- "$_re" "$(disk_untracked "$_base")" >> "$_o.untracked" 2>/dev/null
        elif grep -qxF -- "$_a" "$_o.zero"; then
          echo "$_a matches no file" >> "$_o.problems"
        fi ;;
    esac
  done < "$_pf"
  # A key set twice keeps its last value (a Map's set); then sorted by key, bytewise.
  awk -F"$TAB" '{ if (!($1 in v)) o[++n] = $1; v[$1] = $0 } END { for (i = 1; i <= n; i++) print v[o[i]] }' "$_o.raw" \
    | LC_ALL=C sort -t "$TAB" -k1,1 > "$_o.items"
  LC_ALL=C sort -u "$_o.untracked" -o "$_o.untracked"
  awk -F"$TAB" 'NR > 1 { printf "\n" } { printf "%s %s", $1, substr($0, length($1) + 2) }' "$_o.items" | sha1_in
}

# state_cached SHA PREP AKEY: the hash (and $W/st.<sha>.<akey>.items) of a commit's state.
state_cached() {
  _sc="$W/st.$1.$3"
  [ -f "$_sc.hash" ] || state "$1" "$2" "$_sc" > "$_sc.hash"
  cat "$_sc.hash"
}

# pathspecs PREP: the git pathspecs whose history can change the state, one per line.
pathspecs() {
  awk -F"$TAB" -v R="$REG" -v M="$RM" '
    $1 == "E" { s = R } $1 == "Q" { s = M } $1 == "G" { s = ":(glob)" $2 }
    $1 == "E" || $1 == "Q" || $1 == "G" { if (!seen[s]++) print s }' "$1"
}

# describe BEFORE AFTER PREP: what differs between two item files, grouped under the authority each
# item came from, e.g. "src/**/*.md (sub/b.md, c.md (new)); row:2D.1".
describe() {
  cut -f1 "$1" "$2" | LC_ALL=C sort -u > "$W/keys"
  awk -v B="$1" -v A="$2" -v G="$3" -v K="$W/keys" '
    BEGIN {
      while ((getline l < B) > 0) { t = index(l, "\t"); bv[substr(l, 1, t - 1)] = substr(l, t + 1); bh[substr(l, 1, t - 1)] = 1 }
      while ((getline l < A) > 0) { t = index(l, "\t"); av[substr(l, 1, t - 1)] = substr(l, t + 1); ah[substr(l, 1, t - 1)] = 1 }
      while ((getline l < G) > 0) { split(l, f, "\t"); na++; kd[na] = f[1]; au[na] = f[2]; re[na] = f[3]; bs[na] = f[4] }
      while ((getline k < K) > 0) {
        if ((k in bh) == (k in ah) && bv[k] == av[k]) continue
        label = k; what = ""
        if (substr(k, 1, 5) == "file:") {
          p = substr(k, 6); label = p; b = p
          for (i = 1; i <= na; i++) if (kd[i] == "G" && p ~ re[i]) { label = au[i]; b = bs[i]; break }
          what = (label == p) ? "" : substr(p, (b == "" ? 1 : length(b) + 2))
          tag = !(k in ah) ? "deleted" : (!(k in bh) ? "new" : "")
          if (tag != "") what = (what != "") ? what " (" tag ")" : tag
        } else if (substr(k, 1, 4) == "row:") {
          label = ""
          for (i = 1; i <= na; i++) if (kd[i] != "B" && au[i] == k) { label = k; break }
          if (label == "") for (i = 1; i <= na; i++) if (kd[i] != "B" && substr(au[i], 1, 5) == "gate:") { label = au[i]; break }
          if (label == "") label = k
          what = (label == k) ? "" : k
        }
        if (!(label in cnt)) { ng++; ord[ng] = label; cnt[label] = 0 }
        if (what != "") { cnt[label]++; w[label, cnt[label]] = what }
      }
      out = ""
      for (i = 1; i <= ng; i++) {
        lb = ord[i]; s = lb
        if (cnt[lb] > 0) {
          sh = ""; for (j = 1; j <= cnt[lb] && j <= 3; j++) sh = sh (j > 1 ? ", " : "") w[lb, j]
          if (cnt[lb] > 3) sh = sh ", +" (cnt[lb] - 3) " more"
          s = lb " (" sh ")"
        }
        out = out (i > 1 ? "; " : "") s
      }
      print out
    }'
}

# slice70 TEXT: the first 70 characters (not bytes).
slice70() { printf '%s' "$1" | jq -Rrs '.[0:70]'; }

# locate REV PREP AKEY RECORDED: walk the authorities' history back from REV to the commit whose
# state hashes to RECORDED. Sets loc_found (sha or ""), loc_found_items, loc_changes (file of
# "sha\tday\tsubject", newest first), loc_latest (first log line), loc_stopped (commits walked, or
# "" when the walk was not stopped).
locate() {
  loc_found=""; loc_found_items=""; loc_latest=""; loc_stopped=""; loc_changes="$W/changes"; : > "$loc_changes"
  pathspecs "$2" > "$W/specs"
  # shellcheck disable=SC2046
  ( IFS="$NL"; set -f; g log -n"$WALK_LIMIT" --format='%H%x09%cs%x09%s' "$1" -- $(cat "$W/specs") ) > "$W/log" 2>/dev/null || return 0
  loc_latest=$(head -n 1 "$W/log")
  _n=0; : > "$W/hashes"
  while IFS= read -r _line; do
    if [ $(($(date +%s) - start_s)) -gt "$WALK_BUDGET" ]; then loc_stopped=$_n; return 0; fi
    _sha=${_line%%"$TAB"*}
    _h=$(state_cached "$_sha" "$2" "$3")
    printf '%s\t%s\n' "$_h" "$_line" >> "$W/hashes"
    _n=$((_n + 1))
    if [ "$_h" = "$4" ]; then
      loc_found=$_sha; loc_found_items="$W/st.$_sha.$3.items"
      # Every commit after the match whose state differs from the next older one changed it.
      awk -F"$TAB" '{ h[NR] = $1; l[NR] = substr($0, length($1) + 2) } END { for (k = 1; k < NR; k++) if (h[k] != h[k + 1]) print l[k] }' "$W/hashes" > "$loc_changes"
      return 0
    fi
  done < "$W/log"
}

# ── modes ────────────────────────────────────────────────────────────────────────────────────
CLEAR_NOTE='--note "<refreshed: what changed | reviewed, no change: why>"'

if [ -n "$lint" ]; then
  why=$(reg_load DISK "$W/arts") || die "$why"
  : > "$W/problems"; n=0
  [ -s "$W/arts" ] || echo "$REG registers no artifact at all" >> "$W/problems"
  while IFS= read -r art; do
    n=$((n + 1))
    key=$(printf '%s' "$art" | jq -r .key)
    printf '%s' "$art" | jq -r '.decl | select(. != "") | split("; ")[]' | pfx "$key " >> "$W/problems"
    if printf '%s' "$art" | jq -e '.authorities | type == "array"' >/dev/null; then
      printf '%s' "$art" | prep > "$W/prep"
      state DISK "$W/prep" "$W/lint" > /dev/null
      pfx "$key: " < "$W/lint.problems" >> "$W/problems"
    fi
  done < "$W/arts"
  pfx "PROBLEM " < "$W/problems"
  c=$(wc -l < "$W/problems" | tr -d ' ')
  if [ "$c" -gt 0 ]; then echo "$c declaration problem(s) in $REG."; exit 1; fi
  echo "All $n core artifacts declare readable authorities and a review stamp."
  exit 0
fi

if [ -n "$stamp" ]; then
  [ "$nkeys" -gt 0 ] || die "--stamp needs at least one artifact key (a page path or a walkthrough name)"
  note_t=$(printf '%s' "$note_arg" | sed 's/^[[:space:]]*//; s/[[:space:]]*$//')
  case $note_arg in *"$NL"*) note_t="" ;; esac
  [ -n "$note_set" ] && [ -n "$note_t" ] || die '--stamp needs --note "<one line>": what was refreshed, or why nothing needed to change'
  why=$(reg_load DISK "$W/arts") || die "$why"
  cp "$W/reg.raw" "$W/reg.disk"
  if [ -n "$at_set" ]; then
    src=$(src_resolve "$at_arg") || die "cannot resolve $at_arg"
    subj=$(src_subject "$src" "$at_arg")
  else src=DISK; subj=$(src_subject DISK); fi
  today=$(date +%Y-%m-%d)
  : > "$W/updates"
  printf '%s' "$keys" | while IFS= read -r key; do
    [ -n "$key" ] || continue
    art=$(jq -c --arg k "$key" 'select(.key == $k)' "$W/arts" | head -n 1)
    [ -n "$art" ] || die "$key is not an artifact in $REG ($(jq -r .key "$W/arts" | joinl ', '))"
    printf '%s' "$art" | jq -e '(.authorities | type) == "array" and (.authorities | length) > 0' >/dev/null \
      || die "$key declares no authorities to stamp"
    printf '%s' "$art" | prep > "$W/prep"
    h=$(state "$src" "$W/prep" "$W/stamp")
    # At a PAST revision an authority may legitimately not exist yet (a file added since): the stamp
    # records that absence. Stamping the working tree refuses instead: there a match-nothing glob
    # is a typo.
    if [ -s "$W/stamp.problems" ] && [ -z "$at_set" ]; then die "$key: $(joinl '; ' < "$W/stamp.problems")"; fi
    [ -z "$at_set" ] || pfx "note: $key at $at_arg: " < "$W/stamp.problems"
    pfx "note: $key: " < "$W/stamp.untracked" | sed 's/$/ is untracked and NOT in this stamp; `git add` it and stamp again/'
    printf '%s' "$art" | jq -c --arg h "$h" --arg n "$today: $note_t" '{section, key, h: $h, n: $n}' >> "$W/updates"
    echo "stamped $key = $(printf '%s' "$h" | cut -c1-9) ($subj)"
  done || exit $?
  # Written once, after every key passed: a refused key leaves the registry untouched.
  jq --indent 2 --slurpfile u "$W/updates" \
    'reduce $u[] as $x (.; .[$x.section][$x.key].reviewedAt = $x.h | .[$x.section][$x.key].reviewNote = $x.n)' \
    "$W/reg.disk" > "$W/reg.new" || die "cannot write $REG"
  cat "$W/reg.new" > "$TOP/$REG"
  exit 0
fi

# status
[ -n "$worktree" ] || has_git || die "no git repository at $TOP; use --worktree or --lint"
rev=""
if [ -n "$worktree" ]; then
  subject=DISK; subj=$(src_subject DISK)
else
  rev=${ref_arg:-origin/${MAIN_BRANCH:-main}}
  subject=$(src_resolve "$rev") || die "cannot resolve $rev in $TOP"
  subj=$(src_subject "$subject" "$rev")
fi
why=$(reg_load "$subject" "$W/arts" "$rev") || die "$why"
echo "Core artifacts vs $subj; authorities and review stamps read from the same place:"
head_rev=""
if [ -n "$worktree" ]; then has_git && head_rev=$(src_resolve HEAD); else head_rev=$subject; fi
total=0; notok=0; ai=0
while IFS= read -r art; do
  total=$((total + 1)); ai=$((ai + 1))
  key=$(printf '%s' "$art" | jq -r .key)
  decl=$(printf '%s' "$art" | jq -r .decl)
  if [ -n "$decl" ]; then echo "CANNOT CHECK $key — $decl"; notok=$((notok + 1)); continue; fi
  pf="$W/prep.$ai"
  printf '%s' "$art" | prep > "$pf"
  rat=$(printf '%s' "$art" | jq -r .reviewedAt)
  reviewed=$(printf '%s' "$art" | jq -r .reviewed)
  nowh=$(state "$subject" "$pf" "$W/now.$ai")
  if [ -s "$W/now.$ai.problems" ]; then
    echo "CANNOT CHECK $key — $(joinl '; ' < "$W/now.$ai.problems")"
    notok=$((notok + 1)); continue
  fi
  untracked_note=""
  if [ -s "$W/now.$ai.untracked" ]; then
    un=$(head -n 5 "$W/now.$ai.untracked" | joinl ', ')
    [ "$(wc -l < "$W/now.$ai.untracked")" -gt 5 ] && un="$un, …"
    untracked_note="note    $key — untracked, so not counted (a commit would not carry it): $un. \`git add\` it, then stamp."
  fi
  if [ "$nowh" = "$rat" ]; then
    echo "OK      $key — reviewed $reviewed"
    [ -z "$untracked_note" ] || echo "$untracked_note"
    continue
  fi
  [ -z "$untracked_note" ] || echo "$untracked_note"
  notok=$((notok + 1))
  # Uncommitted edits first (the close's own working tree), then what history says.
  parts=""; base_items="$W/now.$ai.items"
  if [ -n "$worktree" ] && [ -n "$head_rev" ]; then
    hh=$(state_cached "$head_rev" "$pf" "a$ai")
    if [ "$hh" != "$nowh" ]; then
      parts="uncommitted: $(describe "$W/st.$head_rev.a$ai.items" "$W/now.$ai.items" "$pf")"
      base_items="$W/st.$head_rev.a$ai.items"
    fi
    if [ "$hh" = "$rat" ]; then
      echo "BEHIND  $key — reviewed $reviewed; $parts"
      echo "        clear it: refresh the page, or $STAMP_CMD --stamp $key $CLEAR_NOTE"
      continue
    fi
  fi
  if [ -z "$head_rev" ]; then
    echo "BEHIND  $key — reviewed $reviewed; no git history here to say which authority or commit"
    continue
  fi
  locate "$head_rev" "$pf" "a$ai" "$rat"
  latest_txt=""
  if [ -n "$loc_latest" ]; then
    latest_txt="; latest authority change $(printf '%s' "$loc_latest" | cut -f1 | cut -c1-9) $(printf '%s' "$loc_latest" | cut -f2) \"$(slice70 "$(printf '%s' "$loc_latest" | cut -f3-)")\""
  fi
  if [ -n "$loc_found" ]; then
    parts="${parts:+$parts; }changed: $(describe "$loc_found_items" "$base_items" "$pf")"
    nch=$(wc -l < "$loc_changes" | tr -d ' ')
    lt=$(head -n 1 "$loc_changes"); [ -n "$lt" ] || lt=$loc_latest
    if [ -n "$lt" ]; then
      parts="$parts; $nch commit(s) since the review, latest $(printf '%s' "$lt" | cut -f1 | cut -c1-9) $(printf '%s' "$lt" | cut -f2) \"$(slice70 "$(printf '%s' "$lt" | cut -f3-)")\""
    fi
  elif [ -n "$loc_stopped" ]; then
    parts="${parts:+$parts; }the history walk stopped at its $WALK_BUDGET s budget after $loc_stopped commit(s), so no commit is named$latest_txt"
  else
    parts="${parts:+$parts; }the reviewed state is not among the last $WALK_LIMIT commits that touched its authorities$latest_txt"
  fi
  echo "BEHIND  $key — reviewed $reviewed; $parts"
  echo "        clear it: refresh the page, or $STAMP_CMD --stamp $key $CLEAR_NOTE"
done < "$W/arts"
# A registry that holds no artifact learned nothing: never "all 0 are current".
if [ "$total" -eq 0 ]; then
  echo "CANNOT CHECK — $REG registers no artifact (no entry with a url), so nothing is held current."
  notok=1
elif [ "$notok" -eq 0 ]; then
  echo "All $total core artifacts are current with their authorities."
else
  echo "$notok of $total core artifacts need a review; a session-close PR cannot merge until none does (the artifacts module's guard rule)."
fi
[ -n "$check" ] && [ "$notok" -gt 0 ] && exit 1
exit 0
