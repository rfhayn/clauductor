# change.sh: sourced (never run) by the scripts that read a change record: checks/changes.sh,
# .claude/scenario-trace.sh, .claude/verify-change.sh, .claude/change-approval.sh,
# .claude/panel-suggest.sh and pr-merge-guard. ONE reading of the change format (changes/README.md), so two scripts cannot
# disagree about what a scenario ID or an open task is (*A check that reads source is a parser*).
#
# Every function reads a file on stdin or by path (branch_proposals, the repository's refs) and
# prints plain lines; none of them exits.

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

# row_is_table ID: whether the roadmap line on stdin is row ID of the built-in queue table, as
# roadmap-queue.sh reads one: a leading `|`, exactly five cells (an escaped `\|` being text), and ID
# in the first. A project's own ROADMAP_PARSER may name a line of another shape, whose cells
# row_change_cell cannot find: a reader of a row's Risk or Approval refuses it, never reads none.
row_is_table() {
  awk -v r="$1" '{
    gsub(/\\\|/, " "); n = split($0, c, "|"); id = c[2]; t = c[7]
    gsub(/^[ \t]+|[ \t]+$/, "", id); gsub(/[ \t]/, "", t)
    ok = ($0 ~ /^\|/ && n == 7 && t == "" && id == r)
  } END { exit !ok }'
}

# table_rows_naming CHANGE: the line numbers of the queue table's rows (row_is_table's shape) on
# stdin whose Change cell's first backticked token is CHANGE, as the built-in parser reads a row's
# change id. A guard checks the line a ROADMAP_PARSER chose against this: the parser runs from the
# branch's disk, so a branch could point it at a laxer row.
table_rows_naming() {
  awk -v ch="$1" '/^\|/ {
    s = $0; gsub(/\\\|/, " ", s); n = split(s, c, "|"); t = c[7]; gsub(/[ \t]/, "", t)
    if (n == 7 && t == "" && match(c[3], /`[^`]+`/) && substr(c[3], RSTART + 1, RLENGTH - 2) == ch) print NR
  }'
}

# marker_plain [--prose]: stdin as a reader sees it rendered, for finding a marker (`Approval:`,
# `Risk:`). It reads left to right as CommonMark does: a backslash escape is its character, a code
# span is text (a tag or an entity inside it is not markup), and a `<` starts markup only where it
# opens a real tag (`<b>`, `<span title="a>b">`, `</b>`), a comment (`<!-- -->`), an instruction
# (`<?…?>`), a declaration or CDATA; those go (`Appro<b></b>val`), and any other `<` is text, so
# `p95 < 50ms … > 3` hides nothing between. A link reads as its text (`[Approval](x)`,
# `[[Risk]](x)`). Its destination is read three ways, a line each: dropped where a pattern (lk)
# finds a link, so text that is no link (`[x](Approval: owner)`) stays; dropped wherever CommonMark
# closes it, since lk misses some links CommonMark makes; and dropped to its first `)`, as the
# reader before OPS-131 dropped it. A reference label, which renders only where a definition names
# it, is read both ways too. Every entity is decoded: a numeric one to its ASCII character
# (`Ri&#115;k`), a markup character (`` ` * _ | \ < > & ``, which renders as itself, never as
# markup) or a space to a space; a named one as its code point is (`&colon;`, `&nbsp;`, `&mdash;`),
# and a name not listed (`&ratio;`) or a control character as what is not ASCII. Then what is not
# ASCII: an invisible character (zero-width, soft hyphen, BOM) is nothing; a space, a dash, a
# quote, a bullet, a middle dot or an arrow is a space; a colon look-alike (`：`, `∶`) is a colon;
# and anything else is nothing inside a word (`Appro&#847;val`, a combining joiner) and a colon
# outside one or right after `approval` or `risk` (`Approvalːowner`: a marker, so a reader refuses
# rather than miss one). Letters that only look Latin (`Аpproval`, a Cyrillic А) are not read:
# /propose step 3a's judgment. A table line (the default) is read a cell at a time, an escaped `\|`
# being text, as roadmap-queue.sh splits it, so markup never spans two cells. --prose reads
# paragraphs, a blank line apart, each joined into one line as it renders. Bytes, not characters
# (LC_ALL=C), so it reads the same in every locale.
marker_plain() {
  LC_ALL=C awk -v prose="$([ "${1:-}" = --prose ] && echo 1)" '
  function dec(n) {
    if (n == 124 || n == 96 || n == 42 || n == 95 || n == 92 || n == 60 || n == 62 || n == 38) return " "
    if (n > 32 && n < 127) return sprintf("%c", n)
    if (n == 9 || n == 10 || n == 12 || n == 13 || n == 32 || n == 160 || (n >= 8192 && n <= 8202) || n == 8239 || n == 8287 || n == 12288) return " "
    if ((n >= 8208 && n <= 8223) || n == 8226 || n == 8230 || n == 183 || (n >= 8592 && n <= 8703)) return " "
    if (n == 65306 || n == 65109 || n == 65043 || n == 42889 || n == 8758 || n == 760) return ":"
    if ((n >= 8203 && n <= 8207) || (n >= 8288 && n <= 8292) || n == 65279 || n == 173) return ""
    return "\200"  # any other character, a control or NUL too: what is not ASCII is read below
  }
  function num(e,   i, v, h, d) {  # e: the digits after &#, decimal or x-prefixed hex
    v = 0; h = (e ~ /^[xX]/); if (h) e = substr(e, 2)
    for (i = 1; i <= length(e); i++) {
      d = index("0123456789abcdef", tolower(substr(e, i, 1))) - 1
      v = v * (h ? 16 : 10) + d; if (v > 1114111) return 1114112
    }
    return v
  }
  function closer(s, n,   j, r) {  # where the first run of exactly n backticks in s starts, or 0
    j = 1
    while (match(substr(s, j), /`+/)) {
      r = j + RSTART - 1
      if (RLENGTH == n) return r
      j = r + RLENGTH
    }
    return 0
  }
  function skip(s,   j) {  # past the comment, instruction, CDATA or declaration s opens, or 0
    if (substr(s, 1, 5) == "<!-->") return 6
    if (substr(s, 1, 6) == "<!--->") return 7
    if (substr(s, 1, 4) == "<!--") return (j = index(substr(s, 5), "-->")) ? j + 7 : 0
    if (substr(s, 1, 2) == "<?") return (j = index(substr(s, 3), "?>")) ? j + 4 : 0
    if (substr(s, 1, 9) == "<![CDATA[") return (j = index(substr(s, 10), "]]>")) ? j + 12 : 0
    if (s ~ /^<![A-Za-z]/) return (j = index(s, ">")) ? j + 1 : 0
    return 0
  }
  # html: s with its markup gone, read in CommonMark order. What a later step must not read as
  # markup (an escaped character, a code span`s `&` and `]`) is followed by \002, which plain drops.
  # In a table line a code span is wrapped in \003, so row_change_cell drops only a real one.
  function html(s,   o, c, n, r, t) {
    o = ""
    while (s != "") {
      c = substr(s, 1, 1)
      if (c == "\\" && length(s) > 1 && index(pu, substr(s, 2, 1))) {
        c = substr(s, 2, 1); o = o (c == "`" ? q : c "\002"); s = substr(s, 3); continue
      }
      if (c == "`") {
        match(s, /^`+/); n = RLENGTH
        if (r = closer(substr(s, n + 1), n)) {
          t = substr(s, 1, r + 2 * n - 1); gsub(/&|\]/, "&\002", t)
          o = o (prose ? t : "\003" t "\003"); s = substr(s, r + 2 * n); continue
        }
        o = o substr(s, 1, n); s = substr(s, n + 1); continue
      }
      if (c == "<") {
        if (r = skip(s)) { s = substr(s, r); continue }
        if (match(s, otag) || match(s, ctag)) { s = substr(s, RLENGTH + 1); continue }
      }
      o = o c; s = substr(s, 2)
    }
    return o
  }
  function wide(s,   o, a) {  # each run of bytes outside ASCII: nothing inside a word, else a colon
    o = ""
    while (match(s, nb)) {
      o = o substr(s, 1, RSTART - 1); a = substr(o, length(o))
      if (!(a ~ /[A-Za-z0-9]/ && substr(s, RSTART + RLENGTH, 1) ~ /[A-Za-z0-9]/) \
        || tolower(substr(o, length(o) - 7)) ~ /(approval|risk)$/) o = o ":"
      s = substr(s, RSTART + RLENGTH)
    }
    return o s
  }
  # dest: where the `)` closing the destination s opens (s starts at its `(`) is, by CommonMark
  # grammar, or 0: a <…> destination, or a bare one with no space, a `(` in it needing its `)`;
  # then a title in quotes or parens after a space. An escaped character (html follows it with \002)
  # counts for nothing; in a table line a `|` ends the cell, so the destination never closes. It may
  # close one CommonMark leaves open (`(a(b "t")`): safe, since reading 0 keeps that text.
  function dest(s,   n, j, c, t) {
    n = length(s); j = 2
    while (substr(s, j, 1) ~ /[ \t]/) j++
    if (substr(s, j, 1) == "<") {
      for (j++; j <= n; j++) {
        c = substr(s, j, 1)
        if (substr(s, j + 1, 1) == "\002") { j++; continue }
        if (!prose && c == "|") return 0
        if (c == ">") break
      }
      if (j++ > n) return 0
    } else {
      for (t = 0; j <= n; j++) {
        c = substr(s, j, 1)
        if (substr(s, j + 1, 1) == "\002") { j++; continue }
        if (c ~ /[ \t]/ || (!prose && c == "|")) break
        if (c == "(") t++
        else if (c == ")" && t-- == 0) return j
      }
    }
    if (substr(s, j, 1) ~ /[ \t]/) {
      while (substr(s, j, 1) ~ /[ \t]/) j++
      if ((c = substr(s, j, 1)) != "" && index("\"" q "(", c)) {
        if (c == "(") c = ")"
        for (j++; j <= n; j++) {
          if (substr(s, j + 1, 1) == "\002") { j++; continue }
          if (!prose && substr(s, j, 1) == "|") return 0
          if (substr(s, j, 1) == c) break
        }
        if (j++ > n) return 0
        while (substr(s, j, 1) ~ /[ \t]/) j++
      }
    }
    return substr(s, j, 1) == ")" ? j : 0
  }
  # plain: s, from html, as text. A link reads as its text, `[Approval](x):` being `[Approval]:` (the
  # caller drops the brackets). Three readings of its destination, which the caller unites (how):
  # 0, only a link lk matches, an opener then a destination with no space and at most a quoted
  # title, so `note](Approval: owner)` keeps its words; 1, every `](` whose destination CommonMark
  # closes (dest), which lk misses where the text holds a bracket or an escape or the destination a
  # paren (`[[Approval]](x)`, `[\*\*Risk\*\*](x)`, `[Risk](<a (b>)`); 2, every `](` to its first
  # `)`, as the reader before OPS-131 read it, so no reading here is ever weaker than that one: it
  # errs toward a marker (`[Risk](a_(b)):` is `Risk):`, which the caller refuses). The label of a
  # reference (`[x][Approval: owner]`) renders only where a definition names it, which is not read,
  # so it is read both ways too: the label gone, and kept (ref).
  function plain(s, ref, how,   o, e, m, i, j) {
    if (how == 1) {
      o = ""
      while (i = index(s, "](")) {
        o = o substr(s, 1, i); s = substr(s, i + 1)
        if (j = dest(s)) s = substr(s, j + 1)
      }
      s = o s
    }
    if (how == 2) gsub(old, "]", s)
    o = ""
    while (match(s, lk)) {
      m = substr(s, RSTART, RLENGTH); o = o substr(s, 1, RSTART - 1) substr(m, 1, index(m, "]"))
      s = substr(s, RSTART + RLENGTH)
    }
    s = o s
    if (ref) gsub(/\]\[/, "] [", s); else gsub("\\]\\[[^]" bar "]*\\]", "]", s)
    o = ""
    while (match(s, /&(#[0-9]+|#[xX][0-9a-fA-F]+|[A-Za-z][A-Za-z0-9]*);/)) {
      e = substr(s, RSTART + 1, RLENGTH - 2); o = o substr(s, 1, RSTART - 1)
      if (e ~ /^#/) o = o dec(num(substr(e, 2)))
      else o = o (e in ent ? dec(ent[e]) : "\200")
      s = substr(s, RSTART + RLENGTH)
    }
    s = o s
    gsub(zw, "", s); gsub(sp, " ", s); gsub(pn, " ", s); gsub(co, ":", s); gsub(/\002/, "", s); return wide(s)
  }
  function cells(s,   n, c, i) {  # the escaped pipes of a table line back
    n = split(s, c, "\001"); s = c[1]
    for (i = 2; i <= n; i++) s = s "\\|" c[i]
    return s
  }
  BEGIN {
    zw = "\342\200[\213-\217]|\342\201[\240-\244]|\357\273\277|\302\255"
    sp = "\302\240|\342\200[\200-\212]|\342\200\257|\342\201\237|\343\200\200"
    pn = "\342\200[\220-\237]|\342\200[\242\246]|\302\267|\342\206[\220-\277]|\342\207[\200-\277]"
    co = "\357\274\232|\357\271\225|\357\270\223|\352\236\211|\342\210\266|\313\270"
    nb = "[\200-\377]+"
    q = "\047"; pu = "!\"#$%&" q "()*+,-./:;<=>?@[\\]^_`{|}~"
    # A tag as CommonMark reads one: a name, then attributes, a quoted value whole (`title="a>b"`).
    av = "([^ \t\"" q "=<>`]+|\"[^\"]*\"|" q "[^" q "]*" q ")"
    otag = "^<[A-Za-z][A-Za-z0-9-]*([ \t]+[A-Za-z_:][A-Za-z0-9_.:-]*([ \t]*=[ \t]*" av ")?)*[ \t]*/?>"
    ctag = "^</[A-Za-z][A-Za-z0-9-]*[ \t]*>"
    # A `|` ends a cell, so in a table line no link or destination crosses one; in prose it is text.
    bar = prose ? "" : "|"
    lk = "\\[[^][\002" bar "]*\\]\\((<[^<>" bar "]*>|[^ \t()<>" bar "]*)([ \t]+(\"[^\"" bar "]*\"|" q "[^" q bar "]*" q "|\\([^()" bar "]*\\)))?[ \t]*\\)"
    old = "\\]\\([^)" bar "]*\\)"
    # The named entities prose meets, read as their code points are (dec); any other name is read as
    # what is not ASCII, so `Approval&ratio; owner` is a marker and `Appro&DownBreve;val:` one word.
    n = split("colon 58 Tab 9 NewLine 10 nbsp 160 NonBreakingSpace 160 ensp 8194 emsp 8195 emsp13 8196" \
      " emsp14 8197 numsp 8199 puncsp 8200 thinsp 8201 ThinSpace 8201 hairsp 8202 VeryThinSpace 8202" \
      " MediumSpace 8287 amp 38 AMP 38 lt 60 LT 60 gt 62 GT 62 quot 34 QUOT 34 apos 39 ast 42 midast 42" \
      " lowbar 95 UnderBar 95 grave 96 DiacriticalGrave 96 verbar 124 vert 124 VerticalLine 124 bsol 92" \
      " lsqb 91 lbrack 91 rsqb 93 rbrack 93 excl 33 num 35 dollar 36 percnt 37 lpar 40 rpar 41 plus 43" \
      " comma 44 period 46 sol 47 semi 59 equals 61 quest 63 commat 64 Hat 94 lcub 123 lbrace 123" \
      " rcub 125 rbrace 125 hyphen 8208 dash 8208 ndash 8211 mdash 8212 horbar 8213 lsquo 8216" \
      " OpenCurlyQuote 8216 rsquo 8217 rsquor 8217 CloseCurlyQuote 8217 sbquo 8218 lsquor 8218" \
      " ldquo 8220 OpenCurlyDoubleQuote 8220 rdquo 8221 rdquor 8221 CloseCurlyDoubleQuote 8221 bdquo 8222" \
      " ldquor 8222 bull 8226 bullet 8226 hellip 8230 mldr 8230 middot 183 centerdot 183 CenterDot 183" \
      " larr 8592 uarr 8593 rarr 8594 darr 8595 harr 8596 lArr 8656 uArr 8657 rArr 8658 dArr 8659" \
      " hArr 8660 shy 173 zwnj 8204 zwj 8205 lrm 8206 rlm 8207 ZeroWidthSpace 8203" \
      " NegativeVeryThinSpace 8203 NegativeThinSpace 8203 NegativeMediumSpace 8203" \
      " NegativeThickSpace 8203 NoBreak 8288 ApplyFunction 8289 af 8289 InvisibleTimes 8290 it 8290" \
      " InvisibleComma 8291 ic 8291", c, " ")
    for (i = 1; i < n; i += 2) ent[c[i]] = c[i + 1]
    # A control character renders as nothing a reader sees: it is read as what is not ASCII too.
    ctl = "[\001-\010\013\016-\037\177]"
    if (prose) RS = ""
  }
  { gsub(ctl, "\200") }
  function each(s,   k, r, seen) {  # each distinct reading: three of a link, its label gone or kept
    for (k = 0; k < 6; k++) {
      r = plain(s, k % 2, int(k / 2)); if (!prose) r = cells(r)
      if (!(r in seen)) { seen[r] = 1; print r }
    }
  }
  prose { gsub(/\n/, " "); each(html($0)); next }
  {
    s = $0; gsub(/\\\|/, "\001", s); n = split(s, c, "|"); s = html(c[1])
    for (i = 2; i <= n; i++) s = s "|" html(c[i])
    each(s)
  }'
}

# row_change_cell: the Change cell (cell 2 of `| # | Change | Scope | Deps | Status |`) of the
# roadmap table line on stdin (row_is_table), as plain text (marker_plain first). A backticked span
# holding a whole marker, key, colon and value (`Risk: high`, `Approval: owner`), is the row talking
# ABOUT a marker, not stating its own, so it goes; every other span stays, and then the markdown
# marks (`*`, `_`, backticks) go, so emphasis or a span around only part of a marker (Risk:
# `high`, `Approval:` owner) cannot hide it. The cells are split as roadmap-queue.sh splits them,
# an escaped `\|` being text, and the spans read only after, so a stray backtick cannot pair with
# one in the next cell. A span is one marker_plain found (wrapped in \003), opened and closed by
# runs of one length, so a lone backtick before a marker pairs with nothing after it. review-cap.sh
# reads a row's Risk from it, and change-approval.sh its Risk and `Approval:`.
row_change_cell() {
  marker_plain | awk '{
    gsub(/\\\|/, " "); split($0, c, "|"); x = c[3]; y = ""
    while (match(x, /\003[^\003]*\003/)) {
      s = substr(x, RSTART, RLENGTH); y = y substr(x, 1, RSTART - 1)
      if (tolower(s) !~ /(risk|approval)[^a-z0-9]*:[^a-z0-9]*[a-z0-9]/) y = y s
      x = substr(x, RSTART + RLENGTH)
    }
    y = y x; gsub(/[`*_\003]|\[|\]/, "", y); print y
  }'
}

# row_risks: each distinct `Risk: low|normal|high` in the Change cell of the roadmap line on stdin,
# one per line (none: the row is normal; more than one: its risk cannot be read).
row_risks() { row_change_cell | grep -oE 'Risk: (low|normal|high)' | sed 's/^Risk: //' | sort -u; }

# row_risk_odd: the first Risk in the Change cell of the roadmap line on stdin that is not read as
# `Risk: low|normal|high`, or nothing. Any other Risk key (`Risk: High`, `Risk: critical`, `Risk:
# very high`, `Risk -: high`), or a lowercase one naming a tier (`risk:high`), is a floor the row
# meant to set, so a reader refuses it rather than read the row as normal. Lowercase prose naming no
# tier (`the risk: a stale cache`) is not a Risk. A value runs to a space or a sentence's mark, so
# `Risk: low-ish` and `Risk: low/high` are odd, `Risk: high.` is not. change-approval.sh and
# review-cap.sh refuse on it.
row_risk_odd() {
  _ro_c=$(row_change_cell)
  { printf '%s\n' "$_ro_c" | grep -oiE 'risk[^[:alnum:]]*:[^[:alnum:]]*(low|normal|high)[^[:space:].,;:)]*'
    printf '%s\n' "$_ro_c" | grep -oE 'R[Ii][Ss][Kk][^[:alnum:]]*:[^[:alnum:]]*[^[:space:].,;:)]*'; } \
    | grep -vxE 'Risk: (low|normal|high)' | head -1
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

# proposal_budget FILE: the `**Budget:**` line, read ONE way for every reader (change-cost.sh, and
# through it build-change; checks/changes.sh; metrics.sh). Whether a budget is soft is the line's
# own words, never the design's prose: `$N` and `$N, hard` print "N hard" (an overrun stops the
# build), `$N, soft` prints "N soft" (an overrun is reported and the build goes on), no line prints
# nothing, and any other line prints "unreadable": a budget nobody can read is said, never taken
# as hard or soft (OPS-85). The line is a copy: the roadmap row is the authority for its figure and
# its kind (`Budget: $N, soft`, --tsv column 16), and checks/changes.sh fails a line that differs
# from the row as origin/<main> has it, so a `, soft` a branch adds is refused (OPS-91).
proposal_budget() {
  _pb=$(proposal_field "$1" Budget)
  [ -n "$_pb" ] || return 0
  _pb=$(printf '%s\n' "$_pb" | sed -nE 's/^\$([0-9]+(\.[0-9][0-9]?)?)(,[[:space:]]*(soft|hard))?$/\1 \4/p')
  case $_pb in
    '') echo unreadable ;;
    *' ') echo "${_pb% } hard" ;;
    *) echo "$_pb" ;;
  esac
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
    # hasid(S, ID): ID occurs in S as a whole ID (AUTH-1-S1 is not in AUTH-1-S10 or OAUTH-1-S1).
    function hasid(s, id,    p, b, a) {
      while ((p = index(s, id)) > 0) {
        b = (p > 1) ? substr(s, p - 1, 1) : ""; a = substr(s, p + length(id), 1)
        if (b !~ /[A-Za-z0-9-]/ && a !~ /[A-Za-z0-9-]/) return 1
        s = substr(s, p + 1)
      }
      return 0
    }
    function named(L, k,    t) {
      t = L[k, "title"]
      if (L[k, "id"] != "" && hasid(design, L[k, "id"])) return 1
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
      # A new capability has no OpenSpec layout to match (it would write a TBD Purpose), so its
      # Purpose stays a paragraph of its own.
      b = before; sub(/[ \t\n]+$/, "", b)
      out = (b == "" ? "" : b (newcap ? "\n\n" : "\n")) hdr "\n" body "\n" (after ~ /^\n/ ? after : "\n" after)
      while (gsub(/\n\n\n/, "\n\n", out)) {}
      printf "%s", out
    }' "$_sm_g" "$3" "$_sm_l"
}

# branch_proposals SKIP OPEN: the proposals in progress on change branches (local and origin), one
# line per change id: "<id>\t<approved|waiting>\t<branch>". On main a proposal is approved
# (checks/changes.sh refuses one that is not), so one in progress lives on a branch. SKIP lists the
# ids main already has, live or archived; OPEN the change ids of the roadmap's open rows (queued, in
# flight or open), since an old branch still carries a merged or cancelled one. An id is waiting
# (for the owner) only while no ref carries its **Approved:** line, so a stale local copy never
# hides an approval pushed since; <branch> is the first approved ref, else the first. Paths are
# `./`-relative, so run it from the project root: a project in a subdirectory of its repository
# reads its own. ONE reading for panel-suggest.sh and propose's context, so the two cannot disagree.
branch_proposals() {
  for _bp_ref in $(git for-each-ref --format='%(refname:short)' "refs/heads/${BRANCH_CHANGE}*" "refs/remotes/origin/${BRANCH_CHANGE}*" 2>/dev/null); do
    for _bp_id in $(git ls-tree --name-only "$_bp_ref" "./$CHANGES_DIR/" 2>/dev/null | sed 's|.*/||'); do
      case " $1 " in *" $_bp_id "*) continue ;; esac
      case " $2 " in *" $_bp_id "*) ;; *) continue ;; esac
      _bp_st=waiting
      git show "$_bp_ref:./$CHANGES_DIR/$_bp_id/proposal.md" 2>/dev/null | head -n 3 | grep -q '^\*\*Approved:\*\*' && _bp_st=approved
      printf '%s\t%s\t%s\n' "$_bp_id" "$_bp_st" "$_bp_ref"
    done
  done | awk -F'\t' '!($1 in st) { o[++n] = $1; st[$1] = $2; br[$1] = $3; next }
    $2 == "approved" && st[$1] == "waiting" { st[$1] = "approved"; br[$1] = $3 }
    END { for (i = 1; i <= n; i++) printf "%s\t%s\t%s\n", o[i], st[o[i]], br[o[i]] }'
}
