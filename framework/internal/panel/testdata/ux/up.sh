#!/bin/bash
# UX harness (UX-1): start a throwaway panel with three projects and lanes in varied
# states, for the deep UI/UX passes in matrix.cjs and flows.cjs. Several runs can go at
# once: each has its own run id, port, temp HOME and tmux sockets (ux-<runid>-<n>).
#
#   up.sh <runid> <port>     prints one JSON line: {base, token, home, dir, pid,
#                            sockets, projects, lanes, typedLog, bin}
#   down.sh <runid>          tears it down
#
# It never touches the real ~/.claude or ~/.clauductor, launchd, or any tmux socket
# but its own: HOME is a temp dir, the panel's hooks go into that HOME's settings, and
# every project names its socket in its panel.json.
#
# Environment:
#   CLAUDUCTOR_BIN   use this binary instead of building the checkout
#   UX_TMP           where runs live (default ${TMPDIR:-/tmp}/clauductor-ux)
#   UX_PORT_MIN/MAX  the allowed port range (default 4700–4799)
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
fw=$(cd "$here/../../../.." && pwd)
runid=${1:-}
port=${2:-}
[[ "$runid" =~ ^[a-z0-9]{1,16}$ ]] || { echo "usage: up.sh <runid: [a-z0-9]{1,16}> <port>" >&2; exit 2; }
[[ "$port" =~ ^[0-9]+$ ]] && [ "$port" -ge "${UX_PORT_MIN:-4700}" ] && [ "$port" -le "${UX_PORT_MAX:-4799}" ] \
  || { echo "up.sh: port $port is outside ${UX_PORT_MIN:-4700}–${UX_PORT_MAX:-4799}" >&2; exit 2; }
root=${UX_TMP:-${TMPDIR:-/tmp}/clauductor-ux}
mkdir -p "$root"
root=$(cd "$root" && pwd -P)
D="$root/$runid"
[ -e "$D" ] && { echo "up.sh: run $runid exists ($D); run down.sh $runid first" >&2; exit 2; }
if curl -s --max-time 1 -o /dev/null "http://127.0.0.1:$port/healthz"; then
  echo "up.sh: something already answers on port $port" >&2; exit 2
fi
mkdir -p "$D"/{bin,home,projects}
say() { echo "up[$runid]: $*" >&2; }
# down.sh reads what is recorded here, so a failed up still cleans up.
echo "$D" > "$D/.ux-run"
fail() { say "FAILED: $*"; [ -f "$D/panel.log" ] && tail -20 "$D/panel.log" >&2; "$here/down.sh" "$runid" >&2 || true; exit 1; }
trap 'fail "line $LINENO"' ERR

# The environment every command here and the panel see: the temp HOME, the fakes
# first on PATH, no API key (lanes refuse to start with one), not nested in tmux or
# Claude Code.
export HOME="$D/home"
export PATH="$D/bin:$PATH"
export GIT_CONFIG_GLOBAL="$D/gitconfig"
export GIT_CONFIG_NOSYSTEM=1
unset ANTHROPIC_API_KEY ANTHROPIC_AUTH_TOKEN TMUX TMUX_PANE CLAUDECODE CLAUDE_CODE_ENTRYPOINT CLAUDE_CODE_SSE_PORT 2>/dev/null || true
printf '[user]\n\tname = UX Harness\n\temail = ux@example.invalid\n[init]\n\tdefaultBranch = main\n[advice]\n\tdetachedHead = false\n' > "$GIT_CONFIG_GLOBAL"

if [ -n "${CLAUDUCTOR_BIN:-}" ]; then
  cp "$CLAUDUCTOR_BIN" "$D/bin/clauductor"
else
  say "building clauductor from $fw"
  (cd "$fw" && go build -o "$D/bin/clauductor" ./cmd/clauductor)
fi
cp "$here/fake/claude" "$here/fake/gh" "$D/bin/"
chmod +x "$D/bin/"*
C="$D/bin/clauductor"

s1="ux-$runid-1" s2="ux-$runid-2" s3="ux-$runid-3"
printf '%s\n' "$s1" "$s2" "$s3" > "$D/sockets"

P="$D/projects"
A="$P/alpha" B="$P/beta" G="$P/gamma"
mkdir -p "$A/.clauductor" "$A/src" "$A/docs" "$B/.clauductor" "$G/.clauductor"

# Alpha: cards, a template with Up next, a queue; a bare origin so ahead/behind and
# the stale-cards note have something to read.
cat > "$A/.clauductor/panel.json" <<JSON
{ "name": "Alpha", "version": 3, "base": "main", "tmux_socket": "$s1",
  "lanes": { "main": "orchestrator", "change/": "build", "fix/": "fix" },
  "lane_types": { "build": { "model": "opus", "effort": "high" }, "fix": { "model": "sonnet" } },
  "alerts": { "notify": false, "idle_minutes": 30 },
  "cards": [
    { "id": "founder", "title": "Founder queue", "pin": true, "refresh": "interval:3600",
      "command": ["printf", "3 item(s) need the founder:\\n- **Box cleanup** (queued 2026-09-27). On the box, prune images and old volumes before the disk fills\\n- **Ideas page** open it once, then invite a designer\\n- **A deliberately long row title that keeps going to see how the pinned card box wraps or clips it at narrow widths**\\n"] },
    { "id": "queue", "title": "Change queue", "pin": true, "refresh": "interval:3600",
      "command": ["printf", "2C.10 add-score-photo — photograph a scorecard — in flight\\n2C.27 add-group-card — one golfer enters the card — queued\\n2C.31 fix-offline-sync — sync after airplane mode — queued\\n"] },
    { "id": "health", "title": "Health", "refresh": "interval:3600", "command": ["printf", "gate: green\\ncoverage: 81%%\\n"] }
  ],
  "templates": [
    { "id": "propose", "title": "Propose a roadmap row", "lane_type": "build", "branch_pattern": "change/{name}", "first_prompt": "propose {name}",
      "suggest": { "command": ["printf", "[{\\"name\\":\\"add-group-card\\",\\"title\\":\\"one golfer enters the card\\",\\"detail\\":\\"2C.27\\"},{\\"name\\":\\"fix-offline-sync\\",\\"title\\":\\"sync after airplane mode\\",\\"detail\\":\\"2C.31\\"}]"], "refresh": "interval:3600" } },
    { "id": "fix", "title": "Fix an issue", "lane_type": "fix", "branch_pattern": "fix/{name}", "first_prompt": "fix {issue}" }
  ],
  "queues": [ { "id": "gate", "title": "Gate", "lock": "clauductor/gate.lock" } ] }
JSON
printf '# Alpha\n\nA throwaway repository for the UX harness.\n' > "$A/README.md"
printf 'export const answer = 42;\n' > "$A/src/app.ts"
printf 'todo\n' > "$A/docs/todo.md"
git -C "$A" init -q -b main
git -C "$A" add -A
git -C "$A" commit -q -m "Start Alpha"
git clone -q --bare "$A" "$P/alpha-origin.git"
git -C "$A" remote add origin "$P/alpha-origin.git"
git -C "$A" fetch -q origin
git -C "$A" branch -q -u origin/main main

# Beta: a plain project with one card and a lane.
cat > "$B/.clauductor/panel.json" <<JSON
{ "name": "Beta", "version": 3, "base": "main", "tmux_socket": "$s2",
  "lanes": { "main": "orchestrator", "feature/": "build" }, "alerts": { "notify": false },
  "cards": [ { "id": "todo", "title": "To do", "pin": true, "refresh": "watch:todo.md", "command": ["cat", "todo.md"] } ] }
JSON
printf 'Ship the thing\nWrite the docs\n' > "$B/todo.md"
git -C "$B" init -q -b main && git -C "$B" add -A && git -C "$B" commit -q -m "Start Beta"

# Gamma: valid when added, broken before the panel starts, so the project menu shows
# a project that cannot load.
cat > "$G/.clauductor/panel.json" <<JSON
{ "name": "Gamma", "version": 3, "base": "main", "tmux_socket": "$s3", "alerts": { "notify": false } }
JSON
git -C "$G" init -q -b main && git -C "$G" add -A && git -C "$G" commit -q -m "Start Gamma"

A=$(cd "$A" && pwd -P) B=$(cd "$B" && pwd -P) G=$(cd "$G" && pwd -P)
"$C" panel add --project "$A" --default >"$D/add.log" 2>&1
"$C" panel trust --project "$A" >>"$D/add.log" 2>&1
"$C" panel add --project "$B" >>"$D/add.log" 2>&1
"$C" panel trust --project "$B" >>"$D/add.log" 2>&1
"$C" panel add --project "$G" >>"$D/add.log" 2>&1
printf '{ "name": "Gamma", "version": 3, "tmux_socket": "%s", "lanes": { "main": ' "$s3" > "$G/.clauductor/panel.json"

# The panel, in the background, recorded for down.sh.
say "starting the panel on port $port"
nohup "$C" panel --project "$A" --port "$port" --no-open > "$D/panel.log" 2>&1 < /dev/null &
pid=$!
echo "$pid" > "$D/pid"
url=""
for _ in $(seq 150); do
  url=$(grep -o 'http://[^ ]*t=[0-9a-f]*' "$D/panel.log" 2>/dev/null | head -1 || true)
  [ -n "$url" ] && break
  kill -0 "$pid" 2>/dev/null || fail "the panel exited"
  sleep 0.1
done
[ -n "$url" ] || fail "the panel did not print its URL"
tok=${url##*t=}
base="http://127.0.0.1:$port"
curl -s -c "$D/cookies" -o /dev/null "$base/?t=$tok"
api() { # api <project> <path> <json>
  curl -s -b "$D/cookies" -H "Origin: $base" -H 'Content-Type: application/json' -d "$3" "$base/api/p/$1$2"
}
start() { # start <project> <json>; records the lane's answer
  local out; out=$(api "$1" /lanes "$2")
  echo "$out" | grep -q '"ok":true' || fail "could not start a lane in $1: $2 → $out"
}

supports_budget=false
if curl -s -b "$D/cookies" "$base/static/panel.js" | grep -qi 'budget'; then supports_budget=true; fi

say "starting lanes"
start alpha '{"type":"orchestrator","mode":"root","name":"idle"}'
start alpha '{"type":"build","mode":"new","name":"working"}'
start alpha '{"type":"build","mode":"new","name":"waiting"}'
start alpha '{"type":"build","mode":"new","name":"stream"}'
start alpha '{"type":"build","mode":"new","name":"merged"}'
start alpha '{"type":"build","mode":"new","name":"orphan"}'
lanes='"idle","working","waiting","stream","merged","orphan"'
if $supports_budget; then start alpha '{"type":"fix","mode":"new","name":"budget"}'; lanes="$lanes,\"budget\""; fi
start beta '{"type":"orchestrator","mode":"root","name":"beta-one"}'

WT="$A/.claude/worktrees"
# working: one commit pushed with an upstream, one more local (ahead 1), and a change.
git -C "$WT/working" commit -q --allow-empty -m "Working: first step"
git -C "$WT/working" push -q -u origin change/working
printf 'export const more = 1;\n' > "$WT/working/src/more.ts"
git -C "$WT/working" add -A && git -C "$WT/working" commit -q -m "Working: second step"
printf '// edited\n' >> "$WT/working/src/app.ts"
# merged: a commit whose PR gh reports merged, so Close lane removes its branch.
printf 'merged\n' > "$WT/merged/MERGED.md"
git -C "$WT/merged" add -A && git -C "$WT/merged" commit -q -m "Merged: the change"
# main falls 2 behind origin: the cards say they may be stale.
git clone -q "$P/alpha-origin.git" "$P/alpha-other"
git -C "$P/alpha-other" commit -q --allow-empty -m "Upstream 1"
git -C "$P/alpha-other" commit -q --allow-empty -m "Upstream 2"
git -C "$P/alpha-other" push -q origin main
git -C "$A" fetch -q origin
# Lane-less worktrees: a clean detached one, and a dirty one.
git -C "$A" worktree add -q --detach "$WT/leftover-clean" main
git -C "$A" worktree add -q --detach "$WT/leftover-dirty" main
printf 'scratch\n' > "$WT/leftover-dirty/untracked.txt"
printf '// dirty\n' >> "$WT/leftover-dirty/src/app.ts"

# gh: open PRs with checks for the lane branches, and "merged" merged.
mkdir -p "$HOME/fake/gh"
cat > "$HOME/fake/gh/alpha.open.json" <<'JSON'
[
 {"number":41,"title":"Working: add the thing","headRefName":"change/working","author":{"login":"ux"},"isDraft":false,"reviewDecision":"REVIEW_REQUIRED","statusCheckRollup":[
   {"__typename":"CheckRun","name":"ci","status":"COMPLETED","conclusion":"SUCCESS"},
   {"__typename":"CheckRun","name":"e2e","status":"IN_PROGRESS","conclusion":""},
   {"__typename":"StatusContext","context":"local-gate","state":"SUCCESS"}]},
 {"number":42,"title":"Waiting: needs a permission","headRefName":"change/waiting","author":{"login":"ux"},"isDraft":true,"reviewDecision":"","statusCheckRollup":[
   {"__typename":"CheckRun","name":"ci","status":"COMPLETED","conclusion":"FAILURE"}]},
 {"number":43,"title":"deps: bump the routine group with 2 updates","headRefName":"dependabot/npm/routine","author":{"login":"app/dependabot","is_bot":true},"isDraft":false,"reviewDecision":"APPROVED","statusCheckRollup":[]}
]
JSON
echo "change/merged" > "$HOME/fake/gh/alpha.merged"

# orphan: its tmux session goes, as a reboot would take it.
sleep 1
tmux -L "$s1" kill-session -t "=orphan" 2>/dev/null || true
curl -s -b "$D/cookies" -H "Origin: $base" -X POST -o /dev/null "$base/api/p/alpha/refresh" || true

# Wait until the page's state has every lane, and the working lane reads busy.
ok=false
for _ in $(seq 60); do
  st=$(curl -s -b "$D/cookies" "$base/api/state?p=alpha" || true)
  if echo "$st" | grep -q '"working"' && echo "$st" | grep -q '"orphan"' && echo "$st" | grep -q 'busy'; then ok=true; break; fi
  sleep 0.5
done
$ok || say "warning: the state did not show every lane in 30 s"
trap - ERR

out=$(printf '{"runid":"%s","base":"%s","token":"%s","home":"%s","dir":"%s","pid":%s,"bin":"%s","sockets":["%s","%s","%s"],"projects":{"alpha":"%s","beta":"%s","gamma":"%s"},"lanes":{"alpha":[%s],"beta":["beta-one"]},"leftovers":["%s","%s"],"typedLog":"%s","budget":%s}' \
  "$runid" "$base" "$tok" "$HOME" "$D" "$pid" "$C" "$s1" "$s2" "$s3" "$A" "$B" "$G" "$lanes" "$WT/leftover-clean" "$WT/leftover-dirty" "$HOME/typed.log" "$supports_budget")
echo "$out" > "$D/up.json"
echo "$out"
