# Tasks: panel-28-login-refresh

## Progress
- 2026-10-02 proposed; nothing built yet

## Decision log
- (none yet)

## 1. The facts: a lock watch, a log, and the build-time checks
- [ ] 1.1 Re-read the installed Claude Code (its version from the symlink at `command -v claude`, never by running it) for the lock path and options, the owner record, the waiter's retries, and which subcommands reach `refreshOAuthTokenIfNeeded`; record what differs from design.md's *What the installed Claude Code does* in the Decision log
- [ ] 1.2 A lock watch in the machine (`framework/internal/panel/loginlock.go`): resolve the config dir (`$CLAUDE_CONFIG_DIR`, else `~/.claude`), stat `.oauth_refresh.lock` and parse `.oauth_refresh.lock.owner` every 2 s, defensively (an unparseable record is "holder unknown"); never write either. Tested in `framework/internal/panel/loginlock_test.go` against a temp config dir citing [CLAUDECALLS-3-S3] and [CLAUDECALLS-5-S3]
- [ ] 1.3 Track the pids of the panel's own `claude` children (the Runner starts and waits rather than `Run`s, for `claude` argv only), and append to `login-lock.jsonl` in `config.PanelDir`, capped in size: one line per lock seen (holder pid, `ps -o command=`, own child or not and which call, first seen, last touched, how it ended). Tested in `loginlock_test.go` citing [CLAUDECALLS-4-S1] and [CLAUDECALLS-4-S2]
- [ ] 1.4 D6's checks, run by hand once in a trusted project, each run the same call the panel already makes: `claude agents --json` with and without the candidate variable, comparing its debug output for the feature-flag client and its JSON for the fields `signals.ParseAgents` reads; the same JSON comparison for `claude auth status --json` against `signals.ParseAuthStatus`. Record the result and the variable chosen, or why none passed, in the Decision log (manual: this is evidence about the installed Claude Code, not panel behaviour a test can reach)

## 2. Claude calls that can't strand the lock
- [ ] 2.1 A `claude` gate around the Runner (`framework/internal/panel/signals/claudegate.go`), matching argv whose program is `claude` directly or after `/usr/bin/env -u …`: `Cmd.Cancel` sends SIGTERM and `Cmd.WaitDelay` is 15 s (D2); the caller's timeout is unchanged and it returns at once; other commands are untouched. Tested in `framework/internal/panel/signals/claudegate_test.go` with a fake `claude` script on `PATH` citing [CLAUDECALLS-1-S1], [CLAUDECALLS-1-S2] and [CLAUDECALLS-1-S3]
- [ ] 2.2 Log each stopped call (the call, the signal, whether a lock existed) to `login-lock.jsonl`, tested in `claudegate_test.go` citing [CLAUDECALLS-4-S3]
- [ ] 2.3 One machine-wide slot (D3): `pollAgents`, `checkFilter`, `pollAccount` and `pollVersion` try for it and skip, keeping their value with a reason in the source status; the lane manager's `claude agents` reads wait for it within their context. Tested in `framework/internal/panel/claude_slot_test.go` (two projects' runtimes and a lane action against a blocking fake runner) citing [CLAUDECALLS-2-S1] and [CLAUDECALLS-2-S2]
- [ ] 2.4 No call while the lock is live (D4): polls skip with "a login refresh is in progress", lane actions wait within their context; a lock untouched for 60 s or more stops nothing. Tested in `claude_slot_test.go` with a temp config dir citing [CLAUDECALLS-3-S1] and [CLAUDECALLS-3-S2]
- [ ] 2.5 `go test -race ./internal/panel/...` with `leakcheck` passes: no fake `claude` outlives a test

## 3. The panel's own calls with the feature-flag client off (conditional, D6)
- [ ] 3.1 If and only if 1.4 passed both checks: `claude agents` (every call site) and `claude auth status` run with the chosen variable set, through one helper beside `lanes.ScrubbedArgv`, tested in `framework/internal/panel/lanes/scrubbed_test.go` by asserting the argv. If 1.4 did not pass, tick this as "not applied: <reason>" and change nothing

## 4. The owner sees a stuck refresh
- [ ] 4.1 An alert kind in `framework/internal/panel/state/alerts.go`, raised when one lock (same birth time) stays live for more than 2 min, naming the holder, its command and age, and what to do; a stale lock raises none and shows as a diagnostics line. Tested in `framework/internal/panel/state/alerts_test.go` citing [CLAUDECALLS-5-S1] and [CLAUDECALLS-5-S2]
- [ ] 4.2 `docs/panel.md`: the source table and *The account and its quota* say how `claude` calls are stopped, serialized and paused; a section on the login lock covering the alert, the diagnostics line, `login-lock.jsonl`, and that the panel never removes the lock; the in-page Help says what the alert means
- [ ] 4.3 [CLAUDECALLS-5-S1] checked once in a throwaway panel with `CLAUDE_CONFIG_DIR` pointed at a scratch directory where a script touches a fake lock every 5 s (manual: the alert's rendering needs a browser and a live panel; the derivation is unit-tested in 4.1)

- [ ] Slice: an owner running agents alongside the panel never has one die because the panel stranded or contended for the login refresh, and sees a stuck refresh as an alert in the panel
