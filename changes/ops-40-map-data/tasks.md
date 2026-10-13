# Tasks: ops-40-map-data

## Progress
- 2026-10-13 proposed; nothing built yet

## Decision log
- (none yet)

## 1. The shared readers and the map's skeleton
- [ ] 1.1 Add `template/.claude/lib/refs.sh` with `refs_named_paths`, `refs_hook_registrations` and `refs_table_rows`, each moved unchanged from `checks/skills.sh`, `checks/hooks.sh` and `checks/agents-md-budget.sh` (D11). Before deleting the old copies, compare the old and new outputs on this repository in `template/.claude/checks/map.sh`: the same named paths, the same `event TAB command` registrations and the same table rows.
- [ ] 1.2 Switch `checks/skills.sh`, `checks/hooks.sh` and `checks/agents-md-budget.sh` to the shared readers. Each check still passes on this repository. For each, a falsifying edit to its fixture still fails it for the same reason as before the swap. Run by hand and recorded in the Decision log.
- [ ] 1.3 Add `template/.claude/map.sh`'s three modes. `--validate` holds schema v1 (keys, kinds, layers, rels, flags, link ends and empty-layer notes), tested in `template/.claude/checks/map.sh` citing [MAP-6-S1] and [MAP-6-S2] against a panel-built fixture written inline.
- [ ] 1.4 Print the skeleton map: schema v1's top level, the layers, kinds and areas, rule and control nodes with D2's ids, and the `instance` links. Tested in `template/.claude/checks/map.sh` against a throwaway repository, citing [MAP-1-S1], [MAP-1-S2], [MAP-1-S3], [MAP-2-S1] and [MAP-3-S7].

## 2. Mechanisms, skills, agents and their links
- [ ] 2.1 Classify and resolve references as D5 orders it, including globs, ties and the plugin root, and give each node its kind by path (D3). Tested in `template/.claude/checks/map.sh` citing [MAP-2-S2], [MAP-3-S1], [MAP-3-S2] and [MAP-3-S3].
- [ ] 2.2 Build the `executes` links and the `unexecuted` and `by-hand` flags. Tested citing [MAP-2-S5].
- [ ] 2.3 Add skill and agent nodes, the project's and the plugin's, and the `names`, `registers` and `models` links. Tested citing [MAP-2-S3] and [MAP-3-S4], with `CLAUDUCTOR_FW` pointed at a fixture plugin root.
- [ ] 2.4 Add ADR nodes and `enforces` links, and module nodes (the enabled modules and `local`) with `extends` links from `extensions.sh list`. Tested citing [MAP-3-S5].

## 3. Rows, changes, specs, dates and areas
- [ ] 3.1 Add row nodes from `roadmap_queue --tsv`, flagged by state, with `deps` from column 15 as OPS-38 merged it (D10), and the per-layer degradation when the queue cannot be read (D12). Tested in `template/.claude/checks/map.sh` citing [MAP-3-S6] and [MAP-5-S4]. Waits for OPS-38 on `main`. If its column 15 differs from D10's reading, stop and take the delta back to the owner.
- [ ] 3.2 Add change and spec nodes (open and archived, flagged) and the `builds` links. Tested citing [MAP-2-S4].
- [ ] 3.3 Fill `changed` from one `git log` pass (D8): null for shared-file, untracked and plugin nodes, and null everywhere in a shallow clone, with a note. Tested citing [MAP-4-S1], [MAP-4-S2], [MAP-4-S3] and [MAP-4-S4]; S4 uses a counting `git` shim first on PATH.
- [ ] 3.4 Fill `area` from D7's table and the derived areas of controls and ADRs. Tested citing [MAP-2-S6].

## 4. `--check` in the gate, and the record
- [ ] 4.1 Make `--check` fail every unresolved or ambiguous reference from every source, as `FILE:LINE: TOKEN: why`. Tested in `template/.claude/checks/map.sh` citing [MAP-5-S1], [MAP-5-S2], [MAP-5-S3] and [MAP-5-S6].
- [ ] 4.2 Make `checks/map.sh` run `map.sh --check` and `--validate` on the project, with a green line that names its subject (node and link counts, and the sources read). Tested by the gate on this repository, citing [MAP-5-S5]. Confirm that it starts green here, and fix any reference it finds dangling in the same group.
- [ ] 4.3 Add the AGENTS.md row (D15) to `template/AGENTS.md` and to this repository's `AGENTS.md`, and add *The map* paragraph to `template/docs/playbook.md`. `sh .claude/checks/run.sh agents-md-budget skills` passes.
- [ ] 4.4 Run `scripts/build-plugin.sh`, then `framework/clauductor update` from the root with a binary built from this checkout. `sh .claude/checks/run.sh` and `sh template/.claude/checks/run.sh` pass, and `cd framework && go test -short ./internal/plugin/...` passes with a fake `claude` first on PATH (`TestCommittedPluginIsCurrent`).

- [ ] Slice: a project owner can run `sh .claude/map.sh` to get their operating model as a graph, and the gate fails a *What executes* cell or ADR Enforcement path that names nothing
