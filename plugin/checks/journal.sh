#!/bin/sh
STRIDAL_FW=$(cd "$(dirname "$0")/.." && pwd) # stridal plugin: the plugin root (framework/internal/plugin)
# Journal session numbers: the collision rule pr-merge-guard applies at merge
# (.claude/hooks/lib/journal-sessions.sh), in both directions; and the project's journal carries
# no duplicate number, and every session heading added since adoption has the configured shape
# (JOURNAL_HEADING). A heading written before RECORDS_BASELINE is history: accepted as it is, never
# reformatted (.claude/lib/records.sh). Falsified on fixtures shaped like an adopting project's
# journal: legacy headings with no author and with date ranges, then current ones.
. "$(dirname "$0")/lib.sh"
. "$STRIDAL_FW/hooks/lib/journal-sessions.sh"
. "$STRIDAL_FW/lib/records.sh"
. "$STRIDAL_FW/lib/fragments.sh"

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
  cp -R "$STRIDAL_FW/lib" "$F/.claude/"; cp -R "$STRIDAL_FW/hooks/lib" "$F/.claude/hooks/"
  cp "$STRIDAL_FW/checks/lib.sh" "$STRIDAL_FW/checks/journal.sh" "$F/.claude/checks/"
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

# ── Self-test: a row's fragment (lib/fragments.sh), good and broken one way at a time ────────────
FD="$(scratch)/frag"; mkdir -p "$FD"
good() {  # good ID: a fragment of the full shape, to stdout
  cat <<G
# $1 — a sample row

- **PR**: #12
- **Session**: S10, started 2026-10-23 09:15 ET
- **Review**: 2 rounds, peak medium, cap 3
- **Gate**: 14 min
- **Cost**: \$3.10
- **Session start**: 48000 tokens, 2 min

## What happened

The row's narrative.

## Decisions

None.

## Surprises

None.

## Insights

| 2026-10-23 | Process | a/b | An observation. | How to verify it. | Raw |
G
}
fgood() { if got=$(fragment_problems "$FD/$1.md"); then ok "fragment: $2 passes"; else fail "fragment: $2 FAILED: $(echo $got)"; fi; }
fbad() {  # fbad ID LABEL WORDS: it must fail, and for the reason WORDS
  if got=$(fragment_problems "$FD/$1.md"); then fail "fragment: $2 passed"
  elif printf '%s\n' "$got" | grep -qF -- "$3"; then ok "fragment: $2 fails ($3)"
  else fail "fragment: $2 fails, but not for '$3': $(echo $got)"; fi
}
good OPS-1 > "$FD/OPS-1.md"; fgood OPS-1 "the full shape"
good OPS-2 | sed 's/^| 2026.*/None./' > "$FD/OPS-2.md"; fgood OPS-2 "no insights (None.)"
good OPS-9 > "$FD/OPS-3.md"; fbad OPS-3 "a title naming another row" 'line 1 is not "# OPS-3'
good OPS-4 | sed '/\*\*Gate\*\*/d' > "$FD/OPS-4.md"; fbad OPS-4 "a missing field" "no **Gate** field"
good OPS-5 | sed 's/started 2026-10-23 09:15 ET/started this morning/' > "$FD/OPS-5.md"; fbad OPS-5 "a session without its start time" "**Session** is not"
good OPS-6 | sed '/^## Surprises/,/^## Insights/{/^## Insights/!d;}' > "$FD/OPS-6.md"; fbad OPS-6 "a missing section" 'no "## Surprises" section'
good OPS-7 | sed 's/^## What happened/## Session 11 — 2026-10-23 — a — x/' > "$FD/OPS-7.md"; fbad OPS-7 "a session heading in a fragment" "only a session close numbers a session"
# A field twice: the fold reads the first **Also closes**, so a second would go unread unchecked.
good OPS-7 | awk '{ print } /^- \*\*Session start\*\*:/ { print "- **Also closes**: OPS-8"; print "- **Also closes**: OPS-9" }' > "$FD/OPS-7.md"; fbad OPS-7 "a field given twice" "appears twice"
good OPS-8 | sed 's/ Raw |$/ Raw | extra |/' > "$FD/OPS-8.md"; fbad OPS-8 "an insight row of seven cells" "needs six cells"
good OPS-10 > "$FD/OPS-10.md"
printf '| Date | Area | Topic | Observation | How to verify | Status |\n' >> "$FD/OPS-10.md"; fbad OPS-10 "a table header under Insights" "no table header"
good OPS-11 | sed '/^| 2026/d' > "$FD/OPS-11.md"; fbad OPS-11 "an empty Insights section" 'neither an insight row nor "None."'
good 'two words' > "$FD/two words.md"; fbad 'two words' "a file name that is not a row id" "is not <row id>.md"
good OPS-12 | awk '{ print } /^The row.s narrative\./ { print ""; print "```"; print "## What happened"; print "- **PR**: quoted"; print "```" }' > "$FD/OPS-12.md"
fgood OPS-12 "a fenced block quoting a section heading and a field"
good OPS-13 | sed 's/An observation\./a \\| b, escaped as the log writes it/' > "$FD/OPS-13.md"; fgood OPS-13 "an insight row with an escaped \\| in a cell"
good OPS-14 | sed 's/peak medium/peak minor/' > "$FD/OPS-14.md"; fbad OPS-14 "a peak that is not a severity" "**Review** is not"
good OPS-15 > "$FD/OPS-15.md"; printf '\nThe second one was about the gate lease.\n' >> "$FD/OPS-15.md"
fbad OPS-15 "prose under Insights beside a row (the fold would drop it)" "holds only insight rows"
good OPS-16 > "$FD/OPS-16.md"; printf '\n```\n| 2026-10-23 | a | b | c | d | Raw |\n```\n' >> "$FD/OPS-16.md"
fbad OPS-16 "a fenced block under Insights (the fold would drop it)" "holds only insight rows"
good OPS-17 | sed 's/^- \*\*Cost\*\*: .*/- **Cost**: 4.20/' > "$FD/OPS-17.md"; fbad OPS-17 "a cost with no \$" "**Cost** is not"
good OPS-18 | sed 's/^- \*\*Session start\*\*: .*/- **Session start**: about an hour/' > "$FD/OPS-18.md"; fbad OPS-18 "a session start that is not the marker's" "**Session start** is not"
good OPS-19 | sed 's/^- \*\*Cost\*\*: .*/- **Cost**: CANNOT CHECK — no records/' > "$FD/OPS-19.md"; fgood OPS-19 "a cost that could not be read, said so"
# A row two fragments close (an **Also closes** naming a row with its own, or two naming one) is a
# fold refusal at the close (OPS-125): seen here, at the row's gate, the row's author fixes it.
# Only when the branch holds both: two PRs cut from one main each see one, and the fold catches that.
FT="$(scratch)/twice"; mkdir -p "$FT"
good A-1 | awk '{ print } /^- \*\*Session start\*\*:/ { print "- **Also closes**: A-2" }' > "$FT/A-1.md"; good A-2 > "$FT/A-2.md"
tw=$(printf '%s\n' "$FT/A-1.md" "$FT/A-2.md" | fragments_twice 2>&1)
[ "$tw" = A-2 ] && ok "fragments_twice: a row its own fragment and another's Also closes both close" || fail "fragments_twice found '$tw', want A-2"
good A-1 > "$FT/A-1.md"
tw=$(printf '%s\n' "$FT/A-1.md" "$FT/A-2.md" | fragments_twice 2>&1)
[ -z "$tw" ] && ok "fragments_twice: none when each row is closed once" || fail "fragments_twice found '$tw' in two separate rows"
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

# ── This project's fragments: each row's records in the shape the close folds ───────────────────
nfr=0
fragment_files > "$(scratch)/fragments"
while IFS= read -r f; do
  nfr=$((nfr + 1))
  if got=$(fragment_problems "$f"); then ok "fragment ${f#"$ROOT"/} has the shape session-close folds"
  else fail "fragment ${f#"$ROOT"/} (docs/journal.d/README.md has the shape): $(printf '%s\n' "$got" | head -5 | tr '\n' ';')"; fi
done < "$(scratch)/fragments"
[ "$nfr" -gt 0 ] || ok "no row fragment waits in $JOURNAL_FRAGMENTS for a close"
tw=$(fragments_twice < "$(scratch)/fragments" | paste -sd ' ' -)
[ -z "$tw" ] && ok "no row is closed by two of this project's fragments" || fail "row(s) $tw are closed by two fragments (the fold would refuse): keep one **Also closes** or fragment for each"
finish
