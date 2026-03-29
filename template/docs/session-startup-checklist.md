# Session Startup Checklist

**Every session. No exceptions.**

---

## Phase 1: Context Loading (all sessions)

- [ ] Read `docs/project-naming-standards.md` — PREFIX-#.# convention
- [ ] Read `docs/current-story.md` — active milestones, priority queue
- [ ] Read `docs/next-prompt.md` — implementation guidance

## Phase 2: Implementation Prep (development sessions)

- [ ] Review relevant ADRs in `docs/architecture/` if working on architectural areas
- [ ] Search for existing code before creating new modules
- [ ] Run `/prd-audit` if PRD is >2 weeks old
- [ ] Validate architecture approach if feature has multiple possible designs

## Phase 3: Git Setup

- [ ] Create feature branch: `feature/PREFIX-#.#-brief-description`
- [ ] Verify not on `main` before writing code

## Red Flags (stop and fix)

- Using "Phase X" or "Step X" without PREFIX-#.# identifier
- Creating modules that duplicate existing functionality
- Committing directly to main
- Working without a feature branch
