# The Standing Tee swap plan (ST-5)

**Status:** a plan, for the owner's review. Nothing in `rfhayn/standingtee` was changed to write it.
**Inputs:** the Phase 0 convergence map and the Standing Tee docs audit (2026-10-01), the adoption
plan in `docs/prds/active/PRD-change-process.md` (Phases 0–4, D1–D10), `origin/main` at `0b29da5`,
and open PRs #41–#46, #49, #52 and #54.
**Roadmap rows:** ST-3 (converge), ST-4 (hand-over) and ST-6 (designer onboarding). This plan
splits ST-3 into the sub-rows ST-3.0 to ST-3.14 (the grammar allows dotted ids).

## 1. What the swap is

Standing Tee runs an operating model it grew in place: its own merge guard, session skills,
OpenSpec skills, health lines and vitest meta-tests. clauductor's template was extracted from it
(OPS-1). Since then, Phase 1 gave the template extension points: the settings merge, the local
layer, modules, `ROADMAP_PARSER`, record baselines and the gate-steps library. Phase 2 moved Standing
Tee's better parts into the template as core or as modules (#41–#46). **The swap replaces Standing
Tee's copy with the template's, file by file, with its specifics held in `.claude/project.conf`,
modules and `.claude/local/`.** At the end, `.claude/clauductor-template` is written, and
`clauductor update` keeps Standing Tee current from then on.

**What changes:**

| For | What changes |
|---|---|
| **Rich** | One model to keep current, not two. New controls: change PRs need every task ticked (rule 9) and every new scenario cited by a test (rule 10); an archive must be complete (rule 11); squash commits carry provenance trailers (rule 12, if decision 2 says on); a role's model, agent or workflow changes only with an eval receipt (rule 13); session-close decides every Raw insight (compound). New proposals carry a risk tier, a budget and *How we'll know*. Skills are renamed: `/openspec-propose` becomes `/propose`, and archive and apply change the same way. `/verify-change` is new. |
| **Damian** | His lane, tools and first task stay the same. New: a sandbox and deny list on every Claude Code command, which on WSL2 needs bubblewrap, socat and the seccomp filter, with Windows interop denied. The full check also runs a secret scan and the scenario trace. `merge-pr` writes the trailers for him. `design/` gets its own panel lane. His onboarding docs change once, in ST-6. |
| **Agents** | They read the template's skills, with Standing Tee's steps arriving as module and local fragments, and their paths and names come from `project.conf`. Guard rules 5, 6 and 8 become the `write-surfaces`, `premise-check` and `artifacts` module rules, plus core rule 8 (the head contains `origin/main`). Commands change: `node infra/premise-check.mjs` becomes `sh .claude/modules/premise-check/premise-check.sh`, `artifact-currency.mjs --stamp` becomes `.claude/modules/artifacts/bin/currency.sh --stamp`, and `people*.mjs` becomes `.claude/modules/people/people.sh`. |

**What does NOT change:**

- **Records.** The roadmap, specs, archive, ADRs, journal, insights, founder queue and registries
  stay where they are, in their format (PRD Phase 0). There are three exceptions, all headers or
  defects, in ST-3.0: the insights log's `## Log` heading and vocabulary line, and the duplicate
  `2D.7` id (decision 1). Older records pass through `RECORDS_BASELINE` and `CHANGES_LEGACY`, and
  new records use the template's format.
- **The gate command.** `infra/ci/run-local.sh` (`GATE_RUN`) and `infra/ci/gate.sh` (`GATE`) stay,
  and so do the container runner, `gate-lock.sh` and its conformance suite. The template's
  `scripts/ci/*` are not installed, because the install maps them away when `GATE_RUN` points
  elsewhere. Standing Tee's runner calls the model's steps from `scripts/ci/lib/steps.sh`.
- **D10: the repository works without clauductor.** Every skill, hook, check and the gate run in
  Claude Code alone. The panel and the binary are optional, and the plugin is not adopted
  (decision 7). From ST-3.6 on, `checks/no-clauductor.sh` runs in Standing Tee's gate and enforces
  this.
- **Who decides.** Rich decides proposals, designs, ADRs and anything irreversible. Claude merges
  through `merge-pr`.

**How the template arrives without a big bang.** `clauductor install` and `update` refuse a
repository that runs its own model unless given `--force` (`framework/internal/cmd/ownguard.go`),
and Standing Tee has no marker. **That refusal is a safety for the whole swap:** no stray `update`
can overwrite Standing Tee's hooks halfway through. Each PR is built the same way:

1. Pin one clauductor commit for the whole swap (the *swap baseline*, set when §4 is green). Every
   PR body names it. Re-pin only on purpose, in its own PR.
2. In a **throwaway clone** of Standing Tee at the PR's base, with ST-3.3's `project.conf` present,
   run `CLAUDUCTOR_FRAMEWORK=<clauductor checkout at the baseline> clauductor install --force`. The
   install reads Standing Tee's conf, so the gate paths are mapped and `settings.json` is merged,
   not overwritten. Never run it in the main checkout or a lane worktree.
3. Copy only the paths the PR owns into the PR branch. Attach `clauductor diff` to the PR body, so
   each PR shows the divergence getting smaller.
4. ST-4 writes `.claude/clauductor-template`. From then on, `clauductor update` is the path.

## 2. The ordered PR sequence

**Rules for every PR:**

- Each PR is an ordinary Standing Tee PR, merged through its current `/merge-pr`, and it must pass
  the **full gate** (`infra/ci/run-local.sh`).
- A vitest meta-test stays until the template check that replaces it passes in Standing Tee's gate.
  It is retired in that same PR.
- **Hook-changing PRs land with no build lane open**, because `worktree-hook-drift` blocks a live
  lane whose hooks differ from `origin/main`. This applies to ST-3.5, 3.6 and 3.11.
- Onboarding files are **not** edited in ST-3.x; ST-6 does that (owner decision). The PRD's "edit
  onboarding in the same PR" is superseded.
- Every PR is revertible on its own. Records are never rewritten, so a revert loses no data.

### 2.1 Sequence

| # | Branch | Exact contents | Depends on (clauductor) | ST files touched | Must pass | Risk | Rollback |
|---|---|---|---|---|---|---|---|
| **ST-3.0** | `ops/converge-0-records-prep` | (a) `docs/insights-log.md`: add `## Log` above the table, and `Not promoted — <why>` to the vocabulary line (header only). (b) Decide the **three overdue Raw insights** (§2.3). (c) Renumber the duplicate `2D.7` (decision 1), with every citation found by grep. (d) Add `cooldown` to both ecosystems in `.github/dependabot.yml`. | merged: #30 (`compound.sh` reads `## Log`) | `docs/insights-log.md`, `docs/roadmap.md` (one id), `.github/dependabot.yml` | gate; `roadmap-queue-guard` (vitest); template `compound.sh` and `supply-chain.sh` run by hand, output in the PR body | L | revert |
| **ST-3.1** | `ops/converge-1-adoption-adr` | **ADR-0039** (the next free number at write time; `new-adr` reads `origin/main` and open PRs), *Standing Tee runs clauductor's operating model, configured, not forked*, **Proposed** (§2.4). Plus its index row in `docs/adr/README.md`. **When the owner marks it Accepted, ST-3.2 onward may start.** | none | `docs/adr/0039-*.md`, `docs/adr/README.md` | gate; `adr-numbering` (vitest) | L | revert |
| **ST-3.2** | `ops/converge-2-gitleaks-triage` | **The one-time secret triage.** Run `gitleaks dir` over the tracked tree, and `gitleaks git` over the full history, once. Classify every finding: a known non-secret goes in `.gitleaks.toml` with a reason (as in clauductor's OPS-17); a real secret STOPs this PR, and rotating it is the owner's decision. The redacted finding list goes in the PR body. | merged: #39 (the allowlist pattern) | `.gitleaks.toml` | gate; `gitleaks dir` exits 0 on the head | L (H if a real secret turns up) | revert |
| **ST-3.3** | `ops/converge-3-conf` | `.claude/project.conf` with Standing Tee's values (§2.2). The framework libraries that read it: `.claude/lib/{conf,change,records,modules}.sh`, `.claude/extensions.sh` and `.claude/local/README.md`. **Inert:** nothing calls them yet. A `project-conf.test.ts` asserts that every path key exists, and that `PROJECT_SLUG` matches today's focus-file slug. | #41–#46 merged (keys); **OPS-24** (`BRANCH_DESIGN`, `BRANCH_DOCS`) | `.claude/project.conf`, `.claude/lib/*`, `.claude/extensions.sh`, `.claude/local/`, `packages/db/test/project-conf.test.ts` | gate; the new vitest | L | revert (nothing reads it) |
| **ST-3.4** | `ops/converge-4-roadmap-parser` | `infra/roadmap-queue.mjs` gains `--tsv` (§2.2: the owner in column 9, the gate's `started` date in column 13 per #45, the raw status in column 14 per #44), `--check`, and `--text` per the contract. Its statuses map to the contract states: `⬜ planned` and `⬜ deferred — …` go to `open`, `❌ retired` goes to `cancelled`. Budget, due and `## Outcome checks` are added (an empty `## Outcome checks` is appended to `roadmap.md`, a new section, not a reformat). `ROADMAP_PARSER="node infra/roadmap-queue.mjs"` is set. | #44, #45 merged (columns 13 and 14 settled in the contract) | `infra/roadmap-queue.mjs`, `docs/roadmap.md` (one heading), `.claude/project.conf`, `packages/db/test/roadmap-parser-contract.test.ts` | gate; `roadmap-queue-guard` + the new contract test; template `checks/roadmap.sh` through the parser; `roadmap_tsv_errors` clean on the real roadmap | M | revert (readers keep `--text`) |
| **ST-3.5** | `ops/converge-5-leaf-scripts` | Take the template's `statusline.sh`, `status-write.sh`, `hooks/focus-staleness.sh`, `hooks/worktree-hook-drift.sh`, `hooks/lib/{merge-reader.awk,journal-sessions.sh,ci-receipt.sh}`, and `hooks/no-blind-source-rewrite.sh` (its jq parser). `founder-queue.sh` becomes `owner-queue.sh` (`OWNER_QUEUE=docs/founder-queue.md`). Retarget the panel card command and the vitest rows that pin these scripts. | merged (main) | the files listed; `.clauductor/panel.json` (one card); `control-panel-contract`, `founder-queue`, `hook-missing-dependency` (vitest) | gate; those vitest files; template `statusline.sh`, `owner-queue.sh` and `hooks.sh` by hand | L–M | revert |
| **ST-3.6** | `ops/converge-6-settings` | `.claude/settings.json` through the **settings merge**, with the `install --dry-run` diff in the PR body. **Deny list:** the template's rules plus #49's Windows interop rules (`cmd.exe`, `powershell.exe`, `pwsh.exe`, `wsl.exe`, `explorer.exe`, `clip.exe`, `notepad.exe`, `/mnt/c/*`). **Sandbox:** `enabled: true` and **`failIfUnavailable: true`** (owner decision). Its exclusions are derived from `GATE_RUN`/`GATE` (`infra/ci/run-local.sh *`, `infra/ci/gate.sh *`), with no interop command excluded. The network allowlist is the union plus Playwright's download host. The `~/.ssh` and `~/.aws` credentials are denied, and the AWS and release exclusions live per machine (decision 3). `statusLine` and every hook go through `$CLAUDE_PROJECT_DIR` (B14). `format.sh` replaces the inline biome hook (`FORMAT_CMD`, `FORMAT_EXT`). A local health line, `.claude/local/health/wsl2-sandbox.sh`, prints FAILED on WSL2 when bubblewrap, socat or `@anthropic-ai/sandbox-runtime` is missing: the seccomp filter is required there, and nothing else checks it. | **#49** (interop deny + `checks/settings.sh` selftests); merged: #29 (merge) | `.claude/settings.json`, `.claude/hooks/format.sh`, `.claude/local/health/wsl2-sandbox.sh`, `hook-registration.test.ts` | gate; template `settings.sh` (interop selftests) and `hooks.sh` (registration table); `hook-registration` (vitest) | **H** (every command runs sandboxed; a misconfigured WSL2 stops) | revert the file. **Damian must have the WSL2 packages before it merges** (decision 10) |
| **ST-3.7** | `ops/converge-7-no-clauductor` | **The D10 check in Standing Tee's gate:** `checks/no-clauductor.sh`, `checks/lib.sh` and `checks/run.sh`. The runner calls `model_step_no_clauductor` (`scripts/ci/lib/steps.sh`) on the host, before the container. `packages/db/test/no-clauductor.test.ts`, beside `gate-lock-conformance.test.ts`, covers the Standing Tee-only scripts: `gate-lock.sh run -- true` with a shim `clauductor` that exits 97 and logs, a stripped `PATH` and a temporary `HOME`. One AGENTS.md row is added, paid for in bytes (decision 4). **Falsified in the PR:** add `clauductor lock-run gate --` to `gate-lock.sh`, see both fail, restore. | merged: #27, #35 (`model_step_no_clauductor`) | `.claude/checks/{no-clauductor,lib,run}.sh`, `infra/ci/run-local.sh` (one call), `packages/db/test/no-clauductor.test.ts`, `AGENTS.md` | gate; the falsification recorded in the PR body | M | revert; the runner call is one line |
| **ST-3.8** | `ops/converge-8-checks-records` | The template checks run in the gate's process-checks step: journal, adr-numbering, agents-md-budget (`18000`/`300`), compound, owner-queue, review-lane, supply-chain, line-endings, settings, hooks, branch-prefixes, skills, roadmap. `RECORDS_BASELINE` is set to the day ST-3.3 merged. The template's `dev-journal`, `log-insight` and `new-adr` skills are taken (`INSIGHT_AREAS`). Retire the vitest duplicates (`adr-numbering`, `agents-md-budget`, `journal-sessions`, `founder-queue`). | merged: #30 (baselines); **#49** (line-endings); #46 (hooks registration) | `.claude/checks/*`, `.claude/skills/{dev-journal,log-insight,new-adr}/`, `infra/ci/run-local.sh` (`model_step_checks`, host side), the 4 vitest files | gate; each check on Standing Tee, with `compound` passing because ST-3.0 decided the overdue rows | M | revert the subset line |
| **ST-3.9** | `ops/converge-9-modules` | `MODULES="openspec review-page artifacts premise-check write-surfaces people risk-register ci-status"`, with each module's `enable.sh` run and its keys set (§2.2). **`docs/people.json` gains `lanes`** (the founder and designer lanes, from session-start's table; fix its `$comment`). **ci-status:** the runner calls the module's `publish-status.sh`, and `ci.yml`'s report step changes to `github verify=<r> e2e=<r>`. This must be live before ST-3.10, because #54's rule 2(a) blocks a merge with no check reported while `GATE_DISPLAY_CONTEXTS` is set. `artifacts.json`'s `$comment` points at the module's `bin/`. **`people.json` is an authority of the welcome page**, so stamp that entry in the same PR ("lanes key added; no reader-visible change"), or the next close PR is blocked. | **#41, #42, #43, #44** | `.claude/modules/*`, `.claude/project.conf`, `docs/people.json`, `docs/artifacts.json`, `infra/ci/run-local.sh`, `.github/workflows/ci.yml`, `.clauductor/panel.json` (the fix template's premise line) | gate; `checks/{modules,artifacts,premise-check,write-surfaces,people,risk-register,ci-status,openspec}.sh`; `ci-parity`, `artifact-*` (vitest) | M | revert; modules are inert until their consumers land |
| **ST-3.10** | `ops/converge-10-roles-agents` | `.claude/model-roles.json` takes the template schema: tiers, economy, `attribution{}`, `provenance` (decision 2) and prices, with Standing Tee's skill maps kept. The four agents take the template's text, with Standing Tee's lines re-added (migration claims, `conventions.md` pointers, money and tenant severity, `ReportFindings`). `PLAYBOOK=docs/playbook.html`, with skill-table markers. **This lands before the guard**, so rule 13 does not yet apply. It records the first eval receipt, so rule 13 has a baseline (decision 8). | merged: #28, #40 | `.claude/model-roles.json`, `.claude/agents/*.md`, `.claude/evals/`, `docs/playbook.html` (markers), `model-roles.test.ts` | gate; template `model-roles.sh` and `evals.sh`; retire `model-roles.test.ts` | M | revert |
| **ST-3.11** | `ops/converge-11-merge-guard` | **The merge-pr switch.** `hooks/pr-merge-guard.sh` and `hooks/lib/change-guard.sh` (core rules 1–4, 7–13); the module rules run from `guard.d` (premise, write-surfaces, currency). `skills/merge-pr/*` (template, plus the artifacts fragment for step 6) and `skills/verify-change` + `.claude/verify-change.sh` come with it: rule 12 needs merge-pr's `--body-file` trailers, so the two cannot land apart. `CHANGES_LEGACY="add-score-photo add-group-card-entry"`. **A replay:** run the last 30 real merge commands through the old and new guards, and attach the verdict table. The verdicts must be identical except for rules 9–13. | **#42, #43, #45** (rule 8), **#54** (rule 2(a)) | `.claude/hooks/pr-merge-guard.sh`, `.claude/hooks/lib/*`, `.claude/skills/{merge-pr,verify-change}/`, `.claude/verify-change.sh`, the `pr-merge-guard-*` and `premise-check` vitest (retargeted) | gate; template `merge-guard.sh`; the replay table | **H** (the merge gate itself) | revert the hook directory and the skill as one commit. **Land it with no build lane open** |
| **ST-3.12** | `ops/converge-12-session-skills` | **The session-start/close switch.** Template `skills/session-{start,close}/*`. Standing Tee's own steps become fragments: the artifacts, people and risk-register modules, plus `.claude/local/skills/` for living visuals and ideas until P2.11 and P2.12b ship (decision 6). Conflict rows go to `.claude/local/conflicts.tsv`. Health lines move: the template's push-main and scheduled-workflows (#45); `.claude/local/health/` for the migration ledger and the local pnpm audit, replacing the template's `health/dependency-audit.sh`. `machine-quiet.sh` with `MQ_DOCKER=1`, `MQ_COLIMA=1` and the vitest shapes widened. `panel.json` goes to v3: the Health card runs `extensions.sh health`, plus the owner-queue and change-queue cards, `suggest`, and **a `design/` lane and a `docs/` lane** (labelled "docs", reviewer-docs, no start template). `panel-suggest.sh` replaces `panel-suggest.mjs`. | **#43, #44, #45, #46**; **OPS-24** (lane keys) | `.claude/skills/session-{start,close}/`, `.claude/local/`, `.claude/health/`, `.claude/*-health.sh` (removed), `.claude/machine-quiet.sh`, `.clauductor/panel.json`, `.claude/panel-suggest.sh` | gate; template `skills.sh`, `machine-quiet.sh`, `panel-suggest.sh`, `session-context.sh`, `health-lines.sh`, `panel-contract.sh`, `no-clauductor.sh` over every `context.d` section; `control-panel-contract`, `living-visuals`, `people-activity` (vitest) | **H** | revert; modules and local files are additive. **After merging, run `clauductor panel trust` again**: the card commands changed |
| **ST-3.13** | `ops/converge-13-gate-model-steps` | The runner calls `model_steps <sha>` **on the host, before the container**: the scenario trace with `--rev`, and gitleaks over the tracked tree plus the branch's commits (the container has no `.git`). `ci.yml` gets the same secret scan, so CI fails without gitleaks. Also `TEST_GLOBS`, narrowed to `*.test.* *.spec.* e2e/`, and `GATE_TAIL_EXCLUDE='^\{"'`. | merged: #35 (`steps.sh`); **#46** (tail filter) | `infra/ci/run-local.sh`, `.github/workflows/ci.yml`, `.claude/scenario-trace.sh`, `.claude/checks/scenarios.sh` | gate; template `scenarios.sh` and `gate.sh`; `ci-parity`, `run-local-receipt-sha` (vitest) | M | revert; the receipt format is unchanged |
| **ST-3.14** | `ops/converge-14-change-process` | **The skills rename.** Delete `skills/openspec-{propose,apply-change,archive-change}` and take the template's `propose`, `apply-change`, `archive-change` (P2.3: `NOT-SYNCED.md`, RENAMED, superseded-wording) and `verify-change`. The openspec module ships `explore`. `openspec/config.yaml` keeps Standing Tee's rules and merges in the template's (M14). Also `change-approval.sh`, `change-cost.sh`, `compound.sh`, and the template's `build-change.js` (preflight reads `GATE_QUICK_FLAGS="--dirty --no-e2e --tz UTC"`), with **an eval receipt** (rule 13 applies by now). The checks: changes, change-tools, change-process, openspec, build-change. Fix every reference that breaks (M6). **Only after `add-score-photo` has finished building and archived**: the template's preflight STOPs a legacy change, which has no risk or budget (#46, difference 8). | **#41** (explore), **#46** (archive), merged: #22, #36 | `.claude/skills/{propose,apply-change,archive-change}/` (and the `openspec-*` deleted), `.claude/workflows/build-change.js`, `.claude/*.sh`, `openspec/config.yaml`, `.clauductor/panel.json` (propose template) | gate; template `changes.sh` (legacy), `change-tools.sh`, `openspec.sh` (`OPENSPEC_REQUIRED=1`, CLI ≥ 1.13), `build-change.sh`; `proposal-review-page` (vitest) stays | **H** | revert (the `openspec-*` skills come back) |
| **ST-3.15** | `docs/converge-15-operating-docs` | Prose docs: **AGENTS.md** (M1: the renamed rows; rows for rules 9–13, compound, MODULES, sandbox and gitleaks, no-clauductor; inside 18000 B; keep rule 1's (a)–(d) lettering). **conventions.md** (M10). **playbook.html** (M9, O3). **README.md** (S16). **runbooks/workstation-setup.md** (S5). **infra/ci/README.md** (S6). The roadmap's grammar notes (S13). `risk-register.md` (S4), `founder-queue.md` (S3) and `ideas/SKILL.md` (S15). **ADR amendments S7–S12**: amend, never reword (S12: ADR-0037's panel is optional, per D10). | none beyond the PRs above | the files listed | gate; `agents-md-budget.sh`; `docs-html-line-length`, `living-visuals` (vitest); the **full** reviewer, because AGENTS.md is in the diff | L | revert |
| **ST-4** | `ops/converge-handover` | Write `.claude/clauductor-template`. The PR body shows `clauductor diff` and `clauductor update --dry-run` reporting nothing left to change. Republish the playbook's shared copy. | the swap baseline | `.claude/clauductor-template` | gate; `update --dry-run` clean | L | delete the marker (update refuses again) |
| **ST-6.1** | `docs/designer-onboarding` | **Damian's onboarding, in one PR** (owner decision). `docs/onboarding-designer.md` and `docs/designer-welcome.html` change together, for every MUST and SHOULD in the audit's Damian section: §3 lanes are read from `people.json`; §5.2 gitleaks; §5.5 `format.sh`; §5.6 four agents, the deny list and sandbox, the deny list winning over allow, `settings.local.json`, Remote Control; §5.8 nothing needs clauductor; §9 compound, archive, the fast path, the panel line, which rules a `design/` PR meets (1, 2, 7, 10, 11, 12, and 13 only for roles); §10 the trailers and `Review: converged`; §11 the six pages (C1); §12 a worktree, not the main checkout (C5); the appendix gains `project.conf`. **§5.8, the WSL2 path:** (1) `bubblewrap` and `socat`; (2) the seccomp filter, **required**: `npm i -g @anthropic-ai/sandbox-runtime`; (3) the AppArmor profile on Ubuntu 24.04+; (4) clone under `~`, not `/mnt/c`; (5) Windows interop is denied, and why; (6) `failIfUnavailable`, which stops rather than runs unsandboxed, and must not be overridden locally. Welcome: `t-brew` gains gitleaks, `t-cc` the sandbox, `t-check` the secret scan; `flow-h` and `model-h` change to match. Rich's session republishes the shared copy, and the artifacts entry is refreshed. | ST-3.0 to ST-4 merged | `docs/onboarding-designer.md`, `docs/designer-welcome.html`, `docs/artifacts.json` | gate; `docs-html-line-length`, `living-visuals`; artifacts currency OK after the refresh | L | revert; republish the previous copy |
| **ST-6.2** | `docs/designer-onboarding-panel` | **§5.9, "The panel (optional, Mac only)"**, plus the Contents line, the welcome page's optional `t-panel` item (with a `data-optional` count exclusion) and the `tools-h` card. It says to skip the panel on Windows, and that nothing else depends on it. **Only after the v0.1.0 tag** (a released download or Homebrew, never a source build). | REL-1 (#25) merged **and** the v0.1.0 tag | the same two files | as ST-6.1 | L | revert |

**Count:** 19 PRs: ST-3.0 to ST-3.15, ST-4, ST-6.1 and ST-6.2. ST-3.14 also waits on the
`add-score-photo` archive, which is ordinary product work, not a swap PR.

- Sizes: 7 small (3.0, 3.1, 3.2, 3.3, 3.15, 4, 6.2), 8 medium, and 4 high-risk (3.6, 3.11, 3.12,
  3.14).
- **Ordering constraints:**
  - 3.3 comes before everything that reads conf.
  - 3.4 comes before any reader of the queue (3.9, 3.12, 3.14).
  - 3.9's ci-status comes before 3.11, because of #54.
  - 3.10 comes before 3.11, so the roles change without an eval gate. 3.14 comes after
    `add-score-photo` archives.
  - 3.2 comes before 3.13, so the secret scan has no surprises.
  - ST-6.1 comes last but one, and ST-6.2 after the tag.

### 2.2 Standing Tee's `project.conf` (ST-3.3, with the module keys in ST-3.9)

| Group | Values |
|---|---|
| Identity | `PROJECT_NAME="Standing Tee"`, `PROJECT_SLUG=standingtee`, `STATUS_MARK=🏌️` |
| Owner | `OWNER_ROLE=founder`, `OWNER_NAME=Rich`, `OWNER_QUEUE=docs/founder-queue.md` |
| Branches | `BRANCH_CHANGE=change/`, `BRANCH_FIX=fix/`, `BRANCH_OPS=ops/`, **`BRANCH_DESIGN=design/`**, **`BRANCH_DOCS=docs/`** (needs OPS-24). `BRANCH_SESSION_CLOSE` keeps its default, `ops/session-[0-9]*-close*`, which matches `ops/session-101-close`. |
| Records | `RECORDS_BASELINE=<the day ST-3.3 merges>`, `CHANGES_LEGACY="add-score-photo add-group-card-entry"`, `INSIGHTS_TABLE_HEADING="## Log"`, `INSIGHT_AREAS="AWS Auth DB Tenancy Engine API Next.js Mobile Ops"`, `COMPOUND_MAX_SESSIONS=3`, `PLAYBOOK=docs/playbook.html`, `AGENTS_MD_MAX_BYTES=18000`, `AGENTS_MD_MAX_ROW=300` |
| Roadmap | `ROADMAP_PARSER="node infra/roadmap-queue.mjs"`, `ROADMAP_NORMALIZER=""` (the parser emits the contract's states itself). The `--tsv` columns: 1 line · 2 phase · 3 section (the gate) · 4 id · 5 change · 6 kind (`change`, `fix` or `ops`; a `design/` or `docs/` row needs OPS-24's kinds or maps to `ops`) · 7 state (`queued`, `inflight`, `open`, `merged` or `cancelled`) · 8 pr (empty where the row has no `#N`) · **9 owner** (from `**Phase N owner:**` and `**Gate 2X owner:**`) · 10 summary · 11 budget · 12 due · **13 started** (`**Gate 2X started:**`, #45) · **14 raw status** (#44; a `⬜ deferred` row is never Next). |
| Changes | `MODULES` turns on `openspec` and `review-page`, which set `PROPOSALS=openspec` and `REVIEW_PAGE=artifact`. `CHANGES_DIR=openspec/changes`, `SPECS_DIR=openspec/specs`, `SCENARIO_IDS=new-only`, `TEST_GLOBS="*.test.* *.spec.* e2e/"`, `CHANGE_RECORD_EXTRA=""` |
| Gate | `GATE_RUN=infra/ci/run-local.sh`, `GATE=infra/ci/gate.sh`, `GATE_STEPS=infra/ci/steps.sh`, `GATE_QUICK_FLAGS="--dirty --no-e2e --tz UTC"`, `GATE_CLEAN_ROOM=archive`, `GATE_REMOTE_WORKFLOW=ci.yml`, `GATE_DISPLAY_CONTEXTS=<the contexts it posts today>`, `GATE_FAIL_PATTERN=<its gate.sh regex>`, `GATE_TAIL_EXCLUDE='^\{"'` |
| Machine and formatter | `MQ_DOCKER=1`, `MQ_COLIMA=1`, `MQ_ORPHAN_SHAPES` (from `machine-quiet.test.ts`: next dev, vitest, node under `@WT@`), `FORMAT_CMD="node_modules/.bin/biome format --write"`, `FORMAT_EXT="ts tsx js jsx mjs cjs json jsonc css"` |
| Modules (ST-3.9) | `MODULES="openspec review-page artifacts premise-check write-surfaces people risk-register ci-status"`. Plus `ARTIFACT_REGISTRY=docs/artifacts.json`, `ARTIFACT_PUBLISH=claude.ai`, `PREMISE_REQUIRED_ON=fix/`, `WRITE_SURFACE_GLOBS="*app/api/v1/*route.ts *apps/web/app/*page.tsx"`, `PEOPLE=docs/people.json`, `RISK_REGISTER=docs/risk-register.md`, and `CI_STATUS_LOCAL_PASS`, `_LOCAL_FAIL` and `_REMOTE_PASS` set to today's strings. |

### 2.3 The three overdue Raw insights (ST-3.0)

These three Raw rows are dated 2026-09-28, more than `COMPOUND_MAX_SESSIONS` (3) sessions ago, so
`checks/compound.sh` fails Standing Tee's gate from ST-3.8 unless they are decided first. Each
disposition is the session's call (the compound step), not the owner's. The seven newer Raw rows
are decided at the normal session closes.

| Insight | Recommended disposition |
|---|---|
| `review/an-empty-diff-reviewed-reads-as-a-pass` | **Mechanism built.** The template's `review-lane.sh` prints `review-lane: full (… an empty diff is not evidence …)` for an empty diff. It arrives in ST-3.11. |
| `review/diff-the-design-against-the-nearest-existing-implementation` | **Promote** to one line in the reviewer agent ("compare the design with the nearest existing implementation"), landed in ST-3.10, or **Not promoted** with the reason, if the owner prefers. |
| `agents/a-nested-agents-code-review-result-goes-to-the-top-level-session` | **An instance** of clauductor's open lesson that a `reviewer` has no SendMessage, so its hand-back reaches the top session (ST #425 lost about 4 hours). Mark it an instance and point at the clauductor row that fixes it. **That row does not exist yet:** it must be queued before ST-3.0 cites it (§4). |

### 2.4 The ADR (ST-3.1), drafted Proposed

- **Title:** *Standing Tee runs clauductor's operating model, configured, not forked.*
- **Status:** Proposed. It becomes Accepted when the owner says go, and that acceptance is the
  signal for ST-3.2 onward.
- **Context:**
  - Two copies of one model drifted. OPS-1 extracted the template from Standing Tee, and the
    template then grew rules 9–13, compound and the modules.
  - The designer needs a model that works without clauductor (D10).
- **Decision:**
  - The skills, hooks, checks and guard are clauductor's, with Standing Tee's specifics in
    `.claude/project.conf`, enabled modules and `.claude/local/`.
  - Records are never reformatted (`RECORDS_BASELINE`, `CHANGES_LEGACY`).
  - The gate command stays `infra/ci/run-local.sh`.
  - `clauductor update` keeps it current after `.claude/clauductor-template` is written.
  - The plugin is not used (decision 7).
- **Consequences:**
  - New controls: rules 9–13, compound, the scenario trace and the secret scan.
  - Skill renames.
  - Template upgrades arrive as `update` PRs.
  - A Standing Tee-specific change goes to the local layer, or upstream, never into a template
    file.
- **Enforcement:**
  - `checks/no-clauductor.sh` in the gate (D10).
  - The install guard plus the marker.
  - `clauductor diff`: a template-owned file modified in place shows up there.
  - The process checks in the gate.

## 3. Decisions still needed from the owner

| # | Decision | Recommendation |
|---|---|---|
| 1 | **The duplicate `2D.7`.** The `--tsv` contract needs unique ids, and Standing Tee's parser misses the duplicate. Which row keeps the id? | Keep `2D.7` on the older Phase 3 row, `ops/environments-and-operational-hardening` (2026-09-24). Give `add-transactional-email` (2026-09-27) the next free 2D id, and grep every citation in the same PR. |
| 2 | **Provenance trailers (rule 12) in Standing Tee.** | **On**, the template default. Replace the `Co-Authored-By` string with `attribution{}` plus provenance. `merge-pr` writes the trailers, Damian's included. |
| 3 | **Sandbox exclusions for Standing Tee's own scripts.** | Shared settings exclude only the gate (derived). `infra/release/*`, `infra/aws/*`, `ssh`, `scp` and `aws` go in the owner's `settings.local.json`, because they need `~/.aws` and `~/.ssh`, which stay denied. Add Playwright's download host to the network allowlist, and confirm the colima and docker exclusions on the owner's machine in ST-3.6. |
| 4 | **The AGENTS.md budget (18000 B, full).** About 8 new rows are needed. | Keep the ceiling. Pay by merging the vitest rows the new checks replace into one "process checks" row, and by renaming rows in place. Raising the ceiling is the fallback, in its own PR. |
| 5 | **ST-3's dependency on REL-1.** The roadmap gates ST-3 on REL-1, but only §5.9 needs a released install, and the tag is the very last item. | Drop REL-1 from ST-3's deps, and put it (with the v0.1.0 tag) on ST-6.2 only. Also drop **P2.15b** from ST-5's deps: Standing Tee keeps its own parser, which already reads boundary-task lines, so P2.15b matters only to a project on the built-in parser. It stays queued as its own row. |
| 6 | **Living visuals and ideas.** No module exists yet (P2.11 and P2.12b are queued). | Do not wait. Keep them as Standing Tee local fragments (`.claude/local/skills/`, local conflict rows) in ST-3.12, and move them to the modules when those ship, through `update`. |
| 7 | **The plugin in Standing Tee.** | **Not adopted.** The installed copy plus the marker; `/clauductor:init` refuses a marked repo anyway. ST-6.1 says nothing about the plugin. ST-6's roadmap text, "(the panel, the plugin, who decides)", is reworded to drop "the plugin". |
| 8 | **Rule 13 (eval receipts) in Standing Tee.** It costs a reviewer eval run whenever a role, agent or workflow changes. | Adopt it. Record the baseline receipt in ST-3.10, before the guard arrives. |
| 9 | **OpenSpec CLI ≥ 1.13 on both machines and in CI.** 1.2.0 deletes scenarios on archive. | Require it before ST-3.14 (`OPENSPEC_REQUIRED=1` in CI). It is a one-line install for Damian, added to ST-6.1. |
| 10 | **A short note to Damian before ST-3.6 merges** (outward, so the owner sends it). From ST-3.6 on, `failIfUnavailable` stops his Claude Code if the WSL2 sandbox packages are missing, and ST-6.1 lands much later. | Send it: the WSL2 steps (bubblewrap, socat, the seccomp filter, AppArmor on 24.04+, a clone under `~`), plus "the docs catch up in one revision at the end; ask Rich if something reads wrong". |
| 11 | Small ones, defaults unless the owner objects: (a) format per checkout rather than main only (no `FORMAT_MAIN_ONLY` key exists); (b) no ArtifactData persistence for the welcome checklist. | Accept both defaults. |

**Settled since the convergence map's 16 decisions:**

| # | Decision | How it was settled |
|---|---|---|
| 1 | Milestone ids | The P1.x, P2.x and ST-n rows are in use. |
| 2 | The roadmap parser | Settled by `ROADMAP_PARSER` (#30). Only the `2D.7` renumber is left: decision 1 above. |
| 4 | The open changes | Settled by `CHANGES_LEGACY`, and the plan's rule that ST-3.14 comes after `add-score-photo` archives. `add-group-card-entry`'s archive STOP is fixed by #422. |
| 5 | Damian's platform | WSL2 now, a Mac later; §5.9 is "optional, Mac only". |
| 6 | The sandbox | On, with `failIfUnavailable`, Windows interop denied, and the seccomp filter required on WSL2. The exclusions are decision 3. |
| 8 | Vitest vs shell checks | The plan's rule: retire the vitest file only once the template check passes in the gate. |
| 9 | How Damian receives the revision | One dedicated PR, ST-6. |
| 10 | Health lines | Upstreamed by #45; the local ones go to `.claude/local/health/`. |
| 12 | The plugin | The recommendation stands as decision 7, for confirmation. |
| 14 | The guard cutover | The replay is part of ST-3.11. |
| 15 | The format hook's scope | The default stands: decision 11(a). |
| 16 | Rule 8 head-contains-main | Core (#45), one of the 8 accepted differences. |

Map decisions 3, 7, 11 and 13 are the open decisions 2, 4, 11(b) and 9 above. Also already
decided: the `design/` and `docs/` prefixes (kept, with a `design/` lane and a prose-only `docs/`
lane); the adoption ADR (drafted Proposed); the 8 accepted differences of #45; and the v0.1.0 tag,
last.

## 4. Readiness checklist

**Merged in clauductor first:**

| PR / row | Why Standing Tee needs it | Blocks |
|---|---|---|
| **#41** risk-register, ci-status, openspec explore | three modules ST-3.9 enables; `explore` for ST-3.14 | ST-3.9, ST-3.14 |
| **#42** premise-check, write-surfaces | guard rules 6 and 5 as modules | ST-3.9, ST-3.11 |
| **#43** artifacts | the registry, currency and the close-PR currency rule | ST-3.9, ST-3.11, ST-3.12 |
| **#44** people (column 14) | the lanes, who-is-on-what, the owner rule | ST-3.4, ST-3.9, ST-3.12 |
| **#45** health, context, rule 8, roadmap gates (column 13) | the core guard's rule 8, `started` lines, queued-open | ST-3.4, ST-3.11, ST-3.12 |
| **#46** archive, contracts, tail filter, build-change driver | the archive Standing Tee's records need; the hooks registration; `GATE_TAIL_EXCLUDE` | ST-3.8, ST-3.12, ST-3.13, ST-3.14 |
| **#49** WSL2: LF, interop deny, line-endings check | Damian's platform; `update` merges `.gitattributes` | ST-3.6, ST-3.8 |
| **#54** OPS-21: rule 2(a) requires reported CI | the guard Standing Tee receives; ST-3.9's ci-status wiring is planned around it | ST-3.11 |
| **#52** OPS-22: the ST-5, ST-6 and P2.15b rows | the roadmap rows this plan is filed under | the plan's own record |
| **OPS-24 (new, not yet queued)** | `BRANCH_DESIGN` and `BRANCH_DOCS` keys; `checks/branch-prefixes.sh` accepts their panel lanes; `review-lane.sh` sends `docs/` to reviewer-docs; `--tsv` kinds or a mapping for them; a model-roles lanes row. **Without it, ST's `design/` and `docs/` lanes fail `branch-prefixes.sh`.** | ST-3.3, ST-3.12 |
| **A new row for the reviewer hand-back lesson** (not yet queued) | the owner that ST-3.0's third insight cites (rule 1) | ST-3.0 |

Not blocking: #53 (flaky panel tests), #25 (REL-1) except for ST-6.2, P2.11 and P2.12b (decision
6), and P2.15b (decision 5).

**"Ready for Standing Tee" means all of these hold, each checked by existence rather than by a
summary:**

1. Every PR in the table above shows `MERGED` (`gh pr view N --json state`), and OPS-24 and the
   hand-back row exist on `origin/main`'s roadmap.
2. #44 and #45 agree on the contract: `template/docs/roadmap.md` on main states column 13 as
   `started` and column 14 as the raw status, and `checks/roadmap.sh` and `checks/people.sh` both
   pass on main.
3. The swap baseline is chosen: a clauductor `origin/main` commit whose full gate passed (a receipt
   for that sha), with `sh template/.claude/checks/run.sh` green.
4. A dry run on a throwaway Standing Tee clone with ST-3.3's conf:
   - `clauductor install --dry-run` prints the foreign-model refusal (the guard holds);
   - `install --force --dry-run` maps `scripts/ci/*` away and shows a merged, not replaced,
     `settings.json`;
   - `clauductor diff` runs.
5. On Standing Tee:
   - #426 (LF) is merged;
   - the stray worktree `.claude/worktrees/add-group-card-entry`, which is missing 84 tracked
     files, is restored or removed (owner's machine);
   - `add-score-photo`'s lane is at a group boundary.
6. The owner has answered decisions 1–3 and 10, and has accepted ADR-0039. The rest can be
   answered before the PR that needs them.

## 5. Risks and rollback

| Risk | Where | Mitigation | Rollback |
|---|---|---|---|
| **The live panel and lanes.** The owner runs real Standing Tee lanes from the installed panel. | 3.5, 3.6, 3.10–3.12, 3.14 | Never replace the installed panel, and never touch its tmux socket. Every throwaway install runs in a scratch clone. Hook-changing PRs land with no build lane open, because `worktree-hook-drift` would block a live lane until it rebases. After ST-3.12 the panel's cards changed, so re-run `clauductor panel trust`; until then the cards show as untrusted, and lanes keep running. | Revert the PR. The lanes recover on rebase. |
| **Damian mid-onboarding.** His docs describe the old model until ST-6.1. | 3.6 to 6.1 | Decision 10's note before ST-3.6. Keep the stretch from 3.6 to 6.1 short. `design/` PRs meet only rules 1, 2, 7 and 10–12, and `merge-pr` writes the trailers for him. | His work continues on `design/`. Nothing in his lane depends on the swap. |
| **WSL2 hard stop.** With `failIfUnavailable: true`, a missing bubblewrap, socat or seccomp filter stops Claude Code. | 3.6 | That stop is the intent: it never runs unsandboxed silently. The decision 10 note, the `wsl2-sandbox.sh` health line, and ST-6.1 §5.8. | Fix the packages. **Not** a local `failIfUnavailable: false`. |
| **The merge guard swap blocks a legitimate merge, or lets a bad one through.** | 3.11 | The 30-merge replay; `CHANGES_LEGACY`; ci-status live first (#54); no lane open. | One revert commit restores the old hook and `merge-pr` together. |
| **An open change STOPs.** | 3.11, 3.14 | `add-group-card-entry`'s archive STOP is fixed by #422 (its `design.md` names the replaced scenario). `add-score-photo` is legacy for rules 9–11, and ST-3.14 waits until it archives, because the template's preflight would STOP its remaining groups. | Revert ST-3.14; the `openspec-*` skills return. |
| **The parser contract.** A malformed `--tsv` row makes the queue UNKNOWN to every reader. | 3.4 | The duplicate `2D.7` is fixed first (decision 1). The contract test runs on the real roadmap, with `roadmap_tsv_errors` clean. | Revert; `--text` readers are unchanged. |
| **The secret scan finds something.** | 3.2, 3.13 | The one-time triage runs before the step can fail the gate. A real secret is the owner's to rotate. | The allowlist is a reviewed file; revert it. |
| **Artifact currency blocks a close.** | 3.9, 6.1 | Stamp the welcome entry in ST-3.9 (`people.json`). ST-6.1 refreshes and republishes. | Stamp with an honest note. |
| **Template drift during the swap** (main moves while the 19 PRs land). | all | One pinned baseline. Re-pin only in its own PR, with the `clauductor diff` delta attached. | Re-pin back. |
| **Whole-swap abort.** | — | Records are untouched, so nothing is lost. | Revert in reverse order; ADR-0039 is marked Rejected; no marker is written. Until ST-4, `update` refuses Standing Tee anyway. |
