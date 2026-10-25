# Tasks: panel-57-lane-close

## Progress
- 2026-10-24 proposed (session 12); nothing built yet. The build starts from `main` once the owner
  approves. The roadmap row stays `⬜ queued` until its build PR opens.

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

## 1. The close verdict and the socket's route (D2, D4)
- [ ] 1.1 `framework/internal/panel/state/model.go` and `state/stall.go`: `TermLaneView.CloseHeld`
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
- [ ] 1.2 `framework/internal/panel/lanes/close.go`: `ClosePlan.Merged` and `ClosePlan.Unmerged`,
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
- [ ] 1.3 `framework/internal/panel/web/cli.go` (new): `(*Server).CLIHandler(ctx context.Context,
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
- [ ] 1.4 Review round 3's two verdict findings (the owner's decision of 2026-10-24), in
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
- [ ] 1.5 Review round 3's test finding: task 1.3's cancel test must fail without the check. Its
  runner stub cancels the request's context on the last plan call **and still returns the plan's
  normal output**, so the plan stays merged and the unmerged refusal cannot answer in its place;
  the test asserts the cancellation's answer (not a 409 naming the branch), and no stop and no
  removal. Run it once with the `r.Context()` check removed and see it fail. Citing
  [LANECLOSE-4-S1].
- [ ] 1.6 merge-pr's review of the approved proposal (2026-10-24). **An unread lease source holds
  every lane**, by D2's "a failed read keeps more": `m.leases` reads `m.v2.queues`, empty until
  the first good read and kept stale by `ApplyQueues` on an error (`model.go:1709-1716`), so while
  `len(m.cfg.Queues) > 0` and either `!m.stall.queuesRead` or `!m.v2.queuesSrc.OK`, `CloseHeld`
  is "the queues' leases have not been read", or "the queues' leases are not answering", checked
  first, before the `!Running||Dead` exemption, as the stall's `workJudged` (`stall.go:1097`)
  already refuses to judge then. **Auto-close's words:** every status word `CloseHeld` gives
  carries auto-close's `"claude is "` prefix ("claude is working", "claude is waiting on you",
  "claude is starting", …), as LANECLOSE-2-S1 says ("each in auto-close's words") and
  `TestAutoCloseOnMerge` asserts (`autoclose_integration_test.go:79`). Tested in
  `model_lanes_test.go`: a `Dead` lane and an `idle` lane, each held before the first
  `ApplyQueues` and after a failed one, and released after a good read that binds no lease to
  them; confirm the test fails with the check removed. Citing [LANECLOSE-2-S3].

## 2. The socket in the panel's life (D1, D4)
- [ ] 2.1 `framework/internal/panel/install/singleton.go`: `PanelOwner.Socket` (`json:"socket,
  omitempty"`), and `CLISocketPath(home)`, `~/.clauductor/panel/cli.sock`. Tested in
  `framework/internal/panel/singleton_test.go`: an `owner.json` naming a socket comes back from
  `RunningPanel` with it, and one from before this change (no field) comes back with it empty.
- [ ] 2.2 `framework/internal/panel/run.go` and `live.go`: `Options.CLISocket` (default
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
- [ ] 2.3 The close through the socket, in the same test file, on a throwaway tmux socket
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
- [ ] 2.4 merge-pr's review of the approved proposal (2026-10-24): **the command's client is
  tested against the real route.** The socket client the command uses (building the close body,
  decoding `{"ok","plan"}`, `{"ok","result"}` and `writeLaneErr`'s body) is one exported function
  in `framework/internal/panel/install` (the package the command already imports for
  `RunningPanel`), since `panel` cannot import `cmd`. Its consent is `lanes.CloseRequest`: the
  plan's `deleteBranch` (`close.go:47`) is sent as `branch` (`close.go:59`), which the route's
  `decodeStrict` would refuse under any other name. Task 2.3 calls that function, not a raw
  `http.Client`, for the dry run, the `--yes` close and the busy refusal against the real socket.
  Citing [LANECLOSE-1-S1], [LANECLOSE-1-S2], [LANECLOSE-2-S1].

## 3. The command (D2, D3)
- [ ] 3.1 `framework/internal/cmd/lane.go` (new): `clauductor lane close <id> [--yes]`, with no
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
- [ ] 3.2 merge-pr's review of the approved proposal (2026-10-24): `lane.go` closes through task
  2.4's client, and task 3.1's stub panel decodes the close body as `lanes.CloseRequest` with
  `DisallowUnknownFields`, asserting `branch` equals the plan's `deleteBranch` and `worktree` the
  plan's `worktree`. Citing [LANECLOSE-1-S2].

## 4. Docs and the production caller (D5, D7)
- [ ] 4.1 `docs/panel.md`: under *Close lane*, a subsection *Close a lane from the command line*
  (the command, run from the project's checkout with no `--project`, plan first, `--yes`, the
  verdict, merged-or-exited, and the self refusal, the finish after an interrupt, where its line
  goes); under *Close a lane when its PR merges*, that background work in flight, a lease, no
  current `claude agents` reading yet, or a session ended while claude still runs (after a
  `/clear`, which the owner then closes from the page) holds it, and the same in `lanes_auto_close`'s `Doc` in `framework/internal/panel/config/fields.go`,
  with the files it generates refreshed (`go generate ./internal/panel/config`: the config table
  in `docs/panel.md` and the field's `description` in `docs/panel.schema.json`, both held by
  `TestGeneratedFilesAreCurrent`); and under *Security model* the socket (mode 0600, the one route, no token).
  `README.md`'s command table: a `clauductor lane close` row.
- [ ] 4.2 `template/.claude/skills/session-close/SKILL.md`, the *Close the lanes* rule, D5's
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
- [ ] 4.3 Review round 3's retry finding (the owner's decision of 2026-10-24): in task 4.2's
  sentence and task 4.1's docs, on the retry after a timeout **any** "no lane" answer, the
  manager's own `404 "no lane %q"` (`lanes/close.go:343-345`) among them, means "closed or
  closing", and is never a cue to try another id from the unknown-id list; that list is read only
  on a first call. Held by `TestCommittedPluginIsCurrent` and `checks/skills.sh`.

- [ ] Slice: an agent can close a finished panel lane with clauductor lane close, through Close lane's plan and guards, and cannot close a working one
