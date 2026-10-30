# panel-catch-up Specification

## Purpose
After a merge, the panel brings the machine to the merged `main` by itself, where the owner has
turned that on: it fast-forwards the project's main checkout over pull requests merged from the
owner's own account, trusts a changed `panel.json` that reached it that way, and rebuilds and
restarts itself when the merge changed the panel's own source. Whatever it cannot do safely it
leaves as it was, and says why, once.

## Requirements
### Requirement: A machine turns catch-up on per project, and it is off until then
THE SYSTEM SHALL keep catch-up per project in `~/.stridal/panel/catch-up.json`, a list of the repository roots that have it on, off for a root the file does not list or when the file is absent, set only by `stridal panel catch-up --project <path> on|off`, which with no state prints the project's state, and cleared for a root by `stridal panel remove`; SHALL refuse to turn it on for a project whose config is outside its repository, writing nothing, and at each trigger SHALL skip such a project, sending one notice whatever its `alerts.notify` says, titled "stridal panel" and naming the root; SHALL NOT write `projects.json` for it, and SHALL NOT read it from the repository; SHALL read the file again at each trigger of the fast-forward, so a change takes effect with no restart; and WHILE it is off for a project THE SYSTEM SHALL NOT fetch, merge, trust on arrival or rebuild for that project.

#### Scenario: [CATCHUP-1-S1] Catch-up is recorded per project, and printed
- **GIVEN** two registered projects, A and B, and no `catch-up.json`
- **WHEN** the owner runs `stridal panel catch-up --project <A> on`, then `stridal panel catch-up --project <A>` and `stridal panel catch-up --project <B>`
- **THEN** `catch-up.json` lists A's root and not B's, and `projects.json` is byte for byte as before
- **AND** the two prints say "on" for A and "off" for B
- **AND** after `stridal panel catch-up --project <A> off`, the print for A says "off"

#### Scenario: [CATCHUP-1-S2] An attached project cannot turn catch-up on
- **GIVEN** an attached project, its config outside its repository
- **WHEN** the owner runs `stridal panel catch-up --project <it> on`
- **THEN** the command exits non-zero with an error naming the config outside the repository
- **AND** no `catch-up.json` is written, and `projects.json` is byte for byte as before

#### Scenario: [CATCHUP-1-S3] With catch-up off, a merge leaves the checkout alone
- **GIVEN** a project with catch-up off, whose main checkout is one commit behind `origin/main`, and a lane whose pull request #7 is open
- **WHEN** #7 leaves the open list on the open-PR poll
- **THEN** the repository's files and HEAD are as before, no `git fetch` or `git merge` ran, and no catch-up notice goes out

#### Scenario: [CATCHUP-1-S4] A repository cannot turn catch-up on for itself
- **GIVEN** a `panel.json` that sets a `catch_up` key
- **WHEN** the panel loads it
- **THEN** the load fails, and the error names the key

#### Scenario: [CATCHUP-1-S5] Turning catch-up off on a running panel takes effect at once
- **GIVEN** a running panel, a project with catch-up on whose main checkout is one commit behind `origin/main`, and a lane whose pull request #7 is open
- **WHEN** the owner runs `stridal panel catch-up --project <it> off`, and then #7 leaves the open list
- **THEN** the checkout's HEAD and files are as before, and no `git fetch` or `git merge` ran, with no restart of the panel

#### Scenario: [CATCHUP-1-S6] A project whose config moved outside its repository is skipped
- **GIVEN** a project with catch-up on, whose main checkout is one commit behind `origin/main`
- **AND** the owner ran `stridal panel add --project <its root> --config <a file outside it>`
- **WHEN** a pull request leaves the open list
- **THEN** the checkout's HEAD and files are as before, no `git fetch` or `git merge` ran, and one notice names the config outside the repository

#### Scenario: [CATCHUP-1-S7] A project removed and added again starts with catch-up off
- **GIVEN** a project with catch-up on
- **WHEN** the owner runs `stridal panel remove` on it, then registers a repository at the same root again
- **THEN** `catch-up.json` does not list the root, and `stridal panel catch-up --project <root>` prints "off"

### Requirement: The panel fast-forwards the main checkout after a merge, only over the owner's merged pull requests, and only a clean one
WHILE catch-up is on for a project THE SYSTEM SHALL, WHEN a pull request leaves the project's open list on the open-PR poll and at the panel's start, run from outside every project runtime and fast-forward the project's main checkout to its base (`base`, default `origin/main`, read as `<remote>/<branch>`) by `git fetch <remote> <branch>`, then reading once the commit C that `<remote>/<branch>` names, then `git merge --ff-only C`, by C's hash and never by the ref's name, each under a 60 s timeout, only WHEN HEAD is `<branch>` tracking `<remote>/<branch>`, no tracked file differs from HEAD, no merge, rebase or cherry-pick is in progress, `<branch>` has no commit `<remote>/<branch>` lacks, and every first-parent commit in `HEAD..C` is the merge commit of a pull request merged into `<branch>` from the login `gh api user` names, read through GitHub's commit-to-pull-request route and not its search; otherwise, and WHEN git or `gh` refuses or fails, THE SYSTEM SHALL leave the checkout as it was, write a line naming the checkout and the reason to the panel log, and send one notice saying the same and naming any commit that failed the pull-request check, whatever the project's `alerts.notify` says, titled "stridal panel" and naming the checkout's path, at most once per project, reason and remote commit while the panel runs; WHEN `git fetch` or a `gh` call errors or times out, or any git call times out, THE SYSTEM SHALL try the fast-forward again 1, 3 and 6 minutes later, and no more until the next trigger; and after any other refusal, git's own refusals of the merge or of a precondition included, it SHALL NOT try again before the next trigger.

#### Scenario: [CATCHUP-2-S1] A merge brings the main checkout to the merged base
- **GIVEN** a project with catch-up on, whose main checkout is on `main`, tracking `origin/main`, clean, and one commit behind it, that commit being the merge commit of pull request #7 merged into `main` from the login `gh api user` names
- **WHEN** #7 leaves the open list on the open-PR poll
- **THEN** the checkout's HEAD is `origin/main`'s commit, and the panel log has a line naming the old and new commits

#### Scenario: [CATCHUP-2-S2] A tracked change stops the fast-forward, and says so once
- **GIVEN** a project with catch-up on, whose main checkout has a modified tracked file and is one commit behind `origin/main`
- **WHEN** pull request #7 leaves the open list, and then pull request #8 does with `origin/main` unchanged
- **THEN** the checkout's HEAD and files are as before
- **AND** one notice goes out naming the checkout and the tracked change, and the second poll sends none

#### Scenario: [CATCHUP-2-S3] Another branch, a detached HEAD or a commit of its own is left alone
- **GIVEN** in turn a main checkout on a branch other than `main`, one with a detached HEAD, and one on `main` with a commit `origin/main` lacks
- **WHEN** a pull request leaves the open list
- **THEN** each checkout's HEAD and files are as before, and a notice names the reason

#### Scenario: [CATCHUP-2-S4] A checkout behind at the panel's start is caught up
- **GIVEN** a project with catch-up on, whose main checkout is clean on `main` and two commits behind `origin/main`, each the merge commit of a pull request merged from the login `gh api user` names, and no pull request open
- **WHEN** the panel starts
- **THEN** the checkout's HEAD is `origin/main`'s commit

#### Scenario: [CATCHUP-2-S5] An untracked file the fast-forward would overwrite stops it
- **GIVEN** a project with catch-up on, whose clean main checkout holds an untracked file at a path the next commit on `origin/main` adds
- **WHEN** a pull request leaves the open list
- **THEN** the checkout's HEAD and the untracked file are as before
- **AND** the notice names git's error
- **AND** with a minute, 3 minutes and 6 minutes passing and no other pull request leaving the list, no second `git merge` runs

#### Scenario: [CATCHUP-2-S6] A commit no pull request merged stops the fast-forward
- **GIVEN** a project with catch-up on, whose clean main checkout is behind `origin/main` by the merge commit of the owner's pull request #7 and, after it, a commit pushed with no pull request
- **WHEN** #7 leaves the open list
- **THEN** the checkout's HEAD and files are as before
- **AND** the notice names the pushed commit and says it came with no pull request

#### Scenario: [CATCHUP-2-S7] A pull request merged from another account stops the fast-forward
- **GIVEN** a project with catch-up on, `gh api user` naming `owner`, and a clean main checkout one commit behind `origin/main`, that commit being the merge commit of pull request #7 merged from `other`
- **WHEN** #7 leaves the open list
- **THEN** the checkout's HEAD and files are as before
- **AND** the notice names #7 and `other`

#### Scenario: [CATCHUP-2-S8] A pull-request lookup that fails stops the fast-forward
- **GIVEN** a project with catch-up on, and a clean main checkout one commit behind `origin/main`, that commit being the merge commit of the owner's pull request #7
- **AND** `gh` failing when asked which pull request merged the commit
- **WHEN** #7 leaves the open list
- **THEN** the checkout's HEAD and files are as before
- **AND** the notice names the failed lookup

#### Scenario: [CATCHUP-2-S9] A lookup that fails once is tried again
- **GIVEN** a project with catch-up on, and a clean main checkout one commit behind `origin/main`, that commit being the merge commit of the owner's pull request #7
- **AND** `gh` failing at the first lookup and answering at the second
- **WHEN** #7 leaves the open list, and a minute passes with no other pull request leaving the list
- **THEN** the checkout's HEAD is `origin/main`'s commit
- **AND** after a pull-request check that fails as policy (a commit with no pull request), no second attempt runs until the next trigger

#### Scenario: [CATCHUP-2-S10] A base that moves during the check is not merged past what was checked
- **GIVEN** a project with catch-up on, and a clean main checkout one commit behind `origin/main`, that commit C being the merge commit of the owner's pull request #7
- **AND** while the pull-request lookup for C runs, another worktree's fetch moves `origin/main` to a commit D after C that was pushed with no pull request
- **WHEN** #7 leaves the open list and the fast-forward runs
- **THEN** the checkout's HEAD is C, not D
- **AND** D is not looked up, and no notice names it until a later trigger checks it

#### Scenario: [CATCHUP-2-S11] With notifications off, a refused fast-forward still says so
- **GIVEN** a project with catch-up on and `alerts.notify` false, whose main checkout has a modified tracked file and is one commit behind `origin/main`, and a valid `push.json` whose `argv` is a stand-in program that records its arguments
- **WHEN** pull request #7 leaves the open list
- **THEN** the checkout's HEAD and files are as before
- **AND** one notice naming the checkout's path and the tracked change goes out as a macOS notification and as a push, titled "stridal panel"

### Requirement: A changed `panel.json` that the owner's merge brought is trusted on arrival
WHEN the panel's own fast-forward moves a project's HEAD from A to B and changes its in-repository `panel.json`, THE SYSTEM SHALL record the file's exact bytes as trusted, send one notice naming the pull request(s), the merging login, and each line of the config's run list added (`+`) or removed (`−`), and then restart the project's runtime on them from outside every project runtime, with the trust note "trusted on arrival from #N", only WHEN the config the runtime had loaded was trusted and its bytes are the file's blob at A, the file on disk is its blob at B, every commit in `A..B` passed the fast-forward's pull-request check, the new config loads in the binary that will serve it, and the new config's `alerts.notify` is true; WHEN the same fast-forward leads the panel to rebuild itself, THE SYSTEM SHALL decide whether the new config loads, and read its `alerts.notify` and run list, with the new binary before the swap, and WHEN the build or its preflight fails SHALL trust nothing, judge whether the new config loads with the running binary, which then serves it as below, the build's error going in the rebuild's own notice (the next requirement) and not in this requirement's; WHEN the new config loads and any other of those fails, THE SYSTEM SHALL restart the runtime on it untrusted, with **Trust config…** offered, write the reason to the panel log, and, WHILE the loaded config's `alerts.notify` is true, send a notice naming why and saying the config waits for the owner; WHEN the new config does not load, THE SYSTEM SHALL keep the config the runtime had loaded and send a notice naming the error whatever the loaded config's `alerts.notify` says, titled "stridal panel" and naming the checkout's path; the notice that a config waits for the owner is the only notice of this capability that `alerts.notify` withholds; and THE SYSTEM SHALL change neither trust nor the runtime for a fast-forward that leaves the file unchanged or for a change the panel did not fast-forward to.

#### Scenario: [CATCHUP-3-S1] The owner's merged pull request is trusted on arrival
- **GIVEN** a project with catch-up on, serving a trusted `panel.json` whose bytes are the file at HEAD, and `gh api user` naming `owner`
- **AND** pull request #7, merged into `main` from `owner`, whose merge commit adds a card running `sh .claude/x.sh` and removes one running `sh .claude/y.sh`
- **WHEN** #7 leaves the open list and the panel fast-forwards to it
- **THEN** the project serves the new config as trusted, with the new card's command running and the trust note "trusted on arrival from #7"
- **AND** one notice goes out naming "#7", `owner`, a `+` line naming `sh .claude/x.sh` and a `−` line naming `sh .claude/y.sh`

#### Scenario: [CATCHUP-3-S2] A config already on disk but never loaded is not trusted by a later merge
- **GIVEN** a project with catch-up on, serving a trusted `panel.json` with bytes X
- **AND** the owner pulled the main checkout by hand to a commit pushed with no pull request, which changed the file to bytes Y, and the runtime was not restarted
- **WHEN** the owner's pull request #8, which changes the file to bytes Z, leaves the open list and the panel fast-forwards to it
- **THEN** the project serves Z untrusted, with **Trust config…** offered, and Y and Z are not recorded as trusted
- **AND** the notice says the loaded config was not the file the fast-forward started from, and that the config waits for the owner

#### Scenario: [CATCHUP-3-S3] A config untrusted before stays untrusted
- **GIVEN** a project with catch-up on, serving an untrusted `panel.json`, and the owner's merged pull request #7 that changes it
- **WHEN** the panel fast-forwards to it
- **THEN** the project serves the new config untrusted, and the notice says the config was not trusted before

#### Scenario: [CATCHUP-3-S4] With notifications off, nothing is trusted on arrival
- **GIVEN** a project with catch-up on and `alerts.notify` false, serving a trusted `panel.json`, and the owner's merged pull request #7 that changes it and keeps `alerts.notify` false
- **WHEN** the panel fast-forwards to it
- **THEN** the checkout's HEAD is #7's merge commit, and the project serves the new config untrusted
- **AND** no notice goes out, and the panel log names why the config was not trusted

#### Scenario: [CATCHUP-3-S5] A new config that does not load is not served
- **GIVEN** a project with catch-up on, serving a trusted `panel.json`, and the owner's merged pull request #7 whose `panel.json` does not load
- **WHEN** the panel fast-forwards to it
- **THEN** the project serves the config it had loaded, still trusted, and the notice names the load's error
- **AND** with `alerts.notify` false in the loaded config, the notice still goes out

#### Scenario: [CATCHUP-3-S6] A config the owner pulled by hand is not trusted on arrival
- **GIVEN** a project with catch-up on, serving a trusted `panel.json`, and the owner's merged pull request #7 that changes it
- **WHEN** the owner pulls the main checkout by hand before the panel's next poll, and the poll runs
- **THEN** the loaded config is unchanged and still trusted, the file's new bytes are not recorded as trusted, and no catch-up notice names #7

#### Scenario: [CATCHUP-3-S7] A merge that leaves `panel.json` alone changes nothing of the config
- **GIVEN** a project with catch-up on, serving a trusted `panel.json`, and the owner's merged pull request #7 that changes only `README.md`
- **WHEN** the panel fast-forwards to it
- **THEN** the project's runtime is not restarted, and its trust and trust note are as before

#### Scenario: [CATCHUP-3-S8] A config the new binary reads is trusted after the rebuild
- **GIVEN** a panel running as the login agent, built from the project's repository with no local changes, catch-up on, serving a trusted `panel.json`
- **AND** the owner's pull request #7, which raises the config's `version` above what the running binary reads and changes `framework/` to read it
- **WHEN** the panel fast-forwards to it
- **THEN** the new bytes are recorded as trusted before the swap, and the new binary serves them trusted with the trust note "trusted on arrival from #7"
- **AND** no notice names a load error
- **AND** when instead #7's `framework/` does not compile, nothing is recorded as trusted, the running binary serves the config it had loaded, one notice names the build's error, and a second names the new config's load error

#### Scenario: [CATCHUP-3-S9] A merged config that turns notices off is not trusted on arrival
- **GIVEN** a project with catch-up on, serving a trusted `panel.json` with `alerts.notify` true, and the owner's merged pull request #7 that adds a card and sets `alerts.notify` false
- **WHEN** the panel fast-forwards to it
- **THEN** the project serves the new config untrusted, with **Trust config…** offered
- **AND** one notice goes out saying the config turns notices off and waits for the owner

#### Scenario: [CATCHUP-3-S10] A failed build leaves a config the running binary reads untrusted
- **GIVEN** a panel running as the login agent, built from the project's repository with no local changes, catch-up on, serving a trusted `panel.json`
- **AND** the owner's pull request #7, which adds a card the running binary can read and changes `framework/` so that it does not compile
- **WHEN** the panel fast-forwards to it
- **THEN** nothing is recorded as trusted, the installed binary is unchanged, and the running binary serves the new config untrusted, with **Trust config…** offered
- **AND** one notice names the build's error, and a second says the config waits for the owner
- **AND** with `alerts.notify` false in the loaded config, the notice naming the build's error still goes out, and the second does not

### Requirement: The panel rebuilds and restarts itself when a merge changes its own source
WHEN the panel's own fast-forward moves a project's HEAD from A to B and the panel's build information names the module `<root>/framework/go.mod` declares, THE SYSTEM SHALL rebuild itself only WHEN it runs as the login agent, its `vcs.revision` R is a commit of the repository and an ancestor of B, `vcs.modified` is false, `framework/` differs between R and B, and `git status --porcelain` in the main checkout prints nothing; it SHALL build `./cmd/stridal` in `<root>/framework` into `~/.stridal/panel/bin/stridal.new` under a 5-minute timeout, writing nothing in the repository; require the new binary's build information to name the same module, `vcs.revision` B and `vcs.modified` false, and its `version` and `panel list` each to exit 0; rename the installed binary to `stridal.prev` and the new one into its place, and do the same for the `stridal` on its PATH only WHEN that one's build information names the same module and a commit of the repository; leave the token, the login agent's plist and the user's hooks as they were; wait until none of its own `claude` calls runs, up to 10 minutes; send a notice naming R, B and the rollback, adding `stridal panel install` WHEN `R..B` changed `framework/internal/panel/install/launchd.go` or `framework/internal/panel/install/hooks.go`; record a browser open; shut down as on SIGTERM; and exit the process with status 75, which the login agent restarts. The new binary's first start SHALL send a notice naming its commit. THE SYSTEM SHALL send every notice of this requirement whatever the project's `alerts.notify` says, titled "stridal panel", as a macOS notification and through the push command. WHEN the panel's module is not the project's, THE SYSTEM SHALL build nothing and send nothing; WHEN it is, `framework/` changed and any other condition fails, THE SYSTEM SHALL keep running and send one notice saying why and how to update; and WHEN the build or a preflight fails or times out THE SYSTEM SHALL keep running, leave no `stridal.new`, and send one notice naming the error.

#### Scenario: [CATCHUP-4-S1] A merge into `framework/` rebuilds, swaps and restarts the panel
- **GIVEN** a panel running as the login agent, built at commit R of the project's repository with no local changes, catch-up on, and the owner's pull request #7 that changes `framework/internal/panel/server.go`
- **WHEN** the panel fast-forwards to #7's merge commit B
- **THEN** `~/.stridal/panel/bin/stridal` is the new build, its build information naming B and no local changes, and `stridal.prev` is the binary built at R
- **AND** the token file and the plist are byte for byte as before
- **AND** a notice names R, B and the rollback's `mv` and `launchctl kickstart` commands, and the `stridal` process exits with status 75

#### Scenario: [CATCHUP-4-S2] A failed build keeps the running panel
- **GIVEN** a panel that would rebuild, and the owner's pull request #7 whose `framework/` does not compile
- **WHEN** the panel fast-forwards to it
- **THEN** the panel keeps running, `~/.stridal/panel/bin/stridal` is unchanged, and no `stridal.new` is left
- **AND** one notice names the build's error

#### Scenario: [CATCHUP-4-S3] A binary of this source that cannot be rebuilt from it is left alone, and says so
- **GIVEN** in turn a running panel of the project's module with no VCS build information, one built with local changes, and one built at a commit that is not an ancestor of the merge
- **WHEN** the panel fast-forwards to a merge that changes `framework/`
- **THEN** nothing is built and the installed binary is unchanged
- **AND** one notice says why, and how to update the panel

#### Scenario: [CATCHUP-4-S4] A panel started by hand is not replaced
- **GIVEN** a panel started by hand, not as the login agent, built at commit R of the repository, and a merge that changes `framework/`
- **WHEN** the panel fast-forwards to it
- **THEN** nothing is built, the panel keeps running, and one notice says `framework/` changed and how to rebuild and restart

#### Scenario: [CATCHUP-4-S5] The build writes nothing in the repository
- **GIVEN** a panel that rebuilds after a merge that changes `framework/`
- **WHEN** the build and the preflight run
- **THEN** the repository's files are as the fast-forward left them, and its status shows no change

#### Scenario: [CATCHUP-4-S6] A change to what install writes names `stridal panel install`
- **GIVEN** a panel built at R, and an earlier merge that changed `framework/internal/panel/install/launchd.go` but did not rebuild the panel (an untracked file stood in the checkout)
- **WHEN** a later merge A..B, which changes only `framework/internal/panel/server.go`, rebuilds the panel at B and it sends its restart notice
- **THEN** the notice says to run `stridal panel install` once

#### Scenario: [CATCHUP-4-S7] A restart opens no browser tab
- **GIVEN** a panel that rebuilds and restarts after a merge
- **WHEN** the login agent starts the new binary within 5 minutes of the restart
- **THEN** no browser tab is opened

#### Scenario: [CATCHUP-4-S8] The new binary says it is running
- **GIVEN** a panel that restarted after rebuilding at commit B
- **WHEN** the new binary starts
- **THEN** one notice names B, and a second start sends none

#### Scenario: [CATCHUP-4-S9] An untracked file in the checkout means no rebuild
- **GIVEN** a panel that would rebuild, and a main checkout that holds an untracked file after the fast-forward to a merge that changes `framework/`
- **WHEN** the panel decides whether to rebuild
- **THEN** nothing is built and the installed binary is unchanged
- **AND** one notice names the untracked file, and says the build would be stamped as modified

#### Scenario: [CATCHUP-4-S10] Another project's `framework/` is not the panel's source
- **GIVEN** a panel running as the login agent, and another project with catch-up on whose `framework/go.mod` declares a different module
- **WHEN** the panel fast-forwards that project to a merge that changes its `framework/`
- **THEN** nothing is built, the installed binary is unchanged, and no notice about the panel's update goes out

#### Scenario: [CATCHUP-4-S11] With notifications off, the rebuild still says how to roll back
- **GIVEN** a panel running as the login agent that would rebuild, a project with catch-up on and `alerts.notify` false, and a valid `push.json` whose `argv` is a stand-in program that records its arguments
- **WHEN** the panel fast-forwards to a merge that changes `framework/`, rebuilds and restarts
- **THEN** the restart notice naming R, B and the rollback goes out as a macOS notification and as a push, titled "stridal panel"
- **AND** the new binary's notice naming B goes out the same way

