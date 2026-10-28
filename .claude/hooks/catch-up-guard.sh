#!/bin/sh
# Refuses a Bash git command that moves the main checkout (a pull, merge, rebase, reset, ...) while
# the panel catches that checkout up (`clauductor panel catch-up --project <main>` exits 0 and prints
# `on`). PreToolUse (Bash).
#
# WHY (OPS-138, #249 item 2). merge-pr step 5 leaves the main checkout to the panel with catch-up on:
# the panel trusts a merged panel.json, and rebuilds itself, only after a fast-forward it made, so an
# agent's pull takes that from it. checks/change-process.sh holds merge-pr's text to that (nothing
# outside step 5 names a pull, a cd or a -C to the main checkout), but a text check reads only the
# spellings it lists: `x=c; y=d; eval "$x$y" "$main"` and zsh's `chdir` got past it. And Bash keeps
# its cwd between calls, so an agent an earlier command left in the main checkout pulls it with a
# plain `git pull` that no text names. This hook reads the command as it is about to run.
#
# HOW IT READS. lib/gate-reader.awk (`-v gits=1`), the tokenizer no-background-gate, no-silent-wait
# and cd-guard share: quotes, escapes (`g\it`), wrappers and their options (`timeout 60`, `sudo -u
# x`, `nice -n 5`), `sh -c`, `eval`, `$( … )` and backticks, heredocs, and mentions (`grep "git
# pull"`, a commit message) that run nothing. It lists, in order, each cd, each git invocation and
# each command word the shell computes (gate-reader.awk's header).
#
# WHAT IS A MOVE. A git subcommand change-process.sh's text check also counts (pull merge reset
# checkout switch rebase restore stash update-ref; `stash list` and `stash show` excepted), a fetch
# with -u or --update-head-ok, a subcommand the shell computes (`git "$sub"`), or an alias whose value
# starts with one of those or with `!` (a shell alias), from the checkout's config or a `-c alias.X=`
# on the line. `gh pr checkout` and `gh repo sync` move the checkout they run in, as a git checkout
# would. A computed command word (`$g pull`, `eval "$c"`) is a move when the command's text, its
# quotes and backslashes dropped, names a move word anywhere. Refs are shared by every worktree, so
# update-ref and a fetch with -u move the main checkout's branch from a lane too: they count as
# aimed at its repository's main checkout wherever they run.
#
# WHERE IT IS AIMED. The candidates start at the payload's cwd. A literal cd, pushd or chdir this
# shell surely runs replaces each with its target: one the next command follows by `;`, a newline or
# `&&` (sure for the rest of that && chain; after it, either place). A cd this shell may not run
# adds its target instead, a superset: one under if or while, in a case arm, after a `||`, in a pipeline, sent to
# the background, or one a child runs (`sh -c`, `eval`, find -exec). A `( … )` subshell's cds end at
# its `)`. A -C composes on each candidate, as git composes it. UNKNOWN, written "?", is any target
# nobody can read from here: a computed cd or -C, `cd -`, a bare pushd, popd, source, env -C,
# GIT_DIR=, --git-dir, --work-tree, a placeholder filled in at run time (xargs -I, parallel, find
# -execdir), a cd or -C to what is not a directory yet (the command may make it: `ln -s "$M" m`), or
# a computed command word, which may itself be a cd; from then on every move counts as aimed at the
# cwd's main checkout too. The main checkout is the first entry of `git worktree list --porcelain`
# (merge-pr step 5's own reading), and a candidate is aimed there when its git dir is the main
# checkout's (so its `.git` counts, and a lane's worktree does not). So a lane's own catch-up merge
# (merge-pr step 2), in its worktree, or after a literal cd into it, runs.
#
# WHAT IS ASKED. Only clauductor knows whether catch-up is on. Without clauductor on PATH (the repo
# works without it: ADR-0006), or with any answer but `on` and exit 0, catch-up is off: the command
# runs, and the hook says why on stderr. Without jq, or with a payload jq cannot read, the payload is
# read raw, its quotes and backslashes dropped: a `git` word and a move word, refused while catch-up
# is on for the cwd's main checkout. If the reader cannot run, the command is read the same way.
#
# KNOWN LIMITS.
#   - Not followed: an alias of an alias, a config file the command names, a shell function or alias
#     the session defined, a script file (`bash x.sh`, `printf … | sh`), `ssh` (another machine),
#     a gh alias or extension, and every other tool that runs git for you.
#   - Not a move here: a new commit on the checkout (commit, am, cherry-pick, revert, branch -f).
#   - A computed command whose text names no move word, running an alias that moves (`c="git up";
#     eval "$c"`), is not refused.
#   - Not refused yet (#280): fetch's -u bundled with other short options (`-fu`); a git-core
#     program run by path (`git-pull`); a case pattern's `)` inside `$( … )`, which ends it early;
#     a lane's `checkout --ignore-other-worktrees main` and a move after it; a heredoc body's `$( … )`.
#   - Read too widely, each refused only with catch-up on: a `git` word anywhere in a simple command
#     starts a git invocation (`echo git pull` in the main checkout); a -C "$(…)" the shell computes
#     is unknown even when it names the lane (`git -C "$(git rev-parse --show-toplevel)" merge`); a
#     lane's update-ref or `fetch -u` of its own branch; a cd to a directory that does not exist
#     (`cd sub 2>/dev/null; git reset` in a lane without sub); a cd under if or while, or in a case
#     arm, even when it runs; the main checkout as a fallback (`… && git merge origin/main || git merge
#     --abort` from it, when its first command is the fetch and not the cd).
#   - A command whose cwd is in no git repository, re-aimed where it cannot be read, has no main
#     checkout to ask about.
#
# PROTOCOL: exit 0 allows; exit 2 blocks, with the reason on stderr.

TAB=$(printf '\t')
in=$(cat)
MOVES='pull merge reset checkout switch rebase restore stash update-ref'
MOVE_RE='(^|[^A-Za-z0-9_-])(pull|merge|reset|checkout|switch|rebase|restore|stash|update-ref|update-head-ok)([^A-Za-z0-9_-]|$)'
GIT_RE='(^|[^A-Za-z0-9_-])git([^A-Za-z0-9_-]|$)'

catch_up() {  # catch_up MAIN: 0 when the panel catches MAIN up, 2 with no clauductor to ask, else 1
  command -v clauductor >/dev/null 2>&1 || return 2
  _cu=$(clauductor panel catch-up --project "$1" 2>/dev/null) && [ "$_cu" = on ]
}
main_of() {  # main_of DIR: the main checkout of DIR's repository (merge-pr step 5), or exit 1
  git -C "$1" worktree list --porcelain 2>/dev/null | sed -n '1s/^worktree //p' | grep .
}
real() { (cd "$1" 2>/dev/null && pwd -P); }
plain() { printf '%s' "$1" | tr -d '\\'"'\""; }  # plain TEXT: TEXT, its quotes and backslashes dropped
judge() {  # judge MAIN COMMAND: refuse COMMAND (exit 2) when the panel catches MAIN up
  catch_up "$1"; _rc=$?
  if [ "$_rc" = 0 ]; then
    cat >&2 <<EOF
BLOCKED by catch-up-guard: the panel catches up $1 (catch-up is on), so no agent pulls, merges,
rebases, resets or otherwise moves it: the panel trusts a merged panel.json, and rebuilds itself,
only after a fast-forward it made (merge-pr step 5). To confirm a merge, run \`git fetch origin
main\`, then \`git log -1 --oneline origin/main\`. A lane's own catch-up runs in its worktree: from
there, or after a literal \`cd\` or \`-C\` to it. This command aims a move at the main checkout, or
re-aims git where this hook cannot follow (a computed path or command, eval, GIT_DIR=, cd -), which
counts as there.
Command: $2
EOF
    exit 2
  elif [ "$_rc" = 2 ]; then
    echo "catch-up-guard: no clauductor on PATH, so catch-up counts as off for $1: allowed (merge-pr step 5)" >&2
  else
    echo "catch-up-guard: catch-up is off for $1: allowed (merge-pr step 5 pulls it from the main checkout itself)" >&2
  fi
}
raw() {  # raw TEXT: the reading without jq or the reader: a git word and a move word, at the cwd
  _p=$(plain "$1")
  printf '%s' "$_p" | grep -Eq "$GIT_RE" || exit 0
  printf '%s' "$_p" | grep -Eq "$MOVE_RE" || exit 0
  _m=$(main_of "$(pwd)") || exit 0
  judge "$_m" "$2"
  exit 0
}

if ! command -v jq >/dev/null 2>&1 || ! printf '%s' "$in" | jq -e . >/dev/null 2>&1; then
  printf '%s' "$in" | grep -Eq '"tool_name" *: *"Bash"' || exit 0
  raw "$in" "(read without jq) $in"
fi

[ "$(printf '%s' "$in" | jq -r '.tool_name // empty')" = Bash ] || exit 0
cmd=$(printf '%s' "$in" | jq -r '.tool_input.command // "" | tostring')
cwd=$(printf '%s' "$in" | jq -r '.cwd // empty')
[ -n "$cwd" ] && [ -d "$cwd" ] || cwd=$(pwd)
cwd=$(real "$cwd")

# LC_ALL=C: a multibyte locale can make awk reject a command's bytes (merge-reader.awk's #100).
reads=$(printf '%s' "$cmd" | LC_ALL=C awk -v gates="" -v gits=1 -f "$(dirname "$0")/lib/gate-reader.awk" 2>/dev/null) \
  || raw "$cmd" "(read raw: the reader could not run) $cmd"
case $reads in *GIT* | *DYN*) ;; *) exit 0 ;; esac
dynmove=0; plain "$cmd" | grep -Eq "$MOVE_RE" && dynmove=1

moves_sub() {  # moves_sub SUBCOMMAND NEXT-WORD UPDATE-HEAD: the subcommand moves a checkout
  case $1 in
    *'$'*) return 0 ;;
    stash) case $2 in list | show) return 1 ;; esac; return 0 ;;
    # Refs are shared by every worktree: these move the main checkout's branch from a lane too.
    update-ref) shared=1; return 0 ;;
    fetch) [ "$3" = 1 ] && shared=1; [ "$3" = 1 ]; return ;;
  esac
  case " $MOVES " in *" $1 "*) return 0 ;; esac
  return 1
}
is_move() {  # is_move DIR SUBCOMMAND NEXT-WORD UPDATE-HEAD ALIAS-VALUE: it, or the alias it names, moves
  shared=0
  moves_sub "$2" "$3" "$4" && return 0
  _v=$5
  [ -n "$_v" ] || _v=$(git -C "$1" config --get "alias.$2" 2>/dev/null) || return 1
  case $_v in '!'*) return 0 ;; esac
  _u=0; case " $_v " in *' -u '* | *' --update-h'*) _u=1 ;; esac
  moves_sub "$(printf '%s\n' "$_v" | awk 'NR == 1 { print $1 }')" "$(printf '%s\n' "$_v" | awk 'NR == 1 { print $2 }')" "$_u"
}
aimed() {  # aimed DIR: DIR's main checkout, when DIR is in it or its git dir ("?": the cwd's main checkout)
  if [ "$1" = '?' ]; then main_of "$cwd"; return; fi
  [ -d "$1" ] || return 1
  _g=$(git -C "$1" rev-parse --absolute-git-dir 2>/dev/null) || return 1
  _m=$(main_of "$1") || return 1
  [ "$(real "$_g")" = "$(real "$(git -C "$_m" rev-parse --absolute-git-dir 2>/dev/null)")" ] && printf '%s\n' "$_m"
}
move() {  # move DIR SUBCOMMAND NEXT-WORD UPDATE-HEAD ALIAS-VALUE: the main checkout git run in DIR moves
  is_move "$@" || return 0
  if [ "$shared" = 1 ]; then main_of "$1"; else aimed "$1"; fi
}
union() { printf '%s\n%s\n' "$1" "$2" | awk 'NF && !s[$0]++'; }
home() { case $1 in '~') printf '%s\n' "$HOME" ;; '~/'*) printf '%s/%s\n' "$HOME" "${1#\~/}" ;; *) printf '%s\n' "$1" ;; esac; }
under() {  # under BASE PATH: PATH read from BASE (PATH itself when absolute)
  case $2 in .) printf '%s\n' "$1" ;; /* | '~' | '~/'*) home "$2" ;; *) printf '%s/%s\n' "$1" "$2" ;; esac
}
cd_each() {  # cd_each TARGET: each candidate on stdin moved to TARGET; where that is no directory
  # (yet: a placeholder, or one the command makes), the candidate stays and "?" says so.
  while IFS= read -r _c; do
    _r=$(under "$_c" "$1")
    if [ -d "$_r" ]; then real "$_r"; else printf '%s\n?\n' "$_c"; fi
  done
}
settle() {  # settle OP: the pending cd this shell runs, now that OP follows it
  [ -n "$pend" ] || return 0
  if [ "$piped" = 1 ] || [ "$1" = '|' ] || [ "$1" = '&' ] || [ "$mixed" = 1 ]; then
    cands=$(union "$cands" "$pend")  # a pipeline's or the background's subshell, or after a ||
  elif [ "$pos" -gt 0 ]; then
    outer=$(union "$outer" "$cands"); cands=$pend  # sure for the rest of its && chain only
  else
    cands=$pend
  fi
  pend=
}
walk() {  # walk: the reader's lines on stdin; prints each main checkout a move is aimed at
  # cands: where this shell may be. pend: a cd it runs, until the operator after it is known.
  # outer: where it is if a conditional cd's && chain stops; pos: commands before, in this list.
  # A `( … )` subshell starts where this shell is, and this shell is back there at its `)`.
  cands=$cwd; unk=0; pend=; outer=; pos=0; mixed=0; piped=0; lvl=0
  while IFS="$TAB" read -r kind a b c u val; do
    [ "$kind" = SEP ] || settle ';'
    case $kind in
      SEP)
        settle "$a"
        case $a in
          '(')
            lvl=$((lvl + 1))
            eval "sc_$lvl=\$cands sp_$lvl=\$pos so_$lvl=\$outer sm_$lvl=\$mixed sq_$lvl=\$piped"
            pos=0; outer=; mixed=0; piped=0
            ;;
          ')')
            [ "$lvl" -gt 0 ] || continue
            eval "cands=\$sc_$lvl pos=\$sp_$lvl outer=\$so_$lvl mixed=\$sm_$lvl piped=\$sq_$lvl"
            lvl=$((lvl - 1))
            ;;
          '&&') pos=$((pos + 1)); piped=0 ;;
          '||') pos=$((pos + 1)); mixed=1; piped=0; cands=$(union "$cands" "$outer") ;;
          '|') piped=1 ;;
          *) cands=$(union "$cands" "$outer"); outer=; pos=0; mixed=0; piped=0 ;;
        esac
        ;;
      CD)
        case $b in '' | -* | +* | *'$'* | *'`'* | *'*'* | *'?'* | *'['* | '~'[!/]*) unk=1; continue ;; esac
        new=$(printf '%s\n' "$cands" | cd_each "$b")
        if printf '%s\n' "$new" | grep -qx '?'; then unk=1; new=$(printf '%s\n' "$new" | grep -vx '?'); fi
        if [ "$a" = T ]; then pend=$new; else cands=$(union "$cands" "$new"); fi
        ;;
      DYN)
        if [ "$dynmove" = 1 ]; then
          aimed '?'
          printf '%s\n' "$cands" | while IFS= read -r cand; do aimed "$cand"; done
        fi
        unk=1
        ;;
      GIT)
        if [ "$a" = '?' ]; then
          is_move "$cwd" "$b" "$c" "$u" "$val" && aimed '?'
        else
          printf '%s\n' "$cands" | while IFS= read -r cand; do
            d=$(under "$cand" "$a")
            # A -C to no directory (a placeholder xargs or find fills in) is unknown.
            if [ -d "$d" ]; then move "$d" "$b" "$c" "$u" "$val"; else is_move "$cwd" "$b" "$c" "$u" "$val" && aimed '?'; fi
          done
          [ "$unk" = 1 ] && is_move "$cwd" "$b" "$c" "$u" "$val" && aimed '?'
        fi
        ;;
    esac
  done
}

# Each main checkout a move is aimed at, once.
mains=$(printf '%s\n' "$reads" | walk | awk 'NF && !seen[$0]++')

[ -n "$mains" ] || exit 0
while IFS= read -r m; do judge "$m" "$cmd"; done <<EOF
$mains
EOF
exit 0
