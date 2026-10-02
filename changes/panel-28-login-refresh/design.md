# Design: panel-28-login-refresh

## What the panel runs today

Every `claude` the panel spawns goes through one `signals.Runner`, `signals.ExecRunner`
(`framework/internal/panel/signals/run.go`), set once in `run.go` and shared by the runtimes and
the lane manager (`run.go`, `LaneManager{… Run: o.Runner}`):

| Call | Where | When | Timeout |
|---|---|---|---|
| `claude agents --json [--cwd <dir>]` | `runtime.go` `pollAgents` | every 2 s with a lane, 5 s while hooks flow, 15 s when quiet, per project | 10 s |
| `claude agents --json`, then with `--cwd` | `runtime.go` `checkFilter` | every 5 min per project (the filter cross-check) | 10 s each |
| `claude agents --json` | `lanes/lanes.go` `agentStatus`, the first-prompt check, `liveSessions`; `lanes/removewt.go` `removeVerdict` | lane actions | 10 s, or the caller's context |
| `claude auth status --json` (through `/usr/bin/env -u …`, `lanes.ScrubbedArgv`) | `machine.go` `pollAccount` | at start, then every 10 min | 10 s |
| `claude --version` | `machine.go` `pollVersion` | at start, then every 10 min | 10 s |

`ExecRunner` uses `exec.CommandContext` with no `Cancel` and no `WaitDelay`, so a timeout, or the
panel's own shutdown (`cmd/panel.go` cancels on SIGTERM), sends the child SIGKILL.

`claude auth status` feeds only the status bar: the account's mode and plan, and a hash of the org
id that drops a saved quota belonging to another account (`state/quota.go`, `ApplyAccount`). No
lane decision reads it. *Subscription only* reads the environment, not this.

## What is known about Claude Code's refresh

Each claim says where it comes from and whether it was checked. **Read** means read from the
strings of the installed binary, Claude Code 2.1.288 (`~/.local/share/claude/versions/2.1.288`,
a Bun build with its JavaScript embedded), while drafting, with nothing run. **Source** means a
page fetched and read on 2026-10-02. A GitHub issue is a third party's report: it was read, and it
agrees or not with the binary, but nobody at Anthropic has confirmed it.

| Claim | Where from | Checked |
|---|---|---|
| The refresh lock is `<config dir>/.oauth_refresh.lock`, with options `stale: 60000, update: 5000`: the holder touches it every 5 s, and a lock untouched for 60 s counts as stale | Read; the same options are quoted from 2.1.272–2.1.274 in [#95236](https://github.com/anthropics/claude-code/issues/95236) | Yes: the binary and the issue agree |
| A legacy lock, `<config dir>.lock`, is taken as well | Read; #95236 names a "SECOND (legacy) lock" | Read only |
| The holder writes `<config dir>/.oauth_refresh.lock.owner`, JSON with `pid`, `procStart` and `lockBirthtimeMs` | Read | Read only |
| A waiter retries 5 times, 1–2 s apart, waits until about 7.5 s for the lock to change, tries a dead-holder takeover (only when the owner record proves the holder gone, and behind a feature flag), then fails with this row's error | Read | Read only. [errors.md](https://code.claude.com/docs/en/errors.md) says instead that the waiter "times out after 60 seconds" |
| `claude agents --json` starts the feature-flag client without waiting. The client's `createClient` calls `refreshOAuthTokenIfNeeded` first, bounded at 5 s, in a trusted folder: the same function that takes the lock | Read | Read only. Not in #95822, which doesn't mention `claude agents` |
| Every command awaits `init` in the root `preAction` hook. `init` starts an OAuth refresh without awaiting it when the access token is within 5 minutes of expiry or expired. `auth status` then calls `process.exit` directly, skipping the 2 s wait for a refresh in flight. If the refresh POST reached the server, the refresh token is rotated but the new pair is never saved, and the login is spent ("Login expired") | [#95822](https://github.com/anthropics/claude-code/issues/95822), open, filed 2026-09-21 against 2.1.277 | Read in the issue; consistent with the binary's root `preAction` hook, not traced in it |
| A directory lock left by a mid-refresh exit was never reclaimed despite `stale: 60000`, and every call failed until it was deleted by hand | #95236, open, filed 2026-09-17, on Windows | Read in the issue. Whether macOS behaves the same is unknown. The 2026-10-02 lock outliving `/login` is consistent with it |
| A lock stamped in the future (the clock stepped back mid-refresh) blocks every refresh until real time passes it | [#95739](https://github.com/anthropics/claude-code/issues/95739), open, 2.1.267 | Read in the issue |
| `CLAUDE_CONFIG_DIR` gives a directory its own settings, history and login. On macOS it keys the Keychain entry to that directory, so a session with another `CLAUDE_CONFIG_DIR` reads another entry | [authentication.md](https://code.claude.com/docs/en/authentication.md), *Log in with multiple accounts* and *Credential management* | Yes: documented |
| The documented fix for this error: wait 60 s, kill stray `claude` processes, "remove the stale lock file at `~/.claude/token.lock`", run `/login` | errors.md, *Could not refresh your login* | Documented, but the file it names isn't the lock 2.1.288 uses |
| `claude agents --json` prints active sessions and exits; `--cwd` narrows it | [agent-view.md](https://code.claude.com/docs/en/agent-view.md) | Yes: documented |
| No status-line field gives the auth mode or the plan | [#95598](https://github.com/anthropics/claude-code/issues/95598) (open request for an `auth_mode` field) | Yes, as of 2026-10-02 |

### So: two hazards, and the panel feeds both

1. **A stranded lock.**
   - **How:** a process that holds the lock and dies without releasing it makes every waiter fail.
     If #95236 holds on macOS, they keep failing until someone deletes the lock.
   - **The panel's part:** it SIGKILLs every `claude` at 10 s, and SIGKILL allows no release.
2. **A spent login.**
   - **How:** a short-lived command can start a refresh at startup and exit before saving the
     result (#95822).
   - **The panel's part:** it runs short-lived `claude` commands on a timer. `auth status` is the
     one #95822 names. `claude agents --json` runs every 2 s and goes through the same startup.

Between them they fit everything seen:
- agents dying on "another Claude Code process is refreshing it (or exited mid-refresh)";
- a lock still there at 10:12 on 2026-10-02, after `/login`.

### What is Anthropic's to fix, and what the panel can avoid

- **Anthropic's** (reported upstream already, except the last):
  - short-lived commands that start a refresh and exit without saving it (#95822);
  - a directory lock that is never reclaimed (#95236);
  - a future-dated lock (#95739);
  - errors.md naming `~/.claude/token.lock` and a 60 s waiter, where 2.1.288 uses
    `.oauth_refresh.lock` and gives up after about 7.5 s.

  This change fixes none of them. Whether to add our evidence to those issues is the owner's
  decision (Out of scope).
- **The panel's:**
  - SIGKILL (D2);
  - several calls of its own at once (D3);
  - calls while a refresh holds the lock (D4);
  - `auth status` on a timer (D5);
  - possibly the feature-flag refresh in `claude agents` (D6);
  - leaving the owner blind to a stuck or stranded lock (D7).

## The shape of the change

1. **Group 1, the facts.**
   - A lock watch: a stat of the lock and its owner record every 2 s (a stat spawns nothing).
     Pids of the panel's own `claude` children are tracked, and `login-lock.jsonl` goes in the
     panel's directory (`config.PanelDir`, beside `quota.json`), capped in size. It gets one line
     per lock seen (holder pid, `ps -o command=` of the holder, whether it is a panel child, first
     seen, last touched, how it ended) and one line per panel `claude` call that had to be stopped.
   - The sandbox experiment (D1), which decides group 3.
   - A re-read of the then-installed binary, recording what differs from the table above.
2. **Group 2, calls that can't strand the lock.**
   - A `claude` gate wrapping the Runner, for any argv whose program is `claude` (directly or
     after `/usr/bin/env -u …`):
     - SIGTERM, then grace (D2);
     - one call at a time (D3);
     - no call while the lock is live (D4).

     Each call site keeps its own timeout for its result. The gate decides only how the process
     is stopped and whether it starts.
   - `auth status` off the timer (D5).
3. **Group 3, conditional (D6).** What group 1's sandbox shows picks one pre-decided response.
4. **Group 4, the owner sees a stuck or stranded lock** (D7): the alerts, a diagnostics line, the
   docs.

## Refusals

| Situation | What happens instead |
|---|---|
| A poll while another panel `claude` call is in flight | Skipped. The source keeps its last value and says why; the next tick tries again. |
| A poll while the lock is live (touched in the last 60 s) | Skipped, as above, with "a login refresh is in progress". |
| A lane action needing `claude agents` while a call is in flight or the lock is live | It waits within its own timeout, then runs. If the timeout passes first, it fails as it does today when `claude agents` can't be read. |
| A `claude` call past its timeout | SIGTERM; SIGKILL only after the 15 s grace. The caller gets its timeout error at once. |
| The panel shutting down with a `claude` call in flight | The same SIGTERM, then grace. The panel doesn't wait past the grace. |
| The account wanted while a reading under 24 h old is saved | The saved reading is used; no `auth status` runs until **Refresh**. |
| A lock held live for more than 2 min | An alert naming the holder (pid, command, age) and what to do. |
| A stale lock (untouched for 60 s or more) still present 2 min later | A warning naming the lock, its last holder and whether that process is gone, plus the documented remedy, which the owner carries out. |
| Any lock, live or stale | The panel never removes or writes it, or its owner record. |
| An owner record the panel can't parse (a new Claude Code format) | The lock is still watched and alerted by its directory's times. The holder is reported "unknown". |

## Decisions (awaiting the owner)

**D1. How group 1 establishes the facts.**
- **Recommended:** a sandbox login, plus the passive lock watch.
  - **The sandbox:** the owner logs in once under a scratch `CLAUDE_CONFIG_DIR` (documented to
    have its own Keychain entry), then leaves it unused until its access token has expired. An
    unused login always has an expired access token, so per #95822 every short-lived command run
    against it must refresh.
  - **The runs:** the build runs the panel's exact argv against the sandbox, one at a time, with
    the sandbox's lock dir watched. That is `claude agents --json`, then `--cwd`, then
    `auth status --json`, then `--version`.
  - **What it records:** for each run, whether it took the lock, whether the sandbox login still
    works afterwards (`auth status` reports `loggedIn`), and what a SIGTERM or a SIGKILL
    mid-refresh leaves behind.
  - **The watch:** it ships (group 1) and observes the real machine for the outcome check.
- **Alternatives:**
  - the passive watch alone;
  - tracing with `fs_usage`, `dtruss` or `eslogger`;
  - asking Anthropic.
- **Why:**
  - **The sandbox** answers the row's question in an afternoon, causally, on the panel's own argv,
    without touching the owner's login. If an experiment spends a login, it spends the scratch
    one.
  - **Not the watch alone:** it waits for a natural expiry, and it can only correlate.
  - **Not tracing:** it needs root, or SIP off for `dtruss`.
  - **Not asking:** the upstream issues have no maintainer reply yet.
- **Unverified:** that a second login of the same account doesn't affect the first. The docs
  describe separate directories as separate logins, but they show it with two different accounts.
  If the owner's main login shows any sign of trouble during the experiment, the build stops and
  says so.

**D2. A `claude` call past its timeout.**
- **Recommended:** SIGTERM (`exec.Cmd.Cancel`), then SIGKILL after a 15 s grace
  (`exec.Cmd.WaitDelay`). The caller still gets its error at the timeout.
- **Alternatives:**
  - no timeout at all;
  - a longer timeout, still SIGKILL.
- **Why:** SIGKILL is the one way to stop a holder that guarantees it can't release the lock. 15 s
  covers a 5 s refresh bound and a 2 s exit wait with room to spare.
  - **Not "no timeout":** a hung `claude` would hold its poll forever.
  - **Not a longer SIGKILL timeout:** that only moves the window.
- **This is necessary, not sufficient:** per #95822 a command can abandon its own refresh with
  nobody killing it. D5 and D6 address that part.

**D3. The panel's `claude` calls run one at a time.**
- **Recommended:** one machine-wide slot. A poll that finds it taken skips its turn and keeps its
  value. A lane action waits for it within its own timeout.
- **Alternatives:**
  - every call stands alone, as today;
  - a queue for polls too.
- **Why:** in the minutes around the shared token's expiry, every panel call is a refresh attempt.
  Today the panel can bring one per project poll, plus the filter cross-check, `auth status` and
  `--version`. With the slot it brings at most one. Polls refresh within seconds anyway, and a
  queue would pile up stale reads behind a slow call.

**D4. No `claude` call while a refresh holds the lock.**
- **Recommended:** before each call, stat `<config dir>/.oauth_refresh.lock`. The config dir is
  `$CLAUDE_CONFIG_DIR` from the panel's environment, else `~/.claude`.
  - **If the lock was touched in the last 60 s,** polls skip and lane actions wait as in D3.
  - **An older lock** stops nothing, and D7 reports it.
- **Alternative:** ignore the lock.
- **Why:** a live holder is refreshing the login every session shares, so a panel call would only
  add a contender or another short-lived refresher.
- **The race:** a call can start in the instant before a lock appears. D2 and D3 cover that.

**D5. `claude auth status`.**
- **Recommended:** no timer.
  - **When it runs:** at start only when no reading under 24 h old is saved, and on **Refresh**.
  - **The saved reading:** the last one, in the panel's directory beside `quota.json`, keeping
    only the fields kept today.
  - **The guards:** D2–D4 apply.
- **Alternatives:**
  - keep the 10-minute timer;
  - drop it, and infer the account from the status line.
- **Why:**
  - **Not the timer:** #95822 names `auth status` itself as a command that starts a refresh and
    exits without saving it, and the panel runs it 144 times a day. The account changes when the
    owner logs in to another one, which is rare and their own act.
  - **Not dropping it:** there is no status-line field for the auth mode or plan (#95598 asks for
    one), so the API-key and cloud modes, the plan and the account hash would be lost.
- **What it costs:** a change of account shows after **Refresh** or the next start, not within
  10 minutes. Help says so.

**D6. Making the panel's own `claude agents` start no refresh. Conditional, every outcome decided
now.**
- **Recommended:** group 1's sandbox runs pick one outcome:
  - **A. `claude agents --json` takes no lock** against an expired token: group 3 does nothing,
    and the evidence is recorded.
  - **B. It takes the lock, and finishes and saves the refresh before exiting** (the sandbox login
    still works): D2–D4 suffice. Group 3 does nothing; recorded.
  - **C. It takes the lock and the sandbox login is spent afterwards** (it exits without saving, as
    #95822 reports for `auth status`): the build tries, in the sandbox, `DISABLE_TELEMETRY` then
    `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC` (both documented).
    - **If one stops the lock-taking** and leaves the JSON the panel parses unchanged, group 3
      sets it on every panel `claude agents` and `auth status` call.
    - **If neither does,** the build stops at group 3. It brings the evidence to the owner, because
      what remains is theirs to choose (a slower cadence, polling only while no hook flows, or
      adding the evidence to #95822).
- **Alternatives:**
  - always set a variable;
  - decide after the investigation, by editing this design.
- **Why:**
  - **Not always:** the variable is a guess about one build, and it could change the listing
    (feature flags fall back to their defaults).
  - **Not deciding later:** an edit after approval voids the approval. Only C-with-no-variable
    returns to the owner, and only because no panel-side fix is left.

**D7. A stuck or stranded lock.**
- **Recommended:** two alerts, and never a deletion.
  - **The live alert:** the same lock (same birth time) touched within 60 s continuously for more
    than 2 minutes, which is a live holder stuck mid-refresh. The alert names the holder (pid,
    command, age) and what to do: "run /login in any session; if it comes back, end pid N".
  - **The stale warning:** a lock untouched for 60 s or more and still present 2 minutes later.
    The warning names the lock, its last holder and whether that process is gone. It gives the
    documented remedy for the path the panel actually sees: "if sessions fail to refresh, remove
    `~/.claude/.oauth_refresh.lock` and run /login" (errors.md names `token.lock`, which 2.1.288
    doesn't use).
  - **Never** delete or write either file.
- **Alternatives:**
  - alert on the live lock only, trusting the 60 s stale reclaim;
  - remove a stale lock automatically.
- **Why:**
  - **The stale warning:** #95236 reports a directory lock that was never reclaimed, and the
    2026-10-02 lock is consistent with it. On macOS the reclaim is unproven, so a stranded lock
    needs a reader.
  - **No deletion:** deleting a lock a live process holds would let two processes refresh one
    login at once, which is the very thing the lock prevents. Telling a live holder from a gone
    one is exactly what the owner record is for, and its format is internal.

**D8. Which groups ship in which outcome.**
- **Recommended:** groups 1, 2 and 4 ship whatever group 1 finds. Group 3 follows D6's outcome,
  and only D6's outcome C-with-no-variable returns to the owner. The row closes when the change
  merges. Its outcome check reads `login-lock.jsonl` after 14 days.
- **Alternative:** make every fix wait for the investigation, and re-approve.
- **Why:** each of D2–D5 and D7 is right even if the panel never takes part in a refresh:
  - SIGKILL on any `claude` is wrong;
  - contending with a live refresh only adds load;
  - a timer on a command reported to spend logins is a risk with no matching benefit;
  - a stranded lock needs a reader whoever stranded it.
