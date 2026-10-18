**Refresh the owner's roadmap tracker (local layer, OPS-83)**, always, once the close PR has merged
and the main checkout is on the new `main`, so the tracker shows the roadmap the next session
starts from. The tracker is the owner's private dashboard: update it in place, never publish a
copy, share it, or write its address into the repository.

Follow `.claude/local/skills/session-start/roadmap-tracker.md` steps 0–4, with two differences:
step 1 passes the date this session started as the second argument, so the rows merged this
session show as landed, and step 3's `updated.by` is `session-close`. A step-0 skip (no tracker
configured on this machine, or local main behind origin) is reported in the owner's notification
as it is, not as a failure. A refresh that fails after step 0 goes there as *roadmap tracker not
refreshed: <why>*, and the next session-start's line reads STALE.
