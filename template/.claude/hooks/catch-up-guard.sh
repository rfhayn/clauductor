#!/bin/sh
# Refuses a Bash `git pull`, `merge`, `rebase` or `reset` aimed at the main checkout while the panel
# catches that checkout up (`clauductor panel catch-up --project <main>` exits 0 and prints `on`).
# PreToolUse (Bash).
#
# WHY (OPS-138, #249 item 2). merge-pr step 5 leaves the main checkout to the panel with catch-up on:
# the panel trusts a merged panel.json, and rebuilds itself, only after a fast-forward it made, so an
# agent's pull takes that from it. checks/change-process.sh holds merge-pr's text to that (nothing
# outside step 5 names a pull, a cd or a -C to the main checkout), but a text check reads only the
# spellings it lists: `x=c; y=d; eval "$x$y" "$main"` and zsh's `chdir` got past it. And Bash keeps
# its cwd between calls, so an agent an earlier command left in the main checkout pulls it with a
# plain `git pull` that no text names. This hook reads the command as it is about to run.
#
# WHAT IS A MOVE. A git invocation (`git`, or a path ending /git, after env, sudo, an assignment, a
# redirection or a shell keyword) whose subcommand, after git's own options, is pull, merge, rebase
# or reset; one the shell names only when it runs (`git "$sub"`, `git $(…)`); or an alias whose value
# starts with one of those or with `!` (a shell alias), from the checkout's config or a `-c alias.X=`
# on the line. `eval`, and a bare variable as the command (`$g pull`), count as a move too. Every
# quoted string is read again as a command of its own, so `sh -c '…'` and `eval "…"` are seen.
#
# WHERE IT IS AIMED. The checkout -C names (each -C read from the one before, as git does), else the
# payload's cwd. It is UNKNOWN, and taken as the cwd's main checkout, when anything in the command can
# move where git runs: cd, pushd, popd, chdir, eval, source or `.`, env -C, GIT_DIR= or
# GIT_WORK_TREE=, --git-dir, --work-tree or --bare, a -C that is not a literal path, or a variable as
# the command. The main checkout is the first entry of `git worktree list --porcelain` (merge-pr step
# 5's own reading), so a lane's own catch-up merge (merge-pr step 2) is aimed at its worktree and runs.
#
# WHAT IS ASKED. Only clauductor knows whether catch-up is on. Without clauductor on PATH (the repo
# works without it: ADR-0006), or with any answer but `on` and exit 0, catch-up is off: the command
# runs, and the hook says why on stderr. Without jq the payload is read raw: a `git` word and a pull,
# merge, rebase or reset word, refused while catch-up is on for the cwd's main checkout.
#
# KNOWN LIMITS.
#   - Not followed: an alias of an alias, a config file the command names, a shell function or alias
#     the session defined, a script file or `printf … | sh`.
#   - Read too widely, each refused only with catch-up on, and rewritten in one step: a quoted string
#     or here-document line that reads as a pull (`git commit -m 'x; git pull'`), a cd anywhere in the
#     command (it makes every git in it unknown), a -C "$(…)" the shell computes.
#   - A command whose cwd is in no git repository, re-aimed by cd, has no main checkout to ask about.
#
# PROTOCOL: exit 0 allows; exit 2 blocks, with the reason on stderr.

TAB=$(printf '\t')
in=$(cat)

catch_up() {  # catch_up MAIN: 0 when the panel catches MAIN up, 2 with no clauductor to ask, else 1
  command -v clauductor >/dev/null 2>&1 || return 2
  _cu=$(clauductor panel catch-up --project "$1" 2>/dev/null) && [ "$_cu" = on ]
}
main_of() {  # main_of DIR: the main checkout of DIR's repository (merge-pr step 5), or exit 1
  git -C "$1" worktree list --porcelain 2>/dev/null | sed -n '1s/^worktree //p' | grep .
}
real() { (cd "$1" 2>/dev/null && pwd -P); }
judge() {  # judge MAIN COMMAND: refuse COMMAND (exit 2) when the panel catches MAIN up
  catch_up "$1"; _rc=$?
  if [ "$_rc" = 0 ]; then
    cat >&2 <<EOF
BLOCKED by catch-up-guard: the panel catches up $1 (catch-up is on), so no agent pulls, merges,
rebases or resets it: the panel trusts a merged panel.json, and rebuilds itself, only after a
fast-forward it made (merge-pr step 5). To confirm a merge, run \`git fetch origin main\`, then
\`git log -1 --oneline origin/main\`. A lane's own work runs in its worktree, with no cd, -C or other
way of aiming git at the main checkout.
Command: $2
EOF
    exit 2
  elif [ "$_rc" = 2 ]; then
    echo "catch-up-guard: no clauductor on PATH, so catch-up counts as off for $1: allowed (merge-pr step 5)" >&2
  else
    echo "catch-up-guard: catch-up is off for $1: allowed (merge-pr step 5 pulls it from the main checkout itself)" >&2
  fi
}

if ! command -v jq >/dev/null 2>&1; then
  printf '%s' "$in" | grep -Eq '"tool_name" *: *"Bash"' || exit 0
  printf '%s' "$in" | grep -Eq '(^|[^A-Za-z0-9_-])git([^A-Za-z0-9_-]|$)' || exit 0
  printf '%s' "$in" | grep -Eq '(^|[^A-Za-z0-9_-])(pull|merge|rebase|reset)([^A-Za-z0-9_-]|$)' || exit 0
  m=$(main_of "$(pwd)") || exit 0
  judge "$m" "(read without jq) $in"
  exit 0
fi

[ "$(printf '%s' "$in" | jq -r '.tool_name // empty' 2>/dev/null)" = Bash ] || exit 0
cmd=$(printf '%s' "$in" | jq -r '.tool_input.command // empty' 2>/dev/null)
cwd=$(printf '%s' "$in" | jq -r '.cwd // empty' 2>/dev/null)
[ -n "$cwd" ] && [ -d "$cwd" ] || cwd=$(pwd)
case $cmd in *git* | *eval* | *'$'*) ;; *) exit 0 ;; esac

# The reading: "R<TAB>1" when something re-aims git, then "G<TAB>dir<TAB>subcommand<TAB>alias" for
# each git invocation, dir "?" when -C is not a literal path and alias the value a -c alias.X= gives.
reads=$(printf '%s\n' "$cmd" | awk -v cwd="$cwd" '
  { src = src (NR > 1 ? "\n" : "") $0 }
  END {
    queue[nq = 1] = src
    for (qi = 1; qi <= nq; qi++) parse(queue[qi])
    print "R\t" reaim
    for (i = 1; i <= no; i++) print out[i]
  }
  # A quoted string is also a command of its own: sh -c, eval and bash -c pass their script that way.
  function enqueue(s) { if (nq < 64 && s ~ /[^ \t\n]/) queue[++nq] = s }
  function endword() { if (inw) words[++nw] = w; w = ""; inw = 0 }
  function endseg() { endword(); if (nw) segment(); nw = 0 }
  function parse(s,   i, n, ch, q, qs, bt, j) {
    nw = 0; w = ""; inw = 0; q = ""; bt = 0; n = length(s)
    for (i = 1; i <= n; i++) {
      ch = substr(s, i, 1)
      if (q == "\047") { if (ch == "\047") { q = ""; enqueue(qs) } else { w = w ch; qs = qs ch }; continue }
      if (q == "\"") {
        if (ch == "\\" && i < n) { i++; ch = substr(s, i, 1); w = w ch; qs = qs ch; continue }
        if (ch == "\"") { q = ""; enqueue(qs) } else { w = w ch; qs = qs ch }
        continue
      }
      if (ch == "\\") { if (i < n) { i++; ch = substr(s, i, 1); if (ch != "\n") { w = w ch; inw = 1 } }; continue }
      if (ch == "\047" || ch == "\"") { q = ch; qs = ""; inw = 1; continue }
      if (ch == "#" && !inw) { while (i < n && substr(s, i + 1, 1) != "\n") i++; continue }
      if (ch == "$" && substr(s, i + 1, 1) == "{") {
        j = index(substr(s, i), "}"); if (j == 0) j = n - i + 1
        w = w substr(s, i, j); inw = 1; i += j - 1; continue
      }
      # A substitution: the word it stands in is not known until it runs, and its body is a command.
      if (ch == "$" && substr(s, i + 1, 1) == "(") { w = w "$"; inw = 1; endseg(); i++; continue }
      if (ch == "`") { if (!bt) { w = w "$"; inw = 1 }; bt = !bt; endseg(); continue }
      if (ch ~ /[;&|(){}\n]/) { endseg(); continue }
      if (ch == " " || ch == "\t") { endword(); continue }
      w = w ch; inw = 1
    }
    endseg()
  }
  function cdir(dir, x) {
    if (x == "" || x ~ /[$`~*?]/) { lit = 0; return dir }
    return x ~ /^\// ? x : dir "/" x
  }
  function segment(   i, cw, dir, sc, ca, x, k, v, c) {
    for (i = 1; i <= nw; i++) if (words[i] ~ /^(GIT_DIR|GIT_WORK_TREE)=/) reaim = 1
    for (i = 1; i <= nw; ) {
      cw = words[i]
      if (cw ~ /^[0-9]*[<>]+&?$/) { i += 2; continue }
      if (cw ~ /^[A-Za-z_][A-Za-z0-9_]*=/ || cw ~ /^[0-9]*[<>]/ || cw ~ /^(then|do|else|elif|if|while|until|!|time|nohup|command|builtin|exec|nice|xargs|export|local|readonly)$/) { i++; continue }
      if (cw == "sudo") { for (i++; i <= nw && words[i] ~ /^-/; i++) ; continue }
      if (cw == "env") {
        for (i++; i <= nw && (words[i] ~ /^-/ || words[i] ~ /^[A-Za-z_][A-Za-z0-9_]*=/); i++) {
          if (words[i] ~ /^(-C|--chdir)/) reaim = 1
          if (words[i] ~ /^(-C|--chdir|-u|--unset|-S|--split-string)$/) i++
        }
        continue
      }
      break
    }
    if (i > nw) return
    cw = words[i]
    if (cw ~ /^(cd|pushd|popd|chdir|source|\.)$/) { reaim = 1; return }
    if (cw == "eval" || (cw ~ /^\$\{?[A-Za-z_][A-Za-z0-9_]*\}?$/ && i < nw)) { reaim = 1; out[++no] = "G\t?\t$\t"; return }
    if (cw != "git" && cw !~ /\/git$/) return
    dir = cwd; lit = 1; ca = ""
    for (i++; i <= nw; i++) {
      x = words[i]
      if (x == "-C") { dir = cdir(dir, words[++i]); continue }
      if (x ~ /^-C./) { dir = cdir(dir, substr(x, 3)); continue }
      if (x == "-c") { ca = ca "\n" words[++i]; continue }
      if (x ~ /^-c./) { ca = ca "\n" substr(x, 3); continue }
      if (x ~ /^--(git-dir|work-tree)$/) { reaim = 1; i++; continue }
      if (x ~ /^--(git-dir|work-tree)=/ || x == "--bare") { reaim = 1; continue }
      if (x ~ /^--(namespace|exec-path|config-env|super-prefix|attr-source)$/) { i++; continue }
      if (x ~ /^-/) continue
      break
    }
    if (i > nw) return
    sc = words[i]; v = ""
    k = split(ca, c, "\n")
    for (; k >= 1; k--) if (index(c[k], "alias." sc "=") == 1) { v = substr(c[k], length("alias." sc "=") + 1); break }
    out[++no] = "G\t" (lit ? dir : "?") "\t" sc "\t" v
  }
')
reaim=$(printf '%s\n' "$reads" | head -n 1 | cut -f2)

is_move() {  # is_move DIR SUBCOMMAND ALIAS-VALUE: the subcommand, or the alias it names, moves a checkout
  case $2 in pull | merge | rebase | reset | '$'*) return 0 ;; esac
  _v=$3
  [ -n "$_v" ] || _v=$(git -C "$1" config --get "alias.$2" 2>/dev/null) || return 1
  case $_v in '!'*) return 0 ;; esac
  case $(printf '%s\n' "$_v" | awk '{ print $1; exit }') in pull | merge | rebase | reset) return 0 ;; esac
  return 1
}

# Each main checkout a move is aimed at, once.
mains=$(printf '%s\n' "$reads" | sed 1d | while IFS="$TAB" read -r _k dir sc val; do
  [ "$_k" = G ] || continue
  if [ "$reaim" = 1 ] || [ "$dir" = '?' ]; then
    is_move "$cwd" "$sc" "$val" && main_of "$cwd"
  else
    [ -d "$dir" ] && is_move "$dir" "$sc" "$val" || continue
    top=$(git -C "$dir" rev-parse --show-toplevel 2>/dev/null) || continue
    m=$(main_of "$top") || continue
    [ "$(real "$top")" = "$(real "$m")" ] && printf '%s\n' "$m"
  fi
done | awk '!seen[$0]++')

[ -n "$mains" ] || exit 0
while IFS= read -r m; do judge "$m" "$cmd"; done <<EOF
$mains
EOF
exit 0
