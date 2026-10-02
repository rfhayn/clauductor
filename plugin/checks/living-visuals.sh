#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# The living-visuals module (.claude/modules/living-visuals), tested whether a project turns it on
# or not: living.sh's page rules, --regen, --list, the module's project check, its context section
# and its wiring, each against a throwaway project, never its own source. Ported from the generic
# half of Standing Tee's living-visuals.test.ts, and falsified: each red case has a green mirror,
# and the comment above a case names the mutation of living.sh that turned it red.
. "$(dirname "$0")/lib.sh"
need git jq

d=$(scratch)
R="$d/app"; new_repo "$R"
mkdir -p "$R/.claude/lib" "$R/.claude/modules" "$R/.claude/checks" "$R/.claude/skills/update-page" "$R/docs" "$R/src" "$R/bin"
cp "$CLAUDUCTOR_FW/lib/conf.sh" "$CLAUDUCTOR_FW/lib/modules.sh" "$R/.claude/lib/"
cp "$CLAUDUCTOR_FW/checks/run.sh" "$CLAUDUCTOR_FW/checks/lib.sh" "$R/.claude/checks/"
cp -R "$CLAUDUCTOR_FW/modules/artifacts" "$CLAUDUCTOR_FW/modules/living-visuals" "$R/.claude/modules/"
printf 'MODULES="artifacts living-visuals"\n' > "$R/.claude/project.conf"
printf -- '---\nname: update-page\n---\nRefresh it.\n' > "$R/.claude/skills/update-page/SKILL.md"
LV="$R/.claude/modules/living-visuals/bin/living.sh"
lv() { (cd "$R" && sh "$LV" "$@") 2>&1; }
has() {  # has WANT(yes|no) TEXT HAYSTACK LABEL
  case "$3" in *"$2"*) got=yes ;; *) got=no ;; esac
  [ "$got" = "$1" ] && ok "$4" || fail "$4 (looked for '$2'): $(printf '%s' "$3" | grep -v '^ok' | head -c 700)"
}
# red LABEL WANT [ENV...]: --check must exit 1 and say WANT. green LABEL [ENV...]: exit 0.
red() { _l=$1 _w=$2; shift 2; _o=$(cd "$R" && env "$@" sh "$LV" --check 2>&1); _r=$?
  if [ "$_r" = 1 ] && printf '%s' "$_o" | grep -F -- "$_w" | grep -q '^FAIL'; then ok "$_l"; else fail "$_l (exit $_r; wanted a FAIL with '$_w'): $(printf '%s\n' "$_o" | grep -v '^ok' | head -c 600)"; fi; }
green() { _l=$1; shift; _o=$(cd "$R" && env "$@" sh "$LV" --check 2>&1); _r=$?
  if [ "$_r" = 0 ]; then ok "$_l"; else fail "$_l (exit $_r): $(printf '%s\n' "$_o" | grep -v '^ok' | head -c 600)"; fi; }

# The authorities: a queue file the block is generated from, and facts the claims are read from.
printf 'alpha\nbeta\n' > "$R/src/queue.txt"
printf '#!/bin/sh\necho "<ul>"; sed "s|.*|<li>&</li>|" src/queue.txt; echo "</ul>"\n' > "$R/bin/queue-html.sh"
printf '3\n' > "$R/src/count.txt"
printf 'phase-1 Ann\nphase-2 Bo\n' > "$R/src/owners.txt"
page() {  # page [EXTRA-BODY]: the living page, current
  cat > "$R/docs/page.html" <<EOF
<!doctype html>
<html><head><title>Living</title></head>
<body>
<h1>Status</h1>
<p>We have <b data-claim="item-count">three</b> items.</p>
<p>Phase 1: <b data-claim="owner-1">Ann</b>; phase 2: <b data-claim='owner-2'>Bo</b>.</p>
<p>Started <span data-days-since="2026-01-05" data-suffix=" days so far">in progress</span>.</p>
<!-- generated:queue:begin — do not hand-edit: regenerate with living.sh --regen -->
$(cd "$R" && sh bin/queue-html.sh)
<!-- generated:queue:end -->
${1:-}
<script>
  document.querySelectorAll("[data-days-since]").forEach(function (el) { el.textContent = "n"; });
</script>
<script src="https://cdnjs.cloudflare.com/x.js"></script>
</body>
</html>
EOF
}
page
cat > "$R/docs/artifacts.json" <<'EOF'
{
  "pages": {
    "docs/page.html": {
      "url": "https://claude.ai/artifact/LLLLLLLLLLLLLLLLLLLLLL",
      "authorities": ["src/*.txt"],
      "refresh": "update-page",
      "generated": { "queue": { "run": "sh bin/queue-html.sh", "outside": "cat src/queue.txt; echo '<li>'" } },
      "claims": {
        "item-count": "cat src/count.txt",
        "owner-*": { "run": "awk -v p=\"phase-$1\" '$1 == p { print $2 }' src/owners.txt",
                     "each": "sed 's/^phase-//; s/ .*//' src/owners.txt" }
      }
    }
  }
}
EOF
git -C "$R" add -A && git -C "$R" commit -qm start
(cd "$R" && sh .claude/modules/artifacts/bin/currency.sh --stamp docs/page.html --note "seed" >/dev/null) && git -C "$R" commit -qam stamp
reg() { jq --indent 2 "$1" "$R/docs/artifacts.json" > "$d/r" && cat "$d/r" > "$R/docs/artifacts.json"; }
cp "$R/docs/artifacts.json" "$d/reg.base"
base() {
  cp "$d/reg.base" "$R/docs/artifacts.json"; printf 'alpha\nbeta\n' > "$R/src/queue.txt"; printf '3\n' > "$R/src/count.txt"
  printf 'phase-1 Ann\nphase-2 Bo\n' > "$R/src/owners.txt"; page
}

out=$(lv --check); rc=$?
expect_rc 0 "$rc" "check: a current living page passes every rule"
for w in "refreshed by /update-page" "read whole (ends </html>)" "generated block queue matches its command" \
  "nothing block queue carries is hand-mirrored outside it" "claim item-count = \"three\", as its command says" \
  "claim owner-2 = \"Bo\"" "claim family owner-* covers exactly the 2 members" "a script of its own fills the day counters" \
  "no typed elapsed duration"; do
  has yes "$w" "$out" "check: ...$w"
done

# ── the page is whole and runs ─────────────────────────────────────────────────────────────────
echo '{"pages":{}}' > "$R/docs/artifacts.json"
red "check: a registry with no living page FAILS (a check over nothing is decoration)" "no entry in docs/artifacts.json declares"
base; reg '.pages["docs/page.html"].refresh = "update-nothing"'
# Falsified by accepting any string as the refresh: red.
red "check: a refresh naming a skill that is not there FAILS" "names /update-nothing, which is not a skill here"
base; reg '.pages["docs/page.html"].refresh = "edit"'; green "check: refresh \"edit\" is a hand edit, and passes"
base; reg '.pages["docs/page.html"].refresh = 7'; red "check: a refresh that is not a name FAILS" "not a skill name or \"edit\""
base; reg '.pages["docs/gone.html"] = {url: "https://claude.ai/artifact/GGGGGGGGGGGGGGGGGGGGGG", refresh: "edit"}'
red "check: a living page with no file FAILS" "docs/gone.html: is a living page"
# Falsified by dropping the </html> test: red.
base; sed -i.bak '$d' "$R/docs/page.html"; red "check: an HTML page read without its </html> FAILS as incomplete" "read INCOMPLETE"
base; printf '<p>a fragment, <b data-claim="item-count">3</b> items</p>\n<!-- generated:queue:begin -->\n%s\n<!-- generated:queue:end -->\n<script>1</script>\n' "$(cd "$R" && sh bin/queue-html.sh)" > "$R/docs/page.html"
reg '.pages["docs/page.html"].claims = {"item-count": "cat src/count.txt"}'
green "check MIRROR: a fragment (no <html>) is not held to </html>"
if command -v node >/dev/null 2>&1; then
  # Falsified by not running node --check: red. A raw quote inside a quoted string is the classic.
  base; page '<script>var notes = ["a "quoted" word"];</script>'
  red "check: an inline script that does not parse FAILS (the page renders blank)" "an inline script does not parse"
  base; page '<SCRIPT>var x = ;</SCRIPT>'
  red "check: ...an upper-case <SCRIPT> is parsed too" "an inline script does not parse"
  base; page '<script>var notes = ["a "quoted" word"];</script>'
  green "check: LIVING_SCRIPT_CHECK=off does not parse (and says so)" LIVING_SCRIPT_CHECK=off
  has yes "inline scripts NOT parsed (LIVING_SCRIPT_CHECK=off)" "$(cd "$R" && LIVING_SCRIPT_CHECK=off sh "$LV" --check 2>&1)" "check: ...naming the key on the page"
fi
base; red "check: an unknown LIVING_SCRIPT_CHECK FAILS" "is not node or off" LIVING_SCRIPT_CHECK=maybe

# ── generated blocks ───────────────────────────────────────────────────────────────────────────
# Falsified by comparing against the page's own block instead of the command: red.
base; printf 'alpha\nbeta\ngamma\n' > "$R/src/queue.txt"
red "generated: a source moved and the block not regenerated FAILS" "generated block queue no longer matches"
out=$(lv --regen); has yes "docs/page.html: block queue regenerated" "$out" "regen: rewrites the block from its command"
green "regen: ...after which the check passes"
has yes "block queue already current" "$(lv --regen docs/page.html)" "regen: a second run changes nothing"
has yes "<li>gamma</li>" "$(cat "$R/docs/page.html")" "regen: ...and the page carries the new row"
base; sed -i.bak 's|<li>beta</li>|<li>beta, by hand</li>|' "$R/docs/page.html"
red "generated: a hand edit inside the block FAILS" "generated block queue no longer matches"
base; sed -i.bak 's|^<ul>$|   <ul>|; s|^</ul>$|</ul>  \
|' "$R/docs/page.html"
green "generated MIRROR: whitespace at the block's ends is not a difference (both sides trimmed)"
base; sed -i.bak 's|<li>beta</li>|  <li>beta</li>|' "$R/docs/page.html"
red "generated: ...but whitespace inside it is" "generated block queue no longer matches"
base; sed -i.bak '/generated:queue:end/d' "$R/docs/page.html"
red "generated: a lost end marker FAILS" "has 1 generated:queue:begin and 0 generated:queue:end markers"
out=$(lv --regen); rc=$?
expect_rc 1 "$rc" "regen: refuses a block with no end marker..."
has yes "left as it was" "$out" "regen: ...and leaves the page as it was"
base; printf '<!-- generated:extra:begin -->\nx\n<!-- generated:extra:end -->\n' > "$d/x"; page "$(cat "$d/x")"
red "generated: a block on the page that the registry does not declare FAILS" "carries a generated:extra block that its registry entry does not declare"
base; reg '.pages["docs/page.html"].generated.queue.run = "exit 3"'
red "generated: a command that fails FAILS, never reads as a match" "block queue's command (exit 3) exited 3"
# Falsified by searching the whole page (the block included): red on the green mirror below.
base; page '<p>Next up: beta.</p>'
red "generated: a string the block carries, mirrored outside it, FAILS" "names outside its generated:queue block what only the block may carry: beta"
base; page '<ul><li>a list of my own</li></ul>'
red "generated: ...the block's own markup outside it too" "what only the block may carry: <li>"
base; reg '.pages["docs/page.html"].generated.queue.outside = "true"'
red "generated: an outside command that prints nothing FAILS (it searches for nothing)" "printed nothing, so the leak check searches the page for nothing"

# ── claims ─────────────────────────────────────────────────────────────────────────────────────
# Falsified by comparing only the claims' count: red.
base; printf '4\n' > "$R/src/count.txt"
red "claims: a claim its command contradicts FAILS" "claim item-count says \"three\", but its command (cat src/count.txt) says \"4\""
base; sed -i.bak 's/>three</>Three</' "$R/docs/page.html"; green "claims MIRROR: Three, three and 3 are one number"
base; sed -i.bak 's/>three</>3</' "$R/docs/page.html"; green "claims MIRROR: ...digits too"
base; printf 'phase-1 Ann\nphase-2 Cy\n' > "$R/src/owners.txt"
red "claims: a family member its command contradicts FAILS (the * part is \$1)" "claim owner-2 says \"Bo\", but its command"
base; page '<p>Done: <b data-claim="done-count">2</b></p>'
red "claims: an annotated claim with no command FAILS (decoration)" "data-claim=\"done-count\" (\"2\") has no command"
base; sed -i.bak 's/data-claim="item-count"/data-claim="item-total"/' "$R/docs/page.html"
red "claims: a declared claim the page no longer carries FAILS (renamed)" "declares claim item-count, but the page carries no data-claim it matches"
# Falsified by dropping the each comparison: red.
base; printf 'phase-1 Ann\nphase-2 Bo\nphase-3 Cy\n' > "$R/src/owners.txt"
red "claims: a family missing a member its authority defines FAILS" "no claim on the page for: 3"
base; printf 'phase-1 Ann\n' > "$R/src/owners.txt"
red "claims: a family claiming a member its authority does not define FAILS" "claimed, but not a member the authority defines: 2"
base; reg '.pages["docs/page.html"].claims["item-count"] = "sleep 5; echo 3"'
start=$(date +%s); red "claims: a command over LIVING_COMMAND_SECONDS is stopped and FAILS" "did not finish within 1s" LIVING_COMMAND_SECONDS=1
[ $(( $(date +%s) - start )) -le 4 ] && ok "claims: ...promptly" || fail "claims: the stopped command took $(( $(date +%s) - start ))s"
base; start=$(date +%s); green "speed: a command that finishes does not wait for its watcher"
[ $(( $(date +%s) - start )) -le 3 ] && ok "speed: the whole check ran in $(( $(date +%s) - start ))s" || fail "speed: the check took $(( $(date +%s) - start ))s (each command waiting a second?)"

# ── durations ──────────────────────────────────────────────────────────────────────────────────
# Falsified by dropping the round trip: red on 2026-02-30.
base; sed -i.bak 's/2026-01-05/2026-02-30/' "$R/docs/page.html"; red "days: a counter date that is not a real day FAILS" "not a past YYYY-MM-DD: 2026-02-30"
base; sed -i.bak 's/2026-01-05/2999-01-01/' "$R/docs/page.html"; red "days: a counter date in the future FAILS" "2999-01-01"
base; sed -i.bak 's/querySelectorAll("\[data-days-since\]")/querySelectorAll(".dur")/' "$R/docs/page.html"
red "days: counters no script of the page fills FAIL" "none of its scripts mentions data-days-since"
base; sed -i.bak 's|<span data-days-since="2026-01-05" data-suffix=" days so far">in progress</span>|65 days so far|' "$R/docs/page.html"
red "days: a typed elapsed duration FAILS" "a typed elapsed duration is wrong the next morning: \"65 days so far\""
base; page '<!-- the gate ran 65 days so far, once --><script>var s = "3 weeks ago";</script>'
green "days MIRROR: a duration in a comment or a script is not page text"
base; page '<p>Gate 2A closed after 41 days.</p>'; green "days MIRROR: a finished duration (history) is not an elapsed one"
base; sed -i.bak 's|<span data-days-since="2026-01-05" data-suffix=" days so far">in progress</span>|65 days so far|' "$R/docs/page.html"
green "days: LIVING_TYPED_DURATION=\"\" turns the typed-duration rule off" LIVING_TYPED_DURATION=

# ── every annotation is read, or the check says so ─────────────────────────────────────────────
# The claim extractor reads `data-claim="name">value<`; an annotation in any other shape (markup in
# the value, a name with a dot, no quotes, spaces round the =) would be skipped and the page pass.
# Every `data-claim=` is counted and the count must equal the claims read. Falsified by dropping
# the count comparison: each of the four goes green.
base; page '<p>Total <b data-claim="item-count"><i>7</i></b> items.</p>'
red "claims: an annotation whose value is markup is not skipped" "data-claim attributes, but 3 were read"
base; page '<p>Range <b data-claim="mig.range">9</b></p>'
red "claims: an annotation with a name the reader cannot parse is not skipped" "data-claim attributes, but 3 were read"
base; page '<p>Other <b data-claim=other>9</b></p>'
red "claims: an unquoted annotation is not skipped" "data-claim attributes, but 3 were read"
base; page '<p>Other <b data-claim = "other">9</b></p>'
red "claims: an annotation with spaces round its = is not skipped" "data-claim attributes, but 3 were read"
# A value that is only whitespace claims nothing, even against a command that prints nothing.
# Falsified by comparing the trimmed strings only: green.
base; page '<p>Blank <b data-claim="blank-one">   </b></p>'
reg '.pages["docs/page.html"].claims["blank-one"] = "true"'
red "claims: a whitespace-only claim does not equal empty output" "claim blank-one claims nothing"
# A generator that prints nothing would make an empty block that passes forever. Falsified by
# dropping the empty-output test: green.
base; reg '.pages["docs/page.html"].generated.queue.run = "true"'
red "generated: a command that prints nothing FAILS" "block queue's command (true) printed nothing"
has yes "printed nothing" "$(lv --regen docs/page.html)" "regen: ...and refuses to write an empty block"

# --regen rewrites only the block: a page whose bytes are not UTF-8 is refused, never re-encoded
# (jq would turn the bad byte into U+FFFD outside the block, and say "regenerated"). Falsified by
# dropping the round-trip test: the page's bytes change.
base; printf 'alpha\nbeta\nomega\n' > "$R/src/queue.txt"
printf '<p>caf\351 is Latin-1, not UTF-8</p>\n' >> "$R/docs/page.html"
cp "$R/docs/page.html" "$d/latin1.html"
out=$(lv --regen docs/page.html); rc=$?
expect_rc 1 "$rc" "regen: refuses a page that is not UTF-8..."
has yes "not valid UTF-8" "$out" "regen: ...saying why"
cmp -s "$R/docs/page.html" "$d/latin1.html" && ok "regen: ...and leaves its bytes exactly as they were" || fail "regen: rewrote a non-UTF-8 page"
base; printf 'alpha\nbeta\nomega\n' > "$R/src/queue.txt"; printf '<p>café, naïve, 日本</p>\n' >> "$R/docs/page.html"
lv --regen docs/page.html >/dev/null
grep -q '<li>omega</li>' "$R/docs/page.html" && grep -q '<p>café, naïve, 日本</p>' "$R/docs/page.html" \
  && ok "regen MIRROR: a UTF-8 page with non-ASCII text is regenerated, the rest kept byte for byte" || fail "regen: UTF-8 page"
ls "$R/docs" | grep -q 'living-regen' && fail "regen: left a temporary file beside the page" || ok "regen: writes through a temporary file it removes"

# ── --list: currency, with how each page is refreshed ──────────────────────────────────────────
base; git -C "$R" add -A && git -C "$R" commit -qm reset >/dev/null
out=$(lv --list --ref HEAD); rc=$?
expect_rc 0 "$rc" "list: every living page OK exits 0"
has yes "OK      docs/page.html" "$out" "list: ...one line per page"
printf 'alpha\nbeta\ndelta\n' > "$R/src/queue.txt"; git -C "$R" commit -qam "Add delta"
out=$(lv --list --ref HEAD); rc=$?
expect_rc 1 "$rc" "list: a page whose sources moved exits 1"
has yes "BEHIND  docs/page.html" "$out" "list: ...reads BEHIND"
has yes "→ refresh with /update-page, then stamp it" "$out" "list: ...naming the skill that refreshes it"
has yes "Living pages vs HEAD" "$out" "list: ...and the tree it read"
out=$(lv --list --worktree); has yes "BEHIND  docs/page.html" "$out" "list --worktree: reads the files on disk"
has yes "not a living page" "$(lv --regen docs/nope.html)" "regen: a page that is not living is refused"

# ── the project check, the context section ─────────────────────────────────────────────────────
(cd "$R" && sh .claude/modules/living-visuals/bin/living.sh --regen >/dev/null) && git -C "$R" commit -qam regen
r=$(cd "$R" && sh .claude/checks/run.sh living-visuals:pages 2>&1)
has yes "PASS living-visuals:pages" "$r" "living-visuals:pages: passes on a current page, in checks/run.sh"
sed -i.bak 's/>three</>four</' "$R/docs/page.html"
r=$(cd "$R" && sh .claude/checks/run.sh living-visuals:pages 2>&1); rc=$?
expect_rc 1 "$rc" "living-visuals:pages: FAILS the run on a false claim (the gate's control)"
has yes "claim item-count says \"four\"" "$r" "living-visuals:pages: ...naming it"
c=$(sh "$R/.claude/modules/living-visuals/context.d/session-close/living-visuals.sh" 2>&1)
has yes "Living pages vs the working tree" "$c" "context: each living page's currency on the close's tree"
has yes "Page rules (the gate's living-visuals:pages) failing on this tree" "$c" "context: ...and the page rules failing on it"
has yes "claim item-count says \"four\"" "$c" "context: ...each named"

# ── rule 3: the close and the merge invoke the tool ────────────────────────────────────────────
F="$CLAUDUCTOR_FW/modules/living-visuals/skills"
grep -q 'living.sh --regen' "$F/session-close/living-visuals.md" && grep -q 'run.sh living-visuals:pages' "$F/session-close/living-visuals.md" \
  && ok "wiring: session-close regenerates the blocks and runs the check" || fail "wiring: session-close's fragment does not run --regen and the check"
grep -q 'living.sh --list --ref origin/main' "$F/merge-pr/living-visuals.md" && ok "wiring: merge-pr lists the living pages the merge moved" || fail "wiring: merge-pr's fragment does not run --list"
finish
