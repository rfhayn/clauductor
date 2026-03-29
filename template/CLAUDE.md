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
- [ ] **First milestone**: Create your first PREFIX-#.# entry in `docs/current-story.md`

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

## Naming Convention

Use **PREFIX-#.#** format in all code, commits, docs, and branches:

```
AUTH        = Epic (domain area, defined in prefix registry below)
AUTH-1      = Feature within that epic
AUTH-1.3    = Task within that feature
```

Never use "Phase 3", "Step 3", or unlabeled numbers.

### Prefix Registry

> Update this table as epics are created. Prefixes should be 2-5 uppercase characters.

| Prefix | Domain |
|--------|--------|
| _TODO_ | _Define your first epic prefix_ |

### Branch & Commit Format

```
Branch:  feature/AUTH-1.3-brief-description
Commit:  AUTH-1.3: imperative mood description
```

### Depth Flexibility

Choose the granularity that fits the work:
- `AUTH-1` — for features that don't need sub-task tracking
- `AUTH-1.3` — for tasks within a feature
- Sub-tasks below that are checklist items in the task file, not separate IDs

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
- Verify correct PREFIX-#.# format in current-story.md
- Follow established patterns and ADR decisions

## Git Workflow

**One phase = one branch = one PR = one squash commit to main.**

- Branch: `feature/PREFIX-#.#-brief-kebab-case` (3-5 words max)
- Commit: `PREFIX-#.#:` imperative mood. **No Co-Authored-By.**
- Commit every 15-30 min, push after each commit
- Skills: `/new-milestone`, `/commit`, `/pr`, `/build`

## Documentation Updates (After Every Session)

**`current-story.md` is the single source of truth for status.**

- `/log-insight <topic> <insight>` — Log insights IMMEDIATELY
- `/dev-journal` — Session narrative (MANDATORY before every commit)
- `/milestone-complete <PREFIX-#.#>` — Update docs after milestone completion

### Split Next-Prompt Pattern

Implementation guidance uses per-milestone files:
- `docs/next-prompt.md` — Hub/index with pointers to active milestones
- `docs/next-prompt-PREFIX-#.md` or `docs/next-prompt-PREFIX-#.#.md` — Per-milestone guidance
- Created by `/new-milestone`, deleted by `/milestone-complete`
- File name matches the milestone ID you pass (e.g., `next-prompt-AUTH-1.md` or `next-prompt-AUTH-1.3.md`)

## Project Skills

### Development Skills
| Skill | Purpose |
|-------|---------|
| `/session-start` | Startup checklist + worker registration |
| `/new-milestone` | Create branch + per-milestone next-prompt |
| `/build` | Build project (⚠️ configure first) |
| `/commit` | Commit with PREFIX-#.# conventions |
| `/pr` | Create PR with project format |
| `/release-prep` | Deployment pipeline (⚠️ configure first) |
| `/dev-journal` | Session narrative entry |
| `/log-insight` | Log technical insight |
| `/milestone-complete` | Update docs + clean up next-prompt file |
| `/prd-audit` | Verify PRD against code |
| `/architecture-audit` | Check for violations (⚠️ configure first) |

### Orchestration Skills (Clauductor)
| Skill | Purpose |
|-------|---------|
| `/claim` | Declare session type + lock files |
| `/release` | Release locks, deregister worker |
| `/blocked` | Report block, start wait/escalation |
| `/status` | Quick orchestration status |
| `/supervisor` | Launch orchestration (HUD + dispatch) |
| `/spawn` | Start new Claude Code session |
| `/handoff` | Structured handoff between workers |

## Code Standards

```
// Comments explain WHY, not WHAT
// TODOs must include milestone context: TODO (AUTH-2): description
```

## Quality Gates

**Stop and reassess if:** >5 consecutive build errors, >20 min on one issue, breaking existing features, working on main branch without a feature branch.
