**Status:** awaiting approval
**Roadmap row:** PANEL-30
**Risk:** normal

## Why

In the owner's first session on the panel built from source (2026-10-02, issue #67 gap 1), they
removed Standing Tee's `change/add-score-photo` worktree. That change was in flight. They removed
it because the confirmation suggested nothing was. Nothing was lost, but only because of rules the
confirmation didn't explain.

- **The confirmation speaks git.** It says "the worktree <path> (clean: no changes, no untracked
  files; git worktree remove, without --force)", and the button reads "Confirm remove"
  (`RemoveWorktreePlan` in `framework/internal/panel/lanes/removewt.go`). It doesn't say that only
  the folder goes, or that the branch and its commits stay.
- **It doesn't know a change is in flight.** `removeVerdict` keeps a worktree when Close lane's
  worktree rules fail (not listed, the main worktree, outside `worktree_dir`, locked, another lane
  in it, uncommitted or untracked files), when a lane is registered in it, or when a claude session
  runs in it. It never asks whether the branch belongs to an open change or an open pull request.
  The panel already knows both: it reads every open change's proposal (`signals.ReadChanges`), and
  it polls the open pull requests with their branches.
- **"Every commit stays" isn't always true today.** A detached worktree has no branch. If its
  commit is on no branch or tag, `git worktree remove` still removes it, and that commit is then
  reachable from nothing. Close lane's worktree rules (`worktreeVerdict` in `lanes/close.go`) check
  the folder, its lock and its lanes, but not where a detached commit lives, and Remove uses the
  same rules. Reproduced in a scratch repo, and again in review, on 2026-10-02.
- **Ignored files go silently.** "Clean" counts untracked files but not ignored ones (a local
  `.env`, build output), and `git worktree remove` deletes those with the folder. Also reproduced.

## What changes

Before removing a worktree, the owner reads in plain words what goes, what stays, and whether
unfinished work still needs it:

- **What goes and what stays, in plain words.** Removes: "The folder …". Keeps: "The branch … and
  every commit on it" and "Everything on GitHub". When the local branch also goes (it is merged),
  the line says where its commits already are. Ignored files that differ from the project root's
  copy are named, because they go too.
- **Why it is safe, scoped to what is checked**: only a worktree with no uncommitted or untracked
  files can go.
- **A warning for unfinished work.** When the branch is an unmerged open change's, or has an open
  pull request, the confirmation says so first and says how to get the worktree back. The button
  reads "Remove anyway". A merged change that isn't archived yet gets no warning.
- **A commit on no branch is never lost.** Remove and Close lane keep a detached worktree whose
  commit is on no branch or tag, and say how to save it.

## What the existing specs already guarantee

This repo has no living specs yet (`specs/` holds only its README), so nothing here modifies or
contradicts a requirement. PANEL-29 (awaiting approval, revised) adds `panel-new-lane`; this change
doesn't touch it. The behaviour this change alters is documented, not specified, in `docs/panel.md`:
"Remove a worktree" (PANEL-18) and the worktree rules of "Close lane" (PANEL-17). This change adds
the capability spec `panel-remove-worktree` and updates both sections to match.

**One dependency on PANEL-29, for one sentence (D3).** When a template names the worktree's branch,
the way back is "New lane, template …, name …", which works only once PANEL-29 lets a template
start on an existing branch. Without a matching template the way back is a `git worktree add`
command, which works today. The roadmap row's Deps says so. D6 asks whether the data-loss fix (D4)
ships first on its own so that it doesn't wait for PANEL-29.

## Out of scope

- **New lane on an existing branch** (#67 gaps 2–4): PANEL-29.
- **Close lane's own wording.** Its confirmation keeps its git terms; only its worktree rule gains
  the commit check (D4). Not owned: revisit if Close's wording proves as unclear as Remove's.
- **Up next rows and owner-queue items as "in flight"** (D2). Up next lists work not yet started,
  and owner-queue items are prose, so a match would be a guess. Not owned.
- **Refusing to remove an unfinished change's worktree** (D1): the alternative the owner may choose.

## How we'll know

- **Signal:** the next time the owner opens Remove on an unfinished change's worktree, the warning
  shows. They either cancel, or remove it and get the worktree back the way it says, without a git
  error. Removing a merged change's worktree shows no warning. No issue like #67 gap 1 is filed
  again.
- **Check after:** 14 days
