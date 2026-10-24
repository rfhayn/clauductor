#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/../.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# fold.sh: the daily close's fold (OPS-108). Every row fragment waiting in JOURNAL_FRAGMENTS becomes
# one `## Session N` entry in JOURNAL; their insight rows go to the top of INSIGHTS' table; each
# landed row's Status in ROADMAP becomes `✅ merged (#PR)`; the fragments are removed (git rm).
#
#   sh .claude/skills/session-close/fold.sh <N> [--focus "<short focus>"] [--dry-run]
#
# Run on the close's branch (BRANCH_SESSION_CLOSE), cut from origin/main, so the fragments it reads
# are main's. N is the first plan session the entry covers (a gap passes rule 7; a number another
# session holds does not). The heading is `## Session N — <today> — <author> — <focus>`, the focus
# defaulting to the rows' ids. Then /dev-journal adds the close's own narrative under it.
#
# ALL OR NOTHING. A malformed fragment (lib/fragments.sh, the shape checks/journal.sh holds at the
# row's gate), an insights log with no table under INSIGHTS_TABLE_HEADING, a roadmap row whose
# Status cell it cannot find, or a roadmap that no longer parses after the edit: each stops it,
# naming why, before ANY file changes. A fold that wrote half would drop a row's record silently.
# Exit 0 folded (or nothing to fold), 1 refused.
ROOT=$(case $CLAUDUCTOR_FW in (*/.claude) dirname "$CLAUDUCTOR_FW" ;; (*) [ -n "${ROOT:-}" ] && echo "$ROOT" || git rev-parse --show-toplevel 2>/dev/null || pwd ;; esac)
# shellcheck disable=SC1091
. "$CLAUDUCTOR_FW/lib/conf.sh"
# shellcheck disable=SC1091
. "$CLAUDUCTOR_FW/lib/fragments.sh"

refuse() { echo "fold: REFUSED — $*; nothing was changed"; exit 1; }
n="" focus="" dry=""
while [ $# -gt 0 ]; do
  case $1 in
    --focus) [ $# -ge 2 ] && [ -n "$2" ] || refuse "--focus needs the focus after it"; focus=$2; shift 2 ;;
    --dry-run) dry=1; shift ;;
    *) n=$1; shift ;;
  esac
done
printf '%s' "$n" | grep -qE '^[0-9]+(\.[0-9]+)*$' || refuse "usage: fold.sh <N> [--focus \"…\"] [--dry-run] (N, the session number, is required)"

tmp=$(mktemp -d "${TMPDIR:-/tmp}/fold.XXXXXX") || refuse "no temp directory"
trap 'rm -rf "$tmp"' EXIT
fragment_files > "$tmp/list"
[ -s "$tmp/list" ] || { echo "fold: no fragment waits in $JOURNAL_FRAGMENTS; nothing to fold"; exit 0; }

# 1. Every fragment has the shape, or nothing happens.
bad=""
while IFS= read -r f; do
  p=$(fragment_problems "$f") || bad="$bad
  ${f#"$ROOT"/}: $(printf '%s\n' "$p" | head -3 | tr '\n' ';')"
done < "$tmp/list"
[ -z "$bad" ] || refuse "malformed fragment(s), fix each in a row PR first:$bad"

field() { sed -n "s/^- \*\*$2\*\*:[[:space:]]*//p" "$1" | head -1; }
ids=""; while IFS= read -r f; do ids="$ids${ids:+, }$(basename "$f" .md)"; done < "$tmp/list"
[ -n "$focus" ] || focus="rows $ids"
today=$(TZ="$TIME_ZONE" date +%Y-%m-%d)
author=$(git -C "$ROOT" config user.name 2>/dev/null | awk '{print $1}')
[ -n "$author" ] || refuse "git config user.name is empty, so the heading has no author"
heading="## Session $n — $today — $author — $focus"
printf '%s\n' "$heading" | grep -qE -- "$JOURNAL_HEADING" || refuse "the heading '$heading' does not match JOURNAL_HEADING"
grep -qE "^## Session $(printf '%s' "$n" | sed 's/\./\\./g')( |$)" "$ROOT/$JOURNAL" 2>/dev/null \
  && refuse "$JOURNAL already has a Session $n: take the first plan session this close covers that no entry holds"

# 2. The entry: a summary line, then each row's fragment with its headings stepped down, its
# Insights section left out (the rows move to the log; the entry says how many).
gate=0; gates=ok; cost=0; costs=ok
{
  printf '%s\n\n' "$heading"
  printf '**Rows folded.** %s.\n\n' "$(while IFS= read -r f; do printf '%s (%s)\n' "$(basename "$f" .md)" "$(field "$f" PR | sed 's/[[:space:]].*//')"; done < "$tmp/list" | paste -sd ',' - | sed 's/,/, /g')"
  while IFS= read -r f; do
    ni=$(awk '/^(```|~~~)/ { fence = !fence } !fence && /^## Insights/ { on = 1; next } !fence && /^## / { on = 0 } on && !fence && /^\|/ { c++ } END { print c + 0 }' "$f")
    # A fenced block is copied as it stands: a `## ` in it is the fragment quoting, not a section.
    awk -v ni="$ni" '
      NR == 1 { sub(/^# /, "### "); print; next }
      /^(```|~~~)/ { fence = !fence; if (!skip) b[++k] = $0; next }
      fence { if (!skip) b[++k] = $0; next }
      /^## Insights/ { skip = 1; b[++k] = "#### Insights"; b[++k] = ""; b[++k] = (ni ? ni " row(s), moved to the insights log." : "None."); b[++k] = ""; next }
      /^## / { skip = 0; sub(/^## /, "#### ") }
      !skip { b[++k] = $0 }
      END { while (k > 0 && b[k] ~ /^[ \t]*$/) k--; for (i = 1; i <= k; i++) print b[i] }' "$f"
    printf '\n'
  done < "$tmp/list"
} > "$tmp/entry"
while IFS= read -r f; do
  g=$(field "$f" Gate | sed -n 's/^\([0-9][0-9.]*\) min.*/\1/p')
  if [ -n "$g" ]; then gate=$(awk -v a="$gate" -v b="$g" 'BEGIN { print a + b }'); else gates=partial; fi
  c=$(field "$f" Cost | sed -n 's/^\$\([0-9][0-9.]*\).*/\1/p')
  if [ -n "$c" ]; then cost=$(awk -v a="$cost" -v b="$c" 'BEGIN { printf "%.2f", a + b }'); else costs=partial; fi
done < "$tmp/list"
# A reading that failed is CANNOT CHECK, never 0: a total missing one is a floor, and says so.
totals="**Totals.** Gate $gate min"
[ "$gates" = ok ] || totals="$totals (a row's gate was CANNOT CHECK, so this is a floor)"
totals="$totals; cost \$$cost"
[ "$costs" = ok ] || totals="$totals (a row's cost was CANNOT CHECK, so this is a floor)"
awk -v t="$totals." 'NR == 3 { print t; print "" } { print }' "$tmp/entry" > "$tmp/entry2" && mv "$tmp/entry2" "$tmp/entry"

# 3. The journal: the entry above the first numbered session heading (the newest first), or at the
# end. A fenced block is skipped: the journal's own format example (`## Session N — …`) sits in one.
j="$ROOT/$JOURNAL"; [ -f "$j" ] || refuse "JOURNAL $JOURNAL does not exist"
awk -v ef="$tmp/entry" '
  /^(```|~~~)/ { fence = !fence }
  !done && !fence && /^## Session [0-9]/ { while ((getline l < ef) > 0) print l; done = 1 }
  { print }
  END { if (!done) { print ""; while ((getline l < ef) > 0) print l } }' "$j" > "$tmp/journal"

# 4. The insights log: every fragment's rows, right under the table's separator line.
i="$ROOT/$INSIGHTS"; [ -f "$i" ] || refuse "INSIGHTS $INSIGHTS does not exist"
while IFS= read -r f; do
  awk '/^(```|~~~)/ { fence = !fence } !fence && /^## Insights/ { on = 1; next } !fence && /^## / { on = 0 } on && !fence && /^\|/' "$f"
done < "$tmp/list" > "$tmp/rows"
if [ -s "$tmp/rows" ]; then
  H="$INSIGHTS_TABLE_HEADING" awk -v rf="$tmp/rows" '
    function trim(s) { sub(/^[ \t]+/, "", s); sub(/[ \t]+$/, "", s); return s }
    trim($0) == trim(ENVIRON["H"]) { inlog = 1 }
    { print }
    inlog && !done && /^\|[ \t:-]+\|/ && prev ~ /^\|[ \t]*Date[ \t]*\|/ { while ((getline l < rf) > 0) print l; done = 1 }
    { prev = $0 }
    END { if (!done) exit 3 }' "$i" > "$tmp/insights" \
    || refuse "$INSIGHTS has no '| Date | … |' table with its separator under '$INSIGHTS_TABLE_HEADING' (INSIGHTS_TABLE_HEADING)"
else
  cp "$i" "$tmp/insights"
fi

# 5. The roadmap: each row's Status cell, by the line the one parser names (roadmap_queue).
r="$ROOT/$ROADMAP"
tsv=$(roadmap_queue --tsv) || refuse "the roadmap does not parse: $(printf '%s' "$tsv" | head -1)"
cp "$r" "$tmp/roadmap"; : > "$tmp/notes"
while IFS= read -r f; do
  id=$(basename "$f" .md); pr=$(field "$f" PR | sed -n 's/^#\([0-9][0-9]*\).*/\1/p')
  row=$(printf '%s\n' "$tsv" | awk -F'\t' -v a="$id" '$4 == a { print; exit }')
  if [ -z "$row" ]; then echo "  $id: no roadmap row has this id, so no Status was set" >> "$tmp/notes"; continue; fi
  [ "$(printf '%s\n' "$row" | cut -f7)" = merged ] && { echo "  $id: already merged on the roadmap" >> "$tmp/notes"; continue; }
  line=$(printf '%s\n' "$row" | cut -f1)
  awk -v ln="$line" -v st="✅ merged (#$pr)" '
    NR == ln { s = $0; sub(/[ \t]*\|[ \t]*$/, "", s); k = 0; for (p = length(s); p > 0; p--) if (substr(s, p, 1) == "|") { k = p; break }
               if (!k) exit 3; $0 = substr(s, 1, k) " " st " |" }
    { print }' "$tmp/roadmap" > "$tmp/roadmap2" || refuse "roadmap line $line (row $id) has no Status cell to set"
  mv "$tmp/roadmap2" "$tmp/roadmap"
  echo "  $id: Status ✅ merged (#$pr)" >> "$tmp/notes"
done < "$tmp/list"
chk=$(roadmap_queue --check "$tmp/roadmap" 2>&1) || refuse "the roadmap would not parse after the edit: $(printf '%s' "$chk" | head -2 | tr '\n' ' ')"

if [ -n "$dry" ]; then
  echo "fold (dry run): Session $n from $(grep -c . "$tmp/list") fragment(s); $(grep -c . "$tmp/rows") insight row(s)"
  cat "$tmp/notes"; echo "--- the entry:"; cat "$tmp/entry"; exit 0
fi

# 6. Every check passed: write, then remove the fragments.
cp "$tmp/journal" "$j"; cp "$tmp/insights" "$i"; cp "$tmp/roadmap" "$r"
while IFS= read -r f; do
  git -C "$ROOT" rm -q -- "$f" 2>/dev/null || rm -f -- "$f"
done < "$tmp/list"
echo "fold: Session $n written to $JOURNAL from $(grep -c . "$tmp/list") fragment(s) ($ids); $(grep -c . "$tmp/rows") insight row(s) into $INSIGHTS; fragments removed"
cat "$tmp/notes"
