# Using the Clauductor panel

A short how-to for the panel's page. For everything else (every setting, every rule, the security
model) see [panel.md](panel.md). On the page, **?** in the header (or the `?` key) shows the
shortcuts and links back here.

## What a lane is

A lane is one interactive `claude` in its own tmux session, usually in its own git worktree and
branch. tmux runs it, not the page, so a lane keeps running when you close the tab, restart the
panel or open it in Terminal.app. Each lane has a tab, a row in the rail and a terminal.

## Start a lane

1. Click **New lane** (or **+** beside the tabs).
2. Pick a **template** if one fits. It chooses the lane type, names the branch, and types a first
   prompt once claude is ready. Without one, pick a **lane type** and where it runs: a new branch
   and worktree, an existing worktree, or the project root.
3. Give it a name (lower case, digits and dashes) and click **Start lane**.

To start one in a worktree that has no lane, click **New lane here** under it in **Worktrees**:
the Start dialog opens on that worktree, and the lane is a new claude session there.

In a new directory claude first asks whether to trust it, and the default is **No, exit**. Press
↓, then Enter, in the lane's terminal. See [Lane templates](panel.md#lane-templates).

## Type into the terminal, and leave it

The page never puts you inside a terminal by itself. Press **Enter** on the terminal, or click it,
to type there. Inside, every key goes to claude, including Tab and Escape. Press **Ctrl+]** to
leave; you land back on the lane's tab.

## Select, copy and open links

- Drag to select text; **⌘C** copies it. The selection stays until you copy or start another.
- **⌘-click** a link (Ctrl-click off a Mac) to open it in a new tab. A link whose text is not
  its address asks first.
- The mouse wheel scrolls the lane's history. Scroll back to the bottom, or type, to return.

## Give claude an image

Drop an image file on the terminal, or paste one with **⌘V**. Its path is typed at the cursor
with no Enter, so you finish the prompt around it. PNG, JPEG, GIF and WebP, up to 20 MB.

## Interrupt, restart, stop or close

- **Interrupt (Esc)** stops claude's current turn. The lane keeps running.
- **Restart** ends claude and starts the same conversation again in the same place.
- **Stop lane** ends claude and its tmux session. The worktree and branch stay.
- **Close lane** stops the lane, then removes its worktree and branch **only if nothing would be
  lost**: the worktree must have no changes and no untracked files, and the branch must be merged
  (or its pull request merged). The confirmation lists what goes and what stays before anything
  happens. See [Close lane](panel.md#close-lane).

To have the panel close a lane by itself once its pull request merges, set
`"lanes_auto_close": "on_merge"` in `.clauductor/panel.json` (config version 5). It closes only
what Close lane would close without asking twice (claude idle or gone, the worktree clean, the
branch merged at its tip); otherwise **Needs you** says "PR merged: close lane?" and why. See
[Close a lane when its PR merges](panel.md#close-a-lane-when-its-pr-merges).

These buttons sit under the selected lane's terminal. To act on a lane without opening it, use
the **⋯** at the end of its row in **Lanes**, or beside it in **Worktrees**: the same actions,
and each one still asks in the page before anything happens.

## Carry on after the usage limit

When a lane stops at the usage limit, the panel can continue it for you once the 5-hour window
resets: set `"quota_auto_resume": true` (and, if you like, `"quota_resume_line": "continue"`). It
types the line once per stop, only while claude is idle and not asking you anything, and notes
it in the lane's **Activity**. See [Resume after the 5-hour reset](panel.md#resume-after-the-5-hour-reset).

## Is it ready to merge?

Select the lane, then **Checks** in its side panel. One line says **Ready to merge**, or **Not
ready** with every reason: no open pull request, a failing or pending check, a review required,
an unresolved review thread, an unticked task in the change's `tasks.md`, or no gate receipt for
the branch's latest commit. The lines under it show each check. The panel only reports; merge as
you always do. See [Merge readiness](panel.md#merge-readiness).

## Prepare each new worktree

A new lane's worktree is a fresh checkout. Three settings fill it in (config version 5):

- A `.worktreeinclude` file in the project root lists gitignored files to copy into every new
  worktree, such as `.env`, in `.gitignore` syntax.
- `"worktree_setup": { "command": ["make", "setup"] }` runs there before claude starts, and
  `"worktree_teardown": { "command": ["make", "down"] }` before **Close lane** removes it.
- `"ports": { "base": 4400, "per_lane": 10 }` gives each lane a port of its own, shown as
  **Port** in its header and set as `CLAUDUCTOR_PORT` for the lane, its setup and its teardown.

Run `clauductor panel trust` after adding them. See
[Worktree setup, teardown and ports](panel.md#worktree-setup-teardown-and-ports).

## Clean up a worktree with no lane

A session that ended can leave its worktree behind. In **Worktrees**, a worktree with no lane has
**Remove**. It checks first and lists what it would remove and what it would keep, and why: it
removes only a clean worktree (no changes, no untracked files) that no lane and no claude session
uses, and its branch only if it is merged. A detached worktree has no branch to remove. Nothing
happens until you click **Confirm remove**. See [Remove a worktree](panel.md#remove-a-worktree).

## After a crash or a reboot

A reboot ends every tmux session, but the panel remembers each lane. When you open the page, a
**Restore** bar lists the lanes that lost their session: **Restore all** resumes each one's own
conversation. A single lost lane shows **Resume**, **Forget** (drop it from the list) and
**Close lane** instead of a terminal. See [Restore after a reboot](panel.md#restore-after-a-reboot).

## Cards

A card is a command the panel runs in the project's main checkout and shows on the page: where
the project stands, such as a work queue or a to-do list. Each output line is a row. The panel
runs it again when a file you name changes (`watch:<path>`), or every so many seconds
(`interval:<seconds>`). With `"pin": true` it shows beside the lanes; every card also shows in
**Activity**. A project with no card says "No cards yet" there instead.

Add cards to `.clauductor/panel.json`, then run `clauductor panel trust`: the panel runs no
command from the file until you trust it. A minimal one:

```json
"cards": [
  { "id": "todo", "title": "To do", "command": ["cat", "docs/todo.md"],
    "refresh": "watch:docs/todo.md", "pin": true }
]
```

A repository with Clauductor's operating model gets its cards from `clauductor panel init`. See
[Card output](panel.md#card-output) and [Pinned cards](panel.md#pinned-cards).

## Metrics

**Metrics**, next to **Activity**, shows how the work flows and what it costs. Pick a tab
(**Flow**, **Cost**, **Quality**, **Outcomes**; ←/→ move between them), a range (**7d**, **30d**,
**90d**) and a scope (**This project** or **All projects**). The choices are kept per browser.

Every figure says where it came from:

- **built in**: the panel's own, for any repository. Merge frequency and pull-request cycle time
  come from `gh` (read every 10 minutes while the page is open); spend comes from your status
  line's cost, kept a day at a time from the first day the panel ran PANEL-19.
- **project**: the project's metrics command. Clauductor's operating model has one
  (`.claude/metrics.sh`); for another project, add a command that prints the metrics JSON:

  ```json
  "version": 4,
  "metrics": { "command": ["sh", ".claude/metrics.sh"], "refresh": "interval:900" }
  ```

  then run `clauductor panel trust`. Until you trust it, the view says so and shows only the
  built-in figures.

A figure nobody has shows **—** and says why, such as "Only a project's metrics command reports
this" or "No pull request was merged in the last 7d". A command that prints something the panel
cannot read shows its error at the top, with the part that is wrong. See
[Metrics](panel.md#metrics) for the JSON and every figure.

Three things show under **Needs you** without opening the view: a proposal that has waited more
than a day for its **Approved:** line, a change that has spent more than its proposal's
**Budget:** (the lane's header also shows a **Budget** bar next to **Cost**), and a lane with no
commit for three days. Change the times with `alerts.approval_wait_hours` and `alerts.stale_days`
(0 turns one off). See [Needs you from the metrics](panel.md#needs-you-from-the-metrics).

To spend less as the 5-hour quota runs low, set `"quota_economy": { "five_hour_pct": 85 }`. At
that point an **Economy** field appears by the quota, naming the roles that move to a cheaper
model (from `.claude/model-roles.json`), and Clauductor's `build-change` picks it up. It turns off
once the quota is 3 points lower. See [Economy mode](panel.md#economy-mode).

## Queues and the gate

If two lanes' gate scripts would collide (one port, one test database), a queue runs them one
at a time: the script takes the queue with `clauductor lock-run`, and the others wait their turn.
The **Gate** figure in the header shows who holds it and who waits; a lane's **Gate** tab has
**Cancel wait** and **Run in** that lane. See
[Queue and the gate lock protocol](panel.md#queue-and-the-gate-lock-protocol).

## Add another repository

Open the project menu (the project's name at the top left, in its box) and pick **+ Add a
project…** at its foot. Type the repository's path (`~/` is your home); the panel checks it as you
type and shows its root and git directory, or why it cannot be added (a linked worktree, a path
already registered, not a git repository). If the repository has no `.clauductor/panel.json` yet,
the dialog shows what `clauductor panel init` would write, with the reason for each value;
**Create this config** writes it. Then it shows exactly what the config runs (its cards, queue
commands and templates): **Trust and add** trusts those bytes and adds it, **Add without
trusting** adds it with its commands off until you trust it. Either way it is served at once, no
restart, and the page switches to it.

Each project's row in the menu has a **⋯**: **Trust config…** (while it is untrusted, or after its
config changed) and **Remove from panel…**, which only unregisters it (nothing on disk is deleted,
and adding it again brings its lanes back) and is refused while it has lanes.

The commands still work, and a running panel takes them within a second:

```sh
clauductor panel init    # writes .clauductor/panel.json; review it
clauductor panel trust   # allow its commands and templates
clauductor panel add     # register it: the running panel serves it now
```

See [Projects](panel.md#projects).

If the repository runs Clauductor's operating model (it has `.claude/owner-queue.sh` or
`.claude/roadmap-queue.sh`), `init` also adds its cards, lane templates and gate, and prints
each. Otherwise it writes none, and says they come with `clauductor install` or can be added by
hand. See [`panel init`](panel.md#panel-init).

## Drive a lane from your phone

Claude Code's Remote Control lets you continue a session from claude.ai/code or the Claude app.
`clauductor panel install` asks once where it should be on (only if you have not set
`remoteControlAtStartup` yourself):

- **Every Claude session on this Mac**: it sets `remoteControlAtStartup` in
  `~/.claude/settings.json`, and prints how to undo it.
- **Only the panel's lanes**: each lane starts with `claude --remote-control`. A lane started
  before that connects from its **⋯** menu, **Remote control**, which asks first and types
  `/remote-control` only while claude is idle.
- **Not now.**

To choose without the question, or change your mind: `clauductor panel install
--remote-control=all`, `lanes` or `off`. `clauductor panel list` and the lane's header show which.
Anyone signed in to your account on another device can then drive that session and approve its
permission prompts, so keep that account's devices locked. The first time, claude asks you to
confirm Remote Control in the terminal. See [Remote control](panel.md#remote-control).

## Where things are kept

- The project's settings: `.clauductor/panel.json` in the repository.
- The panel's own state: `~/.clauductor/panel/`. It holds the registered projects
  (`projects.json`), each project's lane list (`<project hash>/lanes.json`), dropped images
  (`<project hash>/uploads/`, removed after a day), and the logs (`logs/`).
- The conversations stay where Claude Code keeps them; the panel never deletes one.

## When something goes wrong

1. **The page says Disconnected, or will not load.** The lanes are fine; the panel is down. See
   [The panel is down](panel.md#the-panel-is-down).
2. **A lane dies as soon as it starts.** Usually the trust dialog answered No. See
   [A lane dies at start](panel.md#a-lane-dies-at-start).
3. **New lane is greyed out.** Hover it for the reason: an API key in the environment, or the
   quota guard. See [Subscription only](panel.md#subscription-only).
4. **Templates, cards or Run do nothing.** The config is not trusted as it is now: **Trust
   config…** in its banner or its project row's **⋯** shows what it runs and trusts it. See
   [Cards, RUN and templates do nothing](panel.md#cards-run-and-templates-do-nothing).
5. **No context % or quota.** The panel needs a copy of your status line. See
   [The page shows no context % or quota](panel.md#the-page-shows-no-context--or-quota).
6. **The cards show old data.** They run in the project's main checkout. When its branch is
   behind its upstream, a line above them says "main is N commits behind origin/main (as of last
   fetch) — cards may be stale": pull there, then **Refresh**. See
   [When the cards may be stale](panel.md#when-the-cards-may-be-stale).
