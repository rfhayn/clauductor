#!/bin/sh
# The two GitHub Actions health lines (.claude/health/push-main.sh, scheduled-workflows.sh, over
# .claude/lib/health.sh), driven through a stub `gh` in a throwaway repo, never asserted against
# their own source: a check that re-reads the strings it checks proves only that the file has not
# changed. Ported from the meta-tests of the project these lines were upstreamed from (P2.2); each
# case says which property it holds. Both directions where a property can pass for the wrong
# reason: a sentence that must be ABSENT is asserted next to a case where it must be present.
. "$(dirname "$0")/lib.sh"
need git jq awk

d=$(scratch)
R="$d/p"; new_repo "$R"
mkdir -p "$R/.claude/lib" "$R/.claude/health" "$R/.github/workflows" "$d/bin"
cp "$ROOT/.claude/lib/conf.sh" "$ROOT/.claude/lib/health.sh" "$ROOT/.claude/lib/modules.sh" "$R/.claude/lib/"
cp "$ROOT/.claude/health/push-main.sh" "$ROOT/.claude/health/scheduled-workflows.sh" "$R/.claude/health/"
cp "$ROOT/.claude/extensions.sh" "$R/.claude/"
printf 'name: Audit\non:\n  schedule:\n    - cron: "0 14 * * 1"\n  workflow_dispatch:\njobs:\n  a:\n    runs-on: x\n' > "$R/.github/workflows/audit.yml"
printf 'name: CI\non:\n  push:\n    branches: [main]\n  pull_request:\njobs:\n  a:\n    runs-on: x\n' > "$R/.github/workflows/ci.yml"
git -C "$R" add -A && git -C "$R" commit -qm base
git init -q --bare "$d/origin.git"
git -C "$R" remote add origin "$d/origin.git"
git -C "$R" push -q origin HEAD:main 2>/dev/null && git -C "$R" fetch -q origin
TODAY=2026-10-01
# ago N: TODAY minus N days (GNU date, else BSD). HEALTH_TODAY pins the scripts' today to it.
ago() { date -u -d "$TODAY - $1 days" +%Y-%m-%d 2>/dev/null || date -u -j -v-"$1"d -f %Y-%m-%d "$TODAY" +%Y-%m-%d; }

# The stub gh branches on its ARGUMENTS (a stub that answers two different questions identically
# cannot tell whether the script asked the right one). Every answer comes from the environment.
cat > "$d/bin/gh" <<'EOF'
#!/bin/sh
[ -n "${GH_LOG:-}" ] && echo "$*" >> "$GH_LOG"
[ -n "${GH_FAIL:-}" ] && { echo "gh: simulated failure" >&2; exit 1; }
case "$* " in
  *"repo view"*) echo main ;;
  *"--status success"*) [ -n "${GH_SUCCESS_FAIL:-}" ] && exit 4; echo "${GH_SUCCESS:-[]}" ;;
  *"--event schedule"*) echo "$GH_SCHED" ;;
  *"--event push"*) echo "$GH_PUSH" ;;
  *"run view"*) echo "[${GH_STEPS:-0}]" ;;
  *) exit 1 ;;
esac
EOF
chmod +x "$d/bin/gh"
sched() {  # sched CONCLUSION STATUS DATE: one scheduled run, as gh --json prints it
  printf '[{"conclusion":"%s","status":"%s","createdAt":"%sT19:49:52Z","url":"http://example/1","databaseId":1}]' "$1" "$2" "$3"
}
succ() {  # succ DATE EVENT [SHA]: the last successful run
  printf '[{"createdAt":"%sT12:00:00Z","event":"%s","url":"http://example/ok","headSha":"%s"}]' "$1" "$2" "${3:-abcdef1234567890}"
}
SW() { (cd "$R" && env PATH="$d/bin:$PATH" HEALTH_TODAY="$TODAY" CONTEXT_OFFLINE= "$@" sh .claude/health/scheduled-workflows.sh 2>&1); }
PM() { (cd "$R" && env PATH="$d/bin:$PATH" HEALTH_TODAY="$TODAY" CONTEXT_OFFLINE= "$@" sh .claude/health/push-main.sh 2>&1); }
has() { case "$2" in *"$3"*) ok "$1" ;; *) fail "$1: no \"$3\" in: $2" ;; esac; }
hasnt() { case "$2" in *"$3"*) fail "$1: \"$3\" in: $2" ;; *) ok "$1" ;; esac; }
VERDICT_WORDS='ARE CURRENT|known-vulnerable|too old to call current|nothing has examined|were never examined'

# ── scheduled-workflows: which workflows (the authority, every spelling) ───────────────────────
# list_in NAME BODY ...: --list in a fixture root F holding exactly these workflows.
F="$d/fx"
list_in() {
  rm -rf "$F"; mkdir -p "$F/.claude/lib" "$F/.claude/health" "$F/.github/workflows"
  cp "$R/.claude/lib/conf.sh" "$R/.claude/lib/health.sh" "$F/.claude/lib/"
  cp "$R/.claude/health/scheduled-workflows.sh" "$F/.claude/health/"
  while [ $# -gt 0 ]; do printf '%b' "$2" > "$F/.github/workflows/$1"; shift 2; done
  (cd "$d" && sh "$F/.claude/health/scheduled-workflows.sh" --list) | sort | tr '\n' ' '
}
got=$(list_in quoted.yml 'name: Q\n"on":\n  schedule:\n    - cron: "0 3 * * *"\njobs:\n  a:\n    runs-on: x\n' \
  single.yml "name: S\n'on':\n  schedule:\n    - cron: '0 3 * * *'\njobs:\n  a:\n    runs-on: x\n" \
  inline.yml 'name: I\non: {schedule: [{cron: "0 3 * * *"}]}\njobs:\n  a:\n    runs-on: x\n' \
  block.yml 'name: B\non:\n  schedule:\n    - cron: "0 14 * * 1"\njobs:\n  a:\n    runs-on: x\n')
[ "$got" = "block.yml inline.yml quoted.yml single.yml " ] && ok "scheduled: finds a schedule: trigger in every spelling (\"on\", 'on', inline flow, block)" || fail "scheduled: spellings found: $got"
got=$(list_in decoy.yml 'name: D\n# schedule: this is a comment\non:\n  push:\n    branches: [main]\njobs:\n  schedule:\n    runs-on: x\n    steps:\n      - name: schedule\n        run: echo schedule:\n' \
  trailing-comment.yml 'name: T\non: workflow_dispatch  # no schedule here, runs on demand\njobs:\n  a:\n    runs-on: x\n' \
  bracket-comment.yml 'name: C\non: workflow_dispatch  # the [schedule] moved to audit\njobs:\n  a:\n    runs-on: x\n' \
  real.yml 'name: R\non:\n  schedule:\n    - cron: "0 14 * * 1"\njobs:\n  a:\n    runs-on: x\n')
[ "$got" = "real.yml " ] && ok "scheduled: does not match schedule outside the on: block (a comment, a job, a step, a trailing comment on the on: line)" || fail "scheduled: decoys matched: $got"
got=$(list_in none.yml 'name: N\non:\n  push:\njobs:\n  a:\n    runs-on: x\n')
[ -z "$got" ] && ok "scheduled: --list prints nothing when no workflow is scheduled" || fail "scheduled: --list with none: $got"
has "scheduled: no scheduled workflow still prints a line (before any gh or offline question)" "$(cd "$F" && CONTEXT_OFFLINE=1 sh .claude/health/scheduled-workflows.sh)" "OK no workflow in .github/workflows/ carries a schedule: trigger — nothing to watch"

# The coverage, against an INDEPENDENT reading (a `cron:` key outside a comment), not a second copy
# of the script's own parse: two copies of one heuristic only ever agree.
cover() {  # cover ROOTDIR: the script's --list against a cron grep of the same directory
  want=$(for f in "$1"/.github/workflows/*.yml "$1"/.github/workflows/*.yaml; do
    [ -f "$f" ] && sed 's/#.*$//' "$f" | grep -qE '(^|[^A-Za-z_])cron[[:space:]]*:' && basename "$f"; done | sort | tr '\n' ' ')
  have=$(ROOT="$1" sh -c '. "$ROOT/.claude/lib/conf.sh"; . "$0"; health_scheduled --list' "$ROOT/.claude/lib/health.sh" | sort | tr '\n' ' ')
  [ "$want" = "$have" ] && ok "scheduled: covers exactly the workflows with a cron in ${2:-$1} (${have:-none})" || fail "scheduled: --list ($have) differs from the workflows with a cron ($want) in ${2:-$1}"
}
cover "$R" "the fixture"
if [ -d "$ROOT/.github/workflows" ]; then cover "$ROOT" "this project"; else ok "scheduled: this project has no .github/workflows (the fixture above covers the enumeration)"; fi
# The root comes from the script's own path, never from git or the cwd (a gate's clean room has no
# .git): --list from a directory outside the repo still reads the repo's workflows.
[ "$(cd "$d" && sh "$R/.claude/health/scheduled-workflows.sh" --list)" = audit.yml ] && ok "scheduled: resolves its root from its own path, not from git or the cwd" || fail "scheduled: --list from outside the repo: $(cd "$d" && sh "$R/.claude/health/scheduled-workflows.sh" --list 2>&1)"
# No workflow file named in the executable half: a literal name is a hand list, and the coverage
# case above would then agree with it forever.
code=$(sed 's/#.*$//' "$ROOT/.claude/lib/health.sh" "$ROOT/.claude/health/scheduled-workflows.sh" "$ROOT/.claude/health/push-main.sh")
named=""
for f in "$ROOT"/.github/workflows/*.yml "$ROOT"/.github/workflows/*.yaml audit.yml ci.yml; do
  n=$(basename "$f"); case $n in '*.yml' | '*.yaml') continue ;; esac
  printf '%s' "$code" | grep -qF "$n" && named="$named $n"
done
[ -z "$named" ] && ok "health lines name no workflow file in their executable half (they enumerate)" || fail "health lines name$named in code: enumerate .github/workflows/ instead"

# ── scheduled-workflows: never silence ─────────────────────────────────────────────────────────
out=$(SW GH_FAIL=1)
has "scheduled: a failing gh prints CANNOT CHECK, by name" "$out" "CANNOT CHECK — audit.yml: gh failed (gh: simulated failure). UNKNOWN, not healthy."
out=$(SW CONTEXT_OFFLINE=1 GH_LOG="$d/log0"); has "scheduled: offline is CANNOT CHECK" "$out" "CANNOT CHECK — offline"
[ ! -s "$d/log0" ] && ok "scheduled: ...and makes no gh call" || fail "scheduled: called gh offline: $(cat "$d/log0")"
out=$(SW GH_SCHED='{"message":"not a list"}'); has "scheduled: output gh did not shape as a list is CANNOT CHECK" "$out" "CANNOT CHECK — audit.yml: could not parse gh output"
out=$(SW GH_SCHED='[]'); has "scheduled: no run on record is NEVER RAN" "$out" "NEVER RAN audit.yml: no scheduled run on record yet"
# jq absent (not broken: absent): a MACHINE problem, said as one, never "could not parse gh output".
mkdir -p "$d/nojq"
for t in sh git awk sed grep tr sort cut head tail wc dirname basename pwd printf env date mktemp rm cat; do
  p=$(command -v "$t" 2>/dev/null) && ln -sf "$p" "$d/nojq/$t"
done
ln -sf "$d/bin/gh" "$d/nojq/gh"
out=$(cd "$R" && env PATH="$d/nojq" CONTEXT_OFFLINE= "$d/nojq/sh" .claude/health/scheduled-workflows.sh 2>&1)
has "scheduled: jq absent names jq as a machine problem" "$out" "jq is not installed"

# ── scheduled-workflows: which kind of failure ─────────────────────────────────────────────────
F0=$(sched failure completed "$(ago 1)")
out=$(SW GH_SCHED="$F0" GH_STEPS=0)
has "scheduled: a failed run reaches the failure branch" "$out" "NOBODY IS NOTIFIED OF THIS"
has "scheduled: zero steps executed reads NEVER STARTED" "$out" "NEVER STARTED"
has "scheduled: ...and says it is not a finding of the workflow" "$out" "NOT a finding of the workflow"
has "scheduled: a failure's verdict is FAILED" "$out" "FAILED audit.yml: TRIGGER"
line=$(printf '%s\n' "$out" | grep 'audit.yml:')
case "$line" in *"gh workflow run audit.yml") ok "scheduled: the line ENDS with the command a reader copies, no URL glued after it" ;; *) fail "scheduled: line does not end in the command: $line" ;; esac
out=$(SW GH_SCHED="$F0" GH_STEPS=9)
has "scheduled: a run that executed its steps still reaches the failure branch" "$out" "NOBODY IS NOTIFIED OF THIS"
hasnt "scheduled: a run that executed its steps is never called NEVER STARTED" "$out" "NEVER STARTED"
out=$(SW GH_SCHED="$(sched cancelled completed "$(ago 1)")" GH_STEPS=0)
hasnt "scheduled: a CANCELLED zero-step run is not called a billing block" "$out" "NEVER STARTED"
has "scheduled: ...it says it was cancelled before any step ran" "$out" "was CANCELLED before any step ran"
case "$out" in *quota*|*spending*) ok "scheduled: ...and names the quota cause next to the concurrency one, choosing neither" ;; *) fail "scheduled: cancelled line hides the quota cause: $out" ;; esac
has "scheduled: ...and still offers the re-run command" "$out" "gh workflow run audit.yml"
out=$(SW GH_SCHED="$(sched skipped completed "$(ago 1)")" GH_STEPS=0)
has "scheduled: a SKIPPED run names its conclusion" "$out" "skipped"
case "$out" in *"concurrency rule"*|*"NEVER STARTED"*|*quota*) fail "scheduled: a skipped run borrowed another cause: $out" ;; *) ok "scheduled: ...without inventing a cause for it" ;; esac

# ── scheduled-workflows: the last clean run, REPORTED, never judged ───────────────────────────
out=$(SW GH_SCHED="$(sched failure completed "$(ago 4)")" GH_SUCCESS="$(succ "$(ago 2)" workflow_dispatch)")
has "scheduled: states the last clean run on the default branch" "$out" "LAST CLEAN RUN on main: $(ago 2) (2d ago), workflow_dispatch"
for sc in "failure completed 4 2 workflow_dispatch 0" "failure completed 4 200 workflow_dispatch 0" "failure completed 4 15 schedule 0" \
          "failure completed 4 none - 0" "failure completed 4 2 workflow_dispatch 9" "cancelled completed 4 2 workflow_dispatch 0" " queued 4 2 workflow_dispatch 0"; do
  set -- $sc
  [ "$#" = 5 ] && set -- "" "$@"
  s=$( [ "$4" = none ] && echo '[]' || succ "$(ago "$4")" "$5")
  out=$(SW GH_SCHED="$(sched "$1" "$2" "$(ago "$3")")" GH_SUCCESS="$s" GH_STEPS="$6")
  if printf '%s' "$out" | grep -qE "$VERDICT_WORDS"; then fail "scheduled: a verdict came back for [$sc]: $out"; fi
done
ok "scheduled: draws no conclusion in any scenario (no verdict vocabulary over seven)"
out=$(SW GH_SCHED="$F0" GH_STEPS=9 GH_SUCCESS="$(succ "$(ago 15)" schedule)")
hasnt "scheduled: a run that failed on a real finding is not NEVER STARTED" "$out" "NEVER STARTED"
has "scheduled: ...and the facts a reader needs are there" "$out" "LAST CLEAN RUN on main: $(ago 15) (15d ago), schedule"
out=$(SW GH_SCHED="$F0" GH_SUCCESS="$(succ "$(ago 2)" workflow_dispatch deadbee1234567)")
has "scheduled: names the commit the clean run ran on" "$out" "at deadbee"
: > "$d/log1"; SW GH_SCHED="$F0" GH_LOG="$d/log1" >/dev/null
[ "$(grep -c -- '--status success' "$d/log1")" = 1 ] && grep -- '--status success' "$d/log1" | grep -q -- '--branch main' \
  && ok "scheduled: the clean-run query is scoped to the default branch (asserted on the query)" || fail "scheduled: clean-run query: $(cat "$d/log1")"
: > "$d/log2"; out=$(SW GH_SCHED="$(sched success completed "$(ago 1)")" GH_LOG="$d/log2")
has "scheduled: a green run reads OK, with its age and cron" "$out" "OK audit.yml: last scheduled run OK ($(ago 1), 1d ago, cron 0 14 * * 1)"
[ "$(wc -l < "$d/log2" | tr -d ' ')" = 1 ] && ok "scheduled: the healthy path makes exactly ONE gh call (all calls counted)" || fail "scheduled: the healthy path made $(wc -l < "$d/log2" | tr -d ' ') gh calls: $(cat "$d/log2")"
out=$(SW GH_SCHED="$F0" GH_SUCCESS='[]')
has "scheduled: never succeeded on the default branch says so plainly" "$out" "LAST CLEAN RUN on main: none on record."
out=$(SW GH_SCHED="$F0" GH_SUCCESS_FAIL=1)
has "scheduled: a failed clean-run lookup still reports the failure" "$out" "NOBODY IS NOTIFIED OF THIS"
hasnt "scheduled: ...and adds nothing built from evidence nobody has" "$out" "LAST CLEAN RUN"
out=$(SW GH_SCHED="$(sched "" queued "$(ago 4)")" GH_SUCCESS="$(succ "$(ago 2)" workflow_dispatch)")
has "scheduled: a run queued for days is STUCK, with its age" "$out" "STUCK audit.yml: scheduled run STUCK in queued since $(ago 4) (4d)"
has "scheduled: ...and the stuck path carries the last clean run" "$out" "LAST CLEAN RUN on main"
out=$(SW GH_SCHED="$(sched "" in_progress "$TODAY")" GH_SUCCESS="$(succ "$(ago 2)" workflow_dispatch)")
has "scheduled: a fresh run is RUNNING, no verdict yet" "$out" "RUNNING audit.yml: scheduled run in_progress right now ($TODAY) — no verdict yet."
has "scheduled: ...and the fresh path carries the last clean run" "$out" "LAST CLEAN RUN on main"
out=$(SW GH_SCHED="$(sched failure completed "$(ago 4)")" GH_SUCCESS="$(succ "$(ago 2)" workflow_dispatch)" GH_STEPS=0)
line=$(printf '%s\n' "$out" | grep 'audit.yml:')
case "$line" in *"LAST CLEAN RUN"*"gh workflow run audit.yml") ok "scheduled: keeps the runnable command last, with the clean-run fact present" ;; *) fail "scheduled: order: $line" ;; esac
out=$(SW GH_SCHED="$(sched success completed "$(ago 50)")")
has "scheduled: a green run older than 45 days is STALE, and says GitHub disables at 60" "$out" "STALE audit.yml: last scheduled run OK ($(ago 50), 50d ago, cron 0 14 * * 1) — GitHub disables a schedule after 60d"

# ── push-main: the subject, and never silence ──────────────────────────────────────────────────
CUR=$(git -C "$R" rev-parse origin/main)
pushrun() {  # pushrun CONCLUSION STATUS DATE SHA
  printf '[{"conclusion":"%s","status":"%s","createdAt":"%sT10:00:00Z","url":"https://github.com/x/y/actions/runs/1","headSha":"%s"}]' "$1" "$2" "$3" "$4"
}
out=$(PM GH_PUSH="$(pushrun failure completed "$(ago 1)" "$CUR")")
has "push-main: a FAILED run is a failure, by its conclusion" "$out" "FAILED push:main ci.yml: last run FAILURE 1d ago"
has "push-main: ...says nobody was told, with the URL" "$out" "nobody was told. https://github.com/x/y/actions/runs/1"
out=$(PM GH_PUSH="$(pushrun success completed "$(ago 1)" "$CUR")")
has "push-main: a green run of the current origin/main is OK, naming it" "$out" "OK push:main ci.yml: last run PASSED 1d ago, on the current origin/main ($(printf %.9s "$CUR"))"
hasnt "push-main: ...and is not called stale" "$out" "NOT current"
OLD=$CUR
for i in 1 2 3; do echo "$i" > "$R/f$i"; git -C "$R" add -A; git -C "$R" commit -qm "c$i"; done
git -C "$R" push -q origin HEAD:main 2>/dev/null && git -C "$R" fetch -q origin
out=$(PM GH_PUSH="$(pushrun success completed "$(ago 1)" "$OLD")")
has "push-main: a green run that is not the current origin/main reads STALE" "$out" "STALE push:main ci.yml: last run PASSED"
has "push-main: ...NAMES ITS SUBJECT and the commits it does not cover" "$out" "NOT current origin/main ($(git -C "$R" rev-parse --short=9 origin/main)) — 3 commit(s) since are uncovered"
out=$(PM GH_PUSH="$(pushrun "" queued "$(ago 92)" "$OLD")")
has "push-main: a run stuck in queued reports its AGE (STUCK), never 'no verdict yet' alone" "$out" "STUCK push:main ci.yml: last run is queued 92d ago"
out=$(PM GH_FAIL=1)
has "push-main: a failing gh prints CANNOT CHECK, UNKNOWN, not healthy" "$out" "CANNOT CHECK — push:main ci.yml: gh failed (gh: simulated failure). UNKNOWN, not healthy."
out=$(PM GH_PUSH='[]'); has "push-main: no run yet says so rather than guessing" "$out" "NEVER RAN push:main ci.yml: no run on record yet"
out=$(PM CONTEXT_OFFLINE=1); has "push-main: offline is CANNOT CHECK" "$out" "CANNOT CHECK — offline"
# Every workflow with a push trigger, each its own line; a pull_request-only one is not a push net.
printf 'name: L\non: [push, pull_request]\njobs:\n  a:\n    runs-on: x\n' > "$R/.github/workflows/lint.yml"
printf 'name: P\non: push  # the post-merge net\njobs:\n  a:\n    runs-on: x\n' > "$R/.github/workflows/pages.yml"
printf 'name: R\non:\n  pull_request:\n  workflow_call:\n    inputs:\n      push:\n        type: string\njobs:\n  a:\n    runs-on: x\n' > "$R/.github/workflows/review.yml"
printf 'name: B\non:\n  - push\n  - pull_request\njobs:\n  a:\n    runs-on: x\n' > "$R/.github/workflows/blist.yml"
out=$(PM GH_PUSH="$(pushrun success completed "$(ago 1)" "$OLD")")
n=$(printf '%s\n' "$out" | grep -c 'push:main ')
[ "$n" = 4 ] && ok "push-main: one line per push workflow (block, inline list, block list and scalar spellings)" || fail "push-main: $n line(s) for 4 push workflows: $out"
hasnt "push-main: an input named push under workflow_call is not a push trigger" "$out" "review.yml"
rm -f "$R/.github/workflows/blist.yml" "$R/.github/workflows/lint.yml" "$R/.github/workflows/pages.yml" "$R/.github/workflows/review.yml" "$R/.github/workflows/ci.yml"
has "push-main: no push workflow says there is no post-merge net" "$(PM)" "OK no workflow in .github/workflows/ has a push: trigger"

# ── The contract: every line starts with a verdict, and the runner shows stderr ───────────────
printf 'name: CI\non:\n  push:\njobs:\n  a:\n    runs-on: x\n' > "$R/.github/workflows/ci.yml"
all=$( { SW GH_FAIL=1; SW GH_SCHED="$F0"; PM GH_PUSH="$(pushrun success completed "$(ago 1)" "$OLD")"; PM GH_PUSH='[]'; } )
bad=$(printf '%s\n' "$all" | grep -vE '^(OK|FAILED|STALE|STUCK|RUNNING|NEVER RAN|CANNOT CHECK —) ')
[ -z "$bad" ] && ok "every health line starts with its verdict (.claude/health/README.md)" || fail "lines without a verdict: $bad"
# A plugin project: its health lines are its own files, but lib/health.sh is the plugin's. The
# runner exports the plugin root (CLAUDUCTOR_FW), and the line finds the library there.
mkdir -p "$d/fw/lib"; mv "$R/.claude/lib/health.sh" "$d/fw/lib/health.sh"
has "a health line with no lib/health.sh in its checkout, and no plugin, says so by name" "$(cd "$R" && CONTEXT_OFFLINE=1 CLAUDUCTOR_FW= sh .claude/health/push-main.sh)" "lib/health.sh is missing"
has "a health line finds the plugin's lib/health.sh through CLAUDUCTOR_FW" "$(cd "$R" && CONTEXT_OFFLINE=1 CLAUDUCTOR_FW="$d/fw" sh .claude/health/scheduled-workflows.sh)" "CANNOT CHECK — offline"
mv "$d/fw/lib/health.sh" "$R/.claude/lib/health.sh"
# ...and the runner hands CLAUDUCTOR_FW on. The plugin's copy sets it, NOT exported, in a prologue;
# that is simulated here by an unexported assignment just before the export line (so it is the
# export line, and nothing in the caller's environment, that can make it reach the health line).
printf '#!/bin/sh\necho "OK fw=${CLAUDUCTOR_FW:-unset}"\n' > "$R/.claude/health/zz-fw.sh"
awk -v fw="$d/fw" 'index($0, "|| export CLAUDUCTOR_FW") { print "CLAUDUCTOR_FW=" fw } { print }' "$R/.claude/extensions.sh" > "$R/.claude/extensions-fw.sh"
out=$(cd "$R" && CONTEXT_OFFLINE=1 CLAUDUCTOR_FW= sh .claude/extensions-fw.sh health | grep -A1 'Health: zz-fw')
rm -f "$R/.claude/extensions-fw.sh"
case "$out" in *"OK fw=unset"*|"") fail "extensions.sh does not export CLAUDUCTOR_FW to a health line: $out" ;; *"OK fw="*) ok "extensions.sh exports CLAUDUCTOR_FW to the health lines it runs" ;; esac
rm -f "$R/.claude/health/zz-fw.sh"
printf '#!/bin/sh\necho "CANNOT CHECK — said on stderr" >&2\n' > "$R/.claude/health/zz-stderr.sh"
has "the runner (extensions.sh health) merges a health line's stderr, so a CANNOT CHECK there is read" \
  "$(cd "$R" && sh .claude/extensions.sh health 2>/dev/null)" "CANNOT CHECK — said on stderr"
finish
