#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# An ADR number is never taken twice, every ADR file has a row in the index (docs/adr/README.md),
# and every index row links a file that exists. The set is the directory (the authority), not the
# index: an ADR the index forgot is the failure this exists to catch.
#
# Each ADR added since adoption opens with '# ADR NNNN: <title>' and has an '## Enforcement'
# section. An ADR written before RECORDS_BASELINE (.claude/lib/records.sh) is history, never
# reformatted: its heading may be any '# ADR NNNN' / '# ADR-NNNN' form and it may lack the section.
# Falsified on fixtures shaped like an adopting project's ADR history.
. "$(dirname "$0")/lib.sh"
. "$CLAUDUCTOR_FW/lib/records.sh"

# ── Self-test ─────────────────────────────────────────────────────────────────────────────────────
if [ -z "${ADR_SELFTEST:-}" ]; then
  mka() {  # mka NAME: a fixture project with three legacy ADRs (two heading forms, no Enforcement)
    F="$(scratch)/adr-$1"; A="$F/docs/adr"; mkdir -p "$F/.claude/checks" "$A"
    cp -R "$CLAUDUCTOR_FW/lib" "$F/.claude/"; cp "$CLAUDUCTOR_FW/checks/lib.sh" "$CLAUDUCTOR_FW/checks/adr-numbering.sh" "$F/.claude/checks/"
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
  abad "a number taken twice, legacy or not" "ADR numbers used twice: 0003"
  mka bad; conf 'RECORDS_BASELINE="nope"'; abad "an unresolvable RECORDS_BASELINE" "neither a YYYY-MM-DD date nor a commit"
fi

dir="$ROOT/$ADR_DIR"
[ -d "$dir" ] || { fail "ADR_DIR $ADR_DIR does not exist"; finish; }
idx="$dir/README.md"
[ -f "$idx" ] || { fail "no index at $ADR_DIR/README.md"; finish; }
[ -f "$dir/TEMPLATE.md" ] && ok "TEMPLATE.md exists" || fail "no $ADR_DIR/TEMPLATE.md for new-adr to copy"
[ "$(records_baseline_kind)" = bad ] && { fail "$(records_baseline_bad)"; finish; }

files=$(ls "$dir" | grep -E '^[0-9]{4}-.*\.md$')
dups=$(printf '%s\n' "$files" | grep . | cut -c1-4 | sort | uniq -d)
if [ -z "$dups" ]; then ok "no ADR number is used twice ($(printf '%s\n' "$files" | grep -c .) ADRs)"; else fail "ADR numbers used twice: $(echo $dups) (the branch that merged second takes the next free number)"; fi

nlegacy=0
for f in $files; do
  num=$(printf '%s' "$f" | cut -c1-4)
  if grep -qF "($f)" "$idx"; then :; else fail "$f has no row in $ADR_DIR/README.md's index"; fi
  if adr_is_legacy "$f"; then
    nlegacy=$((nlegacy + 1))
    head -1 "$dir/$f" | grep -qE "^# ADR[ -]$num([^0-9]|$)" || fail "$f: first line must name it, '# ADR $num: <title>' (or, before RECORDS_BASELINE, '# ADR-$num — <title>')"
    continue
  fi
  head -1 "$dir/$f" | grep -qE "^# ADR $num([^0-9]|$)" || fail "$f: first line must be '# ADR $num: <title>'"
  grep -q '^## Enforcement' "$dir/$f" || fail "$f has no '## Enforcement' section (an ADR without one is a wish)"
done
for l in $(grep -oE '\]\([0-9]{4}-[^)]*\.md\)' "$idx" | sed 's/^](//; s/)$//'); do
  [ -f "$dir/$l" ] || fail "the index links $l, which does not exist"
done
[ "$nlegacy" -eq 0 ] || ok "$nlegacy ADR(s) written before RECORDS_BASELINE ($RECORDS_BASELINE) kept as written"
[ "$_fails" -eq 0 ] && ok "every ADR is indexed, titled and has an Enforcement section (those since adoption)"
finish
