#!/bin/sh
# panel-suggest.sh offers, per lane template, exactly what the roadmap's current phase allows:
# build = queued change rows whose proposal exists; propose = the first queued change row, and
# nothing while one is already proposed ahead; ops = queued ops rows by name. Its output is the
# panel's JSON. Also: the project's preset panel config, when present, names only scripts that exist.
. "$(dirname "$0")/lib.sh"
need jq

d=$(scratch)
F="$d/proj"; mkdir -p "$F/.claude/lib" "$F/docs" "$F/changes"
cp "$ROOT/.claude/lib/conf.sh" "$ROOT/.claude/roadmap-queue.sh" "$ROOT/.claude/panel-suggest.sh" "$F/.claude/" 2>/dev/null
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
