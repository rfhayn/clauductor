# PRD: Clauductor — Multi-Worker Orchestration Framework

**Author**: Rich
**Status**: DRAFT
**Created**: 2026-03-28
**Last Updated**: 2026-03-28

---

## Problem Statement

Solo developers and small teams using Claude Code have no way to coordinate multiple concurrent workstreams. The current options are:

1. **Manual** — run multiple Claude Code sessions, manually track who's working on what, hope they don't edit the same files
2. **claude-squad** — a tmux-based process manager that runs agents in parallel but has no inter-agent coordination, no file locking, no shared context, and no intelligence in the orchestration layer

The result: developers underutilize Claude Code's capacity. You could have 3 agents building different features while you review a fourth, but the coordination overhead makes it impractical.

## Vision

Evolve claude-dev-framework from a single-session development methodology into a **multi-worker orchestration framework** where:

- Multiple Claude Code sessions (human or agent) work simultaneously on the same codebase
- A supervisor session dispatches work, monitors progress, and resolves conflicts
- A real-time HUD shows who's working on what, which files are locked, and what's blocked
- The entire system runs as Claude Code skills + a lightweight state viewer — no API keys, no external services, no platform lock-in

## Users

| User Type | Description |
|---|---|
| **Solo developer** | One human running 1-3 agent sessions in parallel, reviewing output |
| **Small team** | 2-4 developers, each with their own agent(s), coordinating on shared codebase |
| **Enterprise team** | Larger team with compliance needs — prompt logging, audit trails, role-based access |

The framework should work naturally for a solo developer and scale to a team without architectural changes.

## Core Concepts

### Session Types

Every Claude Code session running the framework declares a session type on startup:

**Research/Spike Session**
- Purpose: learning, research, planning, architecture exploration
- File access: read anything, modify docs only, warned before touching source code
- Locking: none — does not lock files
- Output: notes, findings, recommendations in docs/
- HUD display: shows as SPIKE/RESEARCH with no lock indicators

**Build/Test Session**
- Purpose: implementing features, writing tests, fixing bugs
- File access: full access, but must declare file manifest before starting
- Locking: claims files from manifest, other build sessions blocked from those files
- PRD-driven: must have a next-prompt-M#.#.md with defined scope before starting
- HUD display: shows as BUILD with locked file list and progress

### File Manifest & Locking

When a build session starts (via `/claim` or `/build-start`):

1. The skill analyzes the PRD/next-prompt-M#.#.md
2. Generates a list of files the session intends to modify
3. Presents the manifest to the user/agent for approval
4. Locks those files in the shared state
5. Other build sessions attempting to modify locked files are blocked

Lock behavior when a session hits a locked file:
- Immediately notifies the user: "src/auth.ts locked by [worker] on M1.2"
- Waits, checking every 30 seconds for lock release
- At 15 minutes, escalates: notifies user with options (keep waiting, reassign, force claim)
- Continues waiting unless user intervenes
- On lock release: automatically resumes

### Workers

A worker is any Claude Code session registered with the framework. Workers have:
- **ID**: unique identifier (see naming below)
- **Session type**: research/spike or build/test
- **Milestone**: which M#.# they're working on
- **File manifest**: files they've claimed (build sessions only)
- **Status**: active, blocked, idle, completed
- **Duration**: how long they've been active

#### Worker Identity

- **Human workers**: name set on first `/session-start` (e.g., `rich`), persisted in config
- **Agent workers**: dynamically named with structure: `{human-owner}-agent-{project}-{timestamp}`
  - Example: `rich-agent-myapp-0328-1423`
  - Always traces back to the human user who spawned the process
  - Timestamp ensures uniqueness across sessions

#### Agent Autonomy Limits

Agents operating in build/test sessions can:
- Read any file
- Modify files within their claimed manifest
- Commit changes
- Build and run tests
- Force-claim locked files (with logging)

Agents **cannot**:
- Delete files or branches
- Create or merge pull requests
- Push to remote (human approval required)
- Modify files outside their claimed manifest without re-claiming

### The Supervisor

The supervisor is a Claude Code session running the `/supervisor` skill. It:
- Launches the HUD viewer process
- Accepts natural language commands from the user (via HUD input or directly in its Claude Code session)
- Spawns new Claude Code sessions with appropriate context
- Monitors worker status via shared state
- Can reassign work, release locks, kill stuck sessions
- Logs all orchestration decisions

The supervisor uses Claude's intelligence through normal Claude Code operation — no separate API calls.

**Dual interaction model**: The user can talk to the supervisor through the HUD command input (quick commands, dispatching) or switch to the supervisor's own Claude Code session for deeper conversation (planning, complex decisions). Both update the same state.

**Session persistence**: If the supervisor session dies, state is preserved in SQLite. A new `/supervisor` session can resume — it reads the worker registry, active locks, and event log to reconstruct context. Worker sessions are unaffected by supervisor restarts since they coordinate through shared state, not direct communication.

### Shared State (SQLite)

All live coordination state lives in a SQLite database:

```
orchestration/
  framework.db        ← workers, locks, events, milestones
  prompts/            ← session prompt logs (optional)
```

Tables:
- `workers` — registered sessions, their type, status, milestone
- `locks` — file locks with owner, timestamp, milestone
- `events` — append-only orchestration log
- `milestones` — status, assignment, progress

SQLite was chosen because: concurrent reads, atomic writes, zero config, portable, queryable, no daemon required.

The `orchestration/` directory is gitignored — it's runtime state, not project state.

Additional runtime file:
- `orchestration/.session-status` — lightweight pipe-delimited status for the Claude Code status line: `MILESTONE|SESSION_TYPE|WORKER_NAME|DESCRIPTION`. Updated by `/session-start`, `/claim`, and other skills. Read by the status line script (no SQLite dependency for fast shell reads).

### Claude Code Status Line

Each Clauductor project includes `.claude/statusline.sh` and a `statusLine` config in `.claude/settings.json`. This displays the current milestone, session type, and context usage in Claude Code's bottom bar:

```
[M1.2] BUILD auth provider | 34% ctx
```

The status line reads from `orchestration/.session-status` (primary) with fallback to parsing the git branch name. Skills update the status file automatically — no manual maintenance.

The `clauductor install` command configures this for existing projects.

### The HUD

A separate lightweight TUI process that reads from SQLite and renders in real-time. Part of this repo, installed as part of framework setup.

The HUD operates in two modes:

**Dashboard mode** (default):
- **Workers panel**: all active sessions, their type, milestone, duration, status
- **File locks panel**: which files are locked by whom
- **Activity feed**: recent orchestration events (claims, releases, spawns, blocks)
- **Milestones panel**: progress on each active milestone, who's assigned
- **Command input**: send commands to the supervisor (natural language or skill invocations)

**Session mode** (toggle into a worker):
- View and interact with a specific Claude Code session
- Toggle with a keypress (e.g., `1`, `2`, `3` or tab to cycle)
- Press `Esc` or `~` to return to dashboard
- Powered by tmux: all sessions (including the HUD) run inside a tmux session managed by the framework
- Toggle switches the active tmux pane — seamless, proven UX

The HUD is both a viewer and a control surface — you can dispatch work, approve actions, and switch between sessions without leaving it.

Tech: Go + Bubble Tea (single binary, cross-platform, proven in this space). tmux is a **required dependency** — the framework manages a tmux session that contains the HUD pane and all worker panes. This is the orchestration substrate.

### Git Integration

Each build session works on its own branch (existing convention):
- Branch: `feature/M#.#.#-brief-kebab-case`
- Git worktrees for parallel branch work without constant switching
- Locks prevent file conflicts within a branch; git prevents conflicts across branches
- The framework does not replace git — it adds a coordination layer on top

### Orchestration Logging

All orchestration events are logged:
- Session start/stop
- Claims, locks, releases
- Blocks and resolutions
- Spawns and kills
- Supervisor commands and decisions

Optional prompt logging captures the full prompt history per session for audit/compliance.

### Code Review (/review)

The framework includes a built-in review skill that checks convention compliance before PRs:

**Checks performed:**
- Naming convention (branch format, commit PREFIX-#.# prefix, imperative mood)
- Manifest compliance (files changed vs files claimed, orchestration mode only)
- Orchestration hooks (modified skills have event logging)
- Documentation currency (journal, current-story, next-prompt)
- Code quality (secrets detection, TODOs without milestone context, debug statements)
- Commit format (length, bullet details, commit count)

**Two modes:**
- `/review` — local branch, pre-PR
- `/review <PR#>` — remote PR via `gh pr diff`

**Verdict:** CLEAN | READY FOR PR (N warnings) | NEEDS FIXES (N failures)

The review supplements human review — it catches convention drift and process compliance, not logic bugs.

## Skill Architecture

### New Skills

| Skill | Purpose |
|---|---|
| `/supervisor` | Main orchestration loop — HUD, dispatch, monitoring |
| `/spawn <type> <milestone> [description]` | Start a new Claude Code session with context |
| `/claim <M#.#.#>` | Declare session type, generate file manifest, lock files |
| `/release` | Release locks, deregister worker |
| `/blocked` | Report a block, start wait/escalation cycle |
| `/handoff <target-worker>` | Structured handoff with context between sessions |
| `/status` | Quick inline status for worker sessions |
| `/review [PR#]` | Pre-PR code review for convention compliance, manifest adherence, code quality |
| `/assign [N]` | Auto-dispatch unclaimed milestones to agents (supervisor-only) |

### Modified Existing Skills

| Skill | Change |
|---|---|
| `/session-start` | Registers worker with sidecar, loads branch-specific next-prompt-M#.#.md |
| `/new-milestone` | Creates next-prompt-M#.#.md (split pattern), registers milestone in sidecar |
| `/commit` | Logs commit event to orchestration log |
| `/milestone-complete` | Releases locks, archives next-prompt-M#.#.md, deregisters workers, removes hub pointer |
| `/pr` | Suggests running /review before PR creation |
| `/claim` | Supports `auto` argument for agent self-assignment from priority queue |
| `/spawn` | Supports `auto` milestone for self-assigning agents |

### Split Next-Prompt Pattern (prerequisite)

Before orchestration, implement the split next-prompt file pattern:
- `docs/next-prompt.md` becomes a hub/index with pointers to active milestones
- `docs/next-prompt-M#.#.md` files hold per-milestone implementation guidance
- Created by `/new-milestone`, archived by `/milestone-complete`
- Major.minor naming (M1.2, not M1.2.3) — sub-tasks share one file

## Repository Structure

The framework repo separates **source** (framework tooling) from **template** (what projects get). Projects never contain framework source code.

```
claude-dev-framework/               ← FRAMEWORK REPO (development only)
├── CLAUDE.md                       ← framework's own dev instructions
├── framework/                      ← compiled tooling source
│   ├── cmd/
│   │   ├── clauductor/              ← single binary: CLI + HUD
│   │   │   └── main.go
│   │   └── ...
│   ├── internal/
│   │   ├── state/                  ← SQLite operations
│   │   ├── hud/                    ← Bubble Tea TUI
│   │   ├── spawn/                  ← session spawning logic
│   │   └── config/
│   ├── go.mod
│   └── go.sum
├── template/                       ← copied to new projects via `clauductor init`
│   ├── CLAUDE.md                   ← project-level instructions
│   ├── .claude/
│   │   └── skills/                 ← coordination protocol
│   │       ├── supervisor/
│   │       ├── spawn/
│   │       ├── claim/
│   │       ├── release/
│   │       ├── blocked/
│   │       ├── handoff/
│   │       ├── status/
│   │       ├── session-start/
│   │       ├── new-milestone/
│   │       ├── commit/
│   │       └── milestone-complete/
│   ├── docs/
│   │   ├── current-story.md
│   │   ├── next-prompt.md          ← hub/index
│   │   ├── project-naming-standards.md
│   │   ├── roadmap.md
│   │   ├── requirements.md
│   │   ├── insights-log.md
│   │   ├── development-journal.md
│   │   └── prds/
│   │       ├── active/.gitkeep
│   │       ├── complete/.gitkeep
│   │       └── archive/.gitkeep
│   ├── .gitignore                  ← includes orchestration/
│   └── README.md                   ← project readme template
├── docs/                           ← framework's own docs & PRDs
│   └── prds/
├── install.sh                      ← build + install framework binary
├── README.md                       ← framework readme
└── LICENSE
```

**A project** created via `clauductor init` gets only the `template/` contents:

```
my-project/                         ← YOUR PROJECT
├── CLAUDE.md
├── .claude/skills/                 ← skills (from template)
├── docs/                           ← project docs (from template)
├── orchestration/                  ← runtime state (gitignored, created at init)
│   ├── framework.db                ← SQLite: workers, locks, events
│   └── prompts/                    ← session prompt logs (optional)
└── src/                            ← your code
```

No framework source code. No Go files. Just skills, docs, and your code.

### Distribution & Install

```bash
# One-time: clone and install the framework
git clone <repo> ~/claude-dev-framework
cd ~/claude-dev-framework && ./install.sh

# install.sh:
#   1. Checks prerequisites: Go toolchain, tmux
#   2. Builds clauductor binary
#   3. Installs to ~/.local/bin/clauductor
#   4. Verifies installation

# Create a new project (empty directory)
clauductor init ~/Development/my-app
#   1. Copies template/ into target directory
#   2. Initializes git if needed
#   3. Creates orchestration/ directory + .gitignore entry
#   4. Initializes framework.db schema
#   5. Ready to go

# Install into an existing repo
cd ~/Development/existing-project
clauductor install
#   1. Detects existing project structure
#   2. Categorizes template files into three tiers:
#      FRAMEWORK files (skills, agents, settings, statusline):
#        → Always install/overwrite — these ARE the framework
#      DOC TEMPLATES (current-story, journal, next-prompt, roadmap, etc.):
#        → Create only if missing — never overwrite project content
#      CONFIG files (CLAUDE.md, .gitignore):
#        → Merge — add framework sections, preserve existing content
#   3. Creates orchestration/ directory + .gitignore entry
#   4. Initializes framework.db schema
#   5. Supports --dry-run to preview changes

# Update a project's skills/templates (preserves customizations)
clauductor update
#   1. Diffs template/ against project
#   2. Shows changes, applies with interactive merge
#   3. Does not touch user code or docs content
```

### Communication Flow

```
Skill (markdown)
  → shells out to `clauductor` CLI
    → writes to SQLite (orchestration/framework.db)
      → HUD reads SQLite, renders in real-time
```

Skills remain readable markdown. The `clauductor` binary handles all state operations. The HUD is a read-only viewer with input forwarding to the supervisor.

### Skill Hot-Reload

Skills are markdown files read by Claude Code at invocation time — they are not compiled or cached. When `clauductor update` modifies a skill file on disk, the next invocation of that skill in any session picks up the change immediately. No session restart required. This is a natural property of how Claude Code skills work (read from disk on each `/skill` call).

### How Sessions Are Spawned

The `/spawn` skill:
1. Determines the right context (next-prompt-M#.#.md, PRD, etc.)
2. Creates a new tmux pane within the Clauductor session
3. Launches Claude Code in the pane with initial context/prompt
4. The new session runs `/session-start` which auto-registers with the sidecar
5. If build session: runs `/claim` to declare manifest and lock files

All worker sessions live as tmux panes — the HUD can toggle between them directly.

## Milestones

### M1: Repo Restructure — Template/Source Separation
**Status: COMPLETE**
**Scope**: Split repo into framework source + project template
- Move existing skills, docs, CLAUDE.md into `template/`
- Create framework's own root CLAUDE.md for framework development
- Set up `framework/` directory with Go module scaffolding
- Create `install.sh` (build binary + install to PATH)
- Implement `clauductor init` (copy template to new directory, init orchestration/)
- Implement `clauductor install` (add framework to existing repo, non-destructive, conflict prompts)
- Implement `clauductor update` (diff template against existing project, interactive merge)
- Verify: clone → install → init → project works with existing skills
- Verify: clone → install → install (existing repo) → framework integrates cleanly

### M2: Split Next-Prompt Pattern
**Status: COMPLETE**
**Scope**: Foundation for multi-milestone awareness
- Restructure template's docs/next-prompt.md as hub/index
- Create next-prompt-M#.#.md pattern
- Update /new-milestone, /session-start, /milestone-complete skills in template
- Update template CLAUDE.md references

### M3: State Layer (SQLite + CLI)
**Status: COMPLETE** (22 unit tests passing)
**Scope**: Shared state infrastructure
- Design SQLite schema (workers, locks, events, milestones)
- Add state commands to `clauductor`: register, deregister, lock, unlock, event, query
- `clauductor init` creates and initializes framework.db
- Unit tests for state operations

### M4: Worker Registration & Locking
**Status: COMPLETE** (all 7 orchestration skills + trigger phrases + orchestration hooks)
**Scope**: Core coordination skills
- `/claim` skill — session type declaration, file manifest generation, locking
- `/release` skill — lock release, worker deregistration
- `/blocked` skill — wait/escalation cycle with 15-min escalation
- Modify `/session-start` to auto-register with sidecar
- Modify `/commit` to log events
- Research/spike sessions: warn on source code modification, allow doc commits

### M5: HUD (TUI Viewer)
**Status: 90%** — HUD built with Bubble Tea, keyboard nav, SQLite wired. Pending: real-world tmux testing.
**Scope**: Real-time ASCII dashboard
- Go + Bubble Tea TUI reading from SQLite
- Workers panel, file locks panel, activity feed, milestones panel
- Auto-refresh on state changes
- Keyboard navigation and basic commands
- Launched via `clauductor watch`
- Well-designed terminal/ASCII aesthetic

### M6: Supervisor & Spawning
**Status: 80%** — Skills written, agent self-assignment added. Pending: tmux spawning E2E test.
**Scope**: Orchestration intelligence
- `/supervisor` skill — main orchestration loop
- `/spawn` skill — launch new Claude Code sessions with context
- `/handoff` skill — structured worker-to-worker handoff
- Natural language command parsing in supervisor
- Cross-platform terminal spawning (macOS native, tmux, fallback)
- Agent self-assignment from unclaimed milestones

### M7: Polish & Enterprise Features
**Status: IN PROGRESS** — /review skill, /assign skill, export command, onboarding guide built. Pending: prompt logging, full E2E testing.
**Scope**: Production readiness
- Prompt logging (optional, per-session)
- Orchestration log export/query
- `/assign` skill — auto-dispatch from priority queue
- Documentation and onboarding guide
- `clauductor update` — upgrade project skills from latest framework

## Constraints

- **No API key required** — all intelligence runs through Claude Code sessions (subscription-based)
- **Minimal dependencies** — requires tmux and Go (for building); works in any terminal emulator on top of tmux
- **Portable** — clone the repo, run install, start working
- **Backwards compatible** — a single-milestone, single-worker project should work naturally without orchestration overhead
- **Git-native persistence** — project state (milestones, PRDs, docs) lives in git; only runtime state (locks, workers) lives in SQLite

## Resolved Decisions

| # | Question | Decision |
|---|---|---|
| 1 | **Worker identity** | Humans: name on first session-start. Agents: `{owner}-agent-{project}-{timestamp}` — always traceable to spawning human. |
| 2 | **Multi-repo** | One repo = one framework instance for v1. |
| 3 | **Remote workers** | Local-only for v1. SQLite is single-machine. Networked state is a v2 concern. |
| 4 | **Agent autonomy** | Agents can commit, build, test, force-claim. Cannot delete, PR, or push. |
| 5 | **Session persistence** | Yes — state is in SQLite. Supervisor can die and resume. Workers are unaffected. |
| 6 | **HUD interaction** | Both — HUD command input for quick dispatch, supervisor's Claude Code session for deeper conversation. HUD toggles into sessions via tmux pane switching. |
| 7 | **v1 scope** | M1-M7 (all milestones). Full orchestration framework. |
| 8 | **Product name** | Clauductor. CLI binary: `clauductor`. |
| 9 | **Skill hot-reload** | Yes — skills are read from disk on each invocation. `clauductor update` changes take effect immediately, no restart needed. |
| 10 | **Existing repos** | `clauductor install` adds framework to existing repos non-destructively. |
| 11 | **tmux** | Required dependency. All sessions (HUD + workers) run as tmux panes in a Clauductor-managed tmux session. |

| 12 | **Naming conflicts** | `clauductor install` prompts for each conflict, recommends using framework version with warning that custom overrides may break functionality. No auto-merge. |
| 13 | **tmux session naming** | `clauductor-{project-dirname}`. Supports multiple concurrent Clauductor-managed projects, each in its own tmux session. |
| 14 | **Claude Code version** | `clauductor` checks for minimum Claude Code version on install/init and warns if incompatible. |
| 15 | **Naming convention** | PREFIX-#.# format (e.g., AUTH-1.3) with configurable prefix registry in CLAUDE.md. Replaces M#.#.# for new projects. Legacy M#.#.# supported for existing projects (e.g., forager). Framework's own milestones (M1-M7) use legacy format for historical reasons. |
| 16 | **Code review** | Built-in `/review` skill checks convention compliance, manifest adherence, doc currency, code quality. Two modes: branch (pre-PR) and PR. Orchestration-aware. |
