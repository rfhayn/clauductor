#!/usr/bin/env bash
# Conformance suite for the clauductor lease protocol (docs/panel.md, "Queue and the
# gate lock protocol"). Any implementation of the protocol runs it:
#
#   conformance.sh <impl> [impl-args...]
#
# <impl> is a command that runs `<impl> [impl-args...] <lockdir> <lane> <command>
# [args...]`: wait its turn for the lease <lockdir>, run the command holding it, and
# exit with the command's status (75 if its wait was cancelled). Adapters:
#
#   clauductor lock-run:  #!/bin/sh
#                         lock=$1 lane=$2; shift 2
#                         exec clauductor lock-run --lane "$lane" "$lock" -- "$@"
#   the docs' lease.sh:   #!/usr/bin/env bash
#                         set -euo pipefail; . /path/to/lease.sh; lease_run "$@"
#
# Each case sets up a lock from the golden records in cases/<case>/ (owner.json, and
# waiters/*.json), with {{PLACEHOLDERS}} filled in from real processes, and checks
# only what every implementation must do: whether and when the command runs, its
# exit status, and the files left behind. It prints TAP and exits 1 on a failure.
#
#   CASES="dead-pid cancel" conformance.sh ...   run some cases
#   conformance.sh --list                        print the case names
#   CONFORMANCE_WAIT=3                           seconds a waiter must keep waiting
#   CONFORMANCE_TIMEOUT=20                       seconds a runnable command may take to run
#
# Needs bash, ps, sed, awk, hostname, and a writable $TMPDIR. Run it on one host.
set -u

ALL_CASES="live-holder dead-pid pid-reuse proc-format no-ps other-host-expired other-host-live
other-host-no-ttl child-alive child-dead ownerless-old live-waiter-ahead dead-waiter-ahead
cancel reclaim-race owner-record"

if [ "${1:-}" = --list ]; then
	printf '%s\n' $ALL_CASES
	exit 0
fi
if [ $# -lt 1 ]; then
	echo "usage: conformance.sh <impl> [impl-args...]   (or --list)" >&2
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

# render TEMPLATE DEST KEY=VALUE...: fill {{KEY}} placeholders.
render() {
	local src=$1 dst=$2 expr=() kv
	shift 2
	for kv in "$@"; do expr+=(-e "s|{{${kv%%=*}}}|${kv#*=}|g"); done
	mkdir -p "$(dirname "$dst")"
	sed "${expr[@]}" "$src" >"$dst"
}

# setup CASE KEY=VALUE...: a fresh lock from the case's golden records.
setup() {
	local c=$1 now
	shift
	now=$(date +%s)
	CASE_DIR="$WORK/$c"
	LOCK="$CASE_DIR/gate.lock"
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
	"${IMPL[@]}" "$LOCK" "$1" sh -c "$2" 2>"$CASE_DIR/$1.err" &
	eval "PID_$1=$!"
}
pid_of() { eval "echo \$PID_$1"; }

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

FAIL=""
fail() { [ -n "$FAIL" ] || FAIL="$*"; }
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
	local farm="$WORK/no-ps-bin" d
	mkdir -p "$farm"
	local IFS=:
	for d in $PATH; do
		[ -d "$d" ] && ln -s "$d"/* "$farm"/ 2>/dev/null
	done
	unset IFS
	rm -f "$farm/ps"
	sleeper
	setup no-ps PID="$SLEEPER"
	local OLDPATH=$PATH
	PATH=$farm
	start a 'echo a >> "$LOG"'
	PATH=$OLDPATH
	still_waiting a
	holder_untouched
	kill_reap "$SLEEPER"
	runs a
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
	still_waiting a
	holder_untouched
	kill_reap "$(pid_of a)"
}

# ttl 0 is no expiry: another host's holder with ttl 0 is never reclaimed by time.
case_other_host_no_ttl() {
	setup other-host-no-ttl
	start a 'echo a >> "$LOG"'
	still_waiting a
	holder_untouched
	kill_reap "$(pid_of a)"
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

# A lock directory with no owner.json, older than 10 s: its holder died between mkdir
# and the write. Stale.
case_ownerless_old() {
	setup ownerless-old
	mkdir -p "$LOCK"
	local t=$(($(date +%s) - 60)) stamp
	stamp=$(date -d "@$t" +%Y%m%d%H%M.%S 2>/dev/null || date -r "$t" +%Y%m%d%H%M.%S)
	touch -t "$stamp" "$LOCK"
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

# The holder's own record, read while its command runs: every field the protocol
# names, a pstart as ps prints it, the command's environment, and its exit status.
case_owner_record() {
	setup owner-record
	local snap="$CASE_DIR/snap"
	export SNAP=$snap
	start a 'cp "$CLAUDUCTOR_LOCK_HELD/owner.json" "$SNAP.tmp"; mv "$SNAP.tmp" "$SNAP"; printf %s "$CLAUDUCTOR_LOCK_HELD" > "$SNAP.held"; sleep 2; exit 3'
	local i=0
	while [ ! -s "$snap" ] && [ $i -lt $((TIMEOUT * 10)) ]; do
		sleep 0.1
		i=$((i + 1))
	done
	if [ ! -s "$snap" ]; then
		fail "the command never ran, or CLAUDUCTOR_LOCK_HELD is not the lock"
	else
		local o pid
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
		[ "$(cat "$snap.held")" = "$LOCK" ] || fail "CLAUDUCTOR_LOCK_HELD is '$(cat "$snap.held")', want $LOCK"
	fi
	finish a
	[ "$RC" = 3 ] || fail "exit $RC, want the command's 3"
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
