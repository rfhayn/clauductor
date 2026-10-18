**Run the session the plan names (local layer).** The Context block's *session plan* line names
the next session of `docs/roadmap.md`'s *Session plan*, its rows in the order the owner agreed and
what the owner decides in it. That order, not the change queue's table order, is the session's:
say the line in the summary, start the session's first row (and the lanes its focus runs beside
it, in their own worktrees), and put each owner decision it names in the owner-queue list. When it
names **cloud lanes**, run each routine first: `RemoteTrigger` (ToolSearch `select:RemoteTrigger`)
`list`, find the routine by its exact name, `run` it, and say each run's link in the summary; never
create or enable one. Each lane pushes a branch and no PR: when it reports, the Mac reviews its
diff, gates and merges it as any lane's, and that PR removes the row's line from
`.claude/local/cloud-lanes.tsv`. A routine the list does not show is a stop for the owner. A
`CANNOT CHECK` line means the plan is unknown: say so, and fall back to the queue only once the
owner agrees.
