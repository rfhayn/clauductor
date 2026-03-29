# Project Name

> Created with [Clauductor](https://github.com/your-org/claude-dev-framework) — a multi-worker orchestration framework for Claude Code.

## Getting Started

```bash
# Start the orchestrator
clauductor start

# Or start a single Claude Code session with framework skills
claude
/session-start
```

## Project Structure

```
├── CLAUDE.md              ← Claude Code instructions
├── .claude/skills/        ← Clauductor skills (coordination protocol)
├── docs/                  ← Project documentation
│   ├── current-story.md   ← Active milestones (source of truth)
│   ├── next-prompt.md     ← Implementation guidance hub
│   └── prds/              ← Product requirement documents
└── orchestration/         ← Runtime state (gitignored)
```

## Key Commands

| Command | Purpose |
|---|---|
| `/session-start` | Begin a session (mandatory) |
| `/new-milestone PREFIX-#.# description` | Start a new milestone |
| `/claim PREFIX-#.#` | Claim work and lock files |
| `/commit` | Commit with project conventions |
| `/status` | View orchestration status |
