**Status:** awaiting approval
**Roadmap row:** PANEL-28
**Risk:** normal

## Why

"Could not refresh your login because another Claude Code process is refreshing it (or exited
mid-refresh)" killed three agents on 2026-10-01 (13:30, 13:40 and 21:55 UTC), and the owner hit it
at startup on 2026-10-02. Each time the agent died and had to be resumed after `/login`.

Every Claude Code process on the machine shares one login. Near its access token's expiry, a
process refreshes it under a lock directory, `~/.claude/.oauth_refresh.lock`, and the others wait
and then fail with this error. On 2026-10-02 the lock was created at 10:01:27 EDT and still existed
at 10:12, after a successful `/login`.

The panel is the most frequent `claude` process on the machine:
- **The calls:** `claude agents --json` every 2 to 15 s per project, and `claude auth status
  --json` and `claude --version` every 10 min.
- **The kill:** it kills each of them with SIGKILL at a 10 s timeout, because
  `signals.ExecRunner` uses `exec.CommandContext`, whose default is `Process.Kill`.

Two things found while drafting say this is a real hazard, not a hypothetical one (design.md, *What
is known*, gives each claim's source and whether it was checked):
- **A stranded lock.**
  - **The binary:** in the installed Claude Code 2.1.288, `claude agents --json` starts a
    feature-flag client whose first step refreshes an expired token, under the same lock.
  - **The panel's part:** a SIGKILLed holder can't release that lock.
  - **Upstream:** an open issue ([#95236](https://github.com/anthropics/claude-code/issues/95236))
    reports such a lock never being reclaimed.
- **A spent login.**
  - **Upstream:** an open issue ([#95822](https://github.com/anthropics/claude-code/issues/95822))
    reports that short-lived commands, `claude auth status` by name, start a refresh at startup
    and exit before saving it. That leaves the login spent.
  - **The panel's part:** it runs that command on a timer.

Neither has been observed on this machine yet. A sandbox login settles the one that matters (D1).

## What changes

The panel stops doing what can strand the lock or spend the login, and shows the owner a lock that
is stuck, left behind or dated in the future. Every `claude` the panel spawns goes through one
gate, which PANEL-31, PANEL-32 and PANEL-25 reuse:

- **No `claude` call is killed outright.**
  - **The caller** gets its timeout at once.
  - **The process** gets SIGTERM, and SIGKILL only 15 s later (3 s at shutdown).
- **One panel `claude` call at a time,** machine-wide.
  - **A call that collides** waits briefly and retries within seconds, so nothing waits out a full
    interval.
  - **Refresh** now re-reads the version and the account too.
- **No `claude` call while a refresh holds a lock** (the current lock or the legacy one). Polls
  pause without being marked failed or passed off as fresh, and resume within seconds of the lock
  clearing.
- **`claude auth status` comes off its timer.** It runs at start only without a recent saved
  reading, and on **Refresh**.
- **Every lock the panel sees is logged** with its holder, and whether the holder is the panel's
  own call.
- **The owner is told about a lock.**
  - **A stuck one** is an alert.
  - **A stranded or future-dated one** is a warning, with the remedy.
  - **The panel never removes or writes** a lock.
- **Last: a sandbox login answers whether `claude agents` itself spends a login.** It is a scratch
  `CLAUDE_CONFIG_DIR` with a trusted folder, checked against a positive control. Each answer has a
  decided response (D6). The fixes above don't wait on it: they're right whatever it finds.

## What the existing specs already guarantee

This repo has no living specs yet (`specs/` holds only its README), so nothing here modifies or
contradicts a requirement. The polled commands, their cadences and the account reading are
documented, not specified, in `docs/panel.md` (the source table, and *The account and its quota*).
This change adds the capability spec `panel-claude-calls` and updates those sections to match.

## Out of scope

- **Claude Code's own bugs.** Four are Anthropic's to fix:
  - short-lived commands abandoning a refresh (#95822);
  - a directory lock never reclaimed (#95236);
  - a future-dated lock ([#95739](https://github.com/anthropics/claude-code/issues/95739));
  - errors.md naming `~/.claude/token.lock` and a 60 s wait, where 2.1.288 uses
    `.oauth_refresh.lock` and gives up after about 7.5 s.

  The panel only stops feeding them. Adding this repo's evidence to those issues is outward, so
  it's the owner's call. Not owned until the owner decides.
- **Fewer concurrent agents.** The 2026-10-01 diagnosis was many agents refreshing one login at
  once. Their refreshes contend with each other whatever the panel does. Not owned: the log and
  the alerts make it visible, and the outcome check says whether a row is needed.
- **Removing a lock.** The panel never deletes it (D7). The warning tells the owner how.

## How we'll know

- **Signal:** no agent killed by "another Claude Code process is refreshing it", and no
  unexplained "Login expired", in 14 days of normal use with the panel running. The panel's lock
  log shows no lock held or left by the panel's own `claude` calls.
- **Check after:** 14 days
