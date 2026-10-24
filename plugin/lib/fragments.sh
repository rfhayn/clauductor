# A row's journal fragment: the shape checks/journal.sh holds and session-close's fold reads
# (.claude/skills/session-close/fold.sh). POSIX sh + awk; source .claude/lib/conf.sh first.
#
# WHY. A row's PR writes its records to JOURNAL_FRAGMENTS/<row id>.md and never edits JOURNAL or
# INSIGHTS (OPS-108: pr-merge-guard rules 7 and 8), so parallel rows never touch one shared line.
# The close folds every fragment on main into one `## Session N`. A fold that met a fragment it
# could not read would drop that row's record silently; so the shape is held at the row's own gate,
# where the row's author still reads the failure, and the fold refuses a malformed one.
#
# The shape (docs/journal.d/README.md is the reader's copy):
#   <row id>.md                       the file name: the roadmap row's id ([A-Za-z0-9._-])
#   # <row id> — <focus>              line 1, the id equal to the file name's
#   - **PR**: #N
#   - **Session**: S<n>, started <YYYY-MM-DD HH:MM ET>     the marker line's (session-start/marker.sh)
#   - **Review**: <n> round(s), peak <severity>, cap <n>   critical|high|medium|low|none; cap: .claude/review-cap.sh
#   - **Gate**: <minutes> min                              the last full gate before the fragment's commit
#   - **Cost**: $<amount>, or CANNOT CHECK and why         usage-report.sh, this row's sessions
#   - **Session start**: <tokens> tokens, <minutes> min    the marker's cost line, or CANNOT CHECK
#   - **Also closes**: <row id>[, <row id>…]   optional: the other rows a batched PR closes, which
#                                               the fold marks merged by the same PR (OPS-125)
#   ## What happened / ## Decisions / ## Surprises / ## Insights    each once, in this order
# Under ## Insights: rows of the insights log's table (`| Date | Area | Topic | Observation | How to
# verify | Status |`, six cells, no header), which the fold moves to the top of the log; or `None.`;
# nothing else, since the fold would drop it.
# A fragment never carries a `## Session` heading: only the close numbers a session.

FRAGMENT_FIELDS="PR|Session|Review|Gate|Cost|Session start"
FRAGMENT_SECTIONS="What happened|Decisions|Surprises|Insights"

# $1 = a fragment's path. Prints one problem per line (none when the shape holds); returns 1 if any.
fragment_problems() {
  _f=$1; _name=$(basename "$_f" .md)
  [ -f "$_f" ] || { echo "$_f: no such file"; return 1; }
  awk -v name="$_name" -v fields="$FRAGMENT_FIELDS" -v sections="$FRAGMENT_SECTIONS" '
    function trim(s) { sub(/^[ \t]+/, "", s); sub(/[ \t]+$/, "", s); return s }
    function bad(m) { print m; n++ }
    BEGIN {
      if (name !~ /^[A-Za-z0-9][A-Za-z0-9._-]*$/) bad("the file name \"" name ".md\" is not <row id>.md ([A-Za-z0-9._-])")
      nf = split(fields, F, "|")
      ns = split(sections, S, "|"); next_s = 1; cur = ""
    }
    NR == 1 {
      want = "# " name " — "
      if (substr($0, 1, length(want)) != want || length($0) == length(want)) bad("line 1 is not \"# " name " — <focus>\" (the id must equal the file name)")
      next
    }
    # The fold carries only rows out of ## Insights (to the log), so anything else there, a fence
    # included, would be dropped: refuse it here, where its author still reads why.
    !fence && cur == "Insights" && /^(```|~~~)/ { bad("line " NR ": ## Insights holds only insight rows or \"None.\" (the fold carries nothing else)"); fence = 1; next }
    # A fenced block is the fragment quoting something: a `## ` or `- **X**:` in it is not shape.
    /^(```|~~~)/ { fence = !fence; next }
    fence { next }
    /^## Session/ { bad("line " NR " is a session heading: only a session close numbers a session (rule 7)"); next }
    /^## / {
      h = trim(substr($0, 4)); cur = h
      if (h in seen) { bad("\"## " h "\" appears twice"); next }
      seen[h] = 1
      if (next_s <= ns && h == S[next_s]) { next_s++; next }
      known = 0; for (i = 1; i <= ns; i++) if (h == S[i]) known = 1
      if (known) bad("\"## " h "\" is out of order (What happened, Decisions, Surprises, Insights)")
      next
    }
    /^- \*\*[^*]+\*\*:/ && cur == "" {
      k = $0; sub(/^- \*\*/, "", k); sub(/\*\*:.*/, "", k)
      v = $0; sub(/^- \*\*[^*]+\*\*:[ \t]*/, "", v)
      if (v == "") bad("the **" k "** field is empty")
      got[k] = v; next
    }
    cur == "Insights" && /^\|/ {
      # An escaped \| is text in a cell, as the log itself writes one.
      line = $0; sub(/[ \t]+$/, "", line); gsub(/\\\|/, "", line)
      c = gsub(/\|/, "|", line)
      if (c != 7 || line !~ /\|$/) { bad("line " NR ": an insight row needs six cells (| Date | Area | Topic | Observation | How to verify | Status |)"); next }
      split(line, C, "|")
      if (trim(C[2]) == "Date" || trim(C[2]) ~ /^-+$/) { bad("line " NR ": no table header under ## Insights, only rows (the log has the header)"); next }
      if (trim(C[2]) !~ /^[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]$/) bad("line " NR ": an insight row starts with its date, YYYY-MM-DD")
      rows++; next
    }
    cur == "Insights" && trim($0) == "None." { none = 1; next }
    cur == "Insights" && trim($0) != "" { bad("line " NR ": ## Insights holds only insight rows or \"None.\" (the fold carries nothing else)"); next }
    END {
      if (NR == 0) { print "the fragment is empty"; exit 1 }
      for (i = 1; i <= nf; i++) if (!(F[i] in got)) bad("no **" F[i] "** field before the first section (- **" F[i] "**: …)")
      if ("PR" in got && got["PR"] !~ /^#[0-9]+/) bad("**PR** is not #N")
      if ("Session" in got && got["Session"] !~ /^S[0-9]+(\.[0-9]+)*, started [0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9] [A-Za-z]+/) bad("**Session** is not \"S<n>, started YYYY-MM-DD HH:MM <zone>\" (the marker'"'"'s time)")
      if ("Review" in got && got["Review"] !~ /^[0-9]+ rounds?, peak (critical|high|medium|low|none), cap [0-9]+/) bad("**Review** is not \"<n> round(s), peak <critical|high|medium|low|none>, cap <n>\"")
      if ("Gate" in got && got["Gate"] !~ /^([0-9]+(\.[0-9]+)? min|CANNOT CHECK)/) bad("**Gate** is not \"<minutes> min\" (or CANNOT CHECK and why)")
      # The fold totals what parses and calls the rest CANNOT CHECK: "4.20" would read as a failure.
      if ("Cost" in got && got["Cost"] !~ /^(\$[0-9]+(\.[0-9]+)?|CANNOT CHECK)/) bad("**Cost** is not \"$<amount>\" (or CANNOT CHECK and why)")
      if ("Session start" in got && got["Session start"] !~ /^([0-9]+ tokens, [0-9]+ min|CANNOT CHECK)/) bad("**Session start** is not \"<tokens> tokens, <minutes> min\" (or CANNOT CHECK and why)")
      if ("Also closes" in got) {
        na = split(got["Also closes"], A, ",")
        for (i = 1; i <= na; i++) {
          a = trim(A[i])
          if (a !~ /^[A-Za-z0-9][A-Za-z0-9._-]*$/) bad("**Also closes** is not row ids separated by commas (\"" trim(got["Also closes"]) "\")")
          else if (a == name) bad("**Also closes** names the fragment'"'"'s own row, " name)
        }
      }
      for (i = next_s; i <= ns; i++) if (!(S[i] in seen)) bad("no \"## " S[i] "\" section")
      if (("Insights" in seen) && !rows && !none) bad("## Insights has neither an insight row nor \"None.\"")
      if (rows && none) bad("## Insights has rows and \"None.\"")
      exit (n > 0)
    }' "$_f"
}

# Prints this project's fragments, one path per line, the README excluded.
fragment_files() {
  _d="$ROOT/$JOURNAL_FRAGMENTS"
  [ -d "$_d" ] || return 0
  for _f in "$_d"/*.md; do
    [ -f "$_f" ] || continue
    [ "$(basename "$_f")" = README.md ] && continue
    printf '%s\n' "$_f"
  done
}

# $1 = a fragment's path. Prints every row id it closes, one per line: its own (the file name),
# then each its **Also closes** names. The fold marks each merged; panel-suggest counts each landed.
# The field is read where fragment_problems reads it, the header, outside a fence: a fragment that
# quotes the line in its body (one describing this very field) closes no row by it.
fragment_rows() {
  basename "$1" .md
  awk 'NR == 1 { next } /^(```|~~~)/ { f = !f; next } f { next } /^## / { exit }
    /^- \*\*Also closes\*\*:/ { sub(/^- \*\*Also closes\*\*:[ \t]*/, ""); print; exit }' "$1" \
    | tr ',' '\n' | sed 's/^[[:space:]]*//; s/[[:space:]]*$//' | grep . || true
}
