## Purpose
The panel's Remove on a worktree with no lane says in plain words what goes (the folder) and what stays (the branch and every commit on it), warns when unfinished work is on the branch and says how to get the worktree back, and never removes a commit that is on no branch.

## ADDED Requirements

### Requirement: The confirmation says what goes and what stays
WHEN Remove is asked for on a worktree that can go THE SYSTEM SHALL list the folder under Removes and the branch with every commit on it under Keeps, in plain words, and SHALL say that only a worktree with no uncommitted or untracked files can go.

#### Scenario: [REMOVEWT-1-S1] A clean worktree on an unmerged branch
- **GIVEN** a clean worktree `.wt/open` on branch `fix/open`, with a commit not in the base
- **WHEN** Remove is asked for on it
- **THEN** Removes lists "The folder .wt/open. It has no uncommitted or untracked files."
- **AND** Keeps lists "The branch fix/open and every commit on it" and "Everything on GitHub"
- **AND** the plan says "Only a worktree with no uncommitted or untracked files can go"

#### Scenario: [REMOVEWT-1-S2] A merged branch says where its commits are
- **GIVEN** a clean worktree on branch `fix/merged`, every commit of which is in the base `main`
- **WHEN** Remove is asked for on it
- **THEN** Removes lists "The local branch fix/merged. Every commit on it is already in main."

#### Scenario: [REMOVEWT-1-S3] Ignored files are named, except copies of the root's
- **GIVEN** a clean worktree holding the ignored files `a.log`, `b.log`, `c.log` and `d.log`
- **AND** an ignored `.env` identical to the project root's `.env`
- **WHEN** Remove is asked for on it
- **THEN** Removes names three of the `.log` files and "1 more" as files git ignores, which go with the folder
- **AND** it doesn't name `.env`

#### Scenario: [REMOVEWT-1-S4] The result uses the same words
- **GIVEN** the clean worktree `.wt/open` on the unmerged branch `fix/open`
- **WHEN** it is removed as the confirmation offered
- **THEN** the result's Removed lists "The folder .wt/open"
- **AND** its Kept lists "The branch fix/open and every commit on it"

#### Scenario: [REMOVEWT-1-S5] A worktree outside the project root is named in full
- **GIVEN** a config whose `worktree_dir` is an absolute path outside the project root
- **AND** a clean worktree there
- **WHEN** Remove is asked for on it
- **THEN** Removes names the folder by its full path

### Requirement: Unfinished work is named before removal
WHEN Remove is asked for on a worktree whose unmerged branch is an open change's, or has an open pull request, THE SYSTEM SHALL show a warning naming them and saying how to get the worktree back, and SHALL still offer removal.

#### Scenario: [REMOVEWT-2-S1] An unfinished change's worktree is flagged, with its template
- **GIVEN** a clean worktree on the unmerged branch `change/add-score-photo`
- **AND** `changes/add-score-photo/proposal.md` exists
- **AND** a template "Build an approved change" whose branch pattern is `change/{name}`
- **WHEN** Remove is asked for on it
- **THEN** the plan warns "Change add-score-photo isn't finished: its proposal is in changes/add-score-photo."
- **AND** it says "To work on it again: New lane, template "Build an approved change", name add-score-photo."
- **AND** removal is still offered

#### Scenario: [REMOVEWT-2-S2] An open pull request with no template gives the command
- **GIVEN** a clean worktree `.wt/typo` on the unmerged branch `fix/typo` with no change directory
- **AND** no template whose branch is `fix/typo`
- **AND** the polled pull requests include #41 from `fix/typo`
- **WHEN** Remove is asked for on it
- **THEN** the plan warns "Pull request #41 is open from this branch."
- **AND** it says "To get the worktree back: run this in the project root: git worktree add .wt/typo fix/typo"

#### Scenario: [REMOVEWT-2-S3] A worktree with nothing unfinished has no warning
- **GIVEN** a clean worktree on an unmerged branch with no open change and no open pull request
- **WHEN** Remove is asked for on it
- **THEN** the plan has no warning

#### Scenario: [REMOVEWT-2-S4] An unread pull request list is said, not guessed
- **GIVEN** a clean worktree on the unmerged branch `change/add-score-photo` with an open change
- **AND** the last poll of the pull requests failed
- **WHEN** Remove is asked for on it
- **THEN** a note says the open pull requests couldn't be checked
- **AND** the change is still named as unfinished

#### Scenario: [REMOVEWT-2-S5] The page puts the warning first and asks "Remove anyway"
- **GIVEN** a plan with a warning and a way back
- **WHEN** the page lays out the confirmation
- **THEN** the warning comes before Removes
- **AND** the button reads "Remove anyway"
- **AND** the result after removing repeats the way back

#### Scenario: [REMOVEWT-2-S6] A merged change that isn't archived yet has no warning
- **GIVEN** a clean worktree on branch `change/done`, squash-merged: pull request #12 merged with the branch's tip as its head
- **AND** `changes/done/proposal.md` still exists
- **WHEN** Remove is asked for on it
- **THEN** the plan has no warning
- **AND** it offers to remove the local branch `change/done`

### Requirement: A commit on no branch is never removed
WHEN a worktree is detached at a commit that no branch, remote-tracking branch or tag holds, or when that can't be read, THE SYSTEM SHALL keep the worktree, in Remove and in Close lane, and SHALL say how to save the commit.

#### Scenario: [REMOVEWT-3-S1] Remove keeps a detached commit of its own
- **GIVEN** a clean worktree detached at a commit that is on no branch or tag
- **WHEN** Remove is asked for on it
- **THEN** the worktree is kept, with the reason that its commit is on no branch or tag
- **AND** the reason names `git branch <name> <sha>`

#### Scenario: [REMOVEWT-3-S2] A detached worktree at a commit on a branch can go
- **GIVEN** a clean worktree detached at a commit that `main` holds
- **WHEN** Remove is asked for on it
- **THEN** the worktree is removed
- **AND** a note says the commit is also on main, so nothing is lost

#### Scenario: [REMOVEWT-3-S3] Close lane keeps a detached commit of its own too
- **GIVEN** a lane in a clean worktree detached at a commit that is on no branch or tag
- **WHEN** Close lane is confirmed on it
- **THEN** the lane closes and its worktree is kept, with the same reason

#### Scenario: [REMOVEWT-3-S4] An unreadable ref list keeps the worktree
- **GIVEN** a clean detached worktree
- **AND** reading which refs contain its commit fails
- **WHEN** Remove is asked for on it
- **THEN** the worktree is kept, with the reason "the panel couldn't tell whether its commit <sha> is on a branch"
