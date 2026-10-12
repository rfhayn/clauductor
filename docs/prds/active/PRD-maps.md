# PRD: MAPS: a map, routines and a skills deck for the panel, and the Standing Tee practices since 2026-10-01

**Status:** decided 2026-10-12 by the owner (plan approved in session). Rows OPS-37 to PANEL-40 are queued in
`docs/roadmap.md` (Phase 2, *MAPS phase 1*), and OPS-48 and PANEL-41 at the end of Phase 2 (*MAPS phase 2*).
Each capability row is still proposed just in time and approved on its own `design.md`. This PRD gives direction;
it does not approve a design.

## Why

On 2026-10-12 the owner reviewed Pav Rusovs' "Turn Claude Code Into Your AI Operating System (4 Layers)"
(youtu.be/Zs3faMCDYNs) and its free guide (github.com/pavrus117/ai-os-maps-guide). It describes a personal
operating system in four layers, MAPS:

| Layer | What it is |
|---|---|
| **Memory** | A signposted map of every file, checked nightly. |
| **Agent** | The same Claude Code on a laptop and an always-on server. |
| **Pulse** | Routines that run unattended, from a registry, through one runner, with a run log and caps. |
| **Screen** | A dashboard that shows and never stores. |

The dashboard is a dark three-column page:

- a "Today" column;
- a colour-coded, animated graph of the workspace, drawn six ways from one data file;
- a column with the next session, what is waiting, a deck of skill buttons and a routines table.

**How Clauductor compares:**

- **Already ahead on the Screen layer.** The panel has lanes, the gate lock, metrics and alerts.
- **Shares several principles.** Each figure names its subject and goes stale rather than hiding, deterministic
  code runs before the model, plans live in the repo, and nothing is deleted.
- **Has no Pulse.** Nothing runs while the owner is away. The only scheduled job is the weekly dependency audit, and
  OPS-35 waits on an observation that nothing makes.
- **Has no picture of itself.** AGENTS.md's *What executes it* table is a graph of rule to mechanism. Today it is read
  as text, and its "Nothing. You." rows are easy to miss.

The Standing Tee playbook (updated 2026-10-07) has also moved past the 2026-10-01 convergence map:

- the gate merges `main` in under its lease;
- `gate-wait` and the no-background-gate hook;
- worktree sessions that report back, and a watcher that wakes the orchestrator;
- a roadmap parser that derives order and the critical path from Deps;
- status-line session counts;
- practice records;
- requests priced before they are relayed.

## Decisions (the owner, 2026-10-12)

1. **Build it into Clauductor's panel**, for every project, through the normal process: roadmap rows, `/propose` one
   ahead, `build-change`, `merge-pr`.
2. **Phase 1 is free:**
   - the Map view;
   - routines;
   - the Today column and its heatmap;
   - the skills deck;
   - the Standing Tee practices above.
3. **The Map embraces his look:** colour as decoration and motion. It is **the one exception** to the panel's
   "grey at rest" rule (`docs/panel.md`, *The page* and *Appearance*). The exception is contained by tests and recorded
   as an ADR with PANEL-35 (see *The Map's exception*).
4. **Review rounds have a soft cap of 5**, not a hard stop. At round 5 the run stops and asks the owner whether to
   continue, and it continues only on their yes (OPS-44).
5. **Phase 2 is the $15 Builder tier** of his Patreon (his dashboard source, "Classic" and "Current" layouts). It is
   studied, never vendored (see *The clean-room rule*).
6. **Order:** after the four rows ahead of it in Phase 2 (OPS-33 #83, OPS-36, PANEL-29, PANEL-28), ahead of every
   other queued row.
   Later the same day, PANEL-42 (attach, `PRD-attach.md`) was queued ahead of MAPS.
7. **Checkpoints:**
   - the tag `checkpoint/2026-10-12-pre-maps` on `010941d` (pushed), plus a dated backup of the installed panel
     binary with its sha256;
   - a new tag and backup at each phase's end, named `checkpoint/<date>-maps-phase-<N>`.

   Rollback goes through PRs, never a reset of `main`. The installed panel is restored only on the owner's go.

## The standing answer to `/propose` step 0a

`changes/` holds approved proposals for PANEL-25, PANEL-29, PANEL-30 and PANEL-31, and an approved PANEL-28, so
`/propose` step 0a ("at most one change proposed ahead") stops on every MAPS row.

**The owner's answer, given once here:** MAPS rows go ahead of the approved PANEL-25, PANEL-30 and PANEL-31
proposals. Those proposals keep their approval and wait behind MAPS. A design whose code moved under it goes back to
the owner before its build. The same day the owner queued PANEL-42 (attach, `PRD-attach.md`) before MAPS, so the
answer covers PANEL-42 as well.

A `/propose` for a MAPS row or for PANEL-42 cites this section and continues. It still stops for anything else ahead
of it.

## Phase 1: the rows

The order and dependencies are the roadmap's. **C** marks a capability change (`/propose`). The rest are `ops/`
lanes, which run alongside but never stack. Two lanes never edit the same file at once: run-local.sh goes OPS-41
then OPS-47, and build-change.js goes OPS-43 then OPS-44.

**Capability chain:** OPS-40 → PANEL-35 → OPS-42 → PANEL-36 → PANEL-37 → PANEL-38 → PANEL-39 → OPS-47 →
PANEL-40.
**Ops lanes:** OPS-38 first, because the chain's root (OPS-40) needs its Deps column; OPS-39 follows it. OPS-41, OPS-43, OPS-44, OPS-45 and OPS-46
fill in alongside.

### The roadmap reads Deps (OPS-38, OPS-39)

- **OPS-38: Deps becomes a contract column.** It is an optional 15th column of `roadmap_queue --tsv` (`lib/conf.sh`
  `roadmap_tsv_errors`).
  - It is validated: an unknown id, a cycle or a cancelled dependency is an error.
  - Today's freeform cells are migrated (`P2.15 (#45)` becomes `#45`).
  - The second roadmap parser in `modules/artifacts/bin/currency.sh` moves onto `roadmap_queue`, which ends a latent
    breach of the one-parser rule.
- **OPS-39: the order is derived.**
  - It ports Standing Tee's `roadmap-queue.mjs` behaviour (#601) to POSIX sh. Each row reads as next, waits on X,
    `blocked — <what> (<owner>)` or `held — <why>`.
  - It marks the critical path: the longest chain of open dependencies, counted in rows.
  - `panel-suggest.sh` offers only rows whose dependencies are met.

### The Map (OPS-40, PANEL-35, PANEL-36)

**Data (OPS-40)**

- `.claude/map.sh` is POSIX sh and jq (ADR-0006). It prints schema v1 to stdout:
  - top level: `{version, generated, commit, nodes, links, areas, kinds, layers}`;
  - each node: `{id, kind, label, area, layer, path, note, changed}`.
- **Kinds:** rule, control, hook, check, script, health, workflow, skill, agent, adr, prac, module, row, change, spec.
- **Layers, from the centre out (they become the rings):**
  - 0: the four rules
  - 1: AGENTS.md controls
  - 2: mechanisms
  - 3: skills and agents
  - 4: records
  - 5: roadmap rows and changes
- **Links reuse what already parses:**

  | Link | Source |
  |---|---|
  | `executes` | The *What executes it* cells. Short forms resolve by basename, and "Nothing. You." is a control with no mechanism. |
  | `names` | The `checks/skills.sh` path pattern, run per file. |
  | `registers` | settings.json hooks. |
  | `enforces` | Each ADR's `## Enforcement` paths. |
  | `models` | `model-roles.json`. |
  | `extends` | `extensions.sh list`. |
  | `deps` | OPS-38's column. |
  | `builds` | A row's `changes/<id>`. |

- **Nothing is committed**, because a committed map would conflict on every squash and go stale.
- `--check` fails on a reference that does not resolve. As `checks/map.sh` it runs in the gate, so the generator has a
  production caller without the panel.

**The view (PANEL-35, then PANEL-36)**

- **Placement:** a full view beside Metrics (button and the `m` key). The panel runs `map.sh` as a trusted command, as
  it runs metrics, and serves it at `GET /api/p/{p}/map`. It refreshes only while a page is in view (PANEL-25).
- **Renderer:** Canvas 2D, written by hand. The CSP allows it, and it needs no library.
  - The force layout is our own, sized for about 500 nodes at 60 fps.
  - The loop pauses while hidden, and reduced motion snaps every transition.
  - Search, the node card and the legend are real DOM elements, so a screen reader reaches them and the arrow keys
    walk neighbours.
- **PANEL-35:** the rings and force layouts.
  - Colour by area, shape by kind, size by link count.
  - Hover dims everything but a node and its neighbours.
  - Clicking a node opens a card (copy path, fly to it), and search finds nodes.
- **PANEL-36:** the areas and timeline layouts.
  - Animated transitions between layouts.
  - The legend isolates an area or a kind.
  - A header with live dots for the stream, the runner and the data's age.

### The Map's exception (with PANEL-35)

PANEL-35's proposal drafts ADR-0009, "The Map is the one view where colour is decoration". Its `## Enforcement` names
these tests:

| | What the test holds |
|---|---|
| (a) | The web tests list `static/` from the directory and fail on a file they do not know. A new stylesheet cannot escape them. |
| (b) | Every `map.css` selector sits under `.map`. Its palette is `--map-*` tokens for light and dark, contrast-tested against every theme and mode. |
| (c) | `tells_test.go` still bans everything it bans in `panel.css`. A negative test fails a `.map` selector or a `--map-` token anywhere in `panel.css` or `themes.css`. |
| (d) | `@keyframes` and animation appear only in `map.css`, inside `prefers-reduced-motion: no-preference`. |

`docs/panel.md` *Appearance* gains a subsection that says so.

### Routines (OPS-42, PANEL-37)

**Runner:** a local runner. Claude's cloud routines run with the laptop closed, but they see only GitHub, not the
spend ledger, the worktrees, the gate receipts or a macOS-only flake. Their writes would also be outward, which is the
owner's decision. The registry keeps a `where` field so a cloud runner can follow.

**The registry: `.claude/routines.json`.** It is the project's own file, and `clauductor update` never overwrites it.

- **Top level:** `{version, caps, routines}`.
- **Caps:** a daily dollar cap, a rolling-window dollar cap and a daily run count.
- **Each routine:** `{name, schedule, machine, run, timeout_s, catch_up, output, enabled}`.
- **`run` is either:**
  - a command, which costs no tokens; or
  - `{skill|prompt, model, effort, allowed_tools, disallowed_tools, max_turns}` for `claude -p`.

**`routines.sh tick`**

- Takes a lock under the git common dir.
- Checks the caps against the run log.
- Reads the quota from `~/.clauductor/panel/quota.json` when it is there.
- Refuses when `ANTHROPIC_API_KEY` is set, so it runs on the subscription only, as lanes do.
- Runs `claude -p --output-format json`, under a timeout that stops only its own child process (ADR-0005).
- Takes the cost and turns from the JSON. A headless run never reaches the panel's spend ledger.

**Output and health**

- The run log is `<git common dir>/routines/runs.jsonl`; outputs go to `routines/out/<name>/`.
- `health/routines.sh` reports OK, FAILED, NEVER RAN, STALE or CAPPED, and names the run.
- `checks/routines.sh` fails a routine that lists a write tool unless it says `writes: true`.

**Who ticks it (PANEL-37)**

- The panel ticks every 5 minutes for each trusted project. This is an explicit exception to PANEL-25's idle back-off.
- The panel's trust covers the registry's hash too, because a change to it changes unattended spend.
- Without the panel, a documented launchd or cron line ticks it (ADR-0006).
- The Map's right column shows the routines table, ▶ run now, and today's and yesterday's runs, failures and cost.

**First routines in this repo:**

- `nightly-checks` (a command).
- `flake-watch` (a command): the observation OPS-35 waits on.
- `morning-digest` (`claude -p`, sonnet, low effort, read-only tools), shown in Today. It does not depend on
  PushNotification under `-p`, which is unverified.

### Today (PANEL-38) and the skills deck (PANEL-39)

**Layout:** three columns inside the Map.

| Column | What it holds |
|---|---|
| Left | Today: a clock, a quarterly day heatmap (activity by default, cost on a toggle), the next 14 days, activity bars and the backlog count. |
| Centre | The graph. |
| Right | Next session (the queue's next row and the critical path), Waiting (the owner queue), the skills deck and the routines. |

**Today's data:** `GET /api/p/{p}/today` aggregates it in Go, read-only and with no transcripts, from:

- the spend ledger;
- merged PRs;
- OPS-41's gate run log;
- the routine run log;
- `(due …)` rows.

**The deck keeps the panel's rule that the browser never sends prompt text.**

- A button sends a template id only.
- `TemplateConfig` gains `deck` and `where: new|root`.
- session-start and session-close are root templates, refused while a root lane is live.
- propose, build, fix and ops open the existing New lane dialog.

### The Standing Tee practices (OPS-41, OPS-43–47, PANEL-40)

Each row reads Standing Tee's implementation first, read-only, and ports its behaviour into the template's POSIX sh
at parity, as the 2026-10-01 convergence decision requires.

| Row | What it brings |
|---|---|
| OPS-41 | The gate's run log and state file, `gate-wait.sh`, and the no-background-gate hook. |
| OPS-43 | `reportTo`: outcomes reach the orchestrator, and a worker reads its reviewer's result back (folds in OPS-25). It does not touch `agents/reviewer.md` or the marked review prompt (rule 13). |
| OPS-44 | The soft cap of 5 review rounds, and the frozen role tables (folds in OPS-23). |
| OPS-45 | `running · gating · open · merged today` on the main checkout's status line. A number it cannot read shows `?`, never 0. |
| OPS-46 | Practice records (`PRAC-NNNN`) beside ADRs, and requests priced before they are relayed. |
| OPS-47 | The gate merges `origin/main` in under its lease ("PUSH BEFORE MERGING"), and merge-guard rule 14 requires every PR's head to contain `origin/main`. On a conflict, a dirty tree, a detached HEAD or a Dependabot branch it refuses and changes nothing. |
| PANEL-40 | Lane outcomes: when a lane's claude ends, is killed or waits, the panel writes an outcome, and a hook wakes the orchestrator to push it. |

Three Claude Code behaviours are unverified and are probed before their designs are approved (rule 2):

- `asyncRewake` hooks;
- `agent_id` in a subagent's hook payload;
- PushNotification under `-p`.

## In an attached repository

Attach mode (PANEL-42, `PRD-attach.md`) means a repository can use the panel with none of Clauductor's model installed
in it. Standing Tee is the first. Each MAPS row works there as follows, and its proposal designs for both kinds of
repository.

| Row | In an attached repository |
|---|---|
| OPS-38, OPS-39 | Not used. The repository's cards run its own roadmap parser, so Standing Tee's derived order is its own. |
| OPS-40 | No `map.sh`. Schema v1 allows a map the panel builds from what it already reads: lanes, worktrees, and the repository's `.claude/skills`, `.claude/agents` and settings hooks. Rule, control and record layers are empty, and the page says why. |
| PANEL-35, PANEL-36 | Draw that panel-built map. The view is never an empty page. |
| OPS-42 | The registry may live beside the sidecar config (`~/.clauductor/projects/<id>/routines.json`), with its run log there too. Its commands run in the repository, which it never writes to. |
| PANEL-37 | Ticks the sidecar registry. The trust that covers the sidecar config covers it too. |
| PANEL-38 | Works unchanged. Its sources are the panel's: the spend ledger, merged PRs and the routine log. With no gate run log, the gate row is absent, not zero. |
| PANEL-39 | Works unchanged. Deck templates come from the sidecar config. |
| PANEL-40 | Works unchanged for lanes. The wake hook is the template's, so an attached repository's orchestrator is woken only if it has one of its own. |
| OPS-41, OPS-43 to OPS-47 | Template practices. They reach an installed repository, never an attached one. Standing Tee already has its own versions. |

## Phase 2: the Builder tier (OPS-48, PANEL-41)

**When.** The owner subscribes for one month when phase 1 ends, after reading Patreon's terms. Until then the two rows
sit at the end of Phase 2, so nothing waits on the purchase.

**The clean-room rule.** Clauductor ships publicly, and paid source cannot be redistributed.

- The source lives outside every repository, worktree and plugin input path.
- **OPS-48:** one session studies it and writes `docs/prds/active/PRD-maps-phase2.md`, a behaviour-only spec with no
  code, no identifiers and no copied values. It covers:
  - his layouts;
  - force tuning and label collision;
  - the 3D orbit's projection;
  - canvas performance;
  - his data build and runner caps;
  - heatmap ramps;
  - the card and keyboard behaviour.
- **PANEL-41:** a session that never saw the source builds from that spec: the circle and orbit layouts, and what else
  the study finds worth having.
- The PRD records which session saw what. The phase ends with its checkpoint tag, and the subscription is cancelled.

## What we leave out

| Left out | Why |
|---|---|
| Syncing two machines through git every five minutes | It fights one branch per change and the gate's clean-tree evidence. |
| A rented server running the subscription | Routines run locally, and cloud routines can follow. |
| A voice bot | Outbound notification already exists. |
| Using any harness but Claude Code itself on the subscription login | `claude -p` is Claude Code; the Agent SDK or another harness with that login is not. |

## Open

- **Caps scope:** whether the machine-wide daily dollar cap lives in the panel (ADR-0003) or only per project.
  OPS-42's design decides.
- **Critical-path weight:** counted in rows (recommended) or by `Budget:`. OPS-39 decides.
- **Typing a deck skill into an existing idle lane:** deferred past PANEL-39.
