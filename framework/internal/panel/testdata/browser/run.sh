#!/bin/bash
# Browser tests for the panel page. It builds clauductor, starts a throwaway panel (its
# own port, HOME, tmux socket and project; a fake `claude` and `gh`), and drives the
# page with Playwright:
# - focus-survives-updates.cjs (PANEL-6): focus and a text selection survive live
#   updates, fed by status posts that change what the page shows;
# - appearance-and-keys.cjs (PANEL-11): every type system loads, a type change refits
#   the terminal, and the tabs, Enter, Ctrl+] and the size keys behave. It needs two
#   lanes, which this starts through the page's own API.
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
  *) exec sleep 3600 ;;
esac
SH
printf '#!/bin/sh\necho "[]"\n' > "$tmp/bin/gh"
chmod +x "$tmp/bin/claude" "$tmp/bin/gh"

proj="$tmp/project"
git -C "$proj" init -q -b main
cat > "$proj/.clauductor/panel.json" <<JSON
{ "name": "Focus test", "version": 2, "base": "main", "lanes": { "main": "orchestrator", "change/": "build" }, "tmux_socket": "$sock" }
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
node "$here/appearance-and-keys.cjs" "$base" "$tok" || status=1
exit $status
