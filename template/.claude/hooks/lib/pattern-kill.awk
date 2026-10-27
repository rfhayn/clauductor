# One command on stdin. One word on stdout: block or allow.
# pkill or killall in command position is a block: a path (/usr/bin/pkill), the bare name,
# or that name after a wrapper (command -p, env PATH=, sudo, sh -c). A mention in an
# argument is not. A script file is not read. Deeper than four shells, any mention blocks.
function next_token(s, i,    n, c, q, w, delim, dash, line) {
  n = length(s)
  while (i <= n && (substr(s, i, 1) == " " || substr(s, i, 1) == "\t")) i++
  if (i > n) { KIND = "end"; TOKEN = ""; return i }
  c = substr(s, i, 1)
  if (c == "\n" || c == ";" || c == "|" || c == "&") {
    if ((c == "|" || c == "&") && substr(s, i + 1, 1) == c) i++
    KIND = "sep"; TOKEN = ""; return i + 1
  }
  if (c == "<" && substr(s, i + 1, 1) == "<") {
    i += 2
    dash = 0
    if (substr(s, i, 1) == "-") { dash = 1; i++ }
    while (i <= n && (substr(s, i, 1) == " " || substr(s, i, 1) == "\t")) i++
    delim = ""; q = ""
    while (i <= n) {
      c = substr(s, i, 1)
      if (q == "" && index(" \t\n;|&<>", c)) break
      if (c == "'" || c == "\"") {
        if (q == "") q = c
        else if (q == c) q = ""
        else delim = delim c
        i++; continue
      }
      delim = delim c; i++
    }
    while (i <= n && substr(s, i, 1) != "\n") i++
    if (i <= n && substr(s, i, 1) == "\n") i++
    while (i <= n) {
      line = ""
      while (i <= n && substr(s, i, 1) != "\n") { line = line substr(s, i, 1); i++ }
      if (i <= n && substr(s, i, 1) == "\n") i++
      if (dash) sub(/^[ \t]+/, "", line)
      if (line == delim) break
    }
    KIND = "heredoc"; TOKEN = ""; return i
  }
  if (c == "<" || c == ">") {
    while (i <= n && (substr(s, i, 1) == "<" || substr(s, i, 1) == ">")) i++
    KIND = "redir"; TOKEN = ""; return i
  }
  w = ""; q = ""
  while (i <= n) {
    c = substr(s, i, 1)
    if (q == "" && index(" \t\n;|&<>", c)) break
    if (q == "" && c == "\\") {
      i++
      if (i <= n) { w = w substr(s, i, 1); i++ }
      continue
    }
    if (c == "'" || c == "\"") {
      if (q == "") q = c
      else if (q == c) q = ""
      else w = w c
      i++; continue
    }
    w = w c; i++
  }
  TOKEN = w; KIND = "word"; return i
}

function is_wrapper(w,    l) {
  l = tolower(w)
  return l == "sudo" || l == "nohup" || l == "nice" || l == "time" || l == "exec" || l == "xargs" || l == "command" || l == "env" || l == "sh" || l == "bash" || l == "dash" || l == "stdbuf"
}

function takes_arg(wrapper, opt) {
  if (opt == "-u" && (wrapper == "sudo" || wrapper == "env")) return 1
  if (opt == "-n" && (wrapper == "nice" || wrapper == "xargs")) return 1
  if (wrapper == "xargs" && (opt == "-I" || opt == "-L" || opt == "-P" || opt == "-a" || opt == "-E")) return 1
  return 0
}

function base_is_tool(w,    b) {
  b = tolower(w)
  sub(/^.*\//, "", b)
  return b == "pkill" || b == "killall"
}

# A command substitution executes. A quoted mention does not contain $( or a backtick.
function subst_is_tool(w,    b) {
  if (w !~ /\$\(|`/) return 0
  b = tolower(w)
  gsub(/[$(){}`]/, "", b)
  sub(/^.*\//, "", b)
  return b == "pkill" || b == "killall"
}

function judge(s, depth,    i, n, looking, wrapper, want_c, skip_arg, word) {
  if (depth > 4) return tolower(s) ~ /(pkill|killall)/
  n = length(s); i = 1; looking = 1; wrapper = ""; want_c = 0; skip_arg = 0
  while (i <= n) {
    i = next_token(s, i)
    if (KIND == "end") break
    if (KIND == "sep") { looking = 1; wrapper = ""; want_c = 0; skip_arg = 0; continue }
    if (KIND == "heredoc" || KIND == "redir") continue
    word = TOKEN
    if (subst_is_tool(word)) return 1
    if (!looking) continue
    if (skip_arg) { skip_arg = 0; continue }
    if (word ~ /^[A-Za-z_][A-Za-z0-9_]*=/) continue
    if (word ~ /^[0-9]*[<>]/) continue
    if (word ~ /^-/) {
      if (wrapper ~ /^(sh|bash|dash)$/ && word ~ /^-[A-Za-z]*c[A-Za-z]*$/) want_c = 1
      if (takes_arg(wrapper, word)) skip_arg = 1
      continue
    }
    if (want_c) {
      if (judge(word, depth + 1)) return 1
      looking = 0; want_c = 0; wrapper = ""; continue
    }
    if (is_wrapper(word)) { wrapper = tolower(word); continue }
    if (base_is_tool(word)) return 1
    looking = 0; wrapper = ""
  }
  return 0
}

{ buf = buf $0 "\n" }
END { if (judge(buf, 0)) print "block"; else print "allow" }
