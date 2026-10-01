#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/../.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# premise-check.sh: does an issue's write-up still match the code? Run it before fixing the issue.
# A port to sh + jq + git (+ gh to read the issue) of Standing Tee's infra/premise-check.mjs, which
# it replaces line for line: the same terms, the same report, the same receipt.
#
# WHY. An issue's write-up is a claim about the code, made once. Fix units start from write-ups the
# code no longer matches, and the drift is found only when someone happens to read the code first.
# This puts the drift in front of whoever is about to fix the issue, before they start.
#
# WHAT IT DOES, per issue: reads the title, body AND comments (a comment is often where the premise
# changed), pulls out every NAMED thing (inline `code` spans and "quoted strings"), and for each
# reports, against a ref:
#   - where it lives now, with the line it sits on, and whether each file holding it changed since
#     filing (an edit AROUND an unchanged quote is invisible to `git log -S`);
#   - the commits that added or removed it (`git log -S`), each dated RELATIVE TO THE FILING, before
#     as well as after: an issue can be wrong at birth, its quoted sentence changed days earlier;
#   - for a path, EVERY file it can mean (a bare `page.tsx` may be thirty files), whether each
#     changed since filing, and for a path that is gone, the commit that deleted or moved it;
#   - a term in NO commit, ever, is prose, a command or a value, and one in more than COMMON code
#     places is too common to be a specific claim. Both are listed, not dropped.
# It cannot judge a NEGATIVE claim ("the message doesn't mention Reopen"): it prints the line and
# the reader judges. Fenced blocks (logs, traces) are skipped and counted.
#
# The last line is the receipt, `premise-check: #N @ <sha>`, one per issue. The module's guard rule
# (guard.d/premise.sh) requires one in the body of a PREMISE_REQUIRED_ON PR for each issue it fixes.
#
# Usage:
#   sh .claude/modules/premise-check/premise-check.sh <issue>... [--at <ref>]
#   sh .claude/modules/premise-check/premise-check.sh --issue-json <file> [--at <ref>]  (no network)
# <ref> defaults to origin/<MAIN_BRANCH>, fetched first. Exit 0 with a report; 1 on a fault, with
# `premise-check: FAULT — <why>` on stderr and nothing on stdout.
#
# PREMISE_DOC_DIRS (project.conf, default from module.conf): directories whose files are docs, not
# code. Code hits are listed first and code commits searched first; docs only when no code commit
# ever touched the term, because a quoted sentence is usually also quoted in a roadmap or journal.
set -f   # no globbing: pathspecs such as **/*.md go to git as written

MAX_TERMS=60 MAX_HITS=3 MAX_HISTORY=4 MAX_PATHS=6 COMMON=40
HERE=$(cd "$(dirname "$0")" && pwd)
JQ_LIB="$HERE/lib/terms.jq"
US=$(printf '\037')
TAB=$(printf '\t')
W=$(mktemp -d "${TMPDIR:-/tmp}/premise.XXXXXX") || { say "premise-check: FAULT — cannot make a temp directory" >&2; exit 1; }
trap 'rm -rf "$W"' EXIT
trap 'rm -rf "$W"; exit 1' INT TERM

# fault MSG: say it (once) and stop. Called inside $(...) it stops only that subshell, so the
# marker file is what the main flow tests: no report is printed after a fault.
fault() {
  [ -e "$W/fault" ] || printf 'premise-check: FAULT — %s\n' "$1" >&2
  : > "$W/fault"
  exit 1
}
faulted() { [ -e "$W/fault" ]; }
# say TEXT: one line, as written (dash's echo would expand a backslash in an issue's text).
say() { printf '%s\n' "$*"; }

# g ARGS: git in the repository's top directory (pathspecs, `git grep` and ls-tree are relative to
# the working directory). A fault stops the run; with G_NOMATCH=1, exit 1 and no stderr is "no
# match" (git grep), not a fault.
TOP=.
g() {
  git -C "$TOP" "$@" 2>"$W/gerr" && return 0
  _grc=$?
  if [ "${G_NOMATCH:-}" = 1 ] && [ "$_grc" -eq 1 ] && [ ! -s "$W/gerr" ]; then return 0; fi
  _gwhy=$(sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//' "$W/gerr")
  fault "git $1${2:+ $2}${3:+ $3} … failed: ${_gwhy:-$_grc}"
}

command -v jq >/dev/null 2>&1 || fault "jq is not installed (the premise check reads issues and terms with it)"
command -v git >/dev/null 2>&1 || fault "git is not installed"
[ -f "$JQ_LIB" ] || fault "cannot find $JQ_LIB"
q() { jq "$@" -f "$JQ_LIB"; }

ref="" fixture="" issues=""
while [ $# -gt 0 ]; do
  case $1 in
    --at) ref=${2:-}; shift ;;
    --issue-json) fixture=${2:-}; shift ;;
    *) n=${1#\#}
       case $n in '' | *[!0-9]*) fault "unknown argument: $1" ;; esac
       issues="$issues $n" ;;
  esac
  shift
done
[ -n "$fixture" ] || [ -n "$issues" ] || fault "usage: premise-check.sh <issue>... [--at <ref>] | --issue-json <file>"

TOP=$(git rev-parse --show-toplevel 2>"$W/gerr") || fault "git rev-parse --show-toplevel … failed: $(cat "$W/gerr")"
ROOT=$TOP
# shellcheck disable=SC1091
[ -f "$CLAUDUCTOR_FW/lib/conf.sh" ] && . "$CLAUDUCTOR_FW/lib/conf.sh"
DOC_DIRS=${PREMISE_DOC_DIRS-docs openspec}
if [ -z "$ref" ]; then
  # The default is the tree a fix would branch from, so it must be CURRENT: a stale origin/main
  # misses exactly the most recent drift, and says nothing. A failed fetch is said.
  ref="origin/${MAIN_BRANCH:-main}"
  git -C "$TOP" fetch -q origin "${MAIN_BRANCH:-main}" >/dev/null 2>&1 \
    || say "premise-check: could not fetch $ref — checking the local copy, which may be stale." >&2
fi

# The issues, read first: a fault on any one prints no report at all.
k=0
if [ -n "$fixture" ]; then
  [ -f "$fixture" ] || fault "cannot read $fixture"
  jq -e 'type == "object"' "$fixture" > /dev/null 2>&1 || fault "$fixture is not an issue's JSON"
  cp "$fixture" "$W/issue.1"; k=1
else
  command -v gh >/dev/null 2>&1 || fault "gh is not installed, so the issue cannot be read (or pass --issue-json <file>)"
  for n in $issues; do
    k=$((k + 1))
    gh issue view "$n" --json number,title,createdAt,body,comments > "$W/issue.$k" 2>"$W/gerr" \
      || fault "$(sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//' "$W/gerr")"
  done
fi

# is_doc PATH: a docs file (PREMISE_DOC_DIRS, or any .md), whose hits sort after code's.
is_doc() {
  case $1 in *.md) return 0 ;; esac
  for _dd in $DOC_DIRS; do case $1 in "$_dd"/*) return 0 ;; esac; done
  return 1
}

# changed_since FILE: the commits on $sha that touched FILE since filing, newest first.
changed_since() { g log --format='%h %cs %s' --since="$created" "$sha" -- ":(literal)$1"; }

# history FILE_OF_SPELLINGS: the commits that added or removed any spelling, newest first, at most
# MAX_HISTORY, CODE first: docs are consulted only when no code commit ever touched the term.
history() {
  : > "$W/hist"
  set --
  for _dd in $DOC_DIRS; do set -- "$@" ":(exclude)$_dd"; done
  while IFS= read -r _n; do
    g log --format='%h%x09%ct%x09%cI%x09%s' "-S$_n" "$sha" -- . "$@" ":(exclude,glob)**/*.md" >> "$W/hist" || return 1
  done < "$W/spell"
  if [ ! -s "$W/hist" ]; then
    while IFS= read -r _n; do
      g log --format='%h%x09%ct%x09%cI%x09%s' "-S$_n" "$sha" >> "$W/hist" || return 1
    done < "$W/spell"
  fi
  awk -F'\t' '!seen[$1]++' "$W/hist" | sort -s -t "$TAB" -k2,2nr | head -n "$MAX_HISTORY" \
    | q -rR --arg mode hist --arg filed "$filed"
}

# deletion PATH: the commit that deleted or MOVED a file ending in PATH, if any.
deletion() {
  g log -M --diff-filter=DR --name-status --format='@%h %cs %s' "$sha" > "$W/del" || return 1
  P=$1 awk -F'\t' '
    function hit(f) { return f == p || (length(f) > length(p) && substr(f, length(f) - length(p)) == "/" p) }
    BEGIN { p = ENVIRON["P"] }
    /^@/ { head = substr($0, 2); next }
    $1 == "D" && hit($2) { print head " — deleted " $2; exit }
    $1 ~ /^R/ && hit($2) { print head " — MOVED " $2 " → " $3; exit }' "$W/del"
}

check_issue() {  # check_issue ISSUE_JSON: the report, on stdout
  number=$(jq -r '.number' "$1"); title=$(jq -r '.title // ""' "$1"); created=$(jq -r '.createdAt' "$1")
  filed=$(jq -r '.createdAt | sub("\\.[0-9]+Z$"; "Z") | fromdateiso8601' "$1" 2>/dev/null) \
    || fault "issue #$number: createdAt \"$created\" is not an ISO 8601 UTC time"
  sha=$(g rev-parse --verify "$ref^{commit}") || exit 1
  ref_date=$(g log -1 --format=%cI "$sha") || exit 1
  g ls-tree -r --name-only "$sha" > "$W/files" || exit 1
  since=$(g rev-list --count --since="$created" "$sha") || exit 1

  say "## Premise check — #$number: $title"
  say "Filed $(printf '%s' "$created" | cut -c1-10). Checked against $ref @ $(printf '%s' "$sha" | cut -c1-12) (committed $(printf '%s' "$ref_date" | cut -c1-10))."
  say "$since commit(s) have landed on $ref since it was filed."
  echo

  q -r --arg mode terms < "$1" > "$W/terms" || fault "issue #$number: could not read its terms"
  total=$(q -r --arg mode count < "$1")
  : > "$W/prose"; : > "$W/common"
  checked=0
  while IFS="$US" read -r term path ident t40 t50; do
    faulted && exit 1
    if [ -n "$path" ]; then
      checked=$((checked + 1))
      P=$path awk 'BEGIN { p = ENVIRON["P"]; wd = index(p, "/") > 0; lp = length(p) }
        wd && ($0 == p || (length($0) > lp && substr($0, length($0) - lp) == "/" p)) { print; next }
        !wd { n = split($0, a, "/"); if (a[n] == p) print }' "$W/files" > "$W/matches"
      m=$(wc -l < "$W/matches" | tr -d ' ')
      if [ "$m" -eq 0 ]; then
        del=$(deletion "$path") || exit 1
        if [ -n "$del" ]; then say "- \`$term\` — **file NOT in the tree**. $del"
        else say "- \`$term\` — **file NOT in the tree** (no deletion found — check the path)"; fi
        continue
      fi
      [ "$m" -gt 1 ] && say "- \`$term\` — **ambiguous: $m files match**"
      head -n "$MAX_PATHS" "$W/matches" > "$W/shown"
      while IFS= read -r f; do
        changed_since "$f" > "$W/touched" || exit 1
        tn=$(grep -c . "$W/touched")
        if [ "$m" -gt 1 ]; then lead="    "; else lead="- \`$term\` "; fi
        if [ "$tn" -eq 0 ]; then say "${lead}→ $f — unchanged since filing"
        else say "${lead}→ $f — **changed ${tn}x since filing** (line numbers in the write-up may be stale)"; fi
        head -n "$MAX_HISTORY" "$W/touched" | sed 's/^/      /'
      done < "$W/shown"
      [ "$m" -gt "$MAX_PATHS" ] && say "    … and $((m - MAX_PATHS)) more"
      continue
    fi

    q -rn --arg mode variants --arg t "$term" > "$W/spell"
    : > "$W/hits"
    while IFS= read -r v; do
      G_NOMATCH=1 g grep -n -F -I -e "$v" "$sha" -- >> "$W/hits" || exit 1
    done < "$W/spell"
    # Unique, in the order found, then code before docs (a stable partition).
    cut -c"$((${#sha} + 2))"- "$W/hits" | awk '!seen[$0]++' > "$W/where"
    : > "$W/code"; : > "$W/docs"
    while IFS= read -r w; do
      if is_doc "${w%%:*}"; then printf '%s\n' "$w" >> "$W/docs"; else printf '%s\n' "$w" >> "$W/code"; fi
    done < "$W/where"
    cat "$W/code" "$W/docs" > "$W/where"
    nw=$(grep -c '' "$W/where")
    if [ "$nw" -eq 0 ]; then
      # Absent now. Did it EVER exist? If so it was removed: the strongest drift signal.
      history > "$W/removed" || exit 1
      if [ ! -s "$W/removed" ]; then printf '"%s"\n' "$t50" >> "$W/prose"; continue; fi
      say "- \"$term\" — **NO LONGER IN THE TREE.** Last added/removed by:"
      sed 's/^/    /' "$W/removed"
      checked=$((checked + 1))
      continue
    fi
    # "Too common" counts CODE hits, and never applies to a real identifier: those are exactly the
    # names a premise rests on. It is for words like "main" and "closed".
    if [ "$ident" != 1 ] && [ "$(grep -c '' "$W/code")" -gt "$COMMON" ]; then
      printf '"%s" (%s)\n' "$t40" "$nw" >> "$W/common"; continue
    fi
    checked=$((checked + 1))
    say "- \"$term\" — present, $nw place(s):"
    : > "$W/seen"
    head -n "$MAX_HITS" "$W/where" | q -rR --arg mode hit > "$W/shown"
    while IFS="$US" read -r file line content; do
      note=""
      if ! grep -qxF -- "$file" "$W/seen"; then
        printf '%s\n' "$file" >> "$W/seen"
        changed_since "$file" > "$W/touched" || exit 1
        tn=$(grep -c . "$W/touched")
        [ "$tn" -gt 0 ] && note="  [file changed ${tn}x since filing]"
      fi
      say "    $file:$line  $content$note"
    done < "$W/shown"
    [ "$nw" -gt "$MAX_HITS" ] && say "    … and $((nw - MAX_HITS)) more"
    history > "$W/removed" || exit 1
    sed 's/^/    history: /' "$W/removed"
  done < "$W/terms"
  faulted && exit 1

  echo
  [ -s "$W/common" ] && say "Too common to be a specific claim, NOT checked: $(awk 'NR > 1 { printf ", " } { printf "%s", $0 }' "$W/common")"
  [ -s "$W/prose" ] && say "Not in any commit, ever — prose, a command or a value, NOT checked: $(awk 'NR > 1 { printf ", " } { printf "%s", $0 }' "$W/prose")"
  [ "$total" -gt "$MAX_TERMS" ] && say "$((total - MAX_TERMS)) further term(s) past the first $MAX_TERMS NOT checked."
  fences=$(q -r --arg mode fences < "$1")
  [ "$fences" -ge 1 ] && say "$fences fenced block(s) skipped (logs/traces) — read them yourself."
  say "$checked named thing(s) checked. This check cannot judge a NEGATIVE claim (\"X doesn't do Y\"): read the lines above against the write-up before starting the fix."
  say "premise-check: #$number @ $(printf '%s' "$sha" | cut -c1-12)"
}

i=0
while [ "$i" -lt "$k" ]; do
  i=$((i + 1))
  [ "$i" -gt 1 ] && printf '\n' >> "$W/report"
  check_issue "$W/issue.$i" > "$W/one" || { faulted || fault "the check of issue $i stopped (exit $?)"; exit 1; }
  faulted && exit 1
  cat "$W/one" >> "$W/report"
done
cat "$W/report"
