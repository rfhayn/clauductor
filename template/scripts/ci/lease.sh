# lease.sh: the gate lease protocol in plain POSIX shell, VENDORED VERBATIM from the clauductor
# repo, docs/panel.md, "The protocol in plain shell" (the block between the lease.sh markers).
# run-local.sh uses it when `clauductor lock-run` is not installed, so a machine or CI without
# clauductor still queues on the same lease as every panel lane. Do not edit it here: update it
# from docs/panel.md. Clauductor's tests fail if this copy drifts from that block, and its
# conformance suite runs against the block.

# clauductor lease protocol v1 in plain POSIX shell: interoperates with
# `clauductor lock-run`. Usage: lease_run <lockdir> <lane> <command> [args...]
# Exit status: the command's; 75 if the wait was cancelled from the panel; 70
# if the lease was taken away while the command ran. The holder writes a TTL
# (CLAUDUCTOR_LEASE_TTL, default 600 s) and renews it every TTL/3; the command
# gets CLAUDUCTOR_LEASE_NONCE, and `lease_verify` says whether it still holds.
# Liveness and start time need ps; without it, kill -0 (EPERM still means alive)
# and /proc/<pid>/stat field 22. A start time from one source is never compared
# with one from the other, and an alive pid that cannot be verified is live.
lease_alive() {
  if command -v ps >/dev/null 2>&1; then [ -n "$(ps -o pid= -p "$1" 2>/dev/null || true)" ]; return; fi
  _e=$(kill -0 "$1" 2>&1) && return 0
  case $_e in *ermitted*) return 0 ;; esac
  return 1
}
lease_pstart() {
  _v=""
  if command -v ps >/dev/null 2>&1; then _v=$(LC_ALL=C ps -o lstart= -p "$1" 2>/dev/null | awk '{$1=$1; print}' || true); fi
  if [ -z "$_v" ] && [ -r "/proc/$1/stat" ]; then _v="proc:$(sed 's/.*) //' "/proc/$1/stat" | awk '{print $20}')"; fi
  printf '%s\n' "$_v"
}
# lease_proc_dead PID RECORDED_START: 0 (true) only when the pid is gone, or was
# reused (a start time from the same source that differs).
lease_proc_dead() {
  lease_alive "$1" || return 0
  _n=$(lease_pstart "$1")
  { [ -n "$2" ] && [ -n "$_n" ]; } || return 1
  _a=${2%%:*} _b=${_n%%:*}
  if { [ "$_a" = proc ] && [ "$_b" = proc ]; } || { [ "$_a" != proc ] && [ "$_b" != proc ]; }; then
    [ "$_n" != "$2" ]; return
  fi
  return 1
}
lease_get() { LC_ALL=C sed -n "s/.*\"$2\"[[:space:]]*:[[:space:]]*\"\{0,1\}\([^\",}]*\).*/\1/p" "$1" 2>/dev/null | head -n 1 | LC_ALL=C sed 's/[[:space:]]*$//' || true; }
# lease_mtime PATH: its modification time in unix seconds: GNU stat, then BSD stat.
# Unknown reads as now (young), so missing data never makes a lock look abandoned.
lease_mtime() {
  _m=$(stat -c %Y "$1" 2>/dev/null) || _m=$(stat -f %m "$1" 2>/dev/null) || _m=""
  case $_m in ''|*[!0-9]*) date +%s ;; *) printf '%s\n' "$_m" ;; esac
}
# lease_valid FILE: 0 (true) for a valid record, the rule Go applies: one flat JSON
# object whose values are strings, integers, true, false or null; v, pid, child_pid,
# started, renewed and ttl integers; nonce, pstart, child_pstart, host, lane and cmd
# strings; and a nonce of 16 lower-case hex digits. An invalid owner.json counts as
# missing; an invalid waiter file holds no place in the queue and is never removed.
# Byte for byte (LC_ALL=C), as Go decodes: a string may hold any byte but \000-\037,
# so DEL (0x7f, which json.Marshal writes unescaped) and bytes that are not UTF-8 are
# allowed whatever the user's locale.
lease_valid() {
  _j=$(LC_ALL=C awk '{ s = s $0 " " } END { print s }' "$1" 2>/dev/null) || return 1
  _c=$(printf '\001-\037')
  _S='"([^"\\'"$_c"']|\\(["\\/bfnrt]|u[0-9a-fA-F]{4}))*"'
  _V="($_S|-?(0|[1-9][0-9]*)|true|false|null)"
  _P="[[:space:]]*$_S[[:space:]]*:[[:space:]]*$_V[[:space:]]*"
  printf '%s\n' "$_j" | LC_ALL=C grep -Eq "^[[:space:]]*\\{($_P(,$_P)*)?\\}[[:space:]]*\$" || return 1
  if printf '%s\n' "$_j" | LC_ALL=C grep -Eq '"(v|pid|child_pid|started|renewed|ttl)"[[:space:]]*:[[:space:]]*[^-0-9[:space:]]'; then return 1; fi
  if printf '%s\n' "$_j" | LC_ALL=C grep -Eq '"(nonce|pstart|child_pstart|host|lane|cmd)"[[:space:]]*:[[:space:]]*[^"[:space:]]'; then return 1; fi
  lease_get "$1" nonce | grep -Eq '^[0-9a-f]{16}$'
}
# lease_dead FILE WAITER_TTL: 0 (true) when the record can be removed: on this host
# only when the holder AND its command (child_pid, written by lock-run) are dead.
lease_dead() {
  _p=$(lease_get "$1" pid); _s=$(lease_get "$1" pstart); _h=$(lease_get "$1" host)
  _cp=$(lease_get "$1" child_pid); _cs=$(lease_get "$1" child_pstart)
  _r=$(lease_get "$1" renewed); _t=$(lease_get "$1" ttl); [ -n "$2" ] && _t=$2
  if [ -n "$_p" ] && [ "$_h" = "$(hostname)" ]; then
    lease_proc_dead "$_p" "$_s" || return 1
    if [ -n "$_cp" ] && ! lease_proc_dead "$_cp" "$_cs"; then return 1; fi
    return 0
  fi
  [ "${_t:-0}" -gt 0 ] && [ "$(date +%s)" -gt $(( ${_r:-0} + _t )) ]   # another host, or no pid: TTL
}
lease_holder_stale() {
  if ! lease_valid "$1/owner.json"; then [ $(( $(date +%s) - $(lease_mtime "$1") )) -ge 10 ]; return; fi
  lease_dead "$1/owner.json" ""
}
# lease_renew LOCK NONCE TTL HOLDER_PID: run in the background while the command holds the lease.
# Every TTL/3 it rewrites owner.json's `renewed` (as lock-run does), so another host never reads a
# live holder as expired; it stops once the holder is gone or the record is no longer its own.
lease_renew() {
  _every=$(( $3 / 3 )); [ "$_every" -ge 1 ] || _every=1
  _last=$(date +%s)
  while sleep 1; do
    lease_alive "$4" || return 0
    [ "$(lease_get "$1/owner.json" nonce)" = "$2" ] || return 0
    _now=$(date +%s); [ $(( _now - _last )) -ge "$_every" ] || continue
    LC_ALL=C sed "s/\"renewed\":[0-9]*/\"renewed\":$_now/" "$1/owner.json" > "$1/.renew.tmp" && mv "$1/.renew.tmp" "$1/owner.json" || true
    _last=$_now
  done
}
# lease_verify LOCK HOLDER_PID: 0 (true) when LOCK is still held for the command asking: owner.json
# carries CLAUDUCTOR_LEASE_NONCE (lease_run exports it to its command), or, from a holder that
# exports none (lock-run), names HOLDER_PID or one of its ancestors as pid or child_pid. A gate
# asks before it records a pass: a run that lost its lease may have run beside another.
lease_verify() {
  lease_valid "$1/owner.json" || return 1
  if [ -n "${CLAUDUCTOR_LEASE_NONCE:-}" ]; then [ "$(lease_get "$1/owner.json" nonce)" = "$CLAUDUCTOR_LEASE_NONCE" ]; return; fi
  _vp=$(lease_get "$1/owner.json" pid) _vc=$(lease_get "$1/owner.json" child_pid) _va=${2:-} _vn=0
  while [ -n "$_va" ] && [ "$_va" != 0 ] && [ "$_vn" -lt 12 ]; do
    { [ "$_va" = "$_vp" ] || [ "$_va" = "$_vc" ]; } && return 0
    command -v ps >/dev/null 2>&1 || return 1
    _va=$(ps -o ppid= -p "$_va" 2>/dev/null | tr -d ' ' || true); _vn=$((_vn + 1))
  done
  return 1
}
lease_run() {
  # A quote or backslash would break owner.json, which readers then judge stale.
  _lock=$1 _lane=$(printf %s "$2" | tr -d '"\\'); shift 2
  _cmd=$(printf %s "$1" | tr -d '"\\')
  _w="$_lock.waiters" _nonce=$(od -An -N8 -tx1 /dev/urandom | tr -d ' \n')
  # The holder's TTL (default 600 s, lock-run's); renewed every TTL/3 while the command runs.
  _ttl=${CLAUDUCTOR_LEASE_TTL:-600}; case $_ttl in '' | *[!0-9]*) _ttl=600 ;; esac
  mkdir -p "$_w"
  _rec="{\"v\":1,\"nonce\":\"$_nonce\",\"pid\":$$,\"pstart\":\"$(lease_pstart $$)\",\"host\":\"$(hostname)\",\"lane\":\"$_lane\",\"cmd\":\"$_cmd\",\"started\":$(date +%s),\"renewed\":$(date +%s),\"ttl\":$_ttl}"
  _me="$_w/$(date +%s)000000000-$_nonce.json"
  printf '%s\n' "$_rec" > "$_me.tmp" && mv "$_me.tmp" "$_me"
  trap 'rm -f "$_me" "$_w/$_nonce.cancel"' EXIT
  _said=""
  while :; do
    if [ -e "$_w/$_nonce.cancel" ]; then echo "lease: wait cancelled from the panel" >&2; return 75; fi
    _first=""
    for _f in $(ls "$_w" 2>/dev/null | grep '\.json$' | sort -t- -k1,1n -k2); do
      lease_valid "$_w/$_f" || continue
      if lease_dead "$_w/$_f" 60; then rm -f "$_w/$_f"; continue; fi
      _first=$_f; break
    done
    if [ "$_first" = "${_me##*/}" ] && mkdir "$_lock" 2>/dev/null; then
      printf '%s\n' "$_rec" > "$_lock/.owner.tmp" && mv "$_lock/.owner.tmp" "$_lock/owner.json"
      rm -f "$_me"
      lease_renew "$_lock" "$_nonce" "$_ttl" $$ </dev/null >/dev/null 2>&1 &
      _renewer=$!
      trap 'kill "$_renewer" 2>/dev/null || :; if [ "$(lease_get "$_lock/owner.json" nonce)" = "$_nonce" ]; then rm -rf "$_lock"; fi' EXIT
      CLAUDUCTOR_LOCK_HELD=$_lock CLAUDUCTOR_LEASE_NONCE=$_nonce "$@" && _rc=0 || _rc=$?
      kill "$_renewer" 2>/dev/null || :; wait "$_renewer" 2>/dev/null || :
      if [ "$(lease_get "$_lock/owner.json" nonce)" = "$_nonce" ]; then rm -rf "$_lock"
      else echo "lease: LOST $_lock while the command ran (another may have run beside it)" >&2; _rc=70; fi
      trap - EXIT
      return "$_rc"
    fi
    if [ "$_first" = "${_me##*/}" ] && [ -d "$_lock" ] && lease_holder_stale "$_lock"; then
      _judged=$(lease_get "$_lock/owner.json" nonce)
      if mkdir "$_lock.reclaim" 2>/dev/null; then
        if [ "$(lease_get "$_lock/owner.json" nonce)" = "$_judged" ] && lease_holder_stale "$_lock"; then
          echo "lease: reclaiming $_lock from a dead holder" >&2; rm -rf "$_lock"
        fi
        rmdir "$_lock.reclaim"; continue
      elif [ $(( $(date +%s) - $(lease_mtime "$_lock.reclaim") )) -ge 30 ]; then rmdir "$_lock.reclaim" 2>/dev/null || true
      fi
    fi
    [ -n "$_said" ] || { echo "lease: waiting for $_lock ($(lease_get "$_lock/owner.json" lane))" >&2; _said=1; }
    sleep 1
  done
}
