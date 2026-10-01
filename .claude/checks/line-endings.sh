#!/bin/sh
# Shell scripts check out with LF on every OS. Git for Windows defaults to core.autocrlf=true, which
# checks text files out with CRLF unless the repository says otherwise, and then every hook, check
# and gate script fails in sh (`$'\r': command not found`), on Windows tools and on a WSL2 checkout
# made by them alike. The rule is the repository's .gitattributes (`* text=auto eol=lf`, which
# `clauductor install` and `update` ship); this holds it.
#
# Fails when: a new shell script would get no LF rule (the probe path below); any tracked *.sh, or
# any file under .claude/hooks/ or .claude/checks/, has no LF rule; or any of them holds a CR in the
# index or in this checkout. The set is the repository's own file list (`git ls-files`), not a list
# typed here. Falsified first in throwaway repos with core.autocrlf=true, which converts on checkout
# on any OS, so this runs the Windows failure on macOS and Linux too.
. "$(dirname "$0")/lib.sh"
need git awk

# le_scan DIR: one line per problem in the repository at DIR, and nothing when it is sound.
le_scan() {
  _p=$(git -C "$1" check-attr eol -- scripts/new-script.sh 2>/dev/null) || _p="(git check-attr failed in $1): failed"
  case $_p in
    *": eol: lf") ;;
    *) echo "no LF rule for shell scripts: a new one would get eol ${_p##*: } (add '* text=auto eol=lf' to .gitattributes)" ;;
  esac
  # Fail closed: a listing git could not produce reads as "nothing to check", never as clean.
  _l=$(git -C "$1" ls-files --eol -- '*.sh' '.claude/hooks/*' '.claude/checks/*' 2>&1) || {
    echo "git ls-files --eol failed in $1, so no script was checked: $(printf '%s' "$_l" | head -1)"; return; }
  printf '%s\n' "$_l" | grep . | awk -F '\t' '
    $1 ~ /[iw]\/(crlf|mixed)/ { print "carriage return in " $2 " (" $1 "): check it out again (rm it, then git checkout -- it), or commit it with LF"; next }
    $1 !~ /eol=lf/ { print "no LF rule for " $2 " (" $1 ")" }'
}
# le_count DIR: how many shell files the scan covers there.
le_count() { git -C "$1" ls-files -- '*.sh' '.claude/hooks/*' '.claude/checks/*' | wc -l | tr -d ' '; }

d=$(scratch)
# fixture NAME [GITATTRIBUTES]: a repo whose scripts were committed with LF and then checked out
# with core.autocrlf=true, as Git for Windows does by default.
fixture() {
  r="$d/$1"; new_repo "$r"
  git -C "$r" config core.autocrlf true
  git -C "$r" config core.safecrlf false   # quiet: the conversion is the point here
  mkdir -p "$r/.claude/hooks" "$r/scripts/ci"
  printf '#!/bin/sh\necho hook\n' > "$r/.claude/hooks/guard.sh"
  printf '#!/bin/sh\necho gate\n' > "$r/scripts/ci/gate.sh"
  [ $# -gt 1 ] && printf '%s\n' "$2" > "$r/.gitattributes"
  git -C "$r" add -A && git -C "$r" commit -qm init
  rm "$r/.claude/hooks/guard.sh" "$r/scripts/ci/gate.sh" && git -C "$r" checkout -q -- .
}

fixture bare
grep -q "$(printf '\r')" "$d/bare/scripts/ci/gate.sh" && ok "fixture: core.autocrlf=true with no rule checks a script out with CRLF" \
  || fail "fixture: the autocrlf checkout did not produce CRLF, so the falsification below learned nothing"
out=$(le_scan "$d/bare")
case $out in *"no LF rule for shell scripts"*) ok "falsified: a repository with no LF rule is caught" ;; *) fail "missed: no .gitattributes ($out)" ;; esac
case $out in *"carriage return in scripts/ci/gate.sh"*) ok "falsified: a script checked out with CRLF is named" ;; *) fail "missed: a CRLF checkout ($out)" ;; esac

fixture ruled '* text=auto eol=lf'
out=$(le_scan "$d/ruled")
[ -z "$out" ] && ok "...and with '* text=auto eol=lf' the same autocrlf checkout is LF and passes" || fail "the template's rule does not pass: $out"

printf '#!/bin/sh\r\necho late\r\n' > "$d/ruled/.claude/hooks/late.sh"
git -C "$d/ruled" add -A && git -C "$d/ruled" commit -qm late
out=$(le_scan "$d/ruled")
case $out in *"carriage return in .claude/hooks/late.sh"*) ok "falsified: a script written with CRLF stays caught in the checkout though the index is LF" ;; *) fail "missed: a CRLF working copy under the rule ($out)" ;; esac

fixture overridden "$(printf '* text=auto eol=lf\n*.sh text eol=crlf')"
out=$(le_scan "$d/overridden")
case $out in *"no LF rule for shell scripts"*) ok "falsified: a later line that gives shell scripts CRLF is caught" ;; *) fail "missed: '*.sh eol=crlf' after the LF rule ($out)" ;; esac

# Fail closed: a repository git cannot list (a corrupt index) is a problem, not an empty list.
fixture broken '* text=auto eol=lf'
printf 'not an index' > "$d/broken/.git/index"
out=$(le_scan "$d/broken")
case $out in *"ls-files --eol failed"*) ok "falsified: a listing git could not produce fails the scan, not passes it" ;; *) fail "a corrupt index read as clean ($out)" ;; esac

# ── This project ────────────────────────────────────────────────────────────────────────────────
git -C "$ROOT" rev-parse --git-dir >/dev/null 2>&1 || { fail "cannot check: $ROOT is not a git checkout"; finish; }
out=$(le_scan "$ROOT")
if [ -z "$out" ]; then
  ok "every tracked shell script in $ROOT ($(le_count "$ROOT") files) has the LF rule and no CR, and a new one gets LF"
else
  printf '%s\n' "$out" | head -12 | while IFS= read -r l; do echo "FAIL $l"; done
  fail "line endings in $ROOT: $(printf '%s\n' "$out" | wc -l | tr -d ' ') problem(s), the first 12 above"
fi
finish
