# gate-reader.awk: reads ONE shell command (on stdin) for no-background-gate.sh, no-silent-wait.sh
# and cd-guard.sh, and answers up to three questions. Not a shell: a tokenizer that knows quotes, escapes, `$(…)`, backticks, `${…}`,
# heredocs, comments, redirections and separators. POSIX awk only (BSD awk on a Mac, mawk on Debian).
#
#   awk -v gates="scripts/ci/run-local.sh scripts/ci/gate.sh scripts/ci/gate-wait.sh" -f gate-reader.awk
#
# Output, one line each, only when true:
#   GATE   a gate is INVOKED: one of `gates` in COMMAND POSITION, as that path, `./` and that path,
#          or any word ending `/<that path>` (an absolute path, "$CLAUDE_PROJECT_DIR"/…, ../…), never
#          another script of the same file name (`.claude/checks/gate.sh`). Command position is the first
#          word of a simple command, after `VAR=value` prefixes and any wrapper (nohup, env,
#          timeout 600, time, exec, nice, xargs, `{`, `!`, if/then/do …), or the script a shell runs
#          (`bash scripts/ci/gate.sh`, `. scripts/ci/gate.sh`), read recursively through `sh -c "…"`,
#          `eval "…"`, `env -S "…"`, `$( … )` and backticks (arithmetic `$(( … ))` is not a command).
#          A MENTION is not an invocation:
#          `cat scripts/ci/gate.sh`, `git log -- scripts/ci/run-local.sh`, a commit message or a
#          heredoc body naming them.
#   AMP    an `&` outside quotes, comments and heredoc bodies sends something to the shell's
#          background (`&&`, `>&2`, `2>&1`, `&>` and `|&` are not that). The hook counts ANY such
#          `&` in a command that invokes a gate, so `sleep 1 & scripts/ci/gate.sh` is refused too:
#          broader than the shell, never narrower.
#   CD     only with `-v cds=1`, one line per `cd` or `pushd` that moves THIS shell, in order: a tab,
#          then its target word as the shell would see it unexpanded (`~` for none; `$`, a glob or
#          `-` left in for the caller to refuse as unresolvable). Only a top-level command counts: a
#          `cd` inside `( … )`, `$( … )`, backticks or `sh -c "…"` moves a child shell, not this one.
#          Through a wrapper that runs a program (env, nohup, timeout, xargs) there is no builtin to run.
#
# KNOWN LIMITS (not seen, so allowed): a computed command word (`$G`, `$(echo gate.sh)`), an alias
# or a function, `find -exec`, a `cd` run through `eval` (read as a child here: CD misses it), a gate run as a script on a shell's stdin (`bash <<EOF`), and a gate
# reached by a path relative to another directory (`cd scripts/ci && ./gate.sh`), and a path that
# is partly computed (`D=scripts/ci; $D/gate.sh &`).
# Checked by .claude/checks/hooks.sh.

BEGIN {
  n = split(gates, g, " ")
  for (i = 1; i <= n; i++) if (g[i] != "") GATES[g[i]] = 1
  n = split("bash sh zsh dash ksh mksh", g, " ")
  for (i = 1; i <= n; i++) SHELLS[g[i]] = 1
  n = split("nohup setsid exec command builtin time nice caffeinate stdbuf env timeout xargs sudo ! { if then elif else while until do", g, " ")
  for (i = 1; i <= n; i++) WRAP[g[i]] = 1
  # Wrapper options that take the next word as their argument.
  n = split("-u -C -a -I -d -E -s -k -w -n -P -L", g, " ")
  for (i = 1; i <= n; i++) OPTARG[g[i]] = 1
  n = split("builtin command ! { if then elif else while until do", g, " ")
  for (i = 1; i <= n; i++) CDWRAP[g[i]] = 1
  D = 0
}
{ src = src $0 "\n" }
END { scan(src); if (G) print "GATE"; if (A) print "AMP" }

function base(t) { sub(/.*\//, "", t); return t }

# isgate T: T names a configured gate path: exactly, `./`-prefixed, or as the end of a longer path.
function isgate(t,   p) {
  # The same path however spelled: `//` and `/./` collapse, and `x/../` resolves lexically.
  gsub(/\/\/+/, "/", t)
  while (sub(/\/\.\//, "/", t)) ;
  while (match(t, /(^|\/)[^\/]+\/\.\.\//) && substr(t, RSTART + (substr(t, RSTART, 1) == "/"), 3) != "../") {
    t = substr(t, 1, RSTART - 1) (substr(t, RSTART, 1) == "/" ? "/" : "") substr(t, RSTART + RLENGTH)
  }
  sub(/^\.\//, "", t)
  for (p in GATES) if (t == p || (length(t) > length(p) && substr(t, length(t) - length(p)) == "/" p)) return 1
  return 0
}

# cdend: a `cd` with no target word goes home.
function cdend() { if (ST[D] == "cd") { print "CD\t~"; ST[D] = "rest" } }

# A new simple command at this depth.
function sep() { cdend(); ST[D] = "cmd"; WK[D] = ""; SKN[D] = 0; CF[D] = 0; NOX[D] = 0; EVS[D] = 0; RD[D] = 0 }

function endword(   t) {
  if (!HV[D]) return
  t = WB[D]; WB[D] = ""; HV[D] = 0
  if (HN[D]) { HQ[D, ++HP[D]] = t; HSTRIP[D, HP[D]] = HN[D] == 2; HN[D] = 0; return }
  if (RD[D]) { RD[D] = 0; return }
  onword(t)
}

# onword T: one finished word of the simple command at depth D.
function onword(t,   b, st) {
  st = ST[D]
  if (st == "rest") return
  if (st == "cd") {
    if (t == "--" || t ~ /^-[LPe@]+$/) return
    print "CD\t" t; ST[D] = "rest"; return
  }
  if (st == "eval") { scan(t); return }
  if (st == "src") { if (isgate(t)) G = 1; ST[D] = "rest"; return }
  if (st == "shell") {
    if (SKN[D]) { SKN[D] = 0; return }
    if (t ~ /^--./) return
    if (t == "-o" || t == "+o" || t == "-O" || t == "+O") { SKN[D] = 1; return }
    if (t ~ /^[-+]./) {
      if (t ~ /^-/ && t ~ /c/) CF[D] = 1
      if (t ~ /^-/ && t ~ /n/) NOX[D] = 1
      if (t ~ /^-/ && t ~ /s/) ST[D] = "rest"
      return
    }
    if (t == "-" || t == "--") return
    if (CF[D]) { ST[D] = "rest"; scan(t); return }
    if (!NOX[D] && isgate(t)) G = 1
    ST[D] = "rest"
    return
  }
  # st == "cmd": looking for the command word.
  if (EVS[D]) { EVS[D] = 0; ST[D] = "rest"; scan(t); return }
  if (SKN[D]) { SKN[D] = 0; return }
  if (t ~ /^[A-Za-z_][A-Za-z0-9_]*\+?=/) return
  if (WK[D] != "" && t ~ /^-/) {
    if (WK[D] == "command" && t ~ /^-[vV]/) { ST[D] = "rest"; return }
    if (WK[D] == "env" && t == "-S") { EVS[D] = 1; return }
    if (t in OPTARG) SKN[D] = 1
    return
  }
  if (WK[D] != "" && t ~ /^[0-9.]+[smhd]?$/) return
  b = base(t)
  if (cds && D == 1 && !SUB && (t == "cd" || t == "pushd") && (WK[D] == "" || WK[D] in CDWRAP)) { ST[D] = "cd"; return }
  if (b in WRAP) { WK[D] = b; return }
  if (isgate(t)) { G = 1; ST[D] = "rest"; return }
  if (b in SHELLS) { ST[D] = "shell"; return }
  if (t == "eval") { ST[D] = "eval"; return }
  if (t == "." || t == "source") { ST[D] = "src"; return }
  ST[D] = "rest"
}

# close_paren S I [ARITH]: the index of the `)` matching the `(` at I, past the end if none. Quotes
# are skipped, and so are heredoc bodies (a `)` in one does not close it), except in arithmetic
# (ARITH), where `<<` is a shift.
function close_paren(s, i, arith,   n, d, c, hd, dl, st, e, line, k, m, q) {
  n = length(s); d = 0; hd = ""
  for (; i <= n; i++) {
    c = substr(s, i, 1)
    if (c == "\\") { i++; continue }
    if (c == "'") { while (++i <= n && substr(s, i, 1) != "'") ; continue }
    if (c == "\"") { while (++i <= n && substr(s, i, 1) != "\"") if (substr(s, i, 1) == "\\") i++; continue }
    if (!arith && c == "<" && substr(s, i + 1, 1) == "<" && substr(s, i + 2, 1) != "<") {
      # The delimiter: after `<<` or `<<-` and blanks, up to a blank or an operator, quotes dropped.
      i += 2; st = 0
      if (substr(s, i, 1) == "-") { st = 1; i++ }
      while (i <= n && substr(s, i, 1) ~ /[ \t]/) i++
      dl = ""
      while (i <= n && substr(s, i, 1) !~ /[ \t\n;&|<>()]/) { q = substr(s, i, 1); if (q != "'" && q != "\"" && q != "\\") dl = dl q; i++ }
      hd = hd SUBSEP st dl
      i--; continue
    }
    if (c == "\n" && hd != "") {
      # Skip each pending body, line by line, to its delimiter.
      m = split(substr(hd, 2), k, SUBSEP); hd = ""; i++
      for (e = 1; e <= m; e++) {
        st = substr(k[e], 1, 1); dl = substr(k[e], 2)
        while (i <= n) {
          q = index(substr(s, i), "\n"); if (q == 0) q = n - i + 2
          line = substr(s, i, q - 1); i += q
          if (st == "1") sub(/^\t+/, "", line)
          if (line == dl) break
        }
      }
      i--; continue
    }
    if (c == "(") d++
    else if (c == ")" && --d == 0) return i
  }
  return n + 1
}

# close_tick S I: the index of the backtick closing the one before I.
function close_tick(s, i,   n, c) {
  n = length(s)
  for (; i <= n; i++) { c = substr(s, i, 1); if (c == "\\") i++; else if (c == "`") return i }
  return n + 1
}

# subst S I: a `$(…)` (I at `$`) or a `…` (I at the backtick) read as commands of their own; returns
# the index after it. The word keeps a placeholder, so `$(…)/scripts/ci/gate.sh` still names a gate.
function subst(s, i,   j) {
  # Arithmetic, `$(( … ))`, holds no command: its `&` is a bitwise and, its `<<` a shift. It is
  # arithmetic only when the inner `(` closes right before the outer `)`: `$((cmd) 2>&1)` is a
  # command substitution that starts with a subshell.
  if (substr(s, i, 3) == "$((" && substr(s, close_paren(s, i + 2, 1) + 1, 1) == ")") {
    j = close_paren(s, i + 1, 1); WB[D] = WB[D] "0"; HV[D] = 1; return j + 1
  }
  if (substr(s, i, 1) == "`") { j = close_tick(s, i + 1); scan(substr(s, i + 1, j - i - 1)) }
  else { j = close_paren(s, i + 1); scan(substr(s, i + 2, j - i - 2)) }
  WB[D] = WB[D] "$()"; HV[D] = 1
  return j + 1
}

function scan(s,   n, i, c, c2, j, k, line, e, dl) {
  D++; ST[D] = ""; sep(); WB[D] = ""; HV[D] = 0; HN[D] = 0; HP[D] = 0
  n = length(s); i = 1
  while (i <= n) {
    c = substr(s, i, 1); c2 = substr(s, i + 1, 1)
    if (c == "\\") { if (c2 != "\n") { WB[D] = WB[D] c2; HV[D] = 1 } i += 2; continue }
    if (c == "'") {
      j = index(substr(s, i + 1), "'"); if (j == 0) j = n - i + 1
      WB[D] = WB[D] substr(s, i + 1, j - 1); HV[D] = 1; i += j + 1; continue
    }
    if (c == "\"") {
      HV[D] = 1; i++
      while (i <= n && substr(s, i, 1) != "\"") {
        c = substr(s, i, 1)
        if (c == "\\") { WB[D] = WB[D] substr(s, i + 1, 1); i += 2 }
        else if (c == "`" || (c == "$" && substr(s, i + 1, 1) == "(")) i = subst(s, i)
        else { WB[D] = WB[D] c; i++ }
      }
      i++; continue
    }
    if (c == "$" && c2 == "'") {
      for (j = i + 2; j <= n && substr(s, j, 1) != "'"; j++) if (substr(s, j, 1) == "\\") j++
      WB[D] = WB[D] substr(s, i + 2, j - i - 2); HV[D] = 1; i = j + 1; continue
    }
    if (c == "$" && c2 == "(") { i = subst(s, i); continue }
    if (c == "`") { i = subst(s, i); continue }
    if (c == "$" && c2 == "{") {
      j = index(substr(s, i), "}"); if (j == 0) j = n - i + 1
      WB[D] = WB[D] substr(s, i, j); HV[D] = 1; i += j; continue
    }
    if (c == "#" && !HV[D]) { while (i <= n && substr(s, i, 1) != "\n") i++; continue }
    if (c == " " || c == "\t") { endword(); i++; continue }
    if (c == "\n") {
      endword(); sep(); i++
      # The bodies of the heredocs opened on this line: skipped, never read as commands.
      for (k = 1; k <= HP[D]; k++) {
        dl = HQ[D, k]
        while (i <= n) {
          e = index(substr(s, i), "\n"); if (e == 0) e = n - i + 2
          line = substr(s, i, e - 1); i += e
          if (HSTRIP[D, k]) sub(/^\t+/, "", line)
          if (line == dl) break
        }
      }
      HP[D] = 0
      continue
    }
    if (c == ";") { endword(); sep(); i++; continue }
    if (c == "&") {
      endword()
      if (c2 == "&") { sep(); i += 2; continue }
      if (c2 == ">") { RD[D] = 1; i += 2; if (substr(s, i, 1) == ">") i++; continue }
      A = 1; sep(); i++; continue
    }
    if (c == "|") { endword(); sep(); i += (c2 == "|" || c2 == "&") ? 2 : 1; continue }
    # SUB: inside a top-level `( … )` subshell, where a `cd` moves only the child.
    if (c == "(" || c == ")") { endword(); sep(); if (D == 1) SUB += c == "(" ? 1 : (SUB > 0 ? -1 : 0); i++; continue }
    if (c == "<" || c == ">") {
      # A word of digits just before is the redirected descriptor (`2>`), not an argument.
      if (HV[D] && WB[D] ~ /^[0-9]+$/) { WB[D] = ""; HV[D] = 0 } else endword()
      if (c2 == "(") { sep(); i += 2; continue }          # <( … ) and >( … ): commands of their own
      if (c == "<" && substr(s, i, 3) == "<<<") { RD[D] = 1; i += 3; continue }
      if (c == "<" && c2 == "<") { HN[D] = substr(s, i + 2, 1) == "-" ? 2 : 1; i += HN[D] == 2 ? 3 : 2; continue }
      RD[D] = 1; i++
      if (c2 == ">" || c2 == "&" || c2 == "|" || (c == "<" && c2 == ">")) i++
      continue
    }
    WB[D] = WB[D] c; HV[D] = 1; i++
  }
  endword(); cdend()
  D--
}
