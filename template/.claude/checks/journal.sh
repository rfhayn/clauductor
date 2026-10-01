#!/bin/sh
# Journal session numbers: the collision rule pr-merge-guard applies at merge
# (.claude/hooks/lib/journal-sessions.sh), in both directions; and the project's journal carries
# no duplicate number, and every session heading added since adoption has the configured shape
# (JOURNAL_HEADING). A heading written before RECORDS_BASELINE is history: accepted as it is, never
# reformatted (.claude/lib/records.sh). Falsified on fixtures shaped like an adopting project's
# journal: legacy headings with no author and with date ranges, then current ones.
. "$(dirname "$0")/lib.sh"
. "$ROOT/.claude/hooks/lib/journal-sessions.sh"
. "$ROOT/.claude/lib/records.sh"

if [ -z "${JOURNAL_SELFTEST:-}" ]; then
h() { printf '## Session %s — 2026-01-0%s — a — x\n' "$@"; }
col() {  # col WANT LABEL BASE HEAD MAIN
  got=$(journal_session_collisions "$3" "$4" "$5"); rc=$?
  if [ "$1" = none ]; then
    [ "$rc" -eq 0 ] && ok "allows: $2" || fail "blocked $2 (collides: $got)"
  else
    [ "$rc" -eq 1 ] && [ "$got" = "$1" ] && ok "blocks: $2 ($got)" || fail "$2: want collision $1, got '${got:-none}'"
  fi
}
B="$(h 1 1)"
col none "a new next number" "$B" "$(h 2 2)
$B" "$B"
col 2 "the other person merged 2 after this branch was cut" "$B" "$(h 2 2)
$B" "$(h 2 3)
$B"
col 2 "keep-both: two entries numbered 2" "$B" "$(h 2 2)
$(h 2 3)
$B" "$B"
col none "a retitled old entry" "$B" "## Session 1 — 2026-01-01 — a — retitled" "$B"
col none "this branch's own heading already squash-merged" "$B" "$(h 2 2)
$B" "$(h 2 2)
$B"
col none "a sub-numbered session (2 and 2.1 differ)" "$B" "## Session 2.1 — x
$B" "$(h 2 2)
$B"
col none "a duplicate main already carried" "$(h 1 1)
$(h 1 1)" "$(h 1 1)
$(h 1 1)" "$(h 1 1)
$(h 1 1)"

# ── Self-test: the project-journal judgement, on fixtures ────────────────────────────────────────
mkj() {  # mkj NAME: a fixture project whose journal has three legacy and two current headings
  F="$(scratch)/j-$1"; mkdir -p "$F/.claude/checks" "$F/.claude/hooks" "$F/docs"
  cp -R "$ROOT/.claude/lib" "$F/.claude/"; cp -R "$ROOT/.claude/hooks/lib" "$F/.claude/hooks/"
  cp "$ROOT/.claude/checks/lib.sh" "$ROOT/.claude/checks/journal.sh" "$F/.claude/checks/"
  cat > "$F/docs/development-journal.md" <<'J'
# Development journal

Each entry starts with `## Session N — YYYY-MM-DD — <author> — <short focus>`.

## Session 4 — 2026-09-28/29 — Rich — **The proposal merged; the review page approved**

## Session 3 — 2026-09-20 — Rich — **Platform support built**

## Session 2 — 2026-07-27/28 — The roadmap's exit was unreachable from its own queue; fixed into three gates

## Session 1 — 2026-07-11 — Phase 0 planning: sequencing and monorepo decisions locked

## Session 0 — 2026-07-11 — Repo founded as the planning home
J
}
jrun() { (ROOT="$F" JOURNAL_SELFTEST=1 sh "$F/.claude/checks/journal.sh") > "$(scratch)/j.out" 2>&1; }
jok() { if jrun; then ok "journal: $1 passes"; else fail "journal: $1 FAILED:"; grep '^FAIL' "$(scratch)/j.out" | sed 's/^/       /'; fi; }
jbad() {  # jbad LABEL WORDS: it must fail, and for the reason WORDS (a fixed string in a FAIL line)
  if jrun; then fail "journal: $1 passed"
  elif grep '^FAIL' "$(scratch)/j.out" | grep -qF -- "$2"; then ok "journal: $1 fails ($2)"
  else fail "journal: $1 fails, but not for '$2':"; grep '^FAIL' "$(scratch)/j.out" | sed 's/^/       /'; fi
}
newer() {  # newer HEADING: add a session entry on top, as /dev-journal does
  awk -v l="$1" '!done && /^## Session / { print l; print ""; done = 1 } { print }' "$F/docs/development-journal.md" > "$F/j.new"
  mv "$F/j.new" "$F/docs/development-journal.md"
}
conf() { printf '%s\n' "$@" > "$F/.claude/project.conf"; }

mkj none; jbad "legacy headings (no author) with no RECORDS_BASELINE" "Session 2 — 2026-07-27/28"
mkj date; conf 'RECORDS_BASELINE="2026-09-01"'
jok "legacy headings before a date RECORDS_BASELINE, current ones after it"
newer '## Session 5 — 2026-09-30 — focus without an author'; jbad "a NEW heading (after the date baseline) with no author" "Session 5 — 2026-09-30"
mkj daterange; conf 'RECORDS_BASELINE="2026-09-21"'
jok "a date-range heading after the baseline (2026-09-28/29) is judged, by its first day"
mkj dup; conf 'RECORDS_BASELINE="2026-09-01"'
newer '## Session 3 — 2026-09-30 — Rich — a second three'; jbad "a duplicate session number, legacy or not" "duplicate sessions: 3"
mkj ref; new_repo "$F"
grep -v '^## Session [34] ' "$F/docs/development-journal.md" > "$F/j.new"; mv "$F/j.new" "$F/docs/development-journal.md"
git -C "$F" add -A >/dev/null 2>&1; git -C "$F" commit -qm adopt >/dev/null 2>&1; git -C "$F" tag adoption
conf 'RECORDS_BASELINE="adoption"'
newer '## Session 3 — 2026-09-20 — Rich — **Platform support built**'
jok "legacy sessions that exist at a ref RECORDS_BASELINE (by number), new ones after it"
newer '## Session 4 — 2026-09-30 — no author here'; jbad "a NEW session (absent at the ref baseline) with no author" "Session 4 — 2026-09-30"
mkj badref; conf 'RECORDS_BASELINE="no-such-ref"'
jbad "a RECORDS_BASELINE ref this clone cannot resolve" "neither a YYYY-MM-DD date nor a commit"
mkj custom
sed 's/^## Session \([0-9]*\) — \([0-9-]*\)[^ ]* — Rich — /## Session \1 | \2 | /' "$F/docs/development-journal.md" > "$F/j.new"; mv "$F/j.new" "$F/docs/development-journal.md"
conf 'RECORDS_BASELINE="2026-09-01"'
jbad "a project's own heading shape against the default JOURNAL_HEADING" "Session 4 | 2026-09-28"
conf 'RECORDS_BASELINE="2026-09-01"' "JOURNAL_HEADING='^## Session [0-9]+ [|] [0-9]{4}-[0-9]{2}-[0-9]{2} [|] .+'"
jok "a project's own heading shape under its own JOURNAL_HEADING"
fi

# ── This project's journal ───────────────────────────────────────────────────────────────────────
j="$ROOT/$JOURNAL"
[ -f "$j" ] || { fail "JOURNAL $JOURNAL does not exist"; finish; }
d=$(grep -E '^## Session [0-9]' "$j" | sed -n 's/^## Session \([0-9.]*\).*/\1/p' | sort | uniq -d)
[ -z "$d" ] && ok "the journal ($JOURNAL) has no duplicate session number" || fail "$JOURNAL has duplicate sessions: $(echo $d)"
kind=$(records_baseline_kind)
if [ "$kind" = bad ]; then fail "$(records_baseline_bad)"; finish; fi
journal_legacy_sessions > "$(scratch)/legacy-sessions"
# Numbered headings only: the file's own preamble shows the shape as `## Session N — …`. awk sorts
# history from new (`L` / `N <line>:<heading>`); grep -E matches the pattern, because an ERE with
# intervals ({4}) is not one every awk reads (mawk, Ubuntu's default, may not).
grep -nE '^## Session [0-9]' "$j" | awk -v kind="$kind" -v base="$RECORDS_BASELINE" -v lf="$(scratch)/legacy-sessions" '
  BEGIN { while ((getline l < lf) > 0) legacy[l] = 1; gsub(/-/, "", base) }
  {
    line = $0; sub(/^[0-9]+:/, "", line)
    n = line; sub(/^## Session /, "", n); sub(/[^0-9.].*$/, "", n)
    old = 0
    if (kind == "ref" && (n in legacy)) old = 1
    if (kind == "date" && match(line, /[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]/)) { dt = substr(line, RSTART, 10); gsub(/-/, "", dt); if (dt + 0 < base + 0) old = 1 }
    print (old ? "L" : "N " $0)
  }' > "$(scratch)/j-judged"
nl=$(grep -c '^L' "$(scratch)/j-judged"); nn=$(grep -c '^N ' "$(scratch)/j-judged")
sed -n 's/^N [0-9]*://p' "$(scratch)/j-judged" | { grep -vE -- "$JOURNAL_HEADING" || true; } > "$(scratch)/j-bad"
nb=$(grep -c . "$(scratch)/j-bad"); bad=$(head -3 "$(scratch)/j-bad" | tr '\n' ' ')
[ "$kind" = none ] || ok "$nl session heading(s) before RECORDS_BASELINE ($RECORDS_BASELINE) kept as written"
if [ "$nb" -eq 0 ]; then ok "every session heading since adoption ($nn) matches JOURNAL_HEADING"
else fail "malformed session headings ($nb of $nn since adoption; JOURNAL_HEADING): $bad"; fi
finish
