# ADR 0019: A control is ported parity-first, and the shell version goes only with a parity receipt

- **Status**: Accepted (the owner, 2026-10-31)
- **Date**: 2026-10-31
- **Tags**: migration, hooks, checks, merge
- **Source**: S19; the researcher's critique (section 3)

## Context
The migration (ADR-0017) rewrites the controls that guard the migration itself. A port that drops a
case does not fail. It allows something that was refused before, which is silence, the failure mode
of AGENTS.md rules 1 to 3. The existing shell checks hold the expected behaviour:
- `checks/hooks.sh`, around line 930, has a table of hook, payload, exit code and stderr.
- `checks/merge-guard.sh` holds its cases in a few helpers that call the hook as `sh <path>`.

If the shell version and its check are deleted in the same PR as the port, the oracle is deleted
with them.

## Decision
For each control:
1. **Extract.** Its check's cases become data files (input, expected exit, expected stderr), which
   the shell check then reads. That is its own PR, with the shell control unchanged and the check
   still green.
2. **Falsify.** Each case is shown failing with the control removed (AGENTS.md rule 2). A case that
   passes without the control is fixed or deleted, never carried over.
3. **Port.** The Go control runs the same data files, and the parity harness runs both
   implementations against them. stderr matches byte for byte, until a case is relaxed on purpose,
   with the reason in the data file.
4. **Delete.** The shell control goes in the port's PR, with the harness's **parity receipt** (both
   implementations, every case, the head commit) in the PR body.
5. **Cases about shell itself** (dash, bash and zsh; launch forms; "no jq") are deleted, not
   ported, and the PR lists them.

No PR merges a half-ported control: one where some events route to Go and others to shell for the
same rule.

## Consequences
### Positive
- No rule is lost silently. Every case that existed is either carried over or deleted by name.
- The data files outlive the shell. They become the Go control's table tests.

### Negative / trade-offs
- About one extra PR per control (the extraction).
- Byte-exact stderr slows ports that want better messages. Relaxing a case is a deliberate,
  reviewed step.

## Enforcement
- **The harness:** a `stridal check parity <control>` command (OPS-185).
- **The receipt:** the merge guard (later `stridal merge`) refuses a PR that deletes a registered
  shell control without a parity receipt for its head commit. Built in OPS-185, which ports no
  control. The first port is OPS-187 (the lease), so every port carries a receipt the guard checks.

## Rests on
- One hook's cases extract into data files and run unchanged against the shell control:
  **untested** — accepted at risk by the owner (2026-10-31); tested by OPS-185, which starts with
  that extraction for one hook. If more than 20% of its cases cannot be expressed as data, this
  ADR is amended before any port.

## Related
- ADR-0017 (the port order), AGENTS.md rule 2, PRAC-0010 (close the class).
