# CLAUDE.md — Clauductor Framework Development

This is the **framework repo** for Clauductor, a multi-worker orchestration framework for Claude Code.

**Do not confuse this with project-level CLAUDE.md** — that lives in `template/CLAUDE.md` and is what end-user projects receive.

## Architecture

```
claude-dev-framework/
├── framework/           ← Go source (CLI + HUD binary)
│   ├── cmd/clauductor/  ← CLI entrypoint
│   └── internal/        ← packages: state, hud, spawn, config
├── template/            ← Project template (copied by `clauductor init`)
│   ├── CLAUDE.md        ← Project-level Claude Code instructions
│   ├── .claude/skills/  ← Orchestration skills
│   ├── .claude/agents/  ← Agent definitions
│   └── docs/            ← Project doc templates
├── docs/                ← Framework's own docs & PRDs
└── install.sh           ← Build + install script
```

## Build & Run

```bash
# Build the clauductor binary
cd framework && go build -o clauductor ./cmd/clauductor

# Run tests
cd framework && go test ./...

# Install globally
./install.sh
```

## Tech Stack

- **Go** — CLI, HUD (Bubble Tea), state management
- **SQLite** — runtime state (workers, locks, events)
- **tmux** — session management substrate
- **Bubble Tea** — TUI framework for the HUD

## Naming Convention

Same M#.#.# format as projects. See `docs/project-naming-standards.md` in `template/`.

## Git Workflow

- Branch: `feature/M#.#.#-brief-kebab-case`
- Commit: `M#.#.#:` imperative mood. No Co-Authored-By.

## Key Files

- `docs/prds/active/PRD-orchestration-framework.md` — master PRD
- `template/` — what projects receive (modify with care)
- `framework/` — Go source code

## Code Standards

```go
// Comments explain WHY, not WHAT
// TODOs must include milestone context: TODO (M4): description
```
