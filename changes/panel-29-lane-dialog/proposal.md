**Approved:** 2026-10-02 by Rich · design 719d960f9c67
**Roadmap row:** PANEL-29
**Risk:** normal

## Why

The owner's first session on the panel built from source (2026-10-02, issue #67) hit three walls in
the New lane dialog, all on one change (Standing Tee's `add-score-photo`, in flight):

- **A template could not use the change's existing worktree.** Choosing a template disables every
  "Where it runs" choice and forces "New branch and worktree", in the page (`panel.js`, `stUpdate`)
  and on the server (`StartLane`: "a template lane starts on a new branch and worktree"). The only
  way to build an in-flight change in its own worktree was to start an empty lane and type the
  template's prompt by hand.
- **An existing branch failed with a raw git error.** "New branch and worktree" for a branch that
  already exists showed `git worktree add failed: git: exit status 255: Preparing worktree (new
  branch 'change/add-score-photo')`. The server checks that the folder is free but not that the
  branch is.
- **The dialog started a real build without showing what was in its way.** The build template types
  `/build-change` as soon as Claude is idle, so agents, a gate and commits start at once. The panel
  already knew the change had no Approved line (its own alert said so), but the dialog didn't show
  it.

"New lane here" on a change's worktree also clears the template, so the owner picked the template
and the change by hand even when the worktree was on `change/add-score-photo` and that change headed
Up next.

## What changes

Starting a lane on work the panel already knows takes one click and doesn't fail on git:

- **A template runs in the change's existing worktree.** "Where it runs" stays usable with a
  template. When the change's branch already has a worktree, the dialog picks it.
- **An existing branch is handled, not failed on.** When the branch exists without a worktree, the
  dialog offers "Its existing branch, in a new worktree" and picks it. The server refuses a new
  branch that already exists with a plain sentence naming the branch, never a raw git error.
- **"New lane here" on a change's worktree pre-selects that change**: its template, its name and the
  worktree.
- **What's in a build's way is shown before Start**: a build of a change with no Approved line gets
  a warning next to "Start anyway".
- **The empty option says what it is**: "No template: a plain Claude session".

## What the existing specs already guarantee

This repo has no living specs yet (`specs/` holds only its README), so nothing here modifies or
contradicts a requirement. The behaviour this change alters is documented, not specified, in
`docs/panel.md` (New lane, Up next from PANEL-12, "New lane here" from PANEL-18). This change adds
the first panel capability spec, `panel-new-lane`, and updates those `docs/panel.md` sections to
match.

## Out of scope

- **Remove worktree guidance** (#67 gap 1): PANEL-30.
- **The Claude Code version check** (#68): PANEL-31.
- **Dependency warnings.** An Up next row's detail is free text, so there's no reliable "unmet"
  signal (D3). A structured unmet-dependencies field in the suggest schema would be its own row. Not
  owned yet.
- **Matching founder-queue prose to a change** (e.g. "Score photo, group 0…"): the queue is free
  text, so a match would be a guess. The dialog shows only what the panel knows structurally
  (D3). Not owned: revisit if the structured warnings prove too thin.
- **Standing Tee's approval format.** Standing Tee records approval in prose, not the template's
  `**Approved:**` line, so its changes show "no Approved line" until the swap's records work
  (ST-3.x). That's why the warning never blocks (D3).

## How we'll know

- **Signal:** the owner starts a build lane on an in-flight change's existing worktree from "New
  lane here", in one click and with no git error, and the dialog's warnings match what the change
  actually needs.
- **Check after:** 7 days
