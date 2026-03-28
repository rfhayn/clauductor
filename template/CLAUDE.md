# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Setup Checklist

> **Complete these items before your first development session.**
> Run `/session-start` — it will remind you of any unconfigured items.

- [ ] **Build command**: Replace the placeholder in Build & Run below
- [ ] **Test command**: Replace the placeholder in Build & Run below
- [ ] **Project description**: Update the Architecture section with your project's structure
- [ ] **Deployment pipeline**: Configure `/release-prep` skill if you have a deployment target
- [ ] **Architecture audit rules**: Configure `/architecture-audit` with your project's patterns
- [ ] **First milestone**: Create your first M#.#.# entry in `docs/current-story.md`

## Session Startup (MANDATORY)

Run `/session-start` at the beginning of every session. No exceptions.

## Build & Run

```bash
# TODO: Replace with your project's build command
# Examples:
#   npm run build
#   cargo build
#   xcodebuild -project MyApp.xcodeproj -scheme MyApp build
#   ./gradlew build
echo "BUILD COMMAND NOT CONFIGURED — update CLAUDE.md"
```

```bash
# TODO: Replace with your project's test command
# Examples:
#   npm test
#   cargo test
#   pytest
#   ./gradlew test
echo "TEST COMMAND NOT CONFIGURED — update CLAUDE.md"
```

## Naming Convention (Zero Tolerance)

Always use **M#.#.# format** in all code, commits, docs, and branches:

```
M1       = Major Feature
M1.2     = Component within that feature
M1.2.3   = Task within that component
```

Never use "Phase 3", "Step 3", or "Story 1.2.3".

Status indicators: `COMPLETE` | `ACTIVE` | `READY` | `PLANNED`

## Architecture

> TODO: Describe your project's architecture here. For larger projects,
> create `docs/architecture/README.md` and link to it.
>
> Key areas to document:
> - Tech stack and frameworks
> - Data layer (database, ORM, API clients)
> - Service/business logic layer
> - UI/presentation layer
> - Key patterns and conventions

## Pre-Development Analysis

Before implementing ANY feature, use these skills as needed:

- `/prd-audit <path>` — Required if PRD is >2 weeks old
- `/architecture-audit` — Required before work touching core patterns (configure rules first)
- Verify correct M#.#.# format in current-story.md
- Follow established patterns and ADR decisions

## Git Workflow

**One phase = one branch = one PR = one squash commit to main.**

- Branch: `feature/M#.#.#-brief-kebab-case` (3-5 words max)
- Commit: `M#.#.#:` imperative mood. **No Co-Authored-By.**
- Commit every 15-30 min, push after each commit
- Skills: `/new-milestone`, `/commit`, `/pr`, `/build`

## Documentation Updates (After Every Session)

**`current-story.md` is the single source of truth for status.**

- `/log-insight <topic> <insight>` — Log insights IMMEDIATELY
- `/dev-journal` — Session narrative (MANDATORY before every commit)
- `/milestone-complete <M#.#.#>` — Update docs after milestone completion

## Project Skills

| Skill | Purpose |
|-------|---------|
| `/session-start` | Startup checklist |
| `/new-milestone` | Set up milestone (branch + docs) |
| `/build` | Build project (⚠️ configure first) |
| `/commit` | Commit with M#.#.# conventions |
| `/pr` | Create PR with project format |
| `/release-prep` | Deployment pipeline (⚠️ configure first) |
| `/dev-journal` | Session narrative entry |
| `/log-insight` | Log technical insight |
| `/milestone-complete` | Update docs after completion |
| `/prd-audit` | Verify PRD against code |
| `/architecture-audit` | Check for violations (⚠️ configure first) |

## Code Standards

```
// Comments explain WHY, not WHAT
// TODOs must include milestone context: TODO (M4): description
```

## Quality Gates

**Stop and reassess if:** >5 consecutive build errors, >20 min on one issue, breaking existing features, working on main branch without a feature branch.
