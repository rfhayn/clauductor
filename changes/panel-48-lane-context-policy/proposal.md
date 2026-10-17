**Status:** awaiting approval
**Roadmap row:** PANEL-48
**Risk:** normal
**Budget:** $40

## Why
On 2026-10-15 the owner reviewed how the panel shapes the cost of the Claude sessions it drives in
a repository it attaches to (`evidence/README.md`). It measured one week of Standing Tee, the first
repository to attach: 2026-10-08 to 10-14, $1,208.28 at list price.

**What the measurement found:**
- **86% of the cost is context:** cache reads 49.6%, cache writes 36.4%, output 14%.
- **The biggest single cost is a session re-reading its own earlier work:** $141 a week. Only a
  session's lifetime moves it.
- **Nothing capped a session's context.** Peaks reached 930k, and calls over 300k were 23% of
  the spend.

**What the panel can change.** In attach mode the panel shapes a session at ONE point, its launch
(`LaneManager.LaneCommand`). Today it sets only `--model` and `--effort`, and only when the config's
`lane_types` names them. The sidecar `panel attach` writes has no `lane_types`, so an attached lane
gets neither. Nothing the panel sets limits a lane's context.

**What a cap is worth.** A simulated 200k auto-compaction cap would have saved **$163.33 that
week**, holding the repository's content fixed:

| Class | Saving |
|---|---|
| Orchestrator | $30.74 |
| Ops lanes | $51.26 |
| Fix lanes | $13.89 |
| Propose lanes | $11.02 |
| Subagents | $56.42 |

Turning claude.ai's connectors off at launch is bounded at about $2 a week per 1k of startup it
removes. The connectors' actual share is not measured yet.

**Where it sits in the queue.** It is NEXT by the owner's order of 2026-10-15: PANEL-48, then
PANEL-49 (attached lanes' measures), then PANEL-50 (the per-lane contributors view).

## What changes
The panel launches every lane with the context policy its config sets, without writing to the
repository.
- **What it sets:** a context window, at which Claude Code compacts, set per lane type with a
  default; claude.ai's connectors off; and, only if the owner turns it on for a lane type, a
  model for its subagents.
- **How it is passed:** through `--settings` on every launch. That covers Start, Restart, Resume
  and Restore. `--settings` ranks above the repository's own settings, and `--resume` does not keep
  it.
- **The defaults:** `panel attach` and `panel init` write a 200k window with connectors off.
  This repository's config and the template's preset get the same.
- **The bar and the alert:** a lane's context bar and its context alert measure against the window
  it was launched with, not the model's full window. Claude Code's `used_percentage` reads low once
  the window is set, so the alert would never fire.
- **Probes first:** the build starts with throwaway-session probes of the points the docs leave
  unverified.

A config that sets none of this launches lanes exactly as today.

## What the existing specs already guarantee
- **`specs/panel-attach`:** ATTACH-1-S2 holds that a sidecar equals what `panel init` plans for the
  repository. The new defaults reach the sidecar through init, so that requirement holds unchanged.
  ATTACH-2 (nothing written in the repository) is untouched: the policy travels in argv.
- **`specs/panel-new-lane`:** it covers where a lane starts, not how it launches. It is unchanged.
- **`specs/panel-claude-calls`:** it covers the panel's own `claude` calls, not a lane's. It is
  unchanged.

This change adds one capability, `panel-lane-context` (LANECTX-1 to LANECTX-3), and modifies no
requirement.

## Out of scope
- **Reading transcripts** for attached lanes' context and cost: PANEL-49. Until it lands, an
  attached lane with no status line shows no bar, so the calibration here shows first in lanes
  whose repository's status line posts, such as this repository's.
- **The per-lane contributors view:** PANEL-50.
- **Hooks passed at launch to trim tool output:** not proposed. Standing Tee trims in its own
  repository, and trimming in both places would trim twice.
- **Restarting with a handoff instead of `--resume`, keep-alive pings, and batching orchestrator
  wakes:** each was measured and set aside (`evidence/README.md`, the ranked table).

## How we'll know
- **Signal:** once the owner's lanes run on a panel built with this change, a week of the
  attached repository's transcripts, measured with `evidence/` as on 2026-10-15, shows:
  - lane and subagent context at p90 near the 200k window (before: orchestrator 442k, ops lanes
    563k);
  - cache read and write cost per class falling toward the simulated figures;
  - no rise in the flow line's review rounds or change-fail rate.
- **Before the clock starts:** an existing sidecar gets none of this, because attach never
  overwrites one. The owner adds the `launch` object to Standing Tee's sidecar by hand and trusts
  it again with `clauductor panel trust --project <repo>`, since the edit changes the sidecar's
  trust hash. The archive puts that step on the owner queue, and the 21 days count from it.
- **Check after:** 21 days
