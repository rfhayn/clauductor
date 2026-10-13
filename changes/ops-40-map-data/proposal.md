**Status:** awaiting approval
**Roadmap row:** OPS-40
**Risk:** normal

## Why

AGENTS.md's *What executes each rule* table is a graph: rule to control to mechanism. Today it
is read only as text. Its "Nothing. You." rows, the controls that no script holds, are easy to
miss, and so is a cell that names a file that no longer exists. Nothing checks that the paths in
a cell, or in an ADR's `## Enforcement` section, point anywhere. `checks/skills.sh` checks only
full `.claude/…` paths, and the table mostly uses short forms (`checks/merge-guard.sh`,
`pr-merge-guard.sh`, `/verify-change`). A short form that names a deleted check reads as a control
and fails nothing. That is rule 4's failure mode exactly: it degrades to silence.

PRD-maps (decided 2026-10-12) gives the panel a Map view (PANEL-35, then PANEL-36). That view
needs data, and every MAPS view after it (Today, the routines table, the skills deck) sits
beside the graph. This row is the chain's root: `.claude/map.sh` prints the operating model as a
graph, and the gate fails a reference that resolves to nothing. That gives the generator a
production caller before any panel draws it (rule 3).

**Step 0a.** Proposed changes sit ahead of this one: PANEL-25, PANEL-28, PANEL-30 and PANEL-31 in
`changes/`, and OPS-42 and PANEL-42 on branches. PRD-maps, *The standing answer to `/propose` step
0a*, covers each of them: OPS-42 is a MAPS row, PANEL-42 is named there, and the panel rows wait
behind MAPS. This proposal cites that answer and continues.

## What changes

A project can:

- **print its operating model as a graph** with `sh .claude/map.sh` (`--json` is the default).
  The output is one JSON object of schema v1 on stdout, and nothing is written or committed. It
  holds:
  - **the nodes:** the four rules, every row of the *What executes* table, the hooks, checks,
    scripts, health lines, the workflow, the modules and config files those rows reach, the
    skills, the agents, the ADRs and specs, every roadmap row and every change;
  - **the links between them:** `executes`, `names`, `registers`, `enforces`, `models`,
    `extends`, `deps`, `builds`, and `instance` (D4);
  - **for each node:** an area, a ring (layer) and the date it last changed.
- **see which controls nothing executes.** A control whose cell names no mechanism carries the
  flag `unexecuted`. One that names a mechanism and also says "Nothing. You." carries `by-hand`
  (D6).
- **have the gate fail a dangling reference.** `sh .claude/map.sh --check` names the file (and
  the line, where there is one) and the token of each reference that resolves to nothing, or to
  two files at once:
  - in a *What executes* cell or an ADR's Enforcement section;
  - a path a skill or agent names;
  - a hook settings.json registers;
  - a skill or agent model-roles.json assigns;
  - a module part.

  A row's Deps are the roadmap parser's to hold (OPS-38). `--check` fails when the parser
  refuses the queue (D10). `checks/map.sh` runs it in every gate, because the checks directory
  is the list. That includes a plugin project's gate and a fresh install before its first
  commit (D5).
- **validate a map it did not make** with `sh .claude/map.sh --validate FILE`. An attached
  repository has no `map.sh`, so the panel will build a map from what it sees (PANEL-35). Schema
  v1 admits that map: it names its `source`, and an empty layer says why it is empty (D13).

This repository's AGENTS.md table gains one row, and so does the template's (D15):
`| Every path the what-executes table and an ADR's Enforcement name resolves | **`.claude/map.sh --check`** via **`checks/map.sh`** (the gate) |`.

## What the existing specs already guarantee

`specs/` holds one living spec, `panel-new-lane`, which this change does not touch. No
requirement covers the table, the ADRs' Enforcement sections or the checks that read them, so
nothing here modifies or contradicts a requirement. This change **adds** the capability `map`
(`specs/map/spec.md`, requirements MAP-1 to MAP-6).

It builds on what is documented but not specified:

- **The roadmap queue's `--tsv` contract.** `lib/conf.sh` `roadmap_tsv_errors` checks it, and
  docs/roadmap.md *Plugging in your own parser* states it. `deps` reads its column 15, which
  OPS-38 adds (D10).
- **The readers that already parse the sources:**
  - the named-path pattern in `checks/skills.sh`;
  - the hook-registration query in `checks/hooks.sh`;
  - the table anchor in `checks/agents-md-budget.sh`;
  - `ext_files` and `ext_dirs` in `lib/modules.sh`, as `checks/run.sh` reads them;
  - `clauductor update`'s reading of table rows, which keys each row by its first cell and
    suggests a missing one (`framework/internal/template/projectfiles.go`).

  D11 moves the first three into one shared file instead of copying them. A new
  `checks/refs.sh` holds the three checks' behaviour across the move and after it.
- **ADR-0006.** `map.sh` is POSIX sh, git and jq, and needs neither the binary nor the panel.
  `checks/no-clauductor.sh` already runs every check with `clauductor` off PATH.
- **ADR-0007.** The plugin's copy finds the model's files through `CLAUDUCTOR_FW`, as
  `metrics.sh` and `roadmap-queue.sh` do.

**Scope, re-measured under step 0c.** The row names two files. The change writes:

- **four new files:** `map.sh`, `checks/map.sh`, `lib/refs.sh`, `checks/refs.sh`;
- **three checks, each a one-line swap to the shared reader:** `checks/skills.sh`,
  `checks/hooks.sh`, `checks/agents-md-budget.sh`;
- **one table row** in each AGENTS.md;
- **a paragraph** in the playbook.

That is over the row's two files. D1 weighs a split. The PR also revises two roadmap rows, each
to be revised again to the owner's decisions:
- OPS-40's scope gains the files above (D1, D11, D15);
- OPS-46's row gains "`map.sh` reads the records as `prac` nodes", and its Deps gain OPS-40
  (D3).

## Out of scope

- **Drawing the map:**
  - the view, the ADR-0009 colour exception and the panel's `map.command`: PANEL-35;
  - the areas and timeline layouts and the legend: PANEL-36.
- **Building a map for an attached repository in Go:** PANEL-35, against the schema and the
  fixture this change ships (D13).
- **Practice records:** OPS-46 creates them. The kind `prac` is in schema v1 now, and OPS-46's
  row gains the reading of them (D3).
- **Marking the critical path on rows:** OPS-39 derives it, and PANEL-38's *Next session* reads
  it from the queue.
- **Go test names in an Enforcement section** (`TestCommittedPluginIsCurrent`): they are not
  path-shaped, so they are not checked (D5, *Refusals*). No row owns resolving them; nothing
  waits on it.
- **Committing the map:** never. A committed map would conflict on every squash and go stale.

## How we'll know

- **Signal:** PANEL-35 draws `map.sh`'s output without needing a change to `map.sh`'s schema
  (no follow-up fix to OPS-40). `checks/map.sh` stays green on `main`. Every red it gives on a
  PR names a reference that was really dangling.
- **Check after:** 21 days
