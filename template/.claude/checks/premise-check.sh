#!/bin/sh
# The premise-check module (.claude/modules/premise-check), tested whether it is on or not, in
# throwaway repositories (no network: an issue fixture and a stub gh). Standing Tee's vitest suite
# for infra/premise-check.mjs and merge-guard rule 6, ported case for case:
#   - premise-check.sh against a synthetic repo built to reproduce Standing Tee's #239 (a sentence
#     quoted with a STRAIGHT apostrophe that the code spells with a CURLY one, a docs copy with the
#     straight one, and the commit that changed it landing BEFORE the issue was filed), and every
#     shape its reviews found: a line-cited file edited since filing, a bare filename that means
#     several files, a deleted file, a moved file, a dotted config key, an HTML entity, a quote in a
#     code span, a premise revised in a comment, the receipt, a bad ref, and a subdirectory;
#   - the receipt library: a receipt per closed issue, and GitHub's closing keywords in a body;
#   - the guard rule through the real pr-merge-guard.sh: blocked without a receipt, allowed with
#     one, the title fallback, the body-only and union cases, nothing to check, a module that is
#     off, and PREMISE_REQUIRED_ON;
#   - enable.sh puts the check into the panel's fix lane, once.
# With the module on, modules/premise-check/checks/project.sh checks this project's own wiring.
. "$(dirname "$0")/lib.sh"
need git jq

M="$ROOT/.claude/modules/premise-check"
S="$M/premise-check.sh"
d=$(scratch)
has() {  # has WANT(yes|no) NEEDLE HAYSTACK LABEL
  case "$3" in *"$2"*) got=yes ;; *) got=no ;; esac
  [ "$got" = "$1" ] && ok "$4" || fail "$4 (looked for '$2'): $(printf '%s' "$3" | head -c 600)"
}
re() {  # re WANT(yes|no) ERE HAYSTACK LABEL
  if printf '%s\n' "$3" | grep -qE -- "$2"; then got=yes; else got=no; fi
  [ "$got" = "$1" ] && ok "$4" || fail "$4 (pattern '$2'): $(printf '%s' "$3" | head -c 600)"
}

# --- premise-check.sh, against a synthetic history -------------------------------------------
R="$d/repo"; new_repo "$R"
put() { mkdir -p "$(dirname "$R/$1")"; printf '%s\n' "$2" > "$R/$1"; }
commit() { git -C "$R" add -A && GIT_AUTHOR_DATE=$2 GIT_COMMITTER_DATE=$2 git -C "$R" commit -qm "$1"; }
put app/Card.tsx 'const msg = "This round is closed.";
const legacyHint = "ask the committee";'
put lib/window.ts 'export const closeWindow = 3;'
put app/score/page.tsx 'export default function Score() {}'   # two files share a basename
put app/legal/page.tsx 'export default function Legal() {}'
put app/old/page.tsx 'export default function Old() {}'
put app/Note.tsx '<p>Another golfer&rsquo;s returned card is waiting.</p>'   # JSX's apostrophe
put vitest.config.ts 'export default { test: { sequence: { shuffle: { files: true } } } };'
put packages/ocr/reader.ts 'export function readCard(image: string) {
  return image.length;
}'
commit "add card screen" 2026-09-01T12:00:00Z
put app/Card.tsx 'const msg = "Reopen it to change a score, or use this week’s corrections screen.";
const legacyHint = "ask the committee";'
commit "name the way to a correction (#215)" 2026-09-16T12:00:00Z
CHANGED=$(git -C "$R" rev-parse --short=7 HEAD)
# Filed 2026-09-18. After it: a docs copy (straight apostrophe), a removed string, edits, a move.
put docs/roadmap.md "The message names \"this week's corrections screen\" with no link."
commit "roadmap: record the campaign finding" 2026-09-19T12:00:00Z
put app/Card.tsx 'const msg = "Reopen it to change a score, or use this week’s corrections screen.";'
put lib/window.ts '// moved
export const closeWindow = 3;'
put app/score/page.tsx '// edited
export default function Score() {}'
git -C "$R" rm -q app/old/page.tsx
commit "drop the legacy hint" 2026-09-20T12:00:00Z
mkdir -p "$R/parked/ocr"; git -C "$R" mv packages/ocr/reader.ts parked/ocr/reader.ts
commit "park the OCR reader" 2026-09-21T12:00:00Z
HEAD12=$(git -C "$R" rev-parse HEAD | cut -c1-12)

# check BODY [COMMENT...]: the report on issue #239 (filed 2026-09-18) at HEAD, from $R or $IN.
check() {
  _b=$1; shift
  jq -n --arg b "$_b" '{number: 239, title: "closed-card message", createdAt: "2026-09-18T02:04:54Z", body: $b, comments: ($ARGS.positional | map({body: .}))}' --args "$@" > "$d/issue.json"
  (cd "${IN:-$R}" && sh "$S" --issue-json "$d/issue.json" --at HEAD 2>&1)
}

out=$(check "It names \"this week's corrections screen.\" and doesn't mention Reopen.")
next=$(printf '%s\n' "$out" | awk 'f { print; exit } /"this week'"'"'s corrections screen" — present/ { f = 1 }')
has yes "app/Card.tsx:1" "$next" "premise: a straight-quoted sentence is found where the code spells it curly, CODE listed before docs"
has yes "Reopen it to change a score" "$next" "premise: ...and the line shown already says what the issue claims it lacks"
out=$(check "It names \"this week's corrections screen.\"")
has yes "$CHANGED" "$out" "premise: the commit that changed the quoted sentence BEFORE filing is named"
re yes '2d before filing\) name the way to a correction' "$out" "premise: ...dated relative to the filing"
re no 'history: [0-9a-f]+ 2026-09-19' "$out" "premise: ...and the docs commit with the same spelling does not crowd it out"
out=$(check 'The card falls back to `ask the committee`.')
re yes '"ask the committee" — \*\*NO LONGER IN THE TREE' "$out" "premise: a named string that is gone says NO LONGER IN THE TREE"
re yes '2d AFTER filing\) drop the legacy hint' "$out" "premise: ...and names the commit that removed it"
out=$(check 'See `window.ts:1`.')
has yes '`window.ts:1` → lib/window.ts — **changed 1x since filing' "$out" "premise: a file cited by line number that changed after filing is flagged"
out=$(check 'See `score/page.tsx:1` and `page.tsx`.')
has yes '`score/page.tsx:1` → app/score/page.tsx — **changed 1x since filing' "$out" "premise: a cited path resolves to its own file"
has no '`score/page.tsx:1` → app/legal' "$out" "premise: ...never to a same-named file elsewhere"
has yes '`page.tsx` — **ambiguous: 2 files match**' "$out" "premise: a bare filename reports EVERY file it can mean"
out=$(check 'The screen lives in `app/old/page.tsx`.')
re yes '`app/old/page\.tsx` — \*\*file NOT in the tree\*\*\. [0-9a-f]+ 2026-09-20 drop the legacy hint' "$out" "premise: a deleted file names the commit that deleted it"
has no "unchanged since filing" "$out" "premise: ...never a same-named survivor as unchanged"
out=$(check 'The reader is `packages/ocr/reader.ts:2`.')
re yes '\*\*file NOT in the tree\*\*\. [0-9a-f]+ 2026-09-21 park the OCR reader — MOVED packages/ocr/reader\.ts → parked/ocr/reader\.ts' "$out" "premise: a moved file says MOVED and where to, not 'check the path'"
out=$(check 'It declares “export const `closeWindow`”.')
has yes '"export const `closeWindow`" — present' "$out" "premise: a code span inside a quote is kept, and found by its backtick-free spelling"
out=$(check 'Under `sequence.shuffle.files`, already on.')
has no "file NOT in the tree" "$out" "premise: a dotted config key is not read as a missing file"
has yes 'NOT checked: "sequence.shuffle.files"' "$out" "premise: ...it is listed as not checked"
out=$(check "It says \"Another golfer's returned card\".")
re yes '"Another golfer'"'"'s returned card" — present, 1 place\(s\):' "$out" "premise: a sentence JSX spells with an HTML entity is found"
has yes "$(printf '1 place(s):\n    app/Note.tsx:1')" "$out" "premise: ...at its line"
body='See `app/score/page.tsx` and `closeWindow`.'
a=$(check "$body"); b=$(IN="$R/lib" check "$body")
[ -n "$a" ] && [ "$a" = "$b" ] && ok "premise: a subdirectory gets the same report as the root" || fail "premise: a subdirectory gets the same report as the root: it differs: $(printf '%s' "$b" | head -c 300)"
out=$(check 'It calls `setStatus(x, "closed-and-gone")` here.')
re yes 'NOT checked: "setStatus\(x, "closed-and-gone"\)"$' "$out" "premise: a quoted word inside a code span is not a second term"
out=$(check 'Nothing here.' 'Actually it is `closeWindow`.')
has yes '"closeWindow" — present' "$out" "premise: COMMENTS are read, where a premise is often revised"
out=$(check 'Run `TZ=UTC vitest run --sequence.seed=1789275698840` to see it.')
has yes 'NOT checked: "TZ=UTC vitest run' "$out" "premise: what it could not check is named, not dropped"
out=$(check "\"this week's corrections screen\"")
[ "$(printf '%s\n' "$out" | tail -n 1)" = "premise-check: #239 @ $HEAD12" ] && ok "premise: the last line is the receipt the guard rule looks for, naming the ref checked" \
  || fail "premise: the last line is the receipt the guard rule looks for: it is $(printf '%s\n' "$out" | tail -n 1)"
out=$(check 'Before
```
Fixes `inside` the fence
```
and `closeWindow` after.')
has no '"inside"' "$out" "premise: a fenced block is skipped..."
has yes "1 fenced block(s) skipped" "$out" "premise: ...and counted"
rc=0; (cd "$R" && sh "$S" --issue-json "$d/issue.json" --at nope >"$d/o" 2>"$d/e") || rc=$?
[ "$rc" = 1 ] && grep -q 'premise-check: FAULT' "$d/e" && [ ! -s "$d/o" ] && ok "premise: a ref that does not exist is a FAULT (exit 1, nothing on stdout), not an empty report" \
  || fail "premise: a ref that does not exist is a FAULT: exit $rc, stderr $(cat "$d/e"), stdout $(head -c 200 "$d/o")"
rc=0; (cd "$R" && sh "$S" >/dev/null 2>"$d/e") || rc=$?
[ "$rc" = 1 ] && grep -q 'usage' "$d/e" && ok "premise: no issue named is a usage FAULT" || fail "premise: no issue named is a usage FAULT: exit $rc $(cat "$d/e")"
# The default ref is origin/<MAIN_BRANCH>, fetched first.
git init -q --bare "$d/origin.git"; git -C "$R" remote add origin "$d/origin.git"; git -C "$R" push -q origin HEAD:main 2>/dev/null
out=$(cd "$R" && sh "$S" --issue-json "$d/issue.json" 2>&1)
has yes "Checked against origin/main @ $HEAD12" "$out" "premise: with no --at it checks origin/main, fetched"

# --- the receipt library ------------------------------------------------------------------------
. "$M/lib/receipt.sh"
miss() { premise_receipt_missing "$1" "$2"; }
miss "x
premise-check: #239 @ ed0c339ff981
premise-check: #241 @ ed0c339ff981" "239 241" >/dev/null && ok "receipt: passes when every closed issue has one" || fail "receipt: passes when every closed issue has one: it refused two receipts"
got=$(miss "premise-check: #2390 @ ed0c339ff981
premise-check: #241 @ abcdef1" "239 241"); rc=$?
[ "$rc" = 1 ] && [ "$got" = "#239" ] && ok "receipt: names the ONE issue without one, and a prefix of another number is not one" || fail "receipt: names the ONE issue without one, and a prefix of another number is not one: got '$got' (exit $rc)"
miss "premise-check: #239 @ TODO" 239 >/dev/null && fail "receipt: a line with no sha is a mention, not a receipt of a run: it was accepted" || ok "receipt: a line with no sha is a mention, not a receipt of a run"
refs() { closing_refs_in_body "$1" "${2:-acme/app}"; }
while IFS='|' read -r label body want; do
  body=$(printf '%b' "$body")
  got=$(refs "$body")
  [ "$got" = "$want" ] && ok "closing refs: reads $label" || fail "closing refs: reads $label: got '$got', want '$want'"
done <<'EOF'
a keyword at the very start, with a trailing period|Closes #409.\n\n## Why|409
a colon after the keyword|This fixes: #12|12
this repo named in full|Resolves acme/app#7|7
this repo named in another case|resolved ACME/App#8, then|8
an issue URL|Fixed https://github.com/acme/app/issues/21!|21
every keyword, once each|close #1 closed #2 fix #3 FIXED #4 Resolve #5 closes #6|1 2 3 4 5 6
several on one line, deduplicated|Fixes #30, fixes #31 and closes #30.|30 31
EOF
while IFS='|' read -r label body; do
  body=$(printf '%b' "$body")
  got=$(refs "$body")
  [ -z "$got" ] && ok "closing refs: ignores $label" || fail "closing refs: ignores $label: got '$got', want nothing"
done <<'EOF'
another repository's issue|Closes owner/other#3
another repository's issue URL|Fixes https://github.com/owner/other/issues/3
a mention with no keyword|See #5 and #6; refs #7
a keyword inside a word|prefixes #9, suffixes #10, unresolved #11
a keyword in inline code|Write `Closes #12` in the body.
a keyword in a fenced block|Example:\n```\nFixes #13\n```\nDone.
EOF

# --- the guard rule, through the real merge guard -------------------------------------------------
G="$d/app"; new_repo "$G"
mkdir -p "$G/.claude/hooks/lib" "$G/.claude/lib" "$G/.claude/modules" "$G/docs" "$d/bin"
cp "$ROOT/.claude/hooks/pr-merge-guard.sh" "$G/.claude/hooks/"
cp "$ROOT/.claude/hooks/lib/"* "$G/.claude/hooks/lib/"
cp "$ROOT/.claude/lib/conf.sh" "$ROOT/.claude/lib/change.sh" "$ROOT/.claude/lib/evals.sh" "$ROOT/.claude/lib/modules.sh" "$G/.claude/lib/"
cp "$ROOT/.claude/scenario-trace.sh" "$G/.claude/"
cp -R "$M" "$G/.claude/modules/premise-check"
printf 'MODULES="premise-check"\n' > "$G/.claude/project.conf"
printf '# Journal\n\n## Session 1 — 2026-01-01 — a — start\n' > "$G/docs/development-journal.md"
git -C "$G" add -A && git -C "$G" commit -qm base
git init -q --bare "$G/acme/app.git"; echo "acme/" >> "$G/.git/info/exclude"
git -C "$G" remote add origin acme/app.git
(cd "$G" && git push -q origin HEAD:main 2>/dev/null && git fetch -q origin)
GH=$(git -C "$G" rev-parse HEAD)
printf '%s\tfull\tclean\tall\n' "$GH" > "$G/.git/ci-receipt"
cat > "$d/bin/gh" <<'EOF'
#!/bin/sh
case "$1 $2" in
  "pr checks") echo '[]' ;;
  "pr view")
    case "$*" in
      *closingIssuesReferences*) [ -n "${FAKE_PR:-}" ] && printf '%s\n' "$FAKE_PR" ;;
      *headRefName*) printf '{"headRefName":"%s","headRefOid":"%s"}\n' "$GH_BRANCH" "$GH_HEAD" ;;
      *headRefOid*) echo "$GH_HEAD" ;;
    esac ;;
  "run list") echo 0 ;;
  *) exit 1 ;;
esac
EOF
chmod +x "$d/bin/gh"
# merge WANT LABEL BODY [CLOSES] [TITLE] [BRANCH]: a merge of PR 290 through the guard.
merge() {
  _w=$1 _l=$2 _b=$3 _c=${4-239} _t=${5:-Fix the closed card} _br=${6:-fix/239-card}
  _pr=$(jq -cn --arg t "$_t" --arg b "$_b" --arg c "$_c" '{title: $t, body: $b, closingIssuesReferences: ($c | split(" ") | map(select(. != "") | {number: tonumber}))}')
  rc=0
  out=$(payload "gh pr merge 290 --squash" "$G" | (cd "$G" && env PATH="$d/bin:$PATH" GH_HEAD="$GH" GH_BRANCH="$_br" FAKE_PR="$_pr" sh "$G/.claude/hooks/pr-merge-guard.sh" 2>"$d/err")) || rc=$?
  expect_rc "$_w" "$rc" "guard rule: $( [ "$_w" = 2 ] && echo blocks || echo allows ) $_l"
  [ "$rc" = "$_w" ] || sed 's/^/       /' "$d/err" | head -4
}
merge 2 "a fix/ PR with no receipt" "Fixes the closed card."
has yes "fixes #239 with no premise-check receipt" "$(cat "$d/err")" "guard rule: ...naming the issue that lacks one"
PC=.claude/modules/premise-check/premise-check.sh   # the script, as a reader types it at the root
has yes "Run:  sh $PC 239" "$(cat "$d/err")" "guard rule: ...and the command that produces it, as typed at the root"
merge 0 "a fix/ PR with a receipt for its issue" "Fixes it.

premise-check: #239 @ $(printf '%s' "$GH" | cut -c1-12)"
has yes "premise check present for every issue PR #290 fixes: #239" "$(cat "$d/err")$out" "guard rule: ...and says so as an advisory"
merge 2 "a PR whose title names the issues and nothing closes them" "No closing keyword." "" "Fix #151/#152: window lifecycle"
has yes "fixes #151 #152 with no premise-check receipt" "$(cat "$d/err")" "guard rule: ...the title's issues need receipts"
merge 2 "a PR whose body closes an issue GitHub's field misses" "Closes #409.

Why: it." "" "Keep live sessions' worktrees"
has yes "fixes #409 with no premise-check receipt" "$(cat "$d/err")" "guard rule: ...the body's closing keywords count"
merge 2 "a PR with a receipt for one of two issues (the union)" "Closes #409. Fixes #410.

premise-check: #409 @ $(printf '%s' "$GH" | cut -c1-12)" "409"
has yes "fixes #410 with no premise-check receipt" "$(cat "$d/err")" "guard rule: ...names the one without"
merge 0 "a fix/ PR that closes and names no issue" "Tidy." "" "Tidy a comment"
has yes "closes no issue and names none in its title" "$(cat "$d/err")$out" "guard rule: ...and says there was no premise to check"
merge 0 "an ops/ PR (not PREMISE_REQUIRED_ON)" "Fixes #12, no receipt." "12" "Tidy" "ops/tidy"
printf 'MODULES="premise-check"\nPREMISE_REQUIRED_ON="fix/ hotfix/"\n' > "$G/.claude/project.conf"
merge 2 "a hotfix/ PR once PREMISE_REQUIRED_ON names hotfix/" "Fixes #12." "12" "Hot" "hotfix/12"
# Unreadable PR: gh answers nothing for its body and closing issues.
rc=0
out=$(payload "gh pr merge 290 --squash" "$G" | (cd "$G" && env PATH="$d/bin:$PATH" GH_HEAD="$GH" GH_BRANCH=fix/1 FAKE_PR= sh "$G/.claude/hooks/pr-merge-guard.sh" 2>"$d/err")) || rc=$?
expect_rc 2 "$rc" "guard rule: blocks when it cannot read the PR (fails closed, not open)"
has yes "could not read PR #290" "$(cat "$d/err")" "guard rule: ...and says it could not read the PR"
printf 'MODULES=""\n' > "$G/.claude/project.conf"
merge 0 "a fix/ PR with no receipt while the module is OFF (the rule is the module's)" "Fixes the closed card."

# --- enable.sh: the panel's fix lane --------------------------------------------------------------
E="$d/proj"; new_repo "$E"; mkdir -p "$E/.claude/modules" "$E/.clauductor"
cp -R "$M" "$E/.claude/modules/premise-check"
jq -n '{templates: [{id: "fix", lane_type: "fix", first_prompt: "Fix GitHub issue {issue}."}, {id: "ops", lane_type: "ops", first_prompt: "Ops."}]}' > "$E/.clauductor/panel.json"
out=$(cd "$E" && sh .claude/modules/premise-check/enable.sh --check 2>&1); rc=$?
[ "$rc" = 1 ] && has yes "FAIL the fix lane template(s) fix" "$out" "enable.sh --check: a fix lane without the check fails" || fail "enable.sh --check: a fix lane without the check fails: it passed: $out"
(cd "$E" && sh .claude/modules/premise-check/enable.sh >/dev/null 2>&1)
p=$(jq -r '.templates[0].first_prompt' "$E/.clauductor/panel.json")
has yes "Fix GitHub issue {issue}. Before fixing, run the premise check, sh $PC {issue}" "$p" "enable.sh: the fix lane's prompt runs the check first, with the command as typed at the root"
has no "premise" "$(jq -r '.templates[1].first_prompt' "$E/.clauductor/panel.json")" "enable.sh: ...and no other lane's prompt changes"
(cd "$E" && sh .claude/modules/premise-check/enable.sh >/dev/null 2>&1)
[ "$(jq -r '.templates[0].first_prompt' "$E/.clauductor/panel.json")" = "$p" ] && ok "enable.sh: a second run changes nothing" || fail "enable.sh: a second run changes nothing: it added the sentence again"
out=$(cd "$E" && sh .claude/modules/premise-check/enable.sh --check 2>&1) && has yes "ok   every fix lane" "$out" "enable.sh --check: passes once the fix lane runs it" || fail "enable.sh --check: passes once the fix lane runs it: it did not: $out"
finish
