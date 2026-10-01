#!/bin/sh
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
#   7. A PR is blocked if its journal claims a `## Session N` another merged session already uses.
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
#   Rules 9–13 live in lib/change-guard.sh.
#   (Numbering follows the rules this was extracted from; 5, 6 and 8 were project-specific.)
#
# A MERGE IN ANOTHER REPOSITORY is not policed beyond rule 1, but it passes only when the whole
# command matches one allowlisted plain shape (foreign_shape, below) and its -R owner/name differs
# from this repo's origin; any other command naming another repo blocks. A merge of THIS repo whose
# PR is not a number or PR URL (flag-first forms, or no PR at all) BLOCKS.
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

ROOT_HOOK=$(cd "$(dirname "$0")/../.." 2>/dev/null && pwd)
ROOT=$ROOT_HOOK
# A `.` of a missing file is fatal with a shell-dependent status, which Claude Code may read as
# "allow": test first, and fall back to the defaults.
if [ -f "$ROOT_HOOK/.claude/lib/conf.sh" ]; then
  # shellcheck disable=SC1091
  . "$ROOT_HOOK/.claude/lib/conf.sh"
else
  MAIN_BRANCH=main JOURNAL=docs/development-journal.md CHANGES_DIR=changes BRANCH_CHANGE=change/ GATE_RUN=scripts/ci/run-local.sh GATE_REMOTE_WORKFLOW=
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
# `pr` before one. Exit 0 = may be a merge.
merge_hint() {
  printf '%s' "$1" | tr -d "\\\\'\"" | tr '\t\r\n' '   ' | tr -s ' ' | grep -Eq \
    -e '(^|[^A-Za-z0-9_-])pr( -[^ ]+( [^ -][^ ]*)?)* merge([^A-Za-z0-9_-]|$)' \
    -e '([)}`]|[$][A-Za-z0-9_@*#?!-]+) merge([^A-Za-z0-9_-]|$)' \
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
#     session, a shell alias from a dotfile (`gh m 5` reads as subcommand `m`). Setting either
#     through this hook blocks (the expansion is a mention beside an unlisted command);
#   - an evaluator whose PROGRAM is computed or piped (`sh -c "$(…)"`, `printf … | sh`,
#     `eval "$(… | base64 -d)"`, `source <(…)`) unless its text names a merge: a hook that reads text
#     cannot run arbitrary programs;
#   - a merge through any other client (curl to the API, a GraphQL query in a file passed some
#     other way), and a click on the website.
# Checked by .claude/checks/merge-guard.sh, with jq and without.
#
# merge_hint (step 2) is defined above unreadable(), which runs before this point and uses it.

# merge_scan — lib/merge-reader.awk on $cmd. Its header lists what it prints.
READER="$(dirname "$0")/lib/merge-reader.awk"
merge_scan() {
  [ -f "$READER" ] || return 1
  printf '%s' "$cmd" | awk -f "$READER"
}
PLAIN_FORM='gh pr merge <n> --squash --delete-branch — gh, pr and merge unquoted and unescaped, nothing substituted, one merge per command'

# Step 1 — read. The reader gets the command exactly as the shell will.
scan=$(merge_scan); scan_ok=$?

# Every text parser below (the backstop, rule 1, foreign_shape, merge_argv) reads the command with
# each backslash-newline deleted, as the shell joins them: `gh pr mer\<newline>ge 5` is `merge`.
# A backslash that is itself escaped is joined too — that only ever ADDS text matches.
cmd=$(printf '%s' "$cmd" | awk '{ b = b $0 "\n" } END { s = substr(b, 1, length(b) - 1); gsub(/\\\n/, "", s); printf "%s", s }')

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
api_repo=""; api_pr=""
if [ -n "$apis" ]; then api_repo=${apis% *}; api_pr=${apis##* }; fi

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
# that stops at `$`, a backtick, a `#` comment, a redirect or a separator:
#   - more than one `gh pr merge` (any blanks between the words) BLOCKS;
#   - a repo named (-R X, -RX, --repo X, --repo=X, or a PR URL) that is not this repo, outside the
#     allowlisted shape, BLOCKS and shows the shape;
#   - the PR is the first positional word (values of gh's value-taking flags skipped, `--` ends
#     flags): a number or this repo's PR URL. Anything else, including no PR at all (the
#     current-branch form), BLOCKS. Reading only "the integer right after `merge`", with none
#     found meaning ALLOW, left flag-first forms and PR URLs unpoliced.
#
# Repos compare as OWNER/NAME, lowercased, HOST IGNORED — gh treats any *.github.com host as
# github.com, and an ssh alias (`git@github-work:o/n`) is still this repo. THIS repo is the origin
# remote (https, ssh or scp form), never a hard-coded name; if it cannot be read, nothing is
# foreign. A merge naming no repo is this repo's (`cd ../other && gh pr merge 2` is policed — pass
# -R). GH_REPO / GH_HOST are not read.
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
# the PR argument. For POLICING only: it decides which PR rules 2-8 check and whether a foreign repo
# is named at all. It never decides that a merge is foreign — foreign_shape does, by allowlist.
merge_argv() {
  printf '%s' "$cmd" | awk '
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
        if (c == "$" || c == "`") break
        if (c == "\\" && j < n) { j++; if (substr(s, j, 1) != "\n") { tok = tok substr(s, j, 1); have = 1 }; continue }
        if (q == "\"") { if (c == "\"") q = ""; else tok = tok c; continue }
        if (c == "#" && !have) break
        if (c == "\047" || c == "\"") { q = c; have = 1; continue }
        if (c == " " || c == "\t") { if (have) { t[++nt] = tok; tok = ""; have = 0 }; continue }
        if (c ~ /[\n;&|()<>]/) {
          if ((c == "<" || c == ">") && tok ~ /^[0-9]*$/) have = 0
          break
        }
        tok = tok c; have = 1
      }
      if (q == "" && have) t[++nt] = tok
      pos = 0; endopts = 0
      for (k = 1; k <= nt; k++) {
        x = t[k]; gsub(/\n/, " ", x)
        if (!endopts && x == "--") { endopts = 1; continue }
        if (!endopts && x ~ /^-/) {
          if (x == "-R" || x == "--repo") { if (k < nt) { y = t[++k]; gsub(/\n/, " ", y); print "R " y } else print "R ?"; continue }
          if (x ~ /^--repo=/) { print "R " substr(x, 8); continue }
          if (x ~ /^-R/) { x = substr(x, 3); sub(/^=/, "", x); print "R " x; continue }
          if (x ~ /^(-b|--body|-F|--body-file|-t|--subject|--match-head-commit|-A|--author-email)$/) k++
          continue
        }
        if (!pos) { pos = 1; print "P " x }
      }
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
if [ -n "$api_pr" ]; then shaped=$(repo_key "$(api_foreign_shape)"); else shaped=$(repo_key "$(foreign_shape)"); fi
if [ -n "$this_repo" ] && [ -n "$shaped" ] && [ "$shaped" != "$this_repo" ]; then
  say "merge targets $shaped, not this repo ($this_repo). This guard polices only this repo's merges beyond --auto/--admin; allowing."
  allow
fi

if [ -n "$api_pr" ]; then
  argv="P $api_pr"
  [ "$api_repo" = "." ] || argv="R $api_repo
$argv"
else
  argv=$(merge_argv)
fi
many=""; own=""; other=""; unparsed=""; pr_arg=""; pr=""
while IFS= read -r line; do
  case "$line" in
    M) many=1 ;;
    "R "* | "P http://"* | "P https://"*)
      named=${line#? }
      key=$(repo_key "$named")
      if [ -z "$key" ]; then unparsed=$named
      elif [ "$key" = "$this_repo" ]; then own=1
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
  http://* | https://*) pr=$(printf '%s\n' "$pr_arg" | sed -n 's|^[^?#]*/pull/\([0-9][0-9]*\)\([/?#].*\)\{0,1\}$|\1|p') ;;
  *) case "$pr_arg" in '' | *[!0-9]*) ;; *) pr=$pr_arg ;; esac ;;
esac
[ -n "$pr" ] || block "cannot tell which PR this merges${pr_arg:+ ('$pr_arg' is not a PR number or PR URL)}, so its CI evidence cannot be checked. Name it: gh pr merge <n> --squash --delete-branch"

# Rule 2 — gate evidence for the exact head commit.
#
# (a) Anything that IS reported and is not passing blocks outright, whatever it is, EXCEPT the
# contexts named in GATE_DISPLAY_CONTEXTS (.claude/project.conf): statuses a project posts purely to
# draw the result on the PR. Those are excluded in BOTH directions. Without that, a display status
# feeds the gate one way only: a green one cannot satisfy (b) (correctly: it is not evidence), while
# a red one blocks, and a later successful remote run cannot clear it because it posts a different
# context. Every other check still blocks when it is red.
# gh normalizes each check into a `bucket`: pass | fail | pending | skipping | cancel.
checks=$(gh pr checks "$pr" --json name,state,bucket 2>/dev/null)
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
    block "PR #$pr has non-passing checks: $summary. Wait for green (gh pr checks $pr --watch), then merge."
  fi
else
  # Nothing reported. Right after a push CI has not registered yet, and an empty list has nothing
  # red in it, so (a) alone would let a merge land before CI exists. Where this project HAS CI,
  # nothing reported is not a pass: GATE_DISPLAY_CONTEXTS or GATE_REMOTE_WORKFLOW names some, or a
  # workflow in .github/workflows runs on pull_request for every path. A project with none of those
  # has no checks to wait for, and keeps merging on its receipt alone.
  ci_expected=""
  [ -n "${GATE_DISPLAY_CONTEXTS:-}" ] && ci_expected="GATE_DISPLAY_CONTEXTS names $GATE_DISPLAY_CONTEXTS"
  [ -z "$ci_expected" ] && [ -n "$REMOTE_WF" ] && ci_expected="GATE_REMOTE_WORKFLOW names $REMOTE_WF"
  if [ -z "$ci_expected" ]; then
    for wf in "$ROOT"/.github/workflows/*.yml "$ROOT"/.github/workflows/*.yaml; do
      [ -f "$wf" ] || continue
      grep -Eq '(^|[^A-Za-z_])pull_request([^A-Za-z_]|$)' "$wf" || continue
      # A path filter may rightly skip this PR, and then nothing would ever report: not a requirement.
      grep -Eq '^[[:space:]]*paths(-ignore)?[[:space:]]*:' "$wf" && continue
      ci_expected="${wf#"$ROOT"/} runs on pull_request"; break
    done
  fi
  [ -n "$ci_expected" ] && block "PR #$pr has no reported checks yet, but this project has CI ($ci_expected): it has not registered for the head commit. Wait for it (gh pr checks $pr --watch), then merge."
fi

# (b) Then require evidence for THIS commit, asked for BY NAME. "All reported checks are green" is
# NOT evidence: when the real CI runs on demand, a PR may report exactly one unrelated check (a
# dependency audit) that passes in seconds, and the gate reads as satisfied by a job that never
# compiled a line. So: the named remote workflow's success on this SHA, or a local receipt. Naming
# the workflow is the authority, not a list of jobs; renamed, it stops finding runs and blocks,
# which is the safe direction. GATE_REMOTE_WORKFLOW="" means the local receipt is the only evidence.
head_sha=$(gh pr view "$pr" --json headRefOid --jq .headRefOid 2>/dev/null)
if [ -z "$head_sha" ]; then
  # gh failed. We have learned nothing; do not read that as "no evidence needed".
  block "cannot read PR #$pr's head SHA (gh error?), so the gate evidence cannot be checked. Fix gh auth, then retry."
fi

ci_runs=0
if [ -n "$REMOTE_WF" ]; then
  ci_runs=$(gh run list --workflow "$REMOTE_WF" --commit "$head_sha" --json conclusion \
    --jq '[ .[] | select(.conclusion == "success") ] | length' 2>/dev/null)
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

# Rules 3, 4 and 7 read the PR's head branch and its tree at the head SHA.
head_json=$(gh pr view "$pr" --json headRefName,headRefOid 2>/dev/null)
branch=$(printf '%s' "$head_json" | jq -r '.headRefName // empty' 2>/dev/null)
[ -n "$branch" ] || block "could not read PR #$pr's head branch, so rules 3 and 7 cannot be evaluated. Retry once 'gh pr view $pr --json headRefName' answers."

# The head commit, locally: rules 3 and 7 read files at it from local git objects (no API size cap,
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


# Rules 9–12 — the change process, read at the head (lib/change-guard.sh). BLOCKING. The libraries
# are tested for, not sourced blind: a `.` of a missing file exits non-2, which reads as "allow".
cg_lib="$(dirname "$0")/lib/change-guard.sh"
ch_lib="$ROOT_HOOK/.claude/lib/change.sh"
{ [ -f "$cg_lib" ] && [ -f "$ch_lib" ]; } || block "cannot find $cg_lib or $ch_lib, so rules 9–12 (the change process) cannot be checked. Restore them."
# A library that does not parse may define some functions and not others, and an undefined
# function's empty output would read as "no reason to block": parse first, then prove each is there.
for f in "$ch_lib" "$cg_lib"; do sh -n "$f" 2>/dev/null || block "$f does not parse, so rules 9–12 cannot be checked. Run: sh -n $f"; done
. "$ch_lib"
. "$cg_lib"
# CHANGES_LEGACY (lib/records.sh): a grandfathered change is not held to the change-record rules
# this format added (rules 9–11, cg_legacy). Set but unreadable is refused: a silent strict or lax
# reading would both be guesses.
rec_lib="$ROOT_HOOK/.claude/lib/records.sh"
if [ -n "${CHANGES_LEGACY:-}" ]; then
  [ -f "$rec_lib" ] && sh -n "$rec_lib" 2>/dev/null || block "CHANGES_LEGACY is set but $rec_lib is missing or does not parse, so which changes are grandfathered cannot be told. Restore it."
  . "$rec_lib"
  command -v change_is_legacy >/dev/null 2>&1 || block "$rec_lib did not define change_is_legacy, so CHANGES_LEGACY cannot be read."
fi
for f in open_tasks cg_build_tasks cg_trace cg_archives cg_trailers; do
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

# Rule 13 — a model choice rests on evidence (OPS-10). The evals library is tested for, parsed and
# proved complete before use, exactly as the change libraries above: a missing function's empty
# output would read as "no receipt needed".
ev_lib="$ROOT_HOOK/.claude/lib/evals.sh"
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
mod_lib="$ROOT_HOOK/.claude/lib/modules.sh"
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
      export GUARD_PR GUARD_HEAD GUARD_BRANCH GUARD_BASE GUARD_REPO GUARD_COMMAND GUARD_PAYLOAD ROOT MAIN_BRANCH
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
