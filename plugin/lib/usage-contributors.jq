# Context attribution for usage-report.sh --contributors (ST-8). Matches Standing Tee usage-report.mjs.
# stdin: {"file":"<path>","e":<event>} per line; empty e counts as parse failure for that file's stream
# --arg since "" or ISO-8601  --arg root repo absolute path  --slurpfile prices .prices object

def CHARS_PER_TOKEN: 2.44;
def IMAGE_CHARS: (1600 * CHARS_PER_TOKEN);

def mtok($n):
  if $n >= 1000000 then "\(($n / 100000 | round / 10))M"
  else "\(($n / 1000 | round))k" end;

def normPath($p):
  ($p | tostring) as $s
  | if ($root != "" and ($s | startswith($root))) then
      ($s | ltrimstr($root) | if startswith("/") then ltrimstr("/") else . end)
    else $s end
  | gsub("^\\.claude/worktrees/[^/]+/"; "")
  | gsub("^/(private/)?tmp/claude-[0-9]+/[^/]+/[^/]+/scratchpad/"; "<scratchpad>/")
  | gsub("^/Users/[^/]+/\\.claude/"; "~/.claude/")
  | gsub("^/Users/[^/]+/"; "~/");

def normBash($cmd):
  ($cmd | tostring) as $raw
  | ($raw | split("\n") | map(gsub("^\\s+|\\s+$"; "")) | map(select(. != "" and (startswith("#") | not))) | join(" ; ")) as $s0
  | reduce range(0; 6) as $_ ($s0;
      . as $before
      | gsub("^cd\\s+\\S+\\s*(&&|;\\s*)"; "")
      | gsub("^[A-Z_][A-Z0-9_]*=(\"[^\"]*\"|'[^']*'|\\S*)\\s+"; "")
      | gsub("^(export\\s+[A-Z_][A-Z0-9_]*=\\S*\\s*(&&|;\\s*))"; "")
      | gsub("^(timeout|time|env|command)\\s+(\\d+[sm]?\\s+)?"; "")
      | gsub("^(set\\s+-\\S+\\s*;\\s*)"; "")
      | gsub("^\\(\\s*"; "")
      | if . == $before then . else . end)
  | if . == "" then "(empty)"
    else
      (split("\\s+") | map(select(. != ""))) as $tok
      | if ($tok | length) == 0 then "(empty)"
        else $tok[0] as $a | $tok[1:] as $rest
        | if ($a | IN("bash", "sh", "zsh", "node")) and ($rest[0]? != null) and ($rest[0] | startswith("-") | not) then
            normPath($rest[0] | gsub("^[\"']|[\"']$"; "")) | gsub("^\\./"; "")
          elif ($a | test("[/.](sh|mjs|js|py)$|/bin/")) or ($a | startswith("./")) or ($a | contains("/")) then
            normPath($a | gsub("^[\"']|[\"']$"; "")) | gsub("^\\./"; "")
          elif $a == "gh" then
            "gh " + (($rest | map(select(startswith("-") | not))) | if length > 0 and .[0] == "api" then .[0:1] else .[0:2] end | join(" "))
          elif $a == "git" then
            ($rest | map(select(. != ""))) as $r
            | "git " + (($r | .[ ($r | to_entries | map(select(.value | startswith("-") | not)) | .[0].key) // 0 ] // "") | tostring)
          elif ($a | IN("pnpm", "npx")) then
            ($rest | reduce .[] as $w ({o: [], skip: false};
              if .skip then .skip = false | .
              elif ($w | IN("-C", "--filter", "--dir", "-F")) then .skip = true | .
              elif ($w | startswith("-")) or ($w | IN("exec", "run", "-r")) then .
              elif (.o | length) < 1 then .o += [$w] | .
              else . end) | "\($a) \(.o | join(" "))")
          elif ($a | IN("cat", "sed", "head", "tail", "less", "nl", "wc")) then
            ($rest | reverse | map(select(test("[/.]\\w") and (startswith("-") | not) and (test("^['\"]?\\d") | not))) | .[0]) as $file
            | "\($a) \(if $file then normPath($file | gsub("^[\"']|[\"']$"; "")) else "(stdin)" end)"
          else $a end
        end
    end;

def lenOf($c):
  if $c == null then 0
  elif ($c | type) == "string" then ($c | length)
  elif ($c | type) == "array" then
    ($c | reduce .[] as $b (0; . + (if ($b.type? // "") == "image" then IMAGE_CHARS else lenOf($b.text // $b.content // "") end)))
  else ($c | tostring | length) end;

def textOf($c):
  if ($c | type) == "string" then $c
  elif ($c | type) == "array" then
    ($c | map(if (.text? | type) == "string" then .text elif (.content? | type) == "string" then .content else "" end) | join("\n"))
  else "" end;

def keyForTool($tu; $resultText):
  if $tu == null then "Other tool (unmatched)"
  else ($tu.name) as $n | ($tu.input // {}) as $i
  | if $n == "Read" then
      "Read \(normPath($i.file_path // "")), \(if ($i.offset != null or $i.limit != null) then "range" else "whole" end)"
    elif $n == "Bash" then "Bash \(normBash($i.command // ""))"
    elif ($n | IN("Agent", "Task")) then
      if ($resultText | test("^Async agent launched|agentId: \\w+ \\(internal|(launched|Spawned) successfully")) then "Agent launch ack"
      else "Agent \($i.subagent_type // "general-purpose") result" end
    elif $n == "Skill" then "Skill launch ack"
    elif ($n | startswith("mcp__")) then "MCP \($n | split("__") | .[1])"
    elif $n == "Artifact" then "Artifact \($i.action // "publish")"
    else $n end
  end;

def userKey($e; $text; $toolUses):
  if ($e.isCompactSummary // false) then "Post-compaction summary"
  elif ($text | test("^Base directory for this skill: (\\S+)")) then
    "Skill \((($text | capture("skills/(?<s>[^\\s/]+)"))? // {s: "?"}) | .s) body"
  elif ($e.sourceToolUseID? // "") != "" and ($toolUses[$e.sourceToolUseID].name? // "") == "Skill" then
    "Skill \($toolUses[$e.sourceToolUseID].input.skill // "?") body"
  elif ($text | test("^<command-name>|<command-message>")) then
    "Slash \((($text | capture("<command-name>/?(?<s>[^<]+)"))? // {s: "?"}) | .s)"
  elif ($text | test("<local-command-stdout>")) then "User local-command output"
  elif ($text | test("cross-session-message")) then "Message cross-session"
  elif ($text | test("<teammate-message")) then "Message teammate"
  elif ($text | test("<task-notification>")) then
    if ($text | test("Stop hook")) then "Hook Stop feedback"
    elif ($text | test("<tool-use-id>|shell|Background command|bash") and ($text | test("agent") | not)) then "Bash background-task notice"
    else "Agent result (async, type unknown)" end
  elif ($e.isMeta // false) then "Reminder meta-user text"
  else "User prompt" end;

def tok($u):
  ($u.input_tokens // 0) + ($u.cache_read_input_tokens // 0) + ($u.cache_creation_input_tokens // 0);

def price($p; $m):
  ($m | sub("\\[[^]]*\\]$"; "")) as $b
  | [ $p | to_entries[] | .key as $k | select(($k | startswith("_") | not) and ($b | startswith($k))) ]
  | sort_by(.key | length) | last | if . == null then null else .value end;

def priceOf($p; $call):
  price($p; $call.model) as $pr
  | if $pr == null then null
    elif ($call.speed // "standard") == "fast" and ($pr.fast_multiplier // null) == null then null
    else (if ($call.speed // "standard") == "fast" then $pr.fast_multiplier else 1 end) as $x
      | {input: ($pr.input * $x), output: ($pr.output * $x), cacheRead: ($pr.cache_read * $x)}
    end;

def inWindow($ts):
  ($ts // "") != "" and ($since == "" or $ts >= $since);

def attributeThread($lines):
  reduce ($lines | map(select(. != null)))[] as $e (
    {calls: [], callIds: [], byId: {}, toolUses: {}, pending: [], parseFailures: 0};
    if ($e.type // "") == "assistant" and ($e.message.model? // "") != "" and ($e.message.model // "") != "<synthetic>" and ($e.message.usage? != null) then
      ($e.message.id // "\($e.timestamp):\(.calls | length)") as $id
      | if .byId[$id] == null then
          .byId[$id] = {id: $id, ts: ($e.timestamp // ""), model: $e.message.model, speed: ($e.message.usage.speed // null),
            items: .pending, out: [], toolUses: [], usage: $e.message.usage}
          | .pending = []
          | .callIds += [$id]
          | .
        else
          .byId[$id].usage = $e.message.usage
          | .
        end
      | reduce (($e.message.content // []) | if type == "array" then . else [] end)[] as $b (.;
          if $b.type == "text" then
            .byId[$id].out += [$b.text // ""] | .
          elif $b.type == "tool_use" then
            .toolUses += {($b.id): $b}
            | .byId[$id].toolUses += [$b]
            | reduce (($b.input // {}) | to_entries[] | .value) as $v (.;
                if ($v | type) == "string" then .byId[$id].out += [$v] | . else . end)
          else . end)
    elif ($e.type // "") == "user" and ($e.message? != null) then
      ($e.message.content) as $content
      | if ($content | type) == "array" and ($content | any(.type? == "tool_result")) then
          reduce ($content | map(select(.type? == "tool_result")))[] as $b (.;
            ($b.tool_use_id) as $tid | (.toolUses[$tid]) as $tu | ($b.content) as $bc
            | .pending += [{key: keyForTool($tu; textOf($bc)), chars: lenOf($bc), text: textOf($bc), tu: $tu, ts: ($e.timestamp // "")}]
            | .)
        else
          textOf($content) as $text
          | .pending += [{key: userKey($e; $text; .toolUses), chars: lenOf($content), text: $text, ts: ($e.timestamp // ""), meta: ($e.isMeta // false)}]
          | .
        end
    elif ($e.type // "") == "attachment" and ($e.attachment? != null) then
      ($e.attachment.type) as $t
      | if ($t | IN("prompt_snapshot", "deferred_tools_record", "command_permissions", "credential_org", "thinking_drop")) then .
        else
          ($t | if startswith("hook_") then "Hook \($e.attachment.hookName // $t)"
                elif . == "instructions" then "Startup CLAUDE.md/AGENTS.md"
                else "Reminder \($t)" end) as $key
          | .pending += [{key: $key, chars: (($e.rendered // "") | tostring | length), ts: ($e.timestamp // "")}]
          | .
        end
    else . end
  )
  | . as $t | .calls = [$t.callIds[] as $i | $t.byId[$i]] | del(.callIds);

def suffixSums($calls):
  ($calls | length) as $n
  | reduce [range($n - 1; -1; -1)][] as $i (
      {suffix: [range(0; $n + 1) | 0], later: [range(0; $n + 1) | 0], unpricedLater: false};
      ($calls[$i]) as $c
      | .suffix[$i] = .suffix[$i + 1] + (if inWindow($c.ts) and (priceOf($prices[0]; $c) != null) then (priceOf($prices[0]; $c).cacheRead / 1000000) else 0 end)
      | .later[$i] = .later[$i + 1] + (if inWindow($c.ts) then 1 else 0 end)
      | if inWindow($c.ts) and (priceOf($prices[0]; $c) == null) then .unpricedLater = true else . end
    );

def pushItem($acc; $i; $key; $tokens; $su):
  if $tokens <= 0 then $acc
  else $acc + [{call: $i, key: $key, tokens: $tokens, later: $su.later[$i + 1],
    downstream: (if $su.unpricedLater then null else $tokens * $su.suffix[$i + 1] end)}]
  end;

def attributeItems($calls; $su):
  reduce range(0; ($calls | length)) as $i (
    {items: [], resets: 0, estSum: 0, measSum: 0};
    ($calls[$i]) as $c
    | if (inWindow($c.ts) | not) then .
      else
        (tok($c.usage)) as $ctxNow
        | (if $i > 0 then $calls[$i - 1] else null end) as $prev
        | (if $prev then $ctxNow - tok($prev.usage) else $ctxNow end) as $delta
        | ($c.items | map(. + {est: (.chars / CHARS_PER_TOKEN)})) as $parts0
        | (if $prev then [{key: "Own output (previous call)", est: ($prev.usage.output_tokens // 0)}] + $parts0 else $parts0 end) as $parts1
        | ($parts1 | map(.est) | add // 0) as $est
        | if ($prev == null) or ($delta < 0) then
            (if $prev != null then .resets += 1 else . end)
            | (if $prev != null then ($parts1 | .[1:]) else $parts1 end) as $parts
            | (if (($parts | map(.est) | add) // 0) > $ctxNow then $ctxNow / (($parts | map(.est) | add) // 1) else 1 end) as $scale
            | (if $prev != null then "Post-compaction base" else "Startup system prompt + tools" end) as $rem
            | reduce $parts[] as $pt (
                {acc: ., assigned: 0};
                ($pt.est * $scale) as $t
                | if $t > 0 then .acc.items = pushItem(.acc.items; $i; $pt.key; $t; $su) | .assigned += $t else . end)
            | .acc as $a | ($a.assigned // 0) as $as
            | $a | .items = .items + (if ($ctxNow - $as) > 0 then pushItem(.items; $i; $rem; ($ctxNow - $as); $su) else .items end)
          else
            (if $est > 0 then $delta / $est else 0 end) as $scale
            | .estSum += $est | .measSum += $delta
            | reduce $parts1[] as $pt (. ; .items = pushItem(.items; $i; $pt.key; ($pt.est * $scale); $su))
            | if $est == 0 and $delta > 0 then .items = pushItem(.items; $i; "Other unattributed growth"; $delta; $su) else . end
          end
      end
  );

def contributorsOne($calls):
  suffixSums($calls) as $su
  | attributeItems($calls; $su);

def mergeRow($m; $it):
  ($m[$it.key] // {key: $it.key, n: 0, tokens: 0, downstream: 0, unpriced: false}) as $g
  | $m + {($it.key): ($g
      | .n = (.n + 1)
      | .tokens = (.tokens + $it.tokens)
      | if $it.downstream == null then .unpriced = true else .downstream = (.downstream + $it.downstream) end)};

def cacheRewrites($calls):
  reduce range(0; ($calls | length)) as $i (
    {n: 0, cost: 0, unpriced: 0, waits: {}};
    ($calls[$i]) as $c
    | (if $i > 0 then $calls[$i - 1] else null end) as $prev
    | if $prev == null or (inWindow($c.ts) | not) then .
      else
        (tok($c.usage)) as $total
        | ($c.usage.cache_creation_input_tokens // 0) as $w
        | if $total < tok($prev.usage) or $w <= 0.5 * $total then .
          else
            .n += 1
            | (priceOf($prices[0]; $c)) as $p
            | if $p == null then .unpriced += 1
              else
                ($c.usage.cache_creation.ephemeral_1h_input_tokens // 0) as $w1
                | .cost += (($w - $w1) * $p.input * 1.25 + $w1 * $p.input * 2) / 1000000
              end
            | (($prev.toolUses | if length > 0 then .[-1] else null end)) as $last
            | (if $last == null then "turn end"
               elif $last.name == "Bash" then "Bash \(normBash($last.input.command // ""))"
               else $last.name end) as $wait
            | .waits[$wait] = ((.waits[$wait] // 0) + 1)
          end
      end
  );

# --- main ---
def mergeWaits($a; $b):
  reduce ($b | to_entries[]) as $e ($a; .[$e.key] = ((.[$e.key] // 0) + $e.value));

[inputs] as $raw
| ($raw | map(select(.e == null)) | length) as $parseFailures
| ($raw | map(select(.e != null))) | group_by(.file) | map({
    file: .[0].file,
    thread: attributeThread(map(.e))
  }) as $files
| reduce $files[] as $f (
    {threads: 0, files: ($files | length), resets: 0, rows: {}, rewrites: {n: 0, cost: 0, unpriced: 0, waits: {}}};
    ($f.thread.calls) as $calls
    | if ($calls | map(inWindow(.ts)) | any | not) then .
      else
        .threads += 1
        | contributorsOne($calls) as $ai
        | .resets += $ai.resets
        | reduce $ai.items[] as $it (. ; .rows = mergeRow(.rows; $it))
        | cacheRewrites($calls) as $rw
        | .rewrites.n += $rw.n | .rewrites.cost += $rw.cost | .rewrites.unpriced += $rw.unpriced
        | .rewrites.waits = mergeWaits(.rewrites.waits; $rw.waits)
      end
  )
| .parseFailures = ($parseFailures + (reduce $files[] as $f (0; . + $f.thread.parseFailures)))
| .rows = (.rows | to_entries | map(.value) | sort_by(-.downstream))
