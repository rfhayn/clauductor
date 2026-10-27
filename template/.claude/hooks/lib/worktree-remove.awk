# worktree-remove.awk: does this Bash command RUN `git worktree remove` with a force
# flag? One line on stdout: `allow`, `force <word>`, or `computed <reason>`.
# Ported from Standing Tee's worktree-command.mjs without node (ADR-0006).
# Not a shell. Quotes, escapes, $(…), backticks, heredoc bodies and comments are
# skipped the way the shell would, so a mention in a commit message is not a run.
#
# KNOWN LIMITS, against Standing Tee's reader: a shell nested deeper than 4; an
# alias set only in GIT_CONFIG_* outside this command's words; a forced remove
# written to a file and then executed (write-then-run). A script FILE is not
# read, so machine-quiet.sh's own `worktree remove -f` is not this hook's.
BEGIN { MAX = 4 }
{ src = src $0 "\n" }
END { SRC = src; scan(src, 1); if (out == "") out = "allow"; print out }

function setv(s) {
  if (s ~ /^force /) { if (out !~ /^force /) out = s }
  else if (out == "") out = s
}
function base(t,   b) { b = t; sub(/.*\//, "", b); return tolower(b) }
function is_shell(t,   b) {
  b = base(t)
  return b == "sh" || b == "bash" || b == "zsh" || b == "dash" || b == "ksh" || b == "mksh" || b == "fish"
}
function isforce(w) { return w ~ /^--f(o(r(ce?)?)?)?$/ || w ~ /^-[A-Za-z]*f[A-Za-z]*$/ }
function expands(w) { return w ~ /[$`]/ || w ~ /\{[^}]*,/ || w ~ /[*?[]/ }
function names_removal(s,   t, n, i, a, b, r) {
  t = s; gsub(/["'\\]/, "", t)
  n = split(t, a, /[[:space:];|&()<>`$={}]+/)
  for (i = 1; i <= n; i++) { if (a[i] == "worktree") b = 1; if (a[i] == "remove") r = 1 }
  return b && r
}
function opt_value(o) {
  if (index(o, "=") > 0) return 0
  return o == "-C" || o == "-c" || o == "--git-dir" || o == "--work-tree" || o == "--namespace" || o == "--config-env" || o == "--super-prefix" || o == "--attr-source"
}
# git_from I APPENDED: argv from the git word at I.
function git_from(i, appended,   k, o, v, later, j) {
  k = i + 1
  while (k <= N && WW[k] ~ /^-/) {
    o = WW[k]
    v = o
    if (index(o, "=") > 0) v = substr(o, index(o, "=") + 1)
    else if (k < N) v = WW[k + 1]
    if ((o == "-c" || o ~ /^--config-env/) && tolower(v) ~ /^alias\./) setv("computed an inline alias")
    if (opt_value(o)) k += 2
    else k++
  }
  if (k > N) return
  if (expands(WW[k])) {
    later = 0
    for (j = k + 1; j <= N; j++) if (WW[j] == "remove" || isforce(WW[j]) || expands(WW[j])) later = 1
    if (later) setv("computed " WW[k])
    return
  }
  if (WW[k] != "worktree") return
  k++
  while (k <= N && WW[k] ~ /^-/ && WW[k] != "--") k++
  if (k <= N && expands(WW[k])) { setv("computed " WW[k]); return }
  if (k > N || WW[k] != "remove") return
  saw = 1
  for (k++; k <= N; k++) {
    if (WW[k] == "--" || WW[k] == "--end-of-options") return
    if (expands(WW[k])) { setv("computed " WW[k]); return }
    if (isforce(WW[k])) { setv("force " WW[k]); return }
  }
  if (appended) setv("computed arguments a runner appends (put `--` before them)")
}
function judge(fed, depth,   i, nn, b, appended, t) {
  nn = N
  for (i = 1; i <= nn; i++) SAV[depth, i] = WW[i]
  appended = 0
  for (i = 1; i <= nn; i++) {
    b = base(SAV[depth, i])
    if (b == "xargs" || b == "parallel") appended = 1
    if (b == "git") { N = nn; for (b = 1; b <= nn; b++) WW[b] = SAV[depth, b]; git_from(i, appended) }
  }
  if (nn < 1 || !is_shell(SAV[depth, 1])) return
  if (depth > MAX) { if (names_removal(SRC)) setv("computed shells nested deeper than " MAX); return }
  for (i = 2; i <= nn; i++) {
    t = SAV[depth, i]
    if (t ~ /^-c./) { scan(substr(t, 3), depth + 1); continue }
    if (t ~ /^-/ && t !~ /^--/ && t ~ /c/ && i < nn) { scan(SAV[depth, ++i], depth + 1); continue }
    if (t ~ /^[+-]/) continue
    if (index(t, "$") > 0 || index(t, "worktree") > 0 || index(t, "remove") > 0) scan(t, depth + 1)
  }
  # A shell with no script of its own, fed on stdin, whose call still names a removal
  # the reader did not parse (a pipe or a heredoc). A script FILE is data, not this.
  if (fed && !saw && names_removal(SRC)) {
    t = ""
    for (i = 2; i <= nn; i++) if (SAV[depth, i] !~ /^[-+]/) { t = SAV[depth, i]; break }
    if (t == "" || (t !~ /\.(sh|bash|zsh)$/ && index(t, "/") == 0)) setv("computed a script on a shell's stdin")
  }
}
function flush(fed, depth) {
  if (N == 0) return
  judge(fed, depth)
  N = 0
}
function endword(   t) {
  if (!HV) return
  t = WB; WB = ""; HV = 0
  if (HN) { HD = t; HSTRIP = (HN == 2); HN = 0; return }
  if (RD) { RD = 0; return }
  WW[++N] = t
}
function scan(s, depth,    n, i, c, c2, j, fed, line, e, body, saveN, si) {
  if (depth > MAX) { if (names_removal(s)) setv("computed shells nested deeper than " MAX); return }
  saveN = N
  for (si = 1; si <= saveN; si++) SV[depth, si] = WW[si]
  n = length(s); i = 1; fed = 0; N = 0; WB = ""; HV = 0; HN = 0; RD = 0; HD = ""
  while (i <= n) {
    c = substr(s, i, 1); c2 = substr(s, i + 1, 1)
    if (c == "\\") { if (c2 != "\n") { WB = WB c2; HV = 1 }; i += 2; continue }
    if (c == "'") {
      j = index(substr(s, i + 1), "'"); if (j == 0) j = n - i + 1
      WB = WB substr(s, i + 1, j - 1); HV = 1; i += j + 1; continue
    }
    if (c == "\"") {
      HV = 1; i++
      while (i <= n && substr(s, i, 1) != "\"") {
        c = substr(s, i, 1)
        if (c == "\\") { WB = WB substr(s, i + 1, 1); i += 2 }
        else if (c == "`" || (c == "$" && substr(s, i + 1, 1) == "(")) { WB = WB "$()"; HV = 1; i = close_at(s, i) }
        else { WB = WB c; i++ }
      }
      i++; continue
    }
    if (c == "$" && c2 == "(") { WB = WB "$()"; HV = 1; i = close_at(s, i); continue }
    if (c == "`") { WB = WB "$()"; HV = 1; i = close_at(s, i); continue }
    if (c == "$") { WB = WB c; HV = 1; i++; continue }
    if (c == "#" && !HV) { while (i <= n && substr(s, i, 1) != "\n") i++; continue }
    if (c == " " || c == "\t") { endword(); i++; continue }
    if (c == "\n") {
      endword()
      if (HD != "") {
        body = ""; i++
        while (i <= n) {
          e = index(substr(s, i), "\n"); if (e == 0) e = n - i + 2
          line = substr(s, i, e - 1); i += e
          if (HSTRIP) sub(/^\t+/, "", line)
          if (line == HD) break
          body = body line "\n"
        }
        if (N >= 1 && is_shell(WW[1])) scan(body, depth + 1)
        HD = ""
      }
      flush(fed, depth); fed = 0; i++; continue
    }
    if (c == ";") { endword(); flush(fed, depth); fed = 0; i++; continue }
    if (c == "&") {
      endword()
      if (c2 == "&") { flush(fed, depth); fed = 0; i += 2; continue }
      flush(fed, depth); fed = 0; i++; continue
    }
    if (c == "|") {
      endword()
      if (c2 == "|") { flush(fed, depth); fed = 0; i += 2; continue }
      flush(fed, depth); fed = 1; i++; continue
    }
    if (c == "<" || c == ">") {
      if (HV && WB ~ /^[0-9]+$/) { WB = ""; HV = 0 } else endword()
      if (c == "<" && c2 == "<") { HN = substr(s, i + 2, 1) == "-" ? 2 : 1; i += HN == 2 ? 3 : 2; continue }
      RD = 1; i++
      if (c2 == ">" || c2 == "&" || c2 == "|" || (c == "<" && c2 == ">")) i++
      continue
    }
    WB = WB c; HV = 1; i++
  }
  endword(); flush(fed, depth)
  N = saveN
  for (si = 1; si <= saveN; si++) WW[si] = SV[depth, si]
}
# close_at S I: index just after a $(…) or backtick that starts at I. Nested enough for a word.
function close_at(s, i,   n, c, d) {
  n = length(s)
  if (substr(s, i, 1) == "`") {
    for (i++; i <= n; i++) { c = substr(s, i, 1); if (c == "\\") i++; else if (c == "`") return i + 1 }
    return n + 1
  }
  d = 0
  for (i++; i <= n; i++) {
    c = substr(s, i, 1)
    if (c == "\\") { i++; continue }
    if (c == "(") d++
    else if (c == ")" && --d == 0) return i + 1
  }
  return n + 1
}
