#!/bin/sh
# fold.sh: session-close's fold turns the row fragments on main into one `## Session N` entry, moves
# their insight rows to the log, sets each row's Status, and removes the fragments; and it changes
# NOTHING when any part cannot be done (a malformed fragment, a log with no table, a taken number).
# Also: session-close's SKILL.md runs it (the caller, rule 3).
. "$(dirname "$0")/lib.sh"

S=$(scratch)
setup() {  # a fresh fixture project, a git repo with two good fragments
  F="$S/proj"; rm -rf "$F"; mkdir -p "$F/.claude/lib" "$F/.claude/skills/session-close" "$F/docs/journal.d"
  cp "$ROOT/.claude/lib/conf.sh" "$ROOT/.claude/lib/fragments.sh" "$F/.claude/lib/" 2>/dev/null
  cp "$ROOT/.claude/roadmap-queue.sh" "$F/.claude/" 2>/dev/null
  cp "$ROOT/.claude/skills/session-close/fold.sh" "$F/.claude/skills/session-close/" 2>/dev/null
  cat > "$F/docs/roadmap.md" <<'EOF'
## Phase 1 — Now
| # | Change | Scope | Deps | Status |
|---|---|---|---|---|
| A-1 | `ops/a-one` — the first row | s | — | ⬜ queued |
| A-2 | `ops/a-two` — the second row | s | — | ⬜ in flight (#12) |
| A-3 | `ops/a-three` — not folded | s | — | ⬜ queued |
EOF
  # The shipped journal's shape: a fenced format example above the first real entry.
  printf '# Journal\n\nEach entry:\n\n```\n## Session N — YYYY-MM-DD — <author> — <short focus>\n```\n\n<!-- Newest session goes below this line. -->\n\n## Session 4 — 2026-01-01 — Ann — before\n\nOld.\n' > "$F/docs/development-journal.md"
  printf '# Insights\n\n## Log\n\n| Date | Area | Topic | Observation | How to verify | Status |\n|---|---|---|---|---|---|\n| 2026-01-01 | Ops | old | o | v | Raw |\n' > "$F/docs/insights-log.md"
  frag A-1 11 '$1.50' '| 2026-01-02 | Ops | new one | o | v | Raw |'
  frag A-2 12 '$2.25' 'None.'
  (cd "$F" && git init -q && git config user.name "Ann Example" && git config user.email a@example.com \
    && git add -A && git commit -qm init) >/dev/null 2>&1
}
frag() {  # frag ID PR COST INSIGHTS-LINE
  cat > "$F/docs/journal.d/$1.md" <<EOF
# $1 — the $1 work

- **PR**: #$2
- **Session**: S5, started 2026-01-02 09:00 ET
- **Review**: 2 rounds, peak low, cap 3
- **Gate**: 7 min
- **Cost**: $3
- **Session start**: 1000 tokens, 3 min

## What happened

Did $1.

## Decisions

Chose.

## Surprises

None.

## Insights

$4
EOF
}
fold() { (cd "$F" && sh .claude/skills/session-close/fold.sh "$@" 2>&1); }
snap() { (cd "$F" && cat docs/development-journal.md docs/insights-log.md docs/roadmap.md; ls docs/journal.d) | cksum; }

setup
out=$(fold 5 --focus "two rows"); rc=$?
J="$F/docs/development-journal.md"
[ $rc -eq 0 ] && ok "fold: folds two fragments (exit 0)" || fail "fold: exit $rc: $out"
grep -m1 '^## Session [0-9]' "$J" | grep -qx '## Session 5 — [0-9-]* — Ann — two rows' \
  && [ "$(grep -n '^## Session 5 ' "$J" | cut -d: -f1)" -lt "$(grep -n '^## Session 4 ' "$J" | cut -d: -f1)" ] \
  && ok "fold: the entry heads the entries, the newest first, as Session 5" || fail "fold: journal is: $(grep -n '^## Session' "$J")"
[ "$(sed -n '5,7p' "$J" | cksum)" = "$(printf '```\n## Session N — YYYY-MM-DD — <author> — <short focus>\n```\n' | cksum)" ] \
  && [ "$(grep -n '^## Session 5 ' "$J" | cut -d: -f1)" -gt 9 ] \
  && ok "fold: the fenced format example stays whole, and the entry goes below it" || fail "fold: the head of the journal is: $(sed -n '1,12p' "$J")"
grep -q '^### A-1 — the A-1 work' "$J" && grep -q '^#### What happened' "$J" \
  && ok "fold: each fragment is a ### block with #### sections" || fail "fold: fragment blocks missing in the entry"
at() { grep -n -- "$1" "$J" | head -1 | cut -d: -f1; }
[ "$(at '^### A-1 ')" -lt "$(at '^- \*\*PR\*\*: #11')" ] && [ "$(at '^- \*\*PR\*\*: #11')" -lt "$(at '^#### What happened')" ] \
  && [ "$(at '^#### Surprises')" -lt "$(at '^#### Insights')" ] \
  && ok "fold: a block keeps its fragment's order: fields, the sections, Insights last" \
  || fail "fold: block order: $(awk '/^### A-1 /{on=1} /^### A-2 /{on=0} on' "$J" | grep -n '^###\|^- \*\*PR\|^#### ')"
grep -q '^\*\*Totals\.\*\* Gate 14 min; cost \$3.75\.' "$J" && ok "fold: totals add the gates and costs" || fail "fold: totals wrong: $(grep Totals "$J")"
grep -q '^| 2026-01-02 | Ops | new one' "$J" && fail "fold: an insight row stayed in the journal" || ok "fold: insight rows leave the entry"
sed -n 7p "$F/docs/insights-log.md" | grep -q 'new one' \
  && ok "fold: the insight row goes to the top of the log's table" || fail "fold: log line 7 is: $(sed -n 7p "$F/docs/insights-log.md")"
grep -q '^| A-1 .*| ✅ merged (#11) |$' "$F/docs/roadmap.md" && grep -q '^| A-2 .*| ✅ merged (#12) |$' "$F/docs/roadmap.md" \
  && grep -q '^| A-3 .*| ⬜ queued |$' "$F/docs/roadmap.md" \
  && ok "fold: each folded row's Status is merged, the others untouched" || fail "fold: roadmap: $(grep '^| A-' "$F/docs/roadmap.md")"
[ -z "$(ls "$F/docs/journal.d" 2>/dev/null)" ] && ok "fold: the fragments are removed" || fail "fold: fragments left: $(ls "$F/docs/journal.d")"
(ROOT=$F; . "$F/.claude/lib/conf.sh"; roadmap_queue --check >/dev/null 2>&1) && ok "fold: the roadmap still parses" || fail "fold: the roadmap no longer parses"
out=$(fold 6); [ $? -eq 0 ] && printf '%s' "$out" | grep -q 'nothing to fold' \
  && ok "fold: no fragment, nothing to fold (exit 0)" || fail "fold: with no fragment: $out"

# A batched PR (OPS-125): its fragment's **Also closes** rows turn merged by the same PR.
setup
awk '{ print } /^- \*\*Session start\*\*:/ { print "- **Also closes**: A-3" }' "$F/docs/journal.d/A-1.md" > "$S/a1" && mv "$S/a1" "$F/docs/journal.d/A-1.md"
out=$(fold 5); rc=$?
[ $rc -eq 0 ] && grep -q '^| A-3 .*| ✅ merged (#11) |$' "$F/docs/roadmap.md" && grep -q '^| A-1 .*| ✅ merged (#11) |$' "$F/docs/roadmap.md" \
  && grep -q '^\*\*Rows folded\.\*\* A-1 (#11, also closes A-3); A-2 (#12)\.' "$J" \
  && ok "fold: a batched PR's Also closes row is merged by its PR, and the entry says so" \
  || fail "fold: Also closes: exit $rc: $out; roadmap: $(grep '^| A-' "$F/docs/roadmap.md" | tr '\n' '~'); $(grep 'Rows folded' "$J")"

# A fragment quoting markdown in a fence keeps it whole; a gate that could not be read is no 0.
setup
awk '{ print } /^Chose\./ { print ""; print "```"; print "## Insights"; print "| 2026-01-09 | Ops | quoted | o | v | Raw |"; print "```" }' \
  "$F/docs/journal.d/A-1.md" > "$S/a1" && mv "$S/a1" "$F/docs/journal.d/A-1.md"
awk '/^- \*\*Gate\*\*:/ { print "- **Gate**: CANNOT CHECK — no receipt"; next } { print }' \
  "$F/docs/journal.d/A-2.md" > "$S/a2" && mv "$S/a2" "$F/docs/journal.d/A-2.md"
out=$(fold 5); rc=$?
[ $rc -eq 0 ] && [ "$(awk '/^Chose\./ { on = 1; next } on && NF { print; n++ } n == 4 { exit }' "$J" | cksum)" = "$(printf '```\n## Insights\n| 2026-01-09 | Ops | quoted | o | v | Raw |\n```\n' | cksum)" ] \
  && ! grep -q 'quoted' "$F/docs/insights-log.md" && grep -q '^1 row(s), moved' "$J" \
  && ok "fold: a fenced block in a fragment is copied whole, its quoted row not moved to the log" \
  || fail "fold: fenced block: exit $rc: $out; entry: $(awk '/^### A-1 /{on=1} /^### A-2 /{on=0} on' "$J" | tr '\n' '~')"
grep -q '^\*\*Totals\.\*\* Gate 7 min (a row.s gate was CANNOT CHECK, so this is a floor); cost \$3.75\.' "$J" \
  && ok "fold: a CANNOT CHECK gate makes the total a floor, said so, never a silent 0" || fail "fold: totals: $(grep Totals "$J")"

# All or nothing: each refusal leaves every file as it was.
refuses() {  # refuses WORDS LABEL ARGS…
  w=$1; l=$2; shift 2; before=$(snap)
  if out=$(fold "$@"); then fail "fold: $l folded; want a refusal"
  elif ! printf '%s' "$out" | grep -qF -- "$w"; then fail "fold: $l refused, not for '$w': $out"
  elif [ "$(snap)" != "$before" ]; then fail "fold: $l refused but changed a file"
  else ok "fold: $l refused, nothing changed ($w)"; fi
}
setup; printf 'stray\n' >> "$F/docs/journal.d/A-2.md"; sed -i.b '/^- \*\*Gate/d' "$F/docs/journal.d/A-2.md"; rm -f "$F/docs/journal.d/A-2.md.b"
refuses "malformed fragment" "a malformed fragment" 5
setup; awk '{ print } /^- \*\*Session start\*\*:/ { print "- **Also closes**: A-3 and A-2" }' "$F/docs/journal.d/A-1.md" > "$S/a1" && mv "$S/a1" "$F/docs/journal.d/A-1.md"
refuses "Also closes" "an Also closes that is not row ids" 5
# Two fragments closing one row (A-1 also closes A-2, which has its own) would set its Status twice.
setup; awk '{ print } /^- \*\*Session start\*\*:/ { print "- **Also closes**: A-2" }' "$F/docs/journal.d/A-1.md" > "$S/a1" && mv "$S/a1" "$F/docs/journal.d/A-1.md"
refuses "closed by two fragments" "a row two fragments close" 5
setup; refuses "already has a Session 4" "a number the journal holds" 4
setup; printf '# Insights\n\nNo table.\n' > "$F/docs/insights-log.md"
refuses "no '| Date" "a log with no table" 5
setup; refuses "usage" "no session number" --focus x
setup; refuses "needs the focus" "--focus with nothing after it" 5 --focus

# A dry run changes nothing and counts each thing once.
setup; frag A-1 11 '$1.50' 'None.'; before=$(snap)
out=$(fold 5 --dry-run)
printf '%s\n' "$out" | head -1 | grep -qx 'fold (dry run): Session 5 from 2 fragment(s); 0 insight row(s)' && [ "$(snap)" = "$before" ] \
  && ok "fold: a dry run with no insight rows says 0 once and changes nothing" || fail "fold: dry run said: $(printf '%s\n' "$out" | head -2)"

# The caller: a fold no close runs is not shipped (rule 3).
grep -q 'session-close/fold\.sh' "$ROOT/.claude/skills/session-close/SKILL.md" 2>/dev/null \
  && ok "session-close's SKILL.md runs the fold" || fail "session-close's SKILL.md does not run .claude/skills/session-close/fold.sh"
finish
