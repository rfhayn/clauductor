#!/bin/sh
STRIDAL_FW=$(cd "$(dirname "$0")/.." && pwd) # stridal plugin: the plugin root (framework/internal/plugin)
# A record number is never taken twice, every record file has a row in its directory's index
# (README.md), and every index row links a file that exists. The set is the directory (the
# authority), not the index: a record the index forgot is the failure this exists to catch.
#
# Two series share ONE number space: ADRs (ADR_DIR, decisions about the system) and practice
# records (PRACTICES_DIR, PRAC-NNNN: how the project works). One number across both lets a citation
# name a record by its digits alone. PRACTICES_DIR is optional: a project with no practice records
# has no such directory.
#
# Each record added since adoption opens with '# ADR NNNN: <title>' (or '# PRAC NNNN: <title>') and
# has an '## Enforcement' section. A record written before RECORDS_BASELINE (.claude/lib/records.sh)
# is history, never reformatted: its heading may be any '# ADR NNNN' / '# ADR-NNNN' form and it may
# lack the section. Falsified on fixtures shaped like an adopting project's ADR history.
. "$(dirname "$0")/lib.sh"
. "$STRIDAL_FW/lib/records.sh"

# ── Self-test ─────────────────────────────────────────────────────────────────────────────────────
if [ -z "${ADR_SELFTEST:-}" ]; then
  mka() {  # mka NAME: a fixture project with three legacy ADRs (two heading forms, no Enforcement)
    F="$(scratch)/adr-$1"; A="$F/docs/adr"; mkdir -p "$F/.claude/checks" "$A"
    cp -R "$STRIDAL_FW/lib" "$F/.claude/"; cp "$STRIDAL_FW/checks/lib.sh" "$STRIDAL_FW/checks/adr-numbering.sh" "$F/.claude/checks/"
    printf '# ADR NNNN: <Title>\n' > "$A/TEMPLATE.md"
    printf '# ADR 0001: The product is multi-tenant\n\n- **Status**: Accepted\n- **Date**: 2026-07-11\n\n## Decision\nx\n' > "$A/0001-multi-tenant.md"
    printf '# ADR-0002 — A passing test is evidence of nothing until it has failed\n\n- **Status**: Accepted\n- **Date**: 2026-08-02\n\n## Decision\nx\n' > "$A/0002-passing-test.md"
    printf '# ADR-0003 — A spec seeds the world it asserts on, and a claim about another is on trust\n\n- **Status:** Accepted\n- **Date:** 2026-09-10\n\n## Decision\nx\n\n## Enforcement\nA test.\n' > "$A/0003-spec-seeds.md"
    printf '# ADRs\n\n| ADR | Title | Status |\n|---|---|---|\n| [0001](0001-multi-tenant.md) | t | Accepted |\n| [0002](0002-passing-test.md) | t | Accepted |\n| [0003](0003-spec-seeds.md) | t | Accepted |\n' > "$A/README.md"
  }
  add_adr() {  # add_adr HEADING ENFORCEMENT(yes|no): ADR 0004, dated 2026-09-29, indexed
    { printf '%s\n\n- **Status**: Accepted\n- **Date**: 2026-09-29\n\n## Decision\nx\n' "$1"; [ "$2" = yes ] && printf '\n## Enforcement\nchecks/x.sh\n'; } > "$A/0004-new.md"
    printf '| [0004](0004-new.md) | t | Accepted |\n' >> "$A/README.md"
  }
  arun() { (ROOT="$F" ADR_SELFTEST=1 sh "$F/.claude/checks/adr-numbering.sh") > "$(scratch)/a.out" 2>&1; }
  aok() { if arun; then ok "adr: $1 passes"; else fail "adr: $1 FAILED:"; grep '^FAIL' "$(scratch)/a.out" | sed 's/^/       /'; fi; }
  abad() {
    if arun; then fail "adr: $1 passed"
    elif grep '^FAIL' "$(scratch)/a.out" | grep -qF -- "$2"; then ok "adr: $1 fails ($2)"
    else fail "adr: $1 fails, but not for '$2':"; grep '^FAIL' "$(scratch)/a.out" | sed 's/^/       /'; fi
  }
  conf() { printf '%s\n' "$@" > "$F/.claude/project.conf"; }

  mka none; abad "legacy ADR headings and no Enforcement, with no RECORDS_BASELINE" "0002-passing-test.md: first line must be"
  mka date; conf 'RECORDS_BASELINE="2026-09-15"'; aok "legacy ADRs dated before a date RECORDS_BASELINE"
  add_adr '# ADR 0004: A new decision' yes; aok "a new ADR in the template's form after the baseline"
  mka date-head; conf 'RECORDS_BASELINE="2026-09-15"'; add_adr '# ADR-0004 — A new decision' yes
  abad "a NEW ADR (after the baseline) in a legacy heading form" "0004-new.md: first line must be"
  mka date-enf; conf 'RECORDS_BASELINE="2026-09-15"'; add_adr '# ADR 0004: A new decision' no
  abad "a NEW ADR (after the baseline) with no Enforcement" "0004-new.md has no '## Enforcement'"
  mka ref; new_repo "$F"; git -C "$F" add -A >/dev/null 2>&1; git -C "$F" commit -qm adopt >/dev/null 2>&1
  conf "RECORDS_BASELINE=\"$(git -C "$F" rev-parse HEAD)\""; aok "legacy ADRs that exist at a ref RECORDS_BASELINE"
  add_adr '# ADR 0004: A new decision' no; abad "a NEW ADR (absent at the ref baseline) with no Enforcement" "0004-new.md has no '## Enforcement'"
  mka dup; conf 'RECORDS_BASELINE="2026-09-15"'; cp "$A/0003-spec-seeds.md" "$A/0003-again.md"; printf '| [0003](0003-again.md) | t | x |\n' >> "$A/README.md"
  abad "a number taken twice, legacy or not" "numbers used twice across docs/adr and docs/practices: 0003"
  mka bad; conf 'RECORDS_BASELINE="nope"'; abad "an unresolvable RECORDS_BASELINE" "neither a YYYY-MM-DD date nor a commit"

  # Practice records (PRACTICES_DIR): one number space with the ADRs, their own index and heading.
  mkp() {  # mkp NAME: the fixture above plus a practices directory with its index and template
    mka "$1"; conf 'RECORDS_BASELINE="2026-09-15"'; P="$F/docs/practices"; mkdir -p "$P"
    printf '# PRAC NNNN: <Title>\n' > "$P/TEMPLATE.md"
    printf '# Practice records\n\n| PRAC | Title | Status |\n|---|---|---|\n' > "$P/README.md"
  }
  add_prac() {  # add_prac NUM HEADING ENFORCEMENT(yes|no) [DATE]: practice record NUM, indexed
    { printf '%s\n\n- **Status**: Accepted\n- **Date**: %s\n\n## Practice\nx\n' "$2" "${4:-2026-09-29}"; [ "$3" = yes ] && printf '\n## Enforcement\nchecks/x.sh\n'; } > "$P/$1-practice.md"
    printf '| [%s](%s-practice.md) | t | Accepted |\n' "$1" "$1" >> "$P/README.md"
  }
  mka noprac; conf 'RECORDS_BASELINE="2026-09-15"'; aok "no practices directory (a project with no practice records yet)"
  mkp prac; add_prac 0004 '# PRAC 0004: A new practice' yes; aok "a practice record in the template's form, numbered past the ADRs"
  mkp prac-dup; add_prac 0003 '# PRAC 0003: A practice' yes
  abad "a practice record that takes an ADR's number" "numbers used twice across docs/adr and docs/practices: 0003"
  mkp prac-idx; add_prac 0004 '# PRAC 0004: A new practice' yes; printf '# Practice records\n' > "$P/README.md"
  abad "a practice record missing from its own index" "0004-practice.md has no row in docs/practices/README.md's index"
  mkp prac-head; add_prac 0004 '# ADR 0004: A new practice' yes
  abad "a practice record headed as an ADR" "0004-practice.md: first line must be '# PRAC 0004: <title>'"
  mkp prac-enf; add_prac 0004 '# PRAC 0004: A new practice' no
  abad "a practice record with no Enforcement" "0004-practice.md has no '## Enforcement'"
  mkp prac-legacy; add_prac 0004 '# PRAC-0004 — An old practice' no 2026-08-01
  aok "a practice record dated before a date RECORDS_BASELINE, in a legacy form"
  mkp prac-noreadme; rm "$P/README.md"; abad "a practices directory with no index" "no index at docs/practices/README.md"
  mkp prac-link; printf '| [0009](0009-gone.md) | t | Accepted |\n' >> "$P/README.md"
  abad "a practices index row linking a missing file" "the index links 0009-gone.md, which does not exist"

  # new-adr's context numbers across both directories on origin/main: a practice record's number
  # is taken for an ADR too.
  mkp ctx; add_prac 0005 '# PRAC 0005: A practice' yes; mkdir -p "$F/.claude/skills/new-adr"
  cp "$STRIDAL_FW/skills/new-adr/context.sh" "$F/.claude/skills/new-adr/"
  new_repo "$F"; git -C "$F" add -A >/dev/null 2>&1; git -C "$F" commit -qm adopt >/dev/null 2>&1
  git -C "$F" update-ref "refs/remotes/origin/$MAIN_BRANCH" HEAD
  rm "$P/0005-practice.md"  # gone from the branch, still on origin/main
  if (cd "$F" && sh .claude/skills/new-adr/context.sh) 2>&1 | grep -q 'Next number: at least 0006'; then
    ok "new-adr context: the next number is past a practice record on origin/main"
  else fail "new-adr context: the next number ignores a practice record on origin/main:"
    (cd "$F" && sh .claude/skills/new-adr/context.sh) 2>&1 | sed 's/^/       /'; fi
fi

dir="$ROOT/$ADR_DIR"
[ -d "$dir" ] || { fail "ADR_DIR $ADR_DIR does not exist"; finish; }
[ "$(records_baseline_kind)" = bad ] && { fail "$(records_baseline_bad)"; finish; }
records() { ls "$ROOT/$1" 2>/dev/null | grep -E '^[0-9]{4}-.*\.md$'; }

# The number space: both directories at once.
nadr=$(records "$ADR_DIR" | grep -c .); nprac=$(records "$PRACTICES_DIR" | grep -c .)
dups=$({ records "$ADR_DIR"; records "$PRACTICES_DIR"; } | cut -c1-4 | sort | uniq -d)
if [ -z "$dups" ]; then ok "no record number is used twice ($nadr ADRs, $nprac practice records)"
else fail "numbers used twice across $ADR_DIR and $PRACTICES_DIR: $(echo $dups) (the branch that merged second takes the next free number)"; fi

# series REL WORD: one record directory, WORD being its heading's word (ADR, PRAC). Its index and
# TEMPLATE.md; every NNNN-*.md has an index row, and (since adoption) the heading and an
# Enforcement section; every index link resolves.
series() {
  sdir="$ROOT/$1"; idx="$sdir/README.md"
  [ -f "$idx" ] || { fail "no index at $1/README.md"; return; }
  [ -f "$sdir/TEMPLATE.md" ] && ok "$1/TEMPLATE.md exists" || fail "no $1/TEMPLATE.md for new-adr to copy"
  nlegacy=0
  for f in $(records "$1"); do
    num=$(printf '%s' "$f" | cut -c1-4)
    grep -qF "($f)" "$idx" || fail "$f has no row in $1/README.md's index"
    if record_is_legacy "$1" "$f"; then
      nlegacy=$((nlegacy + 1))
      head -1 "$sdir/$f" | grep -qE "^# $2[ -]$num([^0-9]|$)" || fail "$f: first line must name it, '# $2 $num: <title>' (or, before RECORDS_BASELINE, '# $2-$num — <title>')"
      continue
    fi
    head -1 "$sdir/$f" | grep -qE "^# $2 $num([^0-9]|$)" || fail "$f: first line must be '# $2 $num: <title>'"
    grep -q '^## Enforcement' "$sdir/$f" || fail "$f has no '## Enforcement' section (a record without one is a wish)"
  done
  for l in $(grep -oE '\]\([0-9]{4}-[^)]*\.md\)' "$idx" | sed 's/^](//; s/)$//'); do
    [ -f "$sdir/$l" ] || fail "the index links $l, which does not exist"
  done
  [ "$nlegacy" -eq 0 ] || ok "$nlegacy record(s) in $1 written before RECORDS_BASELINE ($RECORDS_BASELINE) kept as written"
}
series "$ADR_DIR" ADR
if [ -d "$ROOT/$PRACTICES_DIR" ]; then series "$PRACTICES_DIR" PRAC
else ok "no $PRACTICES_DIR: no practice records yet (new-adr writes the first)"; fi
[ "$_fails" -eq 0 ] && ok "every ADR and practice record is indexed, titled and has an Enforcement section (those since adoption)"
finish
