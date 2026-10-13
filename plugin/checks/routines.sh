#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# The routines runner (.claude/routines.sh, OPS-42), held to specs/routines: the registry's
# validation and the schedule grammar against fixtures, and this project's own registry as its
# process check. Never the real claude (design D13): every check runs with lib.sh's refusing
# `claude` first on PATH, and the case below proves a check that forgets its fake fails.
. "$(dirname "$0")/lib.sh"
need jq git

d=$(scratch)
lib="$(cd "$(dirname "$0")" && pwd)/lib.sh"

# ── The fixture: a repository holding the runner, a roles file of its own and one skill ──────────
# Its own roles file, not the project's: the project's roles are its own to change, and a plugin
# project's scaffolded copy never gains the `routine` role this change adds (`clauductor update`
# adds the template's new keys only where the model is installed).
P="$d/proj"; new_repo "$P"
mkdir -p "$P/.claude/skills/session-start"
cp "$CLAUDUCTOR_FW/routines.sh" "$P/.claude/"
printf '%s\n' '{"roles": {"routine": {"model": "sonnet", "effort": "low"}, "scribe": {"model": "opus", "effort": "medium"}}}' > "$P/.claude/model-roles.json"
git -C "$P" commit -q --allow-empty -m init
T0=1791763200 # Monday 2026-10-12 00:00 UTC
BASE='{"version": 1, "billing": null, "api_key_file": null,
  "caps": {"subscription": {"five_hour_pct": 80, "seven_day_pct": 80, "unknown_quota_daily_runs": 3, "daily_runs": null},
           "api": {"daily_usd": 1.00, "window_usd": 5.00, "window_days": 7, "daily_runs": null}},
  "routines": []}'
# reg FILE BILLING ROUTINES: a registry (BILLING and ROUTINES as JSON).
reg() { mkdir -p "$(dirname "$1")"; printf '%s' "$BASE" | jq --argjson b "$2" --argjson rs "$3" '.billing = $b | .routines = $rs' > "$1"; }
R="$P/.claude/routines.json"
# at T VERB...: the runner in the fixture at unix time T, in UTC unless TZ is given.
at() { _t=$1; shift; (cd "$P" && env TZ="${TZ_AT:-UTC0}" ROUTINES_NOW="$_t" sh .claude/routines.sh "$@") 2>&1; }
cmd() { jq -cn --arg n "$1" --arg s "$2" '{name: $n, schedule: $s, run: {command: "echo hi"}}'; }
# cl NAME TOOLS [EXTRA]: a claude -p routine with role `routine`, TOOLS (JSON), EXTRA merged in.
cl() { jq -cn --arg n "$1" --argjson t "$2" --argjson x "${3:-null}" \
  '{name: $n, schedule: "07:00", run: {prompt: "digest", role: "routine", allowed_tools: $t, max_turns: 6}} * ($x // {})'; }
# rejects NAME WHAT OUT RC MUSTSAY...: validation failed, naming the routine and each MUSTSAY.
rejects() {
  _n=$1; _what=$2; _o=$3; _rc=$4; shift 4
  _miss=""; for _w in "$@"; do case $_o in *"$_w"*) ;; *) _miss="$_miss [$_w]" ;; esac; done
  if [ "$_rc" -eq 2 ] && [ -z "$_miss" ] && case $_o in *"\"$_n\""*) true ;; *) false ;; esac; then ok "$_what"
  else fail "$_what (exit $_rc; missing$_miss or the routine's name): $_o"; fi
}
# row LABEL OUT NAME REGEX: the list row for NAME matches REGEX (extended, the whole line).
row() {
  if printf '%s\n' "$2" | grep -Eq "^$3 +$4\$"; then ok "[$1] list: $3 reads $4"
  else fail "[$1] list: no row '$3 … $4' in: $(printf '%s' "$2" | tr '\n' '|')"; fi
}
other=darwin; [ "$(uname -s)" = Darwin ] && other=linux

# ── [ROUTINES-1-S1] A valid registry lists its routines ─────────────────────────────────────────
reg "$R" '"subscription"' "[$(cmd nightly 02:30), $(cl digest '["Read", "Grep", "Glob"]')]"
L="$P/.git/clauductor/routines"; mkdir -p "$L"
s=$((T0 - 86400 + 9000)) # Sunday 02:30
printf '%s\n' "{\"record\":\"start\",\"id\":\"n1\",\"name\":\"nightly\",\"trigger\":\"schedule\",\"slot\":$s,\"started\":$s}" \
  "{\"record\":\"end\",\"id\":\"n1\",\"name\":\"nightly\",\"verdict\":\"ok\",\"exit\":0,\"ended\":$((s + 41))}" > "$L/runs.jsonl"
out=$(at $((T0 + 60)) list); rc=$?
expect_rc 0 "$rc" "[ROUTINES-1-S1] list exits 0 on a valid registry"
printf '%s\n' "$out" | grep -Eq '^NAME +SCHEDULE +KIND +NEXT DUE +LAST$' && ok "[ROUTINES-1-S1] list heads its columns: name, schedule, kind, next due, last verdict" \
  || fail "[ROUTINES-1-S1] list has no header: $out"
row ROUTINES-1-S1 "$out" nightly '02:30 +command +2026-10-12 02:30 +ok at 2026-10-11 02:30'
row ROUTINES-1-S1 "$out" digest '07:00 +claude -p +now \(catch-up of 2026-10-11 07:00\) +never ran'
out=$(at $((T0 + 60)) validate); rc=$?
expect_rc 0 "$rc" "[ROUTINES-1-S1] validate passes it: $out"

# ── [ROUTINES-1-S3] A write tool needs writes: true ──────────────────────────────────────────────
for t in Edit Write Bash 'Bash(git log *)' Agent NotATool; do
  reg "$R" '"subscription"' "[$(cl digest "[\"Read\", \"$t\"]")]"
  out=$(at "$T0" validate); rc=$?
  rejects digest "[ROUTINES-1-S3] allowed_tools $t without writes: true fails, naming the routine and the tool" "$out" "$rc" "\"$t\"" 'writes'
  reg "$R" '"subscription"' "[$(cl digest "[\"Read\", \"$t\"]" '{"writes": true}')]"
  out=$(at "$T0" validate); rc=$?
  expect_rc 0 "$rc" "[ROUTINES-1-S3] the same routine with writes: true validates ($t): $out"
done
reg "$R" '"subscription"' "[$(cl digest '["Read", "Grep", "Glob", "WebFetch", "WebSearch"]')]"
out=$(at "$T0" validate); rc=$?
expect_rc 0 "$rc" "[ROUTINES-1-S3] the read-only tools need no writes: $out"

# ── [ROUTINES-1-S4] A routine names a role that exists, and a skill directory ────────────────────
reg "$R" '"subscription"' "[$(cl digest '["Read"]' '{"run": {"role": "nope"}}')]"
out=$(at "$T0" validate); rc=$?
rejects digest "[ROUTINES-1-S4] a role model-roles.json does not define fails, naming it" "$out" "$rc" '"nope"' 'run.role'
reg "$R" '"subscription"' "[$(cl digest '["Read"]' '{"run": {"prompt": null, "skill": "no-such-skill"}}' | jq -c 'del(.run.prompt)')]"
out=$(at "$T0" validate); rc=$?
rejects digest "[ROUTINES-1-S4] a skill with no directory of its own fails, naming it" "$out" "$rc" '"no-such-skill"' 'run.skill'
reg "$R" '"subscription"' "[$(cl digest '["Read"]' '{"run": {"skill": "session-start"}}' | jq -c 'del(.run.prompt)')]"
out=$(at "$T0" validate); rc=$?
expect_rc 0 "$rc" "[ROUTINES-1-S4] a skill whose directory exists, and a defined role, validate: $out"
mv "$P/.claude/model-roles.json" "$d/roles.away"
reg "$R" '"subscription"' "[$(cl digest '["Read"]')]"
out=$(at "$T0" validate); rc=$?
rejects digest "[ROUTINES-1-S4] with no model-roles.json the role cannot be checked, and that fails" "$out" "$rc" 'cannot be checked'
mv "$d/roles.away" "$P/.claude/model-roles.json"

# ── A routine for another runner or machine: valid, and listed with what it waits for ────────────
# (Its scenario is cited where the tick drives it, task 2.1.)
reg "$R" '"subscription"' "[$(jq -cn --argjson r "$(cmd cloudy 02:30)" '$r + {where: "cloud"}'),
  $(jq -cn --argjson r "$(cmd elsewhere 02:30)" --arg m "$other" '$r + {machine: $m}'),
  $(jq -cn --argjson r "$(cmd here 02:30)" --arg m "$(uname -s | tr '[:upper:]' '[:lower:]')" '$r + {machine: $m}')]"
out=$(at "$T0" validate); rc=$?
expect_rc 0 "$rc" "routines for another runner or machine are valid: $out"
out=$(at "$T0" list)
row "another runner" "$out" cloudy '02:30 +command +waits for the cloud runner +never ran'
row "another runner" "$out" elsewhere "02:30 +command +waits for a $other machine +never ran"
row "another runner" "$out" here '02:30 +command +now \(catch-up of 2026-10-11 02:30\) +never ran'

# ── [ROUTINES-1-S6] An MCP tool is refused even with writes ──────────────────────────────────────
for x in null '{"writes": true}'; do
  reg "$R" '"subscription"' "[$(cl digest '["Read", "mcp__github__get_issue"]' "$x")]"
  out=$(at "$T0" validate); rc=$?
  rejects digest "[ROUTINES-1-S6] an mcp__ tool fails (extra: $x), saying no MCP server is loaded" "$out" "$rc" '"mcp__github__get_issue"' 'load no MCP server'
done

# ── [ROUTINES-1-S7] A claude -p routine needs a declared billing mode ────────────────────────────
reg "$R" null "[$(cmd nightly 02:30), $(cl digest '["Read"]')]"
out=$(at "$T0" validate); rc=$?
rejects digest "[ROUTINES-1-S7] billing null with an enabled claude -p routine fails, pointing at setup" "$out" "$rc" 'routines.sh setup --billing subscription|api'
reg "$R" null "[$(cmd nightly 02:30)]"
out=$(at "$T0" validate); rc=$?
expect_rc 0 "$rc" "[ROUTINES-1-S7] a registry with only command routines validates with no billing: $out"
reg "$R" null "[$(cmd nightly 02:30), $(cl digest '["Read"]' '{"enabled": false}')]"
out=$(at "$T0" validate); rc=$?
expect_rc 0 "$rc" "[ROUTINES-1-S7] a disabled claude -p routine needs no billing yet: $out"
reg "$R" '"api"' "[$(cmd nightly 02:30)]"
out=$(at "$T0" validate); rc=$?
case $out in *"--key-file"*) [ "$rc" -eq 2 ] && ok "billing api with no api_key_file fails, pointing at setup --key-file" || fail "billing api, no key file: exit $rc" ;;
  *) fail "billing api with no api_key_file: $out" ;; esac

# ── The schedule grammar (D3): next due by `list` at fixed clock times, and validation ──────────
# (Its scenario is cited where the tick runs at those times, task 2.1.) catch_up skip, so each
# next-due time is the grammar's own slot and never a catch-up. The two `every` routines last
# started at T0; the daily ones never ran.
sk() { jq -cn --argjson r "$(cmd "$1" "$2")" '$r + {catch_up: "skip"}'; }
reg "$R" '"subscription"' "[$(sk daily 02:30), $(sk monthu 'Mon,Thu 06:00'), $(sk weekdays 'Mon-Fri 18:15'), $(sk half 'every 30m'), $(sk twohours 'every 2h')]"
printf '%s\n' "{\"record\":\"start\",\"id\":\"h1\",\"name\":\"half\",\"started\":$T0}" "{\"record\":\"end\",\"id\":\"h1\",\"name\":\"half\",\"verdict\":\"ok\"}" \
  "{\"record\":\"start\",\"id\":\"t1\",\"name\":\"twohours\",\"started\":$T0}" "{\"record\":\"end\",\"id\":\"t1\",\"name\":\"twohours\",\"verdict\":\"ok\"}" > "$L/runs.jsonl"
H=3600; D=86400
# grammar T NAME WANT: at time T, NAME's next due reads WANT.
grammar() {
  _o=$(at "$1" list)
  if printf '%s\n' "$_o" | grep -Eq "^$2 +[^ ].* command +$3 +"; then ok "grammar: at $(TZ="${TZ_AT:-UTC0}" jq -nr --argjson t "$1" '$t | localtime | strftime("%a %H:%M")') $2 is due $3"
  else fail "grammar: at $1 $2 should be due $3: $(printf '%s' "$_o" | grep -E "^$2 ")"; fi
}
grammar "$T0" daily '2026-10-12 02:30'
grammar "$T0" monthu '2026-10-12 06:00'
grammar "$T0" weekdays '2026-10-12 18:15'
grammar "$T0" half '2026-10-12 00:30'
grammar "$T0" twohours '2026-10-12 02:00'
grammar $((T0 + 2 * H + 1800)) daily 'now \(slot 2026-10-12 02:30\)'
grammar $((T0 + 2 * H + 1800)) half 'now \(slot 2026-10-12 02:30\)'
grammar $((T0 + 2 * H + 1800)) twohours '2026-10-12 04:00'
grammar $((T0 + 2 * H + 1800 + 899)) daily 'now \(slot 2026-10-12 02:30\)'
grammar $((T0 + 2 * H + 1800 + 900)) daily '2026-10-13 02:30'
grammar $((T0 + 18 * H + 900)) weekdays 'now \(slot 2026-10-12 18:15\)'
grammar $((T0 + 18 * H + 900)) monthu '2026-10-15 06:00'
grammar $((T0 + 3 * D + 6 * H)) monthu 'now \(slot 2026-10-15 06:00\)'
grammar $((T0 + 4 * D + 18 * H + 1800)) weekdays '2026-10-19 18:15'
grammar $((T0 + 4 * D + 18 * H + 1800)) monthu '2026-10-19 06:00'
grammar $((T0 + 5 * D + 12 * H)) weekdays '2026-10-19 18:15'
grammar $((T0 + 5 * D + 12 * H)) daily '2026-10-18 02:30'
# Local time: in UTC+9, T0 is Monday 09:00.
TZ_AT=XYZ-9
grammar "$T0" daily '2026-10-13 02:30'
grammar "$T0" monthu '2026-10-15 06:00'
grammar "$T0" weekdays '2026-10-12 18:15'
TZ_AT=""
for s in 00:00 23:59 'Sat-Sun 10:00' 'Mon,Wed,Fri 07:15' 'every 5m' 'every 1h'; do
  reg "$R" null "[$(cmd ok-one "$s")]"
  out=$(at "$T0" validate); rc=$?
  expect_rc 0 "$rc" "grammar: the schedule '$s' is valid"
done
for s in 'every 3m' '9:5' 'Sun-Mon 01:00' '0 3 * * *' '25:00' '02:60' 'Mon-Mon 01:00' 'Mon,Mon 01:00' 'mon 01:00' \
  'Mon 1:00' 'every 0h' 'every 30s' 'Mon-Fri  18:15' ' 02:30' 'daily 02:30'; do
  reg "$R" null "[$(cmd bad-one "$s")]"
  out=$(at "$T0" validate); rc=$?
  rejects bad-one "grammar: the schedule '$s' is rejected, naming the routine and its field" "$out" "$rc" 'schedule:'
done

# ── Validation: every other field, and the registry's own ───────────────────────────────────────
for f in '{"timeout_s": 0}' '{"timeout_s": 1.5}' '{"catch_up": "all"}' '{"output": "push"}' '{"writes": "yes"}' \
  '{"enabled": 1}' '{"machine": "windows"}' '{"where": ""}' '{"name": "Bad_Name"}' '{"timout_s": 60}' \
  '{"run": {"command": ""}}' '{"run": {"command": "x", "prompt": "y"}}' '{"run": {}}' '{"run": "echo hi"}' \
  '{"name": "bad-one\n"}' '{"schedule": "02:30\n"}' '{"schedule": "every 5m\n"}' '{"schedule": "Mon-Fri 18:15\n"}' \
  '{"schedule": "Mon,Thu 06:00\n"}' '{"schedule": "Mon\n 06:00"}' '{"schedule": "Mon-Fri\n 18:15"}' '{"where": "cloud\n"}'; do
  reg "$R" null "[$(jq -cn --argjson r "$(cmd bad-one 02:30)" --argjson x "$f" '$r + $x')]"
  out=$(at "$T0" validate); rc=$?
  fd=$(printf '%s' "$f" | sed 's/\\n/[newline]/g') # ok and fail echo, and echo may expand \n
  [ "$rc" -eq 2 ] && ok "a routine with $fd fails validation" || fail "a routine with $fd passed: $out"
done
for f in '{"run": {"max_turns": 0}}' '{"run": {"max_usd": 0}}' '{"run": {"allowed_tools": "Read"}}' \
  '{"run": {"disallowed_tools": ["Bash", 3]}}' '{"run": {"context": ""}}' '{"run": {"model": "opus"}}' '{"run": {"skill": "session-start"}}'; do
  reg "$R" '"subscription"' "[$(cl digest '["Read"]' "$f")]"
  out=$(at "$T0" validate); rc=$?
  rejects digest "a claude -p routine with $f fails validation" "$out" "$rc" 'run'
done
# The design's own registries validate: D14's three routines, carrying every optional field the
# design names, and an api registry with its key file.
d14=$(jq -cn --arg c .claude '[
  {name: "nightly-checks", schedule: "02:30", catch_up: "once", run: {command: "sh \($c)/checks/run.sh"}, timeout_s: 1800,
   where: "local", machine: "any", output: "log", writes: false, enabled: true},
  {name: "flake-watch", schedule: "03:30", catch_up: "skip", machine: "darwin", run: {command: "sh scripts/routines/flake-watch.sh"}, timeout_s: 5400},
  {name: "morning-digest", schedule: "07:00", catch_up: "once", output: "show", timeout_s: 600,
   run: {prompt: "At most 15 lines: what merged, what is next, what waits on the owner.", context: "sh \($c)/skills/session-start/context.sh",
         role: "routine", allowed_tools: ["Read", "Grep", "Glob"], disallowed_tools: ["WebFetch"], max_turns: 6, max_usd: 0.50}}]')
reg "$R" '"subscription"' "$d14"
out=$(at "$T0" validate); rc=$?
expect_rc 0 "$rc" "D14's three routines validate, with every optional field the design names: $out"
reg "$R" '"api"' "$d14"
jq '.api_key_file = "~/keys/anthropic"' "$R" > "$R.new" && mv "$R.new" "$R"
out=$(at "$T0" validate); rc=$?
expect_rc 0 "$rc" "an api registry with its api_key_file validates: $out"
reg "$R" '"subscription"' "[$(cmd twin 02:30), $(cmd twin 03:30)]"
out=$(at "$T0" validate); rc=$?
rejects twin "a name used twice fails validation" "$out" "$rc" 'more than once'
reg "$R" '"subscription"' "[$(cmd nightly 02:30), 7]"
out=$(at "$T0" validate); rc=$?
case $out in *"routines[1]: is not an object"*) [ "$rc" -eq 2 ] && ok "an entry that is not an object fails, and so the whole registry" || fail "exit $rc" ;; *) fail "a non-object entry: $out" ;; esac
for x in '.version = 2' '.billing = "cloud"' '.api_key_file = 7' '.caps.subscription.five_hour_pct = 120' \
  '.caps.api.daily_usd = -1' '.caps.api.window_days = 0' '.caps.subscription.daily_runs = "x"' '.caps.extra = {}' \
  '.routines = {}' '.colour = "blue"'; do
  reg "$R" '"subscription"' "[$(cmd nightly 02:30)]"
  jq "$x" "$R" > "$R.new" && mv "$R.new" "$R"
  out=$(at "$T0" validate); rc=$?
  [ "$rc" -eq 2 ] && ok "a registry with $x fails validation" || fail "a registry with $x passed: $out"
done
reg "$R" '"subscription"' "[$(cmd nightly 02:30)]"
jq '._why = "a comment" | .routines[0]._note = "another"' "$R" > "$R.new" && mv "$R.new" "$R"
out=$(at "$T0" validate); rc=$?
expect_rc 0 "$rc" "a field starting with _ is a comment, anywhere: $out"
printf '{"version": 1, ' > "$R"
out=$(at "$T0" validate); rc=$?
case $out in *"does not parse"*) expect_rc 2 "$rc" "a registry that does not parse fails, saying so" ;; *) fail "an unparsable registry: $out" ;; esac
# Two documents, each valid alone, together two billing modes and one name twice: not a registry.
one() { printf '%s' "$BASE" | jq -c --argjson b "$1" --arg s "$2" '. + $b | .routines = [{name: "a", schedule: $s, run: {command: "echo hi"}}]'; }
printf '%s\n%s\n' "$(one '{"billing": "subscription"}' 02:30)" "$(one '{"billing": "api", "api_key_file": "~/k"}' 03:30)" > "$R"
for v in validate list; do
  out=$(at "$T0" "$v"); rc=$?
  case $out in *"one JSON object"*) expect_rc 2 "$rc" "$v over a registry of two JSON documents fails, saying it holds more than one" ;;
    *) fail "$v over a registry of two JSON documents: exit $rc: $(printf '%s' "$out" | tr '\n' '|')" ;; esac
done
# A key given twice: jq keeps the last, so each would read as a registry no one wrote (a routine
# gone, billing switched, a routine disabled). Written by hand: jq never writes one.
for dup in 'routines|{"version":1,"billing":null,"routines":[{"name":"a","schedule":"02:30","run":{"command":"echo hi"}}],"routines":[]}' \
  'billing|{"version":1,"billing":"api","api_key_file":"~/k","routines":[],"billing":"subscription"}' \
  'routines.0.enabled|{"version":1,"billing":null,"routines":[{"name":"a","schedule":"02:30","run":{"command":"echo hi"},"enabled":true,"enabled":false}]}'; do
  printf '%s\n' "${dup#*|}" > "$R"
  out=$(at "$T0" validate); rc=$?
  case $out in *"twice"*"${dup%%|*}"*) expect_rc 2 "$rc" "a registry giving ${dup%%|*} twice fails, naming it" ;;
    *) fail "a registry giving ${dup%%|*} twice: exit $rc: $out" ;; esac
done
rm -f "$R"
out=$(at "$T0" validate); rc=$?
case $out in *"no registry at $R"*) expect_rc 2 "$rc" "validate with no registry fails, naming where it looked" ;; *) fail "no registry: $out" ;; esac
out=$(at "$T0" list); rc=$?
case $out in *"no registry at $R"*) expect_rc 0 "$rc" "list with no registry says so, and exits 0" ;; *) fail "list, no registry: $out" ;; esac
reg "$R" '"subscription"' "[$(cmd nightly 02:30)]"
printf 'not json\n' > "$L/runs.jsonl"
out=$(at "$T0" list); rc=$?
case $out in *"does not parse"*) expect_rc 2 "$rc" "list over a run log that does not parse fails, saying so" ;; *) fail "a corrupt run log: $out" ;; esac
for rec in '{"record":"start","id":"x","name":"nightly","started":"yesterday"}' '[1,2]' '{"record":"end","id":"x","name":7}' \
  '{"record":"start","id":"x","name":"nightly"}' '{"record":"start","name":"nightly","started":1}' \
  '{"record":"end","id":"x","name":"nightly"}' '{"record":"end","name":"nightly","verdict":"ok"}' \
  '{"record":"refused","name":"nightly","reason":"quota"}' '{"record":"bogus","id":"x","name":"nightly","started":1}'; do
  printf '%s\n' "$rec" > "$L/runs.jsonl"
  out=$(at "$T0" list); rc=$?
  case $out in *"wrong shape"*) expect_rc 2 "$rc" "list over a run-log record of the wrong shape ($rec) fails, saying so" ;;
    *) fail "list over the run-log record $rec: exit $rc: $out" ;; esac
done
# Each record type has a well-formed case too: a refusal is read, and shown as the last verdict.
printf '%s\n' "{\"record\":\"refused\",\"name\":\"nightly\",\"at\":$T0,\"reason\":\"quota\"}" > "$L/runs.jsonl"
out=$(at $((T0 + 60)) list); rc=$?
expect_rc 0 "$rc" "list over a well-formed refused record"
row refused "$out" nightly '02:30 +command +2026-10-12 02:30 +refused \(quota\) at 2026-10-12 00:00'
# Two runs of one routine: the newer by time is the last verdict, whatever order the lines are in.
s1=$((T0 - 86400 + 9000)); s2=$((T0 + 9000))
old="{\"record\":\"start\",\"id\":\"n1\",\"name\":\"nightly\",\"started\":$s1}
{\"record\":\"end\",\"id\":\"n1\",\"name\":\"nightly\",\"verdict\":\"ok\"}"
new="{\"record\":\"start\",\"id\":\"n2\",\"name\":\"nightly\",\"started\":$s2}
{\"record\":\"end\",\"id\":\"n2\",\"name\":\"nightly\",\"verdict\":\"failed\"}"
for order in old-new new-old; do
  case $order in old-new) printf '%s\n%s\n' "$old" "$new" ;; *) printf '%s\n%s\n' "$new" "$old" ;; esac > "$L/runs.jsonl"
  out=$(at $((s2 + 60)) list)
  row "two runs, $order" "$out" nightly '02:30 +command +2026-10-13 02:30 +failed at 2026-10-12 02:30'
done
: > "$L/runs.jsonl"
out=$(at "$T0" list); rc=$?
expect_rc 0 "$rc" "list over an empty run log"
mkdir -p "$d/nojq"; for t in sh dirname; do ln -s "$(command -v "$t")" "$d/nojq/$t"; done
out=$(cd "$P" && PATH="$d/nojq" "$d/nojq/sh" .claude/routines.sh validate 2>&1); rc=$?
case $out in *"jq is not installed"*) expect_rc 2 "$rc" "with no jq, validate exits 2 saying so" ;; *) fail "no jq: $out" ;; esac

# ── [ROUTINES-10-S2] A sidecar registry cannot declare writes ────────────────────────────────────
SC="$d/home/.clauductor/projects/p1/routines.json"
wr() { jq -cn --argjson r "$(cmd "$1" 02:30)" '$r + {writes: true}'; }
side() { (cd "$d" && env TZ=UTC0 ROUTINES_NOW="$T0" sh "$P/.claude/routines.sh" --registry "$1" --repo "$P" validate) 2>&1; }
reg "$SC" '"subscription"' "[$(wr writer)]"
out=$(side "$SC"); rc=$?
rejects writer "[ROUTINES-10-S2] a sidecar registry with writes: true fails" "$out" "$rc" 'never writes to it'
reg "$SC" '"subscription"' "[$(cmd reader 02:30)]"
out=$(side "$SC"); rc=$?
case $out in *"a sidecar registry"*) expect_rc 0 "$rc" "[ROUTINES-10-S2] the same sidecar without writes validates, read as a sidecar" ;; *) fail "[ROUTINES-10-S2] sidecar: $out" ;; esac
reg "$R" '"subscription"' "[$(wr writer)]"
out=$(at "$T0" validate); rc=$?
expect_rc 0 "$rc" "[ROUTINES-10-S2] the repository's own registry may declare writes: $out"
# Outside and inside by identity, not by string: a sibling sharing the prefix is outside; the
# repository through a symlink, and another worktree of it, are inside.
reg "$P-keys/routines.json" '"subscription"' "[$(wr writer)]"
out=$(side "$P-keys/routines.json"); rc=$?
rejects writer "[ROUTINES-10-S2] a registry in a sibling sharing the repository's prefix is a sidecar" "$out" "$rc" 'never writes to it'
ln -s "$P" "$d/link"
out=$(side "$d/link/.claude/routines.json"); rc=$?
expect_rc 0 "$rc" "[ROUTINES-10-S2] the repository's registry reached through a symlink is the repository's: $out"
git -C "$P" worktree add -q "$d/wt" 2>/dev/null
reg "$d/wt/.claude/routines.json" '"subscription"' "[$(wr writer)]"
out=$(side "$d/wt/.claude/routines.json"); rc=$?
expect_rc 0 "$rc" "[ROUTINES-10-S2] a registry in another worktree is the repository's: $out"
# A git that cannot list the worktrees leaves "inside" undecided for a registry outside the
# repository itself: refused, never read as the repository's (its writes allowed, its log in the
# git dir) nor as a sidecar (its log beside a registry that may be in the tree).
mkdir -p "$d/badgit"
cat > "$d/badgit/git" <<EOF
#!/bin/sh
a=\$1; [ "\$a" = -C ] && a=\$3
[ "\$a" = worktree ] && { echo "fatal: this stand-in git cannot list worktrees" >&2; exit 128; }
exec "$(command -v git)" "\$@"
EOF
chmod +x "$d/badgit/git"
reg "$SC" '"subscription"' "[$(wr writer)]"
out=$(cd "$d" && env PATH="$d/badgit:$PATH" TZ=UTC0 sh "$P/.claude/routines.sh" --registry "$SC" --repo "$P" validate 2>&1); rc=$?
case $out in *"cannot tell"*"$SC"*"$P"*) expect_rc 2 "$rc" "when git cannot list the worktrees, a registry outside the repository is refused, naming it and the repository" ;;
  *) fail "when git cannot list the worktrees, a sidecar with writes: exit $rc: $out" ;; esac
out=$(cd "$P" && env PATH="$d/badgit:$PATH" TZ=UTC0 sh .claude/routines.sh validate 2>&1); rc=$?
expect_rc 0 "$rc" "when git cannot list the worktrees, the repository's own registry is still decided (no list needed): $out"
# git lists real paths, so a string match would also pass the cases above; a repository named
# through a symlink (here one git does not know) is where only identity holds.
mkdir -p "$d/plain/.claude"; ln -s "$d/plain" "$d/plainlink"
reg "$d/plain/.claude/routines.json" '"subscription"' "[$(wr writer)]"
out=$(cd "$d" && env TZ=UTC0 sh "$P/.claude/routines.sh" --registry "$d/plain/.claude/routines.json" --repo "$d/plainlink" validate 2>&1); rc=$?
expect_rc 0 "$rc" "[ROUTINES-10-S2] a repository named through a symlink holds its own registry, by identity: $out"

# ── The runner's repository, wherever the runner sits ───────────────────────────────────────────
# The plugin's copy sits outside any .claude/: it takes --repo, else the repository the command runs
# in, never a ROOT it inherits (a caller's, or the one a checks run.sh exports). The project's copy
# takes the repository it sits in.
mkdir -p "$d/plugin-root" "$P/sub"; cp "$P/.claude/routines.sh" "$d/plugin-root/routines.sh"
O="$d/other"; new_repo "$O"; reg "$O/.claude/routines.json" '"subscription"' "[$(cmd elsewhere 02:30)]"
reg "$R" '"subscription"' "[$(cmd nightly 02:30)]"
out=$(cd "$P/sub" && env ROOT="$O" TZ=UTC0 sh "$d/plugin-root/routines.sh" validate 2>&1); rc=$?
case $out in "ok $R:"*) expect_rc 0 "$rc" "a runner outside .claude/ reads the registry of the repository it runs in, not an inherited ROOT's" ;;
  *) fail "a runner outside .claude/ run in $P/sub with ROOT=$O: exit $rc: $out" ;; esac
out=$(cd "$P/sub" && env ROOT="$O" TZ=UTC0 sh "$d/plugin-root/routines.sh" --repo "$O" validate 2>&1); rc=$?
case $out in "ok $O/.claude/routines.json:"*) expect_rc 0 "$rc" "a runner outside .claude/ reads another repository's registry when --repo names it" ;;
  *) fail "a runner outside .claude/ with --repo $O: exit $rc: $out" ;; esac
out=$(cd "$d" && env ROOT="$O" TZ=UTC0 sh "$P/.claude/routines.sh" validate 2>&1); rc=$?
case $out in "ok $R:"*) expect_rc 0 "$rc" "the project's copy reads its own repository's registry, wherever it runs and whatever ROOT says" ;;
  *) fail "the project's copy run in $d with ROOT=$O: exit $rc: $out" ;; esac

# ── A check that forgets its fake claude fails, and spends nothing (lib.sh, D13) ────────────────
# (Its scenario is cited where the runner itself calls claude, task 4.1; this is the guard it
# rests on.) A check of its own, sourcing this lib.sh, calls `claude -p` with no fake. Next on its
# PATH sits a stand-in for the real claude, which records any call: the refusing claude must
# answer instead, the check must fail naming it, and the stand-in must never run.
mkdir -p "$d/real-bin"
cat > "$d/real-bin/claude" <<EOF
#!/bin/sh
echo "\$*" >> "$d/real-claude.calls"
echo '{"type":"result","subtype":"success","is_error":false,"total_cost_usd":0.5,"result":"spent"}'
EOF
chmod +x "$d/real-bin/claude"
cat > "$d/forgetful.sh" <<'EOF'
. "$CHECKS_LIB"
out=$(claude -p "summarise the night" --output-format json </dev/null 2>&1); rc=$?
echo "claude answered rc=$rc: $out"
finish
EOF
# forget [VAR=VALUE...]: the forgetful check, on this lib.sh (the plugin's needs CLAUDUCTOR_FW).
forget() { env PATH="$d/real-bin:$PATH" ROOT="$ROOT" CHECKS_LIB="$lib" ${CLAUDUCTOR_FW:+"CLAUDUCTOR_FW=$CLAUDUCTOR_FW"} "$@" sh "$d/forgetful.sh" 2>&1; }
out=$(forget); rc=$?
[ ! -e "$d/real-claude.calls" ] && ok "a check that forgets its fake never starts the claude on its PATH" \
  || fail "the claude on the check's PATH was started: $(cat "$d/real-claude.calls")"
case $out in
  *"rc=97"*) ok "the refusing claude answers a check that forgets its fake (exit 97)" ;;
  *) fail "the refusing claude did not answer: $out" ;;
esac
if [ "$rc" -ne 0 ] && printf '%s\n' "$out" | grep -q '^FAIL .*refusing claude'; then
  ok "the check that reached it fails, naming the refusing claude"
else
  fail "the check that reached the refusing claude exited $rc: $out"
fi
# With no scratch directory there is nowhere to put the refusing claude: the check must fail at
# once, never run on with whatever claude is next on its PATH.
out=$(forget TMPDIR=/nonexistent-dir); rc=$?
if [ "$rc" -ne 0 ] && [ ! -e "$d/real-claude.calls" ] && printf '%s\n' "$out" | grep -q '^FAIL .*scratch'; then
  ok "a check whose scratch directory cannot be made fails at once, and never reaches the claude on its PATH"
else
  fail "a check with no scratch directory (TMPDIR=/nonexistent-dir) exited $rc$([ -e "$d/real-claude.calls" ] && echo ', reaching the claude on its PATH'): $out"
fi
# A ':' in TMPDIR splits the stub's PATH entry in two, so PATH would find another claude: what is
# checked is that PATH finds the refusing one, not only that the file exists.
mkdir -p "$d/co:lon"
out=$(forget TMPDIR="$d/co:lon"); rc=$?
if [ "$rc" -ne 0 ] && [ ! -e "$d/real-claude.calls" ] && printf '%s\n' "$out" | grep -q '^FAIL .*refusing claude'; then
  ok "a check whose PATH does not find the refusing claude (a ':' in TMPDIR) fails at once, and never reaches the claude on its PATH"
else
  fail "a check with a ':' in TMPDIR exited $rc$([ -e "$d/real-claude.calls" ] && echo ', reaching the claude on its PATH'): $out"
fi

# ── This project's own registry (the process check) ─────────────────────────────────────────────
if [ -f "$ROOT/.claude/routines.json" ]; then
  out=$(sh "$CLAUDUCTOR_FW/routines.sh" validate 2>&1); rc=$?
  expect_rc 0 "$rc" "this project's registry: $out"
else
  ok "this project has no .claude/routines.json, so no routine to validate"
fi

finish
