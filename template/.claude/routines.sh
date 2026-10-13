#!/bin/sh
# routines.sh: the routines runner (OPS-42, specs/routines). Scheduled work named in a registry, run
# one at a time on this machine, each run bounded and recorded, and every outcome reported at the
# next session start.
#
#   sh .claude/routines.sh [--registry FILE] [--repo DIR] <verb>
#
#   validate   the registry's checks, shared by every verb that reads routines and by
#              checks/routines.sh: exit 0 naming what it holds, or 2 naming every fault
#   list       each routine: its schedule, kind, next due time and last verdict, or the runner or
#              machine it waits for when it is not this one's
#
# THE REGISTRY is .claude/routines.json, the project's file (`clauductor update` creates it when
# missing and never overwrites it). --repo names the repository the routines run in (default: the
# one this script sits in, <repo>/.claude/routines.sh; for the plugin's copy, which sits in no
# .claude/, the repository the command runs in; never an inherited ROOT), --registry another
# registry. A registry outside the repository (an attached repository's sidecar config, design D12)
# keeps its run log beside itself and may never declare `writes: true`. A malformed entry is an
# error, not a shorter list: nothing runs from a registry that does not validate.
#
# THE SCHEDULE (design D3), in local time, read at the ticker's 5-minute grain:
#   HH:MM           daily                                   02:30
#   <days> HH:MM    Mon,Thu (a list) or Mon-Fri (a range)   Mon-Fri 18:15
#   every <N>m|h    at least 5 minutes, from the last run's start
# A slot is due while no run has started since it: with `catch_up: once` (the default) at the next
# tick however late, a "catch-up" when that is more than 15 minutes after the slot; with
# `catch_up: skip` only within those 15 minutes, else it waits for the next slot. A routine that
# never ran is due at its latest slot. A refused run counts as the slot's run, so a cap is not
# retried every tick.
#
# THE RUN LOG is <git common dir>/clauductor/routines/runs.jsonl, shared by every worktree and never
# committed; beside the registry, in routines/, for a sidecar (design D2). One JSON record per line:
# {record: "start", id, name, started, ...}, {record: "end", id, name, verdict, ...} and
# {record: "refused", name, at, reason, ...}; times are unix seconds.
#
# POSIX sh and jq (ADR-0006): no clauductor, no panel. Every tool is found on PATH and PATH is never
# set here, so a check's shims see every process the runner starts. ROUTINES_NOW (unix seconds)
# stands in for the clock in the checks.
#
# Exit: 0 done; 2 the registry is missing, invalid or unreadable (jq missing included); 64 usage.

usage() { echo "usage: routines.sh [--registry FILE] [--repo DIR] validate|list" >&2; exit 64; }
die() { echo "routines: $*" >&2; exit 2; }
# abs PATH: PATH made absolute against the directory this runs in (its logical pwd), not resolved.
abs() { case $1 in /*) printf '%s' "$1" ;; *) printf '%s/%s' "$(pwd)" "$1" ;; esac; }

# Where this script sits. Not named ROOT: the plugin's packager gives a script's ROOT to an
# inherited ROOT when set (framework/internal/plugin/rewrite.go), and a ROOT exported by a caller,
# or by a checks run.sh, must never steer an unattended runner to another tree.
here=$(cd "$(dirname "$0")" 2>/dev/null && pwd)
repo=""
registry=""
while [ $# -gt 0 ]; do
  case $1 in
    --registry) [ $# -ge 2 ] && [ -n "$2" ] || usage; registry=$(abs "$2"); shift 2 ;;
    --repo)
      [ $# -ge 2 ] && [ -n "$2" ] || usage
      repo=$(cd "$2" 2>/dev/null && pwd) || die "--repo $2: no such directory"
      shift 2 ;;
    -*) usage ;;
    *) break ;;
  esac
done
[ $# -gt 0 ] || usage
verb=$1; shift
if [ -z "$repo" ]; then
  case $here in
    */.claude) repo=${here%/.claude} ;;
    *) repo=$(git rev-parse --show-toplevel 2>/dev/null) || repo=$(pwd) ;;
  esac
fi
[ -n "$registry" ] || registry="$repo/.claude/routines.json"
command -v jq >/dev/null 2>&1 || die "jq is not installed, so no routine can be read or run"
now=${ROUTINES_NOW:-$(date +%s)}
case $now in ''|*[!0-9]*) die "ROUTINES_NOW must be unix seconds, not '$now'" ;; esac
case $(uname -s) in Darwin) machine=darwin ;; Linux) machine=linux ;; *) machine=$(uname -s) ;; esac

nl='
'
# inside_repo FILE: 0 when FILE's directory, or one of its physical ancestors, IS the repository
# itself, its git common dir, or the top level of a worktree git lists for it; 1 when none is; 2
# when it cannot tell (FILE's directory does not resolve, or git knows the repository but fails to
# list its worktrees). Identity (-ef), never a string prefix, so a case-insensitive file system, a
# symlinked parent and a sibling sharing the prefix (<top>-keys/) all come out right, in any shell
# (design D16). The one function that decides "inside". The list is asked for only when the first
# two do not decide, and walked with builtins, not a here-document, whose temp file can fail to be
# made and would then read as "outside". A listed worktree whose directory is gone (prunable) is
# never the same file as anything, so it decides nothing.
inside_repo() {
  _ir_dir=$(cd -P "$(dirname "$1")" 2>/dev/null && pwd -P) || return 2
  _ir_common=$(git -C "$repo" rev-parse --path-format=absolute --git-common-dir 2>/dev/null) || _ir_common=""
  _ir_d=$_ir_dir
  while :; do
    [ "$_ir_d" -ef "$repo" ] && return 0
    [ -n "$_ir_common" ] && [ "$_ir_d" -ef "$_ir_common" ] && return 0
    [ "$_ir_d" = / ] && break
    _ir_d=${_ir_d%/*}
    [ -n "$_ir_d" ] || _ir_d=/
  done
  [ -n "$_ir_common" ] || return 1
  _ir_list=$(git -C "$repo" worktree list --porcelain 2>/dev/null) || return 2
  _ir_d=$_ir_dir
  while :; do
    _ir_rest=$_ir_list
    while [ -n "$_ir_rest" ]; do
      _ir_line=${_ir_rest%%"$nl"*}
      case $_ir_rest in *"$nl"*) _ir_rest=${_ir_rest#*"$nl"} ;; *) _ir_rest="" ;; esac
      case $_ir_line in "worktree "*) [ "$_ir_d" -ef "${_ir_line#worktree }" ] && return 0 ;; esac
    done
    [ "$_ir_d" = / ] && return 1
    _ir_d=${_ir_d%/*}
    [ -n "$_ir_d" ] || _ir_d=/
  done
}

# A registry outside the repository is a sidecar (design D12). Undecided is refused: read as the
# repository's, a sidecar could declare writes and log into the git dir; read as a sidecar, a
# registry in the tree would keep its log there. A missing registry has no place to decide.
sidecar=""
if [ -e "$registry" ]; then
  inside_repo "$registry"
  case $? in
    0) ;;
    1) sidecar=1 ;;
    *) die "cannot tell whether the registry $registry lies inside the repository $repo (git could not list its worktrees), so nothing in it runs" ;;
  esac
fi

# The run log's directory (design D2): beside a sidecar registry, else in the git common dir, which
# every worktree shares and git never commits or cleans.
logdir() {
  if [ -n "$sidecar" ]; then printf '%s/routines' "${registry%/*}"; return 0; fi
  _ld=$(git -C "$repo" rev-parse --path-format=absolute --git-common-dir 2>/dev/null) || return 1
  printf '%s/clauductor/routines' "$_ld"
}

# ── The schedule grammar and its next-due arithmetic (jq), shared by every verb ──────────────────
# Local time without a local mktime: `wall` reads a unix time as the local wall clock counted as if
# it were UTC, so days and slots are whole multiples of 86400 there, and `unwall` turns a wall
# reading back into unix time (twice, so a DST change between the two is taken into account).
# Patterns end in \z, not $: jq's $ also matches before a final newline, and a name or schedule
# ending in one would pass, then split a list row, a run directory and a health line.
JQ_LIB='
def dayno: {"Mon": 1, "Tue": 2, "Wed": 3, "Thu": 4, "Fri": 5, "Sat": 6, "Sun": 7};
def str: type == "string" and length > 0;
def kebab: str and test("^[a-z0-9]+(-[a-z0-9]+)*\\z");
def kind: if (.run | type) != "object" then null
  elif (.run | has("command")) then "command"
  elif (.run | has("prompt") or has("skill")) then "claude"
  else null end;
def hhmm:
  if test("^[0-9]{2}:[0-9]{2}\\z") | not then {err: "the time \"\(.)\" is not HH:MM, two digits each"}
  elif (.[0:2] | tonumber) > 23 then {err: "the hour in \"\(.)\" is not 00 to 23"}
  elif (.[3:5] | tonumber) > 59 then {err: "the minute in \"\(.)\" is not 00 to 59"}
  else {tod: ((.[0:2] | tonumber) * 3600 + (.[3:5] | tonumber) * 60)} end;
def days:
  if test("^[A-Z][a-z]{2}-[A-Z][a-z]{2}\\z") then
    (split("-") | map(dayno[.])) as $r
    | if ($r | any(. == null)) then {err: "a day in \"\(.)\" is not one of Mon Tue Wed Thu Fri Sat Sun"}
      elif $r[0] >= $r[1] then {err: "the range \"\(.)\" does not run forward within Mon to Sun"}
      else {days: [range($r[0]; $r[1] + 1)]} end
  elif test("^[A-Z][a-z]{2}(,[A-Z][a-z]{2})*\\z") then
    (split(",") | map(dayno[.])) as $l
    | if ($l | any(. == null)) then {err: "a day in \"\(.)\" is not one of Mon Tue Wed Thu Fri Sat Sun"}
      elif ($l | unique | length) != ($l | length) then {err: "\"\(.)\" names a day twice"}
      else {days: ($l | sort)} end
  else {err: "the days \"\(.)\" are not a list (Mon,Thu) or a range (Mon-Fri) of Mon Tue Wed Thu Fri Sat Sun"} end;
def sched:
  if type != "string" then {err: "is missing, or not a string"}
  elif test("^every ") then
    (capture("^every (?<n>[1-9][0-9]*)(?<u>[mh])\\z") // null) as $c
    | if $c == null then {err: "\"\(.)\" is not every <N>m or every <N>h"}
      elif $c.u == "m" and ($c.n | tonumber) < 5 then {err: "\"\(.)\" is more often than every 5m, the ticker grain"}
      else {kind: "every", secs: (($c.n | tonumber) * (if $c.u == "m" then 60 else 3600 end))} end
  else split(" ") as $w
    | if ($w | length) == 1 then ($w[0] | hhmm) + {kind: "at", days: [range(1; 8)]}
      elif ($w | length) == 2 then ($w[0] | days) + ($w[1] | hhmm) + {kind: "at"}
      elif ($w | length) == 5 then {err: "\"\(.)\" is a cron line; a schedule is HH:MM, <days> HH:MM or every <N>m|h"}
      else {err: "\"\(.)\" is not HH:MM, <days> HH:MM or every <N>m|h"} end
  end;
def wall: localtime | mktime;
def unwall: . as $w | ($w - (($w | wall) - $w)) as $t | $w - (($t | wall) - $t);
def wday: (. / 86400 | floor) + 3 | . % 7 + 1;
def stamp: if type == "number" then localtime | strftime("%Y-%m-%d %H:%M") else "an unknown time" end;
def due($p; $now; $last; $catch):
  def verdict($s; $since; $next):
    if $s != null and ($catch == "once" or $since < 900)
    then {due: true, slot: $s, trigger: (if $since < 900 then "schedule" else "catch-up" end)}
    else {due: false, next: $next} end;
  if $p.kind == "every" then
    if $last == null then {due: true, slot: $now, trigger: "schedule"}
    else ((($now - $last) / $p.secs) | floor) as $k
      | verdict(if $k >= 1 then $last + $k * $p.secs else null end;
                $now - $last - $k * $p.secs;
                $last + (([$k, 0] | max) + 1) * $p.secs)
    end
  else ($now | wall) as $wn | (($wn / 86400) | floor) as $dn
    | ([range(0; 8) | ($dn - .) * 86400 + $p.tod | select(. <= $wn)
        | select(wday as $d | $p.days | any(. == $d))] | first) as $ws
    | ([range(0; 9) | ($dn + .) * 86400 + $p.tod | select(. > $wn)
        | select(wday as $d | $p.days | any(. == $d))] | first) as $wnext
    | verdict(if $ws != null and ($last == null or ($last | wall) < $ws) then $ws | unwall else null end;
              if $ws != null then $wn - $ws else 0 end;
              $wnext | unwall)
  end;
'

# ── Validation (jq): every fault of the registry, one per line ─────────────────────────────────
JQ_VALIDATE='
def known($k): keys - $k | map(select(startswith("_") | not));
def whole: type == "number" and . == floor and . >= 0;
def readonly: ["Read", "Grep", "Glob", "WebFetch", "WebSearch"];
def skilldirs: $skills | split(" ") | map(select(length > 0));
def capcheck($block; $spec):
  .caps[$block] as $b
  | if $b == null then empty
    elif ($b | type) != "object" then "caps.\($block): is not an object"
    else ($b | known($spec | keys)[] | "caps.\($block): unknown field \"\(.)\""),
      ($spec | to_entries[] | .key as $f | .value as $t | select($b | has($f)) | $b[$f] as $v
        | if $t == "pct" and ($v | type == "number" and . > 0 and . <= 100) then empty
          elif $t == "whole" and ($v | whole) then empty
          elif $t == "runs" and ($v == null or ($v | whole)) then empty
          elif $t == "days" and ($v | whole) and $v >= 1 then empty
          elif $t == "usd" and ($v | type == "number" and . >= 0) then empty
          else "caps.\($block).\($f): \($v | tojson) is not \({pct: "a percentage above 0 and at most 100", whole: "a whole number, 0 or more", runs: "null or a whole number, 0 or more", days: "a whole number of days, 1 or more", usd: "a dollar amount, 0 or more"}[$t])" end)
    end;
def claude_faults($at; $r):
  ($r.run | known(["prompt", "skill", "context", "role", "allowed_tools", "disallowed_tools", "max_turns", "max_usd"])[]
    | "\($at): run: unknown field \"\(.)\" for a claude -p routine"),
  (if ($r.run | has("prompt")) and ($r.run | has("skill")) then "\($at): run: has both a prompt and a skill; it is one or the other"
   elif ($r.run | has("prompt")) and ($r.run.prompt | str | not) then "\($at): run.prompt: is not a prompt"
   elif ($r.run | has("skill")) and (($r.run.skill | kebab | not) or (any(skilldirs[]; . == $r.run.skill) | not))
   then "\($at): run.skill: \($r.run.skill | tojson) is not a skill of this repository (no directory \($repo)/.claude/skills/\($r.run.skill)/)"
   else empty end),
  (if ($r.run | has("context")) and ($r.run.context | str | not) then "\($at): run.context: is not a command" else empty end),
  (if ($r.run.role | str | not) then "\($at): run.role: is missing; a claude -p routine names a role in .claude/model-roles.json"
   elif $roles == null then "\($at): run.role: \"\($r.run.role)\" cannot be checked: \($rolesfile) is missing or has no roles"
   elif any($roles[]; . == $r.run.role) | not then "\($at): run.role: \"\($r.run.role)\" is not a role in \($rolesfile)"
   else empty end),
  (if ($r.run.allowed_tools | type) != "array" or ($r.run.allowed_tools | all(str) | not)
   then "\($at): run.allowed_tools: is missing, or not a list of tool names"
   else $r.run.allowed_tools[] as $t
     | if $t | startswith("mcp__") then "\($at): run.allowed_tools: \"\($t)\" is an MCP tool, and routines load no MCP server (--strict-mcp-config), so it could never be reached"
       elif $r.writes == true or any(readonly[]; . == $t) then empty
       else "\($at): run.allowed_tools: \"\($t)\" is not a read-only tool (\(readonly | join(", "))), so it needs \"writes\": true" end
   end),
  (if ($r.run | has("disallowed_tools")) and (($r.run.disallowed_tools | type) != "array" or ($r.run.disallowed_tools | all(str) | not))
   then "\($at): run.disallowed_tools: is not a list of tool names" else empty end),
  (if $r.run.max_turns | (type == "number" and . == floor and . >= 1) then empty
   else "\($at): run.max_turns: \($r.run.max_turns | tojson) is not a whole number of turns, 1 or more" end),
  (if ($r.run | has("max_usd")) and ($r.run.max_usd | (type == "number" and . > 0) | not)
   then "\($at): run.max_usd: \($r.run.max_usd | tojson) is not a dollar amount above 0" else empty end);
def routine_faults($i):
  . as $r
  | "routines[\($i)]\(if type == "object" and (.name | str) then " \(.name | tojson)" else "" end)" as $at
  | if type != "object" then "\($at): is not an object"
    else
      (known(["name", "schedule", "run", "where", "machine", "timeout_s", "catch_up", "output", "writes", "enabled"])[]
        | "\($at): unknown field \"\(.)\""),
      (if .name | kebab then empty else "\($at): name: \(.name | tojson) is not kebab-case (a-z and 0-9, words joined by -)" end),
      (.schedule | sched | if .err then "\($at): schedule: \(.err)" else empty end),
      (if has("where") | not then empty elif .where | kebab then empty
       else "\($at): where: \(.where | tojson) is not the name of a runner (this one is local)" end),
      (if has("machine") | not then empty elif .machine | IN("any", "darwin", "linux") then empty
       else "\($at): machine: \(.machine | tojson) is not any, darwin or linux" end),
      (if has("timeout_s") | not then empty elif .timeout_s | (type == "number" and . == floor and . >= 1) then empty
       else "\($at): timeout_s: \(.timeout_s | tojson) is not a whole number of seconds, 1 or more" end),
      (if has("catch_up") | not then empty elif .catch_up | IN("once", "skip") then empty
       else "\($at): catch_up: \(.catch_up | tojson) is not once or skip" end),
      (if has("output") | not then empty elif .output | IN("log", "show") then empty
       else "\($at): output: \(.output | tojson) is not log or show" end),
      (if has("writes") | not then empty elif .writes | type == "boolean" then empty
       else "\($at): writes: \(.writes | tojson) is not true or false" end),
      (if has("enabled") | not then empty elif .enabled | type == "boolean" then empty
       else "\($at): enabled: \(.enabled | tojson) is not true or false" end),
      (if $sidecar != "" and .writes == true
       then "\($at): writes: a routine of an attached repository never writes to it (this registry lies outside \($repo))" else empty end),
      (if (.run | type) != "object" then "\($at): run: is missing, or not an object ({command} or a claude -p run)"
       elif (.run | has("command")) and (.run | has("prompt") or has("skill")) then "\($at): run: has a command and a prompt or skill; it is one or the other"
       elif kind == null then "\($at): run: has neither a command nor a prompt or skill"
       elif kind == "command" then
         (.run | known(["command"])[] | "\($at): run: unknown field \"\(.)\" for a command routine"),
         (if .run.command | str then empty else "\($at): run.command: is not a command" end)
       else claude_faults($at; $r) end)
    end;
. as $reg
| (if (.routines | type) == "array" then .routines else [] end) as $rs
| (known(["version", "billing", "api_key_file", "caps", "routines"])[] | "unknown field \"\(.)\" (a field starting with _ is a comment)"),
  (if .version == 1 then empty else "version: \(.version | tojson) is not 1, the version this runner reads" end),
  (if .billing | IN(null, "subscription", "api") then empty
   else "billing: \(.billing | tojson) is not subscription or api: run routines.sh setup --billing subscription|api" end),
  (if .api_key_file == null or (.api_key_file | str) then empty else "api_key_file: \(.api_key_file | tojson) is not a path" end),
  (if .billing == "api" and .api_key_file == null
   then "billing is api but api_key_file is not set: run routines.sh setup --billing api --key-file PATH" else empty end),
  (if has("caps") | not then empty
   elif (.caps | type) != "object" then "caps: is not an object"
   else (.caps | known(["subscription", "api"])[] | "caps: unknown field \"\(.)\""),
     capcheck("subscription"; {five_hour_pct: "pct", seven_day_pct: "pct", unknown_quota_daily_runs: "whole", daily_runs: "runs"}),
     capcheck("api"; {daily_usd: "usd", window_usd: "usd", window_days: "days", daily_runs: "runs"})
   end),
  (if (.routines | type) == "array" then empty else "routines: is missing, or not a list" end),
  ($rs | to_entries[] | .key as $i | .value | routine_faults($i)),
  ([$rs[] | objects | .name | strings] | group_by(.) | map(select(length > 1) | .[0])[]
    | "routines: the name \"\(.)\" is used more than once"),
  (if $reg.billing == null then
     ($rs[] | objects | select(.enabled != false and kind == "claude")
      | "routine \"\(.name)\": runs claude -p, and billing is not declared: run routines.sh setup --billing subscription|api")
   else empty end)
'

# ── The run log (jq), read in one pass ──────────────────────────────────────────────────────────
# The log is never pruned (design D2), so it is streamed once (`inputs`, under jq -n), never slurped:
# each record's shape is checked as it passes, and only each routine's latest attempt (a start or a
# refusal, by time) and that start's verdict are kept. Due-ness is read from these records, so a
# malformed one is an error that stops the pass, never missing history. list reads it so; the tick
# and the health line are to read it the same way.
JQ_RUNS='
def record_ok: type == "object" and (.name | type) == "string"
  and (if .record == "start" then (.id | type) == "string" and (.started | type) == "number"
       elif .record == "end" then (.id | type) == "string" and (.verdict | type) == "string"
       elif .record == "refused" then (.at | type) == "number"
       else false end)
  and all(.started, .ended, .at; . == null or type == "number");
def attempt_time: .started // .at;
def runs_by_name:
  reduce inputs as $x ({};
    if ($x | record_ok | not) then error("wrong-shape record: \($x | tojson | .[0:200])")
    elif $x.record == "end" then
      if .[$x.name].att.record == "start" and .[$x.name].att.id == $x.id then .[$x.name].verdict = $x.verdict else . end
    elif .[$x.name].att == null or ($x | attempt_time) >= (.[$x.name].att | attempt_time) then .[$x.name] = {att: $x, verdict: null}
    else . end);
'

# ── list (jq): one row per routine ─────────────────────────────────────────────────────────────
JQ_LIST='
runs_by_name as $by
| [ $reg[0].routines[] | . as $r
    | $by[$r.name] as $m
    | $m.att as $att
    | (if $att == null then "never ran"
       elif $att.record == "refused" then "refused (\($att.reason // "no reason")) at \($att.at | stamp)"
       elif $m.verdict == null then "running since \($att.started | stamp)"
       else "\($m.verdict) at \($att.started | stamp)" end) as $last
    | (if $r.enabled == false then "disabled"
       elif ($r.where // "local") != "local" then "waits for the \($r.where) runner"
       elif ($r.machine // "any") != "any" and $r.machine != $machine then "waits for a \($r.machine) machine"
       else due($r.schedule | sched; $now; (if $att == null then null else $att.started // $att.at end); $r.catch_up // "once")
         | if .due then "now (\(if .trigger == "catch-up" then "catch-up of" else "slot" end) \(.slot | stamp))"
           else .next | stamp end
       end) as $next
    | [$r.name, $r.schedule, (if ($r | kind) == "command" then "command" else "claude -p" end), $next, $last] ]
| if length == 0 then "no routines in \($registry)"
  else ([["NAME", "SCHEDULE", "KIND", "NEXT DUE", "LAST"]] + .) as $rows
    | [range(0; 5) as $c | $rows | map(.[$c] | length) | max] as $w
    | $rows[] | [range(0; 5) as $c | .[$c] + (" " * ($w[$c] - (.[$c] | length)))] | join("  ") | sub(" +$"; "")
  end
'

# faults: every fault of the registry, one per line; nothing when it validates.
faults() {
  [ -f "$registry" ] || { echo "no registry at $registry"; return 0; }
  jq -s . "$registry" >/dev/null 2>&1 || { echo "$registry does not parse as JSON"; return 0; }
  # Exactly one document: jq reads documents back to back, and two valid ones would each pass alone
  # while together naming two billing modes, or one routine twice.
  jq -se 'length == 1 and (.[0] | type == "object")' "$registry" >/dev/null 2>&1 \
    || { echo "$registry must hold exactly one JSON object; it holds $(jq -s 'length' "$registry") JSON document(s), or something else"; return 0; }
  # A key given twice: jq keeps the last, so the registry would read as one no one wrote (a routine
  # gone, billing switched, a routine disabled). The file's own events, streamed, differ from those
  # of the value jq makes of it only when a key repeats, and first at that key.
  _twice=$(jq -rn --stream '[inputs] as $r | [fromstream($r[]) | tostream] as $o
    | if $r == $o then empty
      else first(range(0; $r | length) as $i | select($r[$i] != $o[$i]) | [$r[$i][0], (($o[$i] // [[]])[0])]) as [$p, $q]
        | reduce range(0; [($p | length), ($q | length)] | min) as $k ([]; if length == $k and $p[$k] == $q[$k] then . + [$p[$k]] else . end)
        | if . == [] then "the top level" else map(tostring) | join(".") end
      end' "$registry" 2>/dev/null) || { echo "$registry cannot be read for a repeated key"; return 0; }
  [ -z "$_twice" ] || { echo "$registry gives a key twice, at $_twice; jq keeps only the last, so it would read as a registry no one wrote"; return 0; }
  _roles=$(jq -c '.roles | keys' "$repo/.claude/model-roles.json" 2>/dev/null) || _roles=null
  _skills=""
  for _s in "$repo"/.claude/skills/*/; do
    [ -d "$_s" ] || continue
    _s=${_s%/}; _skills="$_skills ${_s##*/}"
  done
  jq -r --argjson roles "${_roles:-null}" --arg rolesfile "$repo/.claude/model-roles.json" --arg skills "$_skills" \
    --arg sidecar "$sidecar" --arg repo "$repo" "$JQ_LIB $JQ_VALIDATE" "$registry"
}

# validate: 0 when the registry validates; else every fault on stderr, and 2.
validate() {
  _f=$(faults) || die "cannot validate $registry: jq failed on it"
  [ -z "$_f" ] && return 0
  echo "routines: $registry does not validate, so none of its routines runs:" >&2
  printf '%s\n' "$_f" | sed 's/^/  - /' >&2
  return 2
}

case $verb in
  validate)
    [ $# -eq 0 ] || usage
    validate || exit 2
    jq -r --arg reg "$registry" --arg sidecar "$sidecar" '"ok \($reg): \(.routines | length) routine(s), \([.routines[] | select(.enabled != false)] | length) enabled, billing \(.billing // "not declared")\(if $sidecar != "" then ", a sidecar registry" else "" end)"' "$registry"
    ;;
  list)
    [ $# -eq 0 ] || usage
    [ -f "$registry" ] || { echo "no routines: no registry at $registry"; exit 0; }
    validate || exit 2
    log=$(logdir) || die "$repo is not a git repository, so the run log has nowhere to live"
    set --
    [ -f "$log/runs.jsonl" ] && set -- "$log/runs.jsonl"
    _rows=$(jq -rn --slurpfile reg "$registry" --argjson now "$now" --arg machine "$machine" --arg registry "$registry" \
      "$JQ_LIB $JQ_RUNS $JQ_LIST" "$@" </dev/null 2>&1) || case $_rows in
        *"wrong-shape record"*) die "the run log $log/runs.jsonl holds a record of the wrong shape: each has a string name and a record of start (string id, numeric started), end (string id, string verdict) or refused (numeric at), its times in unix seconds; first: ${_rows#*wrong-shape record: }" ;;
        *) die "the run log $log/runs.jsonl does not parse: $_rows" ;;
      esac
    printf '%s\n' "$_rows"
    ;;
  *) usage ;;
esac
