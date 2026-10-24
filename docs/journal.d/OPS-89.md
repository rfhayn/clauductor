# OPS-89 — a docs gate, measured timeouts, API-key-free tests

- **PR**: #225
- **Session**: S11, started 2026-10-24 09:44 ET
- **Review**: 3 rounds, peak medium, cap 3 (round 3's medium was OPS-103's absence, which the owner moved to #224; two of its three lows fixed after it, not re-reviewed)
- **Gate**: CANNOT CHECK — no full gate ran in this lane before the fragment's commit; the final gate's receipt covers the head after it
- **Cost**: $13.73 (usage-report.sh, this session, at the fragment's commit)
- **Session start**: 122282 tokens, 0 min

## What happened

`run-local.sh --docs` gates a PR whose diff is prose. It runs the model's steps and the project's
`gate_steps docs`, and writes a receipt whose stage is `docs`. The merge guard accepts that receipt
only after it fetches origin/main and finds the PR's own diff is prose (`.claude/lib/docs-gate.sh`).
Prose is `*.md` outside `.claude/`, `agents/`, `commands/` and the specs, and not AGENTS.md,
CLAUDE.md, CLAUDE.local.md or SKILL.md, at any depth and in any case.

The docs gate is opt-in through `GATE_DOCS`. This repo turns it on, and `steps.sh`'s `docs_scope`
refuses the prose that Go tests read. The build-change gate timeouts are now set from
`gate-runs.log`: quick 20 min, full 45 min. The short Go suite has `-timeout 5m`. The panel tests
unset every API-key variable in TestMain through `lanes.APIKeyVars()` (#184).

## Decisions

- OPS-103, the quick gate skipping the lease, is not batched here, though the roadmap rows and S11's plan said it would be. The owner gave it its own lane and PR, #224, with its own fragment. Whichever of the two merges second rebases and runs the full gate again.
- The docs gate takes the lease like a full gate does, so it stays out of OPS-103's area.
- The docs gate is opt-in (`GATE_DOCS`). A project whose tests read prose has to refuse that prose in its own `gate_steps docs` before a docs receipt means anything.
- Both run-local and the guard fetch origin/main before they list the diff. Against a stale ref, a branch that reverts main's newer code lists only its prose.
- #184's fix lives in each package's TestMain, through `lanes.APIKeyVars()`, rather than in `steps.sh`. That way it also holds for a bare `go test`.
- The timeouts come from the log: the longest quick gate ran 722 s and the longest full gate 2024 s.

## Surprises

- A variable prefix on a function call (`VAR=x f`) leaves VAR set after the call in some sh implementations. The tests unset it explicitly.
- macOS's case-insensitive file system writes `.Claude/x.md` into `.claude/`, so the verdict folds case before it matches.
- A rename read as only its destination: `git mv x.sh x.md` looked like prose until `--no-renames`.
- The plugin build rewrites a literal framework skill path inside a check, so the fixture builds the path in pieces.

## Insights

| 2026-10-24 | Gate | gate/stale-base | A diff against a merge-base is only as fresh as the ref it is based on: against a stale origin/main, a branch that reverts main's newer code lists no code, so a guard that classifies a diff fetches first | `template/.claude/checks/gate.sh` "fetches before it lists" fails with run-local's fetch removed | Raw |
