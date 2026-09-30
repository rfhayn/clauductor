#!/bin/sh
# machine-quiet.sh never kills a tmux server, even an orphan whose argv names a worktree (the
# panel's server is exactly that: killing it took down every lane), while it does list an orphan
# of a known leak shape; and it removes no worktree when the live sessions cannot be read.
# Dry run only, in a throwaway repo, with claude, docker and colima off PATH (so the check never
# runs them), and each process it starts killed at the end.
. "$(dirname "$0")/lib.sh"
need git bash

d=$(scratch)
R="$d/repo"; new_repo "$R"
mkdir -p "$R/.claude/lib" "$R/.claude/worktrees"
cp "$ROOT/.claude/machine-quiet.sh" "$R/.claude/"
cp "$ROOT/.claude/lib/conf.sh" "$R/.claude/lib/"
echo '.claude/worktrees/' > "$R/.gitignore"
git -C "$R" add -A && git -C "$R" commit -qm init
git -C "$R" worktree add -q "$R/.claude/worktrees/lane" 2>/dev/null
WT="$R/.claude/worktrees"

# PATH without claude, clauductor, docker or colima.
np=""; IFS_OLD=$IFS; IFS=:
for p in $PATH; do
  [ -x "$p/claude" ] || [ -x "$p/clauductor" ] || [ -x "$p/docker" ] || [ -x "$p/colima" ] && continue
  np="$np${np:+:}$p"
done
IFS=$IFS_OLD

# Two orphans (parent pid 1: started from a subshell that exits at once), both naming a hook in a
# worktree: one whose argv[0] is tmux, shaped like the panel's server; one a plain hook process.
hookpath="$WT/lane/.claude/hooks/x.sh"
bash -c '(exec -a tmux sh -c "sleep 60; :" "-L" "sock" "new-session" "-c" "'"$hookpath"'" & echo $! > "'"$d"'/tmux.pid") &' 2>/dev/null; sleep 0.3
bash -c '(exec -a hook-loop sh -c "sleep 60; :" "'"$hookpath"'" & echo $! > "'"$d"'/hook.pid") &' 2>/dev/null; sleep 1
TP=$(cat "$d/tmux.pid" 2>/dev/null); HP=$(cat "$d/hook.pid" 2>/dev/null)
cleanup_procs() { pkill -P "$TP" 2>/dev/null; pkill -P "$HP" 2>/dev/null; kill "$TP" "$HP" 2>/dev/null; }

ps -o ppid= -p "$TP" 2>/dev/null | grep -qx ' *1' && ok "fixture: the tmux-shaped process is an orphan (ppid 1)" || fail "fixture: could not make the tmux-shaped process an orphan; this check learned nothing about it"
ps -o command= -p "$TP" 2>/dev/null | grep -q "^tmux .*$hookpath" && ok "fixture: its argv[0] is tmux and it names the worktree" || fail "fixture: tmux-shaped argv not as intended: $(ps -o command= -p "$TP" 2>/dev/null)"

out=$(cd "$R" && HOME="$d" PATH="$np" sh .claude/machine-quiet.sh --dry-run 2>&1); rc=$?
expect_rc 0 "$rc" "machine-quiet --dry-run exits 0"
case "$out" in *"orphan pid $TP:"*) fail "machine-quiet would kill the tmux server (pid $TP)" ;; *) ok "a tmux server whose argv names a worktree is never listed" ;; esac
case "$out" in *"orphan pid $HP:"*) ok "an orphan of the hook leak shape IS listed (so the tmux exclusion is what spared the other)" ;; *) fail "the hook-shaped orphan was not listed: $out" ;; esac
case "$out" in *"REMOVING NO WORKTREE"*) ok "with no way to read the live sessions, no worktree is removed (fail closed)" ;; *) fail "worktree removal did not fail closed without claude: $out" ;; esac
kill -0 "$TP" 2>/dev/null && ok "the dry run left the tmux-shaped process running" || fail "the tmux-shaped process is gone after a DRY run"
cleanup_procs
finish
