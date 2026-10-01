# change.sh: sourced (never run) by the scripts that read a change record: checks/changes.sh,
# .claude/scenario-trace.sh, .claude/verify-change.sh, .claude/change-approval.sh and
# pr-merge-guard. ONE reading of the change format (changes/README.md), so two scripts cannot
# disagree about what a scenario ID or an open task is (*A check that reads source is a parser*).
#
# Every function reads a file on stdin or by path and prints plain lines; none of them exits.

# The scenario ID grammar (docs/prds, D3): <CAPABILITY>-<requirement n>-S<scenario n>. A capability
# segment starts with a letter, so the numeric parts cannot be mistaken for it.
SCENARIO_ID_ERE='[A-Z][A-Z0-9]*(-[A-Z][A-Z0-9]*)*-[0-9]+-S[0-9]+'

# sha256_hex: the SHA-256 of stdin, as hex. macOS ships shasum, Linux sha256sum.
sha256_hex() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum | cut -d' ' -f1
  elif command -v shasum >/dev/null 2>&1; then shasum -a 256 | cut -d' ' -f1
  else echo "no-sha256-tool"; fi
}

# change_extra_missing DIR: one line per section CHANGE_RECORD_EXTRA (project.conf) requires that
# the change record in DIR lacks. CHANGE_RECORD_EXTRA is `;`-separated FILE:HEADING entries, e.g.
# "proposal.md:Rollback; design.md:Security review": FILE must carry a `## HEADING` line (## to
# ####). For a project whose records carry more than the template's sections; empty = none.
change_extra_missing() {
  [ -n "${CHANGE_RECORD_EXTRA:-}" ] || return 0
  printf '%s\n' "$CHANGE_RECORD_EXTRA" | tr ';' '\n' | while IFS= read -r _ce; do
    _ce=$(printf '%s' "$_ce" | sed 's/^[[:space:]]*//; s/[[:space:]]*$//')
    [ -n "$_ce" ] || continue
    case $_ce in
      *:*) ;;
      *) echo "CHANGE_RECORD_EXTRA entry '$_ce' is not FILE:HEADING"; continue ;;
    esac
    _cf=${_ce%%:*} _ch=$(printf '%s' "${_ce#*:}" | sed 's/^[[:space:]]*//')
    if [ ! -f "$1/$_cf" ]; then echo "$_cf is missing (CHANGE_RECORD_EXTRA requires its '## $_ch' section)"
    elif ! grep -qE "^#{2,4} +$(printf '%s' "$_ch" | sed 's#[][\.*^$+?(){}|]#\\&#g')[[:space:]]*\$" "$1/$_cf"; then
      echo "$_cf has no '## $_ch' section (CHANGE_RECORD_EXTRA in .claude/project.conf)"
    fi
  done
}

# design_hash DIR: what the owner's approval covers (D8): design.md as written, plus the proposal's
# Risk line (the owner approves the tier with the design, item 15). 12 hex characters.
design_hash() {
  { cat "$1/design.md" 2>/dev/null; grep -E '^\*\*Risk:\*\*' "$1/proposal.md" 2>/dev/null; } | sha256_hex | cut -c1-12
}

# spec_scenarios FILE: one line per scenario header, tab-separated:
#   <section> <requirement name> <id or -> <whole header text>
# <section> is ADDED, MODIFIED, REMOVED or RENAMED inside a delta, LIVING in a living spec.
spec_scenarios() {
  awk -v ere="$SCENARIO_ID_ERE" '
    BEGIN { sec = "LIVING" }
    /^## (ADDED|MODIFIED|REMOVED|RENAMED) Requirements/ { sec = $2; req = ""; next }
    /^## / { if (sec == "LIVING") sec = "LIVING"; req = ""; next }
    /^### Requirement:/ { req = $0; sub(/^### Requirement:[ \t]*/, "", req); sub(/[ \t]+$/, "", req); next }
    /^#### Scenario:/ {
      h = $0; sub(/[ \t]+$/, "", h); id = "-"
      rest = h; sub(/^#### Scenario:[ \t]*/, "", rest)
      if (match(rest, "^\\[" ere "\\]")) id = substr(rest, 2, RLENGTH - 2)
      printf "%s\t%s\t%s\t%s\n", sec, req, id, h
    }' "$1"
}

# spec_malformed FILE: each scenario header whose bracket is not a well-formed ID, or which has no
# bracket at all, one per line: "<id-or-none>\t<header>". The caller decides whether a missing ID
# is an error (SCENARIO_IDS in project.conf).
spec_malformed() {
  awk -v ere="$SCENARIO_ID_ERE" '
    /^#### Scenario:/ {
      h = $0; sub(/[ \t]+$/, "", h); rest = h; sub(/^#### Scenario:[ \t]*/, "", rest)
      if (match(rest, "^\\[" ere "\\] [^ ]")) next
      if (rest ~ /^\[/) print "malformed\t" h; else print "none\t" h
    }' "$1"
}

# requirement_block FILE NAME: the lines of one requirement, header to the next ### or ## heading.
requirement_block() {
  awk -v want="$2" '
    /^### Requirement:/ { n = $0; sub(/^### Requirement:[ \t]*/, "", n); sub(/[ \t]+$/, "", n); on = (n == want) }
    /^## / { on = 0 }
    on' "$1"
}

# open_tasks FILE / done_tasks FILE: checkbox counts, the Slice line excluded: it states what the
# change delivers, it is not a task, and it may stay unticked.
_is_slice='^[[:space:]]*(-[[:space:]]*)?(\[[ xX]\][[:space:]]*)?([0-9]+(\.[0-9]+)*[[:space:]]+)?(\*\*)?Slice:'
open_tasks() {
  { grep -E '^[[:space:]]*- \[ \]' "$1" 2>/dev/null || true; } | { grep -Ev "$_is_slice" || true; } | wc -l | tr -d ' '
}
done_tasks() {
  { grep -E '^[[:space:]]*- \[[xX]\]' "$1" 2>/dev/null || true; } | { grep -Ev "$_is_slice" || true; } | wc -l | tr -d ' '
}

# task_escapes FILE: "<id>\t<manual|untestable>: <reason>" for each tasks.md line that names a
# scenario ID and carries `(manual: <reason>)` or `(untestable: <reason>)` with a non-blank reason.
task_escapes() {
  awk -v ere="$SCENARIO_ID_ERE" '
    match($0, /\((manual|untestable):[ \t]*[^ \t)][^)]*\)/) {
      esc = substr($0, RSTART + 1, RLENGTH - 2); line = $0
      while (match(line, ere)) { print substr(line, RSTART, RLENGTH) "\t" esc; line = substr(line, RSTART + RLENGTH) }
    }' "$1" 2>/dev/null
}

# proposal_field FILE KEY: the value of a `**KEY:** value` line, trailing blanks stripped.
proposal_field() {
  sed -n "s/^\*\*$2:\*\*[[:space:]]*//p" "$1" 2>/dev/null | head -1 | sed 's/[[:space:]]*$//'
}
