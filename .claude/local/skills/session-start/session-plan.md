**Run the session the plan names (local layer).** The Context block's *session plan* line names
the next session of `docs/roadmap.md`'s *Session plan*, its rows in the order the owner agreed and
what the owner decides in it. That order, not the change queue's table order, is the session's:
say the line in the summary, start the session's first row (and the lanes its focus runs beside
it, in their own worktrees), and put each owner decision it names in the owner-queue list. Say
every `warning:` line under it: a planned row it left out is a plan shorter than the owner's.

When it names **cloud lanes**, run each one's routine once: `RemoteTrigger` (ToolSearch
`select:RemoteTrigger`) `list`, find the routine by its exact name, then `list_runs` for it. A
routine that has a run already is not run again (a second session start, after a clear or a
handoff, would otherwise run it twice): report that run's status instead. Otherwise `run` it. Never
create or enable a routine, and a routine the list does not show is a stop for the owner. A run's
link goes in the conversation only, never into the repository (not the journal, not a PR body):
the repository is public. Each lane pushes a branch and opens no PR; when it reports, the Mac
reviews its diff, gates and merges it as any lane's, and the PR that lands the lane's work removes
the row's line from `.claude/local/cloud-lanes.tsv`.

A `CANNOT CHECK` line means the plan is unknown: say so, and fall back to the change queue only once
the owner agrees.
