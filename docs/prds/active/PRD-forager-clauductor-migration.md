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

**ALREADY APPLIED** — `classifyFile` in install.go now treats `.claude/agents/` as tierDoc (create only if missing). Forager's custom agents will not be overwritten. Binary at `~/.local/bin/clauductor` is up to date.

## Milestones

**IMPORTANT: Commit after every milestone step so we can walk changes back individually.**

### FORAGER-1: Preparation (15 min)
1. Create backup branch: `git checkout -b backup/pre-clauductor-migration && git push origin backup/pre-clauductor-migration`
2. Return to main: `git checkout main`
3. Verify `/forager-build` works (iOS app compiles)
4. Record pre-migration state:
   ```bash
   echo "Skills:" && ls .claude/skills/ | wc -l
   echo "Agents:" && ls .claude/agents/
   echo "Docs:" && ls docs/ | wc -l
   ```
5. **Commit**: No changes yet, just verify baseline

### FORAGER-2: Dry Run (10 min)
1. Run from forager repo root:
   ```bash
   clauductor install --dry-run
   ```
2. Verify output shows:
   - **NEW**: 21 skill directories + docs/MEMORY-SETUP.md + orchestration files
   - **DOCS** (keeping existing): all forager docs (current-story, journal, etc.)
   - **CONFIG** (merge): CLAUDE.md and .gitignore
   - **NO overwrites** on agents
3. If anything looks wrong, STOP — do not proceed to FORAGER-3
4. **Commit**: No changes, dry run only

### FORAGER-3: Execute Install (5 min)
1. Run the actual install:
   ```bash
   clauductor install
   ```
2. Verify:
   - orchestration/ directory created with framework.db
   - .gitignore has `orchestration/` entry
   - CLAUDE.md has Clauductor section appended
   - Agents are UNCHANGED (still forager's versions)
3. **Commit**:
   ```bash
   git add -A
   git commit -m "M18: Add Clauductor orchestration framework (install)"
   ```

### FORAGER-4: Post-Install Customization (15 min)
1. Update the installed `/skills` SKILL.md to list BOTH skill sets:
   - Add all 15 forager-* skills to the listing
   - Keep all 21 Clauductor skills
2. Enhance the CLAUDE.md Clauductor section:
   - Add orchestration skills table (claim, release, blocked, status, etc.)
   - Add note: "Forager uses M#.#.# naming (forward-only policy)"
   - Add note: "forager-* skills take precedence for project-specific workflows"
3. Update docs/project-index.md to reference Clauductor orchestration
4. Clean up stale status line files:
   ```bash
   rm -f ~/.claude/forager-status.txt ~/.claude/forager-status-*.txt
   ```
5. **Commit**:
   ```bash
   git add -A
   git commit -m "M18: Customize Clauductor integration for forager"
   ```

### FORAGER-5: Validation (15 min)
**Forager workflows (must still work):**
- [ ] `/forager-session-start` completes
- [ ] `/forager-build` compiles the iOS app
- [ ] `/forager-commit` formats correctly (test with trivial change, then reset)
- [ ] All 15 forager-* skills invocable

**Clauductor orchestration (new capabilities):**
- [ ] `clauductor status` — no errors
- [ ] `clauductor register --name test --type build --milestone M18.1 --owner rich` — registers
- [ ] `clauductor lock --worker-id test --milestone M18.1 --files "test.swift"` — locks
- [ ] `clauductor query workers` — shows test worker
- [ ] `clauductor query locks` — shows locked file
- [ ] `clauductor unlock --worker-id test` — unlocks
- [ ] `clauductor deregister --worker-id test` — clean deregister
- [ ] Statusline shows milestone correctly

**Coexistence:**
- [ ] Total skills count is 36 (15 + 21)
- [ ] No skill name collisions
- [ ] settings.local.json untouched (200+ permissions intact)

6. **Commit** (if any fixes were needed):
   ```bash
   git add -A
   git commit -m "M18: Validation fixes for Clauductor integration"
   ```

### FORAGER-6: Documentation (10 min)
1. Add learning note: `docs/learning-notes/XX-clauductor-integration.md`
   - What was added
   - How forager-* and Clauductor skills coexist
   - Any gotchas discovered
2. Update docs/development-journal.md with session entry
3. **Commit**:
   ```bash
   git add -A
   git commit -m "M18: Document Clauductor integration"
   git push
   ```

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
