# Tasks: ops-118-main-catches-up

## Progress
- 2026-10-22 proposed (session 9); nothing built yet. The build starts from `main` once the owner
  approves, in a later session. The roadmap row stays `⬜ queued` until its build PR opens.
- 2026-10-22 group 1 built (the switch, its command, and the fast-forward on the liveSet's
  catch-up source); groups 2 to 4 not started. Groups 2 and 3 hang off `catchUpResult` (the
  branch, the old and new commits, the pull requests and the login), which `fastForward` returns.
- 2026-10-22 group 1 (The switch and the fast-forward (D2, D3)) built and reviewed: converged in 1 round(s), peak low; grades R1 pass

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

## 1. The switch and the fast-forward (D2, D3)
- [x] 1.1 `framework/internal/panel/config/catchup.go` (new): read and write `~/.clauductor/panel/catch-up.json`, a list of repository roots; absent or unlisted reads off; written atomically. Tested in `framework/internal/panel/config/catchup_test.go` (new). Citing [CATCHUP-1-S1].
- [x] 1.2 `framework/internal/cmd/panel.go`: `clauductor panel catch-up --project <path> [on|off]` beside `panel trust`, writing only `catch-up.json`, refused for an attached project (`configOutside`, in `install/projects.go`) with nothing written; `RemoveProject` (`install/projects.go`) clears the root. Tested in `framework/internal/cmd/panel_catchup_test.go` (new), with `projects.json` compared byte for byte and `catch-up.json` checked absent after a refusal, and a remove then a fresh add at the same root. Citing [CATCHUP-1-S1], [CATCHUP-1-S2], [CATCHUP-1-S7].
- [x] 1.3 A test that a `panel.json` setting `catch_up` fails to load and names the key (the strict decoder, `config.go`). Tested in `framework/internal/panel/config/config_test.go`. Citing [CATCHUP-1-S4].
- [x] 1.4 `framework/internal/panel/catchup.go` (new): the fast-forward of D3 on a machine-level source of the liveSet beside `pollRegistry` (`live.go`), woken without blocking by the open-PR poll when a pull request leaves the open list (`runtime.go`, beside `lanes_auto_close`'s hook in `autoclose.go`) and once at the panel's start; `catch-up.json` read and `configOutside` checked at each wake; every precondition read with `git -C <root>`; `base` read through `BaseRef()` and split as `<remote>/<branch>`; after the fetch, the commit C that `<remote>/<branch>` names read once (`git rev-parse`), each first-parent commit of `HEAD..C` looked up with `gh api repos/{owner}/{repo}/commits/<sha>/pulls`, then `gh api repos/{owner}/{repo}/pulls/<n>` (`merged_by`, `base.ref`, `merge_commit_sha`), against `gh api user`'s login, before `merge --ff-only C` (C's hash, never the ref's name); each git and `gh` call under a 60 s timeout; a transient failure (`git fetch` or `gh` erroring or timing out, or any git call timing out) woken again at 1, 3 and 6 minutes (on the clock the tests drive), and git's own refusals not retried; a catch-up notifier of its own (keys of project, reason and remote commit, created with the source; not `notifyOnce`) and a panel log line for each refusal, the refusal's notice (and the skip of a config outside its repository) sent whatever `alerts.notify` says, titled "clauductor panel" and naming the checkout's path. Tested in `framework/internal/panel/catchup_integration_test.go` (new), with real git repositories (a bare remote and a clone), a fake `gh` written by `internal/testbin` (for [CATCHUP-2-S10], one whose first lookup moves the clone's `origin/main` with `git update-ref` to a commit after C, as another worktree's fetch would), and `internal/treesnap` for "as before". Citing [CATCHUP-1-S3], [CATCHUP-1-S5], [CATCHUP-1-S6], [CATCHUP-2-S1], [CATCHUP-2-S2], [CATCHUP-2-S3], [CATCHUP-2-S4], [CATCHUP-2-S5], [CATCHUP-2-S6], [CATCHUP-2-S7], [CATCHUP-2-S8], [CATCHUP-2-S9], [CATCHUP-2-S10], [CATCHUP-2-S11] (its push read from the stand-in program `push_test.go` uses).

## 2. Trust on arrival, and the push (D1, D6)
- [ ] 2.1 `framework/internal/panel/catchup.go`: after a fast-forward from A to B that changed the in-repository `panel.json`, on the same liveSet source, the checks of D1 (a) to (f): the loaded config's trust and its bytes against the blob at A, the file on disk against the blob at B (`git cat-file`), D3's pull-request check having passed for `A..B`, `canStart`, and the new config's `alerts.notify`; when a rebuild follows (task 3.1), (e) and (f) come from the new binary's `panel config-check` and the trust is recorded before the swap. On success: `install.TrustExact`, the notice, then the reload `liveSet.Trust` does (`canStart`, `restartStop`, `startEntry`, `live.go`) with the note "trusted on arrival from #N" (`markTrusted`); otherwise the untrusted restart or the kept config, with the reason in the panel log; the notice that the config waits for the owner sent only while the loaded config's `alerts.notify` is on, and the notice of a config that does not load sent whatever it says; when the build or its preflight fails, the running binary's own load decides between the two, and the build's error goes in the rebuild's own notice (task 3.2), a second notice. The notice's `+` and `−` lines are the difference between the old and new `TrustRuns` lists. Tested in `framework/internal/panel/catchup_trust_integration_test.go` (new), with a fake `gh` and a stand-in new binary written by `internal/testbin`. Citing [CATCHUP-3-S1], [CATCHUP-3-S2], [CATCHUP-3-S3], [CATCHUP-3-S4], [CATCHUP-3-S5], [CATCHUP-3-S6], [CATCHUP-3-S7], [CATCHUP-3-S8], [CATCHUP-3-S9], [CATCHUP-3-S10].
- [ ] 2.2 `framework/internal/panel/install/trust.go`: a trust record may carry the note it was made with, so the page's trust report shows "trusted on arrival from #N" after a restart. Tested in `framework/internal/panel/install/trust_test.go`. Citing [CATCHUP-3-S1].
- [ ] 2.3 `framework/internal/panel/push.go`: a catch-up notice runs the push command under STALL-3's rules, `alerts.notify` withholding only the notice that a config waits for the owner; only the trust notice titled with a name (the config it trusted), every other catch-up notice (the config waiting, refusals, a config that does not load, the rebuild's of task 3.2) titled "clauductor panel" and naming the checkout's path; a `waiting` alert still never pushes. Tested in `framework/internal/panel/push_test.go` with its stand-in program. Citing [STALL-3-S12], and re-running [STALL-3-S1], [STALL-3-S6], [STALL-3-S10], [STALL-3-S11] unchanged.

## 3. The rebuild and the restart (D4)
- [ ] 3.1 `framework/internal/panel/install/selfupdate.go` (new): from `debug.ReadBuildInfo`, the module, `vcs.revision` and `vcs.modified`; the module `<root>/framework/go.mod` declares, checked first, with nothing sent when it differs; whether R is a commit of the repository and an ancestor of B, and `framework/` differs between R and B; `git status --porcelain` empty; the build into `clauductor.new` under a 5-minute timeout; the preflight (the new binary's build information read with `debug/buildinfo`: the same module, `vcs.revision` B, `vcs.modified` false; then `version`, `panel list`, and `panel config-check` when the merge changed `panel.json`); the two renames for the installed binary and, when its own build information qualifies, the one on PATH. The token, the plist and the hooks are never opened for writing. `framework/internal/cmd/panel.go`: the `panel config-check --config <file>` command, printing the config's `alerts.notify` and run list as JSON, or failing with the load's error. Tested in `framework/internal/panel/install/selfupdate_test.go` (new) with a throwaway module and repository, a stand-in `go` from `internal/testbin` for the failure cases, and `internal/treesnap` over the repository; `config-check` in `framework/internal/cmd/panel_catchup_test.go`. Citing [CATCHUP-4-S1], [CATCHUP-4-S2], [CATCHUP-4-S3], [CATCHUP-4-S5], [CATCHUP-4-S9], [CATCHUP-4-S10].
- [ ] 3.2 `framework/internal/panel/run.go`: under `--launchd` only, the restart: wait until no `claude` call of the panel's runs (the call gate), up to 10 minutes; the notice naming R, B and the rollback, with `clauductor panel install` when `R..B` (the build's range, not the last merge's) changed `install/launchd.go` or `install/hooks.go`; every notice of the rebuild, this one, its refusals and the new binary's, sent whatever `alerts.notify` says, titled "clauductor panel"; a marker file for the new binary's "now running" notice; the browser open recorded as `ShouldOpenAtLogin` records it; then the run's context ended, as SIGTERM ends it, with exit status 75 recorded and returned once the run loop's shutdown is done. `framework/internal/cmd/panel.go` exits the process with that status (`os.Exit`, as `internal/cmd/lockrun.go` does), since `cmd/clauductor/main.go` turns every error into 1. A hand-started panel sends the "update yourself" notice instead. Tested in `framework/internal/panel/run_test.go`, which drives the restart path and then a second start in the same home and asserts it opens no tab, and in `framework/internal/cmd/panel_catchup_test.go`, which runs the built command as a child process and reads its exit status. Citing [CATCHUP-4-S1], [CATCHUP-4-S4], [CATCHUP-4-S6], [CATCHUP-4-S7], [CATCHUP-4-S8], [CATCHUP-4-S11].
- [ ] 3.3 `framework/internal/panel/install/launchd.go`: `go`'s directory joins `pathForAgent`'s tools. Tested in `framework/internal/panel/install/launchd_test.go`.
- [ ] 3.4 On the owner's Mac, after the build merges: catch-up on for this repository, then a small `framework/` PR merged from a lane; the installed binary's `go version -m` names its merge commit, `clauductor.prev` is the old one, and the page's token still works. [CATCHUP-4-S1] (manual: the real login agent and launchd's restart, which no test may touch: the owner's live panel).

## 4. merge-pr, the docs and the template (D5)
- [ ] 4.1 `template/.claude/skills/merge-pr/SKILL.md` step 5: find the main checkout (the first `worktree` entry of `git worktree list --porcelain`) and ask `clauductor panel catch-up --project <that root>`, catch-up counting as on only when it exits 0 and prints `on` (anything else, an older `clauductor` included, is off); with catch-up on, confirm the squash commit on the remote's base (`git fetch`, `git log -1`) and do not pull; with it off, `pull --ff-only` only when run from the main checkout itself (`git rev-parse --git-dir` equal to `--git-common-dir`), and from a lane confirm without pulling; the report names the owner's step unless catch-up is on, leaving out the pull when step 5 made it. `template/.claude/checks/change-process.sh` holds step 5 to those three branches: the root from `git worktree list --porcelain`, the "exits 0 and prints `on`" test, the `--git-common-dir` test before any `pull --ff-only`, and no pull while catch-up is on. Then `scripts/build-plugin.sh`, and `framework/clauductor update` from the repository root with the binary built from this checkout.
- [ ] 4.2 `docs/panel.md`: a *Catch up after a merge* section (the switch, the fast-forward and its refusals, trust on arrival and what it rests on, the rebuild and its rollback); *Config trust* and *Add, trust and remove from the page* name the second way a config is trusted; *When the cards may be stale* says catch-up clears it; *Notifications* names the catch-up notices and the push; *The launchd agent* names exit status 75 and `clauductor.prev`.
- [ ] 4.3 `docs/roadmap.md`'s session-plan bullet *After each merge the owner* says the step applies only while catch-up is off, and the build PR references #164 without closing it.

- [ ] Slice: an owner can merge a PR from a lane and find the main checkout, the panel's config and the installed panel on the merged main without running a command, at the panel
