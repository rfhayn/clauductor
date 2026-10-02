#!/bin/sh
# The write-surfaces module (.claude/modules/write-surfaces), tested whether it is on or not,
# through the real pr-merge-guard.sh in a throwaway repo with a stub gh (no network). The PRs are
# the shapes Standing Tee's rule 5 was calibrated on: a change adding 10 routes and screens (never
# converged), one adding exactly 4 (split mid-flight), and ones that must stay quiet: 3 added, 5
# MODIFIED, 4 renamed, 4 added on a fix/ branch, 4 added that match no pattern. Then the knobs
# (WRITE_SURFACE_MAX, empty WRITE_SURFACE_GLOBS), patterns never expanded against the tree, a bad
# setting said rather than blocking, and the module off. The rule is advisory: every merge here is
# allowed, and what is checked is whether the advisory reaches Claude.
# With the module on, modules/write-surfaces/checks/project.sh checks this project's settings.
. "$(dirname "$0")/lib.sh"
need git jq

M="$ROOT/.claude/modules/write-surfaces"
d=$(scratch)
G="$d/app"; new_repo "$G"
mkdir -p "$G/.claude/hooks/lib" "$G/.claude/lib" "$G/.claude/modules" "$G/docs" "$d/bin"
cp "$ROOT/.claude/hooks/pr-merge-guard.sh" "$G/.claude/hooks/"
cp "$ROOT/.claude/hooks/lib/"* "$G/.claude/hooks/lib/"
cp "$ROOT/.claude/lib/conf.sh" "$ROOT/.claude/lib/change.sh" "$ROOT/.claude/lib/evals.sh" "$ROOT/.claude/lib/modules.sh" "$G/.claude/lib/"
cp "$ROOT/.claude/scenario-trace.sh" "$G/.claude/"
cp -R "$M" "$G/.claude/modules/write-surfaces"
GLOBS='*app/api/v1/*route.ts *apps/web/app/*page.tsx'
conf() { printf '%s\n' "$@" > "$G/.claude/project.conf"; }
conf 'MODULES="write-surfaces"' "WRITE_SURFACE_GLOBS=\"$GLOBS\""
# Untracked, so a setting holds across the branch switches below rather than riding into a commit.
echo ".claude/project.conf" >> "$G/.git/info/exclude"
printf '# Journal\n\n## Session 1 — 2026-01-01 — a — start\n' > "$G/docs/development-journal.md"
mkdir -p "$G/apps/api/app/api/v1/old" "$G/apps/web/app/old"
for i in 1 2 3 4 5; do
  mkdir -p "$G/apps/api/app/api/v1/r$i"; echo "export const GET = $i;" > "$G/apps/api/app/api/v1/r$i/route.ts"
done
# A file a pattern would turn into if the shell expanded it against the tree: the rule must not.
echo "top" > "$G/apps/web/app/top-page.tsx"
git -C "$G" add -A && git -C "$G" commit -qm base
git init -q --bare "$G/acme/app.git"; echo "acme/" >> "$G/.git/info/exclude"
git -C "$G" remote add origin acme/app.git
(cd "$G" && git push -q origin HEAD:main 2>/dev/null && git fetch -q origin)
cat > "$d/bin/gh" <<'EOF'
#!/bin/sh
case "$1 $2" in
  "pr checks") echo '[]' ;;
  "pr view")
    case "$*" in
      *headRefName*) printf '{"headRefName":"%s","headRefOid":"%s"}\n' "$GH_BRANCH" "$GH_HEAD" ;;
      *headRefOid*) echo "$GH_HEAD" ;;
    esac ;;
  "run list") echo 0 ;;
  *) exit 1 ;;
esac
EOF
chmod +x "$d/bin/gh"

# pr BRANCH SHELL-CODE: run SHELL-CODE in the repo on BRANCH (cut from main), commit, back to main;
# HEAD_PR is the new head, with a clean full receipt for it. An ops/ branch: rules 3 and 9 stay out.
pr() {
  git -C "$G" checkout -q -B "$1" main
  (cd "$G" && eval "$2")
  git -C "$G" add -A && git -C "$G" commit -qm "$1"
  HEAD_PR=$(git -C "$G" rev-parse HEAD)
  git -C "$G" checkout -q main
  printf '%s\tfull\tclean\tall\n' "$HEAD_PR" > "$G/.git/ci-receipt"
}
routes() { for r in "$@"; do mkdir -p "apps/api/app/api/v1/$r"; echo "export const POST = 1;" > "apps/api/app/api/v1/$r/route.ts"; done; }
screens() { for s in "$@"; do mkdir -p "apps/web/app/$s"; echo "export default function P() {}" > "apps/web/app/$s/page.tsx"; done; }
# merge LABEL WANT(yes|no) [TEXT] [BRANCH]: the merge is allowed; the advisory reaches Claude or not.
merge() {
  _l=$1 _w=$2 _t=${3:-write surfaces (WRITE_SURFACE_GLOBS} _b=${4:-change/x}
  rc=0
  out=$(payload "gh pr merge 7 --squash" "$G" | (cd "$G" && env PATH="$d/bin:$PATH" GH_HEAD="$HEAD_PR" GH_BRANCH="$_b" sh "$G/.claude/hooks/pr-merge-guard.sh" 2>"$d/err")) || rc=$?
  [ "$rc" = 0 ] || { fail "write surfaces: $_l: the merge was BLOCKED (exit $rc); the rule is advisory: $(head -3 "$d/err")"; return; }
  case "$out" in *'"additionalContext"'*"$_t"*) got=yes ;; *) got=no ;; esac
  [ "$got" = "$_w" ] && ok "write surfaces: $_l" || fail "write surfaces: $_l (advisory '$_t' present: $got, want $_w): $(printf '%s' "$out" | head -c 500)"
}
# rule LABEL WANT [TEXT] [BRANCH]: the same, but the module's rule alone, with the facts the guard
# hands it (GUARD_*). The full guard costs a second a run; two merges through it prove the wiring
# (the advisory reaches Claude; a module that is off adds nothing), so the rest ask the rule.
rule() {
  _l=$1 _w=$2 _t=${3:-write surfaces (WRITE_SURFACE_GLOBS} _b=${4:-change/x}
  rc=0
  out=$(cd "$G" && env ROOT="$G" GUARD_PR=7 GUARD_BRANCH="$_b" GUARD_BASE="$(git -C "$G" merge-base main "$HEAD_PR")" GUARD_HEAD="$HEAD_PR" \
    sh "$G/.claude/modules/write-surfaces/guard.d/write-surfaces.sh" 2>"$d/err") || rc=$?
  [ "$rc" = 0 ] || { fail "write surfaces: $_l: the merge was BLOCKED (exit $rc); the rule is advisory: $(head -3 "$d/err")"; return; }
  case "$out" in *"$_t"*) got=yes ;; *) got=no ;; esac
  [ "$got" = "$_w" ] && ok "write surfaces: $_l" || fail "write surfaces: $_l (advisory '$_t' present: $got, want $_w): $(printf '%s' "$out" | head -c 500)"
}
# The guard's rules 3 and 9 police change/ branches; give the change its slice and its tasks done.
change() { printf '%s\n' "mkdir -p changes/x && printf '## 1. Do\n- [x] 1.1 it\n\n- [ ] Slice: a user can x at /x\n' > changes/x/tasks.md; $1"; }

pr change/x "$(change 'routes a b c d e f; screens s1 s2 s3 s4')"
merge "a change adding 10 routes and screens draws the advisory" yes "adds 10 write surfaces"
case "$out" in *"… and 4 more"*) ok "write surfaces: ...naming six of them and how many more" ;; *) fail "write surfaces: ...naming six of them and how many more: not in the advisory: $(printf '%s' "$out" | head -c 300)" ;; esac
pr change/x "$(change 'routes a b; screens s1 s2')"
rule "a change adding exactly WRITE_SURFACE_MAX (4) draws it" yes "adds 4 write surfaces"
pr change/x "$(change 'routes a b; screens s1')"
rule "a change adding 3 stays quiet" no
pr change/x "$(change 'for i in 1 2 3 4 5; do echo "// edit" >> apps/api/app/api/v1/r$i/route.ts; done')"
rule "a change that only MODIFIES 5 routes stays quiet (added files only)" no
pr change/x "$(change 'for i in 1 2 3 4; do git mv apps/api/app/api/v1/r$i apps/api/app/api/v1/moved$i; done')"
rule "a change that RENAMES 4 routes stays quiet (a rename is not an addition)" no
pr change/x "$(change 'mkdir -p docs/a && for i in 1 2 3 4 5; do echo x > docs/a/route$i.md; done')"
rule "a change adding 5 files that match no pattern stays quiet" no
pr fix/9-x 'routes a b; screens s1 s2'
rule "a fix/ PR adding 4 stays quiet (capability changes only, BRANCH_CHANGE)" no "" fix/9-x
pr change/x "$(change 'screens s1 s2 s3 s4')"
rule "patterns are matched against the added files, never expanded against the tree" yes "adds 4 write surfaces"
pr change/x "$(change 'routes a b c d e f; screens s1 s2 s3 s4')"
conf 'MODULES="write-surfaces"' "WRITE_SURFACE_GLOBS=\"$GLOBS\"" 'WRITE_SURFACE_MAX="11"'
rule "WRITE_SURFACE_MAX=11: 10 stays quiet" no
conf 'MODULES="write-surfaces"' "WRITE_SURFACE_GLOBS=\"$GLOBS\"" 'WRITE_SURFACE_MAX="many"'
rule "a WRITE_SURFACE_MAX that is not a number is said, not blocked on" yes "CANNOT CHECK — WRITE_SURFACE_MAX"
conf 'MODULES="write-surfaces"'
rule "an empty WRITE_SURFACE_GLOBS is said, not silently counted as zero" yes "WRITE_SURFACE_GLOBS is empty"
conf 'MODULES="write-surfaces"' 'WRITE_SURFACE_GLOBS="*apps/web/app/*page.tsx"' 'WRITE_SURFACE_MAX="2"'
pr change/x "$(change 'routes a b c; screens s1 s2')"
rule "the project's own settings: WRITE_SURFACE_MAX=2, screens only" yes "adds 2 write surfaces"
mv "$G/.claude/lib/conf.sh" "$d/conf.bak"
rule "a missing configuration is said as CANNOT CHECK, never a crash the guard would block on" yes "CANNOT CHECK — cannot read the project's configuration"
mv "$d/conf.bak" "$G/.claude/lib/conf.sh"
conf 'MODULES=""' "WRITE_SURFACE_GLOBS=\"$GLOBS\""
pr change/x "$(change 'routes a b c d e f; screens s1 s2 s3 s4')"
merge "with the module OFF, 10 surfaces draw nothing (the rule is the module's)" no "write surface"

# The module's own check: it fails until the project says what a write surface is.
conf 'MODULES="write-surfaces"'
mkdir -p "$G/.claude/checks"; cp "$ROOT/.claude/checks/lib.sh" "$ROOT/.claude/checks/run.sh" "$G/.claude/checks/"
out=$(cd "$G" && sh .claude/checks/run.sh write-surfaces:project 2>&1); rc=$?
[ "$rc" != 0 ] && case "$out" in *"WRITE_SURFACE_GLOBS is empty"*) true ;; *) false ;; esac \
  && ok "write-surfaces:project fails while WRITE_SURFACE_GLOBS is empty" || fail "write-surfaces:project fails while WRITE_SURFACE_GLOBS is empty: it did not: $out"
conf 'MODULES="write-surfaces"' 'WRITE_SURFACE_GLOBS="*app/api/v1/*route.ts *nowhere/*.tsx"'
out=$(cd "$G" && sh .claude/checks/run.sh write-surfaces:project 2>&1); rc=$?
[ "$rc" != 0 ] && case "$out" in *"pattern *nowhere/*.tsx matches no file"*) true ;; *) false ;; esac \
  && ok "write-surfaces:project fails a pattern that matches no file" || fail "write-surfaces:project fails a pattern that matches no file: it did not: $out"
conf 'MODULES="write-surfaces"' 'WRITE_SURFACE_GLOBS="*app/api/v1/*route.ts"'
out=$(cd "$G" && sh .claude/checks/run.sh write-surfaces:project 2>&1) && ok "write-surfaces:project passes a project that names its surfaces" || fail "write-surfaces:project passes a project that names its surfaces: it did not: $out"
finish
