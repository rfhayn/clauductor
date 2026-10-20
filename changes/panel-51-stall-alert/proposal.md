**Approved:** 2026-10-20 by Rich · design 9ee4acecfab1
**Roadmap row:** PANEL-51
**Risk:** normal
**Budget:** $80, soft

## Why
Unattended work stopped silently three times, each time with the work done and nobody awake to
go on: on 2026-10-03 for about 5 h, and twice on the night of 2026-10-18, for about 5 h and 3.5 h
(Session 4's journal; the insights *silent lane*, 2026-10-03, and *nobody awake*, 2026-10-18).
- **11:51 PM to 4:48 AM:** the orchestrator asked the owner a question nothing waited on
  (AskUserQuestion), which froze it, while merge-train's request sat unread.
- **4:55 AM to 8:24 AM:** merge-train's pre-gate check run outlived its timeout, was moved to the
  background, and merge-train ended its turn to wait. The checks passed minutes later, and that
  woke no one.

The owner's decision of 2026-10-18, after coming back to a stalled merge three times: a user of
the app must never find work silently stopped. OPS-90 removes the causes it knows of in the
operating model. This change makes any stall that remains visible on the panel, and pushes it,
including a stall in an attached repository whose sessions the panel did not launch. It takes up
the intent of PANEL-40 (cancelled at the rescope of 2026-10-14): a lane's outcome reaches the
owner.

The panel already sees most of it, and tells no one who is away. A session frozen on a question
reads `waiting` / `input needed` in `claude agents` (probed: design E3), so today's waiting alert
raises a macOS banner after 120 s, on a Mac nobody is looking at. A lane whose turn ended with
work open reads `idle`, an `info` alert that never interrupts. And nothing the panel sends
reaches a phone.

**Where it sits:** session 5, first, beside OPS-90, and on today's page before PANEL-46's
redesign (the owner's decision of 2026-10-18, over building it once on the new look). PANEL-46
carries the stalled state into the new look (design D11).

## What changes
- **A lane can be stalled,** three ways: a session of it has waited on a question or a
  permission prompt for the stall time since it asked; or, once the session or lane it belongs
  to has gone the stall time with no movement and is not working, a session of it ended its
  turn on background work that woke no one, or the lane has open work (an open pull request, a
  clean gate receipt for its HEAD with no pull request after it, or, for a lane the panel
  launched, a claude that exited with work open). A question is timed from the ask, and a
  session's background work on its own clock, so a busy or looping sibling cannot hide them. A
  session the owner opens or resumes counts as a start, so it is not stalled at once. The
  stall time defaults to 30 minutes, and `alerts.stall_minutes` changes it per project (D1, D3,
  D5).
  - **Working:** a `claude agents` reading of `busy` (which holds while a background command
    runs: E1), or a background subagent or workflow its turn ended on that has not yet ended
    (a subagent's `SubagentStop` carries the task's id; a workflow ends when a later `Stop` no
    longer lists it: D3 (d)); and for the lane, holding (alive) or
    waiting for the gate's lease.
  - **Only some listed tasks count:** a shell (a background `Monitor` among them), a monitor
    and a teammate are open work, a subagent and a workflow working, and any other type, such
    as Claude Code's own housekeeping, neither (D3 (d)). A teammate is part of its lead's
    session: its hooks move the lead (D9).
  - **Movement:** a hook from the session (a notification that reports the stop is not one), a
    change in its `claude agents` reading, or its start; for the lane, any of its sessions'
    movement, a lease taken or given up, or its pane dying.
  - **A lane with no live session** does not stall, unless the panel launched it and its claude
    exited (D1).
- **It says what it last waited on.** That is the ask, or the in-flight work Claude Code's `Stop`
  hook lists (`background_tasks`, probed on 2.1.302: E1, E2), or "nothing in flight to wake it",
  with the turn's last line. A session that scheduled its own wakeup (`session_crons`) holds off
  its own background-work stall and its lane's open-work stall while a recurring wakeup is
  listed, or until 5 minutes past a one-shot's fire time; it never holds off a sibling's stall,
  nor its own question (D4).
- **It shows on today's page.** The lane's row and its tab read **stalled**, tinted red, and a
  Needs-you row names the lane, how long it has been quiet, the open work and what it waited on
  (D11).
- **It interrupts once.** One `stalled` alert per stretch, through the existing notifier: a
  macOS notification and, when the owner sets one up, a push through a command of their own
  (`~/.clauductor/panel/push.json`, for example ntfy or Pushover) that reaches a phone (D6, D7).
- **The same rule for a session the panel did not launch,** in its own repository or an
  attached one: the panel's hooks and `claude agents` are the machine's, so they cover it (D2).

## What the existing specs already guarantee
- **`specs/panel-attach`:**
  - ATTACH-2 (attaching and serving write nothing under the repository) holds. The new reads
    (the receipt, a merged-PR read: D10) are reads, and the push writes nothing there.
  - ATTACH-3 (a sidecar runs only the repository's argv commands) is untouched. The push command
    is the machine's, in `~/.clauductor/panel/`, never a project's config (D7).
  - The stall rule applies to an attached project's lanes and sessions with that project's own
    `alerts.stall_minutes`, read from its sidecar like every other alert threshold.
- **`specs/panel-claude-calls`:** it covers the panel's own `claude` calls. This change makes
  none. Unchanged.
- **`specs/panel-lane-context`:** the context policy and its alert. Unchanged; a stall is a
  separate alert.
- **`specs/panel-new-lane`:** where a lane starts. Unchanged.
- **`panel-lane-measures` (PANEL-49, approved, not yet built):** it reads transcripts for
  attached projects only. This change reads none (D2), and its added hook fields keep
  `notranscript_test.go`'s rule.

This change adds one capability, `panel-stall` (STALL-1 to STALL-4), and modifies no requirement.

**The row, re-measured:** its code lands in `state` (the rule, the alert, the view), the panel's
runtime (the reads without a page, and the push), `signals` (the `Stop` fields), `config` (one
key, version 7) and the page (a new module, `stall-view.js`, which `panel.js` calls from the
lane's state cell, tab, tree node and Needs you, and nothing else in `panel.js`), with
`docs/panel.md`. Three surfaces of substance (state, the runtime, the page), no MODIFIED
requirement, and the push in a group of its own, so it stays one row. The row names
`framework/internal/panel/lanes`: the design needs nothing from it, because the state already
holds each tmux lane's pane state (D2). Splitting the push into a row of its own is the
alternative the owner can choose (D7).

**What this PR carries for later rows:** one task appended to PANEL-46's `tasks.md` (2.9: a
stalled lane in the new look) and one to PANEL-25's (3.8: the reads a stall rests on keep running
with no page in view), so neither build can merge without them. Neither `design.md` changes, so
both approvals stand (D10, D11). PANEL-46's roadmap row gains PANEL-51 in its Depends, so the
panel does not offer it first.

## Out of scope
- **Removing the causes** (a turn that ends while its command runs; a question nothing waits
  on): OPS-90.
- **A busy session that never ends,** such as a hung foreground command: it reads as working
  (D1). OPS-90's widened hook holds long commands under a timeout.
- **The new look of the stalled state:** PANEL-46, from the contract D11 gives it.
- **A lane whose session has ended,** its pull request still open: it has no live session and
  is not stalled, unless the panel launched it and its claude exited (D1, D3 (c)).
- **Reminders while a lane stays stalled:** not proposed (D6).

## How we'll know
- **Signal:** over the 14 days after the owner's panel runs a build with this change, and with
  `push.json` set up:
  - every lane the owner finds stopped with work open had been marked stalled within the stall
    time plus a minute, and its push arrived (the notifier's sent count in `/api/state`'s
    observations, and the phone);
  - no stall in that time lasts longer than 1 hour before the owner acts, where the three
    stalls before it lasted 3.5 to 5 hours;
  - the owner reports no more than one false stall a day (a lane marked stalled that was
    working).
- **Check after:** 14 days
