## Purpose
The map is the operating model drawn as a graph. It covers the rules, the controls AGENTS.md
names, the mechanisms that execute them, the skills and agents, the records, the roadmap's rows
and changes, and the links between them. It is generated on demand from what the repository
already holds and is never committed. The panel draws it, and the gate fails any reference in it
that points nowhere.

## ADDED Requirements

### Requirement: Print the map as schema v1
`sh .claude/map.sh` (or `--json`) SHALL print one JSON object of schema version 1 to stdout and
SHALL write nothing to the repository.

- **Top-level keys:** exactly `version` (1), `source`, `generated` (ISO 8601 UTC), `commit`
  (HEAD's full sha, or null), `notes` (strings), `layers`, `kinds`, `areas`, `nodes` and
  `links`.
- **A node:** exactly `id` (unique), `kind`, `label`, `area`, `layer`, `path`, `note` (or null),
  `changed` (`YYYY-MM-DD`, or null) and `flags` (an array). `path` is repository-relative,
  `${CLAUDE_PLUGIN_ROOT}/…` for the plugin's copy of a file, or null.
- **A link:** exactly `source`, `target`, `rel` and `note` (or null).
- **A layer:** `{n, label, note}`. A kind is `{id, label, layer}`, and an area is `{id, label}`.
- **The layers and their kinds:**
  - 0: `rule`
  - 1: `control`
  - 2: `hook`, `check`, `script`, `health`, `workflow`, `module`, `config` and `file`
  - 3: `skill` and `agent`
  - 4: `adr`, `prac` and `spec`
  - 5: `row`, `change`, `lane` and `worktree`
- **The rels:** `instance`, `executes`, `names`, `registers`, `enforces`, `models`, `extends`,
  `deps`, `builds` and `in`.
- **The flags:**
  - a control: `unexecuted` or `by-hand`;
  - a row: `queued`, `inflight`, `open`, `merged` or `cancelled`;
  - a change: `proposed`, `approved` or `archived`;
  - a spec: `proposed`.

#### Scenario: [MAP-1-S1] The map of a repository with the model installed
- **GIVEN** a repository with the operating model installed
- **WHEN** `sh .claude/map.sh --json` runs
- **THEN** stdout is one JSON object with exactly the top-level keys above, `version` 1 and
  `source` "map.sh"
- **AND** every node has exactly the node keys, its id is unique, its kind is in `kinds`, and
  its layer is that kind's layer
- **AND** every link's `source` and `target` is the id of a node in `nodes`, and its `rel` is
  one of the rels above

#### Scenario: [MAP-1-S2] The map writes nothing
- **GIVEN** a repository whose working tree is clean
- **WHEN** `sh .claude/map.sh` runs
- **THEN** `git status --porcelain` is still empty, and no file under the repository has changed

#### Scenario: [MAP-1-S3] Two runs at one commit agree
- **GIVEN** a repository at one commit, and `MAP_NOW` set
- **WHEN** `sh .claude/map.sh` runs twice
- **THEN** the two outputs are byte for byte the same
- **AND** nodes are sorted by `id`, and links by `source`, then `rel`, then `target`

#### Scenario: [MAP-1-S4] Without jq there is no map
- **GIVEN** a PATH on which `jq` cannot be found
- **WHEN** `sh .claude/map.sh` runs
- **THEN** it exits 2, stdout is empty, and stderr says `CANNOT MAP: jq is not installed`

### Requirement: Nodes from what the model already holds
The map SHALL hold one node for each of the following, each at its kind's layer:

- each numbered rule in AGENTS.md (`**N. …**`);
- each row of its *What executes each rule* table;
- each mechanism, skill or agent a link reaches;
- each skill and agent in effect;
- each enabled module;
- each ADR;
- each living spec, and each capability an open change has a delta for;
- each roadmap row;
- each open or archived change.

#### Scenario: [MAP-2-S1] Rules and controls come from AGENTS.md
- **GIVEN** an AGENTS.md with four `**N. …**` rules and a *What executes each rule* table of R
  rows
- **WHEN** the map is printed
- **THEN** it has four `rule` nodes, `rule:1` to `rule:4`, at layer 0, and R `control` nodes at
  layer 1
- **AND** after a row is inserted above another row and the map is printed again, the other
  row's control keeps its id

#### Scenario: [MAP-2-S2] A mechanism's kind comes from its path
- **GIVEN** table cells naming:
  - a hook, a check and a health line;
  - the workflow and a script directly under `.claude/`;
  - `scripts/ci/run-local.sh`;
  - `.claude/settings.json`;
  - a Go package directory
- **WHEN** the map is printed
- **THEN** their nodes' kinds are `hook`, `check`, `health`, `workflow`, `script`, `script`,
  `config` and `file`, in that order, all at layer 2

#### Scenario: [MAP-2-S3] Skills and agents in a plugin project
- **GIVEN** a project that runs the model as a plugin (`CLAUDUCTOR_FW` is the plugin root), with
  one skill of its own under `.claude/skills/`
- **WHEN** the map is printed
- **THEN** it has a `skill` node for each of the plugin's skills and one for the project's own
  skill, at layer 3
- **AND** a plugin skill's node has a `path` starting `${CLAUDE_PLUGIN_ROOT}/skills/`, `changed`
  null, and a note saying it is the plugin's copy

#### Scenario: [MAP-2-S4] Records, rows and changes
- **GIVEN** a repository with:
  - two ADRs and one living spec;
  - roadmap rows in each of the five states;
  - one change awaiting approval, one approved change and one archived change;
  - a delta in the change awaiting approval for a capability no living spec has
- **WHEN** the map is printed
- **THEN** it has two `adr` nodes, a `row` node for every row, and three `change` nodes
- **AND** it has two `spec` nodes: the living one, and the proposed capability flagged
  `proposed`
- **AND** each row's flags hold its state, and the changes are flagged `proposed`, `approved`
  and `archived`

#### Scenario: [MAP-2-S5] A control that nothing executes is flagged
- **GIVEN** a table row whose cell is only "**Nothing. You.**", and another row whose cell names
  `checks/x.sh` and also "**Nothing. You.**"
- **WHEN** the map is printed
- **THEN** the first control has the flag `unexecuted` and no `executes` link
- **AND** the second has the flag `by-hand` and an `executes` link to `.claude/checks/x.sh`

#### Scenario: [MAP-2-S6] Every node has an area
- **GIVEN** a repository with the model installed, one mechanism of the project's own that
  map.sh's area table does not place, and a control whose only mechanism is `pr-merge-guard.sh`
- **WHEN** the map is printed
- **THEN** every node's `area` is the id of an entry in `areas`
- **AND** the project's own mechanism is in `project`, and the control is in `merge`

#### Scenario: [MAP-2-S7] A module that is not enabled is not on the map
- **GIVEN** a repository with one enabled module and one module that ships but is not enabled
- **WHEN** the map is printed
- **THEN** it has a `module` node for the enabled module, and no node for the other module or
  any of its parts

### Requirement: Links from the readers that already parse them
The map SHALL link nodes by rel, each rel from its one source:

- `instance`: AGENTS.md;
- `executes`: the table's second column, *What executes it*;
- `names`: the skill and agent files;
- `registers`: the hook registrations;
- `models`: model-roles.json;
- `extends`: the enabled modules' and the local layer's parts (`lib/modules.sh`);
- `enforces`: each ADR's `## Enforcement`;
- `deps`: `roadmap_queue --tsv` column 15;
- `builds`: `changes/`.

A backticked span that holds whitespace SHALL be read as its first word after an optional `sh`,
`bash` or `node`, and a `:N` or `:N-M` suffix SHALL be stripped. The token is a reference only
when it is path-shaped, is `/<skill>` or `/<namespace>:<skill>`, or is exactly a skill's or an
agent's name.

#### Scenario: [MAP-3-S1] A short form resolves to one file
- **GIVEN** table cells naming `checks/merge-guard.sh`, `pr-merge-guard.sh` and `/verify-change`,
  and `run-local.sh`, which exists at `scripts/ci/run-local.sh` and at
  `template/scripts/ci/run-local.sh`
- **WHEN** the map is printed
- **THEN** their `executes` links reach `.claude/checks/merge-guard.sh`,
  `.claude/hooks/pr-merge-guard.sh`, `skill:verify-change` and `scripts/ci/run-local.sh` (the
  candidate with the fewest path segments)

#### Scenario: [MAP-3-S2] A word that names nothing is not a reference
- **GIVEN** a table cell holding `origin/main`, `tools:`, `BRANCH_*`, `lock-run` and `TestMain`,
  in a repository with no top-level entry named `origin` or `BRANCH_*`
- **WHEN** the map is printed, and when `--check` runs
- **THEN** none of them makes a link
- **AND** `--check` reports none of them

#### Scenario: [MAP-3-S3] A glob links to each match
- **GIVEN** a table cell naming `.claude/health/*.sh`, and three health lines in that directory
- **WHEN** the map is printed
- **THEN** that control has an `executes` link to each of the three

#### Scenario: [MAP-3-S4] Skills name, settings register, and roles model
- **GIVEN**:
  - a SKILL.md that names `.claude/owner-queue.sh`;
  - a settings.json that registers `.claude/hooks/pr-merge-guard.sh`;
  - a model-roles.json that gives the skill `propose` the role `planner`
- **WHEN** the map is printed
- **THEN** the map has these links:
  - a `names` link from that skill to `script:.claude/owner-queue.sh`;
  - a `registers` link from `config:.claude/settings.json` to the hook;
  - a `models` link from `config:.claude/model-roles.json` to `skill:propose`, whose note names
    `planner` with its model and effort

#### Scenario: [MAP-3-S5] ADRs enforce, and modules extend
- **GIVEN** an ADR whose `## Enforcement` names `checks/scenarios.sh`, and an enabled module
  `openspec` that adds a check
- **WHEN** the map is printed
- **THEN** the map has an `enforces` link from the ADR to `.claude/checks/scenarios.sh`, and an
  `extends` link from `module:openspec` to that module's check

#### Scenario: [MAP-3-S6] Rows depend on rows and build changes
- **GIVEN** a roadmap whose row B lists `A,#45` in its Deps (column 15). Row A has an open change
  with a delta for the capability `cap`, and no living spec has `cap`.
- **WHEN** the map is printed
- **THEN** the map has these links:
  - a `deps` link from `row:B` to `row:A`;
  - no link for `#45`;
  - a `builds` link from `row:A` to its change;
  - a `builds` link from that change to `spec:cap`, which is flagged `proposed`

#### Scenario: [MAP-3-S7] Everything holds to rule 4
- **GIVEN** an AGENTS.md with four rules and a table
- **WHEN** the map is printed
- **THEN** rules 1 to 3 and every control each have one `instance` link to `rule:4`
- **AND** `rule:4` has no `instance` link of its own

#### Scenario: [MAP-3-S8] A command or a line reference reads as its file
- **GIVEN** an ADR whose Enforcement names `.claude/scenario-trace.sh --check`, and a table cell
  naming `sh .claude/map.sh --check` and `run-local.sh:42`
- **WHEN** the map is printed, and when `--check` runs
- **THEN** they link to `.claude/scenario-trace.sh`, `.claude/map.sh` and
  `scripts/ci/run-local.sh`
- **AND** `--check` reports none of them

#### Scenario: [MAP-3-S9] A generic document name is not a reference
- **GIVEN** a table cell saying "`/propose` writes `design.md`" and naming `SKILL.md`, in a
  repository with many files of each name
- **WHEN** `--check` runs
- **THEN** `design.md` and `SKILL.md` make no link and are not reported, and `/propose` links to
  `skill:propose`

### Requirement: Dates from one pass over history
Each node that has its own path SHALL carry, as `changed`, the date of the last commit that
touched that path, read from a single `git log` call relative to the project root. A node with
no path of its own, an untracked or plugin file, every node in a repository with no commit, and
every node in a shallow clone SHALL have `changed` null. The map SHALL make at most four git
calls, whatever the number of nodes.

#### Scenario: [MAP-4-S1] A file and a directory are dated
- **GIVEN** a check last committed on 2026-10-01, and a skill directory whose files were last
  committed on 2026-10-05 and 2026-10-09
- **WHEN** the map is printed
- **THEN** the check's `changed` is 2026-10-01 and the skill's is 2026-10-09

#### Scenario: [MAP-4-S2] Nodes that share a file have no date
- **GIVEN** a commit that edits one line of AGENTS.md and one row of the roadmap
- **WHEN** the map is printed
- **THEN** every `rule`, `control` and `row` node has `changed` null

#### Scenario: [MAP-4-S3] Untracked files and shallow clones have no dates
- **GIVEN** a table cell naming an untracked file, and separately a shallow clone of a
  repository with the model installed
- **WHEN** the map is printed in each
- **THEN** the untracked file's node has `changed` null
- **AND** in the shallow clone every `changed` is null, and `notes` says the history is shallow

#### Scenario: [MAP-4-S4] Git is called a bounded number of times
- **GIVEN** a `git` first on PATH that records each subcommand it is called with, then runs the
  real git, and two repositories whose tables have 5 rows and 50 rows
- **WHEN** the map is printed in each
- **THEN** each run called git at most four times and `log` exactly once
- **AND** the two runs made the same number of git calls

### Requirement: Check that every reference resolves
`sh .claude/map.sh --check` SHALL exit non-zero when any reference does not resolve to exactly
one node, or the roadmap queue cannot be read. It SHALL name each failure as
`FILE[:LINE]: TOKEN: why`, with no line for a reference read through jq.
`.claude/checks/map.sh` SHALL run it on the project, so the gate runs it.

#### Scenario: [MAP-5-S1] A dangling path fails
- **GIVEN** a table cell naming `checks/gone.sh`, which does not exist
- **WHEN** `sh .claude/map.sh --check` runs
- **THEN** it exits 1, and its output names `AGENTS.md`, that row's line number and
  `checks/gone.sh`, and says it resolves to nothing

#### Scenario: [MAP-5-S2] An ambiguous short form fails
- **GIVEN** a table cell naming `x.sh`, which exists at `a/b/x.sh` and at `c/d/x.sh` but not
  under `.claude/`
- **WHEN** `--check` runs
- **THEN** it exits 1 and names both candidates

#### Scenario: [MAP-5-S3] Every source is checked
- **GIVEN**, one at a time:
  - an ADR whose Enforcement names a missing path;
  - a skill that names a missing `.claude/` script;
  - a settings.json that registers a missing hook;
  - a model-roles.json that maps a skill that does not exist
- **WHEN** `--check` runs on each
- **THEN** each one exits 1 and names the file and the token

#### Scenario: [MAP-5-S4] A roadmap that does not parse
- **GIVEN** a roadmap with a malformed queue row, and separately one whose Deps name a row id
  that is not in the roadmap
- **WHEN** `--check` runs on each, and the map is printed
- **THEN** `--check` exits 1 with the parser's error
- **AND** the printed map is still valid schema v1, with no `row` nodes, and layer 5's note
  carries the parser's first error line

#### Scenario: [MAP-5-S5] The gate runs it on the project
- **GIVEN** this repository at the change's head commit
- **WHEN** `sh .claude/checks/run.sh map` runs
- **THEN** it passes, and its `ok` line names how many nodes and links it resolved and the
  sources it read

#### Scenario: [MAP-5-S6] A plugin project resolves the model's names in the plugin
- **GIVEN** a project that runs the model as a plugin, with:
  - an AGENTS.md in the shape `scripts/build-plugin.sh` writes, naming
    `${CLAUDE_PLUGIN_ROOT}/hooks/pr-merge-guard.sh`, `checks/merge-guard.sh` and
    `/clauductor:propose`;
  - the plugin's `hooks/hooks.json` registering
    `sh "${CLAUDE_PLUGIN_ROOT}/hooks/if-project.sh" pr-merge-guard.sh`;
  - each of those files only under `CLAUDUCTOR_FW`
- **WHEN** `--check` runs, and the map is printed
- **THEN** `--check` reports none of them
- **AND** the links reach `hook:${CLAUDE_PLUGIN_ROOT}/hooks/pr-merge-guard.sh`,
  `check:${CLAUDE_PLUGIN_ROOT}/checks/merge-guard.sh` and the plugin's `propose` skill
- **AND** a `registers` link runs from the plugin's `hooks/hooks.json` to the merge guard

#### Scenario: [MAP-5-S7] A fresh install passes before its first commit
- **GIVEN** a repository where the model was just installed and nothing is committed yet
- **WHEN** `--check` runs
- **THEN** it exits 0
- **AND** the printed map has `commit` null and every `changed` null

#### Scenario: [MAP-5-S8] A glob that matches nothing fails
- **GIVEN** a table cell naming `.claude/health/*.sh`, and no file in that directory
- **WHEN** `--check` runs
- **THEN** it exits 1 and names the glob as resolving to nothing

#### Scenario: [MAP-5-S9] A missing table or missing rules fail
- **GIVEN** an AGENTS.md with no *What executes each rule* table, and separately one with no
  `**N. …**` rules
- **WHEN** the map is printed, and when `--check` runs, on each
- **THEN** the empty layer has a note saying what was looked for
- **AND** `--check` exits 1

### Requirement: A map that map.sh did not make
Schema v1 SHALL admit a map made by another producer, such as the panel for an attached
repository, and `sh .claude/map.sh --validate FILE` SHALL hold any map to the schema.

- `source` is any non-empty string.
- A layer with no nodes MUST carry a non-null `note` that says why it is empty.

#### Scenario: [MAP-6-S1] A panel-built map validates
- **GIVEN** a map with:
  - `source` "panel";
  - `skill`, `agent`, `hook`, `config`, `lane` and `worktree` nodes;
  - `registers` and `in` links;
  - layers 0, 1 and 4 empty, each with a note saying the repository has no Clauductor model
- **WHEN** `sh .claude/map.sh --validate` runs on it
- **THEN** it exits 0

#### Scenario: [MAP-6-S2] A map that breaks the schema is refused
- **GIVEN**, one at a time:
  - that map with layer 0's note set to null;
  - a map with a node of an unknown kind;
  - a map with a link to a missing node id;
  - a map with an unknown top-level key;
  - a map with two nodes of one id, as two table rows with the same first cell would make
- **WHEN** `--validate` runs on each
- **THEN** each one exits 1 and names the fault
