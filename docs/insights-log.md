# Insights log

Raw, non-obvious technical observations captured **during** the work: platform quirks, gotchas,
trade-offs, debugging wins, guards that failed silently. Newest at the top. This is the intake
tier; durable decisions get **promoted** from here (`/log-insight` runs the check).

## Promotion rules
Routing follows *What this tier holds* in [`adr/README.md`](adr/README.md).
- **A decision with trade-offs, or a lesson with a mechanism** → a new ADR, or a dated amendment
  inside the ADR that already owns the principle. Mark **every** row the ADR cites
  `Promoted → ADR-NNNN`, not only the one that prompted it.
- **Already covered by an existing check** → `Instance of ADR-NNNN (check N)`, or the sentence
  form. Nothing is written in the ADR.
- **The compound step** (`session-close` step 3b, `sh .claude/compound.sh`): every Raw row gets a
  decision (promote, mark an instance, or `Not promoted — <why>`). `checks/compound.sh` fails a row
  still Raw after `COMPOUND_MAX_SESSIONS` sessions (default 3).
- **3+ rows of one shape** → a promotion candidate. Topic tags are typed by hand, so a real group
  can be spread across several tags.
- **A recurring gotcha or convention** → **not `AGENTS.md`**, which changes only to add a
  mechanism or delete something. Amend the owning ADR, add a mechanism, or keep the row as
  `Technique — no mechanism`.

## Status vocabulary

| Status | Means |
|---|---|
| `Raw` | Genuinely un-triaged. Not "we know about it". |
| `Promoted → ADR-NNNN` | The ADR (or its dated amendment) states it now. |
| `Instance of ADR-NNNN (check N)` / `Instance of ADR-NNNN ("<covering sentence>")` | Triaged; an existing ADR or AGENTS.md rule already covers it. Find the sentence first. |
| `Folded → <file>` | Its content moved into a named doc. |
| `Deferred → owned by <change>` | Work remains, and a named, open change owns it (AGENTS.md rule 1). Never an endpoint or a phase. |
| `Decided` | A decision was taken; say whose and where it is recorded. |
| `Fixed → <where>` / `Shipped → <where>` | Closed by named code, checked by existence (`grep`, `ls`, `git log`), not by re-reading the row. |
| `Closed — <what closed it>` / `Partly closed — <what remains>` | Resolved otherwise; say what remains, and who owns it. |
| `Superseded → <what>` | A later row or decision replaced it. |
| `Not promoted — <why>` | The compound step looked, and decided against promoting it: say why (a one-off, no mechanism would have caught it, not worth a rule). |
| `Technique — no mechanism` | Triaged, retained for the pattern, and nobody owes anything. Not a place to park work. |
| `Archived` | No longer relevant. |

Statuses may be emphasised (`**Decided 2026-01-02** — …`): anything that reads this column strips
formatting before matching the vocabulary.

## Log

| Date | Area | Topic | Observation | How to verify | Status |
|------|------|-------|-------------|---------------|--------|
| 2026-09-30 | CLI | install guard | The guard's old-model sign is `orchestration/config.json`, which is gitignored runtime state: on a fresh clone or worktree of an old-model repo (this one) `ownedByClauductor` is false, so install reads it as a foreign model and refuses, and only `--force` gets past. | `ls orchestration` in a fresh worktree of a pre-OPS-8 commit; `clauductor install --dry-run` refuses | Deferred → owned by OPS-14 |
| 2026-09-30 | CLI | install guard | `install --force` over an old-model repo overwrote 6 files and added 103, but left the 16 old skills (claim, spawn, supervisor…), 2 agents and 2 hooks the template no longer has, and the refusal list never named them: the owner has to know what to delete. | the OPS-8 commit's `git rm` list vs `install --force` output (journal Session 1) | Deferred → owned by OPS-14 |
| 2026-09-30 | CLI | old model | `clauductor install` still creates `orchestration/`, its SQLite database and `config.json` for a new-model install: old-model code still runs on the paved road. | `clauductor install` in a scratch repo, then `ls orchestration` | Fixed → `framework/internal/cmd/install.go` (P1, `TestInstallNoOrchestrationForTheNewModel`); OPS-13 deleted the SQLite, HUD and old CLI code |
| 2026-09-30 | CLI | template path | With `CLAUDUCTOR_FRAMEWORK` unset, `TemplatePath` falls back to `~/Development/clauductor/template`, the main checkout, whatever branch it is on: installing from a worktree or another checkout silently uses a different template than the binary was built from. | `grep -n candidates framework/internal/template/template.go` | Deferred → owned by OPS-14 |
| 2026-09-30 | CLI | update prompt | `clauductor update` with stdin closed (a script, CI, an agent) looped forever re-printing `[y/d/s/c] >`: 690 MB in two minutes. And each prompt made a new `bufio.Reader`, so piped answers after the first were swallowed. | `printf 'y\ny\ns\n' \| clauductor update`; `TestReadChoiceCancelsAtEndOfInput` | Fixed → `framework/internal/cmd/install.go` (one stdin reader; EOF cancels) |
| 2026-09-30 | Template | attribution | Turning attribution and provenance off (this repo's rule) means editing `build-change.js` too, a framework-tier file: `checks/model-roles.sh` demands it, and `clauductor update` then lists it as modified on every run. A project setting lives in a file the project is told not to edit. | `sh .claude/checks/run.sh model-roles` after flipping `model-roles.json` only | Deferred → owned by OPS-14 |
| 2026-09-30 | Template | branches | `BRANCH_CHANGE` is configurable in project.conf and honoured by the scripts, but the skills' prose, `build-change.js`'s `branchPrefix` default and panel.json's `branch_pattern` all say `change/`: a project that changes it gets contradictory instructions. This repo kept `change/` for that reason. | `grep -rn 'change/<id>' .claude/skills`; `grep -n branchPrefix .claude/workflows/build-change.js` | Deferred → owned by OPS-14 |
| 2026-09-30 | Template | no-clauductor | session-start printed "Panel: down (start it with: clauductor panel)" and its step 2 told Claude to report the panel down as unhealthy, contradicting D10 (a missing panel is a normal state). | `grep -n 'Panel:' template/.claude/skills/session-start/context.sh` | Fixed → `template/.claude/skills/session-start/` (OPS-8) |
| 2026-09-30 | Gate | no-clauductor | Dropping a PATH directory to hide `clauductor` drops everything beside it (`~/.local/bin` here; `/usr/bin` on Linux, as machine-quiet's check found): the directory has to be mirrored minus the binary, or the check fails for the wrong reason. | `template/.claude/checks/no-clauductor.sh`, the PATH block | Instance of ADR-0006 ("clauductor absent from PATH (its directory mirrored minus the binary)") |
| 2026-09-30 | Template | checks | `checks/merge-guard.sh` built its rule-12 fixture from the PROJECT's model-roles.json and asserted provenance is on: every project that turns provenance off (the setting the template offers) failed its own process checks. A check of the mechanism must not inherit the project's choice. | `sh .claude/checks/run.sh merge-guard` with provenance off | Fixed → `template/.claude/checks/merge-guard.sh` (the fixture forces it on; the default is asserted only in the template) |
| 2026-09-30 | CLI | update scope | `clauductor update` compared a hand-typed list of paths (`template.FindDiffs`) that had drifted from install's own classification: it never saw `change-cost.sh`, `scenario-trace.sh`, `compound.sh`, `examples/`, nor OPS-9's `metrics.sh` and `usage-report.sh`, so an updated project silently kept old copies. Install's list of top-level scripts had drifted the same way. | `TestUpdateComparesEveryFrameworkFile` | Fixed → #29 (B3: `template.Classify` drives `FindDiffs`; `.claude/*.sh` framework by rule), found here in parallel |
| 2026-09-30 | CLI | update scope | `update` never adds a NEW doc-tier template file, nor offers a changed one: a health line the template adds (OPS-9's `health/flow.sh`), OPS-10's `evals/` and model-roles fields, and every new AGENTS.md row and playbook section reach an installed project only by hand. | after `clauductor update`, `diff -rq template/.claude .claude` | Deferred → owned by OPS-14 |
| 2026-09-30 | CLI | settings merge | P1's settings.json merge keeps every hook the project already has, so `clauductor diff` on the pre-OPS-8 tree shows the old model's SessionStart and PostToolUse hooks (`session-register.sh`, `heartbeat.sh`) surviving the merge: delete those scripts afterwards and every hook call fails. A migration from the old model must drop its hooks. | `clauductor diff` in an archive of 7c392c7 | Deferred → owned by OPS-14 |
| 2026-09-30 | CLI | tiers | A new directory under `template/.claude/` falls silently into the doc tier (create once, never update): nobody is asked which tier it belongs to. OPS-10's `evals/` is doc tier on purpose; the next one may not be. | `TestEveryTemplateClaudeDirIsPlaced` | Fixed → `framework/internal/template/tier_test.go` (each directory must be listed as framework or project) |

## History (before OPS-8, kept as written)

The log as the old lock-based model kept it. Its rows are not reformatted, and the compound step
does not read them (it reads only the Log table above).

**Purpose**: Lightweight capture of technical insights discovered during development sessions. Acts as a triage inbox — when 3+ insights cluster around a topic, promote to a Learning Note or ADR.

**Promotion rules**:
- **3+ related insights** → Learning Note (implementation journey)
- **Architectural decision with trade-offs** → ADR
- **Recurring pattern or gotcha** → Add to CLAUDE.md

**Statuses**: Raw | Promoted (to LN/ADR) | Archived

---

## Active Insights

| Date | Milestone | Topic | Insight | Status |
|------|-----------|-------|---------|--------|
| 2026-03-30 | LIFE-1.1 | Hook throttling | Heartbeat uses timestamp file (`orchestration/.last-heartbeat`) with `find -mmin -1` for 60s throttle — avoids SQLite on every tool call | Raw |
| 2026-03-30 | LIFE-1.2 | Path normalization | Lock-guard must normalize absolute paths (from Claude Code stdin) to project-relative paths before DB lookup — locks stored as relative | Raw |
| 2026-03-30 | LIFE-1.3 | Hook `if` field | Claude Code hooks support `if` field (e.g., `Bash(git commit *)`) for argument-level filtering — prevents unnecessary process spawns | Raw |
| 2026-03-30 | LIFE-1.7a | Silent parse failures | `time.Parse` with discarded error (`_`) causes zero-value timestamps (00:00). SQLite drivers may return timestamps in varying formats — always try multiple | Raw |
| 2026-03-30 | LIFE-1.7a | TUI text handling | Truncation with ellipsis is better than wrapping for dashboard panels — wrapping shifts all content below, making layout unstable | Raw |

---

## Framework Bugs

_Track when the framework itself fails to prevent a problem it's designed to prevent._

| Date | Issue | What Failed | Fix Applied |
|------|-------|-------------|-------------|

---

## Promotion Log

| Cluster | Count | Promoted To | Date |
|---------|-------|-------------|------|
