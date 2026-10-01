#!/bin/sh
# scenario-trace.sh: which test files cite each scenario ID (D4 of the change process).
#
#   sh .claude/scenario-trace.sh                 every open change and the living specs
#   sh .claude/scenario-trace.sh --change <id>   one open change's added and modified scenarios
#   sh .claude/scenario-trace.sh --specs         the living specs only
#   sh .claude/scenario-trace.sh --check ...     exit 1 when an enforced scenario is MISSING
#   sh .claude/scenario-trace.sh --now ...       trace a change still being built as if it were
#                                                finished (the reviewer's view, mid-build)
#   sh .claude/scenario-trace.sh --rev <sha> ... read the tree at a commit, not the working tree
#                                                (pr-merge-guard reads the PR's head this way)
#
# One line per scenario:
#   CITED    <id>  <where it is from>  <test file>...
#   MANUAL   <id>  <where>  manual: <reason>          (or untestable: <reason>)
#   MISSING  <id>  <where>
#   PENDING  change <id>: <n> task(s) open; its scenarios are enforced once every task is ticked
#   LEGACY   change <id>: grandfathered (CHANGES_LEGACY); its scenarios are not enforced
#
# THE RULE. Every scenario ID a change adds or modifies (its ADDED and MODIFIED blocks) must appear
# in at least one TEST FILE: in a test name, a comment or a tag, in any language. The other way is
# a line of that change's tasks.md naming the ID with `(manual: <reason>)` or
# `(untestable: <reason>)`. A change is enforced once it is COMPLETE (every task ticked, the Slice
# line aside): a proposal and a build in progress have tests still to write, and a red gate there
# would stop the build it is meant to guard. The living specs are enforced always, so deleting the
# last test that cited a merged scenario fails the gate; their escapes are read from every tasks.md
# under CHANGES_DIR, archived ones included.
#
# WHAT A TEST FILE IS comes from TEST_GLOBS in .claude/project.conf, over the files git knows
# (tracked, plus untracked and not ignored): the authority, not a list of directories typed here.
# An entry ending in `/` is a directory name at any depth (`tests/`); an entry with no `/` is a
# file name at any depth (`*_test.go`); any other entry is a path glob from the root. Files under
# CHANGES_DIR and SPECS_DIR never count: a spec cannot be its own test.
#
# A citation is the ID as a whole token: AUTH-2-S1 is not cited by AUTH-2-S10 or XAUTH-2-S1.
# Grep-level on purpose: it proves a test NAMES the scenario, not that it asserts the THEN. The
# reviewer agent checks that (`.claude/agents/reviewer.md`).
ROOT=$(cd "$(dirname "$0")/.." 2>/dev/null && pwd)
. "$ROOT/.claude/lib/conf.sh"
. "$ROOT/.claude/lib/change.sh"
# shellcheck disable=SC1091
[ -f "$ROOT/.claude/lib/records.sh" ] && . "$ROOT/.claude/lib/records.sh"

check=""; now=""; rev=""; only_specs=""; changes=""; any_change=""
while [ $# -gt 0 ]; do
  case "$1" in
    --check) check=1 ;;
    --now) now=1 ;;
    --rev) shift; rev=$1 ;;
    --specs) only_specs=1 ;;
    --change) shift; changes="$changes $1"; any_change=1 ;;
    -h|--help) sed -n '2,/^ROOT=/p' "$0" | sed '$d; s/^# \{0,1\}//'; exit 0 ;;
    *) echo "scenario-trace: unknown argument $1 (see --help)" >&2; exit 64 ;;
  esac
  shift
done

git -C "$ROOT" rev-parse --git-dir >/dev/null 2>&1 || { echo "CANNOT CHECK — $ROOT is not a git checkout; scenario tracing reads the files git knows"; exit 2; }
if [ -n "$rev" ]; then
  git -C "$ROOT" cat-file -e "$rev^{commit}" 2>/dev/null || { echo "CANNOT CHECK — commit $rev is not available locally"; exit 2; }
fi

tmp=$(mktemp -d "${TMPDIR:-/tmp}/trace.XXXXXX") || exit 2
trap 'rm -rf "$tmp"' EXIT INT TERM

# ls_under DIR: the files under DIR at the rev or in the working tree, relative to ROOT.
ls_under() {
  if [ -n "$rev" ]; then git -C "$ROOT" ls-tree -r --name-only "$rev" -- "$1/" 2>/dev/null
  else git -C "$ROOT" ls-files -co --exclude-standard -- "$1/" 2>/dev/null; fi
}
# fetch PATH: a copy of PATH (at the rev or on disk) under $tmp, printing the copy's path.
fetch() {
  _dst="$tmp/f/$1"; mkdir -p "$(dirname "$_dst")"
  if [ -n "$rev" ]; then git -C "$ROOT" show "$rev:$1" > "$_dst" 2>/dev/null || : > "$_dst"
  else cat "$ROOT/$1" > "$_dst" 2>/dev/null || : > "$_dst"; fi
  printf '%s' "$_dst"
}

# ── The test files' citations: ONE grep over every test file, for every ID-shaped token ──────────
set --
for g in ${TEST_GLOBS:-}; do
  case "$g" in
    */) set -- "$@" ":(glob)**/${g%/}/**" ;;
    */*) set -- "$@" ":(glob)$g" ;;
    *) set -- "$@" ":(glob)**/$g" ;;
  esac
done
set -- "$@" ":(glob,exclude)$CHANGES_DIR/**" ":(glob,exclude)$SPECS_DIR/**"
if [ -z "${TEST_GLOBS:-}" ]; then
  :   # no test files at all: nothing cites anything, and every enforced scenario is MISSING
elif [ -n "$rev" ]; then
  git -C "$ROOT" grep -I -o -E "$SCENARIO_ID_ERE" "$rev" -- "$@" 2>/dev/null | sed "s|^$rev:||"
else
  git -C "$ROOT" grep -I -o --untracked -E "$SCENARIO_ID_ERE" -- "$@" 2>/dev/null
fi | awk -F: '{ f = $1; id = $NF; if (!((id, f) in seen)) { seen[id, f] = 1; files[id] = files[id] " " f } }
     END { for (id in files) print id "\t" substr(files[id], 2) }' > "$tmp/cites"

# ── What is enforced ─────────────────────────────────────────────────────────────────────────────
# $tmp/want: <id> TAB <where> TAB <escapes file>   (the escapes file lists "<id>\t<escape>")
: > "$tmp/want"; : > "$tmp/pending"
all_escapes="$tmp/esc-all"; : > "$all_escapes"
for t in $(ls_under "$CHANGES_DIR" | grep '/tasks\.md$'); do task_escapes "$(fetch "$t")" >> "$all_escapes"; done

if [ -z "$only_specs" ]; then
  if [ -z "$any_change" ]; then
    changes=$(ls_under "$CHANGES_DIR" | awk -F/ -v d="$CHANGES_DIR" '{ p = substr($0, length(d) + 2); split(p, a, "/"); if (a[1] != "archive" && a[1] != "" && index(p, "/")) print a[1] }' | sort -u)
  fi
  for c in $changes; do
    base="$CHANGES_DIR/$c"
    files=$(ls_under "$base")
    [ -n "$files" ] || { echo "MISSING  change $c: no such open change in $CHANGES_DIR"; echo x >> "$tmp/bad"; continue; }
    # A grandfathered change (CHANGES_LEGACY, lib/records.sh) was proposed before adoption, under
    # the project's earlier format: its scenarios are not enforced here. Once archived, any of them
    # that carries an ID is held in the living specs like every other.
    if command -v change_is_legacy >/dev/null 2>&1 && change_is_legacy "$c"; then
      echo "LEGACY   change $c: grandfathered (CHANGES_LEGACY); its scenarios are not enforced" >> "$tmp/pending"
      continue
    fi
    tasks=$(fetch "$base/tasks.md")
    open=$(open_tasks "$tasks")
    if [ "$open" -gt 0 ] && [ -z "$now" ]; then
      echo "PENDING  change $c: $open task(s) open; its scenarios are enforced once every task is ticked" >> "$tmp/pending"
      continue
    fi
    task_escapes "$tasks" > "$tmp/esc-$c"
    for s in $(printf '%s\n' "$files" | grep -E '/specs/[^/]+/spec\.md$'); do
      spec_scenarios "$(fetch "$s")" | awk -F'\t' -v w="change $c" -v e="$tmp/esc-$c" '($1 == "ADDED" || $1 == "MODIFIED") && $3 != "-" { print $3 "\t" w "\t" e }' >> "$tmp/want"
    done
  done
fi
if [ -z "$any_change" ]; then
  for s in $(ls_under "$SPECS_DIR" | grep '/spec\.md$'); do
    case "${s#"$SPECS_DIR"/}" in */*/*) continue ;; esac   # specs/<capability>/spec.md only
    spec_scenarios "$(fetch "$s")" | awk -F'\t' -v w="spec ${s#"$SPECS_DIR"/}" -v e="$all_escapes" '$3 != "-" { print $3 "\t" w "\t" e }' >> "$tmp/want"
  done
fi

# ── Verdicts ─────────────────────────────────────────────────────────────────────────────────────
cat "$tmp/pending"
missing=0
while IFS="$(printf '\t')" read -r id where esc; do
  [ -n "$id" ] || continue
  hit=$(awk -F'\t' -v id="$id" '$1 == id { print $2; exit }' "$tmp/cites")
  if [ -n "$hit" ]; then printf 'CITED    %s  (%s)  %s\n' "$id" "$where" "$hit"; continue; fi
  why=$(awk -F'\t' -v id="$id" '$1 == id { print $2; exit }' "$esc")
  if [ -n "$why" ]; then printf 'MANUAL   %s  (%s)  %s\n' "$id" "$where" "$why"; continue; fi
  printf 'MISSING  %s  (%s)  no test file cites it (TEST_GLOBS: %s), and no tasks.md line escapes it\n' "$id" "$where" "${TEST_GLOBS:-none}"
  missing=$((missing + 1))
done < "$tmp/want"
[ -f "$tmp/bad" ] && missing=$((missing + 1))

n=$(grep -c . "$tmp/want" | tr -d ' ')
if [ "$missing" -gt 0 ]; then
  echo "scenario-trace: $missing of $n enforced scenario(s) MISSING${rev:+ at $(printf %.9s "$rev")}"
  [ -n "$check" ] && exit 1
  exit 0
fi
echo "scenario-trace: all $n enforced scenario(s) cited or escaped${rev:+ at $(printf %.9s "$rev")}"
exit 0
