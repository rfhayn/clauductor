**Status:** awaiting approval
**Roadmap row:** PANEL-28
**Risk:** normal

## Why

"Could not refresh your login because another Claude Code process is refreshing it (or exited
mid-refresh)" killed three agents on 2026-10-01 (13:30, 13:40 and 21:55 UTC), and the owner hit it
at startup on 2026-10-02. Each time the agent died and had to be resumed after `/login`.

Every Claude Code process on the machine shares one login. When its access token expires, each
process that needs it tries to refresh it, and only one may: the holder of a lock directory,
`~/.claude/.oauth_refresh.lock`. The others wait about 7.5 to 10 seconds and then fail with this
error. On 2026-10-02 the lock was created at 10:01:27 EDT and still existed at 10:12, after a
successful `/login`.

The panel is the most frequent `claude` process on the machine. It runs `claude agents --json`
every 2 to 15 s per project, `claude auth status --json` and `claude --version` at start and
every 10 min, and `claude agents` again for lane actions. It kills each of them with SIGKILL at a
10 s timeout (`signals.ExecRunner` uses `exec.CommandContext`, whose default cancel is
`Process.Kill`). A process killed that way cannot release a lock it holds.

The row asked whether those calls take part in the refresh at all. A read of the installed Claude
Code (2.1.288, `~/.local/share/claude/versions/2.1.288`) while drafting this says they very
likely do. `claude agents --json` starts the feature-flag client, and that client's first step is
"refresh the OAuth token if needed", bounded at 5 s, in a trusted folder. That is the same refresh
function that takes the lock. `claude auth status --json` reads stored state in its own handler,
but runs the shared startup every subcommand runs. Nothing has been observed yet (design.md, *What
the installed Claude Code does*).

## What changes

The panel stops being a hazard to the login every session shares, and shows the owner a stuck
refresh:

- **No `claude` call is killed outright.** A timed-out call gets SIGTERM and a grace period before
  any SIGKILL, so it can release the lock.
- **One panel `claude` call at a time**, machine-wide. A poll that finds one already in flight
  skips its turn.
- **No `claude` call while a refresh holds the lock.** Polls pause until the lock clears, then
  resume.
- **Every lock the panel sees is logged**: its holder's pid and command, whether the holder is the
  panel's own child, how long it was held and how it ended. Every `claude` call the panel had to
  stop is logged too. This is the observation the row asked for, gathered in normal use with no
  risk to the login.
- **A stuck refresh is an alert.** A lock held live for more than 2 minutes names its holder and
  what to do. The panel never removes or writes the lock.
- **If a check at build time confirms it** (D6), the panel's own `claude agents` and `auth status`
  calls run with the feature-flag client off, so they never start a refresh at all.

## What the existing specs already guarantee

This repo has no living specs yet (`specs/` holds only its README), so nothing here modifies or
contradicts a requirement. The polled commands and their cadences are documented, not specified,
in `docs/panel.md` (the source table, and *The account and its quota*). This change adds the
capability spec `panel-claude-calls` and updates those sections to match.

## Out of scope

- **Fewer concurrent agents.** The 2026-10-01 diagnosis was many agents refreshing one login at
  once. The panel can't change how many sessions the owner runs, and their refreshes contend with
  each other whatever the panel does. Not owned: the alert (D7) and the log make it visible, and
  the outcome check (How we'll know) says whether a row is needed.
- **Reporting the stuck-lock behaviour to Anthropic.** An issue on `anthropics/claude-code` is
  outward, so it's the owner's call. The log this change adds is the evidence such an issue would
  need. Not owned until the owner decides.
- **Removing a stale lock.** The panel never deletes it (D7). Claude Code's own lock options mark a
  lock stale 60 s after its holder stops updating it, so the next refresh can reclaim it
  (design.md; read from the binary, not yet observed).

## How we'll know

- **Signal:** no agent killed by "another Claude Code process is refreshing it" in 14 days of
  normal use with the panel running, and the panel's lock log shows no refresh ended by the panel
  stopping its own `claude` call.
- **Check after:** 14 days
