# OPS-86 — draft ADR-0012 (Proposed): one merge path in place of a parser of shell text

- **PR**: #TBD
- **Session**: S16, started 2026-10-29 10:32 ET
- **Review**: 0 rounds, peak none, cap 3
- **Gate**: CANNOT CHECK — the lane stopped before the gate on the lead's instruction (a worktree gate is broken in this sandbox)
- **Cost**: CANNOT CHECK — a subagent lane shares the orchestrator's session id, so no row-only cost can be read
- **Session start**: CANNOT CHECK — no marker ran: the lane was told not to push, and the marker opens a draft PR

## What happened

ADR-0012 is drafted Proposed for the owner: `docs/adr/0012-one-merge-path-refused-everywhere-else.md`, with its index row. It weighs five options: keep the parser; one named merge command read as text; Standing Tee's `gh` wrapper (its PRAC-0041); one merge path with the refusal at the argv; and a server-side required check as a complement. It recommends the fourth. `.claude/merge.sh` holds the rules and is the only thing that merges, pinned to the judged head. A deny-only `.claude/bin/gh` refuses every other merge route, on the PATH route `pattern-kill-path.sh` already writes. The hook stops looking for merges and keeps only a route check and coarse refusals of the ways past the wrapper. No hook or code changed.

## Decisions

- OPS-58: cancel under D (its forms close at the argv; its cases become the build's acceptance rows). OPS-32: keep, rescoped to the guards that stay `PreToolUse`-only, no longer waiting on OPS-86. OPS-68: keep, its dependency on OPS-86 dropped (it changes which files the rewrite hook protects, not a reader). These take effect only when the owner accepts the ADR; the roadmap is not edited here.
- The ADR states requirements, not mechanisms, as Standing Tee's PRAC-0041 learned to (mechanism in a design record gets reviewed as if it were the parser).
- Insights-log retags (*shell parser never converges* → ADR-0012) wait for the close, since a row's PR writes only its fragment (new-adr step 5).

## Surprises

- Standing Tee's own move to the wrapper is half done: PR 1 (#681) merged, but PR 2 (#698), which deletes the reader, is parked after 8 rounds peaking high, on a grammar of the ways past the wrapper. So Standing Tee runs both the reader and the wrapper, and the wrapper has a false refusal of its own (`--json mergeCommit` after `-R`).
- The live guard refused two read-only commands while this record was drafted: `gh pr merge --help` piped into a `grep` for the banned flag, and `gh help pr merge`.

## Insights

| 2026-10-29 | Guards | the guard refuses its own help | `pr-merge-guard.sh` refused `gh pr merge --help` piped into a `grep` for `--auto` (read as an `--auto` merge) and `gh help pr merge` (read as a merge after a word that is not a plain gh): two read-only commands in one lane. Each is a false refusal that comes from reading text, not argv. | Run either command in a session with the hook registered | Instance of ADR-0012 (Proposed) |
