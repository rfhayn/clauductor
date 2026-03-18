# Claude Dev Framework

A structured development framework for building software with [Claude Code](https://claude.ai/code). Session management, knowledge capture, and executable workflows that keep AI-assisted projects consistent across dozens or hundreds of sessions.

## The Problem

Claude Code is powerful, but every new conversation starts from zero. Context is lost. Naming drifts. The same mistakes get re-learned. Documentation falls behind. Over a long project, entropy wins.

This framework fixes that. It gives Claude Code a structured memory — documentation patterns, executable skills, and knowledge capture workflows that maintain consistency from session 1 to session 100+.

## What You Need

- **[Claude Code](https://claude.ai/code)** (Anthropic's CLI tool for Claude) — this framework is built specifically for it
- **Git** — the framework uses feature branches, structured commits, and PRs
- **GitHub CLI (`gh`)** — for PR creation (optional but recommended)
- That's it. No other dependencies. Works with any language, framework, or platform.

## Quick Start

### Option A: New project
1. Clone or download this repo
2. Rename the folder to your project name
3. Open it in Claude Code
4. Run `/session-start` — it walks you through setup

### Option B: Existing project
1. Copy `.claude/`, `docs/`, and `CLAUDE.md` into your project root
2. Open it in Claude Code
3. Run `/session-start` — it detects unconfigured items and tells you what to set up

### First session
1. `/session-start` loads context and checks for setup items
2. Complete the Setup Checklist in `CLAUDE.md` (build command, test command, etc.)
3. `/new-milestone M1 Your First Feature` creates your branch and docs
4. Build. `/commit` when ready. `/pr` when done. `/milestone-complete M1` to wrap up.

## What's Included

### 12 Skills (Executable Workflows)

Skills enforce project conventions automatically — naming, commit format, documentation updates. Instead of hoping rules get followed, the skills enforce them.

| Skill | Purpose |
|-------|---------|
| `/session-start` | Load context, check git, report status |
| `/new-milestone` | Create feature branch + update docs |
| `/build` | Build project *(configure for your stack)* |
| `/commit` | Commit with M#.#.# conventions |
| `/pr` | Create structured pull request |
| `/release-prep` | Deployment pipeline *(configure for your stack)* |
| `/dev-journal` | Write session narrative |
| `/log-insight` | Capture technical observation |
| `/milestone-complete` | Update all docs + retrospective |
| `/prd-audit` | Verify spec against current code |
| `/architecture-audit` | Check for violations *(configure your rules)* |
| `/skills` | List all available skills |

### 7 Core Documentation Templates

| Doc | Purpose |
|-----|---------|
| `current-story.md` | **Source of truth** — active milestones, priority queue |
| `next-prompt.md` | Implementation guidance for current work |
| `roadmap.md` | Milestone sequence and planning accuracy |
| `requirements.md` | Feature requirements traceability |
| `project-index.md` | Central navigation hub |
| `insights-log.md` | Technical observations (triage inbox) |
| `development-journal.md` | Narrative chronicle of decisions and learning |

### 2 Composite Agents

- **pre-implementation** — runs architecture checks before starting work
- **session-wrap** — ensures journal + insights are captured before final commit

### Naming Convention (M#.#.#)

A 3-level hierarchy that works for any project:
```
M1       = Major Feature (e.g., "Authentication System")
M1.2     = Component (e.g., "OAuth Integration")
M1.2.3   = Task (e.g., "Google Provider Setup")
```

### Knowledge Capture Pipeline

```
Quick observation → Insights Log (table row)
3+ related insights → Learning Note (narrative doc)
Architectural decision → ADR (formal record)
Recurring pattern → CLAUDE.md (loaded every session)
```

## Project-Specific Configuration

Three skills ship as stubs — configure them for your stack:

| Skill | What to configure |
|-------|-------------------|
| `/build` | Your build command (npm, cargo, xcodebuild, gradle, etc.) |
| `/release-prep` | Your deployment pipeline (if any) |
| `/architecture-audit` | Your project's architectural rules to enforce |

`/session-start` reminds you about unconfigured items until they're set up.

## Philosophy

- **Process-as-code beats process-as-prose** — Skills enforce conventions; docs explain why
- **Progressive disclosure** — Load only what's needed per session; archive the rest
- **Knowledge promotion** — Insights promote to learning notes promote to ADRs
- **Single source of truth** — Status lives in one place; everything else links to it
- **Every commit could be the last** — Documentation must be current, always

## Origin

Built over 87 sessions and 274+ hours developing [forager](https://github.com/rfhayn/forager), an iOS app. Every skill and convention exists because its absence caused a real problem — duplicate services, naming drift, lost insights, documentation that was always "I'll update it later." The framework evolved from pain, not theory.

## License

MIT
