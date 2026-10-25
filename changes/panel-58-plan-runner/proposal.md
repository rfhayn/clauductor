**Status:** awaiting approval
**Roadmap row:** PANEL-58
**Risk:** high

## Why
Today only the owner's click starts a panel lane.
- Up next only suggests: each template's suggest command refreshes it, every 120 s or 300 s.
- The **Start lane** route is the one caller of `StartLane`.
- So each row of a session's plan waits for the owner when the row before it merges, even when it
  needs nothing from them.

The owner wants the panel to run the plan by itself (2026-10-24), stopping only where the owner is
needed. S13 is meant to be the first session the panel starts itself.

## What changes
A project can turn on a **plan runner** in `.clauductor/panel.json`. While it is on and the config
is trusted:
- **It tracks one lane.** That is the lane on the branch of the plan's next row, whether the runner
  started it or the owner did (D6).
- **It acts when that lane's PR merges.** The runner:
  1. checks the PR's `Review:` line (D8);
  2. fetches the base ref and confirms the merge is on it;
  3. reads the plan's next row again;
  4. starts it from its template through the same `StartLane` path, and the same trust and 5-hour
     quota guard, that **Start lane** uses.
  It adds no new endpoint.
- **It runs one lane at a time** (D5). The run ends when the session's close merges (D6).
- **It stops and sends one push saying why** when the next start needs the owner or is unsafe:
  - a decision in the plan names the next row, or the next row is a change with no proposal yet
    (D3);
  - the merged PR was a row's proposal, so its build is next and starts with the owner's click;
  - the merged PR's review was capped or did not converge (D8);
  - the account's 7-day usage reaches 95%, or cannot be read (D4);
  - the plan has no row left;
  - the start is refused.
  It also pushes once for a lane of the run that stops short without a stall alert (D7). A stalled
  lane's push stays the stall alert's.
- **The switch** (D1). `plan_runner: {on, next, seven_day_pct}` is a config version 8 key.
  - It is off by default, and shows in **Trust config…**'s run list.
  - It is a load error while `alerts.notify` is false, so the runner never stops in silence.
  - A merged config with it on is never trusted on arrival, whether it turns the runner on or
    changes anything while it is on: the owner's click turns it on, and approves what it starts
    from (D9).
- **The plan's next row** (D2). A new `panel-suggest.sh next` kind prints it, in the plan's order,
  from the rules the suggest kinds already use.
- **The template ships it off.** This repository turns it on in a PR of its own (D10), the
  PANEL-59 row this PR adds. That PR merges once this change's binary serves the repository, so
  its arrival waits for the owner's **Trust config…**. On this PR's own merge, the panel that
  rebuilds itself is still the older binary, and it would trust the runner with no click.
- **Within the row's scope column** (`framework/internal/panel`, `.clauductor/panel.json`,
  `template/.clauductor/panel.json`, `docs/panel.md`, `docs/panel.schema.json`), this change leaves
  `.clauductor/panel.json` to PANEL-59.
- **Beyond the row's scope column:**
  - `docs/roadmap.md`, the PANEL-59 row (in this proposal PR);
  - `template/.claude/panel-suggest.sh` and its check `template/.claude/checks/panel-suggest.sh`
    (D2);
  - their generated copies in `plugin/`;
  - this repository's own `.claude/panel-suggest.sh` and `.claude/checks/panel-suggest.sh`
    (synced by `clauductor update`);
  - `framework/internal/cmd/panel.go`, the help text of `config-check`, which prints the runner
    too (D9).

## What the existing specs already guarantee
- **STALL-1** defines a lane that is working, has moved, or has open work. D7's idle lane is read in
  those terms, and STALL-1 is unchanged.
- **STALL-3** lets the push carry only the stall notice and catch-up's notices. This change
  **modifies** it to carry the runner's stop notices as well, each once, with the same command,
  file, timeout and title rules (STALL-3-S13).
- **CATCHUP-3** trusts a changed `panel.json` that the owner's merge brought, except for the reasons
  it lists. This change **modifies** it to add one more reason: the config turns the plan runner on,
  or changes it while it is on (CATCHUP-3-S11). Turning the runner off is still trusted.
- **CATCHUP-4** restarts the panel after a merge that changes its source. That is why the run's
  state is a file, and why the runner waits for catch-up's check of the merge to end before a start
  (PLANRUN-2-S8). CATCHUP-4 is unchanged.
- **ATTACH-2** (attaching, loading and polling write nothing in the repository) is unchanged:
  - the runner's reads write nothing;
  - its fetch and its starts are actions the owner turned on, as **Start lane** is.
- **ATTACH-3** holds `next` in an attached project's sidecar to its rule with no change, because
  that rule judges every command the config's run list holds (D1).
- **NEWLANE-1** and **NEWLANE-2** (a template runs where its branch already is; an existing branch is
  never a raw git error) hold for the runner's starts. It chooses the mode as the page does and
  goes through the same server checks.

This change adds a new capability, `panel-plan-runner` (PLANRUN-1 to PLANRUN-9).

## Out of scope
- **Running on into the next session** (D6). A refusal: each session's plan entry is the owner's
  to read before it runs.
- **The plan's lanes side by side** (D5). A refusal while the plan cannot say which rows are
  independent.
- **Approving a proposal or taking a decision.** The runner stops at both (D3), and this is a
  refusal.
- **Removing a decision once it is taken from the plan's *The owner decides* cell.** OPS-128 owns
  it (draft PR #256): its cells keep only what stays the owner's.
- **Closing the run's merged lanes.** Auto-close on merge (PANEL-20) and `clauductor lane close`
  (PANEL-57, PR #250) own it.
- **A dollar ceiling** (D4). The owner chose the 7-day usage (2026-10-25), and this is a refusal.
- **Making "no mediums past the cap" a rule for every change** (D8). It binds this change's build,
  by the owner's word. A general rule would be the owner's decision and its own row.

## How we'll know
- **Signal:** in the first session the runner is on (S13 is planned), every planned row after the
  session's first has one of these within 10 minutes of the row before it merging:
  - its lane started with no click from the owner, and a `plan runner started <lane>` line in the
    panel's log;
  - one push naming why it did not start.

  Each of these is a miss and gets a fix row:
  - a merge followed by neither;
  - two pushes for one stop;
  - a lane started past one of the stops: a decision row, a change with no proposal, 95% or more of
    the 7-day usage, an unreadable usage, a capped or NOT converged review.

  If no session runs with it on in the 14 days, the signal is "not exercised" and the check is
  re-queued for 14 days more.
- **Check after:** 14 days
