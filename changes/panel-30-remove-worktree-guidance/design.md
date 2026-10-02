# Design: panel-30-remove-worktree-guidance

## Where the behaviour lives today

- **Server** (`framework/internal/panel/lanes/removewt.go`):
  - `RemoveWorktreePlan` runs `git fetch`, then builds the confirmation's lines: `Remove`, `Keep`
    and `Notes`, plus the consent flags `Worktree` and `DeleteBranch`. The worktree line is "the
    worktree <absolute path> (clean: no changes, no untracked files; git worktree remove, without
    --force)".
  - `removeVerdict` keeps the worktree when Close's rule (`worktreeVerdict`) fails, when a lane is
    registered in it, or when a claude session runs in it. Nothing else.
  - `RemoveWorktree` checks again, runs `git worktree remove` (no `--force`), then deletes the
    local branch only when `branchVerdict` allows and the confirmation offered it.
- **Shared with Close lane** (`lanes/close.go`):
  - `worktreeVerdict`: listed, not the main worktree, inside `worktree_dir`, not locked, no other
    lane, and `git status --porcelain --untracked-files=all` empty. Ignored files don't count.
    A detached HEAD's commit isn't checked.
  - `branchVerdict`: a branch goes only when every commit is in the base, or its pull request
    merged at the current tip.
- **What the panel already knows about changes** (`signals/changes.go`): `ReadChanges` reads every
  open change's `proposal.md` in each worktree, not the archive; `BranchOfChange` matches
  `change/<id>` (any prefix) to change `<id>`. The metrics source uses both for the approval alert.
- **Page** (`framework/internal/panel/web/static/panel.js`): `renderTree` offers Remove on a
  worktree with no lane. `askRemove` fetches the plan (`dryRun`). `removeWords` renders the
  question, Removes, Keeps and Notes verbatim, with "Confirm remove". `doRemove` sends back the
  consent and shows the result above the lanes.

## The shape of the change

1. **The server writes the plain words.** The plan's lines stay the server's, so the page only
   renders them. Folders are named relative to the project root. The words are below; the server
   produces them for every case in the refusals table.
2. **The plan says whether a change is in flight on the branch** (D2). At plan time it reads the
   open changes (the files the approval alert already reads, in this worktree and the main
   checkout) and asks `gh` for an open pull request from the branch. The facts travel as their own
   part of the plan, apart from the lines, so the page can show them first and relabel the button.
   It is a warning, never a guard (D1): `RemoveWorktree` doesn't check it again.
3. **A detached commit on no branch keeps the worktree** (D4). The rule goes in `worktreeVerdict`,
   so Remove and Close lane both apply it.
4. **Ignored files are named** (D5) under Removes, up to three, then "and N more".
5. **The page** shows the in-flight warning above Removes, the safety line under the lists, and
   "Remove anyway" instead of "Remove worktree" when a change is in flight. The result above the
   lanes repeats how to get the worktree back.
6. **Docs**: `docs/panel.md`'s "Remove a worktree" and Close lane's worktree rules, and the in-page
   Help's line on Stop and Close, say what goes, what stays, and the new rule.

## The words

Placeholders are in angle brackets. `<folder>` is the worktree relative to the project root, and
`<base>` is the configured base (`origin/main`).

**The question** (unchanged in meaning):

- `Remove the worktree <folder>?`
- when nothing can go: `Nothing can be removed from <folder>.`

**In flight** (a warning, shown first; only when the worktree can go):

- a change and a pull request: `Change <id> is in flight on this branch: <changes dir>/<id> is
  open, and so is pull request #<n>.`
- a change only: `Change <id> is in flight on this branch: <changes dir>/<id> is open.`
- a pull request only: `Pull request #<n> is open from this branch.`
- then always: `Its next lane needs a worktree on <branch>. If you remove this one, start that lane
  from New lane and choose "Its existing branch, in a new worktree".`
- then, smaller: `Or in a terminal: git worktree add <folder> <branch>`

**Removes:**

- `The folder <folder>. It has no uncommitted or untracked files, so no work in it is lost.`
- with ignored files: `Files git ignores in it, which go with it: <a>, <b>, <c> and <N> more.`
- a merged branch: `The local branch <branch>. Every commit on it is already in <base>.` or
  `The local branch <branch>. Its pull request #<n> was merged.`

**Keeps:**

- a branch that stays: `The branch <branch> and every commit on it: <why it stays>.` (`<why>` is
  `branchVerdict`'s reason, such as "it is not merged into origin/main and has no merged pull
  request".)
- always: `Everything on GitHub: the panel never pushes or deletes there.`
- a worktree that stays: `The worktree <folder>: <why>.` (as today)

**Notes:**

- detached, its commit on a branch: `No branch: the worktree is detached at <sha>, a commit that is
  also on <ref>, so nothing is lost.`
- the pull request check failed: `Couldn't check for an open pull request (<error>). The open
  changes were checked.`
- the fetch failed: as today.

**Under the lists, always when it can go:** `Removing is safe: only a clean worktree can go, and
the panel checks again before it removes anything.`

**Buttons:** `Remove worktree`, or `Remove anyway` when a change is in flight; `Cancel`.

**The result** above the lanes: `Removed worktree <folder>`, with Removed `The folder <folder>` and
Kept `The branch <branch> and every commit on it`. When the plan warned of a change in flight,
the result repeats its "start that lane from New lane" sentence.

**Close lane and Remove, a detached commit on no branch** (Keeps):
`The worktree <folder>: its commit <sha> is on no branch or tag, so removing the folder would lose
it. Put it on a branch first: git branch <name> <sha>.`

## Refusals

| Situation | What happens instead |
|---|---|
| The branch is an open change's, or has an open pull request | The warning shows first, and the button reads "Remove anyway". Removal is still offered (D1). |
| `gh` can't say whether a pull request is open | A note says so. The open-change check still applies, and removal is still offered. |
| A detached worktree whose commit is on no branch or tag | The worktree stays, in Remove and in Close lane, with the reason and the `git branch` command (D4). |
| Reading which refs hold that commit fails | The worktree stays: a commit that might be lost isn't removed. |
| A change appears in flight between the plan and Confirm | Removed as confirmed: the warning is advice, not a guard (D1). |
| Every refusal Remove has today (dirty, locked, a lane, a session, the main checkout) | Unchanged, under Keeps. |

## Decisions (awaiting the owner)

**D1. A change in flight: warn, or refuse.**
- **Recommended:** warn first, and relabel the button "Remove anyway". Never refuse.
- **Alternative:** refuse while the change is in flight. Removal only by hand, or by Close lane
  from the change's own lane.
- **Why:** removing loses nothing (only a clean worktree goes, and an unmerged branch stays). A
  refusal would send the owner to a terminal, where git checks less than the panel does. A stale
  in-flight worktree is also a real case: removing it and starting fresh is a reasonable choice.

**D2. What counts as "in flight".**
- **Recommended:** an open change whose id the branch names (`changes/<id>/`, not archived, read in
  this worktree and the main checkout), or an open pull request from the branch. The plan reads
  both on the server, when it is asked for.
- **Alternative:** also Up next rows (from the templates' suggest commands) and owner-queue items
  that mention the name.
- **Why:** both recommended facts are structural, and the plan can read them itself. Up next lists
  work not yet started, which rarely has a branch. Owner-queue items are prose, so a match is a
  guess (PANEL-29 D3 declined the same match). The owner's case would have been caught: the panel
  already raised `add-score-photo`'s approval alert from its change directory.

**D3. How the warning says to get the worktree back.**
- **Recommended:** name PANEL-29's New lane choice, "Its existing branch, in a new worktree", with
  the `git worktree add` command as a smaller second line. Build this change after PANEL-29 (the
  roadmap already queues it first); the roadmap row's Deps gains PANEL-29.
- **Alternative:** the command alone. PANEL-30 then doesn't depend on PANEL-29.
- **Why:** a command in a confirmation is the git-terms wording the owner asked to replace. Before
  PANEL-29, New lane can't start on an existing branch at all (it fails with a raw git error), so
  the sentence would be false without that dependency.

**D4. A detached worktree whose commit is on no branch.**
- **Recommended:** keep it, in the rule Remove and Close lane share (`worktreeVerdict`), naming the
  commit and the `git branch` command that saves it.
- **Alternatives:**
  - warn only;
  - Remove only, with Close lane left as it is.
- **Why:** "every commit stays" must be true whenever the confirmation says it. Today a clean
  detached worktree with a commit of its own is removed, and the commit is then on nothing. Git
  deletes it at a later garbage collection. One rule in the shared check closes it for both
  controls; leaving Close out would leave the same gap one menu away.

**D5. Ignored files.**
- **Recommended:** name up to three under Removes (`git status --porcelain --ignored`, folders
  collapsed), then "and N more".
- **Alternative:** one generic sentence: "Files git ignores in it go too."
- **Why:** an ignored `.env` or local config is the one thing in a "clean" worktree someone might
  want. Its name is what makes them stop. Collapsed folders keep the read quick, even with
  `node_modules`.
