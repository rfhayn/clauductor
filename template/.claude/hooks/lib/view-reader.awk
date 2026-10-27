# view-reader.awk: Bash file-view guard for read-by-range.sh (on stdin) Ported from Standing Tee view-command.mjs. Separate from gate-reader.awk so gate guards do not drift. Not a shell: a tokenizer that knows quotes, escapes, `$(…)`, backticks, `${…}`,
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
# With `-v gits=1` (catch-up-guard.sh), in order, at any depth (a child's too: the caller reads
# them as a superset):
#   CD     a tab, "T" when THIS shell, or a `( … )` subshell of it, runs it ("C" in another shell,
#          or under if, then, while and the like, or in a case arm, where it may not run; a case
#          pattern's `)` closes no subshell), a tab and the target word
#          as above (`~` for none), for each
#          cd, pushd and chdir; "$" for a target nobody can read from here: a bare pushd (it swaps
#          the stack), popd, source or `.` (a script may cd), env -C, GIT_DIR=, GIT_WORK_TREE=,
#          GIT_COMMON_DIR= or GIT_CONFIG*=, --git-dir, --work-tree, --config-env, or `-c
#          core.worktree` or core.bare.
#   GIT    one line per git invocation, at its end: tab-separated, the -C chain composed ("." for
#          none, "?" when a -C is computed, or for a git `find -execdir` runs), the subcommand, the
#          word after it ("-" for none), 1 when a later word is -u or --update-head-ok (else 0), and
#          the value a `-c alias.<sub>=` on the line gives (empty for none). Git is `git` or a path
#          ending /git in command position, or ANY such word later in a simple command (an unknown
#          wrapper: `chronic git`). `gh pr checkout` and `gh repo sync` print as a git checkout.
#   DYN    a command word the shell computes (`$g`, `$( … )`): it may be git, or a cd. `eval`'s
#          words are joined and read as one command, as the shell does.
#   SEP    a tab and the operator that ends a simple command THIS shell runs (`;` for a newline
#          too, `&&`, `||`, `|`, `&`), so the caller can tell a cd that is sure to run from one that
#          may not, or that runs in a pipeline's or the background's subshell; and `(` and `)`
#          around a subshell, whose cds end at its `)`.
#
# KNOWN LIMITS (not seen, so allowed): a computed command word (`$G`, `$(echo gate.sh)`), an alias
# or a function, `find -exec`, a `cd` run through `eval` (read as a child here: CD misses it), a gate run as a script on a shell's stdin (`bash <<EOF`), and a gate
# reached by a path relative to another directory (`cd scripts/ci && ./gate.sh`), and a path that
# is partly computed (`D=scripts/ci; $D/gate.sh &`).
# Checked by .claude/checks/hooks.sh.

BEGIN {
  SUBST = "$()"
  n = split("bash sh zsh dash ksh mksh fish csh tcsh", g, " ")
  for (i = 1; i <= n; i++) SHELLS[g[i]] = 1
  n = split("nohup setsid exec command builtin time nice caffeinate stdbuf env timeout xargs sudo ! { if then elif else while until do", g, " ")
  for (i = 1; i <= n; i++) WRAP[g[i]] = 1
  n = split("-u -C -a -I -d -E -s -k -w -n -P -L", g, " ")
  for (i = 1; i <= n; i++) OPTARG[g[i]] = 1
  n = split("if then elif else while until do", g, " ")
  for (i = 1; i <= n; i++) CONDW[g[i]] = 1
  D = 0; ncmd = 0; pipe_start = 1; vcap = 0; vwc = 0
}
{ src = src $0 "\n" }
END {
  scan(src)
  for (i = 1; i <= ncmd; i++) {
    if (cmd_piped[i] || cmd_cap[i] || stdout_away(i)) continue
    argv = command_argv(i)
    if (argv == "") continue
    n = split(argv, av, "\t")
    msg = view_of(av, n)
    if (msg != "") { print "refuse\t" msg; exit }
  }
}

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

# gscope: "T" for a command this shell, or a `( … )` subshell of it the SEP lines bracket, runs
# itself; "C" for one another shell runs (`sh -c`, `eval`, `$( … )`).
function gscope() { return D == 1 ? "T" : "C" }

# cdscope: a cd under if, then, while and the like, or in a case arm, may not run: read like a child's.
function cdscope() { return (CW[D] || (D == 1 && CASEN > 0)) ? "C" : gscope() }

# flush: the simple command at depth D has ended. An eval's joined words are read only now.
function flush(   s) {
  cdend()
  if (!gits) return
  if (ST[D] == "gcd") { print "CD\t" cdscope() "\t" (GCW[D] == "pushd" ? "$" : "~"); ST[D] = "rest" }
  if (ST[D] == "gh") ST[D] = "rest"
  if (ST[D] == "git") gitend()
  if (ST[D] == "eval") { s = EVB[D]; EVB[D] = ""; ST[D] = "rest"; scan(s) }
}

# gitstart: a git invocation begins; one `find -execdir` runs is aimed where nobody can read.
function gitstart() { ST[D] = "git"; GDIR[D] = XD[D] ? "?" : "."; GNX[D] = ""; GCA[D] = ""; GSUB[D] = ""; GARG[D] = "-"; GHA[D] = 0; GU[D] = 0 }

# ghword T: one word after gh, its options skipped; `pr checkout` and `repo sync` move a checkout.
function ghword(t) {
  if (GHS[D]) { GHS[D] = 0; return }
  if (t == "-R" || t == "--repo") { GHS[D] = 1; return }
  if (t ~ /^-/) return
  if (GH1[D] == "") { GH1[D] = t; return }
  if ((GH1[D] == "pr" && t == "checkout") || (GH1[D] == "repo" && t == "sync")) print "GIT\t.\tcheckout\t-\t0\t"
  ST[D] = "rest"
}

# gitdir X: compose a -C the way git does, each read from the one before.
function gitdir(x) {
  if (GDIR[D] == "?" || x == "") return
  if (x ~ /[$`*?[]/ || x ~ /^~[^\/]/) { GDIR[D] = "?"; return }
  if (x ~ /^(\/|~)/ || GDIR[D] == ".") GDIR[D] = x
  else GDIR[D] = GDIR[D] "/" x
}

# gitcfg V: a `-c` setting; one that moves where git works (core.worktree, core.bare) re-aims it.
function gitcfg(v) {
  GCA[D] = GCA[D] "\n" v
  if (tolower(v) ~ /^core\.(worktree|bare)(=|$)/) print "CD\t" gscope() "\t$"
}

# reaims T: T is an assignment that moves where git works, or which config it reads.
function reaims(t) { return t ~ /^GIT_(DIR|WORK_TREE|COMMON_DIR|CONFIG[A-Z0-9_]*)=/ }

# gitword T: one word after git: its own options until the subcommand, then the subcommand's.
function gitword(t,   x) {
  if (GNX[D] != "") {
    x = GNX[D]; GNX[D] = ""
    if (x == "C") gitdir(t)
    else if (x == "c") gitcfg(t)
    return
  }
  if (GSUB[D] == "") {
    if (t == "-C") { GNX[D] = "C"; return }
    if (t ~ /^-C./) { gitdir(substr(t, 3)); return }
    if (t == "-c") { GNX[D] = "c"; return }
    if (t ~ /^-c./) { gitcfg(substr(t, 3)); return }
    if (t ~ /^--(git-dir|work-tree|config-env)$/) { print "CD\t" gscope() "\t$"; GNX[D] = "skip"; return }
    if (t ~ /^--(git-dir|work-tree|config-env)=/ || t == "--bare") { print "CD\t" gscope() "\t$"; return }
    if (t ~ /^--(namespace|super-prefix|attr-source)$/) { GNX[D] = "skip"; return }
    if (t ~ /^-/) return
    GSUB[D] = t; return
  }
  if (!GHA[D]) { GHA[D] = 1; if (t != "") GARG[D] = t }
  if (t == "-u" || t ~ /^--update-h/) GU[D] = 1
}

function gitend(   k, c, v, i, p) {
  ST[D] = "rest"
  if (GSUB[D] == "") return
  v = ""; p = "alias." GSUB[D] "="
  k = split(GCA[D], c, "\n")
  for (i = k; i >= 1; i--) if (index(c[i], p) == 1) { v = substr(c[i], length(p) + 1); break }
  print "GIT\t" GDIR[D] "\t" GSUB[D] "\t" GARG[D] "\t" GU[D] "\t" v
}

# sep [OP]: a new simple command at this depth, after the operator OP (gits: printed for this shell).
function sep(op) {
  flush()
  if (D == 1) vflush(op)
  if (gits && op != "" && D == 1) print "SEP\t" op
  ST[D] = "cmd"; WK[D] = ""; SKN[D] = 0; CF[D] = 0; NOX[D] = 0; EVS[D] = 0; RD[D] = 0; CW[D] = 0; XD[D] = 0
}

function endword(   t) {
  if (!HV[D]) return
  t = WB[D]; WB[D] = ""; HV[D] = 0
  if (HN[D]) { HQ[D, ++HP[D]] = t; HSTRIP[D, HP[D]] = HN[D] == 2; HN[D] = 0; return }
  if (RD[D]) { RD[D] = 0; return }
  if (D == 1) vadd(t)
  onword(t)
}

# onword T: one finished word of the simple command at depth D.
function onword(t,   b, st) {
  st = ST[D]
  if (st == "rest") {
    if (gits && base(t) == "git") gitstart()
    else if (gits && base(t) == "gh") { ST[D] = "gh"; GH1[D] = ""; GHS[D] = 0 }
    else if (gits && reaims(t)) print "CD\t" gscope() "\t$"
    else if (gits && (t == "-execdir" || t == "-okdir")) XD[D] = 1
    return
  }
  if (st == "git") { gitword(t); return }
  if (st == "gh") { ghword(t); return }
  if (st == "gcd") {
    if (t == "--" || t ~ /^-[LPe@]+$/) return
    print "CD\t" cdscope() "\t" t; ST[D] = "rest"; return
  }
  if (st == "eval" && gits) { EVB[D] = EVB[D] " " t; return }
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
  if (t ~ /^[A-Za-z_][A-Za-z0-9_]*\+?=/) { if (gits && reaims(t)) print "CD\t" gscope() "\t$"; return }
  if (WK[D] != "" && t ~ /^-/) {
    if (WK[D] == "command" && t ~ /^-[vV]/) { ST[D] = "rest"; return }
    if (gits && WK[D] == "env" && t ~ /^(-C|--chdir)/) print "CD\t" gscope() "\t$"
    if (WK[D] == "env" && t == "-S") { EVS[D] = 1; return }
    if (t in OPTARG) SKN[D] = 1
    return
  }
  if (WK[D] != "" && t ~ /^[0-9.]+[smhd]?$/) return
  b = base(t)
  if (cds && D == 1 && !SUB && (t == "cd" || t == "pushd") && (WK[D] == "" || WK[D] in CDWRAP)) { ST[D] = "cd"; return }
  # CASEN: inside a top-level case, whose patterns end in a `)` that closes no subshell.
  if (D == 1 && t == "case") { CASEN++; CASESUB[CASEN] = SUB; ST[D] = "rest"; return }
  if (D == 1 && t == "esac" && CASEN > 0) { CASEN--; ST[D] = "rest"; return }
  if (gits) {
    if (b == "git") { gitstart(); return }
    if (b == "gh") { ST[D] = "gh"; GH1[D] = ""; GHS[D] = 0; return }
    if (t == "cd" || t == "pushd" || t == "chdir") { ST[D] = "gcd"; GCW[D] = t; return }
    if (t == "popd" || t == "." || t == "source") { print "CD\t" gscope() "\t$"; ST[D] = "rest"; return }
    if (t == "eval") { ST[D] = "eval"; EVB[D] = ""; return }
    if (t ~ /\$/) { print "DYN"; ST[D] = "rest"; return }
  }
  if (b in WRAP) { WK[D] = b; if (b in CONDW) CW[D] = 1; return }
  if (isgate(t)) { G = 1; ST[D] = "rest"; return }
  if (b in SHELLS) { ST[D] = "shell"; return }
  if (t == "eval") { ST[D] = "eval"; return }
  if (t == "." || t == "source") { ST[D] = "src"; return }
  ST[D] = "rest"
}


function vadd(w) { if (vwc < 512) { vwc++; vw[vwc] = w } }

function vflush(op,   i, idx) {
  if (vwc > 0) {
    ncmd++; idx = ncmd
    cmd_wc[idx] = vwc; cmd_cap[idx] = vcap; cmd_piped[idx] = 0; cmd_gc[idx] = 0
    for (i = 1; i <= vwc; i++) cmd_w[idx, i] = vw[i]
    vwc = 0
  }
  if (op == "|") for (i = pipe_start; i <= ncmd; i++) cmd_piped[i] = 1
  if (op != "" && op != "|") pipe_start = ncmd + 1
}

function computed(w) { return index(w, SUBST) || match(w, /[$*?[\]{}]/) }

function abs_path(w) {
  if (w == "" || w == "-") return ""
  if (w == "~") return ENVIRON["HOME"]
  if (substr(w, 1, 2) == "~/") return ENVIRON["HOME"] substr(w, 2)
  if (substr(w, 1, 1) == "/") return w
  return cwd "/" w
}

function line_count(path,   ap, n, c) {
  ap = abs_path(path)
  if (ap == "" || system("test -f \"" ap "\" && test -r \"" ap "\"") != 0) return -1
  n = 0
  while ((getline c < ap) > 0) n++
  close(ap)
  return n
}

function files_total(words, n,   i, t, lc) {
  t = 0
  for (i = 1; i <= n; i++) {
    if (words[i] == "-" || computed(words[i])) return -1
    lc = line_count(words[i]); if (lc < 0) return -1
    t += lc
  }
  return t
}

function sed_span_one(part, total,   a, b, p) {
  if (part ~ /^[qQ]$/ || part ~ /^[0-9]+[qQ]$/) return 0
  if (!match(part, /^[0-9]+/)) return -1
  a = substr(part, RSTART, RLENGTH) + 0
  p = index(part, ",")
  if (p == 0) b = a
  else {
    b = substr(part, p + 1); sub(/p$/, "", b)
    if (b == "$") b = total
    else if (substr(b, 1, 1) == "+") b = a + substr(b, 2) + 0
    else b = b + 0
  }
  if (b > total) b = total
  if (a > total) return 0
  return (b >= a ? b - a + 1 : 0)
}

function sed_span(scripts, nscr, total,   si, np, parts, pi, span) {
  span = 0
  for (si = 1; si <= nscr; si++) {
    np = split(scripts[si], parts, /[;\n]/)
    for (pi = 1; pi <= np; pi++) {
      gsub(/^[ \t]+|[ \t]+$/, "", parts[pi])
      if (parts[pi] == "") continue
      si2 = sed_span_one(parts[pi], total)
      if (si2 < 0) return -1
      span += si2
    }
  }
  return span
}

function view_of(av, n,   tool, i, w, rest, nrest, total, quiet, scripts, nscr, span, count, c, lines, si2) {
  tool = base(av[1])
  if (tool == "cat") {
    nrest = 0
    for (i = 2; i <= n; i++) {
      w = av[i]
      if (substr(w, 1, 1) == "-" && length(w) > 1 && substr(w, 2, 1) != "-") continue
      rest[++nrest] = w
    }
    total = files_total(rest, nrest)
    if (total > max) return "`cat` here prints " total " lines, over " max ", into your context, and every later call re-reads them as cache."
    return ""
  }
  if (tool == "sed") {
    quiet = 0; nscr = 0; nrest = 0
    for (i = 2; i <= n; i++) {
      w = av[i]
      if (w == "-e" || w == "--expression") { scripts[++nscr] = av[++i]; continue }
      if (substr(w, 1, 14) == "--expression=") { scripts[++nscr] = substr(w, 15); continue }
      if (w == "--quiet" || w == "--silent") { quiet = 1; continue }
      if (w ~ /^-[a-zA-Z]+$/) {
        if (index(w, "n")) quiet = 1
        if (w ~ /[fi]/) return ""
        if (substr(w, length(w), 1) == "e") { scripts[++nscr] = av[++i]; continue }
        continue
      }
      rest[++nrest] = w
    }
    if (!quiet) return ""
    if (nscr == 0 && nrest > 0) {
      scripts[1] = rest[1]; nscr = 1
      for (i = 1; i < nrest; i++) rest[i] = rest[i + 1]
      nrest--
    }
    for (i = 1; i <= nscr; i++) if (index(scripts[i], SUBST) || index(scripts[i], "$")) return ""
    total = files_total(rest, nrest); if (total < 0) return ""
    span = sed_span(scripts, nscr, total)
    if (span > max) return "`sed -n` here prints " span " lines, over " max ", into your context, and every later call re-reads them as cache."
    return ""
  }
  if (tool == "head" || tool == "tail") {
    count = "10"; nrest = 0
    for (i = 2; i <= n; i++) {
      w = av[i]
      if (w == "-n" || w == "--lines") { count = av[++i]; continue }
      if (substr(w, 1, 8) == "--lines=") { count = substr(w, 9); continue }
      if (substr(w, 1, 2) == "-n" && length(w) > 2) { count = substr(w, 3); continue }
      if (w ~ /^-[0-9]+$/) { count = substr(w, 2); continue }
      if (w == "-c" || substr(w, 1, 2) == "-c" || substr(w, 1, 7) == "--bytes" || w == "-f" || w == "-F") return ""
      if (substr(w, 1, 1) == "-" && length(w) > 1) continue
      rest[++nrest] = w
    }
    if (nrest == 0 || count !~ /^[+-]?[0-9]+$/) return ""
    c = count + 0; if (c < 0) c = -c
    lines = 0
    for (i = 1; i <= nrest; i++) {
      total = files_total(rest[i], 1); if (total < 0) return ""
      if (tool == "head") {
        if (substr(count, 1, 1) == "-") lines += (total > c ? total - c : 0)
        else lines += (c < total ? c : total)
      } else {
        if (substr(count, 1, 1) == "+") lines += (total >= c ? total - c + 1 : 0)
        else lines += (c < total ? c : total)
      }
    }
    if (lines > max) return "`" tool "` here prints " lines " lines, over " max ", into your context, and every later call re-reads them as cache."
    return ""
  }
  return ""
}

function redir_stdout(w,   t, op) {
  if (match(w, /^(&|1)?(>&|>>|>\||>)/)) {
    op = substr(w, RSTART, RLENGTH)
    t = substr(w, RSTART + RLENGTH)
    if (op == ">&" && t ~ /^\/dev\/(stdout|stderr|tty|fd\/[12])/) return 0
    if (t ~ /^\/dev\/(stdout|stderr|tty|fd\/[12])/) return 0
    return 1
  }
  if (match(w, /^[0-9]+(>>|>\||>|<|>&)/)) {
    t = substr(w, RSTART + RLENGTH)
    if (t ~ /^\/dev\/(stdout|stderr|tty|fd\/[12])/) return 0
    return 1
  }
  return -2
}

function stdout_away(i,   j, g, away, r) {
  away = -1
  for (j = 1; j <= cmd_wc[i]; j++) { r = redir_stdout(cmd_w[i, j]); if (r >= 0) away = r }
  if (away >= 0) return away
  for (g = 1; g <= cmd_gc[i]; g++)
    for (j = 1; j <= cmd_gwc[i, g]; j++) { r = redir_stdout(cmd_gw[i, g, j]); if (r >= 0) away = r }
  return away >= 0 ? away : 0
}

function strip_redirects(words, n,   out, i, w) {
  out = 0
  for (i = 1; i <= n; i++) {
    w = words[i]
    if (w ~ /^(\d+|&)?(<<<|<>|>>|>\||>&|<&|>|<)$/) { i++; continue }
    if (w ~ /^(\d+|&)?(<<<|<>|>>|>\||>&|<&|>|<)./) continue
    words[++out] = w
  }
  return out
}

function command_argv(i,   words, n, j, w, b, k, s) {
  n = 0
  for (j = 1; j <= cmd_wc[i]; j++) words[++n] = cmd_w[i, j]
  n = strip_redirects(words, n)
  k = 1
  while (k <= n) {
    w = words[k]; b = base(w)
    if (w ~ /^[A-Za-z_][A-Za-z0-9_]*=/) { k++; continue }
    if (b in WRAP) { k++; continue }
    if (b == "env") {
      k++
      while (k <= n && (substr(words[k], 1, 1) == "-" || words[k] ~ /^[A-Za-z_][A-Za-z0-9_]*=/)) {
        if (words[k] == "-S" || words[k] ~ /^-S./ || words[k] ~ /^--split-string=/) return ""
        k++
      }
      continue
    }
    if (b == "timeout" || b == "gtimeout") {
      k++; while (k <= n && substr(words[k], 1, 1) == "-") k += (words[k] ~ /^-[sk]$/) ? 2 : 1
      k++; continue
    }
    if (b == "nice") {
      k++; if (words[k] == "-n") k += 2; else if (k <= n && substr(words[k], 1, 1) == "-") k++
      continue
    }
    if (b == "eval") return ""
    if ((tolower(b) in SHELLS)) return ""
    s = words[k]
    for (j = k + 1; j <= n; j++) s = s "\t" words[j]
    return s
  }
  return ""
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
  if (substr(s, i, 1) == "`") { j = close_tick(s, i + 1); vcap_save = vcap; vcap = 1; scan(substr(s, i + 1, j - i - 1)); vcap = vcap_save }
  else { j = close_paren(s, i + 1); vcap_save = vcap; vcap = 1; scan(substr(s, i + 2, j - i - 2)); vcap = vcap_save }
  WB[D] = WB[D] "$()"; HV[D] = 1
  return j + 1
}

function scan(s,   n, i, c, c2, j, k, line, e, dl) {
  D++; ST[D] = ""; vwc = 0; sep(); WB[D] = ""; HV[D] = 0; HN[D] = 0; HP[D] = 0
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
      # To its `}`; each `$( … )` or backtick in it (`${x:=$(cmd)}`) is read as a command of its
      # own, and the expansion itself is a word, never a command line.
      e = WB[D]
      for (j = i + 2; j <= n; j++) {
        c = substr(s, j, 1)
        if (c == "\\") j++
        else if (c == "`" || (c == "$" && substr(s, j + 1, 1) == "(")) j = subst(s, j) - 1
        else if (c == "}") break
      }
      WB[D] = e substr(s, i, j - i + 1); HV[D] = 1; i = j + 1; continue
    }
    if (c == "#" && !HV[D]) { while (i <= n && substr(s, i, 1) != "\n") i++; continue }
    if (c == " " || c == "\t") { endword(); i++; continue }
    if (c == "\n") {
      endword(); sep(";"); i++
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
    if (c == ";") { endword(); sep(";"); i++; continue }
    if (c == "&") {
      endword()
      if (c2 == "&") { sep("&&"); i += 2; continue }
      if (c2 == ">") { RD[D] = 1; i += 2; if (substr(s, i, 1) == ">") i++; continue }
      A = 1; sep("&"); i++; continue
    }
    if (c == "|") { endword(); sep(c2 == "|" ? "||" : "|"); i += (c2 == "|" || c2 == "&") ? 2 : 1; continue }
    # SUB: inside a top-level `( … )` subshell, where a `cd` moves only the child.
    if (c == "(" || c == ")") {
      endword(); sep()
      if (D == 1 && c == "(") { SUB++; if (gits) print "SEP\t(" }
      else if (D == 1 && CASEN > 0 && SUB == CASESUB[CASEN]) { }   # a case pattern's end
      else if (D == 1 && SUB > 0) { SUB--; if (gits) print "SEP\t)" }
      i++; continue
    }
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
  endword(); flush(); if (D == 1) vflush("")
  D--
}
