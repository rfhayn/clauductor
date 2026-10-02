**Status:** awaiting approval
**Roadmap row:** PANEL-30
**Risk:** normal

## Why

In the owner's first session on the panel built from source (2026-10-02, issue #67 gap 1), they
removed Standing Tee's `change/add-score-photo` worktree. That change was in flight. They removed
it because the confirmation suggested nothing was. Nothing was lost, but only because of rules the
confirmation didn't explain.

- **The confirmation speaks git.** It says "the worktree <path> (clean: no changes, no untracked
  files; git worktree remove, without --force)" (`RemoveWorktreePlan` in
  `framework/internal/panel/lanes/removewt.go`). It doesn't say that only the folder goes, or that
  the branch and its commits stay.
- **It doesn't know a change is in flight.** `removeVerdict` keeps a worktree only when a lane is
  registered in it or a claude session runs in it. It never asks whether the branch belongs to an
  open change or an open pull request. The panel already knows both: it reads every open change's
  proposal for its approval alert (`signals.ReadChanges`), and matches a branch to a change
  (`signals.BranchOfChange`).
- **"Every commit stays" isn't always true today.** A detached worktree has no branch. If its
  commit is on no branch or tag, `git worktree remove` still removes it, and that commit is then
  reachable from nothing. Neither Remove nor Close lane checks for this (`worktreeVerdict` in
  `lanes/close.go` checks only that the folder is clean). Checked in a scratch repo on 2026-10-02.
- **Ignored files go silently.** "Clean" counts untracked files but not ignored ones (a local
  `.env`, build output), and `git worktree remove` deletes those with the folder.

## What changes

Before removing a worktree, the owner reads in plain words what goes, what stays, and whether a
change still needs it:

- **What goes and what stays, in plain words.** Removes: "The folder …". Keeps: "The branch … and
  every commit on it" and "Everything on GitHub". When the local branch also goes (it is merged),
  the line says where its commits already are. Files git ignores are named, because they go too.
- **Why it is safe**: one line saying only a clean worktree can go.
- **A warning for a change in flight.** When the branch is an open change's, or has an open pull
  request, the confirmation says so first. It says how to get the worktree back. The button
  reads "Remove anyway".
- **A commit on no branch is never lost.** Remove (and Close lane, which shares the rule) keeps a
  detached worktree whose commit is on no branch or tag, and says why.

## What the existing specs already guarantee

This repo has no living specs yet (`specs/` holds only its README), so nothing here modifies or
contradicts a requirement. PANEL-29 (approved, not yet built) adds `panel-new-lane`; this change
doesn't touch it. The behaviour this change alters is documented, not specified, in `docs/panel.md`:
"Remove a worktree" (PANEL-18) and the worktree rules of "Close lane" (PANEL-17). This change adds
the capability spec `panel-remove-worktree` and updates both sections to match.

**One dependency on PANEL-29 (D3).** The warning's "how to get it back" sentence names New lane's
"Its existing branch, in a new worktree" choice, which PANEL-29 adds. Everything else stands alone.
The roadmap already queues PANEL-29 first.

## Out of scope

- **New lane on an existing branch** (#67 gaps 2–4): PANEL-29.
- **Close lane's own wording.** Its confirmation keeps its git terms; only its worktree rule gains
  the commit check (D4). Not owned: revisit if Close's wording proves as unclear as Remove's.
- **Up next rows and owner-queue items as "in flight"** (D2). Up next lists work not yet started,
  and owner-queue items are prose, so a match would be a guess. Not owned.
- **Refusing to remove an in-flight change's worktree** (D1): the alternative the owner may choose.

## How we'll know

- **Signal:** the next time the owner opens Remove on an in-flight change's worktree, the warning
  shows. They either cancel, or remove it and get the worktree back from New lane without a git
  error. No issue like #67 gap 1 is filed again.
- **Check after:** 14 days
