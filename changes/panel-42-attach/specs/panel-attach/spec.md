## Purpose
The panel serves a repository that does not install Clauductor. It keeps that repository's panel config beside it, under the user's home, and never writes into the repository. The config's commands still run in the repository's root, against the repository's own scripts, and only once the owner trusts it.

## ADDED Requirements

### Requirement: Attach serves a repository from a config outside it
WHEN the owner runs `clauductor panel attach` on a repository THE SYSTEM SHALL write the panel config `panel init` would write for it to `~/.clauductor/projects/<id>/panel.json`, register the repository with that file as its config, and leave its commands off until the owner trusts it; it SHALL NOT overwrite an existing file.

#### Scenario: [ATTACH-1-S1] Attach writes the sidecar and registers the repository
- **GIVEN** a git repository with no `.clauductor/panel.json` that is not registered
- **WHEN** the owner runs `clauductor panel attach <repo>`
- **THEN** `~/.clauductor/projects/<id>/panel.json` exists, and `projects.json` lists the repository with `config` naming that file
- **AND** the project loads, its config is untrusted, and the output names `clauductor panel trust --project <repo>`

#### Scenario: [ATTACH-1-S2] The sidecar is panel init's reading of the repository
- **GIVEN** a git repository with the operating model's queue scripts, and another without them
- **WHEN** each is attached
- **THEN** each sidecar's bytes equal what `panel init` plans for that repository
- **AND** the output prints init's notes for each choice

#### Scenario: [ATTACH-1-S3] An existing sidecar is never overwritten
- **GIVEN** `~/.clauductor/projects/<id>/panel.json` exists for the id the repository would get
- **WHEN** the owner attaches the repository
- **THEN** attach is refused, and the file's bytes are unchanged
- **AND** the message names the file, `clauductor panel add --project <repo> --config <file>`, and `--id`

#### Scenario: [ATTACH-1-S4] A registered project that cannot load is attached in place
- **GIVEN** a registered project whose registry `config` is empty, with no `.clauductor/panel.json`, and with lanes in its lane registry
- **WHEN** the owner attaches it
- **THEN** its registry entry names the sidecar as its config, with the same id and tmux socket
- **AND** its lane registry is unchanged, and the project loads

#### Scenario: [ATTACH-1-S5] A registered project whose config loads is refused
- **GIVEN** a registered project whose config loads
- **WHEN** the owner attaches it
- **THEN** attach is refused with its id and the config it reads
- **AND** nothing is written under `~/.clauductor/projects/`

#### Scenario: [ATTACH-1-S6] A refused registration leaves no sidecar
- **GIVEN** a repository whose tmux socket would be another registered project's
- **WHEN** the owner attaches it
- **THEN** attach is refused with `panel add`'s reason
- **AND** no sidecar file and no registry entry remain

### Requirement: Attaching and serving write nothing in the repository
THE SYSTEM SHALL leave every file under an attached repository's root, its `.git` directory included, unchanged by attaching it, by loading it, and by polling its git state.

#### Scenario: [ATTACH-2-S1] Attach leaves the repository untouched
- **GIVEN** a git repository with commits, a branch, an untracked file and an ignored file
- **WHEN** the owner runs `clauductor panel attach <repo>`
- **THEN** every file and directory under the root, `.git` included, has the same path, mode, size, modification time and SHA-256 as before

#### Scenario: [ATTACH-2-S2] Serving leaves the repository untouched
- **GIVEN** an attached repository loaded as the panel loads it, whose config has at least one card, so the panel polls the checkout its cards run in
- **AND** one tracked file touched so its bytes are unchanged and its modification time is newer, and another tracked file modified, so the checkout is dirty
- **WHEN** the panel polls the git state of the checkout its cards run in, its dirty-tree diff stat included
- **THEN** every file and directory under the root, `.git/index` included, is as it was before the poll

### Requirement: A config outside its repository runs only the repository's commands
WHERE a project's config resolves outside its repository THE SYSTEM SHALL NOT trust it, by any way of trusting or by a trust entry recorded earlier, while any card, queue, suggest, metrics or worktree-hook command in it names a path outside the repository or a word whose location cannot be known before it runs; the trust report SHALL mark each such command.

#### Scenario: [ATTACH-3-S1] Trust is refused for a command outside the repository
- **GIVEN** an attached project whose sidecar has a card running `/usr/local/bin/report`, and a queue running `sh -c "sh ../other/gate.sh"`
- **WHEN** the owner runs `clauductor panel trust --project <repo>`
- **THEN** trust is refused, and each of the two lines is printed marked as outside the repository, with the word that resolves outside
- **AND** nothing is recorded in the project's `trusted-config.json`

#### Scenario: [ATTACH-3-S2] A trust entry recorded earlier does not run them
- **GIVEN** an attached project whose sidecar names a command starting with `~/bin/`, and whose exact bytes are recorded as trusted in `trusted-config.json`
- **WHEN** the panel loads it
- **THEN** its cards, queue commands and templates are off, under the Config untrusted banner
- **AND** the banner's trust report marks the command as outside the repository

#### Scenario: [ATTACH-3-S3] The repository's own commands are trusted as before
- **GIVEN** an attached project whose sidecar runs `sh -c "sh .claude/owner-queue.sh 2>&1"`, `npm run gate`, `node infra/roadmap-queue.mjs --text` and `scripts/ci/run-local.sh`
- **WHEN** the owner trusts it
- **THEN** it is trusted and recorded, and no line is marked

#### Scenario: [ATTACH-3-S4] An in-repository config keeps today's rules
- **GIVEN** a project whose config is its own `.clauductor/panel.json` and runs `/usr/local/bin/report`
- **WHEN** the owner trusts it
- **THEN** it is trusted and recorded, as before this change

#### Scenario: [ATTACH-3-S5] A symlink out of the repository is outside it
- **GIVEN** an attached project whose sidecar runs `scripts/link.sh`, a symlink inside the repository to a script outside it
- **WHEN** the owner trusts it
- **THEN** trust is refused, and the line is marked as outside the repository

#### Scenario: [ATTACH-3-S6] The page refuses the same trust
- **GIVEN** the attached project of ATTACH-3-S1, served untrusted
- **WHEN** the page's Trust config… asks for its report and then trusts it with the report's hash
- **THEN** the report's lines for the two commands are marked as outside the repository
- **AND** the trust is refused and nothing is recorded

#### Scenario: [ATTACH-3-S7] A bare `..`, and a script behind any shell's flags, are outside it
- **GIVEN** an attached project whose sidecar has a queue running `make -C .. gate`, a card running `bash -lc "sh ../other/gate.sh"`, and a card running `sh -c "cd ..>/dev/null; sh other/gate.sh"`
- **WHEN** the owner trusts it
- **THEN** trust is refused, and all three lines are marked as outside the repository: `..` is a path that leaves the root, every argv word holding whitespace or a shell operator is split and checked as a script, whatever program and flags come before it, and a redirection operator attached to a word does not hide it

#### Scenario: [ATTACH-3-S8] `/dev/null` is allowed only as a redirection's target
- **GIVEN** attached projects whose sidecars each have one card, running `sh -c "sh .claude/owner-queue.sh 2>/dev/null"`, `sh -c "sh .claude/owner-queue.sh >/dev/null"`, `cat /dev/null`, and `sh -c "sh .claude/owner-queue.sh 2>/tmp/x"`
- **WHEN** the owner trusts each
- **THEN** the two that redirect to `/dev/null` are trusted and recorded, with no line marked
- **AND** `cat /dev/null` (an ordinary argument) and `2>/tmp/x` are refused, each line marked as outside the repository with the word `/dev/null` or `/tmp/x`

#### Scenario: [ATTACH-3-S9] A word a shell would expand is refused, whatever the expansion
- **GIVEN** attached projects whose sidecars each have one card, running `sh -c "sh .?/other/gate.sh"`, `bash -c "sh .{.,}/other/gate.sh"`, `bash -c "{/,}usr/local/bin/report"`, `sh -c "PATH=.:~/bin x"` and `sh -c "sh .[.]/x"`
- **WHEN** the owner trusts each
- **THEN** each is refused and its line marked as outside the repository: a word holding an unquoted `*`, `?`, `[`, `{`, `}`, `~`, `$` or backtick, or a `:` entry of a leading assignment's value holding one, is a word whose location cannot be known before it runs
- **AND** every command `panel init` writes is trusted, with no line marked

### Requirement: The registry's config is the one every command reads
THE SYSTEM SHALL read a registered project's config from the registry's `config` in every command and on the page, and SHALL say when a project reads a config outside its repository and when the repository's own `.clauductor/panel.json` is therefore not read.

#### Scenario: [ATTACH-4-S1] With both, the sidecar is read and the panel says so
- **GIVEN** an attached project whose repository also has its own `.clauductor/panel.json`
- **WHEN** the panel loads it, and the owner runs `clauductor panel list`
- **THEN** the panel serves the sidecar's name, cards and templates
- **AND** `panel list` names the sidecar's path as outside the repository, and says the repository's `.clauductor/panel.json` is not read

#### Scenario: [ATTACH-4-S2] panel trust trusts the registry's config
- **GIVEN** an attached project with no `.clauductor/panel.json` in its repository
- **WHEN** the owner runs `clauductor panel trust --project <repo>` without `--config`
- **THEN** the sidecar is trusted and recorded, and the output names its path

#### Scenario: [ATTACH-4-S3] A bare panel in an attached repository serves it
- **GIVEN** an attached project with no `.clauductor/panel.json` in its repository
- **WHEN** the panel starts with `--project <repo>` and no `--config`
- **THEN** it starts and serves the project from its sidecar

#### Scenario: [ATTACH-4-S4] The project menu names a config outside the repository
- **GIVEN** a panel serving an attached project and a project with its own config
- **WHEN** the page renders the project menu
- **THEN** the attached project's row says it reads the sidecar's path, outside the repository
- **AND** the other project's row says nothing about its config

### Requirement: Removing an attached project keeps its sidecar
WHEN an attached project is removed THE SYSTEM SHALL keep its `~/.clauductor/projects/<id>/` folder and SHALL say where it is and how to serve it again.

#### Scenario: [ATTACH-5-S1] Remove keeps the folder, and add brings it back
- **GIVEN** an attached project with no lanes
- **WHEN** the owner runs `clauductor panel remove <id>`
- **THEN** `~/.clauductor/projects/<id>/panel.json` is unchanged, and the output names it and `clauductor panel add --project <repo> --config <file>`
- **AND** running that `panel add` serves the project again from the same file, still trusted when its bytes were trusted

#### Scenario: [ATTACH-5-S2] The page's Remove confirmation says how an attached project comes back
- **GIVEN** a panel serving an attached project with no lanes, and a project with its own config
- **WHEN** the page's Remove from panel… opens its confirmation for each
- **THEN** the attached project's confirmation names its sidecar, says it stays, and gives `clauductor panel add --project <repo> --config <file>` as the way to serve it again, and does not say that adding it again from the page brings it back
- **AND** the other project's confirmation reads as it does today
