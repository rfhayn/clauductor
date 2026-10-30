#!/bin/sh
# checks/run.sh runs the checks in parallel (OPS-175), and a check that shares state with another
# declares it (`# run.sh: serial (...)`) and runs alone. Held in fixture projects that carry this
# directory's run.sh and lib.sh:
#   1. a pair that shares a fixed path fails run in parallel without the declaration, the failing
#      one named, and passes with it;
#   2. a pair that can only pass side by side passes: the checks do run at the same time;
#   3. the report is in the checks' order, each check's lines under its own name, whichever ends
#      first;
#   4. a TERM to the run stops its running check, never starts a waiting one, and the run dies of
#      TERM with its scratch removed.
# Named runs, so a fixture run needs no no-stridal environment of its own.
. "$(dirname "$0")/lib.sh"
need git
d=$(scratch)
dir=$(cd "$(dirname "$0")" && pwd)

fx_project() {  # fx_project DIR: a project with this directory's run.sh and lib.sh, and no checks
  new_repo "$1"; mkdir -p "$1/.claude/checks" "$1/.claude/lib" "$1/tmp"
  cp "$dir/run.sh" "$dir/lib.sh" "$1/.claude/checks/"
  cp "$ROOT/.claude/lib/conf.sh" "$1/.claude/lib/"
}
# fx DIR [VAR=VALUE...] -- CHECK...: a named run in DIR, its output in DIR/out, its status returned.
fx() {
  fxd=$1; shift; _fe=""
  while [ "$1" != -- ]; do _fe="$_fe $1"; shift; done; shift
  # shellcheck disable=SC2086
  (cd "$fxd" && unmarked TMPDIR="$fxd/tmp" $_fe sh .claude/checks/run.sh "$@") > "$fxd/out" 2>&1
}

# ── 1. A shared fixed path ──────────────────────────────────────────────────────────────────────
# a holds $ROOT/lock until b has tried it; b tries only once a holds it. Side by side, b finds it
# held and fails; one after the other, a's wait runs out, a lets go, and b takes it.
P="$d/pair"; fx_project "$P"
cat > "$P/.claude/checks/a.sh" <<'EOF'
. "$(dirname "$0")/lib.sh"
mkdir "$ROOT/lock" && ok "a holds the lock" || fail "a found the lock held"
touch "$ROOT/a.in"
tried() { [ -e "$ROOT/b.tried" ]; }
wait_for "$(cat "$ROOT/patience")" tried
rmdir "$ROOT/lock"; rm -f "$ROOT/a.in" "$ROOT/b.tried"
finish
EOF
cat > "$P/.claude/checks/b.sh" <<'EOF'
. "$(dirname "$0")/lib.sh"
held() { [ -e "$ROOT/a.in" ]; }
wait_for "$(cat "$ROOT/patience")" held
mkdir "$ROOT/lock" && { ok "b holds the lock"; rmdir "$ROOT/lock"; } || fail "b found the lock held by a check running beside it"
touch "$ROOT/b.tried"
finish
EOF
# Side by side each waits for the other's step, so a long patience costs nothing; one after the
# other, a's wait must run out, so the declared run's is short.
echo 30 > "$P/patience"
fx "$P" CHECKS_JOBS=2 -- a b; expect_rc 1 $? "falsified: two checks sharing a fixed path, undeclared, fail run side by side"
grep -qx 'FAIL b' "$P/out" && grep -q '^    FAIL b found the lock held' "$P/out" \
  && ok "...and the run names the failing check and its line" || fail "the shared-state failure is not named: $(cat "$P/out")"
for c in a b; do
  printf '# run.sh: serial (the fixed path $ROOT/lock)\n' | cat - "$P/.claude/checks/$c.sh" > "$d/x" && mv "$d/x" "$P/.claude/checks/$c.sh"
done
rm -rf "$P/lock" "$P/a.in" "$P/b.tried"; echo 1 > "$P/patience"
fx "$P" CHECKS_JOBS=2 -- a b; expect_rc 0 $? "...and pass once both declare it: they run alone, one after the other"

# ── 2. Side by side ─────────────────────────────────────────────────────────────────────────────
# Each passes only if the other starts while it waits: a serial run fails both.
S="$d/side"; fx_project "$S"
for c in x y; do
  o=y; [ $c = x ] || o=x
  cat > "$S/.claude/checks/$c.sh" <<EOF
. "\$(dirname "\$0")/lib.sh"
touch "\$ROOT/$c.up"
up() { [ -e "\$ROOT/$o.up" ]; }
wait_for "\$(cat "\$ROOT/patience")" up && ok "$c saw $o running beside it" || fail "$c never saw $o running"
finish
EOF
done
echo 30 > "$S/patience"
fx "$S" CHECKS_JOBS=2 -- x y; expect_rc 0 $? "two checks run side by side under CHECKS_JOBS=2"
rm -f "$S"/*.up
printf '# run.sh: serial (falsifies the case above)\n' | cat - "$S/.claude/checks/x.sh" > "$d/x" && mv "$d/x" "$S/.claude/checks/x.sh"
echo 1 > "$S/patience"
fx "$S" CHECKS_JOBS=2 -- x y; expect_rc 1 $? "falsified: ...and fail when one runs alone"

# ── 3. Order ────────────────────────────────────────────────────────────────────────────────────
# slow ends after fast, and is printed first, with its own lines under it.
O="$d/order"; fx_project "$O"
cat > "$O/.claude/checks/slow.sh" <<'EOF'
. "$(dirname "$0")/lib.sh"
done_() { [ -e "$ROOT/fast.done" ]; }
wait_for 30 done_ && ok "slow ended after fast" || fail "slow: fast never ended"
fail "slow's own line"
finish
EOF
printf '. "$(dirname "$0")/lib.sh"\nfail "fast'"'"'s own line"\ntouch "$ROOT/fast.done"\nfinish\n' > "$O/.claude/checks/fast.sh"
fx "$O" CHECKS_JOBS=2 -- slow fast
want='FAIL slow
    FAIL slow'"'"'s own line
FAIL fast
    FAIL fast'"'"'s own line'
[ "$(grep -v '^checks:' "$O/out")" = "$want" ] && ok "the report keeps the checks' order and each check's lines under it, though the second ended first" \
  || fail "the report is out of order or interleaved: $(cat "$O/out")"

# ── 4. TERM ─────────────────────────────────────────────────────────────────────────────────────
# One job: a runs in the background, its 60 s child in ITS foreground (so a's own TERM trap waits
# for that child), and b waits for the slot. The sender waits for the child's marker, then TERMs
# the run alone; run.sh must pass it on to a and its child.
T="$d/term"; fx_project "$T"
# The child sleeps 60 s where run.sh can find it (ps runs), so a child that is never told finishes
# and says so; where ps cannot run, run.sh cannot find it, the child part is SKIPPED, and a short
# sleep keeps the check quick.
ps_ok=""; ps -A -o pid= >/dev/null 2>&1 && ps_ok=1
if [ -n "$ps_ok" ]; then echo 60 > "$T/patience"; else echo 1 > "$T/patience"; fi
cat > "$T/.claude/checks/a.sh" <<'EOF'
trap 'touch "$ROOT/a.stopped"; exit 1' TERM
echo $$ > "$ROOT/a.started"
sh -c 'echo $$ > "$1/child.pid"; sh -c '\''echo $$ > "$1/gc.pid"; exec sleep "$(cat "$1/patience")"'\'' gc "$1"; touch "$1/child.finished"' child "$ROOT"
touch "$ROOT/a.done"
EOF
printf 'touch "$ROOT/b.started"\n' > "$T/.claude/checks/b.sh"
(started() { [ -s "$T/gc.pid" ]; }; wait_for 30 started && kill -TERM "$(cat "$T/run.pid")") 2>/dev/null &
_sp=$!
(cd "$T" && unmarked TMPDIR="$T/tmp" CHECKS_JOBS=1 sh -c 'echo $$ > "$1"; exec sh .claude/checks/run.sh a b' term "$T/run.pid") > "$T/out" 2>&1; _rc=$?
kill "$_sp" 2>/dev/null; wait "$_sp" 2>/dev/null
# No wait here for the check and its child: the run waits for its checks to stop before it dies.
_cp=$(cat "$T/child.pid" 2>/dev/null)
if [ -z "$ps_ok" ]; then
  ok "SKIPPED the check's foreground child: ps cannot run here, so run.sh can tell only the check"
elif [ -f "$T/child.finished" ] || { [ -n "$_cp" ] && kill -0 "$_cp" 2>/dev/null; }; then
  fail "the check's foreground child (pid $_cp) was never told: it finished, or outlived the run"
  kill "$_cp" 2>/dev/null
else ok "the check's foreground child is stopped with it, never finishing"; fi
# ...and the whole tree: the child's own child (a sleep) too, not only the depths below the launch.
# The grandchild is orphaned when its parent dies first, so nothing in the run reaps it: wait for
# it to go, a zombie counting as gone (kill -0 reaches a zombie on Linux).
_gp=$(cat "$T/gc.pid" 2>/dev/null)
# kill -0 decides; ps and /proc only recognise a zombie, so a ps that cannot run reads as "still
# there" (a FAIL), never as gone.
gc_gone() {
  kill -0 "$_gp" 2>/dev/null || return 0
  case $(cat "/proc/$_gp/stat" 2>/dev/null) in *") Z "*) return 0 ;; esac
  case $(ps -o stat= -p "$_gp" 2>/dev/null) in Z*) return 0 ;; esac
  return 1
}
if [ -z "$ps_ok" ]; then :
elif [ -n "$_gp" ] && ! wait_for 5 gc_gone; then
  fail "the check's grandchild (pid $_gp) outlived the run: run.sh told only the top of the tree"
  kill "$_gp" 2>/dev/null
else ok "...and the child's own child, the bottom of the tree"; fi
[ "$_rc" -eq 143 ] && [ -f "$T/a.stopped" ] && [ ! -f "$T/a.done" ] \
  && ok "a TERM to the run reaches its background check, which stops, and the run dies of TERM (exit $_rc)" \
  || fail "TERM did not stop the background check and the run (exit $_rc, want 143; stopped: $([ -f "$T/a.stopped" ] && echo yes || echo no), finished: $([ -f "$T/a.done" ] && echo yes || echo no))"
[ ! -f "$T/b.started" ] && ok "...and the waiting check never starts" || fail "the waiting check started after the TERM"
[ -z "$(ls -A "$T/tmp")" ] && ok "...and the run's scratch is removed" || fail "TERM left $(ls -A "$T/tmp") behind"

finish
