# OPS-98 — GitHub Actions runs on PRs again: a draft runs nothing, and a merge waits for `test`

- **PR**: pending
- **Session**: S16
- **Review**: pending
- **Gate**: CANNOT CHECK — the session's lead runs the gate
- **Cost**: CANNOT CHECK — a lane shares the orchestrator's session id
- **Session start**: pending

## What happened

The repository went public on 2026-10-29 (OPS-155), so its standard runners are free, and the owner re-enabled `test.yml` and `release-build.yml` the same night (`gh workflow list --all` shows both active). Fork-PR workflow approval is set to all external contributors.

`test.yml` now skips a draft PR: the `pull_request` trigger lists `opened`, `synchronize`, `reopened` and `ready_for_review`, and the job carries `if: github.event_name != 'pull_request' || github.event.pull_request.draft == false`. Every row opens its PR as a draft (OPS-108), so a push to it spends no minutes, and marking it ready starts the suite. Pushes to `main` run as before (macOS and Ubuntu). `release-build.yml` is unchanged.

This repo's `GATE_PR_CHECKS` is `"test"` again. What it makes the merge guard require (`pr-merge-guard.sh` rule 2(a2)): on `gh pr merge`, the PR's head must have reported a check named `test` or `test (…)` (the matrix job, `test (ubuntu-latest)` on a PR), and every such check must be in the `pass` bucket. A check that has not registered yet blocks ("has not reported the checks GATE_PR_CHECKS requires: test"); one pending, failed or skipped blocks ("not green"), except a bare `test` skipped beside a passing `test (…)` (a draft's skipped run, then the ready PR's); a gh error reading the checks blocks as a gh problem, not CI. So a draft's skipped run never counts, and a merge waits for the ready PR's run. The guard's own cases for each state are in `checks/merge-guard.sh` (rule 2(a2)) and pass. The template's value stays empty (`CONF_NOT_READ` in `scripts/ci/steps.sh`).

OPS-60's Ubuntu leg: pending this PR's first run.

Billing standing: pending this PR's first run.

## Decisions

- The draft condition sits on the job, not on each step: a skipped job spends no runner time at all, where a step-level condition still starts a runner to skip every step.
- The ready PR's run is the one a merge reads; no rerun is needed after marking ready, because `ready_for_review` is a trigger of its own.

## Surprises

Review round 1 (peak high, cap 3) stopped the row; it moves to S18, since S17's rename needs a clean tree and the fixes reach `template/`. S18 starts from commit 0c4f62e2 and fixes:
- **High**: a draft's skipped run reports a bare `test`=skipping on the head, and rule 2(a2) (`pr-merge-guard.sh`, the `bad=` filter) blocks on it forever, even once `test (ubuntu-latest)` passes on the same head after `gh pr ready`; only a new push (and a new receipt) gets out. Fixed in S18: the bare name skipped no longer blocks once a matrix job of that name passed; a skipped matrix job (`test (e2e)`) still does (review round 2's medium: the first fix excused it too). Round 3 narrowed it to a pass of the same workflow (gh's `workflow` field). Fixtures in `checks/merge-guard.sh`: {bare skipping + matrix pass, one workflow} exits 0; {bare and `test (e2e)` skipping + pass}, {bare skip in e2e + matrix pass in ci} and {bare skip + bare pass, two workflows} exit 2; {skipping} alone exits 2. `checks/gate.sh`'s locked fixture now fails if the step ran in the checkout: #343's Ubuntu log showed `repo/locked`, which no path explains, so this PR's first Ubuntu run says where it ran (#343 owns it). The draft condition stays on the job.
- **Medium**: merge-pr waits for `GATE_PR_CHECKS` at step 2 but marks the PR ready only at step 4, so the wait never ends on a draft: `gh pr ready` moves into step 2, before the wait.
- **Low**: `docs/panel.md`'s CI paragraph has a 147-character line; rewrap it.
- Then confirm the real check names on S18's first draft-then-ready run (`gh pr checks <n> --json name,state,bucket` on the head before and after marking ready).

## Insights

None.
