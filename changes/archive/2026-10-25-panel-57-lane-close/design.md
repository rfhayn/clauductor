# Design: panel-57-lane-close

## Evidence
Read on 2026-10-24, at `24aa88ee`.

- **E1. Close lane is one handler over one manager call.** `closeLane`
  (`framework/internal/panel/web/routes.go:199`) decodes a `lanes.CloseRequest` (`dryRun`,
  `worktree`, `branch`; `lanes/close.go:56`) and calls `LaneManager.ClosePlan` for a dry run or
  `LaneManager.Close` otherwise. Every guard is in the manager: the worktree goes only through `git
  worktree remove` without `--force`, after a clean `git status`, inside `worktree_dir` and unlocked;
  the branch only when merged into the base or the head of a merged pull request, deleted at the tip
  it checked (`close.go:17-35`). `Close` checks everything again after claude exits, and removes no
  more than the consent offered (`close.go:364-458`).
- **E2. Close stops claude whatever it is doing.** `Close` calls `stopAndForgetLocked` first
  (`close.go:379`): an idle claude gets `/exit`, anything else Escape, then `kill-session`
  (`docs/panel.md`, *Close lane*, step 1). The page's confirmation is a person looking at the lane.
  Nothing in `Close` asks whether claude is working.
- **E3. Auto-close is the one caller that is not a person, and it adds an idle rule.**
  `pollAutoClose` (`framework/internal/panel/autoclose.go:36`) closes only when `!tv.Running ||
  tv.Dead || (tv.Status == "idle" && !tv.Approx)` (`autoclose.go:68`), and otherwise asks in Needs
  you with the reason, in `statusWords`' words (`autoclose.go:186`). It closes with `{Worktree: true,
  Branch: true}` only when the plan offers both (`autoclose.go:72-81`). It holds back, then, for
  three reasons: claude is not idle, the plan keeps the worktree or the branch, or the project's
  `lanes_auto_close` is `off`, the default (`config/fields.go:195`).
- **E4. Every route but three needs the page's cookie, and every POST needs the page's `Origin`.**
  `Handler` (`web/server.go:163`) refuses a non-loopback peer and a foreign `Host`. It refuses any
  POST whose `Origin` is not `http://<Host>`, except `/hook` and `/status`. `requireAuth`
  (`server.go:225`) wants the cookie `stridal_panel_<port>`, holding the per-launch token. That
  token is on disk only under launchd (`install.TokenPath`, `install/launchd.go:58`). A panel
  started by hand keeps it in memory (`run.go:319-323`). `/hook` and `/status` carry no token and
  are write-only. They are closed to browsers by refusing any request that has an `Origin` or a
  `Sec-Fetch-Site` (`server.go:233-250`).
- **E5. The CLI already finds the live panel without trusting a stale file.** `install.RunningPanel`
  (`install/singleton.go:163`) reads the pid file, checks that the process is alive with the start
  time `owner.json` recorded, and returns the `PanelOwner` (pid, port, projects). The panel holds
  `flock` on `~/.stridal/panel/lock` for its whole life, and removes its marker files at exit
  only while they are still its own (`singleton.go:22-33`). The panel directory is made private by
  `config.EnsurePrivateDir` (`singleton.go:66`). `owner.json` is written once from the value
  `run.go:354` builds, and rewritten from a copy of that value on every live `panel add` or `panel
  remove` (`liveSet.writeOwner`, `live.go:440-455`; the copy is taken at `run.go:413`).
- **E6. The request's context ends with the client.** The HTTP server's `BaseContext` is the
  panel's own context (`run.go:471`). `r.Context()` is also cancelled when the client's connection
  closes. `Close` runs its git commands under it, and a cancelled one counts as "cannot read",
  which keeps the worktree (`close.go:180-185`). An interrupted caller therefore leaves the lane
  stopped and its worktree and branch in place: safe, but half done. Each step bounds itself: the
  teardown with `HookTimeout`, 5 minutes (`lanes/lifecycle.go:31`, `:95`), each git command with
  20 seconds.
- **E7. A project is found by its id or its main checkout.** `config.LoadProjects` and
  `Projects.Find` match an id or an exact root, and `runCatchUp` resolves the path first
  (`cmd/panel.go:652-665`). A lane worktree's path is not a registered root, and `install`'s
  `mainRoot` refuses one (`install/candidate.go:142-143`). The main worktree is the first entry
  `signals.ReadWorktrees` lists for any path in the repository, as `mainRoot` itself reads it
  (`candidate.go:138-141`).
- **E8. Every lane knows its own id.** The panel starts each lane's tmux session with
  `STRIDAL_LANE=<id>` in its environment (`lanes/lanes.go:539`), and claude's Bash tool
  inherits it.
- **E9. An agent's `stridal` runs outside the sandbox and is not pre-allowed.** Both
  `settings.json` files list `stridal *` under `sandbox.excludedCommands`
  (`.claude/settings.json:202`, `template/.claude/settings.json:203`), so the command can reach a
  socket in `~/.stridal`. Neither has an allow rule for it (`template/.claude/settings.json:156`
  lists the gate, the checks and the status line), and lanes start with no permission mode
  (`lanes/lanes.go:440-466`). So in the default mode the owner is asked before an agent runs it.
- **E10. A Unix socket's path is short.** `sun_path` holds 104 bytes on macOS and 108 on Linux.
  `/Users/<name>/.stridal/panel/cli.sock` fits easily. A test's `t.TempDir()` home on macOS
  (`/var/folders/…/T/<TestName><n>/001`) can exceed it.
- **E11. A lane can read `idle` while its turn's work still runs.** A session whose turn ended on a
  background subagent or workflow reads `idle` on `claude agents`. STALL-1 counts it as
  *working*, and the code does too: `sessionWorking` (`state/stall.go:785-802`) holds a session
  working while its last `Stop` listed a subagent or workflow running or pending. A shell, a
  monitor or a teammate in that list is open work whatever its status (`taskOpen`, `stall.go:772`;
  `turnWork`, `stall.go:862-873`: a teammate's status says nothing). A lane that holds or waits for
  a queue's lease, the gate's among them, is never stalled either (STALL-1-S3; `m.leases`,
  `stall.go:649`, binds each lease to its lane), and a gate run from a terminal in the lane reaches
  no `Stop` list at all. E3's rule reads none of this, so auto-close today would close such a lane,
  and so would a command built on that rule alone.

## Decisions

- **D1. The CLI reaches the panel over a Unix socket that only the owner's user can open.**
  **Recommended.** The running panel listens on `~/.stridal/panel/cli.sock`, created with mode
  0600 in the 0700 panel directory, and serves one route there: `POST
  /api/p/{project}/lanes/{id}/close`, the page's path, with the same `CloseRequest` body (D3).
  Any other path or method is 404 or 405. The kernel's file permissions are the credential: no
  token is written, read or sent, and no browser can open a Unix socket, so the TCP server's guards
  (E4) are unchanged and gain no exemption. The socket is made after the machine lock is held and
  before the owner record is first written, so its path goes into the value `run.go:354` builds
  (`PanelOwner.Socket`), and every later rewrite of `owner.json` keeps it (E5). The CLI finds the
  live panel with `install.RunningPanel` (E5) and connects to the path it names, so a socket file
  a killed panel left behind is never mistaken for a live one. At start the panel removes a stale
  socket file at the path before listening (a file of any other kind is left, and the socket
  skipped). At exit the listener's `Close` removes the file (Go unlinks a socket `net.Listen`
  made), and the machine lock, held until then, means no other panel can have replaced it. When the socket cannot be made (E10's path limit, or a listen error), the panel
  starts anyway, prints why, and leaves `Socket` empty, and the CLI then says so. `Options` takes
  the path, so tests pass a short one.
  **Beat (b): a second token, over TCP.** The panel would write `~/.stridal/panel/cli-token`
  (0600, per launch), and the CLI would send it as `Authorization: Bearer` to a route that refuses
  any request with an `Origin` or `Sec-Fetch-Site`, as `/hook` does. It reuses the listener, but it
  carves an `Origin` exemption into the guard every other POST meets, and it adds a secret that an
  agent's command line and transcript then carry. The socket has neither.
  **Beat (c): reuse the page's token.** It is on disk only under launchd (E4). The CLI would have
  to forge the page's `Origin`. And it would hand every agent the page's whole power, terminals
  and lane starts included.
  **Beat (d): no credential, as `/hook`.** `/hook` is write-only and observes. A close stops a
  lane. Any process on the machine, another user's included, could close one.

- **D2. The command closes only a lane whose work has ended and landed, and never the caller's
  own.** **Recommended; its merged condition is the owner's direction of 2026-10-24.** Two tests,
  both needed:
  - **Its work has ended** (`TermLaneView.CloseHeld` is empty). A lane bound to a held or
    waited-for lease is held, whatever claude is doing: the lease covers a gate no `Stop` lists
    (E11). Otherwise, a lane whose claude has exited or is gone (`!Running || Dead`) may close:
    its session's background work ends with it. Otherwise claude must be idle on a current
    `claude agents` reading: `m.agentReading` returns one for the lane's session, and its status
    is `idle`. E3's rule (`Status == "idle" && !Approx`) is not enough on its own:
    `sessionStatus` (`state/blocked.go:49-71`) answers idle, and not approximate, when it has no
    reading and no hook status at all, as for a session heard only from `PreToolUse` right after a
    panel start, before the first `claude agents` answer (which a login-refresh lock can hold
    back, panel-claude-calls). Such a lane may be mid-turn, and is held: "claude has not been
    read yet". A hook's `idle` alone is approximate already, and held as E3 holds it. The lane's
    session can also have ended while its program runs: `/clear` gives the same claude a new
    session id that no lane record holds (`state/context.go:81`, LANECTX-2-S8), the record's id
    drops out of `claude agents`, and `ApplyAgents` ends it (`HookStatus` "ended",
    `binding.go:136`). The panel then cannot tell what the new session is doing, so the lane is
    held, in its own words, until its program exits: "its session ended while claude still runs
    (a /clear?): close it from the page". `s.end` clears the session's reading, so this check
    comes before the reading test, or the lane would read "claude has not been read yet". A
    session ends two ways: `ApplyAgents` ends one that drops out of the reading (`s.end`, which
    clears `Agent`), and a `SessionEnd` hook, which `/clear` sends, sets "ended" and leaves the
    last reading in place (`state/model.go:548`), so until the next poll that reading still says
    `idle`. A new `session.endedAt`, set on both paths, decides it: an ended session holds the
    lane unless a current reading lists it (`m.agentReading` non-nil) and that reading is newer
    than the end (`m.agentsOKAt` after `endedAt`). `ApplyAgents` takes back a session listed
    again ("Listed again, it is back", `state/model.go:780`) without clearing "ended", and such a
    lane is then read as any other. Once
    the panel forgets the ended session
    (`forgetSessions`, `blocked.go:167`), the lane reads as one never heard from and is held as
    "starting", still held. This is a stated cost: after a `/clear`, neither the command nor auto-close
    closes the lane, and the owner does (auto-close used to close it, reading the ended session's
    last `idle` while the new one might be busy). Then the
    lane's own session, the one `tv.SessionID` names, must be at
    rest: not working by STALL-1's rule (`sessionWorking`), and its last `Stop` listing no shell,
    monitor or teammate, whatever its status (`taskOpen`, as `turnWork` reads it), and no subagent
    (not marked ended) or workflow running or pending (`taskWorking`, `inFlight`). A type STALL-1
    counts as neither (a dream, a memory import) does not hold the close. Unlike
    `sessionWorking`, the task list holds the close however long the session has been quiet: a
    long gate is the case this guards, and a later `Stop` with no tasks clears it. (A lane whose
    session the panel has not heard from reads `running`, and is held as "starting".) Other
    sessions in the same worktree are not read: Close removes a worktree only when it is clean
    (E1), so their uncommitted work stays. **The cost, stated for the owner:** a session other
    than the lane's own, such as the owner's `claude` in a terminal there, working in a clean
    worktree whose branch merged, has its directory removed under it, as the page's Close lane
    does today. The merged test makes that rare (the work there has landed). Reading every session
    in the worktree was the first draft's rule; it made the verdict depend on which sessions the
    panel had heard of, and a session the hook never reached would pass it anyway.
  - **Its work has landed.** The lane's branch is merged into the base or is the head of a merged
    pull request, the verdict `branchVerdict` already gives (`close.go:227`), or its claude has
    exited (`Dead`: the program ended and remain-on-exit holds its pane). `ClosePlan` gains
    `Merged` (the verdict's `ok`, "" when not merged) and `Unmerged` (its `why`), set whenever
    the lane has a branch, even when its worktree stays. A lane that is not `Dead` closes only when `Merged` is set, so a plan
    with no branch is refused too: a corrupt record, a worktree gone, unreadable or no longer the
    project's, or a detached head. A failed read keeps more. Otherwise the socket's route refuses
    "its branch is not merged: <why>", or "its branch is not known" when the plan has none. An
    idle lane on unmerged work is most often one waiting for the owner's next prompt, and it stays
    the owner's, closed from the page. A lane on the main checkout is on the base branch, so it is
    refused unless its claude has exited ("its branch is not merged: it is the base branch").
    **A lane whose tmux session is gone is not exempt**, though its work has ended: it is the lane
    the page offers to restore (`Restorable`, `state/model.go:1936`), and after a reboot every
    lane is one. It closes from the command only when its branch is merged; otherwise the page
    keeps its restore. This reads the owner's "its claude has exited" as the program's exit
    (`Dead`), the narrower of the two readings; counting a gone session too would let an agent with
    a wrong id close any lane after a reboot and lose its restore, the harm the merged test exists
    to stop. **The cost of the exit exemption, stated for the owner:** a `Dead` lane keeps its
    tmux session, and the page offers it **Restart**. Closed with a wrong id while on unmerged
    work, it loses that Restart the same way (the record is forgotten, a clean worktree removed),
    and only `claude --resume <session>`, which the plan's Keep line names, brings the
    conversation back; the branch stays. That is the owner's decision as given, on a smaller set
    of lanes than a gone session would add. Auto-close needs no landed test: it acts only on a
    merged pull request already.

  The reducer computes `CloseHeld` in a second pass: `TerminalViews` (`state/model.go:1463`) builds
  the views, then fills each lane's `CloseHeld` from one `m.leases` call and the sessions. The
  first pass becomes an unexported builder that `leaseBinder` (`stall.go:677`) calls, so
  `TerminalViews → leases → leaseBinder` never comes back to `TerminalViews`. `leases` also
  returns each held lane's queue id, in a new map, so `judgeStalls`' reading is unchanged.
  `CloseHeld` reads like auto-close's reasons: "claude is working", "waiting on you", "starting",
  any status with "(not a current reading)" when it is approximate, "its turn ended on background
  work: <the task's description>", "it holds or waits for the <queue> lease", "claude has not
  been read yet", or "its session ended while claude still runs (a /clear?): close it from the
  page". `statusWords`
  moves to `state` with it.

  The socket's route refuses, before the manager is asked to close anything, a lane whose
  `CloseHeld` is set; then it asks for the plan, and refuses an unmerged lane as above. The plan
  can take about a minute (`git fetch`, `worktreeVerdict`'s `git status` and `branchVerdict`'s
  `rev-parse`, `for-each-ref` and `gh pr list`, under three 20 s bounds), and `Close` can then
  wait for the manager's lock up to `HookTimeout` (5 minutes, `lanes/lifecycle.go:31`) behind
  another lane's `worktree_setup` or teardown, or auto-close's own `Close`. So the check that
  counts is made under that lock: a close goes through a new `LaneManager.CloseIf(ctx, id,
  consent, held func() string)`, which calls `held` once it holds `m.mu` and before
  `stopAndForgetLocked`, and refuses 409 with nothing changed when it answers. `Close` becomes
  `CloseIf` with no `held`, so the page's route is unchanged. The socket's route passes a `held`
  that reads `CloseHeld` from a fresh `Hub.View()`, and auto-close passes the same over its hub.
  That takes the hub's lock inside the manager's; no path takes them the other way, since every
  `Hub.Update` and `Hub.Read` closure touches the model alone and `m.changed` only signals a
  channel (`live.go:115`). The view
  lists every registered lane and every tmux session on the panel's socket (`TermLaneView`,
  `state/model.go:1387`), so a lane it does not list is refused as unknown (LANECLOSE-1-S4). A
  lane started seconds ago may not be listed yet, and refusing it is the safe side. The CLI also
  refuses an id equal to its own `STRIDAL_LANE` (E8) before it reads anything or asks the
  panel, and names what does close a lane after its work, in fixed words: the page's Close lane,
  and auto-close where it is on for the lane's type (`AutoCloseOf`; off by default, E3). It reads
  no config to choose them, since a lane type's `auto_close` overrides the project's
  `lanes_auto_close` and the CLI does not know the lane's type. That check only gives a clearer
  message: the panel's rule is the authority, and a lane running the command is never idle. Even
  read under the lock, the view is only as current as the panel's last reading of the lane, and
  claude can start working between that reading and the stop.
  `Close` then stops it as Stop does (E2). This narrows the window and cannot close it, as the
  detached recheck narrows its own (`close.go:129`).
  **Auto-close reads the same `CloseHeld`** in place of its own rule, so it too stops closing a
  lane whose turn ended on background work, that holds a lease, or whose claude has no current
  `claude agents` reading yet (a session heard only from its tool hooks), or whose session ended
  while its program still runs (after a `/clear`) (E11). That is a fix to
  PANEL-20's behaviour, and the owner's to approve with this decision. **Beat: the stricter rule
  for the command only,** leaving auto-close on E3's rule. Two rules for one question, one of them
  knowingly unsafe.
  **Beat: close any idle lane, merged or not** (this proposal's first draft). With D7 any agent
  could then stop an idle lane waiting on the owner, with a wrong id, and its record would be gone:
  its worktree and branch stay, but the page can no longer restore it.
  **Beat: close whatever claude is doing, as the page does.** The page has the owner looking at
  the lane. A command has a lane id typed by an agent. A wrong id, or a right id a few minutes
  early, would end a working turn: a running gate or a build group, its uncommitted work kept but
  its progress lost.

- **D3. The plan first; `--yes` closes.** **Recommended.** `stridal lane close <id>` sends
  `{"dryRun": true}` and prints the plan as the page's confirmation lists it, after a first line
  naming the lane and its branch (`lane <id> on <branch>`): **Removes**, **Keeps** (each with why)
  and the notes. It changes nothing and exits 0. With `--yes` it asks for
  the plan, then closes with the plan's offers as the consent (`worktree` and `branch` as the plan
  set them, as the page's Confirm sends them), prints **Removed** and **Kept** from the result,
  and exits 0. The lane is stopped either way, and what stays is said with why (E1). Any refusal
  (D2, no panel, no such lane, a project the panel does not serve) exits 1 with the reason. D2's
  refusals apply to the dry run too: a lane the command may not close gets the refusal, not a
  plan it could not carry out, so the agent learns it before it confirms anything.
  **Beat: close by default, with `--dry-run` for the plan.** That is one call fewer, but the
  command's first use by an agent would then be the act itself. With the plan first, the agent and
  the owner reading its transcript see what will go before anything does, as the page shows it.

- **D4. A close the panel has started finishes, even if the command is interrupted.**
  **Recommended.** The socket's route runs `Close` under the panel's own context, which
  `CLIHandler` is given, not the request's. A panel shutting down still cancels it, which keeps
  more (E6). No deadline is added: each step keeps its own bound, the teardown its 5 minutes. An
  agent's Bash timeout or a Ctrl-C then leaves the close finished, not the lane stopped with its
  clean, merged worktree left behind. Only `CloseIf` runs under the panel's context: the
  route's checks and its plan run under the request's, so a command cut off before `CloseIf`
  begins changes nothing and leaves no line (a cancelled read keeps more, E6). With `--yes` the
  CLI asks for the plan twice (its dry run, printed first, then the close route's own); the
  route checks the request's context once more just before `CloseIf`, so a cancel that lands
  after the plan still changes nothing; each plan takes up to about a
  minute, plus up to 5 minutes' wait for the manager's lock (`ClosePlan` takes it,
  `lanes/close.go:341`) behind a setup or teardown. D5's sentence gives the `--yes` call a
  10-minute Bash timeout, Bash's ceiling, which covers the usual case and not the worst. A call
  the timeout kills prints at most the plan, so its outcome is unknown: the close may not have
  begun, or may still be finishing. D5 then has the session run the dry run again once, under
  the same timeout: a lane the panel still lists is named for the owner with its plan (a close
  still waiting for the lock may yet close it); one it no longer lists is named "closed or
  closing", since the lane leaves the view when it is stopped, before the teardown and the
  removal, and the owner is pointed to the panel's log line, written once `Close` returns, for
  what was removed and kept. A retry that times out too is named as "outcome unknown".
  The page's route keeps the request's context, as it does today.
  **Beat: the request's context, as the page.** It is safe (a cancelled read keeps more), but the
  CLI is likelier than a browser tab to be cut off mid-close, and what it leaves is the half-done
  state this row exists to clear.

- **D5. session-close closes a merged lane the panel left open.** **Recommended.**
  `template/.claude/skills/session-close/SKILL.md`'s *Close the lanes* rule gains one sentence.
  For each lane whose pull request merged and that is still open: run `stridal lane close
  <id>` from the repository, then, only when the plan's first line names the merged branch, the
  same with `--yes` under a 10-minute Bash timeout (D4), and put the plan's Keeps lines, if any,
  in the final message. When the call timed out, its outcome is unknown, and the session
  follows D4's retry: the dry run once more, under the same timeout, and the lane named "closed
  or closing" with the panel's log line pointed to, or with its plan, or as "outcome unknown". The id to try
  first is the folder name of the merged branch's worktree in `git worktree list` (a new lane's
  worktree is `<worktree_dir>/<id>`, `lanes/lanes.go:712`; a lane started on an existing worktree
  may differ). When that id is unknown, the refusal's `id (branch)` list names the lane on the
  merged branch, and that id is used instead. When it is a lane on another branch (the plan's
  first line says so), nothing more is run for it: the final message names the merged branch
  whose lane was not found, and leaves it to the owner. A refusal (D2) is named in the final
  message too, with its reason and the lane it was for. Skip it when
  `stridal` is not on `PATH` or no panel runs (the repository works without either:
  ADR-0006). This covers every way auto-close holds back (E3) but the first: with auto-close off,
  the command closes what auto-close would have; with a plan that keeps a dirty worktree, the lane
  stops and the worktree and its branch stay, as the page's Close lane leaves them, named for the
  owner. A lane whose work has not ended is refused, and stays the owner's. The plugin is rebuilt and this repository's copy synced. This is the
  production caller (rule 3), and it stays a skill sentence: **Nothing. You.** executes it, as it
  executes the rule today.
  **Beat: `--yes` only when the plan removes both the worktree and the branch.** That is exactly
  the set auto-close closes, so in a project with `on_merge` (this one: `.stridal/panel.json:20`)
  it adds nothing, and the row's case, "auto-close holds back", stays with the owner's click.
  **Beat: the command and `docs/panel.md` only.** That ships a capability whose only caller is
  an agent that happens to read the panel's docs, which rule 3 calls not shipped.

- **D6. Starting a lane from the command line stays out.** **Recommended.** `POST /lanes` takes a
  template, a mode, a name, a worktree and the first prompt, and starting is gated by the config's
  trust and by what is in a build's way (NEWLANE-4). That is a command of its own, with its own
  design. No row asks for it and no session has needed it. If the owner wants it, it becomes its
  own roadmap row in this proposal's PR.
  **Beat: `stridal lane start` in this change.** It would double the change's write surfaces
  and its review, and put two decisions about trust (who may start a lane, with which template) in
  a row sized for one.

- **D7. The template allows the command without asking.** **Recommended.**
  `template/.claude/settings.json`'s `permissions.allow` gains `Bash(stridal lane close *)`, and
  this repository's copy follows through `stridal update`. session-close runs hands-off, and
  without the rule each lane costs two permission prompts in the default mode (E9), which leaves
  the session waiting on the owner it was meant to spare. The rule is narrow: `close` alone, whose
  every effect is D2's and Close lane's guards; no other `stridal` subcommand is allowed. Under
  D2 an agent can close only a lane whose work has ended and whose branch merged, or whose claude
  has exited (a lane whose tmux session is gone keeps its restore unless its branch merged), and
  only in the project of the directory it runs in (no `--project`); a dirty
  worktree or an unmerged branch still stays (E1).
  **Beat: no allow rule.** Each project or owner allows it themselves, and until then D5's
  sentence asks the owner twice per lane, or is skipped under a mode that denies it.

## The command

```
stridal lane close <id> [--yes]
```

- **The panel**, checked first, is the live one `install.RunningPanel` finds (E5), reached at
  `owner.json`'s `Socket` (D1). No live panel, an empty `Socket` (a panel `RunningPanel` found by
  `/healthz` alone has none, `singleton.go:192`), or a socket that does not answer: exit 1, saying
  which and naming `stridal panel`.
- **The project** is the repository of the current directory, and nothing else: the current
  directory is resolved to its main checkout, the first entry of `signals.ReadWorktrees` from
  `git rev-parse --show-toplevel` (E7), and matched in the registry. A lane worktree therefore
  names its project. A directory that is not in a registered project, or one the running panel
  does not serve (`PanelOwner.Projects`), is refused, naming `stridal panel list`. There is no
  `--project`: its only caller (D5) runs in the repository it closes in, and with D7 the flag
  would let a session in one project close another project's lane without a prompt.
- **Output** is lines for people, the page's words: the plan's or the result's lists, each line as
  `close.go` wrote it. No `--json` (rule 3: no caller reads one).
- **The panel's output** (`Options.Out`) gets one line per close through the socket: `lane <id>
  (<project>) closed by stridal lane close: removed …; kept …`. Under launchd that output is the
  panel's log in `install.LogDir`, where its other lines already go.
- **An unknown id** is answered with the project's lanes from the view, each with its branch:
  `no lane "x"; this project's lanes: a (change/a), b (fix/12-b)`.

## Refusals

| Situation | What happens instead |
|---|---|
| The lane's claude is working, waiting on the owner, starting, or its reading is not current | Refused, exit 1, naming the status (D2). Nothing stops. |
| claude runs, but `claude agents` has not read it yet, or its session ended while it still runs (after a `/clear`) | Refused, exit 1: "claude has not been read yet", or "its session ended while claude still runs (a /clear?): close it from the page" (D2). Nothing stops. |
| claude is idle, but its own session's turn ended on background work still in flight, or the lane holds or waits for a lease (exited or not) | Refused, exit 1, naming the task or the lease (D2). Nothing stops. |
| claude is idle, or the lane's tmux session is gone, and its branch is not merged or not known | Refused, exit 1: "its branch is not merged: <why>" or "its branch is not known" (D2). Nothing stops; the page's Close lane, or its restore, still can. |
| The id is the caller's own `STRIDAL_LANE` | Refused by the CLI before it reads anything, naming the page's Close lane, and auto-close where it is on for the lane's type (D2) |
| No live panel, or one with no socket | Exit 1, saying which; nothing is written |
| The current directory is in no project the panel serves | Exit 1, naming `stridal panel list` |
| No such lane in the project | Exit 1, listing the project's lanes, each with its branch |
| The worktree is dirty, locked, outside `worktree_dir`, or another lane's; or, for a lane whose claude has exited, the branch is unmerged | Not a refusal: the lane stops and those stay, each with why, as on the page (E1) |
| A request on the socket for anything but the close | 404 or 405; the socket serves nothing else (D1) |
| `--yes` is interrupted after the panel began the close | The panel finishes it (D4) |
| `stridal` is not on `PATH`, or no panel runs, at session close | session-close skips the step (D5) |
