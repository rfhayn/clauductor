#!/bin/sh
# The full gate's race step (scripts/ci/steps.sh, OPS-19) races only the packages whose tests can
# see the change, and fails safe to the whole suite. This repo's own check: the template does not
# ship it (its steps.sh is the project's own, and has no race scope).
#
# Two halves:
#  1. The mapping against its AUTHORITY, the tests: every framework test file that reads outside
#     framework/ is found by its `..` chain or its repo-root helper, what it reads is named, and
#     race_select on that path must select its package. A new test reading a new path fails here
#     until steps.sh maps it, instead of being skipped by every template-only gate.
#  2. The selection on real diffs, in throwaway repos: template-only -> the subset; framework/ ->
#     everything; a diff that cannot be computed (no base, a shallow clone) -> everything; a path
#     panel tests read -> internal/panel's packages; nothing mapped -> the baseline, never nothing.
. "$(dirname "$0")/lib.sh"
need git

STEPS="$ROOT/scripts/ci/steps.sh"
[ -f "$STEPS" ] || { fail "no $STEPS"; finish; }
. "$STEPS"
command -v race_select >/dev/null 2>&1 || { fail "steps.sh defines no race_select: the race step has no scope"; finish; }

# selects PATH PKG: race_select on PATH races PKG (or everything).
selects() {
  _got=$(printf '%s\n' "$1" | race_select | cut -f1)
  [ "$_got" = ./... ] && return 0
  case " $_got " in *" ./$2 "*) return 0 ;; esac
  return 1
}

# ── 1. The mapping, from the tests ──────────────────────────────────────────────────────────────
pairs=""
nl='
'
found=0
for f in $(cd "$ROOT/framework" && find . -name '*_test.go' -not -path '*/testdata/*' | sed 's|^\./||' | sort); do
  pkg=$(dirname "$f")
  depth=$(printf '%s\n' "framework/$pkg" | awk -F/ '{print NF}')
  src="$ROOT/framework/$f"
  # The longest run of "..", in a filepath.Join, on any line: as deep as the package is, it
  # leaves framework/.
  dots=$(grep -o 'Join("\.\."\(, "\.\."\)*' "$src" | awk -F'"\\.\\."' '{ if (NF-1 > m) m = NF-1 } END { print m+0 }')
  outward=0
  [ "$dots" -ge "$depth" ] && outward=1
  [ "$outward" -eq 1 ] || continue
  found=$((found + 1))
  reads=""
  grep -q '"template"' "$src" && reads="$reads template/x"
  grep -q 'PluginDrift(' "$src" && reads="$reads plugin/x .claude-plugin/x"
  grep -q '"install\.sh"' "$src" && reads="$reads install.sh"
  grep -q '"\.claude-plugin", "marketplace.json"' "$src" && reads="$reads .claude-plugin/x plugin/x"
  for d in $(grep -oE '"docs", "[^"]+"|docsPath\("[^"]+"\)|Join\(docs, "[^"]+"\)' "$src" | grep -oE '"[^"]+"\)?$' | tr -d '")'); do
    reads="$reads docs/$d"
  done
  grep -qE 'Join\((root|repo|repoRoot\(t\)), "\.(claude|clauductor)"' "$src" && reads="$reads .claude/x"
  if [ -z "$reads" ]; then
    fail "framework/$f reads outside framework/ (a \"..\" chain of $dots) but this check cannot tell what: name it here and map it in steps.sh"
    continue
  fi
  for r in $(printf '%s\n' $reads | sort -u); do pairs="$pairs$pkg $r $f$nl"; done
done
[ "$found" -gt 0 ] || fail "found no framework test that reads outside framework/: the scan is broken (internal/plugin's and internal/panel/config's tests do)"

missed=0
while read -r pkg path f; do
  [ -n "$pkg" ] || continue
  if ! selects "$path" "$pkg"; then
    fail "framework/$f ($pkg) reads $path, but a change to $path does not race $pkg: map it in scripts/ci/steps.sh race_select"
    missed=$((missed + 1))
  fi
done <<EOF
$pairs
EOF
[ "$missed" -eq 0 ] && ok "every package whose tests read outside framework/ is raced by what it reads ($found test files: $(printf '%s' "$pairs" | awk '{print $1}' | sort -u | tr '\n' ' '))"

# ── 2. The selection on real diffs ──────────────────────────────────────────────────────────────
d=$(scratch)
R="$d/repo"; new_repo "$R"
mkdir -p "$R/framework" "$R/template" "$R/docs" "$R/.claude"
for p in framework/a.go template/a docs/roadmap.md docs/panel.md .claude/a AGENTS.md; do printf 'x\n' > "$R/$p"; done
git -C "$R" add -A && git -C "$R" commit -qm base
BASE=$(git -C "$R" rev-parse HEAD)

# scope [GATE_RACE_BASE]: race_scope in the repo, as the gate runs it (cwd = the tree).
scope() { (cd "$R" && GATE_RACE_BASE=${1-$BASE} race_scope); }
# change: reset the repo to the base, then apply the shell snippet $1 and commit it.
change() {
  git -C "$R" checkout -q --detach "$BASE" && git -C "$R" clean -qfd
  (cd "$R" && eval "$1") && git -C "$R" add -A && git -C "$R" commit -qm change
}
want() {  # want DESC GOT-LINE WANT-PKGS [WANT-WHY-SUBSTRING]
  _p=$(printf '%s' "$2" | cut -f1); _w=$(printf '%s' "$2" | cut -f2)
  if [ "$_p" = "$3" ] && case "$_w" in *"${4:-}"*) true ;; *) false ;; esac; then ok "$1: $_p ($_w)"
  else fail "$1: got '$_p' ($_w), want '$3'${4:+ (why containing '$4')}"; fi
}

change 'echo y >> template/a; echo y >> docs/roadmap.md'
want "template/ and docs/ only" "$(scope)" "./internal/cmd ./internal/panel/lease ./internal/plugin ./internal/template" "only docs/, template/ changed"

change 'echo y >> template/a; echo y >> framework/a.go'
want "a framework/ change" "$(scope)" "./..." "framework/a.go changed"

change 'echo y >> template/a'
printf 'y\n' > "$R/framework/untracked.go"
want "an untracked framework/ file in a dirty tree" "$(scope)" "./..." "framework/untracked.go changed"
rm -f "$R/framework/untracked.go"

change 'git mv framework/a.go docs/a.go'
want "a file moved out of framework/" "$(scope)" "./..." "framework/a.go changed"

change 'echo y >> docs/panel.md'
got=$(scope)
case " $(printf '%s' "$got" | cut -f1) " in
  *" ./internal/panel/config "*" ./internal/panel/web "*) ok "docs/panel.md, which panel tests read: $(printf '%s' "$got" | cut -f1)" ;;
  *) fail "docs/panel.md, which panel tests read: got '$(printf '%s' "$got" | cut -f1)', want internal/panel/config ... internal/panel/web" ;;
esac

change 'echo y >> .claude/a; echo y >> AGENTS.md'
want "nothing mapped (.claude/, AGENTS.md) still races the baseline" "$(scope)" "./internal/plugin ./internal/template" "only .claude/, AGENTS.md changed"

git -C "$R" checkout -q --detach "$BASE"
want "no change against the base" "$(scope)" "./..." "no change"

change 'echo y >> template/a'
want "a base ref that does not exist" "$(scope refs/remotes/origin/nope)" "./..." "could not be computed"
want "no base given and no origin/main" "$(cd "$R" && unset GATE_RACE_BASE; race_scope)" "./..." "could not be computed"

S="$d/shallow"
git clone -q --depth 1 "file://$R" "$S" 2>/dev/null
if [ "$(git -C "$S" rev-parse --is-shallow-repository 2>/dev/null)" = true ]; then
  want "a shallow clone" "$(cd "$S" && GATE_RACE_BASE=HEAD race_scope)" "./..." "could not be computed"
else
  fail "a shallow clone: could not make one to test"
fi

N="$d/not-a-repo"; mkdir -p "$N"
want "not a git repository (the archive clean room)" "$(cd "$N" && GIT_CEILING_DIRECTORIES=$d GATE_RACE_BASE=HEAD race_scope)" "./..." "could not be computed"

# ── 3. The step itself: it prints the scope and races exactly that ──────────────────────────────
change 'echo y >> template/a'
out=$(cd "$R" && GATE_RACE_BASE=$BASE; export GATE_RACE_BASE
  step() { [ "$1" = "test (race)" ] && { shift; echo "RAN $*"; }; return 0; }
  gate_steps full)
case "$out" in
  *"==> test (race): internal/cmd internal/panel/lease internal/plugin internal/template (only template/ changed)"*) ok "the gate names the race scope and why" ;;
  *) fail "the gate does not name the race scope: $(printf '%s' "$out" | grep 'test (race)')" ;;
esac
case "$out" in
  *"RAN sh -c cd framework && go test -race -timeout 25m \"\$@\" race ./internal/cmd ./internal/panel/lease ./internal/plugin ./internal/template"*) ok "the race step runs the selected packages" ;;
  *) fail "the race step does not run the selected packages: $(printf '%s' "$out" | grep '^RAN')" ;;
esac
out=$(cd "$R" && GATE_RACE_BASE=$BASE; export GATE_RACE_BASE
  step() { [ "$1" = "test (race)" ] && { shift; echo "RAN $*"; }; return 0; }
  gate_steps quick)
case "$out" in *RAN*|*"test (race)"*) fail "a quick gate ran or announced the race step" ;; *) ok "a quick gate has no race step" ;; esac

finish
