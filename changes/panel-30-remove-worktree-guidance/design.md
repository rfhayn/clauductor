# Design: panel-30-remove-worktree-guidance

## Where the behaviour lives today

- **Server** (`framework/internal/panel/lanes/removewt.go`):
  - `RemoveWorktreePlan` runs `git fetch`, takes the lane lock (`m.mu`), then builds the
    confirmation's lines (`Remove`, `Keep`, `Notes`) and the consent flags (`Worktree`,
    `DeleteBranch`). The worktree line is "the worktree <absolute path> (clean: no changes, no
    untracked files; git worktree remove, without --force)". Under the lock it runs
    `removeVerdict` (up to 10 s for `claude agents`) and `branchVerdict` (up to 20 s for `gh`).
  - `removeVerdict` keeps the worktree when Close's rules (`worktreeVerdict`) fail, when a lane is
    registered in it, or when a claude session runs in it. Nothing else.
  - `RemoveWorktree` checks again, runs `git worktree remove` (no `--force`), then deletes the
    local branch only when `branchVerdict` allows and the confirmation offered it.
- **Shared with Close lane** (`lanes/close.go`):
  - `worktreeVerdict`: listed, not the main worktree, inside `worktree_dir`, not locked, no other
    lane, and `git status --porcelain --untracked-files=all` empty. Ignored files don't count. A
    detached commit's refs aren't checked.
  - `branchVerdict`: a branch goes only when every commit is in the base, or its pull request
    merged at the current tip.
- **What the panel already knows:**
  - **Open changes** (`signals/changes.go`): `ReadChanges` reads every open change's `proposal.md`
    in each worktree, never the archive. The approval alert comes from it. `BranchOfChange`
    matches `change/<id>` (any prefix) to change `<id>`; today it feeds the budget bars and links an
    alert to its lane.
  - **Open pull requests**: the runtime's `prs` source polls `gh pr list` every 60 s
    (`runtime.go`), and the state keeps each one's `headRefName`. The lane manager doesn't see it.
  - **Templates** (`config.RenderTemplate`): renders a template's branch from a name.
- **Page** (`framework/internal/panel/web/static/panel.js`): `renderTree` offers Remove on a
  worktree with no lane. `askRemove` fetches the plan (`dryRun`). `removeWords` renders the
  question, Removes, Keeps and Notes as the server wrote them, with "Confirm remove". `doRemove`
  sends back the consent and shows the result above the lanes.

## The shape of the change

1. **A commit on no branch keeps the worktree** (D4). The rule goes in `worktreeVerdict`, so Remove
   and Close lane both apply it. It asks which refs under `refs/heads`, `refs/remotes` and
   `refs/tags` contain the detached commit (never `refs/stash`). None, or a failed read: the
   worktree stays. Otherwise the note names one ref: a local branch first, then a remote-tracking
   branch, then a tag, each the first by name.
2. **The server writes the plain words.** The plan's lines stay the server's. A folder is named
   relative to the project root when it is inside it, and by its full path otherwise (an absolute
   `worktree_dir`).
3. **Ignored files are named** (D5) under Removes: `git status --porcelain --ignored=matching`,
   leaving out each file identical to the project root's file at the same path (a `.worktreeinclude`
   copy loses nothing). Up to three, then "and N more". It runs with its own timeout, as Close's
   `git status` does; a timeout leaves the line out and adds a note.
4. **The plan says whether unfinished work is on the branch** (D2), as its own part of the plan,
   apart from the lines:
   - **The change:** an open change the branch names (`ReadChanges` over this worktree and the main
     checkout, matched by `BranchOfChange`).
   - **The pull request:** from the panel's polled list (`gh pr list`, every 60 s), handed to the
     lane manager as a function. It is set in `live.go`'s `buildRuntime`, beside `lm.Trusted`, the
     one place that has both the lane manager and the hub. No new `gh` call is made, so no network
     wait is added under the lock. The new local reads (the ignored-files status, the change
     files, the templates) do run under it, and the status has its own timeout. A failed or
     pending poll is a note, and so is a list older than two intervals (`Ticks.PRs`, which
     `buildRuntime` passes in with the function): once PANEL-25 stops polling while no page is in
     view, an old list must not read as fresh. `gh pr list` returns at most 30 open pull
     requests, so in a repository with more, the oldest can be missed.
   - **Not when the branch is merged.** When `branchVerdict` judges the branch merged (the plan
     offers to delete it), there is no warning: the work is done, even if `changes/<id>/` still
     waits for archive.
   - It is a warning, never a guard (D1): `RemoveWorktree` doesn't check it again.
5. **The way back** (D3). The plan renders each template with the change's id, or else the
   branch's last segment, as the name, and keeps those whose branch equals the worktree's (a
   template that needs an issue is skipped). With a match, the way back names New lane, the
   template and the name. Without one, or while panel.json is untrusted (templates are off then),
   it is the `git worktree add` command.
6. **The page's choices are one pure function** (`remove-plan.js`, tested in node like
   `term-links.js`): from a plan it decides the order (warning first), the safety line, the
   button's label, and the sentence the result repeats. panel.js only renders it.
7. **Docs**: `docs/panel.md`'s "Remove a worktree" and Close lane's worktree rules, and the in-page
   Help's line on Stop and Close.

## The words

Placeholders are in angle brackets. `<folder>` is as in shape 2; `<base>` is the configured base
(`origin/main`).

**The question:** `Remove the worktree <folder>?`, or when nothing can go, `Nothing can be removed
from <folder>.` (as today).

**Unfinished work** (a warning, shown first; only when the worktree can go and its branch isn't
merged):

- a change and a pull request: `Change <id> isn't finished: its proposal is in <changes dir>/<id>,
  and pull request #<n> is open.`
- a change only: `Change <id> isn't finished: its proposal is in <changes dir>/<id>.`
- a pull request only: `Pull request #<n> is open from this branch.`
- for a change, next: `Its next lane needs a worktree on this branch.`
- then the way back. For a change it starts `To work on it again:`, for a pull request only `To get
  the worktree back:`, followed by one of:
  - a template matches: `New lane, template "<title>", name <name>. It starts on this branch in a
    new worktree.` (several match: `template "<A>" or "<B>"`)
  - none matches: `run this in the project root: git worktree add <folder> <branch>`

**Removes:**

- `The folder <folder>. It has no uncommitted or untracked files.`
- with ignored files that differ from the root's: `Files git ignores, which go with the folder:
  <a>, <b>, <c> and <N> more.`
- a merged branch: `The local branch <branch>. Every commit on it is already in <base>.` or `The
  local branch <branch>. Its pull request #<n> was merged.`

**Keeps:**

- a branch that stays: `The branch <branch> and every commit on it: <why it stays>.` (`<why>` is
  `branchVerdict`'s reason, such as "it is not merged into origin/main and has no merged pull
  request".)
- always: `Everything on GitHub: the panel never pushes or deletes there.`
- a worktree that stays: `The worktree <folder>: <why>.` (as today)

**Notes:**

- detached, its commit on a ref: `No branch: the worktree is detached at <sha>, a commit that is
  also on <ref>, so nothing is lost.`
- the pull request list unread: `Couldn't check for an open pull request (<why>). The open changes
  were checked.`
- the pull request list old: `The open pull requests were last read <age> ago.`
- the ignored files unread: `Couldn't list the files git ignores in it (<why>).`
- the fetch failed: as today.

**Under the lists, when it can go** (the only safety line): `Only a worktree with no uncommitted or
untracked files can go, and the panel checks again before it removes anything.`

**Buttons:** `Remove worktree`, or `Remove anyway` when there is a warning; `Cancel`.

**The result** above the lanes: `Removed worktree <folder>`, Removed `The folder <folder>`, Kept
`The branch <branch> and every commit on it`. After a warning, it repeats the way back.

**A detached commit on no branch** (Keeps, in Remove and in Close lane): `The worktree <folder>: its
commit <sha> is on no branch or tag, so removing the folder would lose it. To keep it, put it on a
branch first: git branch <name> <sha>.` The command is shown because no panel control puts a commit
on a branch, and the name is the person's to choose.

**The refs can't be read** (Keeps, in Remove and in Close lane): `The worktree <folder>: the panel
couldn't tell whether its commit <sha> is on a branch (<why>), so it stays.`

## Refusals

| Situation | What happens instead |
|---|---|
| The branch is an unmerged open change's, or has an open pull request | The warning shows first, and the button reads "Remove anyway". Removal is still offered (D1). |
| The branch is merged, and its change isn't archived yet | No warning: the branch goes as merged, and the work is done. |
| A change's branch with no commits of its own (its build not started) | Counts as merged, as `branchVerdict` judges it: no warning, and the branch goes. New lane makes it again. |
| The polled pull request list failed or isn't read yet | A note says so. The open-change check still applies, and removal is still offered. |
| The polled list is older than two intervals | A note gives its age. It is still used. |
| A pull request opened since the last poll (under 60 s), or beyond `gh pr list`'s 30 | Not seen: no warning for it. The open-change check still applies. |
| panel.json is untrusted | The way back is the command: templates are off. |
| A detached worktree whose commit is on no branch, remote-tracking branch or tag | The worktree stays, in Remove and in Close lane, with the reason and the `git branch` command (D4). |
| Reading which refs hold that commit fails | The worktree stays, with its own reason: a commit that might be lost isn't removed. |
| Unfinished work appears between the plan and Confirm | Removed as confirmed: the warning is advice, not a guard (D1). |
| Every refusal Remove has today (dirty, locked, a lane, a session, the main checkout) | Unchanged, under Keeps. |

## Decisions (the owner decided all as recommended, 2026-10-02, including D6: the data-loss fix ships first as #77)

**D1. Unfinished work on the branch: warn, or refuse.**
- **Recommended:** warn first, and relabel the button "Remove anyway". Never refuse.
- **Alternative:** refuse while the change is unfinished. Removal only by hand, or by Close lane
  from the change's own lane.
- **Why:** removing loses nothing that is checked: only a worktree with no uncommitted or untracked
  files goes, and an unmerged branch stays. A refusal would send the owner to a terminal, where git
  checks less than the panel does. Removing a stale worktree and starting fresh is a fair choice.

**D2. What counts as unfinished.**
- **Recommended:** an open change whose id the branch names, or an open pull request from the
  branch, and only while the branch isn't merged.
- **Alternative:** also Up next rows and owner-queue items that mention the name.
- **Why:** both facts are structural, and the panel already has them. Up next lists work not yet
  started, which rarely has a branch. Owner-queue items are prose, so a match is a guess (PANEL-29
  D3 declined the same). A merged branch is finished even before archive, which is when the owner
  is most likely to remove its worktree. The owner's case would have been caught: the panel already
  raised `add-score-photo`'s approval alert from its change directory.

**D3. How the warning says to get the worktree back.**
- **Recommended:** when a template's branch is this branch, name New lane, that template and the
  name. Otherwise give the `git worktree add` command, to run in the project root. The template
  sentence is true only once PANEL-29 lets a template start on an existing branch, so PANEL-30's
  Deps names PANEL-29 for that sentence, and group 4's test enforces it: it starts the lane the
  sentence names, with PANEL-29's "branch" mode, and can't pass until PANEL-29 is on main.
- **Alternative:** the command always. PANEL-30 then doesn't depend on PANEL-29.
- **Why:** a command in a confirmation is the git-terms wording the owner asked to replace, so the
  panel's own route comes first wherever it works. PANEL-29 offers "Its existing branch, in a new
  worktree" only after a template and a name, and refuses it without a template. So the sentence
  names both, and falls back to the command when no template fits.

**D4. A detached worktree whose commit is on no branch.**
- **Recommended:** keep it, in the rule Remove and Close lane share (`worktreeVerdict`), naming the
  commit and the `git branch` command that saves it.
- **Alternatives:**
  - warn only;
  - Remove only, leaving Close lane as it is.
- **Why:** "every commit stays" must be true whenever the confirmation says it. Today a clean
  detached worktree with a commit of its own is removed, and the commit is then on nothing; git
  deletes it at a later garbage collection. One rule in the shared check closes it for both
  controls. Leaving Close out would leave the same gap one menu away.

**D5. Ignored files.**
- **Recommended:** name up to three under Removes, then "and N more", leaving out files identical
  to the project root's copy.
- **Alternative:** one generic sentence: "Files git ignores in it go too."
- **Why:** an ignored `.env` or local config is the one thing in a "clean" worktree someone might
  want, and its name is what makes them stop. A `.worktreeinclude` copy that matches the root loses
  nothing, so naming it would only teach people to skim the line.

**D6. Whether the data-loss fix waits for PANEL-29.**
- **Recommended:** split D4 out. It ships now as the fix lane for issue #77 ("Remove worktree and
  Close lane can strand a detached commit"), on branch `fix/77-detached-commit`, with commits
  prefixed `PANEL-30:` (the row whose finding it is). It is the fast path: one sentence, "Remove and
  Close lane keep a detached worktree whose commit is on no branch or tag, or can't be checked".
  It writes the rule, its two Keeps sentences, its tests and its docs. PANEL-30 keeps everything
  else, and then:
  - tasks 1.1 and 1.2 only add the REMOVEWT-3 IDs to #77's tests (and add a test for any scenario
    they don't reach);
  - task 6.1 doesn't rewrite the detached-commit docs #77 wrote, and only checks they match.

  D5 stays in PANEL-30 and waits with it: it changes what the confirmation says, not what is
  removed, and its line belongs with this change's words.
- **Alternative:** keep D4 in PANEL-30 as its first group. The whole change, the data-loss fix
  included, then merges only after PANEL-29.
- **Why:** a commit can be lost today, and the fix is one rule with no dependency. It shouldn't wait
  on a dialog redesign.
