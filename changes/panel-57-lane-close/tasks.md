# Tasks: panel-57-lane-close

## Progress
- 2026-10-24 proposed (session 12); nothing built yet. The build starts from `main` once the owner
  approves. The roadmap row stays `⬜ queued` until its build PR opens.

## Decision log

## 1. The close verdict and the socket's route (D2, D4)
- [ ] 1.1 `framework/internal/panel/state/model.go`: `TermLaneView.CloseHeld`, set in
  `TerminalViews` (empty when the lane may close, otherwise why not): E3's rule, then every live
  session in the lane's worktree checked with `sessionWorking` and with its last `Stop`'s tasks
  (a `taskOpen` type whatever its status, as `turnWork` reads it; a `taskWorking` type when
  `inFlight`, a subagent not marked ended), with no `unheard` limit on the tasks; then
  `m.leases`, the lane held while it holds or waits for a lease. `statusWords` moves to `state` from `autoclose.go`, and `pollAutoClose` reads
  `CloseHeld` in place of its own rule. Tested in `framework/internal/panel/state/model_lanes_test.go`:
  a table over `busy`, `waiting`, `running`, `idle`, an exited program (`Dead`), a gone session
  (`!Running`), and each of `busy` and `idle` with `Approx`, checking the verdict and the words;
  then an `idle` lane whose session's last `Stop` lists, in turn, a running subagent, a pending
  workflow, a running shell, a teammate with an empty status, a subagent marked ended, a `dream`
  task, and nothing, the first four held naming the task and the last three not; a running
  subagent still held after `stallUnheard` × the stall time has passed; and an `idle` lane bound to
  a queue's live holder, then to a waiter, held naming the lease, and not once the lease is gone. `TestAutoCloseOnMerge` passes unchanged. Citing
  [LANECLOSE-2-S1], [LANECLOSE-2-S3].
- [ ] 1.2 `framework/internal/panel/web/cli.go` (new): `(*Server).CLIHandler(ctx context.Context,
  out io.Writer) http.Handler`, a mux of its own serving only `POST
  /api/p/{project}/lanes/{id}/close` through `withProject` and `decodeStrict`, with no cookie and
  no `Origin` check (D1: the socket's mode is the credential). Before the manager is asked: a lane
  the project's `Hub.View()` does not list is refused 404 with the view's lanes as `id (branch)`;
  one whose
  `CloseHeld` is set, 409 with it. Then the dry run as `closeLane` answers it, or `Close` under
  the handler's `ctx` (the panel's), with no added deadline, and one line to `out` per close:
  `lane <id> (<project>) closed by clauductor lane close: removed …; kept …`. Tested in
  `framework/internal/panel/web/cli_test.go` (new) through `httptest`, with the project's lanes
  from `testLaneManager` and its view set on the hub: any other path or method, `/api/state` and
  `/lanes/{id}/stop` among them, is 404 or 405; a lane whose view sets `CloseHeld` is refused 409
  with its words, and the manager's runner records no call; an id the view does not list is
  refused with its lanes and their branches; a close whose request context is cancelled before `Close` returns
  still reaches the manager under a live context. Citing [LANECLOSE-3-S1], [LANECLOSE-2-S1],
  [LANECLOSE-2-S3], [LANECLOSE-1-S4], [LANECLOSE-4-S1].

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
  Needs you; a `worktree_teardown` that touches a marker and sleeps, with the client's request
  cancelled once the marker exists, still ends with the worktree removed, the branch deleted and
  the line printed. Citing [LANECLOSE-1-S1], [LANECLOSE-1-S2], [LANECLOSE-1-S3],
  [LANECLOSE-2-S1], [LANECLOSE-2-S3], [LANECLOSE-4-S1].

## 3. The command (D2, D3)
- [ ] 3.1 `framework/internal/cmd/lane.go` (new): `clauductor lane close <id> [--yes] [--project
  <id|path>]`, registered on the root command as `panel` is (`panel.go`). An id equal to
  `$CLAUDUCTOR_LANE` refused before anything is read. The panel first, from
  `install.RunningPanel`, with its `lease.ProcCheck` through a package variable a test replaces;
  no live panel, or an empty `Socket` (the `/healthz` fallback's owner among them), refused naming
  `clauductor panel`. Then the project from
  `--project` or the current directory: an id matched with `config.LoadProjects` and `Find`, or a
  path resolved to its main checkout with a new exported `install.MainWorktree(ctx, run, path)`
  (`git rev-parse --show-toplevel`, then the first entry `signals.ReadWorktrees` lists, the lines
  `mainRoot` reads, which then calls it) and matched by root. Refused, naming `clauductor panel
  list`, when not registered or not among the running panel's `Projects`. Then the dry run, its **Removes**, **Keeps** and
  notes printed; with `--yes`, the close with the plan's `worktree` and `deleteBranch` as the
  consent, its **Removed** and **Kept** printed. A refusal's message printed as the panel wrote
  it, exit 1. Tested in `framework/internal/cmd/lane_test.go` (new) against a stub panel: a
  Unix-socket `httptest` server answering canned plans, results and refusals, with a pid file and
  an `owner.json` in a temporary home and the process check replaced. Citing [LANECLOSE-1-S1] (the
  plan printed, the stub asked for the dry run alone), [LANECLOSE-1-S2] (with `--yes`, the stub
  asked for the dry run then the close with the plan's offers, and the **Removed** lines printed),
  [LANECLOSE-1-S3] (a result with **Kept** lines printed with their reasons, exit 0),
  [LANECLOSE-1-S4] (the refusal's lanes and branches printed), [LANECLOSE-1-S5] (run from a linked worktree
  made with `git worktree add`, then with `--project` as an id and as a subdirectory),
  [LANECLOSE-2-S2] (the stub records no request), [LANECLOSE-2-S3] (a 409 naming background work
  printed, exit 1), [LANECLOSE-3-S2] (no live pid, a socket file present: no connection),
  [LANECLOSE-3-S4] (an `owner.json` with no socket, and a panel found by `/healthz` alone, both
  refused for the socket, not the project). `install`'s tests for `mainRoot` pass
  unchanged.

## 4. Docs and the production caller (D5, D7)
- [ ] 4.1 `docs/panel.md`: under *Close lane*, a subsection *Close a lane from the command line*
  (the command, plan first, `--yes`, the verdict and the self refusal, the finish after an
  interrupt, where its line goes); under *Close a lane when its PR merges*, that background work
  in flight holds it; and under *Security model* the socket (mode 0600, the one route, no token).
  `README.md`'s command table: a `clauductor lane close` row.
- [ ] 4.2 `template/.claude/skills/session-close/SKILL.md`, the *Close the lanes* rule, D5's
  sentence; `template/.claude/settings.json`'s `permissions.allow`, `Bash(clauductor lane close
  *)` (D7). Then `scripts/build-plugin.sh`, and `env -u CLAUDUCTOR_FRAMEWORK framework/clauductor
  update` from the repository root with the binary built from this checkout, its `Template:` line
  checked to name this worktree's `template/`. Commit only `.claude/skills/session-close/SKILL.md`
  and `.claude/settings.json` from it; any other tracked drift it writes is OPS-119's, restored
  with `git checkout --`. An agent's sandbox denies writing `.claude/settings.json`, so if update
  cannot write it, the builder stops and asks the owner to run that one command. Held by `TestCommittedPluginIsCurrent`, `checks/skills.sh` and
  `checks/settings.sh`.

- [ ] Slice: an agent can close a finished panel lane with clauductor lane close, through Close lane's plan and guards, and cannot close a working one
