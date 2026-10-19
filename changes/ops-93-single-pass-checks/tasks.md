# Tasks: ops-93-single-pass-checks

## Progress
- 2026-10-18 proposed; nothing built yet. No spec delta (`.openspec.yaml`): the tests are the
  checks' own fixtures and the plugin test named below.
- 2026-10-18 pre-review (propose step 2b), for the proposal PR's body:
  Review: converged in 3 round(s), peak low, at 9e98b38 · design 78e8b98101ab
  - Rounds: 1 peak medium (2 medium, 4 low, fixed), 2 peak medium (2 medium, 4 low, fixed in
    9e98b38), 3 peak low. Round 3's two lows are left open on purpose for the builder: L1 edit
    `template/AGENTS.md` before the plugin rebuild; L2 `modules.sh`'s whole-set fixture inherits
    the markers.
- 2026-10-18 approved by the owner (design 78e8b98101ab).
- 2026-10-19 group 1: 1.1 done (baseline below). 1.2 to 1.6 not started: the builder's Edit of
  `template/.claude/checks/lib.sh` was refused as a sensitive file (a permission prompt no one can
  answer inside the workflow). Every group-1 file is under `.claude/`, so the group needs a run
  that may edit `template/.claude/checks/` (and, for 1.6, `.claude/` via `clauductor update`).
  Nothing else is edited. Notes for the resumed run, not yet decisions: the plugin builder keeps
  comment lines verbatim (`rewrite.go` `shell`), so comment markers around run.sh's wrap let the
  D7 fixture derive "run.sh with the wrap removed" with `sed`; `modules.sh:68,89,92` start
  whole-set fixture runs and need the markers cleared (pre-review L2); a quick way to see the
  fixtures fail first is `CHECKS_SET=whole sh template/.claude/checks/no-clauductor.sh` (part 2
  fails fast, the rest runs).

## Decision log
- 2026-10-19, group 1, task 1.1 baseline (worktree at bbc23074, Apple M5, 10 cores, before any
  edit; one set at a time, nothing else of this build running): `sh template/.claude/checks/run.sh`
  698 s, 44 checks; `run.sh no-clauductor` alone 354 s (51%). `sh .claude/checks/run.sh` 766 s,
  47 checks; `run.sh no-clauductor` alone 358 s (47%). The share was measured as a separate named
  run, not from inside the whole run (run.sh prints no per-check time), rather than adding timing
  to run.sh for a one-off; it agrees with the amendment's "half the set".

## 1. The single pass (D1, D2, D3, D4, D6, D7)
- [x] 1.1 Before any edit, time a whole-set `sh template/.claude/checks/run.sh` and `sh .claude/checks/run.sh` on the owner's Mac, and record both totals and `no-clauductor`'s share in the Decision log as the baseline. (manual: a one-off measurement on the owner's machine, not shipped code)
- [ ] 1.2 `template/.claude/checks/lib.sh`: move `no-clauductor.sh:20-39` unchanged into one helper, which builds the `PATH` mirror, the fresh `HOME` with its `.gitconfig`, and `bare` (D2). Add D3's assertion as a function: `command -v clauductor` fails, `$HOME` is the environment's `HOME`, and `$HOME/.clauductor` is absent.
- [ ] 1.3 `template/.claude/checks/run.sh`: for a whole-set run with no `CHECKS_DOUBLE_PASS=1`, source `lib.sh` after `ROOT` is set, build the environment, and run itself as a child under it with `CHECKS_BARE` set. Exit with the child's code, and let the `EXIT` trap remove the scratch. Export `CHECKS_SET=whole` for every whole-set run. A whole-set run that finds `CHECKS_BARE` set runs the checks directly: that is the child's stop condition. Named runs and `CHECKS_DOUBLE_PASS=1` do not wrap (D1, D4). Update the header comment to say so.
- [ ] 1.4 `template/.claude/checks/no-clauductor.sh`: use the helper. Part 2 asserts under `CHECKS_BARE`, fails under `CHECKS_SET=whole` with no environment, and re-runs the set otherwise (D3). The re-run clears `CHECKS_SET` and sets `CHECKS_BARE` to its own environment. It runs `$dir/run.sh` (D6). Rewrite the header's step 2 to say all this.
- [ ] 1.5 The fixtures in `template/.claude/checks/no-clauductor.sh` (D7), each shown failing first, by reverting the code it covers and recording the failure in the Decision log. Each fixture run clears `CHECKS_BARE`, `CHECKS_SET` and `CHECKS_DOUBLE_PASS`, then sets only what its case needs, so it behaves the same inside the gate's single pass and inside the weekly double pass:
  - a fixture check that calls `clauductor` fails a whole-set run of the fixture `run.sh`, with a stub `clauductor` on the outer `PATH`;
  - a fixture `run.sh` with the wrap removed fails D3's assertion;
  - a fixture check that fails only with `clauductor` on `PATH` passes the single pass and fails under `CHECKS_DOUBLE_PASS=1`.
- [ ] 1.6 Bring the copies along and measure:
  - run `scripts/build-plugin.sh`, then `framework/clauductor update` from the repo root, with a binary built from this checkout;
  - `sh .claude/checks/run.sh` and `sh template/.claude/checks/run.sh` pass. Time both on the Mac, and record the totals against 1.1's baseline in the Decision log. If they contradict the amendment's figures, stop and ask the owner (design, *What rests on sources outside the repo*);
  - `CHECKS_DOUBLE_PASS=1 sh .claude/checks/run.sh` and `CHECKS_DOUBLE_PASS=1 sh template/.claude/checks/run.sh` pass. This proves the rollback on the real sets, and is what the weekly job runs (D4, D7);
  - `cd framework && go test -count=1 -run 'TestPluginChecksPassInScaffoldedProject|TestCommittedPluginIsCurrent' ./internal/plugin/ ./internal/cmd/` passes. This is the plugin layout's wrapped whole-set run (D6).

## 2. The weekly double pass and the record (D5, D8, D9)
- [ ] 2.1 `.github/workflows/checks-double-pass.yml` (D5):
  - a weekly `schedule:` (the cadence the owner chose in O3) and `workflow_dispatch`, on Ubuntu, with `permissions: contents: read` and actions pinned to `test.yml`'s SHAs;
  - copy `test.yml`'s setup steps (Go, tmux and the OpenSpec CLI), and set `OPENSPEC_REQUIRED=1` and `NODE_REQUIRED=1` as its process-checks step does;
  - build `clauductor` from `framework/` onto `PATH`;
  - run `CHECKS_DOUBLE_PASS=1 sh .claude/checks/run.sh` and `CHECKS_DOUBLE_PASS=1 sh template/.claude/checks/run.sh`;
  - a header comment naming its reader, `.claude/health/scheduled-workflows.sh`.
- [ ] 2.2 `.claude/local/checks/double-pass.sh` (D5): it fails when the workflow is missing, has no `schedule:`, does not set `CHECKS_DOUBLE_PASS=1`, does not put a built `clauductor` on `PATH` before the run, or does not run both sets. It is falsified on fixture copies of the workflow, one per missing piece, each shown failing first. `sh .claude/checks/run.sh local:double-pass` passes.
- [ ] 2.3 Insert design.md's amendment block verbatim into `docs/adr/0006-a-repository-works-without-clauductor.md`, between `## Enforcement` and `## Related` (D9), with the heading date the owner chose in O1. Prove it word for word: extract the fenced block from design.md and from the ADR's amendment section, and `cmp` the two (they differ only where the owner's answers to O1, O2 or O6 changed a word). Run `sh .claude/checks/run.sh adr-numbering`.
- [ ] 2.4 `template/docs/playbook.md` and `docs/playbook.md`: change the no-clauductor bullet to say the checks run once inside that environment, and name `CHECKS_DOUBLE_PASS=1` as the switch back to the double pass (D4, O5). Then rebuild the plugin and run `clauductor update` again. In `template/AGENTS.md` and this repo's `AGENTS.md`, replace the second cell of the row "The repo works without clauductor" with D9's words for that copy. `sh .claude/checks/run.sh agents-md-budget` and `sh template/.claude/checks/run.sh agents-md-budget` pass.
- [ ] 2.5 The release-notes item (D8), in `docs/roadmap.md`: if REL-1's row is still open, append the item to its Change cell; if REL-1 has merged, add a queued Phase 4 row whose Change cell says the next release's `CHANGELOG.md` section carries the item, in *Changed*. Then `sh .claude/roadmap-queue.sh --check` passes. If `CHANGELOG.md` is on `main` and a version section is being prepared, add the item there too, never under `[Unreleased]`.
- [ ] 2.6 The full gate passes (`GATE`). The Mac's before and after totals stay in the Decision log, which the build PR carries.

- [ ] Slice: a contributor runs `sh .claude/checks/run.sh` and every check runs once, inside the no-clauductor environment, in about half the time; `CHECKS_DOUBLE_PASS=1` brings back the double pass, which runs weekly on `main`
