# context.sh: sourced (never run) by the session-start and session-close context scripts, after
# conf.sh. Sections that read GitHub and the roadmap on origin/<main>, each printing its own lines
# and never nothing: a read that failed says CANNOT CHECK (this learned NOTHING), because an absent
# answer read as an empty one is the failure every context section exists to refuse. Upstreamed
# from a project that ran them across two people's sessions (P2.4). POSIX sh, git, gh, jq and awk.
#
#   ctx_open_prs                 the open PRs as gh JSON (number,title,headRefName,author,updatedAt,
#                                files); exit non-zero when they could not be read
#   ctx_loose_branches PRS_JSON  remote branches with no open PR: "branch · author · date"
#   ctx_queued_open PRS_JSON     open PRs whose roadmap row still reads queued on origin/<main>
#   ctx_tbd_specs                living specs still carrying TBD

# _ctx_online: 0 when gh and jq can be asked; else prints the CANNOT CHECK line.
_ctx_online() {
  if [ "${CONTEXT_OFFLINE:-}" = 1 ]; then echo "CANNOT CHECK — offline"; return 1; fi
  command -v gh >/dev/null 2>&1 && command -v jq >/dev/null 2>&1 && return 0
  echo "CANNOT CHECK — gh or jq is not installed; this learned NOTHING, do not read it as none"
  return 1
}

ctx_open_prs() {
  gh pr list --state open --limit 100 --json number,title,headRefName,author,updatedAt,files 2>/dev/null
}

# ctx_loose_branches PRS_JSON: every remote branch but the main one with no open PR, from GitHub
# (another person's branch is not in local refs until fetched), with its last commit's author and
# date from the fetched ref. Skipped when the PR list could not be read: then EVERY branch would
# look PR-less, a confident wrong answer.
ctx_loose_branches() {
  _ctx_online || return 0
  if [ -z "${1:-}" ]; then
    echo "CANNOT CHECK — the open-PR list could not be read, so every branch would look PR-less"; return 0
  fi
  if ! _cl_b=$(gh api --paginate 'repos/{owner}/{repo}/branches?per_page=100' --jq '.[].name' 2>/dev/null); then
    echo "CANNOT CHECK — gh could not list the remote branches; this learned NOTHING, do not read it as none"; return 0
  fi
  _cl_open=$(printf '%s' "$1" | jq -r '.[].headRefName' 2>/dev/null) || {
    echo "CANNOT CHECK — the open-PR list is not readable JSON"; return 0; }
  _cl_out=$(printf '%s\n' "$_cl_b" | while IFS= read -r _cl_n; do
    { [ -z "$_cl_n" ] || [ "$_cl_n" = "$MAIN_BRANCH" ]; } && continue
    printf '%s\n' "$_cl_open" | grep -qxF -- "$_cl_n" && continue
    git --no-optional-locks log -1 --format="$_cl_n · %an · %ad" --date=short "origin/$_cl_n" 2>/dev/null || echo "$_cl_n · (not fetched)"
  done)
  printf '%s\n' "${_cl_out:-none}"
}

# ctx_queued_open PRS_JSON: an open PR whose roadmap row still reads `⬜ queued` on origin/<main>.
# A row's in-flight status written only on its own branch leaves main going from queued straight
# to merged, so every reader of main (the change queue, the panel's suggestions, another person's
# session) sees work under way as not started; session-close sets each flagged row in flight.
#
# The row a PR names: the first row whose change id appears in its branch or title, bounded (not
# inside a longer id), else the first whose row id does (a title led by `PANEL-19:`). Read through
# roadmap_queue, so a project's own parser (ROADMAP_PARSER) answers; no second parser. A capability
# branch (BRANCH_CHANGE) that names no row is noted (a continuation branch?); fix/ and ops/ branches
# are matched but never noted, since most name no row.
ctx_queued_open() {
  _ctx_online || return 0
  [ -n "${1:-}" ] || { echo "CANNOT CHECK — gh could not list open PRs; this learned NOTHING"; return 0; }
  _cq_t=$(mktemp -d "${TMPDIR:-/tmp}/context.XXXXXX" 2>/dev/null) || { echo "CANNOT CHECK — mktemp failed"; return 0; }
  if ! git show "origin/$MAIN_BRANCH:$ROADMAP" > "$_cq_t/roadmap.md" 2>/dev/null; then
    echo "CANNOT CHECK — origin/$MAIN_BRANCH:$ROADMAP could not be read"; rm -rf "$_cq_t"; return 0
  fi
  # stderr apart: a parser's warning on a good read must not become a row.
  if ! roadmap_queue --tsv "$_cq_t/roadmap.md" > "$_cq_t/rows.tsv" 2>"$_cq_t/rows.err"; then
    echo "CANNOT CHECK — the roadmap on origin/$MAIN_BRANCH could not be parsed: $(cat "$_cq_t/rows.tsv" "$_cq_t/rows.err" | head -1)"; rm -rf "$_cq_t"; return 0
  fi
  if ! printf '%s' "$1" | jq -r '.[] | [(.number | tostring), .headRefName, (.title | gsub("\t"; " "))] | @tsv' > "$_cq_t/prs.tsv" 2>/dev/null; then
    echo "CANNOT CHECK — the open-PR list is not readable JSON"; rm -rf "$_cq_t"; return 0
  fi
  awk -F'\t' -v main="$MAIN_BRANCH" -v roadmap="$ROADMAP" -v rmfile="$_cq_t/roadmap.md" -v chg="$BRANCH_CHANGE" -v fix="$BRANCH_FIX" -v ops="$BRANCH_OPS" '
    # bounded TEXT TOKEN CLASS: TOKEN occurs in TEXT with no CLASS character on either side.
    function bounded(t, tok, cls,   p, off, b, a) {
      off = 0
      while (tok != "" && (p = index(substr(t, off + 1), tok)) > 0) {
        p += off; b = (p == 1) ? "" : substr(t, p - 1, 1); a = substr(t, p + length(tok), 1)
        if (b !~ cls && a !~ cls) return 1
        off = p
      }
      return 0
    }
    # clip S N: at most N characters, the last an ellipsis. Characters, not bytes: an awk that
    # counts bytes (mawk, macOS) would cut a status like "⬜ queued — …" short, or mid-glyph.
    function clip(s, n,   i, k, c, out) {
      if (length("⬜") == 1) return length(s) > n ? substr(s, 1, n - 1) "…" : s
      k = 0; for (i = 1; i <= length(s); i++) if (!(substr(s, i, 1) in CONT)) k++
      if (k <= n) return s
      k = 0; out = ""
      for (i = 1; i <= length(s); i++) { c = substr(s, i, 1); if (!(c in CONT) && ++k == n) break; out = out c }
      return out "…"
    }
    BEGIN { for (i = 128; i < 192; i++) CONT[sprintf("%c", i)] = 1 }
    FILENAME == ARGV[1] { n++; L[n] = $1; ID[n] = $4; CID[n] = $5; KIND[n] = $6; ST[n] = $7; next }
    { num[++m] = $1; br[m] = $2; ti[m] = $3 }
    END {
      for (i = 1; i <= m; i++) {
        # A branch names rows of its own lane only: `ops/add-alpha` is ops work, not row add-alpha.
        if (index(br[i], chg) == 1) pk = "change"
        else if (index(br[i], fix) == 1) pk = "fix"
        else if (index(br[i], ops) == 1) pk = "ops"
        else continue
        # The branch first, over every row; only then the title: a title that mentions an earlier
        # row ("add-beta on top of add-alpha") must not beat the row the branch itself names.
        r = 0
        for (j = 1; j <= n && !r; j++) if (KIND[j] == pk && bounded(br[i], CID[j], "[a-z0-9-]")) r = j
        for (j = 1; j <= n && !r; j++) if (KIND[j] == pk && bounded(ti[i], CID[j], "[a-z0-9-]")) r = j
        for (j = 1; j <= n && !r; j++) if (KIND[j] == pk && (bounded(br[i], ID[j], "[A-Za-z0-9_.]") || bounded(ti[i], ID[j], "[A-Za-z0-9_.]"))) r = j
        if (!r) { if (pk == "change") out[++o] = "note: #" num[i] " " br[i] " names no roadmap row (a continuation branch?)"; continue }
        if (ST[r] == "queued") { q[++nq] = i; qr[nq] = r; want[L[r]] = 1 }
      }
      # The status cell as written, read off the line the parser reports (column 1): the last cell,
      # whether or not the row ends in a pipe.
      while ((getline line < rmfile) > 0) {
        ln++
        if (!(ln in want)) continue
        s = line; gsub(/\\\|/, "\001", s); k = split(s, c, "|"); x = c[k]; if (x ~ /^[ \t]*$/ && k > 1) x = c[k - 1]
        gsub(/\001/, "|", x); sub(/^[ \t]+/, "", x); sub(/[ \t]+$/, "", x); status[ln] = x
      }
      if (nq == 0) print "none: no open PR'"'"'s row reads queued on " main
      for (k = 1; k <= nq; k++) {
        i = q[k]; r = qr[k]
        printf "QUEUED ON %s: #%s %s → %s %s reads \"%s\" — set its status to \"⬜ in flight (#%s)\" in %s in this close\n", toupper(main), num[i], br[i], ID[r], CID[r], clip(status[L[r]], 40), num[i], roadmap
      }
      for (k = 1; k <= o; k++) print out[k]
    }' "$_cq_t/rows.tsv" "$_cq_t/prs.tsv"
  rm -rf "$_cq_t"
}

# ctx_tbd_specs: living specs whose text still says TBD (an archive that promoted a spec with no
# Purpose writes one). Without SPECS_DIR there is nothing to promote into, which is not a finding.
ctx_tbd_specs() {
  [ -d "$SPECS_DIR" ] || { echo "none: no $SPECS_DIR/ here"; return 0; }
  _ct_l=$(grep -l TBD "$SPECS_DIR"/*/spec.md 2>/dev/null)
  printf '%s\n' "${_ct_l:-none}"
}
