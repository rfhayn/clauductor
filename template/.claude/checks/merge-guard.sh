#!/bin/sh
# pr-merge-guard.sh, as a payload → exit-code table in both directions, against a throwaway repo
# whose origin is a local bare repo named like `owner/name` (so the foreign-repo rules run) and a
# stub `gh` on PATH (so no network and no GitHub are needed). Also: the receipt library across
# worktrees, the channel an advisory takes, and raw_command identical in both hooks that carry it.
. "$(dirname "$0")/lib.sh"
need git jq

d=$(scratch)
R="$d/app"; new_repo "$R"
mkdir -p "$R/.claude/hooks/lib" "$R/.claude/lib" "$R/docs" "$d/bin"
cp "$ROOT/.claude/hooks/pr-merge-guard.sh" "$R/.claude/hooks/"
cp "$ROOT/.claude/hooks/lib/"* "$R/.claude/hooks/lib/"
cp "$ROOT/.claude/lib/conf.sh" "$R/.claude/lib/"
cat > "$R/.claude/project.conf" <<'EOF'
GATE_DISPLAY_CONTEXTS="ci/local"
EOF
printf '# Journal\n\n## Session 1 — 2026-01-01 — a — start\n' > "$R/docs/development-journal.md"
git -C "$R" add -A && git -C "$R" commit -qm base
# origin: a bare repo INSIDE the checkout at acme/app.git, so `git remote get-url origin` reads
# `acme/app.git`, which the guard keys as acme/app, and fetches still work offline.
git init -q --bare "$R/acme/app.git"
echo "acme/" >> "$R/.git/info/exclude"
git -C "$R" remote add origin acme/app.git
(cd "$R" && git push -q origin HEAD:main 2>/dev/null && git fetch -q origin)

# A capability-change branch with a change directory, and its head.
git -C "$R" checkout -q -b change/add-x
mkdir -p "$R/changes/add-x"
printf '## 1. Do it\n- [x] 1.1 thing\n' > "$R/changes/add-x/tasks.md"
git -C "$R" add -A && git -C "$R" commit -qm "add-x: task group 1"
NOSLICE=$(git -C "$R" rev-parse HEAD)
printf -- '- [ ] Slice: a user can do x at /x\n' >> "$R/changes/add-x/tasks.md"
git -C "$R" commit -qam "add-x: slice"
SLICE=$(git -C "$R" rev-parse HEAD)
git -C "$R" checkout -q main

cat > "$d/bin/gh" <<'EOF'
#!/bin/sh
case "$1 $2" in
  "pr checks") echo "${GH_CHECKS:-[]}" ;;
  "pr view")
    case "$*" in
      *headRefName*) printf '{"headRefName":"%s","headRefOid":"%s"}\n' "${GH_BRANCH:-fix/1-x}" "$GH_HEAD" ;;
      *headRefOid*) echo "$GH_HEAD" ;;
    esac ;;
  "run list") echo "${GH_RUNS:-0}" ;;
  *) exit 1 ;;
esac
EOF
chmod +x "$d/bin/gh"

MAINSHA=$(git -C "$R" rev-parse HEAD)
receipt() { printf '%s\tfull\t%s\t%s\n' "$1" "${2:-clean}" "${3:-all}" > "$R/.git/ci-receipt"; }
guard() {  # guard WANT LABEL COMMAND [env assignments...]
  want=$1 label=$2 c=$3; shift 3
  rc=0
  out=$(payload "$c" "$R" | (cd "$R" && env PATH="$d/bin:$PATH" GH_HEAD="${GH_HEAD:-$MAINSHA}" "$@" sh "$R/.claude/hooks/pr-merge-guard.sh" 2>"$d/err")) || rc=$?
  expect_rc "$want" "$rc" "merge-guard $( [ "$want" = 2 ] && echo blocks || echo allows ): $label"
  [ "$rc" = "$want" ] || sed 's/^/       /' "$d/err" | head -3
}

guard 0 "a command that is not a merge" "git status"
guard 0 "gh pr view" "gh pr view 5"
guard 2 "--auto" "gh pr merge 5 --squash --auto"
guard 2 "--admin" "gh pr merge 5 --squash --admin"
rm -f "$R/.git/ci-receipt"
guard 2 "no evidence at all" "gh pr merge 5 --squash --delete-branch"
receipt "$MAINSHA"
guard 0 "a full clean receipt for the head" "gh pr merge 5 --squash --delete-branch"
case "$out" in *'"additionalContext"'*"passed in full"*) ok "merge-guard: its confirmation reaches Claude as additionalContext" ;; *) fail "merge-guard: no additionalContext on an allowed merge: $out" ;; esac
receipt "$MAINSHA" dirty
guard 2 "a dirty receipt" "gh pr merge 5 --squash"
receipt "$MAINSHA" clean quick
guard 2 "a partial receipt" "gh pr merge 5 --squash"
receipt 0000000000000000000000000000000000000000
guard 2 "a receipt for another commit" "gh pr merge 5 --squash"
rm -f "$R/.git/ci-receipt"
guard 2 "no remote workflow named, so a remote success does not count" "gh pr merge 5 --squash" GH_RUNS=1
echo 'GATE_REMOTE_WORKFLOW="ci.yml"' >> "$R/.claude/project.conf"
guard 0 "the named remote workflow succeeded" "gh pr merge 5 --squash" GH_RUNS=1
guard 2 "the named remote workflow has no success on the head" "gh pr merge 5 --squash" GH_RUNS=0

# A receipt written in a linked worktree's git dir counts.
git -C "$R" worktree add -q "$d/lane" 2>/dev/null
printf '%s\tfull\tclean\tall\n' "$MAINSHA" > "$(git -C "$d/lane" rev-parse --absolute-git-dir)/ci-receipt"
guard 0 "a receipt in a linked worktree's git dir" "gh pr merge 5 --squash"
guard 2 "a red check" "gh pr merge 5 --squash" GH_CHECKS='[{"name":"lint","state":"FAILURE","bucket":"fail"}]'
guard 0 "a red DISPLAY context (GATE_DISPLAY_CONTEXTS)" "gh pr merge 5 --squash" GH_CHECKS='[{"name":"ci/local","state":"FAILURE","bucket":"fail"}]'

# Rule 3: the slice line, read at the head from the PR's own change directory.
printf '%s\tfull\tclean\tall\n' "$NOSLICE" > "$R/.git/ci-receipt"
guard 2 "a change PR whose tasks.md has no Slice line" "gh pr merge 5 --squash" GH_HEAD="$NOSLICE" GH_BRANCH=change/add-x
printf '%s\tfull\tclean\tall\n' "$SLICE" > "$R/.git/ci-receipt"
guard 0 "a change PR that states its slice" "gh pr merge 5 --squash" GH_HEAD="$SLICE" GH_BRANCH=change/add-x

# Rule 7: a journal session number merged elsewhere after this branch was cut.
git -C "$R" checkout -q -b ops/close main
sed -i.bak 's/^## Session 1/## Session 2 — 2026-01-02 — b — mine\n\n## Session 1/' "$R/docs/development-journal.md" 2>/dev/null
printf '# Journal\n\n## Session 2 — 2026-01-02 — b — mine\n\n## Session 1 — 2026-01-01 — a — start\n' > "$R/docs/development-journal.md"
rm -f "$R/docs/development-journal.md.bak"
git -C "$R" commit -qam "close: session 2"
MINE=$(git -C "$R" rev-parse HEAD)
git -C "$R" checkout -q main
printf '# Journal\n\n## Session 2 — 2026-01-02 — c — theirs\n\n## Session 1 — 2026-01-01 — a — start\n' > "$R/docs/development-journal.md"
git -C "$R" commit -qam "their close: session 2"
(cd "$R" && git push -q origin HEAD:main 2>/dev/null && git fetch -q origin)
printf '%s\tfull\tclean\tall\n' "$MINE" > "$R/.git/ci-receipt"
guard 2 "a journal session number another merged session took" "gh pr merge 5 --squash" GH_HEAD="$MINE" GH_BRANCH=ops/close
receipt "$MAINSHA"

# Reading the command.
guard 0 "a commit message that mentions a merge (prose)" 'git commit -m "then gh pr merge 5 --auto"'
guard 2 "a quoted gh (cannot read the site)" '"gh" pr merge 5 --squash'
guard 2 "a computed word" 'gh pr $(echo merge) 5'
guard 2 "no PR named" "gh pr merge --squash"
guard 2 "two merges in one command" "gh pr merge 5 --squash && gh pr merge 6 --squash"
guard 0 "another repo's merge, in the allowlisted shape" "gh pr merge 2 -R other/tool --squash --delete-branch"
guard 2 "another repo named outside the shape" "echo x | xargs gh pr merge 2 -R other/tool"
guard 2 "this repo named by -R is still policed" "gh pr merge 5 -R acme/app --squash" GH_HEAD=0000000000000000000000000000000000000000

# Without jq: fails CLOSED for a merge, open for anything else.
mkdir -p "$d/nojq"
for t in sh cat git awk sed grep tr sort cut head tail wc dirname basename pwd printf env; do
  p=$(command -v "$t" 2>/dev/null) && ln -sf "$p" "$d/nojq/$t"
done
nojq() {
  rc=0; payload "$2" "$R" | (cd "$R" && PATH="$d/nojq" "$d/nojq/sh" "$R/.claude/hooks/pr-merge-guard.sh") >/dev/null 2>&1 || rc=$?
  expect_rc "$1" "$rc" "merge-guard without jq: $3"
}
nojq 2 "gh pr merge 5 --squash" "blocks a merge"
nojq 0 "git status" "allows anything else"

# raw_command is duplicated on purpose (a sourced lib's missing-file failure is shell-dependent);
# the two copies must not drift.
a=$(sed -n '/^raw_command() {/,/^}/p' "$ROOT/.claude/hooks/pr-merge-guard.sh")
b=$(sed -n '/^raw_command() {/,/^}/p' "$ROOT/.claude/hooks/no-blind-source-rewrite.sh")
[ -n "$a" ] && [ "$a" = "$b" ] && ok "raw_command is identical in pr-merge-guard.sh and no-blind-source-rewrite.sh" || fail "raw_command differs between the two hooks"
finish
