#!/bin/sh
# PreToolUse hook (matcher: Bash). Refuses an in-place scripted REWRITE of tracked source.
#
# WHY. A `python3 … s.replace(old, new)` whose anchor was not unique (Python's replace is
# ALL-OCCURRENCES) edited two functions while one was being looked at, and a regex removal that
# matched into a multi-line expression produced a file that no longer parsed. Neither was visible
# in the command that caused it. A log entry about the hazard stopped neither (AGENTS.md rule 4).
#
# THE ALTERNATIVE IS BUILT IN. The Edit tool requires old_string to be UNIQUE and fails loudly
# when it is not. `replace_all: true` exists for when replacing everywhere is the intent, and
# then you have said so.
#
# HOW IT DECIDES, and the failed designs each step replaced (the history is the design):
#   - WHAT IS SOURCE is asked of the authority, `git ls-files`, never of a glob list: a typed list
#     of source extensions left a third of the tracked code uncovered.
#   - WHAT IS WRITTEN is asked by position: heredoc bodies fed to cat or `git commit -F -`,
#     triple-quoted strings, `-m "…"` messages and the arguments of .replace() are prose, and a
#     tracked path merely MENTIONED there blocked docs and commit messages three separate times.
#   - WHAT IS IN-PLACE is the -i flag wherever it sits in the cluster (`perl -0pi`, `sed -E -i`,
#     `--in-place`), not a list of spellings: the list missed `perl -0pi` and it walked through.
#   - WHICH FILES is read the way sh will read the command: a quote-aware scan resolves a literal
#     cd, a glob (via git ls-files) and any worktree of this repository, and FAILS CLOSED on what
#     it cannot resolve (find -exec, xargs, a variable, a substitution, a brace list).
#   - WHAT IS CODE includes a SCRIPT FILE an interpreter is given (`python3 fix.py`, `node x.mjs`,
#     `sh sub.sh`, `perl x.pl`, `./fix.py`, `. sub.sh`, `python3 < fix.py`, `cat fix.py | python3`,
#     `python3 -m fix`): the command carried no rewrite text, so a rewrite one file away walked
#     through twice (#88). The file is read and judged as inline code is, with one narrowing: only
#     a target that RESOLVES to a tracked file counts from a script's text, because refusing what
#     it cannot resolve refused every scratch script editing "$tmp/x". And one widening: an editor
#     a non-shell script runs from inside a string (`os.system("sed -i … f")`) is judged by the
#     paths the script names (step 0 says when). A check-only run
#     (`sh -n`, `node --check`, `perl -c`) executes nothing and reads nothing.
#
# WHICH SCRIPT FILES ARE READ: an UNTRACKED, unignored file in this repository (any worktree) or a
# temp dir, up to 64 KiB, outside node_modules and site-packages. A throwaway rewrite script is all
# of those. A tracked script is reviewed code whose writes are the project's intent (a build step,
# a codemod that was committed); an ignored one is build output (dist/cli.js); a large one or a
# dependency is a program. Reading those would turn their incidental `.replace(` and `.write(` into
# refusals. A script that cannot be read is ALLOWED, the
# direction the inline path takes for what it cannot see as a rewrite: a missing or unreadable file
# is one the interpreter cannot run either, so nothing is rewritten. (Without jq the hook still
# fails closed, now on an interpreter or a script name too: see unreadable() below.)
#
# KNOWN LIMITS of the script-file path, each allowed:
#   - a script written and run in ONE command (`cat > f.py <<EOF … EOF && python3 f.py`): the file
#     does not exist yet when the hook runs, and the heredoc body fed to cat is data, not code;
#   - a script named by a variable or a substitution (`python3 "$S"`, `bash -c "$(cat f)"`), piped
#     in by anything but a lone `cat f`, or kept in an ignored directory (`tmp/`, when ignored);
#   - a runner the hook does not know (`uv run f.py`, `deno run f.ts`, `bun f.ts`, `make`), and a
#     script run by its path with no script extension or behind a prefix (`./edsh`,
#     `timeout 60 ./fix.py`): a path is read as a script only when it is the command word;
#   - a script run by the script read (one level is read, not the tree it starts), including an
#     interpreter a shell script hands an editor in a heredoc or a `-c` string;
#   - a script's target it cannot resolve to a tracked file: a variable or an argument (`sed -i …
#     "$1"`), or a path it builds or finds itself (`Path(__file__).parent / name`, `glob`). A
#     RELATIVE literal is still judged against the command's directories and literal cds, even
#     after the script's own cd to a variable (safe: it can only over-refuse), so a script that
#     edits a scratch copy names it by an absolute path or a variable;
#   - past the 32-base cap, a relative target under a cd composed from two of the script's own cds
#     (each script cd is then resolved against the command's directories alone);
#   - an editor a non-shell script runs from a string with no exec call the hook knows (a call it
#     spells some other way), or as a LIST (`subprocess.run(["sed", "-i", …])`): no `sed -i` to
#     match. One in backticks or qx IS read (the scan reads a backtick as a command), but only
#     with a literal operand: a variable one is missed;
#   - a rewrite the inline detection does not recognise either (perl `$^I`, ruby `gsub`);
#   - (inline too, as before OPS-80) a rewrite that only PRINTS (`print(`, `console.log(`) while
#     the command sends its output onto tracked source (`> tmp && mv tmp src/x.ts`,
#     `| sponge src/x.ts`, `| tee src/x.ts`): a print was never a write;
#   - (OPS-80) a stream write (`process.stdout.write(`, `sys.stdout.write(`) is not counted as a
#     write ONLY on a direct, unpiped, unredirected run of a script (or inline code) that writes to
#     its own standard streams, launches nothing and rebinds no stream (step 2). What still
#     passes: that run's output moved onto tracked source by a LATER Bash call, or a stream
#     rebound by a spelling step 2 does not know (`os.dup2`).
# And three false refusals:
#   - shared with the inline path: a script that reads tracked source, uses `.replace(`, and writes
#     ELSEWHERE is refused when it names the tracked path;
#   - the price of reading an editor run from a string: a non-shell script that execs anything,
#     names `sed -i` anywhere (a comment too) and names a tracked path is refused, even if it only
#     reads that path;
#   - a command that names the tracked original AND runs a script editing only a copy by absolute
#     path (`cp src/x.ts /tmp/x.ts && python3 fix-copy.py`) is refused: the command's own words are
#     read loosely for tracked paths, which is what catches `python3 argv.py src/x.ts`.
#
# Checked by .claude/checks/hooks.sh, a payload → exit-code table in both directions.
#
# Protocol: exit 2 = block (stderr reaches Claude); exit 0 = allow.

payload=$(cat)

# PARSED WITH jq. Without a working jq this hook FAILS CLOSED, for a command it could police and
# only then: when the payload cannot be read, the command's RAW JSON text (raw_command) decides,
# against the words step 1 below gates on: `sed`, `perl`, `ruby`, `.replace(`, `re.sub(`. Every
# command this hook can block contains one of them, and JSON encoding escapes none of their
# characters, so the raw test is at least as broad as the decoded one. A script-file run (#88) can
# be blocked with none of them on the line, so an interpreter word or a script name decides too.
# Anything else (`ls`, `git status`) is still allowed. The price is a false block on a command merely containing one of
# those words while jq is broken, and the message says how to end it. A guard that ALLOWS when a
# dependency is missing is a control silently absent wherever the dependency is.
#
# raw_command: the RAW JSON text of every `"command": "…"` value in the payload, one per line,
# escapes left as they are; the whole payload if there is none. A `"command"` key cannot be forged
# from inside a string, because a quote inside a JSON string is always escaped. The same function
# is in pr-merge-guard.sh, inline rather than a sourced lib because a `.` of a missing file is
# fatal with a shell-dependent exit code.
raw_command() {
  _raw=$(printf '%s' "$payload" | awk '
    { buf = buf $0 "\n" }
    END {
      s = buf; found = 0
      while (match(s, /"command"[ \t\r\n]*:[ \t\r\n]*"/)) {
        s = substr(s, RSTART + RLENGTH); found = 1; n = length(s); i = 1
        while (i <= n) {
          c = substr(s, i, 1)
          if (c == "\\") { i += 2; continue }
          if (c == "\"") break
          i++
        }
        print substr(s, 1, i - 1); s = substr(s, i + 1)
      }
      if (!found) printf "%s", buf
    }
') || return 1
  printf '%s\n' "$_raw"
}
# An interpreter that can be given a script file, as a word; and a script file's name.
interp_re='python[0-9.]*|pypy[0-9.]*|node|nodejs|tsx|ts-node|perl|ruby|sh|bash|zsh|dash|ksh'
script_ext_re='[.](py|js|mjs|cjs|pl|rb|sh)'
unreadable() {
  raw=$(raw_command) || raw=$payload
  case "$raw" in
    *sed*|*perl*|*ruby*|*".replace("*|*"re.sub("*) policed=yes ;;
    *) printf '%s' "$raw" | grep -Eq "(^|[^[:alnum:]_.-])($interp_re|source)([^[:alnum:]_.-]|\$)|$script_ext_re([^[:alnum:]_]|\$)" \
         && policed=yes ;;
  esac
  if [ "${policed:-}" = yes ]; then
    echo "BLOCKED by no-blind-source-rewrite: $1, so this hook cannot read the command." >&2
    echo "It could be an in-place rewrite of tracked source, and the hook refuses rather than guess." >&2
    echo "Install jq (macOS: brew install jq; Debian/Ubuntu: apt-get install jq), check that 'jq --version' works, then retry." >&2
    exit 2
  fi
  exit 0
}
command -v jq >/dev/null 2>&1 || unreadable "jq is not installed (not on PATH)"
# Line 1 is the payload's `cwd`: the Bash tool's own directory, which is NOT necessarily the
# directory this hook runs in (a subagent in a worktree, a session that has cd'd). The rest is the
# command.
parsed=$(printf '%s' "$payload" \
  | jq -rj '((.cwd // "") | tostring | gsub("\n"; "")) + "\n" + ((.tool_input.command // "") | tostring)' 2>/dev/null) \
  || unreadable "jq failed to read the hook payload"
pcwd=$(printf '%s\n' "$parsed" | sed -n 1p)
cmd=$(printf '%s\n' "$parsed" | sed 1d)

# An EMPTY command, read successfully, runs nothing — there is nothing to police.
[ -z "$cmd" ] && exit 0

# HEREDOC BODIES ARE DATA, NOT OPERATIONS — stripped before anything else looks at the command.
#
# A commit message written with `git commit -F - <<'MSG'` is part of the command string, so prose
# describing this very hook ("`sed -i` was never caught", "Python's .replace() is silent") supplied
# both the rewrite verb and the write verb, and any tracked path elsewhere on the line completed
# the match: it blocked the commit that was fixing the hook's first false positive.
#
# Safe to strip: the file a heredoc WRITES to is named outside the body (`cat > f <<EOF`), so
# removing the body never hides a real target.
#
# EXCEPT when the heredoc is fed to an INTERPRETER. `python3 <<'PY' … PY` is code, and it is the
# form most scripted edits actually take; stripping it would blind the hook to its own primary
# target. The distinction is what the body is fed to: an interpreter runs it, `cat >` and
# `git commit -F -` consume it.
#
# Each pass over the command is a function, because a script file's text (step 0 below) goes
# through the same passes before it is judged.
strip_heredocs() {
  awk '
  /<<-?[ ]*'"'"'?[A-Za-z_][A-Za-z0-9_]*'"'"'?/ && !inbody {
    line = $0
    # NOTE: NO APOSTROPHES ANYWHERE IN THIS AWK PROGRAM, comments included. It sits inside a
    # single-quoted shell string, so one apostrophe closes that string and the whole hook becomes a
    # syntax error, which blocks EVERY Bash call until the file is read with a non-Bash tool.
    # ASK WHICH COMMAND OWNS THE HEREDOC, rather than whether the line mentions an interpreter.
    # A substring test let `git status --short` supply "sh", so a commit message body was kept as
    # code; a token test requiring a following space missed `python3<<PY` and still kept the body
    # for any PIPELINE containing an interpreter (`node x.mjs && git commit -F - <<MSG`).
    # The owner is the first word of the last pipeline segment before the `<<`.
    pre = substr(line, 1, index(line, "<<") - 1)
    n = split(pre, segs, /[;&|]+/)
    owner = segs[n]
    sub(/^[ \t]+/, "", owner)
    split(owner, toks, /[ \t]+/)
    name = toks[1]
    sub(/^.*\//, "", name)
    if (name ~ /^(python3?|perl|ruby|bash|sh|node)$/) { print; next }   # body is CODE — keep
    if (match(line, /<<-?[ ]*'"'"'?[A-Za-z_][A-Za-z0-9_]*'"'"'?/)) {
      tag = substr(line, RSTART, RLENGTH)
      gsub(/^<<-?[ ]*'"'"'?/, "", tag); gsub(/'"'"'$/, "", tag)
      inbody = 1; term = tag; print; next
    }
  }
  inbody && $0 == term { inbody = 0; next }
  inbody { next }
  { print }
'
}
cmd=$(printf '%s\n' "$cmd" | strip_heredocs)

# TRIPLE-QUOTED BODIES ARE DATA TOO.
#
# The heredoc strip above keeps an INTERPRETER's body, correctly. But a triple-quoted string INSIDE
# that code is data again, and this hook blocked a journal entry for it: the prose named a tracked
# script, the write target was the journal (excluded as markdown, correctly), and the tracked path
# mentioned in a sentence completed the match. Authority answers "is this source"; position answers
# "is this being written", and both questions have to be asked.
#
# Safe for the same reason the heredoc strip is safe: the file a command WRITES to is named in an
# ordinary string passed to `open(…, "w")` or `write_text(…)`. Nobody writes `open('''p''', 'w')`,
# so removing triple-quoted spans cannot hide a real target. It is applied BEFORE the rewrite and
# write tests on purpose — prose that merely discusses `.replace()` is not an operation either,
# which is the same class of false positive one step earlier.
#
# NOT a general "strip all strings": a single-quoted `p = 'src/x.ts'` on its own line is exactly
# how the real hazard is written (binding the path to a variable is the NORMAL shape of a
# read-modify-write), and stripping that would make a hook that blocks nothing while looking
# correct.
strip_triple() {
  awk '
  {
    line = $0
    if (inbody) {
      idx = index(line, term)
      if (idx == 0) next                                   # still inside the body — drop the line
      line = substr(line, idx + length(term)); inbody = 0   # body ended; keep what follows
    }
    while (1) {
      s3 = index(line, "\x27\x27\x27"); d3 = index(line, "\"\"\"")
      if (s3 == 0 && d3 == 0) break
      if (s3 > 0 && (d3 == 0 || s3 < d3)) { t = "\x27\x27\x27"; p = s3 } else { t = "\"\"\""; p = d3 }
      rest = substr(line, p + 3)
      e = index(rest, t)
      if (e > 0) { line = substr(line, 1, p - 1) substr(rest, e + 3); continue }  # inline pair
      line = substr(line, 1, p - 1); inbody = 1; term = t; break                  # opener only
    }
    print line
  }
'
}
cmd=$(printf '%s\n' "$cmd" | strip_triple)

# Empty once heredoc bodies and triple-quoted spans are gone: the command was only data.
[ -z "$cmd" ] && exit 0

# IN-PLACE STREAM EDITORS, MATCHED BY THE FLAG RATHER THAN BY ITS SPELLING.
#
# A list of spellings (`"sed -i"`, `"perl -pi"`, …) is a SAMPLE of the forms (*Enumerate the
# authority*), and `perl -0pi -e 's/a/b/' tracked.ts` walked straight through one. For sed, perl
# and ruby, `-i` IS the in-place write, wherever it sits in the cluster and whatever it is bundled
# with: `perl -i.bak`, `sed -E -i`, `sed --in-place`, `perl -ni`, `ruby -0pi`.
#
# The regex allows any number of preceding flags, then requires one flag token containing `i`.
# `perl -e`, `perl -MFoo` and `sed -E` alone do not match — their flag tokens have no `i` — so the
# ordinary read-only invocations stay allowed, which the exit-code table asserts in both directions.
inplace_re='(^|[^[:alnum:]_-])(sed|perl|ruby)([[:space:]]+-[[:alnum:].]+)*[[:space:]]+(--in-place|-[[:alnum:]]*i([.][[:alnum:]]+)?([^[:alnum:].]|$))'
if printf '%s' "$cmd" | grep -Eq "$inplace_re"; then
  inplace=yes
else
  inplace=no
fi

# A `-m "…"` MESSAGE IS PROSE TOO.
#
# Heredoc bodies and triple-quoted spans are already stripped as data. `git commit -m "…"` is the
# same thing in a third syntax: `git commit -m "Guard now catches perl -0pi and sed -E -i; see
# src/x.ts"` supplies the rewrite verb, the in-place flag and a tracked path, and writes no source.
#
# Safe: `-m` is a message or module flag in every command that takes it, and nothing writes a file
# through one. Quoted forms only, so `python3 -m mymodule` is untouched.
strip_messages() { sed -E 's/-m[[:space:]]+"[^"]*"/-m/g; s/-m[[:space:]]+'"'"'[^'"'"']*'"'"'/-m/g'; }
cmd=$(printf '%s\n' "$cmd" | strip_messages)

# ── THE COMMAND AS THE SHELL WILL SEE IT ─────────────────────────────────────────────────────
#
# Asking "which TOKENS in this string are tracked paths?" sees a target only if it is SPELLED as a
# tracked path relative to the repo root. `cd pkg && sed -i … src/index.ts`, a glob,
# `find -exec sed -i`, `xargs perl -pi`, a variable, and the hook running in a different directory
# from the Bash tool all passed that way, and every one fails OPEN.
#
# So this reads the command the way `sh` will: a quote-aware split into simple commands and words,
# each word marked for whether it carries an unquoted glob, a brace list, or an expansion (`$`, a
# backtick). It emits one line per fact:
#   INPLACE        an in-place editor (sed/gsed -i, perl/ruby -i) is invoked
#   OPAQUE <why>   ...and its target set is only known at run time — FAIL CLOSED
#   LIT <word>     a literal operand of that editor, resolved below against every cd
#   GLOB <word>    a glob operand, expanded below against `git ls-files` (the authority)
#   CD <dir>       a literal cd/pushd target;  CDX = one that is not a literal
#   SCRIPT <word>  a literal script file an interpreter is given, or run by its path (step 0)
# `sh -c`, `eval`, backticks and `$(…)` inside double quotes are scanned again as commands.
#
# FAIL CLOSED is deliberate. Resolving `find`, `xargs` and variables would mean re-implementing
# them; the one case this can PROVE is literal operands, so everything else in an in-place
# invocation is refused, and the message says why and what to do instead. The check pins the cost
# as its own row: `find /tmp/x -exec sed -i` is blocked too.
#
# Scanned only when the command could hold an in-place editor or name a script file; the python
# path asks for the cd facts later, and only once steps 1 and 2 have already matched.
TAB=$(printf '\t')
scan=""
scan_awk() {
  awk -v INTERP="^($interp_re)\$" -v SCRIPTEXT="$script_ext_re\$" '
    function reset_word() { word = ""; inword = 0; wglob = 0; wexp = 0; wbrace = 0; bopen = 0 }
    function endword() {
      if (inword) {
        # The word after a plain `<` is the file on stdin: a script, when an interpreter reads it.
        if (redir) { if (rin && !wexp && !wglob && !wbrace) RIN = word; redir = 0; rin = 0 }
        else { n++; W[n] = word; G[n] = wglob; X[n] = wexp; BR[n] = wbrace }
      }
      reset_word()
    }
    function lit(j) { return !X[j] && !G[j] && !BR[j] && W[j] !~ /[\t\n]/ }
    function clean(w) { gsub(/[\t\n]/, " ", w); return w }
    function queue(str, feed) { Q[++qn] = str; QF[qn] = feed }
    # WHO RUNS the word at k, when it is not the command word? "" means nobody: it is DATA to the
    # command (grep sed -i, echo … sed -i, find -name sed). find runs a word only after -exec and
    # its kin; a few commands are known to take words as text; ANY OTHER command word is presumed
    # to run it (xargs, env, sudo, timeout, parallel …) so an unknown one fails closed.
    function runner(k, c,   m) {
      if (cname == "find") {
        for (m = c + 1; m < k; m++) if (W[m] ~ /^-(exec|execdir|ok|okdir)$/) return "find"
        return ""
      }
      if (cname ~ /^(grep|egrep|fgrep|rg|ag|ack|git|gh|echo|printf|man|which|type|whatis|apropos|help|info)$/) return ""
      return cname
    }
    function endcmd(   c, k, j, name, tool, isinp, script, sawe, rest, sawc, r, pin, kind) {
      endword()
      # An empty command (the `(` of a subshell) runs nothing, and must not lose what a pipe
      # into it carries.
      if (n == 0) { redir = 0; return }
      # `cat f | python3`: the file the last command piped in, read when this one is an
      # interpreter given no file of its own.
      pin = PIPED; PIPED = ""
      c = 1
      while (c <= n && (W[c] ~ /^[A-Za-z_][A-Za-z0-9_]*=/ || W[c] ~ /^(!|[{]|then|do|else|elif|if|while|until|time)$/)) c++
      # `builtin cd`, `command sed`: the prefix runs the next word as the command itself.
      while (c < n && (W[c] == "builtin" || W[c] == "command") && W[c + 1] !~ /^-/) c++
      cname = ""
      if (c <= n) {
        cname = W[c]; sub(/^.*\//, "", cname)
        if (cname == "cd" || cname == "pushd") {
          for (j = c + 1; j <= n && W[j] ~ /^-/; j++) ;
          if (j > n || X[j] || G[j] || BR[j] || W[j] ~ /[\t\n]/ || W[j] ~ /^~[^\/]/) print "CDX"
          else print "CD\t" W[j]
        }
        if (cname == "eval") { rest = ""; for (j = c + 1; j <= n; j++) rest = rest " " W[j]; queue(rest, FEED) }
      }
      for (k = 1; k <= n; k++) {
        name = W[k]; sub(/^.*\//, "", name)
        # A SHELL WITH -c, wherever -c sits among its options (sh -e -c, bash -o pipefail -c,
        # bash --norc -c). Its script is re-scanned; when find or xargs started the shell, the
        # script inherits that, because they — not the script — decide what `{}` or "$1" is.
        if (name ~ /^(sh|bash|zsh|dash|ksh)$/) {
          sawc = 0
          for (j = k + 1; j <= n; j++) {
            if (W[j] ~ /^[-+][A-Za-z]*[oO]$/) { if (W[j] ~ /^-[A-Za-z]*c/) sawc = 1; j++; continue }
            if (W[j] ~ /^--(rcfile|init-file)$/) { j++; continue }
            if (W[j] ~ /^--/) continue
            if (W[j] ~ /^[-+][A-Za-z]+$/) { if (W[j] ~ /^-[A-Za-z]*c/) sawc = 1; continue }
            break
          }
          if (sawc && j <= n) {
            if (k == c) queue(W[j], FEED)
            else { r = runner(k, c); if (r != "") queue(W[j], r) }
          }
        }
        # AN INTERPRETER GIVEN A SCRIPT FILE, or a script run by its path (#88): the rewrite is in
        # the file, not on this line, so the file is named for step 0 to read. Only a literal name:
        # a variable or a substitution is a known limit (the header), not a guess. The shell reads
        # a sourced file (. f, source f) itself; an interpreter given no file reads stdin (a `<`
        # redirect, or cat piping one in); `python3 -m name` runs name.py from the cwd first.
        # A file a SHELL runs is SHSCRIPT: step 0 judges it by its scan alone (see there).
        if (k == c || runner(k, c) != "") {
          j = 0; kind = "SCRIPT\t"
          if (k == c && (name == "." || name == "source")) { j = (k < n) ? k + 1 : 0; kind = "SHSCRIPT\t" }
          else if (name ~ INTERP) {
            if (name ~ /^(sh|bash|zsh|dash|ksh)$/) kind = "SHSCRIPT\t"
            MODW = 0
            j = script_word(k, name)
            if (j == 0 && RIN != "" && RIN !~ /[\t\n]/) print kind RIN
            else if (j == 0 && pin != "") print kind pin
            if (MODW && lit(MODW)) print "MODULE\t" W[MODW]
          }
          else if (k == c && W[k] ~ /\// && W[k] ~ SCRIPTEXT) { j = k; if (W[k] ~ /[.]sh$/) kind = "SHSCRIPT\t" }
          if (j > 0 && lit(j)) print kind W[j]
        }
        tool = ""
        if (name ~ /^g?sed$/) tool = "sed"
        else if (name ~ /^(perl|ruby)$/) tool = "pl"
        if (tool == "") continue
        isinp = 0; sawe = 0; script = " "
        for (j = k + 1; j <= n; j++) {
          if (tool == "sed") {
            if (W[j] ~ /^--in-place/ || W[j] ~ /^-[A-Za-z]*i/) isinp = 1
            if (W[j] ~ /^(-[A-Za-z]*[ef]|--expression|--file)$/) { sawe = 1; script = script (j + 1) " " }
            if (W[j] ~ /^--(expression|file)=/) sawe = 1
          } else {
            if (W[j] ~ /^-[0-9aclnpsTtUuWwXS]*i/) isinp = 1
            if (W[j] ~ /^-[0-9aclnpsTtUuWwXSi.]*[eE]$/) { sawe = 1; script = script (j + 1) " " }
          }
        }
        if (!isinp) continue
        if (k != c) {
          r = runner(k, c)
          if (r == "") continue
          print "INPLACE"; print "OPAQUE\t" name " is run by " clean(r) ", which decides its files"; continue
        }
        print "INPLACE"
        if (FEED != "") { print "OPAQUE\t" name " runs in a shell started by " clean(FEED) ", which decides its files"; continue }
        if (!sawe) for (j = k + 1; j <= n; j++) if (W[j] != "" && W[j] !~ /^-/) { script = " " j " "; break }
        for (j = k + 1; j <= n; j++) {
          if (W[j] == "" || W[j] ~ /^-/ || index(script, " " j " ")) continue
          if (X[j] || BR[j] || W[j] ~ /[\t\n]/) { print "OPAQUE\tthe operand " clean(W[j]) " is only known at run time"; continue }
          print (G[j] ? "GLOB\t" : "LIT\t") W[j]
        }
      }
      # Only a lone literal file: `cat a b | python3` and `cat -n f` are not a script.
      if (cname == "cat" && SEP == "|" && n == c + 1 && lit(n)) PIPED = W[n]
      n = 0; redir = 0; RIN = ""
    }
    # THE SCRIPT FILE the interpreter at k is given, as a word index. 0: none is named, so the
    # script comes on stdin (also `-`). -1: nothing to read, because the code is inline (python3 -c,
    # node -e or -p, perl -e, sh -c: read as inline code already) or the run only CHECKS it and
    # executes nothing (sh -n, node --check, perl -c). `python3 -m name` is -1 with MODW set to
    # the module word. Options that take a separate value skip it.
    function script_word(k, name,   j, w) {
      for (j = k + 1; j <= n; j++) {
        w = W[j]
        if (w == "--") return (j < n) ? j + 1 : 0
        if (w == "-") return 0
        if (name ~ /^(sh|bash|zsh|dash|ksh)$/) {
          if (w !~ /^[-+]./) return j
          if (w ~ /^--(rcfile|init-file)$/) { j++; continue }
          if (w ~ /^-[A-Za-z]*[cn]/) return -1
          # A cluster ending in o takes the next word as its value: -o pipefail, -euo pipefail.
          if (w ~ /^[-+][A-Za-z]*[oO]$/) j++
          continue
        }
        if (w !~ /^-./) return j
        if (name ~ /^(python|pypy)/) {
          if (w ~ /^-[A-Za-z]*c/) return -1
          if (w ~ /^-[A-Za-z]*m/) { if (w ~ /m$/ && j < n) MODW = j + 1; return -1 }
          if (w ~ /^-[WX]$/) j++
        } else if (name ~ /^(perl|ruby)$/) {
          if (w ~ /^-[IMmr]/) { if (length(w) == 2) j++ }
          else if (w ~ /^-[A-Za-z0-9]*[eE]$/ || w ~ /^-[A-Za-z0-9]*c/) return -1
        } else {
          if (w ~ /^-[A-Za-z]*[epc]$/ || w ~ /^--(eval|print|check)/) return -1
          if (w ~ /^(-[rC]|--(require|import|loader|experimental-loader|conditions))$/) j++
        }
      }
      return 0
    }
    function grab(s, i, opn, cls,   depth, j, ch) {
      depth = 1
      for (j = i; j <= length(s); j++) {
        ch = substr(s, j, 1)
        if (ch == opn && opn != cls) depth++
        else if (ch == cls) { depth--; if (depth == 0) break }
      }
      queue(substr(s, i, j - i), FEED)
      return j
    }
    # THE NEXT CHARACTER, or "" at the end — and "" must never reach index(): BWK awk (macOS)
    # answers index(s, "") with a NON-ZERO, so a `while (index(set, next))` at the end of a string
    # spun forever, and a hook that hangs fails OPEN at Claude Code own timeout, minutes later.
    function isnext(set, s, i,   c) { c = substr(s, i + 1, 1); return c != "" && index(set, c) > 0 }
    function scan(s, feed,   i, L, ch, nx, q) {
      n = 0; redir = 0; rin = 0; RIN = ""; PIPED = ""; reset_word(); q = ""; FEED = feed
      L = length(s)
      for (i = 1; i <= L; i++) {
        ch = substr(s, i, 1)
        if (q == "s") { if (ch == SQ) q = ""; else word = word ch; continue }
        if (q == "d") {
          if (ch == "\\") {
            i++; nx = substr(s, i, 1)
            if (nx == "\n" || nx == "") continue
            if (index("$`\"\\", nx)) word = word nx; else word = word "\\" nx
            continue
          }
          if (ch == DQ) { q = ""; continue }
          if (ch == "$" && substr(s, i + 1, 1) == "(") { wexp = 1; word = word "$"; i = grab(s, i + 2, "(", ")"); continue }
          if (ch == "`") { wexp = 1; word = word "$"; i = grab(s, i + 1, "`", "`"); continue }
          if (ch == "$" && isnext(EXPCH, s, i)) wexp = 1
          word = word ch; continue
        }
        if (ch == "\\") { i++; nx = substr(s, i, 1); if (nx != "\n" && nx != "") { word = word nx; inword = 1 }; continue }
        if (ch == SQ) { q = "s"; inword = 1; continue }
        if (ch == DQ) { q = "d"; inword = 1; continue }
        if (ch == " " || ch == "\t") { endword(); continue }
        if (ch == "#" && !inword) { while (i < L && substr(s, i + 1, 1) != "\n") i++; continue }
        if (ch == ">" || ch == "<" || (ch == "&" && substr(s, i + 1, 1) == ">")) {
          # A bare number glued to the redirect is its FD (`2>/dev/null`), not a word — as the
          # first thing on a line it was being read as the command word.
          if (inword && word ~ /^[0-9]+$/) reset_word(); else endword()
          rin = (ch == "<" && !isnext("<>&(", s, i))
          while (i < L && isnext("<>&|", s, i)) i++
          redir = 1; continue
        }
        if (ch == ";" || ch == "&" || ch == "|" || ch == "(" || ch == ")" || ch == "\n") {
          if (ch == "(" && inword && substr(word, length(word)) == "$") { wexp = 1 }
          SEP = (ch == "|" && isnext("|", s, i)) ? "||" : ch
          endcmd(); continue
        }
        if (ch == "`") { wexp = 1; inword = 1; word = word "$"; i = grab(s, i + 1, "`", "`"); continue }
        if (ch == "$" && isnext(EXPCH, s, i)) wexp = 1
        if (ch == "*" || ch == "?" || ch == "[") wglob = 1
        if (ch == "{") bopen = 1
        if (ch == "," && bopen) wbrace = 1
        word = word ch; inword = 1
      }
      if (q != "") print "UNCLOSED"
      SEP = ""; endcmd()
    }
    BEGIN {
      SQ = sprintf("%c", 39); DQ = "\""
      EXPCH = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ_{(0123456789@*#?!$-"
    }
    { buf = buf $0 "\n" }
    END {
      Q[1] = buf; QF[1] = ""; qn = 1
      for (qi = 1; qi <= qn && qi <= 16; qi++) scan(Q[qi], QF[qi])
      # Past the re-scan cap the command was NOT read to the end: no SCANNED, so it fails closed.
      if (qn > 16) print "CAPPED"; else print "SCANNED"
    }
  '
}
# The scan of the COMMAND ITSELF ($own: the command before step 0 adds any script text). What it
# cannot read (OPAQUE, CDX, UNCLOSED, no SCANNED) fails closed, and only it: see step 0.
scan_cmd() { scan=$(printf '%s\n' "$own" | scan_awk); scanned=yes; }
# A fact of the command, or of a script file step 0 read ($sfacts: targets and cds only).
sfacts=""
facts() { printf '%s\n%s\n' "$scan" "$sfacts" | sed -n "s/^$1$TAB//p"; }
own_facts() { printf '%s\n' "$scan" | sed -n "s/^$1$TAB//p"; }
script_facts() { printf '%s\n' "$sfacts" | sed -n "s/^$1$TAB//p"; }

# WHICH REPOSITORY. "Ours" is the git COMMON dir, so the main checkout and every worktree of it are
# one repository: a subagent's absolute paths point into its worktree while this hook may run from
# the main checkout, and asking `git ls-files` in the wrong tree answers "untracked".
# Normalised with `pwd -P`, because macOS reaches `/var` through a symlink.
common_of() {
  _c=$(git -C "$1" rev-parse --path-format=absolute --git-common-dir 2>/dev/null) || return 1
  (cd "$_c" 2>/dev/null && pwd -P)
}
ours=""; root=""
find_repo() {
  for d in "$pcwd" "$PWD"; do
    [ -n "$d" ] && [ -d "$d" ] || continue
    ours=$(common_of "$d") && [ -n "$ours" ] && root=$(git -C "$d" rev-parse --show-toplevel) && break
  done
}

# THE BASES a relative path can be relative to: the directories given (the Bash tool's cwd from the
# payload, this hook's own cwd, the repo root once known) and every literal cd/pushd in the command,
# each resolved against every base before it, so `cd packages && cd db` composes.
# A UNION, not a simulation of which cd is in force where: `(cd /tmp) && sed -i … src/x.ts` must
# not resolve against /tmp alone, and a union can only over-block.
#
# set_bases CDS DIR...: sets $bases. Fails as soon as they pass 32, before n cds can make 2^n.
# cd_targets CDS: each cd resolved against $bases ALONE, not against each other (linear in n).
# Both collapse `x/..` the way `cd` (logical, the default) does, so `cd a && cd ..` adds nothing.
collapse() {
  awk '{
    n = split($0, p, "/"); k = 0
    for (i = 1; i <= n; i++) {
      if (p[i] == "." || (p[i] == "" && i > 1)) continue
      if (p[i] == "..") {
        if (k > 1 || (k == 1 && o[1] != "" && o[1] != "..")) { if (o[k] != "..") { k--; continue } }
        else if (k == 1 && o[1] == "") continue
      }
      o[++k] = p[i]
    }
    s = o[1]; for (i = 2; i <= k; i++) s = s "/" o[i]
    print (s == "" ? "/" : s)
  }'
}
cd_targets() {
  while IFS= read -r t; do
    case "$t" in "~") t=$HOME ;; "~/"*) t=$HOME/${t#"~/"} ;; esac
    case "$t" in
      /*) printf '%s\n' "$t" ;;
      *) printf '%s\n' "$bases" | while IFS= read -r b; do printf '%s/%s\n' "$b" "$t"; done ;;
    esac
  done <<EOF
$1
EOF
}
set_bases() {
  cds=$1; shift
  bases=$(printf '%s\n' "$@" | grep -v '^$' | collapse | sort -u)
  [ -n "$cds" ] || return 0
  while IFS= read -r t; do
    bases=$(printf '%s\n%s\n' "$bases" "$(cd_targets "$t")" | grep -v '^$' | collapse | sort -u)
    [ "$(printf '%s\n' "$bases" | wc -l | tr -d ' ')" -le 32 ] || return 1
  done <<EOF
$cds
EOF
}

# Every candidate as an absolute path, against every base. `~` and `~/…` are the shell's, so they
# are expanded the way the shell will; `~user` never reaches here as a literal operand.
absolutes() {
  { printf '%s\n@@\n' "$bases"; cat; } | awk -v home="$HOME" '
    $0 == "@@" { second = 1; next }
    !second { if ($0 != "") base[++nb] = $0; next }
    $0 == "" { next }
    { c = $0 }
    c == "~" { c = home }
    substr(c, 1, 2) == "~/" { c = home substr(c, 2) }
    substr(c, 1, 1) == "/" { print c; next }
    { for (b = 1; b <= nb; b++) print base[b] "/" c }
  '
}
show() { case "$1" in "$root"/*) printf '%s' "${1#"$root"/}" ;; *) printf '%s' "$1" ;; esac; }

# ── 0. A SCRIPT FILE IS CODE TOO (#88) ────────────────────────────────────────────────────────
# `python3 rewrite.py` carries no rewrite text, so every step below saw nothing to refuse. Each
# script file the scan names (SCRIPT, MODULE) that is in scope (the header: untracked, unignored,
# in this repository or a temp dir, up to 64 KiB, not a dependency) has its text appended to the
# command after the same passes inline code went through, and steps 1 and 2 judge it as inline.
# Its relative paths resolve against the same bases, which is where the interpreter resolves them.
#
# WHAT A SCRIPT'S TEXT CAN DECIDE IS NARROWER than what the command's can. From the command, a
# target the hook cannot resolve (a variable, a cd to one, a quote it cannot close) is refused;
# from a script that rule would refuse every scratch script that edits "$tmp/x" or "$1", tracked
# source or not. So a script contributes only targets that RESOLVE to tracked files: the literal
# and glob operands of its in-place editors (LIT, GLOB, with its literal cds), and the paths it
# names (the loose extraction in step 3) in two cases:
#  - it does a `.replace(` or `re.sub(`, as inline code does;
#  - it is NOT a shell script, matches the in-place pattern, has an exec call (`system(`,
#    `subprocess`, `popen`, `exec`, `spawn`, `qx`), and its own scan sees no in-place editor: the
#    editor runs from inside a string (`os.system("sed -i … f")`). A shell script needs no such
#    case: the scan sees every command it runs itself (not what it hands an interpreter in a
#    heredoc or a -c string, a known limit), and a comment or a grep pattern that merely names
#    `sed -i` is not a run. The shared price: a non-shell script that execs anything, names
#    `sed -i` anywhere and names a tracked path is refused, even if it only reads that path.
# Its OPAQUE, CDX and UNCLOSED never leave this step.
own=$cmd
scanned=no
scripts=""
case "$cmd" in
  *python*|*pypy*|*node*|*tsx*|*perl*|*ruby*|*sh*|*source*|*". "*|*.py*|*.js*|*.mjs*|*.cjs*|*.pl*|*.rb*)
    scan_cmd
    # One "KIND<TAB>word" per script: SCRIPT, or SHSCRIPT for a file a shell runs. `python3 -m a.b`
    # runs a/b.py, or the package a/b, from the cwd before the standard library.
    scripts=$(printf '%s\n' "$scan" | grep -E "^(SH)?SCRIPT$TAB"
      facts MODULE | tr . / | awk -v t="$TAB" 'NF { print "SCRIPT" t $0 ".py"; print "SCRIPT" t $0 "/__main__.py" }')
    ;;
esac
read_scripts=""
codetext=""
# Step 2's exemption for standard-stream writes (OPS-80): cleared below by a shell script, or a
# backtick launch in a non-JS script, and in step 2 by the rest of its conditions.
stream_ok=yes
if [ -n "$scripts" ]; then
  find_repo
  set_bases "$(facts CD)" "$pcwd" "$PWD" || :
  tmpd=${TMPDIR:-/tmp}; tmpd=${tmpd%/}
  while IFS="$TAB" read -r kind f; do
    [ -f "$f" ] && [ -r "$f" ] || continue
    case "$f" in */node_modules/*|*/site-packages/*) continue ;; esac
    sd=${f%/*}; sd=${sd:-/}
    case "$f" in
      /tmp/*|/private/tmp/*|/var/folders/*|/private/var/folders/*|"$tmpd"/*) ;;
      *) [ -n "$ours" ] && [ "$(common_of "$sd")" = "$ours" ] || continue ;;
    esac
    GIT_LITERAL_PATHSPECS=1 git -C "$sd" ls-files --error-unmatch -- "${f##*/}" >/dev/null 2>&1 && continue
    # Ignored is build output or a dependency (dist/cli.js): a program, not a throwaway script.
    git -C "$sd" check-ignore -q -- "${f##*/}" 2>/dev/null && continue
    [ "$(wc -c < "$f" | tr -d ' ')" -le 65536 ] || continue
    body=$(strip_heredocs < "$f" | strip_triple)
    binp=no
    printf '%s' "$body" | grep -Eq "$inplace_re" && { inplace=yes; binp=yes; }
    body=$(printf '%s\n' "$body" | strip_messages)
    [ "$kind" = SHSCRIPT ] && stream_ok=no
    sed -n 1p "$f" | grep -Eq '^#!.*[/[:space:]](sh|bash|zsh|dash|ksh)([[:space:]]|$)' && stream_ok=no
    case "$f" in *.js|*.mjs|*.cjs|*.ts) ;; *) case "$body" in *'`'*) stream_ok=no ;; esac ;; esac
    cmd=$(printf '%s\n%s\n' "$cmd" "$body")
    bfacts=$(printf '%s\n' "$body" | scan_awk | grep -E "^(INPLACE\$|(LIT|GLOB|CD)$TAB)")
    sfacts=$(printf '%s\n%s\n' "$sfacts" "$bfacts")
    # The two cases above in which the paths it names decide.
    case "$body" in *".replace("*|*"re.sub("*) binp=code ;; esac
    if [ "$binp" = yes ]; then
      sed -n 1p "$f" | grep -Eq '^#!.*[/[:space:]](sh|bash|zsh|dash|ksh)([[:space:]]|$)' && kind=SHSCRIPT
      [ "$kind" != SHSCRIPT ] \
        && printf '%s\n' "$body" | grep -Eq 'system[[:space:]]*[(]|subprocess|[Pp]open|exec|spawn|qx' \
        && ! printf '%s\n' "$bfacts" | grep -qx INPLACE && binp=code
    fi
    [ "$binp" = code ] && codetext=$(printf '%s\n%s\n' "$codetext" "$body")
    read_scripts="$read_scripts $(show "$f")"
  done <<EOF
$(printf '%s\n' "$scripts" | while IFS="$TAB" read -r k w; do
    [ -n "$w" ] && printf '%s\n' "$w" | absolutes | sed "s/^/$k$TAB/"
  done | sort -u)
EOF
fi

case "$cmd" in
  *sed*|*perl*|*ruby*)
    [ "$scanned" = yes ] || scan_cmd
    printf '%s\n%s\n' "$scan" "$sfacts" | grep -qx INPLACE && inplace=yes
    ;;
esac
# THE SCANNER ITSELF MUST NOT FAIL OPEN. Its first draft named an awk parameter `close` — a builtin —
# so awk refused the program, `scan` came back empty, and every row above that depends on it passed
# as "nothing to see". `SCANNED` is printed last; its absence means the scan did not run to the end,
# and step 3 treats an in-place edit it could not read as one it cannot prove safe.

# ── 1. Does it REWRITE by pattern? ────────────────────────────────────────────────────────────
# The hazard is a targeted mutation whose anchor may not be unique. Creating or overwriting a
# whole file is not it — that is what heredocs and the Write tool are for.
case "$cmd" in
  *".replace("*|*"re.sub("*) ;;
  # The LITERAL spellings are kept as a FLOOR. The regex is a union with them, never a
  # replacement, so widening the match can never refuse less than before.
  *"sed -i"*|*"perl -pi"*|*"perl -i"*|*"ruby -pi"*|*"ruby -i"*) ;;
  *) [ "$inplace" = yes ] || exit 0 ;;
esac

# ── 2. Does it WRITE? ─────────────────────────────────────────────────────────────────────────
# A read-only preview — `print(open(f).read().replace(a, b))` — is how you SHOULD inspect a
# candidate edit, and must not be blocked. The in-place flag is itself the write for sed/perl/ruby.
# `writeFile` is node's (writeFileSync, fs.promises.writeFile): `node fix.js` brought it in (#88).
#
# A WRITE TO A STANDARD STREAM IS NOT ONE (OPS-80). `process.stdout.write(` is how node PRINTS, so
# a report script that .replace()s in-memory strings and names a tracked path in a comment was
# refused as a rewrite while writing no file at all. Such writes are removed before the test, the
# way `print(` was never in it, so any OTHER write in the same text (writeFileSync, a stream's
# `.write(`, a handle's) still counts.
#
# ONLY THE SHAPE THAT CAN BE PROVEN SAFE IS EXEMPT, and everything else is judged as before OPS-80
# (*A check that reads source is a parser*). Listing where output can go (tee, sponge, mv, …) is a
# sample of the sinks, and two reviews found sinks past each list. So the stream writes are removed
# only on a DIRECT run whose standard streams reach the terminal and nothing else:
#  1. the command has no `|` at all, and no `>` except to /dev/null or another fd (a `>` inside
#     inline code, an arrow function, counts too: it can only refuse more);
#  2. no script it runs is a shell script (a wrapper's own `>` or pipe is the same hazard);
#  3. no text launches a process or a shell (child_process, exec, spawn, subprocess, os.system,
#     popen, qx, a backtick outside JS), whose command string could redirect;
#  4. no text REBINDS a stream (`sys.stdout = open(…)`, `setattr(sys, 'stdout', …)`,
#     `redirect_stdout(…) as sys.stdout`, `process.stdout.write = …`, `defineProperty(process,
#     'stdout', …)`).
# `writeSync` is node's descriptor write (fs.writeSync(fs.openSync(f, 'w'), …)).
wcmd=$cmd
printf '%s\n' "$cmd" | grep -Eq 'child_process|exec|spawn|subprocess|system[[:space:]]*[(]|[Pp]open|qx' && stream_ok=no
printf '%s\n' "$cmd" | grep -Eq '(stdout|stderr)([.]write)?[[:space:]]*=[^=]|redirect_std|setattr[(][^)]*std(out|err)|as[[:space:]]+sys[.]std(out|err)|defineProperty[(][^)]*std(out|err)' && stream_ok=no
case "$own" in *"|"*) stream_ok=no ;; esac
printf '%s\n' "$own" \
  | sed -E 's/[0-9]*>>?[[:space:]]*\/dev\/null//g; s/&>>?[[:space:]]*\/dev\/null//g; s/[0-9]*>&[0-9-]+//g' \
  | grep -q '>' && stream_ok=no
[ "$stream_ok" = yes ] \
  && wcmd=$(printf '%s\n' "$cmd" | sed -E 's/(process|sys)[.](stdout|stderr)([.]buffer)?[.]write(lines)?[(]//g')
case "$wcmd" in
  *"write_text("*|*".write("*|*"writelines("*|*"writeFile"*|*"writeSync"*) ;;
  *"sed -i"*|*"perl -pi"*|*"perl -i"*|*"ruby -pi"*|*"ruby -i"*) ;;
  *) [ "$inplace" = yes ] || exit 0 ;;
esac

# ── 3. Which of the files it will write are TRACKED? ─────────────────────────────────────────
[ "$scanned" = yes ] || scan_cmd
[ -n "$ours" ] || find_repo
# Not in a repository at all: there is no tracked source to protect. Fails open, as before.
[ -n "$ours" ] || exit 0

block_opaque() {
  echo "BLOCKED by no-blind-source-rewrite: an in-place edit (sed -i / perl -pi / ruby -i) whose files" >&2
  echo "cannot be proven untracked — $1." >&2
  echo "" >&2
  echo "The hook cannot see which files this will reach, so it refuses rather than guess." >&2
  echo "For TRACKED source, use the Edit tool: it requires old_string to be UNIQUE and fails loudly." >&2
  echo "For scratch files, name each one literally — no find, xargs, variable, command substitution" >&2
  echo "or brace list in front of the in-place editor. A literal cd and a glob are resolved, not refused." >&2
  exit 2
}

# FAIL CLOSED: the scan did not finish (see SCANNED above), or the target set is decided at run
# time. The first reason is enough.
case "$cmd" in
  *"sed -i"*|*"perl -pi"*|*"perl -i"*|*"ruby -pi"*|*"ruby -i"*) inplace=yes ;;
esac
if [ "$inplace" = yes ]; then
  printf '%s\n' "$scan" | grep -qx SCANNED \
    || block_opaque "the hook could not read this command to the end (scanner failed, or too many nested shells)"
  # A quote the scanner could not close means everything after it was read as ONE word, so a cd
  # or an operand there is invisible. The shell would refuse it too; the hook must not guess.
  printf '%s\n' "$scan" | grep -qx UNCLOSED \
    && block_opaque "a quote in this command never closes, so what follows it cannot be read"
fi
opaque=$(facts OPAQUE | sed -n 1p)
[ -n "$opaque" ] && block_opaque "$opaque"
if printf '%s\n' "$scan" | grep -qx CDX && printf '%s\n' "$scan" | grep -qx INPLACE; then
  block_opaque "a cd whose target is not a literal path moves every relative operand"
fi

# The repo root is a base too: the original only base, kept so nothing narrows.
# The cap refuses only on the command's own cds: refusing for a script's cds would refuse scripts
# that never touch tracked source (step 0). A script's cds compose with the command's while they
# fit; past the cap each one is resolved against the command's bases ALONE, a linear set that
# still holds `cd packages/a` for a codemod's `sed -i … src/index.ts`, though not a cd composed
# from two of the script's own.
set_bases "$(own_facts CD)" "$pcwd" "$PWD" "$root" || block_opaque "too many cd targets to resolve"
if [ -n "$(script_facts CD)" ]; then
  own_bases=$bases
  set_bases "$(facts CD)" "$pcwd" "$PWD" "$root" || {
    bases=$own_bases
    bases=$(printf '%s\n%s\n' "$own_bases" "$(cd_targets "$(script_facts CD)")" | grep -v '^$' | collapse | sort -u)
  }
fi

# Docs and markdown are excluded on purpose: prose has no unique-anchor hazard worth a block, and
# the first false positive was exactly a docs edit.
is_doc() { case "$1" in *.md|*.mdx|*.txt|*.html) return 0 ;; esac; return 1; }

# Nearest existing directory at or above absolute path $1. Parameter expansion, not `dirname`:
# this runs once per candidate, and a fork per step is what made a long PR body cost 41 s.
up() { _n=${1%/*}; [ "$_n" = "$1" ] && _n=/; printf '%s' "${_n:-/}"; }
existing_dir() {
  _d=$1
  while [ ! -d "$_d" ]; do _n=${_d%/*}; [ "$_n" = "$_d" ] && _n=/; _d=${_n:-/}; done
  printf '%s' "$_d"
}

# Which of the absolute paths on stdin are FILES tracked by OUR repository? Prints each one.
#
# BATCHED — one `git rev-parse` per distinct directory and one `git ls-files` per worktree, not a
# git call per candidate per base. A call per candidate made a PR body naming 800 files take
# 41 s while every Bash call waited on it.
#
# Git is still asked from the file's OWN worktree (`--show-toplevel` of its nearest existing
# directory), so a path into any worktree of this repository is judged there. Literal pathspecs:
# an operand is a file name, and `:/` must not mean "the whole repo". And only an EXACT match
# counts — a pathspec naming a tracked DIRECTORY (`perl -I packages …`) lists the files under it,
# and none of those is the operand.
tracked_files() {
  _pairs=$(while IFS= read -r _p; do
    [ -n "$_p" ] || continue
    _d=${_p%/*}; [ "$_d" = "$_p" ] && _d=/; _d=${_d:-/}
    while [ ! -d "$_d" ]; do _n=${_d%/*}; [ "$_n" = "$_d" ] && _n=/; _d=${_n:-/}; done
    _r=${_p#"$_d"}; _r=${_r#/}
    [ -n "$_r" ] && printf '%s\t%s\n' "$_d" "$_r"
  done)
  [ -n "$_pairs" ] || return 0
  _info=$(printf '%s\n' "$_pairs" | cut -f1 | sort -u | while IFS= read -r _d; do
    _o=$(git -C "$_d" rev-parse --path-format=absolute --git-common-dir --show-toplevel --show-prefix 2>/dev/null) || continue
    { IFS= read -r _c; IFS= read -r _t; IFS= read -r _x; } <<EOF
$_o
EOF
    [ "$(cd "$_c" 2>/dev/null && pwd -P)" = "$ours" ] || continue
    printf '%s\t%s\t%s\n' "$_d" "$_t" "$_x"
  done)
  [ -n "$_info" ] || return 0
  # "top<TAB>path relative to top" for every candidate inside our repository.
  _rels=$(printf '%s\n@@\n%s\n' "$_info" "$_pairs" | awk -F '\t' '
    $0 == "@@" { second = 1; next }
    !second { top[$1] = $2; pre[$1] = $3; next }
    ($1 in top) { print top[$1] "\t" pre[$1] $2 }
  ')
  printf '%s\n' "$_rels" | cut -f1 | grep -v '^$' | sort -u | while IFS= read -r _t; do
    _want=$(printf '%s\n' "$_rels" | awk -F '\t' -v t="$_t" '$1 == t && $2 != "" { print $2 }' | sort -u)
    [ -n "$_want" ] || continue
    _got=$(printf '%s\n' "$_want" | tr '\n' '\0' \
      | GIT_LITERAL_PATHSPECS=1 xargs -0 git -C "$_t" ls-files -z -- 2>/dev/null | tr '\0' '\n')
    printf '%s\n@@\n%s\n' "$_want" "$_got" | awk -v t="$_t" '
      $0 == "@@" { second = 1; next }
      !second { want[$0] = 1; next }
      ($0 in want) { print t "/" $0 }
    '
  done
}

# Does glob $1 (absolute) match a tracked non-doc file? `git ls-files` IS the expansion — the
# authority, not the working tree, so a glob matching only scratch files passes. `:(glob)` gives it
# shell semantics (`*` stops at `/`).
tracked_glob() {
  _pre=$(printf '%s' "$1" | sed 's/[][*?].*//')
  case "$_pre" in */) _pre=${_pre%/} ;; *) _pre=$(up "$_pre") ;; esac
  _d=$(existing_dir "${_pre:-/}")
  [ "$(common_of "$_d")" = "$ours" ] || return 1
  _rel=${1#"$_d"}; _rel=${_rel#/}
  git -C "$_d" ls-files -- ":(glob)$_rel" 2>/dev/null \
    | while IFS= read -r m; do is_doc "$m" || { printf '%s\n' "$m"; break; }; done \
    | grep -q .
}

# CANDIDATES, two sources, as a UNION so the scan can never refuse less than the loose match:
#  - the LITERAL operands of an in-place editor, from the scan (no extension required, so a
#    tracked `Makefile` counts); and
#  - a loose extraction (any token that looks like a path with an extension), which is what
#    sees a path inside `python3 -c "…"` or a `python3 <<PY` body, where no shell word is a target.
#    Over the command itself, and over only those script files that `.replace(` or `re.sub(`, or
#    run an editor from inside a string (step 0): a shell script's `cp src/x.ts "$tmp"` names a
#    path it never edits.
# Loose extraction is safe precisely BECAUSE git is the filter: a token that is not tracked is
# discarded, so over-collecting costs nothing and under-describing costs everything.
#
# The ARGUMENTS to `.replace(` / `re.sub(` are stripped first. A path inside a replacement STRING
# is prose, not a target: a docs-only edit was blocked for exactly that, and asking git instead of
# matching globs brought the false positive straight back, because git happily confirms a path
# quoted inside a sentence is tracked. Authority answers "is this source"; position answers "is
# this being written", and both questions have to be asked.
loose=$(printf '%s\n%s' "$own" "$codetext" \
  | tr -d '\\' \
  | sed -E 's/\.replace\([^)]*\)//g; s/re\.sub\([^)]*\)//g' \
  | tr -c 'A-Za-z0-9_./-' '\n' \
  | grep -E '\.[A-Za-z]+$' \
  | sed -e 's|^\./||')
candidates=$(printf '%s\n%s\n' "$(facts LIT)" "$loose" | grep -v '^$' | sort -u)

# `while read` over a heredoc, never `for x in $(…)`: an unquoted expansion would GLOB the glob
# candidates in this hook's own directory, and split a path containing a space.
tracked=""
if [ -n "$candidates" ]; then
  while IFS= read -r p; do
    [ -n "$p" ] && tracked="$tracked $(show "$p")"
  done <<EOF
$(printf '%s\n' "$candidates" | absolutes | grep -Ev '\.(md|mdx|txt|html)$' | sort -u | tracked_files)
EOF
fi
globs=$(facts GLOB)
if [ -n "$globs" ]; then
  gabs=$(printf '%s\n' "$globs" | absolutes | sort -u)
  while IFS= read -r g; do
    [ -n "$g" ] || continue
    tracked_glob "$g" && tracked="$tracked $(show "$g")"
  done <<EOF
$gabs
EOF
  # AND the shell's own expansion, judged file by file in each file's own worktree. `git ls-files`
  # run where the glob STARTS cannot see into a worktree nested below it (`.claude/worktrees/*/…`
  # from the main checkout), and the shell will expand into it happily.
  expanded=$(printf '%s\n' "$gabs" | while IFS= read -r g; do
    [ -n "$g" ] || continue
    # shellcheck disable=SC2086 # expansion IS the point; IFS is newline so spaces survive
    ( IFS='
'; for m in $g; do [ -f "$m" ] && printf '%s\n' "$m"; done )
  done | grep -Ev '\.(md|mdx|txt|html)$' | sort -u)
  if [ -n "$expanded" ]; then
    while IFS= read -r p; do
      [ -n "$p" ] && tracked="$tracked $(show "$p")"
    done <<EOF
$(printf '%s\n' "$expanded" | tracked_files)
EOF
  fi
fi
[ -z "$tracked" ] && exit 0

echo "BLOCKED by no-blind-source-rewrite: this rewrites TRACKED source in place with a scripted find-replace:$tracked" >&2
[ -z "$read_scripts" ] || echo "(The rewrite was read from the script file(s) this command runs:$read_scripts.)" >&2
echo "" >&2
echo "Use the Edit tool. It requires old_string to be UNIQUE and fails loudly when it is not —" >&2
echo "the property this command lacks. Python's .replace() is all-occurrences and silent: it has" >&2
echo "edited two functions while only one was being looked at." >&2
echo "" >&2
echo "If replacing every occurrence really is the intent, say so: Edit with replace_all: true." >&2
echo "To PREVIEW an edit, print it instead of writing it — that is not blocked." >&2
exit 2
