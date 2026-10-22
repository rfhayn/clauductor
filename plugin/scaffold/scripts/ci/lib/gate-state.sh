# scripts/ci/lib/gate-state.sh: the gate's run STATE and run LOG (OPS-41). Sourced, never run: by
# run-local.sh, which writes both, and by gate-wait.sh, which reads the state. POSIX sh and git.
#
# WHY. A lane that waits on its own background gate goes silent when the gate fails, and a
# subagent is never woken when its own background task ends (two lanes stalled ~5 h on 2026-10-03).
# gate-wait.sh waits in the foreground instead, and needs to know, without asking anyone, whether a
# gate in this checkout is running, passed, failed or never ran, and for WHICH tree.
#
# THE STATE FILE, one per worktree, beside the receipt: $(git rev-parse --absolute-git-dir)/ci-gate.state
#   started pid=<pid> at=<epoch> mode=<full|quick> subject=<commit|worktree> head=<sha> tree=<id> script=<name>
#   exit pid=<the started pid> code=<exit code, or `signal`: interrupted before its verdict> at=<epoch>
# The started line is written BEFORE the gate queues for the lease, so a gate waiting for it already
# reads as running, never as the previous run's verdict; it replaces the file. The exit line is
# appended when the run ends, by the run itself, and only while the started line is still its own
# (a second gate started in this worktree meanwhile owns the file now). `pid` is the process that
# stays alive for the whole run, queue included; `script` is in its command line, so a reused pid
# does not read as a live gate. SIGKILL writes no exit line: gate-wait.sh then finds the pid gone.
# `subject` says what the run tested: `commit` (a clean-room run of HEAD, GATE_CLEAN_ROOM=archive)
# or `worktree` (the checkout as it stood, the default), and `tree` identifies that worktree.
#
# THE RUN LOG, one per repository, beside the gate lease: <git common dir>/clauductor/gate-runs.log
# One tab-separated line per finished run, appended by the run as it ends:
#   <finished, UTC ISO-8601>  <lane>  <branch>  <head sha>  <full|quick>  <pass|fail>  <exit code|signal>  <run s>  <queued s>  <steps>
# `run s` is the run under the lease; `queued s` the wait for it. `steps` (OPS-89) is each step that
# ran, in order, as `<name>=<seconds>` joined by `;` (`scenario trace=2;secrets=9;…`), or `-` when
# none ran; a line written before OPS-89 has nine fields and no `steps`. Append-only: a line is never
# rewritten. Bounded: once it reaches GATE_RUNS_MAX_BYTES (default 1 MiB, some 10,000 runs) it
# becomes gate-runs.log.1, replacing the one before, and a new log starts; a reader reads .1 first.

# gate_state_file: this worktree's state file. gate_runs_file: this repository's run log.
gate_state_file() { _gs_gd=$(git rev-parse --absolute-git-dir 2>/dev/null) && printf '%s/ci-gate.state\n' "$_gs_gd"; }
gate_runs_file() {
  _gs_cd=$(git rev-parse --git-common-dir 2>/dev/null) || return 1
  case $_gs_cd in /*) ;; *) _gs_cd="$PWD/$_gs_cd" ;; esac
  printf '%s/clauductor/gate-runs.log\n' "$_gs_cd"
}

# gate_state_tree: the working tree's identity, READ-ONLY (it writes no object and no index, so a
# sandboxed agent can compute it): HEAD's sha when nothing is uncommitted or untracked, otherwise a
# hash of HEAD, the diff against it and every untracked, unignored file. The same files give the
# same id; any edit gives another.
gate_state_tree() {
  _gs_head=$(git rev-parse -q --verify HEAD 2>/dev/null || echo none)
  _gs_st=$(git --no-optional-locks status --porcelain --untracked-files=all 2>/dev/null) || { echo unknown; return 0; }
  [ -z "$_gs_st" ] && { echo "$_gs_head"; return 0; }
  _gs_new=$(git ls-files -o --exclude-standard 2>/dev/null)
  { echo "$_gs_head"
    git --no-optional-locks diff --binary HEAD 2>/dev/null
    printf '%s\n' "$_gs_new"
    [ -n "$_gs_new" ] && printf '%s\n' "$_gs_new" | git hash-object --stdin-paths 2>/dev/null
  } | git hash-object --stdin 2>/dev/null || echo unknown
}

# gate_state_field LINE NAME: the value of NAME= in a state line.
gate_state_field() { printf '%s\n' "$1" | tr ' ' '\n' | sed -n "s/^$2=//p" | head -n 1; }

# gate_state_start SCRIPT [GATE ARGS...]: write the started line, unless this run's line is already
# written (the same script re-run under the lease: CLAUDUCTOR_GATE_STATE names this file). Exports
# CLAUDUCTOR_GATE_STATE and CLAUDUCTOR_GATE_STATE_PID for the run that ends it.
gate_state_start() {
  _gs_file=$(gate_state_file) || return 0
  [ "${CLAUDUCTOR_GATE_STATE:-}" = "$_gs_file" ] && return 0
  _gs_script=$(basename "$1"); shift
  _gs_mode=full _gs_dirty=0
  for _gs_a in "$@"; do
    case $_gs_a in --quick) _gs_mode=quick ;; --dirty) _gs_dirty=1 ;; -h|--help) return 0 ;; esac
  done
  # The clean room tests HEAD's commit whatever is on disk; otherwise the run tests the checkout.
  _gs_subject=worktree
  # Read in a subshell: run-local.sh calls this before it loads the config itself, so a value the
  # config takes from $ROOT is unset here, and nounset (run-local.sh's set -u) must not end the gate.
  _gs_room=${GATE_CLEAN_ROOM:-}
  if [ -z "$_gs_room" ]; then
    _gs_top=$(git rev-parse --show-toplevel 2>/dev/null) && [ -f "$_gs_top/.claude/project.conf" ] \
      && _gs_room=$(set +eu; . "$_gs_top/.claude/project.conf" >/dev/null 2>&1; printf '%s' "${GATE_CLEAN_ROOM:-}")
  fi
  [ "$_gs_room" = archive ] && [ "$_gs_dirty" -eq 0 ] && _gs_subject=commit
  printf 'started pid=%s at=%s mode=%s subject=%s head=%s tree=%s script=%s\n' "$$" "$(date +%s)" \
    "$_gs_mode" "$_gs_subject" "$(git rev-parse -q --verify HEAD 2>/dev/null || echo none)" \
    "$(gate_state_tree)" "$_gs_script" > "$_gs_file" 2>/dev/null || return 0
  CLAUDUCTOR_GATE_STATE=$_gs_file CLAUDUCTOR_GATE_STATE_PID=$$
  export CLAUDUCTOR_GATE_STATE CLAUDUCTOR_GATE_STATE_PID
}

# gate_state_end CODE MODE SHA RUN_STARTED LANE [STEPS]: the run's end. Appends the exit line (while
# the started line is still this run's) and the run-log line. Never fails the caller.
gate_state_end() {
  _gs_code=$1 _gs_mode=$2 _gs_sha=$3 _gs_began=$4 _gs_lane=$5
  _gs_steps=$(printf '%s' "${6:-}" | tr '\t\n' '  '); [ -n "$_gs_steps" ] || _gs_steps=-
  _gs_now=$(date +%s) _gs_queued_at=$4
  _gs_file=${CLAUDUCTOR_GATE_STATE:-}
  if [ -n "$_gs_file" ] && [ -f "$_gs_file" ]; then
    _gs_first=$(head -n 1 "$_gs_file" 2>/dev/null)
    if [ "$(gate_state_field "$_gs_first" pid)" = "${CLAUDUCTOR_GATE_STATE_PID:-}" ]; then
      printf 'exit pid=%s code=%s at=%s\n' "$CLAUDUCTOR_GATE_STATE_PID" "$_gs_code" "$_gs_now" >> "$_gs_file" 2>/dev/null
      _gs_queued_at=$(gate_state_field "$_gs_first" at)
    fi
  fi
  _gs_log=$(gate_runs_file) || return 0
  mkdir -p "$(dirname "$_gs_log")" 2>/dev/null || return 0
  if [ -f "$_gs_log" ] && [ "$(wc -c < "$_gs_log" | tr -d ' ')" -ge "${GATE_RUNS_MAX_BYTES:-1048576}" ]; then
    mv -f "$_gs_log" "$_gs_log.1" 2>/dev/null
  fi
  case $_gs_queued_at in ''|*[!0-9]*) _gs_queued_at=$_gs_began ;; esac
  [ "$_gs_queued_at" -le "$_gs_began" ] || _gs_queued_at=$_gs_began
  printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$_gs_lane" \
    "$(git branch --show-current 2>/dev/null | grep . || echo detached)" "$_gs_sha" "$_gs_mode" \
    "$([ "$_gs_code" = 0 ] && echo pass || echo fail)" "$_gs_code" "$((_gs_now - _gs_began))" \
    "$((_gs_began - _gs_queued_at))" "$_gs_steps" >> "$_gs_log" 2>/dev/null
  return 0
}
