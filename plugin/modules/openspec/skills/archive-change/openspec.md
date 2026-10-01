The OpenSpec module is on. The promotion and the move may be done by `openspec archive <id> -y`
(CLI 1.13 or later), but only after this skill's own preconditions pass: the CLI archives a change
with an open task or no spec delta, printing only a warning. Then confirm with
`openspec validate --all --strict`, which `sh .claude/checks/run.sh openspec:project` also runs.
