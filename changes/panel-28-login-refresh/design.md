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
| a project card or a template's suggest command whose argv starts with `claude` | `runtime.go` (cards and suggestions, `commandFetch`) | the project's own interval or watch | 30 s |
| `claude -p …`, the Verify now probe (**future**: PANEL-32, split from PANEL-31) | not built | on the owner's click | about 90 s (its own row decides) |

`ExecRunner` uses `exec.CommandContext` with no `Cancel` and no `WaitDelay`, so a timeout, or the
panel's own shutdown (`cmd/panel.go` cancels on SIGTERM), sends the child SIGKILL.

Shutdown today runs in `run.go`: `cancel()`, then `httpSrv.Shutdown` with 2 s, then `wg.Wait()` and
`ls.stopAll()`. `panel install` waits for the old job to go after `bootout` for about 5 s (50 ×
100 ms of `launchctl print`), then retries `bootstrap` once after 2 s (`install/launchd.go`). The
plist doesn't set `AbandonProcessGroup`, so launchd also reaps whatever is left in the job's
process group when the panel exits.

Freshness: a successful agents poll sets `agentsOKAt` (`state/model.go`, `ApplyAgents`). A failed
one sets the source's error and forgets nothing it read but stops calling it current. `blocked.go`
and the first-prompt decisions read `agentsFresh(now)`, so an update applied for a poll that never
ran would either pass a stale read off as fresh or flap the source to failed (M6 in D4).

## Shared with later rows

The build order is PANEL-29, then this, then PANEL-30, PANEL-31 and PANEL-25. Two pieces of this
change are built to be reused:
- **The lock reader** (`claudecall.Lock`): the config dir, the two lock paths, the owner record,
  and a state for each lock (none, live, stale, future) with its holder.
- **The gate** (`claudecall.Gate`): the stop policy (D2), the slot (D3) and the lock pause (D4) for
  every `claude` the panel spawns.

Who reuses them:
- **PANEL-31 and PANEL-32** use both, and drop the separate guard PANEL-31's draft had (one keyed on
  existence, `~/.claude` hard-coded, a 10 s grace). PANEL-32's probe goes through the gate. A long
  call holds the slot like any other, so the probe pauses the agents poll for its length. PANEL-32
  states that cost to the owner, or proposes an exception there.
- **PANEL-25's spawn counter** sits inside the gate, counting processes actually started. A skipped
  or paused call is not a spawn.
- **PANEL-25's estimate of `auth status`** changes under D5: no longer every 10 min.

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
  - leaving the owner blind to a stuck, stranded or future-dated lock (D7).

## The shape of the change

Groups 1–3 build unattended, in order. Group 4 is last because it needs the owner once, and then a
day's wait (D8).

1. **Group 1, the lock reader and the log.**
   - `framework/internal/panel/claudecall`, a new package that `panel` and `lanes` both import:
     - **The reader:** the config dir (`$CLAUDE_CONFIG_DIR`, else `~/.claude`), the current lock
       `<dir>/.oauth_refresh.lock`, the legacy lock `<dir>.lock`, and the owner record.
       - **The state of each lock,** from its mtime against now: live, stale, or future (D4).
       - **A lock's identity:** the owner record's `lockBirthtimeMs` when there is one, else the
         path and an unbroken run of sightings. Birth time comes from the record, never from the
         filesystem, so the code is the same on darwin and on PR CI's Ubuntu.
     - **The watch:** a stat of both locks and the record every 2 s. A stat spawns nothing.
   - **The log:** `login-lock.jsonl` in `config.PanelDir` (beside `quota.json`), capped in size.
     - **Per lock seen,** one line: path, holder pid, `ps -o command=` of the holder, whether it is
       the panel's own call and which, first seen, last modified, and how it ended.
     - **Per stopped call,** one line (group 2 writes these).
   - A re-read of the installed binary, recording in the Decision log what differs from *What is
     known*.
2. **Group 2, the gate.** `claudecall.Gate` wraps the Runner for any argv whose program is
   `claude`, directly or after `/usr/bin/env -u …`. It covers cards and suggest commands too (D9).
   - **How it stops a call:** it starts the process, reaps it in a goroutine, and returns to the
     caller at its deadline (D2).
   - **The slot:** one process at a time, held until reaped (D3).
   - **The lock pause:** no start while a lock is live (D4).
   - **What the callers do:**
     - polls wait briefly and then return a short retry, applying no update;
     - `checkFilter` records its check time only when the check ran;
     - `pollAccount` follows D5;
     - Refresh kicks the machine's sources as well as the project's.
3. **Group 3, the owner sees a lock** (D7): the live alert, the stranded warning, the future-dated
   warning, `docs/panel.md` and Help.
4. **Group 4, last: the sandbox and its response** (D1, D6). It needs the owner's `/login` under a
   scratch config dir, then an expiry. Its outcome picks one response decided in D6.

## Refusals

| Situation | What happens instead |
|---|---|
| A `claude` call while the slot is taken | It waits up to 2 s (a poll) or within its own timeout (a lane action). A poll still blocked returns a retry in 1 s and applies no update: the last reading, its success time and its error stay as they were. |
| A call while a lock is live (modified in the last 60 s, not in the future) | No start. A poll returns a retry in 2 s, applying no update; its source shows "paused: a login refresh is in progress". A lane action waits within its timeout. |
| A long pause (a lock live for minutes) | The agents reading ages out by the existing freshness window, as it would with no poll, and nothing typed waits on a stale read. The source says "paused", not "cannot read". The D7 alert fires at 2 min. |
| A `claude` call past its timeout | The caller gets its timeout error at its deadline; the gate sends SIGTERM, SIGKILLs 15 s later if the process is still running, and keeps the slot until it is reaped. |
| The panel shutting down with a `claude` call in flight | SIGTERM; SIGKILL 3 s later if it is still running; the shutdown waits no longer. |
| The account wanted while a reading under 24 h old is saved | The saved reading is used; no `auth status` runs until **Refresh**. |
| A lock held live for more than 2 min | An alert naming the holder (pid, command, age) and what to do. |
| A lock last modified 60 s or more ago, still present 2 min later | A warning naming that lock's path (current or legacy), its last holder and whether that process is gone, and the remedy, which the owner carries out. |
| A lock modified more than 5 s in the future | Not live: calls run. A warning says it is dated in the future (#95739), and gives the same remedy. |
| Any lock | The panel never removes or writes it, or the owner record. |
| An owner record the panel can't parse (a new Claude Code format) | The lock is still watched and alerted by its mtime. The holder is reported "unknown". |

## Decisions (awaiting the owner)

**D1. How the facts are established.**
- **Recommended:** a sandbox login, plus the passive watch from group 1.
- **Setting up the sandbox,** with the owner once, with no lane running:
  1. Make a scratch `CLAUDE_CONFIG_DIR` and a scratch folder inside it.
  2. In that folder, `CLAUDE_CONFIG_DIR=<scratch> claude`; accept the folder's trust prompt, then
     `/login`. A fresh config dir trusts no folder, and the refresh path in `claude agents` runs
     only in a trusted one.
  3. Right after, check the main login in an already-open session with `/status`. That is a
     human-visible check, not `auth status`, which is itself a suspect (#95822).
  4. Leave the sandbox unused until the next day.
- **The run: one question first,** the panel's exact `claude agents --json`, in the trusted folder.
  Then, at once, a positive control: `claude -p "ok"`, a tiny model call that must refresh an
  expired token. The lock watch on the sandbox says which of these took the lock:

  | agents took the lock | the control | Reading |
  |---|---|---|
  | no | took the lock | **A**: the token had expired, and agents didn't refresh |
  | yes | works, takes no lock | **B**: agents refreshed and saved |
  | yes | fails "Login expired" | **C**: agents spent the login |
  | no | takes no lock | **Inconclusive**: the token hadn't expired; try again later |
- **Every later question is optional,** with one run per later expiry: `auth status`, then
  `--version`, then a SIGTERM and a SIGKILL 1 s into `claude agents`. Each that isn't run stays
  unverified, and D2 already tolerates that.
- **If the main login shows any trouble** at any point, group 4 stops and the owner is told. Its
  tasks are ticked as "stopped: <why>", and the PR merges with groups 1–3 (D8).
- **Alternatives:**
  - the passive watch alone;
  - tracing with `fs_usage`, `dtruss` or `eslogger`;
  - asking Anthropic.
- **Why:**
  - **The sandbox** answers the one question that matters, causally, on the panel's own argv, and
    can't spend the owner's login. `CLAUDE_CONFIG_DIR` has its own Keychain entry, as documented.
  - **The positive control** stops an unexpired token from passing for outcome A.
  - **Not the watch alone:** it waits for a natural expiry, and it can only correlate.
  - **Not tracing:** it needs root, or SIP off for `dtruss`.
  - **Not asking:** the upstream issues have no maintainer reply yet.
- **Unverified:** that a second login of the same account leaves the first alone. The docs show it
  with two different accounts. That is why step 3 checks at once, and why trouble stops only
  group 4.

**D2. How the gate stops a `claude` call.**
- **Recommended:**
  - **The caller** gets its timeout error at its own deadline. The gate starts the process and
    reaps it in a goroutine, so the caller never waits for the exit.
  - **The process** gets SIGTERM at that deadline, and SIGKILL 15 s later if it is still running.
  - **The slot** stays held until the process is reaped (D3).
  - **At panel shutdown,** the grace is 3 s, not 15. The gate's close runs after `wg.Wait()` and
    waits at most 3 s.
- **Alternatives:**
  - `exec.Cmd.Cancel` with `WaitDelay`: `Wait` blocks the caller until the exit or the delay,
    so the caller can't return at its deadline;
  - releasing the slot at the deadline: two `claude` processes at once during the grace;
  - the full 15 s grace at shutdown, or a longer install wait.
- **Why:** SIGKILL is the one way to stop a holder that guarantees it can't release the lock, so it
  comes last.
  - **15 s at runtime** covers a 5 s refresh bound and a 2 s exit wait with room to spare.
  - **3 s at shutdown** keeps the exit inside `panel install`'s wait: about 5 s for the old job to
    go, plus one 2 s retry. A longer install wait would only slow every install. launchd reaps the
    job's process group at exit anyway (`AbandonProcessGroup` is unset).
  - **The residual risk:** a refresh straddling a panel restart. It needs a restart inside the
    seconds a refresh takes.
- **This is necessary, not sufficient:** per #95822 a command can abandon its own refresh with
  nobody killing it. D5 and D6 address that part. That SIGTERM lets Claude Code release the lock
  stays unverified unless D1's optional stop test runs.

**D3. One panel `claude` call at a time, without starving anyone.**
- **Recommended:** one machine-wide slot, held until the process is reaped.
  - **A poll** that finds the slot taken waits for it up to 2 s. If it's still taken, the poll
    returns a retry in 1 s through the poll's own next-wait, rather than its interval, and applies
    no update.
  - **A lane action** waits within its own timeout.
  - **`checkFilter`** records `agentsFilterCheck` only after a cross-check that ran. Today it is set
    before the call (`runtime.go`), so a skip would drop the `--cwd` filter for 5 min.
  - **Refresh** (`refreshAll`) also kicks the machine's sources (`Machine.kickAll`). Today it kicks
    only the project's, so the version and account wait for their timers.
- **Alternatives:**
  - skip to the next interval;
  - a queue;
  - every call stands alone, as today.
- **Why:** at start every source fires at once, so collisions are certain.
  - **Not skipping to the next interval:** a skipped `--version` waits 10 min. A skipped account
    read waits for Refresh (D5). A hook-kicked agents poll under PANEL-25's dormant interval waits
    5 min.
  - **A short wait and a 1 s retry** serialize the start in a second or two, because each call
    takes about 0.1 s.
  - **Not a queue:** it piles up stale reads behind a slow call.
  - **Not standing alone:** in the minutes around the shared token's expiry, every concurrent panel
    call is another refresh attempt.

**D4. No `claude` call while a refresh holds a lock.**
- **Recommended:** before each start, the gate reads both locks' state (D7 defines the states).
  - **Live** (modified in the last 60 s, and no more than 5 s in the future): no start. Polls retry
    in 2 s, applying no update. Lane actions wait within their timeout.
  - **Stale or future:** nothing stops.
  - **The legacy lock `<dir>.lock` counts too.** In 2.1.288 every refresher takes it as well, and a
    stranded one makes the next acquire fail (`tengu_oauth_refresh_legacy_lock_contended`).
- **What freshness does:**
  - **A paused or skipped poll applies no update.** It neither advances `agentsOKAt` (which would
    pass a stale read off as fresh to `blocked.go` and the first-prompt decisions) nor applies an
    error (which would flap the source to failed and back).
  - **During a long live lock,** the agents reading ages out by the existing freshness window,
    exactly as if the poll had not been due.
  - **The status** carries a pause note that leaves `OK` alone, so the banner reads "paused: a
    login refresh is in progress", not "cannot read".
- **Alternative:** ignore the locks.
- **Why:** a live holder is refreshing the login every session shares, so a panel call would only
  add a contender or another short-lived refresher.
  - **Future-dated isn't live** (#95739): its mtime reads "recent" until real time passes it, which
    would pause every poll for hours.
  - **The race:** a call can start in the instant before a lock appears. D2 and D3 cover that.

**D5. `claude auth status`.**
- **Recommended:** no timer.
  - **When it runs:** at start only when no reading under 24 h old is saved, and on **Refresh**,
    which D3 makes kick the machine's sources.
  - **The saved reading:** the last one, in `account.json` beside `quota.json`, keeping only the
    fields kept today.
  - **The guards:** the gate applies.
- **Alternatives:**
  - keep the 10-minute timer;
  - drop it, and infer the account from the status line.
- **Why:**
  - **Not the timer:** #95822 names `auth status` itself as a command that starts a refresh and
    exits without saving it, and the timer runs it 144 times a day. The account changes when the
    owner logs in to another one, which is rare and their own act.
  - **Not dropping it:** no status-line field gives the auth mode or plan (#95598 asks for one).
- **What it costs:** a change of account shows after **Refresh** or the next start. Help says so.

**D6. Making the panel's own `claude agents` start no refresh. Every outcome decided now.**
- **Recommended:** D1's run picks one outcome:
  - **A or B:** nothing more; recorded.
  - **Inconclusive:** run again at the next expiry, up to three times. If it's still inconclusive,
    record it as unanswered and treat it as B.
  - **C:**
    - **The retest:** the owner logs in to the sandbox again, and at the next expiry the build
      repeats the run with `DISABLE_TELEMETRY`, then with
      `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC` (both documented).
    - **If one turns C into A** and leaves the JSON the panel parses unchanged, group 4 sets it on
      every panel `claude agents` and `auth status` call.
    - **If neither does,** no panel-side fix is left. 4.3 is ticked as "no panel-side fix", the
      evidence goes to the owner, and the PR merges. Their options become a new roadmap row: a
      slower cadence, polling only while no hook flows, or adding the evidence to #95822.
- **Alternatives:**
  - always set a variable;
  - decide after the investigation, by editing this design.
- **Why:**
  - **Not always:** the variable is a guess about one build, and it could change the listing.
  - **Not deciding later:** an edit after approval voids the approval.

**D7. A stuck, stranded or future-dated lock.**
- **Recommended:** three states for each of the two locks, read from mtime against now. No
  birth time is read from the filesystem: Go exposes it on darwin only, and PR CI runs on Ubuntu.
  - **Live, continuously, for more than 2 min:** an alert naming the holder (pid, command, age)
    and what to do: "run /login in any session; if it comes back, end pid N".
  - **Stale (60 s or more) and still present 2 min later:** a warning naming that lock's path, its
    last holder and whether that process is gone. Its remedy: "if sessions fail to refresh, remove
    `<path>` and run /login". errors.md names `token.lock`, which 2.1.288 doesn't use.
  - **More than 5 s in the future:** a warning saying so, that Claude Code waits for real time to
    pass it (#95739), with the same remedy.
  - **Never** delete or write a lock or the owner record.
- **Alternatives:**
  - alert on the live lock only, trusting the 60 s stale reclaim;
  - remove a stale lock automatically.
- **Why:**
  - **The warnings:** #95236 reports a directory lock never reclaimed, #95739 a future one that
    blocks for hours, and the 2026-10-02 lock is consistent with the first.
  - **No deletion:** deleting a lock a live process holds would let two processes refresh one
    login at once, which is the very thing the lock prevents.

**D8. Group order, and which groups ship in which outcome.**
- **Recommended:** one change, one PR.
  - **Groups 1–3** build unattended, first, and are right whatever the sandbox finds.
  - **Group 4** (the sandbox and D6's response) is last. The PR waits for it, because
    `pr-merge-guard` rule 9 refuses a change PR with an unticked task.
  - **A stop or a C-with-no-variable** ticks group 4 with its reason, so it never holds the PR.
    Only C-with-no-variable leaves the owner a choice, as a new row.
  - **The outcome check** reads `login-lock.jsonl` after 14 days.
- **Alternatives:**
  - **the sandbox first,** with every fix waiting on it;
  - **group 4 split into its own roadmap row,** created on approval, so groups 1–3 merge without
    waiting for the expiry.
- **Why:**
  - **Groups 1–3 can't depend on the answer:** each of D2–D5 and D7 is right even if the panel
    never takes part in a refresh.
  - **One PR:** it keeps the rule and the record whole, at the cost of the fixes landing a day or
    two later.
  - **The split** lands the fixes sooner. It is the better choice if the owner can't do the
    sandbox `/login` soon after the build.

**D9. Project cards and suggest commands that run `claude`.**
- **Recommended:** gated like every other `claude` call (the stop policy, the slot, the lock pause).
  Such a card waits up to 2 s for the slot and retries on its own cadence. Help says a card running
  `claude` shares the panel's one slot.
- **Alternative:** exempt them, since the project chose to run them.
- **Why:** a project's `claude` card refreshes the same login as the panel's own calls. Exempting
  it would reopen every hazard this change closes. Its 30 s timeout is the cost a project accepts
  by running `claude` on a timer.
