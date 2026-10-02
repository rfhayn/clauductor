## Purpose
The panel's New lane dialog starts a Claude lane on the work the panel already knows: the right template, the right place to run, and what is in the way, without failing on git.

## ADDED Requirements

### Requirement: A template runs where its branch already is
WHEN a template's branch already has a worktree THE SYSTEM SHALL start the template's lane in that worktree, and WHEN the branch exists without a worktree THE SYSTEM SHALL start it on that branch in a new worktree.

#### Scenario: [NEWLANE-1-S1] A template starts in its change's existing worktree
- **GIVEN** a worktree on branch `change/add-score-photo`
- **AND** the build template, whose branch pattern is `change/{name}`
- **WHEN** a lane is started with the build template, the name `add-score-photo` and that worktree
- **THEN** the lane runs in that worktree and no branch is created
- **AND** the template's first prompt is typed once Claude is idle

#### Scenario: [NEWLANE-1-S2] A template starts on its existing branch in a new worktree
- **GIVEN** a local branch `change/add-score-photo` with no worktree
- **WHEN** a lane is started with the build template and the name `add-score-photo` on its existing branch
- **THEN** a new worktree is added on that branch, keeping its commits
- **AND** the template's first prompt is typed once Claude is idle

#### Scenario: [NEWLANE-1-S3] A template refuses another branch's worktree
- **GIVEN** a worktree on branch `change/other-change`
- **WHEN** a lane is started with the build template and the name `add-score-photo` in that worktree
- **THEN** the start is refused with a message naming both branches

### Requirement: An existing branch is never a raw git error
WHEN a new branch is requested that already exists locally or on origin THE SYSTEM SHALL refuse it in a plain sentence that names the branch and says where it can run instead.

#### Scenario: [NEWLANE-2-S1] A new branch that already exists is refused plainly
- **GIVEN** a local branch `change/add-score-photo`
- **WHEN** a lane is started on a new branch and worktree named `change/add-score-photo`
- **THEN** the start is refused with "A branch named change/add-score-photo already exists"
- **AND** the response names its worktree, if one has it

#### Scenario: [NEWLANE-2-S2] The dialog moves to the right place for an existing branch
- **GIVEN** the dialog with the build template and the name `add-score-photo`
- **AND** branch `change/add-score-photo` already has a worktree
- **WHEN** the dialog plans the start
- **THEN** "An existing worktree" is chosen with that worktree selected
- **AND** "New branch and worktree" is not chosen

### Requirement: New lane here picks the change it is on
WHEN "New lane here" opens the dialog on a worktree whose branch matches a template's branch pattern THE SYSTEM SHALL pre-select that template, the name from the branch, and the worktree.

#### Scenario: [NEWLANE-3-S1] A change's worktree pre-selects its build
- **GIVEN** a worktree on `change/add-score-photo`
- **AND** Up next lists `add-score-photo` under the build template
- **WHEN** "New lane here" opens the dialog on that worktree
- **THEN** the build template, the name `add-score-photo` and that worktree are selected

#### Scenario: [NEWLANE-3-S2] An ambiguous pattern follows Up next
- **GIVEN** a build template and a propose template that both name `change/{name}`
- **AND** Up next lists `add-group-card-entry` under the propose template only
- **WHEN** "New lane here" opens the dialog on a worktree on `change/add-group-card-entry`
- **THEN** the propose template is selected

#### Scenario: [NEWLANE-3-S3] A worktree no template names opens with no template
- **GIVEN** a worktree on a branch that matches no template's pattern
- **WHEN** "New lane here" opens the dialog on it
- **THEN** no template is selected, and the worktree is

### Requirement: What is in a build's way is shown before it starts
WHEN a template starts a build on a change that has no Approved line, or whose Up next row names unmet dependencies, THE SYSTEM SHALL show each as a warning before Start and SHALL still let the lane start.

#### Scenario: [NEWLANE-4-S1] An unapproved change is flagged, not blocked
- **GIVEN** change `add-score-photo` with no Approved line
- **WHEN** the dialog plans a build of `add-score-photo`
- **THEN** a warning says it has no Approved line and that /build-change will stop for it
- **AND** the start button reads "Start anyway" and stays enabled

#### Scenario: [NEWLANE-4-S2] An approved change shows no approval warning
- **GIVEN** change `add-score-photo` with an Approved line
- **WHEN** the dialog plans a build of `add-score-photo`
- **THEN** no approval warning is shown and the start button reads "Start lane"

#### Scenario: [NEWLANE-4-S3] Unmet dependencies from Up next are shown
- **GIVEN** an Up next row `add-group-card-entry` whose detail says "needs 2C.10"
- **WHEN** the dialog plans that row's lane
- **THEN** a warning shows "needs 2C.10"

### Requirement: The plain lane is named for what it is
THE SYSTEM SHALL label the dialog's no-template choice "No template: a plain Claude session".

#### Scenario: [NEWLANE-5-S1] The empty option says what it starts
- **GIVEN** the New lane dialog
- **WHEN** it opens
- **THEN** the template list's empty choice reads "No template: a plain Claude session"
