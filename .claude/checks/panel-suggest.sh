#!/bin/sh
# panel-suggest.sh offers, per lane template, exactly what the roadmap allows next: with no Session
# plan, the current phase (build = queued change rows whose proposal exists; propose = the first
# queued change row, and nothing while one is already proposed ahead; ops = queued ops rows by
# name); with one, the plan's queued rows in its order, propose passing an approved proposal but
# stopping at one that awaits the owner; and either way as origin/main has the roadmap and the
# proposals, not the working tree. Its output is the panel's JSON. Also: the project's preset panel
# config, when present, names only scripts that exist.
. "$(dirname "$0")/lib.sh"
need jq

d=$(scratch)
F="$d/proj"; mkdir -p "$F/.claude/lib" "$F/docs" "$F/changes"
cp "$ROOT/.claude/lib/conf.sh" "$ROOT/.claude/roadmap-queue.sh" "$ROOT/.claude/roadmap-plan.sh" "$ROOT/.claude/panel-suggest.sh" "$F/.claude/" 2>/dev/null
mv "$F/.claude/conf.sh" "$F/.claude/lib/conf.sh"
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
## Session plan
| Session | When | Focus | Rows | The owner decides |
|---|---|---|---|---|
| S1 | (done) | done | 1.5 | — |
| S2 | | later rows first | 2.2, 2.1, 1.4 | — |
| S3 | | then these | 1.3, 1.1, 1.2 | — |
EOF
approve() { mkdir -p "$F/changes/$1" && printf '**Approved:** 2026-10-22 by Owner · design 0123456789ab\n' > "$F/changes/$1/proposal.md"; }
waiting() { mkdir -p "$F/changes/$1" && printf '**Status:** awaiting approval\n' > "$F/changes/$1/proposal.md"; }
[ "$(names ops)" = "third second first" ] && ok "with a Session plan, ops offers its queued rows in the plan's order, merged ones left out" || fail "ops with a plan offered: $(names ops)"
[ "$(names propose)" = "add-c" ] && ok "with a Session plan, propose offers the first planned change row with no proposal" || fail "propose with a plan offered: $(names propose)"
approve add-c
[ "$(names propose)" = "add-b" ] && ok "...passing an earlier planned row whose proposal is approved" || fail "propose past an approved proposal offered: $(names propose)"
[ "$(names build)" = "add-c" ] && ok "...which build offers" || fail "build with a plan offered: $(names build)"
waiting add-a
[ "$(names propose)" = "add-b" ] && ok "...and a later row's waiting proposal does not stop it" || fail "a later waiting proposal changed propose: $(names propose)"
waiting add-c
[ "$(names propose)" = "" ] && ok "...but an earlier planned proposal awaiting the owner does (it is the one ahead)" || fail "propose offered past a waiting proposal: $(names propose)"
rm -rf "$F/changes/add-a" "$F/changes/add-c"

# As origin/main has them: the main checkout's working tree lags what the lanes merged.
G="git -c user.name=t -c user.email=t@t"
(cd "$F" && git init -q && git add -A && $G commit -qm base) >/dev/null 2>&1
sed 's/`ops\/third` — third | s | — | ⬜ queued/`ops\/third` — third | s | — | ✅ merged (#4)/' "$F/docs/roadmap.md" > "$d/merged.md"
cp "$d/merged.md" "$F/docs/roadmap.md"; approve add-c
(cd "$F" && git add -A && $G commit -qm merged && git update-ref refs/remotes/origin/main HEAD && git reset -q --hard HEAD~1) >/dev/null 2>&1
[ -d "$F/changes/add-c" ] && fail "the fixture's working tree still has the merged proposal"
[ "$(names ops)" = "second first" ] && ok "the roadmap is read as origin/main has it, not the working tree" || fail "ops read the working tree: $(names ops)"
[ "$(names build)" = "add-c" ] && ok "...and so are the proposals" || fail "build read the working tree: $(names build)"

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
