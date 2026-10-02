The OpenSpec module is on. Promote with `sh .claude/archive-change.sh <id>` (step 1) as for any
project; the CLI may then do only the move: `openspec archive <id> -y --skip-specs`, and only
after this skill's own preconditions pass (the CLI archives a change with an open task or no spec
delta, printing only a warning). Never let the CLI promote: it replaces a shorter MODIFIED
requirement whole, deleting the scenarios the delta did not restate, and syncs a capability that
`NOT-SYNCED.md` holds back. Then confirm with `openspec validate --all --strict`, which
`sh .claude/checks/run.sh openspec:project` also runs.
