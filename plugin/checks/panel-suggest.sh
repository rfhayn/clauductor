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
cp "$CLAUDUCTOR_FW/lib/conf.sh" "$CLAUDUCTOR_FW/lib/change.sh" "$F/.claude/lib/" 2>/dev/null
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
