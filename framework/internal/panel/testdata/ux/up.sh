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
#   UX_LIFECYCLE=1   also start the lanes the lifecycle flows act on (UX-2): shipclean and
#                    shipdirty (lane type ship, closed by lanes_auto_close once their PRs
#                    merge; shipdirty has an untracked file) and limited (auto-resume)
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

# Built under the real HOME, before it moves: go's module cache (read-only files) must
# not land in the run's temp HOME, where down.sh could not remove it.
if [ -n "${CLAUDUCTOR_BIN:-}" ]; then
  cp "$CLAUDUCTOR_BIN" "$D/bin/clauductor"
else
  say "building clauductor from $fw"
  (cd "$fw" && go build -o "$D/bin/clauductor" ./cmd/clauductor)
fi

# The environment every command here and the panel see: the temp HOME, the fakes
# first on PATH, no API key (lanes refuse to start with one), not nested in tmux or
# Claude Code.
export HOME="$D/home"
export PATH="$D/bin:$PATH"
export GIT_CONFIG_GLOBAL="$D/gitconfig"
export GIT_CONFIG_NOSYSTEM=1
unset ANTHROPIC_API_KEY ANTHROPIC_AUTH_TOKEN TMUX TMUX_PANE CLAUDECODE CLAUDE_CODE_ENTRYPOINT CLAUDE_CODE_SSE_PORT 2>/dev/null || true
printf '[user]\n\tname = UX Harness\n\temail = ux@example.invalid\n[init]\n\tdefaultBranch = main\n[advice]\n\tdetachedHead = false\n' > "$GIT_CONFIG_GLOBAL"

cp "$here/fake/claude" "$here/fake/gh" "$D/bin/"
# The metrics command (PANEL-19): the metrics package's fixture, a valid payload in
# Alpha and, with METRICS_FIXTURE=bad, one that breaks the contract in Beta.
cp "$fw/internal/panel/metrics/testdata/metrics.sh" "$fw/internal/panel/metrics/testdata/metrics.json" "$D/bin/"
chmod +x "$D/bin/"*
C="$D/bin/clauductor"

s1="ux-$runid-1" s2="ux-$runid-2" s3="ux-$runid-3"
printf '%s\n' "$s1" "$s2" "$s3" > "$D/sockets"

P="$D/projects"
A="$P/alpha" B="$P/beta" G="$P/gamma"
mkdir -p "$A/.clauductor" "$A/.claude" "$A/src" "$A/docs" "$B/.clauductor" "$G/.clauductor" "$HOME/fake"
now=$(date +%s)

# Alpha: cards, a template with Up next, a queue; a bare origin so ahead/behind and
# the stale-cards note have something to read.
cat > "$A/.clauductor/panel.json" <<JSON
{ "name": "Alpha", "version": 5, "base": "main", "tmux_socket": "$s1",
  "lanes": { "main": "orchestrator", "change/": "build", "fix/": "fix", "ship/": "ship" },
  "lane_types": { "build": { "model": "opus", "effort": "high", "auto_close": "off" }, "fix": { "model": "sonnet", "auto_close": "off" }, "ship": { "model": "sonnet" } },
  "alerts": { "notify": false, "idle_minutes": 30, "approval_wait_hours": 24, "stale_days": 3 },
  "quota_economy": { "five_hour_pct": 40 },
  "lanes_auto_close": "on_merge",
  "quota_auto_resume": true,
  "worktree_setup": { "command": ["sh", "-c", "printf '%s %s\\n' \"\$CLAUDUCTOR_LANE\" \"\$CLAUDUCTOR_PORT\" > .ux-setup && printf '%s %s\\n' \"\$CLAUDUCTOR_LANE\" \"\$CLAUDUCTOR_PORT\" >> '$HOME/fake/setup.log'"] },
  "worktree_teardown": { "command": ["sh", "-c", "printf '%s %s\\n' \"\$CLAUDUCTOR_LANE\" \"\$CLAUDUCTOR_PORT\" >> '$HOME/fake/teardown.log'"] },
  "ports": { "base": 39100, "per_lane": 10 },
  "metrics": { "command": ["sh", "$D/bin/metrics.sh"] },
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
# PANEL-20: a gitignored file .worktreeinclude names (copied into each new worktree; the
# tracked README.md it also names never is), and the marker worktree_setup writes
# (ignored, so a lane's worktree stays clean for Close).
printf '.env.local\n.ux-setup\n' > "$A/.gitignore"
printf '# copied into each new lane worktree when also gitignored\n.env.local\nREADME.md\n' > "$A/.worktreeinclude"
# PANEL-19: the economy mapping the Economy badge names.
printf '{ "economy": { "_why": "ux harness", "scribe": { "model": "sonnet", "effort": "low" }, "mechanic": "haiku/low" } }\n' > "$A/.claude/model-roles.json"
git -C "$A" init -q -b main
git -C "$A" add -A
git -C "$A" commit -q -m "Start Alpha"
printf 'UX_SECRET=from-the-project-root\n' > "$A/.env.local"
# PANEL-19: the changes the Needs-you signals read. The proposals are never committed: a
# committed one would be written afresh in each new worktree, and the copy written last
# counts. add-group-card waits for approval (3 days old); budget is over its $20 (the
# budget lane spends $48); working has spent $3.10 of $3.50 (amber); ready has none.
echo "changes/" >> "$A/.git/info/exclude"
mkdir -p "$A/changes/add-group-card" "$A/changes/budget" "$A/changes/working" "$A/changes/ready"
printf '# add-group-card\n\n**Budget:** $30\n\nOne golfer enters the card.\n' > "$A/changes/add-group-card/proposal.md"
touch -t "$(date -r $((now - 3 * 86400)) +%Y%m%d%H%M)" "$A/changes/add-group-card/proposal.md"
printf '# budget\n\n**Approved:** 2026-09-01 by ux\n**Budget:** $20\n' > "$A/changes/budget/proposal.md"
printf '# working\n\n**Approved:** 2026-09-01 by ux\n**Budget:** $3.50\n' > "$A/changes/working/proposal.md"
printf '# ready\n\n**Approved:** 2026-09-01 by ux\n' > "$A/changes/ready/proposal.md"
git clone -q --bare "$A" "$P/alpha-origin.git"
git -C "$A" remote add origin "$P/alpha-origin.git"
git -C "$A" fetch -q origin
git -C "$A" branch -q -u origin/main main

# Beta: a plain project with one card and a lane.
cat > "$B/.clauductor/panel.json" <<JSON
{ "name": "Beta", "version": 4, "base": "main", "tmux_socket": "$s2",
  "lanes": { "main": "orchestrator", "feature/": "build" }, "alerts": { "notify": false },
  "metrics": { "command": ["env", "METRICS_FIXTURE=bad", "sh", "$D/bin/metrics.sh"] },
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
# PANEL-19: remote control in lanes mode, the machine's choice `panel install` records,
# under the temp HOME: each lane starts with --remote-control and its ⋯ has Remote control.
mkdir -p "$HOME/.clauductor/panel" && chmod 700 "$HOME/.clauductor" "$HOME/.clauductor/panel"
printf '{"mode":"lanes"}' > "$HOME/.clauductor/panel/remote-control.json"
chmod 600 "$HOME/.clauductor/panel/remote-control.json"

# The panel, in the background, recorded for down.sh.
say "starting the panel on port $port"
nohup "$C" panel --project "$A" --port "$port" --no-open > "$D/panel.log" 2>&1 < /dev/null &
pid=$!
echo "$pid" > "$D/pid"
started=$(perl -MTime::HiRes=time -e 'printf "%d\n", time*1000')
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
# Read whole first: `curl | grep -q` under pipefail fails when grep exits early (curl
# gets SIGPIPE), so a build with budgets read as one without.
js=$(curl -s -b "$D/cookies" "$base/static/panel.js" || true)
case "$js" in *[Bb]udget*) supports_budget=true ;; esac

say "starting lanes"
start alpha '{"type":"orchestrator","mode":"root","name":"idle"}'
start alpha '{"type":"build","mode":"new","name":"working"}'
start alpha '{"type":"build","mode":"new","name":"waiting"}'
start alpha '{"type":"build","mode":"new","name":"stream"}'
start alpha '{"type":"build","mode":"new","name":"merged"}'
start alpha '{"type":"build","mode":"new","name":"orphan"}'
start alpha '{"type":"build","mode":"new","name":"quiet"}'
start alpha '{"type":"build","mode":"new","name":"ready"}'
# A long name (PANEL-21): the Lanes table's ⋯ column was pushed out of the rail by one.
long=a-rather-long-lane-name-for-layout
start alpha '{"type":"build","mode":"new","name":"'"$long"'"}'
lanes='"idle","working","waiting","stream","merged","orphan","quiet","ready","'"$long"'"'
if $supports_budget; then start alpha '{"type":"fix","mode":"new","name":"budget"}'; lanes="$lanes,\"budget\""; fi
lifecycle=false
if [ "${UX_LIFECYCLE:-}" = 1 ]; then
  lifecycle=true
  start alpha '{"type":"ship","mode":"new","name":"shipclean"}'
  start alpha '{"type":"ship","mode":"new","name":"shipdirty"}'
  start alpha '{"type":"build","mode":"new","name":"limited"}'
  lanes="$lanes,\"shipclean\",\"shipdirty\",\"limited\""
fi
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
# quiet: its last commit is 5 days old (and its registry record is backdated below), so
# it is a stale lane (alerts.stale_days 3).
old="$((now - 5 * 86400)) +0000"
GIT_AUTHOR_DATE="$old" GIT_COMMITTER_DATE="$old" git -C "$WT/quiet" commit -q --allow-empty -m "Quiet: an old step"
# ready: every task ticked, and a clean gate receipt for HEAD, so with its green,
# approved PR (below) its Checks tab says Ready to merge.
mkdir -p "$WT/ready/changes/ready"
printf '# Tasks\n\n- [x] 1.1 Build it\n- [x] 1.2 Test it\n' > "$WT/ready/changes/ready/tasks.md"
git -C "$WT/ready" add -f changes/ready/tasks.md && git -C "$WT/ready" commit -q -m "Ready: the tasks"
printf '%s\tfull\tclean\tall\n' "$(git -C "$WT/ready" rev-parse HEAD)" > "$(git -C "$WT/ready" rev-parse --absolute-git-dir)/ci-receipt"
if $lifecycle; then
  printf 'ship\n' > "$WT/shipclean/SHIP.md"; git -C "$WT/shipclean" add -A && git -C "$WT/shipclean" commit -q -m "Ship clean: the change"
  printf 'ship\n' > "$WT/shipdirty/SHIP.md"; git -C "$WT/shipdirty" add -A && git -C "$WT/shipdirty" commit -q -m "Ship dirty: the change"
  printf 'not committed\n' > "$WT/shipdirty/scratch.txt"
fi
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
 {"number":43,"title":"deps: bump the routine group with 2 updates","headRefName":"dependabot/npm/routine","author":{"login":"app/dependabot","is_bot":true},"isDraft":false,"reviewDecision":"APPROVED","statusCheckRollup":[]},
 {"number":44,"title":"Ready: all green","headRefName":"change/ready","author":{"login":"ux"},"isDraft":false,"reviewDecision":"APPROVED","statusCheckRollup":[
   {"__typename":"CheckRun","name":"ci","status":"COMPLETED","conclusion":"SUCCESS"},
   {"__typename":"CheckRun","name":"e2e","status":"COMPLETED","conclusion":"SUCCESS"}]}
JSON
if $lifecycle; then
  # The ship lanes' PRs are open at first; the lifecycle flow merges them.
  cat >> "$HOME/fake/gh/alpha.open.json" <<'JSON'
,{"number":45,"title":"Ship clean","headRefName":"ship/shipclean","author":{"login":"ux"},"isDraft":false,"reviewDecision":"APPROVED","statusCheckRollup":[]}
,{"number":46,"title":"Ship dirty","headRefName":"ship/shipdirty","author":{"login":"ux"},"isDraft":false,"reviewDecision":"APPROVED","statusCheckRollup":[]}
JSON
fi
echo "]" >> "$HOME/fake/gh/alpha.open.json"
echo "change/merged" > "$HOME/fake/gh/alpha.merged"
# Review threads (merge readiness): #41 has one unresolved of two; #44 all resolved.
echo '{"data":{"repository":{"pullRequest":{"reviewThreads":{"totalCount":2,"nodes":[{"isResolved":true},{"isResolved":false}]}}}}}' > "$HOME/fake/gh/threads-41.json"
echo '{"data":{"repository":{"pullRequest":{"reviewThreads":{"totalCount":2,"nodes":[{"isResolved":true},{"isResolved":true}]}}}}}' > "$HOME/fake/gh/threads-44.json"
# Merged PRs over the last 30 days, for the Metrics view's built-in figures.
iso() { date -u -r "$1" +%Y-%m-%dT%H:%M:%SZ; }
{
  printf '['
  sep=""
  for i in 1 2 3 4 5 6; do
    m=$((now - i * 4 * 86400)) c=$((now - i * 4 * 86400 - i * 5 * 3600))
    printf '%s{"number":%d,"title":"Merged change %d","headRefName":"change/old-%d","createdAt":"%s","mergedAt":"%s"}' "$sep" $((30 + i)) "$i" "$i" "$(iso $c)" "$(iso $m)"
    sep=","
  done
  printf ']\n'
} > "$HOME/fake/gh/alpha.mergedlist.json"

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

# quiet's registry record says it started 5 days ago: with its old commit, no commit for
# 5 days. The panel re-reads its registry every 30 s. Written last and atomically, once
# the lanes' first hooks (which mark each record's conversation) have come in.
# A write of the panel's own (a hook marking a conversation) between our read and its
# re-read would undo it, so it is written again until the state shows it (up to 40 s).
sleep 2
reg=$(grep -l '"id": *"quiet"' "$HOME"/.clauductor/panel/*/lanes.json 2>/dev/null | head -1 || true)
if [ -n "$reg" ]; then
  backdate() { perl -0pe 's/("id":\s*"quiet".*?"created":\s*)\d+/${1}'"$(( (now - 5 * 86400) * 1000 ))"'/s' "$reg" > "$reg.ux" && mv "$reg.ux" "$reg"; }
  backdate
  shown=false
  for _ in $(seq 20); do
    st=$(curl -s -b "$D/cookies" "$base/api/state?p=alpha" || true)
    c=$(printf '%s' "$st" | perl -0ne 'print $1 if /"id":"quiet"[^{}]*?"created":(\d+)/' || true)
    if [ -n "$c" ]; then
      [ "${#c}" -gt 11 ] && c=$((c / 1000)) # ms or s, as the view carries it
      if [ "$c" -lt $((now - 86400)) ]; then shown=true; break; fi
    fi
    grep -q "\"created\": *$(( (now - 5 * 86400) * 1000 ))" "$reg" || backdate
    sleep 2
  done
  $shown || say "warning: the panel did not take quiet's backdated start in 40 s (no stale lane)"
else
  say "warning: no registry record for quiet"
fi
trap - ERR

out=$(printf '{"runid":"%s","base":"%s","token":"%s","home":"%s","dir":"%s","pid":%s,"bin":"%s","sockets":["%s","%s","%s"],"projects":{"alpha":"%s","beta":"%s","gamma":"%s"},"lanes":{"alpha":[%s],"beta":["beta-one"]},"leftovers":["%s","%s"],"typedLog":"%s","budget":%s,"started":%s,"lifecycle":%s}' \
  "$runid" "$base" "$tok" "$HOME" "$D" "$pid" "$C" "$s1" "$s2" "$s3" "$A" "$B" "$G" "$lanes" "$WT/leftover-clean" "$WT/leftover-dirty" "$HOME/typed.log" "$supports_budget" "$started" "$lifecycle")
echo "$out" > "$D/up.json"
echo "$out"
