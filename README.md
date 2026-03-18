# Claude Dev Framework

A structured development framework for building software with [Claude Code](https://claude.ai/code). Provides session management, documentation patterns, knowledge capture, and executable workflows via Claude Code skills.

## What This Is

A battle-tested methodology developed over 87 sessions and 274+ hours of AI-assisted development. It enforces consistency, captures institutional knowledge across sessions, and prevents the entropy that accumulates in long-running projects.

## Quick Start

### Option A: Start a new project
1. Click **"Use this template"** on GitHub (or clone this repo)
2. Open `CLAUDE.md` and complete the Setup Checklist
3. Run `/session-start` in Claude Code

### Option B: Add to an existing project
1. Copy `.claude/` and `docs/` into your project root
2. Copy `CLAUDE.md` into your project root (merge with existing if needed)
3. Open `CLAUDE.md` and complete the Setup Checklist
4. Run `/session-start` in Claude Code

## The Framework: 6 Pillars

### 1. Session Management
Every session starts with `/session-start` which loads context docs, checks git state, and reports current status. This rebuilds context that would otherwise be lost between conversations.

### 2. Documentation System (7 Core Docs)
| Doc | Purpose |
|-----|---------|
| `current-story.md` | **Source of truth** — active milestones, priority queue |
| `next-prompt.md` | Implementation guidance for current work |
| `roadmap.md` | Milestone sequence and planning accuracy |
| `requirements.md` | Feature requirements traceability |
| `project-index.md` | Central navigation hub |
| `insights-log.md` | Technical observations (triage inbox) |
| `development-journal.md` | Narrative chronicle of decisions and learning |

### 3. Knowledge Capture (3 Tiers)
- **Insights log** — quick observations, promotes to learning notes at 3+ related
- **Learning notes** — milestone-level implementation narratives
- **ADRs** — formal architecture decisions with context and trade-offs

### 4. Skills (Executable Workflows)
Skills enforce conventions automatically. Instead of documenting "always use M#.#.# in commits" and hoping it happens, the `/commit` skill enforces it.

| Category | Skills |
|----------|--------|
| **Workflow** | session-start, new-milestone, build, commit, pr, release-prep |
| **Documentation** | dev-journal, log-insight, milestone-complete |
| **Pre-development** | prd-audit, architecture-audit |

### 5. Agents (Composite Skills)
- `pre-implementation` — chains architecture checks before starting work
- `session-wrap` — chains journal + insights + commit at end of session

### 6. Naming Convention (M#.#.#)
A 3-level hierarchy that scales to any project:
```
M1       = Major Feature (e.g., "Authentication System")
M1.2     = Component (e.g., "OAuth Integration")
M1.2.3   = Task (e.g., "Google Provider Setup")
```

## Project-Specific Configuration

Three skills need project-specific setup before use:

| Skill | What to configure |
|-------|-------------------|
| `/build` | Your build command (npm, cargo, xcodebuild, etc.) |
| `/release-prep` | Your deployment pipeline (if any) |
| `/architecture-audit` | Your project's architectural rules to enforce |

The framework reminds you about unconfigured items during `/session-start`.

## Memory System

The framework includes guidance for Claude Code's persistent memory system (`~/.claude/projects/.../memory/`). Memory captures:
- **User preferences** — how you like to work
- **Feedback** — corrections and confirmed approaches
- **Project context** — ongoing work, goals, deadlines
- **References** — pointers to external resources

## Philosophy

- **Process-as-code beats process-as-prose** — Skills enforce conventions; docs explain why
- **Progressive disclosure** — Load only what's needed per session; archive the rest
- **Knowledge promotion** — Insight → Learning Note → ADR (or CLAUDE.md rule)
- **Single source of truth** — Status lives in one place; everything else links to it
- **Every commit could be the last** — Insights and journal must be current, always

## Origin

Developed during the building of [Forager](https://github.com/rfhayn/forager), an iOS grocery and recipe management app. The framework evolved organically from M1 through M17, with each pain point becoming a skill or convention.

## License

MIT
