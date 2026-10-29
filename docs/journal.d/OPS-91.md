# OPS-91 — the roadmap row is the authority for a budget's kind, as for its figure

- **PR**: #TBD
- **Session**: S16, started 2026-10-29 09:07 ET
- **Review**: TBD
- **Gate**: TBD
- **Cost**: TBD; session-wide: a lane shares the orchestrator's session id, so this is not the row's own cost
- **Session start**: TBD

## What happened

Since OPS-85 a `, soft` on a proposal's `**Budget:**` line turned build-change's hard stop into a warning, and nothing held the mark to the owner: the design hash covers design.md and the Risk line, and `checks/changes.sh` compared only the figure with the roadmap row. The owner chose (2026-10-27) to make the row the authority for the kind too, so no design is re-approved. The parser reads `Budget: $N, soft` on a row as a soft budget and prints the kind as `--tsv` column 16 (`soft`, `hard`, or empty with no budget); `roadmap_tsv_errors` holds a plugged parser's column 16 to the same values. `checks/changes.sh` compares the proposal's figure and kind with the row's, and a parser that emits no column 16 has only hard budgets. The rows that were already soft (OPS-92a, OPS-104, OPS-107, PANEL-48, OPS-95, OPS-100, OPS-101, OPS-105) carry the mark on the row already; the only open soft proposal, PANEL-46, has no row in `docs/roadmap.md` (it moved to `roadmap-history.md` when cancelled, and OPS-154 removes its directory), so it is not compared and still passes.

## Decisions

- The row is read as `origin/<main>` has it (review round 1, medium), as `change-approval.sh` reads a row's Risk: read from disk, a branch that marked both its proposal and its row soft passed, which is the gap this row closes. The disk is read only in a repository with no remote or when main lacks the row (one added in the same PR), and the result names which. Where main cannot be read (round 2, medium: an archive clean room, a remote with no `origin/<main>`, main's roadmap not parsing) a soft proposal fails, as a delegated approval does there; a hard one is held to the disk. The figure gains the same protection, so raising a budget means landing the row's new figure on main first: the stop messages, playbook and apply-change say so, and the refusal points there.
- A row soft and a proposal hard FAILS, as the figure fails either way. The proposal is a copy of the row; a hard copy of a soft row stops a build the owner said should go on, which is a stalled lane nobody asked for.
- A row's mark reads soft only as `, soft` straight after the figure; anything else (`, hard`, prose, a misspelling such as `, softly`) is hard. A misread therefore errs toward the kind that stops a build, and a soft proposal against it fails loudly.
- build-change still reads the proposal's line (through `change-cost.sh`), not the row: the gate holds the line equal to the row, as it holds the figure. An edit to the proposal alone on a change branch would carry the build past its budget until the gate refuses it at merge; that window was there for the figure before this row and is the same for the kind.
- The kind is a new optional column, not a change to column 11: a plugged parser that emits 15 columns still meets the contract, and every reader of column 11 still reads dollars.

## Surprises

- The awk program in `roadmap-queue.sh` sits in single quotes, so an apostrophe in a comment inside it ended the program; the first run failed with a shell syntax error.
- The people check fails in the lane's sandbox, before and after the change, because `ps` is blocked there.
