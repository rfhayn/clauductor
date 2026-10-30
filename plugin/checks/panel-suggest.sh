#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# panel-suggest.sh offers, per lane template, exactly what the roadmap allows next: with no Session
# plan, the current phase (build = queued change rows whose proposal exists; propose = the first
# queued change row, and nothing while one is already proposed ahead; ops = queued ops rows by
# name); with one, the plan's queued rows in its order, propose passing a proposal approved on main;
# either way, propose stopping while a proposal awaits the owner on a change branch, local or
# origin, read as propose's context reads it; and as origin/main has the roadmap and the proposals,
# not the working tree. Its output is the panel's JSON. Also: the project's preset panel config,
# when present, names only scripts that exist.
. "$(dirname "$0")/lib.sh"
need jq

d=$(scratch)
F="$d/proj"; mkdir -p "$F/.claude/lib" "$F/.claude/skills/propose" "$F/docs" "$F/changes"
cp "$CLAUDUCTOR_FW/lib/conf.sh" "$CLAUDUCTOR_FW/lib/change.sh" "$CLAUDUCTOR_FW/lib/fragments.sh" "$F/.claude/lib/" 2>/dev/null
cp "$CLAUDUCTOR_FW/roadmap-queue.sh" "$CLAUDUCTOR_FW/roadmap-plan.sh" "$CLAUDUCTOR_FW/panel-suggest.sh" "$F/.claude/" 2>/dev/null
cp "$CLAUDUCTOR_FW/skills/propose/context.sh" "$F/.claude/skills/propose/" 2>/dev/null
cat > "$F/docs/roadmap.md" <<'EOF'
## Phase 1 — Done
| # | Change | Scope | Deps | Status |
|---|---|---|---|---|
| 1.1 | `add-old` — old | s | — | ✅ merged (#1) |
## Phase 2 — Now
| # | Change | Scope | Deps | Status |
|---|---|---|---|---|
| 2.1 | `add-built` — being built | s | — | ⬜ in flight (#9) |
| 2.2 | `add-next` — the next one | s | — | ⬜ queued |
| 2.3 | `add-later` — later | s | — | ⬜ queued |
| 2.4 | `ops/tidy-up` — tidy | s | — | ⬜ queued |
| 2.5 | `Add_Bad` — not a lane name | s | — | ⬜ queued |
EOF
sug() { (cd "$F" && sh .claude/panel-suggest.sh "$1"); }
names() { sug "$1" | jq -r '[.[].name] | join(" ")'; }

[ "$(names propose)" = "add-next" ] && ok "propose offers only the first queued change row" || fail "propose offered: $(names propose)"
[ "$(names build)" = "" ] && ok "build offers nothing while no queued row has a proposal" || fail "build offered: $(names build)"
[ "$(names ops)" = "tidy-up" ] && ok "ops offers queued ops rows by name" || fail "ops offered: $(names ops)"
mkdir -p "$F/changes/add-next"
[ "$(names build)" = "add-next" ] && ok "build offers a queued row once its proposal exists" || fail "build offered: $(names build)"
[ "$(names propose)" = "" ] && ok "propose offers nothing while one change is proposed ahead" || fail "propose offered with one ahead: $(names propose)"
sug propose | jq -e 'type == "array"' >/dev/null && ok "output is a JSON array" || fail "output is not a JSON array"
case "$(sug build)" in *'"title":"the next one"'*) ok "a row's title is its summary" ;; *) fail "title: $(sug build)" ;; esac
(cd "$F" && sh .claude/panel-suggest.sh nonsense >/dev/null 2>&1) && fail "an unknown kind exited 0" || ok "an unknown kind exits non-zero"
printf 'not a roadmap | x |\n| # | Change | Scope | Deps | State |\n' > "$F/docs/roadmap.md"
(cd "$F" && sh .claude/panel-suggest.sh build >/dev/null 2>&1) && fail "a malformed roadmap gave a list" || ok "a malformed roadmap exits non-zero (the panel keeps the last list)"

# With a Session plan: its queued rows in the plan's order, across phases, not the table's.
rm -rf "$F/changes/add-next"
cat > "$F/docs/roadmap.md" <<'EOF'
## Phase 1 — Now
| # | Change | Scope | Deps | Status |
|---|---|---|---|---|
| 1.1 | `add-a` — a | s | — | ⬜ queued |
| 1.2 | `ops/first` — first | s | — | ⬜ queued |
| 1.3 | `add-b` — b | s | — | ⬜ queued |
| 1.4 | `ops/second` — second | s | — | ⬜ queued |
| 1.5 | `ops/done` — done | s | — | ✅ merged (#3) |
## Phase 2 — Later
| # | Change | Scope | Deps | Status |
|---|---|---|---|---|
| 2.1 | `add-c` — c | s | — | ⬜ queued |
| 2.2 | `ops/third` — third | s | — | ⬜ queued |
| 2.3 | `add-gone` — gone | s | — | ❌ cancelled — dropped |
## Session plan
| Session | When | Focus | Rows | The owner decides |
|---|---|---|---|---|
| S1 | (done) | done | 1.5 | — |
| S2 | | later rows first | 2.2, 2.1, 1.4 | — |
| S3 | | then these | 1.3, 1.1, 1.2 | — |
EOF
approve() { mkdir -p "$F/changes/$1" && printf '**Approved:** 2026-10-22 by Owner · design 0123456789ab\n' > "$F/changes/$1/proposal.md"; }
[ "$(names ops)" = "third second first" ] && ok "with a Session plan, ops offers its queued rows in the plan's order, merged ones left out" || fail "ops with a plan offered: $(names ops)"
[ "$(names propose)" = "add-c" ] && ok "with a Session plan, propose offers the first planned change row with no proposal" || fail "propose with a plan offered: $(names propose)"
approve add-c
[ "$(names propose)" = "add-b" ] && ok "...passing an earlier planned row whose proposal is approved (on main)" || fail "propose past an approved proposal offered: $(names propose)"
[ "$(names build)" = "add-c" ] && ok "...which build offers" || fail "build with a plan offered: $(names build)"
rm -rf "$F/changes/add-c"
cp "$F/docs/roadmap.md" "$d/plan.md"
sed 's/^| S3 | | then these |/| S2 | | then these |/' "$d/plan.md" > "$F/docs/roadmap.md"
(cd "$F" && sh .claude/panel-suggest.sh ops >/dev/null 2>&1) && fail "a malformed Session plan gave a list" || ok "a malformed Session plan exits non-zero, never a shorter list"
cp "$d/plan.md" "$F/docs/roadmap.md"

# As origin/main has them: the main checkout's working tree lags what the lanes merged.
new_repo "$F"
inF() { (cd "$F" && "$@") >/dev/null 2>&1; }
inF git add -A && inF git commit -qm base || fail "the fixture could not commit"
sed 's/`ops\/third` — third | s | — | ⬜ queued/`ops\/third` — third | s | — | ✅ merged (#4)/' "$d/plan.md" > "$F/docs/roadmap.md"
approve add-c
inF git add -A && inF git commit -qm merged && inF git update-ref refs/remotes/origin/main HEAD && inF git reset -q --hard HEAD~1 || fail "the fixture could not set origin/main"
[ -d "$F/changes/add-c" ] && fail "the fixture's working tree still has the merged proposal"
[ "$(names ops)" = "second first" ] && ok "the roadmap is read as origin/main has it, not the working tree" || fail "ops read the working tree: $(names ops)"
[ "$(names build)" = "add-c" ] && ok "...and so are the proposals" || fail "build read the working tree: $(names build)"
[ "$(names propose)" = "add-b" ] && ok "...so propose passes a proposal approved on origin/main" || fail "propose from origin/main offered: $(names propose)"

# A proposal awaiting the owner lives on its change branch, never on main: wherever its row sits in
# the plan, it is the one ahead. on_origin REF MSG CMDS...: commit CMDS on top of origin/main as REF.
on_origin() {
  r=$1; m=$2; shift 2
  inF git checkout -q --detach refs/remotes/origin/main && (cd "$F" && "$@") && inF git add -A && inF git commit -qm "$m" \
    && inF git update-ref "$r" HEAD && inF git checkout -q - || fail "the fixture could not commit $m"
}
on_origin refs/remotes/origin/change/add-a waiting sh -c 'mkdir -p changes/add-a && echo "**Status:** awaiting approval" > changes/add-a/proposal.md'
[ "$(names propose)" = "" ] && ok "a proposal awaiting the owner on a change branch stops propose (it is the one ahead), even for a later row" || fail "propose offered past a waiting proposal: $(names propose)"
on_origin refs/remotes/origin/main archived sh -c 'mkdir -p changes/archive/2026-10-01-add-a && echo done > changes/archive/2026-10-01-add-a/proposal.md'
[ "$(names propose)" = "add-b" ] && ok "an old branch whose change is archived on main does not stop propose" || fail "propose with an archived branch offered: $(names propose)"
on_origin refs/remotes/origin/change/add-gone gone sh -c 'mkdir -p changes/add-gone && echo "**Status:** awaiting approval" > changes/add-gone/proposal.md'
[ "$(names propose)" = "add-b" ] && ok "...nor does one whose row was cancelled" || fail "propose with a cancelled row's branch offered: $(names propose)"
on_origin refs/remotes/origin/change/add-b approved sh -c 'mkdir -p changes/add-b && echo "**Approved:** 2026-10-22 by Owner · design 0123456789ab" > changes/add-b/proposal.md'
[ "$(names propose)" = "add-a" ] && ok "an approved proposal still on its branch is not offered again, and does not stop propose" || fail "propose with an approved branch proposal offered: $(names propose)"
# propose's context reads the branches the same way (lib/change.sh, branch_proposals): it lists the
# approved one as approved, and neither the archived one nor the cancelled row's.
ctx=$(cd "$F" && sh .claude/skills/propose/context.sh 2>/dev/null | sed -n '/^- Proposed on a branch/,/^- Change queue/p')
case "$ctx" in *"add-b on origin/change/add-b (approved;"*) ok "propose's context marks a branch proposal approved, as panel-suggest reads it" ;; *) fail "propose's context: $ctx" ;; esac
case "$ctx" in *add-a*|*add-gone*) fail "propose's context lists a proposal panel-suggest ignores: $ctx" ;; *) ok "...and leaves out the archived change's and the cancelled row's branches" ;; esac
on_origin refs/heads/change/add-b stale sh -c 'mkdir -p changes/add-b && echo "**Status:** awaiting approval" > changes/add-b/proposal.md'
[ "$(names propose)" = "add-a" ] && ok "a stale local copy awaiting the owner does not hide the approval on origin" || fail "propose with a stale local copy offered: $(names propose)"
inF git update-ref -d refs/remotes/origin/change/add-b
[ "$(names propose)" = "" ] && ok "a proposal awaiting the owner on a local branch only stops propose" || fail "propose with a local waiting proposal offered: $(names propose)"
case "$(cd "$F" && sh .claude/skills/propose/context.sh 2>/dev/null)" in *"add-b on change/add-b (awaiting the owner;"*) ok "...and propose's context marks it awaiting the owner" ;; *) fail "propose's context did not mark the local proposal waiting" ;; esac
inF git update-ref refs/remotes/origin/chg-add-b refs/heads/change/add-b && inF git update-ref -d refs/heads/change/add-b
[ "$(names propose)" = "add-b" ] && ok "with its branch gone, the row is offered again" || fail "propose with no branch offered: $(names propose)"
printf 'BRANCH_CHANGE=chg-\n' > "$F/.claude/project.conf"
[ "$(names propose)" = "" ] && ok "a change prefix with no trailing slash still finds its branches" || fail "propose with BRANCH_CHANGE=chg- offered: $(names propose)"
rm -f "$F/.claude/project.conf"; inF git update-ref -d refs/remotes/origin/chg-add-b
on_origin refs/remotes/origin/main "new row" sh -c 'awk "{ print } /add-gone/ { print \"| 2.4 | \`add-new\` — new | s | — | ⬜ queued |\" }" docs/roadmap.md > r.tmp && mv r.tmp docs/roadmap.md'
on_origin refs/remotes/origin/change/add-new "new waiting" sh -c 'mkdir -p changes/add-new && echo "**Status:** awaiting approval" > changes/add-new/proposal.md'
case "$(cd "$F" && sh .claude/skills/propose/context.sh 2>/dev/null)" in *"add-new on origin/change/add-new (awaiting the owner;"*) ok "propose's context reads the open rows as origin/main has them, as panel-suggest does" ;; *) fail "propose's context missed a row only origin/main has" ;; esac
on_origin refs/remotes/origin/main "first merged, b budgeted" sh -c 'sed "s/first | s | — | ⬜ queued/first | s | — | ✅ merged (#5)/; s/— b | s/— b. Budget: \$7, soft | s/" docs/roadmap.md > r.tmp && mv r.tmp docs/roadmap.md'
pctx=$(cd "$F" && sh .claude/skills/propose/context.sh 2>/dev/null)
case "$(printf '%s\n' "$pctx" | sed -n '/^- Change queue/,/^- Living specs/p')" in
  *ops/first*) fail "propose's change queue read the working tree" ;;
  *ops/second*) ok "...and so does its change queue (the current phase's rows)" ;;
  *) fail "propose's change queue is empty" ;;
esac
case "$(printf '%s\n' "$pctx" | sed -n '/^- Budgets on queued rows/,/^- Scenario IDs/p')" in *'add-b: $7, soft'*) ok "...and its budgets, a soft one's kind included (OPS-91)" ;; *) fail "propose's budgets read the working tree, or dropped a row's ', soft'" ;; esac
on_origin refs/remotes/origin/main "a spec, a phase twice" sh -c 'mkdir -p specs/cap && printf "### Requirement: R\n#### Scenario: [CAP-3-S1] s\n" > specs/cap/spec.md && echo "## Phase 1 — Again" >> docs/roadmap.md'
pctx=$(cd "$F" && sh .claude/skills/propose/context.sh 2>/dev/null)
case "$(printf '%s\n' "$pctx" | sed -n '/^- Living specs/,/^- Budgets on queued rows/p')" in *cap*) ok "...and its living specs" ;; *) fail "propose's living specs read the working tree" ;; esac
case "$(printf '%s\n' "$pctx" | sed -n '/^- Scenario IDs/,$p')" in *"CAP: 1 ID(s), highest requirement 3"*) ok "...and the scenario IDs they take" ;; *) fail "propose's scenario IDs read the working tree" ;; esac
case "$(printf '%s\n' "$pctx" | sed -n '/^- Change queue/,/^- Living specs/p')" in
  *propose-*) fail "propose's change queue error names its temporary copy" ;;
  *"origin/main:docs/roadmap.md:"*) ok "...and an error in that roadmap names it on origin/main" ;;
  *) fail "propose's change queue showed no error for a phase heading twice" ;;
esac
mkdir -p "$d/ctxtmp"; (cd "$F" && TMPDIR="$d/ctxtmp" sh .claude/skills/propose/context.sh >/dev/null 2>&1)
[ -z "$(ls -A "$d/ctxtmp")" ] && ok "propose's context removes its copy of origin/main's roadmap and specs" || fail "propose's context left $(ls "$d/ctxtmp")"
inF git update-ref -d refs/remotes/origin/change/add-new
om=$(cd "$F" && git rev-parse refs/remotes/origin/main); inF git update-ref -d refs/remotes/origin/main; : > "$d/victim"
(cd "$F" && tmp="$d/victim" sh .claude/skills/propose/context.sh >/dev/null 2>&1)
[ -f "$d/victim" ] && ok "with no origin/main, propose's context removes no file an inherited \$tmp names" || fail "propose's context removed an inherited \$tmp"
inF git update-ref refs/remotes/origin/main "$om"
on_origin refs/remotes/origin/main "no roadmap" git rm -q docs/roadmap.md
[ "$(names ops)" = "third second first" ] && ok "with no roadmap on origin/main, the working tree's is read" || fail "ops with no roadmap on origin/main offered: $(names ops)"

# A project in a subdirectory of its repository: origin/main and the branches are read at its paths.
M="$d/mono"; new_repo "$M"; mkdir -p "$M/sub/docs" "$M/sub/changes"
cp -R "$F/.claude" "$M/sub/"; cp "$d/plan.md" "$M/sub/docs/roadmap.md"; touch "$M/sub/changes/.keep"
inM() { (cd "$M" && "$@") >/dev/null 2>&1; }
MS="$M/sub"
subnames() { (cd "$MS" && sh .claude/panel-suggest.sh "$1") | jq -r '[.[].name] | join(" ")'; }
inM git add -A && inM git commit -qm base || fail "the subdirectory fixture could not commit"
sed 's/`ops\/third` — third | s | — | ⬜ queued/`ops\/third` — third | s | — | ✅ merged (#4)/' "$d/plan.md" > "$M/sub/docs/roadmap.md"
inM git commit -qam merged && inM git update-ref refs/remotes/origin/main HEAD && inM git reset -q --hard HEAD~1 || fail "the subdirectory fixture could not set origin/main"
[ "$(subnames ops)" = "second first" ] && ok "a project in a subdirectory reads its roadmap on origin/main" || fail "ops in a subdirectory offered: $(subnames ops)"
inM git checkout -q --detach refs/remotes/origin/main && mkdir -p "$M/sub/changes/add-c" && echo "**Approved:** 2026-10-22 by Owner · design 0123456789ab" > "$M/sub/changes/add-c/proposal.md" \
  && inM git add -A && inM git commit -qm approved && inM git update-ref refs/remotes/origin/change/add-c HEAD && inM git checkout -q - || fail "the subdirectory fixture could not branch"
[ "$(subnames propose)" = "add-b" ] && ok "...and its proposals on a change branch, approval included" || fail "propose in a subdirectory offered: $(subnames propose)"

# close (OPS-125): the daily close is offered once every row of the open plan session has landed,
# a batched PR's **Also closes** counting; with no plan, while any fragment waits.
C="$d/close"; mkdir -p "$C/.claude/lib" "$C/docs/journal.d"
cp -R "$F/.claude/." "$C/.claude/"; cp "$CLAUDUCTOR_FW/lib/fragments.sh" "$C/.claude/lib/" 2>/dev/null
cat > "$C/docs/roadmap.md" <<'RM'
## Phase 1 — Now
| # | Change | Scope | Deps | Status |
|---|---|---|---|---|
| 1.1 | `ops/one` — one | s | — | ⬜ queued |
| 1.2 | `ops/two` — two | s | — | ⬜ queued |
| 1.3 | `ops/old` — old | s | — | ✅ merged (#2) |
## Session plan
| Session | When | Focus | Rows | The owner decides |
|---|---|---|---|---|
| S4 | (done) | done | 1.3 | — |
| S5 | | today | 1.1, 1.2 | — |
RM
printf '# Journal\n\n## Session 4 — 2026-01-01 — Ann — x\n' > "$C/docs/development-journal.md"
cnames() { (cd "$C" && sh .claude/panel-suggest.sh close) | jq -r '[.[].name] | join(" ")'; }
# "Nothing" is exit 0 and exactly `[]`: an error's empty output would read as nothing too.
cnone() { out=$(cd "$C" && sh .claude/panel-suggest.sh close 2>&1); [ $? -eq 0 ] && [ "$out" = "[]" ]; }
cnone && ok "close: nothing while no fragment waits" || fail "close with no fragment printed: $(cd "$C" && sh .claude/panel-suggest.sh close 2>&1)"
printf '# 1.1 — one\n' > "$C/docs/journal.d/1.1.md"
cnone && ok "close: nothing while a row of the open session has not landed" || fail "close with 1.2 unlanded printed: $(cd "$C" && sh .claude/panel-suggest.sh close 2>&1)"
# An Also closes the fragment only quotes (in a fence, under a section) closes nothing.
printf '# 1.1 — one\n- **PR**: #9\n\n## What happened\n\n```\n- **Also closes**: 1.2\n```\n' > "$C/docs/journal.d/1.1.md"
cnone && ok "close: an Also closes quoted in the fragment's body closes no row" || fail "close with a quoted Also closes printed: $(cd "$C" && sh .claude/panel-suggest.sh close 2>&1)"
printf '# 1.1 — one\n- **Also closes**: 1.2\n' > "$C/docs/journal.d/1.1.md"
[ "$(cnames)" = "session-5-close" ] && ok "close: offered once every row landed, a batched PR's Also closes counting" || fail "close with all landed offered: $(cnames)"
# A cancelled row closes its session as a merged one does: S4 (not marked done) holds only a merged
# and a cancelled row, so the open session is still S5, and 1.2 has not landed.
cp "$C/docs/roadmap.md" "$C/keep.md"
awk '/^\| 1\.3 /{ print; print "| 1.4 | `ops/gone` — gone | s | — | ❌ cancelled |"; next }
  /^\| S4 /{ print "| S4 | | done | 1.3, 1.4 | — |"; next } { print }' "$C/keep.md" > "$C/docs/roadmap.md"
printf '# 1.1 — one\n' > "$C/docs/journal.d/1.1.md"
cnone && ok "close: a session of merged and cancelled rows is not the open one" || fail "close with S4 merged+cancelled printed: $(cd "$C" && sh .claude/panel-suggest.sh close 2>&1)"
# A plan entry spanning sessions (S5–S7) holds days of rows: each day's close is offered while a
# fragment waits, numbered one past the journal's top, never held until the whole entry lands.
sed 's/^| S5 |/| S5–S7 |/' "$C/keep.md" > "$C/docs/roadmap.md"
[ "$(cnames)" = "session-5-close" ] && ok "close: a spanning plan entry offers the day's close while a fragment waits" || fail "close in a spanning entry offered: $(cnames)"
printf '# Journal\n\n## Session 5 — 2026-01-02 — Ann — x\n\n## Session 4 — 2026-01-01 — Ann — x\n' > "$C/docs/development-journal.md"
[ "$(cnames)" = "session-6-close" ] && ok "...the next day's numbered past the one the journal holds" || fail "close in a spanning entry after S5 offered: $(cnames)"
printf '# Journal\n\n## Session 4 — 2026-01-01 — Ann — x\n' > "$C/docs/development-journal.md"
# A plain session is numbered as fold.sh and session-close number it (OPS-108), by its label: S4.5
# folds Session 4.5 (a lane name takes no dot), never 4 (its order) and never 5.
sed 's/^| S5 |/| S4.5 |/' "$C/keep.md" > "$C/docs/roadmap.md"
printf '# 1.1 — one\n- **Also closes**: 1.2\n' > "$C/docs/journal.d/1.1.md"
[ "$(cnames)" = "session-4-5-close" ] && ok "close: S4.5's close is numbered by its label, as fold.sh's N" || fail "close for S4.5 offered: $(cnames)"
# A label the journal already holds (an entry before it ran long) takes the first number free.
cp "$C/keep.md" "$C/docs/roadmap.md"
printf '# Journal\n\n## Session 5 — 2026-01-02 — Ann — x\n\n## Session 4 — 2026-01-01 — Ann — x\n' > "$C/docs/development-journal.md"
[ "$(cnames)" = "session-6-close" ] && ok "close: a plain label the journal holds takes one past its top" || fail "close for a held S5 offered: $(cnames)"
printf '# Journal\n\n## Session 4 — 2026-01-01 — Ann — x\n' > "$C/docs/development-journal.md"
# Every plan entry done, yet a fragment waits (its row in none, or an entry marked done early): the
# close is still offered, never held until the plan is edited.
sed 's/^| S5 | |/| S5 | (done) |/' "$C/keep.md" > "$C/docs/roadmap.md"
[ "$(cnames)" = "session-5-close" ] && ok "close: with no open plan session, offered while a fragment waits" || fail "close with every entry done offered: $(cnames)"
mv "$C/keep.md" "$C/docs/roadmap.md"
printf '# 1.1 — one\n- **Also closes**: 1.2\n' > "$C/docs/journal.d/1.1.md"
awk '!/^## Session plan/ { print } /^## Session plan/ { exit }' "$C/docs/roadmap.md" > "$C/r" && mv "$C/r" "$C/docs/roadmap.md"
[ "$(cnames)" = "session-5-close" ] && ok "close: with no plan, offered while a fragment waits, one past the journal's top" || fail "close with no plan offered: $(cnames)"

# OPS-126: a landed row (its fragment on main) is never offered again, a batch's later rows get no
# lane of their own, and a planned proposal lands with its proposal on main.
cat > "$C/docs/roadmap.md" <<'RM'
## Phase 1 — Now
| # | Change | Scope | Deps | Status |
|---|---|---|---|---|
| 1.1 | `ops/one` — one | s | — | ⬜ queued |
| 1.2 | `ops/two` — two | s | — | ⬜ queued |
| 1.3 | `ops/three` — three | s | — | ⬜ queued |
| 1.5 | `add-p` — p | s | — | ⬜ queued |
## Session plan
| Session | When | Focus | Rows | The owner decides |
|---|---|---|---|---|
| S5 | | today | 1.1, 1.5 (proposal) | — |
| S6 | | next | 1.2 + 1.3 | — |
RM
cops() { (cd "$C" && sh .claude/panel-suggest.sh "$1") | jq -r '[.[].name] | join(" ")'; }
printf '# 1.1 — one\n' > "$C/docs/journal.d/1.1.md"
[ "$(cops ops)" = "two" ] && ok "ops: a row whose fragment is on main is not offered again, and a batch offers only its first row" || fail "ops with 1.1 landed and 1.2 + 1.3 batched offered: $(cops ops)"
rm -f "$C/docs/journal.d/1.1.md"
[ "$(cops ops)" = "one two" ] && ok "...and a row with no fragment still is" || fail "ops with no fragment offered: $(cops ops)"
printf '# 1.1 — one\n' > "$C/docs/journal.d/1.1.md"
cnone && ok "close: a planned proposal holds the close until its proposal is on main" || fail "close with add-p unproposed printed: $(cd "$C" && sh .claude/panel-suggest.sh close 2>&1)"
mkdir -p "$C/changes/add-p" && echo '**Status:** approved' > "$C/changes/add-p/proposal.md"
[ "$(cnames)" = "session-5-close" ] && ok "...and lets it go once the proposal is on main, with no fragment of its own" || fail "close with add-p proposed offered: $(cnames)"
[ "$(cops build)" = "add-p" ] && ok "build offers the proposed row" || fail "build with add-p proposed offered: $(cops build)"
# A batch's later row waits only while its lead is open: once the lead lands without closing it, it
# is offered, never stranded (S11's OPS-103 shipped apart from its batch).
printf '# 1.2 — two\n\n- **Also closes**: 1.3\n' > "$C/docs/journal.d/1.2.md"
[ -z "$(cops ops)" ] && ok "ops: a lead whose fragment also closes its batch leaves nothing to offer" || fail "ops with 1.2 closing 1.3 offered: $(cops ops)"
printf '# 1.2 — two\n' > "$C/docs/journal.d/1.2.md"
[ "$(cops ops)" = "three" ] && ok "...and one that does not close it offers the later row" || fail "ops with 1.2 landed alone offered: $(cops ops)"
rm -f "$C/docs/journal.d/1.2.md"
sed 's/| 1\.2 | `ops\/two` — two | s | — | ⬜ queued |/| 1.2 | `ops\/two` — two | s | — | ✅ merged (#9) |/' "$C/docs/roadmap.md" > "$C/r" && cp "$C/r" "$C/docs/roadmap.md"
[ "$(cops ops)" = "three" ] && ok "...as does a lead merged on the roadmap" || fail "ops with 1.2 merged offered: $(cops ops)"
sed 's/✅ merged (#9)/⬜ queued/' "$C/docs/roadmap.md" > "$C/r" && mv "$C/r" "$C/docs/roadmap.md"
# A change planned as its proposal (S5) and then its build (S6): S6's close waits for the build's
# own fragment, never counting the proposal that landed in S5.
sed -e 's/| S5 | |/| S5 | (done) |/' -e 's/| 1\.2 + 1\.3 |/| 1.2 + 1.3, 1.5 |/' "$C/docs/roadmap.md" > "$C/r" && mv "$C/r" "$C/docs/roadmap.md"
printf '# 1.2 — two\n\n- **Also closes**: 1.3\n' > "$C/docs/journal.d/1.2.md"
cnone && ok "close: a build planned after its proposal holds its session's close until its fragment" || fail "close with 1.5's build unlanded printed: $(cd "$C" && sh .claude/panel-suggest.sh close 2>&1)"
printf '# 1.5 — p\n' > "$C/docs/journal.d/1.5.md"
[ "$(cnames)" = "session-6-close" ] && ok "...and lets it go once the build's fragment is on main" || fail "close with 1.5 built offered: $(cnames)"
rm -f "$C/docs/journal.d/1.2.md" "$C/docs/journal.d/1.5.md"
sed -e 's/| S5 | (done) |/| S5 | |/' -e 's/| 1\.2 + 1\.3, 1\.5 |/| 1.2 + 1.3 |/' "$C/docs/roadmap.md" > "$C/r" && mv "$C/r" "$C/docs/roadmap.md"
sed 's/1\.2 + 1\.3 |/1.2 + 1.3 (proposal) |/' "$C/docs/roadmap.md" > "$C/r" && mv "$C/r" "$C/docs/roadmap.md"
(cd "$C" && sh .claude/panel-suggest.sh ops >/dev/null 2>&1) && fail "a batch marked (proposal) gave a list" || ok "a batch marked (proposal) is a plan error, never a shorter list"
# Planned twice beyond its proposal and its build, or built before it is proposed: a plan error.
for bad in '| 1.1, 1.5 (proposal) |::| 1.2 + 1.3, 1.5 (proposal) |' '| 1.1, 1.5 |::| 1.2 + 1.3, 1.5 (proposal) |'; do
  sed -e "s/| 1\.1, 1\.5 (proposal) |/${bad%%::*}/" -e "s/| 1\.2 + 1\.3 (proposal) |/${bad##*::}/" "$C/docs/roadmap.md" > "$C/r.bad"
  out=$(sh "$C/.claude/roadmap-plan.sh" "$C/r.bad" 2>&1 >/dev/null) && fail "the plan '$bad' parsed" \
    || case $out in *"planned in S5 and S6"* | *"built in S5 before its proposal in S6"*) ok "a plan error: $out" ;; *) fail "the plan '$bad' failed for another reason: $out" ;; esac
done
# OPS-129: an ops or fix row has no proposal to land, so its `(proposal)` would hold its session's
# close for ever: a plan error, never a silent wait.
sed 's/1\.2 + 1\.3 (proposal) |/1.2 + 1.3 |/' "$C/docs/roadmap.md" > "$C/r" && mv "$C/r" "$C/docs/roadmap.md"
sh "$C/.claude/roadmap-plan.sh" "$C/docs/roadmap.md" >/dev/null 2>&1 && ok "a change row planned as its proposal parses" || fail "a change row's (proposal) was refused"
awk '/^\| 1\.5 /{ print; print "| 1.4 | `fix/4-crash` — crash | s | — | ⬜ queued |"; next } { print }' "$C/docs/roadmap.md" > "$C/r.ok"
for bad in '1.1 (proposal)::1.1 is an ops row' '1.4 (proposal)::1.4 is a fix row'; do
  sed "s/| 1\.1, 1\.5 (proposal) |/| ${bad%%::*}, 1.5 (proposal) |/" "$C/r.ok" > "$C/r.bad"
  out=$(sh "$C/.claude/roadmap-plan.sh" "$C/r.bad" 2>&1 >/dev/null); rc=$?
  [ $rc -eq 1 ] && case $out in *"${bad##*::}"*"no proposal"*) true ;; *) false ;; esac \
    && ok "a plan error: $out" || fail "'${bad%%::*}' on a row with no proposal parsed or failed otherwise (exit $rc): $out"
done
rm -f "$C/r.bad" "$C/r.ok"
# OPS-129: a session that only proposes is offered its close once every proposal it plans is on
# main, with no fragment waiting (a proposal writes none); once the journal holds its session, the
# close has run, and it is not offered again.
rm -f "$C/docs/journal.d/"*.md
cat > "$C/docs/roadmap.md" <<'RM'
## Phase 1 — Now
| # | Change | Scope | Deps | Status |
|---|---|---|---|---|
| 1.5 | `add-p` — p | s | — | ⬜ queued |
| 1.6 | `add-q` — q | s | — | ⬜ queued |
## Session plan
| Session | When | Focus | Rows | The owner decides |
|---|---|---|---|---|
| S5 | | propose | 1.5 (proposal), 1.6 (proposal) | — |
| S6 | | build | 1.5, 1.6 | — |
RM
[ -f "$C/changes/add-p/proposal.md" ] && [ ! -d "$C/changes/add-q" ] || fail "the fixture should hold add-p's proposal and not add-q's"
cnone && ok "close: a proposal-only session waits while one of its proposals is not on main" || fail "close with add-q unproposed printed: $(cd "$C" && sh .claude/panel-suggest.sh close 2>&1)"
mkdir -p "$C/changes/add-q" && echo '**Status:** approved' > "$C/changes/add-q/proposal.md"
[ "$(cnames)" = "session-5-close" ] && ok "close: a proposal-only session is offered its close once every proposal is on main, no fragment waiting" || fail "close with S5's proposals on main offered: $(cnames)"
printf '# Journal\n\n## Session 5 — 2026-01-02 — Ann — x\n\n## Session 4 — 2026-01-01 — Ann — x\n' > "$C/docs/development-journal.md"
cnone && ok "...and not once the journal holds Session 5 (its close ran), while S6's builds wait" || fail "close after Session 5 printed: $(cd "$C" && sh .claude/panel-suggest.sh close 2>&1)"
printf '# Journal\n\n## Session 4 — 2026-01-01 — Ann — x\n' > "$C/docs/development-journal.md"
sed 's/^| S5 | |/| S5 | (done) |/' "$C/docs/roadmap.md" > "$C/r" && mv "$C/r" "$C/docs/roadmap.md"
cnone && ok "...nor once S5 is marked done, with no fragment waiting" || fail "close with S5 done printed: $(cd "$C" && sh .claude/panel-suggest.sh close 2>&1)"
sed 's/^| S6 | |/| S6 | (done) |/' "$C/docs/roadmap.md" > "$C/r" && mv "$C/r" "$C/docs/roadmap.md"
cnone && ok "...nor with every session done, a landed proposal and no fragment: nothing to fold" || fail "close with every session done printed: $(cd "$C" && sh .claude/panel-suggest.sh close 2>&1)"
# The open session spans days (S5–S7) and the landed proposal's own session was closed (the journal
# holds Session 4): with no fragment waiting, a spanning entry is offered nothing.
cp "$C/docs/roadmap.md" "$C/keep.md"
sed -e 's/^| S5 | (done) | propose |/| S4 | | propose |/' -e 's/^| S6 | (done) |/| S5–S7 | |/' "$C/keep.md" > "$C/docs/roadmap.md"
cnone && ok "...nor while a spanning session is open, a landed proposal's session closed and no fragment waiting" || fail "close with S5–S7 open and no fragment printed: $(cd "$C" && sh .claude/panel-suggest.sh close 2>&1)"
mv "$C/keep.md" "$C/docs/roadmap.md"
# The close's number is computed once, here: the dev-journal and session-close contexts print the
# session Up next offers the close as, so a no-fragment close (fold.sh folds nothing, dev-journal
# writes the heading) writes the Session the suggestion waits for, even when its label is not one
# past the journal's top (S5 after Session 3), and is not offered again.
sed -e 's/^| S5 | (done) |/| S5 | |/' -e 's/^| S6 | (done) |/| S6 | |/' "$C/docs/roadmap.md" > "$C/r" && mv "$C/r" "$C/docs/roadmap.md"
printf '# Journal\n\n## Session 3 — 2026-01-01 — Ann — x\n' > "$C/docs/development-journal.md"
mkdir -p "$C/.claude/skills/dev-journal" "$C/.claude/skills/session-close"
cp "$CLAUDUCTOR_FW/skills/dev-journal/context.sh" "$C/.claude/skills/dev-journal/"
[ "$(cnames)" = "session-5-close" ] && ok "close: S5 after Session 3 is offered as Session 5" || fail "close for S5 after Session 3 offered: $(cnames)"
case $(cd "$C" && CONTEXT_OFFLINE=1 sh .claude/skills/dev-journal/context.sh 2>&1) in
  *"This close's session"*": 5"*) ok "...and dev-journal's context names Session 5 for its entry, not the count's 4" ;;
  *) fail "dev-journal's context does not name the close's session 5: $(cd "$C" && CONTEXT_OFFLINE=1 sh .claude/skills/dev-journal/context.sh 2>&1 | grep -i session)" ;;
esac
# Both contexts read the number where they read main: a copy of the fixture with an origin/main,
# so session-close's context reaches its fetched branch, and each must print the number itself.
C2="$d/close2"; cp -R "$C" "$C2"; cp "$CLAUDUCTOR_FW/skills/session-close/context.sh" "$C2/.claude/skills/session-close/"
(cd "$C2" && git init -q && git add -A && git -c user.name=Ann -c user.email=a@example.com commit -qm base && git update-ref refs/remotes/origin/main HEAD) >/dev/null 2>&1 || fail "the second close fixture could not commit"
ctx() { (cd "$C2" && env PATH="$d/nogh:$PATH" CONTEXT_OFFLINE=1 sh ".claude/skills/$1/context.sh" 2>&1) | grep "This close's session"; }
mkdir -p "$d/nogh"; printf '#!/bin/sh\nexit 1\n' > "$d/nogh/gh"; chmod +x "$d/nogh/gh"
for s in dev-journal session-close; do
  case $(ctx $s) in *"This close's session"*": 5"*) ok "...and $s's context, reading origin/main, names Session 5" ;; *) fail "$s's context with origin/main: [$(ctx $s)]" ;; esac
done
# A suggestion that fails is CANNOT CHECK, never read as no close offered: the count would number it.
sed 's/| 1\.5 (proposal), 1\.6 (proposal) |/| 1.5 (proposal), 1.6 (proposal), 1.5 (proposal) |/' "$C2/docs/roadmap.md" > "$C2/r" && mv "$C2/r" "$C2/docs/roadmap.md"
(cd "$C2" && git -c user.name=Ann -c user.email=a@example.com commit -qam bad && git update-ref refs/remotes/origin/main HEAD) >/dev/null 2>&1
for s in dev-journal session-close; do
  case $(ctx $s) in *"This close's session"*"CANNOT CHECK"*) ok "...and a suggestion that fails is CANNOT CHECK in $s's context" ;; *) fail "$s's context with a failing suggestion: [$(ctx $s)]" ;; esac
done
printf '# Journal\n\n## Session 5 — 2026-01-02 — Ann — x\n\n## Session 3 — 2026-01-01 — Ann — x\n' > "$C/docs/development-journal.md"
cnone && ok "...and once that entry is written, the close is not offered again" || fail "close after Session 5 printed: $(cd "$C" && sh .claude/panel-suggest.sh close 2>&1)"
printf '# Journal\n\n## Session 4 — 2026-01-01 — Ann — x\n' > "$C/docs/development-journal.md"

# The preset panel config: every command it names exists in this project.
pj="$ROOT/.clauductor/panel.json"
if [ -f "$pj" ]; then
  jq -e . "$pj" >/dev/null 2>&1 && ok "panel.json parses" || fail "panel.json is not valid JSON"
  for p in $(jq -r '[.cards[]?.command, .templates[]?.suggest.command, .queues[]?.command] | map(select(. != null)) | .[] | .[]' "$pj" \
      | grep -oE '(\.claude|scripts)/[A-Za-z0-9_./-]+\.sh' | sort -u); do
    [ -f "$ROOT/$p" ] && ok "panel.json runs $p, which exists" || fail "panel.json runs $p, which does not exist"
  done
fi
finish
