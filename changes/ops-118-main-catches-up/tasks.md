# Tasks: ops-118-main-catches-up

## Progress
- 2026-10-22 proposed (session 9); nothing built yet. The build starts from `main` once the owner
  approves, in a later session. The roadmap row stays `⬜ queued` until its build PR opens.
- 2026-10-22 group 1 built (the switch, its command, and the fast-forward on the liveSet's
  catch-up source); groups 2 to 4 not started. Groups 2 and 3 hang off `catchUpResult` (the
  branch, the old and new commits, the pull requests and the login), which `fastForward` returns.
- 2026-10-22 group 1 (The switch and the fast-forward (D2, D3)) built and reviewed: converged in 1 round(s), peak low; grades R1 pass
- 2026-10-22 group 2 built (trust on arrival, the note on the record, the push), not yet
  reviewed. Group 3 plugs its rebuild into `Options.selfUpdate` (decision log); until it does,
  every arrival is judged by the running binary, and `configCheck` has only tests as callers.
- 2026-10-23 group 2 review round 1 fixed: catch-up notices reach the Mac whole (decision log).
  Open, outside group 2: group 1's `TestCatchUpFastForwardsAfterAMerge` failed 1 run in 5 (not
  `-short`): it reads the panel log as soon as HEAD moves, and the log line is written after the
  merge returns.
- 2026-10-23 group 2 review round 2 fixed: that test waits on the log line; scenario IDs only on
  the tests that drive them (decision log).
- 2026-10-23 group 2 review round 3 fixed: no catch-up outcome reaches only the panel log (an
  unread config, a reload that fails after its notice) (decision log).
- 2026-10-22 group 2 (Trust on arrival, and the push (D1, D6)) built and reviewed: converged in 4 round(s), peak low; grades R1 fail, R2 concern, R3 concern, R4 pass
- 2026-10-23 group 3 built (3.1 to 3.3: the rebuild, the restart and exit 75, `config-check`,
  `go` on the agent's PATH), not yet reviewed. 3.4 is the owner's, on their Mac after the merge.
  `Options.selfUpdate` is now Run's `selfUpdater`, so `configCheck` has a production caller.
- 2026-10-23 group 3 review finding fixed (a stop during the restart's wait dropped the rollback
  notice; decision log). A resumed run found 3.1 to 3.3 complete on disk: `go test -short ./...`
  passes, the group's tests pass under `-race` without `-short`, CATCHUP-4-S1 to S11 trace as
  CITED, and `TestRestartStoppedDuringItsWaitStillSendsTheRollback` fails with the send on the
  run's context. Left: the orchestrator's gate and review; 3.4 (the owner's, after the merge).
- 2026-10-23 the owner raised the budget from $120 to $175 at $112.95 spent, after two group 3
  review runs stopped "not falling", each round finding a new medium. Open at that stop: (a)
  `panel config-check` judges with `config.LoadConfig` alone, so a config arriving with a
  rebuild skips `canStart` (the tmux socket clash, `live.go`; D1 (e)); (b) no test drives the
  real `selfUpdater`'s preflight config-check failure and its discard of `clauductor.new`: only
  the stand-in at `catchup_trust_integration_test.go:565` re-implements that block.
- 2026-10-23 group 3: (a) and (b) fixed (decision log). `go test -short ./...` passes; the
  group's tests and the new full-panel test pass under `-race` without `-short`, as do
  `internal/panel/...` and `internal/cmd/...` without `-short`; the change's 51 scenarios trace
  as cited. Left: the orchestrator's gate and review; 3.4 (the owner's, after the merge).
- 2026-10-23 group 3: review finding 1 (a stop mid-rebuild lost the rebuild in silence) fixed
  (decision log). `go test -short ./...` passes, the group's tests pass under `-race` without
  `-short`, and each fix's guarding test was seen to fail with the fix reverted. Left: the
  orchestrator's gate and review; 3.4 (the owner's, after the merge).
- 2026-10-23 the owner made the $175 budget soft at $140.83 spent ("I want to see ops-118 get
  completed"): an overrun is reported, not a stop. The third group 3 review run stopped "not
  falling" on one more of review finding 1's class: with no rebuild, `arrive` still loads the
  new config through `ls.tryLoad` on the run's cancelled context (`catchup.go:876`), so a stop
  mid-arrival reports a valid config as not loading and skips trust on arrival. Close the
  class, not the instance: every step of the arrival from the merge on runs off the cancelled
  context, with a test that stops the panel between the merge and a no-rebuild arrival.
- 2026-10-23 group 3: the class is closed (decision log, review run 3). Every read of the arrival
  now runs off the cancelled context: the judgement's load, Swap's PATH check, and the one stop
  read in `arrived`. The reload still ends with the run, as finding 1 decided, but it no longer
  reports a failure that only the stop caused. `go test -short ./...`, `go vet` and `gofmt`
  pass. The catch-up and swap tests pass under `-race` without `-short`, and the new stop test
  passed eight times in a row. Left: the orchestrator's gate and review, and 3.4 (the owner's,
  after the merge).
- 2026-10-23 group 3: review run 4, finding 1 (a failed swap left the withdrawn trust's config
  served trusted here) is fixed and its class closed (decision log). `go test -short ./...`,
  `go vet` and `gofmt` pass, and so do the catch-up, rebuild, restart and trust tests without
  `-short`. Left: the orchestrator's gate and review, and 3.4 (the owner's, after the merge).
- 2026-10-23 group 3 (The rebuild and the restart (D4)) built and reviewed: converged in 2 round(s), peak low; grades R1 concern, R2 pass
- 2026-10-23 group 4 built (4.1, 4.2, and 4.3's roadmap bullet), not yet reviewed. merge-pr
  step 5 takes three branches, held by `change-process.sh` with a falsification per rule; the
  plugin is rebuilt and this repo's copies updated (`.claude/skills/merge-pr/SKILL.md`,
  `.claude/checks/change-process.sh`). Left: the orchestrator's gate and review; 4.3's other
  half, the build PR's body saying `Refs #164` (never Closes, Fixes or Resolves: #164 keeps its
  second item), written by whoever opens the PR, who then ticks 4.3; 3.4 (the owner's).
- 2026-10-23 group 4 review round 1 fixed: step 5's check allows only the spans it lists, per
  part, instead of refusing pull spellings; the *Security model* names catch-up (decision log).
- 2026-10-23 group 4 review round 2 fixed: step 5's check chooses a line's part by where it sits,
  not how it starts (decision log). Left as before: the gate and review, 4.3's PR-body half, 3.4.
- 2026-10-23 3.4 and 4.3's PR-body half leave the task list (decision log): 3.4's checks are in
  proposal.md's *How we'll know*, and the PR body is the opener's. Review run 3 of group 4 found
  the step-5 guard's THIRD instance of one class (a pull of the main checkout the guard does not
  judge: rounds 1 and 2 by branch and by position, now the `### 5.` heading line and all text
  after a `####` sub-heading, since the guard reads only its own cut of step 5). Close the class,
  not the instance: enumerate the authority. The guard judges every code span in the WHOLE of
  `SKILL.md` that pulls, fetches into or fast-forwards the main checkout (`pull`, `merge
  --ff-only`, `reset` or `checkout` of the base against `$main` or the main checkout), and
  allows only the one span in step 5's catch-up-off, main-checkout branch, located by its own
  marker rather than by where the guard thinks step 5 ends; a heading line is text like any
  other. Add a `_red` copy for each hiding place the reviews found (another branch, another
  position, the heading, after a `####`, and a new step outside step 5), each seen failing. Also
  fix review run 3's low: `docs/panel.md`'s push bullet says a failed push counts in the
  notifier's failures, which is true only of a stall's push; a catch-up push's failure goes to
  the panel log only (`push.go` `sendCatchUp`).
- 2026-10-23 group 4 review run 3 fixed: the step-5 guard reads the whole of `SKILL.md`
  (`merge_pr_spans`), and the push bullet of `docs/panel.md` says where a failed catch-up push
  goes (decision log). The nine new copies each give one objection, and each passed the old cut;
  the template's, this repo's and the plugin's `change-process.sh` pass their step-5 lines; the
  plugin is rebuilt and this repo's copy updated (`model-roles.json`'s drift reverted again).
  Left: the orchestrator's gate and review.
- 2026-10-23 group 4 (merge-pr, the docs and the template (D5)) built and reviewed: converged in 1 round(s), peak low; grades R1 pass

## Decision log
- 2026-10-22 (proposal) The panel cannot re-check a merge's gate receipt or its Review line (E5),
  so trust on arrival rests on the account that merged, the panel's own login (D1), and the
  fast-forward checks every commit it moves over against it (D3).
- 2026-10-22 (proposal, review round 1) The switch moved out of `projects.json` into its own file,
  since an older binary refuses a registry field it does not know (E10); catch-up runs on a
  liveSet source, never a runtime goroutine, since `restartStop` waits for those (E3).
- 2026-10-22 (proposal, review round 2) The switch is keyed by root and re-checked at each trigger;
  a merge that changes both `framework/` and `panel.json` is judged by the new binary; the
  pull-request lookup avoids GitHub's search, and a transient failure is retried three times.
- 2026-10-22 (proposal, review round 3; fixed in place on the owner's word) The fast-forward
  merges the commit it checked, not the shared ref, which a lane's fetch can move; the install
  hint reads the build's range `R..B`; the rebuild's notices go whatever `alerts.notify` says;
  merge-pr keeps its pull in the main checkout while catch-up is off; a failed build leaves a
  config the running binary reads untrusted; only a fetch, a `gh` call or a timeout is retried.
- 2026-10-22 (proposal, review round 4; fixed in place on the owner's word) `alerts.notify`
  withholds one catch-up notice only, that a config waits for the owner, which the page's
  untrusted banner backs up; every other catch-up notice goes whatever it says, titled
  "clauductor panel", since merge-pr's report no longer names the owner's step. A failed build
  with a changed config sends two notices, one under each rule. merge-pr counts catch-up as on
  only on exit 0 and `on`.
- 2026-10-22 (proposal, review round 5) The titles are a rule of their own (D6, *Titles*): only
  the trust-on-arrival notice is titled with a name, the config it trusted; every other
  catch-up notice, the waits notice included, is titled "clauductor panel" with the checkout's
  path. This corrects round 4's line above, whose "titled" took in the trust notice.
- 2026-10-22 (group 1) `catch-up.json` is `{"roots":[...]}`, decoded strictly, an unreadable file
  read as off for every project (logged); not a bare list, nor an unreadable file failing the
  source; an object leaves room for a field without breaking an older binary's strict decode, and
  off is the safe reading.
- 2026-10-22 (group 1) `panel catch-up` requires `--project`; not the git toplevel of the working
  directory as a default; a lane's worktree would otherwise name a root no project has.
- 2026-10-22 (group 1) `RemoveProject` clears the root before it saves `projects.json`; not after;
  a failure then leaves the project registered with catch-up as it was, never a listed root that
  nothing serves.
- 2026-10-22 (group 1) The wake is per root (the project whose open list lost a PR); the start
  trigger checks every served project (under `--only`, the primary alone), and a project that
  failed to load is skipped; not one wake checking every project; a merge in one repository says
  nothing about another's checkout.
- 2026-10-22 (group 1) The retries run at 1, 3 and 6 minutes counted from the first transient
  refusal, and a trigger for that root starts them afresh; not 1, 3 and 6 minutes between
  attempts; the scenario's "a minute, 3 minutes and 6 minutes passing" reads as elapsed time.
- 2026-10-22 (group 1) Precondition reads run as `git --no-optional-locks -C <root>`; not plain
  `git -C`; `git status` would otherwise refresh the index, a write to a checkout the owner may be
  using.
- 2026-10-22 (group 1) After the fetch the ancestry is checked again (`rev-list --count C..A` must
  be 0); not only before it; a force-push between the two reads would otherwise go to
  `merge --ff-only`, which refuses it but as git's refusal, not the reason.
- 2026-10-22 (group 1) A refusal before the fetch is keyed on the local `<remote>/<branch>`
  commit; not on a remote read, which would be a fetch; the precondition reads stay read-only.
- 2026-10-22 (group 1) `REVERT_HEAD` counts as an operation in progress with `MERGE_HEAD`,
  `CHERRY_PICK_HEAD` and the two rebase directories; not left out; a fast-forward mid-revert would
  bury the owner's unfinished revert.
- 2026-10-22 (group 1) The refusal's push runs from `catchup.go` through `runPush`; not a new rule
  in `push.go`, which is task 2.3; the push only had to happen for [CATCHUP-2-S11].
- 2026-10-22 (group 1) A config outside its repository is checked against both the served entry
  and `projects.json`'s entry for that root; not the served entry alone; `panel add --config`
  rewrites `projects.json` before the registry tick restarts the runtime on it.
- 2026-10-22 (group 1) A commit's candidate pull requests are filtered by `merge_commit_sha`
  before each one's own lookup; not every PR that contains the commit; GitHub lists a PR whose
  branch merely holds the commit, which did not merge it.
- 2026-10-22 (group 1) A notice's body is cut at 600 runes on one line (`catchUpBody`); not
  `signals.Clip`, whose `OneLine` cuts at 140 first, so a deep checkout's path left no room for
  the reason.
- 2026-10-22 (group 1) [CATCHUP-2-S10]'s test fetches D's objects into `refs/test/d` beforehand,
  so the fake `gh`'s hook can `update-ref` the clone's `origin/main` to it; not a real fetch in
  the hook; the hook then runs with no network or lock of its own during the lookup.
- 2026-10-22 (group 2) The bytes are recorded by `install.TrustArrived` (the hash check, then the
  record with its note, no parse); not `TrustExact`, which has no note and parses with the
  running binary, so it could not record before the swap a config only the new binary reads
  ([CATCHUP-3-S8]). It refuses a config outside its repository, which a parse would have judged.
- 2026-10-22 (group 2) The note lives in the trust file beside the record (`notes`, keyed by
  hash), and `CheckTrust` returns it while the hash matches; not `markTrusted` on the running
  runtime alone; the page's report and a restarted runtime read it from the record. A hand trust
  (`clauductor panel trust`, the page) drops it.
- 2026-10-22 (group 2) Group 3 plugs in through `Options.selfUpdate` (`run.go`), which returns a
  `*rebuilt`: `err` (the build or its preflight failed), `check` (the new binary's
  `config-check`, set when the merge changed the config) and `swap`. Catch-up sends the
  rebuild's failure notice from `rebuilt.err`, so task 3.2 must not send its own for that error,
  and runs `swap` after the trust is recorded and its notice sent; not a separate call from
  group 3; one place orders trust, notice and swap.
- 2026-10-22 (group 2) `config.Check` carries the config's `name` as well as `notify` and `runs`
  (the trust notice's title comes from the binary that read the config), `config.CheckOf` builds
  it from a load, and `configCheck` (`catchup.go`) reads a binary's `panel config-check` answer,
  each field required; not `notify` and the run list alone, as D4 words it. Its production caller
  is task 3.1's preflight, through `selfUpdate`; until then only tests call it.
- 2026-10-22 (group 2) The note names every pull request the fast-forward moved over ("from #7,
  #8"); not the last one alone; each one's config passed through the trusted file.
- 2026-10-22 (group 2) A merge that removes `panel.json` is handled as a config that does not
  load, and a failure to read A's or B's blob is logged and leaves the config as the runtime
  loaded it; not trusted or restarted on a guess.
- 2026-10-22 (group 2) The arrival judges under `liveSet.mu`, then sends the notice and restarts
  with it released, re-checking that the same runtime still serves the project; not one hold
  across the notice and the push command, which would stall the page's Trust and the registry.
- 2026-10-22 (group 2) [STALL-3-S12]'s waiting alert is shown at the notifier (`state.Notifier`,
  then `pushNotice`, as the panel runs them), its catch-up notice end to end in a panel; not one
  panel with a live session waiting on a prompt; [STALL-3-S6] already drives that path end to end.
- 2026-10-22 (group 2) Two test-only notes. [CATCHUP-3-S2] and [CATCHUP-3-S6] pull by hand with a
  fetch and `merge --ff-only` once catch-up's first poll is done; not `git pull`, which merges
  `FETCH_HEAD`, a file a panel fetch rewrites. Only a start or a pull request leaving the list
  starts a check, so no panel fetch races it.
- 2026-10-23 (group 2, review round 1) Class: catch-up notice text cut by the macOS notification
  path. Every catch-up notice now reaches the Mac whole: `sendCatchUp` cuts the body once, at
  `catchUpBodyMax` with its lines kept (`signals.ClipLines`), for the notification and the push
  alike, and the catch-up source shows it with `notifyCatchUp` (`state.NotifyLinesArgv`), never
  `SendNotice`, whose `NotifyArgv` flattens a body and cuts it at `signals.DetailMax` (140). Not a
  shorter body built to fit 140 runes: a deep checkout's path and a reason do not fit there. Not a
  change to `NotifyArgv`, which every stall and lifecycle notice shares. The tests read a notice
  as the Mac gets it: `catchUpNotices` returns `catchUpNotifyArgv`'s last two arguments, so
  [CATCHUP-3-S1], S2, S3, S5 and S9 (and group 1's refusals) fail on the old path, as they did
  when checked.
- 2026-10-23 (group 2, review round 2) Class: a test that waits for one effect and asserts a later
  one. `TestCatchUpFastForwardsAfterAMerge` (group 1's, fixed at its source) waited for HEAD to
  reach C and then read the log line `check()` writes after the merge returns; it now waits on
  the log line, the later effect, and then checks HEAD. Not a wait on the catch-up poll count:
  the line is the THEN itself. Group 2's tests, read for the same class, assert what the
  catch-up poll did only after `arrive` has waited for that poll to finish, or wait on the
  effect they assert (`served`, the card's run).
- 2026-10-23 (group 2, review round 2) Class: a scenario citation on a test that does not drive
  the scenario. A scenario ID goes only on a test that sets up its GIVEN, performs its WHEN and
  asserts its THEN; a test of one part keeps its prose and names no ID, not even in passing, as
  `scenario-trace.sh` counts any ID-shaped token in a test file. Closed across the change's
  citations: `TestCatchUpNoticeReachesTheMacWhole` (it builds bodies by hand),
  `install.TestTrustArrivedCarriesItsNote` (it calls `TrustArrived`, no fast-forward) and group
  1's `config.TestCatchUpRecordedPerRoot` (it calls `SetCatchUp`, not the command) cite none now;
  each scenario stays cited by the test that drives it (`--change --now` shows them CITED).
- 2026-10-23 (group 2, review round 3) Class: a catch-up outcome that reaches only the panel log.
  Every outcome that leaves the checkout, the config served or the binary other than a notice
  said now sends one, whatever `alerts.notify` says: an arrival whose `panel.json` blobs cannot
  be read (`catchUp.unread`: nothing trusted, no restart; the notice says the panel keeps the
  config it loaded, or that the rebuilt one serves the file on disk), and a reload that fails
  after the arrival's notice said it would serve the config (a second notice). An unread config
  is never read as unchanged: `selfUpdate` gets its path, so the preflight's `config-check`
  judges what the new binary will load; not a rebuild skipped, which would add a condition to
  D4's list. This replaces group 2's earlier "a failure to read A's or B's blob is logged". The
  log-only lines left each have another addressee: the fast-forward itself (nothing to act on),
  an unreadable `catch-up.json` (`panel catch-up` fails, so merge-pr counts catch-up off and its
  report names the owner's step, D5), and a project no longer served on that config or started
  again meanwhile (the owner's own remove, add or Trust, which the page shows).
- 2026-10-23 (group 2, review round 3) The round's three new tests (`TestCatchUpSaysItCouldNotReadTheConfig`,
  `…ChecksAConfigItCouldNotReadBeforeARebuild`, `…SaysWhenTheNewConfigNoLongerLoads`) run
  serially; not in parallel; with their five panels parallel, `internal/panel`'s race run failed
  3 of 3 (two gates, one package run) on `TestTemplateLaneGetsItsFirstPromptOnceWhenReady`'s
  "the restart re-notified", and passed with any one of them skipped, and 2 of 2 serial. That
  test is OPS-122's (queued; "its cause is not yet read"). What this build saw of the cause: the
  notice after the restart is grouped by the worktree path, not the lane, so the alert came back
  before the lane was restored, and `Notifier.Process` clears a restored mark whose alert is not
  yet active, which would let it notify again.
- 2026-10-23 (group 3) S3, S6, S9 and S10 are cited in `internal/panel/selfupdate_test.go` and S1,
  S2, S4, S7, S8 and S11 in `run_test.go`; not in `install/selfupdate_test.go` as 3.1 lists; their
  THEN is the panel's notice or its next start, which `install` does not send. S5 stays there
  (`treesnap`), and `install`'s tests cover each condition of `Decide`, `Build`, `Preflight` and
  `Swap` without an ID.
- 2026-10-23 (group 3) A panel that does not qualify gets a notice and `selfUpdate` returns nil;
  not a `rebuilt.err`; that would make D1's trust read "the rebuild failed" for a rebuild that
  never started, where the running binary's own load is the right judge.
- 2026-10-23 (group 3) The build's base is R when R is a commit and an ancestor of B, else A (the
  fast-forward's start); not always A; D4's notice and its `clauductor panel install` line are
  over "the build's range", and the running binary is R's build. A `git` failure in `Decide` is
  said in the notice, never read as framework/ unchanged.
- 2026-10-23 (group 3) The PATH binary is copied (`copyExecutable`, mode 0755) and renamed after
  the installed one, and its failure is only reported in the restart notice; not
  `copyFileAtomic`, which makes the destination's directory 0700, and not a failure of the swap,
  since the login agent runs the installed binary alone.
- 2026-10-23 (group 3) The restart marker (`~/.clauductor/panel/restarted`) is written after the
  swap and before the wait for claude calls, so a stop meanwhile still starts the new binary with
  its notice; `TakeRestartMarker` removes it once read, before parsing it or sending the notice,
  so neither a crash nor an unreadable marker repeats the notice at every start. Not a field in
  an existing file; nothing else there is read once.
- 2026-10-23 (group 3) The wait is `claudecall.Gate.Quiesce`, which takes the call gate's slot
  and keeps it, so no claude call starts between the wait and the run's end; not a poll of the
  running count, which a call could slip past. On its 10 minutes passing the restart goes on and
  the panel log says so.
- 2026-10-23 (group 3) The notice body leads with the rollback (`mv … .prev`, `launchctl
  kickstart`), then the install step, then the merge; the commands name framework/, not its
  path, and a skip's reason is clipped to 200 runes; the body is cut at 600 runes, and these
  are what nothing else says.
- 2026-10-23 (group 3) `clauductor panel` reaches the run through a package variable `runPanel`,
  which the cmd test binary replaces when `CLAUDUCTOR_CMD_HELPER_RESTART` is set, so the exit 75
  is read from a real child process; not a real rebuild in the cmd test, which run_test.go does.
- 2026-10-23 (group 3) The "now running" notice runs on catch-up's source at its first poll
  (`catchUp.atStart`), so it goes through the same sender as the rebuild's other notices; a
  rebuilt binary not swapped (a later check refused) is removed (`rebuilt.discard`).
- 2026-10-23 (group 3) The rebuild's notices pass `send(…, true)`, but `sendCatchUp` withholds
  only a `waits` notice under `alerts.notify` off, so S11's test holds through group 2's sender:
  removing the `true` does not fail it. Left so; a second gate would be a mechanism with no case.
- 2026-10-23 (group 3, review) Class: an early return that skips the one notice a completed
  irreversible step requires. A stop during the restart's wait returned before the rollback
  notice, and a stop between the wait and the send dropped it too (osascript and the push run
  under the run's context). Closed at the source: every notice sent once the swap's renames ran
  (the rollback, a swap that could not put the binary back) and the "now running" notice after
  the marker is taken goes through `selfUpdater.sendAfterSwap`, under `context.WithoutCancel`
  bounded by 10 s, which Run's shutdown waits for; a stop sends the rollback saying the panel
  was stopped before its restart, and goes no further (no browser record, no exit 75). Not a
  notice before the wait: D4 sends it at the restart, and "back in 2 minutes" would then be
  10 minutes early. Tested in `TestRestartStoppedDuringItsWaitStillSendsTheRollback`, which
  fails with the send on the run's context.
- 2026-10-23 (group 3, review) Class: a shared deadline that shortens a per-call timeout the
  spec sets. `sendAfterSwap` put one 10 s deadline around the notification and then the push,
  so a slow osascript took the push's time (STALL-3 gives the push its own 10 s), and the push
  of the rollback could start under an expired context. Closed at the source: the deadline is
  deleted, and `sendAfterSwap` sends under `context.WithoutCancel` alone; the notification
  (`showNotice`, 10 s) and the push (`pushTimeout`, plus its 1 s `WaitDelay`) each keep their
  own bound, so the send ends within about 21 s, which Run's shutdown waits for. Not an outer
  bound of their sum (redundant), and not the push before the notification (group 2's sender,
  every catch-up notice). The worst case, both at their bounds during a stop, is past
  `panel install`'s wait of about 7 s for the old job (the 10 s bound was too) and near
  launchd's default `ExitTimeOut` of 20 s; the usual send takes a second or two. The other
  deadlines of group 3 are each one call's (`git`, `runNew`, `configCheck`, the build), none
  shared. Tested in `TestRestartNoticeIsPushedAfterASlowNotification`, which fails with the
  shared deadline (no push ran).
- 2026-10-23 (group 3, review) Class: a check of D1 (e) that ran only half of `canStart` for a
  config a rebuilt binary is to serve. `panel config-check` judged the parse alone, so a merged
  config naming a tmux socket another served project holds passed the preflight; the swap ran,
  and the new binary refused the project after the restart (the panel itself, when it is the
  primary). Closed at the source: `config.Check` carries the config's `tmux_socket` as written
  (`configCheck` requires the field, so an older binary's answer is no judgement), and the
  preflight runs `liveSet.canServe` on it: the socket the project would be served on
  (`servedSocket`, which `loadProject` now uses too) checked by `socketFree`, the clash loop
  `tryLoad` already ran. A clash is a preflight failure: `clauductor.new` is discarded and the
  running binary judges the config, which `arrive` reports as not loading. Not a check in
  `arrive` after the preflight passed, since the swap would still run and the new binary load
  the clashing file from disk; not `config-check` reading `projects.json`, since a lone binary
  knows neither the projects being served nor the panel's `--tmux-socket`. The worktree list
  `canStart` also reads is the checkout's, not the config's, so it is not checked again.
- 2026-10-23 (group 3, review) The real `selfUpdater`'s preflight failure is now tested on a full
  panel, `TestRebuildPreflightJudgesTheConfigItWillServe`, with no scenario ID: no scenario's
  GIVEN is a config the new binary refuses (CATCHUP-3-S5 starts from a trusted one). The
  throwaway module's `config-check` reads the file instead of printing a fixed answer. Without
  `rb.Discard()` both subtests fail (`clauductor.new` is left); without `su.canServe =
  ls.canServe` the socket subtest fails (the binary was swapped).
- 2026-10-23 (group 3, review finding 1) A stop (logout, launchd unload) during the rebuild or
  between the arrival and the swap dropped the rebuild in silence: the notices went out on the
  cancelled context, the swap was skipped and `clauductor.new` discarded with no word, a trust
  recorded on arrival (judged by the new build) stayed while the old binary kept running, and the
  next start found `from == to`, so nothing re-ran. The class is "a stop mid-operation loses a
  one-shot, event-triggered action with no retry at start". Closed where each step is taken:
  every catch-up notice goes out under `context.WithoutCancel` (`c.send`: a notice records a step
  already taken; the notification and the push keep their own bounds); `check()` runs `arrived`
  after a completed merge before it looks at the stop; the merge itself runs under
  `WithoutCancel`, and no merge starts once the panel is stopping, so a merge is either not begun
  or finished and reported; `Decide` and the config blob reads run under `WithoutCancel`; a
  rebuild stopped before or during its build, or before its swap, says so
  (`errRebuildStopped`, `stoppedRebuildBody`) with the update by hand (`install.UpdateByHand`,
  shared with `Decide`); and a trust recorded on arrival for the rebuilt binary is withdrawn
  (`install.UntrustArrived`, only while the record and its note are still the ones written) when
  the swap does not happen, on a stop or a failed `rb.Swap`, so the config waits for the owner
  instead of being served trusted by a judgement the running binary never made. Not a retry at
  start: it needs a durable record of the pending rebuild and a new D4 trigger, and D4 already
  leaves a merge that could not rebuild to the next rebuild (`Plan.Base` is the running build's
  revision, so the change is in that diff). Not recording the trust after the swap: D1 records it
  before the swap, and the withdrawal keeps that order. The failure table in design.md has no
  stop row; this follows D4's "a merge that could not rebuild leaves its change for the next".
- 2026-10-23 (group 3, review run 3) Class: "a stop turns a read of the arrival into a false
  verdict". Every read the arrival takes after the merge was checked for the run's context, and
  four instances were closed. (1) `arrive`'s judgement loads the config through
  `ls.tryLoad(context.WithoutCancel(ctx), …)`. `tryLoad` now takes its context, and `canStart`
  passes `ls.ctx` as before. `ReadWorktrees` keeps its own 10 s bound. (2) A reload the stop
  cuts short (`ls.reload` failing while `ctx.Err() != nil`), or one the stop forestalls, logs
  that the panel stopped before it served the new config, and sends no "no longer loads" / "did
  not start" notice: the stop made it fail, not the config. (3) `install.Rebuild.Swap` checks
  the PATH binary under `WithoutCancel`, since its `git` calls keep `preflightTimeout`; a stop
  between the two renames no longer skips the PATH binary in silence. (4) `arrived` reads
  `ctx.Err()` once, as `swapNow`, after the judgement, so the unread notice, the discard, the
  swap and the stop's notice follow one answer. Rejected alternative: running the reload under
  `WithoutCancel` too. That would start a runtime (lanes, tmux) for a panel that is stopping,
  and change finding 1's decision that the reload ends with the run. The next start serves the
  file as its trust record says, and the trust notice already said so. Rejected alternative:
  swapping on a stop. That too would change finding 1's decision.
- 2026-10-23 (group 3, review run 3) Tests: `TestCatchUpStoppedMidArrivalStillTrustsTheConfig`
  (`catchup_trust_integration_test.go`) stops the panel right after the merge, and again in the
  reload's first worktree read. Each time the config ends up trusted with the note, one notice is
  sent, and the next start serves the config trusted and runs its card. It cites no scenario: no
  scenario has a stop in its GIVEN. `TestSwapKeepsThePreviousBinaries` gained a stopped case.
  Each test failed with its fix reverted, as follows. With the judgement on `ctx`, the bytes
  stay untrusted. With the reload check off, the config is said to "no longer load". With
  `pathQualifies(ctx, …)`, the PATH binary is not replaced. The single read in (4) has no
  failing test: the race it closes sits between two adjacent reads that no test can stop
  between.
- 2026-10-23 (group 3, review run 4) Class: "a failed swap left unhandled where D1 handles a
  failed build". Closed: a binary built but not put in place now hands the config to the
  running binary just as a failed build does. `rebuilt.unswapped` holds that hand-over. `arrive`
  sets it for a config the new build trusted, and `arrived` sets it for one it could not read.
  `swap` runs it in its error branch, so the one swap notice says the outcome before it is sent.
  For a trusted arrival, it withdraws the trust first. If the withdrawal fails, there is no
  reload, because the record still says trusted. Otherwise it reloads the file on disk
  (`serveArrived`, the same reload `arrive` runs after a failed build). The panel then serves it
  untrusted and offers Trust config…, or it keeps the config it had loaded when the file does
  not load. An unread config is kept as loaded, and the notice says so. The swap error now comes
  last in that notice, so a long path cannot push the config's account past the 600-rune cut.
  Rejected alternatives: a second, separate D1 notice (two notices for one event, and the first
  would claim "keeps running as it was" with no word on the config), and `swap` returning a bool
  for `arrived` to act on (the notice is already sent by then).
- 2026-10-23 (group 3, review run 4) The reload after a failed swap passes no runtime to
  `serveArrived`, so it reloads whatever runtime serves the project now, not only the one that
  was running at the merge. A runtime started in between was started on the trust that has just
  been withdrawn, so it too must load the config again. The log line for a project with no
  runtime now reads "started again or removed meanwhile" (no test named the old wording).
- 2026-10-23 (group 3, review run 4) Test: `TestRebuildSwapThatFailsServesTheConfigHere`
  (`selfupdate_test.go`, a real build, skipped under `-short`) makes the swap's rename fail. It
  has three cases: a config that loads (served untrusted, with Trust config… offered), one that
  does not (the old config stays loaded and trusted, and the notice names the error), and an
  unread one (the old config is kept, and the notice says it could not be read). It cites no
  scenario, because no scenario has a failed swap in its GIVEN. Each case failed with its fix
  reverted. With the withdrawal alone, "loads" timed out waiting for the new bytes and "does not
  load" failed on the notice text. Without the unread hand-over, "unread" failed on the notice.
- 2026-10-23 (group 4) merge-pr's test for the main checkout compares `git rev-parse
  --path-format=absolute --git-dir` with the same for `--git-common-dir`; not the plain pair the
  task names: from a subdirectory of the main checkout `--git-common-dir` prints a relative
  `../.git` while `--git-dir` prints an absolute path, so the plain compare calls the main
  checkout a lane.
- 2026-10-23 (group 4) The owner's step names `./install.sh` and `clauductor panel install` only
  "after a change to the panel's own source (`framework/`, in the repository the panel is built
  from)"; not after any `framework/` change: the skill ships to projects that have no
  `framework/`, and for them that step would be noise.
- 2026-10-23 (group 4) `change-process.sh` holds step 5 with a `step5` function over the step's
  own text (the `### 5.` heading to the next heading), falsified rule by rule on `sed`-edited
  copies, each asserting its own objection; not `has` lines, which cannot say that the
  `--git-common-dir` test comes before the first pull, or that a branch has no pull.
- 2026-10-23 (group 4) ~~"No pull" in the catch-up-on and lane branches matches a command
  (`pull --ff-only`, `git pull`, `git -C <dir> pull`, `pull origin`); not the word "pull", which
  their prose uses to say "do not pull".~~ Superseded by the next entry (review round 1).
- 2026-10-23 (group 4, review round 1) Class: a write to the main checkout that the step-5
  guard's list of pull spellings does not match (`git -C <dir> pull`, `git -C <dir> merge
  --ff-only`, a pull added after the branches). Closed by allowing only what is listed: every
  code span step 5 names (each fenced line, each inline span, wrapped ones joined) must be on
  `step5_allowed`'s list for the part it sits in (before the branches, each branch, the owner's
  pull bullet, the rest), and no `git` or `gh` may be named outside a span; the pull is allowed
  only in the main-checkout branch, after both `rev-parse` tests. Not one more spelling on the
  refused list: the list of writes is open, the list of what step 5 names is not. The cost: an
  edit of step 5 that names anything new adds it to the list, and the objection says so. Not
  closed: a write described in prose with no command and no `git` or `gh` in it, which no check
  of the text can tell from a description.
- 2026-10-23 (group 4, review round 2) Class, a second instance of round 1's: a step-5 write the
  guard lets through because it chose a line's part by how the line STARTS (a bullet shaped like
  the owner's pull, before or between the branches, got the owner's list; a second main-checkout
  heading after the owner's list got the branch's). Closed by choosing parts by POSITION in
  `step5_spans`: the parts open once each, in one order (before the branches, the three branches
  one after another, the owner's step from its bold line, its pull only as that step's first
  bullet), and a line that starts like a heading where its part may not open is an objection of
  its own and is judged by the part it sits in. An unindented line ends a branch into "between or
  after the branches", whose list is empty. Not one more prefix refused: no prefix chooses a part
  anywhere but in its one place. Falsified by six copies (the review's three, a branch heading
  twice, the owner's step twice, a bullet before the owner's pull), each confirmed to pass the
  round-1 guard.
- 2026-10-23 (group 4, review round 1) docs/panel.md's *Security model* names catch-up as its own
  bullet (the switch only you set, the writes to the main checkout, trust on arrival with no
  click, the binaries replaced, the account as the boundary) and the "explicit trust click" line
  names it as the one trust with no click; *What the panel writes to disk* lists `catch-up.json`,
  the main checkout's fetch and fast-forward, the trust record's note, the rebuild's binaries
  (the one on PATH included, outside `~/.clauductor`) and the `restarted` marker. Not a link to
  *Catch up after a merge* alone: the Security model is where the boundary and the writes are
  read whole.
- 2026-10-23 (group 4) *Catch up after a merge* sits under *Operations*, after *The launchd
  agent* it refers to; not under *Config trust*: it covers the checkout and the binary as well
  as trust, and both trust sections link to it.
- 2026-10-23 (group 4) `framework/clauductor update` also merged the template's
  `roles.reviewer.eval.baseline` into `.claude/model-roles.json` and reformatted that file;
  reverted, since it is drift that predates this change and a model-roles edit is
  `pr-merge-guard.sh` rule 13's to judge. This repo's copy still lacks that key.
- 2026-10-23 (the owner) Task 3.4, the check on the owner's Mac after the merge (catch-up on, a
  `framework/` PR merged from a lane, `go version -m`, `clauductor.prev`, the page's token), can
  run only after the build merges, and verify refuses any open task. Its checks move into
  proposal.md's *How we'll know*, read at the outcome check; CATCHUP-4-S1 stays cited by
  `framework/internal/panel/run_test.go`, and its live half is that signal.
- 2026-10-23 4.3's second half, the build PR saying `Refs #164` and never Closes, Fixes or
  Resolves, belongs to whoever opens the PR, which does not exist when verify runs; 4.3 keeps
  the roadmap bullet and is ticked for it.
- 2026-10-23 (group 4, review run 3) Class, the third instance of rounds 1 and 2's: a pull of the
  main checkout the guard never reads, since it read only its own cut of step 5 (the `### 5.`
  heading skipped, the cut ended by a `####`, a step outside 5 never seen). Closed by reading the
  whole skill in one pass, `merge_pr_spans`: every line belongs to a part, a heading included.
  Step 5 runs from its `### 5.` to the next heading of level 1 to 3 (fences skipped), its parts
  open by position as before, its heading is intro text, and a `####` is an unindented line of the
  part it sits in. The text before and after step 5 is held to a closed list, `merge_pr_cmds`:
  the git and gh commands merge-pr runs there (`git diff`, `git worktree remove`, the `gh` reads
  and `gh pr merge`), none of which moves a checkout; a span starting with a command that does
  (`pull`, `reset`, ... with no git before it), text naming git with one, and a second `### 5.`
  are objections too. Not the direction's scan for spans that pull, fetch into or fast-forward
  the main checkout: that is a list of write spellings again, the thing round 1 replaced, where a
  list of what merge-pr runs refuses `gh repo sync` or `git -C "$main" merge origin/main` without
  naming them (so no separate rule for a fetch into a branch: no fetch is listed outside step 5).
  The allowed pulls stay where their own markers put them: the main-checkout branch's, and the
  owner's `git pull --ff-only` as the first bullet of the owner's step, a pull too, which D5 has
  the report name for the owner to run. Falsified by nine copies (the heading, after a `####`, the
  step before 5, a step 6, a merge after the merge, a bare `reset`, `gh repo sync`, git pull in
  prose, a second `### 5.`), each one objection alone and each seen passing the old cut. Not
  closed: a write in prose that names no git ("pull the main checkout").

## 1. The switch and the fast-forward (D2, D3)
- [x] 1.1 `framework/internal/panel/config/catchup.go` (new): read and write `~/.clauductor/panel/catch-up.json`, a list of repository roots; absent or unlisted reads off; written atomically. Tested in `framework/internal/panel/config/catchup_test.go` (new). Citing [CATCHUP-1-S1].
- [x] 1.2 `framework/internal/cmd/panel.go`: `clauductor panel catch-up --project <path> [on|off]` beside `panel trust`, writing only `catch-up.json`, refused for an attached project (`configOutside`, in `install/projects.go`) with nothing written; `RemoveProject` (`install/projects.go`) clears the root. Tested in `framework/internal/cmd/panel_catchup_test.go` (new), with `projects.json` compared byte for byte and `catch-up.json` checked absent after a refusal, and a remove then a fresh add at the same root. Citing [CATCHUP-1-S1], [CATCHUP-1-S2], [CATCHUP-1-S7].
- [x] 1.3 A test that a `panel.json` setting `catch_up` fails to load and names the key (the strict decoder, `config.go`). Tested in `framework/internal/panel/config/config_test.go`. Citing [CATCHUP-1-S4].
- [x] 1.4 `framework/internal/panel/catchup.go` (new): the fast-forward of D3 on a machine-level source of the liveSet beside `pollRegistry` (`live.go`), woken without blocking by the open-PR poll when a pull request leaves the open list (`runtime.go`, beside `lanes_auto_close`'s hook in `autoclose.go`) and once at the panel's start; `catch-up.json` read and `configOutside` checked at each wake; every precondition read with `git -C <root>`; `base` read through `BaseRef()` and split as `<remote>/<branch>`; after the fetch, the commit C that `<remote>/<branch>` names read once (`git rev-parse`), each first-parent commit of `HEAD..C` looked up with `gh api repos/{owner}/{repo}/commits/<sha>/pulls`, then `gh api repos/{owner}/{repo}/pulls/<n>` (`merged_by`, `base.ref`, `merge_commit_sha`), against `gh api user`'s login, before `merge --ff-only C` (C's hash, never the ref's name); each git and `gh` call under a 60 s timeout; a transient failure (`git fetch` or `gh` erroring or timing out, or any git call timing out) woken again at 1, 3 and 6 minutes (on the clock the tests drive), and git's own refusals not retried; a catch-up notifier of its own (keys of project, reason and remote commit, created with the source; not `notifyOnce`) and a panel log line for each refusal, the refusal's notice (and the skip of a config outside its repository) sent whatever `alerts.notify` says, titled "clauductor panel" and naming the checkout's path. Tested in `framework/internal/panel/catchup_integration_test.go` (new), with real git repositories (a bare remote and a clone), a fake `gh` written by `internal/testbin` (for [CATCHUP-2-S10], one whose first lookup moves the clone's `origin/main` with `git update-ref` to a commit after C, as another worktree's fetch would), and `internal/treesnap` for "as before". Citing [CATCHUP-1-S3], [CATCHUP-1-S5], [CATCHUP-1-S6], [CATCHUP-2-S1], [CATCHUP-2-S2], [CATCHUP-2-S3], [CATCHUP-2-S4], [CATCHUP-2-S5], [CATCHUP-2-S6], [CATCHUP-2-S7], [CATCHUP-2-S8], [CATCHUP-2-S9], [CATCHUP-2-S10], [CATCHUP-2-S11] (its push read from the stand-in program `push_test.go` uses).

## 2. Trust on arrival, and the push (D1, D6)
- [x] 2.1 `framework/internal/panel/catchup.go`: after a fast-forward from A to B that changed the in-repository `panel.json`, on the same liveSet source, the checks of D1 (a) to (f): the loaded config's trust and its bytes against the blob at A, the file on disk against the blob at B (`git cat-file`), D3's pull-request check having passed for `A..B`, `canStart`, and the new config's `alerts.notify`; when a rebuild follows (task 3.1), (e) and (f) come from the new binary's `panel config-check` and the trust is recorded before the swap. On success: `install.TrustExact`, the notice, then the reload `liveSet.Trust` does (`canStart`, `restartStop`, `startEntry`, `live.go`) with the note "trusted on arrival from #N" (`markTrusted`); otherwise the untrusted restart or the kept config, with the reason in the panel log; the notice that the config waits for the owner sent only while the loaded config's `alerts.notify` is on, and the notice of a config that does not load sent whatever it says; when the build or its preflight fails, the running binary's own load decides between the two, and the build's error goes in the rebuild's own notice (task 3.2), a second notice. The notice's `+` and `−` lines are the difference between the old and new `TrustRuns` lists. Tested in `framework/internal/panel/catchup_trust_integration_test.go` (new), with a fake `gh` and a stand-in new binary written by `internal/testbin`. Citing [CATCHUP-3-S1], [CATCHUP-3-S2], [CATCHUP-3-S3], [CATCHUP-3-S4], [CATCHUP-3-S5], [CATCHUP-3-S6], [CATCHUP-3-S7], [CATCHUP-3-S8], [CATCHUP-3-S9], [CATCHUP-3-S10].
- [x] 2.2 `framework/internal/panel/install/trust.go`: a trust record may carry the note it was made with, so the page's trust report shows "trusted on arrival from #N" after a restart. Tested in `framework/internal/panel/install/trust_test.go`. Citing [CATCHUP-3-S1].
- [x] 2.3 `framework/internal/panel/push.go`: a catch-up notice runs the push command under STALL-3's rules, `alerts.notify` withholding only the notice that a config waits for the owner; only the trust notice titled with a name (the config it trusted), every other catch-up notice (the config waiting, refusals, a config that does not load, the rebuild's of task 3.2) titled "clauductor panel" and naming the checkout's path; a `waiting` alert still never pushes. Tested in `framework/internal/panel/push_test.go` with its stand-in program. Citing [STALL-3-S12], and re-running [STALL-3-S1], [STALL-3-S6], [STALL-3-S10], [STALL-3-S11] unchanged.

## 3. The rebuild and the restart (D4)
- [x] 3.1 `framework/internal/panel/install/selfupdate.go` (new): from `debug.ReadBuildInfo`, the module, `vcs.revision` and `vcs.modified`; the module `<root>/framework/go.mod` declares, checked first, with nothing sent when it differs; whether R is a commit of the repository and an ancestor of B, and `framework/` differs between R and B; `git status --porcelain` empty; the build into `clauductor.new` under a 5-minute timeout; the preflight (the new binary's build information read with `debug/buildinfo`: the same module, `vcs.revision` B, `vcs.modified` false; then `version`, `panel list`, and `panel config-check` when the merge changed `panel.json`); the two renames for the installed binary and, when its own build information qualifies, the one on PATH. The token, the plist and the hooks are never opened for writing. `framework/internal/cmd/panel.go`: the `panel config-check --config <file>` command, printing the config's `alerts.notify` and run list as JSON, or failing with the load's error. Tested in `framework/internal/panel/install/selfupdate_test.go` (new) with a throwaway module and repository, a stand-in `go` from `internal/testbin` for the failure cases, and `internal/treesnap` over the repository; `config-check` in `framework/internal/cmd/panel_catchup_test.go`. Citing [CATCHUP-4-S1], [CATCHUP-4-S2], [CATCHUP-4-S3], [CATCHUP-4-S5], [CATCHUP-4-S9], [CATCHUP-4-S10].
- [x] 3.2 `framework/internal/panel/run.go`: under `--launchd` only, the restart: wait until no `claude` call of the panel's runs (the call gate), up to 10 minutes; the notice naming R, B and the rollback, with `clauductor panel install` when `R..B` (the build's range, not the last merge's) changed `install/launchd.go` or `install/hooks.go`; every notice of the rebuild, this one, its refusals and the new binary's, sent whatever `alerts.notify` says, titled "clauductor panel"; a marker file for the new binary's "now running" notice; the browser open recorded as `ShouldOpenAtLogin` records it; then the run's context ended, as SIGTERM ends it, with exit status 75 recorded and returned once the run loop's shutdown is done. `framework/internal/cmd/panel.go` exits the process with that status (`os.Exit`, as `internal/cmd/lockrun.go` does), since `cmd/clauductor/main.go` turns every error into 1. A hand-started panel sends the "update yourself" notice instead. Tested in `framework/internal/panel/run_test.go`, which drives the restart path and then a second start in the same home and asserts it opens no tab, and in `framework/internal/cmd/panel_catchup_test.go`, which runs the built command as a child process and reads its exit status. Citing [CATCHUP-4-S1], [CATCHUP-4-S4], [CATCHUP-4-S6], [CATCHUP-4-S7], [CATCHUP-4-S8], [CATCHUP-4-S11].
- [x] 3.3 `framework/internal/panel/install/launchd.go`: `go`'s directory joins `pathForAgent`'s tools. Tested in `framework/internal/panel/install/launchd_test.go`.

## 4. merge-pr, the docs and the template (D5)
- [x] 4.1 `template/.claude/skills/merge-pr/SKILL.md` step 5: find the main checkout (the first `worktree` entry of `git worktree list --porcelain`) and ask `clauductor panel catch-up --project <that root>`, catch-up counting as on only when it exits 0 and prints `on` (anything else, an older `clauductor` included, is off); with catch-up on, confirm the squash commit on the remote's base (`git fetch`, `git log -1`) and do not pull; with it off, `pull --ff-only` only when run from the main checkout itself (`git rev-parse --git-dir` equal to `--git-common-dir`), and from a lane confirm without pulling; the report names the owner's step unless catch-up is on, leaving out the pull when step 5 made it. `template/.claude/checks/change-process.sh` holds step 5 to those three branches: the root from `git worktree list --porcelain`, the "exits 0 and prints `on`" test, the `--git-common-dir` test before any `pull --ff-only`, and no pull while catch-up is on. Then `scripts/build-plugin.sh`, and `framework/clauductor update` from the repository root with the binary built from this checkout.
- [x] 4.2 `docs/panel.md`: a *Catch up after a merge* section (the switch, the fast-forward and its refusals, trust on arrival and what it rests on, the rebuild and its rollback); *Config trust* and *Add, trust and remove from the page* name the second way a config is trusted; *When the cards may be stale* says catch-up clears it; *Notifications* names the catch-up notices and the push; *The launchd agent* names exit status 75 and `clauductor.prev`.
- [x] 4.3 `docs/roadmap.md`'s session-plan bullet *After each merge the owner* says the step applies only while catch-up is off. (The build PR's `Refs #164`, never closing it, is the PR opener's step: decision log.)

- [ ] Slice: an owner can merge a PR from a lane and find the main checkout, the panel's config and the installed panel on the merged main without running a command, at the panel
