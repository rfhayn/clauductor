**Number an outcome check past the history (local layer, OPS-147).** In step 3, `<n>` in `o.<n>`
is one more than the highest `o.<n>` in `docs/roadmap.md` **and** in `docs/roadmap-history.md`
(`sh .claude/local/roadmap.d/history.sh --ids` lists the history's ids): the reset moved o.1 to
o.4 and o.10 there, and their ids stay taken. A reused one makes the change queue UNKNOWN for every
reader (`.claude/local/roadmap.d/history.sh`), so run `sh .claude/roadmap-queue.sh --check` after
adding the row.
