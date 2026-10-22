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
