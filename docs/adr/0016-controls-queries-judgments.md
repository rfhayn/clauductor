# ADR 0016: Each part of the backend is one of five kinds, by what executes it

- **Status**: Accepted (the owner, 2026-10-31)
- **Date**: 2026-10-31
- **Tags**: architecture, hooks, mcp, skills, plugin
- **Source**: S19. The owner's decisions of 2026-10-31: a three-way split; "push as much as we can in
  the MCP and make more agentic where it makes sense". Two research reads of the same day: the
  classification of the template, and the Claude Code capabilities (hook types `command`, `http`,
  `mcp_tool`, `prompt`, `agent`; plugin MCP servers). Supersedes ADR-0006.

## Context
ADR-0006 put every part of the operating model in POSIX sh, so that a contributor would need
nothing beyond Claude Code, git and jq. Its reason was a contributor working without stridal, and
that reason stopped binding when ADR-0011 made the plugin the install. The template's shell now
mixes several kinds of work, and the S19 classification counted them:
- **Rules that must hold every time:** the merge guard, the rewrite guard, the gate.
- **Deterministic changes of state:** `fold.sh`, `archive-change.sh`, `batch.sh`, `marker.sh`,
  `change-approval.sh --record`. About a third of the scripts are of this kind.
- **Readers of state:** the roadmap queue, health, cost, quota, config.
- **Wording and ranking done in awk:** `panel-suggest.sh`'s roughly 200 lines that choose what to
  offer next, the context scripts' instructions in prose, and `compound.sh`'s topic grouping by
  `cut | uniq -c`.
- **Rules stated in skill text that nothing executes:** for example, "never tick a task that was
  not done", "never force the gate green by weakening a test", and "never merge another person's
  PR".

Claude Code now offers a place for each kind. Command hooks run deterministically. `prompt` and
`agent` hooks let a model decide allow or block at an event. A plugin can serve an MCP server's
tools, resources and prompts. The S19 spikes (`docs/plans/s19-spikes.md`) measured how each of
these behaves.

## Decision
Each part is classed by **what executes it**, and it lives where that executor fits:

| Kind | What executes it | Lives in | Examples |
|---|---|---|---|
| **Control** | The harness, every time; deterministic | A `command` hook that calls the engine | The merge path, the rewrite, read, stash and `cd` guards, the gate, the lease, the budget stop |
| **Judgment gate** | The harness, every time; a model decides | A `prompt` or `agent` hook (ADR-0020) | "Was every ticked task done?"; "did this diff weaken a test?"; "did review converge?" |
| **Action** | A caller, deterministically; it changes state | An engine function that writes, reached by a `stridal-write` MCP tool and its CLI twin; its invariant lives in that function, so MCP is only the door | Fold a fragment, archive a change, set a row's state, record an approval, mark a session start |
| **Query** | A caller; it returns typed facts | An engine MCP tool; records as MCP resources | Roadmap rows, the plan, open PRs and overlaps, health facts, cost, quota, config, the next record number |
| **Judgment** | The model | A skill, an agent, or a workflow that orchestrates agents | Propose, review, triage, ranking what comes next, the session summary, clustering insights |

Five rules hold across the kinds:
1. **No control is a skill or a judgment gate.** An invariant always has a deterministic guard.
   A judgment gate adds a check beside it and never replaces it.
2. **A query never decides, and never words.** It returns typed facts and thresholds. The words
   and the ranking belong to judgment. So the context scripts' prose instructions, the verdict
   words in health lines, and `panel-suggest.sh`'s ranking move to skills and agents. Hard limits
   (never offer a landed row, never a batch's later row) stay as filters inside the engine's query.
3. **An action is the only way its state changes.** Skills call the action; they never edit the
   file the action owns (ADR-0021 for the roadmap).
4. **MCP is the interface between the model and the engine.** Skills obtain facts through MCP
   tools and resources, not by running a script and reading its text. A skill's `!` line becomes
   a SessionStart command hook that calls the engine's CLI, or a tool call. It never becomes an
   `mcp_tool` hook: S2d and S2e show those cannot carry context.
5. **Workflows orchestrate judgment.** Their deterministic steps call actions and controls; they do
   not re-implement them.

## Consequences
### Positive
- A reviewer can ask one question of any new piece: which kind is it? A script that words a
  message, or a skill that states a rule nothing executes, is then visibly in the wrong place.
- Typed facts replace text digests, so `digests.sh`'s truncation and the context size budget lose
  most of their reason.
- The ten unexecuted skill rules the S19 classification found each get an executor: a control, a
  judgment gate, or an explicit "Nothing. You."

### Negative / trade-offs
- A guarded repo needs the engine and the plugin (ADR-0017, ADR-0018). ADR-0006's promise is
  withdrawn.
- Judgment gates cost a model call per event and can be wrong. ADR-0020 bounds them.
- Mixed pieces must be split as they move, which makes each port larger.

## Enforcement
- **Every hook registration** is the engine's dispatcher, a named judgment gate, or a named
  extension point: a check, built in OPS-185.
- **A skill's MUST-style rule names its executor** (a control, a judgment gate, or "Nothing. You."):
  `checks/skills.sh`, OPS-192.
- **No skill runs a script whose output it parses** once the matching tool exists: `checks/skills.sh`,
  OPS-198.
- **"A query never words":** review judgment. **Nothing. You.**

## Rests on
- A dead MCP server disappears silently, so MCP cannot hold a control: S2c, **passed**.
- `mcp_tool` hooks cannot carry context: S2d and S2e, **passed**.
- Prompt and agent hooks behave as ADR-0020 records: S3a to S3c, **passed**.
- Scope: Claude Code only (the owner, 2026-10-31). Other clients (Cursor) are out of scope.
  Whether a cloud Claude Code session runs the engine is **untested** — accepted at risk by the
  owner (2026-10-31); tested by OPS-199.

## Related
- Supersedes ADR-0006. ADR-0017 (the engine), ADR-0018 (one version authority), ADR-0019 (parity
  ports), ADR-0020 (judgment gates), ADR-0021 (the roadmap as data). AGENTS.md rule 4.
