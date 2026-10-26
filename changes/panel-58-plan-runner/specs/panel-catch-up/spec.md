## MODIFIED Requirements

### Requirement: A changed `panel.json` that the owner's merge brought is trusted on arrival
WHEN the panel's own fast-forward moves a project's HEAD from A to B and changes its in-repository `panel.json`, THE SYSTEM SHALL record the file's exact bytes as trusted, send one notice naming the pull request(s), the merging login, and each line of the config's run list added (`+`) or removed (`−`), and then restart the project's runtime on them from outside every project runtime, with the trust note "trusted on arrival from #N", only WHEN the config the runtime had loaded was trusted and its bytes are the file's blob at A, the file on disk is its blob at B, every commit in `A..B` passed the fast-forward's pull-request check, the new config loads in the binary that will serve it, the new config's `alerts.notify` is true, and the new config's plan runner is off (`plan_runner.on` false or unset), since a config with the runner on starts lanes from everything it holds; WHEN the same fast-forward leads the panel to rebuild itself, THE SYSTEM SHALL decide whether the new config loads, and read its `alerts.notify`, whether its plan runner is on (off when the new binary prints no `plan_runner`, since such a binary has no plan runner) and its run list, with the new binary before the swap, and WHEN the build or its preflight fails SHALL trust nothing, judge whether the new config loads with the running binary, which then serves it as below, the build's error going in the rebuild's own notice (the next requirement) and not in this requirement's; WHEN the new config loads and any other of those fails, THE SYSTEM SHALL restart the runtime on it untrusted, with **Trust config…** offered, write the reason to the panel log, and, WHILE the loaded config's `alerts.notify` is true, send a notice naming why and saying the config waits for the owner; WHEN the new config does not load, THE SYSTEM SHALL keep the config the runtime had loaded and send a notice naming the error whatever the loaded config's `alerts.notify` says, titled "clauductor panel" and naming the checkout's path; the notice that a config waits for the owner is the only notice of this capability that `alerts.notify` withholds; and THE SYSTEM SHALL change neither trust nor the runtime for a fast-forward that leaves the file unchanged or for a change the panel did not fast-forward to.

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


#### Scenario: [CATCHUP-3-S11] A merged config with the plan runner on is not trusted on arrival
- **GIVEN** a project with catch-up on, serving a trusted `panel.json` with the plan runner off, and the owner's merged pull request #7 that sets `plan_runner.on` true
- **WHEN** the panel fast-forwards to it
- **THEN** the project serves the new config untrusted, with **Trust config…** offered, and the plan runner starts nothing
- **AND** one notice goes out saying the config has the plan runner on and waits for the owner
- **AND** when #7 also changes `framework/`, so the panel rebuilds, the new binary's `config-check` answer carries the plan runner and the outcome is the same
- **AND** when instead the loaded config has the plan runner on and #7 changes its `seven_day_pct`, or changes only a template's `first_prompt`, the new config is served untrusted in the same way
- **AND** when instead the loaded config has the plan runner on and #7 turns it off, the project serves the new config trusted, with the trust note "trusted on arrival from #7"
