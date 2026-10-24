#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# marker.sh: session-start's marker opens a row's PR as a draft whose body starts with the marker
# line and the cost line; a second run adds nothing; an open PR without the marker gets it first;
# and every failure is ONE `marker: WARNING` line with exit 0, since session start never blocks.
# The gh here is a stand-in over a JSON file that applies each call's --jq for real, so a jq the
# real gh would refuse (a null `.[0]` when no PR is open) fails here too.
# Also: session-start's SKILL.md runs it (the caller, rule 3).
. "$(dirname "$0")/lib.sh"
need jq git  # jq: the stand-in gh applies --jq with it

S=$(scratch)
mkdir -p "$S/bin"
cat > "$S/bin/gh" <<'EOF'
#!/bin/sh
# A stand-in gh over one JSON array of PRs ($GH_STATE); every call is logged to $GH_STATE.log.
echo "$*" >> "$GH_STATE.log"
[ -z "${GH_FAIL:-}" ] || { echo "gh: $GH_FAIL" >&2; exit 1; }
cmd="$1 $2"; shift 2; n="" jqx="." body="" title="" head=""
while [ $# -gt 0 ]; do
  case $1 in
    --jq) jqx=$2; shift 2 ;; --body) body=$2; shift 2 ;; --title) title=$2; shift 2 ;; --head) head=$2; shift 2 ;;
    --json|--state|--base) shift 2 ;; --draft) shift ;; *) n=$1; shift ;;
  esac
done
case $cmd in
  "pr list") jq -r --arg h "$head" "[.[] | select(.head == \$h and .state == \"open\") | {number, body}] | ($jqx)" "$GH_STATE" ;;
  "pr view") jq -r --argjson n "$n" ".[] | select(.number == \$n) | {body} | ($jqx)" "$GH_STATE" ;;
  "pr edit") jq --argjson n "$n" --arg b "$body" 'map(if .number == $n then .body = $b else . end)' "$GH_STATE" > "$GH_STATE.new" && mv "$GH_STATE.new" "$GH_STATE" ;;
  "pr create") n=$(jq 'length + 1' "$GH_STATE")
    jq --argjson n "$n" --arg h "$head" --arg t "$title" --arg b "$body" \
      '. + [{number: $n, head: $h, state: "open", draft: true, title: $t, body: $b}]' "$GH_STATE" > "$GH_STATE.new" && mv "$GH_STATE.new" "$GH_STATE"
    echo "https://github.example/o/r/pull/$n" ;;
  *) echo "gh stand-in: no '$cmd'" >&2; exit 1 ;;
esac
EOF
chmod +x "$S/bin/gh"

setup() {  # a fresh project with an origin, on row A-1's branch with nothing ahead of main
  F="$S/proj"; rm -rf "$F" "$S/origin.git"; mkdir -p "$F/.claude/lib" "$F/.claude/skills/session-start" "$F/docs"
  cp "$CLAUDUCTOR_FW/lib/conf.sh" "$F/.claude/lib/" 2>/dev/null
  cp "$CLAUDUCTOR_FW/roadmap-queue.sh" "$CLAUDUCTOR_FW/roadmap-plan.sh" "$F/.claude/" 2>/dev/null
  cp "$CLAUDUCTOR_FW/skills/session-start/marker.sh" "$F/.claude/skills/session-start/" 2>/dev/null
  cat > "$F/docs/roadmap.md" <<'EOF'
## Phase 1 — Now
| # | Change | Scope | Deps | Status |
|---|---|---|---|---|
| A-1 | `ops/a-one` — the first row | s | — | ⬜ queued |
EOF
  printf '# Journal\n\n## Session 9 — 2026-01-01 — Ann — before\n\nOld.\n' > "$F/docs/development-journal.md"
  git init -q --bare "$S/origin.git"
  (cd "$F" && git init -q -b main && git config user.name "Ann Example" && git config user.email a@example.com \
    && git add -A && git commit -qm init && git remote add origin "$S/origin.git" && git push -q origin main \
    && git checkout -q -b ops/a-one) >/dev/null 2>&1
  echo '[]' > "$S/prs.json"; : > "$S/prs.json.log"
}
mark() { (cd "$F" && env PATH="$S/bin:$PATH" GH_STATE="$S/prs.json" CLAUDE_CODE_SESSION_ID= "$@" sh .claude/skills/session-start/marker.sh A-1 ${SN:-} 2>&1); }
body() { jq -r '.[0].body' "$S/prs.json"; }

# No PR open: a draft opens, its body led by the marker and the cost line.
setup
out=$(mark); rc=$?
[ $rc -eq 0 ] && printf '%s' "$out" | grep -q '^marker: opened https://github.example/o/r/pull/1 as a draft' \
  && ok "marker: no open PR, a draft opens" || fail "marker: no open PR: exit $rc: $out"
body | sed -n 1p | grep -qE '^session-start: S10 [0-9]{4}-[0-9]{2}-[0-9]{2} [0-9]{2}:[0-9]{2} ET row A-1$' \
  && ok "marker: the body's first line is the marker, S<n> one past the journal's top session" || fail "marker: body line 1 is '$(body | sed -n 1p)'"
body | sed -n 2p | grep -q '^session-start cost: CANNOT CHECK — ' \
  && ok "marker: the second line is the cost, CANNOT CHECK with no session id (never 0)" || fail "marker: body line 2 is '$(body | sed -n 2p)'"
[ "$(jq -r '.[0] | "\(.draft) \(.head) \(.title)"' "$S/prs.json")" = "true ops/a-one A-1: the first row" ] \
  && ok "marker: a draft on the row's branch, titled by the row" || fail "marker: the PR is $(jq -c '.[0] | {draft, head, title}' "$S/prs.json")"
[ "$(git -C "$F" rev-list --count main..HEAD)" = 1 ] && git -C "$S/origin.git" rev-parse -q --verify refs/heads/ops/a-one >/dev/null \
  && ok "marker: one empty commit, pushed, so GitHub has a PR to open" || fail "marker: the branch is $(git -C "$F" rev-list --count main..HEAD) ahead, origin has: $(git -C "$S/origin.git" branch)"

# A second run finds the marker and adds nothing.
before=$(cksum < "$S/prs.json"); : > "$S/prs.json.log"
out=$(mark); rc=$?
[ $rc -eq 0 ] && printf '%s' "$out" | grep -q 'already carries it' && [ "$(cksum < "$S/prs.json")" = "$before" ] \
  && ! grep -qE '^pr (create|edit)' "$S/prs.json.log" \
  && ok "marker: a second run adds nothing" || fail "marker: second run: exit $rc: $out; gh calls: $(tr '\n' ';' < "$S/prs.json.log")"

# An open PR without the marker gets it first, its body kept below.
setup
printf '[{"number":4,"head":"ops/a-one","state":"open","draft":false,"title":"t","body":"Hand-written.\\n\\nMore."}]\n' > "$S/prs.json"
out=$(SN=7 mark); rc=$?
[ $rc -eq 0 ] && body | sed -n 1p | grep -q '^session-start: S7 .* row A-1$' && [ "$(body | tail -3)" = "$(printf 'Hand-written.\n\nMore.')" ] \
  && [ "$(jq length "$S/prs.json")" = 1 ] \
  && ok "marker: an open PR without one gets the marker first (S7 as given), its body kept" || fail "marker: open PR: exit $rc: $out; body: $(body)"

# Every failure: one WARNING line, exit 0.
warns() {  # warns LABEL [env...]
  l=$1; shift; out=$(mark "$@"); rc=$?
  [ $rc -eq 0 ] && [ "$(printf '%s\n' "$out" | grep -c .)" = 1 ] && printf '%s' "$out" | grep -q '^marker: WARNING' \
    && ok "marker: $l, one WARNING line, exit 0" || fail "marker: $l: exit $rc: $out"
}
setup; warns "gh failing" GH_FAIL="HTTP 502"
setup; git -C "$F" checkout -q -b ops/other; warns "the wrong branch"

# With no row id (a lane template's first prompt knows only its lane's name, OPS-125): the row is
# the one built on this branch, and S<n> the Session plan's session that names it.
setup
cat >> "$F/docs/roadmap.md" <<'EOF'

## Session plan
| Session | When | Focus | Rows | The owner decides |
|---|---|---|---|---|
| S12 | soon | the first row | A-1 | — |
EOF
out=$(cd "$F" && env PATH="$S/bin:$PATH" GH_STATE="$S/prs.json" CLAUDE_CODE_SESSION_ID= sh .claude/skills/session-start/marker.sh 2>&1); rc=$?
[ $rc -eq 0 ] && body | sed -n 1p | grep -q '^session-start: S12 .* row A-1$' \
  && ok "marker: with no arguments, the branch's row and the plan's session (S12)" || fail "marker: no arguments: exit $rc: $out; body: $(body | sed -n 1p)"
setup; git -C "$F" checkout -q -b ops/no-row; : > "$S/prs.json.log"
out=$(cd "$F" && env PATH="$S/bin:$PATH" GH_STATE="$S/prs.json" CLAUDE_CODE_SESSION_ID= sh .claude/skills/session-start/marker.sh 2>&1); rc=$?
[ $rc -eq 0 ] && [ "$(printf '%s\n' "$out" | grep -c .)" = 1 ] && printf '%s' "$out" | grep -q "no roadmap row is built on 'ops/no-row'" \
  && ! [ -s "$S/prs.json.log" ] && [ "$(git -C "$F" rev-list --count main..HEAD)" = 0 ] \
  && ok "marker: on a branch no row names, one line, and no PR, push or commit" || fail "marker: no row: exit $rc: $out; gh calls: $(tr '\n' ';' < "$S/prs.json.log")"

# The caller: a marker no session start runs is not shipped (rule 3).
grep -q 'session-start/marker\.sh' "$CLAUDUCTOR_FW/skills/session-start/SKILL.md" 2>/dev/null \
  && ok "session-start's SKILL.md runs the marker" || fail "session-start's SKILL.md does not run .claude/skills/session-start/marker.sh"
finish
