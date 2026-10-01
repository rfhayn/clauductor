#!/bin/bash
# UX harness (UX-1): tear down what up.sh <runid> started: its panel, its tmux servers
# and their socket files, the fake claude processes, and its temp dir. It acts only on
# what the run recorded: the panel's pid (checked to be this run's binary), the sockets
# named ux-<runid>-<n>, and processes whose command line names the run's own dir.
#   down.sh <runid> [--keep]   --keep leaves the temp dir (logs, HOME) for a look
set -uo pipefail
runid=${1:-}
[[ "$runid" =~ ^[a-z0-9]{1,16}$ ]] || { echo "usage: down.sh <runid> [--keep]" >&2; exit 2; }
root=${UX_TMP:-${TMPDIR:-/tmp}/clauductor-ux}
root=$(cd "$root" 2>/dev/null && pwd -P) || { echo "down[$runid]: nothing to do"; exit 0; }
D="$root/$runid"
[ -f "$D/.ux-run" ] || { echo "down[$runid]: no run at $D"; exit 0; }

# The panel: only if the pid still runs this run's binary.
if [ -f "$D/pid" ]; then
  pid=$(cat "$D/pid")
  if [[ "$pid" =~ ^[1-9][0-9]*$ ]] && ps -p "$pid" -o command= 2>/dev/null | grep -qF "$D/bin/clauductor"; then
    kill "$pid" 2>/dev/null
    for _ in $(seq 50); do kill -0 "$pid" 2>/dev/null || break; sleep 0.1; done
    kill -9 "$pid" 2>/dev/null
  fi
fi

# The run's tmux servers, and their socket files.
sockdir="${TMUX_TMPDIR:-/tmp}/tmux-$(id -u)"
for s in $(cat "$D/sockets" 2>/dev/null); do
  [[ "$s" == "ux-$runid-"* ]] || continue
  env -u TMUX tmux -L "$s" kill-server 2>/dev/null
  rm -f "$sockdir/$s" "/private$sockdir/$s" 2>/dev/null
done

# Whatever else names the run's dir: fake claude sessions and their loops.
pids=$(pgrep -f "$D/" 2>/dev/null | grep -vx "$$" || true)
[ -n "$pids" ] && kill $pids 2>/dev/null
sleep 0.2
pids=$(pgrep -f "$D/" 2>/dev/null | grep -vx "$$" || true)
[ -n "$pids" ] && kill -9 $pids 2>/dev/null

if [ "${2:-}" = --keep ]; then
  echo "down[$runid]: stopped; kept $D"
else
  # A read-only file (a go module cache an older up.sh left in the temp HOME) would
  # stop rm: make the run's own tree writable first.
  chmod -R u+w "$D" 2>/dev/null
  rm -rf "$D"
  if [ -e "$D" ]; then echo "down[$runid]: could not remove all of $D" >&2; exit 1; fi
  echo "down[$runid]: removed $D"
fi
