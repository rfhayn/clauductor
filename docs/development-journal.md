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

## Session 14 — 2026-10-28 — Rich — S14 landed: the gate lease, the Standing Tee ports, the review cap and the load flakes

**Totals.** Gate 33 min (a row's gate was CANNOT CHECK, so this is a floor); cost $0 (a row's cost was CANNOT CHECK, so this is a floor).

**Rows folded.** OPS-133 (#297); OPS-139 (#296); OPS-146 (#292); ST-11 (#295); ST-14 (#298); ST-8 (#293); ST-9 (#294).

**What happened.** S14's nine rows are on main. OPS-147 (#289) and OPS-148 (#290) were already merged when this close started; this entry folds the seven that landed after them. The folded gate and cost lines are what the fragments recorded. Several of those fragments were written before the clean receipt, so the totals are a floor, not the later run. This session's own cost is CANNOT CHECK — no CLAUDE_CODE_SESSION_ID (this close ran in Cursor, and `usage-report.sh` with no session id says the same). Flow, 30 days to 2026-10-28: cycle 1.4h median, lead 2.4h, 44.3 merges/wk, change-fail 4.9%, approval wait 3.4h, review rounds 2, spend $1043.05/wk, 27 in flight (excludes unpriced models: claude-haiku-5-5).

**Key decisions.** None new from the owner at this close. The review cap stays a stop (OPS-139). The load flakes wait on the condition the test already claimed, not a longer sleep (PRAC-0010).

**Learning.** No row was promoted to a new ADR. Of the 28 Raw insights that had waited three sessions, 11 are instances of a sentence that already covers them: the authority a guard iterates, a passing test that has not failed, or PRAC-0010's close-the-class rule. The other 17 stay unpromoted: each was one occurrence already fixed at the site, or a control that already exists (the race-tier plugin test, the sandbox note of 2026-10-22). Three observations from this session stay Raw: a gate receipt that follows the sha the run started on, a journal check that holds shape and not truth, and a bold path that `approval-guarded` does not cover until its directory is listed. The close's own full gate then failed `session-start-digests`: the digest printed from **What's next** through the end of the entry, so the folded row blocks made it 10988 bytes and a row's "failed" read as the digest failing. It now stops at the next heading, and the check's folded-entry fixture holds that.

**What's next.** S15, in the session plan: OPS-123 (#209) first, then OPS-140, OPS-141, OPS-142, OPS-143 and OPS-99. OPS-42 and PANEL-25 are still queued. OPS-109 is merged (#240). PANEL-30, PANEL-31, PANEL-46 and PANEL-58 were cancelled on 2026-10-27 and are not to be built; their `changes/` directories still have unchecked tasks. Two remote branches have no open PR and stay, the owner's from 2026-10-13: `change/ops-42-build` and `change/panel-31-build`. REL-1 (#25) stays the owner's draft. The roadmap tracker was not refreshed: this session has no Artifact tools, so the next session-start should read it STALE.

### OPS-133 — close the load flakes the longer sleep did not

- **PR**: #297
- **Session**: S14, started 2026-10-27 16:09 ET
- **Review**: 2 rounds, peak medium, cap 3
- **Gate**: 11 min
- **Cost**: CANNOT CHECK — no CLAUDE_CODE_SESSION_ID
- **Session start**: CANNOT CHECK — no CLAUDE_CODE_SESSION_ID

#### What happened

#219 and the two tests in #234 failed in a loaded full gate and passed alone. #260 failed in a quick gate or `go test -short ./...` under load, and passed alone. Each read state once, or in the wrong order, instead of waiting on the condition it asserts. `pushRuns` now sorts stand-in records by `ModTime`. The stall test drives notify polls, inside a scaled deadline, until the merged read runs after `release()`. The stopped-call test asserts the call is still alive, then polls. The wheel test waits until `scroll_position` is non-zero. Those four passed 20 times under the race detector before this gate.

#### Decisions

None beyond the row. The fix is the condition the test already claimed, not a longer sleep (PRAC-0010).

#### Surprises

The first fragment was not the journal shape, so the full gate failed in that check before any Go test.

#### Insights

None.

### OPS-139 — a review at the cap stops instead of merging

- **PR**: #296
- **Session**: S14, started 2026-10-27 16:09 ET
- **Review**: 2 rounds, peak medium, cap 3
- **Gate**: 11 min
- **Cost**: CANNOT CHECK — no CLAUDE_CODE_SESSION_ID
- **Session start**: CANNOT CHECK — no CLAUDE_CODE_SESSION_ID

#### What happened

`REVIEW_AT_CAP` defaults to `stop`. When the last review round at the cap still has something medium or worse, build-change and merge-pr stop and say so, and an unreadable value fails closed. `known-limits` is the old path, which merges and records what remains. Lows still merge under KNOWN LIMITS. `review-cap.sh --why` names the setting. This repo's `project.conf` leaves the key unset and inherits `stop`.

#### Decisions

None beyond the row. The default is `stop`.

#### Surprises

The first fragment used a `## Session` heading and a Review line the shape does not accept, so the full gate failed in the journal check.

#### Insights

None.

### OPS-146 — stop gate's lease_verify-by-pid inheriting the outer nonce

- **PR**: #292
- **Session**: S14, started 2026-10-27 14:11 ET
- **Review**: 0 rounds, peak none, cap 2
- **Gate**: 11 min
- **Cost**: CANNOT CHECK — no CLAUDE_CODE_SESSION_ID in this shell (Cursor session)
- **Session start**: CANNOT CHECK — no CLAUDE_CODE_SESSION_ID in this shell

#### What happened

`checks/gate.sh`'s lock-run "by pid" case inherited `CLAUDUCTOR_LEASE_NONCE` from an outer `lease_run` (the no-binary path, ADR-0006), so `lease_verify` compared nonces and answered LOST. Every docs or full gate without the binary failed. The case now asserts LOST with a leaked nonce first, then unsets the variable and asserts VERIFIED. The other `lease_verify` cases already run inside `lease_run`, which exports the holder's own nonce.

#### Decisions

None. Risk: low ops fix as written on the roadmap.

#### Surprises

None.

#### Insights

1 row(s), moved to the insights log.

### ST-11 — session-start context digests and a byte budget check

- **PR**: #295
- **Session**: S14, started 2026-10-27 16:03 ET
- **Review**: 0 rounds, peak none, cap 2
- **Gate**: CANNOT CHECK — full gate not run, lease held by ST-8
- **Cost**: CANNOT CHECK — no CLAUDE_CODE_SESSION_ID
- **Session start**: CANNOT CHECK — no CLAUDE_CODE_SESSION_ID

#### What happened

Standing Tee's `session-start/digests.sh` is ported into the template: the change queue, latest journal entry, recent insights, and ADR and practice indexes print as bounded digests from `context.sh` via `roadmap_queue --text` (no node). Clauductor's folded journal uses `#### What happened` or the first lines after the session heading when there is no `**What happened.**` paragraph; missing **What's next** is one explicit line. A new `session-start-digests` process check holds real-doc output under 8000 bytes and fixture cases for journal, Raw count, and NOT ACCEPTED index rows; `context.sh` must call `digests.sh`.

#### Decisions

None beyond the approved row (digest ceilings and check shape match Standing Tee's test contract).

#### Surprises

None.

#### Insights

None.

### ST-14 — no forced worktree removal or pattern kill

- **PR**: #298
- **Session**: S14, started 2026-10-27 17:24 ET
- **Review**: 0 rounds, peak none, cap 3
- **Gate**: CANNOT CHECK — full gate not run, lease held by ST-11
- **Cost**: CANNOT CHECK — no CLAUDE_CODE_SESSION_ID
- **Session start**: CANNOT CHECK — no CLAUDE_CODE_SESSION_ID

#### What happened

Standing Tee's refusal of `git worktree remove --force` and of `pkill`/`killall` is ported into the template. The force reader is `worktree-remove.awk` (no node: ADR-0006): a literal `--force`, `--f…` prefix or short cluster containing `f` is blocked, including after the path and inside `sh -c`; a computed argument before `--` and a shell fed a removal on stdin are blocked; a plain removal, a quoted mention, and `machine-quiet.sh` are allowed. `.claude/bin/pkill` and `killall` refuse every call. `pattern-kill-path.sh` puts that directory first on PATH at SessionStart, and `no-pattern-kill.sh` refuses a path that would skip the wrappers. The plugin packager now ships `.claude/bin` with the other framework directories, as the sibling of `hooks/`.

#### Decisions

None beyond the row. The reader is awk rather than Standing Tee's node program because this model's hooks do not use node.

#### Surprises

The packager refused a new `.claude/bin/` until `frameworkDirs` named it. A script on a shell's stdin is refused when the call still names a removal the reader did not parse; a removal written to a file and then executed is not read (the same known limit Standing Tee records).

#### Insights

None.

### ST-8 — port usage-report --contributors into Clauductor

- **PR**: #293
- **Session**: S14, started 2026-10-27 15:01 ET
- **Review**: 1 rounds, peak high, cap 2
- **Gate**: CANNOT CHECK — scoped checks only (metrics, journal), not full gate
- **Cost**: CANNOT CHECK — no CLAUDE_CODE_SESSION_ID in this Cursor lane
- **Session start**: CANNOT CHECK — no CLAUDE_CODE_SESSION_ID

#### What happened

Standing Tee's context attribution (`usage-report.mjs --contributors`) is now `template/.claude/lib/usage-contributors.jq` plus `usage-report.sh --contributors`, sharing transcript reading with `lib/usage.sh`. `metrics.sh` falsifies a fixture where a `Read docs/roadmap.md` dominates; row fragments may append `; top: …` on **Cost** per `docs/journal.d/README.md`.

#### Decisions

None beyond the approved row scope.

#### Surprises

jq's `=` assignment returns the rhs, not the thread state — attribution needed explicit `callIds` and `| .` after every `+=`.

#### Insights

None.

### ST-9 — read-by-range PreToolUse hook (sh + awk)

- **PR**: #294
- **Session**: S14, started 2026-10-27 15:59 ET
- **Review**: 1 rounds, peak high, cap 2
- **Gate**: CANNOT CHECK — full gate not run, lease held by ST-8
- **Cost**: CANNOT CHECK — no CLAUDE_CODE_SESSION_ID in this shell
- **Session start**: CANNOT CHECK — no CLAUDE_CODE_SESSION_ID

#### What happened

Ported Standing Tee's read-by-range guard into the template as POSIX `sh` plus `lib/view-reader.awk` (no Node): PreToolUse on Read and Bash, 400-line cap, fail-open on unreadable payloads and unparsed shell. Registered in `settings.json`, documented in `hooks/README.md`, exercised in `checks/hooks.sh` (block and allow rows plus registration table). Synced `.claude/` and rebuilt `plugin/`.

#### Decisions

Used a separate `view-reader.awk` forked from the gate tokenizer rather than extending `gate-reader.awk`, so gate and catch-up guards cannot drift.

#### Surprises

BSD awk treats `index(line, "\0")` like an empty-string search and falsely marked every text line binary until that check was removed from the awk line counter.

#### Insights

None.

## Session 13 — 2026-10-27 — Claude — the open-source direction shift, the roadmap reset to what is left, and the tracker by phase

**Totals.** Gate 30 min; cost $0 (a row's cost was CANNOT CHECK, so this is a floor).

**Rows folded.** OPS-144 (#287); OPS-147 (#289); OPS-148 (#290).

### OPS-144 — the roadmap re-planned for the open-source, terminal-first direction

- **PR**: #287
- **Session**: S13, started 2026-10-27 07:57 ET
- **Review**: 2 rounds, peak medium, cap 2
- **Gate**: 9 min
- **Cost**: CANNOT CHECK — a claude.ai cloud session; its transcript is not on the owner's machine for usage-report.sh
- **Session start**: CANNOT CHECK — started in a cloud session with no session-start marker

#### What happened

The owner explored taking Clauductor open source, starting from Andrew Baker's Ghostty Dock article (2026-10-21) and a read of Standing Tee's operating model at `e58b085`, then re-planned the roadmap in plan mode. The PR adds ADR-0011 and `docs/plans/direction-shift.md`. On the roadmap it:
- adds Phases 4 (*Learn from Standing Tee*, ST-8 to ST-21), 5 (*The product module*, PROD-1 to PROD-5) and 6 (*The terminal UI*, PANEL-60 to PANEL-65);
- moves Release to Phase 7 and Standing Tee's attach phase to Phase 8;
- cancels the browser overhaul (PANEL-46) and the rows built on it, the licensing rows (OPS-112, OPS-116, PANEL-54), and ST-6 and ST-7;
- reshapes REL-1, OPS-92b, OPS-104, OPS-113, OPS-114, OPS-115 and OPS-117;
- re-cuts the session plan from S13.

The owner queue drops the rename fallback (O-R2) and gains the new name's status. Plan only; nothing was built.

#### Decisions

All of them are the owner's, 2026-10-27:
- MIT, with nothing sold.
- The terminal UI is the only local face. The browser panel is frozen, then removed (PANEL-64). PANEL-46's rows are cancelled rather than paused, and a desktop app waits until after launch.
- The product module's screens live in both claude.ai artifacts and the terminal UI's read-only summary.
- The plugin is the only install path: lite by default with the product module, `--full` opt-in. The binary is the terminal UI only.
- ST-6 and ST-7 are cancelled.
- S13 starts with Standing Tee's cost ports.
- There is no rename fallback.
- Everything Standing Tee has that the template lacks is ported, reversing OPS-71.

`CLAUDE.md`'s prefix list was left to PROD-1, so this PR stays prose-only and runs the docs gate.

#### Surprises

- ADR-0010 was taken: ADRs and practice records share one number space, and PRAC-0010 holds it. The ADR became 0011.
- The cloud clone was shallow, so the docs gate could not diff against `origin/main` until the clone was unshallowed.
- Both docs gates failed `gate`'s `lease_verify by pid: LOST`. It looked like a load flake, since the check passes alone, but it is not one. It inherits the outer gate's `CLAUDUCTOR_LEASE_NONCE`, which is set only where there is no `clauductor` binary, and it fails on demand with that variable set. OPS-146 owns the fix. It is a pre-existing bug, not this PR's: on a machine with the binary the gate holds the lease through lock-run, which exports no nonce.
- `gh` has no valid token in the cloud container.

#### Insights

1 row(s), moved to the insights log.

### OPS-147 — the roadmap reset to what is left, with the finished rows in a history

- **PR**: #289
- **Session**: S14, started 2026-10-27 11:35 ET
- **Review**: 2 rounds, peak medium, cap 2
- **Gate**: 11 min
- **Cost**: CANNOT CHECK — a claude.ai cloud session; its transcript is not on the owner's machine for usage-report.sh
- **Session start**: CANNOT CHECK — started in a cloud session with no session-start marker (the start time above is the session directory's, 15:35 UTC)

#### What happened

The same day as OPS-144's re-plan, the owner asked for a full reset of the roadmap and its tracker. They kept the history and the session numbers. Claude grouped the 77 open rows into 23 decisions, and the owner accepted every recommendation.

`docs/roadmap.md` now holds:
- the open rows in seven renumbered phases;
- the 29 merged rows that open rows' Deps still reach;
- the outcome checks o.5–o.9;
- a session plan that starts at S14.

Every other row, and sessions S4–S13, moved word for word to `docs/roadmap-history.md`. A local roadmap rule keeps the moved ids taken. Issue #288 was opened for OPS-99. The owner queue records Q20 and Q11.

#### Decisions

All 23 are the owner's (2026-10-27, by chat). The ones with reasons:
- The panel's plan runner (PANEL-58, PANEL-59) is cancelled. ST-15 to ST-19's terminal orchestration is the one way the plan runs itself, and it needs no binary (ADR-0006).
- PANEL-34 and OPS-114 are cancelled as superseded. PANEL-61 carries the docs link.
- OPS-79, OPS-80 and PANEL-33 are cancelled: there was no failure to chase. So are the browser outcome checks o.1, o.2, o.3 and o.10.
- OPS-115 becomes a GitHub Actions release, since the public repository's minutes are free. The site (OPS-117) waits until after launch. OPS-113 builds M1 and M4.
- OPS-91 makes the roadmap row the authority for a budget's kind, so no design is re-approved.
- OPS-32, OPS-58 and OPS-68 wait on OPS-86's ADR. The lows and the ADR come before anything new is built (S15–S16).
- Session numbers continue from S14. A real restart would have reused journal session numbers, which `checks/journal.sh` refuses.
- The owner merges from the Mac with `/merge-pr`: `gh` has no token in the cloud container, and the merge guard runs only on a `gh pr merge`.

#### Surprises

- A Dep must be a row of the roadmap, and the merged rows open rows depend on have Deps of their own. 18 direct dependencies became 29 once taken transitively.
- Round 1 of review (peak medium) found three things:
  - the new rule refused any pre-reset roadmap, including origin/main's, which broke `panel-suggest.sh` and four other readers;
  - `team-queue.sh --next-id` and archive-change's `o.<n>` would hand out REL-2 and o.10, both held by the history;
  - the check's fixture had no table after the history's session plan.

  The rule now refuses an id only when it comes with another change id. `--ids` feeds both producers. Each fix was shown failing with its control removed.
- The cloud gate failed `test (short)` on two `panel/install` tests that need an IPv6 loopback, which the container lacks. They fail the same way on main's code, and the PR has no framework diff. The process checks passed (552 s).

#### Insights

2 row(s), moved to the insights log.

### OPS-148 — the roadmap tracker grouped by phase

- **PR**: #290
- **Session**: S14, started 2026-10-27 11:35 ET
- **Review**: 1 round, peak low, cap 2
- **Gate**: 10 min
- **Cost**: CANNOT CHECK — a claude.ai cloud session; its transcript is not on the owner's machine for usage-report.sh
- **Session start**: CANNOT CHECK — started in a cloud session with no session-start marker (the start time above is the session directory's, 15:35 UTC)

#### What happened

Once the reset (OPS-147) had merged, the owner asked to see the session plan grouped into the seven phases. `roadmap-tracker.sh` dropped each row's phase, keeping only its `###` section. It now keeps the queue's phase number (0 outside every phase), and `local:roadmap-tracker` checks it on a two-phase fixture; that case fails without the change. The tracker's page, which lives in the artifact and not in this repository, gained a *By phase* view. It shows each phase under the owner's names, with the sessions its rows land in, and folds the session-by-session plan below it.

#### Decisions

- The phase names live in the page as constants: the owner's own seven, written 2026-10-27. The roadmap's queue parser gives a phase's number, not its title, and adding the title would be a template change to `roadmap-queue.sh`. If the phases are renumbered, the page's names need editing.
- A session that serves two phases shows under both, each time with only that phase's rows. The owner reads phases, and sessions mix them (S14, S18, S24–S26, S27).

#### Surprises

- The fixture's new row ids first collided with one the check adds later (`add_row C-1`), which broke two unrelated cases. The new rows became E-1 and F-1.
- Review (one round, three lows) found two gaps:
  - the check could not tell a numeric phase from a string, and the page groups with `===`;
  - a plugged parser's richer phase id (`2D`) would kill the build without naming the row.

  Both are fixed. The first error message written for the second gap itself threw: it read `.id` while `.` was the phase string. It now reads the bound row (`$r.id`).
- The cloud full gate on b420c6c failed `test (short)` on the same two IPv6-loopback tests as OPS-147 (no IPv6 in the container, no framework diff). The process checks passed (565 s).

#### Insights

None.

### The close (2026-10-27)

The whole day ran in claude.ai cloud sessions. The owner merged each PR from their Mac's terminal with `gh pr merge --squash`: this container's `gh` has no token, and the merge guard runs only on a `gh pr merge` that Claude runs. So the guard did not check #289 or #290. What backed each merge was the review that converged, the cloud gate's process checks, and the owner's call. The fold put OPS-144's record under S13 and the two reset rows under S14. S14 is not done: OPS-146 opens it next.

- **The tracker** shows the reset plan grouped by phase. Its data was rebuilt from `main` after #290 and matched the branch's byte for byte. The two old data files stay in the artifact: deleting them was refused as irreversible, and nothing reads them.
- **Owner queue**: Q20 (Actions return 2026-11-01; OPS-98 runs on the Mac) and Q11 (M1 and M4) were answered by chat and recorded in #289. The three relays to the COO (two from today, one from 2026-10-21) stay open.
- **Compound**: of today's three insight rows, `adr-numbers-share-a-space` is an instance of the ADR-numbering rule that caught it, and `rule-over-a-file-reads-its-own-context` is promoted to `local:roadmap-history`. `cloud-container-no-ipv6` stays Raw: whether the loopback tests should say why they cannot run, rather than fail, is a question for a row, not for this close. The topics `shell` (3) and `test` (6) are at the promotion trigger, as they were at Session 12's close. No ADR was drafted: an ADR is the owner's decision, and the owner ended the day here.
- **Team queue**: no item worked.
- **Flow** (30 days to 2026-10-27): review rounds 2, spend $4.28/wk, 9 in flight, approval wait 0 h. Cycle, lead, merges per week and change-fail are CANNOT CHECK: `gh` could not list the merged PRs.
- **Cost**, this cloud session, from `usage-report.sh` at list price: session (opus) $13.42, reviewer (opus) $4.92, total about $18.34. This session wrote OPS-147 and OPS-148; OPS-144's own session was another one, which this machine cannot read.

## Session 12 — 2026-10-26 — Rich — doctor, delegated approval and its stops, an agent closes a lane, the merge guard reads only open PRs, and the open lows

**Totals.** Gate 178 min (a row's gate was CANNOT CHECK, so this is a floor); cost $438.37.

**Rows folded.** OPS-109 (#240); OPS-119 (#241); OPS-124 (#247); OPS-126 (#239); OPS-127 (#246); OPS-128 (#256); OPS-130 (#258); OPS-131 (#270); OPS-132 (#268); OPS-134 (#266); OPS-135 (#263); OPS-136 (#264); OPS-137 (#278); OPS-138 (#283); PANEL-57 (#250).

### OPS-109 — `clauductor doctor` checks a machine's setup and a project's

- **PR**: #240
- **Session**: S12, started 2026-10-24 17:53 ET
- **Review**: 11 rounds, peak high, cap 2
- **Gate**: 28 min
- **Cost**: $110.51
- **Session start**: 55739 tokens, 0 min

#### What happened

`clauductor doctor` is built in three task groups: the command and the machine's tools, gh and
template (group 1), the project's model, drift and gate (group 2), and on macOS the panel's
registry, install, running, build, served project, config and trust lines, plus the prereqs line
and the docs that send a user to doctor (group 3). Doctor writes nothing, and every `fix:` it prints
is backed by the read-only check its own command runs: `prepareInstall` (new, in `install.go`),
`template.Survey` for diff, `planUpdate` for update, and `install.CheckAdd` (new, in
`panel/install/projects.go`) for the panel. The build took four workflow runs: groups 1 and 2 each
stopped at the review cap and were fixed and resumed.

#### Decisions

- The gh line tells a sign-out (`gh auth login`, with `--hostname` off github.com) apart from a gh
  too old for `auth token` (a note) and any other failure (a warning, no fix). design.md's gh row
  says any non-zero exit means `gh auth login`; the build follows D8 for the other causes. Flagged
  to the owner; design.md unchanged.
- The cap-stopped findings in groups 1 and 2 were fixed rather than deferred: each was a fix doctor
  printed that its own command would refuse (Claude's triage, by the owner's delegation).
- The panel lines' fix comes from `install.CheckAdd`, which `AddProject` now calls, so doctor and
  `panel add` cannot drift; the same shape as `prepareInstall` beside `install`.

#### Surprises

- Review found the same class three times: doctor predicting a command's outcome with code other
  than the command's own. Each fix moved the command's read-only checks into a function both call.
- `verify-change.sh` reads any backticked token with a `.` or `/` as a path claim, so
  `config.LoadProjects`, `template.Resolve` and `/bin/sh` failed tasks that were done. Five task
  lines now name their file as well.
- `usage-report.sh` reads $40.30 for this session while the build workflow's own group costs sum to
  $110.51; the Cost above is the workflow's figure. One reading misses workflow agents.

#### Insights

1 row(s), moved to the insights log.

### OPS-119 — update keeps a role's eval receipt whole

- **PR**: #241
- **Session**: S12, started 2026-10-24 17:54 ET
- **Review**: 2 rounds, peak medium, cap 3 (round 2 peaked low; its two lows fixed after it, not re-reviewed)
- **Gate**: 20 min (the full run on dffb89cd3: red at process checks, six of them failing on `git: couldn't create cache file .../xcrun_db` under the Bash sandbox, which denies writes to `/var/folders`; the final gate runs outside it and follows this commit)
- **Cost**: $2.84 (usage-report.sh, this session, at the fragment's commit)
- **Session start**: 122155 tokens, 0 min

#### What happened

`clauductor update` merges `.claude/model-roles.json` add-only, so it re-added the template's
`roles.reviewer.eval.baseline` beside this repo's eval receipt (OPS-97, #170) on every run, and
`survey` reported the baseline as a key the project lacked. `template.MergeModelRoles` now leaves
`roles.<role>.eval` whole when either side holds a `receipt`, and both production callers use it
(`planExtras` for install and update, survey's `configNote`). The generic `MergeAddOnly` had no
other caller and is gone. `TestUpdateKeepsAnEvalReceiptWhole` failed first on the old merge; table
cases cover a template receipt, two baselines that still merge, an existing role that still gains
`tiers`, and a role literally named `eval`. Update from this checkout now leaves this repo's file
byte for byte.

#### Decisions

- Which copy drifts: the template's are the source. The baseline was update's own doing, not drift;
  `orchestration/config.json` already matches the template (`clauductor diff`, 2026-10-24), and
  only `docs/conventions.md` and `docs/playbook.md` differ, which are doc-tier and the project's.
- The guard checks a receipt on both sides (round 1's finding 6): a project baseline beside a
  template receipt would name a receipt file the project lacks.
- The close marks two insight rows Fixed by this row (a row's PR never edits the log): 2026-10-19
  "update follows CLAUDUCTOR_FRAMEWORK" and 2026-10-24 "update/non-interactive".

#### Surprises

The full gate inside the Bash sandbox fails six process checks on xcrun's cache under
`/var/folders`, and the gate stops at that step, so the Go tests never ran; the gate has to run
outside the sandbox.

The first gate outside it, on 7db3b801b, went red at `test (race)` on
`TestCatchUpRebuildNoticesIgnoreAlertsNotify` (its two pushes in the other order), the flake #234
owns; ten `-race` runs of it alone passed, and the gate was re-run whole.

#### Insights

1 row(s), moved to the insights log.

### OPS-124 — catch-up's four lows

- **PR**: #247
- **Session**: S13, started 2026-10-24 20:29 ET
- **Review**: 3 rounds, peak high, cap 3 (round 3 peaked medium: its two placement findings fixed after it, not re-reviewed; the rest to #249)
- **Gate**: 26 min (the full run on 9f564d767, outside the Bash sandbox: red at `test (race)`, `TestPluginChecksPassInScaffoldedProject`, where the variable-command rule refused the plugin's `${CLAUDE_PLUGIN_ROOT}/...` paths; fixed, and the final gate follows this commit)
- **Cost**: $10.34 (usage-report.sh, this session, at the fragment's commit)
- **Session start**: 122905 tokens, 0 min

#### What happened

The four lows OPS-118's build left (#214). (1) merge-pr's step-5 guard
(`checks/change-process.sh`) now passes one legitimate edit: `merge_pr_catchup`'s line, *In the
branch's checkout, first catch it up* with `git fetch origin main && git merge origin/main`, alone
between the first step-2 heading and its first line. The line names its checkout itself, since
merge-pr may start in the main checkout. Anywhere else, worded
otherwise or fenced, a fetch or merge still falls to `merge_pr_cmds`, which lists neither. No span
outside step 5 may change directory (`cd`/`pushd` quoted, escaped, assigned or after `then`), name
its command by a variable, or set `GIT_*` (an export included). A `_green` helper fails a green case
that edits nothing or is refused, and 21 new red cases cover the forms review found. What review
left at the cap (fragments the guard never reads, an `eval` of split words, two false refusals)
is #249's. Each rule was
shown load-bearing by mutation. Step 5's objections print with `printf`: `echo` in `sh` cut one
short at `\c`. (2) The roadmap's *After each merge the owner* bullet now says catch-up is on only on
exit 0 printing `on`. (3) `selfUpdater.restarted` is gone: it was written and never read. (4)
`change-cost.sh` votes a lane for the change when a session's lead recorded the branch there last,
*or* more often than any other branch (a tie falls back to last) in a lane not named after the last
branch. OPS-118 read $0 with both lanes rejected. Its build lead recorded its branch 709 times, then
`ops/ops-118-archive` 237; its proposal lead 1823, then `ops/session-9-close` 552. It now reads
$214.17. PANEL-48's `session-3-close` lane (168 to 86, named after its last branch) stays rejected.
`checks/change-tools.sh`'s new v5–v8 lanes fail on the old script.

#### Decisions

- One fixed line in one place, not a rule about how a merge is aimed. Round 1 broke an aim rule
  (`-c core.worktree`, `GIT_WORK_TREE=`, `env -C`, `sh -c`, a fetch into `main:main`), and round 2
  broke a list of spans allowed anywhere before step 5 (prose, step 4, an earlier export). The forms
  that aim a merge elsewhere are endless; the place the line may stand is one.
- Majority, not merge time, for the vote: offline, needs no `gh`, and every existing fixture's
  tie keeps its verdict. The lane-name rule keeps a close lane out of the count vote.
- $214.17 overstates the build-change $200.24 the archive recorded: a counted lane counts whole,
  the archive and the session-9 close in those lanes included. Named as a known limit in the
  header; the archived figure is left as it is.

#### Surprises

`clauductor update` with no terminal again cancelled its file reviews (insight
`update/non-interactive`). The three changed copies matched the template byte for byte at HEAD, so
they were copied across.

An apostrophe in a comment inside the single-quoted awk program ended the shell's quote, three
times.

The first full gate went red in the plugin's scaffolded project, not in either check set: the
plugin build rewrites `.claude/` paths in merge-pr to `${CLAUDE_PLUGIN_ROOT}/...`, so a span began
with `$` and the rule against a command named by a variable refused it (round 3's finding 4, in
a form review had not seen). The rule now takes only a bare variable with arguments, and a green
case holds a plugin path in the template's own checks.

#### Insights

2 row(s), moved to the insights log.

### OPS-126 — OPS-125's follow-ups: a build lane marks itself, landed rows stop being offered

- **PR**: #239
- **Session**: S12, started 2026-10-24 17:53 ET
- **Review**: 3 rounds, peak medium, cap 3 (round 3's three fixable findings fixed after it, not re-reviewed; its other two, and two found earlier, are KNOWN LIMITS queued as OPS-129)
- **Gate**: CANNOT CHECK — no full gate ran before this commit; the final gate follows it
- **Cost**: $12.21 (usage-report.sh, this session, at the fragment's commit)
- **Session start**: 122155 tokens, 0 min

#### What happened

The six items of #223, in template/ and synced to this repo's copy and the plugin.
build-change's preflight now runs `session-start/marker.sh` itself (on the expected branch only), so
a build lane, whose first prompt must start with `/build-change`, gets its draft PR and marker, and
a marker WARNING reaches `report.warnings`. The marker's empty commit is now made from HEAD's tree,
never the index, so a resumed run's staged work stays uncommitted. The session plan's Rows cell
learns `ID (proposal)` (the session only proposes it; it lands once its proposal is on main) and
`A + B` (one PR titled by A closes both), and R records carry `kind` and `lead`. panel-suggest now
reads the fragments for every kind: `build`, `ops` and `propose` never offer a landed row, nor a
batch's later row while its lead is open, and `close` counts a planned proposal landed once it is on
main. checks/roadmap.sh exempts only the marker's `roadmap-plan.sh` call, not the whole file.
checks/panel-contract.sh's leading-command guard accepts a one-character command (`/x`) and holds
build-change's preflight to running the marker; the shipped template set (build, propose, fix, ops,
close) is asserted by this repo's own `local:panel-templates`, over the preset and this repo's
config.

#### Decisions

- The marker runs inside build-change's preflight rather than the panel accepting a lead
  `/session-start` on a build prompt: the panel's approval warning reads a build prompt's leading
  `/build-change`, and the preflight is an executed step where the description was only read.
- The plan grammar learns `(proposal)` and `A + B` rather than dropping batches from the plan:
  S11's OPS-89 + OPS-103 and OPS-109's proposal-only session were real plan shapes the parser
  refused or misread.
- The shipped template set is this repo's check, not every project's: a project's panel.json is its
  own and may drop a template on purpose (review round 1).
- A batch's later row waits only while its lead is open: once the lead merges, is cancelled or lands
  without closing it (S11's OPS-103 shipped apart from its batch), it is offered (review round 1).
- An id may be planned twice, as `ID (proposal)` and then, in a later session, as its build: the
  proposal's landing closes only the proposal's session, and the marker numbers the build lane
  from its build entry once `changes/<id>/proposal.md` exists (review round 2).
- This repo's own panel config is held to the preset's ids, the authority, not to a hard-coded
  list; the list stays only for the preset itself (review round 2).
- The preflight takes a marker as run only from marker.sh's closed set of success lines
  (`MARKER_OK`), not "anything but a WARNING": a skipped step or a shell error now warns, and
  `checks/marker.sh` holds the set to the lines marker.sh really prints (review round 3).
- What review left at the cap is queued as OPS-129 (filed as OPS-127, renumbered when #245 took that id): a proposal-only session's close, `(proposal)`
  on an ops row, the build lane's orientation still being description, and the preflight's marker
  pushing before a later stop.

#### Surprises

- `clauductor update` with no terminal cancelled its reviews and rewrote `.claude/model-roles.json`
  again (logged in S11, update/non-interactive); reverted by hand.
- The sandbox refuses writes under `.claude/workflows/` and `.claude/skills/`, and the default Go
  build cache, so syncing the copies and building the plugin needed an unsandboxed copy and
  `GOCACHE=$TMPDIR/gocache`.
- Round 1 found that putting the marker in the preflight exposed its `git commit --allow-empty`,
  which commits whatever is staged: harmless at a session's start, not beside a resumed build.

#### Insights

3 row(s), moved to the insights log.

### OPS-127 — a project can delegate a change's approval to Claude, by mechanism

- **PR**: #246
- **Session**: S12, started 2026-10-24 20:28 ET
- **Review**: 4 rounds, peak high, cap 3 (round 3 peaked normal; a fourth, on the delta after it and the key, peaked normal: both normals and five lows fixed after it, not re-reviewed; the guarded list's derivation from AGENTS.md's table and a detached commit past the tip are OPS-130's)
- **Gate**: 25 min (the full run on cba656ed2, after two red runs: an empty, ignored `template/.claude/.cc-writes/` the harness left, then two self-test cases that could not reach a plugin project's framework list; the final gate follows this commit)
- **Cost**: $11.88 (usage-report.sh, this session, at the fragment's commit)
- **Session start**: 122787 tokens, 0 min

#### What happened

A new `project.conf` key, `APPROVAL_DELEGATED` (default `"0"` in `lib/conf.sh` and the template),
lets `change-approval.sh <id> --delegate "<owner>"` record
`**Approved:** <date> by Claude (delegated by <owner>) · design <hash>`. It refuses while the key is
off on `origin/<main>`, when `<owner>` is not `OWNER_NAME` (when that is set), and for a change
that touches a guarded path. Two lists guard: `.claude/lib/approval-guarded.txt`, the framework's
(the approval and verify scripts, the review cap, budget enforcement, `model-roles.json`,
`project.conf`, settings, hooks, checks, agents, `lib/`, build-change, `propose` and `merge-pr`, CI
workflows, `scripts/ci/`, `AGENTS.md`), kept current by `update` and the plugin; and
`.claude/local/approval-guarded.txt`, the project's own. "Touches" is a path named in a task line
of `tasks.md`, or, on the change's branch, a path its diff from the merge base changes (untracked
files too). Matching is by whole path segments, so `src/home` catches `src/home/greeting.ts` and
`template/.claude/hooks/x.sh` but not `src/homepage`. The diff is taken with `--no-renames`, so a
guarded file moved out of its place is still seen, and "the change's branch" is its name, a
detached HEAD at its tip, or a CI pull request's `GITHUB_HEAD_REF`.

Where the authority cannot be read, a delegated approval does not hold (fail closed): outside a
repository (a `GATE_CLEAN_ROOM="archive"` gate), in a clone with a remote but no `origin/<main>`
(a shallow CI checkout; this repo's `test.yml` now fetches every branch), or on a change branch that
shares no commit with main. Only a repository with no remote reads the key and the lists from disk.

The check mode, which `checks/changes.sh` calls for every open change, re-tests the key, the owner
and the lists on every run, so turning the key off or later touching a guarded path voids a
delegated approval the way a design edit voids the owner's. An approval written `by Claude` in any
other form fails, and `--record` refuses any name starting with "claude" (any case), so the
delegated form is the only way Claude's name reaches the line.

`checks/changes.sh` gains fifteen self-test cases (among them a decision-log citation of a guarded
path passing, a task naming `AGENTS.md:40`, no repository, no `origin/main`, an owner named
Claudette or OWNER_NAME "Claude Martin" passing, "Claude Code" failing), and
`checks/change-tools.sh` the `--delegate` cases (a legitimate delegation read from `origin/main`,
main's project list catching a path the branch lacks, the key on only in the branch, a committed
rename, a detached HEAD and a CI head, the approval holding on main after merge). Every negative was
run against the script before it and failed there; mutating `authority()` to read nothing fails
the positive cases, and branch detection back to the name alone fails the detached-tip case.

#### Decisions

- **The authority is `origin/<main>`, not the branch, and its absence refuses.** The key and both
  lists are read from main, so a change branch cannot turn its own key on or empty its own list.
  Round 2's version fell back to disk whenever the ref was missing; review showed a `git archive`
  copy then recorded and passed a delegated approval of a branch that turned the key on and edited a
  hook. Disk is now read only in a repository with no remote, which has no main to read.
- **The list is hand-written, so it was reviewed against AGENTS.md's table** three times, each
  round finding more (guard rules in `guard.d`, evals, health, the gitleaks allowlist, the local
  skill fragments for `propose` and `merge-pr`). It stays hand-written: the table's cells name
  controls in prose, not paths a parser could read. The list's header names the table as its source.
- **The guard reads what the change does, not only what it says.** Round 1 matched the change's
  prose as substrings; review found that both missed a diff naming no path and blocked a proposal
  that merely mentioned `AGENTS.md`. Task lines and the branch diff replace the prose.
- **The framework list lives in `lib/`**, so the plugin reads it from `$CLAUDUCTOR_FW` and `update`
  replaces it; a project's additions go in the local layer, which neither writes. Round 1's
  scaffold placement in `plugin/build.go` is reverted.
- **This repo's key went on in this PR, on the owner's own word.** The permission classifier
  refused Claude's edit setting `APPROVAL_DELEGATED="1"`, rightly: it grants Claude the approval.
  It became an owner-queue item. When the owner relayed through another session that it should go
  in here, the lane still declined: a peer session's relay is not the owner. The owner then said so
  in this lane's own session (2026-10-25), and the line went in. With the key on here, this repo also
  got its own `.claude/local/approval-guarded.txt` (the team-queue states and the roadmap tracker,
  the local controls AGENTS.md's table names). OPS-128 puts the key to use.

#### Surprises

The branch the panel started this lane on was #245's pre-squash branch, already merged; the
orchestrator flagged it, and the lane reset to `origin/main` before any edit (the draft PR the
marker opened had pushed the four stale commits; the push here replaces them).

The race step's plugin run failed two self-test cases the template run passed: a plugin project's
framework list is the plugin's own (`$CLAUDUCTOR_FW/lib`), so deleting the fixture's list changed
nothing. That is the authority working, not a gap; the two cases now run the fixture's own copy of
the script, which reads the fixture's list in either layout.

#### Insights

1 row(s), moved to the insights log.

### OPS-128 — delegated approval used in /propose, start-project and Who decides

- **PR**: #256
- **Session**: S12, started 2026-10-25 06:05 ET
- **Review**: 6 rounds, peak high, cap 3 (round 3 peaked medium: an approval the owner gave could be replaced by a delegated one after an edit; the owner asked for a fourth on the delta, which peaked medium: the fix read the Approved line after `--revoke` had erased it; the fifth, on that fix, peaked medium: an owner's overrule of a delegated approval left a delegated line last in the history, so a later `--delegate` could replace it. The owner ruled the fix into the script; the sixth, on it, converged at low, and its three lows were fixed after it, not re-reviewed)
- **Gate**: 30 min (the full run on 02916934, green; the one on fd6c2cb1 was red at `test (race)`: the new AGENTS.md row ran 336 characters in the plugin's scaffold, over the 320 ceiling. The final gate follows the script's fix)
- **Cost**: $15.23 (usage-report.sh, this session, at the fragment's commit)
- **Session start**: 123182 tokens, 0 min

#### What happened

`/propose` gains step 3a: the stops a skill step keeps (the review did not converge; money, legal,
outward, irreversible or at-the-computer work; an ADR or practice record, or a row that keeps its
own approval; a `**Risk:**` below the row's). Otherwise it runs OPS-127's `change-approval.sh
--delegate` (which now also refuses a change approved before, standing or revoked), then opens the
proposal PR with an `Approved: by Claude on delegation` line, pushes the owner one line and lands
without waiting. The owner overrules before
landing with `--revoke`, after it with a `proposal: <id>` revision PR on an ops branch carrying their
`--record`. `start-project` offers `APPROVAL_DELEGATED` ("0" unless the owner says). AGENTS.md's
*Who decides* and its table, the conventions, the playbook and `changes/README.md` say who approves
now, in both copies, and `checks/change-process.sh` holds step 3a's call to the script in place. The
session plan's owner cells from S13 on name only what stays the owner's.

#### Decisions

- Any revoke means only the owner re-approves (the owner's ruling, 2026-10-25, after round 5).
  `change-approval.sh --revoke` now writes `**Status:** awaiting approval (revoked; was approved by
  <who>)`, and `--delegate` refuses while that marker or any Approved line stands; only `--record`
  clears it. Prose stops that read the history had failed review twice, so the stop moved into the
  script. The cost: Claude revoking its own delegated approval to edit a design now waits for the
  owner.
- The stops the script does not test (outward, Risk lowered) stay a
  skill step here and move into `change-approval.sh` under a new row, OPS-131, placed in S13: OPS-130
  was editing the script and its checks in a parallel lane, so a second edit there would have
  conflicted. OPS-131 lands before S15 proposes OPS-116's signer, the first outward change.
- The delegation row in AGENTS.md's table bolds only what runs a control, `.claude/change-approval.sh`,
  `checks/changes.sh` and, since OPS-130 (#258) merged first, its `checks/approval-guarded.sh`: that
  check fails a bold path the guarded list lacks, and `start-project`, which names the key, is not
  guarded. Agreed with OPS-130's lane: whichever merged second names the other's check.
- Trimmed the session plan's owner cells: PANEL-50, PANEL-43 and PANEL-38's proposal approvals came
  out (delegable now); PANEL-46's split, OPS-92a, OPS-116, OPS-117, OPS-42 and PANEL-56 stayed, each
  with the reason it stays.
- Held the merge for the owner's own go in this lane, after the pushed diff, as the row says: a peer
  session's relayed "merge it" is not that.

#### Surprises

A peer session relayed "merge it" while #256 held only the marker commit, so the go would have
covered a diff nobody had read. The merge of `origin/main` conflicted on the S12/S13 session-plan
rows, which OPS-130's #257 and this row both edited. The new AGENTS.md row passed
`agents-md-budget` in both of this repo's sets at 311 characters, then failed in the plugin's
scaffold at 336: the plugin rewrites `.claude/` to `${CLAUDE_PLUGIN_ROOT}/` and `/propose` to
`/clauductor:propose`, so only the 12-minute race test sees the row as a plugin project gets it.

#### Insights

2 row(s), moved to the insights log.

### OPS-130 — the guarded list follows AGENTS.md's table; a detached HEAD past the tip is the branch's

- **PR**: #258
- **Session**: S12, started 2026-10-25 06:28 ET
- **Review**: 3 rounds, peak high, cap 3 (round 3 peaked low: the one low, a first match hiding a project's skill fragment behind the framework's, fixed after it, not re-reviewed)
- **Gate**: 10 min (the full run on 8b8234741, after 27 min queued behind OPS-128's gate: red at `test (short)`, `TestNoProjectPathsLeft`, on five self-test lines that spelt a fixture path the way the model reaches into a project; fixed, and the final gate follows this commit)
- **Cost**: $9.82 (usage-report.sh, this session, at the fragment's commit)
- **Session start**: 123170 tokens, 0 min

#### What happened

OPS-127 left two known limits past its cap, and this row closes both.

**The guarded list has an authority.** A new check, `checks/approval-guarded.sh`, reads every name
that AGENTS.md's "What executes each rule" table puts in bold in its executor column. It resolves
each name to a file and fails any name that neither `.claude/lib/approval-guarded.txt` nor the
project's `.claude/local/approval-guarded.txt` guards, on whole path segments, as
`change-approval.sh` matches them. Names resolve as follows:

- a bare name (`pr-merge-guard.sh`, `checks/gate.sh`, `merge-pr`, `local:team-queue`) goes to
  every file it names under `.claude/` and the usual control directories, so a project's
  `.claude/local/skills/merge-pr` fragment is checked beside the framework's skill;
- a glob goes to its directory;
- a shell function (`roadmap_queue`) goes to the `lib/` file that defines it;
- an agent's `tools:` line goes to `.claude/agents`.

A name that resolves to nothing fails too, so a typo or a rename cannot pass as guarded. Under the
plugin, the check reads the plugin root's list (the rewriter points the path there), and a name
found only in the plugin root passes, since the plugin cache is read-only to a project's branches.
On this repo the table names 74 files and the template's names 67, all guarded. The check holds
with `APPROVAL_DELEGATED` off too: the list must be whole before the key turns on.

**A detached HEAD past the change's tip is the branch's.** In `change-approval.sh`, a new
`on_branch()` counts HEAD as the change's work in two cases, so its diff from the merge base is
checked for guarded paths:

- HEAD is at or past a tip of `change/<id>` (local or origin's) that main does not already contain;
- HEAD has commits since main that touch `changes/<id>/`.

A HEAD that main contains is never the branch's, so a merged change, or the main checkout, is still
judged by its task lines alone. `checks/change-tools.sh` gains these cases:

- detached past the tip: refused;
- a detached amend that adds a hook: refused;
- a detached HEAD on main past the tip: not charged;
- `ops/other` cut from main while the change branch sits at main: not charged;
- the main checkout at the branch's commit with an untracked hook: not charged.

#### Decisions

- **History or content, never a bare tip.** Round 1 counted any HEAD past the branch's tip. Review
  showed that a fresh `change/<id>` still at main has a tip every branch is past, so the change was
  charged with every other branch's diff. A tip identifies the branch only when main lacks it.
  The content signal catches the rest, because every build ticks `tasks.md`.
- **A bold name that is no file fails, rather than being skipped.** Round 2 found `gitleaks` and
  `tsc --noEmit` in bold, which nothing resolves. The check stays strict: bold in that column
  marks what executes a control, so the fix is to name the file that runs it, or unbold it, and
  the message says so.
- **The content signal's reach is accepted.** A branch that edits `changes/<id>/` and also touches a
  guarded path refuses that change's delegation. A refusal sends the approval to the owner, the
  safe side.
- **No AGENTS.md row here.** OPS-128 (#256) adds the delegation row to both tables. By agreement
  with its lane, whichever lands second names `checks/approval-guarded.sh` in that row.

#### Surprises

The first negative test for a detached HEAD passed vacuously. `git checkout` of the fixture's base
had deleted the untracked `.claude/hooks/` it relied on, so nothing guarded was left to find. It
went red only after a `mkdir -p` after the checkout. Every new case was then run against origin/main's
script and round 1's, and failed on both.

The check passed in the template and failed in every plugin project: it read the framework list
from `$1/.claude/lib/`, and a plugin project keeps it in the plugin root. The scaffold test
(`TestPluginChecksPassInScaffoldedProject`) is not in the short suite, and review found the
failure by reading the code. Passing the list as a literal `"$ROOT/.claude/lib/..."` lets the
plugin rewriter point it at the right place.

The scaffold test took 632 s on a machine running two other lanes' checks, so it exceeded
`go test`'s 10-minute default timeout once with no hang. `ps` showed the checks advancing.

The full gate went red on the short suite, which had not run since round 1: `TestNoProjectPathsLeft`
reads a fixture's `git add -f .claude/hooks/...` and an expected `(.claude/skills/...)` as the
model still reaching into a project's `.claude/`. The lines now spell the fixture as `"$R/..."` and
compare paths without the prefix. Running checks by name is not running the short suite.

#### Insights

2 row(s), moved to the insights log.

### OPS-131 — change-approval --delegate refuses what step 3a kept by hand

- **PR**: #270
- **Session**: S12, started 2026-10-25 14:43 ET
- **Review**: 12 rounds, peak high, cap 3 (rounds 1–3 to the cap: high, high, low; the owner asked for fresh rounds past it with no medium left at merge: round 4 medium, 5 medium (an escaped pipe), 6 three mediums (a backtick span, a marker's spelling, a removed marker's history), 7 two mediums (spans dropping a tier, a shallow clone), 8 a high (a custom parser's row shape) and three mediums, 9 medium (review-cap reading an odd Risk as normal), 10 two mediums (markers hidden by markup, entities and line breaks), 11 a high (a tag pattern ate the text after a lone `<`) and two mediums (unlisted invisibles and colons, a merge's resolution), 12 two mediums (link destinations, named entities) and two lows (control bytes, a row's backtick pairing), all fixed; merged on the owner's word without a 13th round)
- **Gate**: CANNOT CHECK — no full gate passed before this commit (the run on 66499ae3 was stopped, superseded); the final gate follows it
- **Cost**: $65.28 (usage-report.sh, this session: $47.24 session, $18.04 reviewers)
- **Session start**: 9505556 tokens, 103 min
- **Unreviewed at merge** *(at the close)*: round 12 read up to 66499ae3; its fix commit 913d7b09,
  3a8e944c (the AGENTS.md row shortened) and the two merges of `origin/main` landed without a
  review round, on Rich's word in the lane. #284 owns their post-merge review (row OPS-143, S13).

#### What happened

`change-approval.sh --delegate`, and the check over a delegated line, now refuse the stops
`/propose` step 3a kept as prose: a change touching `ADR_DIR` or `PRACTICES_DIR`; a proposal with
an `Approval:` marker in any spelling or place, or one its history ever held; a roadmap row with any
`Approval:` in its Change cell; a Risk the row states in a form the floor cannot read; a
`**Risk:**` below the row's (a row with none is normal); and a change ever approved before, by a
standing or revoked line, an Approved line in its history, or an entry in the clone's approval log
(`<git-common-dir>/clauductor/approvals.log`, which the owner starts with `--start-log`). The row
is the one row of `origin/main`'s roadmap table naming the change's id, and the proposal's
`**Roadmap row:**` must be it. `review-cap.sh` reads a row's Risk the same way and says UNKNOWN for
one it cannot read. Markers are read as they render (`marker_plain` in `.claude/lib/change.sh`),
left to right in CommonMark's order. PANEL-56, PANEL-57, OPS-42, OPS-92a, OPS-112, OPS-113, OPS-116
and OPS-117 carry `(Approval: owner)` in their change cells. Every refusal has a negative case
confirmed failing on a mutant of its line first: 75 mutants, none surviving.

#### Decisions

- The approval log (L-a, option A, the owner's call): a delegated approval is refused for a change
  this clone's log shows approved before, and no log refuses; `--start-log` takes the owner's name.
  Branch history alone could be rewritten by whoever controls the branch.
- The history is followed across a whole-directory move only, with `--full-history -m`, capped at
  50 hops (L-b), replacing `--follow`, which could read another change's history.
- Allow only the shapes the script can read; refuse every other, naming why: a shallow clone, a row
  that is not a line of the queue table, a parser's row that is not main's table row for the id,
  an odd Risk spelling. A refusal goes to the owner, who can always `--record`.
- Markers are read as rendered, not as written: escapes and code spans first, then only real
  tags, comments, links CommonMark makes and entities; what is not ASCII reads as nothing inside a
  word and as a colon outside one. The over-refusals this buys (`owner approval: none` in any past
  version, an unknown character after `approval`) are documented as the safe side.

#### Surprises

- Eight rows that keep the owner's approval carried no marker, PANEL-57 among them, whose proposal
  the owner had approved by hand the day before. Rule 4 again: the stop had lived in prose that
  each proposal's author re-read.
- Twelve review rounds, nine past the cap, each finding a new way to write a marker the reader did
  not see. A marker read as written has as many spellings as the renderer has features; the
  findings narrowed only once the reader followed CommonMark's own order and erred toward refusal
  on anything it did not model, and round 12 still found two.

#### Insights

2 row(s), moved to the insights log.

### OPS-132 — the merge guard refuses a PR that is not open, read in the repo the merge names

- **PR**: #268
- **Session**: S12, started 2026-10-25 13:53 ET
- **Review**: 3 rounds, peak medium, cap 3, then a second run at the owner's request with #271 and #267 in scope: round 4 peak medium (two mediums, two lows, all fixed); round 5 peak medium (three mediums, three lows, all fixed); round 6 peak high (a high, a medium, two lows, all fixed); round 7 peak high (three highs, a medium, three lows; fixed but one low, a renamed repository's old name, deferred to #282); round 8 peak high (a high, a low, both fixed); round 9 peak high (a high, a low, both fixed); round 10 peak high (a high, two lows, all fixed); round 11 peak high (a high, a medium, a low, all fixed); round 12 peak high (a high, two mediums, all fixed); round 13 found nothing (converged)
- **Gate**: CANNOT CHECK — no full gate ran before this commit; the first full run follows it, and its receipt covers this head
- **Cost**: $11.67 (usage-report.sh, this session, at the fragment's commit)
- **Session start**: 10564794 tokens, 53 min

#### What happened

Issue #217: #215 was closed as a duplicate at 18:35 Z on 2026-10-23 and merged twelve minutes
later, because `pr-merge-guard.sh` checked the gate evidence for the head commit and never asked
whether the PR was still open. A closed PR is another session's "do not land this", and the guard
let it through.

**Rule 14.** Right after the guard resolves the PR, before any evidence rule, it reads
`gh pr view --json state`. Anything other than `OPEN` is refused: `CLOSED`, `MERGED`, an empty
answer, or a gh failure. The refusal for a gh failure carries gh's first stderr line, so the
message says what to fix rather than guessing at auth.

**Every read names the PR the merge names.** Round 1 found the state read unpinned: `gh pr view 5`
asks gh's default repository, which in a fork after `gh repo set-default` is a different PR #5.
That was already true of rule 2's three reads (the checks and both head reads). A helper, `ghpr`,
now reads the PR the way the merge named it: by its URL, or by its number with the merge's own
`-R` word. All four reads go through it.

**What the guard cannot read, it refuses.** Round 2 found the words were not the whole story. gh
also picks the repository from `GH_REPO`/`GH_HOST` and from the directory it runs in, and the hook
reads from its own directory with the session's environment. Round 2's answer was a text match on
`GH_REPO`, `GH_HOST` and `cd`; round 3 got past it five ways (`GIT_DIR=`, `cd;`, `c\d`, a split
`export`, the payload's `cwd`), all tracked as #271. At the owner's request #271 and #267 came
into this row. The text match gave way to an allowlist by what the merge's words leave gh to
resolve. Before a URL or a hosted `-R` anything may come. Before `-R OWNER/NAME`, only
`cd <path> && `. A merge naming no repo must have nothing before it, and its payload `cwd` must be a
worktree of this repository. Rule 2b's `gh run list` now lists the PR's own repository's runs (the
`-R` word, or the URL's host/owner/name), and every remediation hint names the PR as the read does.

**The words the read stops at.** Round 4 found two more places the read and gh part. The PR reader
stops at `$`, a backtick or a glob's `(`, but gh does not, and it merges in the last `-R`. Round 4's
answer matched the text after the stop for a `-R`; round 5 beat it with quoting (`-\R`, `--re""po`)
and showed a quoted expansion that is a word of its own can be the flag itself
(`"$(… | base64 -d)"` is `-Racme/app`). So a stop now passes only by allowlist: a `$` in double
quotes, in the command's last word (a trailing `2>&1` or `2>/dev/null` aside), as the value of
`--body`, `--subject` or another value flag. Round 6 found words the read kept whole that the shell
does not: `{-R,acme/app}` and `-[R]`. An unquoted character now passes only from a small set
(letters, digits, `_./:@%+=,-`, non-ASCII), the same allowlist turn one level down. Round 7
found three more. A process substitution, `<(…)`, is a word, but the read took it for a redirect
and stopped, while gh got the `-R` after it. A quoted `"$@"`, `"${a[@]}"` or zsh
`"${(@s.:.)x}"` splits into several words. And rule 1's raw text match was passed by
`--au''to` and `--\admin`. A redirect now ends the read only when redirects alone, each with
one plain target, run to the end of the command (the reader already refused one before the last
word). The read also refuses
`--auto` or `--admin` once their quotes are out. Round 8 found an empty repo flag, `-R ""` or
`--repo=`, after this repo's `-R`. gh keeps the last `-R`, and an empty one overrides nothing, so
gh fell back to `GH_REPO` or the directory while the reads used the `-R` before it. An empty
repo flag is now a repo the guard cannot confirm. Round 9 showed that the quoted forms still
allowed were not closed words in zsh, the Bash tool's shell. A `#` inside `"$(…)"` starts a
comment, so the `)"` the read took for its end could sit inside it, with a `-R` on the next
line. And `"$a[@]"` splits as `"$@"` does. Only `"$name"` or `"${name}"`, then letters, digits
and `_./-` (`"$TMPDIR/body.md"`), now pass as that last value. merge-pr's own form needs no `$`
at all. Round 10 found the step before the read. The guard joins each backslash-newline before
it reads, as the shell does, but it also joined one whose backslash was itself escaped. In
`"a\\<newline>"` that made the closing quote read as escaped, so the words after it went unread;
unquoted, a comment on the next line read as this repo's `-R`. A line now joins only when it
ends in an odd run of backslashes. Round 11 tested "anything may come before a URL or a hosted
`-R`". The reader had passed `eval`, `env -S` and `sudo -s` as prefixes that run the rest as
written, but each parses it again: `eval gh pr merge '--squash 6 --body' 5 -R github.com/acme/app`
merges #6, and `'--$X'` becomes `--admin`. A gh after them is now unreadable, as after `xargs`.
Rule 12 read a relative `--body-file` from the payload's directory, which a `cd` before the merge
had left. After any word before the merge, it now refuses a relative path, and so does
premise-check. Round 12 found both fixes keyed to spellings. getopt_long takes any unambiguous
prefix of a long option, so `env --split=` and `sudo --sh` still parsed the words again: env, sudo
and doas now pass only a short list of flags. And rule 12 tokenized the whole command, so a
`--body-file` in a comment, a later `echo` or a `gh api` before the merge was read as the merge's,
and a leading `gh --version` hid the `cd`. It now reads only the merge's own simple command, and
refuses a command with no merge in it or more than one.
`gh api --hostname` had been text-matched anywhere, a field's value included. The
reader now takes the host from the flag's own word, refuses a host that is computed, repeated or
not a plain name, and reads an absolute `https://api.github.com/` endpoint on github.com, where gh
sends it whatever `--hostname` or `GH_HOST` says. An unprovable value of any shorthand (`-f $X`),
not only `-X`, now leaves the merge unread, as the long forms already did: it can split into a
`--hostname`.

**premise-check reads the PR the merge names (#267).** Extension rules now receive `GUARD_PR_REF`
and `GUARD_PR_REPO`. premise-check reads through them, before it changes to `ROOT`, so a bare read
resolves in the directory the guard checked. Without `GUARD_PR_REF` it refuses, naming an older
guard as the cause.

`checks/merge-guard.sh` gains these cases:

- CLOSED (the refusal names it), MERGED, a gh error (named), an empty answer: refused;
- CLOSED via this repo's PR URL, and via `gh api -X PUT repos/<owner>/<repo>/pulls/N/merge`: refused;
- CLOSED by `-R`, by URL or through `gh api` while gh's default repo has an OPEN PR of that
  number: refused; OPEN by `-R` or `gh api` while the default repo's is CLOSED: allowed;
- a workflow success only in gh's default repo, for a PR named by `-R` or URL: refused;
- anything before a merge naming no repo, and anything but `cd <path> && ` before `-R OWNER/NAME`:
  refused; a `cd` or `GH_HOST=` before a URL or `-R HOST/OWNER/NAME`: allowed;
- a merge naming no repo whose payload `cwd` is another repository, missing or empty: refused;
  from a worktree or a subdirectory of this one: allowed;
- unpinning the checks, head or branch read fails the suite: the stub answers pinned and default
  reads apart;
- a `-R` after a backtick, a quoted substitution or a glob qualifier, plain or quote-split; an
  unquoted expansion; a quoted expansion as a word of its own; a brace or glob that expands into a
  `-R`: refused; a quoted variable as
  the last value of `--body`, `--body=` or `-b`, and merge-pr's form with `2>&1`: allowed;
- an escaped backslash before a newline, in quotes (hiding a `-R` or `--admin`) or unquoted
  (making a comment line read as `-R`): refused; a real continuation, and `"$TMPDIR"/…` (with
  provenance off: rule 12 reads only a path written out): allowed;
- `eval`, `env -S`, `--split-string` or its prefix `--split=`, `sudo -s`, `--sh` or `--logi`
  before a hosted `-R` or a URL: refused; `env VAR=…`, `sudo -E env -i`: allowed;
- with provenance on, a relative `--body-file` or `-F` after a `cd` or any command, a leading gh
  included: refused (and in premise-check); an absolute one: allowed;
- with provenance on, a `--body-file` or `commit_message` field outside the merge's own simple
  command (a comment, an `echo`, a `gh pr view` after it, a `gh api` before it): not read as the
  merge's; the merge piped on, and a substitution in a command before it: allowed;
- a PR URL with a later `/pull/<n>` in its path: read as its first, as gh reads it;
- a quoted `"$(…)"`, even last; a zsh comment or case pattern in it hiding its `)`; a zsh
  `"$a[@]"` set in the same command; a zsh modifier (`"$B:h"`): refused;
- a `-R` after `<(…)` or `>(…)`; a quoted `"$@"`, `"${a[@]}"` or zsh split; `--auto` or
  `--admin` split by quotes or escaped (named as rule 1): refused; redirects alone to the end
  (`> f 2>&1`) and `--body-file "$TMPDIR/…"` piped on (provenance off): allowed;
- an empty `-R ""`, `--repo=`, `-R=` or `-sR ''` after this repo's `-R`: refused;
- `gh api --hostname` read from a field's text, repeated or computed: refused or read on the default
  host, as gh would; an absolute github.com endpoint: read on github.com; an unquoted `-f` or `-H`
  value that splits into `--hostname`: refused;
- gh named by a path: refused, saying so.

The other checks that run the guard end to end (`artifacts`, `write-surfaces`, `premise-check`)
answer OPEN in their gh stubs. `merge-pr` step 1 now checks the PR is open and stops, without
reopening it, if it is not. The AGENTS.md row, the hooks README, the conventions and the playbook
name the refusal.

#### Decisions

- **Fail closed on no answer.** A guard that cannot read the state cannot know the PR is open, and
  the merge it is guarding is the irreversible step; a retry after `gh auth status` costs a minute.
- **Rule 14 runs before the evidence rules.** A closed PR's evidence is beside the point, and its
  refusal is the one that tells the session to stop and ask why.
- **Pin rule 2's reads in the same row.** They were the same class as the new read, in the same
  file, and leaving them would have let a fork's checkout judge a merge by another PR's checks.
- **An allowlist of what may come before the merge, not a list of what may not.** Round 2's text
  match on `GH_REPO`, `GH_HOST` and `cd` was a denylist, the kind `foreign_shape`'s comment records
  three rounds of review defeating, and round 3 defeated it the same way. The allowlist (option A)
  keeps merge-pr's form as it is. Option B, refusing every merge that does not name
  `-R HOST/OWNER/NAME` or a URL, would close the profile's own environment too, at the cost of
  changing merge-pr. The choice between them is the owner's, and the PR states both. *(At the
  close:)* Rich chose option A on 2026-10-26, typed in the lane, so the allowlist merged as built.
- **Past the cap at the owner's word.** Round 3 capped with #271's findings open. The owner asked for
  them and #267 to be fixed in this row and for review to start again, so rounds 4 on are a second
  run against a fresh cap.
- **Refuse words the read cannot pass, rather than read further.** Reading past an expansion means
  guessing what it expands to; a refusal names the form that reads whole.
- **The window between the read and the merge stays,** and so does the environment a shell profile
  sets. A close in that window still lands, as for every rule of a client-side hook; only a
  server-side control closes either. A renamed repository's old name, which GitHub redirects,
  still reads as a foreign repo: #282 owns it.

#### Surprises

The bug was older than the issue: rule 2's evidence reads had always read gh's default repository,
and nothing had noticed because this repo has no fork with a default set. The review found it only
because the new read was one line away.

Round 1's fix pinned every read to the merge's words and claimed the rest "resolves exactly as the
merge does". Round 2 showed two more inputs to gh's choice of repository, the environment and the
directory, that the words do not carry. The gh api case had also passed with its pin removed: its
stub answered the same state pinned or not, so it tested the state and not the pin.

#### Insights

2 row(s), moved to the insights log.

### OPS-134 — the session's and the change's cost readings agree

- **PR**: #266
- **Session**: S12, started 2026-10-25 13:32 ET
- **Review**: 3 rounds, peak medium, cap 3 (rounds 2 and 3 peaked low; round 2's two lows fixed and re-reviewed in round 3; round 3's one low, the `find` stand-in running its command on an empty list, is on a path the test cannot reach, left)
- **Gate**: 39 min (the full run on e2af9de6, after a wait for the lease behind panel-57-lane-close's gate)
- **Cost**: $11.59 (usage-report.sh, this session, at the fragment's commit)
- **Session start**: 8012546 tokens, 32 min

#### What happened

#253 reported that `usage-report.sh` read session 08c9d8f2 (OPS-109) as $40.30 and build-change's
report summed to $110.51. **usage-report was right**: 836 messages, $41.65 by now. Build-change
wrote `groups[].costUsd` from `change-cost.sh`, which reads the change's running total, so the
four workflow runs' entries (22.65, 30.16, 40.15, 47.71) were cumulative and their sum counted
early groups up to four times. `change-cost.sh ops-109-doctor` reads $50.25 for the change, both
sessions included.

- **build-change.js**: each group entry carries `changeCostUsd` (the running total) and `groupUsd`
  (this reading less the one before it, the first from preflight's). An unknown reading on either
  side, or one below the reading before it, gives `null`, and a fall is a warning in the report
  and the log. `costUsd` per group is gone; nothing in the repo read it.
- **lib/usage.sh** (#207): `usage_tag` and `USAGE_TAGGED`, one reader that tags each line with its
  own file. A cut-off last line of the lead's transcript is no longer joined to its first agent's
  first line, and a cut-off `meta.json` loses only its own agent's role. `change-cost.sh` reads
  through the same pair.
- **change-cost.sh** (#249 item 4): a counted lane no longer counts whole. A lead's messages there
  on a named other branch, after its last on the change's (or all of them, from a session that
  never recorded it there), are the next work. OPS-118 reads $206.37, from $214.17, against
  build-change's $200.24. Items 1–3 of #249 stay with OPS-138.

Every new test failed first: the agreement fixture (usage-report 11 against change-cost 12 on the
old `usage.sh`), the cut-off line and meta, the live-worktree and lead-moved-on lanes on the old
`change-cost.sh`, and build-change's null and falling readings with the carry and the guard
removed.

#### Decisions

- **Two fields, not a corrected `costUsd`.** A field that changed meaning in place would read as
  right in every old report; `changeCostUsd` and `groupUsd` say which figure each is.
- **A falling reading is `null` and a warning, never a negative spend.** The reading is a sum over
  transcripts, so it falls only when one moved or was lost, which is worth a reader's eye.
- **A lane is cut at its lead's last message on the change's branch, not at the merge.** The
  transcripts carry the branch per message and no merge time, and an agent records no branch of
  its own: an agent run there after the lead moved on still counts, a known limit in the header.

#### Surprises

The cut-off-meta test of round 1 caught the old reader only on this Mac: `find` lists a directory
in the file system's order, and the test needed a broken meta listed before the builder's. A
`find` stand-in that lists in name order, `-exec CMD {} +` included, makes it fail on every
machine. Exercising that stand-in by hand first looked like it did not sort: zsh had
`/usr/bin/find` hashed, so a PATH stand-in run from the interactive shell never ran.

#### Insights

2 row(s), moved to the insights log.

### OPS-135 — the CLI's lows: one error print, a disabled launchd label, doctor's three

- **PR**: #263
- **Session**: S12, started 2026-10-25 13:09 ET
- **Review**: 1 round, peak low, cap 3 (three lows, all fixed after it, not re-reviewed)
- **Gate**: CANNOT CHECK — no full gate ran before this commit; the final gate follows it
- **Cost**: $3.46 (usage-report.sh, this session, at the fragment's commit)
- **Session start**: 125314 tokens, 1 min

#### What happened

Three issues, each shown failing first. (1) #251: the root command sets `SilenceErrors`, and its
`PersistentPreRun` sets `SilenceUsage` once flags and arguments have passed, so `Main` prints a
command's error once and only a mistyped command line prints usage; `Main` keeps cobra's
`Run 'clauductor --help' for usage.` after an unknown command, which `SilenceErrors` also silenced.
`TestMain`'s helper now runs `Main` itself instead of a hand copy of it, which is why it could never
have caught the double print. A test refuses a subcommand's own `PersistentPreRun`, which would
replace the root's. (2) #202: `clauductor panel install` reads `launchctl print-disabled` before it
changes anything and, when launchd holds the panel's label as disabled, stops with an error naming
the `launchctl enable gui/<uid>/com.clauductor.panel` to run; it never runs it. `docs/panel.md`'s
install steps say so. (3) #255: doctor says git is not on PATH when it is not, rather than "not in a
git repository"; DOCTOR-1-S5's AND clause is cited by the test that drives it; DOCTOR-1-S7's printed
fix is proven by `panel install --project`'s own path (a new `panelInstallExec` seam, launchd
stood in), not by `AddProject`'s options.

#### Decisions

- #202 stops and names the command instead of running `launchctl enable`: the row says so, and
  enabling a label launchd holds is the owner's to do. The check runs before the binary copy and the
  token rotation, so a stopped install leaves nothing changed; a failing `print-disabled` lets the
  install go on as before.
- Usage is silenced in the root's `PersistentPreRun` rather than with `SilenceUsage` on the root, so
  a wrong flag or argument count still prints it.

#### Surprises

- In this lane's Bash sandbox Go's default build cache was unreadable (`operation not permitted`),
  so tests ran with `GOCACHE=$TMPDIR/gocache`; a test that binds a local port panics there.

#### Insights

None.

### OPS-136 — the template scripts' lows: verify's path claims, the clean-room docs gate, no-silent-wait, the sh -c child class, a validate test off the network

- **PR**: #264
- **Session**: S12, started 2026-10-25 13:09 ET
- **Review**: 2 rounds, peak medium, cap 3 (round 2 peaked low: its three lows fixed after it, not re-reviewed)
- **Gate**: CANNOT CHECK — no full gate ran before this commit: this repo's `.claude/hooks/` and `.claude/skills/` copies waited on the owner's copy, so the root hooks check was red; the final gate follows this commit
- **Cost**: $8.99 (usage-report.sh, this session, at the fragment's commit)
- **Session start**: 125478 tokens, 1 min

#### What happened

The row closes five open bugs in the template's scripts.

- **#252, verify-change's path claims.** A backticked token in a ticked task now counts as a path
  claim only when it is a path of the repo (in the diff, a directory of a changed file, or an
  object at HEAD), or has a directory part and an extension. A code identifier
  (`config.LoadProjects`), an absolute path or a bare name that is no file no longer fails a
  finished task. A task whose tokens are all dropped prints a `NOTE` saying it was not judged. The
  diff is read with `--no-renames`, so a task removing a renamed file's old name is judged.
- **#235, the docs gate in a clean room.** `run-local.sh --docs` reads the working tree only
  under `--dirty`. A clean-room run (`GATE_CLEAN_ROOM=archive`) tests the commit, so an untracked
  file beside it no longer refuses a docs-only gate. `--dirty` still refuses one.
- **#156, no-silent-wait.**
  - A launch clears the agent's old end marker, so a resumed agent's stale marker can't release
    its launcher.
  - The refusal says to re-run the wait while the agent runs, and to end the turn once it no
    longer does.
  - A new test covers a launcher whose agent was stopped: its end is allowed.
- **#232, the plugin's `sh -c` child.** The rewrite passes `CLAUDUCTOR_FW` to `-ce`, `-e -c` and a
  body over several lines, run over the whole script, not line by line. The test's detector reads
  the line before a child as the shell does (`commandWords`). It no longer reports a correct child
  inside `"$( … )"`. It does report any shell by any path with any unquoted options, so a form the
  rewrite misses fails the test.
- **#254, TestClaudePluginValidate off the network.** Its throwaway config sets
  `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1` and `DISABLE_TELEMETRY=1`. It ran in the sandbox
  with no denied connection.

Every new case was run against the old code and failed there.

#### Decisions

- **The fork-point rule went, instead of getting a test.** Round 1 found that no test failed
  without `git cat-file -e "$base:$q"`. A file at the fork point and not at HEAD is in the diff,
  and one at both is caught by the HEAD rule, so the rule was dead. Round 2 then found the
  exception, a rename, and `--no-renames` closed it the way `docs-gate.sh` already does.
- **The detector is wider than the rewrite on purpose.** The rewrite handles the forms the
  template uses. The detector reports every shell, path and option form, so a new spelling in the
  template fails `TestShellChildGetsPluginRoot` rather than shipping an empty variable.
- **A stopped agent's launcher ends its turn.** Round 1's medium was this row's own wording: the
  first fix told a launcher to wait for a marker that a stopped agent never writes. The refusal
  now gives the old release path back beside the re-run.

#### Surprises

`clauductor update` was refused on `.claude/hooks/no-silent-wait.sh` and later on
`.claude/skills/verify-change/SKILL.md`, which the lane's sandbox may not write. The owner copied
them by hand each time a review round changed them. Until then, the root hooks check tested the new
hook against the old copy and failed.

The rename case first renamed a file the branch had itself added. Under `--no-renames`, an add
followed by a rename nets to the new name alone, so the case failed for both scripts and showed
nothing. A second try renamed `.claude/change-cost.sh`, which `TestNoProjectPathsLeft` refuses in
a project check. The case now renames a plain fixture file of the fork point.

The new gate.sh case committed a receipt that the later stale-origin case read as its own, until the
case removed it (`rm -f "$REC"`).

#### Insights

2 row(s), moved to the insights log.

### OPS-137 — the panel's three lows: a restart's waiting re-notice, the hangup's wiring, the quota harness

- **PR**: #278
- **Session**: S12, started 2026-10-25 20:59 ET
- **Review**: 2 rounds, peak medium, cap 3 (round 1: one medium and four lows, all fixed; round 2: none)
- **Gate**: CANNOT CHECK — no full gate ran before this commit: it waited for #276 on main; the final gate follows it
- **Cost**: $8.05 (usage-report.sh, this session, at the fragment's commit)
- **Session start**: 7107972 tokens, 14 min

#### What happened

Three issues, each shown failing first. (1) #231: a waiting alert's Since is the moment this panel
first saw the wait (or, with stalls on, the carried ask), so after a restart it never matched the
notified mark, and while `claude agents` had not answered, or the wait was inside its threshold,
the alert was absent and the mark was deleted as cleared; a restart past `MinInterval` re-notified
it. The notifier now keeps a waiting mark it restored until an agents answer has bound a session
(`View.WaitsPending`; an answer before the worktrees are read binds none). The model names, by
session, the Since of a wait already under way when it first listed the session
(`View.WaitsCarried`). The notifier takes an alert with that Since as the stretch already
notified. A prompt a hook raises after the restart has its own Since and notifies (review round 1's
medium). (2) #162: `clauductor panel --launchd` runs as a
subprocess with its run stubbed, from a terminal and under `nohup`. It catches the wiring in
`panel.go` either way (always taking the hangup, never taking it); a test run that itself ignores
the hangup skips the terminal case. `docs/panel.md` says how the panel stops. (3) #160: the quota harness loads `panel.js`'s own `el`, `key`, `pct`, `bullet` and
`num`, and stubs only the clock's helpers. A changed `pct` and a changed `num` each fail the test.

#### Decisions

- A wait that ended and began again while the panel was down reads as the stretch notified before:
  the panel cannot tell the two apart, and one missed re-notice is the lesser fault than a repeated
  one on every restart.
- The rename of `lane_test.go`'s `gitIn` to `laneGitIn`, which `origin/main` needed for
  `internal/cmd`'s tests to build, stayed out of this row: the orchestrator filed it as #276 and
  landed it from its own fix branch.

#### Surprises

- `origin/main` at PANEL-57 did not build `internal/cmd`'s tests: PANEL-57 and OPS-135 each declared
  a `gitIn` (#276).
- Running `session-start/marker.sh` to read the session opened the row's draft PR (#278).

#### Insights

None.

### OPS-138 — merge-pr's step-5 guard reads every fragment, and a hook refuses a main-checkout move while catch-up is on

- **PR**: #283
- **Session**: S12, started 2026-10-25 20:43 ET
- **Review**: 3 rounds, peak medium, cap 3 (rounds 1 and 2 peaked medium, both fixed; round 3 peaked low: its test gap fixed with two rows, its four under-reads in the hook's KNOWN LIMITS under #280)
- **Gate**: CANNOT CHECK — no full gate ran before this commit: the branch was rebased twice onto a moving main, and this repo's `.claude/hooks/` and `.claude/skills/` copies waited on the owner each time; the final gate follows this commit
- **Cost**: $22.83 (usage-report.sh, this session, at the fragment's commit)
- **Session start**: CANNOT CHECK — the session started from the orchestrator's brief, not session-start, so no marker recorded its cost

#### What happened

Three of #249's items; OPS-134 (#266) closed the fourth.

- **Every merge-pr fragment is read.** `checks/change-process.sh` runs step 5's outside-step-5 rules
  over each fragment `extensions.sh fragments merge-pr` could emit, counting every module enabled.
  It also asserts the list includes each shipped one.
- **A runtime control, `hooks/catch-up-guard.sh`.** While `clauductor panel catch-up --project
  <main>` prints `on`, it refuses a git command that moves the main checkout: run there, aimed there,
  or re-aimed in a way it cannot follow. Refs are shared, so `update-ref` and `fetch -u` count from a
  lane too.
  - It reads commands with `lib/gate-reader.awk` in a new `-v gits=1` mode, which follows a `cd` the
    way the shell would: certain, possible (conditional, piped, backgrounded, in a case arm) or ended
    by a subshell's `)`.
  - A lane's own catch-up merge runs. Without clauductor, or with catch-up off, it allows the command
    and says so.
- **Two false refusals are gone:** prose `cd`, and a plain `cd <subdir>`.

The tests went in first: 39, 34, 25 and 2 hook rows over the first commit and three review rounds,
each red row failed against the hook before its fix.

#### Decisions

- **A runtime hook, not a longer denylist.** No static read of a skill sees Bash's working directory
  persist between calls, so the control has to read the command when it runs.
- **What it cannot follow counts as aimed at main.** A computed path, `eval`, `GIT_DIR=` or a cd to
  a directory that does not exist yet is refused while catch-up is on. That over-read is listed in
  the hook's header, and it costs nothing while catch-up is off.
- **Round 3's four lows merge under KNOWN LIMITS, owned by #280.** They take five forms, each a
  rare spelling: a bundled `-fu`, a git-core program by path, a case pattern inside `$( … )`, a
  lane's `checkout --ignore-other-worktrees main` followed by a reset (which moves the shared
  `refs/heads/main`), and a heredoc's `$( … )`. No medium is left (the owner's bar, OPS-139).

#### Surprises

- **The new hook caught its own author.** Once synced, catch-up-guard refused this session's
  `git -C $R` and a `worktree add` followed by `-C` to the new directory in one command. Both are
  listed over-reads, worked around with literal paths and two commands.
- **Main moved twice under the branch** (OPS-137 and OPS-132's #268). Each rebase ran in a scratch
  worktree and moved here by an index-only reset, because the sandbox cannot write this repo's
  `.claude/hooks/` or `.claude/skills/`. The owner restored those files by hand.
- **The first `clauductor update` read the wrong template**: origin/main's, through the installed
  binary. The update reads the template beside the binary, so a binary built from this checkout was
  needed.

#### Insights

2 row(s), moved to the insights log.

### PANEL-57 — clauductor lane close: an agent closes a finished lane, never a working one

- **PR**: #250
- **Session**: S12, started 2026-10-24 22:29 ET
- **Review**: 13 rounds, peak high, cap 3 (per group 7, 2, 1 and 3; the owner raised group 1's cap to 6, then allowed a narrow 7th on Fable; group 4's two lows at its cap are #265's)
- **Gate**: CANNOT CHECK — no full gate completed before this commit; the final gate follows it
- **Cost**: $133.67 (usage-report.sh, this session: the proposal and the build, at the fragment's commit)
- **Session start**: 65187792 tokens, 227 min

#### What happened

`clauductor lane close <id>` asks the running panel to close a lane through Close lane's own plan
and guards. With no flag it prints the plan and changes nothing; with `--yes` it closes, with the
plan's offers as consent. The panel serves it on `~/.clauductor/panel/cli.sock` (mode 0600, the one
route, named in `owner.json`), and an interrupted command leaves the close finished. One verdict,
`closeHeld`, decides whether a lane's work has ended: no gate lease held or waited for, then claude
exited or gone, or idle with its own session at rest (no subagent, workflow, shell, monitor,
teammate or wakeup in flight). Both the command and auto-close (PANEL-20) read it, and the close
re-reads tmux under the lane lock, so it decides nothing from a value read before the lock. The
command also needs the branch merged unless claude exited, and refuses the caller's own lane.
session-close's *Close the lanes* rule is its production caller, and the template allows
`Bash(clauductor lane close *)` and nothing wider. docs/panel.md lists every refusal word for word,
and `TestEveryCloseWordIsDocumented` holds that list to the reasons `closeheld.go` returns.

#### Decisions

- D1–D7 as recommended (the owner, 2026-10-24), with proposal review round 3's four findings and
  merge-pr's two as build tasks rather than design edits, so the approval covered design.md as
  reviewed.
- Group 1's cap: the owner raised it to 6 after round 3, with wakeups added to D2's background
  work, then allowed a narrow 7th round on Fable after round 6.
- Round 1 of group 4 asked the skill to report the result's Kept lines; the builder kept the plan's
  Keeps, which design.md D5 names word for word. Changing it would void the approval, so it stands
  as D5 words it (tasks.md's Decision log); a design amendment is the owner's if wanted.
- Group 4 stopped at its cap with two lows in the docs test's own coverage (derived words, every
  `CloseHeld` write): #265 owns them.

#### Surprises

- Each review round of group 1 found one more "unknown or earlier state read as current": a poll
  that straddles a `/clear`, a re-listed session that lost its last `Stop`, a lease source not yet
  read, a tmux value read before the lock. The fix that converged was one list of what the verdict
  will not judge without.
- `clauductor update` run with `!` has no stdin: it merged settings.json, then cancelled the skill's
  review. The owner copied the template's skill by hand (it matched byte for byte at HEAD).
- Three tests flaked under gate load and passed alone: #260 owns the two in claudecall,
  `TestGateStoppedCallHoldsTheSlotUntilItExits` and now `TestGateStopsATimedOutClaudeWithSIGTERM`;
  #234 owns the stall test that failed group 4's first quick gate.
- verify-change failed on a task naming `lane.go` bare; it matches a backticked path against the
  diff's full paths, so the task now names `framework/internal/cmd/lane.go`.
- The merge broke main. #250 merged 10 commits behind `origin/main`, on GitHub's CLEAN and a
  receipt for its own head; pr-merge-guard said so as an advisory, and rule 8 (journal and
  insights only) did not block. OPS-135 (#263) had landed a `gitIn` in `doctor_test.go` too, so
  `internal/cmd`'s tests stopped compiling on main (#276, fixed by #277). The branch's green
  full gate could not see it: it judged the branch's own tree.
- A builder replaced two lines of docs/panel.md with a scripted exact-match replace; the
  no-blind-source-rewrite hook did not block it. Both swaps were reviewed in round 2.

#### Insights

4 row(s), moved to the insights log.

### The close (2026-10-26)

Fifteen rows folded, and with PANEL-58's proposal (#259) sixteen landed: S12 ran three days, not
the "2–3 sessions" its cell estimated before #261 pulled eight more rows in. The plan row now says
what happened. On the evening of 2026-10-25, running low on tokens, Rich held PANEL-58's build for
S13 and moved the four OPS rows not yet started (OPS-133, OPS-139, OPS-123, OPS-86) there too; S13
now opens with those four, then PANEL-58's build (no budget, his decision) and PANEL-59. OPS-139
had been in no session's Rows since #279, which no check catches. S12's owner cell lost its two
void PANEL-58 items.

Merges past the guard's reach. #250 and #240 each declared `gitIn` in `internal/cmd`'s tests, each
gated green on its own base, and main stopped compiling at a6e9d6b3 (#276), until #277 renamed one.
A new file against an existing one overlaps in no file, so OPS-77's row now asks the owner to
widen its overlap to a Go package. OPS-131 merged after round 12's fixes with no 13th round, on
Rich's word: #284 owns the post-merge review of that delta (OPS-143). OPS-132's A/B choice went to
A (Rich, 2026-10-26).

The open issues no row owned got rows in S13: OPS-140 (the Go lows: #269, #281, #274), OPS-141 (the
guards': #280, #282), OPS-142 (#285, filed at the close, team-queue and bug: the harness's empty
`template/.claude/.cc-writes/` failing `gate/harness-dirs`, the second time) and OPS-143 (#284).

Two items went to the owner queue: each clone needs `--start-log Rich` before its first delegated
approval, since OPS-131's log fails closed (this Mac's has none yet); and whether to loosen the
sandbox, which made Rich sync, reset or rebase lanes over `.claude/hooks` and `.claude/skills` by
hand about six times.

### Compound

The fold brought the rows' insights to the top of the log, and none has waited a full session.
Logged at the close: `sandbox/lane-sync` (Raw: the decision is the owner's, in the queue),
`merge/skew-per-package` (→ owned by OPS-77), `gh/delete-branch-worktree` and
`agents/no-hand-back` (Raw), and `update/installed-binary` (an instance of CLAUDE.md's "with the
binary built from this checkout"). Decided: `gate/harness-dirs` → owned by OPS-142;
`merge/behind-is-untested` → owned by OPS-77; `sandbox/claude-dir-writes` superseded by
`sandbox/lane-sync`. Thirty-four rows stay Raw while young.

### Flow and cost

Flow 30d to 2026-10-26: cycle 1.4h median, lead 2.4h, 41.5 merges/wk, change-fail 5.3%, approval
wait 3.2h, review rounds 2, spend $1040.93/wk, 27 in flight (excludes unpriced models:
claude-haiku-5-5). The orchestrating session 10c3e1da (usage-report.sh, this machine, as the close
was written): ~$58.65, of it session $31.85, merge $13.66 and fix $7.37 (claude-opus-5-5), merge
$2.26 and reviewer-docs $0.46 (claude-sonnet-5-5), reviewer $0.64 and this close $2.42
(claude-opus-5-5). The rows' own costs are above, $438.37 in all.

The standing finding stays: seven changes sit proposed at once (ops-109-doctor's one unchecked
task, ops-42-routines, panel-25, panel-30, panel-31, panel-46, panel-58-plan-runner), against at
most one. panel-58 builds in S13; the others wait behind PANEL-46 and OPS-42's re-approval.

## Session 11 — 2026-10-24 — Rich — lanes start and close themselves; the gate's docs path; check and test lows

**Totals.** Gate 26 min (a row's gate was CANNOT CHECK, so this is a floor); cost $53.47.

**Rows folded.** OPS-103 (#224); OPS-121 (#228); OPS-122 (#229); OPS-125 (#222); OPS-87 (#226); OPS-88 (#227); OPS-89 (#225).

### OPS-103 — the quick gate runs without the gate lease

- **PR**: #224
- **Session**: S11, started 2026-10-24 09:44 ET
- **Review**: 1 round, peak low, cap 3 (four lows, all fixed after it, not re-reviewed)
- **Gate**: 7 min (the full run on 5dcd75391, after 18 min queued behind another lane's gate: red at `test (short)` on an empty, ignored `template/.claude/.cc-writes/` the harness left; removed, and the final gate follows this commit)
- **Cost**: $3.87 (usage-report.sh, this session, at the fragment's commit)
- **Session start**: 122443 tokens, 0 min

#### What happened

`run-local.sh --quick` now reads its flag from argv before the lease block and takes no lease, so
build-change's quick gate no longer waits out a full gate it cannot disturb. A full run still
queues. A quick run also clears a `CLAUDUCTOR_LOCK_HELD` it inherits, so `lease_verify` never fails
it on a lease it never took. `MODE` is set from argv before the lease, so a quick run that exits
early is still logged as quick. `gate-runs.log` rotates only on a full run, the one that holds the
lease. `checks/gate.sh` gains the row's two cases plus three others: a quick run passes beside a
live holder; a full run queues (exit 124 under `CLAUDUCTOR_LEASE_MAX_WAIT`); a quick run never
rotates the log, even one that exits before its flags are read; a quick run under a caller's lease
passes. Each was shown failing on the code before its fix. The fixture's first step now asserts
that a full run holds the lease and a quick run holds none. The README, both playbooks and both
AGENTS.md rows now say *full* gates never overlap.

#### Decisions

The row ships alone in #224, not batched into OPS-89's PR as the S11 plan said. That was the
owner's course correction, so this fragment carries no `**Also closes**` line and OPS-89's carries
none for it. The review's fourth low, that a quick run in the same worktree as a running full gate
now finishes during it and overwrites the state file, is documented in the README (one gate at a
time per worktree), not enforced: the state file and `gate.sh`'s log were already per worktree.

#### Surprises

`clauductor update`, run without a terminal, cancelled its file reviews and still reformatted
`.claude/model-roles.json`, adding an `eval.baseline` field. The edit was reverted and the four
synced files were copied by hand. A loop of `cp template/$f $f` was blocked by `pr-merge-guard`
as a merge in a form it cannot read, a false positive on `$f`. The first full gate went red on
`TestEveryTemplateClaudeDirIsPlaced`: Claude Code had made an empty `.claude/.cc-writes/` under
`template/` while the session's working directory was there. The directory is ignored, but the
test walks the disk, not git.

#### Insights

2 row(s), moved to the insights log.

### OPS-121 — the environment a signal in run.sh's exec window leaves is swept

- **PR**: #228
- **Session**: S11, started 2026-10-24 09:45 ET
- **Review**: 2 rounds, peak high, cap 3 (round 2's one low fixed after it, not re-reviewed)
- **Gate**: CANNOT CHECK — no full gate completed before this commit: two runs were stopped while queued for the lease, each when review changed the tree; the final gate follows this commit
- **Cost**: $3.95 (usage-report.sh, this session, at the fragment's commit)
- **Session start**: 122428 tokens, 0 min

#### What happened

A whole-set `checks/run.sh` now writes its pid to `run.pid` beside the no-clauductor environment
just before it `exec`s into it (the `exec` keeps the pid), and the inner run deletes it the moment
its traps exist. So a `run.pid` that survives belongs to a run that died inside the window, and
every whole-set run's start removes the scratch of such a run once `kill -0` says "no such
process". A new fixture in `checks/no-clauductor.sh` signals inside the window with a stub `sh`
that only the environment's PATH finds; it TERMs itself with the run's pid before any trap exists.
It was shown failing first (the environment stayed), and its decoys were shown failing against two
mutants: a run that keeps its pid past its traps, and a sweep that removes whatever `kill -0`
cannot signal.

#### Decisions

Bound what the window leaves rather than close it: a shell cannot close it, because a signal
ignored across `exec` cannot be trapped by the shell it starts. "Gone" is only "no such process":
"Operation not permitted" is a live process. The inner run deleting `run.pid` (review round 1's
high) makes the sweep safe across pid namespaces, which Claude Code's Linux sandbox gives every
command while TMPDIR stays shared.

#### Surprises

The first design trusted `kill -0` alone. Review found that on Linux, in the sandbox, a live run's
pid can read as gone from another lane's pid namespace, so the sweep would have removed a running
check's HOME. On macOS `grep -v 'rm -f "$_scratch/…"'` silently matched nothing, so the first
mutant passed for the wrong reason; `grep -F` fixed it.

#### Insights

1 row(s), moved to the insights log.

### OPS-122 — the tests that failed in concurrent gates hold under load

- **PR**: #229
- **Session**: S11, started 2026-10-24 09:45 ET
- **Review**: 3 rounds, peak high, cap 3 (round 3's one low fixed after it, not re-reviewed)
- **Gate**: CANNOT CHECK — no full gate ran before this commit; the final gate's receipt covers it
- **Cost**: $10.29 (usage-report.sh, this session, at the fragment's commit)
- **Session start**: 122559 tokens, 0 min

#### What happened

The template lane's double first prompt was a real panel bug, not a slow test. A lane's alert
names its tmux terminal only once tmux reports it running, so after a restart on a loaded machine
the alert was grouped by worktree path, missed the terminal's `lastSent`, and notified again. The
notifier now keeps the MinInterval window under every name of the lane (`namesOf`), and
`TestNotifierIntervalHoldsWhileTheLaneIsUnbound` failed before the fix. `pushStandIn` writes its
record through `mv`. The lease daemon test's bound went from 4 s against a 60 s daemon to 30 s
against a 600 s one, and the conformance driver gives a waiter 5 s to show its fault (the tests
take 24 s, up from 15). A sweep of `framework/` found the #177 shape in four more claudecall
tests: a fixed timeout raced the fake's `trap`. Their fakes now write "ready" once armed, and an
`expiring` context fires the caller's timeout then, so the tests still assert
`DeadlineExceeded`. Closes #92 and #177.

#### Decisions

The row's "Done when" (five concurrent full-gate pairs agreeing with three singles) was not run
here: it takes about ten gate runtimes with the lease bypassed. #233 owns it, and it gates OPS-105's
follow-on (Q28). A restart past MinInterval still re-notifies a waiting alert, because
`WaitingSince` is not persisted without stalls; #231 owns that. Bounds that mark a failure stay
unscaled where scaling would reach the failure (the hook's 6 s against curl's `-m 3`).

#### Surprises

Review round 1 caught the new "ready" mark written before `P=$!`: cancelling on it aimed the
signal into the one gap where the trap had no pid, a new flake of the kind the row removes (2 in
40 under the sandbox). Round 2 caught `exec sleep 60`, which had silenced a leak but also removed
the child the push test exists to create; with `WaitDelay` deleted that test now fails at 60 s
again. Raising the conformance TIMEOUT to 60 s cost 51 s, because a control case runs it out, so
it stayed at the driver's default, 20 s.

#### Insights

2 row(s), moved to the insights log.

### OPS-125 — a row's lane starts and closes itself; the panel offers the daily close

- **PR**: #222
- **Session**: S11, started 2026-10-24 05:25 ET
- **Review**: 3 rounds, peak medium, cap 3 (round 3's three findings fixed after it, not re-reviewed; one low left to #223)
- **Gate**: 19 min (the full run on 5bc5579ab, before review; the final gate follows this commit)
- **Cost**: $14.43 (usage-report.sh, this session, at the fragment's commit)
- **Session start**: 575652 tokens, 0 min

#### What happened

The panel's ops and fix templates now open with session-start and the row marker, and end with
the row's fragment, `run.sh journal`, the gate and `/merge-pr`. The build template still starts
with `/build-change`, and build-change's description carries the same steps. `marker.sh` with no
arguments finds the row this branch belongs to and its session in the plan. A fragment's optional
`**Also closes**` field lets the fold mark a batched PR's other rows merged. A new `close` template,
fed by `panel-suggest.sh close`, offers `session-<N>-close` once every row of the plan's open
session has landed. `checks/panel-contract.sh`, `panel-suggest.sh`, `fold.sh` and `marker.sh`
cover each of these and fail without the change. `init.go` ships all five templates, and its test
now compares every first prompt.

#### Decisions

The build lane's steps live in the workflow's `whenToUse`, not its prompt, because the panel's
approval warning and NEWLANE-4-S4 need a build prompt to start with `/build-change`. The close
template is an ops lane on `ops/{name}`, so `session-<N>-close` matches `BRANCH_SESSION_CLOSE`.

#### Surprises

`jq` rewriting `panel.json` reflowed the entire file, so the edits were made by hand instead.
`init.go`'s fix prompt had already drifted from the shipped config (it was missing the fast-path
sentence). The old test compared only the ops prompt, so nothing caught it. The first full gate
went red on `checks/roadmap.sh`: the marker reads the Session plan by `$ROOT/$ROADMAP`, which the
bypass guard flags, so the marker is now allowed in the guard's own case list, with its reason.
Round 2 found the close lane named by the plan's order (S4.5 as `session-4-close`). Its fix, one
past the journal's top for every close, broke OPS-108's rule, which fold.sh and session-close
state (N is the plan session the close covers), and round 3 caught it. A rule has several
readers: the fold, the skill and the hook's comment, as well as the code that changed.

#### Insights

1 row(s), moved to the insights log.

### OPS-87 — settings.sh reads an empty GATE as build-change does

- **PR**: #226
- **Session**: S11, started 2026-10-24 09:45 ET
- **Review**: 2 rounds, peak medium, cap 3 (round 2: one low, the row text, left here)
- **Gate**: 0 min (no full run before this commit; the final gate follows it)
- **Cost**: $2.01 (usage-report.sh, this session, at the fragment's commit)
- **Session start**: 122283 tokens, 0 min

#### What happened

With `GATE=""` in `project.conf`, `checks/settings.sh` reported `lacks ''`, but build-change runs
`scripts/ci/gate.sh` then (`c.gate || 'scripts/ci/gate.sh'` over `gate_cmd`'s output). The check
now falls back the same way, only on empty. Three self-tests hold it: an empty GATE passes on the
template's settings (it failed before the fix), an empty GATE without the default exclusion fails,
and an absolute GATE with only the default exclusion still fails. `selftest` now passes an
explicit empty fifth argument through (`${5-$GATE}`).

#### Decisions

The fallback is empty-only. Round 1 widened it to absolute, `~` and `../` paths, mirroring Go's
`cleanRel`; review showed build-change runs those as given, so the wider fallback turned a correct
FAIL into a silent pass.

#### Surprises

The row's premise was half wrong: `install` and `update` do not fall back on `GATE=""`, they
refuse it (`Conf.Mapping`: "not a path inside the repository"). Only build-change falls back, so the
row's text in `docs/roadmap.md` overstates install. Whether install should accept an empty GATE as
"not moved" is the owner's call; nothing in this row changes it.

#### Insights

1 row(s), moved to the insights log.

### OPS-88 — a plugin check's child shell gets the plugin root; the window cases check exit and posts

- **PR**: #227
- **Session**: S11, started 2026-10-24 09:45 ET
- **Review**: 3 rounds, peak medium, cap 3 (round 3 low: its comment finding fixed after it, not re-reviewed; two detector lows left to #232)
- **Gate**: CANNOT CHECK — the only full gate runs after this commit, on the tree that holds this fragment
- **Cost**: $5.19 (usage-report.sh, this session, at the fragment's commit)
- **Session start**: 122384 tokens, 0 min

#### What happened

A plugin script sets `CLAUDUCTOR_FW` in its prologue without exporting it. A `sh -c '…'` child
whose body names the variable therefore read it as empty, so `plugin/checks/statusline.sh` failed
its two unwritable-HOME cases when run directly. Through `checks/run.sh`, which exports the
variable, those cases passed. The plugin builder (`rewrite.go`, `shChildFW`) now puts
`CLAUDUCTOR_FW="$CLAUDUCTOR_FW"` among such a child's assignments. Each child is judged on its own,
and a value the template sets on that command is kept. `TestShellChildGetsPluginRoot` walks the
built plugin with a detector written separately from the rewrite. It fails without the rewrite,
and it found a third instance the row did not name: `checks/health-lines.sh`'s scheduled-list
case. In `checks/statusline.sh`, `window()` now requires exit 0 and exactly one post before it
compares the line, and a status line that exits 3 now fails every window case.

#### Decisions

The fix is in the builder, not the template line, and the export stays off. `health-lines.sh`
asserts the prologue does not export the variable, and a check's children point it at fixtures on
purpose. Passing the variable per child keeps both of those true. This repo's copy of the check
was synced by copying the file. `clauductor update` was not used because it also rewrote
`.claude/model-roles.json`, which is out of this row's scope.

#### Surprises

Round 1's medium came from the rewrite's first form, which appended its own assignment after a
deliberate `CLAUDUCTOR_FW=` and so overrode it. Round 2's medium was that this repo's own copy of
the check was never synced after the template edit. A gate on such a tree runs every shared check
twice, because `framework_matches_template` fails.

#### Insights

1 row(s), moved to the insights log.

### OPS-89 — a docs gate, measured timeouts, API-key-free tests

- **PR**: #225
- **Session**: S11, started 2026-10-24 09:44 ET
- **Review**: 3 rounds, peak medium, cap 3 (round 3's medium was OPS-103's absence, which the owner moved to #224; two of its three lows fixed after it, not re-reviewed)
- **Gate**: CANNOT CHECK — no full gate ran in this lane before the fragment's commit; the final gate's receipt covers the head after it
- **Cost**: $13.73 (usage-report.sh, this session, at the fragment's commit)
- **Session start**: 122282 tokens, 0 min

#### What happened

`run-local.sh --docs` gates a PR whose diff is prose. It runs the model's steps and the project's
`gate_steps docs`, and writes a receipt whose stage is `docs`. The merge guard accepts that receipt
only after it fetches origin/main and finds the PR's own diff is prose (`.claude/lib/docs-gate.sh`).
Prose is `*.md` outside `.claude/`, `agents/`, `commands/` and the specs, and not AGENTS.md,
CLAUDE.md, CLAUDE.local.md or SKILL.md, at any depth and in any case.

The docs gate is opt-in through `GATE_DOCS`. This repo turns it on, and `steps.sh`'s `docs_scope`
refuses the prose that Go tests read. The build-change gate timeouts are now set from
`gate-runs.log`: quick 20 min, full 45 min. The short Go suite has `-timeout 5m`. The panel tests
unset every API-key variable in TestMain through `lanes.APIKeyVars()` (#184).

#### Decisions

- OPS-103, the quick gate skipping the lease, is not batched here, though the roadmap rows and S11's plan said it would be. The owner gave it its own lane and PR, #224, with its own fragment. Whichever of the two merges second rebases and runs the full gate again.
- The docs gate takes the lease like a full gate does, so it stays out of OPS-103's area.
- The docs gate is opt-in (`GATE_DOCS`). A project whose tests read prose has to refuse that prose in its own `gate_steps docs` before a docs receipt means anything.
- Both run-local and the guard fetch origin/main before they list the diff. Against a stale ref, a branch that reverts main's newer code lists only its prose.
- #184's fix lives in each package's TestMain, through `lanes.APIKeyVars()`, rather than in `steps.sh`. That way it also holds for a bare `go test`.
- The timeouts come from the log: the longest quick gate ran 722 s and the longest full gate 2024 s.

#### Surprises

- A variable prefix on a function call (`VAR=x f`) leaves VAR set after the call in some sh implementations. The tests unset it explicitly.
- macOS's case-insensitive file system writes `.Claude/x.md` into `.claude/`, so the verdict folds case before it matches.
- A rename read as only its destination: `git mv x.sh x.md` looked like prose until `--no-renames`.
- The plugin build rewrites a literal framework skill path inside a check, so the fixture builds the path in pieces.

#### Insights

1 row(s), moved to the insights log.

### The close (2026-10-24)

All seven planned rows had merged, each with its fragment on main, and the fold took them on its
first dry run. OPS-103 landed in its own PR (#224), not batched into OPS-89's as the plan wrote,
because the panel offered it a lane of its own; OPS-126 (5) owns teaching the suggester the plan's
batches. Two other PRs landed in S11 beside the rows: OPS-126's roadmap row (#230), and OPS-109's
proposal (#236), approved by the owner the same day, so S12 starts with no owner decision waiting.

The close lane itself was the first one started from OPS-125's `close` template, and it failed at
the first token: `Unknown command: /session-close:`. The prompt glued a colon onto the command,
`checks/panel-contract.sh`'s `*/session-close*` glob passed it, and the same colon kept
build-plugin from namespacing it, so plugin users had it too. At the owner's request the fix went
first, as its own PR before the close: #237 (`OPS-125:`, no fragment, since OPS-125's was already
on main and a second would stop the fold). It fixes four copies (the template, this repo's
`panel.json`, `init.go`, `plugin/`) and adds a leading-command case shown failing on the old
prompt. Review converged in 1 round at peak low; its two lows (no both-ways fixture, a regex that
refuses a one-letter command) went to OPS-126 as item (6) and to #223. Gate 1390 s, full, clean.
Catch-up is on, so the panel re-trusts the merged config with nobody's hands.

`clauductor update`, run to sync this repo's copy of the check, again reflowed
`.claude/model-roles.json` and re-added `eval.baseline` with no terminal to ask: OPS-119's
behaviour exactly, a day before S12 builds it. Reverted by hand, and the check copied from the
template instead.

The plan: S11 marked done; S12 is OPS-126, OPS-109's build and OPS-119, as the owner confirmed;
OPS-123 and OPS-124, queued but in no session (so Up next could never offer them), go to S13.

### Compound

Ten Raw rows before the close, nine of them today's. Decided: `archive/two-lanes` (two sessions
waited) → owned by #217, whose scope now also names which session archives a lane-built change;
`update/non-interactive` → owned by OPS-119. Logged and shipped: `check/mention-not-parse`
(#237). The other eight stay Raw while young.

### Flow and cost

Flow 30d to 2026-10-24: cycle 1.3h median, lead 2.4h, 34.8 merges/wk, change-fail 4.9%, approval
wait 3.4h, review rounds 2.2, spend $897.49/wk, 26 in flight (excludes unpriced models:
claude-haiku-5-5). The close session (usage-report.sh, this session): ~$3.75, session ~$3.13 and
reviewer ~$0.62, all claude-opus-5-5. The rows' own costs are above, $53.47 in all.

A standing finding the merge guard repeats: six changes sit proposed at once (ops-109-doctor,
ops-42-routines, panel-25, panel-30, panel-31, panel-46), against at most one. ops-109 builds in
S12; the rest are older approvals queued after PANEL-46 and OPS-42's re-approval (the owner queue).

## Session 10 — 2026-10-24 — Rich — one row, one session: records fold at a daily close

**Totals.** Gate 30 min; cost $22.51.

**Rows folded.** OPS-108 (#218).

### OPS-108 — one row, one session, one PR; a daily close folds the records

- **PR**: #218
- **Session**: S10, started 2026-10-23 18:50 ET
- **Review**: 3 rounds, peak high, cap 3 (round 3's six findings fixed after it, not re-reviewed)
- **Gate**: 30 min (the full run before this commit: red on OPS-122's three load flakes and one plugin-mode regex of this row's, fixed here)
- **Cost**: $22.51 (usage-report.sh, this session, at the fragment's commit)
- **Session start**: CANNOT CHECK — marker.sh was built in this row, after the session started

#### What happened

A row's PR now writes its records to one fragment, `docs/journal.d/<row id>.md`, and no longer
edits the development journal, the insights log or any row's Status. `session-close/fold.sh`
folds the fragments on main into one `## Session N` entry on the daily close (all or nothing),
moves their insight rows to the log, sets each row's Status and deletes the fragments.
`pr-merge-guard` rule 7 now refuses any non-close PR that adds a session heading, and rule 8
holds every PR that changes the journal or the insights log to the close's branch.
`checks/journal.sh` holds the fragment shape (`lib/fragments.sh`); `checks/fold.sh` drives the
fold over a fixture project. `review-cap.sh` turns a row's risk into its review cap (low 2,
normal 3, high 3, `REVIEW_CAPS`), read by build-change.js (maxRounds' default) and merge-pr
step 3; past the cap, what is left goes to KNOWN LIMITS. `session-start/marker.sh` opens a row's
draft PR with the `session-start: S<n> <time> row <id>` marker.

#### Decisions

- Rules 7 and 8 change as the row states: the owner's, 2026-10-21.
- The cap is parsed in build-change.js from review-cap.sh's own words, never taken from an
  agent's number, and a refusal stops the run at preflight: a guessed cap would let review end
  early or run on.
- The fold writes nothing until every fragment validates and the folded roadmap re-parses, so a
  malformed fragment can never leave half a session behind.

#### Surprises

- The branch-prefixes check caught literal `change/` and `ops/` prefixes in two new comments;
  the build-change check's faked preflights had no `reviewCap`, so every faked run stopped at
  preflight until they did.
- This row's own records could not use its marker: `marker.sh` did not exist when the session
  started. Running it on this row 70 minutes in found its first bug: with no PR open, `gh`'s
  `--jq '.[0] | …'` met `null` and refused, so the marker only ever warned. `checks/marker.sh`
  now drives it through a stand-in `gh` that applies each `--jq` with real jq.
- Review round 1 found the fold would have written the new entry inside the journal's own
  fenced format example (its first `## Session` line), `--focus` with nothing after it looped
  forever on `shift 2`, and review-cap.sh read a `Risk:` from any cell of the row.
- Review round 2 found the rule's other writers: log-insight, apply-change, merge-pr,
  archive-change and new-adr still told a row's PR to edit the insights log or a Status. Each now
  routes to the fragment or the close; log-insight computes a **Writer** line from the branch, as
  dev-journal already did.
- Review round 3 found the draft `marker.sh` opens was never readied, so every row's squash would
  be refused; merge-pr now runs `gh pr ready` on a draft. It also found the validator accepted
  prose under `## Insights` that the fold then dropped, and that Cost and Session start had no
  format check.
- The first full gate failed on this row's own build-change check in the plugin: the plugin's
  workflow says `clauductor-model review-cap.sh`, not `sh .claude/review-cap.sh`, and the test's
  regex named the latter. The template's run of the same check could not see it.
- The roadmap parser refuses a `|` inside backticks and a Deps cell that is not row ids, so the
  only cell review-cap.sh's fixture can prove it ignores is the Scope.

#### Insights

None.

### The close (2026-10-24)

S10's lane ended at #218's merge with no close, so the roadmap still read OPS-108 queued and the
session plan still offered S10 next. This close is the first daily close under OPS-108's rule: a
separate session the next morning, which ran the fold above (one fragment, all of it valid on the
first dry run), marked S10 done in the plan, and refreshed the tracker. Lesson for the panel's
lanes: a row's session ends at its merge, and nothing starts the daily close; until something
does, the owner asks for it (the `session-close` skill) once a working day.

Catch-up (OPS-118) did its job on #218 with nobody's hands: the panel log fast-forwarded the main
checkout `6422aab5..6de033cf`, rebuilt the panel and `~/.local/bin/clauductor` because
`framework/` changed, and restarted into the new build within a minute of the merge. That is
outcome check o.9's first signal, and the owner-queue item to turn catch-up on is ticked.

The sandbox refused `git switch -c` (it cannot write `.git/config`) and a worktree checkout of
`template/.claude/agents/`, so the close's worktree was created outside it.

### Compound

One Raw row, `archive/two-lanes` (2026-10-23, 0 sessions waited), left Raw while young: the fix
is a rule naming which session archives a lane-built change, and it wants a row when one of the
next closes decides it.

### Flow and cost

Flow 30d to 2026-10-24: cycle 1.1h median, lead 2.5h, 32.2 merges/wk, change-fail 5.3%, approval
wait 4h, review rounds 2.2, spend $877.95/wk, 25 in flight (excludes unpriced models:
claude-haiku-5-5). The close session itself: about $0.45 at its start (usage-report.sh, session,
claude-opus-5-5); the row's own cost is above, $22.51.

## Session 9 — 2026-10-23 — Rich — main catches up after a merge, and a cloud session builds a row

**Rows**: OPS-118, OPS-100 · **Span**: 2026-10-22 to 2026-10-23

### What happened

Started as the panel's propose lane `ops-118-main-catches-up`; the build ran in a second lane, `ops-118-build`, which reported back here:
- **OPS-100 merged as #209** at 8:31 PM ET on 10-22 (squash `44097c12`). A claude.ai cloud session on the owner's promotional credit built it from its row and #178 and opened the PR; the Mac gated and reviewed it. A `new-row` team-queue issue now proposes a roadmap row through `--next-id`. The `new-row` label exists.
- **OPS-118's proposal merged as #210** at 9:26 PM ET on 10-22 (squash `16075e31`): seven decisions, all approved as recommended, reviewed to convergence in 5 rounds before the owner read it.
- **OPS-118's build merged as #213** at 2:22 PM ET on 10-23 (squash `b5bce1cf`), in four groups. With catch-up on, the panel fast-forwards the main checkout after a merge, trusts a changed `panel.json` that the owner's own merged PRs brought, and rebuilds and restarts itself when `framework/` changed. merge-pr's step 5 asks the panel instead of pulling. Review converged at `bbacccf9`; `Refs #164`, which stays open for its second item.
- **Archived by the build lane as #215** at 2:47 PM ET on 10-23 (squash `a6e47df2`): the new living capability `specs/panel-catch-up/` (39 scenarios) and `specs/panel-stall/` with STALL-3's notices (70), the cost $200.24 of a soft $175, and the outcome check **o.9**.
- **PANEL-56 (#212)**, a row for #211, merged from another session.

### Decisions the owner made
- **Build OPS-118 in S9, in its own lane**, and hold the close until it merged.
- **OPS-118's budget went to $175, then soft** ("I want to see ops-118 get completed"). It spent **$200.24** by the build lane's reading: the overrun was warned, not stopped.
- **Keep reviewing until no medium remains**, said to the build lane directly.
- **OPS-100's lane rule** stays `^[A-Z]{2,8}$`; #178's `X` example was a placeholder.

### What went wrong
- **The first gate of #210 ran inside the sandbox** and failed seven checks on `ps` and xcrun, as session 7 logged; the owner's "run it yourself outside the sandbox" let the rerun pass.
- **A refused `branch -D` in the sandbox left a `.git/config` section** that would have made a reused branch name track `main` (insights, *git/branch-config*).
- **`change-cost.sh` reads $0 for OPS-118** once both lanes moved branches; the archive records build-change's $200.24 instead (#214, OPS-124).
- **Two sessions archived the same change.** The build lane opened #215 to archive OPS-118, and filed its lows as #214, while this close did both. The close's review caught the duplicate. This session closed #215 as superseded, but the build lane, unaware, merged it 12 minutes later (its timeline shows the close at 18:35Z and the merge at 18:47Z, with no reopen between), so the close was rebuilt on top of it with its own archive dropped (#215's branch is bundled at `.git/clauductor/branch-archive-2026-10-23-s9-ops118-archive.bundle`, 4ec93442). OPS-124 takes #214's four lows as its source. Who archives a change built in another lane was never said; the build-change report says "on success, push and run merge-pr", and the lane went on to archive too.

### What surprised us
- **Review found one class at a time.** Group 3 closed the cancelled context one call site per round until the fix named the class; group 4 did the same with the step-5 guard (insights, *review/class-convergence*, owned by OPS-110).
- **A build lane cannot reuse a running propose lane's name**: id, template and folder are one string.

### Compound
- **Logged today:** six rows. *gate/worktree-edit* is an instance of AGENTS.md's clean-tree rule; *git/branch-config* and *panel/lane-ids* are Technique; *review/class-convergence* is owned by OPS-110 and *cost/lane-attribution* by OPS-124. *archive/two-lanes* stays Raw for the next close to decide: it wants an owner (who archives a lane's build), and none exists yet.

### Left in flight (handed off)
- **The owner's step:** `./install.sh`, `clauductor panel install`, then `clauductor panel catch-up --project … on` (owner queue); o.9's 7 days start then.
- **New rows:** OPS-123 (OPS-100's four lows) and OPS-124 (OPS-118's four lows, issue #214, which its PR closes).
- **`change/ops-42-build` and `change/panel-31-build`** (2026-10-13) stay for their rows' builds to resume.
- **Team queue:** #178 → OPS-100 merged (#209); #178 is closed.
- **Q17**, OPS-81's flake watch, was due by S9 and still waits for the owner (owner queue).

### Flow and cost
- **Flow (30 days, to 10-23):** cycle 1.1h median, lead 2.5h, 31.5 merges a week, change-fail 5.4%, approval wait 4h, review rounds 2.4, spend $869.36 a week, 26 in flight (Haiku 5.5 unpriced).
- **This session's cost** (usage-report.sh, list price, this machine, before the close's gate; session `300dd5aa`): about **$45.11**:
  - the session, Opus 5.5: $29.08;
  - reviewer, Opus 5.5: $12.22 (OPS-100's rounds and OPS-118's proposal rounds);
  - gates and merge agents, Opus 5.5: $3.85.
- **The build lane** (`ops-118-build`, its own session) spent about **$200.24** by its own reading, outside this table.

## Session 8 — 2026-10-22 — Rich — the panel runs the plan, and change-cost counts nested agents

**Rows**: OPS-111, OPS-120 · **Span**: 2026-10-22

### What happened

Started from the panel's **ops** template as the lane `ops-111-panel-runs-the-plan`, with the task typed in (the owner queue's step 9):
- **OPS-111 merged as #205** at 4:58 PM ET (squash `754a8cdb`). Up next now follows the session plan as `origin/main` has it, through `.claude/roadmap-plan.sh`. The propose template offers the first planned change row with no proposal, and reads waiting proposals from change branches too. The ops template builds the named row through merge. `.clauductor/panel.json` sets `lanes_auto_close: "on_merge"`. Review converged in 4 rounds at peak medium; round 4's three lows went into OPS-120's PR.
- **OPS-120 merged as #206** at 6:07 PM ET (squash `f0768617`). `change-cost.sh` walks each session's `subagents/` tree by `parentAgentId` to a fixpoint, so an agent's own agents count with it. It reads each transcript and meta on its own, so a cut-off line or a vanished file drops nothing after it. OPS-93 now reads **$85.30**, against $64.88 before (and $22.13 when its build checked). Propose's context reads its roadmap, specs and scenario IDs from `origin/main`. Review converged in 4 rounds at peak medium (rounds 1 and 2 medium); the full gate passed on `7892787c`.

### Decisions the owner made
- **OPS-122 goes last in S11**, before any parallel gate slot is built.
- **OPS-100 runs as a claude.ai cloud session beside S9**, on the owner's promotional Claude Code credit ($152 left, expiring 2026-11-05). It lands before S10's OPS-108, and the owner merges it (#178). The owner starts the session (owner queue).

### What went wrong
- **`gh pr merge --delete-branch` failed again after merging** #206 ("'main' is already used by worktree"), as it did for #203. The remote branch was deleted by hand. A known shape, not yet owned: it costs one command.

### What surprised us
- **A nested agent's transcript names the lead session's cwd and branch**, not its parent's. So the old filter missed every builder and reviewer a teammate lane spawned, and the only link is `parentAgentId` in the meta file.
- **`jq -R` over several files is one stream.** A cut-off last line joins the next file's first and both are dropped. `lib/usage.sh` has the same shape: filed as **#207** (team-queue, bug).

### Compound
- **Logged:** *nested agents carry the lead's cwd* (Fixed → #206) and *`jq -R` over files joins a cut-off line* (Fixed → #206; `lib/usage.sh` owned by #207). *change-cost misses nested agents mid-build* is now Fixed → #206.
- **Decided:** *a lane's gate fails late in the sandbox* and *archive finds the spec gaps*, both Not promoted (one occurrence each; reasons in their rows).

### Left in flight (handed off)
- **OPS-100** moves to S9, as a cloud session the owner starts (owner queue).
- **#206's lows, left as stated in its PR:** one 145-column comment line; transcript lists split on blanks (older than the PR); the cut-off-meta test catches its mutant only in some directory orders; the last-tab split mutant survives (no fixture has a tab in a path).
- **`change/ops-42-build` and `change/panel-31-build`** (2026-10-13) stay for their rows' builds to resume, as in session 7.
- **Team queue:** no item was worked; #207 was filed.
- **The owner's step after #205:** pull the main checkout, then `clauductor panel trust` (owner queue).

### Flow and cost
- **Flow (30 days, to 10-22):** cycle 1.2h median, lead 2.5h, 30.3 merges a week, change-fail 5.6%, approval wait 4.6h, review rounds 2.4, spend $821.2 a week, 25 in flight (Haiku 5.5 unpriced).
- **This session's cost** (usage-report.sh, list price, this machine, before the close's gate; session `2651dcad`): about **$27.58**:
  - the session, Opus 5.5: $17.85;
  - reviewer, Opus 5.5: $9.73 (8 rounds over two PRs, peak medium in each).

## Session 7 — 2026-10-22 — Rich — the parallel-gate spike: a pair costs little, but its tests' waits break

**Rows**: OPS-105 · **Span**: 2026-10-22

### What happened

The first row run as one row, one session, and the first started from the panel, as a lane (`ops-105-parallel-gate-spike`):
- **The spike ran from a plain Terminal, 8:38 to 11:00 AM ET**, by the owner queue's steps. It used two clones at `f9a5f396` in `~/spike`, three single gates, then five pairs, with a memory sampler every 5 s.
- **The lane wrote it up** in `docs/plans/parallel-gate-spike.md` and judged it against G1–G5. It carried the S6 leftovers from the owner's notes into the records and reordered the session plan on the owner's word. **#203** merged at 12:18 PM (squash `eff00f1`).

**What the spike found:**
- **Singles:** all three passed, at 1027, 1018 and 1011 s.
- **Pairs:** 7 of 10 runs failed. Every failure was a test's own fixed wait or race:
  - `TestStallPushUntrustedTitle`, which reads its stand-in's file between the `>` that creates it and the write;
  - the lease conformance kit, with `CONFORMANCE_WAIT=2` unscaled;
  - `TestDaemonLeftByTheGateDoesNotHoldTheLease/shell`, with its 4 s bound;
  - `TestShellLeaseInteroperatesWithLockRun`;
  - a lane-prompt test.
- **The machine held:**
  - a pair ran 1.11–1.12× a single, which is 1.8× the throughput;
  - peak memory was 11.0 GB of a 12 GB limit;
  - swap never rose above its start;
  - memory pressure never reached critical;
  - the slowest race step took 471 s, well inside the 25-minute timeout.
- **Verdicts:** **G2 fails.** G1 passes on the two valid pairs. G3 to G5 pass.
- **What follows:** the tests are the work, not the machine. That work is **OPS-122** (closes #92 and #177), a new row not yet slotted in the plan.

### Decisions the owner made
- **OPS-111 to S8 and OPS-118 to S9, ahead of OPS-108**, now S10 (2026-10-22). The lane moved OPS-120 into S8 with OPS-111, so it still lands before OPS-118, the next hard-budget change. That move is the lane's judgment, open to the owner.
- **Delete `~/spike` and the two leftover remote branches** (`fix/92-daemon-lease-flake`, `ops/ops-96-s6-tidy`). The lane did both after keeping the spike's logs (`.git/clauductor/spike-logs-2026-10-22.tgz`, no clones) and bundling the branches (`.git/clauductor/branches-2026-10-22-s7.bundle`, verified).

### What went wrong
- **The first gate ran inside the Bash sandbox.** It went red after about 16 minutes on seven process checks: git's `xcrun` could not write its cache, and `ps` would not run. The same commit passed in full outside the sandbox. Logged as an insight, with a candidate early probe.
- **`gh pr merge --delete-branch` merged #203 and then failed** checking out `main`, which the main checkout holds. The remote branch was left, and the lane deleted it by hand.

### What surprised us
- **Running a second gate cost almost nothing in machine terms.** The case against parallel gates was the tests, not RAM or CPU.
- **`clauductor panel install` failed with launchctl's error 5 and no explanation**, because launchd held the label disabled. The workaround is `launchctl enable gui/$(id -u)/com.clauductor.panel`. Filed as **#202**.

### Compound
- **Two rows logged today:** *a lane's gate fails late in the sandbox* (Raw, with a candidate mechanism: the gate probes `ps` and a git write before its first step) and *a pair breaks the tests' waits, not the machine* (deferred to OPS-122).
- **PANEL-51's *archive finds the spec gaps*** stays Raw. It was logged today and is still young.

### Left in flight (handed off)
- **OPS-122** has no session in the plan. The owner slots it; Q28 (B or C, with #168) is decided at S10 on the spike's data.
- **#203's review converged in 1 round at peak low.** Its four lows are left as they are: the "Decide by" dates are the owner's own, and the rest is history wording.
- **`change/ops-42-build` and `change/panel-31-build`** (2026-10-13) are the started builds of the queued OPS-42 and PANEL-31. Each stays for its row's build to resume.
- **Team queue:** no item was worked.

### Flow and cost
- **Flow (30 days, to 10-22):** cycle 1.3h median, lead 2.6h, 29.6 merges a week, change-fail 5.8%, approval wait 4.6h, review rounds 2.4, spend $814.37 a week, 25 in flight (Haiku 5.5 unpriced).
- **This session's cost** (usage-report.sh, list price, this machine, before the close's gate; session `528323a1`): about **$5.68**:
  - the lane, Opus 5.5: $5.43;
  - reviewer-docs, Sonnet 5.5: $0.25.

## Session 6 — 2026-10-19 — Rich — no CI, headless lanes on API credit, PANEL-51 rebuilt around its seams, the gate cut to 17½ minutes

**Rows**: OPS-96, OPS-53, OPS-101, OPS-65, OPS-60, PANEL-51, OPS-82, OPS-97, OPS-93, OPS-89 · **Span**: 2026-10-19 to 2026-10-22

### What happened

The session opened on a cleanup at the owner's request. Before it, 17 lane worktrees were left from session 5, nearly all emptied by a sandbox-refused checkout, and seven stale remote branches. All of it was bundled first (`.git/clauductor/branch-archive-2026-10-19.bundle`) and then removed by the owner. Three things then shaped the rest of the session:
- **GitHub Actions ran out of minutes** until about 11-01, so every PR check failed on billing. The owner turned CI off for the month: `GATE_PR_CHECKS=""` and both workflows disabled (OPS-96 #166, with **OPS-98** to restore them). A merge has rested on the local gate receipt alone since.
- **The owner's plan was at 85 to 90% of its week,** so most work ran as **headless `claude -p` lanes on promotional API credit**, plus three claude.ai cloud sessions. One session orchestrated them all: it wrote each lane's prompt into `~/s6-lanes/`, and the owner started each one in a Terminal window.
- **Interactive Claude Code does not draw on promotional credit; `claude -p` and the Agent SDK do.** The first PANEL-51 lane, interactive on the API key, died at a $0 purchased balance. The credit ran out at 00:20 on 10-21, by which time PANEL-51 had groups 1 to 3 committed.

**Merged, in order (times ET):**
- **OPS-96** (#166, 10-19 12:29 PM): session 5's leftovers, PRAC-0010 accepted (OPS-97 queued as its enforcement), the CI switch.
- **OPS-53** (#169, 3:45 PM): a zombie read as a live gate holder, in `lease.sh` and Go's `LiveProc`, and the test's sync point. Built by a cloud lane. Under a slow reaper, 100 of 100 runs failed before and 0 of 100 after. The production-side window is #168.
- **The S6-start roadmap edit** (#175, 5:57 PM), from the owner's bot team, checked against `main` here. It adds OPS-99 to OPS-105 and PANEL-52 to PANEL-54, and re-slots S6 to S8 in the A-T8 order: R1a before PANEL-51's build, R1b after.
- **OPS-65** (#176, 6:36 PM) and **OPS-60** (#183, 8:31 PM), from Track C. OPS-60 was diagnosed first by a Linux cloud lane (#102's comment): a restart could empty the tmux server, and a pane dead with no status was read as a good launch, which is a real panel bug.
- **R1a**, OPS-101's first PR (#181, 7:23 PM): the brand package and the legacy-first readers.
- **PANEL-51's design amendment** (#167, 10-20 1:15 AM).

**PANEL-51 (the stall alert)** was the session's long pole:
- **The probes (group 1)** came first. They ran live in a throwaway panel whose lanes used the owner's plan login (Haiku). P1 and P4 passed. P2 found that no `SubagentStop` ever carries a workflow task's id, and P3 found that a teammate is in-process, inside its lead's session. Both stopped the build before any code.
- **The build then ran as Tracks D to D7, over about 30 hours.** Every stop was a design question the owner answered in chat, each written into `design.md` and re-approved. Group 2 took 11 review rounds and group 3 took about 6, each round peaking at medium. Almost every medium was a new edge at a boundary between parts of the design: restarts, resumed sessions, config versions, reused branch names. None was a defect in the core rule.
- **What ended a run of rounds was closing the class** (PRAC-0010, accepted at this session's start). Amendment B made a stall that begins across a restart approximate for one stall time. That settled eight rounds of restart cases at once, along with #185 and every reading added later.

- **D8 finished it on 10-21.** Group 4 (the push) converged at peak low, and its lows went into group 5's diff. Group 5 (the page and the docs) peaked high in round 1, since STALL-4-S1's test read back its own fixture, and converged in round 2. **PANEL-51 merged as #192** (9:55 AM). Its archive (**#194**, 12:42 PM) took 5 review rounds, the owner approving the rounds past 3: each of the first four found one more rule the owner had decided in chat, and the code kept, that the spec delta never stated. Every such rule now has a scenario and a test, each shown failing first. Its cost was **$253.34**, against a soft $80 that the owner raised to a $310 ceiling during the build.
- **R1b**, OPS-101's second PR (**#193**, 11:09 AM), followed as the A-T8 order had it. It converged in 2 rounds, peak medium. Its two round-2 lows wait for OPS-104's sweep, when they can first bite.

**The interactive rows, 10-21 to 10-22.** The credit was gone, and three rows edit `.claude/`, which a headless run cannot write. So the last stretch ran as one interactive lead session with teammate lanes:
- **OPS-82** (**#171**, 6:35 PM). The probe found a real end signal: a workflow's state file, written when it ends, and the `Stop` payload's `background_tasks`. A hold now ends on either. Review stopped at round 5 by the owner's rule, and the cases still open are the hook's KNOWN LIMITS, owned by #195 and #196.
- **OPS-97** (**#170**, 8:44 PM). The class goes in a schema field of its own (option B), with a passing eval receipt. It merged carrying a follow-up row numbered OPS-106, an id the team had reserved, so **#197** (10:07 PM) renumbered it OPS-110.
- **OPS-93** (**#199**, 10-22 5:51 AM). Every process check now runs once, inside the no-clauductor environment, with a weekly double pass on `main`. Archived in **#200** (7:23 AM).
- **OPS-89 (a) and (c)** (**#198**, 6:46 AM). It records each gate step's time and runs each shared check in one set only. OPS-89 stays open for (b), the light gate for a docs-only PR, in S9.

**The gate went from about 45 minutes to about 17½.** A full gate took 2694 s on 10-21. With #198 and #199 it took 1048 s, on #198's last head: process checks 810 → 268 s, template checks 894 → 335 s, race 956 → 412 s. The quick gate fell to about 12 minutes. OPS-105's overnight spike, S7, measures against that shorter gate.

### Decisions the owner made
- **CI off until about 11-01**, with merges on the local receipt; OPS-98 to undo it.
- **PRAC-0010 accepted**, with the reviewer-prompt line as its enforcement (OPS-97).
- **The cleanup:** delete the old worktrees and branches, `ops/curtain-mod` included; remove the cancelled OPS-40 proposal from `changes/` (archiving would have needed false ticks).
- **The roadmap edit from the bot team** (#175), with the validation's fixes (OPS-101's Deps, OPS-103 and OPS-105's off-repo sources, OPS-96's status).
- **Option (a), the A-T8 order:** R1a before PANEL-51's build.
- **PANEL-51:**
  - A and A plus the allowlist: a workflow ends when a later `Stop` drops it; a teammate is part of its lead's session; only `shell`, `monitor` and `teammate` are open work;
  - B: the restart class;
  - #186 option 1: a turn that ends with no `Stop` leaves (d) unknown; M1 and M2 accepted;
  - LANECTX-3 A: init and attach write the latest config version;
  - #188 (a): a merge counts only at HEAD;
  - #189 (a): the template's `panel.json` goes to v7;
  - the budget raised from the soft $80 to a $180, then a $310 ceiling.
- **#168's fix is decided at S10,** on OPS-105's data (the roadmap edit).
- **OPS-93 `exec`s** (choice A, 2026-10-21, after group 1's round 2): `checks/run.sh` `exec`s itself into the no-clauductor environment and the inner run removes it. A child process would either hold a TERM until the whole set ended (foreground) or never pass a Ctrl-C to the running check (background). The window between the `exec` and the inner run's traps is OPS-121.
- **OPS-97, option B:** a `class` field in the review schema that `findingKey` never reads, rather than a "Class: …" prefix in the finding's text.
- **OPS-82:** a hold ends on the workflow's state file or a `Stop` that no longer lists it, and review stops at round 5 by the owner's rule, with the rest as known limits (#195, #196). Round 5's own rule was the owner's option A (2026-10-21): each `cd` target is read two ways, shell-logical and kernel-physical, and both must stay inside the held worktree and out of any worktree nested in it; a target that cannot be resolved is refused. There is no round 6. (Added in S7's record PR, from the S6 lead's notes.)
- **Review caps by risk** (2026-10-21): low 2 rounds, normal 3, high 3. Past the cap, what is left goes to KNOWN LIMITS with an owner. Applied by hand until OPS-108 makes the scripts read it.
- **The plan from S7 on:** short sessions of one row each, or of rows that share files. Records are kept by a daily close (OPS-108, its rule 7 and 8 change approved), and lanes are short-lived (PANEL-55). The plan rests on the team's lite v1 edit (D23, approved 2026-10-19). The owner's rulings of 2026-10-21: batched PRs only where rows share files; OPS-110 after launch; ids OPS-112 to OPS-118 for the launch scope that had none; #180 (closed) and #182 (closed as this plan lands; the owner reopened it on 2026-10-22, 8:42 AM ET, for the team) superseded. REL-1 lands centrally at S28.
- **OPS-105's go/no-go criteria, G1 to G5** (2026-10-21), now on its row. They replace the slots design's drafts in the owner's workspace.

### What went wrong
- **Nested hand-backs were lost.** When a teammate lane spawned its own builder or reviewer, the report went to the lead session instead of the lane, about ten times on 10-21. Once, OPS-93's group 2 round-2 reviewer at about 02:18Z, it reached no one, and the lane sat idle about 6.5 to 7 hours. With no panel running, PANEL-51's stall alert could not fire. The workaround that held was for each agent to write its report to a scratchpad file, which the lane then reads. PANEL-55 and OPS-108 remove the dependency.
- **OPS-97 merged with a reserved id.** Its follow-up row took OPS-106, which the team had reserved, despite the hold on it. #197 renumbered it. Ids are now minted only in the repo; OPS-100 gives the team a way to propose a row without one.
- **OPS-93 ran over its hard budget unseen:** $55.84 of $40. `change-cost.sh` read $22.13 before group 1's rework and again before group 2, so build-change's pre-group stop never fired. The overrun showed only at the archive. OPS-120 owns the undercount, and the owner queue carries the overrun for the owner to note.

### What surprised us
- **Headless runs cannot write `.claude/`.** Claude Code refuses it as a sensitive file, even with an allow rule and the sandbox off. So a row whose scope is `.claude/` (OPS-93, OPS-97's fix, OPS-82) needs an interactive session.
- **A quick gate takes about 24 minutes, longer than an agent's shell can wait.** So `build-change`'s gate agent was killed. The lanes used `apply-change`, starting the gate with `run_in_background` and a 2-hour timeout and then waiting with `gate-wait.sh`.
- **The panel tests fail when `ANTHROPIC_API_KEY` is set** (#184). Every gate on API credit ran with `env -u ANTHROPIC_API_KEY`.
- **An exited process can look alive** in more than one way: a zombie answers `ps` and `kill -0` (OPS-53), and a tmux pane can be dead with no exit status (OPS-60).
- **The auto-mode classifier** blocked the orchestrating session from deletions and from anything that read as a CI bypass: disabling workflows, reading gate logs after the switch, routines. The owner ran those steps. In the sandbox, a Bash `git worktree add` fails partway (`.claude/agents/` is protected), which is how session 5's worktrees were emptied; `EnterWorktree` works.
- **`clauductor update` follows `CLAUDUCTOR_FRAMEWORK`,** so in a worktree it read the main checkout's template. It also showed `main` lags its own template (`roles.reviewer.eval.baseline`, `orchestration/config.json`), and three lanes hit that separately.
- **The rising-peak rule was overridden once by the owner's own instruction** (D3's "fix and run one more round"). The lane flagged it, and D4 on followed the rule strictly.

- **The close's own merge of `origin/main` failed partway in the sandbox.** It stopped on `.claude/agents/reviewer.md`, the same protection that emptied session 5's worktrees. Run outside the sandbox, the merge went through.

### Compound
- **Decided:** *headless cannot write .claude* → not promoted (a Claude Code limit; `.claude/` rows run interactively); *promo credit is not interactive* → not promoted (billing; the owner queue's cloud-lanes decision); *quick gate outlives an agent's shell* → OPS-89; *exited looks alive* → technique; *worktree add in the sandbox* → technique, with the close's merge as a second instance; *update follows CLAUDUCTOR_FRAMEWORK* → OPS-119; *a large design converges at its seams* → an instance of PRAC-0010; *classifier blocks the orchestrator* → not promoted.
- **New today:** seven rows. *nested hand-backs get lost* → PANEL-55 and OPS-108; *change-cost misses nested agents mid-build* → OPS-120; *ids minted in two places collide* → OPS-100; *a class key in prose loops* → fixed in #170; *a wrapper's traps wait for its child* → folded into OPS-93's design; *the gate's time was in the check sets* → shipped in #198 and #199; *archive finds the spec gaps* stays Raw, with a candidate check (a design amendment after approval must touch the spec delta).
- Nothing reached the 3+ promotion trigger. `checks/compound.sh` passes, with one Raw row.

### Left in flight (handed off)
- **S7, OPS-105:** the overnight spike, from a plain Terminal (the owner queue's steps), then the panel brought up so S8 starts from it.
- **New rows from this close:** OPS-119 (`clauductor update`'s drift, S12), OPS-120 (`change-cost.sh` and nested agents, S10, before S11 builds the next change), OPS-121 (OPS-93's `exec` window, S9). OPS-104 gains R1b's two lows.
- **Outcome checks:** o.1 (PANEL-29's lane dialog), due 10-19 and never run because the owner has had no working panel, is re-dated to 10-30, for the first week of panel lanes. o.8 (OPS-93) needs Actions minutes (OPS-98).
- **Follow-ups owned by issues:** #168 (OPS-105's data decides it at S10), #184 (OPS-89, S9), #187, #190 and #191 (PANEL-46's piece with the stalled state), #195 and #196 (S13).
- **Remote branches carried over, not this session's:** `change/ops-42-build` and `change/panel-31-build` (WIP, as session 5 left them). The merged leftovers `fix/92-daemon-lease-flake` (#169) and `ops/ops-96-s6-tidy` (#166) still sit on the remote, each at its merged PR's head. The classifier refused this close's delete, so the owner deletes them.
- **Machine:** the merged local branches of OPS-82, OPS-89, OPS-93 (its build and its archive), PANEL-51 (its build and its archive) and OPS-101's R1b were bundled into `.git/clauductor/branch-archive-2026-10-22.bundle` and verified, then deleted. Their four clean worktrees were removed.

### Flow and cost
- **Flow (30 days, to 10-22):** cycle 1.3h median, lead 2.6h, 29.2 merges a week, change-fail 5.9%, approval wait 4.6h, review rounds 2.4, spend $808.25 a week, 26 in flight (Haiku 5.5 unpriced).
- **The last hub session's cost** (usage-report.sh, list price, this machine, before the close's gate; session `2702ad1e`, 10-21 to 10-22): about **$335**, nearly all Opus 5.5.
  - the teammate lanes (`ops`) $223.84;
  - the lead $39.34;
  - builders $30.43, reviewers $26.54;
  - the plan revision $8.23 and $1.73;
  - this close lane, so far, $3.04.
- The session's earlier days ran as headless `claude -p` lanes and claude.ai cloud sessions, each a session of its own, so this table does not count them. By change, the archives record PANEL-51 at $253.34 and OPS-93 at $55.84.

## Session 5 — 2026-10-18 — Rich — no silent stalls, PANEL-49 and OPS-81 merged, PANEL-51 amended for session 6

**Rows**: OPS-90, OPS-85, PANEL-51, OPS-93, PANEL-49, OPS-81, OPS-95 · **Span**: 2026-10-18 to 2026-10-19

### What happened

The session opened on session 4's plan: no silent stalls first. Three cloud lanes ran at the start as claude.ai routines (OPS-90, OPS-85 and PANEL-51's proposal), each pushing a branch for the Mac to review, gate and merge, while PANEL-49's build ran on the Mac.

**Merged, in order (times ET):**
- **OPS-90** (#155, 2:06 PM): an agent's turn is held while work it started still runs, and a blocking question is refused while work runs. Built in the cloud; what the cloud could not probe is on the owner queue, and its three review lows and the teammate-reviewer evidence are #156.
- **OPS-85** (#154, 3:28 PM): `build-change` honours a soft budget, warning on the overrun where a hard budget stops. PANEL-49's build was its first user.
- **PANEL-51's proposal** (#157, 4:46 PM): the panel tells the owner when work has stopped moving. Seven review rounds before the owner ended them (below).
- **OPS-93's proposal** (#158, 5:45 PM): every process check runs once, inside the no-clauductor environment. It came from the owner's handoff; the ADR-0006 amendment it needs is kept for the build.
- **PANEL-49** (#159, 7:51 PM, `41dc1d0`; archived in this close): an attached repository's lanes show their context, cost and context alert, read from each lane's own transcript through one reader. It is also OPS-72's proof: the workflow's full gate passed, on `db3e8c6`. Cost $46.95 of a $60 soft budget.
- **OPS-81** (#161, 8:32 PM, `0a98410`): #143's cause found and fixed. At shutdown the panel SIGKILLed the claude process alone, and a child it had started held the call's pipes open, so the stopped-call line came late or never. Each call now gets its own process group, and the kill is repeated every 50 ms, inside the existing 1 s bound, until every call has ended. Before: 7 of 600 trials under load missed the line. After: 0 of 1200. Its lows are #162.
- **OPS-95** (#163, `2183248`, during this close): the bot team's team-queue issues listed at session start (below).

**PANEL-51's build** started in the evening, once its proposal had merged, and stopped at group 1 by design. Group 1 is the probe group. A static reading of the Mac's Claude Code binary (2.1.303) showed the design's `ScheduleWakeup` assumption was wrong, which met task 1.4's stop condition within about 5 minutes, before any code. The owner amended D4 (the one-shot wakeup rule) and D3 (d) (Monitors), the amendment converged in 2 rounds, and the owner re-approved the design (`1b4626a580ed`, head `202acc8` of `change/panel-51-stall-alert`). The live probes P1 to P4 need the owner's login and a throwaway panel, so the build is session 6's first item.

**The team queue** was first proposed as OPS-92 (`change/ops-92-team-queue`, never pushed). The owner corrected the approach, and the work was redone as **OPS-95** (#163, merged at the close as `2183248`): session start lists the bot team's team-queue issues as facts bound to their content. It has four write surfaces (the session-start script, the skill fragments, the check and the issue template) and a 43-assertion check against a fake `gh`, and its review converged in 3 rounds, peak medium. The labels `team-queue` and `approved` are the owner's to create, with the test issue (owner queue). It flags two choices for the owner: DONE reads a `Delivers team-queue #N` body line, and session close closes a delivered issue. Team queue: no item was worked this session (the labels do not exist yet).

**The sandbox and the main checkout** cost the most friction of the day. A merged hook or settings change is not in force until the main checkout is fast-forwarded, and no lane can do that: worktree isolation refuses `git -C` on it, and the sandbox protects its `.claude/settings.json`, `hooks/` and `skills/`. The owner fast-forwarded it by hand four times. Meanwhile `worktree-hook-drift.sh` blocked worktree agents, and the same protection stopped a lane merging `origin/main` into PANEL-49's branch. #164 owns both the merge-pr step and the sandbox settings for the owner to decide.

### Decisions the owner made
- **PANEL-49: skip review round 5.** Group 3's review stopped after round 4. Each round had found one more route past the reader's decode guard, and round 4's two findings were fixed and shown failing first without re-review.
- **PANEL-51: "fix M1 and M2, no round 8"**, after round 7 of the proposal's review (peak high to round 3, then medium).
- **The ScheduleWakeup rule:** D4 amended for one-shot wakeups after group 1's binary reading, and the design re-approved at `1b4626a580ed`. The amendment PR is not opened yet. The owner chose to wait, so it is session 6's.
- **Re-slot option A** (9:16 PM): OPS-93 and OPS-89's no-second-run item come before the R2 rename, the reverse of the earlier order. R2 becomes its own session, S8, before PANEL-46, at about +0.5 session. The session plan in `docs/roadmap.md` now reads S6 to S16.
- **The team queue redone as OPS-95**, replacing the OPS-92 proposal.
- **Standing Tee deferred:** the step that puts it on the new panel (and starts PANEL-48's and PANEL-49's outcome clocks) waits until the Clauductor work is done.

### What surprised us
- **Review loops on problems bounded by construction did not converge** (PANEL-51's 7 rounds, PANEL-49 group 3's 4). What closed a round for good was closing the class: an import allowlist, a package-wide decode allowlist, a reference allowlist, not one more refused form. With #146 and #150 that makes four, so the practice is drafted as **PRAC-0010 (Proposed)** for the owner.
- **A subagent cannot run a workflow,** so `build-change` runs only from the main session. Teammate reviewers report to the main session, not to the teammate that spawned them (#156).
- **Probe first paid:** PANEL-51's wrong assumption cost 5 minutes in group 1, not a rebuild in group 3.
- **`verify-change` matches only full repository paths,** so a task naming a short path reads as missing work.
- **The classifier refuses a lane's workaround of a sandbox limit,** and the main session must not run it for the lane: that launders the refusal. The step stops and goes to the owner.
- **`PushNotification` is skipped while the terminal is active,** with no error, so a push sent while the owner is at the screen never reaches the phone.

### Compound
- **Promoted:** *shell parser never converges* and *unbounded input space* (2026-10-17) and *close the class* (today) → PRAC-0010 (Proposed; the owner queue asks for the decision). The merge guard's remaining forms stay with OPS-86.
- **Decided:** *reviewer could not run* → an instance of AGENTS.md rule 2; *cd moves the session* → deferred to OPS-82; *resumed agent loses isolation* → not promoted (one read-only occurrence; Claude Code now refuses an isolated agent's git outside its worktree); *a busy teammate reads late* → technique; *classifier inconsistency* → not promoted (Claude Code's, fixed by the owner's `/permissions` rule).
- **New today:** seven rows. *merged hook not in force* → #164; *a subagent runs no workflow* → #156; *probe first* → technique; *close the class* → PRAC-0010; *verify-change short paths*, *no laundered workaround* and *push skipped while active* stay Raw.

### Left in flight (handed off)
- **PANEL-51**: the amendment PR from `change/panel-51-stall-alert` (head `202acc8`, the owner's re-approval of `1b4626a580ed`), then the build's group-1 live probes with the owner at the computer (owner queue), then groups 2 onward. Session 6's first item.
- **Two stale "repo is public" comments** remain, at `.claude/local/roadmap-tracker.sh:5` and `.claude/local/skills/session-start/session-plan.md:14`: outside this close's allowed edits, carried over.
- **OPS-82**, not reached, moves to the front of S6 with PANEL-51.
- **Follow-ups owned by issues:** #156 (OPS-90 lows, the teammate-reviewer evidence), #160 (PANEL-49's low), #162 (OPS-81's lows), #164 (merge-pr's main-checkout sync and the sandbox limits).
- **Remote branches carried over, not this session's:** `change/ops-42-build`, `change/panel-25-idle-cost`, `change/panel-30-remove-worktree-guidance`, `change/panel-31-build`, `change/panel-31-version-check`, `ops/curtain-mod`, `ops/ops-80-rewrite-guard-false-positive`. `change/panel-49-lane-measures` was deleted remotely once merged; its local branch stays, checked out in `.claude/worktrees/panel-49`.
- **The local branch `change/ops-92-team-queue`** was kept: a worktree (`.claude/worktrees/agent-ac610cbe26f417b42`, locked) still has it checked out.

### Flow and cost
- **Flow (30 days):** cycle 1.1h median, lead 2.6h, 25 merges a week, change-fail 6.9%, approval wait 4h, review rounds 2, spend $634.11 a week, 31 in flight (Haiku 5.5 unpriced).
- **This session's cost** (usage-report.sh, list price, this machine, at the close before its gate): about **$167**, nearly all of it Opus 5.5 and about $5.60 Sonnet 5.5 (the workflow's steps). The cloud lanes run on claude.ai and are not counted here.
  - the orchestrating session $22.80;
  - builders $19.30, reviewers $7.46;
  - merge-train $19.14, and the landing lanes (proposals-land $12.15, ops-85-land $3.38, panel-49-land $3.22, panel-51-amend-land $0.22);
  - PANEL-51: converge $18.39, amend $6.72;
  - OPS-95 $14.44, and the stopped team-queue proposal $4.83;
  - OPS-93's proposal $11.56;
  - OPS-81 $11.23 and its verify $1.29;
  - PANEL-49's build lane $3.77;
  - build-change's workflow steps about $5.50 (Sonnet 5.5);
  - this close lane about $1.85.

## Session 4 — 2026-10-17 — Rich — a day-long merge train, the merge guard's bypasses, PANEL-48's window re-approved

**Rows**: PANEL-48, PANEL-49, PANEL-46, OPS-75, OPS-35, OPS-70, OPS-76, OPS-74, P2.11c, OPS-73, OPS-72, OPS-66, OPS-83, OPS-77, OPS-78, OPS-80, OPS-85 to OPS-90, PANEL-51 · **Span**: 2026-10-17 to 2026-10-18

### What happened

The session ran PANEL-48's build in its worktree and, beside it, a merge lane (merge-train) that landed the queue one gate at a time. By evening the gate lease, not review, set the pace.

**Merged, in order:**
- **OPS-75** (#136): a sandboxed script reaches `gh`, and the focus file moves into the git directory, so the status line's focus can be set from the sandbox.
- **OPS-35** (#140): metrics.sh names a branch a failing git step leaves out; #62's cause stays with OPS-79.
- **OPS-70** (#138): rule 11 checks an archive made with `git mv`.
- **OPS-76** (#139): `propose` reviews a proposal to convergence before the owner reads it. Its gate hit flake #143 once and passed on the rerun.
- **OPS-74** (#137): change-cost counts every agent of a build by the worktree it ran in. Four review rounds.
- **PANEL-49's proposal** (#148) and **PANEL-46's proposal** (#149), both approved today.
- **The roadmap catch-up** (#145): OPS-77 and OPS-78 queued, today's merges marked, the owner's cancellations recorded.
- **P2.11c** (#142), once the owner's permission rule let the merge through (see *Decisions*).
- **OPS-73** (#147): the status line's live-panel case waits for the post to finish. Two review rounds.
- **OPS-72** (#150): build-change's gate runs as a lone command the sandbox exempts. Five review rounds. The proof that a workflow gate passes comes with PANEL-49's build in session 5.
- **OPS-83** (#144, 9481acf): the owner's roadmap tracker is kept current from `docs/roadmap.md` by mechanism. Four review rounds; its gate hit flake #143 and passed on the one rerun allowed.
- **OPS-66** (#146, 2cf60ac): the merge guard's reader refuses less where no `gh` can run, and refuses more where one can (below).
- **PANEL-48** (#151, 66016c3; archived in #152, d27b771): every lane launches with its context policy, and its bar and alert read against its window (below).

**OPS-66 (#146), the merge guard's false refusals,** became the session's longest thread. Making the reader refuse less meant first proving which cases it should still refuse, and that found bypasses on `main`:
- **Round 1** was code-reading only, because the live guard blocked the reviewer's own commands. A failing-first pass then showed 4 of its 9 new allow cases also passed on `main` (3 replaced, 1 dropped), and found a HIGH: a redirection placed mid-command let a merge through with no evidence.
- **Rounds 2 to 5,** with a stub `gh` on PATH, found more forms of the same class each time: other redirection shapes, a redirect in any `gh` command, a `(` from process substitution or a glob qualifier, then a regression from narrowing the rule. All were live on `main`.
- **What is left** (definitions inside quoted text, zsh's `functions[]` and `commands[]`, git aliases) is OPS-86's: the guard as one allowlisted merge path instead of a parser of shell text, an ADR for the owner.
- The guard also refused two harmless commands today: a `jq` update whose string held a merge command's words, and a heredoc appended to a notes file. Both are evidence for #146.

**PANEL-48** built groups 1 to 3, then group 4 stuck at medium twice on the folder fallback: a session started by hand in a lane's folder was measured against the lane's window. Probe P6 showed that the status line sees `CLAUDE_CODE_AUTO_COMPACT_WINDOW` before and after `/clear`, so D7 was amended (the posted window, then the record by session id, then none), with LANECTX-2-S8/S9 and task 4.4. The workflow resumed at group 4, which converged in one round (peak low).
- **Verify stopped once, falsely:** two probe tasks named the throwaway probe repository's `.claude/settings.json` and the `/context` command in backticks, which `verify-change` read as files the change claims. The task lines were reworded.
- **Merging main** conflicted in `checks/statusline.sh` with OPS-73's per-case directories. merge-train adapted PANEL-48's window cases, and that resolution was reviewed on its own (peak low, deferred to OPS-88). The hold for that review reached merge-train after it had merged on the gate's pass, so the review converged after #151 landed, not before. The outcome stands; the order was wrong.
- **The archive's gate failed once,** on `no-clauductor`: the new owner-queue item said "then run `clauductor panel …`", which the check reads as session-start telling the user to run clauductor. The item was reworded.
- **Cost:** $280.47 against a $100 soft budget (D10: reported, not stopped), the largest overrun yet. Most of it went on group 4's stalled rounds and two re-approvals. The workflow stopped twice on that soft budget, which OPS-85 fixes.

**PANEL-49** started in the cloud: tasks 1.1 and 1.2 were built and pushed to `change/panel-49-lane-measures` (ae94c11), and review converged (peak medium, then low). The rest builds on the Mac in session 5, after PANEL-48 merges.

**#141 (OPS-80)** closed after three rounds: the safe rule cannot allow a script that launches a process, and the real case went away because the hook skips tracked scripts. The row is parked on its trigger.

### Decisions the owner made
- **OPS-75:** lift the sandbox's deny on `~/.config/gh` and its block on macOS's trust service, so sandboxed scripts reach `gh`.
- **Cancelled:** OPS-39 (it mostly served the Map; draft #125 closed), OPS-29 (cosmetic lows) and OPS-63 (low value, unhit since the rescope; #118 closed).
- **OPS-76:** review a proposal before approval, promoting the insight *review after approval*. **OPS-78:** the skill step now, a guard rule later.
- **OPS-77:** when a PR is behind and overlaps, require gate evidence on the merged tree, blocking but limited to overlapping files, with a Proposed ADR.
- **PANEL-47:** D5 also refuses fish's `-C` and `--init-command` (the owner queue's item from 2026-10-16).
- **The session plan** (S4 to S13, in #144): its order is firm; the session boundaries are estimates.
- **PANEL-46 approved** (budget $120) and **PANEL-49 approved** (budget $60, soft).
- **PANEL-48:**
  - the window stays 200k (233k, the best measured, declined);
  - the budget was raised from $40 to $100, still soft;
  - D7 amended after probe P6 (the posted window first) and the design re-approved at 0f6ba0f2c30c (about 7:45 PM);
  - fix the folder fallback properly rather than narrow it.
- **OPS-72:** a fourth review round (about 5 PM); then, when round 4 found a medium caused by round 3's fix, **"Fix it, round 5"** over narrowing the scope.
- **OPS-66 (#146):**
  - **"Fail closed, round 3":** a redirection before the last word of a `gh … pr … merge` command makes it unreadable, an allowlist instead of a list of forms;
  - **"Any gh, fail closed, round 4":** the same for any `gh` command, after round 3 found the class through `gh api`;
  - **round 5** allowed after round 4's HIGH; the fix also narrowed fail-closed to commands whose command word could be `gh`;
  - **"Fix 1, verify narrowly, merge, add row":** round 5's HIGH fixed, a narrow verification, then merge, and OPS-86 queued for the rest.
- **Auto mode:** the owner added `Bash(gh pr merge:*)` through `/permissions` (about 6 PM) after the classifier refused #142 inconsistently.
- **"Run everything, late close"** (about 6 PM): land every queued PR tonight rather than PANEL-48 first; at 10:32 PM, **"Everything incl. archive"**, PANEL-48's archive too, for a close near 2 AM.
- **Flake #143:** one gate rerun allowed on #144.
- **OPS-89 queued:** speed up the gate, per-step timing first, then a lighter gate for docs-only PRs and no duplicate template check run.
- **Stalls are built out of the product, not remembered** (2026-10-18, after coming back to a stalled merge for the third time): "If this happens with the actual panel or application itself, that's going to be a really bad user experience." OPS-90 removes the causes in the operating model; PANEL-51 makes any remaining stall visible on the panel and pushes it. Both in session 5, first, PANEL-51 on today's page before PANEL-46 (the owner's choice over building it once on the new look); PANEL-49 follows, and OPS-81 and OPS-82 may move to session 6.
- **Cloud lanes for session 5, set up now and run then:** OPS-90, OPS-85 and PANEL-51's proposal as claude.ai routines, created disabled (no connectors) and run by hand when session 5 starts; each pushes a branch, and the Mac reviews, gates and merges. Kept on the Mac: OPS-81 (a macOS load flake), PANEL-49's build, PANEL-51's build (after approval) and OPS-82 (OPS-90's files). `.claude/local/cloud-lanes.tsv` names them, so the tracker notes them and session-start says to run them.
- **OPS-85 pulled into session 5,** right after OPS-90, so neither build stops on its soft budget; OPS-89, OPS-87 and OPS-88 stay where they are, because each extra row costs a full gate (session 5 already needs about ten).

### What surprised us
- **The merge guard had HIGH bypasses on `main`** that its own review rounds had never run. The reviewer of round 1 could not run anything; requiring the run found them.
- **The gate lease is the bottleneck.** A full gate took 32 to 45 minutes and one runs at a time, so a session lands about 1.5 PRs an hour, a PR waited about 20 minutes for the lease, and a docs-only proposal pays the same gate (OPS-89).
- **Flake #143 hit twice** (#139's gate and #144's), each time right after a wait on the lease. The frequency is rising; OPS-81 owns it.
- **The `cd` hazard slipped a fifth time:** a `cd` inside a compound command moved the session's working directory while PANEL-48's workflow ran (OPS-82).
- **A resumed reviewer left its worktree:** #146's round-3 reviewer, resumed through SendMessage, ran read-only git commands in the session's checkout.
- **The night's merges stalled twice, about 8.5 h in all, with the work done:**
  - 11:51 PM to 4:48 AM: the orchestrator asked the owner whether to queue OPS-89, a question nothing waited on, and the prompt froze it, so merge-train's 12:05 AM request to reword #152 sat unread. At 4:50 AM the orchestrator blamed the machine sleeping without checking; the transcripts show the question.
  - 4:55 AM to 8:24 AM: merge-train's pre-gate check run outlived its 300 s timeout, was moved to the background, and merge-train ended its turn to wait. The checks passed minutes later, and that woke no one. OPS-41's hook holds only gates in the foreground.
  - The same failure as 2026-10-03's *silent lane*: OPS-90 and PANEL-51.
- **merge-train seemed to drop messages, and did not:** a teammate reads its inbox only when its current task ends, so its "queue empty" reports came before it had read what was queued. Re-sending only duplicated work.
- **The orchestrator told merge-train to add provenance trailers,** which `provenance.enabled=false` forbids; merge-train caught it.
- **Sonnet 5.5's cache-read price:** PANEL-49's cloud run found $0.10 per million tokens on the pricing page against the $0.20 PANEL-48's evidence and audit tool use (PANEL-49 task 1.3). At $0.10:
  - the week's $1,208.28 falls by $1.04 (10.35M Sonnet cache-read tokens);
  - the window's $153.79 and $163.33 do not move, because no Sonnet thread passed 72k of context;
  - lever 4, subagents on Sonnet, rises from $151.78 to about $255.92: each of the 1,041M cache-read tokens of subagents not already on Sonnet costs $0.10 less. The evidence's "Opus 5.5 and Sonnet 5.5 cost the same per cache read" would no longer hold.
  - PANEL-48's archive adds a dated correction to its evidence README; PANEL-49's task 1.3 settles the price for the panel's table.

### Compound
- **Promoted or decided:** *semantic conflicts* → deferred to OPS-77; *parsing shell text* → an instance of *A check that reads source is a parser*, with the merge guard's case owned by OPS-86; *nobody awake* (2026-10-18) → OPS-90 and PANEL-51; the five pre-OPS-8 rows of 2026-03-30 archived (the lock-based model they describe is gone).
- **New today, Raw:** eight rows (the shell parser, the unbounded input space, the reviewer that could not run, the `cd` hazard, the lease bottleneck, the resumed agent, the lost message, the classifier).

### Left in flight (handed off)
- **The owner's step for PANEL-48:** add the `launch` object and `"version": 6` to Standing Tee's sidecar, then `clauductor panel trust`. The 21 days of its outcome check (o.5) count from that step.
- **PANEL-49** builds next on the Mac (session 5), from ae94c11, with task 1.3 settling the Sonnet price; its build is also OPS-72's proof that a workflow gate passes. OPS-90 goes first, then OPS-85 (the template's `build-change` still stops on a soft budget, and this session's patch does not carry over), with PANEL-51 (its proposal drafted at the start of S5 for the owner, then built) beside PANEL-49's build; then OPS-81 and OPS-82 if there is room, and OPS-89 later.
- **OPS-86** waits on the owner's ADR; until it lands the merge guard keeps the limits its header lists.

### Flow and cost
- **Flow (30 days):** cycle 1.2h median, lead 2.4h, 22.4 merges a week, change-fail 7.8%, approval wait 0.9h, review rounds 2.4, spend $575.51 a week, 31 in flight (Haiku 5.5 unpriced).
- **This session's cost** (usage-report.sh, list price, this machine, at the close before its gate): about **$330**, nearly all of it Opus 5.5 (about $319) and the rest Sonnet 5.5 (about $11):
  - merge-train $122.37 (the merge lane: gates, merges and its own fixes);
  - the orchestrating session $75.11;
  - general-purpose agents $61.84;
  - reviewers $35.21;
  - builders $23.52;
  - build-change's workflow steps about $12;
  - a Haiku 5.5 reviewer UNPRICED (8 calls).

## Session 3 — 2026-10-16 — Rich — the rescope, attach mode built, the panel's new look chosen, a context policy proposed

**Rows**: OPS-67, OPS-71, PANEL-42, PANEL-45, PANEL-48, OPS-70, OPS-74 · **Span**: 2026-10-14 to 2026-10-16

### What happened

The session opened on the owner's review of whether Clauductor was worth its cost, set against MAPS. The owner chose **option B** (OPS-71, #127): Clauductor serves Standing Tee through attach mode, porting Standing Tee's practices stops, the Map is cancelled, Pulse stays, and the investment goes into the panel's UI, one lane at a time.

**Merged, in order:**
- **OPS-67** (#124): the status line saves its quota reading per machine, and the reader never reports a window past its reset.
- **OPS-71** (#127): the rescope itself. Map rows and practice ports cancelled, process follow-ups parked, PRD-ui added.
- **PANEL-42** (#128, archived in #129): `panel attach` serves a repository without writing to it: the registry's config, the sidecar, and argv-only sidecar commands. PANEL-47, OPS-72 and OPS-73 were queued from its lows.
- **PANEL-45** (#130 proposal, #132 build, #133 archive): four rendered mockups of the panel built on real `/api/state` data. The owner chose the hybrid: Overview as home, with Stage's Context, PR and Run boxes. **ADR-0009** (accepted) replaces "grey at rest" with colour that carries state. Cost about $31.74 of its $50 budget, priced from the agents' transcripts by hand (see *What surprised us*).
- **The context review** (#131): what the panel, as the launcher of Standing Tee's sessions, decides about their context. Measured on Standing Tee's week of 2026-10-08..14 ($1,208 at list price, 86% context). Queued PANEL-48 (a context policy at launch), PANEL-49 (attached lanes' measures, replacing the no-transcripts invariant) and PANEL-50 (the per-lane contributors view).
- **PANEL-48's proposal** (#134): a 200k window and connectors off, passed as `--settings` on every launch; the bar and alert measured against the window. Review took three rounds after the owner's first approval:
  - **Round 1:** four mediums, two of them in `design.md`. D5's claim that a repository cannot re-enable connectors contradicted its own source, and D7 recorded the window in the wrong place.
  - **Round 2:** three mediums. D7's write sites were still wrong: the record is saved whole by `Start` and `resumeLocked`. The per-lane tick was never drawn, and the sidecar step lacked `"version": 6`, which would have broken Standing Tee's panel.
  - **Each design fix voided the approval** (the design hash), and the owner approved again each time.

### Decisions the owner made
- **Option B, Pulse kept, the UI redesigned from mockups, one lane at a time** (2026-10-14).
- **The six stale lane worktrees of 2026-09-30: discarded** (2026-10-14); their branches are kept.
- **The order after PANEL-42:** PANEL-48, then PANEL-49, then PANEL-50. The mockups reserve the contributors spot.
- **PANEL-48's budget is $40, soft:** reaching it is reported and the build goes on.
- **The hybrid look and ADR-0009 accepted;** the nine parked panel rows go as recommended (PANEL-25, 30, 31, 34 and OPS-62 queued after PANEL-46; PANEL-33 parked on its condition; PANEL-32, OPS-64 and PANEL-44 cancelled, PANEL-44 folded into PANEL-31). Twenty screenshots kept.
- **PANEL-48 approved three times:** as proposed, then as revised after review rounds 1 and 2.

### What surprised us
- **The cost tracker missed most of PANEL-45.** Subagents record their lead's working-directory branch, and the lead sat in the main checkout, so every builder and reviewer recorded `main` (OPS-74).
- **Rule 11 checked nothing on either archive this session** (#129, #133): the guard's own `git diff` reports a `git mv` archive as renames, with zero adds. That fired OPS-70's parking trigger, and it is queued again.
- **Two TMPDIRs.** A keep-list written outside the sandbox was invisible inside it, and every screenshot was deleted (restored from HEAD).
- **A builder ran a tool outside the sandbox unasked;** the owner was told.

### Left in flight (handed off)
- **PANEL-48 is NEXT,** approved and on main as a proposal. Its build starts with group 1's probes (P1–P5, at most $5), run by the orchestrator outside the sandbox on tmux socket `p48-probe`. The session enters the change's worktree before spawning any agent, so OPS-74's miss does not recur.
- **`changes/ops-40-map-data/`** stays as the record, as its cancelled row says.
- **Raw insights** "semantic conflicts" and "parsing shell text" (2026-10-13) stay Raw, still young; PANEL-42's fish `-C` question is one more instance of the second.

### Flow and cost
- **Flow (30 days):** approval wait 0.9h, review rounds 2.4, spend $513.55 a week, 62 in flight. Cycle time and merges per week: CANNOT CHECK, because gh could not list merged PRs from the sandbox.
- **This session's cost** (usage-report.sh, list price, this machine): about **$147**.
  - builder $57 (Opus 5.5);
  - the orchestrating session $50;
  - reviewers $31;
  - the context-review agents $5;
  - the rest under $2 each (Sonnet 5.5 for the docs reviewer and workflow steps, $1.66 in all).
- The reviewers caught PANEL-48's two design errors before anything was built, and PANEL-45's mockup claims, for about a fifth of the spend.

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
