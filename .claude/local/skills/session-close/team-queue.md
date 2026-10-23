**Close out the team queue (local layer, OPS-95).** The owner's rule, in their words: "I apply the
`approved` label myself, by hand, on github.com. Bots never apply it." So no step here applies,
removes or asks for `approved`, or for `new-row`, which only the proposing bot applies.

1. For each team-queue item worked this session, after its PR merges (or the session stops it),
   write one line in this session's journal entry: `Team queue: #<N> → <row> <outcome>`, the
   outcome `merged (#<PR>)`, `open (#<PR>)` or `blocked: <why>`. This line never stands in for
   the delivering PR's `Delivers team-queue #<N>` line, which only a delivering PR carries.
2. Then add a short closing comment on the issue that says the same, with the PR's link:
   `gh issue close <N> --comment "<row> <outcome>: <PR link>"` once it merged, else
   `gh issue comment <N> --body "…"` and leave it open. Comments are written, never read. The
   issues still open on GitHub are the list the bot team reads; nothing else is posted.
3. A bug or flake found during the session that no row or issue owns yet: file it as an issue
   with the labels `team-queue` and the existing `bug` (never `approved`, `new-row` or any new label), so
   the bot team can triage it. It reads UNAPPROVED or NEEDS ROW until the owner says otherwise.
