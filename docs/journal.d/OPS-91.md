# OPS-91 — the roadmap row is the authority for a budget's kind, as for its figure

- **PR**: #338
- **Session**: S16, started 2026-10-29 09:07 ET
- **Review**: 4 rounds, peak medium, cap 3 (the owner lifted the cap at round 3's medium and asked for Fable reviewers from round 4)
- **Gate**: CANNOT CHECK — the main session runs the gate
- **Cost**: CANNOT CHECK — a lane shares the orchestrator's session id
- **Session start**: CANNOT CHECK — a lane shares the orchestrator's session id

## What happened

Since OPS-85 a `, soft` on a proposal's `**Budget:**` line turned build-change's hard stop into a warning, and nothing held the mark to the owner: the design hash covers design.md and the Risk line, and `checks/changes.sh` compared only the figure with the roadmap row. The owner chose (2026-10-27) to make the row the authority for the kind too, so no design is re-approved. The parser reads `Budget: $N, soft` on a row as a soft budget and prints the kind as `--tsv` column 16 (`soft`, `hard`, or empty with no budget); `roadmap_tsv_errors` holds a plugged parser's column 16 to the same values. `checks/changes.sh` compares the proposal's figure and kind with the row's, and a parser that emits no column 16 has only hard budgets. The rows that were already soft (OPS-92a, OPS-104, OPS-107, PANEL-48, OPS-95, OPS-100, OPS-101, OPS-105) carry the mark on the row already. The only open soft proposal, PANEL-46's, had no row in `docs/roadmap.md` (it moved to `roadmap-history.md` when cancelled), so under round 3's fix it failed this check; OPS-154 (#331) removed its directory, and this branch was rebased after it.

## Decisions

- The row is read as `origin/<main>` has it (review round 1, medium), as `change-approval.sh` reads a row's Risk: read from disk, a branch that marked both its proposal and its row soft passed, which is the gap this row closes. The disk is read only in a repository with no remote or when main lacks the row (one added in the same PR), and the result names which. Where main cannot be read (round 2, medium: an archive clean room, a remote with no `origin/<main>`, main's roadmap not parsing) a soft proposal fails, as a delegated approval does there; a hard one is held to the disk. The figure gains the same protection, so raising a budget means landing the row's new figure on main first: the stop messages, playbook and apply-change say so, and the refusal points there.
- A soft proposal with no row to hold it to fails (round 3, medium): one that names no row, one whose row no roadmap holds, one whose row has no Budget, or one whose disk roadmap does not parse (round 4, low). The `**Roadmap row:**` line is outside the design hash, so dropping or renaming it had turned the check off. A hard one with no row still passes: it is the kind that stops.
- A row soft and a proposal hard FAILS, as the figure fails either way. The proposal is a copy of the row; a hard copy of a soft row stops a build the owner said should go on, which is a stalled lane nobody asked for.
- A row's mark reads soft only as `, soft` straight after the figure; anything else (`, hard`, prose, a misspelling such as `, softly`) is hard. A misread therefore errs toward the kind that stops a build, and a soft proposal against it fails loudly.
- build-change still reads the proposal's line (through `change-cost.sh`), not the row: the gate holds the line equal to the row, as it holds the figure. An edit to the proposal alone on a change branch would carry the build past its budget until the gate refuses it at merge; that window was there for the figure before this row and is the same for the kind.
- The kind is a new optional column, not a change to column 11: a plugged parser that emits 15 columns still meets the contract, and every reader of column 11 still reads dollars.

## Surprises

- The awk program in `roadmap-queue.sh` sits in single quotes, so an apostrophe in a comment inside it ended the program; the first run failed with a shell syntax error.
- The people check fails in the lane's sandbox, before and after the change, because `ps` is blocked there.

## Insights

| 2026-10-29 | Process | key-outside-the-hash | A check that holds a copy to its authority is turned off by any field it looks the authority up by, when that field is outside what the approval covers: dropping or renaming a proposal's `**Roadmap row:**` line made the budget comparison find no row and pass, though the budget itself was checked. A copy with no authority found must fail for the case that matters (here a soft budget), and the authority must be read where the branch cannot edit it (origin/main) | in a fixture, delete or rename the proposal's `**Roadmap row:**` line and add `, soft`: `checks/changes.sh` passed before OPS-91's round-3 fix and fails after (softnorow, softrenamed) | Raw |
| 2026-10-29 | Tooling | awk-comment-apostrophe | A comment inside a single-quoted awk program in a shell script is still inside the shell's quotes: an apostrophe in it (`budget's`) ends the program, and the script fails with a shell syntax error far from the comment | add `# the budget's kind` inside `roadmap-queue.sh`'s awk program and run `sh .claude/roadmap-queue.sh --check`: a syntax error at the printf line | Raw |
