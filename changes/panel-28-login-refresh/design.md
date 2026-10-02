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

## What the installed Claude Code does

This was read, while drafting, from the strings of the installed binary, Claude Code 2.1.288
(`~/.local/share/claude/versions/2.1.288`, a Bun build with its JavaScript embedded). Nothing was
run. It is evidence about one version and not an observation. Group 1 re-reads whatever version is
installed when it builds, and the lock log (D1) observes the rest.

- **The lock.** The refresh takes a lock at `<config dir>/.oauth_refresh.lock` with the options
  `stale: 60000, update: 5000`: the holder touches the directory every 5 s, and a lock untouched
  for 60 s counts as stale. A legacy lock beside the config directory (`<config dir>.lock`) is
  taken too. The holder writes `<config dir>/.oauth_refresh.lock.owner`, a JSON record with
  `pid`, `procStart` and `lockBirthtimeMs`.
- **A waiter.** It retries 5 times, 1–2 s apart, and waits until about 7.5 s for the lock to change.
  It then tries a dead-holder takeover, which runs only when the owner record proves the holder
  gone, and only behind a feature flag. Otherwise it gives up as `lock_busy` (holder alive) or
  `lock_timeout`. That is the error the agents died on.
- **When a refresh happens.** Only when the stored access token has expired, or after a 401.
  Otherwise the check is local and returns at once, which fits the 0.09 s `claude agents --json`
  measured today.
- **`claude agents --json`.** Its handler starts the feature-flag client without waiting for it
  (`kickGrowthBook`). The client's `createClient` calls `refreshOAuthTokenIfNeeded` first, bounded
  at 5 s, when the folder is trusted. That is the same refresh function, so it takes the same
  lock. The client is off when analytics is disabled or `DISABLE_GROWTHBOOK` is set.
- **`claude auth status --json`.** Its handler reads stored state and calls no refresh. Every
  subcommand also runs the root startup hook, which starts remote-settings and feature-flag work
  whose auth use was not traced to the end.
- **On exit.** A process that is exiting waits up to 2 s for a refresh in flight. SIGKILL skips
  that wait, and skips any lock release.

So the panel's `claude agents` probably refreshes the shared login whenever it is the first process
to find the token expired. At a 2 s cadence it often is. If that refresh is slow, the panel kills
it at 10 s while it holds the lock, and every waiter fails until the lock goes stale 60 s later.
That fits the error's own wording: "or exited mid-refresh".

What this reading can't show: whether the panel actually was the holder on 2026-10-01 and
2026-10-02, and why a lock outlived `/login` on 2026-10-02. A left-behind lock is harmless once
stale, if Claude Code reclaims it as its options say. Nobody needed a refresh after `/login`, so
nothing reclaimed it. The log (D1) answers both from now on.

## The shape of the change

1. **Group 1, the facts.**
   - A lock watch in the machine: a stat of the lock and its owner record every 2 s. A stat spawns
     nothing.
   - The panel's own `claude` children are tracked by pid.
   - `login-lock.jsonl` in the panel's directory (`config.PanelDir`, beside `quota.json`), capped
     in size. It gets one line per lock seen (holder pid, `ps -o command=` of the holder, whether
     it is a panel child, first seen, last touched, how it ended) and one line per panel `claude`
     call that had to be stopped.
   - The build re-reads the installed Claude Code as above, and records what differs in the
     Decision log.
   - It runs D6's two checks.
2. **Group 2, calls that can't strand the lock.** A `claude` gate wrapping the Runner, for any argv
   whose program is `claude` (directly or after `/usr/bin/env -u …`):
   - SIGTERM, then SIGKILL only after a grace (D2);
   - one call at a time (D3);
   - no call while the lock is live (D4).

   Every call site keeps its own timeout for its result. The gate decides only how the process is
   stopped and whether it starts.
3. **Group 3, conditional (D6).** The panel's own `claude agents` and `claude auth status` run with
   the feature-flag client off, if and only if group 1's checks pass. Otherwise group 3's tasks are
   ticked as "not applied", with the evidence in the Decision log.
4. **Group 4, the owner sees a stuck refresh** (D7): an alert, a diagnostics line, the docs.

## Refusals

| Situation | What happens instead |
|---|---|
| A poll while another panel `claude` call is in flight | Skipped. The source keeps its last value and says why; the next tick tries again. |
| A poll while the lock is live (touched in the last 60 s) | Skipped, as above, with "a login refresh is in progress". |
| A lane action needing `claude agents` while a call is in flight or the lock is live | It waits within its own timeout, then runs. If the timeout passes first, it fails as it does today when `claude agents` can't be read. |
| A `claude` call past its timeout | SIGTERM; SIGKILL only after the 15 s grace. The caller gets its timeout error at once and doesn't wait for the exit. |
| The panel shutting down with a `claude` call in flight | The same SIGTERM, then grace. The panel doesn't wait past the grace. |
| A stale lock (untouched for 60 s or more) | Not removed and not alerted. Diagnostics shows "left behind by pid N; Claude Code reclaims it at the next refresh". Calls proceed. |
| A lock held live for more than 2 min | An alert naming the holder (pid, command, age) and what to do. The panel never removes it. |
| An owner record the panel can't parse (a new Claude Code format) | The lock is still watched and alerted by its directory's times. The holder is reported "unknown". |

## Decisions (awaiting the owner)

**D1. How group 1 establishes the facts.**
- **Recommended:** two sources. A static read of the installed Claude Code, re-done at build time
  because it holds for one version only. And a passive lock watch the panel ships, logging every
  lock it sees, its holder, and whether the holder was the panel's own child.
- **Alternatives:**
  - a throwaway `CLAUDE_CONFIG_DIR` with an expired token;
  - tracing with `fs_usage`, `dtruss` or `eslogger`;
  - asking Anthropic.
- **Why:**
  - **The watch:** it observes the real machine in normal use, with no risk to the login, and it
    is the same mechanism the alert (D7) needs, so nothing is built twice.
  - **Not the throwaway directory:** Claude Code does read `CLAUDE_CONFIG_DIR` (the
    `auth status` code compares it). But on macOS the login lives in the Keychain, not
    `~/.claude/.credentials.json` (absent on this machine). Whether a different config directory
    gets its own Keychain entry is unverified. The experiment needs a second real login of the
    owner's account, and it tests a synthetic expiry, not the panel's real timing.
  - **Not tracing:** the tools need root, or SIP off for `dtruss`.
  - **Not asking Anthropic:** an answer doesn't come on a schedule, and it's outward (Out of scope).

**D2. A `claude` call past its timeout.**
- **Recommended:** SIGTERM (`exec.Cmd.Cancel`), then SIGKILL after a 15 s grace
  (`exec.Cmd.WaitDelay`). The caller still gets its error at the timeout.
- **Alternatives:**
  - no timeout at all, letting every call finish;
  - a longer timeout, still SIGKILL.
- **Why:** a terminated process can release its lock and finish its exit wait (2 s), and SIGKILL
  allows neither. 15 s covers the 5 s refresh bound and the 2 s exit wait with room to spare.
  - **Not "no timeout":** a hung `claude` would hold its poll forever.
  - **Not a longer SIGKILL timeout:** that only moves the window.
- **Unverified:** that Claude Code releases the lock on SIGTERM. Its lock options are those of a
  library that releases on a signal exit, but the build can't trigger a refresh to watch it. If
  the log shows a stopped call left a lock, that is the evidence.

**D3. The panel's `claude` calls run one at a time.**
- **Recommended:** one machine-wide slot. A poll that finds it taken skips its turn and keeps its
  value. A lane action waits for it within its own timeout.
- **Alternatives:**
  - every call stands alone, as today;
  - a queue for polls too.
- **Why:** when the shared token expires, the panel today brings up to one contender per project
  poll, plus the filter cross-check, `auth status` and `--version`. With the slot, the panel brings
  at most one. Polls refresh within seconds anyway, so skipping costs nothing. A queue would pile
  up stale reads behind a slow call.

**D4. No `claude` call while a refresh holds the lock.**
- **Recommended:** before each call, stat `<config dir>/.oauth_refresh.lock`. The config dir is
  `$CLAUDE_CONFIG_DIR` from the panel's environment, else `~/.claude`.
  - **If the lock was touched in the last 60 s** (Claude Code's own stale threshold), polls skip,
    and lane actions wait as in D3.
  - **An older lock** doesn't stop anything.
- **Alternative:** ignore the lock.
- **Why:** a live holder is refreshing the login every session shares, so the panel's call can
  only add a contender. If the token has expired, the call would need the refresh itself.
- **The race:** a call can start in the instant before a lock appears. D2 and D3 cover that.

**D5. `claude auth status`.**
- **Recommended:** keep it, at start and every 10 minutes, under D2–D4 (and D6 if it applies).
- **Alternative:** drop it, and infer the account from the status line (`rate_limits` present
  means a subscription).
- **Why:** dropping it loses three things:
  - the API-key and cloud modes (no status-line field says them);
  - the plan;
  - the account hash that stops a saved quota of one account showing for another.

  Its own handler reads stored state. Its shared startup is the same as `claude agents`', which
  D2–D4 already make safe.

**D6. The panel's own calls with the feature-flag client off. Conditional, decided now.**
- **Recommended:** apply if and only if group 1 shows both of these, else don't apply and record
  why:
  - **(a)** a documented variable (`DISABLE_TELEMETRY` or `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC`,
    the narrowest that works) stops `claude agents --json` starting the feature-flag client,
    seen in its debug output;
  - **(b)** the variable leaves the `claude agents --json` and `auth status --json` output the
    panel parses unchanged.

  The checks run `claude agents --json` by hand twice. That risks the login no more than the
  panel's own poll does every 2 s. If the subcommand has no debug output to show (a), the check
  fails and D6 is not applied.
- **Alternatives:**
  - always apply it;
  - never apply it.
- **Why:**
  - **What it would remove:** the panel's calls read only local state. A refresh they start is a
    side effect of the feature-flag client, and with the client off, D2–D4 become a second layer.
  - **Not always:** the variable is a heuristic reading of one build, and it could change the
    listing (feature flags fall back to their defaults).
  - **Not never:** that throws away the one change that removes the panel from the race entirely.
  - **No re-approval in either outcome:** the condition is fixed here.
  - **Not `DISABLE_GROWTHBOOK`:** it's undocumented.

**D7. A stuck or left-behind lock.**
- **Recommended:**
  - **Alert** when the same lock (same birth time) has been touched within 60 s continuously for
    more than 2 minutes: a live holder stuck mid-refresh, which blocks every session. The alert
    names the holder (pid, command, age) and what to do: "run /login in any session; if it comes
    back, end pid N".
  - **Diagnostics only** for a stale lock (untouched for 60 s or more): one line in the panel's
    diagnostics.
  - **Never delete or write** either file.
- **Alternatives:**
  - alert on any lock directory older than N minutes;
  - remove a stale lock.
- **Why:**
  - **Not "older than N minutes":** by Claude Code's own options a stale lock is reclaimed at the
    next refresh, so it's harmless. 2026-10-02's lock sat there after `/login` because nobody
    needed a refresh. An alert on it would be wrong most of the time and would teach the owner to
    ignore the alert.
  - **Not removing it:** deleting a lock a live process holds would let two processes refresh one
    login at once, which is the very thing the lock prevents.

**D8. Which groups ship in which outcome.**
- **Recommended:** groups 1, 2 and 4 ship whatever group 1 finds; only group 3 is conditional
  (D6). The row closes when the change merges. Its outcome check reads the log after 14 days.
- **Alternative:** make groups 2–4 depend on what the investigation finds, and re-approve.
- **Why:** each of D2–D4 and D7 is right even if the panel never takes part in a refresh:
  - SIGKILL on any `claude` is wrong;
  - contending with a live refresh only adds load;
  - a stuck holder needs a reader whoever it is.

  Deciding it now means no edit to this design after approval, which would void the approval.
