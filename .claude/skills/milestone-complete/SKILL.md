---
name: milestone-complete
description: Update core documentation files after completing a milestone. Use when marking any PREFIX-#.# as COMPLETE. Ensures documentation stays synchronized. TRIGGER when the user says "milestone is done", "mark it complete", "this milestone is finished", "wrap up the milestone", or any indication that a milestone has been completed.
argument-hint: <PREFIX-#.#>
---

# Milestone Completion Documentation

**Milestone to complete**: $ARGUMENTS

## Current State

- Branch: !`git branch --show-current`
- Uncommitted changes: !`git status --short`

## Files to Update

### 1. `docs/current-story.md` (source of truth for status)
- Move milestone from Active to Recently Completed table
- Add actual hours spent
- Update "Last Updated" date to today
- Update priority queue (remove completed, add next)

### 2. `docs/next-prompt.md`
- Remove completed milestone guidance
- Add guidance for the next milestone in the priority queue

### 3. `docs/roadmap.md`
- Add milestone to Completed table with actual hours
- Move from Active table if present
- Update priority queue to match current-story

### 4. `docs/requirements.md`
- Mark related requirements as COMPLETE (only if new requirements were fulfilled)

### 5. `docs/insights-log.md`
- Review: are there any unlogged insights from this milestone?
- Check promotion rules: 3+ insights on same topic → suggest Learning Note

### 6. `docs/development-journal.md`
- Write or update the narrative session entry for this milestone completion
- Include the **Retro** section (see below)

## Retrospective (mandatory)

Add to the journal entry:

```markdown
**Retro**:
- Estimate vs actual: [Xh estimated, Yh actual]
- What surprised you: [unexpected complexity, discovery, or outcome]
- Process improvement: [what would help next time]
```

## Verification

After updating:
- [ ] `current-story.md` has milestone in Recently Completed table
- [ ] Priority queue is consistent across current-story and roadmap
- [ ] Actual hours recorded
- [ ] Journal has retro section
- [ ] Any unlogged insights captured
