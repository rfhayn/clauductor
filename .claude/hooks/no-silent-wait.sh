#!/bin/sh
# Refuses to let unattended work stop on a wait that will wake nobody. One script, registered for
# six events and keyed by the payload's hook_event_name:
#   PreToolUse (Bash)             refuses a subagent's or teammate's shell `&` (nothing tracks it)
#   PostToolUse (Bash|Monitor|Agent|Task)  records each background task or agent an agent starts
#   SubagentStart                 records the subagent as running
#   SubagentStop, TeammateIdle    refuse the agent's turn ending while a task IT started still runs
#   PreToolUse (AskUserQuestion)  refuses the main session's blocking question while anything runs
#
# WHY (OPS-90). Unattended work stalled for hours three times with the work done. 2026-10-03 and
# 2026-10-18: an agent's command outlived its Bash timeout, was moved to the background, and the
# agent ended its turn to wait; nothing woke it. OPS-41's no-background-gate.sh holds a GATE in the
# foreground, and the next stall came through a check run. 2026-10-18 again: the orchestrator asked
# the owner a question (AskUserQuestion) at midnight, which froze it while a lane's request sat
# unread for 5 h. A rule in a prompt was there each time: an instruction is not a control.
#
# WHAT CLAUDE CODE SENDS (probed on 2.1.302 with `claude -p`, 2026-10-18; OPS-90's report):
#   - PostToolUse for a Bash call that went to the background (run_in_background, or a foreground
#     call that outlived its timeout: in some contexts such a call is killed instead, and makes no
#     task) has tool_response.backgroundTaskId; a Monitor's has taskId. A subagent's payload has
#     agent_id; the main session's has none, and its tasks are recorded as `_main`.
#   - A foreground call that ran to its timeout can leave a regular output file with no end line
#     behind (seen in the session that built this): only RECORDED tasks count, never the directory.
#   - SubagentStop has background_tasks, but they are the SESSION's tasks: a subagent's stop listed
#     a shell the main session had started. So only a task recorded for this agent_id holds its turn.
#   - Refused (exit 2), the subagent kept working and its task ran to its end. Allowed, the task
#     was killed as the turn ended ("[killed]"): its result was lost, not merely unread.
#   - A task's output is <dirname of scratchpad_dir>/tasks/<id>.output: a regular file for a shell
#     (a symlink for an agent), whose last line is "[exited with code N]" or "[killed]" once it ended.
#     That line is what ends a task here, whatever background_tasks' status says.
#   - An Agent call launched in the background (a subagent has no foreground choice in an
#     interactive session) has tool_response.isAsync true and agentId; SubagentStop lists that
#     agent in background_tasks as type "subagent". Its report goes to whoever is still awake: with
#     the launcher's turn ended, to the MAIN session (probed on the Mac, 2026-10-18). Held, the
#     launcher waited for the agent's record here to go and then received the report itself, under
#     `claude -p`. Seen interactively (2026-10-28, OPS-151), with the launcher a background teammate
#     lane: the reviewer's hand-back went to the MAIN session, after the launcher's turn had ended
#     ("the agent that spawned you is no longer running"; it had no SendMessage). Whether a HELD
#     launcher receives it interactively is not yet probed (#305).
#   - TeammateIdle has agent_id but no background_tasks (code.claude.com/docs/en/hooks), so a
#     teammate's tasks, and the shells AskUserQuestion counts, are read from those files.
#   - A shell `&` makes no task at all, so it is refused up front (lib/gate-reader.awk reads the
#     command; `… & wait` waits, and is allowed).
# PreToolUse carries nothing about running work, so the running agents are this hook's own record:
# SubagentStart adds one, an allowed SubagentStop removes it, TeammateIdle adds a teammate.
#
# THE RECORD lives in the git common dir (every worktree shares it), per session:
# <common dir>/clauductor-wait/<session_id>/{agents,tasks,refusals,ended}/<agent_id>; outside a git
# repository, under TMPDIR. Another session's records untouched for two days are pruned. ended/ is
# written by an allowed SubagentStop: what a launcher waiting on that agent waits for.
#
# BOUNDED. A task that never ends (a server left running) would hold a turn forever, so after
# NO_SILENT_WAIT_MAX_REFUSALS (default 20) refusals of one agent the end is allowed; every refusal
# says how many remain and how to end the task by hand. For a background agent the bound is about
# MAX times the Bash timeout, since each end tried while it runs costs a refusal: the refusal says
# to re-run the wait instead.
#
# KNOWN LIMITS.
#   - Only THIS session's agents, teammates and commands are seen. A lane that is a session of its
#     own (a panel lane) is not: a stall there is the panel's to show.
#   - A teammate's end and a subagent stopped with TaskStop are not seen: its record stays, and
#     AskUserQuestion is refused until it is removed. The refusal names the file: loud, not silent.
#   - Without scratchpad_dir (an older Claude Code, or `claude -p` on macOS: 2.1.302 sent none
#     there), a teammate's tasks and the session's shells cannot be read, and only the agent
#     records count. A subagent's stop still holds: background_tasks lists what runs.
#   - Teammates were not probed (no agent teams in the probing build). Seen 2026-10-28: a teammate
#     lane went idle while its reviewer still ran, and nothing held it (#305 owns the probe).
#
# WITHOUT jq, for what it polices (as no-blind-source-rewrite): AskUserQuestion is refused, a
# subagent's Bash whose raw text holds a backgrounding `&` is refused, and a subagent's stop is
# refused ONCE (stop_hook_active ends the loop); a teammate's idle is allowed (no flag would ever
# end its refusal). Recording needs jq too.
#
# Protocol: exit 2 = block (stderr reaches Claude); exit 0 = allow. Checked by .claude/checks/hooks.sh.

payload=$(cat)

root=$(cd "$(dirname "$0")/../.." 2>/dev/null && pwd) || root=""
[ -n "${CLAUDE_PROJECT_DIR:-}" ] && [ -f "$CLAUDE_PROJECT_DIR/.claude/project.conf" ] && root=$CLAUDE_PROJECT_DIR
ROOT=$root
# A `.` of a missing file is fatal under dash (exit 2 reads as a BLOCK): test first.
if [ -f "$(dirname "$0")/../lib/conf.sh" ]; then
  # shellcheck disable=SC1091
  . "$(dirname "$0")/../lib/conf.sh"
fi
QUEUE=${OWNER_QUEUE:-docs/owner-queue.md}

block() { echo "BLOCKED by no-silent-wait: $1" >&2; exit 2; }
event_is() { printf '%s' "$payload" | grep -Eq "\"hook_event_name\"[[:space:]]*:[[:space:]]*\"$1\""; }

if ! command -v jq >/dev/null 2>&1 || ! printf '%s' "$payload" | jq -e . >/dev/null 2>&1; then
  why="jq could not read the hook payload (jq missing, or a malformed payload)"
  fix="Install jq (macOS: brew install jq; Debian/Ubuntu: apt-get install jq)."
  tool_is() { printf '%s' "$payload" | grep -Eq "\"tool_name\"[[:space:]]*:[[:space:]]*\"$1\""; }
  if event_is PreToolUse && tool_is AskUserQuestion; then
    block "$why, so whether anything in this session is still running cannot be told. $fix Meanwhile put the question in $QUEUE or send it with PushNotification, and keep working."
  fi
  # A subagent's Bash with a backgrounding `&` anywhere in the raw text (broader than the reader,
  # never narrower): a refused command can always be rewritten, so this refusal can end.
  if event_is PreToolUse && tool_is Bash && printf '%s' "$payload" | grep -q '"agent_id"' \
    && printf '%s' "$payload" | sed -e 's/&&//g' -e 's/[<>|]&//g' -e 's/&>//g' | grep -q '&'; then
    block "$why, and this subagent's command holds an '&', which may send it where nothing waits for it. $fix Or run it without '&': in the foreground (Bash timeout up to 600000) or with run_in_background."
  fi
  if event_is SubagentStop && ! printf '%s' "$payload" | grep -Eq '"stop_hook_active"[[:space:]]*:[[:space:]]*true'; then
    block "$why, so whether a command you started still runs cannot be told. $fix If one does, wait for it now in the foreground (Bash timeout 600000) and read its result; then end your turn."
  fi
  exit 0
fi

# One jq for every field: this runs on every Bash call, the main session's included.
# Ids and session ids become file names: keep them to a safe alphabet.
safe() { printf '%s' "$1" | tr -cd 'A-Za-z0-9_.-' | sed 's/^\.*//'; }
fields=$(printf '%s' "$payload" | jq -r '[.hook_event_name, .agent_id, .session_id, .scratchpad_dir, .tool_name,
  (.tool_response | objects | .backgroundTaskId // .taskId // (if .isAsync == true then .agentId else null end))] | map(. // "" | tostring | gsub("[\n\u001f]"; " ")) | join("\u001f")')
# A separator that is not IFS white space, so an empty field (no agent_id) is kept, not collapsed.
IFS="$(printf '\037')" read -r event agent sid scratch tool task_id <<EOF
$fields
EOF
agent=$(safe "$agent"); task_id=$(safe "$task_id")
sid=$(safe "$sid"); [ -n "$sid" ] || sid=unknown
tasks_dir=""; [ -n "$scratch" ] && tasks_dir="$(dirname "$scratch")/tasks"

# A subagent's or teammate's shell `&`: refused before it runs, since no task records it.
if [ "$event" = PreToolUse ] && [ "$tool" = Bash ]; then
  [ -n "$agent" ] || exit 0
  cmd=$(printf '%s' "$payload" | jq -r '.tool_input.command // "" | tostring')
  case $cmd in *'&'*) ;; *) exit 0 ;; esac
  verdict=$(printf '%s' "$cmd" | LC_ALL=C awk -v gates="" -f "$(dirname "$0")/lib/gate-reader.awk" 2>/dev/null) || exit 0
  case $verdict in *AMP*) ;; *) exit 0 ;; esac
  # `a & b & wait`: the shell waits for them before the call returns. `wait` as a command (at the
  # start of a line or after ; & | ( {), not a word in a comment or an echo.
  printf '%s' "$cmd" | grep -Eq '(^|[;&|({])[[:space:]]*wait([;&|)}[:space:]]|$)' && exit 0
  block "this command sends something to the shell's background with '&', and nothing waits for it: the call returns, your turn can end, and its result reaches nobody. Run it in the FOREGROUND (Bash timeout up to 600000), or with the Bash tool's run_in_background (your turn is then held until it ends); to run several at once, end the command with 'wait'."
fi
# Nothing to record: the common case, left before any git call.
[ "$event" = PostToolUse ] && [ -z "$task_id" ] && exit 0
[ "$event" = PreToolUse ] && [ "$tool" != AskUserQuestion ] && exit 0

common=$(git -C "${CLAUDE_PROJECT_DIR:-.}" rev-parse --path-format=absolute --git-common-dir 2>/dev/null) \
  || common="${TMPDIR:-/tmp}"
BASE="$common/clauductor-wait"
STATE="$BASE/$sid"
MAX=${NO_SILENT_WAIT_MAX_REFUSALS:-20}
case $MAX in ''|*[!0-9]*) MAX=20 ;; esac
END_LINE='^\[(exited with code -?[0-9]+|killed)\]$'

# note DIR NAME [LINE]: create or append a record, and mark the session as alive for the prune.
note() { mkdir -p "$STATE/$1" 2>/dev/null || return 0; if [ -n "${3:-}" ]; then printf '%s\n' "$3" >> "$STATE/$1/$2"; else : > "$STATE/$1/$2"; fi; touch "$STATE"; }
# task_state ID: "running" or "ended" from the task's output file; "unknown" when it is not found.
# The same last-line test the refusal hands the agent, so the two never disagree.
task_state() {
  _f="$tasks_dir/$1.output"
  { [ -n "$tasks_dir" ] && [ -f "$_f" ] && [ ! -L "$_f" ]; } || { echo unknown; return; }
  if tail -n 1 "$_f" | grep -Eq "$END_LINE"; then echo ended; else echo running; fi
}
recorded() { [ -n "$agent" ] && [ -f "$STATE/tasks/$agent" ] && grep -v '^$' "$STATE/tasks/$agent" | sort -u; }
# still_running: the tasks THIS agent started that are still running, one "id<TAB>kind<TAB>command"
# a line, kind "agent" (a background Agent call) or "shell" (a command or a Monitor).
# background_tasks, when the payload has it, names what is in flight; a task whose output file has
# its end line has ended whatever its status says. Without the list, the output files decide (an
# agent's file is a symlink with no end line: a teammate's background agent is not seen).
still_running() {
  _ids=$(recorded) || return 0
  [ -n "$_ids" ] || return 0
  if printf '%s' "$payload" | jq -e 'has("background_tasks")' >/dev/null 2>&1; then
    printf '%s' "$payload" | jq -r --arg ids "$_ids" '($ids | split("\n")) as $mine
      | .background_tasks[]? | select((.id // "") as $i | $mine | index($i))
      | select((.status // "running" | tostring | ascii_downcase) | test("^(completed|failed|killed|stopped|exited|done|error|cancel)") | not)
      | [.id, (if .type == "subagent" then "agent" else "shell" end),
         (.command // .description // "" | tostring | gsub("[\t\n]"; " "))] | @tsv' \
    | while IFS="$(printf '\t')" read -r _i _k _c; do
        [ "$(task_state "$_i")" = ended ] && continue
        { [ "$_k" = agent ] && [ -e "$STATE/ended/$_i" ]; } && continue
        printf '%s\t%s\t%s\n' "$_i" "$_k" "$_c"
      done
  else
    for _i in $_ids; do [ "$(task_state "$_i")" = running ] && printf '%s\tshell\t\n' "$_i"; done
  fi
  return 0
}
refuse_end() {  # refuse_end LIST: the turn may not end while LIST's tasks run, up to MAX times
  _n=$(( $(cat "$STATE/refusals/$agent" 2>/dev/null || echo 0) + 1 ))
  if [ "$_n" -gt "$MAX" ]; then
    echo "no-silent-wait: allowed $agent's end after $MAX refusals; its command(s) still ran: $(printf '%s' "$1" | cut -f1 | tr '\n' ' ')" >&2
    # Released: those tasks no longer hold this agent (a teammate would pay the bound every idle).
    printf '%s\n' "$1" | cut -f1 > "$STATE/released.$agent"
    grep -vxF -f "$STATE/released.$agent" "$STATE/tasks/$agent" > "$STATE/kept.$agent" 2>/dev/null
    mv "$STATE/kept.$agent" "$STATE/tasks/$agent" 2>/dev/null; rm -f "$STATE/released.$agent"
    return 0
  fi
  mkdir -p "$STATE/refusals" 2>/dev/null && printf '%s\n' "$_n" > "$STATE/refusals/$agent"
  _list=""; _wait=""; _shells=""; _agents=""
  while IFS="$(printf '\t')" read -r _id _kind _cmd; do
    [ -n "$_id" ] || continue
    _f="$tasks_dir/$_id.output"
    _list="$_list ${_id}${_cmd:+ ($_cmd)};"
    if [ "$_kind" = agent ]; then _agents="$_agents $STATE/ended/$_id"; continue; fi
    _shells=1
    if [ -n "$tasks_dir" ] && [ -f "$_f" ]; then _wait="$_wait $_f"; fi
  done <<EOF
$1
EOF
  _how="the output file the Bash tool named when it moved the command to the background"
  [ -n "$_wait" ] && _how="$(printf '%s' "$_wait" | sed 's/^ //')"
  # printf, not echo: the list holds commands, and echo would read a backslash in one.
  printf '%s\n' "BLOCKED by no-silent-wait: you started command(s) or agent(s) that are still running:${_list%;}. Nothing wakes you when one ends (on Claude Code 2.1.302 a command is killed as your turn ends, and an agent's report goes to the main session), so its result would reach nobody. Refusal $_n of $MAX." >&2
  [ -n "$_shells" ] && echo "Wait for each command in THIS turn, in the FOREGROUND (Bash timeout 600000), on $_how: until tail -n 1 FILE | grep -Eq '$END_LINE'; do sleep 5; done; tail -n 40 FILE  (call it again if the tool times out). Read the result, act on it, then end your turn." >&2
  # The marker, not the agent's record, is the wait: an agent not yet started has no record either.
  # A stopped agent (TaskStop, by anyone) never writes one, nor does an agent that ended before its
  # launch was recorded (that PostToolUse clears the marker): so a wait that times out with the agent
  # no longer running ends the turn, and the stop re-checks background_tasks' status.
  [ -n "$_agents" ] && printf '%s\n' "Wait for each agent in THIS turn, in the FOREGROUND (Bash timeout 600000), until its end marker exists:$_agents (until [ -e MARKER ]; do sleep 5; done). Its report then reaches you as a notification: read it, act on it, then end your turn. Re-run the wait if the tool times out while the agent still runs. If it no longer runs (stopped, by you or anyone, or its status says it ended) and no marker came, end your turn: this hook checks again and refuses only while it runs. Each end tried while the agent runs costs one refusal, and the last one releases you with its report lost." >&2
  echo "If you no longer need a command's result (a server you started, say), end it yourself first: TaskStop if you have it, else kill it by its PID (find yours with ps -eo pid,ppid,args), never by pattern (pkill -f): another session may run the same command. After $MAX refusals your turn is allowed to end and the command's result is lost: say so in your final message." >&2
  exit 2
}

case $event in
  PostToolUse)
    # Agent ids are hex: `_main`, the main session's own tasks, cannot collide with one.
    note tasks "${agent:-_main}" "$task_id"
    # A launch of an agent that ran before (resumed) must not find its old end marker: whether a
    # resumed agent fires SubagentStart, which also clears it, was never probed (#156).
    rm -f "$STATE/ended/$task_id"
    exit 0 ;;
  SubagentStart)
    [ -n "$agent" ] || exit 0
    note agents "$agent"
    rm -f "$STATE/ended/$agent"  # a resumed agent runs again
    find "$BASE" -mindepth 1 -maxdepth 1 -type d ! -name "$sid" -mtime +2 -exec rm -rf {} + 2>/dev/null
    exit 0 ;;
  SubagentStop|TeammateIdle)
    [ -n "$agent" ] || exit 0
    # An idle teammate still counts as running: it can message, and a frozen lead reads nothing.
    [ "$event" = TeammateIdle ] && note agents "$agent"
    running=$(still_running)
    [ -n "$running" ] && refuse_end "$running"
    rm -f "$STATE/refusals/$agent"
    # Free to end: a subagent's turn is over, so it no longer counts as running.
    # Its end marker tells a launcher waiting on it that it has ended (and that the background_tasks
    # status a moment later may still lag).
    if [ "$event" = SubagentStop ]; then rm -f "$STATE/agents/$agent" "$STATE/tasks/$agent"; note ended "$agent"; fi
    exit 0 ;;
  PreToolUse)
    # A subagent's question returns to the session that is awake to answer it.
    [ -z "$agent" ] || exit 0
    agents=""; [ -d "$STATE/agents" ] && agents=$(ls "$STATE/agents" 2>/dev/null | tr '\n' ' ' | sed 's/ $//')
    # Recorded tasks only (anyone's, the main session's included): a file nothing recorded may be
    # a foreground call's leftover, and would refuse every question for the rest of the session.
    shells=""
    if [ -d "$STATE/tasks" ]; then
      for i in $(cat "$STATE/tasks"/* 2>/dev/null | grep -v '^$' | sort -u); do
        [ "$(task_state "$i")" = running ] && shells="$shells $i"
      done
      shells=${shells# }
    fi
    [ -n "$agents$shells" ] || exit 0
    what=""
    [ -n "$agents" ] && what="agent(s) $agents"
    [ -n "$shells" ] && what="${what:+$what and }background command(s) $shells"
    block "$what still running in this session. While AskUserQuestion waits for the owner this session reads none of their results or messages, so a lane's request sits unread until the owner answers (5 h on 2026-10-18). Put the question in $QUEUE, or send it with PushNotification, and keep working; ask with AskUserQuestion only once nothing is running. If an agent named here has already ended (stopped with TaskStop, or a teammate shut down), remove its record: rm $STATE/agents/<id>. A background command that will never end (a server): stop it (TaskStop) or kill it." ;;
esac
exit 0
