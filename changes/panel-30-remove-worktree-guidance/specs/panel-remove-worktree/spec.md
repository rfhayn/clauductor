## Purpose
The panel's Remove on a worktree with no lane says in plain words what goes (the folder) and what stays (the branch and every commit on it), warns when a change is still in flight on the branch, and never removes a commit that is on no branch.

## ADDED Requirements

### Requirement: The confirmation says what goes and what stays
WHEN Remove is asked for on a worktree that can go THE SYSTEM SHALL list the folder under Removes and the branch with every commit on it under Keeps, in plain words, and SHALL say that only a clean worktree can go.

#### Scenario: [REMOVEWT-1-S1] A clean worktree on an unmerged branch
- **GIVEN** a clean worktree `.wt/open` on branch `fix/open`, with a commit not in the base
- **WHEN** Remove is asked for on it
- **THEN** Removes lists "The folder .wt/open"
- **AND** Keeps lists "The branch fix/open and every commit on it" and "Everything on GitHub"
- **AND** the confirmation says "Removing is safe: only a clean worktree can go"

#### Scenario: [REMOVEWT-1-S2] A merged branch says where its commits are
- **GIVEN** a clean worktree on branch `fix/merged`, every commit of which is in the base `main`
- **WHEN** Remove is asked for on it
- **THEN** Removes lists "The local branch fix/merged. Every commit on it is already in main."

#### Scenario: [REMOVEWT-1-S3] Ignored files are named
- **GIVEN** a clean worktree holding the ignored files `.env`, `a.log`, `b.log` and `c.log`
- **WHEN** Remove is asked for on it
- **THEN** Removes names three of them and "1 more" as files git ignores, which go with the folder

#### Scenario: [REMOVEWT-1-S4] The result uses the same words
- **GIVEN** the clean worktree `.wt/open` on the unmerged branch `fix/open`
- **WHEN** it is removed as the confirmation offered
- **THEN** the result's Removed lists "The folder .wt/open"
- **AND** its Kept lists "The branch fix/open and every commit on it"

### Requirement: A change in flight is named before removal
WHEN Remove is asked for on a worktree whose branch is an open change's, or has an open pull request, THE SYSTEM SHALL show a warning naming them and saying how to get the worktree back, and SHALL still offer removal.

#### Scenario: [REMOVEWT-2-S1] An open change's worktree is flagged
- **GIVEN** a clean worktree on branch `change/add-score-photo`
- **AND** an open change `changes/add-score-photo/`
- **WHEN** Remove is asked for on it
- **THEN** the plan warns "Change add-score-photo is in flight on this branch"
- **AND** the warning says to start its lane from New lane with "Its existing branch, in a new worktree"
- **AND** removal is still offered

#### Scenario: [REMOVEWT-2-S2] An open pull request is flagged
- **GIVEN** a clean worktree on branch `fix/typo` with no change directory
- **AND** pull request #41 open from `fix/typo`
- **WHEN** Remove is asked for on it
- **THEN** the plan warns "Pull request #41 is open from this branch"

#### Scenario: [REMOVEWT-2-S3] A worktree with nothing in flight has no warning
- **GIVEN** a clean worktree on a branch with no open change and no open pull request
- **WHEN** Remove is asked for on it
- **THEN** the plan has no in-flight warning

#### Scenario: [REMOVEWT-2-S4] An unreadable pull request list is said, not guessed
- **GIVEN** a clean worktree on branch `change/add-score-photo` with an open change
- **AND** `gh` fails when asked for open pull requests
- **WHEN** Remove is asked for on it
- **THEN** a note says the open pull requests couldn't be checked
- **AND** the change is still named as in flight

#### Scenario: [REMOVEWT-2-S5] The page asks "Remove anyway" for a change in flight
- **GIVEN** a plan that warns of a change in flight
- **WHEN** the page shows the confirmation
- **THEN** the warning is shown above Removes
- **AND** the button reads "Remove anyway"
- **AND** the result after removing repeats how to get the worktree back

### Requirement: A commit on no branch is never removed
WHEN a worktree is detached at a commit that no branch or tag holds THE SYSTEM SHALL keep the worktree, in Remove and in Close lane, and SHALL say how to save the commit.

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
