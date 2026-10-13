# Tasks: ops-40-map-data

## Progress
- 2026-10-13 proposed; nothing built yet
- 2026-10-13 revised after the proposal's review (4 high, 1 medium, 10 low); still awaiting approval

## Decision log
- (none yet)

## 1. The shared readers and the map's skeleton
- [ ] 1.1 Write `template/.claude/checks/refs.sh` first, against the current checks, and see it pass (D11). Each case runs a check with `ROOT=<fixture>`:
  - `checks/skills.sh` fails "named but missing" for a missing `.claude/` path named in each input kind: a SKILL.md (`.sh`), an agent file (`.js`), AGENTS.md (`.awk`) and settings.json (`.json`). It passes once each path exists, and passes when only `.claude/settings.local.json` is named and absent;
  - `checks/hooks.sh` fails when settings.json does not parse (its own non-vacuity line, expectation table and `scripts()` table keep holding the rest);
  - `checks/agents-md-budget.sh` passes a 320-character row, fails a 321-character one, and fails when the table is missing.
- [ ] 1.2 Add `template/.claude/lib/refs.sh` (D11) and switch `checks/skills.sh`, `checks/hooks.sh` and `checks/agents-md-budget.sh` to it. `checks/refs.sh` passes unchanged, and each of the three checks still passes on this repository. The functions:
  - `refs_named_paths`: `FILE TAB LINE TAB PATH`, with the `settings.local.json` exclusion inside it, and `${CLAUDE_PLUGIN_ROOT}/…` matched in a plugin project;
  - `refs_hook_registrations`: `EVENT TAB COMMAND`;
  - `refs_hook_scripts`: `checks/hooks.sh`'s `scripts()`, its table in `hooks.sh` running against the moved function;
  - `refs_table_rows`: `LINE TAB ROW`, with ROW byte for byte the line.
- [ ] 1.3 Add `template/.claude/map.sh`'s three modes. `--validate` holds schema v1: keys, kinds, layers, rels, flags, unique ids, link ends and empty-layer notes. Tested in `template/.claude/checks/map.sh` citing [MAP-6-S1] and [MAP-6-S2], against a panel-built fixture written inline. The jq-missing exit is tested citing [MAP-1-S4].
- [ ] 1.4 Print the skeleton map: schema v1's top level, the layers, kinds and areas, rule and control nodes with D2's ids, and the `instance` links. For the ids, one `cksum` call reads a `mktemp -d` scratch directory holding one file per cell, removed on exit. `map.sh` sets `ROOT` before it sources `lib/conf.sh`. Tested in `template/.claude/checks/map.sh` against a throwaway repository, citing [MAP-1-S1], [MAP-1-S2], [MAP-1-S3], [MAP-2-S1], [MAP-3-S7] and [MAP-5-S9].

## 2. Mechanisms, skills, agents and their links
- [ ] 2.1 Normalise, classify and resolve references as D5 says: commands, line suffixes, `.md` names, globs, ties, `git ls-files -co --exclude-standard` relative to `ROOT`, and the plugin root without its `scaffold/`. Give each node its kind by path (D3). Tested in `template/.claude/checks/map.sh` citing [MAP-2-S2], [MAP-3-S1], [MAP-3-S2], [MAP-3-S3], [MAP-3-S8] and [MAP-3-S9].
- [ ] 2.2 Build the `executes` links and the `unexecuted` and `by-hand` flags. Tested citing [MAP-2-S5].
- [ ] 2.3 Add skill and agent nodes, the project's and the plugin's, and the `names`, `registers` and `models` links. `registers` reads each command through `refs_hook_scripts`, with the project-dir and plugin-root prefixes and the plugin's `if-project.sh` argument. Tested citing [MAP-2-S3] and [MAP-3-S4], with `CLAUDUCTOR_FW` pointed at a fixture plugin root.
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
