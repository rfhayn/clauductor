# OPS-86 — ADR-0012, accepted: one merge path in place of a parser of shell text

- **PR**: #324
- **Session**: S16, started 2026-10-29 10:32 ET
- **Review**: 2 rounds, peak medium, cap 3 (round 1 medium, round 2 low; all lows fixed)
- **Gate**: CANNOT CHECK — the lane stopped before the gate on the lead's instruction (a worktree gate is broken in this sandbox)
- **Cost**: CANNOT CHECK — a subagent lane shares the orchestrator's session id, so no row-only cost can be read
- **Session start**: CANNOT CHECK — no marker ran: the lane was told not to push, and the marker opens a draft PR

## What happened

ADR-0012 was drafted Proposed, reviewed to convergence, and accepted by the owner on 2026-10-29: `docs/adr/0012-one-merge-path-refused-everywhere-else.md`, with its index row. It weighs five options: keep the parser; one named merge command read as text; Standing Tee's `gh` wrapper (its PRAC-0041); one merge path with the refusal at the argv; and a server-side required check as a complement. It recommended the fourth, D, which the owner accepted. `.claude/merge.sh` holds the rules and is the only thing that merges, pinned to the judged head. A deny-only `.claude/bin/gh` refuses every other merge route, on the PATH route `pattern-kill-path.sh` already writes. The hook stops looking for merges and keeps only a route check and coarse refusals of the ways past the wrapper. No hook or code changed. The roadmap gained the build rows OPS-162 (`merge.sh`), OPS-163 (the wrapper), OPS-164 (the reader deleted) and OPS-165 (a server-side required check), and the Session plan moved every session after S21 two places.

## Decisions

- **The owner, 2026-10-29:** option D accepted. A merge in another repository is refused (`merge.sh` has no `-R`). A gh location holding a `$` is refused outright, and the refusal says to write `gh` directly; `merge.sh`'s own lookup is exempt by path. Yes to the server-side check as a complement (OPS-165, after OPS-98; the ruleset is the owner's step, queued when the row starts). OPS-162 goes before OPS-77 and OPS-78 in S21, and OPS-163 and OPS-164 follow in S22–S23, so packaging, the terminal UI, the views, the launch and after-launch each move two sessions later.
- OPS-58 cancelled (its forms close at the argv; its cases become OPS-164's acceptance rows). OPS-32 kept, rescoped to the guards that stay `PreToolUse`-only, no longer waiting on OPS-86. OPS-68 kept, its dependency on OPS-86 dropped (it changes which files the rewrite hook protects, not a reader). OPS-77, OPS-78 and OPS-92c now write into `merge.sh`. This PR changes only those rows' Change and Deps cells; a row's PR sets no Status (this README), and `fold.sh` sets only `✅ merged`, so **the close sets these three Status cells by hand** (session-close, *The roadmap*):
  - OPS-58: `❌ cancelled — ADR-0012 (the owner, 2026-10-29): its forms close by construction at the argv the gh wrapper receives, the reader it extends is deleted (OPS-164), and its cases become OPS-164's acceptance rows`
  - OPS-32: `⬜ queued — rescoped by ADR-0012 (accepted 2026-10-29), which no longer holds it`
  - OPS-68: `⬜ queued — ADR-0012 (accepted 2026-10-29) drops its wait on OPS-86: it changes which files the rewrite hook protects, not how any reader reads shell`
- The ADR states requirements, not mechanisms, as Standing Tee's PRAC-0041 learned to (mechanism in a design record gets reviewed as if it were the parser).
- Insights-log retags (*shell parser never converges* → ADR-0012) wait for the close, since a row's PR writes only its fragment (new-adr step 5).

## Surprises

- Standing Tee's own move to the wrapper is half done: PR 1 (#681) merged, but PR 2 (#698), which deletes the reader, is parked after 8 rounds peaking high, on a grammar of the ways past the wrapper. So Standing Tee runs both the reader and the wrapper, and the wrapper has a false refusal of its own (`--json mergeCommit` after `-R`).
- The live guard refused three commands that merge nothing while this record was drafted, each re-runnable in a session with the hook registered:
  - `gh pr merge --help 2>&1 | grep -n -- '--auto'`, refused with "'--auto' merges before checks settle …" (`template/.claude/hooks/pr-merge-guard.sh:355`), twice;
  - `gh help pr merge 2>&1 | head -3`, refused with "the merge reads as [help pr merge], and what comes before pr is not a plain gh" (`template/.claude/hooks/lib/merge-reader.awk:616`), twice;
  - a `python3` heredoc of the ADR's edits whose text quoted the first command, refused with the `--auto` message (`pr-merge-guard.sh:355`).
- Round 1 (reviewer-docs, peak medium) found two mediums: OPS-92c's `--explain` is tied to option B in its row, not to D, and `worktree-hook-drift.sh` cannot hold D5, since it compares only `.claude/hooks/` and `settings.json` at agent spawn. Both were fixed in the ADR, along with six lows.

## Insights

| 2026-10-29 | Guards | the guard refuses its own help | `pr-merge-guard.sh` refused `gh pr merge --help` piped into a `grep` for `--auto` (read as an `--auto` merge) and `gh help pr merge` (read as a merge after a word that is not a plain gh), each twice on 2026-10-29, and a heredoc that held an ADR's edits (read as an `--auto` merge). Each is a false refusal that comes from reading text, not argv. | Run either gh command in a session with the hook registered | Instance of ADR-0012 (Proposed) |
