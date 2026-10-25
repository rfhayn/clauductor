# Design: panel-58-plan-runner

## Evidence
Read on 2026-10-25, at `0cfced2b`.

- **E1. A lane starts only from the page.**
  - `StartLane` (`framework/internal/panel/lanes/lanes.go:1377`) is reached from one route, `startLane`
    (`framework/internal/panel/web/routes.go:130`).
  - That route passes `startGate()` (`web/routes.go:365`): the config's trust and the quota guard,
    from `orchestration()` (`framework/internal/panel/queues.go:24-35`).
  - `StartRequest` (`lanes.go:589`) carries the type, the mode (`new`, `existing`, `branch`, `root`),
    the name, the worktree, the template, the issue and `OverrideQuota`.
- **E2. Where a template's lane runs is decided in the page.**
  - `planStart` (`framework/internal/panel/web/static/start-plan.js:97-145`) reads
    `TemplateBranch`'s facts (`lanes.go:917`, `BranchFacts` at `lanes.go:125`) and chooses:
    1. a busy branch starts nowhere;
    2. a worktree a lane already runs in starts nothing (the server refuses `path-taken`);
    3. a worktree runs `existing`;
    4. a local or remote branch runs `branch`;
    5. otherwise `new`.
  - The server checks each choice, but nothing in Go makes it.
- **E3. Up next is not plan order.**
  - `renderNext` (`web/static/panel.js:3007`) groups items by template, in the config's order.
  - The build template lists every planned change row whose proposal is on main.
  - So the page's first item is the first build, not the plan's next row.
  - `template/.claude/panel-suggest.sh` reads `origin/<main>`, puts the rows in the plan's order
    (`$tmp/plan`), skips a row a fragment on main names and a batch's later rows, and offers the
    close once every row of the open session has landed.
  - Its kinds are `build|propose|ops|fix|close` (line 42).
- **E4. The plan's decisions.**
  - `template/.claude/roadmap-plan.sh` prints a `D <id> <decision>` record for each `ID: decision`
    in *The owner decides*.
  - Nothing removes one when it is taken. The row relies on OPS-128 for that: "the cells keep only
    what stays the owner's".
- **E5. The first prompt.**
  - A started lane's prompt is `pending` (`lanes.go:1339`), then goes `typing`, `sent`,
    `delivered`, or `skipped` (`types/types.go:119-122`).
  - `runtime.go:933-975` acts on `PromptDecisions` through `DeliverFirstPrompt`.
- **E6. A merge is read per branch.**
  - `mergedPRList` (`framework/internal/panel/autoclose.go:157`) runs
    `gh pr list --head <branch> --state merged` for the fields asked.
  - `mergedPRRead` (`autoclose.go:136`) counts a merge only when its head is the lane's HEAD (#188).
- **E7. Pushes.**
  - `runPush` (`framework/internal/panel/push.go:233`) runs `push.json`'s argv.
  - `sendCatchUp` (`push.go:289`) sends a catch-up notice as an OS notification and a push.
  - STALL-3 allows only `stalled` and the catch-up notices through the push.
- **E8. Trust on arrival.**
  - `liveSet.arrive` (`framework/internal/panel/catchup.go:881`) trusts a merged `panel.json` unless
    one of its `why` reasons holds. One of those is `!chk.Notify`: a config that turns notices off
    (CATCHUP-3-S9).
  - `config.Check` (`config/catchup.go:123`) holds what the arriving config is judged on: `name`,
    `notify`, `runs`, `tmux_socket`.
  - When the merge also rebuilds the panel, the new binary prints that `Check`
    (`clauductor panel config-check`, `framework/internal/cmd/panel.go:119`). The running binary
    then decides from it: `configCheck` (`catchup.go:1160-1179`) requires all four fields.
  - So on the merge that rebuilds it, the running binary judges a config only on the fields it
    knows.
- **E9. The 7-day usage already has a reader.**
  - Each status line post's `rate_limits` is folded into the panel's account quota by `foldQuota`
    (`framework/internal/panel/state/quota.go:160`), `seven_day` among its windows.
  - `quotaAt` (`quota.go:225`) drops a window whose `resets_at` has passed.
  - `Quota.At` is the time of the last post that carried any window, and a reading from another
    account is dropped (`ApplyAccount`).
  - `foldQuota` merges per window and keeps a window a post leaves out, so `Quota.At` does not
    date the `seven_day` window: a stream of posts carrying `five_hour` alone keeps it fresh while
    `seven_day` ages. No window carries a time of its own.
  - The only guard on it today, `quotaGuardBlock` (`quota.go:244`), reads `five_hour` alone and never
    blocks on an unknown reading.
- **E10. Config keys and versions.**
  - `config/fields.go:31-32` declares `MinVersion = 1` and `LatestVersion = 7`.
  - Each key carries its version, and `go generate ./internal/panel/config` writes `docs/panel.md`'s
    table and `docs/panel.schema.json` (`TestGeneratedFilesAreCurrent`).
  - The template declares version 7 and this repo's `.clauductor/panel.json` version 6.
  - STALL-1 requires the template to declare the latest version.
- **E13. One list of what a config runs.**
  - `runEntries` (`config/config.go:1083`) lists each command a config runs.
  - `RunList` prints that list for **Trust config…**.
  - `SidecarRefusals` (`config/sidecar.go:52`) judges every command on that list for an attached
    project's sidecar (ATTACH-3).
- **E11. The review cap.**
  - `build-change.js:524-527` stops a group at the cap, saying what is left goes to KNOWN LIMITS.
  - `merge-pr` step 3 (`template/.claude/skills/merge-pr/SKILL.md:117-135`) puts it there and
    merges, writing `Review: capped at <n> round(s), peak <sev>, at <sha>; see KNOWN LIMITS`.
  - It writes `Review: converged in <n> round(s), peak <sev>, at <sha>` when the last round was
    clean.
- **E12. Per-project state.**
  - Per-project state lives in `config.ProjectDir(home, project)` (`lanes.json`, `notifier.json`,
    `spend.json`, `stall.json`).
  - Catch-up's rebuild restarts the panel after a merge (CATCHUP-4), so a state held only in memory
    is lost exactly when the runner needs it.
  - That rebuild is set off by the same merge as the runner's start, so the two are ordered: the
    runner waits for catch-up to settle (the runner's step 3).

## Decisions

- **D1. The switch is a `plan_runner` object in `panel.json`, config version 8, read only while the
  config is trusted.** **Recommended.**
  - **The keys:**
    - `on`: default false.
    - `next`: the command that names the plan's next row, an argv like a suggest command's. The
      template's is `["sh", ".claude/panel-suggest.sh", "next"]`.
    - `seven_day_pct`: the ceiling, D4.
  - **Load errors:**
    - a version below 8 that sets any of them;
    - `on` with no `next`;
    - `seven_day_pct` outside 1 to 100;
    - `on` while `alerts.notify` is false, since every stop is a notice, and a runner that stops in
      silence is the failure this row exists to avoid.
  - **Trust:**
    - The plan runner becomes one of the config's run entries (E13), with the line
      `plan runner runs <next>, starting lanes until the 7-day usage reaches <pct>%`.
      **Trust config…** then shows what the owner turns on.
    - Because it is on that list, an attached project's sidecar holds `next` to ATTACH-3's rule
      (literal argv, nothing outside the repository) with no change to that rule.
    - A merged config with the runner on is never trusted on arrival (D9).
  - The template ships `on: false` and declares version 8.
  - **Beat (b): a machine-local file like `catch-up.json`.**
    - The row asks for a project key.
    - The plan is the project's, so its runner's settings belong beside its templates.
    - The owner's trust still makes each machine's choice.

- **D2. The next row comes from a new `panel-suggest.sh next` kind, and the runner starts it only
  when exactly one template's fresh Up next lists its name.** **Recommended.**
  - **What `next` prints.** At most one item, using the rules the other kinds already use:
    - the first entry of the open session (the one `close` reads) in the plan's order that has not
      landed and is not a batch's later row;
    - or, once every one of them has landed, that session's close;
    - or `[]` when there is no plan or no open session.
  - **Its fields.** The item carries `kind` (`build`, `propose`, `ops`, `fix` or `close`), `row`,
    and `decision`, which is the `D` record's text when the plan names one for that row.
  - **Its name** is the one its kind's list uses: the change id, the `ops/` branch without its
    prefix, `session-N-close`.
    - A `fix/` row's is its branch without its prefix.
    - The fix template lists open issues, not roadmap rows (`panel-suggest.sh:45-53`), so it names
      that item only when the branch is the issue's `<n>-<title slug>`. Otherwise step 5 stops the
      run, naming the item, and the owner starts the fix by hand.
  - **The runner's check.** Before a start, it runs every template's suggest command once more. It
    goes on only when exactly one fresh list names the item, so it starts nothing the page would
    not offer.
  - **Beyond the row's surfaces.** This adds `template/.claude/panel-suggest.sh`, its check and the
    plugin build. The row was not split and has taken on nothing since, so step 0c does not split
    it.
  - **Beat (b): Go reads the plan's `pos` fields itself, or takes Up next's first item.**
    - The first is a second parser of the plan, which the one-parser rule forbids.
    - The second is the wrong row (E3).

- **D3. A decision still the owner's stops the runner before the start.** **Recommended.**
  - The runner offers the row and pushes a notice naming the decision when either:
    - the plan's *The owner decides* names the next row (any `D` record for its id);
    - the next row is a change with no proposal on main (`kind: propose`), whose design is the
      owner's by *Who decides*. It stops even where a project delegates a change's approval to
      Claude (OPS-127): a delegated approval is still a decision, and the runner takes none.
  - This is fail-safe: a decision already taken but still in the cell stops the run for nothing,
    never runs past one.
  - OPS-128 makes the cells keep only what stays the owner's.
  - **Beat (b): start the proposal's lane and end the run there.**
    - A propose lane's PR waits for the owner, so the run would wait on it.
    - When that PR merges, the build of the same change needs the propose lane's branch, so it
      would be refused `path-taken`.
  - **Beat (c): read the decision's words for "approved".** That guesses at prose.

- **D4. The ceiling: the account's 7-day usage, 95% by default. The owner's decision, 2026-10-25
  ("95% weekly").** **Decided.**
  - **The rule.**
    - `plan_runner.seven_day_pct` (default 95) is the amount, so it stays the owner's to change.
    - Before every start, the runner reads the `seven_day` window of the panel's account quota
      (E9's reader: the status line's `rate_limits`, folded by `foldQuota`, read through `quotaAt`).
    - At or above the amount, it stops with a push naming the reading.
  - **Unreadable stops the run as well (fail closed).** Each of these counts as unreadable:
    - no reading;
    - no `seven_day` window;
    - a window with no percentage, or past its reset;
    - a `seven_day` window last carried by a post more than 30 minutes ago.
    The runner never starts on a number it does not have.
  - **Each window's own time.** `QuotaWindow` gains `at`, set by `foldQuota` on each window a post
    carries. The 30 minutes are measured on the `seven_day` window's `at`, never on `Quota.At`
    (E9).
  - **The other guards still apply.** The 5-hour quota guard still refuses through `StartLane`,
    and the runner never sets `OverrideQuota`.
  - **No new reader.** The 7-day reading is the one the page's quota gauge already shows, so no
    reader is added.
  - **Beat (b), the recommendation before the owner decided: the run's lanes' measured list-price
    dollars per run.**
    - It counted only this project's lanes.
    - It had no reading for the quota the account actually runs out of.

- **D5. One lane of the run at a time.** **Recommended.**
  - The runner starts a lane only when the lane it tracks merges, so it never runs two.
  - Lanes the owner starts by hand are untouched.
  - Two full gates on one machine never overlap anyway: the gate lease serializes them. So the
    row's question about overlapping gates does not arise from the runner.
  - **Beat (b): the plan's lanes side by side.**
    - The plan's order encodes what waits for what, and the runner cannot read which rows are
      independent.
    - A later row started early would build on a main that lacks the row before it.

- **D6. The run tracks one branch, starts on its merge, and ends at the session's close.**
  **Recommended.** The mechanism is below.
  - **Which lane the run tracks.**
    - It tracks the lane on the branch of `next`'s item, whether the runner started it or the owner
      did. That is how a run begins and how a stopped run resumes: the owner starts the next row's
      lane, and the runner adopts it.
    - The trigger is a pull request of that branch merging after the runner began tracking it.
  - **What happens on the merge.** The runner:
    1. reads the merged PR's `Review:` line (D8);
    2. fetches the config's base ref;
    3. confirms the merge commit is on it;
    4. reads `next` again;
    5. starts the item.
  - **The close.** When the merged PR was the session's close, the run ends with a push: the next
    session starts from the owner's click.
  - **Beat (b): run on into the next session.**
    - Each session's plan entry has its own owner decisions and estimate.
    - A run that never ends would cross them unread.

- **D7. A lane of the run that stops short gets one push, from the stall alert when it stalls and
  from the runner when it does not.** **Recommended.**
  - **When the lane is stalled.** A failed gate never merges, so the run waits on the lane. When
    PANEL-51 marks the lane stalled, the stall's push is the one push, and the run's state says
    "waiting on <lane>: stalled".
  - **When it is not.** A lane can stop with no open work, for example after a failed gate with no
    PR and no receipt; STALL-1 then marks nothing. The runner pushes once when both hold for the
    stall time (`alerts.stall_minutes`, or 30 when that is 0):
    - no live session of the lane is working;
    - the lane has not moved (both in STALL-1's sense).
  - **The first prompt.** The runner pushes when a lane it started has not had its first prompt
    delivered within 10 minutes, or had it skipped.
  - The run continues when the lane merges.
  - **Beat (b): the runner pushes on every stall as well.** That sends two pushes for one event.

- **D8. No mediums. The owner's decision, 2026-10-25 ("I don't want any mediums").** **Decided.**
  - **(a) This change's own build.**
    - Every group converges with nothing medium or worse left.
    - When a group's cap round still finds a medium or worse, `build-change.js` stops there (E11).
    - The build session then stops for the owner instead of listing the findings under KNOWN
      LIMITS and merging.
    - At merge, `merge-pr`'s cap rule likewise stops instead of merging past a medium.
    - What executes this: **Nothing. You.** The build session alone. No hook or check reads it,
      and the workflow's own stop message points the other way.
  - **(b) The runner stops on what merged past its review.**
    - It stops the chain and pushes when the merged PR's body has a `Review:` line saying
      `capped`, or `NOT converged`, or has none, or more than one, or one it cannot parse.
    - It goes on after `Review: converged in <n> round(s), peak <sev>, at <sha>`, whatever the
      peak: the peak is the worst finding of any round, fixed ones included.
    - A proposal PR's line, which also ends `· design <hash>`, reads the same.

- **D9. A merged config with the runner on is never trusted on arrival.** **Recommended.**
  - CATCHUP-3 gains one more reason not to trust on arrival: the new config's `plan_runner.on` is
    true.
    - That covers turning the runner on, and changing any key while it stays on.
    - Turning it off is still trusted on arrival. So is any config with the runner off, as today.
  - **Why every change while on, not only the runner's keys.** The runner starts lanes from the
    whole config: a template's `first_prompt`, `branch_pattern` and suggest command, and
    `worktree_setup`. A merged config that rewrites any of those would set what the runner starts
    next with nobody watching, possibly on the very merge that brought it.
  - Claude merges from the owner's account, so without this a pull request could grant itself a
    runner, or rewrite the config the runner starts from.
  - **What trust does not cover.** The scripts the config runs are repository files: `next`
    (`.claude/panel-suggest.sh`), the suggest commands, and the framework source catch-up rebuilds
    from. Any merged PR can change them with no trust step, since the run list trusts their argv,
    not their content. This is true of every suggest command today. `checks/panel-suggest.sh` and
    the review that every PR gets hold them, not D9.
  - **The cost.** While the runner is on, every merged `panel.json` change waits for the owner's
    **Trust config…**, and the runner pauses until then (an untrusted config runs nothing). The
    file changes rarely.
  - **When the merge rebuilds the panel.** `config.Check` gains whether the runner is on, so
    `config-check` prints it and the running binary reads it.
  - **An answer with no `plan_runner` reads as off.** It comes from an older binary, say one a
    revert builds.
    - Such a binary has no runner, and a config that sets the key does not load in it.
    - So off is the truth, and the rebuild is not refused for it. A failed `config-check` discards
      the whole rebuild (`selfupdate.go:142-151`).
  - **Beat (b): guard only the runner's own keys** (on, or changed while on), and trust every other
    change as catch-up does today. It leaves the templates the runner starts from to the arrival
    notice's `+`/`−` lines, which name only the run list. Those tell the owner after the fact,
    while the runner already acts.
  - **Beat (c): rely on the run list's diff in the arrival notice alone.** The same objection, for
    the switch itself.

- **D10. This repository turns the runner on in a PR of its own, once this change's binary serves
  it. The template ships it off.** **Recommended.**
  - This PR leaves this repository's `.clauductor/panel.json` alone.
  - **Why not in this PR.**
    - This PR changes `framework/`, so its merge rebuilds the panel (CATCHUP-4).
    - On that merge, the running binary is the one from before this change, and it judges the
      arriving config on `name`, `notify`, `runs` and `tmux_socket` alone (E8).
    - It would trust a config that turns the runner on. Only the notice's `+ plan runner runs …`
      line would tell the owner, after the fact.
  - **The follow-up** is PANEL-59 (`ops/panel-59-plan-runner-on`), a roadmap row this PR adds.
    - It moves this repository's config to version 8, with `plan_runner.on` true and the
      template's `next`, and `seven_day_pct` left at its default of 95.
    - It merges once this change's binary serves the repository, so D9 judges its arrival and it
      waits for the owner's **Trust config…**.
    - **What holds that order** is the version, not a reader. A version-8 config does not load in
      a binary whose `LatestVersion` is 7, so if PANEL-59 arrived first, CATCHUP-3 would keep the
      loaded config and send the load error. Its bytes are never recorded as trusted, so the next
      binary serves the file untrusted.
    - Then S13 can be the first session the panel starts itself.
  - **Beat (b): turn it on in this PR.** The older binary would trust it with no click.
  - **Beat (c): leave the turn-on to the owner's hand edit.**
    - Catch-up needs the main checkout clean, so the edit is a PR anyway.
    - Nothing in the roadmap would name it.

- **D11. One change, in seven groups.** **Recommended.**
  - **Beat (b): split the row into the `next` kind with the config, and the runner.**
    - Neither half ships a capability alone. The `next` kind and the key would have no production
      caller until the runner merged (AGENTS.md rule 3).
    - Each group here is sized for one review round.

## The runner

The runner is one loop of the project's runtime, next to auto-close. It acts only while
`plan_runner.on` is true and the config is trusted. Its state is `plan-run.json` in the project's
state directory, written before each step that starts or stops something, so a restart (catch-up's
rebuild, a crash) resumes it:

```json
{"branch": "change/panel-58-plan-runner", "row": "PANEL-58", "kind": "build", "lane": "panel-58",
 "since": 1761400000, "state": "watching", "stop": {"why": "…", "key": "…", "at": 1761400000}}
```

`state` is one of:
- `idle`: no branch is tracked;
- `watching`: the tracked branch's lane works;
- `starting`: a start is under way, written before `StartLane` and cleared after it.

**Adopt.** While idle, the runner reads `next` on the suggest commands' cadence. A running lane on
the item's branch is adopted: the runner records its branch, its row, its kind and the time. A lane
whose HEAD a merged PR already holds is not adopted, and `planRun` says why.

**Trigger.** While watching, the runner reads the tracked branch's merged pull requests
(`mergedPRList`, with `number,body,mergedAt,mergeCommit`):
- when the branch leaves the open list;
- every 5 minutes;
- on start.

A PR merged after `since` is the trigger. Then the runner goes through these steps in order. Each
one that fails is a stop:
1. **The `Review:` line**, as D8 (b) reads it.
2. **The close.** When the tracked kind was `close`, the run ends: a stop whose notice says the
   session has closed.
3. **The fetch.** `git fetch <remote> <branch>` of the config's base ref, in the main checkout,
   under a timeout. Then `git merge-base --is-ancestor <merge commit> <base ref>`.
   - A failed fetch is retried once, 30 seconds later, before it is a stop. Lanes and catch-up
     fetch the same ref at the same time, and catch-up already treats its own failed fetch as
     transient (`catchup.go:411-416`).
   - **Then catch-up settles.** While catch-up is on for the project, the runner wakes it
     (`catchUp.wake`) and waits for a check of the project's root that began after the fetch to
     end, for 15 minutes at most; past that, it is a stop.
   - Why: the same merge often changes `framework/`, so catch-up rebuilds the panel and restarts
     it (CATCHUP-4). The restart's `quiesce` (`selfupdate.go:209`) waits for the panel's claude
     calls, not for a lane start. A restart during step 8 is the `starting` stop, and one during
     the new lane's first prompt leaves it `typing`, which is never retyped (`types.go:119-121`).
   - A check that rebuilds never ends in this process: the panel restarts first. The state is
     still `watching`, so the new binary finds the merge again on start and runs these steps
     there.
   - **Beat (b): make the restart's quiesce wait out a `starting` run.** That covers step 8, but
     not the first prompt typed after it, and it holds every rebuild on the runner.
4. **The next row.** `next` again. `[]` means the plan has no row left. A `decision`, or
   `kind: propose`, is D3's stop.
   - **The row and kind that just merged** are a stop as well. `next` passes a row only once a
     fragment on main names it, and nothing requires one (writing it is **Nothing. You.**).
   - Without this stop, a merged row with no fragment would be named again. Its branch is deleted
     and its worktree closed, so step 6 would pick `new`, and the runner would rebuild merged work.
   - The stop compares the row **and** its kind. When the tracked lane was the row's `propose`
     and `next` now names the same row's `build`, the proposal has landed, and that is not a
     missing fragment. Writing one would mark the row landed, and `panel-suggest.sh` would then
     never offer its build.
   - **A merged proposal stops the run too**, with its own notice: the proposal of the row has
     merged, and its build starts with the owner's click. A build is the first run of a design
     the owner has just approved, and the propose skill gives it a session of its own.
   - **Beat (b): start the build.** The approval is on main by then, so this is safe. But it
     would make the owner's approval click also start the build, and the owner has not asked for
     that.
5. **The template.** Every template's suggest command runs once, and its result updates Up next as
   a refresh would. Exactly one list must name the item.
6. **Where it runs.** `TemplateBranch`'s facts, chosen as `planStart` chooses (E2), in Go, in this
   order:
   - busy is a stop;
   - a branch or worktree whose HEAD a merged PR already holds (`mergedPRRead`) is a stop. That is
     merged work, a finished propose lane on the same `change/<id>` among it, and a lane would
     build on it;
   - a lane already in the worktree is adopted, and nothing starts;
   - otherwise `existing`, `branch` or `new`.
   - The Go rule runs over the same cases as `planStart`'s own test, `web/start_plan_test.go`, so
     the two copies cannot drift apart unseen.
7. **The ceiling**, D4.
8. **The start.** `state: starting`, then `StartLane` with the template, the name, the mode, the
   worktree and the item's issue, through the same `StartGate` the route passes. Then
   `state: watching` on the new lane's branch, with `since` set to now.

**After the start.** The first prompt and a lane that stops short are D7's.

**A restart.** On start, a `starting` state is a stop ("the panel restarted while starting <row>").
Adoption then tracks the lane if one runs on that branch.

**A stop.**
- The runner clears the tracked branch, writes `stop`, and sends one notice. The notice is an OS
  notification and a push through `push.json`, with the stall push's file, timeout and title rules
  (STALL-3).
- `stop.key` is the reason and the merge or row it is about. A restart does not send the same
  notice twice.
- `/api/state` carries `planRun`: `on`, the state, the row, the lane, and the last stop's text and
  time. Up next shows it as one line above its rows.

**What it never does.**
- It never starts a lane while `on` is false or the config is untrusted.
- It never sets `OverrideQuota`.
- It never starts while it tracks a lane whose PR has not merged.
- It never starts a lane `next` did not name.

**ATTACH-2** holds:
- its reads of `next` and the merged PRs write nothing;
- its fetch and its starts are actions the owner turned on, as **Start lane** is, not attaching,
  loading or polling.

## Refusals

| Situation | What happens instead |
|---|---|
| `on` is false, or the config is untrusted | Nothing starts; `planRun.on` says why |
| A version below 8 sets `plan_runner`, `on` without `next`, a `seven_day_pct` outside 1–100, or `on` with `alerts.notify` false | The config does not load, naming the key |
| A merged config has the runner on (turned on, or any change while on) | Not trusted on arrival; the notice says the config waits for the owner (D9) |
| The merged PR's `Review:` line is capped, NOT converged, missing, doubled or unreadable | Stop, push naming the PR and the line |
| The merged PR was the session's close | The run ends, push naming the session |
| The fetch fails twice, 30 seconds apart, or the merge commit is not on the base ref | Stop, push naming the error |
| Catch-up's check after the fetch has not ended in 15 minutes | Stop, push naming the project |
| `next` fails, times out or prints something unreadable | Stop, push naming the error |
| `next` prints `[]` | Stop, push: the plan has no row left |
| `next` names the row and kind that just merged (no fragment on main names it) | Stop, push naming the row and the PR |
| The tracked lane was a row's proposal, and `next` names that row's build | Stop, push: the proposal merged, and its build starts with the owner's click |
| The next row has a decision in the plan, or is a change with no proposal on main | Stop, push naming the row and the decision; the row stays offered |
| No template's fresh Up next names the item, or more than one does | Stop, push naming the item |
| The item's branch is busy, or holds work already merged | Stop, push naming the branch |
| A lane already runs on the item's branch | It is adopted; nothing starts |
| The 7-day usage is at or above `seven_day_pct`, or unreadable (its window older than 30 minutes included) | Stop, push naming the reading or why there is none |
| `StartLane` refuses (the 5-hour guard, `path-taken`, `branch-exists`, …) | Stop, push naming the refusal |
| The panel restarted during a start | Stop, push; a lane on that branch is adopted |
| The first prompt is not delivered within 10 minutes, or is skipped | Push once; the run keeps tracking the lane |
| The run's lane is not working and has not moved for the stall time, unstalled and unmerged | Push once; the run keeps tracking the lane |
| The run's lane is stalled | The stall's push is the one push; `planRun` says the run waits on it |
