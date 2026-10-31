# ADR 0020: Judgment gates: a model checks at an event, beside a control and never instead of one

- **Status**: Accepted (the owner, 2026-10-31)
- **Date**: 2026-10-31
- **Tags**: hooks, agents, review, evals
- **Source**: S19. The owner asked for more agentic work "where it makes sense". The S19 spikes
  measured how `prompt` and `agent` hooks behave (S3a to S3c, `docs/plans/s19-spikes.md`). The S19
  classification listed ten skill rules nothing executes.

## Context
Some rules need judgment, not a pattern:
- "Never tick a task that was not done" (verify-change).
- "Never force the gate green by weakening a test" (session-close).
- "Do not write an exempt line just to clear the trace" (merge-pr).
- "Merge only once review has converged" (merge-pr step 3; AGENTS.md admits no hook checks it).

Today these are held by "Nothing. You." Claude Code can run a model at an event, and the spikes
measured what it actually does:
- A **`prompt` hook** that returns ok:false on PreToolUse blocks the call **and ends the whole
  session** (S3a). In a hands-off lane, that is an outage.
- An **`agent` hook** that returns ok:false on PreToolUse blocks **only the call**, and the session
  goes on. It took 2.2 s (S3c). Agent hooks are marked experimental.
- On **Stop or SubagentStop**, ok:false does not refuse: the reason becomes the session's next
  instruction, and `stop_hook_active` bounds it to one extra loop (S3b).
- Neither hook type can write a file: an agent hook has Read, Grep and Glob, and a prompt hook
  returns only `{ok, reason}`.
- A hook that reads repository content can be steered by prompt injection in that content.

## Decision
1. **Two shapes, by event.**
   - **A tool-call gate** is an `agent` hook on PreToolUse with a narrow matcher (for example
     `gh pr merge` or `stridal merge`). It blocks that call with a reason. A `prompt` hook is never
     used on PreToolUse.
   - **A self-check** is a `prompt` hook on Stop or SubagentStop. When it fails, the session or the
     subagent fixes what the reason names. The gate corrects; it does not refuse.
2. **It never stands alone.** Each gate names the deterministic control that holds the rule's
   mechanical part: for example, verify-change's diff-path check beside "was every ticked task
   done?", and OPS-78's review-before-approval rule beside "did review converge?". A judgment gate
   is never the only thing between a session and an irreversible action.
3. **It fails open, and says so.** On a timeout or an error the event proceeds, and the hook's
   error appears in the transcript. The control beside it is the floor (ADR-0018's fail-closed rule
   covers controls, not gates).
4. **The transcript is its log.** A gate cannot write a file, but each verdict appears in the
   session's transcript: "Agent hook condition was not met: …", or "Stop hook feedback: …". The
   engine's transcript reader (PANEL-50's, one implementation) extracts the verdicts, and
   `stridal merge` reads them for the PR's sessions. A block that was then routed around is visible
   at merge.
5. **It is a model role with evals.** Each gate's model and effort live in `model-roles.json`, its
   prompt is listed in `evals.triggers`, and its eval suite under `template/.claude/evals/` has a
   violation it must catch, a clean case it must pass, and an injection case it must not follow.
   Rule 13 then holds every prompt change to a passing receipt.
6. **There are few of them:** at most about six, each on a narrow matcher.
7. **Built late, on evidence.** The first gates come in Phase 5 (OPS-197), after the ports. They are
   reconsidered if agent hooks are still experimental then, or if Claude Code changes their
   semantics. OPS-107's compatibility probe re-runs S3a to S3c on every Claude Code version it
   records.

## Consequences
### Positive
- Rules held today by "Nothing. You." get an executor that runs every time.
- A Stop self-check turns a would-be violation into a fix inside the same session.
- Each gate's accuracy is measured by its evals, not assumed.

### Negative / trade-offs
- An agent hook costs about 2 s and a model call per matched event. Narrow matchers and the cap
  bound it.
- Agent hooks are experimental, and Claude Code changes hooks often (its changelog, 2.1.301 to
  2.1.316). The compatibility probe is the canary.
- A gate that fails open misses a violation during an outage. The control beside it, and the
  verdicts read at merge, are the backstop.
- A gate reading untrusted content can be steered. The injection eval case and the control beside
  it bound that.

## Enforcement
- **Every judgment gate has its control, role, evals and shape:** a check that each registered
  `prompt` or `agent` hook names a control, a `model-roles.json` role, an eval suite, and is on an
  allowed event (an agent hook on PreToolUse; a prompt hook on Stop or SubagentStop). OPS-197.
- **A prompt change needs a passing eval receipt:** rule 13 (`stridal merge`).
- **A routed-around block is visible at merge:** `stridal merge` reads the transcript verdicts.
  OPS-197, on PANEL-50's reader.

## Rests on
- Gate semantics: S3a to S3c, **passed** (as recorded above).
- The transcript holds each verdict in a parseable form: S3b and S3c showed the text. **The
  parser is untested** — accepted at risk by the owner (2026-10-31); tested by OPS-197, which
  builds it.
- Agent hooks remain available in the Claude Code version OPS-107 records: **re-checked per
  version**.

## Related
- ADR-0016 (the kinds), ADR-0017 (the engine), ADR-0018 (fail closed for controls). Rule 13.
  PRAC-0014. PRAC-0023 (evidence before acceptance).
