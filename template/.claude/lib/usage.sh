# usage.sh: sourced (never run) by .claude/usage-report.sh and .claude/metrics.sh. ONE reading of
# Claude Code's transcripts and ONE pricing of them (the list prices in .claude/model-roles.json
# `.prices`), so the session report, the Metrics view's Cost tab and a change's budget never
# disagree about what a message cost (*A check that reads source is a parser*).
#
# WHERE THE FIGURES COME FROM. ${CLAUDE_PROJECTS_DIR:-${CLAUDE_CONFIG_DIR:-~/.claude}/projects}: one
# directory per checkout a session was STARTED in, named after its path with every non-alphanumeric
# character a '-'. This repository's are its main checkout's and every `<that>--claude-worktrees-<n>`
# (a session that ran EnterWorktree has its whole transcript moved there). In each, `<session>.jsonl`
# is the session and `<session>/subagents/**/agent-<id>.jsonl` its agents, each beside an
# `agent-<id>.meta.json` naming its type and parent. Every way this can be wrong degrades to a
# plausible number, so each is handled explicitly (the lessons of StandingT's usage-report.mjs):
#   - workflow agents nest a directory deeper (subagents/workflows/<run>/): found by `find`, any depth;
#   - an agent spawned BY an agent is charged to its ROOT's role; a parent chain that cycles or
#     names a missing meta is pooled under `?`, never guessed;
#   - a streamed response repeats its message id on several lines: the caller dedupes by id;
#   - a model (or fast mode) with no price is UNPRICED (usd null), listed, never costed at zero;
#   - a line cut off mid-write is skipped, not fatal (jq -R + fromjson?).
#
# Dollars are list-price API equivalents: on a subscription they are a proxy for how fast the
# limit drains, not a bill. Everything needs jq.

# usage_projects: the transcript root.
usage_projects() {
  printf '%s' "${CLAUDE_PROJECTS_DIR:-${CLAUDE_CONFIG_DIR:-$HOME/.claude}/projects}"
}

# usage_main: this repository's main checkout, whatever worktree we run in.
usage_main() {
  _c=$(git -C "$ROOT" rev-parse --path-format=absolute --git-common-dir 2>/dev/null) || return 1
  dirname "$_c"
}

# usage_dirs MAIN: this repository's transcript directories, one per line (only those that exist).
# Not `<slug>*`: that would take `-repo-other`, another repository whose name starts the same.
usage_dirs() {
  _p=$(usage_projects)
  _s=$(printf '%s' "$1" | sed 's/[^A-Za-z0-9]/-/g')
  [ -d "$_p/$_s" ] && printf '%s\n' "$_p/$_s"
  for _d in "$_p/$_s--claude-worktrees-"*; do [ -d "$_d" ] && printf '%s\n' "$_d"; done
  return 0
}

# The jq definitions every reader shares.
#   price($p; $m)   the .prices entry for model id $m: the longest key that prefixes it (a context
#                   tag such as `[1m]` ignored), or null
#   usd($p; $u)     the message's cost from its usage object $u, or null when unpriced. Fast mode is
#                   priced only where the entry says so (`fast_multiplier`)
#   agentlabel($m)  an agent's own label from its meta.json
#   rootrole($metas; $id)  the label of the ROOT of an agent's spawn chain, or "?"
# shellcheck disable=SC2016
USAGE_JQ_DEFS='
def price($p; $m): ($m | sub("\\[[^]]*\\]$"; "")) as $b
  | [ $p | to_entries[] | .key as $k | select(($k | startswith("_") | not) and ($b | startswith($k))) ]
  | sort_by(.key | length) | last | if . == null then null else .value end;
def usd($p; $model; $u):
  price($p; $model) as $pr
  | if $pr == null then null
    elif ($u.speed // "standard") == "fast" and ($pr.fast_multiplier // null) == null then null
    else (if ($u.speed // "standard") == "fast" then $pr.fast_multiplier else 1 end) as $x
      | ($u.cache_creation.ephemeral_5m_input_tokens // null) as $w5
      | ($u.cache_creation.ephemeral_1h_input_tokens // null) as $w1
      | $x * ( ($u.input_tokens // 0) * $pr.input
             + ($u.output_tokens // 0) * $pr.output
             + ($u.cache_read_input_tokens // 0) * $pr.cache_read
             + (if $w5 == null and $w1 == null then ($u.cache_creation_input_tokens // 0) * $pr.input * 1.25
                else ($w5 // 0) * $pr.input * 1.25 + ($w1 // 0) * $pr.input * 2 end) ) / 1000000
    end;
def agentlabel($m):
  if $m == null then "?"
  elif ($m.customAgentType // "") != "" then $m.customAgentType
  elif $m.agentType == "workflow-subagent" then "workflow:" + ($m.workflowPhase // "?")
  else (if ($m.agentType == "general-purpose" or $m.agentType == "fork") and ($m.name // "") != ""
        then $m.name else ($m.agentType // "?") end) | sub("(-[0-9]+)+$"; "")
  end;
def rootrole($metas; $id):
  def walk($cur; $seen; $root):
    if ($cur // "") == "" then $root
    elif $metas[$cur] == null or ($seen | any(. == $cur)) then "?"
    else walk($metas[$cur].parentAgentId; $seen + [$cur]; agentlabel($metas[$cur])) end;
  walk($id; []; "?");
'

# usage_records SINCE_ISO [SESSION]: one JSON line per assistant message with usage, in this
# repository's transcripts, from SINCE_ISO (an ISO-8601 UTC time; "" = all) or of one SESSION id:
#   {"id","ts" (unix s),"sid","role","model","branch","cwd","read","write","out" (tokens: cache read,
#    cache write, output),"usd" (null = unpriced)}
# NOT deduplicated: a streamed message appears more than once, and the caller keeps one per id.
# The main session's role is "session"; an agent's is its root's label (rootrole). Returns 2 with a
# reason on stderr when it cannot read (no jq, no git checkout, no .prices).
usage_records() {
  command -v jq >/dev/null 2>&1 || { echo "jq is not installed" >&2; return 2; }
  _roles="$ROOT/.claude/model-roles.json"
  jq -e '.prices | type == "object"' "$_roles" >/dev/null 2>&1 || { echo "$_roles has no .prices table" >&2; return 2; }
  _main=$(usage_main) || { echo "$ROOT is not a git checkout" >&2; return 2; }
  _since=$1 _sess=${2:-}
  _tmp=$(mktemp -d "${TMPDIR:-/tmp}/usage.XXXXXX") || return 2
  jq '.prices' "$_roles" > "$_tmp/prices.json"
  _ref=""
  if [ -n "$_since" ]; then
    # A session file last written before the window cannot hold a message inside it.
    _stamp=$(printf '%s' "$_since" | sed -n 's/^\([0-9]\{4\}\)-\([0-9]\{2\}\)-\([0-9]\{2\}\)T\([0-9]\{2\}\):\([0-9]\{2\}\).*/\1\2\3\4\5/p')
    [ -n "$_stamp" ] && TZ=UTC0 touch -t "$_stamp" "$_tmp/ref" 2>/dev/null && _ref="$_tmp/ref"
  fi
  usage_dirs "$_main" | while IFS= read -r _d; do
    for _f in "$_d"/*.jsonl; do
      [ -f "$_f" ] || continue
      _sid=$(basename "$_f" .jsonl)
      if [ -n "$_sess" ]; then [ "$_sid" = "$_sess" ] || continue
      elif [ -n "$_ref" ] && [ ! "$_f" -nt "$_ref" ]; then
        # The session file is older than the window, but an agent of it may have written since.
        [ -n "$(find "$_d/$_sid" -name '*.jsonl' -newer "$_ref" 2>/dev/null | head -1)" ] || continue
      fi
      _sub="$_d/$_sid/subagents"
      if [ -d "$_sub" ]; then
        find "$_sub" -name '*.meta.json' -exec jq -c '{f: input_filename, m: .}' {} + 2>/dev/null \
          | jq -s 'map({key: (.f | sub(".*/"; "") | sub("\\.meta\\.json$"; "") | sub("^agent-"; "")), value: .m}) | from_entries' \
          > "$_tmp/metas.json" 2>/dev/null || echo '{}' > "$_tmp/metas.json"
        find "$_sub" -name '*.jsonl' > "$_tmp/agents" 2>/dev/null
      else
        echo '{}' > "$_tmp/metas.json"; : > "$_tmp/agents"
      fi
      # shellcheck disable=SC2046
      { printf '%s\n' "$_f"; cat "$_tmp/agents"; } | while IFS= read -r _t; do printf '%s\0' "$_t"; done \
        | xargs -0 jq -cR --slurpfile p "$_tmp/prices.json" --slurpfile metas "$_tmp/metas.json" \
            --arg since "$_since" --arg sid "$_sid" --arg mainf "$_f" "$USAGE_JQ_DEFS"'
          select(contains("\"usage\"")) | (fromjson? // empty)
          | select(.type == "assistant" and (.message.usage | type) == "object" and (.message.model // "") != ""
                   and .message.model != "<synthetic>")
          | select($since == "" or ((.timestamp // "") >= $since))
          | . as $e
          | (input_filename) as $file
          | { id: ($e.message.id // $e.uuid // ($file + ":" + ($e.timestamp // ""))),
              ts: (($e.timestamp // "") | sub("\\.[0-9]+"; "") | (fromdateiso8601? // 0)),
              sid: $sid,
              role: (if $file == $mainf then "session"
                     else rootrole($metas[0]; ($file | sub(".*/"; "") | sub("\\.jsonl$"; "") | sub("^agent-"; ""))) end),
              model: (($e.message.model | sub("\\[[^]]*\\]$"; "")) + (if ($e.message.usage.speed // "") == "fast" then " (fast)" else "" end)),
              branch: ($e.gitBranch // ""),
              cwd: ($e.cwd // ""),
              read: ($e.message.usage.cache_read_input_tokens // 0),
              write: ($e.message.usage.cache_creation_input_tokens // 0),
              out: ($e.message.usage.output_tokens // 0),
              usd: usd($p[0]; $e.message.model; $e.message.usage) }' 2>/dev/null
    done
  done
  rm -rf "$_tmp"
  return 0
}
