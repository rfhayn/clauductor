# Design: ops-118-main-catches-up

## Evidence
Read on 2026-10-22, at `62fa18f0`.

- **E1. What trust guards today.** Trust records a hash of `panel.json`'s exact bytes
  (`install/trust.go`: `ConfigHash`, `CheckTrust`, `TrustPath`; `install/candidate.go`:
  `TrustExact`). It does not cover the scripts those bytes run. This repository's cards run
  `sh -c "sh .claude/owner-queue.sh 2>&1"` and the like, its queue runs `scripts/ci/run-local.sh`,
  and Up next runs `sh .claude/panel-suggest.sh <type>`. All of those scripts change with ordinary
  merges, and none of those merges asks for re-trust. Re-trust guards the argv and prompts in
  `panel.json`, not what they run.
- **E2. How often `panel.json` changes, and how.** Five commits: its creation (`649aa492`), then
  four changes. Three changed what it runs: `b163aeb3` added the metrics command, `bd6a8c0b`
  replaced an inline `sh -c` health loop, and `754a8cdb` (#205) changed the suggest refresh and
  the ops template's first prompt. One, `66016c3e`, changed only `version` and `launch`, in the
  same pull request as the framework code that reads them. A binary refuses a config whose
  version is above its own (`config.go:736`), so such a config loads only in the new binary.
- **E3. How the panel loads a config.** A project's runtime reads `panel.json` once, at its start
  (`live.go`: `buildRuntime`, `startEntry`). Nothing watches the file. `pollTrust`
  (`runtime.go:829`) runs only while the loaded config is untrusted, against the loaded hash. The
  page's Trust (`liveSet.Trust`, `live.go:661`) trusts the exact bytes on disk. When they differ
  from the loaded ones, it checks they load (`canStart`) and restarts the project's runtime on
  them (`restartStop`, `startEntry`). That is the reload this change reuses. `markTrusted(note)`
  sets the trust note the page shows. `restartStop` halts the runtime and waits for all of its
  goroutines (`live.go:138–159`), so it cannot be called from one of them. The liveSet's own
  machine-level source, `pollRegistry` (`live.go:220`), reconciles `projects.json` from outside
  every runtime.
- **E4. The sidecar rule would not fit.** `config/sidecar.go` refuses shell text and interpreter
  code flags (`sh -c`) for a config outside its repository. Its premise (PANEL-42 D5) is that an
  in-repository config is reviewed with the repository. Every card of this repository's
  `panel.json`, and of the template's, runs `sh -c "… 2>&1"` (ATTACH-6: an in-repository card
  shows stdout alone). So "trust it when it passes the sidecar rule" would never apply here.
- **E5. What the panel can check of a merge.** Gate receipts live in each worktree's git dir
  (`.git/worktrees/<lane>/ci-receipt`). They go with the lane's worktree, and an agent can write
  them. Review convergence is a skill step (merge-pr step 3) whose `Review:` line nothing checks.
  `gh` can tell the panel which pull request a commit merged (`mergeCommit`), into which branch
  (`baseRefName`) and from which account (`mergedBy`), and `gh api user` the login the panel runs
  as. Every lane's merge-pr merges through that same login, and so does the owner on github.com,
  so the account says *merged from this account*, not *merged by the owner in person*.
- **E6. The installed binary.** `go version -m` on both `~/.clauductor/panel/bin/clauductor` and
  `~/.local/bin/clauductor` shows `path github.com/clauductor/clauductor/cmd/clauductor`,
  `vcs.revision=f9a5f396…`, `vcs.modified=false`. That is #201, one `framework/` change behind
  `main` (#205's `install/init.go`). `install.sh` builds in `framework/` with a plain `go build`,
  so the build stamps that information, and Go stamps `vcs.modified` true when `git status
  --porcelain` prints anything, an untracked file included. `install.Install` copies the running
  binary, **rotates the token on every install**, rewrites the plist, and boots the agent out and
  in. The plist (`renderPlist`) has `KeepAlive` with `SuccessfulExit` false, so an unclean exit
  restarts the panel and a clean one stops it. Its `ThrottleInterval` is 30. `ShouldOpenAtLogin`
  opens a browser tab on a launchd start unless one opened in the last 5 minutes. The run loop
  shuts down when its context ends (`run.go`: the HTTP server, then `stopAll`, then the `claude`
  call gate), which is what SIGTERM does.
- **E7. `go` on the agent's PATH.** `pathForAgent` (`install/launchd.go:163`) adds
  `/opt/homebrew/bin`, `~/.local/bin`, the directories of `claude`, `tmux`, `git`, `gh`, `node` and
  `jq`, and the system directories. It does not add `go`'s. On this Mac `go` is
  `/opt/homebrew/bin/go`, so the installed agent finds it already.
- **E8. Pushes.** STALL-3 (`specs/panel-stall/spec.md`) sends a `stalled` alert as a macOS
  notification and through `~/.clauductor/panel/push.json`, and pushes no other alert. Today the
  lifecycle notices (`notifyOnce`, `autoclose.go:199`) go as macOS notifications only, once per key
  per run, and only while `alerts.notify` is on. `notifyOnce` keeps its keys in auto-close's state,
  which only `pollAutoClose` creates, so it cannot be reused as it stands. This Mac has no
  `push.json`.
- **E9. Seeing a merge.** The open-PR poll (`runtime.go:266`) runs `gh pr list` every 60 s.
  `lanes_auto_close` (`autoclose.go`) already acts when a branch leaves that list and reads merged
  pull requests (`mergedPRList`). The panel already fetches when a lane starts and when Close lane
  or Remove plans (`docs/panel.md`, *When the cards may be stale*).
- **E10. The registry.** `projects.json` is decoded strictly (`config/projects.go`,
  `DisallowUnknownFields`), so a field a binary does not know makes that binary refuse the whole
  registry. A runtime keeps the entry it started with; `reconcile` restarts it only when its root
  or config path changes.
- **E11. The main checkout today:** on `main`, upstream `origin/main`, `+0 -0`, clean. The
  project's `base` is `origin/main`.

## Decisions

- **D1. What replaces the owner's review at re-trust: trust follows `main`.**
  **Recommended.** Catch-up moves the main checkout only over commits that each came through a
  pull request merged into the base from the owner's account (D3). A changed `panel.json` that
  arrives that way is trusted on arrival when all of these hold:
  - (a) catch-up is on for the project on this machine (D2);
  - (b) the config the runtime had loaded was trusted, and its bytes are the file's blob at A,
    the commit the fast-forward started from;
  - (c) the panel's own fast-forward (D3) moved HEAD from A to B, and the file on disk is its blob
    at B;
  - (d) every commit in `A..B` passed D3's check (each first-parent commit is the merge commit of
    a pull request merged into the base's branch from the login `gh api user` names);
  - (e) the new config loads (`canStart`) in the binary that will serve it: the running one, or,
    when the same merge changes `framework/` and D4 rebuilds, the new one (E2);
  - (f) `alerts.notify` is on in the new config, so a merged config cannot silence its own
    trust notice.

  The panel then records the new bytes (`TrustExact`, the page's own path). It sends one notice
  (D6) naming the pull request(s), the login, and each line of the run list (`TrustRuns`) added
  (`+`) and removed (`−`), clipped to fit, with the full list on the page's trust report. The
  notice goes **before** the restart. Then the panel restarts the project's runtime on the new
  bytes, the reload E3 describes, with the trust note "trusted on arrival from #N". Undo is a
  revert pull request, whose config arrives trusted the same way.

  **When a rebuild follows,** the decision waits for it. The running binary checks (a) to (d).
  D4's preflight then runs `clauductor.new panel config-check --config <file>`, a new command that
  loads the file and prints its `alerts.notify` and run list, for (e) and (f) and the notice's
  lines. When it passes, the bytes are recorded as trusted before the swap, and the new binary
  starts the runtime on them trusted. When the build or the preflight fails, nothing is trusted.
  The runtime keeps the config it loaded when the running binary cannot load the new one, and the
  notice names both the build's error and the config waiting for the owner.

  ((d) cannot fail after a fast-forward: D3 refuses to make one.) When (b) or (f) fails and the
  new config loads, the panel restarts the runtime on the new
  bytes untrusted. Its cards, queue commands and templates are off under the Config untrusted
  banner, and **Trust config…** is offered: the same state as a pull followed by a restart today.
  The panel log says which condition failed. The notice says the same, and that the config waits
  for the owner, while the **loaded** config's `alerts.notify` is on. When both are off, no notice
  goes. When (e) fails, the runtime keeps the config it loaded, and the notice names the error. A config the panel did not
  fast-forward to (the owner's own pull, an edit, another branch) is trusted only by the owner, as
  today, and (b) keeps such bytes from being trusted later by an unrelated merge.

  **So what replaces the owner's review, in order:**
  1. Before the merge, the same review every other line of the repository gets: the gate on the
     pull request's exact head (pr-merge-guard rules 1–2, blocking) and merge-pr's converged
     review (a skill step no hook checks).
  2. At arrival, what the panel can check itself: that every commit came through a pull request
     merged from the owner's account (D3), and that the trusted bytes it replaces are the ones it
     started from ((b)). It cannot re-check the gate receipt or the Review line (E5).
  3. After arrival, the owner reads the notice when they see it, not before the commands run.

  **What it gives up, and what the boundary is.** The owner no longer reads new argv before it
  first runs. That is accepted because the scripts a trusted config runs already change with every
  merge without the owner's review (E1). The boundary is the owner's account, not the owner in
  person (E5): a pull request written by anyone, Dependabot, a teammate or a cloud session, is
  trusted once this account merges it, and so is anything an agent session of this account merges
  through merge-pr. A commit pushed with no pull request, or merged from another account, stops
  the fast-forward (D3), so what the panel runs never moves past it without the owner's own pull.
  What stays behind the owner's own trust: a project never trusted, a config not reached through
  the panel's fast-forward, and every attached project (D2).

  **Alternatives it beat:**
  - **B: trust on arrival only when the run list is unchanged.** It would have spared the owner
    one re-trust in four (E2). Every re-trust that changes what runs would still wait for the owner.
  - **C: no trust on arrival; the notice links Trust config….** The owner's step shrinks to one
    tap, but stays, and the row asks to remove it.
  - **D: trust it when it passes the sidecar rule.** This repository's own cards fail that rule
    (E4).
  - **E: re-check gate evidence and the Review line at arrival.** Receipts go with the lane and
    agents can write them, and the Review line is unchecked prose (E5). This adds ceremony, not
    assurance.

- **D2. The switch: per machine, per project, off by default.**
  **Recommended:** a file of its own, `~/.clauductor/panel/catch-up.json`, listing the
  repository roots that have catch-up on, set by `clauductor panel catch-up --project <path>
  on|off`. With no argument it prints the state. A root not listed is off. A file of its own
  rather than a field in `projects.json`, because a binary that predates this change refuses a
  registry with a field it does not know (E10): a release build, or a `clauductor` on PATH that
  D4 declines to replace. Keyed by root, not by project id, because an id is a slug of the
  config's name and can be taken again after a removal (`install/projects.go`); `clauductor panel
  remove` also clears the root. The panel reads the file at each trigger (D3), so turning
  catch-up off or on takes effect without a restart. It is refused, and nothing is written, for a
  project whose config is outside its repository (`configOutside`, as ATTACH-2 requires). The
  panel checks that again at each trigger, because `panel add` can move a registered root's
  config outside it in place; such a project is skipped with one notice. The owner turns catch-up
  on for this repository after the build merges.
  **Alternative:** a `panel.json` key, as `lanes_auto_close` is. Rejected: a repository could then
  grant itself writes to the owner's checkout and trust on arrival. The key would sit under the
  same trust it relaxes, and a team would share one machine's choice.

- **D3. The fast-forward.** **Recommended.** Catch-up runs on a machine-level source of the
  liveSet, as `pollRegistry` does (E3), never on a project runtime's own goroutines. Two things
  wake it:
  - a pull request leaving a project's open list, which the open-PR poll signals without waiting
    (E9);
  - the panel's start.

  For each project with catch-up on, it reads with `git -C <root>` on the main checkout that:
  - HEAD is the local branch of the base (`origin/main` gives remote `origin`, branch `main`),
    tracking it;
  - no tracked change, and no merge, rebase or cherry-pick in progress;
  - the branch has no commit the remote lacks.

  The panel then runs `git fetch <remote> <branch>`. Before it merges, every first-parent commit
  in `HEAD..<remote>/<branch>` must be the merge commit of a pull request merged into `<branch>`
  from the login `gh api user` names. The lookup goes through GitHub's commit-to-pull-request
  route, not its search index, which can lag a fresh merge: `gh api
  repos/{owner}/{repo}/commits/<sha>/pulls`, then `gh api repos/{owner}/{repo}/pulls/<n>` for
  `merged_by`, `base.ref` and `merge_commit_sha`. Only then does it run `git merge --ff-only
  <remote>/<branch>`. Each git and `gh` call has a 60 s timeout. A base not in `<remote>/<branch>`
  form is refused. Untracked files stay; if the fast-forward would overwrite one, git refuses, and
  so does the panel.

  Each refusal sends one notice, keyed on the project, the reason and the remote's commit, once
  per panel run, and puts a line in the panel log. A commit that fails the pull-request check is
  named, so the owner can pull past it by hand. A **transient** refusal (a git or `gh` call that
  fails or times out) wakes the source again, at most 3 times, 1, 3 and 6 minutes after it. A
  refusal of policy (the checkout's state, a commit that fails the check) waits for the next
  trigger. The retries exist only while a transient refusal is pending, so the idle panel still
  makes no network call.

  The notices go through a catch-up notifier with its own keys, created with the source, under
  the project's `alerts.notify` and D6's push rules. It does not reuse `notifyOnce`, whose keys
  live in auto-close's state (E8).

  **Alternatives:**
  - **Fast-forward over any commit, and check pull requests only for `panel.json`.** Rejected: the
    trusted cards run scripts and D4 builds code from every commit, so the boundary would be write
    access to `main`, not the owner's account.
  - **A periodic fetch,** every 10 minutes say. It would catch a push to `main` with no pull
    request, but it costs a network call per tick while nothing happens (PANEL-7's idle cost).
    The start trigger catches those pushes at the next start.

- **D4. The rebuild and restart.** **Recommended.** After a fast-forward from A to B, the panel
  first asks whether this project is its own source: its build information
  (`debug.ReadBuildInfo`) names the module `<root>/framework/go.mod` declares. When it does not,
  as for any other project with a `framework/` folder, it does nothing more and sends nothing. For
  its own source, it rebuilds when all of these hold:
  - it runs as the login agent (`--launchd`);
  - its `vcs.revision` R is a commit of the repository and an ancestor of B (`git merge-base
    --is-ancestor`), so a build the owner installed from another branch is not replaced by `main`;
  - `vcs.modified` is false;
  - `git diff --quiet R B -- framework/` reports a difference;
  - `git status --porcelain` in the main checkout prints nothing, so the new build is stamped
    unmodified (E6).

  The panel runs `go build -o ~/.clauductor/panel/bin/clauductor.new ./cmd/clauductor` in
  `<root>/framework`, with a 5-minute timeout and `go` from its PATH. The build writes nothing in
  the repository (Go's caches are outside it). The preflight requires:
  - the new binary's build information names the same module, `vcs.revision` B, and
    `vcs.modified` false, so the next merge can rebuild again;
  - its `version` and `panel list` (which reads `projects.json`) both exit 0;
  - when the merge changed the in-repository `panel.json`, its `panel config-check` on the file
    (D1 (e) and (f)).

  The installed binary is renamed to `clauductor.prev` and the new one renamed into its place.
  The `clauductor` on the panel's PATH is replaced the same way, keeping its own `.prev`, only when
  its build information names the same module and a commit of the repository. The token, the
  plist and the user hooks are not touched.

  When none of the panel's own `claude` calls is running (the call gate), or after 10 minutes,
  the panel restarts:
  - It sends the notice: the old and new commits, and "if the panel is not back in 2 minutes:
    `mv ~/.clauductor/panel/bin/clauductor.prev ~/.clauductor/panel/bin/clauductor`, then
    `launchctl kickstart -k gui/$(id -u)/<label>`".
  - It records a browser open, so the restart opens no tab (E6).
  - It ends the run's context, as SIGTERM does, with exit status 75 recorded, so the run loop
    shuts down as it always does. `clauductor panel` then exits 75 itself (`os.Exit`, as
    `lock-run` does in `internal/cmd/lockrun.go`), since `main.go` turns every returned error into
    exit 1. launchd starts the new binary (E6). Nothing on a source or runtime goroutine waits for
    the shutdown it started.
  - The new binary's first start sends "now running <commit>".

  If the merge's diff touches `framework/internal/panel/install/launchd.go` or `hooks.go` (what
  `clauductor panel install` writes beyond the binary), the notice also says to run
  `clauductor panel install` once. A failed build or preflight leaves the running binary and no
  `.new` file, and sends one notice with the error. A panel of this source started by hand, or
  one whose build does not qualify, is not rebuilt, and one notice says how to update it. `go`
  joins `pathForAgent`'s
  tools for later installs.
  **Alternatives:**
  - **Run `clauductor panel install` from the new binary.** Rejected: it rotates the token (E6),
    which signs every open page out after every framework merge.
  - **Roll back on its own.** Rejected: the binary that would have to judge the new one is the one
    being replaced, and a watcher process is a second thing to own. The pre-restart notice is the
    addressee instead.
  - **Leave the binary to the owner and catch up only the checkout and the config.** It is
    smaller, but the reinstall step stays (D7).

- **D5. merge-pr step 5 and its report.** **Recommended:** step 5 (`template/.claude/skills/merge-pr/SKILL.md`)
  confirms the squash commit is on the remote's base (`git fetch` and `git log -1`). It no longer
  pulls the main checkout, which fails from a lane (#164). The skill's report names the owner's
  step (pull; after a `framework/` change, `./install.sh` and `clauductor panel install`; after a
  `panel.json` change, `clauductor panel trust`) unless `clauductor panel catch-up` reports catch-up
  on for the project. Without `clauductor` on PATH, it names the step, so the repository still works
  without clauductor. This is a template change, so the plugin is rebuilt and this repository
  updated (`clauductor update`). **Alternative:** leave the report to OPS-108's item (6). Rejected:
  step 5 fails from a lane today, and this change is what decides when the owner's step applies.

- **D6. The notices go as a macOS notification and through the owner's push command.**
  **Recommended:** every catch-up notice goes through the catch-up notifier (D3) as a macOS
  notification and, when `push.json` holds a valid command, through it too. The same file, argv,
  timeout and `alerts.notify` rules apply, and the title names the project only while its config
  is trusted. STALL-3 says the push carries no alert but `stalled`, so this modifies STALL-3 (the
  delta in `specs/panel-stall/`) and adds STALL-3-S12. **Alternative:** a macOS notification
  only. That leaves STALL-3 untouched, but trust on arrival (D1) and the pre-restart warning (D4)
  would reach only a Mac someone is looking at. This Mac has no `push.json` today (E8), so until
  the owner adds one, the two choices notify the same way.

- **D7. Size, budget and scope.** **Recommended:** one change of four groups (switch and
  fast-forward; trust on arrival and the push; rebuild and restart; merge-pr and the docs), at Risk
  high (the trust boundary, and a binary that replaces itself), with a hard budget of $120. The
  row estimates 0.5 session; with D4, about 1 is more likely. The row carries no budget, but
  session 8's plan calls OPS-118 "the next change with a hard budget". Past panel changes cost
  $47 (PANEL-49), $71 (PANEL-42, its trust work the nearest kin), $253 (PANEL-51) and $280
  (PANEL-48). **Alternative:** split D4 into its own row, the budget about $70. The reinstall step
  would stay until that row builds.

## Refusals
| Situation | What happens instead |
|---|---|
| Catch-up is off for the project (the default) | Nothing: no fetch, no merge, no trust, no build. The owner's step stands, and merge-pr's report names it (D2, D5) |
| Catch-up is turned off while the panel runs | The next trigger reads `catch-up.json` and does nothing (D2) |
| The project is attached (its config outside its repository) | `panel catch-up … on` is refused, and nothing is written (D2, ATTACH-2) |
| A root with catch-up on is re-added with its config outside it | Skipped at each trigger: no fetch, no merge. One notice says why (D2) |
| A project is removed | `panel remove` clears its root, so a project added there later starts with catch-up off (D2) |
| The main checkout has a tracked change, another branch, a detached HEAD, a commit of its own, or an operation in progress | Left as it is. One notice and a log line name the checkout and the reason (D3) |
| A commit on the base came with no pull request, from a pull request merged from another account, or `gh` cannot say (offline, rate limit) | No fast-forward. The notice names the commit and the reason, and the owner pulls past it by hand (D3) |
| The fast-forward would overwrite an untracked file | git refuses. The checkout is as it was, and the notice names git's error (D3) |
| The fetch, the merge or a `gh` lookup outlasts 60 s or fails | As a refusal, tried again 1, 3 and 6 minutes later, then at the next trigger (D3) |
| The base is not `<remote>/<branch>` | Refused, with a notice (D3) |
| The loaded config was not trusted, or its bytes are not the file's blob where the fast-forward started | The new bytes are served untrusted, Trust config… offered (D1 (b)) |
| `alerts.notify` is false in the new config | Nothing is trusted on arrival, and the new bytes are served untrusted. The fast-forward and the rebuild still run, and the page and log say what happened; a notice goes only while the loaded config's `alerts.notify` is on (D1 (f)) |
| The new config does not load | The runtime keeps the config it loaded, trusted or not. The notice names the error (D1 (e)) |
| The same merge changes `framework/` and `panel.json` | Trust waits for the rebuild: the new binary's `panel config-check` decides (e) and (f). A failed build trusts nothing (D1, D4) |
| The owner pulled the main checkout by hand | The panel made no fast-forward, so nothing arrives: today's rules (D1 (c)) |
| The panel runs by hand, not as the login agent | No rebuild. One notice says `framework/` changed and how to rebuild and restart (D4) |
| The running binary's module is not the project's `framework/go.mod` module | Not the panel's source: nothing more, and no notice (D4) |
| The running binary has no VCS build information, local changes, or a commit that is not an ancestor of the merge | No rebuild. One notice says why (D4) |
| The main checkout holds an untracked file at build time | No rebuild, since the build would be stamped modified. One notice names the file (D4) |
| `go` is not on the panel's PATH | The build fails, with a notice naming `clauductor panel install` (which adds `go`'s directory) (D4) |
| The build or the preflight fails, or takes over 5 minutes | The running binary stays, no `.new` is left, and one notice names the error (D4) |
| The merge changed `install/launchd.go` or `install/hooks.go` | Rebuilt as usual, and the notice says to run `clauductor panel install` once (D4) |
| A panel-spawned `claude` call is running | The restart waits for it, up to 10 minutes (D4) |
| The new binary does not come back | launchd retries every 30 s. The pre-restart notice names the rollback (D4) |
| Hooks sent while the panel restarts | Lost, as at any restart. Stalls are approximate for one stall time after it (STALL-1) |
| No `push.json` | A macOS notification only, as for a stall (D6, STALL-3) |
