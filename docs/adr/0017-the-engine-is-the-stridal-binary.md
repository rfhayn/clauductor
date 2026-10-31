# ADR 0017: The controls and queries live in the engine, the `stridal` binary

- **Status**: Accepted (the owner, 2026-10-31)
- **Date**: 2026-10-31
- **Tags**: architecture, framework, hooks, gate, merge
- **Source**: S19, the owner's decision of 2026-10-31 to re-architect fully; the S19 researcher's
  critique. Amends ADR-0012.

## Context
The S19 researcher's count of the template's shell, by bucket. The named files' counts were
re-verified by the docs reviewer; the bucket totals are the researcher's. The reviewer counted
33.5k lines of template shell and awk in all:

- **About 5.5k lines do a program's job.**
  - Shell-text readers: `view-reader.awk` 632, `gate-reader.awk` 408, `no-blind-source-rewrite.sh`
    919, `worktree-remove.awk`, `pattern-kill.awk`, `read-by-range.sh`.
  - Structured-data logic: `lib/change.sh` 628, `conf.sh` plus `roadmap-queue.sh`,
    `change-approval.sh`, health, quota, metrics, `change-cost.sh`.
  - Stateful hooks: `no-silent-wait`, `cd-guard`, `catch-up-guard`.
- **The gate lease is implemented twice**: `lease.sh` and `internal/panel/lease/lease.go`. A
  721-line conformance kit exists only to keep the two in agreement.
- **About 2.3k lines are about to be rewritten in shell by ADR-0012**: `merge.sh`, the `bin/gh`
  wrapper and the slimmed hook. None of the three exists yet.
- **About 2.5k lines are thin git, gh and jq glue.**
- **About 15.4k lines are checks.** About 1.2k of those test shell itself.

The review cost lands in the first bucket: OPS-169's 12 rounds, and OPS-174, a row written to
mutation-test a reader of shell.

A language change does not fix the class ADR-0012 diagnosed. A guard that parses command text
converges only as a coarse refusal, in Go as in shell. What Go buys is a real parser, typed data
and unit tests.

## Decision
1. **The engine is the `stridal` binary, with two front doors over one core.**
   - **The CLI.** Every control (ADR-0016) runs through it: the `command` hooks behind one
     dispatcher per event, `stridal gate`, the lease as one implementation, and the merge path.
     Every query and action has a CLI twin, so the gate, CI, the checks and a person at a terminal
     work without a model.
   - **The MCP server** (`stridal mcp`, stdio), which the plugin declares. Its binary lives under
     `engine/`, never a top-level `bin/` (claude.ai and Cowork refuse one; S2f). It serves:
     - the queries and actions as **tools with input and output schemas**, kept to about 20
       well-named tools returning structured JSON, not one tool per script. Plugin tools are
       deferred behind tool search, so each server's instructions say what it is for;
     - a few records as **resources**, only where the same facts are also available from a tool:
       the ADR and PRAC indexes, the latest journal entry, and the current change's files.
   - **Only the stable subset of MCP is used.** It is built on the official Go SDK
     (`modelcontextprotocol/go-sdk`), using tools and resources over stdio. No roots, sampling,
     logging or server-pushed elicitation: the 2026-07-28 spec deprecates or reworks each, and the
     wire format has been rewritten twice in a year.
   - **No `mcp_tool` hooks.** They are refused at SessionStart, and at UserPromptSubmit their
     output never reaches the model (S2d, S2e). The session-start and session-close context comes
     from a **command** hook that calls the CLI (`stridal context session-start`), which prints the
     typed facts as compact text. The skill then calls the MCP tools for anything more.
   - **The MCP server never holds a control.** A model chooses whether to call a tool, a user can
     switch a server off in `/mcp`, and a dead server silently disappears (S2c). Every rule that
     must hold is a `command` hook.
   - **The server holds no state between calls that git or a file does not.** Each session runs
     its own server, so the lease and every record stay in files.
   - **Two servers, split by whether they write.** `stridal-read` serves the queries and the
     resources; `stridal-write` serves the actions. Each is the same binary started with its own
     flag.
   - **The template's settings allow both servers' tools** (`mcp__plugin_stridal_stridal-read__*`,
     `mcp__plugin_stridal_stridal-write__*`), so hands-off runs never stall on a permission prompt.
     Who may write is decided per agent: each agent's `tools:` line names the servers it may call,
     and `checks/model-roles.sh` treats a `stridal-write` tool as a write tool. A read-only role
     (the reviewers, the researcher) can therefore never call an action.
   - **An action's invariant lives in one engine function.** The MCP action, its CLI twin and any
     command hook that guards the same change all call that function. So an action that writes
     obeys the same rule as the control, and MCP is only the door, never the holder of the rule.
   - **Concurrent writes are safe by construction.** Each session runs its own server. A write
     replaces a file atomically (a temporary file, then a rename), and one file per row (ADR-0021)
     leaves git to show any conflict between sessions.
2. **ADR-0012 is built in Go from the start:** `stridal merge`, the `gh` wrapper and the hook
   reader's deletion. OPS-162 to OPS-164 are never written in shell first. OPS-77 and OPS-78 write
   their rules into `stridal merge`.
3. **Extensions keep a shell contract.** The local layer, the module `guard.d/*.sh` and the
   project's `roadmap.d/*.sh` source today's libraries. Before any library moves, the engine
   provides a CLI for each sourced function (`stridal queue --tsv`, `stridal conf get`,
   `stridal with-timeout`, ...), with sourced shims that call it. The shims retire only once no
   tracked caller remains. A broken extension blocks; it never allows.
4. **The port order follows the gain:**
   - first, the engine's lock and release (ADR-0018) and the parity harness (ADR-0019);
   - then the extension contract, the lease and the merge path;
   - then the readers, the stateful hooks and the libraries;
   - then the checks.
   **The glue comes last, and each glue script is a separate decision.** A short pipeline that
   only calls `stridal` subcommands may stay shell.
5. **The checks that test shell itself are deleted, not ported,** once their subject is gone:
   `mktemp-in-tmpdir`, `line-endings` (for scripts), `wall-clock`, `no-stridal`, the multi-shell
   rows of `hooks.sh`, and `worktree-hook-drift.sh`.

## Consequences
### Positive
- One language for every control, with typed data and tests. One lease.
- ADR-0012 is written once, not written and then ported.
- The terminal orchestration rows (ST-15 to ST-19) share the panel's lane derivation directly.

### Negative / trade-offs
- About 30 to 40 PRs, which the researcher estimates at 8 to 12 sessions, ±50%. Many are Risk high
  under the owner's close-the-class bar.
- During the migration, shell and Go controls coexist. Only ADR-0019 keeps that coexistence honest.
- Adopters' CI fetches a release binary, not only a git clone at a tag.

## Enforcement
- **The extension contract:** a check runs every tracked extension against the shims, with the old
  library files absent. Built in OPS-186.
- **The port order:** the roadmap's Deps column, read by `roadmap_queue`.
- **"Glue that may stay shell calls only `stridal` and git, gh and jq":** OPS-192 builds the
  decision list. Until then, **Nothing. You.**

## Rests on
- A plugin's stdio MCP server built on the official Go SDK works, from `engine/`: S2b and S2f,
  **passed**.
- A headless run reaches plugin tools only with an allow rule, and the per-server wildcard works:
  S2a and S2b, **passed**.
- A panel lane reaches them too: **untested** — accepted at risk by the owner (2026-10-31); tested
  by OPS-199.
- Go ports converge in fewer review rounds than shell did: **untested**, and the evidence so far
  says no (OPS-109 11 rounds, PANEL-57 13, both Go) — accepted at risk by the owner (2026-10-31);
  tested by OPS-187, the first port, which is the measurement. A port past 5 rounds triggers a
  re-estimate of the ports after it. A port past 6 rounds stops for redesign, and is named in Gate
  D's record.

## Related
- ADR-0016 (the five kinds), ADR-0018 (one version authority), ADR-0019 (parity ports), ADR-0020
  (judgment gates), ADR-0021 (the roadmap as data). Amends ADR-0012.
