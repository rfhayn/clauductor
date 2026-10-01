# records.sh: sourced (never run) by the checks that judge records: checks/journal.sh,
# checks/adr-numbering.sh and checks/changes.sh. ONE answer to "was this record written before the
# project adopted the operating model?", so an adopting project's history is accepted as it is
# (existing records are never reformatted) while every record added since is held to the format.
#
# RECORDS_BASELINE in .claude/project.conf (conf.sh sets the default, ""):
#   ""            no baseline: every record is judged (a project that started on the template)
#   YYYY-MM-DD    a record dated BEFORE this day is legacy: a journal heading by its own date, an ADR
#                 by its `- **Date**:` line, a change by its .openspec.yaml `created:`; git's first
#                 commit of the file only when the record carries no date
#   <git ref>     a record that EXISTS at this ref (a tag or sha: the adoption commit) is legacy: a
#                 journal session by its number, an ADR by its number, a change by its directory
#
# A baseline that names a ref this clone cannot resolve (a shallow CI checkout, a typo) is not
# "no baseline": records_baseline_kind prints "bad" and the caller FAILS, because judging the
# history strictly or not at all would both be silent guesses.
#
# Needs ROOT and the conf.sh keys. Every function prints plain lines and never exits.

# records_baseline_kind: none | date | ref | bad
records_baseline_kind() {
  case "${RECORDS_BASELINE:-}" in
    '') echo none ;;
    [0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]) echo date ;;
    *) if git -C "$ROOT" rev-parse -q --verify "${RECORDS_BASELINE}^{commit}" >/dev/null 2>&1; then echo ref; else echo bad; fi ;;
  esac
}

# records_baseline_bad: the one FAIL message for an unresolvable baseline.
records_baseline_bad() {
  printf "RECORDS_BASELINE '%s' is neither a YYYY-MM-DD date nor a commit this clone has (a shallow clone? fetch the ref, or use the adoption date), so the history cannot be told from new records" "$RECORDS_BASELINE"
}

# records_before DATE: 0 when DATE (YYYY-MM-DD) is a day before a date RECORDS_BASELINE; 1 when it
# is empty or not before. As integers: `[ a \< b ]` is not POSIX test.
records_before() {
  case "$1" in [0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]) ;; *) return 1 ;; esac
  [ "$(printf '%s' "$1" | tr -d -)" -lt "$(printf '%s' "$RECORDS_BASELINE" | tr -d -)" ]
}

# records_added_date PATH: the day git first committed PATH (relative to ROOT); empty if never.
records_added_date() {
  git -C "$ROOT" log --diff-filter=A --format=%cs -- "$1" 2>/dev/null | tail -1
}

# journal_legacy_sessions: under a ref baseline, the session numbers the journal had at that ref,
# one per line. Empty for any other kind.
journal_legacy_sessions() {
  [ "$(records_baseline_kind)" = ref ] || return 0
  git -C "$ROOT" show "$RECORDS_BASELINE:$JOURNAL" 2>/dev/null | sed -n 's/^## Session \([0-9][0-9.]*\).*/\1/p'
}

# adr_is_legacy FILE (a basename in ADR_DIR): 0 when that ADR predates the baseline.
adr_is_legacy() {
  case $(records_baseline_kind) in
    ref) git -C "$ROOT" cat-file -e "$RECORDS_BASELINE:$ADR_DIR/$1" 2>/dev/null && return 0
         # Renamed since? The number is the record's identity.
         git -C "$ROOT" ls-tree --name-only "$RECORDS_BASELINE" -- "$ADR_DIR/" 2>/dev/null | sed 's|.*/||' | grep -q "^$(printf '%s' "$1" | cut -c1-4)-" ;;
    date) _rd=$(sed -n 's/^- \*\*Date\*\*:[[:space:]]*\([0-9]\{4\}-[0-9]\{2\}-[0-9]\{2\}\).*/\1/p' "$ROOT/$ADR_DIR/$1" 2>/dev/null | head -1)
          [ -n "$_rd" ] || _rd=$(records_added_date "$ADR_DIR/$1")
          records_before "$_rd" ;;
    *) return 1 ;;
  esac
}

# change_created_legacy ID: 0 when open change ID predates the baseline (CHANGES_LEGACY="baseline",
# or to verify an explicit list).
change_created_legacy() {
  case $(records_baseline_kind) in
    ref) git -C "$ROOT" ls-tree --name-only "$RECORDS_BASELINE" -- "$CHANGES_DIR/$1/" 2>/dev/null | grep -q . ;;
    date) _rd=$(sed -n 's/^created:[[:space:]]*["'"'"']\{0,1\}\([0-9]\{4\}-[0-9]\{2\}-[0-9]\{2\}\).*/\1/p' "$ROOT/$CHANGES_DIR/$1/.openspec.yaml" 2>/dev/null | head -1)
          [ -n "$_rd" ] || _rd=$(records_added_date "$CHANGES_DIR/$1/proposal.md")
          records_before "$_rd" ;;
    *) return 1 ;;
  esac
}

# change_is_legacy ID: 0 when CHANGES_LEGACY grandfathers open change ID: it is listed by name, or
# CHANGES_LEGACY is "baseline" and the change predates RECORDS_BASELINE. A grandfathered change was
# proposed under the project's earlier format; it is still read (its deltas count when a later
# change modifies what it adds) but its format is not judged.
change_is_legacy() {
  case " ${CHANGES_LEGACY:-} " in *" $1 "*) return 0 ;; esac
  [ "${CHANGES_LEGACY:-}" = baseline ] && change_created_legacy "$1"
}
