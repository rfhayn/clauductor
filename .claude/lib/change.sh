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

# scenario_counts FILE: "<count>\t<requirement>" for EVERY requirement in FILE, in order, 0 included.
# Standing Tee's archive step counted with an awk that printed only the requirements it saw a
# scenario under, so a missing line there meant 0; this prints the 0, so no caller must remember.
scenario_counts() {
  awk '/^### Requirement:/ { n = $0; sub(/^### Requirement:[ \t]*/, "", n); sub(/[ \t]+$/, "", n); if (!(n in c)) { o[++k] = n; c[n] = 0 } next }
       /^#### Scenario:/ && n != "" { c[n]++ }
       END { for (i = 1; i <= k; i++) printf "%d\t%s\n", c[o[i]], o[i] }' "$1" 2>/dev/null
}

# spec_merge MODE CAP DELTA LIVING DESIGN: apply one capability's delta to its living spec.
#   MODE plan   one TSV line per operation: <op>\t<requirement>\t<detail>; op is NEW, RENAME,
#               REMOVE, REPLACE, MERGE, ADD, NOTE or STOP. A STOP means applying would be wrong.
#   MODE apply  the new living spec on stdout (run it only when plan printed no STOP).
# LIVING and DESIGN may be missing (a new capability; a change with no design.md).
#
# The operations run in OpenSpec's order: RENAMED (`- FROM: \`### Requirement: Old\`` then
# `- TO: \`### Requirement: New\``), REMOVED, MODIFIED, ADDED. A MODIFIED requirement replaces the
# living one whole only when the delta restates every living scenario (by ID when both carry one,
# else by title), or design.md names each one it leaves out (its ID, or its title in quotes,
# backticks or emphasis). Otherwise a literal replace would delete what the delta left out while
# `openspec validate --strict` passes the shrunken spec. A delta with FEWER scenarios than the living
# requirement restated only what it changed, so it is MERGED: the delta's body, then the living
# scenarios in their order (the delta's version where it restates one), then the delta's new ones.
# One with as many or more that still leaves a living scenario out is a reword or a drop, which only
# its author can tell apart: a STOP. The body follows the same count rule: in a merge, a
# delta body with fewer paragraphs than the living one is partial too, and which living paragraph
# it keeps is a judgement, so that is a STOP: restate the whole body in the delta (the record).
spec_merge() {
  _sm_l=$4; [ -f "$_sm_l" ] || _sm_l=/dev/null
  _sm_g=$5; [ -f "$_sm_g" ] || _sm_g=/dev/null
  awk -v mode="$1" -v cap="$2" -v dfile="$3" -v lfile="$_sm_l" -v gfile="$_sm_g" -v ere="$SCENARIO_ID_ERE" '
    function rname(s) { sub(/^### Requirement:[ \t]*/, "", s); sub(/[ \t]+$/, "", s); return s }
    function rnm(s) { sub(/.*### Requirement:[ \t]*/, "", s); sub(/`.*$/, "", s); sub(/[ \t]+$/, "", s); return s }
    function trim(s) { sub(/^\n+/, "", s); sub(/[ \t\n]+$/, "", s); return s }
    function stitle(h) { sub(/^#### Scenario:[ \t]*/, "", h); sub(/[ \t]+$/, "", h); if (match(h, "^\\[" ere "\\][ \t]*")) h = substr(h, RLENGTH + 1); return h }
    function sid(h) { sub(/^#### Scenario:[ \t]*/, "", h); if (match(h, "^\\[" ere "\\]")) return substr(h, 2, RLENGTH - 2); return "" }
    # same(D, m, L, k): delta scenario m restates living scenario k. By ID when both carry one; else
    # by title, since under SCENARIO_IDS=new-only a living scenario may have no ID that its restated
    # delta copy (which must) carries.
    function same(D, m, L, k) { if (D[m, "id"] != "" && L[k, "id"] != "") return D[m, "id"] == L[k, "id"]; return D[m, "title"] == L[k, "title"] }
    # named(K): design.md names living scenario k as removed: its ID, or its title in quotes,
    # backticks or emphasis (a bare substring would match unrelated prose).
    function named(L, k,    t) {
      t = L[k, "title"]
      if (L[k, "id"] != "" && index(design, L[k, "id"])) return 1
      return index(design, "\"" t "\"") || index(design, "`" t "`") || index(design, "*" t "*") || index(design, "“" t "”")
    }
    # An empty name prints as "-": the reader splits on tabs, and tabs are IFS whitespace, which collapses.
    function emit(op, name, detail) { if (mode == "plan") printf "%s\t%s\t%s\n", op, (name == "" ? "-" : name), detail }
    # parse(BLOCK, A): A["head"] (header and body), A["n"] scenarios, A[i,"id"], A[i,"title"], A[i,"hdr"], A[i,"text"].
    function parse(blk, A,    n, L, i, k) {
      for (i in A) delete A[i]
      n = split(blk, L, "\n"); A["head"] = ""; k = 0
      for (i = 1; i <= n; i++) {
        if (L[i] ~ /^#### Scenario:/) { k++; A[k, "hdr"] = L[i]; A[k, "id"] = sid(L[i]); A[k, "title"] = stitle(L[i]); A[k, "text"] = L[i]; continue }
        if (k == 0) A["head"] = A["head"] (i > 1 ? "\n" : "") L[i]; else A[k, "text"] = A[k, "text"] "\n" L[i]
      }
      A["n"] = k
    }
    # paragraphs(TEXT, P): the blank-line-separated paragraphs after the header line, blanks folded.
    function paragraphs(t, P,    n, L, i, cur, k) {
      for (i in P) delete P[i]
      n = split(t, L, "\n"); cur = ""; k = 0
      for (i = 2; i <= n + 1; i++) {
        if (i > n || L[i] ~ /^[ \t]*$/) { if (cur != "") P[++k] = cur; cur = ""; continue }
        cur = cur (cur == "" ? "" : " ") L[i]
      }
      for (i = 1; i <= k; i++) { gsub(/[ \t]+/, " ", P[i]); sub(/^ /, "", P[i]); sub(/ $/, "", P[i]) }
      return k
    }
    FILENAME == gfile { design = design $0 "\n"; next }
    FILENAME == dfile {
      if ($0 ~ /^## (ADDED|MODIFIED|REMOVED|RENAMED) Requirements/) { sec = $2; cur = ""; inp = 0; next }
      if ($0 ~ /^## /) { sec = ""; cur = ""; inp = ($0 ~ /^## Purpose/); next }
      if (sec == "") { if (inp) purpose = purpose $0 "\n"; next }
      if (sec == "RENAMED") {
        if ($0 ~ /FROM:/) { rf[++rn] = rnm($0); rt[rn] = "" }
        else if ($0 ~ /TO:/ && rn) rt[rn] = rnm($0)
        next
      }
      if ($0 ~ /^### Requirement:/) { cur = rname($0); k = sec SUBSEP cur; if (!(k in dblk)) dord[sec, ++dn[sec]] = cur; dblk[k] = $0; next }
      if (cur != "") dblk[sec SUBSEP cur] = dblk[sec SUBSEP cur] "\n" $0
      next
    }
    # The living spec: before, the "## Requirements" line, its preamble, the requirement blocks, and
    # after (from the next "## " heading). Rebuilt the way OpenSpec 1.2 rebuilds it, so a project
    # moving between the CLI and this script sees no whitespace churn.
    FILENAME == lfile {
      if (!seen) cur = ""
      seen = 1
      if (st == 0) { if ($0 ~ /^##[ \t]+Requirements[ \t]*$/) { st = 1; hdr = $0 } else before = before $0 "\n"; next }
      if (st == 1) {
        if ($0 ~ /^### Requirement:/) { cur = rname($0); lord[++ln] = cur; lblk[cur] = $0; next }
        if ($0 ~ /^##[ \t]/) { st = 2; after = $0; next }
        if (cur != "") lblk[cur] = lblk[cur] "\n" $0; else preamble = preamble $0 "\n"
        next
      }
      after = after "\n" $0
    }
    END {
      newcap = !seen
      if (!newcap && st == 0) { emit("STOP", "", "the living spec has no \"## Requirements\" heading to merge into"); exit }
      if (newcap) {
        p = trim(purpose)
        if (p == "" || p ~ /^TBD/) emit("STOP", "", "a new capability, and the delta has no ## Purpose to create it with (never a placeholder)")
        else emit("NEW", "", "a new capability, created with the Purpose the delta states")
        before = "# " cap " Specification\n\n## Purpose\n" p "\n\n"; hdr = "## Requirements"
      }
      for (i = 1; i <= rn; i++) {
        if (rt[i] == "") { emit("STOP", rf[i], "a RENAMED FROM line with no TO line"); continue }
        if (!(rf[i] in lblk)) { emit("STOP", rf[i], "RENAMED, but the living spec has no such requirement"); continue }
        if (rt[i] in lblk) { emit("STOP", rt[i], "RENAMED to a name the living spec already has"); continue }
        # Concatenation, not sub(): an & or \ in the new name is literal text, not a back-reference.
        b = lblk[rf[i]]; p = index(b, "\n"); b = "### Requirement: " rt[i] (p ? substr(b, p) : ""); lblk[rt[i]] = b; delete lblk[rf[i]]
        for (j = 1; j <= ln; j++) if (lord[j] == rf[i]) lord[j] = rt[i]
        emit("RENAME", rt[i], "from \"" rf[i] "\"")
      }
      for (i = 1; i <= dn["REMOVED"]; i++) {
        nme = dord["REMOVED", i]
        if (!(nme in lblk)) { emit("STOP", nme, "REMOVED, but the living spec has no such requirement"); continue }
        delete lblk[nme]; emit("REMOVE", nme, "")
      }
      for (i = 1; i <= dn["MODIFIED"]; i++) {
        nme = dord["MODIFIED", i]; d = dblk["MODIFIED" SUBSEP nme]
        if (!(nme in lblk)) { emit("STOP", nme, "MODIFIED, but the living spec has no such requirement" (rn ? " (after the renames)" : "")); continue }
        parse(d, D); parse(lblk[nme], L)
        dropped = ""; nd = 0; unnamed = 0
        for (k = 1; k <= L["n"]; k++) {
          hit = 0; for (m = 1; m <= D["n"]; m++) if (same(D, m, L, k)) { hit = 1; break }
          if (!hit) { nd++; dropped = dropped (dropped == "" ? "" : "; ") L[k, "title"]; if (!named(L, k)) unnamed++ }
        }
        if (nd == 0) {
          lblk[nme] = d
          emit("REPLACE", nme, D["n"] " scenario(s), restating all " L["n"] " of the living requirement")
          continue
        }
        if (unnamed == 0) {
          lblk[nme] = d
          emit("REPLACE", nme, D["n"] " scenario(s); design.md names each living one dropped: " dropped)
          continue
        }
        # Not restated and not named: with FEWER scenarios the delta is partial (merge below); with as
        # many or more it is either a reword or a drop, and only the author knows which.
        if (D["n"] >= L["n"]) { emit("STOP", nme, "the delta does not restate " nd " living scenario(s), and design.md does not name them as removed: " dropped ". Restate each (word for word, or by its ID), or name it in design.md (its ID, or its title in quotes)"); continue }
        # The body by the same rule as the scenarios: a delta body with fewer paragraphs than the
        # living one is partial, and which living paragraph it means to keep is a judgement.
        np = paragraphs(L["head"], LP); ndp = paragraphs(D["head"], DP); lost = ""
        if (ndp < np) for (k = 1; k <= np; k++) { found = 0; for (m in DP) if (DP[m] == LP[k]) { found = 1; break }; if (!found) lost = lost (lost == "" ? "" : " | ") substr(LP[k], 1, 70) }
        if (lost != "") { emit("STOP", nme, "a partial MODIFIED (" D["n"] " of " L["n"] " scenarios) whose body leaves out living paragraph(s): " lost ". Restate the whole body in the delta, or name each dropped scenario in design.md"); continue }
        out = trim(D["head"])
        for (k = 1; k <= L["n"]; k++) {
          t = L[k, "text"]; for (m = 1; m <= D["n"]; m++) if (same(D, m, L, k)) { t = D[m, "text"]; used[m] = 1; break }
          out = out "\n\n" trim(t)
        }
        for (m = 1; m <= D["n"]; m++) if (!(m in used)) out = out "\n\n" trim(D[m, "text"])
        for (m in used) delete used[m]
        lblk[nme] = out
        emit("MERGE", nme, "a partial delta restates " D["n"] " of " L["n"] " scenario(s); kept from the living spec: " dropped)
      }
      for (i = 1; i <= dn["ADDED"]; i++) {
        nme = dord["ADDED", i]
        if (nme in lblk) { emit("STOP", nme, "ADDED, but the living spec already has it"); continue }
        lblk[nme] = dblk["ADDED" SUBSEP nme]; lord[++ln] = nme; emit("ADD", nme, "")
      }
      if (mode != "apply") exit
      body = preamble; sub(/[ \t\n]+$/, "", body); if (body !~ /[^ \t\n]/) body = ""
      if (after != "") after = after "\n"
      for (j = 1; j <= ln; j++) if (lord[j] in lblk) { body = body (body == "" ? "" : "\n\n") trim(lblk[lord[j]]); delete lblk[lord[j]] }
      b = before; sub(/[ \t\n]+$/, "", b)
      out = (b == "" ? "" : b "\n") hdr "\n" body "\n" (after ~ /^\n/ ? after : "\n" after)
      while (gsub(/\n\n\n/, "\n\n", out)) {}
      printf "%s", out
    }' "$_sm_g" "$3" "$_sm_l"
}
