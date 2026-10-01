---
name: dev-journal
model: opus
effort: medium
description: "Write or update this session's narrative entry in the development journal (JOURNAL in .claude/project.conf). Explains WHY: decisions, surprises, what is next. session-close runs it; otherwise it runs only when asked. TRIGGER when the user says 'journal this', 'update the journal', or 'write the session entry'."
---

# Development journal entry

Write or update the current session's narrative in the journal (`JOURNAL` in
`.claude/project.conf`, default `docs/development-journal.md`): newest session first, one entry
per session.

## Context: the state, computed
!`sh .claude/skills/dev-journal/context.sh`

If the line above shows as literal text instead of output, this harness does not pre-execute it:
run `sh .claude/skills/dev-journal/context.sh` yourself and read the result before step 1.

## Steps
1. **Take N from `origin/main`, not from this branch** (the context block prints it). A branch cut
   before someone else's session merged reads a stale top number; `pr-merge-guard` blocks the
   duplicate at merge, but it is cheaper not to write it. If this session already wrote an entry,
   update it instead.
2. Write the entry at the top, `<author>` being the first word of `git config user.name`:

   ```
   ## Session N — YYYY-MM-DD — <author> — <short focus>

   **What happened.** 1–3 sentences, then bullets naming PRs by number.
   **Key decisions.** Bullets with the reason; whose decision it was if it was the owner's.
   **Learning.** Non-obvious things found (each also logged with /log-insight).
   **What's next.** Where the next session picks up, naming the change or row that owns it.
   ```

## Rules
- Narrative, not a changelog: explain WHY. The PR list is in git; the reasons are not.
- Reference ADRs and insights rows rather than duplicating them.
- "What's next" names owners, not hopes (AGENTS.md rule 1): a change, a roadmap row, an issue.
- Never renumber another person's entry. On a collision at merge, yours takes the next number.
