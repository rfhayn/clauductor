# Sourced by .claude/hooks/pr-merge-guard.sh (rule 7) and .claude/checks/journal.sh. POSIX sh;
# needs only grep, sed and sort.
#
# WHY. When several people run sessions against one repo, each session-close writes
# `## Session N` at the top of the journal with N = the latest number + 1. Two sessions that both
# read `Session 91` both write `Session 92`. Git surfaces the TEXT (both insert above the same
# line, so the second PR conflicts), but the obvious resolution, "keep both", produces two entries
# with one number, and nothing after that notices.
#
# WHAT IT DECIDES, from three heading lists (the lines matching `^## Session [0-9]+`):
#   BASE = the journal at merge-base(origin/main, head)   HEAD = at the PR's head
#   MAIN = at origin/main
# A number collides if either:
#   (a) HEAD carries more headings with that number than MAIN does (a new duplicate: the careless
#       "keep both" resolution; a duplicate main already carries is history, not this PR's doing);
#   (b) HEAD ADDS a heading (absent from BASE and not carried verbatim by MAIN) whose number MAIN
#       already has and BASE did not: the other person merged that number after this branch was
#       cut. BASE keeps (b) from firing on a retitle, and "verbatim on MAIN" keeps it from firing
#       on this branch's own squash-merged heading.
# Sub-numbered sessions (`58`, `58.1`) are distinct numbers.
#
# WHAT IT CANNOT DECIDE: that the number is the NEXT one (a gap passes).

_journal_numbers() {
  printf '%s\n' "$1" | sed -n 's/^## Session \([0-9][0-9]*\(\.[0-9][0-9]*\)*\).*/\1/p'
}

# $1 = BASE headings, $2 = HEAD headings, $3 = MAIN headings (newline-separated lines).
# Prints the colliding session numbers, space-separated; returns 1 if there are any.
journal_session_collisions() {
  _base_nums=$(_journal_numbers "$1")
  _main_nums=$(_journal_numbers "$3")
  _hits=$(_journal_numbers "$2" | sort | uniq -d | while IFS= read -r _n; do
    _in_head=$(_journal_numbers "$2" | grep -cxF -- "$_n")
    _in_main=$(_journal_numbers "$3" | grep -cxF -- "$_n") || _in_main=0
    [ "$_in_head" -gt "$_in_main" ] && printf '%s\n' "$_n"
  done)
  _added=$(printf '%s\n' "$2" | grep -E '^## Session [0-9]+' | while IFS= read -r _line; do
    printf '%s\n' "$1" | grep -qxF -- "$_line" && continue
    printf '%s\n' "$3" | grep -qxF -- "$_line" && continue
    printf '%s\n' "$_line"
  done)
  for _n in $(_journal_numbers "$_added"); do
    printf '%s\n' "$_main_nums" | grep -qxF -- "$_n" || continue
    printf '%s\n' "$_base_nums" | grep -qxF -- "$_n" && continue
    _hits="$_hits
$_n"
  done
  _hits=$(printf '%s\n' "$_hits" | grep -E '^[0-9]+(\.[0-9]+)*$' | sort -u | tr '\n' ' ')
  _hits=${_hits% }
  [ -z "$_hits" ] && return 0
  printf '%s\n' "$_hits"
  return 1
}
