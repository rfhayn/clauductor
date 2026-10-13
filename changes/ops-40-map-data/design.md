# Design: ops-40-map-data

## Where the behaviour lives today

- **The table is read in three places, none of which follows its references:**
  - `checks/agents-md-budget.sh` finds it under `### What executes each rule` and measures its
    rows;
  - `clauductor update` keys its rows by first cell (`projectfiles.go` `agentsRows`), and
    *suggests* a row the template has and the project lacks (`MissingAgentsRows`);
  - `checks/skills.sh` greps full `.claude/…\.(sh|js|awk|json)` paths out of AGENTS.md, the
    skills, the agents and settings.json, and fails a missing one.

  Short forms (`checks/merge-guard.sh`, `pr-merge-guard.sh`, `run-local.sh`, `/verify-change`)
  are never resolved. Paths outside `.claude/` (`scripts/ci/run-local.sh`, `.github/…`) are
  never checked.
- **ADR Enforcement sections are prose.** `checks/adr-numbering.sh` checks that each ADR has
  one, not that what it names exists. Some of them name commands, not bare paths
  (`.claude/scenario-trace.sh --check` in ADR-0002).
- **The other link sources already parse:**
  - hook registrations: `checks/hooks.sh`'s jq over `.hooks`;
  - the role per skill and agent: `model-roles.json` `.skills`, `.agents` and `.roles`;
  - module parts: `ext_files` and `ext_dirs` in `lib/modules.sh` over `MODULE_POINTS`, as
    `checks/run.sh` reads them (`extensions.sh list` prints the same parts for people);
  - rows: `roadmap_queue --tsv`, the one parser;
  - a row's change: `changes/<id>/` or `changes/archive/<date>-<id>/`;
  - a change's spec deltas: `changes/<id>/specs/<cap>/`.
- **A plugin project names the model differently.** `scripts/build-plugin.sh` rewrites the
  scaffold's AGENTS.md:
  - `.claude/X` becomes `${CLAUDE_PLUGIN_ROOT}/X`;
  - `/propose` becomes `/clauductor:propose`.

  Diff `plugin/scaffold/AGENTS.md` against `template/AGENTS.md` to see it. The plugin's
  `hooks/hooks.json` registers each guard through a wrapper:
  `sh "${CLAUDE_PLUGIN_ROOT}/hooks/if-project.sh" pr-merge-guard.sh`. The plugin's own gate
  runs the checks, so `checks/map.sh` runs there too (`plugin/scaffold/scripts/ci/steps.sh`).
- **A fresh install has no commit.** `start-project` runs the checks before the project's first
  commit, so the model's files are untracked at that moment.
- **The panel runs a project's metrics command** as trusted argv, with a 30-second timeout and
  1 MB of output (`docs/panel.md`, `metrics.command`). PANEL-35 will run `map.sh` the same way,
  so the output must fit those bounds. This repository's map is an estimated 260 nodes, around
  100 KB.
- **A rough prototype run on 2026-10-13** applied a resolution order like D5's, by hand, to this
  repository's table and its eight ADRs' Enforcement sections. With D5's revised classifier,
  every token it would treat as a reference had a file. The builder confirms this on the real
  implementation; it is a claim until then. A reference that still dangles in an ADR goes to
  the owner: the build never edits an ADR (task 4.2).

## The shape of the change

1. **`.claude/lib/refs.sh`** (D11): the shared readers. Each is moved from its check, and its
   output format is stated here because the format is the contract:
   - `refs_named_paths FILE…`: `FILE TAB LINE TAB PATH`, one line per `.claude/…` path matching
     `checks/skills.sh`'s pattern. `skills.sh` takes the third column, `sort -u`, which is the
     set it reads today. `map.sh` keeps the file and line.
   - `refs_hook_registrations SETTINGS`: `EVENT TAB COMMAND` per command hook, exactly the
     `regs` lines `checks/hooks.sh` builds today. It exits non-zero when the file does not
     parse.
   - `refs_table_rows AGENTS_MD`: `LINE TAB ROW` per row under `### What executes each rule`,
     with the header and separator excluded. `ROW` is byte for byte the file's line, so
     `agents-md-budget.sh` measures `cut -f2-`, the same characters it measures today.

   Each of those three checks calls its function instead of its own copy. A new
   `checks/refs.sh` holds the three checks' behaviour across the swap and keeps holding it:
   it runs each check against a fixture with `ROOT=<fixture>` (D11).
2. **`.claude/map.sh`** (POSIX sh, git, jq) has three modes:
   - `--json` (the default): prints the map;
   - `--check`: exits 1 naming each unresolved reference as `FILE[:LINE]: TOKEN: why`. A
     reference read through jq (settings.json, model-roles.json) has no line;
   - `--validate FILE`: holds a map from any source to schema v1, exiting 1 naming each fault.

   It runs from the project root (`ROOT`), which may be a subdirectory of the git repository
   (a monorepo, or `template/` in this repository). Every git call is relative to `ROOT`.
   - **Files** come from `git ls-files -co --exclude-standard`: tracked files, plus untracked
     files that are not ignored. A fresh install resolves before its first commit.
   - **Dates** come only from git history.
   - **Bounds:** at most four git calls whatever the node count (`rev-parse`, `ls-files`,
     `log`, and one shallow test), and one `cksum` call for every control id (D2).
   - **Writes:** nothing.
   - **Test seams:** `MAP_NOW` (unix seconds) and `PATH` (git, jq).
3. **`.claude/checks/map.sh`** does two things:
   - runs `map.sh --check` on the project, and validates its `--json` output with
     `--validate`. The green line names its subject: the counts of nodes and links, and the
     sources read;
   - holds every MAP scenario against throwaway repositories, a fake plugin root built from
     `plugin/scaffold/`'s rewritten AGENTS.md shape, and a panel-built fixture written inline.
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
- **Uniqueness:** node ids are unique. A duplicate is a `--validate` fault, and so is two
  controls with the same first cell.
- **Plugin paths:** a node for the plugin's copy of a file has `path`
  `${CLAUDE_PLUGIN_ROOT}/<path in the plugin>`, literally, so the id is the same on every
  machine (D5).
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
| `executes` | control → mechanism, skill or agent | the table's second column, *What executes it* (D5) |
| `names` | skill or agent → mechanism | `refs_named_paths` per SKILL.md and agent file |
| `registers` | `config:.claude/settings.json` → hook; in a plugin project, also the plugin's `hooks/hooks.json` → the hook its `if-project.sh` argument names | `refs_hook_registrations` |
| `models` | `config:.claude/model-roles.json` → skill or agent; the note is `role: model/effort` | `.skills`, `.agents` and `.roles` |
| `extends` | module (or `module:local`) → each part it adds | `ext_files` and `ext_dirs` over `MODULE_POINTS` (`lib/modules.sh`) |
| `enforces` | ADR → mechanism | its `## Enforcement` section, classified as in D5 |
| `deps` | row → each row it depends on | `roadmap_queue --tsv` column 15 (D10) |
| `builds` | row → its change; change → each spec it has a delta for | `changes/` and `changes/archive/` (D9) |

## Refusals

| Situation | What happens instead |
|---|---|
| `jq` is missing | Nothing on stdout. `CANNOT MAP: jq is not installed` on stderr, exit 2. The panel shows the error. |
| AGENTS.md has no `**N. …**` rules or no *What executes* table | That layer is empty, and its `note` says what was looked for. `--check` fails, as `agents-md-budget.sh` already does for a missing table. |
| The roadmap does not parse (`roadmap_queue` fails, including OPS-38's refusal of an unknown, cyclic or cancelled Deps id) | `--json` still prints. Layer 5's rows are absent, and the layer's note carries the parser's first error line. `--check` fails with that error. |
| The parser prints no column 15 (a `ROADMAP_PARSER` of the project's own, or before OPS-38) | No `deps` links. Layer 5's note says the queue has no Deps column. Not an error. |
| A backticked token that is not path-shaped and names no skill or agent (`origin/main`, `tools:`, `BRANCH_*`, `install`, `TestMain`) | Not a reference: no link and no error (D5). |
| A backticked command (`.claude/scenario-trace.sh --check`, `sh .claude/map.sh --check`) | Read as its first word after an optional `sh`, `bash` or `node`: here, the script (D5). |
| A path with a line suffix (`run-local.sh:42`, `x.sh:10-20`) | The suffix is stripped before resolving (D5). |
| A bare `.md` name with no `/` (`SKILL.md`, `design.md`, `AGENTS.md`) | Not a reference (D5): generic document names would otherwise be ambiguous everywhere. |
| A short form that matches two files at the same depth | Unresolved. `--check` names both candidates and asks for the longer form. |
| A glob (`.claude/health/*.sh`) | Links to each match. A glob that matches nothing is unresolved. |
| A file named only in prose, not in backticks | Not read. The table and the ADRs put paths in backticks. |
| An untracked file that is not ignored | Resolves. Its `changed` is null (D8). |
| A shallow clone | Every `changed` is null. `notes` says the history is shallow (D8). |
| A repository with no commit yet (a fresh install) | Resolves as usual. `commit` is null, every `changed` is null, and `--check` passes if every reference resolves. |
| A model file in a plugin project | Its node's `path` is `${CLAUDE_PLUGIN_ROOT}/…`. It resolves against `CLAUDUCTOR_FW`, its `changed` is null, and its note says "the plugin's copy". |
| A module that is not enabled | Not on the map. The map shows what runs, not what ships. |
| A dangling reference in an ADR's Enforcement section | `--check` fails. The ADR is the owner's, so the build reports it and never edits the ADR to make the check pass. |

## Decisions (awaiting the owner)

**D1. One change, or two.**
- **Recommended: one change, in four groups.** Re-measured under step 0c, the change has:
  - four new files (`map.sh`, `checks/map.sh`, `lib/refs.sh`, `checks/refs.sh`);
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

**D2. Node ids.** *(Revised after review: the control hash.)*
- **Recommended: `<kind>:<key>`.**
  - The key is the path for a path node: repository-relative, or `${CLAUDE_PLUGIN_ROOT}/…` for
    the plugin's copy.
  - It is the name for a skill, agent or module, the number for an ADR (`adr:0005`), the row id
    (`row:OPS-40`), the change id, and the capability for a spec.
  - Rules are `rule:1` to `rule:4`.
  - A control is `control:` plus 8 hex digits of the POSIX `cksum` of its trimmed first cell.
    One `cksum` call covers every cell, so the cost does not grow with the table. The first
    cell is the key `clauductor update` already uses for a row, so reordering rows or
    inserting one keeps every id. Rewording a rule's cell makes a new control, which it is.
  - Two controls with the same first cell would share an id. `--validate` refuses a duplicate
    id, so the gate fails it.
- **Alternatives:**
  - the row's position (`control:12`): every insertion renumbers the rows below it, and
    PANEL-36's animated transitions would fly the wrong nodes;
  - a slug of the first cell: readable, but up to 60 characters per id, and still changed by
    any rewording;
  - `git hash-object` per cell (the first draft): one git process per control, so the number
    of calls grows with the table.

**D3. The kinds.** *(Revised after review: the spec flags.)*
- **Recommended:** the PRD's fifteen kinds, plus four more:
  - `config`: settings.json, model-roles.json, panel.json, project.conf, `.gitattributes`,
    dependabot.yml, and the plugin's `hooks/hooks.json`. The table names these, and
    `registers` and `models` start from them;
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

**D5. What counts as a reference, and how a short form resolves.** *(Revised after review:
commands, globs, `.md` names and plugin projects.)*
- **Recommended:**
  - **Normalising a backticked span:**
    - a span that holds whitespace is a command: the token is its first word, after an
      optional leading `sh`, `bash` or `node` (`.claude/scenario-trace.sh --check` →
      `.claude/scenario-trace.sh`);
    - a trailing `:N` or `:N-M` line suffix is stripped.
  - **What counts as a reference.** A token is path-shaped when it:
    - ends in a script or config extension (`.sh .js .mjs .awk .json .yml .yaml .conf .go`), or
      contains a `/` and ends in `.md`; or
    - has a first segment that is `.claude`, `${CLAUDE_PLUGIN_ROOT}` or a top-level entry of
      the project root (`framework/…`, `scripts/…`, `.github/…`, `.gitattributes`).

    A token that ends in `/` or holds a glob character is path-shaped only by the second rule,
    so `BRANCH_*` and `docs/` in prose are not references unless their first segment is a
    top-level entry. `/name` and `/<namespace>:name` must name a skill (a plugin skill for the
    namespaced form). A bare word is a reference only when it is exactly a skill's or an
    agent's name (`merge-pr`, `session-start`). Anything else is not a reference.
  - **How it resolves.** The candidates are the files under `ROOT` from
    `git ls-files -co --exclude-standard`, and in a plugin project the files under
    `CLAUDUCTOR_FW`. First match wins:
    1. the token as written, from `ROOT`. In a plugin project, `${CLAUDE_PLUGIN_ROOT}/X` is
       `$CLAUDUCTOR_FW/X`, and `.claude/X` that is not in the project is `$CLAUDUCTOR_FW/X`;
    2. `.claude/<token>` (and, in a plugin project, `$CLAUDUCTOR_FW/<token>`);
    3. a suffix match on a path boundary among the files under `.claude/` (and the plugin
       root);
    4. a suffix match among all the candidates.

    Within steps 3 and 4, the candidate with the fewest path segments wins. Segments are
    counted from the candidate's own root (`ROOT` or the plugin root). So
    `scripts/ci/run-local.sh` beats `template/scripts/ci/run-local.sh` and
    `plugin/scaffold/…`. A tie at the fewest is ambiguous, which is an error. In a downstream
    project there are no template or plugin copies, and step 3 almost always decides.
- **Alternatives:**
  - every backticked token must resolve: more than a dozen of the table's tokens are config
    keys, refs or commands (`origin/main`, `ROADMAP_PARSER`, `lock-run`), and each would fail;
  - require full, bare paths in the table and the ADRs: unambiguous, but it rewrites most of
    the table, adds bytes to a file every agent pays for at startup, and edits the owner's
    ADRs.
- **What this costs:**
  - a bare word that misspells a skill (`verfy-change`) is silently not a reference, though the
    `/verify-change` form is checked;
  - a bare `.md` name is never checked; a path to a document needs a `/`.

**D6. How "Nothing. You." shows.**
- **Recommended:** a `flags` array on every node, from a closed set the spec lists:
  - for a control: `unexecuted` when its cell names no mechanism, and `by-hand` when its cell
    names one but also says "Nothing. You." (as the budget row does);
  - for a row: its state (`queued`, `inflight`, `open`, `merged`, `cancelled`);
  - for a change: `proposed`, `approved` or `archived`;
  - for a spec: `proposed` when no living spec has that capability yet (D9).

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

**D8. The `changed` dates.** *(Revised after review: relative paths, and the bound on git
calls.)*
- **Recommended:** one `git log --relative --format=%x00%cs --name-only -- <every node path>`
  pass, run from `ROOT`. The first date seen for a path is its last change, and a directory
  takes the latest date under it.
  - Nodes that share a file (rules, controls, rows) get null. A file-level date would say that
    every rule changed whenever any line of AGENTS.md did.
  - An untracked or plugin file gets null.
  - With no commit yet, every date is null.
  - In a shallow clone every date is null, with a note: the boundary commit would otherwise
    date every file as "added that day".
  - The whole map makes at most four git calls, whatever the node count.
- **Alternatives:**
  - `git log -1` per node: about 260 processes, several seconds, and a risk to the panel's
    30-second timeout on a large repository;
  - `git blame` per row: true dates for rows and controls, but one blame per file at every
    refresh, for a field the timeline uses only coarsely.

**D9. Which rows, changes and specs.** *(Revised after review: proposed specs.)*
- **Recommended:**
  - **every roadmap row**, open or closed, flagged by state (D6). This repository has 104 rows.
    PANEL-36's timeline needs the merged ones, and `deps` needs both ends of every link;
  - **every change**, open or archived;
  - **every spec:** the living specs, plus every capability an open change has a delta for,
    flagged `proposed` when no living spec has it. Today five open changes here propose
    capabilities that have no living spec, so without that rule each of their `builds` links
    would dangle. An archived change links only to capabilities that are living now. A
    capability that was later removed makes no link and no error.
- **Alternative: open rows only, and living specs only.** A smaller graph, but a dependency on
  a merged row and every new capability's `builds` link would point nowhere, and the timeline
  would have no past.

**D10. `deps`, designed against OPS-38's contract.** *(Revised after review: the parser holds
the ids.)*
- **Recommended:** read `roadmap_queue --tsv` column 15, which OPS-38 defines.
  - OPS-38's work in progress (`.claude/worktrees/ops-38`, `roadmap_deps_errors`) makes column
    15 empty or comma-separated tokens with no spaces. Each token is a row id or `#N`, and
    column 14 is empty from the template's parser.
  - Each row-id token becomes a `deps` link. `#N` names an issue or PR, not a node, so it makes
    no link.
  - The parser already refuses the whole queue on an unknown, cyclic or cancelled dependency.
    So `map.sh` does not check the ids again: such a roadmap is a queue that does not parse
    (*Refusals*), and `--check` fails with the parser's own error.
  - Without a column 15 there are no `deps` links, and a note says why.
  - The build waits for OPS-38 to merge, as the row's Deps already say. If OPS-38 lands a
    different column, the delta goes back to the owner before group 3 is built.
- **Alternative: parse the Deps cell from the roadmap file directly.** It needs no OPS-38, but
  it would be a second roadmap parser, the breach the one-parser rule forbids and that OPS-38
  closes for `currency.sh`.

**D11. Share the readers, or copy them.** *(Revised after review: formats, and lasting
fixtures.)*
- **Recommended:** move the named-path pattern, the hook-registration query and the table
  anchor into `.claude/lib/refs.sh`, with the output formats given in *The shape of the
  change*. `checks/skills.sh`, `checks/hooks.sh` and `checks/agents-md-budget.sh` then call it,
  and so does `map.sh`. Two copies of one parser disagree silently, which is the rule
  `roadmap-queue.sh` exists for. The Go reader in `projectfiles.go` is another language's and
  stays. D2 keys controls the way it does.
  - **What holds the swap:** comparing old and new output once proves nothing after the old
    copies are deleted. A new `checks/refs.sh` instead runs each of the three checks with
    `ROOT=<fixture>`, before the swap and from then on:
    - `skills.sh` fails "named but missing: .claude/x.sh" for a skill naming a missing script,
      and passes when the script exists;
    - `hooks.sh` fails when settings.json does not parse;
    - `agents-md-budget.sh` passes a 320-character row, fails a 321-character one, and fails
      when the table is missing.
- **Alternative: copy the three readers into `map.sh`.** It leaves three existing checks
  untouched, but it gives the codebase two of each.
- **The risk:** `checks/hooks.sh` holds the merge guard's registration. If the swap changed what
  it reads, the guard's check could weaken without anyone noticing, which is why
  `checks/refs.sh` keeps the fixtures running for good.

**D12. What `--check` fails, and how `--json` degrades.** *(Revised after review: `deps` and
line numbers.)*
- **Recommended:**
  - **`--check`** fails every unresolved or ambiguous reference from `executes`, `enforces`,
    `names`, `registers`, `models`, `extends` and `builds`, and a queue the parser refuses.
    `deps` ids are the parser's to hold (D10).
    - Several of these are already checked elsewhere (`skills.sh`, `hooks.sh`,
      `model-roles.sh`). The map resolves each anyway, and one rule ("every edge resolves") is
      simpler to hold than seven exceptions.
    - New coverage: table short forms, paths outside `.claude/`, and ADR Enforcement paths.
    - Each failure reads `FILE[:LINE]: TOKEN: why`. A reference read through jq has no line.
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

**D15. A row in AGENTS.md's table.** *(Revised after review: how installed projects get it.)*
- **Recommended: yes, in the template and in this repository.**
  - `checks/map.sh` is a new control: nothing else checks that ADR Enforcement paths and table
    short forms resolve.
  - The file's own rule is "change this file only to add a mechanism … and give the mechanism
    its row".
  - The row is about 150 characters, under both the 320-character row ceiling and the
    16,000-byte file ceiling (11,392 bytes today).
  - `clauductor update` *suggests* the row to an installed project that lacks it
    (`MissingAgentsRows`); it does not add it. The check itself reaches every project with the
    model's files, row or not.
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
