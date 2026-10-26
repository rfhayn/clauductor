## Purpose
The panel runs a session's plan by itself where a project turns that on. When the lane of the
plan's next row merges, the panel starts the row after it, through the same path and guards as
**Start lane**. It stops with one notice wherever the owner is needed or a start is unsafe: an
owner's decision, a review that did not converge, the 7-day usage, the end of the session.

## ADDED Requirements

### Requirement: The plan runner is a project key, off until a trusted config turns it on
THE SYSTEM SHALL read the plan runner from the config's `plan_runner` object, a config version 8
key, with `on` (default false), `next` (an argv) and `seven_day_pct` (default 95). SHALL refuse to
load a config that sets it in a version below 8, sets `on` true with no `next`, sets
`seven_day_pct` outside 1 to 100, or sets `on` true while `alerts.notify` is false, naming the key.
WHILE `on` is true THE SYSTEM SHALL list the runner in the config's run list as
`plan runner runs <next>, starting lanes until the 7-day usage reaches <pct>%`, followed by
`, adopting lanes of kinds <kinds>`, the kinds of PLANRUN-9 that have a template, each once, in
sorted order, joined by `, `, or, WHEN no template has one, by `, adopting no lane: no template's suggest command is
<next> with its last element replaced`. WHILE `on` is false
or the config is not trusted THE SYSTEM SHALL NOT run `next` or start a lane for the runner.
WHILE a trusted config has `on` false or no `plan_runner`, and `plan-run.json` tracks a branch, is
`starting` or holds the session's close (PLANRUN-2), THE SYSTEM SHALL write it `idle` once,
tracking nothing and holding nothing, so that turning the runner on again resumes only through
PLANRUN-9's check and never acts on a merge made while it was off. WHILE the config is not
trusted, THE SYSTEM SHALL keep the state as it is.

#### Scenario: [PLANRUN-1-S1] With no key, a merge starts nothing
- **GIVEN** a trusted config with no `plan_runner`, a session plan whose next rows are A and B, and
  a lane on A's branch
- **WHEN** A's pull request merges
- **THEN** no lane starts and `next` is not run
- **AND** `/api/state` carries `planRun.on` false

#### Scenario: [PLANRUN-1-S2] A malformed key does not load
- **GIVEN** in turn a config at version 7 that sets `plan_runner`, one with `on` true and no `next`,
  one with `seven_day_pct` 0, one with `seven_day_pct` 101, and one with `on` true and
  `alerts.notify` false
- **WHEN** each is loaded
- **THEN** each fails to load, with an error naming the key

#### Scenario: [PLANRUN-1-S3] Trust config… names the runner
- **GIVEN** a config with `plan_runner.on` true, `next` `["sh", ".claude/panel-suggest.sh", "next"]`,
  no `seven_day_pct`, and two templates whose suggest commands are
  `["sh", ".claude/panel-suggest.sh", "propose"]` and `["sh", ".claude/panel-suggest.sh", "build"]`
- **WHEN** its run list is printed for **Trust config…**
- **THEN** it holds the line `plan runner runs ["sh" ".claude/panel-suggest.sh" "next"], starting
  lanes until the 7-day usage reaches 95%, adopting lanes of kinds build, propose`
- **AND** with `next` `["sh", "scripts/next-row.sh"]` instead, the line ends `, adopting no lane: no
  template's suggest command is ["sh" "scripts/next-row.sh"] with its last element replaced`
- **AND** with `on` false the run list holds no plan runner line

#### Scenario: [PLANRUN-1-S4] An untrusted config runs nothing, and trusting it resumes the run
- **GIVEN** a runner tracking the lane on `change/a`, the plan's row A, and then a config with the
  plan runner on that is not trusted, served while A's pull request merges
- **WHEN** the runner's next tick runs
- **THEN** no lane starts and `next` is not run, and `plan-run.json` still tracks `change/a`
- **AND** after the owner's **Trust config…**, A's merge is found and the row after A starts

#### Scenario: [PLANRUN-1-S5] A sidecar's `next` is held to the sidecar rule
- **GIVEN** an attached project whose sidecar sets `plan_runner.on` true and `next`
  `["sh", "-c", "echo []"]`
- **WHEN** the owner runs `clauductor panel trust --project <repo>`
- **THEN** trust is refused, and the plan runner's line is marked as not allowed in a sidecar

#### Scenario: [PLANRUN-1-S6] Turned off, the run lets go, and turning it on acts on no old merge
- **GIVEN** a runner tracking the lane on `change/a`, the plan's row A, whose plan's row after A is B
- **WHEN** a trusted config turns the runner off, A's pull request merges, and the owner then
  turns the runner on again and trusts that config
- **THEN** `plan-run.json` is `idle`, tracking nothing, from the runner's first tick while off
- **AND** after the turn-on, no lane starts and no notice goes out, and A's lane is not tracked
- **AND** when instead the runner was holding the session's close when it was turned off, and the
  owner started a build lane on the next session's first row while it was off, that lane is
  tracked at the turn-on's first idle read

### Requirement: The run tracks one lane, and starts the plan's next row when that lane merges
WHILE the plan runner is on and the config is trusted, THE SYSTEM SHALL track the lane on the
branch of `next`'s item, whether the runner started it or the owner did. A lane the runner did not
start in that same step SHALL be tracked only as PLANRUN-9 allows, whether or not any Up next list
names the item. WHEN a pull request of the tracked branch has merged since the branch
was tracked, and the checks of the next three requirements pass, THE SYSTEM SHALL start `next`'s
item:
- from the one template of the item's kind (PLANRUN-9) whose fresh suggest list names it, the
  lists of templates of other kinds or of none not read for it, or, WHEN no template is of any
  kind, from the one template among all whose list names it;
- through `StartLane` with the trust and quota gate the **Start lane** route passes, never setting
  `OverrideQuota`;
- in the mode the page would choose for that branch.

THE SYSTEM SHALL start at most one lane for a merge, none while the tracked lane's pull request has
not merged, and none for the merge of a branch it does not track. SHALL keep the run's state in
`plan-run.json` in the project's state directory, so that a restart neither loses a merge nor
starts a lane twice. WHILE catch-up is on for the project, SHALL begin no start until catch-up has
settled on the merge: a check of the project's root that began after the runner's fetch has ended
with the panel's context alive, and either the main checkout's HEAD contains the merge commit or
no retry of catch-up is pending for the root. That way a rebuild the merge sets off, a retried one
included, restarts the panel before the start and never during it. SHALL stop with a notice WHEN
catch-up has not settled within 15 minutes of the fetch. WHEN the project runtime's context ends
during a step, whatever ends it (the panel's exit, a config arrival's reload, a **Trust config…**
that reloads, `projects.json` moving or removing the project), THE SYSTEM SHALL treat the step as
cut short, not failed: it SHALL write nothing and send no notice. Before the start the state
SHALL stay `watching`, so the next runtime finds the merge again. During the start it SHALL stay
`starting`, and no runtime's stop SHALL wait for more than the start's return on its cancelled
context. The `starting` state SHALL record `next`'s item, whole, and its branch. WHEN a runtime
starts on a `starting` state, THE SYSTEM SHALL run PLANRUN-9's check for the recorded item and
track the lane it allows with no notice, or, WHEN no template is of the item's kind or no lane runs
on any candidate, start nothing and stop with a notice naming the row, the branch and what is on
it, a lane running there included, whatever the check's candidates were. And
WHEN the merged pull request was the session's close, whatever its `Review:` line, THE SYSTEM
SHALL start nothing and end the run with a notice, reading the close before that line, and from then on SHALL track no lane created at or before that merge, until it
tracks a lane created after it. WHILE watching, WHEN no pull request of the tracked branch has
merged since it was tracked, the tracked lane has no record in the lane registry, and the branch
has no open pull request, THE SYSTEM SHALL start nothing and stop with a notice naming the lane, so
the run is idle and tracks the next lane PLANRUN-9 allows. A failed read of either list SHALL stop
nothing.

#### Scenario: [PLANRUN-2-S1] A tracked merge starts the plan's next row with no click
- **GIVEN** a trusted config with the plan runner on, a session plan whose next rows are A then B
  (two change rows with their proposals on main), and a lane on `change/a` that the runner tracks
- **WHEN** A's pull request #9 merges with the line `Review: converged in 1 round(s), peak low, at
  abc1234`
- **THEN** a lane of the build template starts on `change/b` in a new worktree, through `StartLane`
- **AND** `plan-run.json` tracks `change/b`, and the panel's log has a `plan runner started` line
  naming the lane

#### Scenario: [PLANRUN-2-S2] A lane the owner started is adopted
- **GIVEN** a trusted config with the plan runner on, tracking nothing, and a plan whose next row is
  A
- **WHEN** the owner starts a lane on A's branch, and the runner's next tick runs
- **THEN** `plan-run.json` tracks A's branch
- **AND** when A's pull request merges, the row after A starts
- **AND** when instead `next` names A's proposal and the owner's propose lane on `change/a` has
  already pushed it, so the propose list no longer names A, that lane is tracked all the same,
  with kind `propose`

#### Scenario: [PLANRUN-2-S3] The mode follows where the branch already is
- **GIVEN** a tracked merge whose next row is B
- **WHEN** in turn B's branch has a worktree with no lane in it, B's branch exists with no worktree,
  and a lane already runs in B's worktree
- **THEN** the runner starts B's lane in `existing` mode, then in `branch` mode, and in the third
  case, where the running lane is of B's kind, starts nothing and tracks the running lane
- **AND** when B's branch's HEAD is a merged pull request's head, nothing starts, nothing is
  adopted, and one notice names the branch

#### Scenario: [PLANRUN-2-S4] A merge the run does not track starts nothing
- **GIVEN** a trusted config with the plan runner on, tracking `change/a`
- **WHEN** a pull request of `fix/12-typo` merges
- **THEN** no lane starts, and `plan-run.json` still tracks `change/a`

#### Scenario: [PLANRUN-2-S5] A restart neither loses a merge nor starts twice
- **GIVEN** a runner tracking `change/a`, and A's pull request merged while the panel was stopped
- **WHEN** the panel starts, and later restarts again
- **THEN** the row after A starts once, at the first start, and the second restart starts nothing

#### Scenario: [PLANRUN-2-S6] The next runtime resolves a start that was cut short
- **GIVEN** `plan-run.json` in state `starting` for B on `change/b`, left by a runtime stopped
  during `StartLane`, or by a panel process killed during it
- **WHEN** the next runtime starts, and a lane runs on `change/b`
- **THEN** no lane starts, no notice goes out, and `plan-run.json` is `watching` `change/b`
- **AND** when instead no lane runs on `change/b`, no lane starts, and one notice says a restart
  cut short the start of B on `change/b`, naming what is there now

#### Scenario: [PLANRUN-2-S7] The session's close ends the run
- **GIVEN** a runner tracking the lane `session-13-close` of the close template
- **WHEN** its pull request merges
- **THEN** no lane starts, `plan-run.json` tracks nothing, and one notice says the session has
  closed
- **AND** when `next` then names the next session's first row A, and a build lane the owner
  started before the close merged runs on `change/a`, it is not tracked, no notice goes out, and
  `planRun.waiting` names the lane, says it started before the session closed, and says to close
  it keeping its worktree and start the row from Up next
- **AND** when the owner closes that lane keeping its worktree and starts a lane on `change/a`
  after the close, that lane is tracked, which ends the hold
- **AND** when instead the close's pull request merges with `Review: capped at 3 round(s), peak
  medium, at abc1234; see KNOWN LIMITS`, or with no `Review:` line, the run ends the same way, with
  the one notice that the session has closed, and the build lane started before it is not tracked

#### Scenario: [PLANRUN-2-S8] A start waits for the rebuild its merge sets off
- **GIVEN** a project with catch-up on, a runner tracking `change/a`, and A's pull request #9,
  which changes `framework/`, merged with a converged `Review:` line
- **WHEN** the runner's fetch succeeds while catch-up's check of the project is rebuilding the
  panel
- **THEN** no lane starts in the running panel, and `plan-run.json` is still `watching`
  `change/a`
- **AND** when the rebuilt panel starts, the row after A starts once
- **AND** when instead catch-up's check ends with no rebuild, the row after A starts after it ends
- **AND** when instead catch-up's check ends on a failed fetch with a retry pending, no lane starts
  until the retry has fast-forwarded the checkout to #9's merge commit and its rebuild has
  restarted the panel, and the row after A then starts once, in the rebuilt panel
- **AND** when instead the check ends on a refusal of policy (a dirty main checkout) with no retry
  pending, the row after A starts after it ends
- **AND** when instead the check and each of catch-up's retries fail on a fetch, so the last one
  leaves no retry pending, the row after A starts after the last retry ends, within the 15 minutes
- **AND** when instead catch-up has not settled within 15 minutes, no lane starts, and one notice
  names the project and says catch-up has not settled

#### Scenario: [PLANRUN-2-S9] A step the runtime cuts short is not a stop
- **GIVEN** a runner tracking `change/a`, and A's pull request #9 merged with a converged
  `Review:` line
- **WHEN** the project's runtime is stopped while the runner is waiting for catch-up or running
  `next`: in turn by the panel's exit, by `projects.json` moving the project to another config
  path, and by the owner's **Trust config…** while the config file on disk differs from the bytes
  the runtime loaded
- **THEN** in each case the runner sends no notice, and `plan-run.json` is still `watching`
  `change/a` with no `stop`
- **AND** the next runtime starts the row after A once
- **AND** the runtime's stop is not held for the wait's 15 minutes
- **AND** when instead #9 changes `panel.json`, its arrival reloads the runtime on a config that is
  not trusted: the runner sends no notice, catch-up's notice says the config waits for the owner,
  no lane starts, and `plan-run.json` is still `watching` `change/a`
- **AND** after the owner's **Trust config…** of those bytes, which restarts nothing, the row
  after A starts once, at the runner's next trigger
- **AND** when instead the runtime is stopped during a `StartLane` that blocks in
  `worktree_setup`, the stop waits only for `StartLane` to return on its cancelled context, and
  `plan-run.json` is still `starting`, for PLANRUN-2-S6 to resolve

#### Scenario: [PLANRUN-2-S10] A tracked lane closed unmerged lets the run go
- **GIVEN** a runner tracking the lane `a` on `change/a`, whose build failed with no PR merged
- **WHEN** the owner closes `a`, deleting its registry record, and `change/a` has no open PR
- **THEN** no lane starts, one notice names `a` and says the run is idle, and `plan-run.json` is
  `idle`
- **AND** when the owner then starts row B's lane by hand, the idle read tracks it
- **AND** when instead `change/a` still has an open PR, or the read of the open list fails, the run
  keeps watching `change/a` and sends no notice
- **AND** when instead `a`'s PR merged and auto-close deleted its record, the merge is the trigger
  and no notice says the lane was closed
- **AND** when instead `a`'s session exited and its record stays, the run keeps watching

### Requirement: The run goes on only after a review that converged
WHEN the tracked pull request merges and it was not the session's close (PLANRUN-2), THE SYSTEM
SHALL read the `Review:` lines of its body, and go
on only when there is exactly one and it starts `Review: converged in <n> round(s), peak <sev>, at
<sha>`, whatever the peak, where `round(s)` may also read `rounds` or `round`, `<sev>` is one of
`none`, `low`, `medium`, `high` and `critical`, and `<sha>` is 7 to 40 lowercase hex characters
ended by the line's end or a character that is not hex. Whatever follows the sha SHALL NOT be
read. THE SYSTEM SHALL stop, with
a notice naming the pull request and the line, WHEN that line reads `capped` or `NOT converged`, or
WHEN the body holds no such line, more than one, or one it cannot parse.

#### Scenario: [PLANRUN-3-S1] A review that converged after a medium goes on
- **GIVEN** a runner tracking `change/a`
- **WHEN** A's pull request merges with the line `Review: converged in 2 round(s), peak medium, at
  abc1234`
- **THEN** the row after A starts
- **AND** the row after A starts the same way when the line is instead `Review: converged in 1
  round(s), peak low, at 8da629eb (three lows fixed after it)`, `Review: converged in 2 round(s),
  peak medium, at 59b438a6 · design 0d8677f3b540 (merge-pr step 3)`, `Review: converged in 1
  round(s), peak none, at dffb89cd3`, or `Review: converged in 3 rounds, peak medium, at e2af9de6`

#### Scenario: [PLANRUN-3-S2] A review capped past a medium stops the run
- **GIVEN** a runner tracking `change/a`
- **WHEN** A's pull request #9 merges with the line `Review: capped at 3 round(s), peak medium, at
  abc1234; see KNOWN LIMITS`
- **THEN** no lane starts, and one notice names "#9" and "capped"

#### Scenario: [PLANRUN-3-S3] A review that did not converge, or none to read, stops the run
- **GIVEN** a runner tracking `change/a`
- **WHEN** A's pull request merges, in turn with `Review: NOT converged after 3 round(s), peak high,
  open: one at abc1234`, with no `Review:` line, with two of them, with `Review: looks fine`, with
  `Review: converged in 4 groups (group 1: 1 round, peak low), at bbacccf9`, with `Review:
  converged in 1 round(s), peak minor, at abc1234`, and with `Review: converged in 1 round(s), peak
  low, at abc12`
- **THEN** in each case no lane starts, and one notice names the pull request and why

### Requirement: The run stops, and says why, before any start it cannot make safely
Before a start, THE SYSTEM SHALL start nothing and send one notice naming why WHEN:
- the fetch of the config's base ref fails, and fails again when retried 30 seconds later, or the
  merge commit is not on it;
- `next` fails, outlasts its timeout of 30 seconds, or prints anything but `[]` or one item;
- `next` prints `[]`;
- `next`'s item has the row and the kind of the tracked lane whose pull request just merged, which
  no fragment on main names yet;
- the tracked lane was a row's proposal (`kind: propose`) and `next`'s item is that row's build,
  the notice saying the proposal merged and that its build starts with the owner's click;
- the open session's *The owner decides* names the item's row (the item's `decision`,
  PLANRUN-8), or the item is a proposal to write;
- no fresh suggest list of a template of the item's kind (of any template, WHEN no template is of
  any kind: PLANRUN-2) names the item and PLANRUN-9's check allows no running lane
  for it, the notice naming the row, its roadmap state and the check's reason; or more than one
  list names it;
- the item's branch is busy, or its branch's or worktree's HEAD is a merged pull request's head;
- `StartLane` refuses.

WHEN the open session's *The owner decides* names the item's row, the item SHALL stay offered in
Up next.

#### Scenario: [PLANRUN-4-S1] A decision still the owner's stops the run
- **GIVEN** a tracked merge whose next row is B, and a plan whose open session's *The owner
  decides* reads `B: set its budget`
- **WHEN** the runner reads `next`
- **THEN** no lane starts, one notice names B and "set its budget", and B is still offered in Up
  next

#### Scenario: [PLANRUN-4-S2] A change with no proposal stops the run
- **GIVEN** a tracked merge whose next row is a change with no proposal on main
- **WHEN** the runner reads `next`, whose item has `kind` `propose`
- **THEN** no lane starts, and one notice names the row and says its proposal is the owner's to
  approve

#### Scenario: [PLANRUN-4-S3] A plan with no row left stops the run
- **GIVEN** a tracked merge, and `next` printing `[]`
- **WHEN** the runner reads it
- **THEN** no lane starts, and one notice says the plan has no row left

#### Scenario: [PLANRUN-4-S4] A `next` that fails or prints something else stops the run
- **GIVEN** a tracked merge
- **WHEN** `next` in turn exits 1, prints `{}`, prints two items, and never answers
- **THEN** in each case no lane starts, and one notice names the error, the last naming the timeout

#### Scenario: [PLANRUN-4-S5] A failed fetch, or a merge not on the base ref, stops the run
- **GIVEN** a tracked merge
- **WHEN** in turn the fetch of the base ref fails and fails again on its retry 30 seconds later,
  and the merge commit is not an ancestor of the base ref
- **THEN** in each case no lane starts, and one notice names why
- **AND** when instead the fetch fails once and its retry succeeds, the run goes on with no notice

#### Scenario: [PLANRUN-4-S6] An item no template lists, or two do, stops the run
- **GIVEN** a tracked merge whose next item is B
- **WHEN** in turn no fresh suggest list of a template of B's kind names B, and two such lists name
  it
- **THEN** in each case no lane starts, and one notice names B
- **AND** when B is instead `in flight`, so no list offers it, and a build lane runs on `change/b`,
  that lane is tracked, no lane starts and no notice goes out
- **AND** when B is `in flight` with no lane running, or is the ops row `ops/B-Release`, no lane
  starts, and one notice names B and its state, and for the second, that name
- **AND** when B is a build item that the build list names, and an ops template's list names `b`
  too, the ops list is not read for it, and B's lane starts from the build template

#### Scenario: [PLANRUN-4-S7] A busy branch, or a refused start, stops the run
- **GIVEN** a tracked merge whose next item is B
- **WHEN** in turn B's branch is busy, and `StartLane` refuses because the 5-hour usage is at the
  quota guard
- **THEN** in each case no lane starts, and one notice names the branch or the refusal
- **AND** the start request never sets `OverrideQuota`

#### Scenario: [PLANRUN-4-S8] A merged row that `next` still names is not started again
- **GIVEN** a runner tracking `change/a`, the plan's row A, and A's pull request #9 merged with a
  converged `Review:` line but no fragment naming A on main, its branch deleted and its worktree
  removed
- **WHEN** the runner reads `next`, which names A again
- **THEN** no lane starts, and one notice names A and "#9" and says no fragment on main names it

#### Scenario: [PLANRUN-4-S9] A merged proposal stops the run before its build
- **GIVEN** a runner tracking the propose lane on `change/a`, of row A, and A's proposal pull
  request #9 merged with a line `Review: converged in 2 round(s), peak medium, at abc1234 · design
  0123abcd`
- **WHEN** the runner reads `next`, which names A with `kind` `build`
- **THEN** no lane starts, and one notice names A and "#9" and says the proposal merged and its
  build starts with the owner's click
- **AND** the notice does not say a fragment is missing, and A's build is still offered in Up next

### Requirement: The run starts no lane at or above the 7-day ceiling, or on a reading it does not have
Before each start, THE SYSTEM SHALL read the `seven_day` window of the project's account quota, and
start only WHEN it reads below `seven_day_pct`. WHEN it reads at or above it, THE SYSTEM SHALL stop
with a notice naming the reading and the ceiling. WHEN there is no reading, no `seven_day` window, a
window with no percentage or past its reset, or a `seven_day` window last carried by a status post
more than 30 minutes ago (whatever the other windows' age), THE SYSTEM SHALL stop with a notice
saying why.

#### Scenario: [PLANRUN-5-S1] Below the ceiling, the run starts
- **GIVEN** a tracked merge, `seven_day_pct` 95, and a `seven_day` reading of 94% taken 5 minutes
  ago
- **WHEN** the runner reaches the start
- **THEN** the next row's lane starts

#### Scenario: [PLANRUN-5-S2] At the ceiling, the run stops
- **GIVEN** a tracked merge, `seven_day_pct` 95, and a `seven_day` reading of 95% taken 5 minutes
  ago
- **WHEN** the runner reaches the start
- **THEN** no lane starts, and one notice names 95% and the ceiling of 95%

#### Scenario: [PLANRUN-5-S3] An unreadable 7-day usage stops the run
- **GIVEN** a tracked merge, and `seven_day_pct` 95
- **WHEN** in turn there is no quota reading, the reading has no `seven_day` window, its window is
  past its reset, and the reading was taken 31 minutes ago
- **THEN** in each case no lane starts, and one notice says why there is no reading
- **AND** when a post 31 minutes ago carried `seven_day` at 50% and every post since carried
  `five_hour` alone, no lane starts, and the notice says the 7-day reading is 31 minutes old

### Requirement: A lane of the run that stops short is pushed once
THE SYSTEM SHALL record in `plan-run.json` whether it started the tracked lane: true for its own
start, and for the lane on the recorded branch that a restart's resolution of a `starting` state
tracks (PLANRUN-2), and false for every other lane PLANRUN-9 allows. WHEN a lane so recorded as
started has not had its first prompt delivered within 10 minutes of its start, or had it skipped,
THE SYSTEM SHALL send one notice naming the lane, and SHALL send none for a lane not so recorded. WHEN the tracked lane is
not stalled and has not merged, and no live session of it is working and the lane has not moved,
both in the stall's terms, for `alerts.stall_minutes` (30 when that is 0), THE SYSTEM SHALL send
one notice naming the lane. WHEN the tracked lane is stalled, approximate or not, THE SYSTEM SHALL
send no notice of its own, and `planRun.stalled` SHALL be true, read from the stall's own state:
an approximate stall's one push is the stall's, once it is no longer approximate (STALL-1-S25). In each case THE SYSTEM SHALL keep
tracking the lane, and go on WHEN it merges.

#### Scenario: [PLANRUN-6-S1] A first prompt not delivered is pushed, and the run goes on
- **GIVEN** a lane the runner started at 00:00 whose first prompt is still `pending` at 00:10
- **WHEN** the runner's tick runs at 00:10 and at 00:20
- **THEN** one notice naming the lane goes out at 00:10, and none at 00:20
- **AND** when the lane's pull request later merges, the row after it starts
- **AND** when instead the first prompt is `skipped`, one notice names the lane
- **AND** a lane the owner started at 00:00 and the runner adopted, its first prompt `pending` at
  00:10, gets no notice
- **AND** a lane on the recorded branch that a restart's resolution tracked, its first prompt
  `pending` 10 minutes after the recorded start, gets one notice naming it

#### Scenario: [PLANRUN-6-S2] A lane that stops with no open work is pushed once
- **GIVEN** `alerts.stall_minutes` 30, and a tracked lane whose gate failed with no pull request and
  no receipt, whose sessions are not working and which has not moved since 00:00
- **WHEN** the runner's tick runs at 00:30 and at 01:00
- **THEN** one notice naming the lane goes out at 00:30, and none at 01:00

#### Scenario: [PLANRUN-6-S3] A stalled lane gets the stall's push only
- **GIVEN** a tracked lane with its pull request open, stalled
- **WHEN** the notifier and the runner's tick run
- **THEN** the stall's notice goes out and the runner sends none
- **AND** `/api/state`'s `planRun.stalled` is true, and its `waiting` is empty
- **AND** when instead the lane's stall began across a restart at 01:00, with `alerts.stall_minutes`
  30, so it is approximate until 01:30, the runner sends no notice at 01:01 or at 01:29 while
  `planRun.stalled` is true, and at 01:30 the stall's one push goes out and the runner's tick still
  sends none

### Requirement: Each notice of the run goes out once, and the page shows the run
THE SYSTEM SHALL send each notice of the runner once, as a macOS notification and through the push
(`panel-stall`'s third requirement). SHALL record it in `plan-run.json` by its reason and the merge
or row it names before it sends it, so that a restart does not send it again: a notice lost to a
death between the two is accepted, and a notice sent twice is not. `/api/state` SHALL carry `planRun`, with
`on`, the state, the row, the lane, the last stop's text and time, `waiting` and `stalled`, each in
its own field. `waiting` SHALL be what the last idle read found: PLANRUN-9's refusal, or that `next`
named no row, or that it failed, naming the error. While idle, a `next` that fails SHALL send no
notice. `waiting` SHALL be empty while the runner is not idle, and while it is off or the config
is untrusted. The page SHALL show them as one line above Up next, with `waiting` beside the last
stop, neither replacing the other.

#### Scenario: [PLANRUN-7-S1] A stop's notice is not sent again after a restart
- **GIVEN** a run stopped at a capped review of #9, its notice sent
- **WHEN** the panel restarts and the runner's tick runs
- **THEN** no second notice goes out

#### Scenario: [PLANRUN-7-S2] `/api/state` and the page show the run
- **GIVEN** a runner tracking the lane `panel-58-plan-runner` on `change/panel-58-plan-runner`, and in turn a
  run stopped at a capped review of #9
- **WHEN** `/api/state` is read and the page renders Up next
- **THEN** `planRun` carries `on` true, the state, the row and the lane, and the page's line above
  Up next names the lane
- **AND** for the stopped run, `planRun` carries the stop's text naming "#9" and its time, and the
  page's line shows it
- **AND** when that run is idle and PLANRUN-9's check then refuses, waiting for a lane on
  `change/a`, `planRun` still carries the stop naming "#9" and carries `waiting` naming
  `change/a`, and the page's line shows both
- **AND** when that idle run's `next` then fails, no notice goes out and `waiting` names the
  error; when it prints `[]`, `waiting` says `next` names no row

### Requirement: `panel-suggest.sh next` names the plan's next row, in the plan's order
`panel-suggest.sh next` SHALL print, from `origin/<main>`, a JSON array of at most one item:
- the first entry of the open session, in the plan's order, that has not landed and is not a
  batch's later row, whatever its state on the roadmap and whatever its name, where an entry has
  landed as `close` counts it: its row merged or cancelled on the roadmap, or named by a fragment
  on main, and a planned `ID (proposal)` entry once its proposal is on main or archived, being
  otherwise a proposal to write. It SHALL NOT pass an entry that the other kinds' lists would not
  offer, for its state or for its name;
- or, once every entry of the open session has landed, that session's close;
- or `[]` when there is no plan or no open session.

WHEN that first entry not landed is an id that no roadmap row has, `panel-suggest.sh next` SHALL
print nothing on stdout, print a line naming the id on stderr, and exit non-zero. It SHALL NOT
pass the id for the entry after it.

The item SHALL carry `name`, `title`, `detail`, `issue`, `kind` (`build`, `propose`, `ops`, `fix`
or `close`), `row` (the roadmap row's id, and empty for the close, which is no roadmap row) and,
WHEN the open session's own *The owner decides* cell names its row,
`decision`: the text of each part of that cell naming the row, joined by `; ` in the cell's order.
A part in another session's cell SHALL NOT be the item's `decision`. Its `name`
SHALL be the one its kind's list uses, even WHEN that list would not offer it: the change id, the `ops/` branch without its prefix, the
close's `session-N-close`, and for a `fix/` row its branch without its prefix, which the fix
template's list of open issues names only WHEN it is that issue's `<n>-<title slug>`. Its `issue`
SHALL be `#<n>` for a `fix/` row whose name is `<n>-<slug>`, the form the fix kind's list gives
the same issue, and empty for every other item.

#### Scenario: [PLANRUN-8-S1] The next row is the plan's next, not the first template's
- **GIVEN** an open session whose plan lists an ops row A, then a change row B whose proposal is on
  main
- **WHEN** `panel-suggest.sh next` runs with neither landed, and again with A's fragment on main
- **THEN** it prints A, with `kind` `ops`, and then B, with `kind` `build`

#### Scenario: [PLANRUN-8-S2] A row the owner still decides carries its decision
- **GIVEN** an open session whose next row is B, and *The owner decides* reading `B: set its budget`
- **WHEN** `panel-suggest.sh next` runs
- **THEN** its item for B carries `decision` "set its budget"
- **AND** when that cell reads `B: set its budget; B: name its owner`, the item's `decision` is
  "set its budget; name its owner"
- **AND** when B was planned as `B (proposal)` in an earlier session whose cell reads `B: approve
  its proposal`, and the open session's cell names no part for B, its item carries no `decision`

#### Scenario: [PLANRUN-8-S3] A batch's later row is passed, and the close follows the last row
- **GIVEN** an open session N whose plan lists `A + B`, then C, and A's fragment, whose
  **Also closes** names B
- **WHEN** `panel-suggest.sh next` runs with A's fragment on main, and again with A's and C's
- **THEN** it prints C first, and then the close `session-N-close` with `kind` `close`
- **AND** when A's fragment does not name B, it prints B once A's fragment is on main, as the
  other kinds offer a batch's later row that its first row's PR did not close

#### Scenario: [PLANRUN-8-S4] With no plan or no open session, nothing is next
- **GIVEN** in turn a roadmap with no `## Session plan`, and one whose every session is `(done)`
- **WHEN** `panel-suggest.sh next` runs
- **THEN** it prints `[]`

#### Scenario: [PLANRUN-8-S5] A change with no proposal on main is a proposal to write
- **GIVEN** an open session whose next row is a change with no proposal on main
- **WHEN** `panel-suggest.sh next` runs
- **THEN** its item has `kind` `propose`

#### Scenario: [PLANRUN-8-S6] A planned proposal lands with its proposal, not its build
- **GIVEN** an open session whose plan lists `A (proposal)`, then an ops row B, and a later session
  that plans A's build
- **WHEN** `panel-suggest.sh next` runs with A's proposal not on main, and again with it on main
  and no fragment naming A
- **THEN** it prints A with `kind` `propose`, and then B with `kind` `ops`
- **AND** with A's proposal on main and B's fragment on main, it prints the open session's close

#### Scenario: [PLANRUN-8-S7] A fix row carries its issue, and no other item does
- **GIVEN** an open session whose plan lists a fix row on `fix/12-typo`, then an ops row B
- **WHEN** `panel-suggest.sh next` runs, and again with the fix row's fragment on main
- **THEN** it prints the fix row with `kind` `fix`, `name` `12-typo` and `issue` `#12`, and then B
  with `issue` empty
- **AND** a fix row on `fix/typo`, with no number, prints with `issue` empty

#### Scenario: [PLANRUN-8-S8] A planned row no list offers is still the next row
- **GIVEN** an open session whose plan lists a change row A whose proposal is on main, then an ops
  row B, with neither landed
- **WHEN** `panel-suggest.sh next` runs with A `in flight`, and again with A `deferred`, and again
  with A's change id `feature/A-release`
- **THEN** each time it prints A, never B: the first two with `kind` `build` and `name` A's change
  id, the last with `kind` `propose`, since no proposal is under that id, and `name`
  `feature/A-release`
- **AND** with A merged on the roadmap, it prints B

#### Scenario: [PLANRUN-8-S9] A planned id with no roadmap row fails `next`, naming it
- **GIVEN** an open session whose plan lists X, which no roadmap row has, then an ops row B, with
  neither landed
- **WHEN** `panel-suggest.sh next` runs
- **THEN** it exits non-zero, prints nothing on stdout, and its stderr names X
- **AND** with a fragment on main naming X, it prints B

### Requirement: A lane the runner did not start is tracked only through one check
THE SYSTEM SHALL read the item's candidate branches in one place. A template is of kind K WHEN its
suggest command is the `next` argv with its last element replaced by K, element for element. The
candidates are the branches the templates of the item's kind name for the item, filled by the
same function and from the same fields, the name and the issue, that a start fills its branch
from, equal ones counted once. A template whose branch cannot be filled for the item is no
candidate. THE SYSTEM SHALL record `next`'s item whole wherever the run keeps it, never a subset
of its fields. THE SYSTEM SHALL track a lane it did not start in that same step only
WHEN one check over those candidates allows it. The check SHALL take only the item, and look up
the project's running lanes on the candidates itself. It allows WHEN one lane runs on exactly one
candidate, that lane's recorded template is of the item's kind, and the merged pull requests are
read and none has the lane's HEAD as its head. Every path that tracks such a lane SHALL go through
that check: adoption while idle, step 5 for an item no list names, step 6, and a restart's
resolution of a `starting` state. WHILE the session's close holds (PLANRUN-2), the check SHALL also
require the lane to have been created after the close merged. WHEN
the check refuses, THE SYSTEM SHALL track nothing, and the reason SHALL be one of, in this order:
no template is of the item's kind, or each one of it was refused for the item, the reason saying
which and naming each refused template's error; no lane runs on any candidate; lanes, of any
template, run on more than one candidate; the lane's template is of another kind or none; the
merged pull requests could not be read; the lane's HEAD is merged work; the lane started before
the session closed, while the close holds, the reason naming the lane and the way through: close
it keeping its worktree and branch, and start the row from Up next. A failed read SHALL NOT be
taken for no merged pull request. While idle, THE SYSTEM SHALL name the reason in
`planRun.waiting` and send no notice. At step 6, no template of the item's kind and no lane on any
candidate SHALL each let the step choose a mode. On a restart, each of those two SHALL be
PLANRUN-2's stop naming the row and the branch. At step 6 and on a restart, every other reason
SHALL be a stop with its notice. At step 5, every reason SHALL be PLANRUN-4's stop for an item no
list names.

#### Scenario: [PLANRUN-9-S1] While idle, each refusal of the check names its reason
- **GIVEN** a trusted config with the plan runner on, tracking nothing, and `next` naming A's build
- **WHEN** in turn: `next` is `["sh", "scripts/next-row.sh"]` while a build lane runs on
  `change/a`; only an ops lane runs, on `ops/a`; lanes of the build kind run on two candidates,
  from a config whose two build templates have different patterns; a lane of the propose template
  runs on `change/a`; a lane started with no template runs on `change/a`; reading the merged pull
  requests fails while a build lane runs on `change/a`; and a build lane runs on `change/a` whose
  HEAD is merged pull request #8's head
- **THEN** in each case nothing is tracked, `plan-run.json` stays `idle`, and no notice goes out
- **AND** `planRun.waiting` names, in turn: A's kind with no template's suggest command matching
  `next`; that it waits for a lane on `change/a`; A and both branches; the lane, the propose
  template and A's kind; the lane and that it has no template; the failed read; and #8

#### Scenario: [PLANRUN-9-S2] Step 6 and a restart refuse through the same check
- **GIVEN** a tracked merge whose next row is B's build, B's proposal on main, and a lane of the
  propose template still running on `change/b` with commits not yet merged
- **WHEN** the runner reaches step 6
- **THEN** no lane starts, nothing is tracked, and one notice names the lane, its template and B's
  kind
- **AND** when instead a runtime starts on a `starting` state for B's build on `change/b`, and that
  propose lane is what runs there, nothing is tracked, and one notice names the same reason

#### Scenario: [PLANRUN-9-S3] A `next` that gives no template a kind still starts at step 6
- **GIVEN** a trusted config with the plan runner on, `next` `["sh", "scripts/next-row.sh"]`, a
  tracked merge whose next row is B's build, B's proposal on main, and no lane on `change/b`
- **WHEN** the runner reaches step 6
- **THEN** no notice goes out, and a lane of the build template starts on `change/b`
- **AND** when instead a runtime starts on a `starting` state for that start on `change/b`, under
  the same `next`, nothing is tracked, no lane starts, and one notice says a restart cut short the
  start of B on `change/b`, naming what is there now
- **AND** when, in that restart, a lane of the build template runs on `change/b`, nothing is
  tracked, no lane starts, and the one notice names that lane

#### Scenario: [PLANRUN-9-S4] A fix row is tracked and started with its issue
- **GIVEN** a trusted config with the plan runner on, whose fix template, of kind `fix`, has the
  shipped branch pattern `fix/{name}` and a first prompt naming `{issue}`, and `next` naming the
  fix item `12-typo` with issue `#12`
- **WHEN** a lane of the fix template runs on `fix/12-typo`, and the runner's idle read runs
- **THEN** `plan-run.json` tracks `fix/12-typo`, with the item whole, issue `#12` included
- **AND** when instead `plan-run.json` is `starting` for that item on `fix/12-typo`, and that lane
  runs there when the next runtime starts, it is tracked with `started` true and no notice goes out
- **AND** when instead a tracked merge's next row is that item, no lane runs on `fix/12-typo` and
  the fix list names `12-typo`, a lane of the fix template starts there, its first prompt naming
  `#12`
- **AND** when the fix template's pattern is instead `fix/{issue}-{name}`, it is no candidate, and
  `planRun.waiting` names the template and that its branch is not a valid branch name
