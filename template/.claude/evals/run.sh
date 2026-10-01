#!/bin/sh
# run.sh: the seeded-defect eval of one role's agent on one model and effort (OPS-10). It works
# like mutation testing: each case plants known defects in a small diff, the agent reviews it
# exactly as build-change asks it to, and the score says how many it caught, how often it cried
# wolf, and what that cost.
#
#   sh .claude/evals/run.sh --role reviewer --model opus --effort high
#   sh .claude/evals/run.sh --role reviewer --model sonnet --effort high --cases go-tenant-scope,sh-clean-portable
#   sh .claude/evals/run.sh --role reviewer --model haiku --effort low --estimate   # the $ before you spend it
#
# Options: --cases a,b (a subset: its receipt is marked partial and is never evidence), --budget USD
# (the cap per case, default $EVAL_BUDGET_USD or 3), --out DIR (where the receipt goes, default
# .claude/evals/receipts), --keep (keep each case's scratch repository and the raw output).
#
# HOW THE AGENT IS RUN: `claude -p`, not the Agent SDK. The CLI is what every Clauductor project
# already has, it runs the agent with the same tools, skills and permission model as a real
# review, and its JSON result carries the run's cost and usage. Per case:
#   claude -p "<build-change's review prompt>" --agents <the agent, as JSON> --agent <name>
#     --model M --effort E --output-format json --json-schema <build-change's REVIEW schema>
#     --no-session-persistence --setting-sources project --permission-mode dontAsk
#     --allowedTools "Read Grep Glob Skill Agent Bash(git *)" --max-budget-usd B
# in a throwaway git repository holding the case's BEFORE tree committed and its AFTER tree as
# the uncommitted diff, plus a change record (changes/eval-<id>/design.md, tasks.md) written from
# the case's brief and the suite's AGENTS.md. The agent's frontmatter model and effort are
# replaced by the ones under test. `--setting-sources project` keeps the user's own hooks and
# plugins out of the measurement. Add flags with EVAL_CLAUDE_FLAGS; EVAL_CLAUDE names the binary
# (the checks point it at fake-claude.sh, so no test spends a token).
#
# A CASE is .claude/evals/<role>/cases/<id>/ with case.json, before/ and after/:
#   {"id", "lang": go|ts|py|sh, "kind": defect|clean, "charter", "title", "brief", "tasks": [...],
#    "delete": [paths the change deletes], "expected": [{"id", "severity", "file", "lines": [lo, hi],
#    "keywords": [...], "why"}]}
# A clean case expects nothing: it measures false positives.
#
# SCORING. A finding catches a planted defect when it names the same file (a path suffix is
# enough) and either cites a line within 3 of the defect's range or uses one of its keywords. Each
# defect is caught at most once and each finding catches at most one: a line hit is preferred to a
# keyword hit, and an actionable finding (medium or above) to a low.
#   recall             defects caught / defects planted
#   precision          actionable findings that caught a defect / all actionable findings (1 if none)
#   fp_rate            clean cases with any actionable finding / clean cases
#   severity_accuracy  caught defects graded at exactly the expected severity / defects caught
# A `low` finding never counts against precision: the reviewer is told never to inflate one.
# `pass` is recall, fp_rate and severity_accuracy against model-roles.json .evals.thresholds, a
# complete suite and no case that failed to run.
#
# HASHES. The receipt's `hashes.role` (the role's model, effort and tier variants) and
# `hashes.triggers` (each trigger input model-roles.json .evals.triggers declares for the role, by
# its id) are what pr-merge-guard rule 13 holds it to (OPS-16; .claude/lib/evals.sh). It also
# names, as context only, the hash of every role's choice, the agent's blob and the whole
# workflows tree it ran beside (`model_roles`, `agent`, `workflows`).
#
# Exit: 0 pass, 1 ran but did not pass, 2 could not run.
set -u

dir=$(cd "$(dirname "$0")" && pwd)
ROOT=$(cd "$dir/../.." && pwd)
roles_json="$ROOT/.claude/model-roles.json"
CLAUDE=${EVAL_CLAUDE:-claude}

die() { echo "run.sh: $*" >&2; exit 2; }
usage() { sed -n '2,/^set -u$/p' "$0" | sed '$d' | sed 's/^# \{0,1\}//'; }

role="" model="" effort="" cases="" estimate="" keep="" out="$dir/receipts"
budget=${EVAL_BUDGET_USD:-3}
while [ $# -gt 0 ]; do
  case "$1" in
    --role|--model|--effort|--cases|--budget|--out)
      [ $# -ge 2 ] || die "$1 needs a value"
      case "$1" in
        --role) role=$2 ;; --model) model=$2 ;; --effort) effort=$2 ;;
        --cases) cases=$(printf '%s' "$2" | tr ',' ' ') ;; --budget) budget=$2 ;; --out) out=$2 ;;
      esac
      shift 2 ;;
    --estimate) estimate=1; shift ;;
    --keep) keep=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) die "unknown argument '$1' (see --help)" ;;
  esac
done
[ -n "$role" ] || die "--role is required (a directory under $dir with a cases/ suite)"
[ -n "$model" ] || die "--model is required (opus, sonnet, haiku, fable or a claude-* id)"
command -v jq >/dev/null 2>&1 || die "jq is not installed"
command -v git >/dev/null 2>&1 || die "git is not installed"
case " low medium high xhigh max " in *" ${effort:-?} "*) ;; *) die "--effort must be low, medium, high, xhigh or max" ;; esac
suite="$dir/$role/cases"
[ -d "$suite" ] || die "no suite for role '$role' ($suite)"
[ -f "$roles_json" ] || die "cannot find $roles_json"

all=$(cd "$suite" && for c in */case.json; do [ -f "$c" ] && dirname "$c"; done | LC_ALL=C sort)
[ -n "$all" ] || die "the $role suite has no cases"
complete=true
if [ -n "$cases" ]; then
  for c in $cases; do [ -f "$suite/$c/case.json" ] || die "no case '$c' in $suite"; done
  [ "$(printf '%s\n' $cases | LC_ALL=C sort -u)" = "$all" ] || complete=false
else
  cases=$all
fi
n=$(printf '%s\n' $cases | grep -c .)

# ── --estimate: the price of one run, before spending it ────────────────────────────────────
# Per case, a review is ~10 turns over a context that grows from Claude Code's system prompt and
# the agent's (~18k tokens) to ~30k: about 30k tokens written to the cache, 240k read from it, 3k
# uncached and 10k output at high effort (thinking included). Output scales with effort. If the
# agent runs the code-review skill, its finder subagents multiply this 2 to 4 times. Override the
# assumptions with EVAL_EST_WRITE, EVAL_EST_READ, EVAL_EST_INPUT and EVAL_EST_OUTPUT.
if [ -n "$estimate" ]; then
  key=$(jq -r --arg m "$model" '(.prices // {}) | keys_unsorted | map(select(. != "_why"))
      | (if ($m | startswith("claude-")) then map(select(. as $k | $m | startswith($k))) | sort_by(length) | reverse
         else map(select(startswith("claude-" + $m))) end) | first // empty' "$roles_json")
  [ -n "$key" ] || die "model-roles.json .prices has no entry for '$model'"
  jq -r --arg k "$key" --arg e "$effort" --argjson n "$n" \
     --argjson w "${EVAL_EST_WRITE:-30000}" --argjson r "${EVAL_EST_READ:-240000}" \
     --argjson i "${EVAL_EST_INPUT:-3000}" --argjson o "${EVAL_EST_OUTPUT:-10000}" '
    .prices[$k] as $p
    | ({"low": 0.4, "medium": 0.7, "high": 1, "xhigh": 1.5, "max": 2}[$e]) as $s
    | ((($w * $p.input * 1.25) + ($r * $p.cache_read) + ($i * $p.input) + ($o * $s * $p.output)) / 1000000) as $c
    | def usd: ((. * 100 + 0.5 | floor) / 100 | tostring) as $v | "$" + $v;
      "estimate for \($n) case(s) on \($k) (\($e) effort): ~\($c | usd) a case, ~\($c * $n | usd) a run; tokens a case: \($w) cache write, \($r) cache read, \($i) input, \($o * $s | floor) output. With the code-review fan-out: up to ~\($c * $n * 4 | usd)."' "$roles_json"
  exit 0
fi

command -v "$CLAUDE" >/dev/null 2>&1 || die "cannot run '$CLAUDE' (install Claude Code, or set EVAL_CLAUDE)"

# ── Hashes: identical in .claude/lib/evals.sh (checks/evals.sh holds the copies together) ────
evals_roles_hash() {  # evals_roles_hash MODEL_ROLES_JSON_FILE: every role's choice (receipt context only)
  jq -cS '.roles | map_values({model: .model, effort: .effort, tiers: .tiers})' "$1" 2>/dev/null | git hash-object --stdin
}
evals_role_hash() {  # evals_role_hash ROLE MODEL_ROLES_JSON_FILE: one role's model, effort and tier variants
  jq -cS --arg r "$1" '(.roles[$r] // {}) | {model: .model, effort: .effort, tiers: .tiers}' "$2" 2>/dev/null | git hash-object --stdin
}
evals_triggers() {  # evals_triggers ROLE MODEL_ROLES_JSON_FILE: the role's trigger inputs, one per line, sorted
  jq -r --arg r "$1" '
    ([(.agents // {}) | to_entries[] | select(.value == $r) | .key] | first // $r) as $a
    | ((.evals.triggers // {})[$r] // [".claude/agents/\($a).md", ".claude/workflows/"]) | .[]' "$2" 2>/dev/null | LC_ALL=C sort -u
}
evals_section() {  # evals_section MARKER < FILE: the lines between the MARKER lines; fails unless each is there once, in order
  awk -v o="<$1>" -v c="</$1>" '
    { t = $0; m = (t ~ /^[ \t]*(\/\/|#)/); sub(/^[ \t]*(\/\/|#)[ \t]*/, "", t); sub(/[ \t]+$/, "", t) }
    m && t == o { no++; if (inside || no > 1) bad = 1; inside = 1; next }
    m && t == c { nc++; if (!inside || nc > 1) bad = 1; inside = 0; next }
    inside { print }
    END { if (bad || no != 1 || nc != 1 || inside) exit 1 }'
}
evals_blob() {  # evals_blob FILE: its blob id, or "none"
  if [ -f "$1" ]; then git hash-object "$1"; else echo none; fi
}
evals_tree_hash() {  # evals_tree_hash DIR REL: one id for every file under DIR, named by REL/<path>
  if [ -d "$1" ]; then
    (cd "$1" && find . -type f ! -name .DS_Store | sed 's|^\./||' | LC_ALL=C sort | while IFS= read -r _f; do
      printf '%s %s/%s\n' "$(git hash-object "$_f")" "$2" "$_f"
    done)
  fi | LC_ALL=C sort -k2 | git hash-object --stdin
}
evals_input_hash() {  # evals_input_hash ROOT INPUT: one trigger input's id; "none" if absent, "markers-missing" for a section that is not there
  case "$2" in
    *'#'*)
      if [ ! -f "$1/${2%%#*}" ]; then echo none
      elif _es=$(evals_section "${2#*#}" < "$1/${2%%#*}"); then printf '%s\n' "$_es" | git hash-object --stdin
      else echo markers-missing; fi ;;
    */) evals_tree_hash "$1/${2%/}" "${2%/}" ;;
    *) evals_blob "$1/$2" ;;
  esac
}
# ── end of the duplicated functions ───────────────────────────────────────────────────────────

# What the receipt is held to (OPS-16): the role's own choice and its trigger inputs, hashed BEFORE
# any case runs, so an input edited mid-run cannot be certified. A marked section that is missing
# stops the run here: a receipt naming "markers-missing" would certify nothing.
hrole=$(evals_role_hash "$role" "$roles_json")
trig=$(evals_triggers "$role" "$roles_json" | while IFS= read -r _i; do printf '%s\t%s\n' "$_i" "$(evals_input_hash "$ROOT" "$_i")"; done)
mm=$(printf '%s\n' "$trig" | awk -F'\t' '$2 == "markers-missing" { print $1 }' | tr '\n' ' ')
[ -z "$mm" ] || die "trigger input(s) ${mm}have no marked section (each marker once, in order, on lines of their own); restore the markers before running the eval"
trig_json=$(printf '%s\n' "$trig" | jq -Rn '[inputs | select(length > 0) | split("\t") | {(.[0]): .[1]}] | add // {}') || die "could not hash the trigger inputs"

# The agent under test: the one model-roles.json .agents maps to the role.
agent=$(jq -r --arg r "$role" '(.agents // {}) | to_entries[] | select(.value == $r) | .key' "$roles_json" | head -1)
[ -n "$agent" ] || agent=$role
agent_md=${EVAL_AGENT_FILE:-$ROOT/.claude/agents/$agent.md}
[ -f "$agent_md" ] || die "cannot find the $agent agent ($agent_md); a plugin project names its copy with EVAL_AGENT_FILE"

work=$(mktemp -d "${TMPDIR:-/tmp}/evals.XXXXXX") || die "mktemp failed"
[ -n "$keep" ] || trap 'rm -rf "$work"' EXIT
trap 'rm -rf "$work"; exit 2' INT TERM

# The agent as --agents JSON, with the model under test in place of its frontmatter's.
front=$(awk 'NR==1 && $0!="---"{exit} NR>1 && $0=="---"{exit} NR>1' "$agent_md")
body=$(awk 'n>=2{print} /^---$/{n++}' "$agent_md")
fmv() { printf '%s\n' "$front" | sed -n "s/^$1:[[:space:]]*//p" | head -1 | sed 's/^"//; s/"$//'; }
jq -n --arg n "$agent" --arg d "$(fmv description)" --arg p "$body" --arg t "$(fmv tools)" --arg m "$model" \
  '{($n): {description: $d, prompt: $p, model: $m, tools: ($t | split(",") | map(gsub("^\\s+|\\s+$"; "")) | map(select(length > 0)))}}' \
  > "$work/agents.json" || die "could not build the agent JSON from $agent_md"

# build-change.js's REVIEW schema: the findings the workflow grades rounds by.
schema='{"type":"object","properties":{"findings":{"type":"array","items":{"type":"object","properties":{"severity":{"type":"string","enum":["critical","high","medium","low"]},"file":{"type":"string"},"line":{"type":"number"},"summary":{"type":"string"},"failure":{"type":"string","description":"concrete input/state -> wrong result"},"group":{"type":"number"}},"required":["severity","file","summary","failure","group"]}}},"required":["findings"]}'
tools="Read Grep Glob Skill Agent Bash(git *)"

: > "$work/results.jsonl"
i=0
for c in $cases; do
  i=$((i + 1))
  cd_="$suite/$c"; w="$work/$c"
  mkdir -p "$w"
  git -C "$w" init -q 2>/dev/null
  git -C "$w" config user.email eval@example.com; git -C "$w" config user.name eval; git -C "$w" config commit.gpgsign false
  [ -f "$dir/$role/AGENTS.md" ] && cp "$dir/$role/AGENTS.md" "$w/AGENTS.md"
  [ -d "$cd_/before" ] && cp -R "$cd_/before/." "$w/"
  mkdir -p "$w/changes/eval-$c"
  jq -r '"# " + .title + "\n\n## Context\n\n" + .brief + "\n"' "$cd_/case.json" > "$w/changes/eval-$c/design.md"
  jq -r '"## 1. " + .title + "\n\n" + ((.tasks // []) | to_entries | map("- [x] 1.\(.key + 1) \(.value)") | join("\n")) + "\n"' "$cd_/case.json" > "$w/changes/eval-$c/tasks.md"
  git -C "$w" add -A && git -C "$w" commit -qm "before" || die "case $c: could not commit its before tree"
  [ -d "$cd_/after" ] && cp -R "$cd_/after/." "$w/"
  for p in $(jq -r '(.delete // [])[]' "$cd_/case.json"); do git -C "$w" rm -q -- "$p" || die "case $c: cannot delete $p"; done
  # As the caller does: new files intent-to-add, deletions staged, so `git diff HEAD` is the change.
  git -C "$w" add -N . 2>/dev/null
  [ -n "$(git -C "$w" diff HEAD --stat)" ] || die "case $c: its before and after trees do not differ"

  title=$(jq -r .title "$cd_/case.json")
  prompt="Review task group 1 (\"$title\") of change \"eval-$c\" (changes/eval-$c/). The group's work is the current UNCOMMITTED working-tree diff."
  printf '[%s/%s] %s ... ' "$i" "$n" "$c" >&2
  rc=0
  # shellcheck disable=SC2086
  (cd "$w" && EVAL_CASE_DIR="$cd_" "$CLAUDE" -p "$prompt" --agents "$work/agents.json" --agent "$agent" \
      --model "$model" --effort "$effort" --output-format json --json-schema "$schema" \
      --no-session-persistence --setting-sources project --permission-mode dontAsk \
      --allowedTools "$tools" --max-budget-usd "$budget" ${EVAL_CLAUDE_FLAGS:-} \
      > "$work/$c.out" 2> "$work/$c.err" < /dev/null) || rc=$?
  r=$(jq -c --arg id "$c" --argjson rc "$rc" --slurpfile k "$cd_/case.json" '
      (.structured_output // (.result | if type == "string" then (try fromjson catch null) else . end)) as $o
      | { id: $id, kind: $k[0].kind, expected: ($k[0].expected // []),
          findings: (if ($o | type) == "object" and ($o.findings | type) == "array" then $o.findings else [] end),
          cost_usd: (.total_cost_usd // 0), usage: (.usage // {}),
          error: (if $rc != 0 then "exit \($rc)"
                  elif .is_error == true then "is_error: \(.result // "" | tostring | .[0:200])"
                  elif (($o | type) != "object" or ($o.findings | type) != "array") then "no findings array in the result"
                  else null end) }' "$work/$c.out" 2>/dev/null)
  if [ -z "$r" ]; then
    r=$(jq -cn --arg id "$c" --argjson rc "$rc" --arg e "$(head -c 300 "$work/$c.err" 2>/dev/null)" --slurpfile k "$cd_/case.json" \
      '{id: $id, kind: $k[0].kind, expected: ($k[0].expected // []), findings: [], cost_usd: 0, usage: {},
        error: ((if $rc != 0 then "exit \($rc)" else "unreadable output" end) + (if $e != "" then ": " + $e else "" end))}')
  fi
  printf '%s\n' "$r" >> "$work/results.jsonl"
  printf '%s\n' "$r" | jq -r 'if .error then "ERROR \(.error)" else "\(.findings | length) finding(s), $\(.cost_usd)" end' >&2
done

# ── Score ─────────────────────────────────────────────────────────────────────────────────────
date=${EVAL_DATE:-$(date -u +%Y-%m-%d)}
mkdir -p "$out" || die "cannot create $out"
safe=$(printf '%s-%s-%s' "$role" "$model" "$effort" | tr -c 'A-Za-z0-9._\n-' '_')
receipt="$out/$safe-$date$( [ "$complete" = true ] || echo -partial).json"
suite_hash=$(evals_tree_hash "$suite" ".claude/evals/$role/cases")

jq -s --arg role "$role" --arg model "$model" --arg effort "$effort" --arg date "$date" \
   --arg at "$(date -u +%Y-%m-%dT%H:%M:%SZ)" --arg agent "$agent" --argjson complete "$complete" \
   --arg hr "$(evals_roles_hash "$roles_json")" --arg ha "$(evals_blob "$agent_md")" \
   --arg hw "$(evals_tree_hash "$ROOT/.claude/workflows" .claude/workflows)" --arg hs "$suite_hash" \
   --arg hrole "$hrole" --argjson trig "$trig_json" \
   --slurpfile mr "$roles_json" '
  def rank: {"low": 1, "medium": 2, "high": 3, "critical": 4}[.] // 0;
  def r3: if type == "number" then (. * 1000 + 0.5 | floor) / 1000 else . end;
  def norm: ascii_downcase | sub("^\\./"; "");
  def samefile($a; $b): ($a | norm) as $x | ($b | norm) as $y
    | $x == $y or ($x | endswith("/" + $y)) or ($y | endswith("/" + $x));
  def linehit($f; $d):
    samefile($f.file // ""; $d.file)
    and ($f.line | type) == "number" and $f.line >= ($d.lines[0] - 3) and $f.line <= ($d.lines[1] + 3);
  def kwhit($f; $d):
    ((($f.summary // "") + " " + ($f.failure // "")) | ascii_downcase) as $t
    | samefile($f.file // ""; $d.file)
      and any(($d.keywords // [])[]; . as $k | $t | contains($k | ascii_downcase));
  # A candidate finding'"'"'s priority for a defect: a line hit before a keyword hit, and an
  # actionable finding before a low one. 9 and above is no match.
  def prio($f; $d):
    (if linehit($f; $d) then 0 elif kwhit($f; $d) then 2 else 9 end)
    + (if ($f.severity | rank) >= 2 then 0 else 1 end);
  def score_case:
    . as $c
    | (reduce ($c.expected | to_entries[]) as $e ({used: [], m: []};
        . as $st
        | ([ $c.findings | to_entries[]
             | select(.key as $k | any($st.used[]; . == $k) | not)
             | . + {p: prio(.value; $e.value)} | select(.p < 9) ]
           | sort_by(.p) | first) as $hit
        | if $hit == null then .m += [{defect: $e.value.id, expected: $e.value.severity, caught: false}]
          else .used += [$hit.key]
             | .m += [{defect: $e.value.id, expected: $e.value.severity, caught: true, severity: $hit.value.severity, finding: $hit.key}]
          end)) as $r
    | ([$c.findings | to_entries[] | select(.value.severity | rank >= 2) | .key]) as $act
    | $c + { matches: $r.m,
             caught: ([$r.m[] | select(.caught)] | length),
             actionable: ($act | length),
             true_positives: ([$act[] | . as $k | select(any($r.used[]; . == $k))] | length) }
    | . + { false_positives: (.actionable - .true_positives) };
  ($mr[0].evals.thresholds // {}) as $t
  | map(score_case) as $cs
  | ([$cs[] | .expected | length] | add // 0) as $planted
  | ([$cs[] | .caught] | add // 0) as $caught
  | ([$cs[] | .actionable] | add // 0) as $act
  | ([$cs[] | .true_positives] | add // 0) as $tp
  | ([$cs[] | select(.kind == "clean")]) as $clean
  | ([$cs[] | .matches[] | select(.caught)]) as $got
  | ([$cs[] | select(.error != null)] | length) as $errors
  | ([$cs[] | .cost_usd] | add // 0) as $cost
  | { recall: (if $planted > 0 then $caught / $planted else null end),
      precision: (if $act > 0 then $tp / $act else 1 end),
      fp_rate: (if ($clean | length) > 0 then ([$clean[] | select(.actionable > 0)] | length) / ($clean | length) else null end),
      severity_accuracy: (if ($got | length) > 0 then ([$got[] | select(.severity == .expected)] | length) / ($got | length) else 0 end),
      severity_within_one: (if ($got | length) > 0 then ([$got[] | select(((.severity | rank) - (.expected | rank)) as $x | (if $x < 0 then -$x else $x end) <= 1)] | length) / ($got | length) else 0 end)
    } | map_values(r3) as $s
  | { schema: 1, role: $role, model: $model, effort: $effort, date: $date, run_at: $at, agent: $agent,
      hashes: { role: $hrole, triggers: $trig, model_roles: $hr, agent: $ha, workflows: $hw },
      suite: { cases: ($cs | length), defect_cases: ([$cs[] | select(.kind != "clean")] | length),
               clean_cases: ($clean | length), planted: $planted, hash: $hs },
      complete: $complete, errors: $errors,
      thresholds: { recall: ($t.recall // 0.8), fp_rate: ($t.fp_rate // 0.2), severity_accuracy: ($t.severity_accuracy // 0) },
      scores: $s,
      cost: { usd: ($cost | r3),
              input_tokens: ([$cs[] | .usage.input_tokens // 0] | add // 0),
              output_tokens: ([$cs[] | .usage.output_tokens // 0] | add // 0),
              cache_read_tokens: ([$cs[] | .usage.cache_read_input_tokens // 0] | add // 0),
              cache_write_tokens: ([$cs[] | .usage.cache_creation_input_tokens // 0] | add // 0),
              usd_per_catch: (if $caught > 0 then ($cost / $caught | r3) else null end),
              catches_per_usd: (if $cost > 0 then ($caught / $cost | r3) else null end) },
      cases: [ $cs[] | { id, kind, error, planted: (.expected | length), caught, actionable, false_positives,
                         cost_usd: (.cost_usd | r3), matches, findings } ] }
  | . + { pass: (.complete and .errors == 0
                 and ($s.recall | type) == "number" and $s.recall >= .thresholds.recall
                 and ($s.fp_rate | type) == "number" and $s.fp_rate <= .thresholds.fp_rate
                 and $s.severity_accuracy >= .thresholds.severity_accuracy) }
' "$work/results.jsonl" > "$receipt.tmp" && mv "$receipt.tmp" "$receipt" || die "could not score the run"

[ -n "$keep" ] && echo "kept: $work" >&2
rel=${receipt#"$ROOT"/}
jq -r --arg p "$rel" '
  "\(.role) on \(.model)/\(.effort): recall \(.scores.recall), precision \(.scores.precision), false-positive rate \(.scores.fp_rate), severity accuracy \(.scores.severity_accuracy); $\(.cost.usd) (\(.cost.catches_per_usd // "-") catches per $)",
  "\(if .pass then "PASS" else "NOT PASSING" end) against recall >= \(.thresholds.recall), fp_rate <= \(.thresholds.fp_rate), severity_accuracy >= \(.thresholds.severity_accuracy)\(if .complete then "" else "; partial: a --cases run is never evidence" end)\(if .errors > 0 then "; \(.errors) case(s) failed to run" else "" end)",
  "receipt: \($p)",
  (if .pass and .complete then "record it: model-roles.json .roles.\(.role).eval = {\"receipt\": \"\($p)\", \"recall\": \(.scores.recall), \"precision\": \(.scores.precision), \"fp_rate\": \(.scores.fp_rate), \"severity_accuracy\": \(.scores.severity_accuracy), \"cost_usd\": \(.cost.usd)}" else empty end)' "$receipt"
jq -e .pass "$receipt" >/dev/null
