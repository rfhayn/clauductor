#!/bin/bash
# Browser tests for the panel page. It builds clauductor, starts a throwaway panel (its
# own port, HOME, tmux socket and project; a fake `claude` and `gh`), and drives the
# page with Playwright:
# - focus-survives-updates.cjs (PANEL-6): focus and a text selection survive live
#   updates, fed by status posts that change what the page shows;
# - appearance-and-keys.cjs (PANEL-11): every type system loads, a type change refits
#   the terminal, and the tabs, Enter, Ctrl+] and the size keys behave. It needs two
#   lanes, which this starts through the page's own API;
# - project-and-side.cjs (PANEL-12): the pinned cards' tab box, rows that stay open,
#   the side panel's edge, and Up next in the Start dialog;
# - terminal-links-selection.cjs (PANEL-14): a selection survives the pointer moving on
#   under a lane that asks for every motion, and links open on ⌘-click;
# - lane-row-actions.cjs (PANEL-18): the "⋯" actions menu on a lane row and a tree
#   node opens without selecting, moves by keys, and every item only asks; Remove on a
#   lane-less worktree asks with its plan, then removes it.
#
#   framework/internal/panel/testdata/browser/run.sh
#
# Needs node and Playwright with Chromium. Set PLAYWRIGHT to the playwright package
# directory if `require("playwright")` does not find it. Not part of `go test`: CI
# may run it where a browser is installed. Synthetic data only.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
fw=$(cd "$here/../../../.." && pwd)
port=${PORT:-4589}
tmp=$(mktemp -d)
sock="clauductor-browser-test-$$"
cleanup() {
  [ -n "${pid:-}" ] && kill "$pid" 2>/dev/null || true
  tmux -L "$sock" kill-server 2>/dev/null || true
  rm -rf "$tmp"
}
trap cleanup EXIT

(cd "$fw" && go build -o "$tmp/clauductor" ./cmd/clauductor)
mkdir -p "$tmp/bin" "$tmp/home" "$tmp/project/.clauductor"
cat > "$tmp/bin/claude" <<'SH'
#!/bin/sh
case "$1" in
  --version) echo "2.1.284 (Claude Code)" ;;
  agents) echo "[]" ;;
  # A lane's claude records the bytes typed into it (raw, unechoed), so the browser
  # test can check which keys reach claude. Lane "second" also behaves as claude's
  # fullscreen TUI does for terminal-links-selection.cjs: it asks for every mouse
  # motion, and prints a URL and two OSC 8 links.
  *) if [ "$CLAUDUCTOR_LANE" = second ]; then
       printf '\033[?1000h\033[?1002h\033[?1003h\033[?1006h'
       printf 'Selectable words on this row\n'
       printf 'PR: https://github.com/o/r/pull/12 is open.\n'
       printf '\033]8;;https://example.com/elsewhere\033\\Docs here\033]8;;\033\\ and \033]8;;https://example.com/same\033\\https://example.com/same\033]8;;\033\\\n'
     fi
     stty raw -echo 2>/dev/null; exec cat >> "$HOME/typed.log" ;;
esac
SH
printf '#!/bin/sh\necho "[]"\n' > "$tmp/bin/gh"
chmod +x "$tmp/bin/claude" "$tmp/bin/gh"

proj="$tmp/project"
git -C "$proj" init -q -b main
cat > "$proj/.clauductor/panel.json" <<JSON
{ "name": "Focus test", "version": 3, "base": "main", "lanes": { "main": "orchestrator", "change/": "build" }, "tmux_socket": "$sock",
  "cards": [
    { "id": "founder", "title": "Founder queue", "pin": true, "refresh": "interval:3600",
      "command": ["printf", "2 item(s) need the founder:\\n- **Box cleanup** (queued 2026-09-27). On the box, prune images\\n- **Ideas page** open it once, then invite a designer\\n"] },
    { "id": "queue", "title": "Change queue", "pin": true, "refresh": "interval:3600",
      "command": ["printf", "2C.10 add-score-photo — photograph a scorecard — in flight\\n2C.27 add-group-card — one golfer enters the card — queued\\n"] }
  ],
  "templates": [
    { "id": "propose", "title": "Propose a roadmap row", "lane_type": "build", "branch_pattern": "change/{name}", "first_prompt": "propose {name}",
      "suggest": { "command": ["printf", "[{\\"name\\":\\"add-group-card\\",\\"title\\":\\"one golfer enters the card\\",\\"detail\\":\\"2C.27\\"}]"], "refresh": "interval:3600" } }
  ] }
JSON
git -C "$proj" -c user.name=t -c user.email=t@example.invalid commit -q --allow-empty -m start
proj=$(cd "$proj" && pwd -P)

HOME="$tmp/home" PATH="$tmp/bin:$PATH" "$tmp/clauductor" panel --project "$proj" --port "$port" --no-open --trust-config > "$tmp/panel.log" 2>&1 &
pid=$!
for _ in $(seq 100); do grep -q 't=' "$tmp/panel.log" 2>/dev/null && break; sleep 0.1; done
url=$(grep -o 'http://[^ ]*t=[0-9a-f]*' "$tmp/panel.log" | head -1)
[ -n "$url" ] || { cat "$tmp/panel.log"; echo "the panel did not start"; exit 1; }

tok=${url##*t=}
base="http://127.0.0.1:$port"
curl -s -c "$tmp/cj" -o /dev/null "$base/?t=$tok"
for body in '{"type":"orchestrator","mode":"root","name":"main"}' '{"type":"build","mode":"new","name":"second"}'; do
  curl -sf -b "$tmp/cj" -H "Origin: $base" -H 'Content-Type: application/json' -d "$body" "$base/api/lanes" > /dev/null \
    || { cat "$tmp/panel.log"; echo "could not start a lane: $body"; exit 1; }
done
status=0
node "$here/focus-survives-updates.cjs" "$base" "$tok" "$proj" || status=1
node "$here/appearance-and-keys.cjs" "$base" "$tok" "$tmp/home/typed.log" || status=1
node "$here/project-and-side.cjs" "$base" "$tok" || status=1
node "$here/terminal-links-selection.cjs" "$base" "$tok" "$tmp/home/typed.log" || status=1
node "$here/lane-row-actions.cjs" "$base" "$tok" "$proj" || status=1
exit $status
