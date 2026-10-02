# Optional module: the premise check

An issue's write-up is a claim about the code, made once. A fix that starts from a write-up the
code no longer matches fixes the wrong thing, and every test and review still passes, because none
of them looks at the premise. This module checks it before the fix starts, and makes the check a
condition of merging the fix.

**Off by default.** Needs git and jq; gh to read issues and PRs. No Node: the script is POSIX sh.
It comes from Standing Tee (`infra/premise-check.mjs` and merge-guard rule 6), and its report is
byte-for-byte the same as the original's.

## What it does

`sh .claude/modules/premise-check/premise-check.sh <issue>...` reads each issue's title, body and
comments, pulls out every named thing (inline `code` spans and "quoted strings"), and reports
against `origin/<MAIN_BRANCH>` (fetched first; `--at <ref>` for another):

- where each named thing lives now, with its line, and whether that file changed since filing;
- the commits that added or removed it (`git log -S`), dated relative to the filing, before as well
  as after (an issue can be wrong at birth). Code commits come first; docs only if no code commit
  ever touched it (`PREMISE_DOC_DIRS`);
- for a path, every file it can mean, whether each changed since filing, and for a path that is
  gone, the commit that deleted or moved it;
- what it could not check (prose, a command, a value, a term too common to be a claim), named.

It cannot judge a negative claim ("the message doesn't mention X"): it prints the line, and you
judge. The last line is the receipt, `premise-check: #N @ <sha>`, one per issue.
`--issue-json <file>` reads an issue from a file instead (no network).

## What it enforces

`guard.d/premise.sh` runs in the merge guard: a PR on a `PREMISE_REQUIRED_ON` branch (default
`fix/`) is **blocked** unless its body carries a receipt for every issue it fixes. "Every issue"
is the union of whatever names one: GitHub's closing references (this repository's), the PR body's
closing keywords (GitHub's field can read empty at merge time), the squash commit's subject
(`--subject`/`-t`, or an API merge's `commit_title`) and body (`--body`/`--body-file`, or
`commit_message`; with none, the branch's commit messages, GitHub's default squash body), the `#N`
in the PR title, and the number in a `fix/<n>-<slug>` branch. **`<n>` must be the issue number**:
a date-shaped prefix (`fix/2026-10-01-cleanup`) is not read as one, but any other leading number
is. A PR naming no issue is allowed, with a note; a merge message it cannot read blocks. It checks that the receipt is there, not that the check ran before the fix or
that anyone read it.

## Switching it on

1. Set `MODULES="premise-check"` (with any others) in `.claude/project.conf`. Optional:
   `PREMISE_REQUIRED_ON="fix/ hotfix/"`, `PREMISE_DOC_DIRS="docs openspec"` (the defaults are in
   `module.conf`).
2. `sh .claude/modules/premise-check/enable.sh` adds one sentence to every panel fix lane's first
   prompt (`.clauductor/panel.json`, from `panel/fix-prompt.txt`), so a fix lane starts from the
   check. Without a panel there is nothing to do. jq rewrites the file, so its layout becomes jq's.
3. `sh .claude/checks/run.sh premise-check:project` checks the wiring: the script reports on this
   project, the rule parses, and every fix lane runs the check.

With the clauductor plugin the module lives in the plugin, not in the repository: run it through
`sh scripts/ci/clauductor-model.sh modules/premise-check/premise-check.sh <issue>`; the guard's
message and `enable.sh` name that form.

## Tested

`.claude/checks/premise-check.sh` (whether the module is on or not) ports Standing Tee's vitest
suite case for case: the synthetic repo that reproduces its #239, the receipt and closing-keyword
library, and the rule through the real merge guard.
