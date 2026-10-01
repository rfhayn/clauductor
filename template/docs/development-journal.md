# Development journal

The narrative record: WHY decisions were made, what surprised us, what is next. Most recent
session first. Written by `/dev-journal`, which `session-close` runs.

Each entry starts with a heading of exactly this shape, because scripts read it:

```
## Session N — YYYY-MM-DD — <author> — <short focus>
```

N comes from `origin/main`, never from a branch; `pr-merge-guard` blocks a PR that would put two
entries with one number on `main`. `checks/journal.sh` holds each heading to `JOURNAL_HEADING`
(`.claude/project.conf`); headings from before `RECORDS_BASELINE` stay as they were written.

<!-- Newest session goes below this line. -->
