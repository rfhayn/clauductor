# PRD: Clauductor Integration into Forager

**Author**: Rich
**Status**: READY
**Created**: 2026-03-29
**Milestone**: FORAGER-MIGRATION (capstone validation)

---

## Problem Statement

Forager is a production iOS app with 94 sessions, 320+ hours, and 40+ completed milestones. It uses a comprehensive single-session workflow with 15 custom skills, 2 agents, 200+ documentation files, and M#.#.# naming. Clauductor adds multi-worker orchestration that forager lacks. The integration must be purely additive — zero data loss, zero workflow disruption.

This is the capstone validation of `clauductor install` on a real, complex project.

## Scope

### In Scope
- Adding 21 Clauductor generic skills alongside existing 15 forager-* skills
- Creating orchestration/ directory with framework.db
- Merging settings.json and CLAUDE.md
- Adding orchestration/ to .gitignore

### Out of Scope
- Renaming any existing forager files, skills, milestones, or branches
- Modifying forager's M#.#.# naming convention
- Replacing forager's agents
- Changing settings.local.json
- Modifying any existing documentation content

## File-by-File Migration Plan

### Files That Will Be CREATED (21 skills + infrastructure)

| File | Notes |
|------|-------|
| `.claude/skills/claim/SKILL.md` | Orchestration: session type + file locking |
| `.claude/skills/release/SKILL.md` | Orchestration: unlock + deregister |
| `.claude/skills/blocked/SKILL.md` | Orchestration: wait/escalation cycle |
| `.claude/skills/status/SKILL.md` | Orchestration: inline status |
| `.claude/skills/supervisor/SKILL.md` | Orchestration: main loop |
| `.claude/skills/spawn/SKILL.md` | Orchestration: launch sessions |
| `.claude/skills/handoff/SKILL.md` | Orchestration: work transfer |
| `.claude/skills/assign/SKILL.md` | Orchestration: auto-dispatch |
| `.claude/skills/review/SKILL.md` | Pre-PR code review |
| `.claude/skills/session-start/SKILL.md` | Generic (forager has forager-session-start) |
| `.claude/skills/commit/SKILL.md` | Generic (forager has forager-commit) |
| `.claude/skills/build/SKILL.md` | Generic (forager has forager-build) |
| `.claude/skills/new-milestone/SKILL.md` | Generic workflow |
| `.claude/skills/milestone-complete/SKILL.md` | Generic workflow |
| `.claude/skills/dev-journal/SKILL.md` | Generic workflow |
| `.claude/skills/log-insight/SKILL.md` | Generic workflow |
| `.claude/skills/pr/SKILL.md` | Generic workflow |
| `.claude/skills/prd-audit/SKILL.md` | Generic workflow |
| `.claude/skills/architecture-audit/SKILL.md` | Generic workflow |
| `.claude/skills/release-prep/SKILL.md` | Generic workflow |
| `.claude/skills/skills/SKILL.md` | Lists all skills |
| `docs/MEMORY-SETUP.md` | New doc |
| `orchestration/framework.db` | SQLite database |
| `orchestration/prompts/` | Prompt log directory |

### Files That Will Be KEPT AS-IS (200+ forager files)

All existing forager docs, skills, agents, and settings.local.json are untouched:
- 15 forager-* skills
- 2 forager agents (pre-implementation, session-wrap)
- All docs/ content (current-story, journal, insights, 16 ADRs, 44 learning notes, 97+ PRDs, etc.)
- settings.local.json (200+ permissions)
- statusline.sh (already Clauductor-compatible)

### Files That Will Be MERGED

| File | Behavior |
|------|----------|
| `CLAUDE.md` | Append Clauductor orchestration section |
| `.gitignore` | Add `orchestration/` entry |

## Pre-Migration Framework Fix

**Agents must be treated as tierDoc** — change `classifyFile` in `framework/internal/cmd/install.go` to treat `.claude/agents/` as create-only-if-missing instead of always-overwrite. This prevents forager's custom agents from being destroyed.

## Milestones

### FORAGER-1: Preparation (15 min)
- Create backup branch: `backup/pre-clauductor-migration`
- Verify `/forager-build` works
- Record pre-migration state

### FORAGER-2: Dry Run (10 min)
- `clauductor install --dry-run`
- Verify: 21 NEW, 0 overwrites, all docs skipped, CLAUDE.md + .gitignore merged

### FORAGER-3: Execute Install (5 min)
- `clauductor install`
- Verify orchestration/ created

### FORAGER-4: Post-Install Customization (10 min)
- Customize `/skills` SKILL.md to list both forager-* and Clauductor skills
- Enhance CLAUDE.md Clauductor section with orchestration skills table
- Note: forager uses M#.#.# (forward-only policy)
- Update docs/project-index.md

### FORAGER-5: Validation (15 min)
- `/forager-session-start` works
- `/forager-build` compiles
- `clauductor status/register/lock/unlock/deregister` lifecycle works
- All 36 skills listed and invocable
- Statusline shows milestone

### FORAGER-6: Commit (5 min)
- Commit as `M18.X: Integrate Clauductor orchestration framework`
- Push to remote

## Rollback Plan

```bash
# Partial: remove Clauductor skills only
rm -rf .claude/skills/{claim,release,blocked,status,supervisor,spawn,handoff,assign,review}
rm -rf .claude/skills/{session-start,commit,build,new-milestone,milestone-complete}
rm -rf .claude/skills/{dev-journal,log-insight,pr,prd-audit,architecture-audit,release-prep,skills}

# Full: restore everything
git checkout backup/pre-clauductor-migration -- .claude/ CLAUDE.md .gitignore
rm -rf orchestration/
```

## Success Criteria

1. All 36 skills (15 forager-* + 21 Clauductor) available
2. `/forager-session-start`, `/forager-build`, `/forager-commit` work unchanged
3. `clauductor status/register/lock/unlock` work
4. Zero data loss across 200+ docs
5. M#.#.# naming preserved
6. Migration committed using forager's own convention
