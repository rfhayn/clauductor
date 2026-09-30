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

These buttons sit under the selected lane's terminal. To act on a lane without opening it, use
the **⋯** at the end of its row in **Lanes**, or beside it in **Worktrees**: the same actions,
and each one still asks in the page before anything happens.

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

## Queues and the gate

If two lanes' gate scripts would collide (one port, one test database), a queue runs them one
at a time: the script takes the queue with `clauductor lock-run`, and the others wait their turn.
The **Gate** figure in the header shows who holds it and who waits; a lane's **Gate** tab has
**Cancel wait** and **Run in** that lane. See
[Queue and the gate lock protocol](panel.md#queue-and-the-gate-lock-protocol).

## Add a second repository

In the other repository's checkout:

```sh
clauductor panel init    # writes .clauductor/panel.json; review it
clauductor panel trust   # allow its commands and templates
clauductor panel add     # register it with the panel
```

Then restart the panel (`add` prints the command). The project's name at the top of the page
switches between projects. See [Projects](panel.md#projects).

If the repository runs Clauductor's operating model (it has `.claude/owner-queue.sh` or
`.claude/roadmap-queue.sh`), `init` also adds its cards, lane templates and gate, and prints
each. Otherwise it writes none, and says they come with `clauductor install` or can be added by
hand. See [`panel init`](panel.md#panel-init).

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
4. **Templates, cards or Run do nothing.** The config is not trusted as it is now. See
   [Cards, RUN and templates do nothing](panel.md#cards-run-and-templates-do-nothing).
5. **No context % or quota.** The panel needs a copy of your status line. See
   [The page shows no context % or quota](panel.md#the-page-shows-no-context--or-quota).
6. **The cards show old data.** They run in the project's main checkout. When its branch is
   behind its upstream, a line above them says "main is N commits behind origin/main (as of last
   fetch) — cards may be stale": pull there, then **Refresh**. See
   [When the cards may be stale](panel.md#when-the-cards-may-be-stale).
