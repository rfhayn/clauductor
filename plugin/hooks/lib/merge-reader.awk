# merge-reader.awk — reads ONE shell command (on stdin) the way the shell would, for
# pr-merge-guard.sh. Not a shell: a tokenizer that knows quotes, escapes, backslash-newline,
# `$(…)`, backticks, `${…}`, `$name`, `$'…'`, globs, brace expansion, zsh `=cmd`, heredocs,
# comments, redirections and separators — and that, where it cannot know, says so (ODD) rather
# than guess. POSIX awk only: the hook runs under BSD awk on a Mac and mawk in the Debian CI image.
#
# Every word is kept four ways: W its text (quotes removed), F how it was spelled (q quoted,
# e escaped, d computed by an expansion, g globbed, b brace-expanded, s unquoted expansion or glob,
# which can split into several words), P a regex of every string it
# could become, K its literal characters. A word "could be" gh, pr or merge when P matches it.
#
# Output, one line per finding:
#   PLAIN <n>        n readable `gh pr merge` sites — all three words plain, adjacent
#   API <repo> <pr>  a readable `gh api -X PUT repos/<o>/<n>/pulls/<pr>/merge` (repo `.` = this one)
#   ODD <why>        a merge, or a construct around one, this reader cannot read: the hook blocks
#   MENTION          merge words inside ONE word or heredoc body: prose, or code for an evaluator
#   UNSAFE <word>    with a MENTION, a command not on the inert list (prose_safe)

function norm(t) { gsub(/\\/, "", t); gsub(/"/, "", t); gsub(/'/, "", t); gsub(/[ \t\r\n]+/, " ", t); return t }
function hits(t,   l) {
  t = " " norm(t) " "
  if (t ~ /[^A-Za-z0-9_-]pr( -[^ ]+( [^ -][^ ]*)?)* merge[^A-Za-z0-9_-]/) return 1
  if (t ~ /([)}`]|[$][A-Za-z0-9_@*#?!-]+) merge[^A-Za-z0-9_-]/) return 1
  if (t ~ /[^A-Za-z0-9_-]pr [$`]/) return 1
  l = tolower(t)
  return l ~ /pulls\/[^ ]*\/merge|mergepullrequest|automerge/
}
function odd(why) { if (!(why in SEEN)) { SEEN[why] = 1; ODDS[++NO] = why } }
# A construct the reader could not follow. Reported only when the command could concern a merge.
function broken(why) { if (!(why in SEEN)) { SEEN[why] = 1; BROKE[++NB] = why } }
function newcmd() { N[++NC] = 0; return NC }

# rx — a regex matching exactly s.
function rx(s,   o, i, c) {
  if (s !~ /[\\^]/) { gsub(/[^A-Za-z0-9_]/, "[&]", s); return s }
  o = ""
  for (i = 1; i <= length(s); i++) {
    c = substr(s, i, 1)
    if (c ~ /[A-Za-z0-9_]/) o = o c
    else if (c == "\\") o = o "\\\\"
    else if (c == "^") o = o "\\^"
    else o = o "[" c "]"
  }
  return o
}

# The word being built: tok (text), fl (flags), pat (regex), sk (literal chars), bd (brace depth).
function lit(s) { tok = tok s; sk = sk s; pat = pat rx(s); have = 1 }
function wild(p, t, f) { tok = tok t; pat = pat p; fl = fl f; have = 1 }
function reset() { tok = ""; fl = ""; pat = ""; sk = ""; have = 0; bd = 0 }
function addword(cid, t, f, p, s) {
  N[cid]++; W[cid, N[cid]] = t; F[cid, N[cid]] = f; P[cid, N[cid]] = p; K[cid, N[cid]] = s
}
# A word ends. After an output redirection it is the target: RO marks a command that writes a file
# (anything but /dev/null) — prose written to a file can be run by the next command.
function flush(cid,   e) {
  if (have) {
    # A mention is TEXT — a word with blanks in it. `repos/o/n/pulls/2/merge` alone is an argument.
    if (tok ~ /[ \t\n]/ && hits(tok)) MENTION = 1
    if (RT) { if (tok != "/dev/null" || fl != "") RO[RT] = 1; RT = 0 }
    if (index(pat, LB)) {
      NE = 0; XP = 0; expand(pat, sk, 0)
      if (NE > 64) { broken("a brace expansion too large to read"); addword(cid, tok, fl "b", ".*", "") }
      else for (e = 1; e <= NE; e++) {
        if (EP[e] == "") continue
        if (XP) addword(cid, ES[e], fl "b", EP[e], ES[e]); else addword(cid, tok, fl, EP[e], ES[e])
      }
    } else addword(cid, tok, fl, pat, sk)
  }
  reset()
}

# Brace expansion, on the regex and the literal text together (their markers are in step). Each
# alternative becomes a word of its own: `{gh,pr,merge}` is three words, `{merge,}` is `merge`.
function group(t, i, C,   d, k, c) {
  C[0] = 0; d = 0
  for (k = i + 1; k <= length(t); k++) {
    c = substr(t, k, 1)
    if (c == LB) d++
    else if (c == RB) { if (d == 0) return k; d-- }
    else if (c == CM && d == 0) C[++C[0]] = k
  }
  return 0
}
function expand(p, s, depth,   i, j, np, ns, PC, SC, pre_p, post_p, pre_s, post_s, a, b, k, pe, se, c) {
  if (NE > 64) return
  if (depth > 20) { NE = 65; return }
  i = index(p, LB)
  if (!i) {
    gsub(CM, "[,]", p); gsub(RB, "[}]", p); gsub(CM, ",", s); gsub(RB, "}", s)
    EP[++NE] = p; ES[NE] = s; return
  }
  j = index(s, LB)
  np = group(p, i, PC); ns = group(s, j, SC)
  if (!np || !ns || PC[0] != SC[0]) {
    expand(substr(p, 1, i - 1) "[{]" substr(p, i + 1), substr(s, 1, j - 1) "{" substr(s, j + 1), depth + 1)
    return
  }
  pre_p = substr(p, 1, i - 1); post_p = substr(p, np + 1)
  pre_s = substr(s, 1, j - 1); post_s = substr(s, ns + 1)
  if (PC[0] == 0) {
    c = substr(s, j + 1, ns - j - 1)
    if (c ~ /^([A-Za-z]|-?[0-9]+)\.\.([A-Za-z]|-?[0-9]+)(\.\.-?[0-9]+)?$/) {
      XP = 1; expand(pre_p "(.|-?[0-9]+)" post_p, pre_s post_s, depth + 1); return
    }
    expand(pre_p "[{]" substr(p, i + 1, np - i - 1) "[}]" post_p, pre_s "{" c "}" post_s, depth + 1)
    return
  }
  XP = 1; a = i; b = j
  for (k = 1; k <= PC[0] + 1; k++) {
    pe = (k <= PC[0]) ? PC[k] : np
    se = (k <= SC[0]) ? SC[k] : ns
    expand(pre_p substr(p, a + 1, pe - a - 1) post_p, pre_s substr(s, b + 1, se - b - 1) post_s, depth + 1)
    a = pe; b = se
  }
}

# ${...}: skipped to its matching brace; a merge inside it (${X:-$(gh pr merge 5)}) is ODD.
function brace(i,   d, s, c) {
  d = 1; s = i
  while (i <= L) {
    c = substr(S, i, 1)
    if (c == "\\") { i += 2; continue }
    if (c == "{") d++
    if (c == "}" && --d == 0) { if (hits(substr(S, s, i - s))) odd("a ${...} expansion names the merge"); return i + 1 }
    i++
  }
  broken("an unterminated ${"); return i
}
# $'...': literal up to its first escape; from there on computed ($'\x67h' is gh).
function ansi(i,   c, esc) {
  esc = 0
  while (i <= L) {
    c = substr(S, i, 1)
    if (c == "\\") { if (!esc) wild(".*", "", "d"); esc = 1; i += 2; continue }
    if (c == "'") return i + 1
    if (!esc) lit(c)
    i++
  }
  broken("an unterminated $' quote"); return i
}
# A command substitution: its own command list, read with the word being built set aside. The
# word itself only records that something was computed.
function subst(i, mode,   st, sf, sp, ss, sh, sb) {
  st = tok; sf = fl; sp = pat; ss = sk; sh = have; sb = bd; reset()
  i = scan(i, mode)
  tok = st; fl = sf; pat = sp; sk = ss; have = sh; bd = sb
  wild(".*", "$()", "d")
  return i
}
# "$@" and "${a[@]}" are several words even inside quotes: flag s, as for an unquoted expansion.
function dollar(i,   nx, j) {
  nx = substr(S, i + 1, 1)
  if (nx == "(") return subst(i + 2, "paren")
  if (nx == "{") { j = i; i = brace(i + 2); wild(".*", "${}", substr(S, j + 2, i - j - 3) ~ /^@|\[@\]/ ? "ds" : "d"); return i }
  if (match(substr(S, i + 1, 128), /^([A-Za-z_][A-Za-z0-9_]*|[0-9@*#?!$-])/)) {
    wild(".*", "$" substr(S, i + 1, RLENGTH), substr(S, i + 1, 1) == "@" ? "ds" : "d"); return i + 1 + RLENGTH
  }
  lit("$"); return i + 1
}
# Double-quoted text from i to the closing quote, or, with stop, an unquoted heredoc body up to
# stop. $( ) and backticks inside are read as commands of their own.
function dq(i, stop,   c, nx, lim) {
  while (i <= L) {
    if (stop && i > stop) return i
    c = substr(S, i, 1)
    if (!stop && c == "\"") return i + 1
    if (c == "\\") {
      nx = substr(S, i + 1, 1)
      if (nx != "\n") { if (nx ~ /[$`\\]/ || (!stop && nx == "\"")) lit(nx); else lit("\\" nx) }
      i += 2; continue
    }
    if (c == "`") { i = subst(i + 1, "bq"); continue }
    if (c == "$") { i = dollar(i); continue }
    lim = stop ? stop - i + 1 : 512; if (lim > 512) lim = 512
    if (match(substr(S, i, lim), DQRUN)) { lit(substr(S, i, RLENGTH)); i += RLENGTH } else { lit(c); i++ }
  }
  if (!stop) broken("an unterminated double quote")
  return i
}
# A glob bracket `[…]`: a simple set becomes a regex bracket; anything else stays a literal `[`.
function bracket(i,   j, body) {
  j = index(substr(S, i + 1, 64), "]")
  body = j > 1 ? substr(S, i + 1, j - 1) : ""
  if (body ~ /^[!^]?[A-Za-z0-9_.-]+$/) {
    wild("[" (body ~ /^!/ ? "^" substr(body, 2) : body) "]", "[" body "]", "gs"); return i + j + 1
  }
  lit("["); return i + 1
}
function heredoc(i,   tabs, d, q, c, j) {
  tabs = 0; if (substr(S, i, 1) == "-") { tabs = 1; i++ }
  while (substr(S, i, 1) ~ /[ \t]/) i++
  d = ""; q = 0
  while (i <= L) {
    c = substr(S, i, 1)
    if (c ~ /[ \t\n;&|<>()]/) break
    if (c == "\\") { q = 1; d = d substr(S, i + 1, 1); i += 2; continue }
    if (c == "'" || c == "\"") {
      q = 1; j = index(substr(S, i + 1), c)
      if (!j) { broken("an unterminated heredoc delimiter"); return L + 1 }
      d = d substr(S, i + 1, j - 1); i += j + 1; continue
    }
    d = d c; i++
  }
  if (d == "") broken("a heredoc with no delimiter")
  HD[++HN] = d; HQ[HN] = q; HT[HN] = tabs
  return i
}
# The bodies of every heredoc opened on the line just ended. A quoted delimiter makes the body
# literal text; an unquoted one expands $( ) and backticks, which are read as commands.
function bodies(i,   h, s, e, j, line, nx, cmp, st, sf, sp, ss, sh, sb) {
  for (h = 1; h <= HN; h++) {
    s = i; e = L
    while (i <= L) {
      j = index(substr(S, i), "\n")
      line = j ? substr(S, i, j - 1) : substr(S, i)
      nx = j ? i + j : L + 1
      cmp = line; if (HT[h]) sub(/^\t+/, "", cmp)
      if (cmp == HD[h]) { e = i - 1; i = nx; break }
      i = nx
    }
    if (HQ[h]) { if (hits(substr(S, s, e - s + 1))) MENTION = 1 }
    else if (e >= s) {
      st = tok; sf = fl; sp = pat; ss = sk; sh = have; sb = bd; reset()
      dq(s, e); if (hits(tok)) MENTION = 1
      tok = st; fl = sf; pat = sp; sk = ss; have = sh; bd = sb
    }
  }
  HN = 0
  return i
}
# One command list from i: to the end ("top"), to its unmatched ")" ("paren") or backtick ("bq").
function scan(i, mode,   cid, c, nx, j, depth, line) {
  cid = newcmd(); depth = 0
  while (i <= L) {
    c = substr(S, i, 1)
    if (c == "\\") {
      nx = substr(S, i + 1, 1)
      if (nx == "\n" || nx == "") { i += 2; continue }
      lit(nx); fl = fl "e"; i += 2; continue
    }
    if (c == "'") {
      j = index(substr(S, i + 1), "'")
      if (!j) { broken("an unterminated single quote"); lit(substr(S, i + 1)); fl = fl "q"; i = L + 1; break }
      lit(substr(S, i + 1, j - 1)); fl = fl "q"; i += j + 1; continue
    }
    if (c == "\"") { have = 1; fl = fl "q"; i = dq(i + 1, 0); continue }
    if (c == "$") {
      nx = substr(S, i + 1, 1)
      if (nx == "'") { have = 1; fl = fl "q"; i = ansi(i + 2); continue }
      if (nx == "\"") { have = 1; fl = fl "q"; i = dq(i + 2, 0); continue }
      i = dollar(i); fl = fl "s"; continue
    }
    if (c == "`") {
      if (mode == "bq") { flush(cid); return i + 1 }
      i = subst(i + 1, "bq"); fl = fl "s"; continue
    }
    if (c == "#" && !have) {
      j = index(substr(S, i), "\n"); line = j ? substr(S, i, j - 1) : substr(S, i)
      if (hits(line)) odd("a comment names the merge")
      i += length(line); continue
    }
    # zsh: `=gh` at the start of a word is the path of gh.
    if (c == "=" && !have && substr(S, i + 1, 1) ~ /[A-Za-z_]/) { wild(".*", "=", "d"); i++; continue }
    if (c == " " || c == "\t" || c == "\r") { flush(cid); i++; continue }
    if (c == "\n") { flush(cid); cid = newcmd(); i++; if (HN) i = bodies(i); continue }
    if (c == "<" && substr(S, i, 2) == "<<" && substr(S, i + 2, 1) != "<") { flush(cid); i = heredoc(i + 2); continue }
    if (c == "<" || c == ">" || (c == "&" && substr(S, i + 1, 1) == ">")) {
      if (tok ~ /^[0-9]+$/ && fl == "") reset(); else flush(cid)
      nx = (c != "<" || substr(S, i + 1, 1) == ">")
      i++
      while (substr(S, i, 1) ~ /[<>|]/) i++
      if (substr(S, i, 1) == "&") { i++; while (substr(S, i, 1) ~ /[0-9-]/) i++ }
      else if (nx) RT = cid
      continue
    }
    if (c == ";" || c == "&" || c == "|") { flush(cid); cid = newcmd(); i++; continue }
    if (c == "(") { flush(cid); cid = newcmd(); depth++; i++; continue }
    if (c == ")") {
      flush(cid)
      if (depth == 0 && mode == "paren") return i + 1
      if (depth > 0) depth--
      cid = newcmd(); i++; continue
    }
    if (c == "{") { tok = tok c; pat = pat LB; sk = sk LB; have = 1; bd++; i++; continue }
    if (c == "}" && bd > 0) { tok = tok c; pat = pat RB; sk = sk RB; bd--; i++; continue }
    if (c == "," && bd > 0) { tok = tok c; pat = pat CM; sk = sk CM; i++; continue }
    if (c == "?") { wild(".", "?", "gs"); i++; continue }
    if (c == "*") { wild(".*", "*", "gs"); i++; continue }
    if (c == "[") { i = bracket(i); continue }
    if (match(substr(S, i, 512), RUN)) { lit(substr(S, i, RLENGTH)); i += RLENGTH; continue }
    lit(c); i++
  }
  flush(cid)
  if (mode == "paren") broken("an unterminated $(")
  if (mode == "bq") broken("an unterminated backtick")
  return i
}

# prose_safe — a simple command that hands none of its words, stdin or environment to a shell.
# Deliberately short: sed (e), awk (system), sort (--compress-program), rg (--pre), git grep (-O),
# git -c, xargs, env and every shell can run text, so a mention beside any of them blocks.
function prose_safe(c,   w, k) {
  if (RO[c]) return 0
  if (N[c] == 0) return 1
  w = W[c, 1]
  if (F[c, 1] != "" || w ~ /=/) return 0
  if (w ~ /^(echo|printf|cat|grep|egrep|fgrep|head|tail|wc|uniq|cut|tr|nl|cd|ls|pwd|true|false)$/) return 1
  if (w == "git") {
    k = 2; if (W[c, 2] == "-C" && F[c, 3] !~ /d/) k = 4
    return F[c, k] == "" && W[c, k] ~ /^(add|commit|tag|log|show|status|diff|push)$/
  }
  if (w == "gh")
    return F[c, 2] == "" && F[c, 3] == "" && W[c, 2] ~ /^(pr|issue)$/ &&
           W[c, 3] ~ /^(create|edit|comment|review|view|list|close|reopen|checks|diff|status)$/
  return 0
}

function could(c, k, x) { return match(x, "^(" P[c, k] ")$") }
# could be gh, or a path ending in /gh
function could_gh(c, k,   p, j, m) {
  if (could(c, k, "gh")) return 1
  p = P[c, k]; j = 0
  while ((m = index(substr(p, j + 1), "[/]")) > 0) j += m + 2
  return j > 0 && match("gh", "^(" substr(p, j + 1) ")$")
}
# provable: its text is exactly what runs (quotes and escapes allowed; no expansion, glob or brace)
function provable(c, k) { return F[c, k] !~ /[dgb]/ }
function plainw(c, k) { return F[c, k] == "" }
function flagskip(c, j, n) {
  while (j <= n && provable(c, j) && W[c, j] ~ /^-/) j += (W[c, j] == "-R" || W[c, j] == "--repo") ? 2 : 1
  return j
}

# A word that could be gh: its subcommand must be readable. `gh pr merge` is a site; `gh api` goes
# to api(); a computed, globbed or brace-expanded word where the subcommand goes, that could be pr,
# api or merge, is ODD. (One that cannot — `grep "gh" --include=*.ts` — is left alone.)
# heads(c, k) — the words before k leave k in command position: assignments, and only prefixes that
# run the rest of the line AS WRITTEN (sudo, env, nice, time, …) with their flags and numbers. xargs,
# find -exec, parallel and the like ADD or SUBSTITUTE words, so gh after them is not.
function heads(c, k,   x, w) {
  for (x = 1; x < k; x++) {
    w = W[c, x]
    if (!provable(c, x)) return 0
    if (w ~ /^[A-Za-z_][A-Za-z0-9_]*=/ || w ~ /^-/ || w ~ /^[0-9]+[smhd]?$/) continue
    if (w ~ /^(then|do|else|elif|if|while|until|!|\{|time|command|builtin|exec|nohup|sudo|doas|env|nice|timeout|eval|noglob|nocorrect|stdbuf)$/) continue
    return 0
  }
  return 1
}
function ghword(c, k, n,   j, x, m) {
  # gh fed its words by another command (`echo merge 5 | xargs gh pr`, `parallel gh pr ::: merge`,
  # `find . -exec gh pr merge 5 \;`), or with no subcommand at all: where the command names a merge
  # anywhere, its words cannot be read from here.
  if (MERGE_ANY && !heads(c, k)) { odd("gh (" W[c, k] ") is an argument of " W[c, 1] ", which can add or substitute its words"); return }
  if (k == n) { if (MERGE_ANY) odd("gh has no subcommand here, and the command names a merge"); return }
  if (!provable(c, k + 1)) {
    # An unquoted expansion or glob can split into several words (`gh $X 5`, X="pr merge"): any
    # that could start with pr or api is ODD. A quoted one is one word: ODD only if a merge could
    # follow it (`grep "gh" "$f"` is left alone).
    if (F[c, k + 1] ~ /s/) m = could(c, k + 1, "pr") || could(c, k + 1, "api")
    else {
      j = flagskip(c, k + 2, n)
      m = could(c, k + 1, "pr") && j <= n && could(c, j, "merge")
      if (could(c, k + 1, "api")) for (x = k + 2; x <= n; x++) if (tolower(W[c, x]) ~ /merge/) m = 1
    }
    if (m) odd("the word after " W[c, k] " is computed, globbed or brace-expanded (" W[c, k + 1] ")")
    return
  }
  if (W[c, k + 1] == "api") { api(c, k, n); return }
  if (W[c, k + 1] != "pr") return
  j = flagskip(c, k + 2, n)
  if (j > n) { if (MERGE_ANY) odd("gh pr has no subcommand here, and the command names a merge"); return }
  if (!provable(c, j)) {
    if (could(c, j, "merge")) odd("the word after " W[c, k] " pr is computed, globbed or brace-expanded (" W[c, j] ")")
    return
  }
  if (W[c, j] != "merge") return
  if (j == k + 2 && plainw(c, k) && plainw(c, k + 1) && plainw(c, j) && W[c, k] ~ /^([^ ]*\/)?gh$/) { PLAIN++; SITE[c] = 1; return }
  odd("the merge reads as [" W[c, k] " pr … merge], with " ((j > k + 2) ? "flags between pr and merge" : "a quoted, escaped, globbed or computed gh, pr or merge"))
}

# gh api — a PUT to repos/<o>/<n>/pulls/<pr>/merge merges exactly like `gh pr merge <pr>`, so it is
# reported (API) for the hook to police the same way. One whose endpoint or method cannot be read
# and that names a merge is ODD, as is a GraphQL call naming a merge or reading its query from a file.
function api(c, k, n,   j, w, x, m, mw, ep, ew, ei, men, gq, e, r, q, pos, nf, M) {
  m = ""; mw = 0; ep = ""; ew = 0; ei = 0; men = 0; gq = 0; nf = 0
  for (j = k + 2; j <= n; j++) {
    w = W[c, j]
    if (tolower(w) ~ /merge/) men = 1
    if (!provable(c, j)) { if (ep == "") { ep = w; ew = 1; ei = j } else mw = 1; continue }
    if (w == "-X" || w == "--method") {
      if (++j <= n) { if (tolower(W[c, j]) ~ /merge/) men = 1; if (provable(c, j)) m = W[c, j]; else mw = 1 }
      continue
    }
    if (w ~ /^--method=/) { m = substr(w, 10); continue }
    if (w ~ /^(--field|--raw-field|--header|--input|--jq|--template|--hostname|--cache|--preview)$/) {
      if (w == "--input") gq = 1
      if (w ~ /^--(field|raw-field|input)$/) nf = 1
      if (++j <= n) { if (tolower(W[c, j]) ~ /merge/) men = 1; if (W[c, j] ~ /^[^=]*=@/) gq = 1; if (!provable(c, j)) mw = 1 }
      continue
    }
    if (w ~ /^--/) {
      if (w ~ /^--input=/ || w ~ /^--(field|raw-field)=[^=]*=@/) gq = 1
      if (w ~ /^--(field|raw-field|input)=/) nf = 1
      continue
    }
    if (w ~ /^-./) {
      # a cluster of shorthands: the first that takes a value takes the rest of the word, or the next
      for (pos = 2; pos <= length(w); pos++) {
        x = substr(w, pos, 1)
        if (x !~ /[XfFHqtp]/) continue
        r = substr(w, pos + 1); sub(/^=/, "", r)
        if (r == "" && ++j <= n) { r = W[c, j]; if (tolower(r) ~ /merge/) men = 1; if (!provable(c, j)) { if (x == "X") mw = 1; r = "" } }
        if (x == "X") m = r
        if (x ~ /[fF]/) nf = 1
        if (x ~ /[fF]/ && r ~ /^[^=]*=@/) gq = 1
        break
      }
      continue
    }
    if (ep == "") { ep = w; ei = j }
  }
  # GitHub routes …/merge?x=1 and …/merge#x to the merge endpoint: the query and fragment go first.
  e = tolower(ep); sub(/[?#].*$/, "", e); sub(/^https:\/\/api\.github\.com\//, "", e); sub(/^\//, "", e)
  # The method gh sends: -X, else POST when any field or --input is given, else GET.
  M = toupper(m); if (M == "" && nf) M = "POST"
  if (ew) { if (men || M == "PUT" || mw) odd("gh api with a computed endpoint (" ep ") that may merge"); return }
  if (e == "graphql") { if (men || gq || mw) odd("gh api graphql that names a merge or reads its query from a file"); return }
  if (e ~ /^repos\/[^\/]+\/[^\/]+\/pulls\/[0-9]+\/merge\/?$/ && !mw && M == "PUT") {
    if (!(plainw(c, k) && plainw(c, k + 1) && plainw(c, ei) && W[c, k] ~ /^([^ ]*\/)?gh$/)) {
      odd("gh api merges with a quoted, escaped or computed gh, api or endpoint"); return
    }
  } else {
    # Anything else that WRITES (or may) and names a merge — a POST to …/merges, a PATCH, a method
    # it cannot read — is not the one form this guard can police, so it is refused.
    if (men && (mw || (M != "" && M != "GET"))) odd("gh api " (mw ? "with a computed method" : M) " that names a merge, outside the one form this guard reads (-X PUT repos/<o>/<n>/pulls/<pr>/merge)")
    return
  }
  split(e, q, "/")
  r = q[2] "/" q[3]; if (r ~ /^(\{owner\}|:owner)\/(\{repo\}|:repo)$/) r = "."
  APIS[++NA] = r " " q[5]; SITE[c] = 1
}

{ buf = buf $0 "\n" }
END {
  LB = "\034"; CM = "\035"; RB = "\036"
  RUN = "^[^ \t\r\n;&|()<>'\"\\\\$`{},?*[#=]+"
  DQRUN = "^[^\"\\\\`$]+"
  S = substr(buf, 1, length(buf) - 1); L = length(S)
  reset()
  i = 1; while (i <= L) i = scan(i, "top")
  rel = tolower(S); gsub(/[^a-z0-9]/, "", rel)
  MERGE_ANY = rel ~ /merg/
  for (c = 1; c <= NC; c++) {
    n = N[c]
    for (k = 1; k <= n; k++) {
      if (K[c, k] != "" && could_gh(c, k)) { ghword(c, k, n); continue }
      # `pr merge` after something that is not a literal-bearing gh: $GH, an alias, a function
      if (!could(c, k, "pr")) continue
      if (k > 1 && K[c, k - 1] != "" && could_gh(c, k - 1)) continue
      j = flagskip(c, k + 1, n)
      if (j > n || !could(c, j, "merge")) continue
      if (K[c, k] == "" && K[c, j] == "") continue
      odd("the merge reads as [" (k > 1 ? W[c, k - 1] " " : "") W[c, k] " " W[c, j] "], and what comes before pr is not a plain gh")
    }
  }
  if (rel ~ /gh|merge/) for (x = 1; x <= NB; x++) print "ODD " BROKE[x]
  for (x = 1; x <= NO; x++) print "ODD " ODDS[x]
  if (MENTION) for (c = 1; c <= NC; c++) if (!SITE[c] && !prose_safe(c)) { print "UNSAFE " W[c, 1] (RO[c] ? " > a file" : ""); break }
  if (PLAIN) print "PLAIN " PLAIN
  for (x = 1; x <= NA; x++) print "API " APIS[x]
  if (MENTION) print "MENTION"
}
