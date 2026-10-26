# Tasks: panel-57-lane-close

## Progress
- 2026-10-24 proposed (session 12); nothing built yet. The build starts from `main` once the owner
  approves. The roadmap row stays `⬜ queued` until its build PR opens.
- 2026-10-24 group 1 built (tasks 1.1–1.6), uncommitted for review. Groups 2 onward not started:
  `CLIHandler` has no production caller until group 2 serves it on the socket.
- 2026-10-25 group 1 (The close verdict and the socket's route) built and reviewed: converged in 7
  round(s), peak high (the owner raised the cap to 6, then allowed a narrow 7th; rounds 6–7 on Fable).
  Two lows left from round 7: the auto-close test's negative path fails by a timeout hang, and the
  lock-time tmux rule is in the Decision log, not the spec delta.
- 2026-10-25 group 2 (tasks 2.1–2.4) built, uncommitted for review. The panel serves `CLIHandler`
  on `~/.clauductor/panel/cli.sock` (`Options.CLISocket`), `owner.json` names it, and
  `install.CloseLane` is the command's client. Round 7's F1 (the auto-close test's hang) is fixed
  too. Next: group 3, the command (`framework/internal/cmd/lane.go`). It owns the three
  scenarios still MISSING in `scenario-trace.sh --now`: 1-S5, 2-S2 and 3-S2.
- 2026-10-25 group 2 (The socket in the panel's life) built and reviewed: converged in 2 round(s),
  peak high. Four lows from round 2 go to group 3's builder: no test starts a panel on the default
  socket path (`install.CLISocketPath`); `CloseLane`'s cut-off words claim a begun close on a dry
  run; its doc comment omits that third kind of error; `deadSocketFile` takes its descriptor
  outside `syscall.ForkLock`. The claudecall flake seen in both groups' gates is #260.
- 2026-10-25 group 3 (tasks 3.1–3.2) built, uncommitted for review: `clauductor lane close <id>
  [--yes]` in `framework/internal/cmd/lane.go`, tested against a stub panel in `lane_test.go`;
  `install.MainWorktree`, which `mainRoot` now calls. Group 2's four lows are fixed too (a test on
  the default socket path, `CloseLane`'s dry-run words and doc comment, `deadSocketFile` under
  `ForkLock`). Next: group 4, the docs and the production caller (D5, D7).
- 2026-10-25 group 3 (The command) built and reviewed: converged in 1 round(s), peak low. Five
  lows go to group 4's builder: `laneProject` runs git through `signals.ExecRunner`, not the
  package's GIT_DIR-free runner; the default-path test needs a writable `/tmp`; the stub skips its
  consent check on a dry answer it cannot read; `RunE` reads `$HOME` before the self refusal; a
  hand-written loop duplicates `config.Projects.Find`.
- 2026-10-25 group 4 (tasks 4.1, 4.3, 4.4) built, uncommitted for review; group 3's five lows are
  fixed. Task 4.2 is built but for one step, so it stays unticked: the template edits and
  `scripts/build-plugin.sh` are done; `env -u CLAUDUCTOR_FRAMEWORK framework/clauductor update`
  (its `Template:` line named this worktree's `template/`) stopped on the sandbox's denial to
  open `.claude/settings.json`, having written nothing (`git status --porcelain` unchanged, no
  file newer than the run's start), so there was nothing to undo. Left: the owner runs that one
  command from the worktree's root, keeps only `.claude/skills/session-close/SKILL.md` and
  `.claude/settings.json` from it, and ticks 4.2.
- 2026-10-25 group 4 (Docs and the production caller) built and reviewed: converged in 3
  round(s), peak low. The owner ran `clauductor update` (its `Template:` line named this
  worktree's `template/`). It merged `.claude/settings.json`, the one allow rule and nothing more.
  Its skill review had no stdin and cancelled, so the owner copied the template's `SKILL.md` over
  this repo's copy, which matched it byte for byte at HEAD. No other drift. Round 1's finding 4
  stays as D5 words it (disputed, Decision log). Round 3's two lows, both about hardening
  `TestEveryCloseWordIsDocumented`, are #265's.

## Decision log
- 2026-10-24: the owner directed all six round-3 findings fixed; finding 2 decided as the reviewer
  recommended: a lane closes only when its branch is merged or its claude has exited (D2).
  "Exited" is read as the program's exit (`Dead`), not a gone tmux session, so a restorable lane
  keeps its restore unless its branch merged (revision review M3); the owner confirms it with D2.
- 2026-10-24: the owner approved D1–D7 as recommended, "with the four as build tasks": the four
  findings review round 3 left open at 595bfe19 are tasks 1.4, 1.5 and 4.3, not design edits, so
  `design.md` stays as approved. Where one of them refines `design.md`'s words (when an ended
  session's hold lifts, D5's retry), the task governs.
- 2026-10-24: merge-pr's review of the approved proposal found two medium and one low, each fixed
  in `tasks.md` alone (tasks 1.6, 2.4, 3.2), `design.md` untouched. Task 1.6 adds one hold with
  new words, "the queues' leases have not been read" or "are not answering": D2's own "a failed
  read keeps more" applied to the lease source, as the stall's `workJudged` already does.
- 2026-10-24, group 1: the verdict lives in a new `state/closeheld.go` (`fillCloseHeld`,
  `closeHeld`, `closeWork`, `statusWords`, `View.LaneCloseHeld`), not in `model.go` or
  `stall.go`. Alternative: add it to `stall.go`, beside `turnWork`. Why: it reads the stall's
  state but is not part of the stall, and `stall.go` is already long.
- 2026-10-24, group 1: the poll's start (task 1.4) is `now - dur` from the *uncapped* duration
  `ApplyAgentsTimed` is given, stored as `Model.agentsPollStart`. `ApplyAgents` without a
  duration records `start = now`. Alternative: cap `dur` at `agentsPollCap` as the reading's age
  is capped. Why: an uncapped start is earlier, so it holds longer, never shorter. The apply
  happens a moment after the poll ends, so the start comes out slightly late; a `/clear` in that
  moment could still be lifted by the straddling poll. That window is milliseconds.
- 2026-10-24, group 1: the `Stop` the verdict reads is a separate `session.closeStop`, merged
  into as `keepStop` merges into the stall's copy, and set by `restoreKeep` when it is not older.
  It survives `s.end` and every `stallPolled` clear, and goes only with a newer `Stop` or with the
  session. A subagent's end is paired at read time, against `s.stall.stops` and `t.ended`.
  Alternative: stop `s.end` from dropping the stall's saved `Stop`. Why: the task says the stall's
  own reading stays unchanged; a separate copy keeps more and leaves the stall alone.
- 2026-10-24, group 1: words D2 does not fix: a lease holds as "it holds or waits for the <queue
  id> lease", naming the first queue in config order; background work is named by each task's
  description, or "a <type>" without one, joined "; ". `sessionWorking` remains a last fallback
  ("claude is working") after `closeWork`. A lane missing from a fresh view is held as "the panel
  no longer lists lane <id>". Alternative: name every lease, or leave the fallback out. Why: one
  name is enough to say why; the fallback costs nothing and keeps more.
- 2026-10-24, group 1: `CloseIf` asks `held` after `closeTarget` and its 404, before
  `stopAndForgetLocked`, and refuses with 409 `held`, "lane <id> is not closed: <why>". The hub's
  lock is taken while the lane lock is held. Alternative: ask `held` before taking the lane lock.
  Why: task 1.2 wants the check made under the lock. No path takes the lane lock while holding
  the hub's (the hub's closures only touch the model), so the order cannot deadlock.
- 2026-10-24, group 1: the socket route's answers: unmerged and unknown-branch refusals are 409
  code `not-merged`; a request cancelled before the close is 503 code `cancelled`, "the request
  was cancelled before the close began; nothing changed"; an unknown lane is 404 `not-found`,
  naming each lane as `id (branch)`, `id (no branch)` when the view has none. Writes to `out`
  share one mutex, and an empty removed or kept list reads "nothing". Alternative: 400 or 410 for
  the cancellation. Why: 503 says the server did not act, which is the truth here, and the
  command reads the code, not the status.
- 2026-10-24, group 1: in `cli_test.go` the manager finds no tmux server, so a close forgets a
  registered lane; whether claude runs is the hub's view, set directly. "No stop" is asserted as
  the registry record untouched (a stop starts by marking it `stop`) plus no `git worktree remove`
  and no `git update-ref`. Alternative: emulate a live tmux session through `Exec`. Why: the
  handler decides from the view; emulating send-keys, `claude agents` and the wait for death
  would test the manager's stop, which its own tests already cover.
- 2026-10-24, group 1: `TestCloseIfAsksHeldUnderTheLock` sleeps `testwait.Scale(100ms)` while
  the plan holds the lock, and only so the mutant (held asked before the lock) loses the race. The
  correct code passes deterministically.
- 2026-10-24, group 1, review round 1: the socket route's `held` also repeats the
  merged-or-exited rule on the fresh view, sharing `unmergedLive` with the check made before the
  plan. A lane that read exited when asked and runs again by the time the lock is held is refused
  with the not-merged words, but under `CloseIf`'s code `held`, not `not-merged`. Alternative: a
  second callback, or a code `CloseIf` takes as a parameter. Why: the words are what the owner
  reads; the code changes only which of two refusals the command reports, and both exit 1.
- 2026-10-24, group 1, review round 1: a lane whose own session has `HookStatus` busy or waiting
  from a hook heard after the current reading's poll *began* (`agentsPollStart`) is held as
  "claude is working" or "claude is waiting on you", after the reading check and before the
  background work. Alternative: compare with the apply time `agentsOKAt`. Why: the poll can read
  the session before a hook that arrives during it, so the start holds in that case and the
  apply time would release. `TestCloseHeldByAHookSinceTheReading` fails with `agentsOKAt`. A
  `PreToolUse` sets no status: it holds only while the turn's prompt left `HookStatus` busy.
- 2026-10-24, group 1, review round 1: citations corrected to the scenario whose GIVEN each
  test sets up: the plan's merged answer, and the CLI's exit re-read, cite LANECLOSE-2-S4;
  `CloseIf`'s held and the CLI's verdict re-read cite 2-S1; one session only cites 2-S3.
  `TestCLICloseCancelledBeforeItBegins` keeps LANECLOSE-4-S1, as task 1.5 directs. It is
  4-S1's other side: a close not yet begun when the request goes changes nothing.
- 2026-10-24, group 1, review round 2: two more holds, after the hook-since-the-reading rule and
  before the background work. One is a turn whose `Stop` was not heard (`stall.unreported`):
  `CloseUnheardWords`. The other is a kept `Stop` that is `!reported()` (no `background_tasks`):
  `CloseNotReportedWords`. Each says "what it left running is not known". The alternative was
  closeWork alone, which reads a missing list as an empty one. That breaks D2's "a failed read
  keeps more", and the stall's `lastWaitedOn` already treats both marks as not known. A session
  with no `Stop` heard at all (no kept stop, not unreported) is still not held by these two.
  Task 1.6's reading does not ask for that.
- 2026-10-24, group 1, review round 2: auto-close's lock re-read is tested in the panel package
  on a hand-built `Runtime` (`autoclose_test.go`): a stub `LaneManager` whose `git fetch`
  changes the hub, and no tmux. The alternative was the integration harness, which runs a real
  panel and tmux and has no hook into the plan's middle. The test fails with `held` nil and with
  `Close`.
- 2026-10-25, owner: D2's background work includes the last Stop's session_crons and wakeups (STALL-1's wakeupDue)
- 2026-10-25, group 1, review round 3: the wakeup hold reads the verdict's kept `Stop` with
  `cronsDue`, the rule `wakeupDue` now calls. It comes after the tasks, worded "its turn ended with
  a wakeup scheduled (a cron or a one-shot still due)". Alternative: call `wakeupDue`. Why: that
  reads only the stall's copy, which `s.end` drops; the verdict's copy outlives it (task 1.4).
- 2026-10-25, group 1, review round 3: a restart is seen by the verdict on its own record of the
  process (`closePID`, `closeStarted`, kept past `s.end`). Alternative: `stallListed`'s `was`
  check. Why: `was` is nil after `s.end`, the usual restart path, so it never compares the two
  processes. A listing naming another process sets `closeSince` to the new start, or to now when
  the start is not listed. It drops a close `Stop` heard before `closeSince`. An unheard turn
  holds only while its prompt (`promptAt`) is not before `closeSince`. The stall's
  `s.stall.unreported` is left as the stall keeps it. Alternative: clear it. Why: task 1.4 keeps
  the stall's own reading unchanged.
- 2026-10-25, group 1, review round 3: "no turn since the panel started" holds when there is no
  kept `Stop` and the process may have run before the panel. When its listed start is known, it
  decides: a start before `m.startedAt` holds, a later one does not, even at the first poll. With
  no start listed, being listed at the first poll holds. Alternative: either sign alone holds.
  Why: a known start is exact, and a claude started between the panel's start and its first poll
  had every turn heard. Fixtures whose claude starts after the panel (`closeRig`, `newCLIRig`)
  now begin with an empty first poll.
- 2026-10-25, group 1, review round 3: `leases(now, registered)`. The verdict binds a registered
  lane's id whether or not its tmux session runs; the stall's two callers pass false and are
  unchanged. `ApplyAgentsTimed` takes the iteration's start (`iterStart`); a zero start falls back
  to now less the duration. Auto-close words a refusal under the lock with the verdict's own
  words, with no "Close failed:" prefix. The unknown-id refusal uses design.md's words, and
  "this project has no lanes" when the project has none.
- 2026-10-25, group 1, review round 4: this replaces round 3's "or to now when the start is not
  listed". A listing naming another process drops what came before only when the new start is
  listed. Then `closeSince` is that start, and a close `Stop` or own prompt from before it no
  longer holds. With no start listed, nothing is dropped and `closeSince` stays as it was. The
  kept `Stop` and prompt are read as the new process's: they can only hold more, and only until
  its next `Stop` replaces them. Alternative: a "start not known" verdict that holds until a
  `Stop` heard after the listing. Why: keeping is the same rule as "cannot tell another process"
  (a PID or start of 0), needs no new field or words, and never releases. The cost is that the
  old process's work holds the lane until the new one ends a turn. Cases checked:
  - a new PID with a known start (before or after the panel, chained after an unknown one);
  - a new PID with no start;
  - the same PID, or a PID of 0;
  - a `Stop` or prompt before or after the listing;
  - a session that ended, then was re-listed;
  - the first listing ever.
  `TestCloseHeldByARestartsTurnBeforeItsListing` runs the failing sequence in two shapes:
  restarted between two polls, and re-listed after a poll without it. The stall's `end` drops its
  copy only in the first shape, because `was` is nil after a poll without it.
- 2026-10-25, group 1, review round 4: a close's re-check under the lane lock reads one lane, not
  a whole `Snapshot`. It uses `Model.LaneView` and `LaneCloseHeld` through `Hub.Lane` and
  `Hub.LaneCloseHeld`. `View.LaneCloseHeld` is removed. The verdict reads the leases over the lane
  views it already built (`leasesIn`). `leases(now, registered)` stays as the stall's entry point.
  Alternative: keep `Hub.View()`. Why: the re-check runs under the lane lock and the hub's lock,
  and needs only the lanes and the leases. `session.promptAt` is now `ownPromptAt`, commented
  against `LastPromptAt`, which also takes a teammate's prompt.
- 2026-10-25, group 1, review round 5: a kept `Stop` from before this panel started is no turn
  heard. A Stop restored from stall.json by `ApplyStallState` and `restoreKeep` counts as no
  `Stop`. "No turn of it has ended since the panel started" holds while the process may have run
  before the panel: `(st == nil || st.at.Before(m.startedAt)) && liveBeforePanel`. The hold is
  needed because a whole turn taken while the panel was down leaves no prompt and no unheard
  mark. Alternative: trust the restored Stop's lists. Why: they would release a lane whose lost
  turn started a shell. The other readers of the kept Stop were checked:
  - The not-reported hold comes first and only holds.
  - `closeWork` and `cronsDue` come after this hold. There, a pre-panel Stop can only be read
    for a process that started after the panel, and this panel heard all of that process's
    turns.
- 2026-10-25, group 1, review round 6: a close never acts on a stale exit. `CloseIf` gives held
  what tmux says of the lane under the lane lock, as a `types.LiveLane{Read, Running, Dead}` from
  the `list-panes` its `closeTarget` already runs. `state.Model.LaneView`, and through it
  `LaneCloseHeld`, `Hub.Lane`, the CLI's held and auto-close's held, may only add a hold from it:
  - tmux not answering holds (`CloseTmuxUnreadWords`);
  - a claude tmux shows running, that the last tmux poll showed exited (`Dead`) or gone (not
    `Running`), holds (`CloseRunsAgainWords`): a Restart or a Resume that ran under the lock
    first. The panel's readings are of the process before.
  - Otherwise the view's verdict and its `Dead` stand. A claude that tmux shows exited or gone
    runs nothing a close could stop, so a stale "running" reading only holds more, and the next
    poll decides.
  Cases, the same for auto-close and the CLI. The view's facts are running, dead or gone; tmux
  now is running, dead, gone or not answering:
  - tmux not answering: held, whatever the view shows.
  - view dead or gone, tmux running: held (the finding's failure).
  - view running, tmux running: the view's verdict.
  - tmux dead or gone: the view's verdict. It can only hold more than tmux would, and it reads
    no claude that still runs.
  `unmergedLive` keeps reading the view's `Dead`. The one stale exit that would waive the merged
  rule (view dead, tmux running) is held before it.
  Alternative: replace the view's `Running` and `Dead` with tmux's. Why not: design.md D2 says
  "a lane whose tmux session is gone is not exempt", so a view-dead lane that tmux shows gone
  would turn from exempt to refused. That breaks
  `TestCLICloseRefusesUnmergedUnlessExited`'s exited close, which the round's prompt keeps
  unchanged. Replacing also adds nothing to safety.
  Not covered: a Restart of a lane that was running in both readings. tmux carries no pane PID,
  so the respawn cannot be seen. That is not an exit, and the new claude has run no turn.
- 2026-10-25, group 2: the command's client is `install.CloseLane(ctx, socket, project, id,
  lanes.CloseRequest)` in a new `install/laneclose.go`, answering `LaneCloseAnswer{Plan, Result}`;
  a refusal is a `*lanes.LaneError` with the route's status, code and message (a body that is not
  the route's JSON, such as the mux's own 404 or 405, gives the trimmed body and no code).
  Alternative: two functions (plan, close), or a client local to `cmd`. Why not: task 2.4 wants
  one function the panel's tests drive, and `panel` cannot import `cmd`. The `LaneError` lets
  group 3 print the route's words and choose its exit status.
- 2026-10-25, group 2: the socket's failures print one line to `o.Out`, "no command socket, so
  `clauductor lane close` cannot reach this panel: <why>". A path too long for `sun_path` adds its
  length and the limit (`len(syscall.RawSockaddrUnix{}.Path)`), and a file at the path that is not
  a socket is left alone and named. A socket server that stops with an error other than
  `ErrServerClosed` prints "the command socket stopped serving" and the panel serves on.
  Alternative: fail the start. Why not: D1 says a panel that cannot make its socket starts
  anyway ([LANECLOSE-3-S4]).
- 2026-10-25, group 2: the socket is listened on before the owner value is built (so `Socket`
  names a path that answers), but served only after the Server is wired, beside the TCP serve
  goroutines. A connection that comes in meanwhile waits in the listen backlog. `Shutdown` runs
  after the TCP server's, and the deferred `Close` unlinks the file before the machine lock is
  released. Between `net.Listen` and `Chmod 0600` the socket has the umask's mode, but it sits in
  the directory `EnsurePrivateDir` has just made 0700, so no other user can reach it then.
  Alternative: set the process umask around the listen. Why not: the umask is process-wide, and
  the panel's other goroutines create files at the same time.
- 2026-10-25, group 2 (round 7's low F1): in `TestAutoCloseReadsTheVerdictAgainUnderTheLock`,
  the tmux stub now records a stop (`send-keys` or `kill-session`), and from then on `list-panes`
  shows no lane. A close the guard did not hold now ends at once, instead of waiting out the stop
  timeout. With `CloseIf`'s held check turned off, all three subtests failed in about 0.3 s, each
  with its own assertion, not a timeout.
- 2026-10-25, group 2: task 2.3 asks for a lane program that is "a long `sleep`". The test's
  program is a loop that waits for a file named after the lane's directory, so that
  [LANECLOSE-2-S4]'s exited case can end one claude. To the panel and tmux it is the same: a
  pane command that runs until told. The Needs-you cases of auto-close each get a lane of their
  own (`bgauto`, `unread`), so no CLI close races auto-close. Their pull requests are listed open
  first, then merged, because auto-close checks a lane again only when its PR leaves the open
  list. [LANECLOSE-4-S1] runs in the same panel. The `worktree_teardown` marks every lane it
  runs for, and sleeps only for lane `slow`, so one panel serves every case.
- 2026-10-25, group 2: the controls were checked by failing them, then restored. A socket mode
  of 0644 fails the mode assertion. `MkdirAll` in place of `EnsurePrivateDir` fails the
  directory's 0700. Not removing the stale socket leaves nothing answering and no socket in
  `owner.json`. `CloseIf` under the request's context in place of the panel's fails
  [LANECLOSE-4-S1] (the worktree stays).
- 2026-10-25, group 2 review round 1, the stale socket's precondition (finding 2): the test
  plants it with `deadSocketFile`, a socket that is bound and never listened on. `bind` makes
  the file. A child a parallel test forks may inherit the descriptor, but it cannot accept, so
  the precondition still asserts that a dial is refused, which is what a killed panel leaves.
  Alternatives: plant it from a child process that exits, wait until the dial is refused, or
  drop the dial check. Why not: a child process is a helper leakcheck must then track, a wait
  only shortens the window, and dropping the check would stop proving that nothing answered
  before the panel. testbin, leakcheck and testwait had no helper for this. Seven runs of the
  group's tests together passed.
- 2026-10-25, group 2 review round 1 (finding 5): `CloseLane` tells apart an error before the
  connection was made ("the panel's socket … does not answer") and one after the request was
  sent ("the panel's answer did not arrive (…); a close it had begun, it finishes: see the lane
  on the panel's page"). The dial records whether it connected. Alternative: classify by error
  type (`context.Canceled`, `net.OpError`). Why not: a cancel can come on either side of the
  dial, and only "connected" says whether the panel may have begun (D4). Redirects are not
  followed (finding 6): a 3xx comes back as a `LaneError` with its status, and the request is
  sent once.
- 2026-10-25, group 2 review round 1 (finding 4): lane `slow`'s teardown waits for a release
  file. The test creates it only after the cancelled command has returned, so the window no
  longer depends on how fast the machine is. The wait is capped at 60 s, and a cleanup creates
  the file too, so a failed test leaves no teardown running for long. Uninstall now removes
  `cli.sock` with the rest of the panel's files (finding 7), and
  `TestUninstallRemovesPanelFilesButKeepsLiveRegistries` lists it.
- 2026-10-25, group 2 review round 1: each control was broken on purpose, then restored, and a
  test failed on its own assertion. The dirty re-check's reason dropped in `CloseIf` fails
  [LANECLOSE-1-S3] (the Kept line read "the confirmation did not include it", which the old
  assertion passed). A regular file removed like a socket fails
  `TestAPanelLeavesAFileAtItsSocketPathThatIsNotASocket`. Every error worded "does not answer"
  fails `TestCloseLaneTellsNoAnswerFromAnAnswerCutOff`. Redirects followed fails
  `TestCloseLaneFollowsNoRedirect` (ten requests, the GETs among them). `cli.sock` left out of
  uninstall's list fails the uninstall test. The stale socket not removed, and `CloseIf` under
  the request's context, still fail as before.
- 2026-10-25, group 3: the command's output. The plan's first line is `lane <id> on <branch>`,
  or `lane <id>, its branch not known` for a plan with none (only an exited lane gets that far);
  then `Removes:`, `Keeps:` with each line as `  - <line>`, an empty list printed not at all (as
  the page shows none), and the notes as plain lines. With `--yes` the plan is printed first, then
  `Closed lane <id>.` (the page's own title), `Removed:` and `Kept:` the same way. Alternative:
  no title line, or print the lists only. Why: D3 fixes the first line and the lists, not their
  layout; the title says the close happened when both lists are empty.
- 2026-10-25, group 3: a refusal is returned as the panel's `*lanes.LaneError`, so `main` prints
  `Error: <the panel's message>` to stderr and exits 1, as every other command's error does. The
  message itself is unchanged. Alternative: print the bare message and exit 1 by hand. Why not:
  it would need an exit path of its own beside `main`'s, for a prefix.
- 2026-10-25, group 3: `install.MainWorktree(ctx, run, path)` returns `(main, top string, err
  error)`, its error a "not-git" `*Refusal` with `mainRoot`'s own words, so `mainRoot` calls it
  and keeps only its linked-worktree check. Alternative: return the main worktree alone. Why not:
  `mainRoot` needs the top to refuse a linked worktree, and computing it twice would run git twice.
- 2026-10-25, group 3: a live panel whose socket does not answer is said by `CloseLane`'s own
  not-answering error, which now ends "nothing was asked of it (`clauductor panel` starts a
  panel)" (the design's "a socket that does not answer … naming `clauductor panel`"). Alternative:
  a typed error the command wraps. Why not: `CloseLane` is the command's one client, and the
  words are true wherever it is used. A cut-off dry run now says "a dry run changes nothing, so ask
  again" instead of claiming a begun close (group 2's low).
- 2026-10-25, group 3: the stub panel's `[LANECLOSE-3-S2]` socket is one still served, not a dead
  file, and the test counts its connections (`ConnState`): a dead file could not show a connection
  attempt, a served one shows it. With `RunningPanel` bypassed (owner.json trusted), the test
  failed connecting twice. Other controls broken on purpose, each failing its own assertion, then
  restored: the self check (the process check called, and the subprocess not refusing); matching
  by the worktree's top instead of the main checkout (the linked worktree refused); the served
  check (delta's close asked); the consent built from `worktree` for `branch` (the stub's
  `DisallowUnknownFields` decode and its consent assertion); the empty-socket check (refused for
  the project instead); the notes dropped (`[LANECLOSE-1-S1]`'s output).
- 2026-10-25, group 3 (group 2's low 1): `TestThePanelListensOnTheDefaultSocketPath` starts a
  panel with no `CLISocket` and a home made by `os.MkdirTemp("/tmp", "h")`: under leakcheck the
  `TMPDIR` is long enough (114 bytes with the home) to overflow macOS's 104-byte `sun_path`.
  Alternative: `os.MkdirTemp("", …)` as `shortSocket` does. Why not: that failed on this Mac.
  With run.go's default path changed, it failed on owner.json's socket, the file and the dial.
- 2026-10-25, group 4 (group 3's low 4): `runLaneClose` takes `homeDir func() (string, error)`
  and calls it after the self refusal; `RunE` passes `os.UserHomeDir`. Alternative: a second
  self check in `RunE` before the home lookup. Why not: two copies of the one ordering rule, and
  the in-process tests would not hold the order. Broken on purpose (the home read first):
  `TestLaneCloseRefusesItsOwnLaneWithNoHome` failed with `$HOME is not defined`.
- 2026-10-25, group 4 (group 3's low 2): `shortHome` tries `os.MkdirTemp("", "h")`, falls back to
  `/tmp` only when `CLISocketPath` would not fit `len(syscall.RawSockaddrUnix{}.Path)` (104 on
  macOS, 108 on Linux; the NUL counts), and skips naming both reasons when `/tmp` gives no home or
  is too long too. A first `MkdirTemp("")` that errors fails the test rather than falling back:
  the brief allows `/tmp` only for the length, and every `t.TempDir()` there would fail as well.
  Exercised: a 90-byte `TMPDIR` took the `/tmp` branch and passed; the same inside the sandbox
  (no `/tmp`) skipped with both reasons.
- 2026-10-25, group 4 (group 3's lows 1 and 3): `laneRun` is set to `gitFreeRunner` in
  `newLaneWorld`, so every in-process case gets it. Broken on purpose (the override removed),
  with `GIT_DIR` exported, the cases answered for the outer repository and failed; restored,
  they pass with it exported. The stub's consent check now errors on a close whose dry answer
  holds no `lanes.ClosePlan`: with the plan stored as a pointer (which the old check skipped
  silently), the three `--yes` tests failed.
- 2026-10-25, group 4 (task 4.1): the command's docs are a `####` under *Close lane* with their
  own refusals table, in the panel's words, the two lock-time refusals (task 4.4) among them, and
  the retry rule of task 4.3; the socket is a *Security model* bullet and a line in *What the
  panel writes to disk*. Alternative: a `###` section beside *Close a lane when its PR merges*.
  Why not: the task names it a subsection of *Close lane*. The log line's place is given as
  `~/.clauductor/panel/logs/` (`install.LogDir`), where the docs already name the launchd logs.
- 2026-10-25, group 4 (task 4.2): D5's sentence is appended to the *Close the lanes* rule as one
  sentence, and names the Bash tool's `timeout: 600000` for the 10-minute bound. The final
  message carries the plan's **Keeps** lines, as D5 words it, not the result's **Kept**. Review
  round 1 of group 4 asked for the result's **Kept** instead; not applied, since it changes D5's
  words (design.md, D5), and returned to the owner as disputed. A reason to keep D5's: a `--yes`
  that timed out has no result, while the plan's **Keeps** came from the dry run before it.
- 2026-10-25, group 4 review round 1: the first dry run runs under the same 10-minute timeout as
  `--yes`, and one that timed out is named for the owner in the final message (nothing changed).
  D5 sets no timeout for it; the alternative, none, leaves a hung panel holding session-close.
- 2026-10-25, group 4 review round 1: `closeHeld`'s fixed words became constants
  (`CloseQueuesUnreadWords`, `CloseQueuesFailingWords`, `CloseNotReadWords`, `CloseWorkWords`,
  the last a prefix before ": " and the tasks), same values, so
  `TestEveryCloseWordIsDocumented` (`state/closeheld_docs_test.go`) finds every reason by parsing
  the package's non-test source for `Close*Words` constants and checks each in `docs/panel.md`.
  Alternative: a hand-kept list in the test. Why not: a new reason would not be on it. Left
  literal: the lease words (they name the queue), `claude is <status>` (statusWords' map) and
  `LaneGoneWords` (a func of the id); the refusals table names them, unchecked. The docs' table is
  now one row per reason in the code's words, with the panel-restart hold named as a change to
  PANEL-20's auto-close; the auto-close paragraph and the `lanes_auto_close` Doc point to it.
  The test reads `docs/panel.md`, so `scripts/ci/steps.sh` `race_select` now maps that file to
  `internal/panel/state` too (the `race-scope` check required it).
- 2026-10-25, group 4 review round 2: `TestEveryCloseWordIsDocumented` walks the authority, the
  reasons themselves: every `return` of `closeHeld` and `LaneCloseHeld` and every assignment to
  `CloseHeld` in `LaneView`, parsed from `closeheld.go`. Each must be a `Close*Words` constant or
  a string literal whose words are in `docs/panel.md`, or a form the test's `closeForms` names by
  its source with the docs words it must show (the lease, `claude is <status>`, the background
  work prefix, `LaneGoneWords`, and the pass-throughs). Alternative: promote the remaining
  literals to constants, or narrow the comment. Why not: the walk catches a new literal and a new
  form alike, with no rule a future edit must remember. The route's branch and cancelled words
  (`web/cli.go`) stay outside it, which the test's comment says by naming the functions it walks.
  Auto-close's scope is stated once in the same words in three places: every reason in the table
  but its last two (the branch, read on auto-close's own stricter terms, and a cancelled request).
- 2026-10-25, group 4 (task 4.4): `TestCLICloseOfAnExitedLaneRestartedSinceThePoll` covers the
  restart at the route; tmux not answering is covered in `TestCloseIfTakesTheExitFromTmuxNow`
  alone, both cited `[LANECLOSE-2-S4]`. No new test: the task asks for the citations only.

## 1. The close verdict and the socket's route (D2, D4)
- [x] 1.1 `framework/internal/panel/state/model.go` and `state/stall.go`: `TermLaneView.CloseHeld`
  (empty when the lane's work has ended, otherwise why not), filled by `TerminalViews` in a second
  pass from one `m.leases` call once the views are built; the first pass becomes an unexported
  builder that `leaseBinder` calls in place of `TerminalViews`, so the two do not recurse, and
  `leases` also returns each held lane's queue id in a new map (its callers at `stall.go:620` and
  in `judgeStalls` take the new value and ignore it). The verdict, in D2's order: a lease held or
  waited for holds the lane whatever claude is doing; otherwise `!Running||Dead` may close;
  otherwise a lane whose own session (`tv.SessionID` in `m.sessions`) has `HookStatus` "ended"
  and no current reading newer than its end (`session.endedAt`, new, set by `s.end` and by the
  `SessionEnd` hook; held unless `m.agentReading` is non-nil and `m.agentsOKAt` is after
  `endedAt`) is held as "its session ended while claude still runs (a /clear?): close it from the page",
  checked before the reading because `s.end` clears `Agent`; otherwise E3's rule and a current `claude agents` reading of `idle` for the lane's session
  (`m.agentReading` non-nil), else held as "claude has not been read yet"; then the lane's own
  session (`tv.SessionID`, looked up in `m.sessions`) checked with `sessionWorking` and with its last `Stop`'s tasks (a `taskOpen` type whatever its
  status, as `turnWork` reads it; a `taskWorking` type when `inFlight`, a subagent not marked
  ended), with no `unheard` limit on the tasks. No other session in the worktree is read. `statusWords` moves to `state` from
  `autoclose.go`, and `pollAutoClose` reads `CloseHeld` in place of its own rule and closes
  through `CloseIf` (task 1.2) with a `held` that reads `CloseHeld` from its hub. Tested in
  `framework/internal/panel/state/model_lanes_test.go`: a table over `busy`, `waiting`, `running`,
  `idle`, an exited program (`Dead`), a gone session (`!Running`), and each of `busy` and `idle`
  with `Approx`, checking the verdict and the words; then an `idle` lane whose session's last
  `Stop` lists, in turn, a running subagent, a pending workflow, a running shell, a teammate with
  an empty status, a subagent marked ended, a `dream` task, and nothing, the first four held naming
  the task and the last three not; a running subagent still held after `stallUnheard` × the stall
  time has passed; an `idle` lane bound to a queue's live holder, then to a waiter, held naming the
  lease, and not once the lease is gone; a `Dead` lane bound to a holder still held; a lane whose
  session the panel has not heard from held as "starting"; a lane whose session is known only from
  a `PreToolUse` hook, with no agents reading (so `sessionStatus` answers `idle`, not approximate),
  held as "claude has not been read yet", and not once an agents reading of `idle` arrives; a
  running lane whose record's session `ApplyAgents` has ended (dropped from the reading, as after
  a `/clear`) held as "its session ended while claude still runs (a /clear?): close it from the
  page", and as "starting" once `forgetSessions` drops it, but not once a newer agents reading
  lists that session again with `idle`; a lane whose session read `idle` and then posted a
  `SessionEnd` (reason `clear`) to `/hook` held with the same words before the next agents
  reading, its old `idle` reading still current; and an `idle` lane whose own session is
  at rest while another session
  in the same worktree is working, not held. `TestAutoCloseOnMerge` and the stall tests pass
  unchanged. Citing [LANECLOSE-2-S1], [LANECLOSE-2-S3].
- [x] 1.2 `framework/internal/panel/lanes/close.go`: `ClosePlan.Merged` and `ClosePlan.Unmerged`,
  `branchVerdict`'s `ok` and `why`, set whenever the plan has a branch, even when the worktree
  stays. Tested in `framework/internal/panel/lanes/close_test.go`: a plan for a lane on a merged
  branch carries the `ok` reason and no `Unmerged`, one on an unmerged branch the `why` and no
  `Merged`, one on a dirty worktree whose branch merged still carries `Merged`, and the existing
  plans' other fields are unchanged. `LaneManager.CloseIf(ctx, id, consent, held func() string)`:
  `held` called once `m.mu` is held and before `stopAndForgetLocked`, an answer refused 409
  (`held`) with nothing changed; `Close` is `CloseIf` with a nil `held`. Tested in the same file:
  a `held` that answers leaves the registry, the tmux session, the worktree and the branch as
  they were, and the runner records no stop; a `held` that sees a change made while another call
  held the lock (a runner stub that blocks under `m.mu`, then the answer flipped before it is
  released) refuses; a nil `held` closes as `Close` did. Citing [LANECLOSE-2-S1],
  [LANECLOSE-2-S4].
- [x] 1.3 `framework/internal/panel/web/cli.go` (new): `(*Server).CLIHandler(ctx context.Context,
  out io.Writer) http.Handler`, a mux of its own serving only `POST
  /api/p/{project}/lanes/{id}/close` through `withProject` and `decodeStrict`, with no cookie and
  no `Origin` check (D1: the socket's mode is the credential). Before the manager is asked: a lane
  the project's `Hub.View()` does not list is refused 404 with the view's lanes as `id (branch)`;
  one whose `CloseHeld` is set, 409 with it. Then the plan, under the request's context (D4); a lane whose view is not `Dead` and
  whose plan has an empty `Merged` is refused 409 before anything changes: "its branch is not
  merged: <`Unmerged`>", or "its branch is not known" when the plan has no branch. Then the dry
  run as `closeLane` answers it; or, for a close, the request's context checked once more (a
  cancelled one answers with nothing changed), then `CloseIf` with a `held` that reads `CloseHeld`
  from a fresh `Hub.View()` (task 1.2), under
  the handler's `ctx` (the panel's), with no added deadline, and one line to `out` per close:
  `lane <id> (<project>) closed by clauductor lane close: removed …; kept …`. Tested in
  `framework/internal/panel/web/cli_test.go` (new) through `httptest`, with the project's lanes
  from `testLaneManager` and its view set on the hub: any other path or method, `/api/state` and
  `/lanes/{id}/stop` among them, is 404 or 405; a lane whose view sets `CloseHeld` is refused 409
  with its words, and the manager's runner records no call, for a dry run as for a close; an idle lane on a branch not merged
  is refused 409 naming it, and the runner records no removal, while the same lane with its
  program exited is closed with the branch kept; an idle lane whose plan has no branch (its
  worktree gone) is refused "its branch is not known"; a registered lane with no tmux session on
  an unmerged branch is refused, and on a merged one is closed; a lane whose view is clear when
  asked and sets `CloseHeld` before `Close` holds the lock (the runner's `git fetch` stub sets the
  hub's view) is refused 409 with no removal; an id the view does not list is
  refused with its lanes and their branches; a close whose request context is cancelled after
  the plan and before `CloseIf` (the runner's last plan call cancels it) changes nothing, the
  runner recording no stop and no removal; a close whose request context is cancelled before `Close` returns
  still reaches the manager under a live context. Citing [LANECLOSE-3-S1], [LANECLOSE-2-S1],
  [LANECLOSE-2-S3], [LANECLOSE-2-S4], [LANECLOSE-1-S4], [LANECLOSE-4-S1].
- [x] 1.4 Review round 3's two verdict findings (the owner's decision of 2026-10-24), in
  `state/model.go` and `state/stall.go`. **A poll that straddles a `/clear`:** an ended session's
  hold lifts only on a reading whose poll *started* after `endedAt`, not one merely applied after
  it. `m.agentsOKAt` is the apply time (`model.go:703`), so `ApplyAgentsTimed` also stores the
  poll's start (`now - dur`) beside it, and task 1.1's rule compares `endedAt` with that start. A
  poll can run up to `agentsPollCap` (30 s, `state/blocked.go:224`). **A re-listed session keeps
  its last `Stop`:** `s.end` drops the stall's saved `Stop` (`stall.go:379-381`) and
  `stallListed` cannot restore it once `stallPolled` has cleared it (`stall.go:338-357`), so the
  tasks task 1.1's verdict reads survive `s.end`, kept on the session until a newer `Stop`
  replaces them or `forgetSessions` drops the session. The stall's own reading is unchanged.
  Tested in `framework/internal/panel/state/model_lanes_test.go`: an `idle` lane whose session
  posts `SessionEnd` (reason `clear`) while a poll is under way, that poll then applied listing
  the old session `idle`, is still held with the ended words, and released by the next poll
  started after the end; a session whose last `Stop` lists a running subagent, then ended, then
  listed `idle` again, is held naming the subagent, and released once a later `Stop` lists none;
  the stall tests pass unchanged. Confirm each test fails with the change reverted. Citing
  [LANECLOSE-2-S1], [LANECLOSE-2-S3].
- [x] 1.5 Review round 3's test finding: task 1.3's cancel test must fail without the check. Its
  runner stub cancels the request's context on the last plan call **and still returns the plan's
  normal output**, so the plan stays merged and the unmerged refusal cannot answer in its place;
  the test asserts the cancellation's answer (not a 409 naming the branch), and no stop and no
  removal. Run it once with the `r.Context()` check removed and see it fail. Citing
  [LANECLOSE-4-S1].
- [x] 1.6 merge-pr's review of the approved proposal (2026-10-24). **An unread lease source holds
  every lane**, by D2's "a failed read keeps more": `m.leases` reads `m.v2.queues`, empty until
  the first good read and kept stale by `ApplyQueues` on an error (`model.go:1709-1716`), so while
  `len(m.cfg.Queues) > 0` and either `!m.stall.queuesRead` or `!m.v2.queuesSrc.OK`, `CloseHeld`
  is "the queues' leases have not been read", or "the queues' leases are not answering", checked
  first, before the `!Running||Dead` exemption, as the stall's `workJudged` (`stall.go:1097`)
  already refuses to judge then. **Auto-close's words:** every status word `CloseHeld` gives
  carries auto-close's `"claude is "` prefix ("claude is working", "claude is waiting on you",
  "claude is starting", …), as LANECLOSE-2-S1 says ("each in auto-close's words") and
  `TestAutoCloseOnMerge` asserts (`autoclose_integration_test.go:77`). Tested in
  `model_lanes_test.go`: a `Dead` lane and an `idle` lane, each held before the first
  `ApplyQueues` and after a failed one, and released after a good read that binds no lease to
  them; confirm the test fails with the check removed. (Tasks 1.1 and 2.3 cover the scenarios.)

## 2. The socket in the panel's life (D1, D4)
- [x] 2.1 `framework/internal/panel/install/singleton.go`: `PanelOwner.Socket` (`json:"socket,
  omitempty"`), and `CLISocketPath(home)`, `~/.clauductor/panel/cli.sock`. Tested in
  `framework/internal/panel/singleton_test.go`: an `owner.json` naming a socket comes back from
  `RunningPanel` with it, and one from before this change (no field) comes back with it empty.
- [x] 2.2 `framework/internal/panel/run.go` and `live.go`: `Options.CLISocket` (default
  `install.CLISocketPath(o.Home)`). After the machine lock is held and before the owner value at
  `run.go:354` is built: the socket's directory made private (`config.EnsurePrivateDir`); a socket
  file already at the path removed (any other kind of file left, and the socket skipped);
  `net.Listen("unix", …)`, then mode 0600; its path set in the owner value as `Socket`, so
  `ClaimPanelFiles` writes it and `liveSet.writeOwner` keeps it on every rewrite. It is served by
  an `http.Server` of its own over `CLIHandler(ctx, o.Out)`, with `BaseContext` the panel's
  context. A path too long for `sun_path` or a listen error prints why to `o.Out` and leaves
  `Socket` empty, and the panel serves on. At exit the listener closes, which unlinks the file,
  before the machine lock is released. Tested in
  `framework/internal/panel/lane_close_cli_integration_test.go` (new), through `startPanelWith`
  with `CLISocket` set to a short path under `os.MkdirTemp("", "lc")` (E10), and an `http.Client`
  dialing it: the mode, the directory's mode and `owner.json`'s path, and the path still there
  after a live `panel add` rewrites `owner.json`; another route on the socket refused; the TCP
  close route still refusing a request with no cookie, then one with no `Origin`; a stale socket
  file at the path replaced, and the file gone after `stop`; a 200-byte path leaving the page
  served, the line printed and `Socket` empty. Citing [LANECLOSE-3-S1], [LANECLOSE-3-S3],
  [LANECLOSE-3-S4].
- [x] 2.3 The close through the socket, in the same test file, on a throwaway tmux socket
  (`throwawaySocket`) with `TrustConfig` and `stallWorld`'s runner (`stall_integration_test.go`)
  faking `claude agents`: a lane whose program is a long `sleep`, its tmux session still there,
  made idle by an `agent(…, "idle", "")` reading for its session, its branch at `main`'s tip. A
  dry run changes nothing (the registry, the tmux session, the worktree and the branch read before
  and after); `{worktree: true, branch: true}` removes the lane from the registry, its tmux
  session, the worktree and the branch, answers the **Removed** lines and prints the output line;
  the same on a worktree holding an untracked file keeps it and the branch, with **Kept** saying
  why; the lane read `busy` is refused and its tmux session is still there; the lane read `idle`
  after a `Stop` posted to `/hook` listing a running subagent (as `stall_integration_test.go`
  posts one) is refused naming it, closes once a later `Stop` lists none, and, with
  `lanes_auto_close` `on_merge` and its PR merged, is not closed by auto-close but asked about in
  Needs you; the same lane with `on_merge`, its PR merged, heard of only from a `PreToolUse`
  posted to `/hook` and with no `claude agents` reading for its session, is not closed by
  auto-close but asked about in Needs you with "claude has not been read yet"; the lane read `idle` on a branch with a commit `main` lacks is refused, "its branch
  is not merged", with the registry, its tmux session, the worktree and the branch unchanged, and
  once its program has exited (the pane's command ended, `remain-on-exit` holding it) is closed
  with the branch kept and **Kept** saying it is not merged; a `worktree_teardown` that touches a
  marker and sleeps, with the client's request cancelled once the marker exists, still ends with
  the worktree removed, the branch deleted and the line printed. Citing [LANECLOSE-1-S1],
  [LANECLOSE-1-S2], [LANECLOSE-1-S3], [LANECLOSE-2-S1], [LANECLOSE-2-S3], [LANECLOSE-2-S4],
  [LANECLOSE-4-S1].
- [x] 2.4 merge-pr's review of the approved proposal (2026-10-24): **the command's client is
  tested against the real route.** The socket client the command uses (building the close body,
  decoding `{"ok","plan"}`, `{"ok","result"}` and `writeLaneErr`'s body) is one exported function
  in `framework/internal/panel/install` (the package the command already imports for
  `RunningPanel`), since `panel` cannot import `cmd`. Its consent is `lanes.CloseRequest`: the
  plan's `deleteBranch` (`close.go:47`) is sent as `branch` (`close.go:59`), which the route's
  `decodeStrict` would refuse under any other name. Task 2.3 calls that function, not a raw
  `http.Client`, for the dry run, the `--yes` close and the busy refusal against the real socket.
  Citing [LANECLOSE-1-S1], [LANECLOSE-1-S2], [LANECLOSE-2-S1].

## 3. The command (D2, D3)
- [x] 3.1 `framework/internal/cmd/lane.go` (new): `clauductor lane close <id> [--yes]`, with no
  `--project` (D7), registered on the root command as `panel` is (`panel.go`). An id equal to
  `$CLAUDUCTOR_LANE` refused before anything is read, in fixed words naming the page's Close lane,
  and auto-close where it is on for the lane's type. The panel first, from
  `install.RunningPanel`, with its `lease.ProcCheck` through a package variable a test replaces;
  no live panel, or an empty `Socket` (the `/healthz` fallback's owner among them), refused naming
  `clauductor panel`. Then the project from the current directory alone, resolved to its main
  checkout with a new exported `install.MainWorktree(ctx, run, path)` (`git rev-parse
  --show-toplevel`, then the first entry `signals.ReadWorktrees` lists, the lines `mainRoot`
  reads, which then calls it) and matched by root. Refused, naming `clauductor panel list`, when
  not registered or not among the running panel's `Projects`. Then the dry run: the plan's first
  line, `lane <id> on <branch>`, then its **Removes**, **Keeps** and notes printed; with `--yes`,
  the close with the plan's `worktree` and `deleteBranch` as the consent, its **Removed** and
  **Kept** printed. A refusal's message printed as the panel wrote it, exit 1. Tested in
  `framework/internal/cmd/lane_test.go` (new), each case moving into its directory with `t.Chdir`,
  against a stub panel: a
  Unix-socket `httptest` server answering canned plans, results and refusals, with a pid file and
  an `owner.json` in a temporary home and the process check replaced. Citing [LANECLOSE-1-S1] (the
  plan printed, the stub asked for the dry run alone), [LANECLOSE-1-S2] (with `--yes`, the stub
  asked for the dry run then the close with the plan's offers, and the **Removed** lines printed),
  [LANECLOSE-1-S3] (a result with **Kept** lines printed with their reasons, exit 0),
  [LANECLOSE-1-S4] (the refusal's lanes and branches printed), [LANECLOSE-1-S5] (run from a linked worktree
  made with `git worktree add` and from a subdirectory, each asking about the main checkout's
  project; run from a repository no project names, refused with the stub recording no request),
  [LANECLOSE-2-S2] (the stub records no request, and no config is read: run from a directory in
  no project, the same refusal), [LANECLOSE-2-S3] (a 409 naming background work printed, exit 1),
  [LANECLOSE-2-S4] (a 409 naming the unmerged branch printed, exit 1), [LANECLOSE-2-S1] (a 409
  to the dry run printed with no plan, exit 1), [LANECLOSE-3-S2] (no live pid, a socket file present: no connection),
  [LANECLOSE-3-S4] (an `owner.json` with no socket, and a panel found by `/healthz` alone, both
  refused for the socket, not the project). `install`'s tests for `mainRoot` pass
  unchanged.
- [x] 3.2 merge-pr's review of the approved proposal (2026-10-24): `framework/internal/cmd/lane.go` closes through task
  2.4's client, and task 3.1's stub panel decodes the close body as `lanes.CloseRequest` with
  `DisallowUnknownFields`, asserting `branch` equals the plan's `deleteBranch` and `worktree` the
  plan's `worktree`. Citing [LANECLOSE-1-S2].

## 4. Docs and the production caller (D5, D7)
- [x] 4.1 `docs/panel.md`: under *Close lane*, a subsection *Close a lane from the command line*
  (the command, run from the project's checkout with no `--project`, plan first, `--yes`, the
  verdict, merged-or-exited, and the self refusal, the finish after an interrupt, where its line
  goes); under *Close a lane when its PR merges*, that background work in flight, a lease, no
  current `claude agents` reading yet, or a session ended while claude still runs (after a
  `/clear`, which the owner then closes from the page) holds it, and the same in `lanes_auto_close`'s `Doc` in `framework/internal/panel/config/fields.go`,
  with the files it generates refreshed (`go generate ./internal/panel/config`: the config table
  in `docs/panel.md` and the field's `description` in `docs/panel.schema.json`, both held by
  `TestGeneratedFilesAreCurrent`); and under *Security model* the socket (mode 0600, the one route, no token).
  `README.md`'s command table: a `clauductor lane close` row.
- [x] 4.2 `template/.claude/skills/session-close/SKILL.md`, the *Close the lanes* rule, D5's
  sentence (run from the repository; the lane's id tried from its worktree's folder name, else,
  when that id is unknown, from the refusal's `id (branch)` list; `--yes` only when the plan's
  first line names the merged branch, and a lane on another branch left to the owner, named;
  `--yes` under a 10-minute Bash timeout; after a call that timed out, D4's retry: the dry run once
  more under the same timeout, the lane named "closed or closing" with the panel's log line
  pointed to, else with its plan, else "outcome unknown"); `template/.claude/settings.json`'s `permissions.allow`, `Bash(clauductor lane close
  *)` (D7). Then `scripts/build-plugin.sh`, and `env -u CLAUDUCTOR_FRAMEWORK framework/clauductor
  update` from the repository root with the binary built from this checkout, its `Template:` line
  checked to name this worktree's `template/`. Commit only `.claude/skills/session-close/SKILL.md`
  and `.claude/settings.json` from it; any other drift it writes is OPS-119's: a tracked file
  restored with `git checkout --`, and a file it created (a `??` line in `git status
  --porcelain` that was not there before update ran) removed. An agent's sandbox may deny writing
  `.claude/settings.json` and `.claude/skills/`, so if update cannot write either, or stops
  part-way, the builder undoes what it wrote the same way, stops, and asks the owner to run that one command. Held by `TestCommittedPluginIsCurrent`, `checks/skills.sh` and
  `checks/settings.sh`.
- [x] 4.3 Review round 3's retry finding (the owner's decision of 2026-10-24): in task 4.2's
  sentence and task 4.1's docs, on the retry after a timeout **any** "no lane" answer, the
  manager's own `404 "no lane %q"` (`lanes/close.go:343-345`) among them, means "closed or
  closing", and is never a cue to try another id from the unknown-id list; that list is read only
  on a first call. Held by `TestCommittedPluginIsCurrent` and `checks/skills.sh`.
- [x] 4.4 Review round 7's spec finding (group 1, 2026-10-25): the spec delta states the lock-time
  rule, so it stays cited once archived. Under [LANECLOSE-2-S4], add an **AND** line: a lane the
  panel last read as exited or gone, whose tmux shows claude running when the close takes the lane
  lock (a Restart or Resume since), or whose tmux does not answer then, is refused the same way,
  with nothing stopped. That is, after taking the lock, the close decides nothing from a tmux value
  read before it. Cite [LANECLOSE-2-S4] on `TestCloseIfTakesTheExitFromTmuxNow`
  (`lanes/close_test.go`) and `TestCLICloseOfAnExitedLaneRestartedSinceThePoll`
  (`web/cli_test.go`). Task 4.1's refusals table gets the two refusals' words. design.md is not
  edited: the rule is group 1's Decision log entry, and it narrows D2, never widens it. Held by
  `.claude/scenario-trace.sh` and `checks/changes.sh`.

- [ ] Slice: an agent can close a finished panel lane with clauductor lane close, through Close lane's plan and guards, and cannot close a working one
