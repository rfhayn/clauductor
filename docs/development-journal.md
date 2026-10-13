# Development journal

The narrative record: WHY decisions were made, what surprised us, what is next. Most recent
session first. Written by `/dev-journal`, which `session-close` runs.

Each entry starts with a heading of exactly this shape, because scripts read it:

```
## Session N — YYYY-MM-DD — <author> — <short focus>
```

N comes from `origin/main`, never from a branch; `pr-merge-guard` blocks a PR that would put two
entries with one number on `main`.

<!-- Newest session goes below this line. -->

## Session 2 — 2026-10-13 — Rich — MAPS planned and approved, attach mode, an overnight merge train

**Rows**: OPS-33 to OPS-61, PANEL-28, PANEL-29, PANEL-42, OPS-40, OPS-42 and others · **Span**: 2026-10-12 afternoon to 2026-10-13 evening

### What happened

The session started from Pav Rusovs' "AI operating system" video (MAPS: Memory, Agent, Pulse, Screen). The owner wanted its look and its parts built into Clauductor through this repository's own process. That became the MAPS program (PRD-maps, OPS-37): a Map view, routines, a Today column and a skills deck in phase 1, and a clean-room study of the paid source in phase 2. A rollback checkpoint was tagged first (`checkpoint/2026-10-12-pre-maps`). Midway the owner chose **attach mode** over converging Standing Tee into the template (PRD-attach, OPS-49): Standing Tee stays independent, and the panel serves it from a sidecar config.

**27 PRs merged on 2026-10-12/13.** By theme:
- **The flaky gate fixed at its causes:**
  - the Go 1.26.9 pin for the darwin race runtime (#90), and the guard against an affected toolchain (#106);
  - the #78 flakes (#83);
  - tests that launched the owner's real claude (#97, #98).
- **The in-flight three:** PANEL-29, the new-lane dialog (#94, archived in #99), and PANEL-28, the login-refresh race (#104, archived in #122).
- **The merge guard:**
  - commands with non-ASCII characters read (#103);
  - PR numbers read exactly (#112);
  - the rewrite hook reads script files (#111).
- **The gate:** a run log, a state file, `gate-wait.sh` and the background-gate hook (#119).
- **Agents:** reports reach their own lead (#121).
- **Records:**
  - practice records beside ADRs (#115);
  - the roadmap's Deps column, made strict (#117);
  - this repository's drift cleanup (#108).
- **The panel:** typing while scrolled back (#110), and tonight's lows (#116).
- **Approved and on main as proposals:** OPS-42 routines (#105), PANEL-42 attach (#96), OPS-40 map data (#113).

### Decisions the owner made (selected)
- **Routines' caps follow the billing mode.**
  - On a subscription, quota only, refusing at 80% of either window, and at most 3 runs a day while the quota is unknown.
  - On an API key, dollar caps, with the key read from a file outside the repository.
- **PANEL-42's sidecar commands are argv-only, with no shell text.** That ended a review arms race; the earlier `/dev/null` exception is superseded.
- **Review-round caps.** The owner chose "keep going until clean" for OPS-42's proposal (7 rounds), and "round 4, then land" for OPS-54. The soft cap of 5 otherwise.
- **Guard bypass families.** Two cheap ones were fixed; the rest are named as known limits, by class only, because the repository is public.
- **The rewrite hook will cover `changes/`, `specs/` and `.html`** (OPS-68).
- **A standing rule:** every template PR syncs this repository's own `.claude/` in its own branch.

### What surprised us
- **Reports kept landing in the top session.** In fork mode a subagent's spawns always run in the background, and a lead that hands back loses its children's results. The first fix, a foreground flag, was inert, and its fallback would have relaunched builders onto the same tree. A reviewer probed the real harness and caught it (#121).
- **Pushes that never happened.** Two `git push -q` calls failed silently: one branch had no upstream, and one's upstream was `origin/main`. PRs sat on stale heads for hours. Explicit refspecs since then.
- **Semantic conflicts.** #83's new testbin rule failed #97 and #104, though both merged cleanly as text. Merges behind main now test the merge-tree result first.
- **A stale quota reading.** The panel's `quota.json` was 9 days old and was read as 93%. The owner's real figure was about 80% (OPS-67).
- **Rule 11 checks nothing on a real archive**, because `git mv` shows renames, not adds (OPS-70).

### Left in flight (handed off)
- **PANEL-31:** groups 1–2 committed on `change/panel-31-build`; group 3 (docs and the 175% browser look) is next.
- **OPS-42:** group 1 through review round 3, committed as a WIP on `change/ops-42-build`. Round 4 and the orchestrator's rulings are listed in its tasks.md Progress line.
- **Three cloud sessions overnight,** on the owner's cloud credits: the OPS-40 build, OPS-39 and OPS-67. Each opens a PR for the next session to gate and merge (owner queue).
- **Held for the 08:00 quota reset:** PANEL-42's build, OPS-44, OPS-58, OPS-62 and the flakes (OPS-53, OPS-60, OPS-65).

### Flow and cost
- **Flow (30 days):** approval wait 0.9h, review rounds 3.5, spend $478.27 a week, 58 in flight. Cycle time and merges per week: CANNOT CHECK, because gh could not list merged PRs from the sandbox.
- **This session's cost** (usage-report.sh, list price, this machine): about **$554**.
  - builder $163;
  - the orchestrating session $158;
  - OPS-42's build lead $98;
  - reviewers $65;
  - the proposal drafters $38;
  - the rest small.
- The reviewers caught a critical (#121), several highs (OPS-40's proposal, #104, #111) and many mediums for about an eighth of the spend. The Opus reviewer role is earning its cost. OPS-42's group 1 took 3+ rounds and about $100 of lead and builder time; its later groups should split work more finely.

## Session 1 — 2026-09-30 — Rich (Claude, OPS-8 lane) — Clauductor adopts its own operating model

**Row**: OPS-8 · **Branch**: `feature/OPS-8-own-model`

### What happened

This repo stopped running the old lock-based skills and installed the model it ships, using the
product itself: a binary built from this checkout, `clauductor install` in the repo root. It is the
rehearsal for Standing Tee's convergence, so the friction is the point, and it is recorded below as
it happened.

1. **The guard refused, and was right to look twice.** `clauductor install --dry-run` said: *"install
   refused: … runs an operating model of its own (no .claude/clauductor-template). It would
   OVERWRITE 6 file(s) the project has changed: .claude/hooks/README.md, .claude/settings.json,
   .claude/skills/{dev-journal,log-insight,session-start}/SKILL.md, .claude/statusline.sh. It would
   ADD 78 framework file(s) beside the project's own."* This repo IS an old-model install, but
   `ownedByClauductor` looks for `orchestration/config.json`, which is gitignored runtime state, so
   a fresh worktree has none. The refusal list was accurate for what it covered: all six overwrites
   were old-model files we meant to replace. What it did not say: 16 old skills (assign, blocked,
   build, claim, commit, handoff, milestone-complete, new-milestone, pr, prd-audit, release, review,
   skills, spawn, status, supervisor), 2 agents and 2 hooks would be left behind. After reviewing
   the list, `install --force` ran: 103 new files, 6 updated, 6 docs kept, CLAUDE.md and .gitignore
   merged. It also created `orchestration/` and its SQLite database, old-model code on the new
   paved road (deleted; OPS-13 removes the code).
2. **The template path is ambient.** Without `CLAUDUCTOR_FRAMEWORK`, the binary takes its template
   from `~/Development/clauductor/template`, the main checkout, which was on another branch. Every
   command here set the variable to this worktree. A user with two checkouts would install the
   wrong template and not know.
3. **The old model removed by hand**: the 20 files above; the architecture-audit and release-prep
   stubs replaced with the template's (install keeps a project's copy of those, so the old-model
   stubs survived); old docs deleted (current-story, next-prompt×2, project-index,
   project-naming-standards, requirements, session-startup-checklist, MEMORY-SETUP); the LIFE-1 and
   auto-lock PRDs moved to `docs/prds/archive/`. The journal and insights history are kept
   verbatim under a history heading; only new entries use the new formats.
4. **Configuration.** project.conf (owner Rich, `TEST_GLOBS` = Go tests plus the template's
   checks, `gofmt -w` as the formatter), model-roles.json with attribution and provenance OFF,
   `scripts/ci/steps.sh` (process checks, gofmt, vet, `go test -short`, the template's checks, the
   plugin freshness check through `go run`, and `go test -race` in the full gate),
   `.clauductor/panel.json` (not installed into the panel: OPS-15), the roadmap in the queue
   grammar with milestone ids as row ids, and ADRs 0001–0008 for this cycle's decisions.
5. **Two places a project setting lives in a framework file.** Flipping attribution off in
   model-roles.json failed `checks/model-roles.sh` until `build-change.js` was edited too; update
   now flags that file forever. And `BRANCH_CHANGE` is configurable but the skills, the workflow's
   default and panel.json all say `change/`. I first set it to `feature/` to keep the repo's old
   branch names, found the contradiction, and went back to the model's lanes; commits keep the
   `PREFIX-N:` rule, which is what CLAUDE.md actually cared about.
6. **`clauductor update` hung an agent.** Run with stdin closed to sync the template change, it
   looped on its prompt and wrote 690 MB in two minutes before I stopped it. Fixed inline: one
   stdin reader for every prompt, end of input cancels, with a test. Piped answers (`y\ny\ns`) now
   work too; before, the first prompt's reader swallowed the rest.
7. **D10 built** (`template/.claude/checks/no-clauductor.sh`), wired into run.sh by existing as a
   file, named in AGENTS.md's table and the playbook. Writing it found the session-start
   contradiction (a missing panel reported as unhealthy), fixed in the template. Falsified by hand:
   a hard `clauductor panel status` step in steps.sh, and a `clauductor version` line in the status
   line, each failed it.
8. **The process checks failed on this repo's own choice.** `checks/merge-guard.sh` copied the
   project's model-roles.json into its rule-12 fixture and asserted provenance is on, so turning
   provenance off (a setting the template offers) failed the project's checks. Fixed in the
   template: the fixture forces it on, and the default is asserted only in the template itself.
9. **The update that followed OPS-9 showed what `update` cannot see.** After rebasing on OPS-9
   (#26), `clauductor update` listed none of its new top-level scripts: update compared its own
   hand-typed list of paths, which had drifted from install's classification (and install's list
   of top-level scripts had drifted too). Fixed: update now takes install's classification, and
   every script directly in `.claude/` is framework tier by rule, with a test that enumerates the
   template. Still open (OPS-14): update never adds a new doc-tier file, so `health/flow.sh` was
   copied by hand. P1 (#29) landed the same fix for update's scope (B3) while this PR was open,
   so this PR took P1's version and dropped its own.
10. **`clauductor diff` (from P1) on the pre-OPS-8 tree and after.** Before: *"framework: 83
    missing, 5 differs, 18 extra"*. The 18 extras are exactly the 16 old skills and 2 old hooks
    that install never named, so diff now answers finding 2 (the 2 old agents are doc tier and
    are not listed). *"doc: 26 missing, 6 present"*, and *"settings: .claude/settings.json merge,
    1 conflict(s)"* (the status line). The merge would also keep the old SessionStart and
    PostToolUse hooks pointing at `session-register.sh` and `heartbeat.sh`, so deleting those
    scripts afterwards would break every hook call. After: *"framework: 85 identical, 3 differs"*
    (P1's two hook changes, which update then took, and `build-change.js`, which differs on
    purpose), with no extras, *"doc: 32 present"*, and *".gitignore — lacks
    .claude/settings.local.json"*, which update does not merge, so it was appended by hand.
11. **Rebasing on OPS-10 (#28)** meant hand-merging its new fields into this repo's
    model-roles.json (the reviewer's `eval` baseline and `evals` thresholds: doc tier, so update
    does not offer them), copying the new `.claude/evals/` (doc tier on purpose), and adding its
    AGENTS.md row (rule 13) by hand. Answering `y` to update's review overwrote `build-change.js`,
    which lost the attribution settings again (finding 6). A new test now fails when a directory
    under `template/.claude/` is placed in neither tier.
12. **B12, from the Standing Tee convergence map**, is the session-start line this PR had already
    reworded. The no-clauductor check now also scans every script of the model for an
    echo/printf line that tells the user to run clauductor. It was falsified with the old line,
    which both the scan and the context run caught.

### Decisions

- **Keep the model's lanes (`change/`, `fix/`, `ops/`), not `feature/`.** The repo's rule was
  about commit messages; branch names were the old model's. Fighting the skills' prose costs more.
- **This repo's plugin check uses `go run`**, so its own gate obeys D10 too.
- **Old journal and insights rows are not converted.** The checks read only the new structures
  (`## Session N` headings, the `## Log` table), so history can stay exactly as written.

### What's next

OPS-13 (remove the old model from the CLI), then OPS-14 (the rehearsal fixes too big for this PR:
old-model detection, stale-file listing, the template path, settings in framework files, branch
prefixes), before Standing Tee's Phase 0 (ST-0).

---

# History (before OPS-8, kept as written)

Entries from the old lock-based model, in their own format. They are not renumbered or reformatted.

**Purpose**: A narrative chronicle of building this project — capturing decisions, learning moments, AI tooling evolution, and the story behind the code. Unlike the insights log (quick-reference table) or learning notes (milestone summaries), this journal tells the *why* behind the *what*.

**Format**: Session-level entries in reverse chronological order.

---

## 2026-03-30 — LIFE-1: Lifecycle Automation (Session 1)

**Milestone**: LIFE-1.1 through LIFE-1.4, plus LIFE-1.7a
**Branch**: `feature/LIFE-1-lifecycle-automation`

### What Happened

Built the entire hook infrastructure and lifecycle skills layer for Clauductor in a single session. Started with LIFE-1.1 (hook directory, session-register, heartbeat) and progressed through LIFE-1.2 (lock-guard + check-lock CLI), LIFE-1.3 (journal-check + status-sync hooks), and LIFE-1.4 (start-project, start-work, done, pane skills). Also fixed two HUD bugs as LIFE-1.7a (timestamp display and activity text wrapping).

### Key Decisions

- **Timestamp file throttle over SQLite queries**: The heartbeat hook uses `find -mmin -1` on a file instead of querying the DB. This keeps the fast path under 200ms even on slow filesystems. The trade-off is slight imprecision (file mtime has ~1s resolution), but for a 60s throttle this is irrelevant.

- **Warn, don't block for lock-guard**: The hook exits 0 with a warning message rather than exit 2 (block). This respects human autonomy — the developer sees the warning but can proceed. Projects wanting hard enforcement just change one line.

- **`parseSQLiteTime` multi-format fallback**: Rather than guessing which format the SQLite driver returns, the fix tries 5 common formats in order. This is defensive but correct — different go-sqlite3 versions behave differently.

- **Truncation over wrapping in HUD**: For the activity panel, truncating long lines with `…` keeps layout stable. Wrapping would cause panel height to fluctuate as data changes, breaking the dashboard aesthetic.

- **Added `/pane` skill**: The PRD was updated to include a `/pane` skill for quick tmux pane management. Simpler than `/spawn` — no orchestration, just "give me another terminal."

### AI Tooling Observations

- Claude Code's hook `if` field enables argument-level filtering (e.g., `Bash(git commit *)`) — this prevents the hook process from spawning at all for non-matching commands. Significant performance win vs. filtering inside the script.

- Skills-as-markdown is a powerful pattern: the 4 lifecycle skills are pure instructions, no executable code. Claude reads and follows them, using its own tools. This makes skills composable (chaining via `/skill-name`) without any runtime dependency.

### What's Next

LIFE-1.5 (context CLI + spawn enhancement), LIFE-1.6 (team startup), LIFE-1.7 (docs + polish).
