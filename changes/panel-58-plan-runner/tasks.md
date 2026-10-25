# Tasks: panel-58-plan-runner

## Progress
- 2026-10-25 proposed (session 12). Nothing is built yet.
- The build starts from `main` once the owner approves.
- The roadmap row stays `⬜ queued` until its build PR opens.

## Decision log
- 2026-10-25: the owner decided D4: "95% weekly". The ceiling is the account's 7-day usage, from
  the status line's `rate_limits`, set by `plan_runner.seven_day_pct` with a default of 95. An
  unreadable reading stops the run.
- 2026-10-25: the owner decided D8: "I don't want any mediums".
  - (a) This build binds every group to converge with nothing medium or worse left. When a group's
    cap round still finds a medium or worse, the build session stops and notifies the owner.
    `merge-pr`'s cap rule would instead list the findings under KNOWN LIMITS and merge, and here it
    does not.
  - What executes (a): **Nothing. You.** The build session, by this line.
  - (b) The runner stops on a merged PR whose `Review:` line is capped, NOT converged, missing,
    doubled or unreadable (PLANRUN-3).

## 1. The plan's next row (D2)
- [ ] 1.1 `template/.claude/panel-suggest.sh` gains the kind `next`, in its usage line and its
  header.
  - It prints at most one item from the plan's order within the open session, the one `close`
    reads. That is the first entry not landed and not a batch's later row, by the rules `build`,
    `ops`, `propose` and `close` already apply.
  - Once every entry has landed, the item is the session's close. With no plan or no open session,
    `next` prints `[]`.
  - The item is `{name, title, detail, issue, kind, row}`, plus `decision` when a `D` record of
    `roadmap-plan.sh` names its row. Its `issue` is `#<n>` for a `fix/` row whose name is
    `<n>-<slug>`, the fix kind's form, and `""` for every other item (D2).
  - Its `kind` is `build` for a change row whose proposal is on main, `propose` for one without,
    `ops` for an `ops/` row, `fix` for a `fix/` row, and `close` for the close.
  - A planned `ID (proposal)` entry is `propose` until its proposal is on main or archived, and
    has landed after that, counted per entry by the same rule `close` uses (`$tmp/proposed`), so
    it is never offered as a build.
  - It reads the plan only through `roadmap-plan.sh` and the queue only through `roadmap_queue`:
    no second parser.
  - Tested in `template/.claude/checks/panel-suggest.sh`, citing [PLANRUN-8-S1], [PLANRUN-8-S2],
    [PLANRUN-8-S3], [PLANRUN-8-S4], [PLANRUN-8-S5], [PLANRUN-8-S6] and [PLANRUN-8-S7]. The fixture roadmaps are built as the
    check's OPS-126 cases build theirs.
- [ ] 1.2 Run `scripts/build-plugin.sh`, and `framework/clauductor update` from the repository root
  with the binary built from this checkout. This syncs this repository's `.claude/panel-suggest.sh`
  and `.claude/checks/panel-suggest.sh`. Held by `TestCommittedPluginIsCurrent`.

## 2. The switch and its trust (D1, D9)
- [ ] 2.1 `framework/internal/panel/config`: the `plan_runner` key.
  - `LatestVersion` moves to 8 in `fields.go`, with a version comment line: "8 plan runner
    (PANEL-58): plan_runner".
  - Three fields go in `Fields`: `plan_runner.on` (boolean, false), `plan_runner.next` (argv) and
    `plan_runner.seven_day_pct` (number, 95, 1 to 100). Their docs point to *Plan runner* in
    `docs/panel.md`.
  - `Config.PlanRunner` and a resolver are added. Its load errors: a version below 8 with the key;
    `on` with no `next`; a percentage outside 1 to 100; `on` with `alerts.notify` false. Each names
    the key.
  - `runEntries` adds `plan runner runs %q, starting lanes until the 7-day usage reaches %v%%` while
    `on` is true, with `next` as its command, so `RunList` and `SidecarRefusals` both read it.
    The line ends `, adopting lanes of kinds <kinds>` (each kind once, sorted, joined by `, `), or
    `, adopting no lane: no
    template's suggest command is %q with its last element replaced`.
  - `Config.TemplateKind(t)` (new) is the one reading of the kind rule: a template is of kind K
    when its suggest command is `plan_runner.next` with its last element replaced by K, element
    for element. `runEntries` and the runner's `candidates` both call it.
  - Run `go generate ./internal/panel/config` for `docs/panel.md`'s table and
    `docs/panel.schema.json`.
  - Tested in `framework/internal/panel/config/config_test.go`, citing [PLANRUN-1-S2] and
    [PLANRUN-1-S3]. Tested in `framework/internal/panel/config/sidecar_test.go`, citing
    [PLANRUN-1-S5]. Held by `TestGeneratedFilesAreCurrent` and `version_test.go`.
- [ ] 2.2 `template/.clauductor/panel.json` declares version 8, and sets `plan_runner` to
  `{"on": false, "next": ["sh", ".claude/panel-suggest.sh", "next"]}`.
  - Then `scripts/build-plugin.sh` again.
  - This repository's own `.clauductor/panel.json` is left alone. PANEL-59 turns it on (D10).
  - Held by `TestTemplatePanelConfigLoads` (`framework/internal/template/template_test.go:99`,
    [STALL-1-S29]: the template declares the latest version), `internal/plugin/plugin_test.go` and
    `TestCommittedPluginIsCurrent`.
- [ ] 2.3 Trust on arrival (CATCHUP-3, D9).
  - `config.Check` gains `PlanRunner` (whether the runner is on), set in `CheckOf`, and
    `config-check` prints it. Its `Short` and `Long` text (`framework/internal/cmd/panel.go:121-127`),
    which name the four fields it prints, name the fifth.
  - `configCheck` (`catchup.go:1160`) reads a missing `plan_runner` as off and does not refuse the
    answer for it.
  - `liveSet.arrive` (`catchup.go:881`) adds one reason not to trust: the new config's runner is
    on. Its log and notice say the config has the plan runner on, and that it waits for the owner.
  - Tested in `framework/internal/panel/catchup_trust_integration_test.go`, citing
    [CATCHUP-3-S11]: the plain merge, the rebuild through a stand-in `config-check` that prints
    `plan_runner`, a changed `seven_day_pct`, a changed template `first_prompt` while on, and
    turning it off.
  - Also tested, in `framework/internal/panel/config/catchup_test.go`: a `config-check` answer
    with no `plan_runner` decodes with the runner off.

## 3. The run's state, its trigger and the review line (D6, D8)
- [ ] 3.1 `framework/internal/panel/planrun.go` (new): the runner's loop in the project runtime,
  started beside auto-close.
  - Its state file is `plan-run.json` in `config.ProjectDir(home, project)`, holding `branch`,
    `lane`, `started`, `item` (`next`'s item whole, one value, never a subset of its fields),
    `since`, `state` (`idle`, `watching`, `starting`), `stop` and the keys of the notices sent. `waiting` is the runtime's alone, never written. It is written atomically (`config.WriteAtomic`) before each step that
    starts or stops anything.
  - It runs only while `plan_runner.on` is true and the config is trusted. Otherwise it keeps its
    state and does nothing.
  - **Who may be tracked** (PLANRUN-9).
    - `candidates(item)` returns the branches `Config.TemplateBranch(id, item.name, item.issue)`
      gives the templates `Config.TemplateKind` gives the item's kind, deduplicated. A template it
      refuses for the item is no candidate. When none is left, it returns the reason in one of
      refusal 1's two forms: no template of the kind, or each one refused, naming
      `TemplateBranch`'s errors. No Up next list is read.
    - `adoptable(item)` looks up the project's running lanes on the candidates itself, and
      returns an `Adoption` or the reason, in design.md's order: no template of the item's kind
      (either form); no lane on any candidate; running lanes, of any template, on more than one
      candidate; a lane record whose `Template`
      is of another kind or empty; `mergedPRRead` returned an error (never read as no merged
      PR); a HEAD a merged PR holds, naming the PR.
    - `track(Adoption)` is the only writer of `watching` for a lane the runner did not start in
      that same step. `Adoption`'s fields are unexported, and only `adoptable` builds one. It
      carries the lane, the item and `started`, false unless the restart marks it (task 4.2).
  - **Adopt.** While idle, on the suggest cadence, it runs `next` under a timeout and calls
    `adoptable` for the item. An allowed lane is tracked with `next`'s item, whole, and
    `started` false. A refusal sets the runtime's `waiting` to its reason and pushes nothing.
    A `next` that prints `[]` sets it to "next names no row", and one that fails, times out or
    prints something unreadable sets it to the error, with no stop and no push. It is empty
    while the runner is not idle, off or untrusted. Tracking a lane or writing a stop clears it.
  - Tested in `framework/internal/panel/planrun_test.go`, one table over `adoptable`'s reasons
    with a stand-in `gh`, citing [PLANRUN-9-S1], its rows in the same order: `next`
    `["sh", "scripts/next-row.sh"]`; an ops lane on `ops/a` while `next` names A's build; build
    lanes on two candidates, from a config with two build templates of different patterns; a
    propose lane on `change/a`; a lane with no template; a stand-in `gh` that fails the
    merged-PR read; and a HEAD merged PR #8 holds. Each case asserts its `planRun.waiting`
    reason, that `plan-run.json` stays `idle`, and that no notice goes out. Two rows beyond the
    scenario: a build lane on one candidate and a propose lane on another (lanes on two
    candidates), and a fix template whose pattern names `{issue}` (refusal 1's second form).
  - Tested in the same file, citing [PLANRUN-7-S2]'s idle `next` cases: a stand-in `next` that
    fails, and one that prints `[]`, each with no notice and its `waiting`.
  - **A step cut short.** Every step, the catch-up wait and `StartLane` take the runtime's
    context. An error with that context done writes nothing and sends nothing: before the start it
    leaves `watching`, and during it `starting`. Tested in task 4.2, once the wait, `next`'s second
    run and the start exist.
  - **Trigger.** While watching, it reads the tracked branch's merged PRs through `mergedPRList`
    with `number,body,mergedAt,mergeCommit`: when the branch leaves the open list, every 5
    minutes, and on start. A PR merged after `since` is the trigger. Merges of other branches are
    never read.
  - `mergedRow` (`autoclose.go:149-152`) holds only `Number` and `HeadRefOid`. It gains `Body`,
    `MergedAt` and `MergeCommit` (gh's `{oid}` object), or the runner decodes into its own row.
  - **The close.** When the tracked kind is `close`, the trigger ends the run.
  - Tested in `framework/internal/panel/planrun_test.go` with a stand-in `gh`, citing
    [PLANRUN-1-S1], [PLANRUN-2-S4] and [PLANRUN-2-S7].
- [ ] 3.2 The `Review:` line (D8 (b)).
  - The runner reads the lines of the body that start with `Review:`. It goes on only for exactly
    one, matching `Review: converged in <n> round(s), peak <sev>, at <sha>`, with an optional
    ` · design <hash>`.
  - It stops naming the PR and the line for `capped`, `NOT converged`, none, two, or any other
    text.
  - Tested in `framework/internal/panel/planrun_test.go`, a table over the formats in
    `template/.claude/skills/merge-pr/SKILL.md`, citing [PLANRUN-3-S1], [PLANRUN-3-S2] and
    [PLANRUN-3-S3].

## 4. The start (D2, D3, D5, D6)
- [ ] 4.1 After the trigger and the review line, the runner goes through the steps in `design.md`'s
  order.
  - It fetches the config's base ref in the main checkout under a timeout, as catch-up's fetch does
    (`catchup.go:411`), retries a failed fetch once 30 seconds later, then checks
    `git merge-base --is-ancestor`.
  - **Catch-up settles.** While catch-up is on for the project, the runner calls `catchUp.wake`
    for the root and waits, for 15 minutes at most, until catch-up has settled. That is a check
    of the root, begun after the fetch, that ended with the context alive, after which either the
    main checkout's HEAD contains the merge commit (`git merge-base --is-ancestor <merge> HEAD`),
    or `catchUp.retries` holds no retry for the root.
  - `catchUp` gains the hook it waits on: per root, a check's start and end, and whether a retry
    is pending when it ends. A rebuild restarts the panel inside that check, so the start never
    begins in the old binary.
  - It runs `next`: `[]` stops, and so do a `decision` or `kind: propose` (D3), a failed or
    timed-out command, and any output but one item.
  - An item with the tracked lane's `row` **and** `kind` stops too: no fragment on main names it
    yet. A tracked `propose` followed by the same row's `build` is its own stop: the proposal
    merged, and its build starts with the owner's click.
  - It runs every template's suggest command once through `suggestSource` and applies the results
    as a refresh does, then requires exactly one list to name the item.
  - Tested in `framework/internal/panel/planrun_test.go`, citing [PLANRUN-4-S1], [PLANRUN-4-S2],
    [PLANRUN-4-S3], [PLANRUN-4-S4], [PLANRUN-4-S5], [PLANRUN-4-S6], [PLANRUN-4-S8] and
    [PLANRUN-4-S9].
  - Tested in `framework/internal/panel/planrun_test.go` with a stand-in catch-up hook, citing
    [PLANRUN-2-S8]: a check still running when the runner stops (the rebuild), a second runner
    started on the same `plan-run.json` (the new binary), a check that ends with no rebuild, a
    check that ends on a transient refusal with a retry pending and then a retry that moves HEAD,
    one that ends on a policy refusal with none pending, a check whose retries all fail until none
    is pending, and one that never ends.
- [ ] 4.2 Where the lane runs, in Go, mirroring `planStart` (`web/static/start-plan.js:97-145`)
  over `TemplateBranch`'s facts.
  - A busy branch is a stop, and so is a HEAD a merged PR holds.
  - `adoptable(item)` (task 3.1) runs next: an allowed lane is tracked and nothing starts; "no
    template of the item's kind" and "no lane on any candidate" go on to the mode; any other
    refusal is a stop naming its reason.
  - Otherwise the mode is `existing`, `branch` or `new`.
  - The runner writes `starting` with the item, whole, its branch and the time. It calls
    `StartLane` on the runtime's context with the template, name, mode, worktree and issue and the
    gate `orchestration()` builds (trust and the 5-hour `QuotaGuard`), never `OverrideQuota`. Then
    it writes `watching` on the new lane's branch, with `started` true.
  - A refusal is a stop naming it.
  - On start, a `starting` state is resolved first (D12), through `adoptable` for the recorded
    item, whole. An allowed lane is tracked with no notice; when it is on the recorded
    branch, the restart marks the `Adoption` started before `track`. "No template of the item's
    kind" and "no lane on any candidate" are each a stop naming the row, the branch and what step
    6 finds there. Any other refusal is a stop naming its reason.
  - The log line is `plan runner started <lane> (<template>, <mode>) after #<n>`.
  - Tested in `framework/internal/panel/planrun_test.go` over a fake lane starter that records its
    requests, citing [PLANRUN-2-S1], [PLANRUN-2-S3], [PLANRUN-2-S6] and [PLANRUN-4-S7].
  - Tested in `framework/internal/panel/planrun_test.go`, with a stand-in `gh` and the fake lane
    starter, citing:
    - [PLANRUN-2-S2]: an adopted lane whose merge starts the row after it, and a propose lane
      whose item no list names.
    - [PLANRUN-9-S2]: a propose lane on `change/b` at step 6 while `next` names B's build, and the
      same lane found by a restart's resolution of `starting` for B's build.
    - [PLANRUN-9-S3]: `next` `["sh", "scripts/next-row.sh"]` with no lane on `change/b`, where
      step 6 goes on and the fake starter records B's build start, with no notice; and a
      restart's resolution of `starting` for that start under the same `next`, which tracks
      nothing and sends the cut-short notice.
    - [PLANRUN-9-S4]: the shipped fix template (`fix/{name}`, a first prompt naming `{issue}`)
      and the item `12-typo` with issue `#12`: its lane adopted while idle; the same lane found by
      a restart's resolution of `starting` and tracked with `started` true; a start the fake
      starter records with issue `#12`, whose request `RenderTemplate` renders with no error and
      a first prompt naming `#12` (a stand-in `gh` lists issue 12 for step 5); and a `fix/{issue}-{name}`
      pattern leaving `waiting` naming the template and its invalid branch.
    - [PLANRUN-1-S4]: an untrusted config that starts nothing, then **Trust config…** and the start.
    - [PLANRUN-2-S5]: a merge found on start, then a second restart that starts nothing.
    - [PLANRUN-2-S9]: the context cancelled during the catch-up wait and during `next`'s second
      run; a second runner on the same `plan-run.json`; a runtime reloaded untrusted and then
      trusted with no restart; and a cancel during a stand-in `StartLane` that blocks in
      `worktree_setup` until its context ends, where `halt` returns once the stand-in has
      returned.
  - The Go mode rule is also run over the cases `framework/internal/panel/web/start_plan_test.go`
    gives `planStart`, from one shared table, so the two copies cannot drift apart unseen.
  - Also tested, in `framework/internal/panel/planrun_integration_test.go` (a real tmux socket and a
    stand-in `claude`, under `leakcheck`): one end-to-end start, citing [PLANRUN-2-S1].

## 5. The 7-day ceiling (D4)
- [ ] 5.1 `framework/internal/panel/state/quota.go` gains `SevenDay(now)`.
  - `QuotaWindow` gains `At`, which `foldQuota` sets on each window a post carries and keeps on a
    window the post leaves out.
  - `SevenDay` returns the `seven_day` window's percentage from `quotaAt(now)`, or why there is
    none: no reading; no window; no percentage or past its reset; the window's own `At` more than
    30 minutes before `now`, whatever `Quota.At` says.
  - The runner calls it before every start, stopping at or above `seven_day_pct` with the reading
    and the ceiling in its notice, and on any "why".
  - Tested in `framework/internal/panel/state/quota_test.go` (the reader) and
    `framework/internal/panel/planrun_test.go` (the stop), citing [PLANRUN-5-S1], [PLANRUN-5-S2]
    and [PLANRUN-5-S3].

## 6. Notices, and a lane that stops short (D7, STALL-3)
- [ ] 6.1 The runner's notices go through `push.go`.
  - Each is a macOS notification and one `runPush`, with `sendCatchUp`'s pattern, file and timeout
    rules.
  - The title names the project while the config is trusted, else "clauductor panel".
  - Each is recorded in `plan-run.json` by its key before it is sent, so a restart does not send it
    again.
  - The notifier's push filter admits the runner's notices beside `stalled` and catch-up's, and
    still no other alert.
  - Tested in `framework/internal/panel/push_test.go`, citing [STALL-3-S13]. Tested in
    `framework/internal/panel/planrun_test.go`, citing [PLANRUN-7-S1].
- [ ] 6.2 A lane that stops short.
  - The first prompt: a tracked lane whose `plan-run.json` records `started` true, and whose
    `PromptState` is not `delivered` 10 minutes after its start, or is `skipped`, gets one notice.
    A lane with `started` false gets none. Nothing else infers who started it.
  - The idle lane: a tracked lane that is not stalled and not merged, none of whose live sessions is
    working, and that has not moved for `alerts.stall_minutes` (30 when 0), gets one notice. It
    uses the stall's own `sessionWorking` and last-movement reading, not a copy.
  - A stalled lane gets no notice from the runner, and the run's state reads "`<lane>` is
    stalled". `planRun.waiting` stays empty: it is the idle read's alone.
  - Tested in `framework/internal/panel/planrun_test.go`, citing [PLANRUN-6-S1], [PLANRUN-6-S2] and
    [PLANRUN-6-S3].

## 7. The page and the docs (D1, D10)
- [ ] 7.1 `/api/state` carries `planRun`: `on`, `state`, `row`, `lane`, `branch`, `why`, `at`,
  `waiting`. `web/static/panel.js` renders it as one line above Up next's rows: tracking, waiting
  on a stalled lane, or the last stop and its time, with `waiting` beside the last stop, so
  neither replaces the other.
  - Tested in `framework/internal/panel/web/plan_run_view_test.go`, over the state's JSON and the
    page's rendered line, citing [PLANRUN-7-S2].
- [ ] 7.2 `docs/panel.md` gains a *Plan runner* section: the key, what it starts and when, every
  stop and its notice, the 7-day ceiling, `plan-run.json`, and the trust on arrival. Held by
  `TestGeneratedFilesAreCurrent` for the generated table, and by review.
- [ ] 7.3 The roadmap's PANEL-59 row, added with this proposal, still names what turns the runner
  on here. Check it is open and unchanged before the build PR merges (AGENTS.md rule 1).
- [ ] Slice: with the plan runner turned on and trusted, the panel starts the session plan's next row by itself when the row before it merges, and stops with one push naming why wherever the owner is needed or a start is unsafe
