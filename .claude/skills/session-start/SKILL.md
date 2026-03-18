---
name: session-start
description: Run the mandatory session startup checklist. Reads context docs, checks git status, reports current milestone and branch. Use at the start of every Claude Code session.
---

# Session Startup Checklist

Run this checklist at the start of every session. No exceptions.

## Step 1: Check Setup

Read `CLAUDE.md` and check if the Setup Checklist has uncompleted items. If so, remind the user.

## Step 2: Load Context Documents

Read these 3 files in order:
1. `docs/project-naming-standards.md`
2. `docs/current-story.md`
3. `docs/next-prompt.md`

## Step 3: Check Git State

Current branch and status:
- Branch: !`git branch --show-current`
- Status: !`git status --short`
- Recent commits: !`git log --oneline -5`

## Step 4: Report

After reading all documents, provide a concise status report:

1. **Current milestone**: What M#.#.# is active, what status
2. **Branch check**: Are we on the correct feature branch? Flag if on `main`
3. **Uncommitted work**: Any staged/unstaged changes?
4. **Setup status**: Any unconfigured items in CLAUDE.md Setup Checklist?
5. **Next action**: What should we work on based on current-story.md and next-prompt.md

## Step 5: Red Flag Check

Verify:
- [ ] Not on `main` (should be on feature branch for any code work)
- [ ] Using correct M#.#.# naming convention
- [ ] Current work is documented in current-story.md

If any red flags are found, report them before proceeding.
