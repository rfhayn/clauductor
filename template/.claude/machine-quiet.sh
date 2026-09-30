#!/bin/sh
# machine-quiet.sh: leave the machine quiet at session close. Run by session-close after the last
# gate; `--dry-run` prints the same list and changes nothing.
#
# WHY A SCRIPT AND NOT A SKILL SENTENCE (AGENTS.md rule 4). The trigger was a hot laptop: hook
# `awk` loops orphaned by an agent's probes had spun at ~100% CPU each for 13 hours, and an agent's
# `restart: unless-stopped` stack had run for 6 days from a worktree nobody remembered. Each was
# invisible from inside a session. So this looks, acts, and prints what it did.
#
# What it does, in order, each only on things a session or its agents left behind:
#   1. kills ORPHANED processes (parent pid 1) that match a KNOWN LEAK SHAPE (MQ_ORPHAN_SHAPES),
#      never merely "anything under .claude/worktrees/" (see step 1 for why);
#   2. (optional, MQ_DOCKER=1) removes docker containers whose compose project lives under
#      .claude/worktrees/ or a Claude scratchpad: agent stacks, never your own;
#   3. removes CLEAN, UNLOCKED git worktrees under .claude/worktrees/ that no live Claude session
#      has as (or under) its cwd, no panel lane registers, and no process names; and NONE at all
#      while the live sessions cannot be read or another live session sits at the repo root;
#   4. (optional, MQ_COLIMA=1) stops colima if it runs and no gate (GATE_RUN) is running.
#
# It never touches: a tmux server or client, a claude or clauductor process, containers outside
# those paths, volumes, worktrees that are dirty, locked or a live session's cwd, or colima while
# a gate runs. Checked by .claude/checks/machine-quiet.sh.

set -u
# No pathname expansion: the orphan shapes below are glob PATTERNS, split on spaces, and must never
# expand against the current directory. (A `case` pattern still matches with -f.)
set -f
DRY=0
[ "${1:-}" = "--dry-run" ] && DRY=1
ROOT=$(git rev-parse --show-toplevel 2>/dev/null) || { echo "machine-quiet: not in a git repo"; exit 0; }
# The checkout's own config; a missing one leaves the defaults below.
if [ -f "$ROOT/.claude/lib/conf.sh" ]; then
  # shellcheck disable=SC1091
  . "$ROOT/.claude/lib/conf.sh"
fi
WT="$ROOT/.claude/worktrees"
act() { if [ "$DRY" = 1 ]; then echo "  would: $*"; else "$@"; fi; }

echo "machine-quiet ($([ "$DRY" = 1 ] && echo dry run || echo acting)):"

# 1. Orphaned processes of a known leak shape.
#
# NOT "any orphan whose command line mentions .claude/worktrees/". The panel's tmux SERVER is
# exactly that: it daemonizes (parent pid 1) and keeps the argv of the command that first started
# it, `tmux -L <socket> new-session -d -s <lane> -c <repo>/.claude/worktrees/<lane> …`. A version
# that killed every such orphan SIGKILLed the tmux server at a session close, taking down every
# lane, the closing session's own included (exit 137). So:
#   - a process whose argv[0] is tmux (or `tmux: server`/`tmux: client`), claude or clauductor is
#     never killed, whatever its arguments name;
#   - a process in any live Claude session's tree (`claude agents --json` pids) is never killed;
#   - and of the rest, only a process matching one of MQ_ORPHAN_SHAPES is: shell glob patterns
#     over the command line, `@WT@` standing for this repo's worktree directory. The default is the
#     one leak shape actually observed: a hook's process (a shell or awk running a script from a
#     worktree's own .claude/hooks/). Add your own shapes (a dev server started from a lane) in
#     .claude/project.conf.
shapes=${MQ_ORPHAN_SHAPES:-"*@WT@/*/.claude/hooks/*"}
session_pids=""
if command -v claude >/dev/null 2>&1 && command -v jq >/dev/null 2>&1; then
  session_pids=$(claude agents --json 2>/dev/null </dev/null | jq -r '.[]? | .pid? // empty' 2>/dev/null | tr '\n' ' ')
fi
ps -Ao pid=,ppid=,command= | while read -r pid ppid cmd; do
  [ "$ppid" = 1 ] || continue
  argv0=${cmd%% *}; base=${argv0##*/}
  case "$base" in tmux | tmux: | claude | clauductor) continue ;; esac
  case "$cmd" in "tmux: "*) continue ;; esac
  case " $session_pids " in *" $pid "*) continue ;; esac
  hit=""
  for s in $shapes; do
    pat=$(printf '%s' "$s" | sed "s|@WT@|$WT|g")
    # shellcheck disable=SC2254 # the pattern is the point
    case "$cmd" in $pat) hit=1; break ;; esac
  done
  [ -n "$hit" ] || continue
  echo "  orphan pid $pid: $(printf '%s' "$cmd" | cut -c1-110)"
  act kill "$pid" 2>/dev/null
done

# 2. Agent compose stacks (optional; only if docker is reachable: a stopped VM holds nothing).
if [ "${MQ_DOCKER:-0}" = 1 ] && command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
  docker ps -a --format '{{.ID}}|{{.Names}}|{{index .Labels "com.docker.compose.project.working_dir"}}' 2>/dev/null |
    while IFS='|' read -r id name dir; do
      case "$dir" in "$WT"/*|/private/tmp/claude-*|/tmp/claude-*) ;; *) continue ;; esac
      echo "  agent container $name (from $dir)"
      act docker rm -f "$id" >/dev/null
    done
fi

# 3. Clean agent worktrees nobody is using.
#
# "Nobody is using it" is asked of the AUTHORITY for live sessions, `claude agents --json` (cwd +
# status), not inferred from a process scan: `pgrep -f <path>` does not see a Claude Code session
# whose cwd is the worktree, and a scan-based version removed the worktrees of a busy and an idle
# live session from under them. A worktree that equals or contains any listed session's cwd is
# kept, whatever the status. If that list cannot be read (no `claude`, no python3, a failure, a 5 s
# timeout, JSON that is not a list of sessions with a cwd), NO worktree is removed (fail closed):
# a leftover worktree costs disk, a wrong removal costs a live session its working directory. The
# panel's lane registry (~/.clauductor/panel/*/lanes.json, each lane's `path`) is a second,
# optional authority: a lane it started is kept even while its session is down.
#
# A cwd only protects the worktree it is in. A session whose cwd is the repo root (an
# orchestrator) reaches worktrees by path and through its subagents, so while any such session is
# live, NO worktree is removed, except when that session is this script's caller (its pid is an
# ancestor). THE GAPS LEFT: the caller's own subagents working in a clean worktree (so run this
# with none in flight), and a session elsewhere using a worktree by path.
worktrees=$(git -C "$ROOT" worktree list --porcelain | awk '/^worktree /{sub(/^worktree /, ""); print}')
locked=$(git -C "$ROOT" worktree list --porcelain |
  awk '/^worktree /{p = substr($0, 10)} /^locked/{r = substr($0, 8); print p "\t" (r == "" ? "no reason given" : r)}')
live=$(printf '%s\n' "$worktrees" | python3 -c '
import glob, json, os, shutil, signal, subprocess, sys

def forms(p):
    p = p.rstrip("/") or "/"
    return {os.path.normpath(p), os.path.realpath(p)}

def within(wt, cwd):
    """True when cwd is wt or lies under it."""
    if any(c == w or c.startswith(w.rstrip("/") + "/") for w in forms(wt) for c in forms(cwd)):
        return True
    if not (os.path.exists(wt) and os.path.exists(cwd)):
        return False
    a = os.path.realpath(cwd)
    while True:
        try:
            if os.path.samefile(a, wt):
                return True
        except OSError:
            return False
        parent = os.path.dirname(a)
        if parent == a:
            return False
        a = parent

def fail(why):
    print("FAIL\t" + why)
    sys.exit(0)

wt_dir = sys.argv[1]
worktrees = [l for l in sys.stdin.read().split("\n") if l]
exe = shutil.which("claude")
if not exe:
    fail("the claude CLI is not on PATH, so the live sessions cannot be listed")
try:
    proc = subprocess.Popen([exe, "agents", "--json"], stdout=subprocess.PIPE,
                            stderr=subprocess.DEVNULL, stdin=subprocess.DEVNULL,
                            start_new_session=True)
except OSError as e:
    fail("claude agents --json could not start (%s)" % e)
try:
    out, _ = proc.communicate(timeout=5)
except subprocess.TimeoutExpired:
    try:
        os.killpg(proc.pid, signal.SIGKILL)
    except OSError:
        pass
    fail("claude agents --json did not answer within 5 s")
if proc.returncode != 0:
    fail("claude agents --json exited %d" % proc.returncode)
try:
    sessions = json.loads(out)
except ValueError:
    fail("claude agents --json printed something that is not JSON")
if not isinstance(sessions, list) or not all(
        isinstance(s, dict) and isinstance(s.get("cwd"), str) and s["cwd"] for s in sessions):
    fail("claude agents --json is not a list of sessions that each name a cwd")

def ancestors():
    """This process chain up to (not including) pid 1, or None if any link cannot be read."""
    seen, pid = set(), os.getppid()
    while pid > 1 and pid not in seen:
        seen.add(pid)
        try:
            r = subprocess.run(["ps", "-o", "ppid=", "-p", str(pid)], capture_output=True,
                               text=True, timeout=5)
            pid = int(r.stdout.strip()) if r.returncode == 0 else None
        except (OSError, ValueError, subprocess.TimeoutExpired):
            pid = None
        if pid is None:
            return None
    return seen

def label(s):
    return "%s (%s)" % (s.get("name") or s.get("sessionId") or "?", s.get("status") or "status unknown")

root_sessions = [s for s in sessions if within(s["cwd"], wt_dir)]
if root_sessions:
    mine = ancestors()
    for s in root_sessions:
        pid = s.get("pid")
        if mine is not None and isinstance(pid, int) and pid in mine:
            print("NOTE\tthe calling session %s is at the repo root; it does not block removal" % label(s))
            continue
        fail("a session at the repo root may be using worktrees by path: %s at %s%s"
             % (label(s), s["cwd"],
                "" if mine is not None else " (the caller of this script could not be determined)"))

keep = {}
for s in sessions:
    for w in worktrees:
        if w not in keep and within(w, s["cwd"]):
            who = s.get("name") or s.get("sessionId") or "?"
            keep[w] = "live Claude session %s (%s) has its cwd there" % (who, s.get("status") or "status unknown")

for reg in sorted(glob.glob(os.path.join(os.path.expanduser("~"), ".clauductor", "panel", "*", "lanes.json"))):
    try:
        with open(reg) as f:
            lanes = json.load(f).get("lanes") or []
    except (OSError, ValueError, AttributeError) as e:
        print("NOTE\tcould not read the panel lane registry %s (%s); relying on claude agents alone" % (reg, e))
        continue
    for lane in lanes:
        p = lane.get("path") if isinstance(lane, dict) else None
        if not isinstance(p, str) or not p:
            continue
        for w in worktrees:
            if w not in keep and within(w, p):
                keep[w] = "the panel lane %s is registered there" % (lane.get("id") or "?")

for w, why in keep.items():
    print("KEEP\t%s\t%s" % (w, why))
' "$WT" 2>&1) || live="FAIL	python3 could not run the live-session check (${live:-no output})"
[ -n "$live" ] || live="OK"
live_fail=$(printf '%s\n' "$live" | awk -F '\t' '$1 == "FAIL" { print $2; exit }')
[ -n "$live_fail" ] && echo "  REMOVING NO WORKTREE: $live_fail (fail closed)"
printf '%s\n' "$live" | awk -F '\t' '$1 == "NOTE" { print "  note: " $2 }'

printf '%s\n' "$worktrees" | while read -r w; do
  case "$w" in "$WT"/*) ;; *) continue ;; esac
  if [ -n "$(git -C "$w" status --porcelain 2>/dev/null)" ]; then
    echo "  KEEP worktree $w: it has uncommitted changes"
    continue
  fi
  lock=$(printf '%s\n' "$locked" | awk -F '\t' -v w="$w" '$1 == w { print $2; exit }')
  if [ -n "$lock" ]; then
    echo "  KEEP worktree $w: it is locked ($lock)"
    continue
  fi
  if [ -n "$live_fail" ]; then
    echo "  KEEP worktree $w: removing none (see above)"
    continue
  fi
  why=$(printf '%s\n' "$live" | awk -F '\t' -v w="$w" '$1 == "KEEP" && $2 == w { print $3; exit }')
  if [ -n "$why" ]; then
    echo "  KEEP worktree $w: $why"
    continue
  fi
  if pgrep -f "$w" >/dev/null 2>&1; then
    echo "  KEEP worktree $w: a live process is using it"
    continue
  fi
  br=$(git -C "$w" branch --show-current 2>/dev/null)
  echo "  clean worktree $w${br:+ ($br)}"
  act git -C "$ROOT" worktree remove -f "$w"
  case "$br" in worktree-agent-*) act git -C "$ROOT" branch -D "$br" >/dev/null 2>&1 ;; esac
done
[ "$DRY" = 1 ] || git -C "$ROOT" worktree prune

# 4. The Docker VM (optional).
if [ "${MQ_COLIMA:-0}" = 1 ] && command -v colima >/dev/null 2>&1 && colima list 2>/dev/null | awk '$2 == "Running" { f = 1 } END { exit !f }'; then
  if pgrep -f "${GATE_RUN:-scripts/ci/run-local.sh}" >/dev/null 2>&1; then
    echo "  colima left running: a gate is in progress"
  else
    echo "  colima running with no gate in progress: stopping it"
    act colima stop >/dev/null 2>&1
  fi
fi
echo "machine-quiet: done"
