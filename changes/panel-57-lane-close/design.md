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
  (`server.go:225`) wants the cookie `clauductor_panel_<port>`, holding the per-launch token. That
  token is on disk only under launchd (`install.TokenPath`, `install/launchd.go:58`). A panel
  started by hand keeps it in memory (`run.go:319-323`). `/hook` and `/status` carry no token and
  are write-only. They are closed to browsers by refusing any request that has an `Origin` or a
  `Sec-Fetch-Site` (`server.go:233-250`).
- **E5. The CLI already finds the live panel without trusting a stale file.** `install.RunningPanel`
  (`install/singleton.go:163`) reads the pid file, checks that the process is alive with the start
  time `owner.json` recorded, and returns the `PanelOwner` (pid, port, projects). The panel holds
  `flock` on `~/.clauductor/panel/lock` for its whole life, and removes its marker files at exit
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
  `CLAUDUCTOR_LANE=<id>` in its environment (`lanes/lanes.go:539`), and claude's Bash tool
  inherits it.
- **E9. An agent's `clauductor` runs outside the sandbox and is not pre-allowed.** Both
  `settings.json` files list `clauductor *` under `sandbox.excludedCommands`
  (`.claude/settings.json:202`, `template/.claude/settings.json:203`), so the command can reach a
  socket in `~/.clauductor`. Neither has an allow rule for it (`template/.claude/settings.json:156`
  lists the gate, the checks and the status line), and lanes start with no permission mode
  (`lanes/lanes.go:440-466`). So in the default mode the owner is asked before an agent runs it.
- **E10. A Unix socket's path is short.** `sun_path` holds 104 bytes on macOS and 108 on Linux.
  `/Users/<name>/.clauductor/panel/cli.sock` fits easily. A test's `t.TempDir()` home on macOS
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
  **Recommended.** The running panel listens on `~/.clauductor/panel/cli.sock`, created with mode
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
  **Beat (b): a second token, over TCP.** The panel would write `~/.clauductor/panel/cli-token`
  (0600, per launch), and the CLI would send it as `Authorization: Bearer` to a route that refuses
  any request with an `Origin` or `Sec-Fetch-Site`, as `/hook` does. It reuses the listener, but it
  carves an `Origin` exemption into the guard every other POST meets, and it adds a secret that an
  agent's command line and transcript then carry. The socket has neither.
  **Beat (c): reuse the page's token.** It is on disk only under launchd (E4). The CLI would have
  to forge the page's `Origin`. And it would hand every agent the page's whole power, terminals
  and lane starts included.
  **Beat (d): no credential, as `/hook`.** `/hook` is write-only and observes. A close stops a
  lane. Any process on the machine, another user's included, could close one.

- **D2. The command closes only a lane whose work has ended, and never the caller's own.**
  **Recommended.** A lane may close when its claude has exited or is gone (`!Running || Dead`:
  background work ends with it), or when claude is idle on a current reading (E3's rule) and no
  live session in the lane's worktree is still at work, and the lane holds no lease (E11): none
  is working by STALL-1's rule (`sessionWorking`); none's last `Stop` lists a shell, a monitor or a
  teammate, whatever its status (`taskOpen`, as `turnWork` reads it), or a subagent (not marked
  ended) or a workflow running or pending (`taskWorking`, `inFlight`); and `m.leases` binds no
  held or waited-for lease to the lane. A type STALL-1 counts as neither (a dream, a memory
  import) does not hold the close. Unlike `sessionWorking`, the task list holds the close however
  long the session has been quiet: a long gate is the case this guards, and a later `Stop` with no
  tasks clears it. The lease covers the gate no `Stop` lists. The reducer
  computes this once per lane, as `TermLaneView.CloseHeld` (empty when the lane may close,
  otherwise why not), in `TerminalViews` (`state/model.go:1463`), so the reason reads like
  auto-close's: "claude is working", "waiting on you", "starting", any status with "(not a current
  reading)" when it is approximate, "its turn ended on background work: <the task's
  description>", or "it holds or waits for the <queue> lease". `statusWords` moves to `state` with it. The socket's route refuses, before asking
  the manager anything, a lane whose `CloseHeld` is set. The view lists every registered lane and
  every tmux session on the panel's socket (`TermLaneView`, `state/model.go:1387`), so a lane it
  does not list is refused as unknown (LANECLOSE-1-S4). A lane started seconds ago may not be
  listed yet, and refusing it is the safe side. The CLI also refuses an id equal to its own
  `CLAUDUCTOR_LANE` (E8) before it asks the panel, and names the two things that do close a lane
  after its work: auto-close once the turn ends, and the page's Close lane. That check only gives
  a clearer message: the panel's rule is the authority, and a lane running the command is never
  idle. The view can be a few seconds old, and claude can start working between the check and the
  stop. `Close` then stops it as Stop does (E2). This narrows the window and cannot close it, as
  the detached recheck narrows its own (`close.go:129`).
  **Auto-close reads the same `CloseHeld`** in place of its own rule, so it too stops closing a
  lane whose turn ended on background work (E11). That is a fix to PANEL-20's behaviour, and the
  owner's to approve with this decision. **Beat: the stricter rule for the command only,**
  leaving auto-close on E3's rule. Two rules for one question, one of them knowingly unsafe.
  **Beat: close whatever claude is doing, as the page does.** The page has the owner looking at
  the lane. A command has a lane id typed by an agent. A wrong id, or a right id a few minutes
  early, would end a working turn: a running gate or a build group, its uncommitted work kept but
  its progress lost.

- **D3. The plan first; `--yes` closes.** **Recommended.** `clauductor lane close <id>` sends
  `{"dryRun": true}` and prints the plan as the page's confirmation lists it: **Removes**,
  **Keeps** (each with why) and the notes. It changes nothing and exits 0. With `--yes` it asks for
  the plan, then closes with the plan's offers as the consent (`worktree` and `branch` as the plan
  set them, as the page's Confirm sends them), prints **Removed** and **Kept** from the result,
  and exits 0. The lane is stopped either way, and what stays is said with why (E1). Any refusal
  (D2, no panel, no such lane, a project the panel does not serve) exits 1 with the reason.
  **Beat: close by default, with `--dry-run` for the plan.** That is one call fewer, but the
  command's first use by an agent would then be the act itself. With the plan first, the agent and
  the owner reading its transcript see what will go before anything does, as the page shows it.

- **D4. A close the panel has started finishes, even if the command is interrupted.**
  **Recommended.** The socket's route runs `Close` under the panel's own context, which
  `CLIHandler` is given, not the request's. A panel shutting down still cancels it, which keeps
  more (E6). No deadline is added: each step keeps its own bound, the teardown its 5 minutes. An
  agent's Bash timeout or a Ctrl-C then leaves the close finished, not the lane stopped with its
  clean, merged worktree left behind. The page's route keeps the request's context, as it does
  today.
  **Beat: the request's context, as the page.** It is safe (a cancelled read keeps more), but the
  CLI is likelier than a browser tab to be cut off mid-close, and what it leaves is the half-done
  state this row exists to clear.

- **D5. session-close closes a merged lane the panel left open.** **Recommended.**
  `template/.claude/skills/session-close/SKILL.md`'s *Close the lanes* rule gains one sentence.
  For each lane whose pull request merged and that is still open: run `clauductor lane close
  <id>`, then the same with `--yes`, and put the plan's Keeps lines, if any, in the final message.
  The id is the folder name of the merged branch's worktree in `git worktree list` (a lane's
  worktree is `<worktree_dir>/<id>`, `lanes/lanes.go:712`); when that is refused as unknown, the
  refusal's `id (branch)` list names the lane on that branch. A refusal (D2) is named in the final
  message too, with its reason. Skip it when `clauductor` is not on `PATH`
  or no panel runs (the repository works without either: ADR-0006). This covers every way
  auto-close holds back (E3) but the first: with auto-close off, the command closes what
  auto-close would have; with a plan that keeps the worktree or branch, the lane stops and they
  stay, as the page's Close lane leaves them, named for the owner. A lane not idle is refused, and
  stays the owner's. The plugin is rebuilt and this repository's copy synced. This is the
  production caller (rule 3), and it stays a skill sentence: **Nothing. You.** executes it, as it
  executes the rule today.
  **Beat: `--yes` only when the plan removes both the worktree and the branch.** That is exactly
  the set auto-close closes, so in a project with `on_merge` (this one: `.clauductor/panel.json:20`)
  it adds nothing, and the row's case, "auto-close holds back", stays with the owner's click.
  **Beat: the command and `docs/panel.md` only.** That ships a capability whose only caller is
  an agent that happens to read the panel's docs, which rule 3 calls not shipped.

- **D6. Starting a lane from the command line stays out.** **Recommended.** `POST /lanes` takes a
  template, a mode, a name, a worktree and the first prompt, and starting is gated by the config's
  trust and by what is in a build's way (NEWLANE-4). That is a command of its own, with its own
  design. No row asks for it and no session has needed it. If the owner wants it, it becomes its
  own roadmap row in this proposal's PR.
  **Beat: `clauductor lane start` in this change.** It would double the change's write surfaces
  and its review, and put two decisions about trust (who may start a lane, with which template) in
  a row sized for one.

- **D7. The template allows the command without asking.** **Recommended.**
  `template/.claude/settings.json`'s `permissions.allow` gains `Bash(clauductor lane close *)`, and
  this repository's copy follows through `clauductor update`. session-close runs hands-off, and
  without the rule each lane costs two permission prompts in the default mode (E9), which leaves
  the session waiting on the owner it was meant to spare. The rule is narrow: `close` alone, whose
  every effect is D2's and Close lane's guards; no other `clauductor` subcommand is allowed.
  **Beat: no allow rule.** Each project or owner allows it themselves, and until then D5's
  sentence asks the owner twice per lane, or is skipped under a mode that denies it.

## The command

```
clauductor lane close <id> [--yes] [--project <id|path>]
```

- **The panel**, checked first, is the live one `install.RunningPanel` finds (E5), reached at
  `owner.json`'s `Socket` (D1). No live panel, an empty `Socket` (a panel `RunningPanel` found by
  `/healthz` alone has none, `singleton.go:192`), or a socket that does not answer: exit 1, saying
  which and naming `clauductor panel`.
- **The project** is `--project` (an id, or any path in the repository), or else the repository
  of the current directory. A path is resolved to its main checkout, the first entry of
  `signals.ReadWorktrees` from `git rev-parse --show-toplevel` (E7), and matched in the registry.
  A lane worktree therefore names its project. A path that is not a registered project, or one the
  running panel does not serve (`PanelOwner.Projects`), is refused, naming `clauductor panel list`.
- **Output** is lines for people, the page's words: the plan's or the result's lists, each line as
  `close.go` wrote it. No `--json` (rule 3: no caller reads one).
- **The panel's output** (`Options.Out`) gets one line per close through the socket: `lane <id>
  (<project>) closed by clauductor lane close: removed …; kept …`. Under launchd that output is the
  panel's log in `install.LogDir`, where its other lines already go.
- **An unknown id** is answered with the project's lanes from the view, each with its branch:
  `no lane "x"; this project's lanes: a (change/a), b (fix/12-b)`.

## Refusals

| Situation | What happens instead |
|---|---|
| The lane's claude is working, waiting on the owner, starting, or its reading is not current | Refused, exit 1, naming the status (D2). Nothing stops. |
| claude is idle, but its turn ended on background work still in flight, or the lane holds or waits for a lease | Refused, exit 1, naming the task or the lease (D2). Nothing stops. |
| The id is the caller's own `CLAUDUCTOR_LANE` | Refused by the CLI before the panel is asked, naming auto-close and the page's Close lane (D2) |
| No live panel, or one with no socket | Exit 1, saying which; nothing is written |
| The project is not one the panel serves | Exit 1, naming `clauductor panel list` |
| No such lane in the project | Exit 1, listing the project's lanes, each with its branch |
| The worktree is dirty, locked, outside `worktree_dir`, or another lane's; the branch unmerged | Not a refusal: the lane stops and those stay, each with why, as on the page (E1) |
| A request on the socket for anything but the close | 404 or 405; the socket serves nothing else (D1) |
| `--yes` is interrupted after the panel began the close | The panel finishes it (D4) |
| `clauductor` is not on `PATH`, or no panel runs, at session close | session-close skips the step (D5) |
