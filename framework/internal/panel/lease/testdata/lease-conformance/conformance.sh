#!/usr/bin/env bash
# Conformance suite for the clauductor lease protocol (docs/panel.md, "Queue and the
# gate lock protocol"). Any implementation of the protocol runs it.
#
# THE INTERFACE. An implementation is a command that waits its turn for a lease,
# runs a command while holding it, releases it, and exits with the command's status
# (75 if its wait was cancelled). The driver hands it the lock in one of two ways:
#
#   conformance.sh <impl> [impl-args...]
#       runs  <impl> [impl-args...] <lockdir> <lane> <command> [args...]
#
#   conformance.sh --lock-env VAR <impl> [impl-args...]
#       runs  VAR=<lockdir> CLAUDUCTOR_LANE=<lane> <impl> [impl-args...] <command> [args...]
#       for an implementation that takes its lock path from the environment, such as
#       a project's gate wrapper whose lock is normally <git common dir>/..., so it can
#       run the suite with no adapter (make the path overridable by VAR).
#
# Reference adapters for the first form:
#
#   clauductor lock-run:  #!/bin/sh
#                         lock=$1 lane=$2; shift 2
#                         exec clauductor lock-run --lane "$lane" "$lock" -- "$@"
#   the docs' lease.sh:   #!/usr/bin/env bash
#                         set -euo pipefail; . /path/to/lease.sh; lease_run "$@"
#
# The command must see CLAUDUCTOR_LOCK_HELD equal to the lock path EXACTLY as it was
# given (cleaned, never symlink-resolved): a gate script compares the two to detect
# re-entry, and a resolved path would make it queue behind itself. The
# symlinked-lock case checks it.
#
# Each case sets up a lock from the golden records in cases/<case>/ (owner.json, and
# waiters/*.json), with {{PLACEHOLDERS}} filled in from real processes; a placeholder
# left unfilled fails the case. It checks only what every implementation must do:
# whether and when the command runs, its exit status, and the files left behind. It
# prints TAP and exits 1 on a failure.
#
#   CASES="dead-pid cancel" conformance.sh ...   run some cases
#   conformance.sh --list                        print the case names
#   CONFORMANCE_WAIT=3                           seconds a waiter must keep waiting
#   CONFORMANCE_TIMEOUT=20                       seconds a runnable command may take to run
#
# Needs bash, ps, sed, awk, hostname, and a writable $TMPDIR. Run it on one host.
set -u

ALL_CASES="live-holder dead-pid pid-reuse proc-format no-ps pstart-no-ps no-ps-foreign-pid
other-host-expired other-host-live other-host-no-ttl missing-pid missing-pid-expired missing-host
child-alive child-dead child-reused ownerless-old ownerless-young truncated-owner-old
truncated-owner-young bad-nonce-owner-old live-waiter-ahead dead-waiter-ahead
other-host-waiter-stale other-host-waiter-fresh malformed-waiters cancel reclaim-race
owner-record symlinked-lock"

if [ "${1:-}" = --list ]; then
	printf '%s\n' $ALL_CASES
	exit 0
fi
LOCK_ENV=""
if [ "${1:-}" = --lock-env ]; then
	LOCK_ENV=${2:-}
	shift 2 || true
	if ! printf %s "$LOCK_ENV" | grep -Eq '^[A-Za-z_][A-Za-z0-9_]*$'; then
		echo "conformance.sh: --lock-env needs a variable name" >&2
		exit 2
	fi
fi
if [ $# -lt 1 ]; then
	echo "usage: conformance.sh [--lock-env VAR] <impl> [impl-args...]   (or --list)" >&2
	exit 2
fi
IMPL=("$@")
HERE=$(cd "$(dirname "$0")" && pwd)
WAIT=${CONFORMANCE_WAIT:-3}
TIMEOUT=${CONFORMANCE_TIMEOUT:-20}
HOST=$(hostname)
tmp=${TMPDIR:-/tmp}
# A clean path: implementations may clean the lock path they report (a/b, not a//b).
WORK=$(mktemp -d "${tmp%/}/lease-conformance.XXXXXX")
cleanup() {
	# Only this shell's own jobs that are still running: a pid it already reaped
	# may belong to someone else by now.
	for p in $(jobs -p); do kill -9 "$p" 2>/dev/null; done
	wait 2>/dev/null
	rm -rf "$WORK"
}
trap cleanup EXIT

pstart() { LC_ALL=C ps -o lstart= -p "$1" 2>/dev/null | awk '{$1=$1; print}'; }

# sleeper: a live process of this host, reaped by the driver when it is killed (an
# unreaped child is a zombie, and a zombie still answers kill -0).
sleeper() {
	sleep 600 &
	SLEEPER=$!
}
kill_reap() { kill "$1" 2>/dev/null; wait "$1" 2>/dev/null; }

deadpid() {
	local p
	p=$(sh -c 'echo $$')
	while kill -0 "$p" 2>/dev/null; do p=$(sh -c 'echo $$'); done
	echo "$p"
}

# ago SECONDS: a touch -t stamp that many seconds in the past.
ago() {
	local t=$(($(date +%s) - $1))
	date -d "@$t" +%Y%m%d%H%M.%S 2>/dev/null || date -r "$t" +%Y%m%d%H%M.%S
}

FAIL=""
fail() { [ -n "$FAIL" ] || FAIL="$*"; }

# render TEMPLATE DEST KEY=VALUE...: fill {{KEY}} placeholders. One left unfilled is
# a broken case, never a record an implementation may be judged on.
render() {
	local src=$1 dst=$2 expr=() kv
	shift 2
	for kv in "$@"; do expr+=(-e "s|{{${kv%%=*}}}|${kv#*=}|g"); done
	mkdir -p "$(dirname "$dst")"
	sed "${expr[@]}" "$src" >"$dst"
	if grep -q '{{' "$dst"; then
		fail "unfilled placeholder in $(basename "$src"): $(grep -o '{{[A-Z_]*}}' "$dst" | head -n 1)"
	fi
}

# setup CASE KEY=VALUE...: a fresh lock from the case's golden records.
setup() {
	local c=$1 now
	shift
	now=$(date +%s)
	CASE_DIR="$WORK/$c"
	LOCK=${LOCK_PATH:-"$CASE_DIR/gate.lock"}
	LOG="$CASE_DIR/log"
	mkdir -p "$CASE_DIR"
	: >"$LOG"
	export LOG
	if [ -f "$HERE/cases/$c/owner.json" ]; then
		mkdir -p "$LOCK"
		render "$HERE/cases/$c/owner.json" "$LOCK/owner.json" HOST="$HOST" NOW="$now" AGO="$((now - 100))" "$@"
	fi
	if [ -d "$HERE/cases/$c/waiters" ]; then
		for f in "$HERE/cases/$c/waiters/"*.json; do
			render "$f" "$LOCK.waiters/$(basename "$f")" HOST="$HOST" NOW="$now" AGO="$((now - 100))" "$@"
		done
	fi
}

# start NAME BODY: run the implementation as lane NAME, with command `sh -c BODY`.
start() {
	if [ -n "$LOCK_ENV" ]; then
		env "$LOCK_ENV=$LOCK" CLAUDUCTOR_LANE="$1" "${IMPL[@]}" sh -c "$2" 2>"$CASE_DIR/$1.err" &
	else
		"${IMPL[@]}" "$LOCK" "$1" sh -c "$2" 2>"$CASE_DIR/$1.err" &
	fi
	eval "PID_$1=$!"
}
pid_of() { eval "echo \$PID_$1"; }

# no_ps_path: a PATH with everything on this PATH except ps.
no_ps_path() {
	local farm="$WORK/no-ps-bin" d
	if [ ! -d "$farm" ]; then
		mkdir -p "$farm"
		local IFS=:
		for d in $PATH; do
			[ -d "$d" ] && ln -s "$d"/* "$farm"/ 2>/dev/null
		done
		unset IFS
		rm -f "$farm/ps"
	fi
	echo "$farm"
}
# start_no_ps NAME BODY: start, with no ps on the implementation's PATH.
start_no_ps() {
	local old=$PATH
	PATH=$(no_ps_path)
	start "$@"
	PATH=$old
}

# finish NAME: wait for it to exit (up to TIMEOUT) and set RC.
finish() {
	local p i=0
	p=$(pid_of "$1")
	while kill -0 "$p" 2>/dev/null && [ $i -lt $((TIMEOUT * 10)) ]; do
		sleep 0.1
		i=$((i + 1))
	done
	if kill -0 "$p" 2>/dev/null; then
		RC=timeout
		kill -9 "$p" 2>/dev/null
		wait "$p" 2>/dev/null
		return
	fi
	wait "$p"
	RC=$?
}

log_is() { [ "$(tr '\n' ' ' <"$LOG" | sed 's/ $//')" = "$1" ] || fail "log is '$(tr '\n' ' ' <"$LOG")', want '$1'"; }

# still_waiting NAME: for WAIT seconds, NAME stays alive and runs nothing.
still_waiting() {
	local p i=0
	p=$(pid_of "$1")
	while [ $i -lt $((WAIT * 10)) ]; do
		if ! kill -0 "$p" 2>/dev/null; then
			fail "$1 exited while it had to wait ($(tail -n 1 "$CASE_DIR/$1.err" 2>/dev/null))"
			return
		fi
		if [ -s "$LOG" ]; then
			fail "$1 ran its command while it had to wait"
			return
		fi
		sleep 0.1
		i=$((i + 1))
	done
}

# runs NAME: NAME runs its command, exits 0 and releases the lease.
runs() {
	finish "$1"
	[ "$RC" = 0 ] || fail "$1 exit $RC, want 0 ($(tail -n 1 "$CASE_DIR/$1.err" 2>/dev/null))"
	log_is "$1"
	[ ! -e "$LOCK" ] || fail "the lease was not released"
}

# waits_then_stop NAME: NAME keeps waiting; then the driver stops it.
waits_then_stop() {
	still_waiting "$1"
	kill_reap "$(pid_of "$1")"
}

holder_untouched() {
	grep -q '"nonce":"c0ffee0000000001"' "$LOCK/owner.json" 2>/dev/null || fail "a live holder's owner.json was removed or changed"
}

# A live holder on this host (pid alive, start time matches) is never stale, however
# long it is silent; it holds until its process ends.
case_live_holder() {
	sleeper
	setup live-holder PID="$SLEEPER" PSTART="$(pstart "$SLEEPER")"
	start a 'echo a >> "$LOG"'
	still_waiting a
	holder_untouched
	# The waiter's own record: <arrival>-<nonce>.json, naming the waiter.
	local w
	w=$(ls "$LOCK.waiters" 2>/dev/null | grep -E '^[0-9]+-[0-9a-f]{16}\.json$' | head -n 1)
	if [ -z "$w" ]; then
		fail "no <arrival>-<nonce>.json waiter file"
	else
		local nonce=${w#*-}
		nonce=${nonce%.json}
		grep -q "\"nonce\":\"$nonce\"" "$LOCK.waiters/$w" || fail "waiter file $w does not carry its nonce"
		grep -q "\"host\":\"$HOST\"" "$LOCK.waiters/$w" || fail "waiter file $w does not name this host"
	fi
	kill_reap "$SLEEPER"
	runs a
}

# The holder's pid is gone: stale, reclaimed.
case_dead_pid() {
	setup dead-pid DEAD="$(deadpid)"
	start a 'echo a >> "$LOG"'
	runs a
}

# The pid is alive but started at another time: the pid was reused. Stale.
case_pid_reuse() {
	sleeper
	setup pid-reuse PID="$SLEEPER"
	start a 'echo a >> "$LOG"'
	runs a
}

# A start time from /proc is never compared with one from ps: the alive pid cannot be
# verified, so it is live.
case_proc_format() {
	if ! command -v ps >/dev/null 2>&1; then
		SKIP="no ps on this host"
		return
	fi
	sleeper
	setup proc-format PID="$SLEEPER"
	start a 'echo a >> "$LOG"'
	still_waiting a
	holder_untouched
	kill_reap "$SLEEPER"
	runs a
}

# No ps on PATH, and a record with no start time: liveness is kill -0. An alive pid
# is live; once it is gone the lease is reclaimed.
case_no_ps() {
	sleeper
	setup no-ps PID="$SLEEPER"
	start_no_ps a 'echo a >> "$LOG"'
	still_waiting a
	holder_untouched
	kill_reap "$SLEEPER"
	runs a
}

# No ps on PATH, and a record WITH a start time from ps: the start time cannot be
# read (or only from /proc, another source), so the alive pid is live, never reused.
case_pstart_no_ps() {
	sleeper
	setup pstart-no-ps PID="$SLEEPER" PSTART="$(pstart "$SLEEPER")"
	start_no_ps a 'echo a >> "$LOG"'
	still_waiting a
	holder_untouched
	kill_reap "$SLEEPER"
	runs a
}

# No ps, and a holder that is somebody else's process (pid 1): kill -0 answers
# EPERM, which still means alive.
case_no_ps_foreign_pid() {
	if kill -0 1 2>/dev/null; then
		SKIP="running as root: kill -0 1 succeeds, so there is no EPERM to test"
		return
	fi
	setup no-ps-foreign-pid
	start_no_ps a 'echo a >> "$LOG"'
	waits_then_stop a
	holder_untouched
}

# Another host's pids mean nothing here: its lease expires at renewed + ttl.
case_other_host_expired() {
	setup other-host-expired
	start a 'echo a >> "$LOG"'
	runs a
}

case_other_host_live() {
	setup other-host-live
	start a 'echo a >> "$LOG"'
	waits_then_stop a
	holder_untouched
}

# ttl 0 is no expiry: another host's holder with ttl 0 is never reclaimed by time.
case_other_host_no_ttl() {
	setup other-host-no-ttl
	start a 'echo a >> "$LOG"'
	waits_then_stop a
	holder_untouched
}

# A record with no pid has no process to judge, so only its TTL can expire it.
case_missing_pid() {
	setup missing-pid
	start a 'echo a >> "$LOG"'
	waits_then_stop a
	holder_untouched
}

case_missing_pid_expired() {
	setup missing-pid-expired
	start a 'echo a >> "$LOG"'
	runs a
}

# A record with no host is not this host's: its (dead) pid means nothing, and ttl 0
# never expires.
case_missing_host() {
	setup missing-host DEAD="$(deadpid)"
	start a 'echo a >> "$LOG"'
	waits_then_stop a
	holder_untouched
}

# The holder died but its command (child_pid) still runs: the lease is still held.
case_child_alive() {
	sleeper
	setup child-alive DEAD="$(deadpid)" CHILD_PID="$SLEEPER" CHILD_PSTART="$(pstart "$SLEEPER")"
	start a 'echo a >> "$LOG"'
	still_waiting a
	holder_untouched
	kill_reap "$SLEEPER"
	runs a
}

case_child_dead() {
	setup child-dead DEAD="$(deadpid)" CHILD_DEAD="$(deadpid)"
	start a 'echo a >> "$LOG"'
	runs a
}

# The holder died and its child_pid is alive with another start time: reused, so
# both are dead.
case_child_reused() {
	sleeper
	setup child-reused DEAD="$(deadpid)" CHILD_PID="$SLEEPER"
	start a 'echo a >> "$LOG"'
	runs a
}

# A lock directory with no owner.json, older than 10 s: its holder died between mkdir
# and the write. Stale.
case_ownerless_old() {
	setup ownerless-old
	mkdir -p "$LOCK"
	touch -t "$(ago 60)" "$LOCK"
	start a 'echo a >> "$LOG"'
	runs a
}

# Younger than 10 s, its holder is starting: wait. Once it is 10 s old, reclaim.
case_ownerless_young() {
	setup ownerless-young
	mkdir -p "$LOCK"
	start a 'echo a >> "$LOG"'
	still_waiting a
	runs a
}

# A truncated owner.json is not a valid record: as good as none.
case_truncated_owner_old() {
	sleeper
	setup truncated-owner-old PID="$SLEEPER"
	touch -t "$(ago 60)" "$LOCK"
	start a 'echo a >> "$LOG"'
	runs a
}

case_truncated_owner_young() {
	sleeper
	setup truncated-owner-young PID="$SLEEPER"
	start a 'echo a >> "$LOG"'
	still_waiting a
	runs a
}

# Complete JSON with a nonce that is not 16 hex digits is not a valid record either,
# even with a live pid in it.
case_bad_nonce_owner_old() {
	sleeper
	setup bad-nonce-owner-old PID="$SLEEPER" PSTART="$(pstart "$SLEEPER")"
	touch -t "$(ago 60)" "$LOCK"
	start a 'echo a >> "$LOG"'
	runs a
}

# FIFO: a live waiter that arrived first goes first; a newcomer never jumps it.
case_live_waiter_ahead() {
	sleeper
	setup live-waiter-ahead PID="$SLEEPER" PSTART="$(pstart "$SLEEPER")"
	start a 'echo a >> "$LOG"'
	still_waiting a
	[ -f "$LOCK.waiters/1-c0ffee00000000aa.json" ] || fail "a live waiter's file was removed"
	kill_reap "$SLEEPER"
	runs a
	[ ! -e "$LOCK.waiters/1-c0ffee00000000aa.json" ] || fail "a dead waiter's file was left behind"
}

# A waiter whose pid is gone is skipped and its file removed.
case_dead_waiter_ahead() {
	setup dead-waiter-ahead DEAD="$(deadpid)"
	start a 'echo a >> "$LOG"'
	runs a
	[ ! -e "$LOCK.waiters/1-c0ffee00000000bb.json" ] || fail "a dead waiter's file was left behind"
}

# Another host's waiter is judged by a 60 s TTL, whatever ttl its file names.
case_other_host_waiter_stale() {
	setup other-host-waiter-stale
	start a 'echo a >> "$LOG"'
	runs a
	[ ! -e "$LOCK.waiters/1-c0ffee00000000dd.json" ] || fail "a dead waiter's file was left behind"
}

case_other_host_waiter_fresh() {
	setup other-host-waiter-fresh
	start a 'echo a >> "$LOG"'
	waits_then_stop a
	[ -f "$LOCK.waiters/1-c0ffee00000000ee.json" ] || fail "a live waiter's file was removed"
}

# Files in the waiters directory that are not valid records are not waiters: they
# hold no place, and nobody removes them.
case_malformed_waiters() {
	sleeper
	setup malformed-waiters PID="$SLEEPER" PSTART="$(pstart "$SLEEPER")"
	start a 'echo a >> "$LOG"'
	runs a
	for f in 1-garbage.json 2-c0ffee00000000cc.json 3-badnonce.json; do
		[ -f "$LOCK.waiters/$f" ] || fail "removed $f, which is not a waiter record to judge"
	done
}

# <lock>.waiters/<nonce>.cancel makes that waiter give up: exit 75, nothing run, its
# files removed. The holder is untouched.
case_cancel() {
	sleeper
	setup cancel PID="$SLEEPER" PSTART="$(pstart "$SLEEPER")"
	start a 'echo a >> "$LOG"'
	local w="" i=0
	while [ -z "$w" ] && [ $i -lt $((TIMEOUT * 10)) ]; do
		w=$(ls "$LOCK.waiters" 2>/dev/null | grep -E '^[0-9]+-[0-9a-f]{16}\.json$' | head -n 1)
		sleep 0.1
		i=$((i + 1))
	done
	if [ -z "$w" ]; then
		fail "no waiter file appeared"
		return
	fi
	local nonce=${w#*-}
	nonce=${nonce%.json}
	: >"$LOCK.waiters/$nonce.cancel"
	finish a
	[ "$RC" = 75 ] || fail "exit $RC after a cancel, want 75"
	[ ! -s "$LOG" ] || fail "a cancelled waiter ran its command"
	[ ! -e "$LOCK.waiters/$w" ] || fail "a cancelled waiter left its waiter file"
	[ ! -e "$LOCK.waiters/$nonce.cancel" ] || fail "a cancelled waiter left its cancel file"
	holder_untouched
}

# Three waiters meet one dead holder at once: it is reclaimed, and they then run one
# at a time, each exactly once.
case_reclaim_race() {
	setup reclaim-race DEAD="$(deadpid)"
	local n
	for n in a b c; do start "$n" "echo $n-start >> \"\$LOG\"; sleep 1; echo $n-end >> \"\$LOG\""; done
	for n in a b c; do
		finish "$n"
		[ "$RC" = 0 ] || fail "$n exit $RC ($(tail -n 1 "$CASE_DIR/$n.err" 2>/dev/null))"
	done
	local lines prev=""
	lines=$(tr '\n' ' ' <"$LOG")
	[ "$(wc -l <"$LOG" | tr -d ' ')" = 6 ] || fail "log '$lines': want each of three commands started and ended once"
	while read -r l; do
		if [ -z "$prev" ]; then
			prev=$l
		else
			[ "${prev%-start}-end" = "$l" ] || fail "commands overlapped: '$lines'"
			prev=""
		fi
	done <"$LOG"
	[ ! -e "$LOCK" ] || fail "the lease was not released"
}

# owner_record_check SNAP: the holder's own record, read while its command runs.
owner_record_check() {
	local snap=$1 o pid re
	o=$(cat "$snap")
	for re in '"v":1' '"nonce":"[0-9a-f]{16}"' '"pid":[0-9]+' '"pstart":"[^"]+"' "\"host\":\"$HOST\"" '"lane":"a"' '"started":[0-9]+' '"renewed":[0-9]+' '"ttl":[0-9]+'; do
		printf %s "$o" | grep -Eq "$re" || fail "owner.json lacks $re: $o"
	done
	pid=$(printf %s "$o" | sed -n 's/.*"pid":\([0-9]*\).*/\1/p')
	if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
		local want
		want=$(pstart "$pid")
		printf %s "$o" | grep -Fq "\"pstart\":\"$want\"" || fail "pstart is not the holder's start time as ps prints it ('$want'): $o"
	else
		fail "owner.json's pid $pid is not alive while its command runs"
	fi
}

# wait_for FILE: until it has content, or TIMEOUT.
wait_for() {
	local i=0
	while [ ! -s "$1" ] && [ $i -lt $((TIMEOUT * 10)) ]; do
		sleep 0.1
		i=$((i + 1))
	done
}

# Every field of the holder's record, a pstart as ps prints it, the command's
# environment, and its exit status.
case_owner_record() {
	setup owner-record
	export SNAP="$CASE_DIR/snap"
	start a 'cp "$CLAUDUCTOR_LOCK_HELD/owner.json" "$SNAP.tmp"; mv "$SNAP.tmp" "$SNAP"; printf %s "$CLAUDUCTOR_LOCK_HELD" > "$SNAP.held"; sleep 2; exit 3'
	wait_for "$SNAP"
	if [ ! -s "$SNAP" ]; then
		fail "the command never ran, or CLAUDUCTOR_LOCK_HELD is not the lock"
	else
		owner_record_check "$SNAP"
		[ "$(cat "$SNAP.held")" = "$LOCK" ] || fail "CLAUDUCTOR_LOCK_HELD is '$(cat "$SNAP.held")', want $LOCK"
	fi
	finish a
	[ "$RC" = 3 ] || fail "exit $RC, want the command's 3"
	[ ! -e "$LOCK" ] || fail "the lease was not released"
}

# A lock path through a symlinked directory: CLAUDUCTOR_LOCK_HELD is the path as
# given, not resolved, or a gate script's re-entry check never matches.
case_symlinked_lock() {
	mkdir -p "$WORK/symlinked-lock/real"
	ln -s real "$WORK/symlinked-lock/link"
	LOCK_PATH="$WORK/symlinked-lock/link/gate.lock" setup symlinked-lock
	export SNAP="$CASE_DIR/snap"
	start a 'printf %s "$CLAUDUCTOR_LOCK_HELD" > "$SNAP"'
	finish a
	[ "$RC" = 0 ] || fail "exit $RC ($(tail -n 1 "$CASE_DIR/a.err" 2>/dev/null))"
	[ "$(cat "$SNAP" 2>/dev/null)" = "$LOCK" ] || fail "CLAUDUCTOR_LOCK_HELD is '$(cat "$SNAP" 2>/dev/null)', want the path as given: $LOCK"
	[ ! -e "$LOCK" ] || fail "the lease was not released"
}

selected=${CASES:-$ALL_CASES}
set -- $selected
echo "1..$#"
n=0
failed=0
for c in "$@"; do
	n=$((n + 1))
	FAIL=""
	SKIP=""
	fn="case_$(echo "$c" | tr - _)"
	if ! declare -F "$fn" >/dev/null; then
		echo "not ok $n - $c: no such case"
		failed=1
		continue
	fi
	"$fn"
	if [ -n "$SKIP" ]; then
		echo "ok $n - $c # SKIP $SKIP"
	elif [ -n "$FAIL" ]; then
		echo "not ok $n - $c: $FAIL"
		failed=1
	else
		echo "ok $n - $c"
	fi
done
exit $failed
