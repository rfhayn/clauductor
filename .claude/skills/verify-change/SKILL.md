---
name: verify-change
model: sonnet
effort: low
description: "Verify a built change before it merges: the approval still covers the design, every task is ticked, every scenario it adds or modifies is cited by a test (or escaped with a reason), and the diff touches what tasks.md claims. Read-only. TRIGGER when the user says 'verify the change', 'is <change> ready to merge', or merge-pr reaches a change PR; build-change runs the same script as its last step."
argument-hint: <change-id> [--base <ref>]
---

# Verify a change

One script does the work; this skill runs it and reports what it says, word for word
(*A green result must name its SUBJECT*: quote the last line, which names the change and the
commit).

```bash
sh .claude/verify-change.sh <id>
```

- **Exit 0**: report the last line. The change may go on to `merge-pr`.
- **Exit 1**: report every `FAIL` line and **stop**. Each one names what to fix:
  - *approval*: `design.md` or the Risk line changed after the owner approved. The owner approves
    again (`sh .claude/change-approval.sh <id> --revoke`, then `--record` on their word). Never
    re-record an approval yourself.
  - *tasks still open*: finish them, or name the change or row that owns each (AGENTS.md rule 1)
    and tick it with that note. Never tick a task that was not done.
  - *scenarios*: write the test that asserts the scenario's THEN and name the ID in it, or add a
    `(manual: <reason>)` / `(untestable: <reason>)` line naming the ID to `tasks.md`. An escape
    is for a scenario no test can reach, not for one nobody wrote a test for.
  - *a task claims paths the diff never touched*: the task is ticked but its work is not in the
    branch. Do it, or untick it.
- `NOTE` lines (changed files no task names) are the reviewer's first question, not a failure.

Change nothing while verifying: the gate's receipt names the commit, and a commit voids it.
