#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# metrics.sh and usage-report.sh (OPS-9), falsified in a throwaway repository with a fake gh and
# fake transcripts, against the panel's metrics contract (docs/panel.md in the clauductor
# repository, *Metrics*, version 1):
#   the contract   a validator in jq of what the panel's parser refuses (an unknown key, another
#                  version or window, a negative or non-finite number, a percent over 100, a count
#                  that is not whole, control characters, text over 300, lists over 500, series over
#                  120); first it must refuse each of those, and accept the contract's own example
#   the figures    lead and cycle time, merge frequency, change-fail rate and escaped defects from
#                  merged PRs (a revert, and a fix/ PR naming a change); approval wait (to the hour,
#                  and to the day where one commit carries both); review rounds; aging work in
#                  progress (a merged branch left out); outcomes due and checked; cost by role
#                  (an agent's agent charged to its root, a parent cycle to "?"), model, change (with
#                  its budget) and project, once per message id across directories, an unpriced
#                  model named, another repository's transcripts ignored
#   failing soft   no gh, gh failing, offline, no transcripts, no jq: still a valid payload, each
#                  missing figure null with a note saying why
#   usage-report   the same pricing by role and model, CANNOT CHECK (exit 2) with nothing to report
. "$(dirname "$0")/lib.sh"
need git jq

d=$(scratch)

# ── The contract, as a validator ──────────────────────────────────────────────────────────────
# Prints the first violation as "<path>: <why>", or "valid". Mirrors the panel's metrics.Parse.
cat > "$d/contract.jq" <<'JQ'
def e($p; $m): "\($p): \($m)";
def num($p; $max): if type != "number" then e($p; "must be a number")
  elif . < 0 then e($p; "must be a number, 0 or more")
  elif $max > 0 and . > $max then e($p; "must be at most \($max)") else empty end;
def int($p): if type != "number" or . != floor or . < 0 then e($p; "must be a whole number, 0 or more") else empty end;
def ctl: explode | any(. < 32 or (. >= 127 and . < 160) or . == 173 or (. >= 1536 and . <= 1541)
  or . == 1564 or . == 1757 or . == 1807 or . == 6158 or (. >= 8203 and . <= 8207) or (. >= 8234 and . <= 8238)
  or (. >= 8288 and . <= 8292) or (. >= 8294 and . <= 8303) or . == 65279 or (. >= 65529 and . <= 65531));
def text($p; $req): if . == null then (if $req then e($p; "is required") else empty end)
  elif type != "string" then e($p; "must be text")
  elif $req and test("^\\s*$") then e($p; "is required")
  elif length > 300 then e($p; "is longer than 300 characters")
  elif ctl then e($p; "contains a control character") else empty end;
def only($p; $k): if type != "object" then e($p; "must be an object") else (keys - $k)[] as $x | e($p + "." + $x; "is not a key of the contract") end;
def list($p): if type != "array" then e($p; "must be a list") elif length > 500 then e($p; "has more than 500 entries") else empty end;
def opt($k; f): if type == "object" and has($k) and .[$k] != null then .[$k] | f else empty end;
def items($p): if type == "array" then to_entries[] | {p: "\($p)[\(.key)]", v: .value} else empty end;
def stat($p; $max): if . == null then empty else only($p; ["value", "series", "n", "note"]),
  opt("value"; num("\($p).value"; $max)),
  opt("series"; if type != "array" then e("\($p).series"; "must be a list") elif length > 120 then e("\($p).series"; "has more than 120 points")
                else to_entries[] | select(.value != null) | .key as $i | .value | num("\($p).series[\($i)]"; $max) end),
  opt("n"; int("\($p).n")), opt("note"; text("\($p).note"; false)) end;
def amounts($p): list($p), (items($p) | .p as $q | .v | only($q; ["name", "usd", "budget_usd"]), (.name | text("\($q).name"; true)),
  opt("usd"; num("\($q).usd"; 0)), opt("budget_usd"; num("\($q).budget_usd"; 0)));
def window($p):
  only($p; ["flow", "cost", "quality", "outcomes"]),
  opt("flow"; "\($p).flow" as $f | only($f; ["lead_time", "cycle_time", "approval_wait", "merge_frequency", "change_fail_rate", "aging_wip"]),
    (.lead_time | stat("\($f).lead_time"; 0)), (.cycle_time | stat("\($f).cycle_time"; 0)),
    (.approval_wait | stat("\($f).approval_wait"; 0)), (.merge_frequency | stat("\($f).merge_frequency"; 0)),
    (.change_fail_rate | stat("\($f).change_fail_rate"; 100)),
    opt("aging_wip"; list("\($f).aging_wip"), (items("\($f).aging_wip") | .p as $q | .v | only($q; ["id", "title", "age_days", "stage"]),
      (.id | text("\($q).id"; true)), opt("title"; text("\($q).title"; false)), opt("stage"; text("\($q).stage"; false)),
      opt("age_days"; num("\($q).age_days"; 0))))),
  opt("cost"; "\($p).cost" as $c | only($c; ["total_usd", "per_week", "by_role", "by_model", "by_change", "by_project"]),
    opt("total_usd"; num("\($c).total_usd"; 0)), (.per_week | stat("\($c).per_week"; 0)),
    opt("by_role"; amounts("\($c).by_role")), opt("by_model"; amounts("\($c).by_model")),
    opt("by_change"; amounts("\($c).by_change")), opt("by_project"; amounts("\($c).by_project"))),
  opt("quality"; "\($p).quality" as $q | only($q; ["review_rounds", "reviewer_recall", "escaped_defects"]),
    (.review_rounds | stat("\($q).review_rounds"; 0)), (.escaped_defects | stat("\($q).escaped_defects"; 0)),
    opt("reviewer_recall"; list("\($q).reviewer_recall"), (items("\($q).reviewer_recall") | .p as $r | .v | only($r; ["model", "pct", "n"]),
      (.model | text("\($r).model"; true)), opt("pct"; num("\($r).pct"; 100)), opt("n"; int("\($r).n"))))),
  opt("outcomes"; "\($p).outcomes" as $o | only($o; ["hypotheses"]),
    opt("hypotheses"; list("\($o).hypotheses"), (items("\($o).hypotheses") | .p as $h | .v | only($h; ["change", "hypothesis", "due", "checked", "result"]),
      (.change | text("\($h).change"; true)), (.hypothesis | text("\($h).hypothesis"; true)), opt("result"; text("\($h).result"; false)),
      opt("checked"; if type != "boolean" then e("\($h).checked"; "must be true or false") else empty end),
      opt("due"; if type != "string" or (. != "" and (test("^[0-9]{4}-[0-9]{2}-[0-9]{2}$") | not)) then e("\($h).due"; "must be a date, YYYY-MM-DD") else empty end))));
[ ( if type != "object" then e("metrics JSON"; "must be an object") else
      only("metrics JSON"; ["version", "generated_at", "windows"]),
      (if .version != 1 then e("version"; "must be 1") else empty end),
      opt("generated_at"; int("generated_at")),
      (if (.windows | type) != "object" then e("windows"; "is required (an object keyed 7d, 30d, 90d)")
       else .windows | to_entries[] | .key as $k
         | if ($k | IN("7d", "30d", "90d") | not) then e("windows.\($k)"; "a window is 7d, 30d or 90d")
           elif .value == null then e("windows.\($k)"; "is null")
           else .value | window("windows.\($k)") end end)
    end ) ] | first // "valid"
JQ
validate() { # FILE: "valid", or the first violation; refuses output of 1 MB or more, and two values
  if [ "$(wc -c < "$1")" -ge 1048576 ]; then echo "metrics output is 1 MB or more"; return; fi
  _n=$(jq -s length "$1" 2>/dev/null) || { echo "not JSON: $(head -c 120 "$1")"; return; }
  [ "$_n" = 1 ] || { echo "metrics JSON: $_n values, want exactly one"; return; }
  jq -r -f "$d/contract.jq" "$1" 2>&1 | head -1
}

# The validator first: it must refuse each thing the panel refuses (else every pass below is evidence
# of nothing), and accept the contract's own example.
good='{"version":1,"generated_at":1790000000,"windows":{"30d":{"flow":{"lead_time":{"value":44,"series":[50,null,46],"n":12},"cycle_time":{"value":7.5,"n":12},"approval_wait":{"value":null,"note":"none"},"merge_frequency":{"value":2.8},"change_fail_rate":{"value":8.3,"n":12},"aging_wip":[{"id":"add-score-photo","title":"Photograph a scorecard","age_days":4.5,"stage":"build"}]},"cost":{"total_usd":162.4,"per_week":{"value":37.9},"by_role":[{"name":"builder","usd":90.1}],"by_model":[{"name":"opus","usd":140.4}],"by_change":[{"name":"add-score-photo","usd":61.5,"budget_usd":50}],"by_project":[{"name":"My Project","usd":162.4}]},"quality":{"review_rounds":{"value":1.5,"n":12},"reviewer_recall":[{"model":"opus","pct":92,"n":40}],"escaped_defects":{"value":1}},"outcomes":{"hypotheses":[{"change":"add-score-photo","hypothesis":"Half of new cards start from a photo","due":"2026-10-20","checked":false,"result":""}]}}}}'
printf '%s\n' "$good" > "$d/good.json"
v=$(validate "$d/good.json"); [ "$v" = valid ] && ok "contract: the validator accepts the contract's example" || fail "contract: the example was refused: $v"
bad() { # LABEL JQ-EDIT WANT: the edited example must be refused, naming WANT
  printf '%s\n' "$good" | jq -c "$2" > "$d/bad.json"
  v=$(validate "$d/bad.json")
  case "$v" in *"$3"*) ok "contract: the validator refuses $1 ($v)" ;; *) fail "contract: $1 was not refused as '$3' (got: $v)" ;; esac
}
bad "a percent over 100" '.windows."30d".flow.change_fail_rate.value = 140' "change_fail_rate.value: must be at most 100"
bad "an unknown key" '.windows."30d".flow.velocity = {value: 1}' "flow.velocity: is not a key"
bad "another version" '.version = 2' "version: must be 1"
bad "another window" '.windows."14d" = {}' "windows.14d: a window is"
bad "a negative figure" '.windows."30d".cost.total_usd = -1' "total_usd: must be a number, 0 or more"
bad "a count that is not whole" '.windows."30d".flow.cycle_time.n = 1.5' "cycle_time.n: must be a whole number"
bad "a control character" '.windows."30d".flow.aging_wip[0].title = "a\u0007b"' "title: contains a control character"
bad "text over 300 characters" '.windows."30d".cost.by_role[0].name = ("x" * 301)' "by_role[0].name: is longer than 300"
bad "a blank required name" '.windows."30d".cost.by_role[0].name = " "' "by_role[0].name: is required"
bad "a series over 120 points" '.windows."30d".flow.lead_time.series = [range(121)]' "lead_time.series: has more than 120"
bad "a list over 500 entries" '.windows."30d".flow.aging_wip = [range(501) | {id: "x", age_days: 1}]' "aging_wip: has more than 500"
bad "a recall over 100" '.windows."30d".quality.reviewer_recall[0].pct = 101' "pct: must be at most 100"
bad "a malformed due date" '.windows."30d".outcomes.hypotheses[0].due = "20 Oct"' "due: must be a date"
bad "a string for a number" '.windows."30d".flow.lead_time.value = "44"' "lead_time.value: must be a number"
printf '%s\n%s\n' "$good" "$good" > "$d/two.json"
case "$(validate "$d/two.json")" in *"2 values"*) ok "contract: the validator refuses two values" ;; *) fail "contract: two values were accepted" ;; esac

# ── A repository with history, change records and a roadmap ─────────────────────────────────
R="$d/app"; new_repo "$R"; R=$(cd "$R" && pwd -P)
mkdir -p "$R/.claude/lib" "$R/changes/archive" "$R/docs"
cp "$CLAUDUCTOR_FW/lib/conf.sh" "$CLAUDUCTOR_FW/lib/change.sh" "$CLAUDUCTOR_FW/lib/usage.sh" "$R/.claude/lib/"
cp "$CLAUDUCTOR_FW/metrics.sh" "$CLAUDUCTOR_FW/usage-report.sh" "$CLAUDUCTOR_FW/roadmap-queue.sh" "$ROOT/.claude/model-roles.json" "$R/.claude/"
printf 'PROJECT_NAME="Fixture"\n' > "$R/.claude/project.conf"
at() { # DATE MESSAGE: commit everything, dated
  git -C "$R" add -A && GIT_AUTHOR_DATE="$1" GIT_COMMITTER_DATE="$1" git -C "$R" commit -qm "$2"
}
cat > "$R/docs/roadmap.md" <<'MD'
# Roadmap

## Phase 1 — Cards
**Owner:** Ana

| # | Change | Scope | Deps | Status |
|---|--------|-------|------|--------|
| 1.1 | `add-photo` — Photograph a scorecard | photo | — | ✅ merged (#1) |
| 1.2 | `group-card` — One golfer enters the card | card · Budget: $50 | — | ⬜ in flight (#6) |
| 1.3 | `tee-times` — Book a tee time | times | — | ⬜ queued |
| 1.4 | `old-thing` — The old thing | old | — | ✅ merged (#7) |

## Outcome checks

| # | Change | Scope | Deps | Status |
|---|--------|-------|------|--------|
| o.1 | `ops/check-outcome-add-photo` — check the outcome of add-photo (due 2026-10-21) | photos | — | ⬜ queued |
| o.2 | `ops/check-outcome-old-thing` — check the outcome of old-thing (due 2026-09-01) | old | — | ✅ merged (#8) |
MD
at 2026-07-01T00:00:00Z base
proposal() { # DIR STATUS-LINE [BUDGET]
  mkdir -p "$1"
  { printf '%s\n**Roadmap row:** 1.1\n**Risk:** low\n' "$2"; [ -n "${3:-}" ] && printf '**Budget:** $%s\n' "$3"
    printf '\n## Why\nBecause.\n\n## How we'"'"'ll know\n- **Signal:** %s\n- **Check after:** 30 days\n' "${4:-}"; } > "$1/proposal.md"
}
# old-thing: approved and archived long ago; its outcome was checked (row o.2 merged) on 2026-09-01.
proposal "$R/changes/old-thing" "**Approved:** 2026-07-20 by Ana · design 000000000000" "" "The old signal"
at 2026-07-20T00:00:00Z "old-thing: propose"
git -C "$R" mv changes/old-thing changes/archive/2026-08-01-old-thing && at 2026-08-01T00:00:00Z "old-thing: archive"
# add-photo: written at 09-10 00:00, approved by a later commit at 06:00 (6 h), built in 2 + 1 rounds,
# archived 09-20; its outcome row is due 2026-10-21.
proposal "$R/changes/add-photo" "**Status:** awaiting approval" "" "Half of new cards start from a photo"
at 2026-09-10T00:00:00Z "add-photo: propose"
sed 's/^\*\*Status:\*\* awaiting approval/**Approved:** 2026-09-10 by Ana · design 000000000000/' "$R/changes/add-photo/proposal.md" > "$d/p" && cp "$d/p" "$R/changes/add-photo/proposal.md"
printf '# Tasks\n\n## Progress\n- 2026-09-15 group 1 (a) built and reviewed: converged in 2 round(s), peak low; grades R1 concern, R2 pass\n- 2026-09-16 group 2 (b) built and reviewed: converged in 1 round(s), peak none; grades R1 pass\n\n## 1. a\n- [x] 1.1 done\n' > "$R/changes/add-photo/tasks.md"
at 2026-09-10T06:00:00Z "add-photo: approve"
git -C "$R" mv changes/add-photo changes/archive/2026-09-20-add-photo && at 2026-09-20T00:00:00Z "add-photo: archive"
# group-card: proposed and approved in ONE commit (a squash) on 09-25, Approved line dated 09-26:
# day precision gives 24 h. Budget $50, 3 review rounds on 09-27, tasks open, an open PR: review.
proposal "$R/changes/group-card" "**Approved:** 2026-09-26 by Ana · design 000000000000" 50
printf '# Tasks\n\n## Progress\n- 2026-09-27 group 1 (a) built and reviewed: converged in 3 round(s), peak low\n\n## 1. a\n- [x] 1.1 done\n- [ ] 1.2 open\n' > "$R/changes/group-card/tasks.md"
at 2026-09-25T00:00:00Z "group-card: propose"
# tee-times: written 09-29 12:00, awaiting approval.
proposal "$R/changes/tee-times" "**Status:** awaiting approval"
at 2026-09-29T12:00:00Z "tee-times: propose"
# Branches: fix/12-slow in flight since 09-28 12:00; ops/tidy merged by PR #2 after its last commit.
git -C "$R" checkout -q -b fix/12-slow && echo slow > "$R/slow" && at 2026-09-28T12:00:00Z "slow"
git -C "$R" checkout -q main
git -C "$R" checkout -q -b ops/tidy && echo tidy > "$R/tidy" && at 2026-09-19T20:00:00Z "tidy"
git -C "$R" checkout -q main
git -C "$R" update-ref refs/remotes/origin/main main

# ── A fake gh ─────────────────────────────────────────────────────────────────────────────────
# Merged PRs (now = 2026-09-30 12:00):
#   #1 change/add-photo  first commit 09-11, opened 09-12, merged 09-14: lead 72 h, cycle 48 h; FAILED (#3 names add-photo)
#   #2 ops/tidy          first 09-19 20:00, opened 09-20 00:00, merged 02:00: lead 6, cycle 2; FAILED (#5 reverts it)
#   #3 fix/9-photo-crash merged 09-21: a remediation naming the change id add-photo
#   #4 ops/docs          first 09-27, opened 09-28, merged 09-29: lead 48, cycle 24
#   #5 Revert "Tidy the ops", merged 09-29 06:00: a remediation of #2
B="$d/bin"; mkdir -p "$B"
cat > "$d/search.json" <<'JSON'
{"data":{"search":{"pageInfo":{"hasNextPage":false,"endCursor":null},"nodes":[
 {"number":1,"title":"Photograph a scorecard","body":"Builds add-photo.","headRefName":"change/add-photo","createdAt":"2026-09-12T00:00:00Z","mergedAt":"2026-09-14T00:00:00Z","commits":{"nodes":[{"commit":{"authoredDate":"2026-09-11T00:00:00Z"}}]}},
 {"number":2,"title":"Tidy the ops","body":"","headRefName":"ops/tidy","createdAt":"2026-09-20T00:00:00Z","mergedAt":"2026-09-20T02:00:00Z","commits":{"nodes":[{"commit":{"authoredDate":"2026-09-19T20:00:00Z"}}]}},
 {"number":3,"title":"Fix the photo crash","body":"The add-photo upload crashed on large files (#40 is unrelated).","headRefName":"fix/9-photo-crash","createdAt":"2026-09-21T00:00:00Z","mergedAt":"2026-09-21T01:00:00Z","commits":{"nodes":[{"commit":{"authoredDate":"2026-09-21T00:00:00Z"}}]}},
 {"number":4,"title":"Document the cards","body":"","headRefName":"ops/docs","createdAt":"2026-09-28T00:00:00Z","mergedAt":"2026-09-29T00:00:00Z","commits":{"nodes":[{"commit":{"authoredDate":"2026-09-27T00:00:00Z"}}]}},
 {"number":5,"title":"Revert \"Tidy the ops\"","body":"","headRefName":"revert-2-tidy","createdAt":"2026-09-29T05:00:00Z","mergedAt":"2026-09-29T06:00:00Z","commits":{"nodes":[{"commit":{"authoredDate":"2026-09-29T05:00:00Z"}}]}},
 {}
]}}}
JSON
echo '[{"number":6,"title":"One golfer enters the card","headRefName":"change/group-card","createdAt":"2026-09-27T00:00:00Z"}]' > "$d/open.json"
cat > "$B/gh" <<SH
#!/bin/sh
printf '%s\n' "\$*" >> "$d/gh.calls"
[ -n "\${FAKE_GH_FAIL:-}" ] && exit 1
case "\$1 \$2" in
  "api graphql") cat "$d/search.json" ;;
  "pr list") cat "$d/open.json" ;;
  *) exit 1 ;;
esac
SH
chmod +x "$B/gh"

# ── Fake transcripts ──────────────────────────────────────────────────────────────────────────
P="$d/projects"; slug=$(printf '%s' "$R" | sed 's/[^A-Za-z0-9]/-/g')
S="$P/$slug"; mkdir -p "$S/sid1/subagents/workflows/run1" "$P/$slug--claude-worktrees-lane1" "$P/$slug-other"
msg() { # ID TIME BRANCH MODEL USAGE-JSON
  printf '{"type":"assistant","timestamp":"%s.000Z","gitBranch":"%s","cwd":"%s","sessionId":"x","message":{"id":"%s","model":"%s","usage":%s}}\n' "$2" "$3" "$R" "$1" "$4" "$5"
}
{
  msg m1 2026-09-29T10:00:00 change/group-card 'claude-opus-5-5[1m]' '{"input_tokens":1000000,"output_tokens":0}'   # $4
  msg m1 2026-09-29T10:00:00 change/group-card 'claude-opus-5-5[1m]' '{"input_tokens":1000000,"output_tokens":0}'   # streamed: once
  msg m2 2026-09-10T10:00:00 main claude-sonnet-5 '{"output_tokens":100000}'                                     # $1, 30d only
  msg m3 2026-09-29T11:00:00 main claude-mystery-9 '{"output_tokens":5}'                                         # unpriced
  msg m0 2026-05-01T00:00:00 main claude-opus-5-5 '{"input_tokens":1000000}'                                     # before 90d
  printf '{"type":"user","timestamp":"2026-09-29T10:00:00.000Z","message":{"role":"user","content":"hi"}}\n'
  printf '{"type":"assistant","message":{"usage"\n'                                                               # cut off mid-write
} > "$S/sid1.jsonl"
msg m4 2026-09-29T12:00:00 change/group-card claude-opus-5-5 '{"input_tokens":500000}' > "$S/sid1/subagents/agent-aaa.jsonl"   # $2, builder
echo '{"agentType":"builder"}' > "$S/sid1/subagents/agent-aaa.meta.json"
msg m5 2026-09-29T13:00:00 fix/12-slow claude-opus-5-5 '{"cache_read_input_tokens":1000000}' > "$S/sid1/subagents/workflows/run1/agent-bbb.jsonl"  # $0.20, root builder
echo '{"agentType":"general-purpose","name":"code-review-2","parentAgentId":"aaa"}' > "$S/sid1/subagents/workflows/run1/agent-bbb.meta.json"
msg m6 2026-09-29T14:00:00 fix/12-slow claude-opus-5-5 '{"output_tokens":10000}' > "$S/sid1/subagents/agent-ccc.jsonl"   # $0.20, a cycle: "?"
echo '{"agentType":"general-purpose","parentAgentId":"ddd"}' > "$S/sid1/subagents/agent-ccc.meta.json"
echo '{"agentType":"general-purpose","parentAgentId":"ccc"}' > "$S/sid1/subagents/agent-ddd.meta.json"
msg m1 2026-09-29T10:00:00 change/group-card claude-opus-5-5 '{"input_tokens":1000000}' > "$P/$slug--claude-worktrees-lane1/sid2.jsonl"  # moved by EnterWorktree: once
msg m9 2026-09-29T10:00:00 main claude-opus-5-5 '{"input_tokens":25000000}' > "$P/$slug-other/sidx.jsonl"   # another repository: $100, ignored

NOW=1790769600 # 2026-09-30T12:00:00Z
M() { # [ENV=…]…: run metrics.sh in the fixture with these settings, stdout to $d/m.json
  (cd "$R" && env PATH="$B:$PATH" CONTEXT_OFFLINE= CLAUDE_PROJECTS_DIR="$P" METRICS_NOW=$NOW "$@" sh .claude/metrics.sh) > "$d/m.json"
}
MA() { # ARGS: run metrics.sh in the fixture with these arguments, stdout to $d/m.json
  (cd "$R" && env PATH="$B:$PATH" CONTEXT_OFFLINE= CLAUDE_PROJECTS_DIR="$P" METRICS_NOW=$NOW sh .claude/metrics.sh "$@") > "$d/m.json"
}
q() { jq -c "$1" "$d/m.json"; }
is() { # LABEL JQ WANT
  got=$(q "$2" 2>&1)
  [ "$got" = "$3" ] && ok "$1" || fail "$1: $2 is $got, want $3"
}

M; expect_rc 0 $? "metrics.sh exits 0"
v=$(validate "$d/m.json"); [ "$v" = valid ] && ok "metrics.sh's payload meets the contract" || fail "metrics.sh's payload breaks the contract: $v"
is "it reports the three windows" '.windows | keys' '["30d","7d","90d"]'
is "generated_at is now" '.generated_at' "$NOW"
grep -q 'merged:>=2026-07-02' "$d/gh.calls" && ok "it asks gh for the merges of the widest window (90d)" || fail "gh was not asked for merged:>=2026-07-02: $(cat "$d/gh.calls")"
W=.windows.\"30d\"
is "30d merge frequency: 5 merges in 30 days, per week" "$W.flow.merge_frequency | [.value, .n, (.series | length)]" '[1.2,5,10]'
is "30d cycle time: median of 48, 2, 24 h (remediations excluded)" "$W.flow.cycle_time | [.value, .n]" '[24,3]'
is "30d lead time: median of 72, 6, 48 h from the first commit" "$W.flow.lead_time | [.value, .n]" '[48,3]'
is "30d change-fail rate: 2 of 3 (a revert, a fix naming the change id; its #40 is not a link to #4)" "$W.flow.change_fail_rate | [.value, .n]" '[66.7,3]'
is "30d escaped defects: the 2 remediations" "$W.quality.escaped_defects | [.value, .n]" '[2,3]'
is "30d approval wait: median of 6 h (a later commit) and 24 h (day precision)" "$W.flow.approval_wait | [.value, .n]" '[15,2]'
is "30d review rounds: median of 1.5 (add-photo) and 3 (group-card)" "$W.quality.review_rounds | [.value, .n]" '[2.3,2]'
is "aging WIP: the open changes and the unmerged branch, oldest first; the merged branch left out" \
   "[$W.flow.aging_wip[] | [.id, .age_days, .stage, .title]]" \
   '[["group-card",5.5,"review","One golfer enters the card"],["fix/12-slow",2,"build",null],["tee-times",1,"approval","Book a tee time"]]'
is "outcomes: due from the roadmap row, a checked one inside the window" \
   "[$W.outcomes.hypotheses[] | [.change, .due, .checked, .hypothesis]]" \
   '[["old-thing","2026-09-01",true,"The old signal"],["add-photo","2026-10-21",false,"Half of new cards start from a photo"]]'
is "30d cost: m1 once + m2 + m4 + m5 + m6, not m0, not another repository" "$W.cost | [.total_usd, .per_week.value]" '[7.4,1.73]'
is "30d cost by project" "$W.cost.by_project" '[{"name":"Fixture","usd":7.4}]'
W7=.windows.\"7d\"
is "7d flow: one deploy (#4), not failed; 2 merges" "$W7.flow | [.cycle_time.value, .lead_time.value, .change_fail_rate.value, .merge_frequency.value, (.merge_frequency.series | length)]" '[24,48,0,2,7]'
is "7d approval wait: group-card's 24 h" "$W7.flow.approval_wait | [.value, .n]" '[24,1]'
is "7d review rounds: group-card's 3" "$W7.quality.review_rounds.value" '3'
is "7d outcomes: a checked one due before the window is left out" "[$W7.outcomes.hypotheses[].change]" '["add-photo"]'
is "7d cost total and per week" "$W7.cost | [.total_usd, .per_week.value]" '[6.4,6.4]'
is "7d cost by role: an agent's agent charged to its root, a parent cycle to ?" "$W7.cost.by_role" \
   '[{"name":"session","usd":4},{"name":"builder","usd":2.2},{"name":"?","usd":0.2}]'
is "7d cost by model, the unpriced one at 0" "$W7.cost.by_model" '[{"name":"opus-5-5","usd":6.4},{"name":"mystery-9","usd":0}]'
is "7d cost by change, with its budget" "$W7.cost.by_change" '[{"name":"group-card","usd":6,"budget_usd":50}]'
is "an unpriced model is named, never costed as zero silently" "$W7.cost.per_week.note" '"excludes unpriced models: claude-mystery-9"'
is "90d series has 15 buckets" '.windows."90d".flow.merge_frequency.series | length' '15'

MA --line; cp "$d/m.json" "$d/line"
grep -q '^OK flow 30d to 2026-09-30: cycle 24h median, lead 48h, 1.2 merges/wk, change-fail 66.7%, approval wait 15h, review rounds 2.3, spend \$1.73/wk, 3 in flight' "$d/line" \
  && ok "--line: one health line, verdict first, naming its window and day" || fail "--line: $(cat "$d/line")"

MA --window 7d
is "--window 7d reports that window alone" '.windows | keys' '["7d"]'

# ── Failing soft ──────────────────────────────────────────────────────────────────────────────
soft() { # LABEL WANT-NOTE-SUBSTRING (after M …)
  v=$(validate "$d/m.json")
  [ "$v" = valid ] && ok "$1: still a valid payload" || fail "$1: the payload breaks the contract: $v"
  n=$(q "$W.flow.cycle_time.note // empty")
  case "$n" in *"$2"*) ok "$1: cycle time is null and says why ($n)" ;; *) fail "$1: cycle time note is '$n', want '$2'" ;; esac
  is "$1: approval wait still drawn from the change records" "$W.flow.approval_wait.value" '15'
}
M FAKE_GH_FAIL=1; soft "gh failing" "gh could not list"
M CONTEXT_OFFLINE=1; soft "offline" "offline"
# A PATH of exactly the tools the scripts use: without gh, then without jq too.
mkdir -p "$d/nogh" "$d/nojq"
for t in git jq sh awk sed cat cut head tail tr rm mktemp basename dirname find grep wc sort touch xargs date env mkdir ls; do
  p=$(command -v "$t") || continue
  ln -s "$p" "$d/nogh/$t"; [ "$t" = jq ] || ln -s "$p" "$d/nojq/$t"
done
(cd "$R" && env PATH="$d/nogh" CONTEXT_OFFLINE= CLAUDE_PROJECTS_DIR="$P" METRICS_NOW=$NOW sh .claude/metrics.sh) > "$d/m.json"
soft "no gh" "gh is not installed"
M CLAUDE_PROJECTS_DIR="$d/none"
v=$(validate "$d/m.json"); [ "$v" = valid ] && ok "no transcripts: still a valid payload" || fail "no transcripts: $v"
is "no transcripts: spend is null and says why, nothing else of cost" "$W.cost | [(.per_week.value), (.per_week.note | startswith(\"no cost: no transcripts for this repository\")), (keys)]" '[null,true,["per_week"]]'
is "no transcripts: the flow still draws" "$W.flow.cycle_time.value" '24'
(cd "$R" && PATH="$d/nojq" METRICS_NOW=$NOW sh .claude/metrics.sh) > "$d/m.json"
v=$(validate "$d/m.json"); [ "$v" = valid ] && ok "no jq: still a valid payload" || fail "no jq: $v ($(cat "$d/m.json"))"
is "no jq: says so" "$W.flow.cycle_time.note" '"metrics.sh needs jq, which is not installed"'
(cd "$R" && PATH="$d/nojq" METRICS_NOW=$NOW sh .claude/metrics.sh --line) > "$d/line"
grep -qx 'CANNOT CHECK — jq is not installed' "$d/line" && ok "no jq: --line says CANNOT CHECK" || fail "no jq --line: $(cat "$d/line")"
# A source that breaks: a malformed search result must not crash the payload.
cp "$d/search.json" "$d/search.good"; echo '{"data":' > "$d/search.json"
M; soft "gh printing broken JSON" "gh could not list"
cp "$d/search.good" "$d/search.json"
(cd "$R" && env PATH="$B:$PATH" CLAUDE_PROJECTS_DIR="$P" METRICS_NOW=$NOW sh .claude/metrics.sh 2>&1) > "$d/m.json"
v=$(validate "$d/m.json"); [ "$v" = valid ] && ok "stderr folded into stdout (the panel preset's sh -c '… 2>&1') stays valid JSON" || fail "with 2>&1: $v"

# ── usage-report.sh ───────────────────────────────────────────────────────────────────────────
U() { (cd "$R" && env CLAUDE_PROJECTS_DIR="$P" USAGE_NOW=$NOW sh .claude/usage-report.sh "$@") > "$d/u" 2>&1; }
U --since 2026-09-01 --json; expect_rc 0 $? "usage-report --since --json"
jq -e '.total_usd == 7.4 and .messages == 6 and .sessions == 2 and .unpriced == ["claude-mystery-9"]' "$d/u" >/dev/null \
  && ok "usage-report: the same total as metrics.sh, each message once, the unpriced model named" || fail "usage-report --json: $(cat "$d/u")"
jq -e '.by_role[0] == {"name":"session","usd":5,"calls":3} and (.by_role | map(.name) | sort) == ["?","builder","session"]' "$d/u" >/dev/null \
  && ok "usage-report: by role, agents charged to their root" || fail "usage-report by_role: $(jq -c .by_role "$d/u")"
U --session sid1
head -1 "$d/u" | grep -q '^Usage — session sid1, whole session: ' && ok "usage-report: names its subject first" || fail "usage-report subject: $(head -1 "$d/u")"
grep -q '^| builder | claude-opus-5-5 | 2 | 1M | 0k | 0k | 2.2 |$' "$d/u" && ok "usage-report: a row per role and model, cache read split out" || fail "usage-report rows: $(cat "$d/u")"
grep -q '^UNPRICED (excluded from the total.*claude-mystery-9' "$d/u" && ok "usage-report: lists UNPRICED models" || fail "usage-report: no UNPRICED line"
(cd "$R" && env -u CLAUDE_CODE_SESSION_ID CLAUDE_PROJECTS_DIR="$P" sh .claude/usage-report.sh) > "$d/u" 2>&1; rc=$?
expect_rc 2 "$rc" "usage-report with no session to report is CANNOT CHECK (exit 2), not \$0"
grep -q '^CANNOT CHECK — no session to report' "$d/u" && ok "...and says why" || fail "usage-report no session: $(cat "$d/u")"
U --session nope; expect_rc 2 $? "usage-report on an unknown session is CANNOT CHECK"

# ── The health line ──────────────────────────────────────────────────────────────────────────
out=$(CONTEXT_OFFLINE=1 sh "$ROOT/.claude/health/flow.sh")
[ "$out" = "CANNOT CHECK — offline" ] && ok "health/flow.sh: offline says CANNOT CHECK — offline" || fail "health/flow.sh offline: $out"

finish
