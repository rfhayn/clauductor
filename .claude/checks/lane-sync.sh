#!/bin/sh
# lane-sync.sh (OPS-151) runs outside the sandbox on a per-run approval, so what it refuses is the
# control: the main checkout first (a reset there would drop the owner's main), then a directory
# outside git, another repository's worktree, an unknown subcommand and an argument it does not
# list. Each refusal is shown leaving the tree as it was. Then the positive cases, in a lane
# worktree whose .claude/hooks lag origin/main: rebase, merge and reset bring the hook along, and
# update runs a stand-in clauductor in the worktree with only the arguments given.
. "$(dirname "$0")/lib.sh"
need git

d=$(scratch)
R="$d/repo"; new_repo "$R"
mkdir -p "$R/.claude/lib" "$R/.claude/hooks" "$d/bin" "$d/plain"
cp "$ROOT/.claude/lane-sync.sh" "$R/.claude/"
cp "$ROOT/.claude/lib/conf.sh" "$R/.claude/lib/"
echo '.claude/worktrees/' > "$R/.gitignore"
echo 'v1' > "$R/.claude/hooks/h.sh"
git -C "$R" add -A && git -C "$R" commit -qm base
old=$(git -C "$R" rev-parse HEAD)
git init -q --bare "$d/origin.git"
git -C "$R" remote add origin "$d/origin.git"
git -C "$R" push -q origin "HEAD:$MAIN_BRANCH" 2>/dev/null

# Two lanes cut from the old main, each with a commit of its own; then main moves the hook on.
for l in one two; do
  git -C "$R" worktree add -q -b "lane-$l" "$R/.claude/worktrees/$l" "$old" 2>/dev/null
  echo "$l" > "$R/.claude/worktrees/$l/$l.txt"
  git -C "$R/.claude/worktrees/$l" add -A && git -C "$R/.claude/worktrees/$l" commit -qm "lane $l"
done
WT="$R/.claude/worktrees"; W1="$WT/one"; W2="$WT/two"
echo 'v2' > "$R/.claude/hooks/h.sh"
git -C "$R" commit -qam 'hook v2' && git -C "$R" push -q origin "HEAD:$MAIN_BRANCH" 2>/dev/null
git -C "$R" fetch -q origin
# The main checkout then carries a commit origin has not got: a reset there would drop it.
echo 'mine' > "$R/owner.txt"; git -C "$R" add owner.txt && git -C "$R" commit -qm 'owner, unpushed'
mainhead=$(git -C "$R" rev-parse HEAD)

# A stand-in clauductor, first on PATH, records where it ran and with what: a refused update must
# leave no line, and the real binary never runs here.
cat > "$d/bin/clauductor" <<EOF
#!/bin/sh
echo "\$(pwd -P) \$*" >> "$d/update.log"
EOF
chmod +x "$d/bin/clauductor"
ls_sync() { (cd "$1" && shift && PATH="$d/bin:$PATH" sh "$@" 2>&1); }

# ── Refusals ──
out=$(ls_sync "$R" "$R/.claude/lane-sync.sh" reset); rc=$?
expect_rc 2 "$rc" "the main checkout is refused"
case $out in *"REFUSED"*"main checkout"*) ok "the refusal names the main checkout" ;; *) fail "the main checkout's refusal says: $out" ;; esac
[ "$(git -C "$R" rev-parse HEAD)" = "$mainhead" ] && [ -f "$R/owner.txt" ] &&
  ok "the refused reset left the main checkout's unpushed commit in place" ||
  fail "a reset ran in the main checkout: HEAD is $(git -C "$R" rev-parse --short HEAD), not ${mainhead%"${mainhead#???????}"}"
mkdir -p "$R/sub"
out=$(ls_sync "$R/sub" "$WT/one/.claude/lane-sync.sh" update); rc=$?
expect_rc 2 "$rc" "the main checkout is refused from a subdirectory, by a lane's copy of the script"
[ ! -f "$d/update.log" ] || fail "update ran in the main checkout: $(cat "$d/update.log")"

out=$(ls_sync "$d/plain" "$R/.claude/lane-sync.sh" reset); rc=$?
expect_rc 2 "$rc" "a directory outside git is refused"

# Another repository with the model and an origin/<main>, so only the refusal stops a reset there.
O="$d/other"; new_repo "$O"; mkdir -p "$O/.claude/lib"; cp "$ROOT/.claude/lib/conf.sh" "$O/.claude/lib/"
git -C "$O" add -A && git -C "$O" commit -qm x
git init -q --bare "$d/other.git"; git -C "$O" remote add origin "$d/other.git"
git -C "$O" push -q origin "HEAD:$MAIN_BRANCH" 2>/dev/null; git -C "$O" fetch -q origin
git -C "$O" worktree add -q -b lane "$d/other-lane" 2>/dev/null
echo y > "$d/other-lane/y"; git -C "$d/other-lane" add y && git -C "$d/other-lane" commit -qm y
ohead=$(git -C "$d/other-lane" rev-parse HEAD)
out=$(ls_sync "$d/other-lane" "$R/.claude/lane-sync.sh" reset); rc=$?
expect_rc 2 "$rc" "another repository's linked worktree is refused"
case $out in *"another repository"*) ok "the refusal names the other repository" ;; *) fail "the other repository's refusal says: $out" ;; esac
[ "$(git -C "$d/other-lane" rev-parse HEAD)" = "$ohead" ] && ok "the other repository's lane kept its commit" ||
  fail "a reset ran in another repository's worktree"

# A copy outside any .claude/ (the plugin's, which serves every project) still refuses a main checkout.
mkdir -p "$d/fw"; cp "$R/.claude/lane-sync.sh" "$d/fw/"
out=$(ls_sync "$R" "$d/fw/lane-sync.sh" reset); rc=$?
expect_rc 2 "$rc" "the plugin-shaped copy refuses the main checkout too"
[ "$(git -C "$R" rev-parse HEAD)" = "$mainhead" ] || fail "the plugin-shaped copy reset the main checkout"

w1head=$(git -C "$W1" rev-parse HEAD)
for args in "" "fetch" "push origin HEAD" "reset --hard HEAD~1" "rebase -i origin/main" "merge origin/other" "update --force" "update --template-dir /tmp"; do
  # shellcheck disable=SC2086
  out=$(ls_sync "$W1" "$WT/one/.claude/lane-sync.sh" $args); rc=$?
  expect_rc 2 "$rc" "'lane-sync.sh $args' is refused"
done
[ "$(git -C "$W1" rev-parse HEAD)" = "$w1head" ] && [ ! -f "$d/update.log" ] &&
  ok "no refused command moved the lane or ran update" || fail "a refused command changed the lane"

# ── A lane worktree ──
out=$(ls_sync "$W1" "$WT/one/.claude/lane-sync.sh" rebase); rc=$?
expect_rc 0 "$rc" "rebase runs in a lane worktree"
[ "$(cat "$W1/.claude/hooks/h.sh")" = v2 ] && [ -f "$W1/one.txt" ] &&
  [ "$(git -C "$W1" rev-parse HEAD~1)" = "$(git -C "$R" rev-parse origin/"$MAIN_BRANCH")" ] &&
  ok "rebase put the lane's commit on origin/$MAIN_BRANCH, hook included" || fail "rebase left: $out"

out=$(ls_sync "$W2" "$R/.claude/lane-sync.sh" merge); rc=$?
expect_rc 0 "$rc" "merge runs in a lane worktree"
[ "$(cat "$W2/.claude/hooks/h.sh")" = v2 ] && [ -f "$W2/two.txt" ] &&
  git -C "$W2" merge-base --is-ancestor "origin/$MAIN_BRANCH" HEAD &&
  ok "merge brought origin/$MAIN_BRANCH's hook into the lane, keeping its commit" || fail "merge left: $out"

echo dirty >> "$W1/one.txt"
out=$(ls_sync "$W1" "$WT/one/.claude/lane-sync.sh" reset); rc=$?
expect_rc 0 "$rc" "reset runs in a lane worktree"
[ "$(git -C "$W1" rev-parse HEAD)" = "$(git -C "$R" rev-parse "origin/$MAIN_BRANCH")" ] && [ ! -f "$W1/one.txt" ] &&
  ok "reset put the lane on origin/$MAIN_BRANCH" || fail "reset left HEAD at $(git -C "$W1" rev-parse --short HEAD)"
case $out in *"dropping commits"*"lane one"*"discarding uncommitted"*) ok "reset said what it dropped before it dropped it" ;; *) fail "reset did not list what it dropped: $out" ;; esac

# update, in a lane
out=$(ls_sync "$W2" "$WT/two/.claude/lane-sync.sh" update --dry-run --create-missing); rc=$?
expect_rc 0 "$rc" "update runs in a lane worktree"
w2=$(cd "$W2" && pwd -P)
[ "$(cat "$d/update.log" 2>/dev/null)" = "$w2 update --dry-run --create-missing" ] &&
  ok "update ran clauductor update in the lane's worktree with exactly the arguments given" ||
  fail "update ran: $(cat "$d/update.log" 2>/dev/null)"
finish
