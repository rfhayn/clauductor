# Optional module: ci-status

When the gate runs locally (`GATE_RUN`), a pull request shows no checks at all: the evidence, the
receipt in `.git/ci-receipt`, is real, and nothing renders it. This module draws the verdict on the
PR as a **commit status** (`ci/local`), with gh's ordinary token and no Actions minutes.

**It is display. It is never evidence.** The merge guard decides on the receipt or a named remote
workflow's success (rule 2(b)) and ignores every context in `GATE_DISPLAY_CONTEXTS` in both
directions (rule 2(a)): a green status cannot satisfy it, and a red one cannot block. Anyone with
write access can post a status by hand, so a guard that counted one could be passed with one curl.
`publish-status.sh`'s header has the whole argument.

**Off by default.** Nothing here runs until `MODULES` names it.

## Switching it on

Add `ci-status` to `MODULES` in `.claude/project.conf`. Its defaults (`module.conf`) then apply
wherever project.conf sets nothing: `GATE_DISPLAY_CONTEXTS="ci/local ci/github"`, the contexts, and
the descriptions. Write your own descriptions in `CI_STATUS_LOCAL_PASS` and `CI_STATUS_LOCAL_FAIL`
(140 characters at most): say where the result came from, never just "passed", because the reader
who matters is a human deciding from a browser.

## When it posts

| Run | Status on the tested commit |
|---|---|
| full, passed, clean tree | `ci/local` = success |
| full, passed, tree changed or HEAD moved during the run | none: the receipt says `dirty`, and a status has no such field |
| full, failed (including a lost gate lease) | `ci/local` = failure: evidence and picture are retracted together |
| `--dirty` or `--quick` | none: neither speaks for the commit |

The model's runner calls it through `model_publish_status` in `scripts/ci/lib/steps.sh`, a no-op
while the module is off. A project with its own runner calls that function, or the script:
`sh .claude/modules/ci-status/scripts/publish-status.sh local pass|fail` with `SHA` set to the
commit it tested.

A remote workflow's report job may post `ci/github` the same way, with each job's result:
`sh .claude/modules/ci-status/scripts/publish-status.sh github verify=${{ needs.verify.result }} e2e=${{ needs.e2e.result }}`
(it needs `statuses: write`). Success only when every job's result is `success`: `skipped` and
`cancelled` are not.

**It never fails its caller and never goes quiet**: no gh, no auth, a commit not yet pushed, a
context missing from `GATE_DISPLAY_CONTEXTS` or a failed POST each print one `publish-status:` line
and exit 0.

## Checks

- `checks/display.sh` (as `ci-status:display`, while on): every context it posts is in
  `GATE_DISPLAY_CONTEXTS`, and `GATE_RUN` really calls it.
- `.claude/checks/ci-status.sh` (always): the payload for each verdict, the refusals, the runner's
  calls in each row of the table above, and that the merge guard ignores the statuses both ways.
