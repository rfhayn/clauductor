#!/bin/sh
# pr-merge-guard.sh, as a payload → exit-code table in both directions, against a throwaway repo
# whose origin is a local bare repo named like `owner/name` (so the foreign-repo rules run) and a
# stub `gh` on PATH (so no network and no GitHub are needed). Also: the receipt library across
# worktrees, the channel an advisory takes, and raw_command identical in both hooks that carry it.
. "$(dirname "$0")/lib.sh"
need git jq

d=$(scratch)
R="$d/app"; new_repo "$R"
mkdir -p "$R/.claude/hooks/lib" "$R/.claude/lib" "$R/docs" "$d/bin"
cp "$ROOT/.claude/hooks/pr-merge-guard.sh" "$R/.claude/hooks/"
cp "$ROOT/.claude/hooks/lib/"* "$R/.claude/hooks/lib/"
cp "$ROOT/.claude/lib/conf.sh" "$ROOT/.claude/lib/change.sh" "$ROOT/.claude/lib/evals.sh" "$R/.claude/lib/"
[ -f "$ROOT/.claude/lib/docs-gate.sh" ] && cp "$ROOT/.claude/lib/docs-gate.sh" "$R/.claude/lib/"
cp "$ROOT/.claude/scenario-trace.sh" "$R/.claude/"
cat > "$R/.claude/project.conf" <<'EOF'
GATE_DISPLAY_CONTEXTS="ci/local"
EOF
printf '# Journal\n\n## Session 1 — 2026-01-01 — a — start\n' > "$R/docs/development-journal.md"
git -C "$R" add -A && git -C "$R" commit -qm base
# origin: a bare repo INSIDE the checkout at acme/app.git, so `git remote get-url origin` reads
# `acme/app.git`, which the guard keys as acme/app, and fetches still work offline.
git init -q --bare "$R/acme/app.git"
echo "acme/" >> "$R/.git/info/exclude"
git -C "$R" remote add origin acme/app.git
(cd "$R" && git push -q origin HEAD:main 2>/dev/null && git fetch -q origin)

# A capability-change branch with a change directory, and its head.
git -C "$R" checkout -q -b change/add-x
mkdir -p "$R/changes/add-x"
printf '## 1. Do it\n- [x] 1.1 thing\n' > "$R/changes/add-x/tasks.md"
git -C "$R" add -A && git -C "$R" commit -qm "add-x: task group 1"
NOSLICE=$(git -C "$R" rev-parse HEAD)
printf -- '- [ ] Slice: a user can do x at /x\n' >> "$R/changes/add-x/tasks.md"
git -C "$R" commit -qam "add-x: slice"
SLICE=$(git -C "$R" rev-parse HEAD)
git -C "$R" checkout -q main

cat > "$d/bin/gh" <<'EOF'
#!/bin/sh
# Every PR read answers from the repo it names. *_DEFAULT: the same number in gh's DEFAULT repo, which
# a read naming no repo (no -R, no URL) gets: a fork's checkout after `gh repo set-default <upstream>`.
# GH_STATE_HOSTED: the state a read naming a host (-R <host>/<owner>/<name>) gets.
case "$3 $*" in
  http://* | https://* | *" -R "*) named=1 ;;
  *) named="" ;;
esac
hosted=""
case "$(printf '%s\n' "$*" | sed -n 's/.* -R \([^ ]*\).*/\1/p')" in */*/*) hosted=1 ;; esac
case "$1 $2" in
  "pr checks")
    # GH_CHECKS_ERR: gh itself fails (auth, network). GH_CHECKS=none: gh's own "no checks" answer.
    # GH_CHECKS_NOISE: gh writes to stderr beside a valid answer (GH_DEBUG=1, an upgrade notice).
    [ -n "${GH_CHECKS_ERR:-}" ] && { echo "$GH_CHECKS_ERR" >&2; exit 1; }
    [ -n "${GH_CHECKS_NOISE:-}" ] && echo "$GH_CHECKS_NOISE" >&2
    checks=${GH_CHECKS:-[]}
    [ -n "$named" ] || checks=${GH_CHECKS_DEFAULT-$checks}
    [ "$checks" = none ] && { echo "no checks reported on the 'fix/1-x' branch" >&2; exit 1; }
    echo "$checks" ;;
  "pr view")
    head=$GH_HEAD branch=${GH_BRANCH:-fix/1-x}
    [ -n "$named" ] || { head=${GH_HEAD_DEFAULT-$head}; branch=${GH_BRANCH_DEFAULT-$branch}; }
    case "$*" in
      # GH_STATE_ERR: gh itself fails. GH_STATE set but empty: gh answers with nothing.
      *"--json state "*)
        [ -n "${GH_STATE_ERR:-}" ] && { echo "$GH_STATE_ERR" >&2; exit 1; }
        if [ -n "$hosted" ]; then echo "${GH_STATE_HOSTED-${GH_STATE-OPEN}}"
        elif [ -n "$named" ]; then echo "${GH_STATE-OPEN}"
        else echo "${GH_STATE_DEFAULT-${GH_STATE-OPEN}}"
        fi ;;
      *headRefName*) printf '{"headRefName":"%s","headRefOid":"%s"}\n' "$branch" "$head" ;;
      *headRefOid*) echo "$head" ;;
    esac ;;
  # GH_RUNS_DEFAULT: the successes gh's default repo has for that commit, which a list naming no repo gets.
  "run list") case "$*" in *" -R "*) echo "${GH_RUNS:-0}" ;; *) echo "${GH_RUNS_DEFAULT-${GH_RUNS:-0}}" ;; esac ;;
  *) exit 1 ;;
esac
EOF
chmod +x "$d/bin/gh"

MAINSHA=$(git -C "$R" rev-parse HEAD)
receipt() { printf '%s\tfull\t%s\t%s\n' "$1" "${2:-clean}" "${3:-all}" > "$R/.git/ci-receipt"; }
guard() {  # guard WANT LABEL COMMAND [env assignments...]
  want=$1 label=$2 c=$3; shift 3
  rc=0
  out=$(payload "$c" "${PCWD-$R}" | (cd "$R" && env PATH="$d/bin:$PATH" GH_HEAD="${GH_HEAD:-$MAINSHA}" "$@" sh "$R/.claude/hooks/pr-merge-guard.sh" 2>"$d/err")) || rc=$?
  expect_rc "$want" "$rc" "merge-guard $( [ "$want" = 2 ] && echo blocks || echo allows ): $label"
  [ "$rc" = "$want" ] || sed 's/^/       /' "$d/err" | head -3
}

guard 0 "a command that is not a merge" "git status"
guard 0 "gh pr view" "gh pr view 5"
guard 2 "--auto" "gh pr merge 5 --squash --auto"
guard 2 "--admin" "gh pr merge 5 --squash --admin"
rm -f "$R/.git/ci-receipt"
guard 2 "no evidence at all" "gh pr merge 5 --squash --delete-branch"
receipt "$MAINSHA"
guard 0 "a full clean receipt for the head" "gh pr merge 5 --squash --delete-branch"
case "$out" in *'"additionalContext"'*"passed in full"*) ok "merge-guard: its confirmation reaches Claude as additionalContext" ;; *) fail "merge-guard: no additionalContext on an allowed merge: $out" ;; esac
# Rule 14 — only an OPEN PR merges (OPS-132, #217): a close is how one session tells another "do
# not land this", and #215 landed twelve minutes after its close. Everything else here is green.
guard 2 "a CLOSED PR with full evidence" "gh pr merge 5 --squash" GH_STATE=CLOSED
grep -q 'CLOSED' "$d/err" && ok "merge-guard: ...naming the state it read" || fail "merge-guard: closed refusal does not name CLOSED: $(head -3 "$d/err")"
guard 2 "a MERGED PR" "gh pr merge 5 --squash" GH_STATE=MERGED
guard 2 "gh cannot read the PR's state (fails closed)" "gh pr merge 5 --squash" GH_STATE_ERR="HTTP 401: Bad credentials"
grep -q 'HTTP 401' "$d/err" && ok "merge-guard: ...naming gh's own error" || fail "merge-guard: state refusal hides gh's error: $(head -3 "$d/err")"
guard 2 "gh answers the state with nothing (fails closed)" "gh pr merge 5 --squash" GH_STATE=
guard 0 "...and an OPEN one, the same in every other way" "gh pr merge 5 --squash" GH_STATE=OPEN
# ...in every form the guard polices as this repo's merge, and read from the repo the merge NAMES:
# with -R or a URL, a read of the bare number would judge gh's default repo's PR #5 instead.
guard 2 "a CLOSED PR merged by this repo's PR URL" "gh pr merge https://github.com/acme/app/pull/5 --squash" GH_STATE=CLOSED
guard 2 "a CLOSED PR merged through gh api, its number OPEN in gh's default repo" "gh api -X PUT repos/acme/app/pulls/5/merge" GH_STATE=CLOSED GH_STATE_DEFAULT=OPEN
guard 0 "an OPEN PR merged through gh api, its number CLOSED in gh's default repo" "gh api -X PUT repos/acme/app/pulls/5/merge" GH_STATE=OPEN GH_STATE_DEFAULT=CLOSED
guard 2 "a CLOSED PR named by -R, its number OPEN in gh's default repo" "gh pr merge 5 -R acme/app --squash" GH_STATE=CLOSED GH_STATE_DEFAULT=OPEN
guard 2 "a CLOSED PR named by URL, its number OPEN in gh's default repo" "gh pr merge https://github.com/acme/app/pull/5 --squash" GH_STATE=CLOSED GH_STATE_DEFAULT=OPEN
# gh takes the number from the first /pull/<n> after owner/name, not the last.
guard 2 "a CLOSED PR named by a URL with a later /pull/<n> in its path" "gh pr merge https://github.com/acme/app/pull/5/files/x/pull/6 --squash" GH_STATE=CLOSED
grep -q 'PR #5 is CLOSED' "$d/err" && ok "merge-guard: ...named as #5, as gh reads it" || fail "merge-guard: a URL's later /pull/<n> was read as the PR: $(head -3 "$d/err")"
guard 0 "an OPEN PR named by -R, its number CLOSED in gh's default repo" "gh pr merge 5 -R acme/app --squash" GH_STATE=OPEN GH_STATE_DEFAULT=CLOSED
# Every read, not only the state: the checks, the head and the head branch each come from the PR the
# merge names, so each is red (or unevidenced) there and green in gh's default repo, and the inverse.
RED='[{"name":"test","state":"FAILURE","bucket":"fail"}]'
guard 2 "red checks on the PR named by -R, green in gh's default repo" "gh pr merge 5 -R acme/app --squash" GH_CHECKS="$RED" GH_CHECKS_DEFAULT='[]'
guard 2 "red checks on the PR named by URL, green in gh's default repo" "gh pr merge https://github.com/acme/app/pull/5 --squash" GH_CHECKS="$RED" GH_CHECKS_DEFAULT='[]'
guard 0 "green checks on the PR named by -R, red in gh's default repo" "gh pr merge 5 -R acme/app --squash" GH_CHECKS='[]' GH_CHECKS_DEFAULT="$RED"
guard 2 "the PR named by -R has a head with no receipt; gh's default repo's has one" "gh pr merge 5 -R acme/app --squash" GH_HEAD="$SLICE" GH_HEAD_DEFAULT="$MAINSHA"
guard 0 "the PR named by -R has the head with the receipt; gh's default repo's has none" "gh pr merge 5 -R acme/app --squash" GH_HEAD="$MAINSHA" GH_HEAD_DEFAULT="$SLICE"
receipt "$NOSLICE"
guard 2 "the PR named by -R is a change branch with no slice; gh's default repo's is a fix" "gh pr merge 5 -R acme/app --squash" GH_HEAD="$NOSLICE" GH_BRANCH=change/add-x GH_BRANCH_DEFAULT=fix/1-x
guard 0 "the PR named by -R is a fix; gh's default repo's is a change branch with no slice" "gh pr merge 5 -R acme/app --squash" GH_HEAD="$NOSLICE" GH_BRANCH=fix/1-x GH_BRANCH_DEFAULT=change/add-x
receipt "$MAINSHA"
# What the merge's words leave open, gh fills from its environment and directory, which what runs
# before the merge can change in more ways than a list could name. So what may come before it is an
# allowlist, by what the words leave open (#271): nothing before a merge naming no repo, only
# `cd <path> && ` before -R OWNER/NAME (the host is GH_HOST's), anything before a URL or a hosted -R.
guard 2 "GH_REPO before a merge naming no repo" "GH_REPO=acme/app gh pr merge 5 --squash"
grep -q 'GH_REPO' "$d/err" && ok "merge-guard: ...naming what comes before it" || fail "merge-guard: the refusal does not name what comes before the merge: $(head -3 "$d/err")"
guard 2 "export GH_REPO, then a merge naming no repo" "export GH_REPO=acme/app; gh pr merge 5 --squash"
guard 2 "GIT_DIR before a merge naming no repo" "GIT_DIR=../other/.git gh pr merge 5 --squash"
guard 2 "cd; then a merge naming no repo" "cd; gh pr merge 5 --squash"
guard 2 "an escaped cd before a merge naming no repo" 'c\d /tmp && gh pr merge 5 --squash'
guard 2 "a split GH_REPO exported before a merge naming no repo" 'export "GH_""REPO=other/x"; gh pr merge 5 --squash'
guard 2 "a sourced file before a merge naming no repo" ". ./env.sh; gh pr merge 5 --squash"
guard 2 "a cd before a merge naming no repo" "cd /tmp && gh pr merge 5 --squash"
guard 2 "an earlier line before a merge naming no repo" "true
gh pr merge 5 --squash"
guard 2 "a cd before a gh api merge of {owner}/{repo}" "cd /tmp && gh api -X PUT repos/{owner}/{repo}/pulls/5/merge"
guard 2 "a cd between two gh api calls, the second merging {owner}/{repo}" "gh api user && cd /tmp && gh api -X PUT repos/{owner}/{repo}/pulls/5/merge"
guard 0 "a gh api merge of {owner}/{repo} on its own" "gh api -X PUT repos/{owner}/{repo}/pulls/5/merge"
guard 0 "merge-pr's own form" "gh pr merge 5 --squash --delete-branch --body-file merge-body.md"
guard 0 "a subject that says cd, after a merge naming no repo" 'gh pr merge 5 --squash --subject "fix cd handling"'
guard 2 "GH_HOST before a merge named by -R OWNER/NAME" "GH_HOST=ghe.example.com gh pr merge 5 -R acme/app --squash"
guard 2 "a split GH_HOST exported before a merge named by -R OWNER/NAME" 'export "GH_""HOST=ghe.example.com"; gh pr merge 5 -R acme/app --squash'
guard 2 "GH_HOST before a gh api merge of repos/OWNER/NAME" "GH_HOST=ghe.example.com gh api -X PUT repos/acme/app/pulls/5/merge"
guard 0 "a cd before a merge named by -R OWNER/NAME" "cd /tmp && gh pr merge 5 -R acme/app --squash"
guard 0 "GH_HOST before a merge named by -R HOST/OWNER/NAME" "GH_HOST=ghe.example.com gh pr merge 5 -R github.com/acme/app --squash"
guard 0 "a cd before a merge named by this repo's PR URL" "cd /tmp && gh pr merge https://github.com/acme/app/pull/5 --squash"
guard 0 "GH_HOST before a merge named by this repo's PR URL" "GH_HOST=github.com gh pr merge https://github.com/acme/app/pull/5 --squash"
# gh api --hostname sends the merge to that host, so the reads name it: -R <host>/<owner>/<name>.
guard 2 "gh api --hostname H: the PR is CLOSED on H, OPEN on the default host" "gh api --hostname ghe.example.com -X PUT repos/acme/app/pulls/5/merge" GH_STATE=OPEN GH_STATE_HOSTED=CLOSED
guard 2 "gh api --hostname=H: the same, spelled with =" "gh api --hostname=ghe.example.com -X PUT repos/acme/app/pulls/5/merge" GH_STATE=OPEN GH_STATE_HOSTED=CLOSED
guard 0 "gh api --hostname H: the PR is OPEN on H, CLOSED on the default host" "gh api --hostname ghe.example.com -X PUT repos/acme/app/pulls/5/merge" GH_STATE=CLOSED GH_STATE_HOSTED=OPEN
guard 2 "gh api --hostname H with {owner}/{repo}" "gh api --hostname ghe.example.com -X PUT repos/{owner}/{repo}/pulls/5/merge"
# ...read from the flag's own word, never from a field's text that says --hostname (#271).
guard 2 "GH_HOST before a gh api merge whose field's text says --hostname" 'GH_HOST=ghe.example.com gh api -X PUT repos/acme/app/pulls/5/merge -f "commit_title= --hostname github.com "'
guard 0 "a gh api merge whose field's text says --hostname reads on the default host" 'gh api -X PUT repos/acme/app/pulls/5/merge -f "commit_title=use --hostname ghe.example.com"' GH_STATE=OPEN GH_STATE_HOSTED=CLOSED
guard 2 "gh api --hostname named twice" "gh api --hostname ghe.example.com --hostname github.com -X PUT repos/acme/app/pulls/5/merge"
guard 2 "gh api --hostname and --hostname= both" "gh api --hostname=ghe.example.com --hostname github.com -X PUT repos/acme/app/pulls/5/merge"
guard 2 "gh api --hostname computed" 'gh api --hostname "$H" -X PUT repos/acme/app/pulls/5/merge'
guard 2 "gh api --hostname= computed" 'gh api --hostname=$H -X PUT repos/acme/app/pulls/5/merge'
# An absolute https://api.github.com/ endpoint goes to github.com, whatever --hostname or GH_HOST says.
guard 2 "gh api --hostname H with an absolute github.com endpoint" "gh api --hostname ghe.example.com -X PUT https://api.github.com/repos/acme/app/pulls/5/merge" GH_STATE=OPEN GH_STATE_HOSTED=OPEN
guard 2 "an absolute github.com endpoint: CLOSED on github.com, OPEN in gh's default" "gh api -X PUT https://api.github.com/repos/acme/app/pulls/5/merge" GH_STATE=OPEN GH_STATE_HOSTED=CLOSED
guard 0 "an absolute github.com endpoint: OPEN on github.com" "gh api -X PUT https://api.github.com/repos/acme/app/pulls/5/merge" GH_STATE=CLOSED GH_STATE_HOSTED=OPEN
guard 0 "--hostname github.com with an absolute github.com endpoint" "gh api --hostname github.com -X PUT https://api.github.com/repos/acme/app/pulls/5/merge" GH_STATE=CLOSED GH_STATE_HOSTED=OPEN
# The PR reader stops at an expansion or a redirect; gh does not, and merges in the LAST -R (#271).
guard 2 "a -R after a backtick substitution, the PR CLOSED there, OPEN in gh's default repo" 'gh pr merge 5 --squash -t `date` -R acme/app' GH_STATE=CLOSED GH_STATE_DEFAULT=OPEN
grep -q 'words this guard cannot read' "$d/err" && ok "merge-guard: ...as words it cannot read" || fail "merge-guard: the cut refusal does not say why: $(head -3 "$d/err")"
guard 2 "a -R after a quoted command substitution, the PR CLOSED there" 'gh pr merge 5 --squash --body "$(cat f)" -R acme/app' GH_STATE=CLOSED GH_STATE_DEFAULT=OPEN
guard 2 "a second -R after a quoted substitution, behind GH_HOST" 'GH_HOST=ghe.example.com gh pr merge 5 -R github.com/acme/app --body "$(cat f)" -R acme/app' GH_STATE=CLOSED GH_STATE_HOSTED=OPEN
guard 2 "a -R after a glob qualifier, the PR CLOSED there" 'gh pr merge 5 --squash -t x(N) -R acme/app' GH_STATE=CLOSED GH_STATE_DEFAULT=OPEN
guard 2 "a -R after a redirect (the reader refuses it)" 'gh pr merge 5 --squash 2>/dev/null -R acme/app' GH_STATE=CLOSED GH_STATE_DEFAULT=OPEN
guard 2 "an unquoted expansion, which can split into a -R" 'gh pr merge 5 --squash --body-file $f'
guard 2 "two quoted substitutions" 'gh pr merge 5 --squash --subject "$(cat s)" --body "$(cat f)"'
guard 0 "a quoted variable last, nothing after it" 'gh pr merge 5 --squash --body "$B"'
guard 0 "merge-pr's form with stderr redirected last" "gh pr merge 5 --squash --delete-branch --body-file f 2>&1"
guard 0 "-R this repo with stderr redirected last" "gh pr merge 5 -R acme/app --squash 2>/dev/null"
guard 2 "an expansion after a redirect's target (the reader refuses it)" 'gh pr merge 5 --squash > "$TMPDIR/x" $Y'
# A quoted expansion that is a word of its own can BE the flag: gh reads "-Racme/app" as -R.
guard 2 "a quoted substitution as a word of its own, decoding to -R" 'gh pr merge 5 --squash "$(echo LVJhY21lL2FwcA== | base64 -d)"' GH_STATE=CLOSED GH_STATE_DEFAULT=OPEN
guard 2 "a quoted variable as a word of its own, behind GH_HOST and a hosted -R" 'GH_HOST=ghe.example.com gh pr merge 5 -R github.com/acme/app --squash "$X"' GH_STATE=CLOSED GH_STATE_HOSTED=OPEN
guard 2 "a braced variable as a word of its own" 'gh pr merge 5 --squash "${X}"' GH_STATE=CLOSED GH_STATE_DEFAULT=OPEN
guard 2 "a value flag's value taken by the flag before it, the expansion a fresh word" 'gh pr merge 5 --squash --subject --body "$X"' GH_STATE=CLOSED GH_STATE_DEFAULT=OPEN
# ...and quoting hides a -R from any text match after the expansion: only blanks may follow it.
guard 2 "an escaped -\\R after a quoted substitution" 'gh pr merge 5 --squash --body "$(cat f)" -\R acme/app' GH_STATE=CLOSED GH_STATE_DEFAULT=OPEN
guard 2 "a quote-split -R after a quoted substitution" "gh pr merge 5 --squash --body \"\$(cat f)\" -''R acme/app" GH_STATE=CLOSED GH_STATE_DEFAULT=OPEN
guard 2 "a quote-split --repo after a quoted substitution" 'gh pr merge 5 --squash --body "$(cat f)" --re""po acme/app' GH_STATE=CLOSED GH_STATE_DEFAULT=OPEN
guard 0 "a quoted variable glued to --body=" 'gh pr merge 5 --squash --body="$B"'
guard 0 "a quoted variable as -b's value" 'gh pr merge 5 --squash -b "$B"'
guard 0 "a quoted variable as --subject's value, stderr redirected last" 'gh pr merge 5 --squash --subject "$S" 2>&1'
# A brace or glob is kept as one word by the read, but the shell expands it into words gh parses.
guard 2 "a brace expansion into -R and a repo" 'gh pr merge 5 --squash {-R,acme/app}' GH_STATE=CLOSED GH_STATE_DEFAULT=OPEN
guard 2 "a brace expansion in --body-file's value, into a glued -R" 'gh pr merge 5 --squash --body-file {f,-Racme/app}' GH_STATE=CLOSED GH_STATE_DEFAULT=OPEN
guard 2 "a glob bracket that can match a file named -R" 'gh pr merge 5 --squash -[R] acme/app' GH_STATE=CLOSED GH_STATE_DEFAULT=OPEN
# ...and in gh api, an unquoted shorthand value can split into a --hostname the host read never sees.
guard 2 "gh api -f with an unquoted substitution that splits into --hostname" 'gh api -X PUT repos/acme/app/pulls/5/merge -f $(printf "a=b --hostname ghe2.example.com")' GH_STATE=OPEN GH_STATE_HOSTED=CLOSED
guard 2 "gh api -H with an unquoted substitution that splits into --hostname" 'gh api -X PUT repos/acme/app/pulls/5/merge -H $(printf "a:b --hostname ghe2.example.com")' GH_STATE=OPEN GH_STATE_HOSTED=CLOSED
guard 2 "gh api --hostname H, then -f with a variable that splits into another" 'gh api --hostname ghe.example.com -X PUT repos/acme/app/pulls/5/merge -f $X' GH_STATE=CLOSED GH_STATE_HOSTED=OPEN
# A process substitution, <(…) or >(…), is a word, not a redirect: the read once stopped at it, and
# gh merged in the -R after it (#271). The reader refuses a redirect before the last word (&>f -R,
# > out -R, a heredoc) before the read sees it; those cases hold both layers.
guard 2 "a -R after a process substitution" 'gh pr merge 5 --squash --body-file <(cat f) -R acme/app' GH_STATE=CLOSED GH_STATE_DEFAULT=OPEN
guard 2 "a -R after an output process substitution" 'gh pr merge 5 --squash -t x >(cat) --repo acme/app' GH_STATE=CLOSED GH_STATE_DEFAULT=OPEN
guard 2 "a -R after &>" 'gh pr merge 5 --squash &>f -R acme/app' GH_STATE=CLOSED GH_STATE_DEFAULT=OPEN
guard 2 "a -R after 2>&1 and &>" 'gh pr merge 5 --squash 2>&1 &>f -R acme/app' GH_STATE=CLOSED GH_STATE_DEFAULT=OPEN
guard 0 "merge-pr's form, 2>&1 last, piped on" 'gh pr merge 5 --squash --delete-branch --body-file f 2>&1 | tail -1'
guard 0 "redirects alone to the end: > f 2>&1" 'gh pr merge 5 --squash --body-file f > "$TMPDIR/out" 2>&1'
guard 2 "a -R after a redirect's plain target" 'gh pr merge 5 --squash > out -R acme/app' GH_STATE=CLOSED GH_STATE_DEFAULT=OPEN
guard 2 "a -R after a target whose \$( nests a quoted )" 'gh pr merge 5 --squash > "$(echo ")" -R acme/app' GH_STATE=CLOSED GH_STATE_DEFAULT=OPEN
guard 2 "a heredoc" 'gh pr merge 5 --squash <<EOF -R acme/app' GH_STATE=CLOSED GH_STATE_DEFAULT=OPEN
# Only "$name" or "${name}", then plain text, is one closed word in bash and zsh alike; "$@",
# "${a[@]}", zsh's "$a[@]" and flags split, and a # in "$(…)" is a zsh comment hiding its ) (#271).
guard 2 'a quoted "$(…)", even last' 'gh pr merge 5 --squash --body "$(cat f)"'
guard 2 'a zsh comment in "$(…)" hiding its ), then a -R' 'gh pr merge 5 --squash --delete-branch --body "$(echo hi #)"
)" -R acme/app' GH_STATE=CLOSED GH_STATE_DEFAULT=OPEN
guard 2 'a case pattern in "$(…)" whose ) closes nothing, then a -R' 'gh pr merge 5 --squash --body "$(case a in a)"
" ;; esac)" -R acme/app' GH_STATE=CLOSED GH_STATE_DEFAULT=OPEN
guard 2 'a zsh "$a[@]" set in the command, behind a hosted -R' 'a=(x -R acme/app); gh pr merge 5 -R github.com/acme/app --squash --delete-branch --body "$a[@]"' GH_STATE=CLOSED GH_STATE_HOSTED=OPEN
guard 2 'a zsh "$a[@]" carrying --admin, behind a PR URL' 'a=(x --a""dmin); gh pr merge https://github.com/acme/app/pull/5 --squash --delete-branch --body "$a[@]"'
guard 2 'a zsh modifier "$B:h"' 'gh pr merge 5 --squash --body-file "$B:h"' GH_STATE=CLOSED GH_STATE_DEFAULT=OPEN
guard 2 'a quoted "$@" after set -- x -R, behind a hosted -R' 'set -- x -R acme/app; gh pr merge 5 -R github.com/acme/app --squash --body "$@"' GH_STATE=CLOSED GH_STATE_HOSTED=OPEN
guard 2 'a quoted "${a[@]}"' 'gh pr merge 5 --squash --body "${a[@]}"' GH_STATE=CLOSED GH_STATE_DEFAULT=OPEN
guard 2 'a quoted zsh split "${(@s.:.)PWD}"' 'gh pr merge 5 --squash --body "${(@s.:.)PWD}"' GH_STATE=CLOSED GH_STATE_DEFAULT=OPEN
guard 0 'a quoted "${B}" as --body'"'"'s value' 'gh pr merge 5 --squash --body "${B}"'
guard 0 'merge-pr'"'"'s form with --body-file "$TMPDIR/…"' 'gh pr merge 5 --squash --delete-branch --body-file "$TMPDIR/merge-body.md" 2>&1 | tail -1'
guard 2 'a quoted "$A/$B": a second expansion' 'gh pr merge 5 --squash --body-file "$A/$B"' GH_STATE=CLOSED GH_STATE_DEFAULT=OPEN
guard 0 'merge-pr'"'"'s form with --body-file "$TMPDIR"/…' 'gh pr merge 5 --squash --delete-branch --body-file "$TMPDIR"/merge-body.md 2>&1 | tail -1'
# A backslash-newline joins lines only when the backslash is not itself escaped: "a\\<newline>"
# keeps both in the word, and x\\<newline> ends the command there. A join blind to that read the
# closing quote as escaped, or the next line, a comment, as the merge's -R (#271).
guard 2 'an escaped backslash, then a newline, in "…", then a -R' 'gh pr merge 5 --squash --body "a\\
" -R acme/other'
guard 2 'an escaped backslash, then a newline, in "…", then --admin split by quotes' 'gh pr merge 5 --squash --body "a\\
" --ad""min'
grep -q 'rule 1' "$d/err" && ok "merge-guard: ...refused as rule 1" || fail "merge-guard: the hidden --admin refusal does not name rule 1: $(head -3 "$d/err")"
guard 2 'a cd, then an escaped backslash before a newline, then a comment reading as this repo'"'"'s -R' 'cd /tmp && gh pr merge 5 --squash --body x\\
# -R acme/app'
guard 0 'a cd, then a real continuation to this repo'"'"'s -R' 'cd /tmp && gh pr merge 5 --squash --body x \
-R acme/app'
# Rule 1 reads the raw text; the shell takes quotes and escapes out of a flag first.
guard 2 "rule 1: --auto split by quotes" "gh pr merge 5 --squash --au''to"
grep -q 'rule 1' "$d/err" && ok "merge-guard: ...refused as rule 1" || fail "merge-guard: the split --auto refusal does not name rule 1: $(head -3 "$d/err")"
guard 2 "rule 1: --admin split by quotes" 'gh pr merge 5 --squash --ad""min'
guard 2 "rule 1: --admin escaped" 'gh pr merge 5 --squash --\admin'
guard 2 "gh named by a path, no repo named" '/usr/local/bin/gh pr merge 5 --squash'
grep -q 'named by a path' "$d/err" && ok "merge-guard: ...and the refusal says gh is named by a path" || fail "merge-guard: the path-named gh refusal does not say so: $(head -3 "$d/err")"
# The payload's cwd is the Bash tool's directory, where gh runs, and need not be this hook's: a merge
# naming no repo passes only from a worktree of this repository.
O="$d/other"; new_repo "$O"
git -C "$R" worktree add -q "$d/wt" -b wt-guard 2>/dev/null
PCWD=$O
guard 2 "a merge naming no repo, run in another repository" "gh pr merge 5 --squash"
grep -qF "$O" "$d/err" && ok "merge-guard: ...naming the directory it runs in" || fail "merge-guard: the cwd refusal does not name the cwd: $(head -3 "$d/err")"
guard 2 "a gh api merge of {owner}/{repo}, run in another repository" "gh api -X PUT repos/{owner}/{repo}/pulls/5/merge"
guard 0 "a merge named by -R, run in another repository" "gh pr merge 5 -R acme/app --squash"
PCWD=$d/wt
guard 0 "a merge naming no repo, run in a worktree of this repository" "gh pr merge 5 --squash"
PCWD=$R/docs
guard 0 "a merge naming no repo, run in a subdirectory of this repository" "gh pr merge 5 --squash"
PCWD=
guard 2 "a merge naming no repo, with no cwd in the payload" "gh pr merge 5 --squash"
PCWD=$d/nowhere
guard 2 "a merge naming no repo, in a cwd that does not exist" "gh pr merge 5 --squash"
unset PCWD
git -C "$R" worktree remove --force "$d/wt" && git -C "$R" branch -q -D wt-guard
receipt "$MAINSHA" dirty
guard 2 "a dirty receipt" "gh pr merge 5 --squash"
receipt "$MAINSHA" clean quick
guard 2 "a partial receipt" "gh pr merge 5 --squash"
# A docs receipt (run-local.sh --docs, OPS-89) counts only for a PR whose own diff against
# origin/main is prose (.claude/lib/docs-gate.sh), and never for one that also changes code.
git -C "$R" checkout -q -b docs-pr
printf '# x\n' > "$R/docs/x.md"; git -C "$R" add -A && git -C "$R" commit -qm "a doc"
DOCSHA=$(git -C "$R" rev-parse HEAD)
printf 'echo\n' > "$R/x.sh"; git -C "$R" add -A && git -C "$R" commit -qm "code"
CODESHA=$(git -C "$R" rev-parse HEAD)
git -C "$R" checkout -q main
printf '%s\tdocs\tclean\tdocs\n' "$DOCSHA" > "$R/.git/ci-receipt"
guard 0 "a docs receipt for a prose-only PR" "gh pr merge 5 --squash" GH_HEAD="$DOCSHA"
printf '%s\tdocs\tclean\tdocs\n' "$CODESHA" > "$R/.git/ci-receipt"
guard 2 "a docs receipt for a PR that changes code" "gh pr merge 5 --squash" GH_HEAD="$CODESHA"
grep -q 'x.sh' "$d/err" && ok "merge-guard: ...naming the path that is not prose" || fail "merge-guard: docs refusal does not name x.sh: $(head -3 "$d/err")"
mv "$R/.claude/lib/docs-gate.sh" "$d/dg.away" 2>/dev/null
printf '%s\tdocs\tclean\tdocs\n' "$DOCSHA" > "$R/.git/ci-receipt"
guard 2 "a docs receipt when docs-gate.sh is missing (fails closed)" "gh pr merge 5 --squash" GH_HEAD="$DOCSHA"
mv "$d/dg.away" "$R/.claude/lib/docs-gate.sh" 2>/dev/null
# Main moves on with code (M1). A rename of that code to Markdown deletes it, so it is not prose;
# nor is a PR that reverts M1's code, though against a stale origin/main (B) it lists only its note:
# the guard fetches before it lists.
B=$(git -C "$R" rev-parse origin/main)
git -C "$R" checkout -q -b m1; printf 'echo\n' > "$R/y.sh"; git -C "$R" add -A && git -C "$R" commit -qm "main's code"
M1=$(git -C "$R" rev-parse HEAD); git -C "$R" push -q origin HEAD:main
git -C "$R" checkout -q -b ren; git -C "$R" mv y.sh y.md && git -C "$R" commit -qm "y.sh to y.md"; RENSHA=$(git -C "$R" rev-parse HEAD)
git -C "$R" checkout -q -b rev "$M1"; git -C "$R" rm -q y.sh; printf '# n\n' > "$R/docs/n.md"; git -C "$R" add -A && git -C "$R" commit -qm "revert, and a note"
REVSHA=$(git -C "$R" rev-parse HEAD); git -C "$R" checkout -q main
printf '%s\tdocs\tclean\tdocs\n' "$RENSHA" > "$R/.git/ci-receipt"
guard 2 "a docs receipt for a PR that renames code to Markdown" "gh pr merge 5 --squash" GH_HEAD="$RENSHA"
grep -q 'y\.sh' "$d/err" && ok "merge-guard: ...naming the code the rename deletes" || fail "merge-guard: rename refusal does not name y.sh: $(head -3 "$d/err")"
git -C "$R" update-ref refs/remotes/origin/main "$B"
printf '%s\tdocs\tclean\tdocs\n' "$REVSHA" > "$R/.git/ci-receipt"
guard 2 "a docs receipt for a PR that reverts main's code, against a stale origin/main" "gh pr merge 5 --squash" GH_HEAD="$REVSHA"
grep -q 'y\.sh' "$d/err" && ok "merge-guard: ...naming the code it reverts (fetched, not the stale ref)" || fail "merge-guard: stale-origin refusal does not name y.sh: $(head -3 "$d/err")"
# ...and when origin cannot be fetched, the stale ref is not trusted: it blocks.
git -C "$R" update-ref refs/remotes/origin/main "$B"; git -C "$R" remote set-url origin acme/gone.git
guard 2 "a docs receipt when origin/main cannot be fetched (fails closed, not on the stale ref)" "gh pr merge 5 --squash" GH_HEAD="$REVSHA"
grep -q 'cannot be fetched' "$d/err" && ok "merge-guard: ...naming the fetch" || fail "merge-guard: unfetchable-origin docs message: $(head -3 "$d/err")"
git -C "$R" remote set-url origin acme/app.git
git -C "$R" push -q -f origin "$B:main"; git -C "$R" fetch -q origin; git -C "$R" branch -q -D m1 ren rev
receipt 0000000000000000000000000000000000000000
guard 2 "a receipt for another commit" "gh pr merge 5 --squash"
rm -f "$R/.git/ci-receipt"
guard 2 "no remote workflow named, so a remote success does not count" "gh pr merge 5 --squash" GH_RUNS=1
echo 'GATE_REMOTE_WORKFLOW="ci.yml"' >> "$R/.claude/project.conf"
guard 0 "the named remote workflow succeeded" "gh pr merge 5 --squash" GH_RUNS=1
guard 2 "the named remote workflow has no success on the head" "gh pr merge 5 --squash" GH_RUNS=0
guard 0 "...a success in the repo -R names, none in gh's default repo" "gh pr merge 5 -R acme/app --squash" GH_RUNS=1 GH_RUNS_DEFAULT=0
guard 2 "...a success only in gh's default repo, for a PR -R names" "gh pr merge 5 -R acme/app --squash" GH_RUNS=0 GH_RUNS_DEFAULT=1
guard 2 "...a success only in gh's default repo, for a PR named by URL" "gh pr merge https://github.com/acme/app/pull/5 --squash" GH_RUNS=0 GH_RUNS_DEFAULT=1

# A receipt written in a linked worktree's git dir counts.
git -C "$R" worktree add -q "$d/lane" 2>/dev/null
printf '%s\tfull\tclean\tall\n' "$MAINSHA" > "$(git -C "$d/lane" rev-parse --absolute-git-dir)/ci-receipt"
guard 0 "a receipt in a linked worktree's git dir" "gh pr merge 5 --squash"
guard 2 "a red check" "gh pr merge 5 --squash" GH_CHECKS='[{"name":"lint","state":"FAILURE","bucket":"fail"}]'
guard 0 "a red DISPLAY context (GATE_DISPLAY_CONTEXTS)" "gh pr merge 5 --squash" GH_CHECKS='[{"name":"ci/local","state":"FAILURE","bucket":"fail"}]'
# ...and display the other way: a GREEN one is not evidence (the ci-status module posts these).
lane_receipt="$(git -C "$d/lane" rev-parse --absolute-git-dir)/ci-receipt"
mv "$lane_receipt" "$d/receipt.keep"
guard 2 "a GREEN display context with no receipt (display is not evidence)" "gh pr merge 5 --squash" GH_CHECKS='[{"name":"ci/local","state":"SUCCESS","bucket":"pass"}]'
# The ci-status module on, GATE_DISPLAY_CONTEXTS left to its module.conf default: both directions
# hold for both contexts its publisher posts, and every other check still gates.
mkdir -p "$R/.claude/modules"; cp -R "$ROOT/.claude/modules/ci-status" "$R/.claude/modules/"
cp "$ROOT/.claude/lib/modules.sh" "$R/.claude/lib/"
cp "$R/.claude/project.conf" "$d/conf.keep"
printf 'MODULES="ci-status"\nGATE_REMOTE_WORKFLOW="ci.yml"\n' > "$R/.claude/project.conf"
guard 2 "ci-status on: green ci/local and ci/github with no receipt" "gh pr merge 5 --squash" GH_CHECKS='[{"name":"ci/local","state":"SUCCESS","bucket":"pass"},{"name":"ci/github","state":"SUCCESS","bucket":"pass"}]'
mv "$d/receipt.keep" "$lane_receipt"
guard 0 "ci-status on: red ci/local and ci/github with a valid receipt" "gh pr merge 5 --squash" GH_CHECKS='[{"name":"ci/local","state":"FAILURE","bucket":"fail"},{"name":"ci/github","state":"FAILURE","bucket":"fail"}]'
guard 2 "ci-status on: a red check that is not a display context" "gh pr merge 5 --squash" GH_CHECKS='[{"name":"ci/local","state":"SUCCESS","bucket":"pass"},{"name":"lint","state":"FAILURE","bucket":"fail"}]'
cp "$d/conf.keep" "$R/.claude/project.conf"; rm -rf "$R/.claude/modules" "$R/.claude/lib/modules.sh"
guard 2 "a red check while gh also writes to stderr (only stdout is the answer)" "gh pr merge 5 --squash" GH_CHECKS='[{"name":"lint","state":"FAILURE","bucket":"fail"}]' GH_CHECKS_NOISE="[git remote -v]"
grep -q "non-passing checks: lint=fail" "$d/err" && ok "...and names the red check" || fail "red-check-with-stderr message: $(cat "$d/err")"

# Rule 2(a2): GATE_PR_CHECKS. Empty (the default), nothing reported is judged as before, whatever
# the workflow files say: no file is read to guess which checks a PR will get.
guard 0 "GATE_PR_CHECKS empty: no reported checks" "gh pr merge 5 --squash" GH_CHECKS='[]'
guard 0 "GATE_PR_CHECKS empty: gh's own 'no checks reported'" "gh pr merge 5 --squash" GH_CHECKS=none
guard 0 "GATE_PR_CHECKS empty: a gh error is left to rule 2(b), as before" "gh pr merge 5 --squash" GH_CHECKS_ERR="HTTP 401: Bad credentials"
mkdir -p "$R/.github/workflows"
probe() {  # probe LABEL WORKFLOW-TEXT: a workflow shape that gets no PR checks must not block
  printf '%b' "$2" > "$R/.github/workflows/ci.yml"
  guard 0 "GATE_PR_CHECKS empty, no checks reported, workflow $1" "gh pr merge 5 --squash" GH_CHECKS='[]'
}
probe "naming pull_request only in a comment" 'on:\n  push:\n    branches: [main]  # pull_request runs elsewhere\n'
probe "naming pull_request only in an if: expression" "on: [push]\njobs:\n  t:\n    if: github.event_name == 'pull_request'\n"
probe "on pull_request types: [closed]" 'on:\n  pull_request:\n    types: [closed]\n'
probe "on pull_request for another base branch" 'on:\n  pull_request:\n    branches: [release]\n'
probe "on pull_request_target" 'on:\n  pull_request_target:\n'
rm -rf "$R/.github"
cp "$R/.claude/project.conf" "$d/conf.ci"
echo 'GATE_PR_CHECKS="test"' >> "$R/.claude/project.conf"
M='[{"name":"test (macos-latest)","state":"SUCCESS","bucket":"pass"},{"name":"test (ubuntu-latest)","state":"SUCCESS","bucket":"pass"}]'
guard 2 "GATE_PR_CHECKS=test: nothing reported yet (CI not registered)" "gh pr merge 5 --squash" GH_CHECKS='[]'
grep -q "has not reported the checks GATE_PR_CHECKS requires: test" "$d/err" && ok "...and says which required check has not reported" || fail "missing-check message: $(cat "$d/err")"
guard 2 "GATE_PR_CHECKS=test: gh's 'no checks reported'" "gh pr merge 5 --squash" GH_CHECKS=none
grep -q "has not reported the checks GATE_PR_CHECKS requires: test" "$d/err" && ! grep -q "gh problem" "$d/err" \
  && ok "...and reads gh's 'no checks reported' as nothing reported, not as a gh failure" || fail "no-checks message: $(cat "$d/err")"
guard 0 "GATE_PR_CHECKS=test: the required check green while gh also writes to stderr" "gh pr merge 5 --squash" GH_CHECKS="[{\"name\":\"test\",\"state\":\"SUCCESS\",\"bucket\":\"pass\"}]" GH_CHECKS_NOISE="[git remote -v]"
guard 2 "GATE_PR_CHECKS=test: only another check reported" "gh pr merge 5 --squash" GH_CHECKS='[{"name":"audit","state":"SUCCESS","bucket":"pass"}]'
guard 2 "GATE_PR_CHECKS=test: a different check that merely starts with 'test' does not count" "gh pr merge 5 --squash" GH_CHECKS='[{"name":"test-flaky","state":"SUCCESS","bucket":"pass"}]'
guard 2 "GATE_PR_CHECKS=test: one matrix job pending" "gh pr merge 5 --squash" GH_CHECKS='[{"name":"test (macos-latest)","state":"PENDING","bucket":"pending"},{"name":"test (ubuntu-latest)","state":"SUCCESS","bucket":"pass"}]'
guard 2 "GATE_PR_CHECKS=test: one matrix job failed" "gh pr merge 5 --squash" GH_CHECKS='[{"name":"test (macos-latest)","state":"FAILURE","bucket":"fail"},{"name":"test (ubuntu-latest)","state":"SUCCESS","bucket":"pass"}]'
guard 2 "GATE_PR_CHECKS=test: a skipped required check (rule 2(a) alone passes it)" "gh pr merge 5 --squash" GH_CHECKS='[{"name":"test","state":"SKIPPED","bucket":"skipping"}]'
grep -q "required checks (GATE_PR_CHECKS) are not green: test=skipping" "$d/err" && ok "...and names the check that is not green" || fail "not-green message: $(cat "$d/err")"
guard 0 "GATE_PR_CHECKS=test: every matrix job green" "gh pr merge 5 --squash" GH_CHECKS="$M"
guard 2 "GATE_PR_CHECKS=test: gh itself fails" "gh pr merge 5 --squash" GH_CHECKS_ERR="HTTP 401: Bad credentials (https://api.github.com/graphql)"
grep -q "cannot read PR #5's checks (gh: HTTP 401" "$d/err" && grep -q "not CI" "$d/err" && ! grep -q "has not reported" "$d/err" \
  && ok "...and says gh failed, not that CI has not registered" || fail "gh-error message: $(cat "$d/err")"
cp "$d/conf.ci" "$R/.claude/project.conf"

# Rule 3: the slice line, read at the head from the PR's own change directory.
printf '%s\tfull\tclean\tall\n' "$NOSLICE" > "$R/.git/ci-receipt"
guard 2 "a change PR whose tasks.md has no Slice line" "gh pr merge 5 --squash" GH_HEAD="$NOSLICE" GH_BRANCH=change/add-x
printf '%s\tfull\tclean\tall\n' "$SLICE" > "$R/.git/ci-receipt"
guard 0 "a change PR that states its slice" "gh pr merge 5 --squash" GH_HEAD="$SLICE" GH_BRANCH=change/add-x

# Rule 7: a journal session number merged elsewhere after this branch was cut.
git -C "$R" checkout -q -b ops/close main
printf '# Journal\n\n## Session 2 — 2026-01-02 — b — mine\n\n## Session 1 — 2026-01-01 — a — start\n' > "$R/docs/development-journal.md"
git -C "$R" commit -qam "close: session 2"
MINE=$(git -C "$R" rev-parse HEAD)
git -C "$R" checkout -q main
printf '# Journal\n\n## Session 2 — 2026-01-02 — c — theirs\n\n## Session 1 — 2026-01-01 — a — start\n' > "$R/docs/development-journal.md"
git -C "$R" commit -qam "their close: session 2"
(cd "$R" && git push -q origin HEAD:main 2>/dev/null && git fetch -q origin)
printf '%s\tfull\tclean\tall\n' "$MINE" > "$R/.git/ci-receipt"
guard 2 "a journal session number another merged session took" "gh pr merge 5 --squash" GH_HEAD="$MINE" GH_BRANCH=ops/session-2-close
grep -q 'claims Session 2, which another merged session already uses' "$d/err" && ok "rule 7: ...and the block is the collision's, naming the number" || fail "rule 7 collision message: $(cat "$d/err")"
receipt "$MAINSHA"

# Rules 9–12: the change process, read at the head (lib/change-guard.sh).
# head_of BRANCH: commit what the caller staged on BRANCH (made from main), echo the sha, back to main.
on() { git -C "$R" checkout -q -B "$1" main; }
head_of() { git -C "$R" add -A && git -C "$R" commit -qm "$1" && h=$(git -C "$R" rev-parse HEAD) && git -C "$R" checkout -q main && printf '%s\n' "$h" > "$d/head"; }
at() { printf '%s\tfull\tclean\tall\n' "$(cat "$d/head")" > "$R/.git/ci-receipt"; }
H() { cat "$d/head"; }

# Rule 7's second half (OPS-108): only a session close numbers a session; a row writes a fragment.
J="$R/docs/development-journal.md"
on ops/ops-1-row
printf '# Journal\n\n## Session 3 — 2026-01-03 — d — a row numbers one\n\n## Session 2 — 2026-01-02 — c — theirs\n\n## Session 1 — 2026-01-01 — a — start\n' > "$J"
head_of "a row writes a session entry"; at; ROW3=$(H)
guard 2 "rule 7: a row's PR (not a session close) that adds a '## Session N'" "gh pr merge 5 --squash" GH_HEAD="$ROW3" GH_BRANCH=ops/ops-1-row
grep -q 'only a session close' "$d/err" && grep -q 'journal.d/<row id>.md' "$d/err" && ok "rule 7: ...and the block names the fragment to write instead" || fail "rule 7 row-heading message: $(cat "$d/err")"
guard 0 "rule 7: the same entry from a session close" "gh pr merge 5 --squash" GH_HEAD="$ROW3" GH_BRANCH=ops/session-3-close
cp "$R/.claude/project.conf" "$d/conf7.bak"; echo 'BRANCH_SESSION_CLOSE=""' >> "$R/.claude/project.conf"
guard 0 "rule 7: BRANCH_SESSION_CLOSE=\"\" turns its second half off, with rule 8" "gh pr merge 5 --squash" GH_HEAD="$ROW3" GH_BRANCH=ops/ops-1-row
cp "$d/conf7.bak" "$R/.claude/project.conf"
on ops/ops-2-row; sed 's/— start$/— start, retitled/' "$J" > "$J.new" && mv "$J.new" "$J"
head_of "a row retitles an old entry"; at
guard 0 "rule 7: a row's PR that retitles an old entry numbers nothing" "gh pr merge 5 --squash" GH_HEAD="$(H)" GH_BRANCH=ops/ops-2-row
on ops/ops-3-row; mkdir -p "$R/docs/journal.d"; printf '# OPS-3 — a row\n' > "$R/docs/journal.d/OPS-3.md"
head_of "a row writes its fragment"; at
guard 0 "rule 7: a row's PR that writes only its fragment" "gh pr merge 5 --squash" GH_HEAD="$(H)" GH_BRANCH=ops/ops-3-row
receipt "$MAINSHA"

# Rule 2(c) and rule 8: a head behind origin/main. 2(c) advises on any PR; rule 8 BLOCKS a session
# close (BRANCH_SESSION_CLOSE) until its head contains origin/main. Ported from the meta-tests of
# the project rule 8 was upstreamed from (P2.5), with real git: the branch is cut, then main moves.
seen() { printf '%s' "$out" | jq -r '.hookSpecificOutput.additionalContext // empty' 2>/dev/null; }
on ops/session-9-close; echo "close" > "$R/close.txt"; head_of "the close's records"; at; CUT=$(H)
guard 0 "rule 8: a session close whose head contains origin/main" "gh pr merge 5 --squash" GH_HEAD="$CUT" GH_BRANCH=ops/session-9-close
case "$(seen)" in *"rule 8"*"contains origin/main"*) ok "rule 8: ...and says so, naming the main it contains" ;; *) fail "rule 8 advisory on a current close: $(seen)" ;; esac
case "$(seen)" in *"behind origin/main"*) fail "rule 2(c) warned on a head that IS origin/main plus its own commit: $(seen)" ;; *) ok "rule 2(c): silent when the head contains origin/main" ;; esac
for i in 1 2; do echo "$i" > "$R/other$i.txt"; git -C "$R" add -A; git -C "$R" commit -qm "another PR lands $i"; done
(cd "$R" && git push -q origin HEAD:main 2>/dev/null && git fetch -q origin)
guard 2 "rule 8: a session close whose head does not contain origin/main" "gh pr merge 5 --squash" GH_HEAD="$CUT" GH_BRANCH=ops/session-9-close
grep -q 'does not contain origin/main' "$d/err" && grep -q 'git merge origin/main' "$d/err" && ok "rule 8: ...and the block says why and what to run" || fail "rule 8 block message: $(cat "$d/err")"
for b in ops/session-93-close-addendum ops/session-94-close-2; do
  guard 2 "rule 8: the close variant $b, behind" "gh pr merge 5 --squash" GH_HEAD="$CUT" GH_BRANCH="$b"
done
for b in ops/session-start-x ops/session-plan-gate-fixes ops/session-71-cleanup fix/1-x; do
  guard 0 "rule 8: $b is not a session close, so behind is advice, not a block" "gh pr merge 5 --squash" GH_HEAD="$CUT" GH_BRANCH="$b"
  case "$(seen)" in *"rule 8"*) fail "rule 8 ran on $b: $(seen)" ;; esac
done
case "$(seen)" in *"2 commit(s) behind origin/main"*"git merge origin/main"*) ok "rule 2(c): a behind head WARNS, naming the count and the merge, in what Claude reads" ;; *) fail "rule 2(c) advisory: $(seen)" ;; esac
cp "$R/.claude/project.conf" "$d/conf8.bak"
echo 'BRANCH_SESSION_CLOSE=""' >> "$R/.claude/project.conf"
guard 0 "rule 8: BRANCH_SESSION_CLOSE=\"\" turns it off" "gh pr merge 5 --squash" GH_HEAD="$CUT" GH_BRANCH=ops/session-9-close
cp "$d/conf8.bak" "$R/.claude/project.conf"; echo 'BRANCH_SESSION_CLOSE="ops/close-*"' >> "$R/.claude/project.conf"
guard 2 "rule 8: a project's own close pattern is the one enforced" "gh pr merge 5 --squash" GH_HEAD="$CUT" GH_BRANCH=ops/close-12
guard 0 "rule 8: ...and the default pattern then no longer applies" "gh pr merge 5 --squash" GH_HEAD="$CUT" GH_BRANCH=ops/session-9-close
cp "$d/conf8.bak" "$R/.claude/project.conf"
git -C "$R" checkout -q ops/session-9-close && git -C "$R" merge -q --no-edit origin/main && MERGED=$(git -C "$R" rev-parse HEAD) && git -C "$R" checkout -q main
receipt "$MERGED"
guard 0 "rule 8: the same close, once it merges origin/main" "gh pr merge 5 --squash" GH_HEAD="$MERGED" GH_BRANCH=ops/session-9-close
case "$(seen)" in *"rule 8"*) ok "rule 8: ...and says rule 8 ran" ;; *) fail "rule 8 silent on the merged close: $(seen)" ;; esac
on ops/session-9-close; echo "unfetched" > "$R/close.txt"; head_of "a head nobody fetched"; GONE=$(H); git -C "$R" branch -D -q ops/session-9-close
printf '%s\tfull\tclean\tall\n' "$GONE" > "$R/.git/ci-receipt"
# No origin/main, and none to fetch: the remote is unreachable (the guard's fetches repeat, so a
# deleted ref alone would come straight back).
git -C "$R" update-ref -d refs/remotes/origin/main; git -C "$R" remote set-url origin acme/gone.git
guard 2 "rule 8: origin/main unreadable (fails closed, not open)" "gh pr merge 5 --squash" GH_HEAD="$GONE" GH_BRANCH=ops/session-9-close
grep -q 'cannot read origin/main, so rule 8' "$d/err" && ok "rule 8: ...and the block is rule 8's, naming the fetch to run" || fail "rule 8 unreadable-main message: $(cat "$d/err")"
git -C "$R" remote set-url origin acme/app.git; (cd "$R" && git fetch -q origin)
receipt "$MAINSHA"

# Rule 8's second half (OPS-108): any PR that changes JOURNAL or INSIGHTS lands on top of
# origin/main, whatever its branch, so a close that rides a row's PR is held as a close is.
land() { echo "$1" > "$R/land.txt"; git -C "$R" add -A; git -C "$R" commit -qm "another PR lands: $1"; (cd "$R" && git push -q origin HEAD:main 2>/dev/null && git fetch -q origin); }
on ops/ops-4-row; printf '# Insights\n\n| a row adds a line |\n' > "$R/docs/insights-log.md"; head_of "a row writes the insights log"; at; INS=$(H)
land ins
guard 2 "rule 8: a row's PR that changes the insights log, behind origin/main" "gh pr merge 5 --squash" GH_HEAD="$INS" GH_BRANCH=ops/ops-4-row
grep -q 'changes docs/insights-log.md' "$d/err" && grep -q 'does not contain origin/main' "$d/err" && ok "rule 8: ...and the block names the record it changes" || fail "rule 8 record message: $(cat "$d/err")"
on ops/ops-5-row; printf '\nmore\n' >> "$J"; head_of "a row edits the journal's prose"; at; JRN=$(H)
land jrn
guard 2 "rule 8: a row's PR that changes the journal, behind origin/main" "gh pr merge 5 --squash" GH_HEAD="$JRN" GH_BRANCH=ops/ops-5-row
git -C "$R" checkout -q ops/ops-4-row && git -C "$R" merge -q --no-edit origin/main && INSM=$(git -C "$R" rev-parse HEAD) && git -C "$R" checkout -q main
receipt "$INSM"
guard 0 "rule 8: the same row's PR, once it merges origin/main" "gh pr merge 5 --squash" GH_HEAD="$INSM" GH_BRANCH=ops/ops-4-row
case "$(seen)" in *"rule 8"*"changes docs/insights-log.md"*"contains origin/main"*) ok "rule 8: ...and says rule 8 ran, and why" ;; *) fail "rule 8 silent on the merged row: $(seen)" ;; esac
on ops/ops-6-row; mkdir -p "$R/docs/journal.d"; printf '# OPS-6 — a row\n' > "$R/docs/journal.d/OPS-6.md"; head_of "a row writes only its fragment"; at; FRG=$(H)
land frg
guard 0 "rule 8: a row's PR that writes only its fragment, behind origin/main (advice, not a block)" "gh pr merge 5 --squash" GH_HEAD="$FRG" GH_BRANCH=ops/ops-6-row
case "$(seen)" in *"rule 8"*) fail "rule 8 ran on a fragment-only row: $(seen)" ;; *) ok "rule 8: silent on a fragment-only row" ;; esac
# The journal edit is main's, carried in by a merge: the PR itself changes neither record.
printf '\nmain edits its journal\n' >> "$J"; git -C "$R" commit -qam "a close lands"; (cd "$R" && git push -q origin HEAD:main 2>/dev/null && git fetch -q origin)
git -C "$R" checkout -q ops/ops-6-row && git -C "$R" merge -q --no-edit origin/main && CARRY=$(git -C "$R" rev-parse HEAD) && git -C "$R" checkout -q main
land carry; receipt "$CARRY"
guard 0 "rule 8: a row's PR whose only journal change is main's, merged in" "gh pr merge 5 --squash" GH_HEAD="$CARRY" GH_BRANCH=ops/ops-6-row
case "$(seen)" in *"rule 8"*) fail "rule 8 counted main's journal edit as the row's: $(seen)" ;; esac
receipt "$MAINSHA"

on change/add-y; mkdir -p "$R/changes/add-y" "$R/src"
printf '## 1. Do it\n- [x] 1.1 thing\n- [ ] 1.2 other\n\n- [ ] Slice: a user can y at /y\n' > "$R/changes/add-y/tasks.md"
echo 'code' > "$R/src/y.txt"; head_of "build with an open task"; at
guard 2 "rule 9: a build PR whose change still has an open task" "gh pr merge 5 --squash" GH_HEAD="$(H)" GH_BRANCH=change/add-y
on change/add-y; mkdir -p "$R/changes/add-y"
printf '## 1. Do it\n- [ ] 1.1 thing\n- [ ] 1.2 other\n\n- [ ] Slice: a user can y at /y\n' > "$R/changes/add-y/tasks.md"
printf '# Roadmap\n' > "$R/docs/roadmap.md"
head_of "the proposal, and its roadmap note"; at
guard 0 "rule 9: a proposal PR (the change's own files and the roadmap, every task open)" "gh pr merge 5 --squash" GH_HEAD="$(H)" GH_BRANCH=change/add-y
on change/add-y; mkdir -p "$R/changes/add-y"
printf '## 1. Do it\n- [ ] 1.1 thing\n\n- [ ] Slice: a user can y at /y\n' > "$R/changes/add-y/tasks.md"
printf '# Guide\n' > "$R/docs/guide.md"
head_of "a docs-only build"; at
guard 2 "rule 9: a build that only edits docs (not the roadmap) is still a build" "gh pr merge 5 --squash" GH_HEAD="$(H)" GH_BRANCH=change/add-y
on change/add-y; mkdir -p "$R/changes/add-y" "$R/src"
printf '## 1. Do it\n- [x] 1.1 thing\n- [x] 1.2 other\n\n- [ ] Slice: a user can y at /y\n' > "$R/changes/add-y/tasks.md"
echo 'code' > "$R/src/y.txt"; head_of "build, every task done"; at
guard 0 "rule 9: a build PR whose every task is ticked" "gh pr merge 5 --squash" GH_HEAD="$(H)" GH_BRANCH=change/add-y
# CHANGE_RECORD_EXTRA: the project's own required sections, read at the head.
cp "$R/.claude/project.conf" "$d/conf.bak"; echo 'CHANGE_RECORD_EXTRA="tasks.md:Rollback"' >> "$R/.claude/project.conf"
guard 2 "rule 9: a build PR whose change lacks a CHANGE_RECORD_EXTRA section" "gh pr merge 5 --squash" GH_HEAD="$(H)" GH_BRANCH=change/add-y
grep -q "changes/add-y: tasks.md has no '## Rollback' section" "$d/err" && ok "rule 9: ...and names the missing section" || fail "rule 9 CHANGE_RECORD_EXTRA message: $(cat "$d/err")"
on change/add-y; mkdir -p "$R/changes/add-y" "$R/src"
printf '## 1. Do it\n- [x] 1.1 thing\n\n## Rollback\nRevert.\n\n- [ ] Slice: a user can y at /y\n' > "$R/changes/add-y/tasks.md"
echo 'code' > "$R/src/y.txt"; head_of "build, with its rollback"; at
guard 0 "rule 9: a build PR carrying every CHANGE_RECORD_EXTRA section" "gh pr merge 5 --squash" GH_HEAD="$(H)" GH_BRANCH=change/add-y
cp "$d/conf.bak" "$R/.claude/project.conf"

on ops/spec; mkdir -p "$R/specs/cap" "$R/tests"
printf '# Cap\n\n## Purpose\nx\n\n## Requirements\n\n### Requirement: R\nThe system SHALL r.\n\n#### Scenario: [CAP-1-S1] r\n- **THEN** r\n' > "$R/specs/cap/spec.md"
head_of "a living scenario no test cites"; at
guard 2 "rule 10: a living scenario no test cites at the head" "gh pr merge 5 --squash" GH_HEAD="$(H)" GH_BRANCH=ops/spec
on ops/spec; mkdir -p "$R/specs/cap" "$R/tests"
printf '# Cap\n\n## Purpose\nx\n\n## Requirements\n\n### Requirement: R\nThe system SHALL r.\n\n#### Scenario: [CAP-1-S1] r\n- **THEN** r\n' > "$R/specs/cap/spec.md"
echo '# CAP-1-S1' > "$R/tests/cap.sh"; head_of "a living scenario a test cites"; at
guard 0 "rule 10: a living scenario a test cites at the head" "gh pr merge 5 --squash" GH_HEAD="$(H)" GH_BRANCH=ops/spec

A=changes/archive/2026-01-05-add-z
archive() {  # archive TASKS [SPEC|skip|none] [KNOW] [ROW]
  on ops/archive; mkdir -p "$R/$A"
  printf '%b' "$1" > "$R/$A/tasks.md"
  printf '# p\n' > "$R/$A/proposal.md"
  case "$2" in spec) mkdir -p "$R/$A/specs/z"; printf '## ADDED Requirements\n' > "$R/$A/specs/z/spec.md" ;; skip) printf 'skip_specs: true\n' > "$R/$A/.openspec.yaml" ;; esac
  [ -n "${3:-}" ] && printf "## How we'll know\n- **Signal:** s\n" >> "$R/$A/proposal.md"
  [ -n "${4:-}" ] && printf '| o.1 | `ops/check-outcome-add-z` — check the outcome of add-z (due 2026-02-05) | s | — | ⬜ queued |\n' > "$R/docs/roadmap.md"
  head_of "archive add-z"; at
}
DONE='- [x] 1.1 a\n\n## Progress\n- 2026-01-05 archived: actual cost $3.10 of budget $15\n'
archive "$DONE" spec
guard 0 "rule 11: archiving a finished change (tasks ticked, a delta, its cost recorded)" "gh pr merge 5 --squash" GH_HEAD="$(H)" GH_BRANCH=ops/archive
archive '- [ ] 1.1 a\n- actual cost unknown (built on another machine)\n' spec
guard 2 "rule 11: archiving a change with an open task" "gh pr merge 5 --squash" GH_HEAD="$(H)" GH_BRANCH=ops/archive
archive '- [x] 1.1 a\n' spec
guard 2 "rule 11: archiving a change with no recorded actual cost" "gh pr merge 5 --squash" GH_HEAD="$(H)" GH_BRANCH=ops/archive
archive "$DONE" none
guard 2 "rule 11: archiving a change with no spec delta and no skip_specs" "gh pr merge 5 --squash" GH_HEAD="$(H)" GH_BRANCH=ops/archive
archive "$DONE" skip
guard 0 "rule 11: archiving a no-delta change that declares skip_specs: true" "gh pr merge 5 --squash" GH_HEAD="$(H)" GH_BRANCH=ops/archive
archive "$DONE" spec know
guard 2 "rule 11: archiving a change that says how we'll know, with no outcome check queued" "gh pr merge 5 --squash" GH_HEAD="$(H)" GH_BRANCH=ops/archive
archive "$DONE" spec know row
guard 0 "rule 11: ...and with its outcome check queued in the roadmap" "gh pr merge 5 --squash" GH_HEAD="$(H)" GH_BRANCH=ops/archive
# An archive made the way archive-change makes it: the open change lands on main, then a PR moves it
# with `git mv` and appends its cost. git diff reports each moved file as renamed (R), not added, so
# a reader of added paths alone checked nothing on a real archive (OPS-70: #129 and #133).
mv_archive() {  # mv_archive ID TASKS
  git -C "$R" checkout -q main; mkdir -p "$R/changes/$1/specs/$1"
  printf '%b' "$2" > "$R/changes/$1/tasks.md"
  printf '# p\n' > "$R/changes/$1/proposal.md"
  printf '## ADDED Requirements\n' > "$R/changes/$1/specs/$1/spec.md"
  git -C "$R" add -A && git -C "$R" commit -qm "propose $1"
  (cd "$R" && git push -q origin HEAD:main 2>/dev/null && git fetch -q origin)
  on "ops/archive-$1"; mkdir -p "$R/changes/archive"
  git -C "$R" mv "changes/$1" "changes/archive/2026-01-06-$1"
  printf -- '- 2026-01-06 archived: actual cost $2.40 of budget $15\n' >> "$R/changes/archive/2026-01-06-$1/tasks.md"
  head_of "archive $1 with git mv"; at
  # The case proves nothing unless git really reports the move as a rename. -M: porcelain diff
  # honours the user's diff.renames, and with it false no move is ever a rename.
  git -C "$R" diff -M --name-status "$(git -C "$R" merge-base origin/main "$(H)")" "$(H)" | grep -q "^R[0-9]*	changes/$1/tasks.md	changes/archive/2026-01-06-$1/tasks.md$" \
    && ok "rule 11 fixture: git reports the git-mv archive of $1 as a rename" || fail "rule 11 fixture: the archive of $1 is not a rename: $(git -C "$R" diff -M --name-status origin/main "$(H)")"
}
MVT='## 1. Do it\n- [x] 1.1 the first task of the change\n- [x] 1.2 the second task of the change\n- [x] 1.3 the third task of the change\n'
mv_archive add-mv-open "$MVT- [ ] 1.4 the fourth task, still open\n\n## Progress\n"
guard 2 "rule 11: a change archived with git mv (a rename) and an open task" "gh pr merge 5 --squash" GH_HEAD="$(H)" GH_BRANCH=ops/archive-add-mv-open
grep -q "changes/archive/2026-01-06-add-mv-open is archived with 1 open task" "$d/err" && ok "rule 11: ...and names the renamed archive and its open task" || fail "rule 11 git-mv message: $(cat "$d/err")"
mv_archive add-mv-done "$MVT- [x] 1.4 the fourth task, done\n\n## Progress\n"
guard 0 "rule 11: a finished change archived with git mv (a rename)" "gh pr merge 5 --squash" GH_HEAD="$(H)" GH_BRANCH=ops/archive-add-mv-done
# Rule 9 and a rename: a build that `git mv`s a file into its change directory. With rename
# detection, --name-only lists only the new path (a record), and the build read as a proposal.
git -C "$R" checkout -q main; mkdir -p "$R/src"; printf 'moved by the build\n' > "$R/src/z.txt"
git -C "$R" add -A && git -C "$R" commit -qm "src/z.txt" && (cd "$R" && git push -q origin HEAD:main 2>/dev/null && git fetch -q origin)
on change/add-z; mkdir -p "$R/changes/add-z"
printf '## 1. Do it\n- [ ] 1.1 thing\n\n- [ ] Slice: a user can z at /z\n' > "$R/changes/add-z/tasks.md"
git -C "$R" mv src/z.txt changes/add-z/z.txt; head_of "a build whose only other edit is a git mv into its record"; at
guard 2 "rule 9: a build whose only non-record edit is a git mv into changes/<id>/ (a rename)" "gh pr merge 5 --squash" GH_HEAD="$(H)" GH_BRANCH=change/add-z

# CHANGES_LEGACY (lib/records.sh): a grandfathered change keeps the format it was approved in, so
# rules 9–11 do not ask it for the records this format added; finishing is still asked.
git -C "$R" checkout -q main
cp "$ROOT/.claude/lib/records.sh" "$R/.claude/lib/"
git -C "$R" add -A && git -C "$R" commit -qm "records library"
cp "$R/.claude/project.conf" "$d/conf.bak"
legacy_on() { cp "$d/conf.bak" "$R/.claude/project.conf"; printf '%s\n' "$@" >> "$R/.claude/project.conf"; }
archive '- [x] 1.1 a\n' none
guard 2 "CHANGES_LEGACY: an archive with no cost and no delta, not grandfathered" "gh pr merge 5 --squash" GH_HEAD="$(H)" GH_BRANCH=ops/archive
legacy_on 'CHANGES_LEGACY="add-z"'
guard 0 "rule 11: the same archive of a grandfathered change (CHANGES_LEGACY by name)" "gh pr merge 5 --squash" GH_HEAD="$(H)" GH_BRANCH=ops/archive
archive '- [ ] 1.1 a\n' none
guard 2 "rule 11: a grandfathered change is still archived only when finished" "gh pr merge 5 --squash" GH_HEAD="$(H)" GH_BRANCH=ops/archive
mv "$R/.claude/lib/records.sh" "$d/records.bak"
archive '- [x] 1.1 a\n' none
guard 2 "CHANGES_LEGACY set with lib/records.sh missing (fails closed, not open)" "gh pr merge 5 --squash" GH_HEAD="$(H)" GH_BRANCH=ops/archive
mv "$d/records.bak" "$R/.claude/lib/records.sh"
# Rule 10: a finished open change whose scenario no test cites; grandfathered, it is not enforced.
on ops/legacy; mkdir -p "$R/changes/add-old/specs/old"
printf '## 1. Old\n- [x] 1.1 done\n' > "$R/changes/add-old/tasks.md"
printf '## ADDED Requirements\n\n### Requirement: O\nThe system SHALL o.\n\n#### Scenario: [OLD-1-S1] o\n- **THEN** o\n' > "$R/changes/add-old/specs/old/spec.md"
head_of "a finished change, uncited"; at
legacy_on
guard 2 "rule 10: a finished open change with an uncited scenario" "gh pr merge 5 --squash" GH_HEAD="$(H)" GH_BRANCH=ops/legacy
legacy_on 'CHANGES_LEGACY="add-old"'
guard 0 "rule 10: the same change grandfathered (its scenarios not enforced until archived)" "gh pr merge 5 --squash" GH_HEAD="$(H)" GH_BRANCH=ops/legacy
# Rule 9: CHANGE_RECORD_EXTRA is not asked of a grandfathered change.
on change/add-old; mkdir -p "$R/changes/add-old" "$R/src"
printf '## 1. Old\n- [x] 1.1 done\n\n- [ ] Slice: exempt — legacy\n' > "$R/changes/add-old/tasks.md"
echo 'code' > "$R/src/old.txt"; head_of "build of a grandfathered change"; at
legacy_on 'CHANGE_RECORD_EXTRA="tasks.md:Rollback"'
guard 2 "rule 9: a build lacking a CHANGE_RECORD_EXTRA section, not grandfathered" "gh pr merge 5 --squash" GH_HEAD="$(H)" GH_BRANCH=change/add-old
legacy_on 'CHANGE_RECORD_EXTRA="tasks.md:Rollback"' 'CHANGES_LEGACY="add-old"'
guard 0 "rule 9: the same build of a grandfathered change" "gh pr merge 5 --squash" GH_HEAD="$(H)" GH_BRANCH=change/add-old
cp "$d/conf.bak" "$R/.claude/project.conf"
git -C "$R" checkout -q main; receipt "$MAINSHA"

# Rule 12: provenance trailers, while model-roles.json enables them.
# The template ships it on; an installed project (it has an install or plugin marker) may turn it
# off, as clauductor's own repo does, so the fixture forces it on to test the rule either way.
if [ -f "$ROOT/.claude/clauductor-template" ] || [ -f "$ROOT/.claude/clauductor-plugin" ]; then
  ok "this project sets provenance.enabled to $(jq -r .provenance.enabled "$ROOT/.claude/model-roles.json"); rule 12 is tested with it on"
else
  jq -e '.provenance.enabled == true' "$ROOT/.claude/model-roles.json" >/dev/null && ok "the template turns provenance on by default" || fail "model-roles.json provenance.enabled is not true in the template"
fi
jq '.provenance.enabled = true' "$ROOT/.claude/model-roles.json" > "$R/.claude/model-roles.json"
sguard() {  # sguard WANT LABEL COMMAND — with a session id in the payload, as Claude Code sends
  rc=0
  jq -cn --arg c "$3" --arg d "$R" '{tool_name:"Bash", cwd:$d, session_id:"sess-1", tool_input:{command:$c}}' \
    | (cd "$R" && env PATH="$d/bin:$PATH" GH_HEAD="$MAINSHA" sh "$R/.claude/hooks/pr-merge-guard.sh" 2>"$d/err") >/dev/null || rc=$?
  expect_rc "$1" "$rc" "merge-guard $( [ "$1" = 2 ] && echo blocks || echo allows ): $2"
  [ "$rc" = "$1" ] || sed 's/^/       /' "$d/err" | head -4
}
trail() { printf 'Adds x.\n\nChange: %s\nAgent-Role: %s\nModel: opus\nSession: %s\n' "${1:-add-x}" "${2:-builder}" "${3:-sess-1}"; }
trail > "$R/body.txt"
sguard 0 "rule 12: a --body-file ending in all four trailers" "gh pr merge 5 --squash --delete-branch --body-file body.txt"
sguard 2 "rule 12: a squash with no --body at all" "gh pr merge 5 --squash --delete-branch"
grep -q 'Session: sess-1' "$d/err" && ok "...and the block names the trailers to add, this session's id included" || fail "block message: $(cat "$d/err")"
sguard 0 "rule 12: a quoted --body with the four trailers" "gh pr merge 5 --squash --body \"$(trail)\""
sguard 2 "rule 12: a body missing the Session trailer" "gh pr merge 5 --squash --body \"$(trail | grep -v '^Session')\""
sguard 2 "rule 12: a Session trailer naming another session" "gh pr merge 5 --squash --body \"$(trail add-x builder sess-9)\""
sguard 2 "rule 12: an Agent-Role that is not a role" "gh pr merge 5 --squash --body \"$(trail add-x wizard)\""
sguard 2 "rule 12: a body built by a substitution (unreadable)" 'gh pr merge 5 --squash --body "$(cat body.txt)"'
# The glued short forms gh accepts carry a body too (change-guard.sh cg_field), and are held to it.
sguard 0 "rule 12: a glued -F<file> ending in all four trailers" "gh pr merge 5 --squash -Fbody.txt"
sguard 0 "rule 12: a glued -b<body> with all four trailers" "gh pr merge 5 --squash -b\"$(trail)\""
sguard 2 "rule 12: a glued -b<body> missing the Session trailer" "gh pr merge 5 --squash -b\"$(trail | grep -v '^Session')\""
# A value that starts with - is text: a --subject reading "-b<trailers>" sets no body.
sguard 2 "rule 12: a --subject whose text starts with -b is not a body" "gh pr merge 5 --squash --subject \"-b$(trail)\""
grep -q 'sets no --body' "$d/err" && ok "rule 12: ...it is read as no --body at all" || fail "rule 12: ...it is read as no --body at all: $(cat "$d/err")"
trail > "$R/-"   # a file named "-" with good trailers: stdin must not be read as that file
sguard 2 "rule 12: a --body-file read from stdin (unreadable)" "gh pr merge 5 --squash --body-file -"
rm -f "$R/-"
jq '.provenance.enabled = false' "$R/.claude/model-roles.json" > "$d/mr" && cp "$d/mr" "$R/.claude/model-roles.json"
sguard 0 "rule 12 is off when provenance.enabled is false" "gh pr merge 5 --squash --delete-branch"
rm -f "$R/.claude/model-roles.json" "$R/body.txt"
cp "$R/.claude/hooks/lib/change-guard.sh" "$d/cg.bak"; printf 'broken() { "\n' >> "$R/.claude/hooks/lib/change-guard.sh"
guard 2 "a change-guard library that does not parse (fails closed, not open)" "gh pr merge 5 --squash"
cp "$d/cg.bak" "$R/.claude/hooks/lib/change-guard.sh"

# Reading the command.
guard 0 "a commit message that mentions a merge (prose)" 'git commit -m "then gh pr merge 5 --auto"'
guard 2 "a quoted gh (cannot read the site)" '"gh" pr merge 5 --squash'
guard 2 "a computed word" 'gh pr $(echo merge) 5'
guard 2 "no PR named" "gh pr merge --squash"
# OPS-66 (#120): commands that cannot reach a merge were refused as merges the reader could not
# read, so agents routed around the guard. A redirect target, a computed path that is only an
# argument, and a computed word inside git were each read as a gh, pr or merge that could run.
# Each must pass; a gh word or a computed command word that can reach a merge must not.
# No evidence anywhere, the lane's receipt included; both come back after.
mv "$R/.git/ci-receipt" "$d/receipt.ops66" 2>/dev/null || :
mv "$lane_receipt" "$d/receipt.ops66lane" 2>/dev/null || :
guard 0 "OPS-66: git -C a literal path, then a quoted --work-tree, merging main" 'git -C /some/path --work-tree "$WT" merge origin/main'
guard 0 "OPS-66: git --no-pager -C a quoted variable, merging main" 'git --no-pager -C "$WT" merge origin/main'
guard 0 "OPS-66: git -C a quoted variable, merging main in a worktree" 'git -C "$WT" merge origin/main'
guard 0 "OPS-66: the same inside a loop over worktrees" 'for w in a b; do git -C "$w" merge --no-edit origin/main; done'
guard 0 "OPS-66: a copy of merge-guard.sh to a computed path" 'cp checks/merge-guard.sh backup/$f'
guard 0 "OPS-66: a loop comparing files named merge-guard.sh" 'for f in hooks/pr-merge-guard.sh checks/merge-guard.sh; do cmp -s upstream/$f local/$f || echo $f; done'
guard 0 "OPS-66: a loop copying files named merge-guard.sh" 'for f in merge-guard.sh hooks.sh; do cp upstream/checks/$f local/checks/$f; done'
guard 0 "OPS-66: a head into an unquoted computed scratch path" 'head -n 40 merge-guard.sh > $TMPDIR/$name'
guard 0 "OPS-66: a sed into a scratch copy named by a variable" "sed 's/a/b/' merge-guard.sh > \"\$TMPDIR/scratch/\$name\""
# With evidence for PR 5's head, so that a block below is the reader refusing, not rule 2.
receipt "$MAINSHA"
unread() {  # unread LABEL COMMAND: refused as a merge the reader cannot read, or prose that may run
  guard 2 "OPS-66: $1" "$2"
  [ "$rc" != 2 ] || grep -qE 'may merge in a form this guard cannot read|inside text, beside' "$d/err" \
    || fail "OPS-66: $1: blocked, but not by the reader: $(head -n 1 "$d/err")"
}
unread "gh pr merge held in a variable" 'X="gh pr merge 5 --squash"; $X'
unread "gh pr merge held in an alias" 'alias m="gh pr merge"; m 5 --squash'
unread "gh held in a variable" 'G=gh; $G pr merge 5 --squash'
unread "every word but merge computed" '$A $B merge 5'
unread "a computed path to gh run by find" 'find . -exec bin/$G pr merge 5 \;'
unread "a computed path to gh run by xargs, pr after it" 'echo merge | xargs -I X bin/$G pr X 5'
unread "a computed path to gh run by xargs, merge after it" 'echo pr | xargs -I X bin/$G X merge 5'
unread "a computed gh with its output redirected" '$G pr merge 5 > "$TMPDIR/$x"'
unread "git with an unquoted expansion that can split into -c alias...=!gh" 'git -C $X merge 5'
unread "git with a computed -c (an alias defined in a variable)" 'git -c "$A" "$P" merge 5'
# A redirection anywhere before the last word of a command naming gh, pr and merge is unreadable,
# whatever its operator: the shell runs `gh pr merge 5` for each of these, and each operator form
# below was once read as no merge at all and ALLOWED. Put the redirection last.
unread "a redirection inside the merge: >& and a file" 'gh pr >& x merge 5'
unread "a redirection inside the merge: >&file" 'gh pr >&x merge 5'
unread "a redirection inside the merge: 1>& and a file" 'gh pr 1>& x merge 5'
unread "a redirection inside the merge: zsh >!" 'gh pr >! x merge 5'
unread "a redirection inside the merge: zsh >>!" 'gh pr >>! x merge 5'
unread "a redirection inside the merge: zsh &>!" 'gh pr &>! x merge 5'
unread "a redirection inside the merge: zsh >&!" 'gh pr >&! x merge 5'
unread "a redirection inside the merge: bash >! (! is the file)" 'gh pr >! merge 5'
unread "a redirection inside the merge: a named fd" 'gh pr {fd}>x merge 5'
unread "a redirection inside the merge: a named fd duplicated" 'gh pr {fd}<&0 merge 5'
unread "a redirection inside the merge: 2>/dev/null" 'gh pr 2>/dev/null merge 5'
# A regression guard, not a failing-first case: main's reader already refused this one.
unread "a redirection inside the merge: a named fd between gh and pr" 'gh {fd}>x pr merge 5'
# Widened to ANY gh (OPS-66, round 3): `gh api` merges too, and names no pr or merge word.
unread "a redirection inside a gh command: >& before api's merge endpoint" 'gh >& x api -X PUT repos/acme/app/pulls/5/merge'
unread "a redirection inside a gh command: a named fd before api's merge endpoint" 'gh {fd}>x api -X PUT repos/acme/app/pulls/5/merge'
unread "a redirection inside a gh command: 2>/dev/null before api graphql" 'gh 2>/dev/null api graphql -f query=x'
# The accepted cost: a gh command that merges nothing is refused too. Put the redirection last.
unread "a redirection inside a gh command: a list that merges nothing" 'gh pr list 2>/dev/null --json number'
# A regression guard: refused since round 2 (pr and merge are words), so it did not fail first.
unread "a redirection inside a gh command: >x between gh and pr" 'gh >x pr merge 5'
# A `(` inside a word or after `<`/`>` is part of that word (OPS-66, round 5): it once started a new
# command, so gh api lost its endpoint and each of these merged, allowed with no evidence.
unread "a process substitution before api's merge endpoint" "gh api -X PUT --input <(echo '{}') repos/acme/app/pulls/5/merge"
unread "a process substitution glued to --input= before the endpoint" 'gh api -XPUT --input=<(true) repos/acme/app/pulls/5/merge'
unread "a process substitution, --method PUT and a leading /" 'gh api --method PUT --input <(true) /repos/acme/app/pulls/5/merge'
unread "a process substitution before a quoted {owner}/{repo} endpoint" 'gh api -X PUT --input <(true) "repos/{owner}/{repo}/pulls/5/merge"'
unread "an output process substitution before the endpoint" 'gh api -X PUT --input >(cat) repos/acme/app/pulls/5/merge'
unread "a zsh glob qualifier before the endpoint" 'gh api x(N) -X PUT repos/acme/app/pulls/5/merge'
# inside() judges the command word (OPS-66, round 5). Regression guards: each was refused before the
# narrowing, and must stay refused after it, because gh still runs.
unread "a redirection first, then gh api's merge" '>& x gh api -X PUT repos/acme/app/pulls/5/merge'
unread "zsh >! first (bash: the file !), then gh api's merge" '>! x gh api -X PUT repos/acme/app/pulls/5/merge'
unread "a named fd first, then gh api's merge" '{fd}>x gh api -X PUT repos/acme/app/pulls/5/merge'
unread "env runs gh, a redirection inside" 'env >x gh api -X PUT repos/acme/app/pulls/5/merge'
unread "a computed command word, a redirection inside" '$G >x api -X PUT repos/acme/app/pulls/5/merge'
guard 0 "OPS-66: a merge with its redirection last (2>&1), with evidence" 'gh pr merge 5 --squash --delete-branch 2>&1'
# The command word decides what runs: a redirection inside a command whose literal command word is
# not gh and runs no argument is read as before (OPS-66, round 5; each was refused at round 4).
guard 0 "OPS-66: echo with >&2 before a computed argument" 'echo >&2 "$msg"'
guard 0 "OPS-66: jq with its output redirected before its input file" 'jq . > "$out" in.json'
guard 0 "OPS-66: sed with 2>/dev/null before a computed file" 'sed -n 1p 2>/dev/null "$f"'
guard 0 "OPS-66: a loop comparing files, its output redirected in the middle" 'for f in a b; do cmp "$f" > /dev/null "other/$f"; done'
# A name defined in the command runs what the reader cannot see at the name's use: an alias, a
# hash entry or a function makes the whole command unreadable, whatever it runs (OPS-66). With
# evidence for PR 5's head, so a block here is the reader's.
unread "an alias to gh, then gh api's merge through it" "$(printf 'alias jq=gh\njq api -X PUT repos/acme/app/pulls/5/merge')"
unread "a function that runs a computed gh, called with api's merge" 'f() { $G "$@"; }; f api -X PUT repos/acme/app/pulls/5/merge'
unread "a hash entry from a variable that makes jq run gh" 'hash -p "$P" jq; jq api -X PUT repos/acme/app/pulls/5/me$x'
# A computed word that could be gh, then api, is held to the plain form as `pr merge` after one is.
unread "a computed gh, then api's merge endpoint" '$G api -X PUT repos/acme/app/pulls/5/merge'
# Regression guards: each was already refused, as a gh the reader saw beside a merge.
unread "an alias to gh, then pr merge through it" "alias g='gh'; g pr merge 5"
unread "a function that merges, then called" 'f() { gh pr merge 5; }; f'
unread "a function named jq that runs gh, its endpoint computed at the call" 'jq() { gh "$@"; }; jq api -X PUT repos/acme/app/pulls/5/me$x'
unread "a function defined with the function keyword" 'function f { gh pr merge 5; }; f'
unread "a hash entry that makes jq run gh" 'hash -p /usr/bin/gh jq; jq api -X PUT repos/acme/app/pulls/5/merge'
guard 0 "OPS-66: hash -r alone is not a definition" 'hash -r'
guard 0 "OPS-66: an empty arithmetic expansion is not a function definition" 'echo $(()) $(( ))'
# zsh reads `>!` as one operator; bash reads `!` as the file and the next word as an argument. A
# command with an operator followed by `!` is judged with every word, operands included, as a word
# (OPS-66, round 6): each of these ran gh's merge in bash, and was allowed.
unread "<>! before gh, after /usr/bin/env" '/usr/bin/env <>! gh api -X PUT repos/acme/app/pulls/5/merge'
unread ">! before gh, after /usr/bin/nohup" '/usr/bin/nohup >! gh api -X PUT repos/acme/app/pulls/5/merge'
unread ">&! before gh api, handed to sh -c" "sh -c '\"\$@\"' sh >&! gh api -X PUT repos/acme/app/pulls/5/merge"
# A regression guard: `pr merge` after sh was already refused.
unread ">&! before gh pr merge, handed to sh -c" "sh -c '\"\$@\"' sh >&! gh pr merge 5"
guard 0 "OPS-66: gh pr view with its output redirected last" 'gh pr view 5 --json state > out.json'
guard 0 "OPS-66: gh pr list with 2>/dev/null last" 'gh pr list 2>/dev/null'
rm -f "$R/.git/ci-receipt"
# The backstop's git -C rewrite takes no value that "$@" or "${a[@]}" can split into several words
# (`-c alias.x='!gh pr' x`). The reader refuses both first, so merge_hint is tested on its own.
hint=$(sed -n '/^HIBYTES=/,/^}/p' "$ROOT/.claude/hooks/pr-merge-guard.sh")
for v in '"$@"' '"${a[@]}"'; do
  (eval "$hint"; merge_hint "git -C $v merge 5") \
    && ok "OPS-66: merge_hint still matches git -C $v before merge" \
    || fail "OPS-66: merge_hint read git -C $v merge as git's own merge"
done
(eval "$hint"; ! merge_hint 'git -C "$WT" merge origin/main') \
  && ok "OPS-66: merge_hint reads git -C \"\$WT\" merge as git's own merge" \
  || fail "OPS-66: merge_hint matched git -C \"\$WT\" merge origin/main"
guard 2 "OPS-66: gh api PUT to this repo's merge endpoint, without evidence" 'gh api -X PUT repos/acme/app/pulls/5/merge'
grep -q 'PR #5 has no gate evidence' "$d/err" && ok "OPS-66: ...read as PR 5's merge and held to its evidence" || fail "OPS-66: gh api merge: $(head -n 1 "$d/err")"
guard 2 "OPS-66: a plain merge with its output redirected, without evidence" 'gh pr merge 5 --squash > "$TMPDIR/$x"'
grep -q 'PR #5 has no gate evidence' "$d/err" && ok "OPS-66: ...read as PR 5's merge and held to its evidence" || fail "OPS-66: redirected merge: $(head -n 1 "$d/err")"
# A redirection's operand is not an argument: the shell runs `gh pr merge 5` for each of these, and
# the reader once read the operand as the word after pr, so each was ALLOWED without evidence.
guard 2 "OPS-66: a merge with a file redirection between pr and merge, without evidence" 'gh pr >"$TMPDIR/x" merge 5 --squash'
guard 2 "OPS-66: a merge with a here-string between pr and merge, without evidence" 'gh pr <<<x merge 5 --squash'
mv "$d/receipt.ops66" "$R/.git/ci-receipt" 2>/dev/null || :
mv "$d/receipt.ops66lane" "$lane_receipt" 2>/dev/null || :
# OPS-57: a PR word the reader cut short. merge_argv stops at a `$` or a backtick, and once kept the
# digits before it as the PR, so the guard judged a shorter number than the shell merged. With
# evidence for that shorter number's head, each of these was ALLOWED. The PR word is either read
# whole or not at all: no PR, so the merge is refused as one whose PR cannot be told.
cp "$R/.git/ci-receipt" "$d/receipt.ops57" 2>/dev/null || :
receipt "$MAINSHA"
cutpr() {  # cutpr LABEL COMMAND: refused as a merge whose PR cannot be told, not on its evidence
  guard 2 "OPS-57: $1" "$2"
  [ "$rc" != 2 ] || grep -q 'cannot tell which PR this merges' "$d/err" \
    || fail "OPS-57: $1: blocked, but not as a PR it cannot tell: $(head -n 1 "$d/err")"
}
cutpr "a PR number with a parameter expansion after its digits" 'gh pr merge 5$X --squash'
cutpr "a PR number with a braced expansion after its digits" 'gh pr merge 5${X} --squash'
cutpr "a PR number with a command substitution after its digits" 'gh pr merge 5$(echo 1) --squash'
cutpr "a PR number with a backtick substitution after its digits" 'gh pr merge 5`echo 1` --squash'
cutpr "a PR number with an expansion after its digits, flags first" 'gh pr merge --squash 5$X'
cutpr "a PR URL with an expansion after its number" 'gh pr merge https://github.com/acme/app/pull/5$X --squash'
cutpr "a quoted PR number with an expansion inside its quotes" 'gh pr merge "5$X" --squash'
# A `(` after the digits is zsh's glob qualifier, which can empty or rewrite the word.
cutpr "a PR number with a glob qualifier after its digits" 'gh pr merge 5(N) --squash'
cutpr "a PR URL with a glob qualifier after its number" 'gh pr merge https://github.com/acme/app/pull/5(N) --squash'
# A repo flag glued to a cut value still names a repo, one the guard cannot confirm.
cutrepo() {  # cutrepo LABEL COMMAND: refused as a repo it cannot confirm
  guard 2 "OPS-57: $1" "$2"
  [ "$rc" != 2 ] || grep -q 'cannot confirm which repository' "$d/err" \
    || fail "OPS-57: $1: blocked, but not as a repo it cannot confirm: $(head -n 1 "$d/err")"
}
cutrepo "a glued -R whose value is cut" 'gh pr merge 5 --squash -R$X'
cutrepo "a glued -R whose value is cut after this repo's name" 'gh pr merge 5 --squash -Racme/app$X'
cutrepo "a --repo= whose value is cut" 'gh pr merge 5 --squash --repo=acme/app$X'
cutrepo "a --repo= whose value is all expansion" 'gh pr merge 5 --squash --repo=$X'
# gh reads a cluster of its single-letter flags (-d -m -r -s on/off; -A -b -F -t and -R take a value)
# as each flag in turn: a repo flag at the end of a cluster names a repo, and a value-taking one takes
# the next word, which is then no PR.
cutrepo "a repo flag after on/off flags in one word, its value separate" 'gh pr merge 5 -sR other/tool'
cutrepo "a repo flag after on/off flags in one word, its value attached" 'gh pr merge 5 -dsRother/tool'
cutrepo "a repo flag after on/off flags in one word, its value cut" 'gh pr merge 5 -sR$X'
# An empty repo flag overrides nothing: gh keeps the last -R and, empty, falls back to GH_REPO or
# the directory, while the reads would use the -R before it (#271).
cutrepo "an empty -R \"\" after this repo's -R" 'cd /tmp && gh pr merge 5 -R acme/app -R "" --squash'
cutrepo "an empty --repo= after this repo's hosted -R" 'GH_REPO=other/x gh pr merge 5 -R github.com/acme/app --repo= --squash'
cutrepo "an empty -R= last" 'gh pr merge 5 -R acme/app --body x -R='
cutrepo "an empty -sR '' after this repo's -R=" "gh pr merge 5 -R=acme/app -sR '' --squash"
cutpr "a subject flag after an on/off flag in one word takes the next word" 'gh pr merge -st 5 feature --squash'
guard 0 "OPS-57: a repo flag after on/off flags naming this repo is this repo's merge" "gh pr merge 5 -sR acme/app"
guard 0 "OPS-57: -R with this repo attached still reads as this repo" "gh pr merge 5 -s -Racme/app"
guard 0 "OPS-57: -R with this repo separate still reads as this repo" "gh pr merge 5 -R acme/app -s"
guard 0 "OPS-57: on/off flags in one word with no repo flag stay this repo's merge" "gh pr merge 5 -sd"
# What still reads exactly: each of these is PR 5, allowed on PR 5's evidence.
guard 0 "OPS-57: a plain PR number still reads exactly" "gh pr merge 5 --squash --delete-branch"
guard 0 "OPS-57: this repo's PR URL still reads exactly" "gh pr merge https://github.com/acme/app/pull/5 --squash"
guard 0 "OPS-57: a quoted PR number still reads exactly" 'gh pr merge "5" --squash'
guard 0 "OPS-57: an expansion in a later flag's value leaves the PR word whole" 'gh pr merge 5 --squash --subject "$X"'
cutpr "the current-branch form (no PR word) is still refused" "gh pr merge --squash --delete-branch"
rm -f "$R/.git/ci-receipt"; mv "$d/receipt.ops57" "$R/.git/ci-receipt" 2>/dev/null || :
guard 2 "two merges in one command" "gh pr merge 5 --squash && gh pr merge 6 --squash"
guard 0 "another repo's merge, in the allowlisted shape" "gh pr merge 2 -R other/tool --squash --delete-branch"
guard 2 "another repo named outside the shape" "echo x | xargs gh pr merge 2 -R other/tool"
guard 2 "this repo named by -R is still policed" "gh pr merge 5 -R acme/app --squash" GH_HEAD=0000000000000000000000000000000000000000

# Non-ASCII in a UTF-8 locale (#100): BSD awk died on a byte of `✅` or `🤖` taken apart one at a
# time, and the guard refused harmless commands as unreadable. Only a UTF-8 locale shows it (mawk
# and the C locale read bytes), so these run in one where the machine has it. Each label names the
# locale it ran in: under C they pass without having shown the crash cannot recur.
U8=$(locale -a 2>/dev/null | grep -iE '^(en_US|C)\.utf-?8$' | head -n 1)
U8=${U8:-C}
in8="(LC_ALL=$U8)"
# No evidence anywhere for these, the lane's receipt included; both come back after.
mv "$R/.git/ci-receipt" "$d/receipt.main" 2>/dev/null || :
mv "$lane_receipt" "$d/receipt.lane" 2>/dev/null || :
guard 0 "#100: a read-only grep for 'merged' beside ✅ $in8" "grep -n '✅ merged\\|merge guard' docs/roadmap.md" LC_ALL="$U8"
guard 0 "#100: 'merged' beside 🤖 in a printf body $in8" "printf '🤖 merged\\n' > /dev/null" LC_ALL="$U8"
guard 0 "#100: gh pr create with 'merged' beside 🤖 in a printf body $in8" "gh pr create --title fix --body \"\$(printf '🤖 merged\\n')\"" LC_ALL="$U8"
# A heredoc body is read whole, never one character at a time: this one passed before #100's fix too.
guard 0 "#100: 'merge' beside 🤖 and ’ in a heredoc body $in8" "$(printf "cat <<'EOF'\n🤖 merged: the merge guard’s fix\\\\n\nEOF")" LC_ALL="$U8"
guard 2 "#100: a real merge whose body path has 🤖 is still a merge (no evidence) $in8" "gh pr merge 5 --squash --body-file /tmp/🤖body.md" LC_ALL="$U8"
grep -q 'PR #5 has no gate evidence' "$d/err" && ! grep -q 'could not read this command' "$d/err" \
  && ok "#100: ...blocked by the evidence rule, not as unreadable" || fail "#100 merge block: $(cat "$d/err")"
receipt "$MAINSHA"
guard 0 "#100: the same merge with a receipt for its head is allowed (read as PR 5) $in8" "gh pr merge 5 --squash --body-file /tmp/✅\\ body.md" LC_ALL="$U8"
rm -f "$R/.git/ci-receipt"
# zsh in a UTF-8 locale takes a non-ASCII letter into a parameter name (#100 review): `$é` is an
# expansion, so each of these runs a merge there. Read as a literal `$é`, they were ALLOWED.
oddmerge() {  # oddmerge LABEL COMMAND: blocked as a form the reader cannot read
  guard 2 "$1" "$2" LC_ALL="$U8"
  [ "$rc" != 2 ] || grep -q 'may merge in a form this guard cannot read' "$d/err" \
    || fail "$1: blocked, but not as a form it cannot read: $(head -n 1 "$d/err")"
}
oddmerge "#100: \$é splits merge, then an escaped g $in8" 'gh pr mer$é\ge 5 --squash'
oddmerge "#100: \$é splits merge, then a quoted ge $in8" 'gh pr mer$é"ge" 5 --squash'
oddmerge "#100: \$é splits pr, then an escaped r $in8" 'gh p$é\r merge 5 --squash'
oddmerge "#100: pr held in \$é $in8" 'é=pr; gh $é merge 5'
oddmerge "#100: pr held in \$xé (a non-ASCII byte after an ASCII start) $in8" 'xé=pr; gh $xé merge 5'
oddmerge "#100: control: pr held in an ASCII \$X $in8" 'X=pr; gh $X merge 5'
oddmerge "#100: \${é} splits merge $in8" 'gh pr mer${é}ge 5 --squash'
oddmerge "#100: \${é:-} splits merge $in8" 'gh pr mer${é:-}ge 5 --squash'
# The reader's own reason, not merge_hint's backstop: hits() reads `$é merge` inside a ${...}.
oddmerge "#100: a \${...} default naming the merge through \$é $in8" '${GH:-$é merge 5}'
grep -q 'a ${...} expansion names the merge' "$d/err" \
  && ok "#100: ...as the reader's \${...} reason" || fail "#100 \${...} reason: $(head -n 1 "$d/err")"
receipt "$MAINSHA"
oddmerge "#100: a non-ASCII word before gh, with a receipt $in8" 'é=x gh pr merge 5 --squash'
rm -f "$R/.git/ci-receipt"
oddmerge "#100: a non-ASCII word before gh, without evidence $in8" 'é=x gh pr merge 5 --squash'
guard 0 "#100: \$é in a command that names no merge is allowed $in8" 'echo $é done' LC_ALL="$U8"
guard 0 "#100: \"\$é\" grepped for in a file named merge-notes is allowed $in8" 'grep -n "$é" docs/merge-notes.md' LC_ALL="$U8"
mv "$R/.claude/hooks/lib/merge-reader.awk" "$d/reader.bak"
guard 2 "#100: the reader missing still fails closed for a command naming a merge $in8" "grep -n '✅ merged\\|merge guard' docs/roadmap.md" LC_ALL="$U8"
grep -q 'could not read this command to decide whether it merges' "$d/err" \
  && ok "#100: ...with the unreadable message" || fail "#100 unreadable message: $(cat "$d/err")"
guard 0 "#100: the reader missing still allows a command naming neither gh nor merge $in8" "echo ✅ done" LC_ALL="$U8"
mv "$d/reader.bak" "$R/.claude/hooks/lib/merge-reader.awk"
rm -f "$R/.git/ci-receipt"
mv "$d/receipt.main" "$R/.git/ci-receipt" 2>/dev/null || :
mv "$d/receipt.lane" "$lane_receipt" 2>/dev/null || :

# Without jq: fails CLOSED for a merge, open for anything else.
mkdir -p "$d/nojq"
for t in sh cat git awk sed grep tr sort cut head tail wc dirname basename pwd printf env; do
  p=$(command -v "$t" 2>/dev/null) && ln -sf "$p" "$d/nojq/$t"
done
nojq() {
  rc=0; payload "$2" "$R" | (cd "$R" && PATH="$d/nojq" "$d/nojq/sh" "$R/.claude/hooks/pr-merge-guard.sh") >/dev/null 2>&1 || rc=$?
  expect_rc "$1" "$rc" "merge-guard without jq: $3"
}
nojq 2 "gh pr merge 5 --squash" "blocks a merge"
nojq 0 "git status" "allows anything else"
# merge_hint reads a non-ASCII parameter name too (#100 review): without jq it is the only test.
nojq 2 'é=pr; gh $é merge 5' "#100: blocks pr held in \$é"
nojq 2 'X=pr; gh $X merge 5' "#100: control: blocks pr held in an ASCII \$X"
# merge_hint reads git's own merge only past an option value the shell cannot split, as git_sub does
# (OPS-66): unquoted, $X could split into `-c alias.x='!gh pr' x`.
nojq 2 'git -C $X merge 5' "OPS-66: blocks git -C with an unquoted, splitting value before merge"

# raw_command is duplicated on purpose (a sourced lib's missing-file failure is shell-dependent);
# the two copies must not drift.
a=$(sed -n '/^raw_command() {/,/^}/p' "$ROOT/.claude/hooks/pr-merge-guard.sh")
b=$(sed -n '/^raw_command() {/,/^}/p' "$ROOT/.claude/hooks/no-blind-source-rewrite.sh")
[ -n "$a" ] && [ "$a" = "$b" ] && ok "raw_command is identical in pr-merge-guard.sh and no-blind-source-rewrite.sh" || fail "raw_command differs between the two hooks"

# Rule 13: a model choice rests on an eval receipt (OPS-10). main gets model-roles.json (provenance
# off, so rule 12 stays out of it), two agents, a workflow and a two-case reviewer suite with the
# real runner. Receipts are written by that runner over fake-claude.sh, so these cases also prove
# the runner's working-tree hashes equal the guard's, read from the commit.
git -C "$R" checkout -q main
mkdir -p "$R/.claude/agents" "$R/.claude/workflows" "$R/.claude/evals/reviewer/cases"
jq '.provenance.enabled = false' "$ROOT/.claude/model-roles.json" > "$R/.claude/model-roles.json"
cp "$ROOT/.claude/agents/reviewer.md" "$ROOT/.claude/agents/builder.md" "$R/.claude/agents/"
wf() {  # wf [PROMPT] [OUTSIDE] [CALL] [AGENT_TYPE]: a build-change.js with both marked sections
  printf '// the workflow %s\nconst a = 1\n// <review-prompt>\nconst REVIEW_PROMPT = "%s"\nconst REVIEW = {"type":"object","properties":{"findings":{"type":"array"}},"required":["findings"]}\nconst reviewSpawn = () => agent(REVIEW_PROMPT, { schema: REVIEW, agentType: %s })\n// </review-prompt>\nconst b = 2\n// <review-call>\nconst rev = await %s\n// </review-call>\nconst pick = (role) => {\n  return ROLES[role]\n}\n' \
    "${2:-}" "${1:-Review group {n} of {change}.}" "${4:-'reviewer'}" "${3:-reviewSpawn()}" > "$R/.claude/workflows/build-change.js"
}
wf
cp "$ROOT/.claude/evals/run.sh" "$ROOT/.claude/evals/fake-claude.sh" "$R/.claude/evals/"
for c in one two; do
  mkdir -p "$R/.claude/evals/reviewer/cases/$c/before" "$R/.claude/evals/reviewer/cases/$c/after"
  echo "echo a" > "$R/.claude/evals/reviewer/cases/$c/before/$c.sh"; echo "echo b" > "$R/.claude/evals/reviewer/cases/$c/after/$c.sh"
done
jq -n '{id: "one", lang: "sh", kind: "defect", title: "t", brief: "b", tasks: ["t"], expected: [{id: "D1", severity: "high", file: "one.sh", lines: [1, 1], keywords: ["echo"], why: "w"}]}' > "$R/.claude/evals/reviewer/cases/one/case.json"
jq -n '{id: "two", lang: "sh", kind: "clean", title: "t", brief: "b", tasks: ["t"], expected: []}' > "$R/.claude/evals/reviewer/cases/two/case.json"
echo "The project's rules every case runs under." > "$R/.claude/evals/reviewer/AGENTS.md"
git -C "$R" add -A && git -C "$R" commit -qm "evals base"
(cd "$R" && git push -q origin HEAD:main 2>/dev/null && git fetch -q origin)
ev() {  # ev MODEL EFFORT [EVAL_FAKE]: run the eval on the working tree, receipt into .claude/evals/receipts/
  (cd "$R" && EVAL_ALLOW_FAKE_RECEIPT=1 EVAL_CLAUDE="$R/.claude/evals/fake-claude.sh" EVAL_FAKE="${3:-perfect}" EVAL_DATE=2026-02-01 \
    sh .claude/evals/run.sh --role reviewer --model "$1" --effort "$2" >/dev/null 2>&1)
}
roles_set() { jq "$1" "$R/.claude/model-roles.json" > "$d/mr13" && cp "$d/mr13" "$R/.claude/model-roles.json"; }
g13() {  # a block must be rule 13's, not some other rule's
  guard "$1" "rule 13: $2" "gh pr merge 5 --squash" GH_HEAD="$(H)" GH_BRANCH=ops/models
  [ "$1" != 2 ] || [ "$rc" != 2 ] || grep -q 'rule 13' "$d/err" || fail "rule 13: $2: blocked, but not by rule 13: $(head -2 "$d/err")"
}

on ops/models; roles_set '.roles.reviewer.model = "sonnet"'; head_of "reviewer on sonnet, no receipt"; at
g13 2 "the reviewer's model changes with no receipt"
grep -q 'sh .claude/evals/run.sh --role reviewer --model sonnet --effort high' "$d/err" && ok "...and the block names the eval to run" || fail "rule 13 block message: $(cat "$d/err")"
on ops/models; roles_set '.roles.reviewer.model = "sonnet"'; ev sonnet high; head_of "reviewer on sonnet, with its receipt"; at
g13 0 "the reviewer's model changes with a passing receipt at the head's hashes"
on ops/models; roles_set '.roles.reviewer.model = "sonnet"'; ev opus high; head_of "reviewer on sonnet, a receipt for opus"; at
g13 2 "a receipt for another model than the head's"
on ops/models; roles_set '.roles.reviewer.model = "sonnet"'; ev sonnet high silent; head_of "reviewer on sonnet, a failing receipt"; at
g13 2 "a receipt that does not pass (recall 0)"
on ops/models; roles_set '.roles.reviewer.model = "sonnet"'; ev sonnet high; roles_set '.roles.reviewer.tiers.high.effort = "max"'; head_of "a receipt, then the reviewer's tier variant changed"; at
g13 2 "a receipt run before the reviewer's tier variants changed (stale role hash)"
on ops/models; roles_set '.roles.reviewer.model = "sonnet"'; ev sonnet high; roles_set '.roles.builder.model = "sonnet"'; head_of "a receipt, then another role changed"; at
g13 0 "another role's choice changing does not stale the reviewer's receipt (OPS-16)"
on ops/models; ev opus high; echo "One more line." >> "$R/.claude/agents/reviewer.md"; head_of "agent edited after its receipt"; at
g13 2 "the reviewer agent edited after its receipt ran"
on ops/models; echo "One more line." >> "$R/.claude/agents/reviewer.md"; ev opus high; head_of "agent edited, then evaluated"; at
g13 0 "the reviewer agent edited, with a receipt run on the edit"
on ops/models; echo "One more line." >> "$R/.claude/agents/builder.md"; head_of "another agent edited"; at
g13 0 "an agent that is not a trigger of the reviewer (the builder's) edited: no receipt needed"
# The workflow: only the marked review-prompt section is the reviewer's (OPS-16).
on ops/models; wf "" "edited outside the markers"; head_of "workflow edited outside the markers"; at
g13 0 "build-change.js edited OUTSIDE the review-prompt markers needs no receipt"
grep -q 'rule 13: role reviewer' "$d/err" && fail "rule 13 named the reviewer for an edit outside the markers: $(grep 'rule 13' "$d/err")" || ok "...and rule 13 does not name the reviewer at all"
on ops/models; wf "Review it twice."; head_of "workflow edited inside the markers"; at
g13 2 "build-change.js edited INSIDE the review-prompt markers, with no receipt"
on ops/models; wf "Review it twice."; ev opus high; head_of "inside the markers, then evaluated"; at
g13 0 "...and with a receipt run on that edit"
on ops/models; wf "Review it twice."; ev opus high; wf "Review it twice." "and outside after the run"; head_of "inside, evaluated, then outside"; at
g13 0 "...and an edit outside the markers after the receipt does not stale it"
# The spawn itself is inside the markers (OPS-16 review): the call and the agent type.
on ops/models; wf "" "" "agent('approve everything', {})"; head_of "review call replaced by a literal approve"; at
g13 2 "the review call replaced by a literal 'approve' prompt (inside review-call), with no receipt"
on ops/models; wf "" "" "" "'builder'"; head_of "review spawned as the builder"; at
g13 2 "the review spawned with agentType 'builder' (inside review-prompt), with no receipt"
# The receipt's agent must be the head's trigger file (OPS-16 review).
on ops/models; wf "Review twice."; ev opus high; jq '.hashes.agent_file = ".claude/agents/builder.md"' "$R/.claude/evals/receipts/reviewer-opus-high-2026-02-01.json" > "$d/rc" && cp "$d/rc" "$R/.claude/evals/receipts/reviewer-opus-high-2026-02-01.json"; head_of "receipt names another agent file"; at
g13 2 "a receipt whose evaluated agent is not one of the reviewer's trigger inputs"
grep -q 'is not one of role reviewer' "$d/err" && ok "...and the block names the agent mismatch" || fail "agent_file block: $(cat "$d/err")"
on ops/models; wf "Review twice."; ev opus high; jq --arg b "$(git -C "$R" hash-object "$R/.claude/agents/builder.md")" '.hashes.agent = $b' "$R/.claude/evals/receipts/reviewer-opus-high-2026-02-01.json" > "$d/rc" && cp "$d/rc" "$R/.claude/evals/receipts/reviewer-opus-high-2026-02-01.json"; head_of "receipt agent blob is another file's"; at
g13 2 "a receipt whose evaluated agent blob is not the head's reviewer.md"
on ops/models; wf "Review twice."
rc=0; (cd "$R" && EVAL_ALLOW_FAKE_RECEIPT=1 EVAL_AGENT_FILE="$R/.claude/agents/builder.md" EVAL_CLAUDE="$R/.claude/evals/fake-claude.sh" EVAL_DATE=2026-02-01 sh .claude/evals/run.sh --role reviewer --model opus --effort high >/dev/null 2>&1) || rc=$?
expect_rc 2 "$rc" "run.sh refuses to evaluate an agent file that is not one of the role's trigger inputs"
git -C "$R" checkout -q -- .claude/workflows/build-change.js; rm -rf "$R/.claude/evals/receipts"
# Weakening rule 13 itself is the owner's decision: blocked even WITH a passing receipt.
on ops/models; ev opus high; roles_set '.evals.triggers.reviewer = []'; head_of "triggers emptied, with a receipt"; at
g13 2 "the reviewer's triggers emptied, even with a receipt"
grep -q 'weakens rule 13 itself' "$d/err" && ok "...and the block says it is the owner's decision" || fail "policy block: $(cat "$d/err")"
on ops/models; roles_set '.evals.triggers.reviewer += [".claude/agents/extra.md"]'; ev opus high; head_of "a trigger added, with a receipt"; at
g13 0 "a trigger ADDED (widening), with a receipt run on it"
on ops/models; rm -rf "$R/.claude/evals/reviewer"; head_of "the suite deleted"; at
g13 2 "the reviewer's eval suite deleted (would turn rule 13 into an advisory)"
on ops/models; ev opus high; roles_set '.evals.thresholds.fp_rate = 0.5'; head_of "fp_rate ceiling raised, with a receipt"; at
g13 2 "the fp_rate ceiling raised, even with a receipt"
on ops/models; roles_set '.evals.thresholds.recall = 0.7'; head_of "recall floor lowered"; at
g13 2 "the recall floor lowered"
on ops/models; ev opus high; roles_set '.evals.thresholds.recall = 0.9'; head_of "recall floor raised"; at
g13 0 "the recall floor RAISED (stricter) is not a weakening"
# pick() picks the reviewer's model at run time, outside every hashed section (OPS-16 round 2).
on ops/models; sed 's/^const pick = (role) => {$/&\
  if (role === "reviewer") return { model: "haiku", effort: "low" }/' "$R/.claude/workflows/build-change.js" > "$d/wf" && cp "$d/wf" "$R/.claude/workflows/build-change.js"; ev opus high; head_of "pick routes the reviewer to haiku"; at
g13 2 "pick() edited to route the reviewer to haiku, even with a receipt (no receipt hashes pick)"
grep -q "changes (or removes) build-change.js's pick()" "$d/err" && ok "...and the block names pick()" || fail "pick block: $(cat "$d/err")"
# A suite weakened without being deleted, and a receipt scored on another suite.
on ops/models; rm -rf "$R/.claude/evals/reviewer/cases/two"; head_of "a clean case removed"; at
g13 2 "a base eval case removed (the suite weakened, not deleted)"
on ops/models; jq '.expected = []' "$R/.claude/evals/reviewer/cases/one/case.json" > "$d/cj" && cp "$d/cj" "$R/.claude/evals/reviewer/cases/one/case.json"; head_of "a case's planted defect dropped"; at
g13 2 "a base case's planted defects changed"
on ops/models; jq '.brief = "b. Hint: the defect is on line 1."' "$R/.claude/evals/reviewer/cases/one/case.json" > "$d/cj" && cp "$d/cj" "$R/.claude/evals/reviewer/cases/one/case.json"; head_of "a hint added to a case's brief"; at
g13 2 "a hint added to a base case's brief (case.json beyond .expected)"
on ops/models; echo "echo c" > "$R/.claude/evals/reviewer/cases/one/after/one.sh"; head_of "a case's after/ tree edited"; at
g13 2 "a base case's after/ tree edited"
grep -q 'it changes eval case one' "$d/err" && ok "...and the block names the case" || fail "case tree block: $(cat "$d/err")"
on ops/models; echo "Hint: every echo is a defect." >> "$R/.claude/evals/reviewer/AGENTS.md"; head_of "a hint in the suite's AGENTS.md"; at
g13 2 "a hint added to the suite's AGENTS.md"
grep -q 'AGENTS.md, which every case' "$d/err" && ok "...and the block names the suite's AGENTS.md" || fail "suite AGENTS.md block: $(cat "$d/err")"
on ops/models; roles_set '.roles.reviewer.model = "sonnet"'; ev sonnet high
mkdir -p "$R/.claude/evals/reviewer/cases/three/before" "$R/.claude/evals/reviewer/cases/three/after"
echo "echo a" > "$R/.claude/evals/reviewer/cases/three/before/three.sh"; echo "echo d" > "$R/.claude/evals/reviewer/cases/three/after/three.sh"
jq -n '{id: "three", lang: "sh", kind: "clean", title: "t", brief: "b", tasks: ["t"], expected: []}' > "$R/.claude/evals/reviewer/cases/three/case.json"
head_of "receipt, then a case added"; at
g13 2 "a receipt scored on another suite than the head's (a case added after it ran; adding is allowed)"
grep -q 'scored on another suite' "$d/err" && ok "...and the block names the suite" || fail "suite hash block: $(cat "$d/err")"
rcp="$R/.claude/evals/receipts/reviewer-sonnet-high-2026-02-01.json"
on ops/models; roles_set '.roles.reviewer.model = "sonnet"'; ev sonnet high; jq '.suite.agents_md = "0000000000000000000000000000000000000000"' "$rcp" > "$d/rc" && cp "$d/rc" "$rcp"; head_of "receipt names another suite AGENTS.md"; at
g13 2 "a receipt that ran under another suite AGENTS.md than the head's"
on ops/models; roles_set '.roles.reviewer.model = "sonnet"'; ev sonnet high; jq 'del(.suite.agents_md)' "$rcp" > "$d/rc" && cp "$d/rc" "$rcp"; head_of "a receipt from before suite.agents_md"; at
g13 0 "a receipt from before OPS-16 round 3 (no suite.agents_md) is accepted on its cases hash"
# Deleting the declaration falls back to the BROADER default: not a narrowing (a receipt still needed).
on ops/models; roles_set 'del(.evals.triggers.reviewer)'; ev opus high; head_of "declaration deleted, default applies"; at
g13 0 "the reviewer's declaration deleted, with a receipt: the broader default is not a narrowing"
grep -q 'narrows' "$d/err" && fail "a fallback to the broader default was reported as narrowing: $(grep narrows "$d/err")" || ok "...and nothing calls it narrowing"
on ops/models; printf '// the workflow, markers gone\nconst reviewPrompt = () => "Review it."\nconst pick = (role) => {\n  return ROLES[role]\n}\n' > "$R/.claude/workflows/build-change.js"; head_of "markers removed"; at
g13 2 "the review-prompt markers removed (fails closed: the section cannot be read)"
grep -q 'review-prompt cannot be read at the head' "$d/err" && ok "...and the block names the missing markers" || fail "rule 13 markers block: $(cat "$d/err")"
on ops/models; printf '// <review-prompt>\nconst reviewPrompt = () => "Review it."\n' > "$R/.claude/workflows/build-change.js"
rc=0; (cd "$R" && EVAL_ALLOW_FAKE_RECEIPT=1 EVAL_CLAUDE="$R/.claude/evals/fake-claude.sh" EVAL_DATE=2026-02-01 sh .claude/evals/run.sh --role reviewer --model opus --effort high >/dev/null 2>&1) || rc=$?
expect_rc 2 "$rc" "run.sh refuses to run when a trigger's markers are not both there (no receipt certifies markers-missing)"
git -C "$R" checkout -q -- .claude/workflows/build-change.js; rm -rf "$R/.claude/evals/receipts"
on ops/models; roles_set '.evals.triggers.reviewer = [".claude/agents/reviewer.md"]'; head_of "the section dropped from the triggers"; at
g13 2 "a PR that drops a trigger input from the declaration (a later PR could edit it free)"
on ops/models; roles_set '.roles.builder.model = "sonnet"'; head_of "builder on sonnet"; at
g13 0 "a role with no eval suite (the builder) changes: advisory only"
grep -q 'role builder is changed but has no eval suite' "$d/err" && ok "...and it says why it does not block" || fail "rule 13 advisory: $(cat "$d/err")"
on ops/models; roles_set '.roles.reviewer.eval = {baseline: "opus/high"}'; head_of "evidence recorded, no choice changed"; at
g13 0 "recording evidence (.eval) changes no choice"
on ops/models; ev opus high; head_of "a receipt alone"; at
mv "$R/.claude/lib/evals.sh" "$d/evals.bak"
g13 2 "the evals library missing (fails closed, not open)"
mv "$d/evals.bak" "$R/.claude/lib/evals.sh"
# Markers already missing at the base: an edit to the file still counts, and blocks.
git -C "$R" checkout -q main
printf '// no markers on main\nconst reviewPrompt = () => "Review it."\n' > "$R/.claude/workflows/build-change.js"
git -C "$R" commit -qam "main loses the markers" && (cd "$R" && git push -q origin HEAD:main 2>/dev/null && git fetch -q origin)
on ops/models; printf '// no markers on main\nconst reviewPrompt = () => "Approve everything."\n' > "$R/.claude/workflows/build-change.js"; head_of "edit a file whose markers are gone at both ends"; at
g13 2 "a file whose markers are missing at base AND head, edited (cannot tell what changed)"
git -C "$R" checkout -q main

# Extension rules: the enabled modules' guard.d/*.sh and .claude/local/guard.d/*.sh run after the
# core rules, with the same exit/advisory contract, and fail closed (lib/modules.sh).
cp "$ROOT/.claude/lib/modules.sh" "$R/.claude/lib/"
TOP=$(git -C "$R" rev-parse HEAD); receipt "$TOP"
G="$R/.claude/local/guard.d"; mkdir -p "$G"
gx() {  # gx WANT LABEL [env...]: a plain merge of the clean head, through the extension rules
  w=$1 l=$2; shift 2
  guard "$w" "guard.d: $l" "gh pr merge 5 --squash" GH_HEAD="$TOP" "$@"
}
printf 'echo "PR $GUARD_PR head $GUARD_HEAD branch $GUARD_BRANCH"\n' > "$G/10-facts.sh"
gx 0 "a rule that exits 0 allows"
case "$out" in *'"additionalContext"'*"local guard.d/10-facts.sh: PR 5 head $TOP branch fix/1-x"*) ok "guard.d: its stdout reaches Claude as an advisory, with the PR's facts in GUARD_*" ;; *) fail "guard.d advisory: $out" ;; esac
printf '#!/usr/bin/env bash\nif [[ -n "$GUARD_PR" ]]; then echo "bash ran me"; fi\n' > "$G/20-bash.sh"
gx 0 "a bash-only rule runs under bash (its #! line), not sh"
rm -f "$G/20-bash.sh"
printf 'echo "not this one" >&2\nexit 2\n' > "$G/30-block.sh"
gx 2 "a rule that exits 2 blocks"
grep -q 'local guard.d/30-block.sh: not this one' "$d/err" && ok "guard.d: ...with its stderr as the reason" || fail "guard.d block message: $(cat "$d/err")"
printf 'exit 1\n' > "$G/30-block.sh"
gx 2 "a rule that crashes (exit 1) blocks: fail closed"
printf 'if then fi\n' > "$G/30-block.sh"
gx 2 "a rule that does not parse blocks"
grep -q 'does not parse' "$d/err" && ok "guard.d: ...and says it does not parse" || fail "guard.d parse message: $(cat "$d/err")"
printf 'sleep 20\n' > "$G/30-block.sh"
gx 2 "a rule that outlives GUARD_RULE_TIMEOUT blocks" GUARD_RULE_TIMEOUT=1
grep -q 'did not finish within 1 s' "$d/err" && ok "guard.d: ...and says it timed out" || fail "guard.d timeout message: $(cat "$d/err")"
rm -f "$G/30-block.sh"
mkdir -p "$R/.claude/modules/strict/guard.d"
printf 'name="strict"\nrequires=""\nenables="guard.d"\n' > "$R/.claude/modules/strict/module.conf"
printf 'echo "strict says no" >&2\nexit 2\n' > "$R/.claude/modules/strict/guard.d/no.sh"
gx 0 "a DISABLED module's rule does not run"
echo 'MODULES="strict"' >> "$R/.claude/project.conf"
gx 2 "an ENABLED module's rule runs"
grep -q 'module strict guard.d/no.sh: strict says no' "$d/err" && ok "guard.d: ...and the block names the module" || fail "guard.d module block: $(cat "$d/err")"
sed -i.bak 's/^MODULES=.*/MODULES="nosuch"/' "$R/.claude/project.conf" && rm -f "$R/.claude/project.conf.bak"
gx 2 "a module in MODULES that cannot load blocks (its rules cannot run)"
sed -i.bak '/^MODULES=/d' "$R/.claude/project.conf" && rm -f "$R/.claude/project.conf.bak"
mv "$R/.claude/lib/modules.sh" "$d/modules.bak"
gx 2 "the loader missing while .claude/local/guard.d exists (fails closed, not open)"
mv "$d/modules.bak" "$R/.claude/lib/modules.sh"
rm -rf "$G" "$R/.claude/modules"
gx 0 "no extension rules at all: the guard is unchanged"
finish
