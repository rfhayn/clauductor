# panel-attach Specification

## Purpose
The panel serves a repository that does not install Clauductor. It keeps that repository's panel config beside it, under the user's home, and never writes into the repository. The config's commands still run in the repository's root, against the repository's own scripts, and only once the owner trusts it.

## Requirements
### Requirement: Attach serves a repository from a config outside it
WHEN the owner runs `clauductor panel attach` on a repository THE SYSTEM SHALL write the panel config `panel init` would write for it, with its commands in argv form, to `~/.clauductor/projects/<id>/panel.json`, register the repository with that file as its config, and leave its commands off until the owner trusts it; it SHALL NOT overwrite an existing file.

#### Scenario: [ATTACH-1-S1] Attach writes the sidecar and registers the repository
- **GIVEN** a git repository with no `.clauductor/panel.json` that is not registered
- **WHEN** the owner runs `clauductor panel attach <repo>`
- **THEN** `~/.clauductor/projects/<id>/panel.json` exists, and `projects.json` lists the repository with `config` naming that file
- **AND** the project loads, its config is untrusted, and the output names `clauductor panel trust --project <repo>`

#### Scenario: [ATTACH-1-S2] The sidecar is panel init's reading of the repository
- **GIVEN** a git repository with the operating model's queue scripts, and another without them
- **WHEN** each is attached
- **THEN** each sidecar equals what `panel init` plans for that repository, except that every command is an argv list with no `sh -c` and no `2>&1` (the operating model's cards run `["sh", ".claude/owner-queue.sh"]` and `["sh", ".claude/roadmap-queue.sh", "--text"]`)
- **AND** the output prints init's notes for each choice, and the sidecar passes the sidecar command rule with no line marked

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

### Requirement: A config outside its repository runs only the repository's commands, as argv with no shell text
WHERE a project's config resolves outside its repository THE SYSTEM SHALL NOT trust it, by any way of trusting or by a trust entry recorded earlier, while any card, queue, suggest, metrics or worktree-hook command in it holds shell or interpreter code, a word that is not literal, or a path outside the repository; the trust report SHALL mark each such command with the word, why, and "move it into a script in the repository".

#### Scenario: [ATTACH-3-S1] Trust is refused for a command outside the repository
- **GIVEN** an attached project whose sidecar has a card running `/usr/local/bin/report`, and a queue running `sh ../other/gate.sh`
- **WHEN** the owner runs `clauductor panel trust --project <repo>`
- **THEN** trust is refused, and each of the two lines is printed marked as not allowed in a sidecar, with the word that resolves outside and "move it into a script in the repository"
- **AND** nothing is recorded in the project's `trusted-config.json`

#### Scenario: [ATTACH-3-S2] A trust entry recorded earlier does not run them
- **GIVEN** an attached project whose sidecar names a command starting with `~/bin/`, and whose exact bytes are recorded as trusted in `trusted-config.json`
- **WHEN** the panel loads it
- **THEN** its cards, queue commands and templates are off, under the Config untrusted banner
- **AND** the banner's trust report marks the command as not allowed in a sidecar

#### Scenario: [ATTACH-3-S3] The repository's own argv commands are trusted
- **GIVEN** an attached project whose sidecar runs each argv command `panel init` writes for an attached repository, `["sh", ".claude/owner-queue.sh"]`, `["sh", ".claude/roadmap-queue.sh", "--text"]`, `["sh", ".claude/panel-suggest.sh", "build"]`, `["scripts/ci/run-local.sh"]`, and each detected gate, `["npm", "run", "gate"]`, `["pnpm", "run", "gate"]`, `["yarn", "run", "gate"]`, `["bun", "run", "gate"]` and `["make", "gate"]`, and also `["sh", ".claude/x.sh", "arg"]`, `["node", "infra/roadmap-queue.mjs", "--text"]`, `["git", "log", "--format=%h"]`, `["git", "log", "main..HEAD"]`, `["go", "test", "."]`, `["go", "test", "./..."]`, `["git", "add", "."]`, `["make", "-j4", "gate"]` and `["make", "-C", "sub", "gate"]`
- **AND** every script file named exists in the repository, and `sub` is a directory inside it
- **WHEN** the owner trusts it
- **THEN** it is trusted and recorded, and no line is marked

#### Scenario: [ATTACH-3-S4] An in-repository config keeps today's rules
- **GIVEN** a project whose config is its own `.clauductor/panel.json`, and runs `/usr/local/bin/report` and `sh -c "sh .claude/owner-queue.sh 2>&1"`
- **WHEN** the owner trusts it
- **THEN** it is trusted and recorded, as before this change

#### Scenario: [ATTACH-3-S5] A symlink out of the repository is outside it
- **GIVEN** an attached project whose sidecar runs `scripts/link.sh`, a symlink inside the repository to a script outside it
- **WHEN** the owner trusts it
- **THEN** trust is refused, and the line is marked as not allowed in a sidecar

#### Scenario: [ATTACH-3-S6] The page refuses the same trust
- **GIVEN** the attached project of ATTACH-3-S1, served untrusted
- **WHEN** the page's Trust config… asks for its report and then trusts it with the report's hash
- **THEN** the report's lines for the two commands are marked as not allowed in a sidecar
- **AND** the trust is refused and nothing is recorded

#### Scenario: [ATTACH-3-S7] Shell and interpreter code is refused
- **GIVEN** attached projects whose sidecars each have one card, running `["sh", "-c", "sh .claude/x.sh"]`, `["bash", "-lc", "make gate"]`, `["env", "sh", "-c", "make"]`, `["eval", "make"]`, `["xargs", "make"]`, `["find", ".", "-exec", "make", ";"]`, `["python3", "-c", "print(1)"]`, `["python3", "-m", "http.server"]`, `["node", "-e", "1"]`, `["node", "--import", "x.mjs", "a.js"]`, `["caffeinate", "-i", "bash", "-c", "make"]`, `["env", "-S", "make"]`, `["npx", "tool"]`, `["npm", "exec", "tool"]`, `["npm", "-y", "exec", "tool"]`, `["pnpm", "-C", "sub", "dlx", "tool"]`, `["npm", "create", "app"]`, `["go", "run", "example.com/tool@v1"]`, `["timeout", "9", "npx", "tool"]`, `["env", "FOO=1", "npx", "tool"]`, `["nice", "-n", "5", "xargs", "make"]`, `["env", "nice", "npx", "tool"]`, `["sh"]` with no script file, and `["sh", ".claude/missing.sh"]` naming a file that does not exist
- **WHEN** the owner trusts each
- **THEN** each is refused, its line marked as not allowed in a sidecar with the word, why, and "move it into a script in the repository"; `["env", "-S", "make"]`'s reason is `env -S` itself, since the word has no space
- **AND** a code flag after an interpreter is refused behind any launcher (`caffeinate` is on no list). Every program-position rule is judged at `argv[0]` or after a launcher's own flags, their values, `env`'s `NAME=value` words and `timeout`'s duration, through chained launchers: `.`, `eval`, `source`, `env -S`, `awk` and `sed` without `-f`, `xargs`, `find -exec`, fetch-and-run, and the script-file rule. So `["go", "test", "."]` is trusted (ATTACH-3-S3)

#### Scenario: [ATTACH-3-S8] A word that is not literal is refused, and so is a redirection
- **GIVEN** attached projects whose sidecars each have one card, running `["ls", "$HOME"]`, `["ls", "*.go"]`, `["ls", "~"]`, `["git", "log", "--format=%h %s"]`, `["sh", ".claude/x.sh", "2>/dev/null"]`, `["sh", ".claude/x.sh", ">", "out.txt"]`, `["sh", ".claude/x.sh", "'a'"]`, `["sh", ".claude/x.sh", "a\\b"]` and `["git", "-c", "alias.x=!gate", "x"]` (refused for the character `!`)
- **WHEN** the owner trusts each
- **THEN** each is refused, its line marked as not allowed in a sidecar: with no shell, `$`, `*`, `~`, whitespace, quotes, a backslash, `!` and a redirection would reach the program as they are
- **AND** no `/dev/null` exception applies: the owner's exception of 2026-10-13 for a redirection to `/dev/null` is superseded, because a sidecar has no redirections

#### Scenario: [ATTACH-3-S9] A path reached through `..`, a flag or an assignment stays inside
- **GIVEN** attached projects whose sidecars each have one card, running `["make", "-C", "..", "gate"]`, `["make", "-C..", "gate"]`, `["npm", "--prefix=..", "run", "gate"]`, `["git", "--git-dir=/tmp/x", "log"]`, `["git", "--exec-path=..", "log"]`, `["env", "PATH=x:..", "gate"]`, `["env", "NODE_OPTIONS=--require=../x.js", "make"]`, `["cc", "-I../lib", "-o/tmp/x", "a.c"]`, `["cargo", "build", "--manifest-path=../o/Cargo.toml"]`, `["curl", "-d", "@/etc/hosts"]`, `["make", "-p.."]`, `["make", "-sC..", "gate"]`, `["make", "-sC/tmp", "gate"]` and `["cat", "docs/../../x"]`
- **WHEN** the owner trusts each
- **THEN** each is refused, its line marked as not allowed in a sidecar with the piece that resolves outside the repository. A word holding `/` or `..` is split at `=`, `:` and `,`; a piece starting with a single `-` is judged from its first `/` or `.`; a leading `@` is stripped; then each piece is checked as a path, with no list of flags
- **AND** `["git", "log", "main..HEAD"]`, `["make", "-j4", "gate"]`, `["go", "test", "./..."]` and `["make", "-C", "sub", "gate"]` are trusted (ATTACH-3-S3)

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

### Requirement: A sidecar's card shows its stderr without a shell
WHERE a project's config resolves outside its repository THE SYSTEM SHALL show each card's stderr together with its stdout, so the card needs no `2>&1`; a metrics or suggest command's stdout SHALL stay alone, because the panel parses it.

#### Scenario: [ATTACH-6-S1] A card's stderr shows in a sidecar, and not in an in-repository config
- **GIVEN** an attached project, and a project with its own config, each with a card running a repository script that writes one line to stdout and one to stderr and exits 0
- **WHEN** the panel runs each card
- **THEN** the attached project's card shows both lines
- **AND** the other project's card shows the stdout line alone, as today
- **AND** the attached project's suggest command, which writes its suggestions to stdout and a line to stderr, still parses: only a card's output is merged
- **AND** an attached project's card whose stdout is JSON and which writes a line to stderr is still a JSON card: its stdout is parsed alone first, and stderr lines are added only to a text card

