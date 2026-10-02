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

Neither has been observed on this machine yet, which is what group 1 does first.

## What changes

The panel stops doing what can strand the lock or spend the login. It shows the owner a lock that
is stuck or left behind, and it finds out the rest with a scratch login rather than the owner's
own:

- **No `claude` call is killed outright.** A timed-out call gets SIGTERM and a grace period before
  any SIGKILL.
- **One panel `claude` call at a time,** machine-wide. A poll that finds one in flight skips its
  turn.
- **No `claude` call while a refresh holds the lock.** Polls pause until it clears.
- **`claude auth status` comes off its timer.** It runs at start only without a recent saved
  reading, and on **Refresh**.
- **Every lock the panel sees is logged** with its holder, and whether the holder is the panel's
  own call.
- **A stuck lock is an alert, and a stranded one is a warning,** each with what to do. The panel
  never removes or writes the lock.
- **A sandbox login under a scratch `CLAUDE_CONFIG_DIR` answers whether `claude agents` itself
  spends a login.** Each answer has a decided response (D6). Only one returns to the owner: the
  case where no panel-side fix is left.

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
