#!/bin/sh
# The ideas module (.claude/modules/ideas), tested whether a project turns it on or not: its
# renderer, its project check, enable.sh, its context section and its wiring, each against a
# throwaway project, never its own source. Ported from Standing Tee's ideas-render.test.ts, and
# falsified: the comment above a case names the mutation that turned it red.
. "$(dirname "$0")/lib.sh"
need git jq

MOD="$ROOT/.claude/modules/ideas"
d=$(scratch)
R="$d/app"; new_repo "$R"
mkdir -p "$R/.claude/lib" "$R/.claude/modules" "$R/.claude/checks" "$R/docs"
cp "$ROOT/.claude/lib/conf.sh" "$ROOT/.claude/lib/modules.sh" "$R/.claude/lib/"
cp "$ROOT/.claude/checks/run.sh" "$ROOT/.claude/checks/lib.sh" "$R/.claude/checks/"
cp -R "$ROOT/.claude/modules/artifacts" "$MOD" "$R/.claude/modules/"
printf 'PROJECT_NAME="Demo"\nOWNER_ROLE="owner"\nOWNER_NAME=""\nMODULES="artifacts ideas"\n' > "$R/.claude/project.conf"
RS="$R/.claude/modules/ideas/bin/render.sh"
run() { sh "$RS" "$@"; }
has() {  # has WANT(yes|no) TEXT HAYSTACK LABEL
  case "$3" in *"$2"*) got=yes ;; *) got=no ;; esac
  [ "$got" = "$1" ] && ok "$4" || fail "$4 (looked for '$2'): $(printf '%s' "$3" | head -c 600)"
}
dump() {  # dump ID=JSON...: a fresh dump directory as ArtifactData writes it; prints its path
  # mktemp, not a counter: this runs inside $( ), where a counter's increment is lost.
  _dd=$(mktemp -d "$d/dump.XXXXXX"); mkdir -p "$_dd/ideas"
  for _kv in "$@"; do printf '%s' "${_kv#*=}" > "$_dd/ideas/${_kv%%=*}.json"; done
  printf '%s' "$_dd"
}
idea() {  # idea TEXT CREATED [JQ-OBJECT-TO-MERGE]
  jq -cn --arg t "$1" --arg c "$2" --argjson x "${3:-null}" '{text: $t, authorId: "u_rich00000000000000000000", createdAt: $c, updatedAt: $c, status: "not-touched"} + ($x // {})'
}
A1=$(idea 'Skins carryover visible on the card?' 2026-09-27T10:00:00.000Z)
B2=$(idea 'Dark-mode scorecard' 2026-09-24T09:00:00.000Z '{"authorId":"u_damian000000000000000000","status":"discussed","updatedAt":"2026-09-26T09:00:00.000Z","note":"for night rounds\nunder lights"}')
C3=$(idea 'Tee-sheet export' 2026-09-20T09:00:00.000Z '{"status":"roadmap","roadmapRow":"2C.19"}')
D4=$(idea 'Older not-touched idea' 2026-09-21T09:00:00.000Z '{"authorId":null}')
E5=$(idea 'Parked thing' 2026-09-22T09:00:00.000Z '{"status":"parked"}')
Q=$(dump a1="$A1" b2="$B2" c3="$C3" d4="$D4" e5="$E5")
echo '{"u_rich00000000000000000000":"Rich"}' > "$d/names.json"

# ── rendering ──────────────────────────────────────────────────────────────────────────────────
# Falsified by iterating the documents' own order instead of the four statuses: red.
md=$(run "$Q" --names "$d/names.json" 2>&1); rc=$?
expect_rc 0 "$rc" "render: the queue renders"
order=$(printf '%s\n' "$md" | grep '^## ' | tr '\n' '|')
[ "$order" = "## Not touched (2)|## Discussed (1)|## On the roadmap (1)|## Parked (1)|" ] && ok "render: the four statuses in a fixed order, each counted" || fail "render: section order $order"
# Falsified by dropping the reverse (oldest first): red.
first=$(printf '%s\n' "$md" | grep -n 'Skins carryover' | cut -d: -f1); older=$(printf '%s\n' "$md" | grep -n 'Older not-touched' | cut -d: -f1)
[ "${first:-99}" -lt "${older:-0}" ] && ok "render: newest first inside a status" || fail "render: newest-first order ($first vs $older)"
has yes "- **Skins carryover visible on the card?** — Rich, 2026-09-27" "$md" "render: a named author, and the creation day"
has yes "- **Dark-mode scorecard** — someone, 2026-09-24. Note: for night rounds under lights" "$md" "render: an id with no name reads 'someone', and the note is one line"
has yes "- **Older not-touched idea** — someone, 2026-09-21" "$md" "render: a null author reads 'someone'"
has yes "- **Tee-sheet export** → roadmap row 2C.19 — Rich, 2026-09-20" "$md" "render: a roadmap idea names its row"
has no "u_" "$md" "render: an author id is never printed"
has yes "The owner scopes which ideas" "$md" "render: the owner is named by OWNER_ROLE when OWNER_NAME is empty"
has yes "data-through: 2026-09-27T10:00:00.000Z · body-sha256: " "$md" "render: the header stamps the latest updatedAt"

# Determinism. The listing is by NAME, so reversing the write order alone proves nothing: rename the
# files so the listing order itself reverses. Falsified by removing the sort_by: red.
R2=$(dump a-e5="$E5" b-d4="$D4" c-c3="$C3" d-b2="$B2" e-a1="$A1")
Q2=$(dump e5="$E5" d4="$D4" c3="$C3" b2="$B2" a1="$A1")
[ "$(run "$R2" | sed 1d)" = "$(run "$Q2" | sed 1d)" ] && [ "$(run "$Q2")" = "$(run "$Q")" ] \
  && ok "render: the same documents in any file order give the same bytes" || fail "render: output depends on the listing order"

# Falsified by dropping the escape gsub: red.
X=$(dump x="$(idea '**bold** <script>alert(1)</script> [link](http://x) | pipe' 2026-09-27T00:00:00.000Z)")
out=$(run "$X")
has no "<script>" "$out" "render: HTML a viewer typed is neutralised"
has yes '\*\*bold\*\* \<script\>' "$out" "render: ...and Markdown escaped"
has yes '\| pipe' "$out" "render: ...pipes too"

# A missing dump directory is refused, naming the mkdir. Falsified by rendering an empty queue
# for it: red (an ArtifactData list that never ran would wipe the file).
gone="$d/never-created"
out=$(run "$gone" 2>&1); rc=$?
expect_rc 2 "$rc" "render: a MISSING dump directory is refused"
has yes "mkdir -p $gone/ideas" "$out" "render: ...naming the mkdir the session runs after a successful list"
E=$(dump)
out=$(run "$E")
has yes "data-through: none" "$out" "render: an empty queue is stamped data-through none"
[ "$(printf '%s\n' "$out" | grep -c '^None\.$')" = 4 ] && ok "render: ...and renders four empty sections" || fail "render: empty queue: $out"

# Refusals, each with a good document beside it. Falsified by skipping a bad document: red here.
for c in 'no text|{"status":"not-touched","createdAt":"2026-09-27T00:00:00.000Z"}|has no text' \
  'an unknown status|{"text":"x","status":"done"}|unknown status "done"' \
  'a status that is a prefix|{"text":"x","status":"not"}|unknown status "not"' \
  'no status|{"text":"x","createdAt":"2026-09-27T00:00:00.000Z"}|unknown status undefined' \
  'an array|[1,2]|not a JSON object' 'broken JSON|{ not json|not JSON' 'a BOM|﻿{"text":"x","status":"parked"}|not JSON'; do
  label=${c%%|*}; rest=${c#*|}; body=${rest%|*}; why=${rest##*|}
  B=$(dump ok="$A1" bad="$body")
  run "$B" > "$d/o" 2> "$d/e"; rc=$?
  if [ "$rc" = 2 ] && [ ! -s "$d/o" ] && grep -q 'bad.json' "$d/e" && grep -qF "$why" "$d/e"; then ok "render: refuses a document with $label, naming the file"
  else fail "render: a document with $label: exit $rc, stdout $(wc -c < "$d/o") bytes, stderr $(cat "$d/e")"; fi
done

# ── the summary session-start prints ───────────────────────────────────────────────────────────
# Rendered without the newest idea, then one status change after it. Falsified by comparing
# createdAt instead of updatedAt: red (the status change is missed).
OLD=$(dump b2="$B2" c3="$C3" d4="$D4" e5="$E5"); run "$OLD" > "$d/older.md"
E5b=$(printf '%s' "$E5" | jq -c '.status = "discussed" | .updatedAt = "2026-09-28T00:00:00.000Z"')
NOW=$(dump a1="$A1" b2="$B2" c3="$C3" d4="$D4" e5="$E5b")
out=$(run "$NOW" --summary --against "$d/older.md")
[ "$out" = "Ideas: 2 not touched, 2 discussed, 0 parked, 1 on the roadmap. 2 ideas newer than docs/ideas.md" ] \
  && ok "summary: counts every status, and the ideas changed after the file's data-through" || fail "summary: $out"
run "$Q" > "$d/current.md"
has yes "docs/ideas.md is current" "$(run "$Q" --summary --against "$d/current.md")" "summary: says current when nothing changed"
printf '# Ideas\n' > "$d/bare.md"
has yes "has no generated header: CANNOT CHECK" "$(run "$Q" --summary --against "$d/bare.md")" "summary: CANNOT CHECK on a file with no header"
run "$E" > "$d/empty.md"
has yes "5 ideas newer than docs/ideas.md" "$(run "$Q" --summary --against "$d/empty.md")" "summary: every idea is newer than a file rendered from an empty queue"

# ── --check: the rendered file is the renderer's, unedited ─────────────────────────────────────
# Falsified by comparing only the header's shape (no hash): the hand edit passed (red).
expect_rc 0 "$(run --check "$d/current.md" >/dev/null 2>&1; echo $?)" "check: the renderer's output passes"
sed 's/## Parked/## Parked for now/' "$d/current.md" > "$d/edited.md"
out=$(run --check "$d/edited.md" 2>&1); rc=$?
expect_rc 2 "$rc" "check: a hand edit to the body fails"
has yes "edited by hand" "$out" "check: ...saying so"
sed 1d "$d/current.md" > "$d/headless.md"
has yes "does not begin with the generated header" "$(run --check "$d/headless.md" 2>&1)" "check: a missing header fails"
# A file Standing Tee's infra/ideas-render.mjs rendered (its header names that script) passes.
sed '1s|generated by [^ ]* —|generated by infra/ideas-render.mjs —|' "$d/current.md" > "$d/st.md"
expect_rc 0 "$(run --check "$d/st.md" >/dev/null 2>&1; echo $?)" "check: a file Standing Tee's renderer wrote passes (any renderer's header)"

# ── --url and --mint ───────────────────────────────────────────────────────────────────────────
out=$(cd "$R" && run --url 2>&1); rc=$?
expect_rc 2 "$rc" "url: no registry is refused, never an empty url"
cat > "$R/docs/artifacts.json" <<'EOF'
{ "pages": { "docs/ideas.html": { "url": "https://claude.ai/artifact/IIIIIIIIIIIIIIIIIIIIII", "authorities": ["docs/ideas.html"] } } }
EOF
[ "$(cd "$R" && run --url)" = "https://claude.ai/artifact/IIIIIIIIIIIIIIIIIIIIII" ] && ok "url: read from the registry's IDEAS_PAGE entry" || fail "url: $(cd "$R" && run --url 2>&1)"
m=$(run --mint)
printf '%s' "$m" | jq -e '(.id | test("^cc-[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$")) and (.now | test("^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9:.]{12}Z$"))' >/dev/null \
  && ok "mint: a cc- version-4 uuid and a UTC ISO time" || fail "mint: $m"
[ "$(run --mint | jq -r .id)" != "$(run --mint | jq -r .id)" ] && ok "mint: two ids differ" || fail "mint: the same id twice"

# ── enable.sh, the project check, the context section ──────────────────────────────────────────
(cd "$R" && sh .claude/modules/ideas/enable.sh --check >/dev/null 2>&1); expect_rc 1 $? "enable --check: fails before anything is made"
out=$(cd "$R" && sh .claude/modules/ideas/enable.sh 2>&1); rc=$?
expect_rc 0 "$rc" "enable: makes the skill, the page and the file"
[ -f "$R/.claude/skills/ideas/SKILL.md" ] && [ -f "$R/docs/ideas.html" ] && [ -f "$R/docs/ideas.md" ] && ok "enable: ...each one is there" || fail "enable: $out"
grep -q '__PROJECT__\|__OWNER__' "$R/docs/ideas.html" && fail "enable: the page keeps a placeholder" || ok "enable: the page names the project and owner"
grep -q '<p class="eyebrow">Demo</p>' "$R/docs/ideas.html" && grep -q 'for the owner to scope' "$R/docs/ideas.html" && ok "enable: ...from project.conf" || fail "enable: page text"
if command -v node >/dev/null 2>&1; then
  sed -n '/<script>/,/<\/script>/p' "$R/docs/ideas.html" | sed '1d;$d' > "$d/page.js"
  node --check "$d/page.js" 2>/dev/null && ok "enable: the page's script parses" || fail "enable: the page's script does not parse"
fi
expect_rc 0 "$(run --check "$R/docs/ideas.md" >/dev/null 2>&1; echo $?)" "enable: the file it renders is the renderer's"
sed -i.bak 's/^effort: low$/effort: medium/' "$R/.claude/skills/ideas/SKILL.md" && rm -f "$R/.claude/skills/ideas/SKILL.md.bak"
(cd "$R" && sh .claude/modules/ideas/enable.sh --check >/dev/null 2>&1); expect_rc 0 $? "enable --check: a project's own effort: line is not a difference"
printf 'local edit\n' >> "$R/.claude/skills/ideas/SKILL.md"
(cd "$R" && sh .claude/modules/ideas/enable.sh --check >/dev/null 2>&1); expect_rc 1 $? "enable --check: an edit to the skill's body is"
(cd "$R" && sh .claude/modules/ideas/enable.sh >/dev/null 2>&1)
grep -q '^effort: medium$' "$R/.claude/skills/ideas/SKILL.md" && ! grep -q '^local edit$' "$R/.claude/skills/ideas/SKILL.md" \
  && ok "enable: a refresh restores the body and keeps the project's effort: line" || fail "enable: refresh"

# From the plugin (CLAUDUCTOR_FW names the plugin root, not a .claude/), the installed skill runs the
# tool through clauductor-model, and still compares equal. Falsified by comparing raw bodies: red.
mv "$R/.claude/skills/ideas" "$d/skill.keep"
(cd "$R" && CLAUDUCTOR_FW="$d/plugin-root" sh .claude/modules/ideas/enable.sh >/dev/null 2>&1)
grep -q 'clauductor-model modules/ideas/bin/render.sh --url' "$R/.claude/skills/ideas/SKILL.md" && ! grep -q 'sh .claude/modules' "$R/.claude/skills/ideas/SKILL.md" \
  && ok "enable from the plugin: the skill runs the tool through clauductor-model" || fail "enable from the plugin: $(grep -m2 'render.sh' "$R/.claude/skills/ideas/SKILL.md")"
(cd "$R" && sh .claude/modules/ideas/enable.sh --check >/dev/null 2>&1); expect_rc 0 $? "enable --check: ...and that copy is the module's skill"
rm -rf "$R/.claude/skills/ideas"; mv "$d/skill.keep" "$R/.claude/skills/ideas"
git -C "$R" add -A >/dev/null && git -C "$R" commit -qm start
r=$(cd "$R" && sh .claude/checks/run.sh ideas:queue 2>&1)
has yes "PASS ideas:queue" "$r" "ideas:queue: passes on a project with the file, the url, the page and the skill"
cp "$R/docs/ideas.md" "$d/keep.md"; sed -i.bak 's/^None\.$/Nothing yet./' "$R/docs/ideas.md"; rm -f "$R/docs/ideas.md.bak"
r=$(cd "$R" && sh .claude/checks/run.sh ideas:queue 2>&1)
has yes "edited by hand" "$r" "ideas:queue: FAILS a hand edit to the rendered file (the gate's control)"
cp "$d/keep.md" "$R/docs/ideas.md"
cp "$R/docs/artifacts.json" "$d/reg.json"; echo '{"pages":{}}' > "$R/docs/artifacts.json"
r=$(cd "$R" && sh .claude/checks/run.sh ideas:queue 2>&1)
has yes "registers no url for docs/ideas.html" "$r" "ideas:queue: FAILS when the registry gives the page no url"
cp "$d/reg.json" "$R/docs/artifacts.json"; rm -rf "$R/.claude/skills/ideas"
r=$(cd "$R" && sh .claude/checks/run.sh ideas:queue 2>&1)
has yes "FAIL ideas:queue" "$r" "ideas:queue: FAILS when the skill is not installed"
(cd "$R" && sh .claude/modules/ideas/enable.sh >/dev/null 2>&1)

c=$(sh "$R/.claude/modules/ideas/context.d/session-start/ideas.sh" 2>&1)
has yes "Ideas page: https://claude.ai/artifact/IIIIIIIIIIIIIIIIIIIIII" "$c" "context: the page's link"
has yes "docs/ideas.md as the last close rendered it (data through none): 0 not touched, 0 discussed, 0 on the roadmap, 0 parked" "$c" "context: what the last close rendered"
sed -i.bak 's/^None\.$/Nothing./' "$R/docs/ideas.md"; rm -f "$R/docs/ideas.md.bak"
has yes "STALE — docs/ideas.md is not the renderer's output" "$(sh "$R/.claude/modules/ideas/context.d/session-start/ideas.sh" 2>&1)" "context: a hand-edited file is STALE, never counted"
echo '{"pages":{}}' > "$R/docs/artifacts.json"
has yes "CANNOT CHECK — the Ideas page" "$(sh "$R/.claude/modules/ideas/context.d/session-start/ideas.sh" 2>&1)" "context: no url is CANNOT CHECK"

# ── rule 3: the sessions and /ideas invoke the renderer, and read the url from the registry ────
# A renderer nothing calls leaves the file empty forever. Falsified by deleting the render line
# from the session-close fragment: red.
grep -qE 'render\.sh <scratchpad>/ideas-dump --names \S+ --out <IDEAS_FILE>' "$MOD/skills/session-close/ideas.md" && ok "wiring: session-close renders IDEAS_FILE" || fail "wiring: session-close's fragment has no render --out line"
grep -q -- '--summary --against <IDEAS_FILE>' "$MOD/skills/session-start/ideas.md" && ok "wiring: session-start prints the summary against IDEAS_FILE" || fail "wiring: session-start's fragment has no --summary --against"
for f in "$MOD/skill/SKILL.md" "$MOD/skills/session-start/ideas.md" "$MOD/skills/session-close/ideas.md"; do
  grep -q 'render.sh --url' "$f" && ok "wiring: ${f#"$MOD"/} takes the page's url from the registry" || fail "wiring: ${f#"$MOD"/} does not use render.sh --url"
  grep -q 'node ' "$f" && fail "wiring: ${f#"$MOD"/} still runs node" || ok "wiring: ${f#"$MOD"/} needs no Node"
done
grep -q 'render.sh --mint' "$MOD/skill/SKILL.md" && ok "wiring: /ideas mints ids with render.sh --mint" || fail "wiring: /ideas does not use --mint"
finish
