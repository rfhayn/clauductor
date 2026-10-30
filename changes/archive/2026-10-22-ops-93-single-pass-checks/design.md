# Design: ops-93-single-pass-checks

**The owner's decision.** On 2026-10-18 the owner approved amending ADR-0006. Every check runs
once, inside the no-stridal environment, instead of twice. A weekly double pass on `main`
covers the gap. Rollback is `CHECKS_DOUBLE_PASS=1`, or a revert. The release notes must tell
adopters that a project whose checks need a gh login, ssh keys, signing or `CI=true` will fail
after updating. It is pre-launch work, after rename step 2, paired with OPS-89's dedupe, and it
keeps the 2026-10-18 date.

The decision reached this repo as a handoff written outside it. The amendment text below came
attached, and its `Source` line calls the decision "R23". Nothing in this repo records R23. This
design is that record, and the archived change keeps it.

## The amendment

The owner approved this text on 2026-10-18. The build inserts it verbatim into
`docs/adr/0006-a-repository-works-without-stridal.md`, between the end of `## Enforcement`
(line 36 on `main` at d9f2a98) and `## Related` (line 38), with one blank line on each side. Only
the owner can change a word of it (O1, O2 and O6 name the words that could change). The ADR's
header block and its index row stay as they are. An amendment carries no status of its own, as
ADR-0008's shows.

```markdown
## Amendment — 2026-10-18: the checks run once, inside the no-stridal environment
- **Source**: OPS-93; the owner's decision R23 (2026-10-18). Measured the same day: in the template's
  check set, `no-stridal` took 201.9 s of 400.25 s, the other 43 checks 198.1 s together.

The decision stands: everything a contributor needs runs in Claude Code alone, and the binary, the
panel and the plugin stay conveniences. How the check proves it changes. This amendment replaces the
first negative trade-off above and the Enforcement paragraph's "every other process check":
- The check set no longer runs twice. `checks/run.sh`, run for the whole set, builds the
  no-stridal environment once and runs every check inside it: this directory's checks, the enabled
  modules' and `.claude/local/checks`. That environment is unchanged: every `PATH` directory that
  holds `stridal` replaced by a mirror of links to everything else in it, a fresh `HOME` with no
  `~/.stridal` and only a git identity, and `CI` and `STRIDAL_LOCK_HELD` emptied. One helper in
  `checks/lib.sh` builds it for `run.sh` and `no-stridal.sh` alike, so the two cannot drift.
- `no-stridal.sh` keeps its own parts: the output scan, the gate in a throwaway repo and its
  falsification, the `GATE_STEPS` scan, session-start's context script and the status line. Instead of
  re-running the set, it asserts that it runs inside that environment, started by a whole-set
  `run.sh`. Run on its own, it re-runs the set as before.
- What the double pass caught and this does not: a check that passes without stridal but fails
  with it on `PATH`, with the caller's `HOME`, or with `CI` set. No check in the template reads any of
  these from its caller today; each builds its own. A weekly job on `main` runs the old double pass
  (`CHECKS_DOUBLE_PASS=1`) with a freshly built `stridal` on `PATH`, so such a check is still found,
  days after merge instead of before it.
- The first trade-off's "about 45 s" was measured with 26 checks. With 44 the second pass cost 201.9 s
  on an 8-core Linux machine, half the set. The single pass removes it. In its place, a module's or a
  project's own check now also runs without the caller's `HOME` (its gh login, ssh keys, git signing).
  A check that needs them sets them up in its own scratch, as the template's do.

**Enforcement** (replacing the original's "every other process check" clause): `checks/run.sh` runs the
whole set inside the environment `checks/lib.sh` builds. `checks/no-stridal.sh` fails when it runs
outside that environment under a whole-set `run.sh`, and when `stridal` is still on the stripped
`PATH`. Its gate, `GATE_STEPS`, context-script and status-line parts and their fixtures are unchanged.
Falsified in fixtures: a check that calls `stridal` fails the single pass; a `run.sh` that skips the
environment fails `no-stridal`. The scheduled double pass on `main` keeps the case the single pass
gives up. `CHECKS_DOUBLE_PASS=1` restores the old behaviour in any run.
```

### The approved text checked against the repo

Checked against `main` at d9f2a98:

- **Verified in the code.**
  - The environment it describes is today's (`no-stridal.sh:20-39`): a mirror for each `PATH`
    directory that holds `stridal`, a fresh `HOME`, and `CI=` and `STRIDAL_LOCK_HELD=`.
  - The re-run runs only this directory's checks (`:110` passes names, so `run.sh:46-47` skips
    the modules' checks and `.claude/local/checks`). After the change those run in the stripped
    environment for the first time.
  - `649aa49`, where the check landed, had 26 checks (28 `.sh` files). The template has 44 now.
  - This repo's `.claude/checks/no-stridal.sh` is byte-identical to the template's.
- **Spot-checked, not proven.** "No check in the template reads any of these from its caller":
  every `HOME=` and `CI=` in `template/.claude/checks/*.sh` is set by the check itself (`gate.sh`,
  `ci-status.sh`, `quota-reading.sh`, `statusline.sh`, `openspec.sh`, `hooks.sh`,
  `machine-quiet.sh`), and every `gh` they call is a stub. Group 1's fixtures and the first weekly
  run are what test the claim.
- **Small imprecisions, left as approved.**
  - The fresh `HOME`'s `.gitconfig` holds more than "only a git identity": it also sets
    `commit.gpgsign = false` and `init.defaultBranch = main` (`no-stridal.sh:38`).
  - "Measured with 26 checks" infers when the 45 s was measured from the check count at
    `649aa49`; ADR-0006 records neither the set nor the hardware.
  - "Replaces the first negative trade-off above" reads as voiding the whole bullet at
    `0006…md:26-27`. That bullet is the ADR's only trade-off, and it has two clauses. Its second
    clause, "every new panel feature needs a no-panel path", stays true after this change. The
    text means only the first clause, the second run. This is O6.

### What rests on sources outside the repo

The handoff cites these, and none of them is on this machine:
- the timing probe (`/tmp/cto-probe-times.tsv`): 201.9 s of 400.25 s, the 198.1 s for the other
  43 checks, and "an 8-core Linux machine";
- `TECH-PLAN.md`: the CI step durations (12.4 min on macOS, 6.8 min on Ubuntu), the acceptance
  criterion of about 210 s, and the S8 schedule;
- `STATUS.md` and `DECISIONS-R22-R24.md`: R23 itself, and the time it was approved;
- `ENG-PLAN.md`: what rename step 2 renames.

So the amendment's figures are the probe's. Task 1.6 measures the template set on the owner's Mac
before and after the change and records both in the Decision log. If the Mac's figures contradict
the text, the build stops and asks the owner rather than editing the approved words.

## Decisions

- **D1 A whole-set `run.sh` builds the environment once, and runs itself again inside it.**
  - With no names and no `CHECKS_DOUBLE_PASS=1`, `run.sh` builds the environment with D2's helper.
    It then `exec`s itself under that environment, with a marker set: `CHECKS_BARE`, the
    environment's directory. The new process enumerates and runs the checks as today, so its exit
    code is the run's.
  - **It `exec`s, and the inner run removes the environment** (amended 2026-10-21, the owner's
    choice A after group 1's review round 2; it was "a child, not `exec`"). An `exec` drops the
    outer run's `EXIT` trap, so the inner run takes over the cleanup: it is told which directory
    to remove (the outer run's scratch, which holds the mirror and the `HOME`) and removes it on
    `EXIT`. The child the approved text chose had to run in the background for the parent's trap
    to fire on a signal, and a background child ignores `INT` (POSIX, no job control), so a
    Ctrl-C never reached the running check. With `exec` there is one process in the terminal's
    foreground group, and `INT` and `TERM` reach it and its check as they do today.
    **Beat:** a foreground child (a signal waits for the whole set to finish) and a background
    child (a Ctrl-C does not reach the check).
  - **The inner run does not wrap again:** a whole-set `run.sh` that finds `CHECKS_BARE` set runs the
    checks directly. That marker is the stop condition, and D3 checks the real state behind it,
    so a stale marker cannot pass a run that stripped nothing.
  - Named runs (`run.sh hooks roadmap`) keep running in the caller's environment, as today. The
    gate and CI call the whole set, and a named run is for working on one check.
  - Every whole-set run exports `CHECKS_SET=whole`, in the wrapped inner run and in a double pass
    alike, so D3 can tell "under a whole-set run" from "run alone".
  - **The markers are inherited.** `bare` overrides only `HOME`, `PATH`, `CI` and
    `STRIDAL_LOCK_HELD`, and passes every other exported variable through. So `CHECKS_BARE`,
    `CHECKS_SET` and `CHECKS_DOUBLE_PASS` reach every check and every nested run. Whatever starts
    a whole-set `run.sh` or a `no-stridal.sh` of its own (D3's re-run, D7's fixtures) sets or
    unsets all three explicitly, and never inherits them. Named runs ignore the markers, so the
    checks that already start a named fixture `run.sh` (`ci-status.sh`, `ideas.sh`) need no
    change.
  - **Beat:** wrapping each check on its own, which builds the mirror 44 times; and keeping
    `no-stridal.sh`'s re-run while the gate skips the outer pass, which works only for gates
    that know to do that. Every project's gate calls `run.sh`, so `run.sh` is where the change
    holds everywhere.

- **D2 One helper in `checks/lib.sh` builds the environment, for `run.sh` and `no-stridal.sh`.**
  - `no-stridal.sh:20-39` moves into it unchanged: the `PATH` mirror, the fresh `HOME` and its
    `.gitconfig`, and `bare`.
  - It goes in `lib.sh` because `run.sh` treats every other `*.sh` in `checks/` as a check
    (`run.sh:49`). A new helper file there would run as a check.
  - `run.sh` does not source `lib.sh` today. It starts to, after it has set `ROOT`. `lib.sh` then
    makes its scratch and traps, and that scratch holds the environment for the run.
  - **Beat:** a copy of the code in each script, which is how the two drift apart; and a file
    under `.claude/lib/`, which the plugin builder would then need to rewrite on one more path.

- **D3 `no-stridal.sh` part 2 asserts the environment, and re-runs the set only when it runs
  alone.**
  - **Under a wrapped whole-set run** (`CHECKS_BARE` set), it checks the real state, not just the
    marker: `command -v stridal` fails, `$HOME` is the environment's `HOME`, and
    `$HOME/.stridal` does not exist. Then it reports that the set ran there, and does not run
    it again.
  - **Under a whole-set run with no environment** (`CHECKS_SET=whole`, no `CHECKS_BARE`, no
    `CHECKS_DOUBLE_PASS=1`), it fails. This is the amendment's "fails when it runs outside that
    environment under a whole-set `run.sh`".
  - **Run alone** (no `CHECKS_SET`), or under `CHECKS_DOUBLE_PASS=1`, it re-runs the set under the
    helper's environment, exactly as today. The re-run is a named run, so it does not wrap; it
    clears `CHECKS_SET` and sets `CHECKS_BARE` to its own environment, so a check inside it sees
    the same markers it would under a wrapped whole-set run.
  - Its self-test, which fails if `stridal` is still on the stripped `PATH`, stays.
  - **Beat:** trusting the marker alone. A stale or exported `CHECKS_BARE` would then pass a run
    that never stripped anything.

- **D4 `CHECKS_DOUBLE_PASS=1` is the old behaviour, exactly.**
  - `run.sh` does not wrap, every check runs in the caller's environment, and `no-stridal.sh`
    re-runs this directory's checks under the helper's environment. That is today's double pass,
    the same names included.
  - It works from the environment, so rolling back means setting it in `steps.sh` or in the
    shell, with no revert. The template's playbook gets one line naming it (task 2.4).

- **D5 The weekly double pass is a workflow in this repo, read by the existing health line.**
  - **The workflow:** `.github/workflows/checks-double-pass.yml`, on a weekly `schedule:` and
    `workflow_dispatch`, on Ubuntu. It builds `stridal` from `framework/` and puts it on
    `PATH`, then runs `CHECKS_DOUBLE_PASS=1 sh .claude/checks/run.sh` and the same for
    `template/.claude/checks/run.sh`.
  - **What the outer pass sees:** a real binary and `CI=true`, which GitHub sets, and the
    runner's own `HOME`.
  - **What it does not reproduce:** the old outer pass on the owner's Mac. That pass had a
    populated `~/.stridal` and a running panel, a gh login, and `STRIDAL_LOCK_HELD` set
    inside the gate. The runner's `HOME` has none of those, and the job runs outside any gate. So
    it covers "with stridal on `PATH`" and "with `CI` set" fully, and "with the caller's `HOME`"
    only for a plain `HOME`. Production gates now always run stripped, so a check that passes
    only with the owner's own `HOME` no longer passes anywhere that counts.
  - **The same set as PR CI:** the job copies `test.yml`'s setup steps and sets the same
    variables as its process-checks step: Go, tmux and the OpenSpec CLI (`test.yml:39-55`), and
    `OPENSPEC_REQUIRED=1` and `NODE_REQUIRED=1` (`:72-77`). Without them it would run a weaker set
    than a PR does.
  - **Actions are pinned** to the commit SHAs `test.yml` uses, with `permissions: contents: read`
    and no secrets.
  - **Its reader:** `.claude/health/scheduled-workflows.sh` already lists every workflow whose
    `on:` has a `schedule:`, taken from the workflows themselves. So a failed run shows at the next
    session start with no new health script. It goes `STALE` only past 45 days, so the weekly
    cadence itself is held by the workflow's `cron`, not by the health line.
  - **What makes it exist:** `.claude/local/checks/double-pass.sh`, a check in this repo's local
    layer, which `stridal update` never touches. It fails when the workflow is missing, has no
    `schedule:`, does not set `CHECKS_DOUBLE_PASS=1`, does not build `stridal` onto `PATH`
    before the run, or does not run both sets. It runs in every gate, inside the single pass.
  - **Beat:** a nightly schedule (O3); and a template-shipped workflow (O5).

- **D6 Part 2 runs `run.sh` from its own directory.**
  - It uses `$dir/run.sh`, not `$ROOT/.claude/checks/run.sh`.
  - The handoff worried that the plugin's part 2 runs the wrong `run.sh`. That does not hold on
    `main`: the plugin builder already rewrites the line to `$STRIDAL_FW/checks/run.sh`
    (`plugin/checks/no-stridal.sh:112`), and in the plugin that is `$dir`. With `$dir`, the
    template stops depending on that rewrite.
  - **The plugin layout's test:** `TestPluginChecksPassInScaffoldedProject`
    (`framework/internal/plugin/plugin_test.go`) runs the plugin's whole-set `run.sh` in a
    scaffolded project. After the change, that run is wrapped, so the test covers the helper and
    D3 in the plugin layout.
  - **Beat:** keeping `$ROOT/.claude/checks/run.sh` and relying on the builder's rewrite. It works
    today, but nothing states why.

- **D7 The fixtures show each control failing first.** They live in `no-stridal.sh`, on a
  fixture project that holds copies of `run.sh`, `lib.sh` and `.claude/lib/conf.sh`, plus a stub
  `stridal` on the outer `PATH`:
  - a fixture check that calls `stridal` fails a whole-set run. This is the single pass doing
    the old second pass's job;
  - a fixture `run.sh` with the wrap removed fails D3's assertion. This is the amendment's second
    falsification;
  - a fixture check that fails only when `stridal` is on `PATH` passes the single pass and
    fails under `CHECKS_DOUBLE_PASS=1`. This is the gap the weekly job exists for, shown both
    ways.

  D3's assertion is a function in `lib.sh`, so these fixtures drive it without starting
  `no-stridal.sh`'s gate parts again inside themselves. Each fixture run clears
  `CHECKS_BARE`, `CHECKS_SET` and `CHECKS_DOUBLE_PASS` first, then sets only the ones its case
  needs (D1, *The markers are inherited*). Otherwise the weekly job's exported
  `CHECKS_DOUBLE_PASS=1` would stop fixture 1 from wrapping, and an inherited `CHECKS_BARE` would
  let fixture 2 fail for the wrong reason.

  Fixtures prove the mechanism on a toy project. The rollback is proven on the real sets too:
  task 1.6 runs both with `CHECKS_DOUBLE_PASS=1` once before merge.

- **D8 The release-notes item goes where the release notes will be read.**
  - No release-notes file exists on `main`. `CHANGELOG.md`, `docs/release.md` and
    `scripts/changelog.sh` come with REL-1 (#25, open). On that branch, the GitHub release's notes
    are the version's `CHANGELOG.md` section.
  - On that branch, `scripts/changelog.sh` regenerates `[Unreleased]` whole, from PR titles and
    `Slice:` lines. A hand-written item under `[Unreleased]` would be lost the next time it runs.
  - **The item:** "A project's own process checks (a module's, or one in `.claude/local/checks`)
    now run without the caller's `HOME` (its gh login, ssh keys and git signing) and with `CI`
    empty. A check that needs any of these fails after `update`: set it up in the check's own
    scratch, as the template's checks do, or set `CHECKS_DOUBLE_PASS=1` while you fix it."
  - **Where it goes:** to the change that will write the release notes, as a roadmap row. That is
    something the builder can do in a commit, and anyone can check it exists (AGENTS.md rules 1
    and 2). The build PR does not exist while group 2 runs: build-change ends before the PR is
    opened, and `merge-pr` opens it with `gh pr create --fill`. So no task can write to its body.
    - **If REL-1's row is still open** (`⬜ queued` or `⬜ in flight`), the build appends the item
      to REL-1's Change cell. REL-1 writes the first release notes, the `[0.1.0]` section, by
      hand. This is the expected case: the owner placed OPS-93 before launch, and REL-1 is the
      session plan's S12.
    - **If REL-1 has merged,** no change owns the next release's notes, so the build creates one,
      as rule 1 asks: a queued row in Phase 4, whose Change cell says the next release's
      `CHANGELOG.md` section carries the item, in *Changed*.
  - **And, as well:** if `CHANGELOG.md` is on `main` and a version section is being prepared (not
    `[Unreleased]`), the item goes in that section's *Changed* in the build.
  - **Beat:**
    - a hand-written item under `[Unreleased]`, which the next regeneration deletes without a
      word;
    - a `Slice:` line in the build PR's body. On #25's branch, `scripts/changelog.sh` copies such
      lines into `[Unreleased]` and keeps them across regenerations, but no process writes the
      line: the PR is opened after the build, from commit info;
    - editing #25 from this change, which is REL-1's PR, not this change's.

- **D9 The amendment lands with the code, in group 2.** It is not inserted in the proposal PR.
  - Merged before group 1, it would describe a `run.sh` that does not build the environment and a
    `no-stridal.sh` that cannot fail outside it. That is a stated control with nothing executing
    it (AGENTS.md rule 4).
  - Group 2 inserts it after group 1's code and fixtures have passed the gate.
  - **AGENTS.md:** its header says the file changes to add a mechanism, and the mechanism gets its
    row. Group 2 edits the existing row "The repo works without stridal" rather than adding
    one, because after the change `checks/run.sh`, not `checks/no-stridal.sh`, runs the checks
    off `PATH`. The first cell stays. The second cell becomes, word for word:
    - **the template's copy** (`template/AGENTS.md`): ``**`checks/run.sh`** runs the checks with
      `stridal` off `PATH`; **`checks/no-stridal.sh`** asserts it, and runs the gate, a
      context script and the status line there``. The whole row is 248 characters;
    - **this repo's copy** (`AGENTS.md`): ``**`checks/run.sh`** runs the checks with `stridal`
      off `PATH`; **`checks/no-stridal.sh`** asserts it and runs the gate there; weekly,
      **`checks-double-pass.yml`** (`local:double-pass`)``. The whole row is 269 characters. The
      weekly job is this repo's alone (O5), so only this copy names it.
  - Both rows are under `checks/agents-md-budget.sh`'s 320-character row ceiling, and the two
    files have about 4,000 bytes of headroom under its 16,000-byte ceiling.

## Open: the owner decides

Each comes from the handoff's open points (§3.7), except O5 and O6, which this design adds. None
blocks group 1. O1, O2 and O6 decide the amendment's wording, and group 2 inserts it.

- **O1 The heading's date.** **Recommend 2026-10-18**, the day the owner decided and the text was
  written.
  - `new-adr` dates an amendment `<today>`, the day it is written (its step 0a).
  - ADR-0008's amendment is dated 2026-10-01, the day the change it records merged (OPS-14, #36,
    0b29da5). The amendment itself merged on 2026-10-02 (2e43b35, #61). So the precedent is not
    the amendment's own merge day.
  - **Beat:** the build's merge day, which would date the text after the decision it records.

- **O2 The order against rename step 2.** The rename has no roadmap row, so nothing can hold
  OPS-93 behind it. Deps take only row ids and issues.
  - **Recommend:** keep the owner's order, and make it executable. Queue the rename as a row when
    its scope is known, and give OPS-93 a Dep on it. If the rename is not queued by the time
    OPS-93 is next, build OPS-93 first, and the rename renames `no-stridal` in this amendment
    with the rest. That is a mechanical edit in either order.
  - **Beat:** holding OPS-93 on an order written only in prose. It would wait on something the
    queue cannot see, and nothing would say why it is not next.

- **O3 Weekly or nightly.** **Recommend weekly.**
  - The loss is theoretical today: no template check reads the ambient state, as spot-checked
    above.
  - The handoff's estimate is 13–14 min per double pass on Ubuntu: about 1 h of Actions time a
    month weekly against about 7 h nightly, on a private repository. This rests on its probe.
  - Either cadence surfaces at the next session start, so nightly only shortens the delay by a few
    days.
  - **Beat:** nightly.

- **O4 The plugin path of part 2's `run.sh`.** **Recommend D6:** `$dir/run.sh`, with
  `TestPluginChecksPassInScaffoldedProject` covering the plugin layout. The handoff's concern does
  not hold on `main`, because the builder already rewrites the path.
  - **Beat:** leaving the line as it is and relying on the rewrite.

- **O5 The weekly job is this repo's alone.** **Recommend this repo's only.** Adopters get
  `CHECKS_DOUBLE_PASS=1`, named in the template's playbook (task 2.4) and in the release-notes
  item (D8).
  - A template workflow would need a `stridal` binary in every adopter's CI, and none exists
    before a release ships one (REL-1). ADR-0006 is this repo's record; the template ships no ADRs.
  - **Beat:** shipping the workflow in `template/.github/workflows/`.

- **O6 "Replaces the first negative trade-off above".** ADR-0006's only trade-off bullet has two
  clauses, and the second ("every new panel feature needs a no-panel path") stays true.
  - **Recommend:** keep the approved words, read as replacing the first clause only, the second
    run. The amendment's own fourth bullet makes that reading clear: it talks only about the 45 s
    and what replaces it.
  - **Beat:** changing "the first negative trade-off above" to "the first clause of the negative
    trade-off above". That is more exact, but it edits words the owner approved.
  - The owner may prefer the exact wording. If so, group 2 inserts it.

## Refusals

| Situation | What happens instead |
|---|---|
| `run.sh` with names | No wrap. The named checks run in the caller's environment, as today. |
| `CHECKS_DOUBLE_PASS=1` | No wrap. `no-stridal.sh` re-runs the set under the environment: the old double pass. |
| `no-stridal.sh` run alone | It re-runs the set under the environment, as today. |
| A whole-set run whose environment was skipped | `no-stridal.sh` fails, and says it ran outside the environment under a whole-set run. |
| `stridal` still resolves under the stripped `PATH` | `no-stridal.sh` fails: "cannot check" (unchanged). |
| A project check that needs the caller's `HOME` or `CI=true` | It fails under the single pass. The release notes say so (D8), and `CHECKS_DOUBLE_PASS=1` is the stopgap. |
| The weekly job has never run, or failed | `scheduled-workflows.sh` prints `NEVER RAN` or `FAILED` at session start. |
