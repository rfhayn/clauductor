#!/bin/sh
STRIDAL_FW=$(cd "$(dirname "$0")/.." && pwd) # stridal plugin: the plugin root (framework/internal/plugin)
# PreToolUse hook (matcher: Bash). The merge gate: `merge-pr` merges without waiting for a human
# (AGENTS.md, "Who decides"), so this hook is the whole gate, not a backstop behind one. It polices
# the merge ACTION, client-side, because server-side required status checks cannot see a local
# gate run. Rules:
#
#   1. --auto / --admin are refused outright. `--auto` merges the moment a PR is "mergeable",
#      which with no required check configured is immediately: that is how a PR merged while its
#      checks still read pending. `--admin` bypasses checks too.
#   2. A merge is blocked unless there is GATE EVIDENCE for the exact head commit: (a) no reported
#      check is red, and (b) either the named remote workflow (GATE_REMOTE_WORKFLOW in
#      .claude/project.conf) succeeded on that SHA, or a receipt from a complete, clean local gate
#      run (GATE_RUN) names it, in any worktree of this repository. Other green checks do not
#      count. (c) Advisory: the head is behind origin/<main>.
#   3. A capability-change PR (BRANCH_CHANGE) is blocked unless the change's tasks.md states its
#      slice (`Slice: …`). A presence check: it makes the question be answered out loud.
#   4. Advisory: more than one change is proposed at once (propose just in time).
#   7. A PR is blocked if its journal claims a `## Session N` another merged session already uses,
#      or if it ADDS a `## Session N` and is not a session close (BRANCH_SESSION_CLOSE): a row
#      writes a fragment (JOURNAL_FRAGMENTS/<row id>.md), and only a close numbers a session.
#   8. A session-close PR (BRANCH_SESSION_CLOSE), or any PR that changes JOURNAL or INSIGHTS, is
#      blocked unless its head contains origin/<main>. BRANCH_SESSION_CLOSE="" turns off rule 8
#      and rule 7's second half.
#   9. A build PR on a change branch is blocked until every task of the change it touches is ticked.
#  10. A merge is blocked while an enforced scenario is cited by no test at the head
#      (.claude/scenario-trace.sh --rev; D4 of the change process).
#  11. A PR that archives a change is blocked if the change has an open task, no spec delta and no
#      `skip_specs: true`, no recorded actual cost, or (when it says how we'll know) no queued
#      outcome-check row in the roadmap.
#  12. While model-roles.json `provenance.enabled` is true, a merge is blocked unless its squash body
#      (--body or --body-file) ends in Change:, Agent-Role:, Model: and Session: trailers.
#  13. A PR that changes what a role with a suite IS (its model, effort or tier variants in
#      model-roles.json, or a trigger input model-roles.json .evals.triggers declares for it: for
#      the reviewer, its agent file and the marked review-prompt section of build-change.js) is
#      blocked unless the head holds a passing eval receipt (.claude/evals/run.sh) for that role,
#      run at the head's hashes of exactly those inputs (OPS-10, OPS-16; .claude/lib/evals.sh).
#      A PR that narrows a role's triggers, deletes a suite, removes or changes a case or the
#      suite's AGENTS.md, weakens .evals.thresholds, or changes build-change.js's pick() is blocked
#      outright: the owner's decision, which no receipt excuses. A receipt must also have been
#      scored on the head's suite. Scope: ACCIDENTAL drift; a receipt is self-reported, so a
#      deliberate forger is review's and the owner's to catch (lib/change-guard.sh, SCOPE).
#  14. A merge is blocked unless the PR's state is OPEN, read before any evidence; a state gh cannot
#      read blocks too. A close is how one session tells another not to land a PR (OPS-132).
#  15. While model-roles.json `attribution.enabled` is not true, only a squash merges, and it is
#      blocked unless it names its body (--body-file, or a literal --body) and no line of it is a
#      Co-authored-by trailer: GitHub's default squash body copies the branch's trailers, and a
#      merge commit or a rebase lands the branch's commits as they are (OPS-156).
#   Rules 9–13 live in lib/change-guard.sh; rule 15 reads the merge with that library's cg_field.
#   (Numbering follows the rules this was extracted from; 5 and 6 were project-specific, and so
#   was the other half of 8, which the artifacts module adds as an extension rule.)
#
# A MERGE IN ANOTHER REPOSITORY is not policed beyond rule 1, but it passes only when the whole
# command matches one allowlisted plain shape (foreign_shape, below) and its -R owner/name differs
# from this repo's origin; any other command naming another repo blocks. A merge of THIS repo whose
# PR is not a number or PR URL (flag-first forms, or no PR at all) BLOCKS, and so does one whose
# words leave gh to resolve the repository from something the guard does not read (an allowlist, at
# ghpr). The environment a shell profile sets is not read: it is the session's, not the command's.
#
# Known residual, accepted: a merge clicked on the website never runs this.
#
# Protocol: exit 2 = block (stderr is shown to Claude); exit 0 = allow, with advisories carried as
# hookSpecificOutput.additionalContext (the only exit-0 channel that reaches Claude).
#
# THE SELF-FILTER BELOW IS THE AUTHORITY, deliberately: there is no `if` in settings.json. An
# `if: Bash(gh pr merge *)` is strictly NARROWER than the reader below, which catches a merge
# anywhere in the command string, including forms a permission rule may not match. For a safety
# gate, breadth beats the cost of a shell that exits 0 at once.
#
# Dependencies: jq and gh. Checked by .claude/checks/merge-guard.sh.

ROOT_HOOK=$(case $STRIDAL_FW in (*/.claude) dirname "$STRIDAL_FW" ;; (*) [ -n "${ROOT_HOOK:-}" ] && echo "$ROOT_HOOK" || git rev-parse --show-toplevel 2>/dev/null || pwd ;; esac)
ROOT=$ROOT_HOOK
# A `.` of a missing file is fatal with a shell-dependent status, which Claude Code may read as
# "allow": test first, and fall back to the defaults.
if [ -f "$STRIDAL_FW/lib/conf.sh" ]; then
  # shellcheck disable=SC1091
  . "$STRIDAL_FW/lib/conf.sh"
else
  MAIN_BRANCH=main JOURNAL=docs/development-journal.md INSIGHTS=docs/insights-log.md JOURNAL_FRAGMENTS=docs/journal.d CHANGES_DIR=changes BRANCH_CHANGE=change/ GATE_RUN=scripts/ci/run-local.sh GATE_REMOTE_WORKFLOW=
fi
REMOTE_WF=${GATE_REMOTE_WORKFLOW:-}  # the default is conf.sh's, stated there once
payload=$(cat)

block() {
  echo "BLOCKED by pr-merge-guard: $1" >&2
  exit 2
}

# Defined HERE, beside block(): `sh` executes function definitions in order, so a call above the
# definition is "say: not found" on stderr, non-fatal and therefore invisible.
#
# On an ALLOWED call (exit 0) Claude Code sends a PreToolUse hook's stdout and stderr to the debug
# log only; the model sees stderr on exit 2 and `hookSpecificOutput.additionalContext` JSON on
# stdout, nothing else. An advisory printed plainly reached nobody (five merges in a row showed not
# one), so `say` COLLECTS each line and allow() emits the collection as additionalContext. stderr
# is kept: on a block it is what Claude reads, and it is the debug log's copy. The check asserts
# the JSON on stdout, not merged output, which is exactly how this once hid.
SAID=""
say() {
  echo "pr-merge-guard: $1" >&2
  SAID="${SAID}${SAID:+
}pr-merge-guard: $1"
}

# allow — exit 0, carrying everything `say` collected to Claude. jq escapes the text; without jq
# a sed escape of backslash, quote and newline does the same job.
allow() {
  if [ -n "$SAID" ]; then
    if command -v jq >/dev/null 2>&1; then
      jq -cn --arg c "$SAID" '{hookSpecificOutput:{hookEventName:"PreToolUse",additionalContext:$c}}'
    else
      esc=$(printf '%s' "$SAID" | sed 's/\\/\\\\/g; s/"/\\"/g' | awk 'BEGIN{ORS=""} NR>1{print "\\n"} {print}')
      printf '{"hookSpecificOutput":{"hookEventName":"PreToolUse","additionalContext":"%s"}}\n' "$esc"
    fi
  fi
  exit 0
}

# raw_command — the RAW JSON text of every `"command": "…"` value in the payload, one per line, with
# escapes left as they are; the whole payload if there is none. For the fallback below ONLY: it lets
# the fallback match the command without matching the Bash tool's `description` or the payload's
# `cwd` (prose, where "gh pr merge" is ordinary). A `"command"` key cannot be forged from inside a
# string, because a quote inside a JSON string is always escaped. If awk itself fails, the caller
# uses the whole payload — broader, never narrower.
#
# The SAME function is in `no-blind-source-rewrite.sh`, deliberately inline rather than a sourced
# lib: a hook whose `.` of a missing file fails exits non-2 or 2 depending on the shell, and either
# is wrong. .claude/checks/merge-guard.sh asserts the two copies are identical.
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

# merge_hint — step 1 of "WHICH COMMANDS ARE A MERGE" (read that block, below the jq check). Here
# because unreadable() uses it and runs first. Quotes and backslashes deleted, blank runs (newlines
# too) squeezed, then: the words `pr merge`, flags allowed between; `merge` after a computed word;
# `pr` before one. Exit 0 = may be a merge. A parameter name may hold bytes 0x80-0xFF, as zsh's
# `$é` does (#100): HIBYTES is that range, matched as bytes under LC_ALL=C. Git's own merge with
# its global options first (`git -C "$WT" merge origin/main`) is read as `git merge` before the
# match: a computed -C value before `merge` is not a gh (OPS-66; the reader's git_sub). Only a value
# the shell cannot split is taken (GITV), judged BEFORE the quotes go: quoted, with no blank,
# separator, substitution or `@` inside (`"$@"` and `"${a[@]}"` split even quoted), or plain text
# with no expansion at all. So `git -C $X merge 5` still matches, as git_sub refuses it: unquoted,
# $X could split into `-c alias.x='!gh pr' x`.
HIBYTES=$(printf '\200-\377')
GITV='("[^" ;&|()<>`\\@]*"|'"'"'[^'"'"' ;&|()<>`\\]*'"'"'|[^ ;&|()<>`$"'"'"'\\{}*?[]+)'
merge_hint() {
  printf '%s' "$1" | tr '\t\r\n' '   ' | tr -s ' ' \
    | LC_ALL=C sed -E 's/(^|[^A-Za-z0-9_-])git(( -P| --no-pager| --no-optional-locks| --no-replace-objects| --bare| (-C|--git-dir|--work-tree) '"$GITV"')+) merge([^A-Za-z0-9_-]|$)/\1git merge\6/g' \
    | tr -d "\\\\'\"" | tr -s ' ' | LC_ALL=C grep -Eq \
    -e '(^|[^A-Za-z0-9_-])pr( -[^ ]+( [^ -][^ ]*)?)* merge([^A-Za-z0-9_-]|$)' \
    -e '([)}`]|[$][A-Za-z0-9_@*#?!'"$HIBYTES"'-]+) merge([^A-Za-z0-9_-]|$)' \
    -e '(^|[^A-Za-z0-9_-])pr [$`]'
}

# WITHOUT A WORKING jq THIS HOOK FAILS CLOSED — for a merge, and only for a merge.
#
# Reading the command with jq and nothing else, a missing jq made `cmd` empty, and the merge was
# ALLOWED with nothing but "jq: command not found" on stderr. A guard that allows when a dependency
# is missing degrades to silence (AGENTS.md rule 4), and this hook is the whole merge gate.
#
# So when the payload cannot be read, the command's RAW JSON text decides (raw_command, above). If
# it contains `gh pr merge`, the hook blocks and names jq. That test is at least as broad as the
# decoded one below: JSON encoding escapes none of those characters, so a command containing
# `gh pr merge` carries it literally. (An encoder that \u-escaped plain ASCII would slip past;
# Claude Code's JSON.stringify does not.) Anything else — `ls`, `git status`, `git merge` — is still
# allowed, as is a command whose DESCRIPTION mentions a merge. Blocking EVERY Bash call whenever jq
# is missing would also be fail-closed, and would stop the session dead over a risk confined to one
# command. The price is a false block when `gh pr merge` appears in the command without being run
# (`grep 'gh pr merge' …`) — only while jq is broken, and the message says how to end it.
#
# "Present but failing" is the same case: a jq that exits non-zero is treated exactly like none.
# The normalised test (merge_hint) runs here too, on the raw JSON with its escapes turned to
# blanks: `\"gh\" pr merge 5` is a merge without jq exactly as with it. The check removes jq from
# PATH to prove it.
unreadable() {
  raw=$(raw_command) || raw=$payload
  # Blank runs squeezed to one space, JSON's `\t` escape included, so `gh  pr<TAB>merge` matches too.
  # Squeezing only ever ADDS matches: a single-spaced `gh pr merge` survives it.
  raw=$(printf '%s' "$raw" | sed 's/\\t/ /g' | tr -s ' \t' '  ')
  hit=""
  case "$raw" in *"gh pr merge"*) hit=1 ;; esac
  [ -n "$hit" ] || ! merge_hint "$(printf '%s' "$raw" | sed 's/\\[nr]/ /g')" || hit=1
  if [ -n "$hit" ]; then
    block "$1, so this hook cannot read the command and refuses rather than let a merge through unchecked. Install it (macOS: brew install jq; Debian/Ubuntu: apt-get install jq), check that 'jq --version' works, then retry."
  fi
  exit 0
}
command -v jq >/dev/null 2>&1 || unreadable "jq is not installed (not on PATH)"
cmd=$(printf '%s' "$payload" | jq -r '.tool_input.command // empty' 2>/dev/null) \
  || unreadable "jq failed to read the hook payload"

# WHICH COMMANDS ARE A MERGE. One text match (`gh pr merge` in the command) is not enough: the
# shell spells a word many ways, so `"gh" pr merge 5`, `g\h pr merge 5`, `gh "pr" merge 5` and
# `$(echo gh) pr merge 5` passed such a match before any rule ran, and so did words assembled from
# pieces (`gh pr me$()rge 5`, `gh pr merg? 5`, `gh pr {merge,} 5`, `gh pr mer\<newline>ge 5`). So a
# TEXT test does not decide whether the reader runs. Instead:
#
#   1. READ EVERY COMMAND, as the shell would (lib/merge-reader.awk: quotes, escapes,
#      backslash-newline, `$(…)`, backticks, `${…}`, `$'…'`, globs, brace expansion, zsh `=gh`,
#      heredocs, comments, separators). Each word keeps a regex of every string it could become.
#      Wherever a word could be gh, the words after it must be readable: `gh pr merge` with all three
#      words plain is a merge SITE (policed below); `gh api` PUT to …/pulls/<n>/merge is one too
#      (policed as `gh pr merge <n>`); a quoted, escaped, globbed, brace-expanded or computed
#      word where gh, pr, merge or the api endpoint goes, and that could be one, is ODD and BLOCKS
#      with the plain form — the rules below parse a merge from its text, and cannot parse one they
#      cannot read. `pr merge` after anything but a plain gh (`$GH`, an alias) is ODD too.
#      A REDIRECTION INSIDE A gh COMMAND IS UNREADABLE (OPS-66, the owner's call): in a simple
#      command whose command word could be gh (literal, quoted, `=gh`, a path, or COMPUTED: `$cmd`
#      counts as gh) or runs its arguments as a command (env, xargs, sudo, exec, …), a redirection
#      operator of ANY form before the command's last word is ODD (inside()). The shell lifts a
#      redirection out from anywhere and runs the rest (`gh pr >& x merge 5`, `gh {fd}>x api -X
#      PUT …/pulls/5/merge`, zsh `>!`, `2>/dev/null`), the operator's spellings are too many to read
#      one by one, and `gh api` merges with no merge word to look for, so this fails closed on all
#      of them, listed or not. The command word decides what runs: a literal one that is neither
#      (`jq . > "$out" in.json`, `echo >&2 "$msg"`) is read as before. A command word given by
#      path (`/usr/bin/env`) or a shell (`sh -c '"$@"' sh …`) runs its arguments too. Where the
#      operand is in doubt, operands count: one that is gh as written, and, after an operator
#      with a `!` (zsh's `>!` clobbers; bash reads `!` as the file and the next word as an
#      argument), every word that could be gh, computed included. The accepted cost: a gh
#      command, or one whose command word is computed, with a redirection in its middle is refused,
#      a merge or not (`gh pr list 2>/dev/null --json number`, `$cmd >&2 x`); put the redirection
#      last (`gh pr merge 5 --squash 2>&1`, `gh pr list 2>/dev/null` are read).
#      A `(` right after `<` or `>`, or inside a word (`<(…)`, `>(…)`, zsh `=(…)` and `x(N)`), is
#      part of that word, its contents read as commands: only a `(` that starts a word starts a
#      subshell, so `gh api --input <(…) …/pulls/5/merge` keeps its endpoint.
#      Elsewhere a redirection's operand (a file name, a here-string's text) is never a word of the
#      command: `sed … > "$TMPDIR/$n"` hands sed no gh.
#      NOT A MERGE, so never ODD (OPS-66, #120):
#      a word that could be a path to gh (`template/$f`) handed to a command that runs none of its
#      arguments (inert(): cp, cmp, cat, sed, …) with nothing after it that could be pr, api or
#      merge; and git's own merge after its global options (`git -C "$WT" merge origin/main`,
#      git_sub(): git cannot alias a builtin), unless an option's value is unquoted and could split.
#   2. THE TEXT MATCH IS A BACKSTOP, no longer a filter (merge_hint, and the old literal match, on the
#      command with backslash-newlines removed): if it hits and the reader found neither a site nor a
#      mention, the reader missed something, so the hook blocks. If the reader is missing or fails,
#      a command that names gh or merge at all blocks.
#   3. PROSE PASSES. `pr merge` inside ONE word — a quoted argument, a heredoc body — is a MENTION,
#      not a site: `git commit -m "…gh pr merge…"`, `gh pr create --body "$(cat <<'EOF' …)"` and
#      `grep -n "gh pr merge" f` are allowed without being policed, even with `--auto` in the text.
#      But the same word is CODE to `bash -c`, `eval`, `alias`, `xargs sh -c` or
#      `| sh`, and a script once written to a file, so a mention passes only when EVERY simple
#      command in the line (substitutions included) is one prose_safe names as inert and none writes
#      a file. Anything else BLOCKS. An ALLOWLIST again, for the reason foreign_shape is one.
#
# A comment that mentions the merge blocks: zsh without interactivecomments reads `#` as a word.
#
# KNOWN LIMITS — a text guard cannot close this in general (a server-side required status check is
# the complete answer, and cannot see a local gate run):
#   - a command whose EVERY word is computed (`$A $B $C 5`), or a merge assembled across commands
#     without its words ever appearing (pieces written to a file one by one, then run);
#   - an alias or function defined OUTSIDE the command: a `gh alias set m 'pr merge'` in an earlier
#     session, a shell alias, hash entry or function from a dotfile (`gh m 5` reads as subcommand
#     `m`). Setting a gh alias through this hook blocks (the expansion is a mention beside an
#     unlisted command). A shell `alias`, a `hash` entry (but `hash -r`), the `function` keyword
#     or a `name()` ANYWHERE in the command makes it unreadable, whatever the name runs
#     (defines(), OPS-66): at the name's use, `jq api -X PUT …/pulls/5/merge` reads as jq. The
#     cost: a command that defines a helper is refused, and so are a bare `alias` (a listing) and
#     `hash jq` (a lookup); inline its body, or use a script file. defines() reads only the
#     command's own words, so these definitions are NOT closed:
#       - one inside quoted program text: `eval 'f() { gh "$@"; }'; f api …`, `sh -c 'alias …'`;
#       - zsh's parameter tables: `functions[jq]='gh "$@"'`, `commands[jq]=/usr/bin/gh`, then
#         `jq api …`, an assignment the reader takes for a plain one;
#       - git's own aliases, below.
#     The owner's call (OPS-66, #146 round 5): list these, and rethink the guard in OPS-86
#     rather than close each form here;
#   - an evaluator whose PROGRAM is computed, piped or in a file (`sh -c "$(…)"`, `printf … | sh`,
#     `eval "$(… | base64 -d)"`, `source <(…)`, `sh notes.sh`, and GNU sed's `e` command or `s///e`
#     flag, which run the text sed reads: `sed e notes.txt`) unless its text names a merge: a hook
#     that reads text cannot run arbitrary programs. That includes the OUTPUT of a command on
#     inert()'s list whose computed path argument is gh (`ls -d bin/$G | sh`): since OPS-66 that
#     word is a file name;
#   - git with an alias defined in the same command (`git -c alias.y='!gh pr' y merge 5`): the
#     alias's text names no `pr merge` and its name is not pr, so nothing reads it as a merge;
#   - a merge through any other client (curl to the API, a GraphQL query in a file passed some
#     other way), and a click on the website.
# Checked by .claude/checks/merge-guard.sh, with jq and without.
#
# merge_hint (step 2) is defined above unreadable(), which runs before this point and uses it.

# merge_scan — lib/merge-reader.awk on $cmd. Its header lists what it prints.
# Under LC_ALL=C (#100): the reader walks the command one character at a time with substr, which
# BSD awk counts in BYTES, then matches each piece against a regex, which a UTF-8 locale converts
# to a wide character. A piece that is part of `✅` or `🤖` does not convert, awk dies ("towc:
# multibyte conversion failure"), and the guard refused harmless commands as unreadable. The reader
# needs only bytes: its syntax characters are ASCII, and a non-ASCII letter in a parameter name
# (zsh's `$é`) is matched as any byte 0x80-0xFF.
READER="$(dirname "$0")/lib/merge-reader.awk"
merge_scan() {
  [ -f "$READER" ] || return 1
  printf '%s' "$cmd" | LC_ALL=C awk -f "$READER"
}
PLAIN_FORM='gh pr merge <n> --squash --delete-branch — gh, pr and merge unquoted and unescaped, nothing substituted, one merge per command'

# Step 1 — read. The reader gets the command exactly as the shell will.
scan=$(merge_scan); scan_ok=$?

# Every text parser below (the backstop, rule 1, foreign_shape, merge_argv) reads the command with
# each backslash-newline deleted, as the shell joins them: `gh pr mer\<newline>ge 5` is `merge`.
# Only a line ending in an ODD run of backslashes joins: in "a\\<newline>" or x\\<newline> the
# backslash is escaped, and joining there read the closing quote as escaped, or a comment on the
# next line as the merge's -R (#271). In single quotes the shell joins nothing, but the quotes
# still close where they did, so only the text inside them reads differently.
cmd=$(printf '%s' "$cmd" | awk '
  { l[NR] = $0 }
  END {
    for (i = 1; i <= NR; i++) {
      if (i < NR && match(l[i], /\\+$/) && RLENGTH % 2) printf "%s", substr(l[i], 1, length(l[i]) - 1)
      else printf "%s%s", l[i], (i < NR ? "\n" : "")
    }
  }')

# Step 2 — the backstop: the old literal match, and merge_hint.
hint=""
case "$(printf '%s' "$cmd" | tr -s ' \t' '  ')" in *"gh pr merge"*) hint=1 ;; esac
[ -n "$hint" ] || ! merge_hint "$cmd" || hint=1

if [ "$scan_ok" -ne 0 ]; then
  # Missing or failing reader: fail closed for anything that names gh or merge at all.
  if [ -n "$hint" ] || printf '%s' "$cmd" | tr -cd 'A-Za-z0-9' | tr 'A-Z' 'a-z' | grep -Eq 'gh|merge'; then
    block "could not read this command to decide whether it merges ($READER missing or failed), so it refuses. Restore it, then retry the merge as: $PLAIN_FORM"
  fi
  exit 0
fi
odd=$(printf '%s\n' "$scan" | sed -n 's/^ODD //p' | head -n 1)
unsafe=$(printf '%s\n' "$scan" | sed -n 's/^UNSAFE //p' | head -n 1)
plain=$(printf '%s\n' "$scan" | sed -n 's/^PLAIN //p')
apis=$(printf '%s\n' "$scan" | sed -n 's/^API //p')
mention=""
case "$scan" in *MENTION*) mention=1 ;; esac

if [ -z "$odd$unsafe$plain$apis" ]; then
  # Step 3 — prose: the words only inside text that no command here runs. Before rule 1 on purpose: a
  # commit message may say `--auto`.
  [ -n "$mention" ] && exit 0
  [ -n "$hint" ] && block "this command names 'pr merge' but this guard could not find where it runs, so it refuses rather than let a merge through unchecked. Merge with the plain form: $PLAIN_FORM"
  exit 0
fi

# Rule 1 — no --auto / --admin (they don't wait for checks to settle).
case "$cmd" in
  *--auto*)
    block "'--auto' merges before checks settle — a PR once merged that way with its checks still pending. Watch checks green first, then merge explicitly: gh pr merge <n> --squash --delete-branch"
    ;;
  *--admin*)
    block "'--admin' bypasses checks. Merge explicitly only after CI is green."
    ;;
esac

# Steps 2 and 3, the blocks. Every rule below parses the merge from its text, so a merge it cannot
# read is refused here rather than read wrong.
[ -z "$odd" ] || block "this command may merge in a form this guard cannot read: $odd. So its CI evidence cannot be checked. Merge with the plain form: $PLAIN_FORM"
[ -z "$unsafe" ] || block "this command carries the words 'pr merge' inside text, beside '$unsafe', which could run that text as a command — only a short list (git commit/log/show/…, gh pr|issue create/edit/comment/…, echo, printf, cat, grep, head, tail, cd) is treated as inert. If it is a merge, run it on its own: $PLAIN_FORM. If it is prose, run that command on its own, or pass the text in a file (git commit -F, gh pr create --body-file)."

# One merge per command, whichever way it is spelled.
nsites=$((${plain:-0} + $(printf '%s' "$apis" | grep -c .)))
[ "$nsites" -le 1 ] || block "this command runs more than one merge, and only one could be checked. Run each merge as a command of its own: gh pr merge <n> --squash --delete-branch"
# A `gh api` merge: the reader found `gh api … PUT repos/<o>/<n>/pulls/<pr>/merge`, with
# every word it needs plain. Policed below exactly as `gh pr merge <pr>` — `.` is this repo
# ({owner}/{repo}); any other repo takes the foreign rules, with its own allowlisted shape.
api_repo=""; api_pr=""; api_host=""
if [ -n "$apis" ]; then read -r api_repo api_pr api_host <<EOF
$apis
EOF
fi

# WHICH REPO AND WHICH PR — both from ONE parse of the merge's argv, AFTER rule 1, which is merge
# safety anywhere.
#
# Rules 2-7 are about THIS repo: its CI, its local receipts, its tasks.md, its journal. A merge in
# another repository (`gh pr merge 2 -R <owner>/<tool>`) was once judged as this repo's PR #2 and
# blocked.
#
# FOREIGN IS DECIDED BY ALLOWLIST, NEVER BY PARSING (foreign_shape). A merge passes as another
# repo's only when the WHOLE command, blank runs squeezed, is exactly
#     [cd <path> && ][VAR=value ]...gh pr merge <n> -R <owner>/<name> [flags] [2>/dev/null|2>&1]
# with only the flags listed at foreign_shape, each at most once, and no backslash, backtick, `$`,
# `|`, `;`, `(`, `)`, `{`, `}`, `<`, other `>`, newline, or `#` outside a quoted subject/body — AND
# that owner/name differs from this repo's. Three review rounds of a denylist each found a new way
# to make the shell run something other than what the parser read (a `#` comment, `xargs` appending
# a later `-R`, an escaped `\;`); an allowlist has no such tail.
#
# Everything else is POLICED as this repo's merge, from merge_argv — a small shell-word tokenizer
# that stops at `$`, a backtick, a `#` comment, a redirect or a separator. A word it stops INSIDE at
# `$`, a backtick or `(` is dropped, never kept cut short: its reading would be a shorter number than
# the shell merges (OPS-57). A repo flag glued to such a word still reads as a repo, unconfirmed:
#   - more than one `gh pr merge` (any blanks between the words) BLOCKS;
#   - the read ends only at a separator, or at redirects that alone, each with one plain target,
#     run to the end of the command (> f 2>&1); <(…) is a word, not a redirect. Any other stop BLOCKS
#     unless it is "$name" or "${name}", with only letters, digits and _./- around the closing
#     quote, in the command's last word (a trailing 2>&1 or 2>/dev/null aside; no "$(…)", whose )
#     a zsh comment can hide),
#     as the value of --body, --subject or another value flag: anything the read never saw could
#     be the -R gh merges in (#271). So does an unquoted character outside letters, digits,
#     _./:@%+=,- and non-ASCII (a brace, glob or tilde expands into words the read never sees);
#   - --auto or --admin as a word, its quotes taken out, BLOCKS (rule 1, which reads raw text);
#   - a repo named (-R X, -RX, -sR X, --repo X, --repo=X, or a PR URL) that is not this repo,
#     outside the allowlisted shape, BLOCKS and shows the shape;
#   - the PR is the first positional word (values of gh's value-taking flags skipped, `--` ends
#     flags): a number or this repo's PR URL. Anything else, including no PR at all (the
#     current-branch form), BLOCKS. Reading only "the integer right after `merge`", with none
#     found meaning ALLOW, left flag-first forms and PR URLs unpoliced.
#
# Repos compare as OWNER/NAME, lowercased, HOST IGNORED — gh treats any *.github.com host as
# github.com, and an ssh alias (`git@github-work:o/n`) is still this repo. THIS repo is the origin
# remote (https, ssh or scp form), never a hard-coded name; if it cannot be read, nothing is
# foreign. A merge naming no repo is this repo's (`cd ../other && gh pr merge 2` is policed — pass
# -R). What may come before a merge of this repo depends on what its words leave gh to resolve
# (GH_REPO, GH_HOST, the directory): an allowlist, at ghpr.
#
# repo_key — lowercased owner/name of an owner/name, host/owner/name, https/ssh/scp URL or PR URL.
repo_key() {
  printf '%s\n' "$1" | tr '[:upper:]' '[:lower:]' | sed -e 's|^[a-z+]*://||' -e 's|^[^/@]*@||' \
    -e 's|^\([^/:]*\):[0-9][0-9]*/|\1/|' -e 's|^\([^/:]*\):|\1/|' -e 's|/pull/.*$||' \
    -e 's|/*$||' -e 's|\.git$||' \
    | awk -F/ 'NR == 1 {
        for (i = 1; i <= NF; i++) if ($i !~ /^[a-z0-9._-]+$/ || $i == "." || $i == "..") exit
        if (NF == 2) print $1 "/" $2; else if (NF == 3) print $2 "/" $3
      }'
}
# merge_argv — one line per finding: `M` more than one merge, `R <repo>` per repo named, `P <word>`
# the PR argument, `U` words it cannot read that gh still gets, `A` an --auto/--admin word, `W
# <method>` per merge method enabled (squash, merge, rebase). For POLICING only: it decides which PR rules 2-8 check and whether a foreign repo
# is named at all. It never decides that a merge is foreign — foreign_shape does, by allowlist.
# LC_ALL=C for the reason merge_scan has it (#100): it, too, matches one character at a time.
merge_argv() {
  printf '%s' "$cmd" | LC_ALL=C awk '
    # cut(ch, quoted, rest, tok) — the read stops at ch with rest unread, which gh still gets, and gh
    # merges in the LAST -R (#271). Matching rest for a -R loses to quoting (-\R, --re""po), so a
    # stop passes only by allowlist: a `$` inside double quotes whose word ends the command (a
    # trailing 2>&1 or 2>/dev/null aside) and is the value of a flag, glued (--body=...) or confirmed
    # by the parse below (VCUT); never a word of its own, which gh reads as a flag (-Racme/app).
    # Only "$name" or "${name}", with letters, digits and _./- before or after the closing quote
    # ("$TMPDIR/body.md", "$TMPDIR"/body.md), is one closed word in bash and zsh alike: zsh splits
    # "$a[@]" as "$@", reads :h or :s after a name as a modifier, and takes a # inside "$(…)" for a
    # comment, so the ) the read sees can sit in it.
    function cut(ch, quoted, rest, tok) {
      if (ch != "$" || !quoted || rest !~ /^([A-Za-z_][A-Za-z0-9_]*|\{[A-Za-z_][A-Za-z0-9_]*\})[A-Za-z0-9_.\/-]*"[A-Za-z0-9_.\/-]*[ \t]*([ \t]2>(&1|\/dev\/null))?[ \t]*([\n;|]|&$|&[^>]|$)/) { print "U"; return }
      LAST = 1; GLUED = (tok ~ /^--(body|body-file|subject|match-head-commit|author-email)=/)
    }
    { buf = buf $0 "\n" }
    END {
      rest = buf; count = 0; off = 0; start = 0
      while (match(rest, /gh[ \t]+pr[ \t]+merge/)) {
        count++
        if (count == 1) start = off + RSTART + RLENGTH
        off += RSTART + RLENGTH - 1; rest = substr(rest, RSTART + RLENGTH)
      }
      if (count == 0) exit
      if (count > 1) print "M"
      s = substr(buf, start); n = length(s); q = ""; tok = ""; have = 0; nt = 0
      for (j = 1; j <= n; j++) {
        c = substr(s, j, 1)
        if (q == "\047") { if (c == "\047") q = ""; else tok = tok c; continue }
        if (c == "$" || c == "`") {
          # A repo flag glued to a cut value still names a repo: one that cannot be confirmed.
          if (tok ~ /^(-[dmrs]*R|--repo=)/) t[++nt] = "-R"
          cut(c, q == "\"", substr(s, j + 1), tok)
          have = 0; break
        }
        if (c == "\\" && j < n) { j++; if (substr(s, j, 1) != "\n") { tok = tok substr(s, j, 1); have = 1 }; continue }
        if (q == "\"") { if (c == "\"") q = ""; else tok = tok c; continue }
        if (c == "#" && !have) break
        if (c == "\047" || c == "\"") { q = c; have = 1; continue }
        if (c == " " || c == "\t") { if (have) { t[++nt] = tok; tok = ""; have = 0 }; continue }
        # A redirect ends the read only when redirects alone, each with one plain target, run to the
        # end of the command: a word after a target (&>f -R o/n), a process substitution <(…) or a
        # heredoc is something gh still gets (#271). A target holds no $( or backtick: inside one,
        # quotes nest, so a ) the read takes for the end may sit inside the target. Digits before
        # the operator are its descriptor (2>&1); any other glued text is a word (--squash>f).
        if (c == "<" || c == ">" || (c == "&" && substr(s, j + 1, 1) == ">")) {
          if (have && tok !~ /^[0-9]+$/) t[++nt] = tok
          r = substr(s, j); fine = 0; have = 0
          while (match(r, /^[0-9]*(&>>?|>>?|>&|>\||<&|<>|<)/)) {
            r = substr(r, RLENGTH + 1)
            if (!match(r, /^[ \t]*("([^"\\$`]|\$[A-Za-z_][A-Za-z0-9_]*|\$\{[A-Za-z_][A-Za-z0-9_]*\})*"|\047[^\047]*\047|\$[A-Za-z_][A-Za-z0-9_]*|\$\{[A-Za-z_][A-Za-z0-9_]*\}|[^ \t\n;&|()<>"\047\\$`])+/)) break
            r = substr(r, RLENGTH + 1); sub(/^[ \t]*/, "", r)
            if (r == "" || r ~ /^(#|[\n;|)]|&$|&[^>])/) { fine = 1; break }
          }
          if (!fine) print "U"
          break
        }
        if (c ~ /[\n;&|()]/) {
          # In zsh a `(` inside a word is a glob qualifier: dropping the word covers one that
          # empties a PR word, not one that rewrites it into other words.
          if (c == "(" && have) have = 0
          # A glob qualifier ends the read, not the command: words follow it.
          if (c == "(") print "U"
          break
        }
        # Unquoted, only characters that stay as written: a brace, glob or tilde ({-R,o/n}, -[R], -?)
        # expands into words this read never sees, so anything else is `U` (#271). A byte of a
        # non-ASCII character is no shell syntax (#100: a body file named with a 🤖).
        if (c !~ /[A-Za-z0-9_.\/:@%+=,-]/ && c < "\200") { print "U"; have = 0; break }
        tok = tok c; have = 1
      }
      if (q == "" && have) t[++nt] = tok
      pos = 0; endopts = 0
      for (k = 1; k <= nt; k++) {
        x = t[k]; gsub(/\n/, " ", x)
        if (!endopts && x == "--") { endopts = 1; continue }
        if (!endopts && x ~ /^-/) {
          # Rule 1 matches the raw text; the shell takes the quotes out of --au""to first.
          if (x ~ /^--(auto|admin)(=|$)/) print "A"
          if (x == "-R" || x == "--repo") { if (k < nt) { y = t[++k]; gsub(/\n/, " ", y); print "R " y } else print "R ?"; continue }
          if (x ~ /^--repo=/) { print "R " substr(x, 8); continue }
          if (x ~ /^(--body|--body-file|--subject|--match-head-commit|--author-email)$/) { if (k == nt) VCUT = 1; k++; continue }
          # The method, for rule 15: a value pflag reads as false enables none.
          if (x ~ /^--(squash|merge|rebase)(=|$)/) {
            m = substr(x, 3); sub(/=.*/, "", m)
            if (x !~ /=(0|f|F|false|FALSE|False)$/) print "W " m
            continue
          }
          if (x ~ /^--/) continue
          # A cluster of shorthands, as gh reads it: on/off letters (-d -m -r -s) in turn, until one
          # that takes a value (-R -A -b -F -t) takes the rest of the word, or the next word.
          for (p = 2; p <= length(x); p++) {
            ch = substr(x, p, 1)
            if (ch == "s") print "W squash"; else if (ch == "m") print "W merge"; else if (ch == "r") print "W rebase"
            if (ch ~ /[dmrs]/) continue
            if (ch !~ /[RAbFt]/) break
            v = substr(x, p + 1); sub(/^=/, "", v)
            if (substr(x, p + 1) == "") { if (k < nt) { v = t[++k]; gsub(/\n/, " ", v) } else { v = "?"; if (ch != "R") VCUT = 1 } }
            if (ch == "R") print "R " v
            break
          }
          continue
        }
        if (!pos) { pos = 1; print "P " x }
      }
      if (LAST && !VCUT && !GLUED) print "U"
    }'
}
# foreign_shape — the -R value when the WHOLE command, blanks squeezed, is exactly
#   [cd <path> && ][VAR=value ]...gh pr merge <n> [flag]...[ 2>/dev/null| 2>&1]
# with each flag at most once from: -R|--repo <owner>/<name> (or --repo=), --squash|--merge|--rebase,
# --delete-branch[=false], --subject|-t "<text>", --body|-b "<text>". <path> and <value> are
# [A-Za-z0-9_./~@:+-]+; <text> is double-quoted. Nothing at all otherwise.
foreign_shape() {
  printf '%s' "$cmd" | tr -s ' \t' '  ' | awk '
    { lines++; line = $0 }
    END {
      if (lines != 1) exit
      s = line; sub(/^ /, "", s); sub(/ $/, "", s)
      if (s ~ /[\\`$|;(){}<]/) exit
      if (match(s, / 2>(\/dev\/null|&1)$/)) s = substr(s, 1, RSTART - 1)
      if (s ~ />/) exit
      if (match(s, /^cd [A-Za-z0-9_.\/~@:+-]+ && /)) s = substr(s, RLENGTH + 1)
      while (match(s, /^[A-Za-z_][A-Za-z0-9_]*=[A-Za-z0-9_.\/~@:+-]+ /)) s = substr(s, RLENGTH + 1)
      if (!match(s, /^gh pr merge [0-9]+( |$)/)) exit
      s = substr(s, RLENGTH + 1); repo = ""
      while (s != "") {
        if (match(s, /^(-R |--repo |--repo=)[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+( |$)/)) {
          if (seen["R"]++) exit
          repo = substr(s, 1, RLENGTH); sub(/ $/, "", repo); sub(/^(-R |--repo |--repo=)/, "", repo)
        } else if (match(s, /^--(squash|merge|rebase)( |$)/)) { if (seen["m"]++) exit }
        else if (match(s, /^--delete-branch(=false)?( |$)/)) { if (seen["d"]++) exit }
        else if (match(s, /^(--subject|-t) "[^"]*"( |$)/)) { if (seen["t"]++) exit }
        else if (match(s, /^(--body|-b) "[^"]*"( |$)/)) { if (seen["b"]++) exit }
        else exit
        s = substr(s, RLENGTH + 1)
      }
      if (repo != "") print repo
    }'
}
# api_foreign_shape — the same allowlist for a `gh api` merge: the -R value's counterpart is
# the endpoint's owner/name. The WHOLE command, blanks squeezed, is exactly
#   [cd <path> && ][VAR=value ]...gh api <word>...[ 2>/dev/null| 2>&1]
# with each word, in any order, from: -X PUT|-XPUT|--method PUT|--method=PUT (once, required),
# [/]repos/<owner>/<name>/pulls/<n>/merge (once, required), -f|-F|--field|--raw-field <key>=<value>
# (value [A-Za-z0-9_.:/@+-]* or double-quoted), --silent.
api_foreign_shape() {
  printf '%s' "$cmd" | tr -s ' \t' '  ' | awk '
    { lines++; line = $0 }
    END {
      if (lines != 1) exit
      s = line; sub(/^ /, "", s); sub(/ $/, "", s)
      if (s ~ /[\\`$|;(){}<]/) exit
      if (match(s, / 2>(\/dev\/null|&1)$/)) s = substr(s, 1, RSTART - 1)
      if (s ~ />/) exit
      if (match(s, /^cd [A-Za-z0-9_.\/~@:+-]+ && /)) s = substr(s, RLENGTH + 1)
      while (match(s, /^[A-Za-z_][A-Za-z0-9_]*=[A-Za-z0-9_.\/~@:+-]+ /)) s = substr(s, RLENGTH + 1)
      if (!match(s, /^gh api /)) exit
      s = substr(s, RLENGTH + 1); repo = ""; put = 0
      while (s != "") {
        if (match(s, /^\/?repos\/[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+\/pulls\/[0-9]+\/merge( |$)/)) {
          if (seen["e"]++) exit
          repo = substr(s, 1, RLENGTH); sub(/^\/?repos\//, "", repo); sub(/\/pulls\/.*$/, "", repo)
        } else if (match(s, /^(-X |-X|--method |--method=)(PUT|put)( |$)/)) { if (seen["m"]++) exit; put = 1 }
        else if (match(s, /^(-f|-F|--field|--raw-field) [A-Za-z_]+=([A-Za-z0-9_.:\/@+-]*|"[^"]*")( |$)/)) {}
        else if (match(s, /^--silent( |$)/)) {}
        else exit
        s = substr(s, RLENGTH + 1)
      }
      if (put && repo != "") print repo
    }'
}
SHAPE='[cd <path> && ][VAR=value ]gh pr merge <n> -R <owner>/<name> [--squash|--merge|--rebase] [--delete-branch[=false]] [--subject "<text>"] [--body "<text>"] [2>/dev/null|2>&1], or gh api -X PUT repos/<owner>/<name>/pulls/<n>/merge [-f <key>=<value>]... — one merge, nothing piped, no xargs/env/subshell, no $, backtick, backslash or comment'

this_repo=$(repo_key "$(git --no-optional-locks remote get-url origin 2>/dev/null)")
# A NAME IS NOT A REPOSITORY (#282). GitHub redirects a renamed or transferred repository's old
# owner/name to its new one, for the API as for the web, so a merge naming the old name merges this
# repo's PR, and an origin remote never updated after a rename names the old one. A named repo that
# differs from origin's counts as another only when GitHub resolves the two to different
# repositories; when gh cannot answer, it is not known to be another.
# canon KEY — sets canon_out to the owner/name GitHub answers for KEY, lowercased; empty when gh
# cannot say. Each answer is kept for this run in the shell itself, never in a $( … ) subshell: a
# merge's name is asked about twice (the foreign shape, then argv), and this repo's every time.
# HOST, when given, is the server asked (gh api --hostname); else gh's own, as the merge's is.
canon_seen="" canon_out=""
canon() {
  _ck="${2:-.}:$1"
  case $canon_seen in
    *"
$_ck "*) canon_out=${canon_seen##*"
$_ck "}; canon_out=${canon_out%%"
"*}; return ;;
  esac
  canon_out=$(gh api ${2:+--hostname "$2"} "repos/$1" --jq .full_name </dev/null 2>/dev/null | tr '[:upper:]' '[:lower:]' | sed -n '1{/^[a-z0-9._-][a-z0-9._-]*\/[a-z0-9._-][a-z0-9._-]*$/p;}')
  canon_seen="$canon_seen
$_ck $canon_out
"
}
# this_host — origin's server when it is a GitHub Enterprise one (a dotted host name, not github.com
# or *.github.com, which gh reads as github.com): this repo is asked about there. An ssh alias
# (`git@github-work:o/n`) has no dot, and stays gh's own.
this_host=$(git --no-optional-locks remote get-url origin 2>/dev/null | tr '[:upper:]' '[:lower:]' \
  | sed -n -e 's|^[a-z+]*://\([^@/]*@\)\{0,1\}\([^/:]*\).*|\2|p' -e 's|^[^/@:]*@\([^/:]*\):.*|\1|p' | sed -n 1p)
case $this_host in *.*) case $this_host in github.com | *.github.com) this_host= ;; esac ;; *) this_host= ;; esac
# same_repo KEY — 0 when KEY is this repo, 1 when GitHub resolves it to another, 2 when it cannot say.
same_repo() {
  [ "$1" = "$this_repo" ] && return 0
  canon "$this_repo" "$this_host"; _t=$canon_out
  canon "$1"; _c=$canon_out
  [ -n "$_t" ] && [ -n "$_c" ] || return 2
  [ "$_c" = "$_t" ]
}
if [ -n "$api_pr" ]; then shaped=$(repo_key "$(api_foreign_shape)"); else shaped=$(repo_key "$(foreign_shape)"); fi
if [ -n "$this_repo" ] && [ -n "$shaped" ] && [ "$shaped" != "$this_repo" ]; then
  same_repo "$shaped"; _same=$?
  [ "$_same" != 2 ] || block "this merge names $shaped, and GitHub did not say which repository that is (gh api repos/$shaped failed), so it may be this repo ($this_repo) under an old name: a renamed or transferred repository's old name still reaches it. Check gh can reach GitHub (gh auth status), then merge again."
  if [ "$_same" = 1 ]; then
    say "merge targets $shaped, not this repo ($this_repo). This guard polices only this repo's merges beyond --auto/--admin; allowing."
    allow
  fi
fi

if [ -n "$api_pr" ]; then
  # `gh api --hostname H` sends the merge to H, so the reads name H too (-R H/<owner>/<name>): read
  # on the default host, they would judge another server's PR. The reader takes H from the flag's
  # own word (`-` none, `?` one it cannot read), never from text that merely says --hostname, and
  # an absolute https://api.github.com/ endpoint goes to github.com whatever H or GH_HOST says.
  case "$api_host" in
    - | '') ;;
    '?') block "this gh api merge names --hostname in a form this guard cannot read (computed, repeated, or not a plain host name), so it cannot read the PR on the host the merge goes to. Name it once, plainly: gh api --hostname <host> -X PUT repos/<owner>/<name>/pulls/<n>/merge" ;;
    *)
      [ "$api_repo" != "." ] || block "this gh api merge goes to $api_host (its --hostname or an absolute endpoint) and names {owner}/{repo}, which gh fills from this directory's repository. Name the repository: gh api --hostname $api_host -X PUT repos/<owner>/<name>/pulls/$api_pr/merge"
      api_repo="$api_host/$api_repo" ;;
  esac
  argv="P $api_pr"
  [ "$api_repo" = "." ] || argv="R $api_repo
$argv"
else
  argv=$(merge_argv)
fi
many=""; hidden=""; own=""; own_repo=""; other=""; unparsed=""; pr_arg=""; pr=""; ways=""
while IFS= read -r line; do
  case "$line" in
    M) many=1 ;;
    U) hidden=1 ;;
    "W "*) ways="$ways ${line#W }" ;;
    A) block "this merge passes --auto or --admin, quoted or escaped (rule 1): neither waits for checks to settle. Watch checks green first, then merge explicitly: gh pr merge <n> --squash --delete-branch" ;;
    "R "* | "P http://"* | "P https://"*)
      named=${line#? }
      key=$(repo_key "$named")
      # An empty -R "" overrides nothing: gh falls back to GH_REPO or the directory, past the -R before it.
      if [ -z "$key" ]; then unparsed=${named:-an empty -R}
      # This repo under another name (#282) is this repo: policed, its reads by the name it gave.
      elif same_repo "$key"; then
        own=1
        case "$line" in "R "*) own_repo=$named ;; esac
      else other=$key
      fi
      case "$line" in "P "*) pr_arg=$named ;; esac
      ;;
    "P "*) pr_arg=${line#P } ;;
  esac
done <<EOF
$argv
EOF

[ -z "$many" ] || block "this command runs more than one 'gh pr merge', and only one could be checked. Run each merge as a command of its own: gh pr merge <n> --squash --delete-branch"

# A repo other than this one is named, but the command is not in the allowlisted shape above: it may
# be anything (`echo -R <this repo> | xargs gh pr merge 5 -R <other>` merges OUR PR 5), so block.
if [ -n "$other" ] || [ -n "$unparsed" ]; then
  why="it names ${other:-$unparsed}"
  [ -n "$own" ] && why="$why and this repo ($this_repo) both"
  [ -z "$this_repo" ] && why="$why, and this checkout's origin remote is not a GitHub repository"
  block "cannot confirm which repository this merge targets: $why. A merge in another repository passes only in exactly this shape: $SHAPE"
fi

# The PR: a number, or this repo's PR URL. Nothing else can be checked, so nothing else merges.
case "$pr_arg" in
  # The first /pull/<n> after OWNER/NAME, as gh reads it: a greedy match took .../pull/5/files/x/pull/6 as 6.
  http://* | https://*) pr=$(printf '%s\n' "$pr_arg" | sed -n 's|^[a-z]*://[^/?#]*/[^/?#]*/[^/?#]*/pull/\([0-9][0-9]*\)\([/?#].*\)\{0,1\}$|\1|p') ;;
  *) case "$pr_arg" in '' | *[!0-9]*) ;; *) pr=$pr_arg ;; esac ;;
esac
[ -n "$pr" ] || block "cannot tell which PR this merges${pr_arg:+ ('$pr_arg' is not a PR number or PR URL)}, so its CI evidence cannot be checked. Name it: gh pr merge <n> --squash --delete-branch"
# The PR and its repo read whole, but the read stopped short of words gh still gets (#271).
[ -z "$hidden" ] || block "this merge has words this guard cannot read (a \$ or backtick expansion, a glob, a brace or tilde expansion) that could name another repository (-R), which gh would merge in instead of the one the checks read. Pass the text in a file, its path written out with no \$ (rule 12 reads it): gh pr merge <n> --squash --delete-branch --body-file <file>"

# ghpr SUBCOMMAND [ARGS...] — `gh pr SUBCOMMAND` for the PR this merge names, named the way the merge
# names it: its PR URL, or its number with the merge's own -R word. A bare number resolves through
# gh's default repository (`gh repo set-default`), which in a fork's checkout is not origin, so a
# read without the merge's -R can judge another repo's PR #n (OPS-132).
# run_repo is the same repository for `gh run list`, which takes no PR URL: the URL's host/owner/name.
gh_sel=$pr gh_repo="" run_repo=""
case "$pr_arg" in
  http://* | https://*) gh_sel=$pr_arg; run_repo=$(printf '%s\n' "$pr_arg" | sed -e 's|^[a-z]*://||' -e 's|/pull/.*$||') ;;
  *) gh_repo=$own_repo; run_repo=$own_repo ;;
esac
pr_ref="$gh_sel${gh_repo:+ -R $gh_repo}"
ghpr() {
  _sub=$1; shift
  if [ -n "$gh_repo" ]; then gh pr "$_sub" "$gh_sel" -R "$gh_repo" "$@"; else gh pr "$_sub" "$gh_sel" "$@"; fi
}

# THE READS RESOLVE AS THE MERGE DOES, BY ALLOWLIST. Whatever the merge's words leave open, gh fills
# from its environment and directory, which the command before the merge can change in more ways
# than any list of them (GH_REPO=, GIT_DIR=, `cd;`, `c\d`, `export "GH_""REPO=…"`, a sourced file:
# round 3 of OPS-132 passed each past a text match). So what may come before the merge depends on
# what its words leave open:
#   - a PR URL, or -R HOST/OWNER/NAME: nothing. Anything may come before it, so long as the reader
#     can read the merge's words (not after eval, or env or sudo with a flag outside the reader's
#     short list, as env -S and sudo -s parse them again: merge-reader.awk heads()); but a
#     --body-file is read only after nothing or `cd <path>` alone, written out in full after a cd
#     (rules 12 and 15 cannot know the directory, nor what wrote the file since: cg_field).
#   - -R OWNER/NAME: the host, which gh takes from GH_HOST. Only `cd <path> && ` may come before it.
#   - no repo (the bare number, gh api's {owner}/{repo}): the repository, from GH_REPO and the
#     directory's remotes. Nothing may come before it, and the payload's cwd (the Bash tool's
#     directory, not necessarily this hook's) must be in this repository: a worktree of it shares
#     its remotes and gh's default repo.
# Either way the reads run here, with the session's environment, which is also the merge's unless the
# Bash tool's shell adds to it (a profile exporting GH_REPO): that, no hook can see.
# merge_prefix — what comes before the merge's gh word, blank runs and newlines squeezed to one
# space, the ends trimmed; `?` when the command runs gh api more than once (which one merges is the
# reader's to know, not this text match's).
merge_prefix() {
  printf '%s' "$cmd" | LC_ALL=C awk -v api="${api_pr:+1}" '
    { b = b $0 "\n" }
    END {
      re = api ? "gh[ \t\n]+api[ \t\n]" : "gh[ \t]+pr[ \t]+merge"
      if (!match(b, re)) { print "?"; exit }
      p = substr(b, 1, RSTART - 1); rest = substr(b, RSTART + RLENGTH)
      if (api && match(rest, re)) { print "?"; exit }
      gsub(/[ \t\n]+/, " ", p); sub(/^ /, "", p); sub(/ $/, "", p); print p
    }'
}
# common_dir DIR — the physical path of the git directory DIR's worktree shares; empty if none.
common_dir() {
  (cd "$1" 2>/dev/null || exit 1
   g=$(git rev-parse --git-common-dir 2>/dev/null) && [ -n "$g" ] && cd "$g" 2>/dev/null && pwd -P)
}
pin_hint="Name the repo, and run the merge on its own: gh pr merge $pr -R ${this_repo:-<owner>/<name>} --squash --delete-branch"
prefix=$(merge_prefix)
# A prefix ending in / is a path glued to gh (/usr/local/bin/gh): say so, not "before it: …/bin/".
case "$prefix" in */) pin_hint="gh named by a path ($prefix) is not run on its own; call it plain gh. $pin_hint" ;; esac
case "$pr_arg/$gh_repo" in
  http://* | https://* | */*/*/*) ;;
  */?*)
    case "$prefix" in
      '') ;;
      *)
        printf '%s\n' "$prefix" | grep -Eq '^cd [A-Za-z0-9_./~@:+-]+ &&$' \
          || block "this merge names its repo without a host (-R $gh_repo), so gh takes the host from GH_HOST, which what comes before it ($prefix) can set while this guard reads without it. Run it on its own or after only 'cd <path> && ', or name the host: gh pr merge $pr -R github.com/$gh_repo --squash --delete-branch" ;;
    esac ;;
  *)
    [ -z "$prefix" ] || block "this merge names no repository and does not run on its own (before it: $prefix), so gh may resolve PR #$pr in another repository than the one this guard reads. $pin_hint"
    pcwd=$(printf '%s' "$payload" | jq -r '.cwd // empty' 2>/dev/null)
    [ -n "$pcwd" ] || block "this merge names no repository, and the hook payload names no cwd, so which repository gh resolves PR #$pr in cannot be checked. $pin_hint"
    here_git=$(common_dir .); there_git=$(common_dir "$pcwd")
    [ -n "$there_git" ] && [ "$there_git" = "$here_git" ] \
      || block "this merge names no repository and runs in $pcwd, which is not a worktree of the repository this guard reads PR #$pr in ($(pwd -P)), so gh may resolve another repository's PR #$pr. $pin_hint" ;;
esac

# Rule 14 — only an OPEN PR merges (OPS-132). A close is how one session tells another "do not land
# this", and the lane that opened the PR is not told: #215, closed as a duplicate, landed twelve
# minutes later through its own lane's merge. First, before any evidence is read: none of it matters
# for a PR that must not land. Anything but OPEN blocks, and so does no answer: gh failing, or
# answering with nothing, has told us nothing, which is not "open".
state_errf=$(mktemp "${TMPDIR:-/tmp}/guard-state.XXXXXX") || block "cannot make a temp file to read PR #$pr's state"
pr_state=$(ghpr view --json state --jq .state 2>"$state_errf") || pr_state=""
state_err=$(grep . "$state_errf" | head -n 1); rm -f "$state_errf"
[ -n "$pr_state" ] || block "cannot read PR #$pr's state (gh: ${state_err:-no answer}), so whether it is still open cannot be checked. Fix what gh reports (gh auth status, the PR number), then retry."
[ "$pr_state" = OPEN ] || block "PR #$pr is $pr_state, not OPEN (rule 14). A closed PR is another session's 'do not land this': find out why it was closed (gh pr view $pr_ref --comments) before anything else. If it should land after all, reopen it first; a merged one has nothing left to merge."

# Rule 2 — gate evidence for the exact head commit.
#
# (a) Anything that IS reported and is not passing blocks outright, whatever it is, EXCEPT the
# contexts named in GATE_DISPLAY_CONTEXTS (.claude/project.conf): statuses a project posts purely to
# draw the result on the PR. Those are excluded in BOTH directions. Without that, a display status
# feeds the gate one way only: a green one cannot satisfy (b) (correctly: it is not evidence), while
# a red one blocks, and a later successful remote run cannot clear it because it posts a different
# context. Every other check still blocks when it is red.
# gh normalizes each check into a `bucket`: pass | fail | pending | skipping | cancel.
# stdout alone is the answer when it is a JSON array (gh exits non-zero for pending checks and still
# prints it), whatever gh also wrote to stderr: GH_DEBUG or a notice there must not hide a red check.
# Otherwise stderr is gh saying why it has none, kept for GATE_PR_CHECKS below.
checks_errf=$(mktemp "${TMPDIR:-/tmp}/guard-checks.XXXXXX") || block "cannot make a temp file to read PR #$pr's checks"
checks_out=$(ghpr checks --json name,state,bucket 2>"$checks_errf")
checks_stderr=$(cat "$checks_errf"); rm -f "$checks_errf"
if printf '%s' "$checks_out" | jq -e 'type == "array"' >/dev/null 2>&1; then
  checks=$checks_out checks_err=""
else
  checks="" checks_err=$(printf '%s\n%s\n' "$checks_stderr" "$checks_out" | grep . | head -n 1)
  case "$checks_stderr$checks_out" in *"no checks reported"*) checks="[]" checks_err="" ;; esac
fi
display_re=$(printf '%s' "${GATE_DISPLAY_CONTEXTS:-}" | tr ' ' '\n' | grep . | sed 's/[][\.*^$+?(){}|]/\\&/g' | paste -sd'|' -)
[ -z "$display_re" ] && display_re='\u0000never-matches'
if [ -n "$checks" ] && [ "$checks" != "[]" ]; then
  notpass=$(printf '%s' "$checks" | jq --arg d "^($display_re)$" \
    '[ .[] | select(.name | test($d) | not)
           | select(.bucket as $b | ["pass","skipping"] | index($b) | not) ] | length')
  if [ "${notpass:-0}" -gt 0 ]; then
    summary=$(printf '%s' "$checks" | jq -r --arg d "^($display_re)$" \
      '[ .[] | select(.name | test($d) | not)
             | select(.bucket as $b | ["pass","skipping"] | index($b) | not)
             | "\(.name)=\(.bucket)" ] | join(", ")')
    block "PR #$pr has non-passing checks: $summary. Wait for green (gh pr checks $pr_ref --watch), then merge."
  fi
fi

# (a2) The checks this project REQUIRES on a PR, by name: GATE_PR_CHECKS (.claude/project.conf). An
# empty list reported by gh has nothing red in it, so (a) alone lets a merge land in the minutes
# before CI registers. Which checks a PR must have cannot be read reliably from workflow files
# (triggers in comments and `if:` expressions, `types:`, `branches:`, pull_request_target), so it
# is an explicit opt-in: empty, nothing changes. Each name must have reported AND passed on the
# head; a name matches a check of exactly that name, or that name followed by " (" (a matrix job:
# `test` matches `test (ubuntu-latest)`). A skipped required check is not a pass. The one excused:
# the bare name skipped beside a pass of a matrix job, since a draft's skipped run leaves a bare
# `test`=skipping on the head and the run `gh pr ready` starts reports `test (ubuntu-latest)` on
# the same head. A skipped matrix job (`test (e2e)`) still blocks.
if [ -n "${GATE_PR_CHECKS:-}" ]; then
  [ -z "$checks_err" ] || block "cannot read PR #$pr's checks (gh: $checks_err), and GATE_PR_CHECKS requires $GATE_PR_CHECKS to have passed. That is a gh problem (auth or network), not CI: fix it (gh auth status), then retry."
  missing="" notgreen=""
  for want in $GATE_PR_CHECKS; do
    got=$(printf '%s' "${checks:-[]}" | jq -r --arg w "$want" \
      '[ .[] | select(.name == $w or (.name | startswith($w + " ("))) | "\(.name)=\(.bucket)" ] | join(", ")')
    if [ -z "$got" ]; then missing="$missing $want"; continue; fi
    bad=$(printf '%s' "$checks" | jq -r --arg w "$want" \
      '[ .[] | select(.name == $w or (.name | startswith($w + " ("))) ] as $m
       | ($m | any(.bucket == "pass")) as $passed
       | [ $m[] | select(.bucket != "pass") | select(.bucket != "skipping" or .name != $w or ($passed | not))
           | "\(.name)=\(.bucket)" ] | join(", ")')
    [ -z "$bad" ] || notgreen="${notgreen:+$notgreen, }$bad"
  done
  [ -z "$missing" ] || block "PR #$pr has not reported the checks GATE_PR_CHECKS requires:$missing (CI has not registered them for the head commit yet, or the names in .claude/project.conf are wrong: compare gh pr checks $pr_ref). Wait (gh pr checks $pr_ref --watch), then merge."
  [ -z "$notgreen" ] || block "PR #$pr's required checks (GATE_PR_CHECKS) are not green: $notgreen. Wait for green (gh pr checks $pr_ref --watch), then merge."
  say "GATE_PR_CHECKS ($GATE_PR_CHECKS) reported and passed on PR #$pr."
fi

# (b) Then require evidence for THIS commit, asked for BY NAME. "All reported checks are green" is
# NOT evidence: when the real CI runs on demand, a PR may report exactly one unrelated check (a
# dependency audit) that passes in seconds, and the gate reads as satisfied by a job that never
# compiled a line. So: the named remote workflow's success on this SHA, or a local receipt. Naming
# the workflow is the authority, not a list of jobs; renamed, it stops finding runs and blocks,
# which is the safe direction. GATE_REMOTE_WORKFLOW="" means the local receipt is the only evidence.
head_sha=$(ghpr view --json headRefOid --jq .headRefOid 2>/dev/null)
if [ -z "$head_sha" ]; then
  # gh failed. We have learned nothing; do not read that as "no evidence needed".
  block "cannot read PR #$pr's head SHA (gh error?), so the gate evidence cannot be checked. Fix gh auth, then retry."
fi

ci_runs=0
if [ -n "$REMOTE_WF" ]; then
  # In the PR's own repository (run_repo, at ghpr): a fork's default repo has runs of its own.
  set -- run list --workflow "$REMOTE_WF" --commit "$head_sha" --json conclusion \
    --jq '[ .[] | select(.conclusion == "success") ] | length'
  [ -z "$run_repo" ] || set -- "$@" -R "$run_repo"
  ci_runs=$(gh "$@" 2>/dev/null)
fi
if [ "${ci_runs:-0}" -gt 0 ]; then
  say "$REMOTE_WF succeeded for $(printf %.9s "$head_sha")…: gate evidence satisfied."
else
  # A local run counts if it covered the whole gate and tested this exact commit. The receipt is
  # written by GATE_RUN into the git dir of the checkout it ran in, so it is per-clone and cannot
  # travel to another machine. lib/ci-receipt.sh reads the git dir of EVERY worktree of this
  # repository: a lane's run writes `.git/worktrees/<name>/ci-receipt`, which a lookup from the
  # hook's own cwd never sees. If the library is missing, BLOCK: `.` of a missing file would exit
  # with a non-2 status, which Claude Code reads as "allow".
  receipt_lib="$(dirname "$0")/lib/ci-receipt.sh"
  [ -f "$receipt_lib" ] || block "cannot find $receipt_lib, so the local gate receipt cannot be checked. Restore it."
  . "$receipt_lib"
  if receipt_msg=$(ci_receipt_verdict "$head_sha" "$pr"); then
    say "$receipt_msg"
  else
    block "$receipt_msg"
  fi
fi

# (c) Advisory: evidence proves this COMMIT was tested, not that it was tested against what merging
# would produce. A branch cut before a sibling PR merged carries a valid, SHA-exact receipt for a
# tree main has moved past, and `mergeable` answers a conflict question, not a tested-together one.
# Advisory, because a branch one docs commit behind does not need a re-run; the point is that it
# stops being invisible. Best-effort: offline, it says nothing.
git fetch origin "$MAIN_BRANCH" --quiet 2>/dev/null
behind=$(git rev-list --count "origin/$MAIN_BRANCH" "^$head_sha" 2>/dev/null)
case "$behind" in
  '' | 0) ;;
  *) say "PR #$pr is $behind commit(s) behind origin/$MAIN_BRANCH: its gate evidence was produced against a tree that is not what merging would produce. Consider: git merge origin/$MAIN_BRANCH && $GATE_RUN" ;;
esac

# Rules 3, 4, 7 and 8 read the PR's head branch and its tree at the head SHA.
head_json=$(ghpr view --json headRefName,headRefOid 2>/dev/null)
branch=$(printf '%s' "$head_json" | jq -r '.headRefName // empty' 2>/dev/null)
[ -n "$branch" ] || block "could not read PR #$pr's head branch, so rules 3, 7 and 8 cannot be evaluated. Retry once 'gh pr view $pr_ref --json headRefName' answers."

# The head commit, locally: rules 3, 7 and 8 read files at it from local git objects (no API size cap,
# nothing remote-tracking to be stale). Fetched once if missing; fails CLOSED if still missing.
if ! git cat-file -e "$head_sha^{commit}" 2>/dev/null; then
  git fetch origin "pull/$pr/head" --quiet 2>/dev/null
fi
have_head=""
git cat-file -e "$head_sha^{commit}" 2>/dev/null && have_head=1

# Rule 3 — a capability change must STATE its slice.
#
# THREE THINGS THIS RULE MUST NOT DO: derive the change id from the branch name (a continuation
# branch has no matching directory), read `origin/<branch>` (a local ref, stale either way), or
# fail open silently. The change is discovered from the PR's own diff: the change directories its
# files touch, archive excluded.
case "$branch" in
  "$BRANCH_CHANGE"*)
    [ -n "$have_head" ] || block "cannot find PR #$pr's head $head_sha locally, so rule 3 (the slice line) cannot be evaluated. Run: git fetch origin pull/$pr/head"
    base3=$(git merge-base "origin/$MAIN_BRANCH" "$head_sha" 2>/dev/null) || base3="origin/$MAIN_BRANCH"
    ids=$(git diff --name-only "$base3" "$head_sha" 2>/dev/null \
      | grep -E "^$CHANGES_DIR/[^/]+/|^openspec/changes/[^/]+/" | grep -v '/archive/' \
      | awk -F/ '{ print ($1 == "openspec" ? $1 "/" $2 "/" $3 : $1 "/" $2) }' | sort -u)
    if [ -z "$ids" ]; then
      say "PR #$pr ($branch) touches no $CHANGES_DIR/<id>/ directory: a continuation branch, so the slice rule does not apply. Not blocking."
    else
      found=""; checked=""
      for dir in $ids; do
        checked="$checked $dir/tasks.md"
        # Bare, bulleted, checkboxed and/or numbered, optionally bold: `Slice: …`, `- [ ] 5.4 Slice: …`, `**Slice:** …`.
        if git show "$head_sha:$dir/tasks.md" 2>/dev/null | grep -qE '^[[:space:]]*(-[[:space:]]*)?(\[[ xX]\][[:space:]]*)?([0-9]+(\.[0-9]+)*[[:space:]]+)?(\*\*)?Slice:'; then found=1; break; fi
      done
      [ -n "$found" ] || block "PR #$pr ($branch) has no slice line in:$checked
End tasks.md with ONE of:
    - [ ] Slice: a <role> can <action> at <where>
    - [ ] Slice: exempt — pure substrate (<reason>)
Do NOT write an exempt line just to clear the gate: exempt is for work with no user-facing surface by definition."
    fi
    ;;
esac

# Rule 4 — proposal inventory (ADVISORY). Not blocking: a second proposal can exist briefly during a
# handoff, and wedging a green merge over a filing question trains the habit of bypassing the guard.
# Counts DIRECTORIES only, so a stray README does not read as a proposed change.
if [ -d "$ROOT_HOOK/$CHANGES_DIR" ]; then
  names=$(find "$ROOT_HOOK/$CHANGES_DIR" -mindepth 1 -maxdepth 1 -type d ! -name archive -exec basename {} \; 2>/dev/null | sort | tr '\n' ' ')
  n4=$(printf '%s' "$names" | wc -w | tr -d ' ')
  [ "${n4:-0}" -gt 1 ] && say "$n4 changes proposed at once ($names). Propose just in time, at most one ahead: a proposal written before its turn goes stale invisibly. Not blocking."
fi

# Rule 7 — a journal session number is not claimed twice. BLOCKING.
#
# Every session-close writes `## Session N` with N = latest + 1, so two people's sessions that both
# read `Session 91` both write `Session 92`. The decision is lib/journal-sessions.sh's; this reads
# the three journals it compares from local git objects. Skipped when origin/<main> has no journal.
if git cat-file -e "origin/$MAIN_BRANCH:$JOURNAL" 2>/dev/null; then
  [ -n "$have_head" ] || block "cannot find PR #$pr's head $head_sha locally, so rule 7 (journal session numbers) cannot be evaluated. Run: git fetch origin pull/$pr/head"
  journal_base=$(git merge-base "origin/$MAIN_BRANCH" "$head_sha" 2>/dev/null) \
    || block "no merge base between origin/$MAIN_BRANCH and PR #$pr's head, so rule 7 cannot be evaluated."
  journal_lib="$(dirname "$0")/lib/journal-sessions.sh"
  [ -f "$journal_lib" ] || block "cannot find $journal_lib, so rule 7 (journal session numbers) cannot be checked. Restore it."
  . "$journal_lib"
  heads_of() { git show "$1:$JOURNAL" 2>/dev/null | grep -E '^## Session [0-9]+'; }
  if ! clash=$(journal_session_collisions "$(heads_of "$journal_base")" "$(heads_of "$head_sha")" \
                 "$(heads_of "origin/$MAIN_BRANCH")"); then
    block "PR #$pr's journal claims Session $clash, which another merged session already uses (rule 7).
Renumber this PR's entry to one past the top '## Session N' on origin/$MAIN_BRANCH, keep BOTH entries, re-run the gate ($GATE_RUN), then merge."
  fi
fi
# ...and only a close numbers a session (OPS-108). A row's PR writes its records to a fragment of its
# own ($JOURNAL_FRAGMENTS/<row id>.md), and the close folds the fragments into one entry, so a PR off
# BRANCH_SESSION_CLOSE whose journal has a heading numbered as neither the merge base nor
# origin/<main> numbers one (journal_sessions_numbered) is a row numbering a session nobody closed.
# A retitle, and main's own headings carried in by a merge of origin/<main>, number nothing. Off
# with rule 8 (BRANCH_SESSION_CLOSE ""): a project without a close pattern has nothing else that may
# number a session.
if [ -n "${BRANCH_SESSION_CLOSE:-}" ]; then
  # shellcheck disable=SC2254
  case "$branch" in
    $BRANCH_SESSION_CLOSE) ;;
    *)
      [ -n "$have_head" ] || block "cannot find PR #$pr's head $head_sha locally, so rule 7 (only a session close adds a '## Session N') cannot be evaluated. Run: git fetch origin pull/$pr/head"
      added_base=$(git merge-base "origin/$MAIN_BRANCH" "$head_sha" 2>/dev/null) \
        || block "no merge base between origin/$MAIN_BRANCH and PR #$pr's head, so rule 7 cannot be evaluated."
      journal_lib="$(dirname "$0")/lib/journal-sessions.sh"
      [ -f "$journal_lib" ] || block "cannot find $journal_lib, so rule 7 (journal session numbers) cannot be checked. Restore it."
      . "$journal_lib"
      command -v journal_sessions_numbered >/dev/null 2>&1 || block "$journal_lib did not define journal_sessions_numbered, so rule 7 cannot be checked. Run: sh -n $journal_lib"
      heads_at() { git show "$1:$JOURNAL" 2>/dev/null | grep -E '^## Session [0-9]+'; }
      added=$(journal_sessions_numbered "$(heads_at "$added_base")" "$(heads_at "$head_sha")" "$(heads_at "origin/$MAIN_BRANCH")")
      [ -z "$added" ] || block "PR #$pr ($branch) adds a journal session heading, and only a session close (BRANCH_SESSION_CLOSE: $BRANCH_SESSION_CLOSE) numbers a session (rule 7):
$(printf '%s\n' "$added" | head -3 | sed 's/^/    /')
Move the entry's content into this row's fragment, $JOURNAL_FRAGMENTS/<row id>.md (session-close folds the fragments into one entry), drop the heading, re-run the gate ($GATE_RUN), then merge."
      ;;
  esac
fi

# Rule 8 — a session close lands on top of origin/<main>. BLOCKING.
#
# A close PR is the one per session whose job is to leave the records (roadmap statuses, journal,
# owner queue, insights) true of what main IS. Its review and its gate judged the head's tree, but
# the squash lands that tree PLUS whatever merged since the branch was cut: a status set from a
# stale picture, or a record another close changed, lands unexamined with every other rule green.
# Requiring the head to contain origin/<main> makes the head's tree the merge result. Only the close
# (BRANCH_SESSION_CLOSE, a shell pattern; "" = off): blocking every behind PR would force a merge
# and a re-run into the middle of a build, which rule 2(c) only advises. The pattern must not match
# session TOOLING branches (`ops/session-start-…`), which are ordinary ops work. Fails CLOSED when
# there is no origin/<main> to compare with: the remedy is one command. The fetch repeats rule
# 2(c)'s and is best-effort: if it fails, an older local origin/<main> is what is compared (the
# merge itself, which needs the same network, then usually fails too).
# ...and any PR that changes JOURNAL or INSIGHTS, whatever its branch (OPS-108): only a close writes
# them now, so one that does it from a row's branch (a close riding a row's PR) is held the same way.
# Which files the PR changes is read from merge-base..head, so main's own edits merged in do not
# count; a head the guard cannot read fails closed, as above.
if [ -n "${BRANCH_SESSION_CLOSE:-}" ]; then
  why8=""
  # shellcheck disable=SC2254
  case "$branch" in
    $BRANCH_SESSION_CLOSE) why8="is a session close" ;;
    *)
      [ -n "$have_head" ] || block "cannot find PR #$pr's head $head_sha locally, so rule 8 (a PR that changes $JOURNAL or $INSIGHTS contains origin/$MAIN_BRANCH) cannot be evaluated. Run: git fetch origin pull/$pr/head"
      rec_base=$(git merge-base "origin/$MAIN_BRANCH" "$head_sha" 2>/dev/null) \
        || block "no merge base between origin/$MAIN_BRANCH and PR #$pr's head, so rule 8 cannot tell whether it changes $JOURNAL or $INSIGHTS."
      rec=$(git diff --name-only "$rec_base" "$head_sha" -- "$JOURNAL" "$INSIGHTS" 2>/dev/null | tr '\n' ' ')
      [ -n "$rec" ] && why8="changes ${rec% } (only a session close writes the journal and the insights log)"
      ;;
  esac
  if [ -n "$why8" ]; then
    [ -n "$have_head" ] || block "cannot find PR #$pr's head $head_sha locally, so rule 8 (a session close contains origin/$MAIN_BRANCH) cannot be evaluated. Run: git fetch origin pull/$pr/head"
    git fetch origin "$MAIN_BRANCH" --quiet 2>/dev/null
    git rev-parse --verify --quiet "origin/$MAIN_BRANCH" >/dev/null \
      || block "cannot read origin/$MAIN_BRANCH, so rule 8 cannot tell whether PR #$pr's head is what merging would produce. Run: git fetch origin $MAIN_BRANCH"
    git merge-base --is-ancestor "origin/$MAIN_BRANCH" "$head_sha" 2>/dev/null \
      || block "PR #$pr ($branch) $why8, and its head does not contain origin/$MAIN_BRANCH (rule 8). Its records and its gate evidence were checked against a tree that is not what merging would produce. Run: git merge origin/$MAIN_BRANCH, resolve per session-close's shared-file table, re-run the gate ($GATE_RUN), push, then merge."
    say "rule 8: PR #$pr ($branch) $why8, and its head contains origin/$MAIN_BRANCH ($(git rev-parse --short "origin/$MAIN_BRANCH" 2>/dev/null))."
  fi
fi


# Rules 9–12 — the change process, read at the head (lib/change-guard.sh). BLOCKING. The libraries
# are tested for, not sourced blind: a `.` of a missing file exits non-2, which reads as "allow".
cg_lib="$(dirname "$0")/lib/change-guard.sh"
ch_lib="$STRIDAL_FW/lib/change.sh"
{ [ -f "$cg_lib" ] && [ -f "$ch_lib" ]; } || block "cannot find $cg_lib or $ch_lib, so rules 9–12 (the change process) cannot be checked. Restore them."
# A library that does not parse may define some functions and not others, and an undefined
# function's empty output would read as "no reason to block": parse first, then prove each is there.
for f in "$ch_lib" "$cg_lib"; do sh -n "$f" 2>/dev/null || block "$f does not parse, so rules 9–12 cannot be checked. Run: sh -n $f"; done
. "$ch_lib"
. "$cg_lib"
# CHANGES_LEGACY (lib/records.sh): a grandfathered change is not held to the change-record rules
# this format added (rules 9–11, cg_legacy). Set but unreadable is refused: a silent strict or lax
# reading would both be guesses.
rec_lib="$STRIDAL_FW/lib/records.sh"
if [ -n "${CHANGES_LEGACY:-}" ]; then
  [ -f "$rec_lib" ] && sh -n "$rec_lib" 2>/dev/null || block "CHANGES_LEGACY is set but $rec_lib is missing or does not parse, so which changes are grandfathered cannot be told. Restore it."
  . "$rec_lib"
  command -v change_is_legacy >/dev/null 2>&1 || block "$rec_lib did not define change_is_legacy, so CHANGES_LEGACY cannot be read."
fi
for f in open_tasks cg_build_tasks cg_trace cg_archives cg_trailers cg_body cg_field; do
  command -v "$f" >/dev/null 2>&1 || block "$cg_lib or $ch_lib did not define $f (a syntax error?), so rules 9–12 cannot be checked. Run: sh -n $cg_lib"
done
[ -n "$have_head" ] || block "cannot find PR #$pr's head $head_sha locally, so rules 9–12 cannot be evaluated. Run: git fetch origin pull/$pr/head"
base9=$(git merge-base "origin/$MAIN_BRANCH" "$head_sha" 2>/dev/null) || base9="origin/$MAIN_BRANCH"
case "$branch" in
  "$BRANCH_CHANGE"*)
    why=$(cg_build_tasks "$base9" "$head_sha" "${ids:-}")
    [ -z "$why" ] || block "rule 9: $why"
    ;;
esac
why=$(cg_trace "$head_sha")
[ -z "$why" ] || block "rule 10: $why"
why=$(cg_archives "$base9" "$head_sha")
[ -z "$why" ] || block "rule 11: $why"
roles_json="$ROOT_HOOK/.claude/model-roles.json"
if jq -e '.provenance.enabled == true' "$roles_json" >/dev/null 2>&1; then
  why=$(cg_trailers "$cmd" "$(printf '%s' "$payload" | jq -r '.cwd // empty')" "$(printf '%s' "$payload" | jq -r '.session_id // empty')" "$roles_json")
  [ -z "$why" ] || block "rule 12: $why"
  say "rule 12: the squash body carries its provenance trailers."
fi

# Rule 15 — while attribution is off, a squash merge carries no Co-authored-by trailer (OPS-156).
# A squash that sets no body gets GitHub's default, the branch's commit messages with their
# trailers: that is how #289, #290 and #291, merged from Cursor, came to carry one. So a squash
# names its body, a file (--body-file) or a literal --body read the same way (cg_body), and no line
# of it is a Co-authored-by trailer: a line whose first word, past blanks, is the key, in any case,
# then its colon. A mention inside a line (prose, a code span) is no trailer, and is allowed: the
# squash body of this very rule's PR names the key. The switch is model-roles.json's
# attribution.enabled, the one build-change reads (project-config.sh): off unless it is true, while
# the file is there; a file that does not parse keeps the rule on. While it is off, ONLY a squash
# merges: a merge commit or a rebase lands every branch commit as it is, trailers and all, and one
# squash commit per PR is this model's convention anyway. gh pr merge names its method in words
# (merge_argv's W lines; none at all is held as a squash, which gh requires anyway when it cannot
# prompt); a gh api merge in its merge_method field (cg_field), and GitHub's default there is a
# merge commit, so an API merge must say squash. Self-contained, so it moves to .claude/merge.sh
# whole (ADR-0012, OPS-162): it needs cg_body and cg_field (lib/change-guard.sh) and the methods
# merge_argv read.
coauthor_why() {  # coauthor_why COMMAND CWD ROLES_JSON METHODS API: why to refuse; empty allows
  [ -f "$3" ] || return 0
  jq -e '.attribution.enabled == true' "$3" >/dev/null 2>&1 && return 0
  _off="while attribution is off (.claude/model-roles.json)"
  _fix="Write the squash body to a file with no Co-authored-by line and merge with: gh pr merge <n> --squash --delete-branch --body-file <file>"
  if [ -n "$5" ]; then
    _cm=$(cg_field "$1" "$2" method); _cmr=$?
    case "$_cmr" in
      0) _cm=$(printf '%s' "$_cm" | tr -d '[:space:]') ;;
      1) _cm=merge ;;   # the API's default
      *) echo "cannot read this gh api merge's merge_method, so whether it squashes cannot be checked; $_off only a squash merges. Pass it as plain text: -f merge_method=squash"; return ;;
    esac
    [ "$_cm" = squash ] || { echo "this gh api merge's method is '$_cm', not squash: a merge commit or a rebase lands every branch commit, Co-authored-by trailers and all, so $_off only a squash merges. Add -f merge_method=squash and a commit_message with no Co-authored-by line."; return; }
  else
    for _w in $4; do
      [ "$_w" = squash ] || { echo "this merge uses --$_w, not --squash: a merge commit or a rebase lands every branch commit, Co-authored-by trailers and all, so $_off only a squash merges. $_fix"; return; }
    done
  fi
  _cb=$(cg_body "$1" "$2"); _crc=$?
  case "$_crc" in
    0) ;;
    1) echo "this squash merge sets no body, so GitHub writes its default: the branch's commit messages, Co-authored-by trailers and all, $_off. $_fix"; return ;;
    3) echo "this merge names a relative --body-file after another command, which may have changed directory, so whether its body holds a Co-authored-by trailer cannot be checked. Write its path out in full, or run the merge on its own. $_fix"; return ;;
    4) echo "this merge names a --body-file after a command other than cd <path>, or beside a redirect, which may write the file after this check reads it, so the body gh sends cannot be checked. Run the merge on its own (or after only cd <path> &&). $_fix"; return ;;
    *) echo "cannot read this merge's body (a substitution in --body, or a --body-file that is not a readable regular file: stdin, /dev or /proc), so whether it holds a Co-authored-by trailer cannot be checked. $_fix"; return ;;
  esac
  _cl=$(printf '%s\n' "$_cb" | grep -Ein '^[[:space:]]*co-authored-by[[:space:]]*:' | head -n 1)
  [ -z "$_cl" ] || echo "the squash body holds a Co-authored-by trailer (line ${_cl%%:*}: ${_cl#*:}), $_off. Remove the line from the body. $_fix"
}
why=$(coauthor_why "$cmd" "$(printf '%s' "$payload" | jq -r '.cwd // empty')" "$roles_json" "$ways" "$api_pr")
[ -z "$why" ] || block "rule 15: $why"

# Rule 13 — a model choice rests on evidence (OPS-10). The evals library is tested for, parsed and
# proved complete before use, exactly as the change libraries above: a missing function's empty
# output would read as "no receipt needed".
ev_lib="$STRIDAL_FW/lib/evals.sh"
[ -f "$ev_lib" ] || block "cannot find $ev_lib, so rule 13 (eval receipts) cannot be checked. Restore it."
sh -n "$ev_lib" 2>/dev/null || block "$ev_lib does not parse, so rule 13 cannot be checked. Run: sh -n $ev_lib"
. "$ev_lib"
for f in evals_verdict evals_role_hash evals_triggers evals_section evals_triggers_at evals_input_hash_at evals_blob_at evals_tree_hash_at evals_suite_roles_at cg_eval_roles cg_eval_receipt cg_eval_policy; do
  command -v "$f" >/dev/null 2>&1 || block "$ev_lib or $cg_lib did not define $f, so rule 13 cannot be checked."
done
why=$(cg_eval_policy "$base9" "$head_sha" | tr '\n' ';' | sed 's/;$//; s/;/; /g')
[ -z "$why" ] || block "rule 13: this PR weakens rule 13 itself ($why). That is the owner's decision, not a receipt's: no eval run excuses it, and there is no approval marker for it, so Claude cannot merge this PR. The owner reviews and merges it on GitHub themselves."
roles13=$(cg_eval_roles "$base9" "$head_sha")
if [ -n "$roles13" ]; then
  suites13=$(evals_suite_roles_at "$head_sha")
  for r in $roles13; do
    if printf '%s\n' "$suites13" | grep -qx "$r"; then
      why=$(cg_eval_receipt "$head_sha" "$r")
      [ -z "$why" ] || block "rule 13: $why"
      say "rule 13: role $r is changed, and the head holds a passing eval receipt for it at the head's hashes."
    else
      say "rule 13: role $r is changed but has no eval suite (.claude/evals/$r/cases/), so no receipt can be required. Not blocking."
    fi
  done
fi

# Extension rules — the enabled modules' guard.d/*.sh, then the project's .claude/local/guard.d/*.sh
# (lib/modules.sh; .claude/local/README.md has the contract). Each runs as its own process, per its
# #! line, with the PR's facts in GUARD_* and the hook payload on stdin:
#   exit 0  allow; each stdout line is an advisory that reaches Claude, like `say`;
#   exit 2  BLOCK; stderr (else stdout) is the reason.
# Anything else blocks, FAIL CLOSED: a rule that does not parse, cannot start, crashes (exit 1),
# or outlives GUARD_RULE_TIMEOUT seconds (default 60) never reads as "no reason to block". The
# loader is tested for, not sourced blind; if it is missing while a guard.d exists, block.
mod_lib="$STRIDAL_FW/lib/modules.sh"
if [ -f "$mod_lib" ]; then
  sh -n "$mod_lib" 2>/dev/null || block "$mod_lib does not parse, so the extension rules (guard.d) cannot be found. Run: sh -n $mod_lib"
  . "$mod_lib"
  for f in ext_files shebang_interp shebang_parse with_timeout modules_problems; do
    command -v "$f" >/dev/null 2>&1 || block "$mod_lib did not define $f, so the extension rules (guard.d) cannot be run."
  done
  probs=$(modules_problems)
  [ -z "$probs" ] || block "a module in MODULES (.claude/project.conf) cannot load, so its guard rules cannot run: $(printf '%s' "$probs" | head -n 1)"
  rules=$(ext_files guard.d .sh)
elif [ -d "$ROOT_HOOK/.claude/local/guard.d" ] || [ -n "${MODULES:-}" ]; then
  block "cannot find $mod_lib, so the extension rules (MODULES, .claude/local/guard.d) cannot run. Restore it."
else
  rules=""
fi
if [ -n "$rules" ]; then
  gtmp=$(mktemp -d "${TMPDIR:-/tmp}/guard.XXXXXX") || block "cannot make a temp directory for the extension rules"
  printf '%s' "$payload" > "$gtmp/payload"
  TAB=$(printf '\t')
  while IFS="$TAB" read -r glabel gf; do
    gname="${glabel} guard.d/$(basename "$gf")"
    shebang_parse "$gf"; prc=$?
    [ "$prc" -eq 127 ] && { rm -rf "$gtmp"; block "extension rule $gname needs $(shebang_interp "$gf" | cut -d' ' -f1) (its #! line), which is not installed: it cannot run, so this refuses."; }
    [ "$prc" -eq 0 ] || { rm -rf "$gtmp"; block "extension rule $gname does not parse, so it cannot be checked: run $(shebang_interp "$gf" | cut -d' ' -f1) -n $gf"; }
    # A subshell that exports, not assignments before a function call: whether those reach the
    # function's children differs between shells.
    (
      GUARD_PR="$pr" GUARD_HEAD="$head_sha" GUARD_BRANCH="$branch" GUARD_BASE="$base9" GUARD_REPO="$this_repo"
      GUARD_COMMAND="$cmd" GUARD_PAYLOAD="$gtmp/payload" ROOT="$ROOT_HOOK"
      # How ghpr names the PR, so a rule reads the PR the merge names (#267): the URL or number,
      # and the merge's -R word (empty for a URL).
      GUARD_PR_REF="$gh_sel" GUARD_PR_REPO="$gh_repo"
      export GUARD_PR GUARD_HEAD GUARD_BRANCH GUARD_BASE GUARD_REPO GUARD_COMMAND GUARD_PAYLOAD ROOT MAIN_BRANCH GUARD_PR_REF GUARD_PR_REPO
      # shellcheck disable=SC2046
      with_timeout "${GUARD_RULE_TIMEOUT:-60}" $(shebang_interp "$gf") "$gf" <"$gtmp/payload" >"$gtmp/out" 2>"$gtmp/err"
    )
    grc=$?
    case $grc in
      0) while IFS= read -r l; do [ -n "$l" ] && say "$gname: $l"; done < "$gtmp/out" ;;
      2) why=$(cat "$gtmp/err"); [ -n "$why" ] || why=$(cat "$gtmp/out"); rm -rf "$gtmp"
         block "$gname: ${why:-it blocked without saying why}" ;;
      143 | 137) rm -rf "$gtmp"; block "extension rule $gname did not finish within ${GUARD_RULE_TIMEOUT:-60} s, so it learned nothing: this refuses rather than allow unchecked." ;;
      *) why=$(tail -n 3 "$gtmp/err"); rm -rf "$gtmp"
         block "extension rule $gname failed (exit $grc), so it learned nothing: this refuses rather than allow unchecked.${why:+ Its stderr: $why}" ;;
    esac
  done <<EOF
$rules
EOF
  rm -rf "$gtmp"
fi

allow
