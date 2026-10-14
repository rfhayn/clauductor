# Tasks: ops-40-map-data

## Progress
- 2026-10-13 proposed; nothing built yet
- 2026-10-13 revised after the proposal's review (4 high, 1 medium, 10 low); still awaiting approval
- 2026-10-13 group 1 built (1.1–1.4), uncommitted for review; group 2 is next. `map.sh` prints rules, controls and `instance` links only; layers 2–5 carry the fallback note until groups 2–3 fill them
- 2026-10-14 group 1 (The shared readers and the map's skeleton) built and reviewed: converged in 3 round(s), peak medium (rounds: medium, medium, low)

## Decision log
- 2026-10-13, group 1: `refs_project_dir_prefix` prints the ERE rather than testing a path. The alternative was a predicate. An ERE lets `hooks.sh` test with `grep -E` and lets 2.3's `registers` strip the prefix with `sed -E`, from one copy.
- 2026-10-13, group 1: `refs_named_paths` always matches `${CLAUDE_PLUGIN_ROOT}/…` as well as `.claude/…`. The alternative was to detect a plugin project first. The template never spells `${CLAUDE_PLUGIN_ROOT}`, and `skills.sh` keeps only `.claude/` paths (`cut -f3 | grep '^\.claude/'`), so its set is unchanged. Checked identical, old reader against new, on this repository, `template/` and `plugin/scaffold/`.
- 2026-10-13, group 1: `hooks.sh` calls `refs_hook_scripts` at its three call sites, and reads the prefix once (`pdp`). The alternative was to keep a local `scripts()` wrapper. A wrapper would be a second name for one reader.
- 2026-10-13, group 1: `checks/refs.sh` copies only what each check reads into its fixture: `lib/`, `checks/lib.sh` and the check, plus `hooks/` and `status-write.sh` for `hooks.sh`. The alternative was to copy the whole `.claude/`. Explicit `"$ROOT/.claude/X"` copies are what the plugin build rewrites to `$CLAUDUCTOR_FW/X`, so a plugin project gets the same fixture. The `hooks.sh` case asserts its parse FAIL line, not that it is the only FAIL; today it is the only one.
- 2026-10-13, group 1, **for the owner**: `--validate` requires exactly the six layers 0–5 in order, and exactly schema v1's 19 kinds, each at its layer. Labels may be any non-empty string. Areas belong to the producer: any unique non-empty ids, and each node's area must be one of them. Not enforced: the `<kind>:` id prefix, node and link order (MAP-1-S3 tests that on map.sh's own output), and which kinds a rel joins. The alternative was to accept a subset of kinds. That would let a panel-built map ship an incomplete legend. PANEL-35's producer therefore has to emit all 19 kinds.
- 2026-10-13, group 1: `--validate` exits 2 when FILE cannot be read. A file that is not JSON, or that holds several JSON values, is a fault (exit 1). The alternative was exit 1 for an unreadable file too. Exit 2 keeps "no answer" apart from "an invalid map", as `CANNOT MAP` does.
- 2026-10-13, group 1: the map prints as compact JSON (`jq -c`). The alternative was pretty-printed JSON, about twice the bytes against the panel's 1 MB bound. A reader can pipe it through `jq`.
- 2026-10-13, group 1: labels and paths. A rule's label is its bold sentence without the `N. `. A control's label is its trimmed first cell with `**` removed and backticks kept. Rules and controls have `path` and `note` null, because they share AGENTS.md and D8 dates them null anyway. The alternative was `path` "AGENTS.md", which would read as a node with a file of its own.
- 2026-10-13, group 1: the `cksum` input is the trimmed first cell, with no trailing newline. The cell is cut the way `agentsRows` cuts it: drop the leading `|`s, end at the next `|`, trim spaces and tabs. The alternative was to hash the cell with a newline. That would hash a byte that is not in the key.
- 2026-10-13, group 1: a layer with no nodes and no specific note gets "no nodes: nothing map.sh read reaches this layer". The alternative was a per-group placeholder. This one keeps map.sh's own output valid now, and stays true later wherever a group sets no note of its own.
- 2026-10-13, group 1, **for the owner**: every rule other than `rule:4`, and every control, links `instance` to `rule:4`, and only when `rule:4` exists. The design's links table says "rules 1–3"; this reading also links a project's own fifth rule, which AGENTS.md does not call an instance. The alternative was to link rules 1–3 by number. A missing rule 4 would then leave dangling link ends.
- 2026-10-13, group 1: in group 1, `--check` fails a missing AGENTS.md, missing rules, a missing table, two rules with one number, and two controls with one id. On success it prints one line naming the node and link counts and what it read, for 4.2's green line. On a duplicate id, `--json` still prints both nodes, so `--validate` names the id, as D2 says.
- 2026-10-13, group 1: the jq test comes before argument parsing, so any run without jq says `CANNOT MAP`. A `MAP_NOW` that is not whole seconds exits 2.
- 2026-10-13, group 1 (review round 1): MAP-1-S1's throwaway installs the model the way this project has it. It copies `.claude/` (skipping `worktrees` and `settings.local.json`), the instructions, `.gitattributes`, `.gitignore`, `.clauductor`, `.github`, `docs`, `changes`, `specs` and `scripts`. In a plugin project, `CLAUDUCTOR_FW`'s files (except `scaffold/`) go in first and the project's own files over them. A case asserts that the result holds the config, the hooks, skills, agents, checks and the roadmap. The alternative was to run `clauductor install`, which a check must not need (ADR-0006, `checks/no-clauductor.sh`). `TestPluginChecksPassInScaffoldedProject` passes with the fixture in place.
- 2026-10-14, group 1 (review round 2): MAP-1-S1 counts what the copied AGENTS.md holds, without map.sh's readers: its `**N. ` rules, its table rows, and so its instance links (each other rule and each control, when there is a rule 4). The alternative was a fixed 4 rules, which would turn a project's gate red the day it adds a rule of its own. A five-rule AGENTS.md case holds this.
- 2026-10-14, group 1 (review round 2): a table row with an empty first cell makes no control, as `agentsRows` skips it. Without this, two such rows would collide on `control:ffffffff`. `refs_table_rows` still returns every row byte for byte, so `agents-md-budget.sh` measures what it always did. Only map.sh skips them. The alternative was to fail an empty first cell in `--check`. The table's own reader in Go accepts the row, so the map follows it.
- 2026-10-14, group 1 (review round 2): any `**N. ` line in AGENTS.md is a rule, wherever it stands. The alternative was to read rules only above the *What executes* heading. The spec defines a rule as a `**N. …**` line in AGENTS.md, and narrowing that is the owner's call. A stray numbered bold line is loud rather than silent: it shows on the map, and a repeated number fails `--check`.
- 2026-10-14, group 1 (review round 2): `MAP_NOW` is checked only when the map is made, and `--validate` needs a regular readable file, else exit 2. The alternative was to check `MAP_NOW` for every mode. `--validate` never reads it.
- 2026-10-13, group 1: area and kind labels. D7's area ids get the labels Rules, Changes, Merging, Roadmap, Sessions, Models, Safety, Records, Modules and Project. The kind labels are in `SCHEMA_JQ`. Rules and controls are in `rules` until 3.4 derives a control's area. The alternative was to wait for 3.4, but `areas` must be complete for `--validate` now.

## 1. The shared readers and the map's skeleton
- [x] 1.1 Write `template/.claude/checks/refs.sh` first, against the current checks, and see it pass (D11). Each case runs a check with `ROOT=<fixture>`:
  - `checks/skills.sh` fails "named but missing" for a missing `.claude/` path named in each input kind: a SKILL.md (`.sh`), an agent file (`.js`), AGENTS.md (`.awk`) and settings.json (`.json`). It passes once each path exists, and passes when only `.claude/settings.local.json` is named and absent;
  - `checks/hooks.sh` fails when settings.json does not parse (its own non-vacuity line, expectation table and `scripts()` table keep holding the rest);
  - `checks/agents-md-budget.sh` passes a 320-character row, fails a 321-character one, and fails when the table is missing.
- [x] 1.2 Add `template/.claude/lib/refs.sh` (D11) and switch `checks/skills.sh`, `checks/hooks.sh` and `checks/agents-md-budget.sh` to it. `checks/refs.sh` passes unchanged, and each of the three checks still passes on this repository. The functions:
  - `refs_named_paths`: `FILE TAB LINE TAB PATH`, with the `settings.local.json` exclusion inside it, and `${CLAUDE_PLUGIN_ROOT}/…` matched in a plugin project;
  - `refs_hook_registrations`: `EVENT TAB COMMAND`;
  - `refs_hook_scripts`: `checks/hooks.sh`'s `scripts()`, moved; its test table stays in `hooks.sh` and runs against the moved function;
  - `refs_project_dir_prefix`: `hooks.sh`'s project-root pattern, moved; `hooks.sh`'s relative-path test uses it, with its cases staying in `hooks.sh`;
  - `refs_table_rows`: `LINE TAB ROW`, with ROW byte for byte the line.
- [x] 1.3 Add `template/.claude/map.sh`'s three modes. `--validate` holds schema v1: keys, kinds, layers, rels, flags, unique ids, link ends and empty-layer notes. Tested in `template/.claude/checks/map.sh` citing [MAP-6-S1] and [MAP-6-S2], against a panel-built fixture written inline. The jq-missing exit is tested citing [MAP-1-S4].
- [x] 1.4 Print the skeleton map: schema v1's top level, the layers, kinds and areas, rule and control nodes with D2's ids, and the `instance` links. For the ids, one `cksum` call reads a `mktemp -d` scratch directory holding one file per cell, removed on exit. `map.sh` sets `ROOT` before it sources `lib/conf.sh`. Tested in `template/.claude/checks/map.sh` against a throwaway repository, citing [MAP-1-S1], [MAP-1-S2], [MAP-1-S3], [MAP-2-S1], [MAP-3-S7] and [MAP-5-S9].

## 2. Mechanisms, skills, agents and their links
- [ ] 2.1 Normalise, classify and resolve references as D5 says: commands, line suffixes, `.md` names, globs, ties, `git ls-files -co --exclude-standard` relative to `ROOT`, and the plugin root without its `scaffold/`. Give each node its kind by path (D3). Tested in `template/.claude/checks/map.sh` citing [MAP-2-S2], [MAP-3-S1], [MAP-3-S2], [MAP-3-S3], [MAP-3-S8] and [MAP-3-S9].
- [ ] 2.2 Build the `executes` links and the `unexecuted` and `by-hand` flags. Tested citing [MAP-2-S5].
- [ ] 2.3 Add skill and agent nodes, the project's and the plugin's, and the `names`, `registers` and `models` links. `registers` reads each command through `refs_hook_scripts` and `refs_project_dir_prefix`: every project-root form `hooks.sh` accepts, `${CLAUDE_PLUGIN_ROOT}/` after a leading quote, the plugin's `if-project.sh` argument, and no link for an absolute path. `names` matches `${CLAUDE_PLUGIN_ROOT}/…` in a plugin project. Tested citing [MAP-2-S3] and [MAP-3-S4], with `CLAUDUCTOR_FW` pointed at a fixture plugin root.
- [ ] 2.4 Add ADR nodes and `enforces` links, and module nodes (the enabled modules and `local`) with `extends` links from `ext_files` and `ext_dirs` over `MODULE_POINTS`. Tested citing [MAP-3-S5] and [MAP-2-S7].

## 3. Rows, changes, specs, dates and areas
- [ ] 3.1 Add row nodes from `roadmap_queue --tsv`, flagged by state, with `deps` from column 15 as OPS-38 merged it (D10), and the per-layer degradation when the queue cannot be read (D12). Tested in `template/.claude/checks/map.sh` citing [MAP-3-S6] and [MAP-5-S4]. This task waits for OPS-38 on `main`. If its column 15 differs from D10's reading, stop and take the delta back to the owner.
- [ ] 3.2 Add change and spec nodes, with the `builds` links (D9):
  - open and archived changes, flagged;
  - living specs, plus each capability an open change has a delta for, flagged `proposed`;
  - an archived change linking only to living specs.

  Tested citing [MAP-2-S4] and [MAP-3-S6] (the `builds` links and the `proposed` spec).
- [ ] 3.3 Fill `changed` from one `git log --relative` pass run from `ROOT` (D8):
  - null for shared-file, untracked and plugin nodes;
  - null everywhere with no commit, or in a shallow clone, with a note;
  - at most four git calls in all.

  Tested citing [MAP-4-S1], [MAP-4-S2], [MAP-4-S3] and [MAP-4-S4]. S4 uses a counting `git` shim first on PATH.
- [ ] 3.4 Fill `area` from D7's table and the derived areas of controls and ADRs. Tested citing [MAP-2-S6].

## 4. `--check` in the gate, and the record
- [ ] 4.1 Make `--check` fail every unresolved or ambiguous reference from every source, and a queue the parser refuses, as `FILE[:LINE]: TOKEN: why`. Tested in `template/.claude/checks/map.sh` citing [MAP-5-S1], [MAP-5-S2], [MAP-5-S3], [MAP-5-S6] (a fixture AGENTS.md in `plugin/scaffold/AGENTS.md`'s rewritten shape, a `hooks.json` using `if-project.sh`, and a fake plugin root that holds `scaffold/`), [MAP-5-S7] and [MAP-5-S8].
- [ ] 4.2 Make `checks/map.sh` run `map.sh --check` and `--validate` on the project, with a green line that names its subject (node and link counts, and the sources read). Tested by the gate on this repository, citing [MAP-5-S5]. If it is red here:
  - fix a dangling reference in AGENTS.md's table in this group;
  - **never edit an ADR**. A dangling ADR reference is the owner's: stop and report it.
- [ ] 4.3 Add the AGENTS.md row (D15) to `template/AGENTS.md` and to this repository's `AGENTS.md`, and add *The map* paragraph to `template/docs/playbook.md`. `sh .claude/checks/run.sh agents-md-budget skills map` passes.
- [ ] 4.4 Bring the copies along and run the checks:
  - run `scripts/build-plugin.sh`, then `framework/clauductor update` from the root with a binary built from this checkout;
  - `sh .claude/checks/run.sh` and `sh template/.claude/checks/run.sh` pass;
  - `cd framework && go test -short ./internal/plugin/...` passes with a fake `claude` first on PATH (`TestCommittedPluginIsCurrent`, and the scaffolded plugin gate running `checks/map.sh`).

- [ ] Slice: a project owner can run `sh .claude/map.sh` to get their operating model as a graph, and the gate fails a *What executes* cell or ADR Enforcement path that names nothing
