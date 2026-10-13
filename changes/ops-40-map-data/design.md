# Design: ops-40-map-data

## Where the behaviour lives today

- **The table is read in three places, none of which follows its references:**
  - `checks/agents-md-budget.sh` finds it under `### What executes each rule` and measures its
    rows;
  - `clauductor update` merges its rows by first cell (`projectfiles.go` `agentsRows`);
  - `checks/skills.sh` greps full `.claude/…\.(sh|js|awk|json)` paths out of AGENTS.md, the
    skills, the agents and settings.json, and fails a missing one.

  Short forms (`checks/merge-guard.sh`, `pr-merge-guard.sh`, `run-local.sh`, `/verify-change`)
  are never resolved. Paths outside `.claude/` (`scripts/ci/run-local.sh`, `.github/…`) are
  never checked.
- **ADR Enforcement sections are prose.** `checks/adr-numbering.sh` checks that each ADR has
  one, not that what it names exists.
- **The other link sources already parse:**
  - hook registrations: `checks/hooks.sh`'s jq over `.hooks`;
  - the role per skill and agent: `model-roles.json` `.skills`, `.agents` and `.roles`;
  - module parts: `extensions.sh list`;
  - rows: `roadmap_queue --tsv`, the one parser;
  - a row's change: `changes/<id>/` or `changes/archive/<date>-<id>/`;
  - a change's spec deltas: `changes/<id>/specs/<cap>/`.
- **The panel runs a project's metrics command** as trusted argv, with a 30-second timeout and
  1 MB of output (`docs/panel.md`, `metrics.command`). PANEL-35 will run `map.sh` the same way,
  so the output must fit those bounds. This repository's map is an estimated 260 nodes, around
  100 KB.
- **A rough prototype run on 2026-10-13** applied D5's resolution order, by hand, to this
  repository's table and its eight ADRs' Enforcement sections. It found a file for every
  path-shaped token, with the ties D5 breaks, so the new check should start green here. The
  builder confirms this; it is a claim until then.

## The shape of the change

1. **`.claude/lib/refs.sh`** (D11): the shared readers.
   - `refs_named_paths FILE…`: `checks/skills.sh`'s pattern, moved here unchanged.
   - `refs_hook_registrations SETTINGS`: `checks/hooks.sh`'s jq, as `event TAB command` lines,
     moved here unchanged.
   - `refs_table_rows AGENTS_MD`: the rows under `### What executes each rule`, with their line
     numbers. This is `agents-md-budget.sh`'s anchor.

   Each of those three checks calls its function instead of its own copy.
2. **`.claude/map.sh`** (POSIX sh, git, jq) has three modes:
   - `--json` (the default): prints the map;
   - `--check`: exits 1 naming each unresolved reference as `FILE:LINE: TOKEN: why`;
   - `--validate FILE`: holds a map from any source to schema v1, exiting 1 naming each fault.

   It reads only tracked files and git, and writes nothing. Test seams are `MAP_NOW` (unix
   seconds) and `PATH` (git).
3. **`.claude/checks/map.sh`** does two things:
   - runs `map.sh --check` on the project, and validates its `--json` output with
     `--validate`. The green line names its subject: the counts of nodes and links, and the
     sources read;
   - holds every MAP scenario against throwaway repositories, a fake plugin root, and a
     panel-built fixture written inline.
4. **The record:**
   - one AGENTS.md row, in the template and in this repository (D15);
   - `template/docs/playbook.md` gains a short *The map* paragraph under *What runs without
     being asked*;
   - `scripts/build-plugin.sh` and `clauductor update` bring the plugin and this repository's
     copy along.

### Schema v1

The spec states it as requirements, and `map.sh --validate` is its executable form (D14).

```json
{
  "version": 1,
  "source": "map.sh",
  "generated": "2026-10-13T12:00:00Z",
  "commit": "<HEAD's full sha, or null>",
  "notes": ["<a whole-map note, e.g. a shallow clone>"],
  "layers": [{"n": 0, "label": "Rules", "note": null}, "… 0 to 5"],
  "kinds":  [{"id": "rule", "label": "Rule", "layer": 0}, "…"],
  "areas":  [{"id": "merge", "label": "Merging"}, "…"],
  "nodes":  [{"id": "check:.claude/checks/merge-guard.sh", "kind": "check",
              "label": "merge-guard.sh", "area": "merge", "layer": 2,
              "path": ".claude/checks/merge-guard.sh", "note": null,
              "changed": "2026-10-12", "flags": []}],
  "links":  [{"source": "control:3f9a1c2e", "target": "check:.claude/checks/merge-guard.sh",
              "rel": "executes", "note": null}]
}
```

- **Order:** nodes are sorted by `id`, and links by `source`, `rel`, then `target`. Two runs
  at one commit differ only in `generated`.
- **The JSON carries no colours.** The palette is the view's (PANEL-35's `--map-*` tokens).

**Layers and kinds** (D3):

| Layer | Label | Kinds |
|---|---|---|
| 0 | Rules | rule |
| 1 | Controls | control |
| 2 | Mechanisms | hook, check, script, health, workflow, module, config, file |
| 3 | Skills and agents | skill, agent |
| 4 | Records | adr, prac, spec |
| 5 | Work | row, change, lane, worktree |

**Links:**

| rel | From → to | Source |
|---|---|---|
| `instance` | rules 1–3 and every control → rule 4 | AGENTS.md's own words: rules 1–3 are "instances" of rule 4, and the table is "rule 4 applied" (D4) |
| `executes` | control → mechanism, skill or agent | the table's third cell (D5) |
| `names` | skill or agent → mechanism | `refs_named_paths` per SKILL.md and agent file |
| `registers` | `config:.claude/settings.json` (or the plugin's `hooks/hooks.json`) → hook | `refs_hook_registrations` |
| `models` | `config:.claude/model-roles.json` → skill or agent; the note is `role: model/effort` | `.skills`, `.agents` and `.roles` |
| `extends` | module (or `module:local`) → each part it adds | `extensions.sh list` |
| `enforces` | ADR → mechanism | its `## Enforcement` section, classified as in D5 |
| `deps` | row → each row it depends on | `roadmap_queue --tsv` column 15 (D10) |
| `builds` | row → its change; change → each spec it has a delta for | `changes/` and `changes/archive/` |

## Refusals

| Situation | What happens instead |
|---|---|
| `jq` is missing | Nothing on stdout. `CANNOT MAP: jq is not installed` on stderr, exit 2. The panel shows the error. |
| AGENTS.md has no `**N. …**` rules or no *What executes* table | That layer is empty, and its `note` says what was looked for. `--check` fails, as `agents-md-budget.sh` already does for a missing table. |
| The roadmap does not parse (`roadmap_queue` fails) | `--json` still prints. Layer 5's rows are absent, and the layer's note carries the parser's first error line. `--check` fails. |
| The parser prints no column 15 (a `ROADMAP_PARSER` of the project's own, or before OPS-38) | No `deps` links. Layer 5's note says the queue has no Deps column. Not an error. |
| A backticked word that is not path-shaped and names no skill or agent (`origin/main`, `tools:`, `BRANCH_*`, `install`, `TestMain`) | Not a reference: no link and no error (D5). |
| A short form matches two files at the same depth | Unresolved. `--check` names both candidates and asks for the longer form. |
| A glob (`.claude/health/*.sh`) | Links to each match. A glob that matches nothing is unresolved. |
| A file named only in prose, not in backticks | Not read. The table and the ADRs put paths in backticks. |
| A shallow clone | Every `changed` is null. `notes` says the history is shallow (D8). |
| A model file in a plugin project | Its node's `path` is the `.claude/…` form the project names. It resolves against `CLAUDUCTOR_FW`, its `changed` is null, and its note says "the plugin's copy". |
| A module that is not enabled | Not on the map. The map shows what runs, not what ships. |

## Decisions (awaiting the owner)

**D1. One change, or two.**
- **Recommended: one change, in four groups.** Re-measured under step 0c, the change has:
  - three new files;
  - three one-line swaps in existing checks;
  - two table rows and a playbook paragraph.

  It is one capability with no MODIFIED requirement, and each group is one builder's and one
  reviewer's worth. PANEL-35 needs the whole of schema v1. A version that shipped half its
  links would either be bumped at once or would freeze a gap.
- **Alternative: split.** OPS-40 would keep the nodes, `executes`, `enforces` and `--check`. A
  new row would take `names`, `registers`, `models`, `extends`, `deps`, `builds`, the dates and
  the areas.
- **Why not split:** the second half adds no new kind of risk. It is the same resolver over
  other sources. The split would cost a second proposal review and leave PANEL-35 waiting on
  both.

**D2. Node ids.**
- **Recommended: `<kind>:<key>`.**
  - The key is the repository-relative path for a path node.
  - It is the name for a skill, agent or module, the number for an ADR (`adr:0005`), the row id
    (`row:OPS-40`), the change id, and the capability for a spec.
  - Rules are `rule:1` to `rule:4`.
  - A control is `control:` plus the first 8 hex digits of `git hash-object` of its first cell,
    trimmed. That is the same key `clauductor update` merges rows by, so reordering rows or
    inserting one keeps every id. Rewording a rule's cell makes a new control, which it is.
- **Alternatives:**
  - the row's position (`control:12`): every insertion renumbers the rows below it, and
    PANEL-36's animated transitions would fly the wrong nodes;
  - a slug of the first cell: readable, but up to 60 characters per id, and still changed by
    any rewording.

**D3. The kinds.**
- **Recommended:** the PRD's fifteen kinds, plus four more:
  - `config`: settings.json, model-roles.json, panel.json, project.conf, `.gitattributes` and
    dependabot.yml. The table names these, and `registers` and `models` start from two of them;
  - `file`: any other path a reference reaches, such as a Go package an ADR names
    (`framework/internal/panel`);
  - `lane` and `worktree`: used only by a panel-built map (D13).

  A mechanism's kind comes from its path, first match wins:
  - under `.claude/` (or the plugin root): `hooks/` → `hook`, `checks/` → `check`, `health/` →
    `health`, `workflows/` → `workflow`;
  - an enabled module's directory → `module`;
  - a config file named above → `config`;
  - `.sh`, `.js`, `.mjs` or `.awk` → `script`;
  - anything else → `file`.

  `prac` is in `kinds[]` from v1, and `map.sh` emits no `prac` node until OPS-46 creates
  practice records. This PR adds "map.sh reads them as `prac` nodes" to OPS-46's row (its
  owner, open and queued), so the reading has a named owner (rule 1).
- **Alternatives:**
  - fold configs and other paths into `script`: settings.json would then be drawn as a
    script, mislabelled;
  - leave `lane` and `worktree` to a v2: the panel-built map would need a schema bump before
    it shipped.

**D4. How the rules connect.**
- **Recommended:** a ninth rel, `instance`. Rules 1–3 → rule 4, and every control → rule 4.
  AGENTS.md says both: rules 1–3 are "the three instances" of rule 4, and the table is "Rule 4
  applied to this file". The rings then read as rule 4 at the centre, held by everything
  around it.
- **Alternative:** no links to the rules. They would be four isolated nodes at the centre,
  with nothing on the page saying why they are there.

**D5. What counts as a reference, and how a short form resolves.**
- **Recommended:**
  - **What counts.** A backticked token is path-shaped when it:
    - ends in a file extension (`.sh .js .mjs .awk .json .yml .yaml .md .go .conf`); or
    - ends in `/`; or
    - is a glob; or
    - has a first segment that is a top-level entry of the repository (`framework/…`,
      `scripts/…`, `.github/…`).

    `/name` must name a skill. A bare word is a reference only when it is exactly a skill's or
    an agent's name (`merge-pr`, `session-start`). Anything else is not a reference.
  - **How it resolves**, first match wins:
    1. the token as written, from the root;
    2. `.claude/<token>`;
    3. a suffix match on a path boundary among the tracked files under `.claude/`;
    4. a suffix match among all tracked files;
    5. in a plugin project, the same steps against `CLAUDUCTOR_FW` for a `.claude/…` path.

    Within steps 3 and 4, the candidate with the fewest path segments wins, so
    `scripts/ci/run-local.sh` beats `template/scripts/ci/run-local.sh` and
    `plugin/scaffold/…`. A tie at the fewest is ambiguous, which is an error. In a downstream
    project there are no template or plugin copies, and step 3 almost always decides.
- **Alternatives:**
  - every backticked token must resolve: more than a dozen of the table's tokens are config
    keys, refs or commands (`origin/main`, `ROADMAP_PARSER`, `lock-run`), and each would fail;
  - require full paths in the table: unambiguous, but it rewrites most of the table and adds
    bytes to a file every agent pays for at startup (the AGENTS.md budget).
- **What this costs:** a bare word that misspells a skill (`verfy-change`) is silently not a
  reference. The `/verify-change` form is checked.

**D6. How "Nothing. You." shows.**
- **Recommended:** a `flags` array on every node, from a closed set the spec lists:
  - for a control: `unexecuted` when its cell names no mechanism, and `by-hand` when its cell
    names one but also says "Nothing. You." (as the budget row does);
  - for a row: its state (`queued`, `inflight`, `open`, `merged`, `cancelled`);
  - for a change: `proposed`, `approved` or `archived`.

  The view can then make the controls nothing holds the most visible thing on the page.
- **Alternatives:**
  - a synthetic node "You." that the hand-held controls `execute` into: it reads well, but it
    is a node with no path, and the PRD calls such a row "a control with no mechanism";
  - only the absence of `executes` links: a `by-hand` row would look fully held.

**D7. Areas: what the colour means.**
- **Recommended: concerns, from a table in `map.sh`.**
  - **The areas:** `rules`, `change`, `merge`, `roadmap`, `session`, `models`, `safety`,
    `records`, `modules` and `project`.
  - **Mechanisms, skills and agents:** the table maps the model's own files to an area by path
    pattern. For example, `pr-merge-guard.sh`, `merge-pr`, `run-local.sh` and `lease.sh` are
    `merge`. A module's parts are `modules`. Anything the table does not place is `project`.
  - **Everything else:**
    - a control takes the area most of its mechanisms share (the first on a tie), and
      `rules` when it has none;
    - an ADR does the same over what it enforces, and is `records` when it enforces nothing;
    - rows are `roadmap`;
    - changes and specs are `change`.

  An area is decoration (ADR-0009, with PANEL-35), so a misplaced colour is not a broken
  reference, and nothing fails on it. A new model file without a pattern shows as `project`
  until someone adds one: **Nothing. You.**, and said so.
- **Alternatives:**
  - **ownership** (`model`, `module:<name>`, `local`, `project`), derived with no table to keep:
    honest and never stale, but most of the graph is one colour, and colour no longer shows
    which mechanisms belong together;
  - **a project-owned `.claude/map-areas.json`:** the project can place its own files, but it
    is a second file to keep current, read by one script.

**D8. The `changed` dates.**
- **Recommended:** one `git log --format=%x00%cs --name-only -- <every node path>` pass. The
  first date seen for a path is its last change, and a directory takes the latest date under
  it.
  - Nodes that share a file (rules, controls, rows) get null. A file-level date would say that
    every rule changed whenever any line of AGENTS.md did.
  - An untracked or plugin file gets null.
  - In a shallow clone every date is null, with a note: the boundary commit would otherwise
    date every file as "added that day".
- **Alternatives:**
  - `git log -1` per node: about 260 processes, several seconds, and a risk to the panel's
    30-second timeout on a large repository;
  - `git blame` per row: true dates for rows and controls, but one blame per file at every
    refresh, for a field the timeline uses only coarsely.

**D9. Which rows and changes.**
- **Recommended: every roadmap row, open or closed, flagged by state (D6).** Also every change,
  open or archived. This repository has 104 rows. PANEL-36's timeline needs the merged ones,
  and `deps` needs both ends of every link.
- **Alternative: open rows only.** A smaller graph, but a dependency on a merged row would
  point nowhere, and the timeline would have no past.

**D10. `deps`, designed against OPS-38's contract.**
- **Recommended:** read `roadmap_queue --tsv` column 15, which OPS-38 defines.
  - Each token that is a row id becomes a `deps` link. A `#N` reference names an issue or PR,
    not a node, so it makes no link.
  - Without a column 15 there are no `deps` links, and a note says why (*Refusals*).
  - The build waits for OPS-38 to merge, as the row's Deps already say. If OPS-38 lands a
    different column, the delta goes back to the owner before group 3 is built.
- **Alternative: parse the Deps cell from the roadmap file directly.** It needs no OPS-38, but
  it would be a second roadmap parser, the breach the one-parser rule forbids and that OPS-38
  closes for `currency.sh`.

**D11. Share the readers, or copy them.**
- **Recommended:** move the named-path pattern, the hook-registration query and the table
  anchor into `.claude/lib/refs.sh`. `checks/skills.sh`, `checks/hooks.sh` and
  `checks/agents-md-budget.sh` then call it, and so does `map.sh`. Two copies of one parser
  disagree silently, which is the rule `roadmap-queue.sh` exists for. `refs_table_rows` does
  not touch the Go reader in `projectfiles.go`, which is another language's. D2 keys controls
  the way that reader does.
- **Alternative: copy the three readers into `map.sh`.** It leaves three existing checks
  untouched, but it gives the codebase two of each.
- **The risk:** `checks/hooks.sh` holds the merge guard's registration. If the swap changed
  what it reads, the guard's check could weaken without anyone noticing. The swap is
  therefore its own task, and the task's test compares the old and new outputs on this
  repository before the old code is deleted.

**D12. What `--check` fails, and how `--json` degrades.**
- **Recommended:**
  - **`--check`** fails every unresolved reference from every source: `executes`, `enforces`,
    `names`, `registers`, `models`, `extends`, `deps` and `builds`.
    - Several of these are already checked elsewhere (`skills.sh`, `hooks.sh`,
      `model-roles.sh`, OPS-38's Deps validation). The map resolves each anyway, and one rule
      ("every edge resolves") is simpler to hold than eight exceptions.
    - New coverage: table short forms, paths outside `.claude/`, and ADR Enforcement paths.
  - **`--json`** degrades per layer, as `metrics.sh` degrades per figure: a source that cannot
    be read empties what it feeds, with a note, and everything else still draws.
- **Alternatives:**
  - `--check` only on what nothing else checks: less duplication, but a reader of the map
    could not trust that every link it draws resolves;
  - `--json` that fails whole on any error: the Map view would go blank over one bad
    roadmap line.

**D13. A map the panel builds (an attached repository).**
- **Recommended: schema v1 admits it now.**
  - `source` is a free non-empty string: `map.sh`, `panel`, or a repository's own generator.
  - A layer with no nodes must carry a `note` saying why ("this repository has no Clauductor
    model: no AGENTS.md table to read").
  - The kinds `lane` and `worktree` and the rel `in` (lane → worktree) exist for it.
  - `checks/map.sh` validates an inline panel-built fixture with `--validate`, and PANEL-35's
    Go tests can run the same `--validate` on the panel's output where sh and jq exist. One
    executable schema, two producers.
- **Alternative: a v2 when PANEL-35 builds it.** v1 would ship narrower, and the panel would
  need to read two versions from its first day.

**D14. Where the schema lives.**
- **Recommended:** `specs/map/spec.md` holds the schema as requirements, and `map.sh
  --validate` executes it. PANEL-35 adds a *Map* section to `docs/panel.md` that cites the
  spec, as *Metrics* documents its own contract.
- **Alternative: write the field-level contract into `docs/panel.md` now**, as Metrics did. It
  follows the precedent, but it would document a panel view that does not exist yet, in a file
  this change has no other reason to touch.

**D15. A row in AGENTS.md's table.**
- **Recommended: yes, in the template and in this repository.** `checks/map.sh` is a new
  control: nothing else checks that ADR Enforcement paths and table short forms resolve. The
  file's own rule is "change this file only to add a mechanism … and give the mechanism its
  row". The row is about 150 characters, under both the 320-character row ceiling and the
  16,000-byte file ceiling (11,392 bytes today). `clauductor update` adds it to installed
  projects by its first cell.
- **Alternative: no row, and treat the check as part of the `checks/skills.sh` row.** It costs
  no bytes, but the table would not name what holds the table itself.

**D16. Risk: normal.**
- **Recommended: normal.**
  - **Why not low:** `map.sh` is read-only and writes nothing, but the change adds a gate check
    that can turn every PR red. It also swaps the reader under `checks/hooks.sh`, which holds
    the merge guard's registration (D11).
  - **Why not high:** nothing is irreversible, nothing outward, nothing kills a process or
    spends.
- **Alternatives:**
  - low, because the generator only reads;
  - high, because of the guard's check.
