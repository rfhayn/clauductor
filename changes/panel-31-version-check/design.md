# Design: panel-31-version-check

## Where the behaviour lives today

- **The check** (`framework/internal/panel/state/verify.go`, PANEL-13):
  - `checking` is true while the running version is neither `HeuristicsVerifiedOn` ("2.1.284",
    `state/model.go`) nor the version verified from live hooks.
  - `observePost` and `observeStart` count each Agent launch: a confirmation, unmatched (an
    "orphan", after 10 s) or unnamed.
  - `confirm` verifies the version at three confirmations with no orphan. Two orphans, or three
    unnamed with no confirmation, is a break.
  - The counts live only in memory. The verified version is kept in `verified.json`.
- **Saving it** (`framework/internal/panel/machine.go`):
  - `pollVersion` reads `claude --version` and logs a warning line when it isn't 2.1.284.
  - `saveReadings` writes the first project's verified version it finds to `verified.json` and
    pushes it to every project with `RestoreAutoVerified`.
  - `RestoreAutoVerified` only fills an empty slot, so it never replaces an older version a project
    already holds.
- **Routing hooks** (`machine.go`, `route` and `applyHook`): a hook is routed by session id, then
  by the deepest project worktree holding its cwd. A session outside every project is counted as
  "other projects" and dropped (`Model.ApplyHook`), so the check never sees it.
- **The page** (`state/model.go`, the view, then `web/static/panel.js`):
  - The view adds the check as a *warning* (`v.warn`, key `version:<v>` or `version:<v>:broken`).
  - `renderBanners` draws each warning as a full-width `.warnbar` with a ×. The × closes it for
    this browser by key, so it comes back on a new version.
  - A lane's Agents tab says "Approximate: …" while `heuristicsApprox` is true, which is the
    only use of the check in the view (`lv.SubagentsApprox`).
  - The footer shows the version under "All counters".
- **Help** (`web/index.html`, `#helpdlg`) says nothing about versions.

## The shape of the change

1. **The check's state, made explicit.** The view's `verification` says which of these holds:
   *verified* (nothing to show), *checking* (with the count), *unmatched* (three or more
   confirmations held open by one orphan), *changed* (a break, with what broke), *unread* (the
   version can't be read) or *verifying* (Verify now is running). The page renders it and decides
   nothing. The `version:<v>` warning for *checking* goes away. The break and *unread* warnings
   stay.
2. **A verification counts for every project, and is kept.** `saveReadings` saves the version
   verified for the running Claude Code, whichever project verified it. A verified version replaces
   an older one in every project, instead of only filling an empty slot.
3. **The status-bar field.** It's a field like Gate: a key, a value, and a click. It's shown only
   while the state is checking, unmatched, verifying or changed. Clicking it opens the version
   dialog: the explanation, the count, Verify now with its cost, and the last result.
4. **Verify now** (D3, D4, D5):
   - **One machine-wide probe, started only by the page's POST.** It runs `claude -p` with the
     lanes' scrubbed environment, in a new empty folder under `~/.clauductor/panel/`, on its own
     session id. The session has Haiku, only the Agent tool, one agent defined on the command line
     that has no tools, no MCP servers, no session saved to disk, a $0.25 budget cap, and nothing
     that can wait on a permission prompt.
   - **Its hooks are the panel's own, passed on the command line.** The probe gets the same
     async command hooks the panel installs (`install/hooks.go`, `hookEntry`), pointing at this
     panel's port, and loads no settings file. So it doesn't depend on the install, and it runs
     none of the user's or a project's other hooks. If Claude Code won't load zero settings
     sources (task 3.1 finds out), the fallback is the user's settings, where the installed
     panel's hooks already are.
   - **Its hooks are recognised by session id before routing.** They go to the probe's own count,
     using the same rules as `verify.go`, and never into a project's sessions, feed or "other
     projects" count.
   - **The verdict.** Three confirmations and no orphan verify the version for every project, saved
     as above. A break is a break: the same warning bar, saying it came from Verify now. Anything
     less is "not settled", and nothing changes.
   - **It stops** when the session exits, or at 90 s. At 90 s it gets an interrupt, then 10 s more,
     then a kill. The panel shutting down stops it the same way. The folder is removed afterwards.
   - **The result** (verified, not settled, changed, or stopped) goes into the view with its time
     and its cost at list price (from the session's JSON result). It shows in the dialog until
     the next version.
5. **Help** gets a "Claude Code versions" section (the wording is below). `docs/panel.md`'s two
   sections are rewritten to match.

## The wording

**The status-bar field.** The key is "Claude Code". The value is one of:

| State | Value | Ink |
|---|---|---|
| checking | `2.1.288 · checking, 1 of 3` | normal |
| unmatched | `2.1.288 · checking, 1 unmatched` | normal |
| verifying | `2.1.288 · verifying…` | normal |
| changed | `2.1.288 · changed` | warn |

Its tooltip: "Subagent lists are approximate until Claude Code 2.1.288 is checked. Click for what
that means and Verify now."

**The version dialog** (title "Claude Code 2.1.288"):

> The panel matches each subagent to the lane that started it using two details of Claude Code's
> hooks that aren't documented. They were checked on 2.1.284. On 2.1.288 the panel is checking
> them against the hooks your own sessions send: **1 of 3** subagent launches confirmed so far.
> It clears by itself after three. Nothing needs a restart, and the result is kept across
> restarts.
>
> Until then, only the subagent lists and counts (a lane's Agents tab, the Agents column) are
> approximate. Lane states, Needs you, the quota and the costs don't depend on it.

In the *unmatched* state, the count sentence reads instead: "3 subagent launches confirmed, but 1
launch named an agent Claude Code never announced, so the check can't clear by itself. Verify now
settles it, and so does a restart, which starts the count over."

Below that is the **Verify now** button, with this line under it:

> Runs one short Claude Code session on Haiku in an empty folder: three subagents that each answer
> "ok", with no tools. About 5 cents at list price; on a subscription it counts toward your 5-hour
> limit instead. It stops at $0.25 or 90 seconds, and never runs by itself.

**The results:**

- **Verified:** "Verified: Claude Code 2.1.288 matched 3 of 3 subagent launches (24 s, $0.04 at
  list price)."
- **Not settled:** "Not settled: 1 of 3 confirmed before the session ended (<why>). Nothing
  changed; the check carries on from your own sessions."
- **Changed:** "Changed: <what broke>. The warning bar says what to do."
- **Stopped:** "Stopped after 90 s with 1 of 3 confirmed. Nothing changed."

**The break bar** keeps today's text and adds where the evidence came from: "Claude Code 2.1.288
changed what the subagent pairing relies on: <what>. Subagent lists stay approximate until the
panel is updated for it (docs/panel.md, Version pinning)." When the break came from the probe,
the bar ends with " (found by Verify now)".

**Help**, a new section after "How to":

> **Claude Code versions**
>
> The panel matches each subagent to the lane that started it using two details of Claude Code's
> hooks that aren't documented. When Claude Code updates, the panel re-checks them from the hooks
> your own sessions send, and the status bar shows "Claude Code · checking, n of 3". Three matched
> subagent launches clear it by itself. Nothing needs a restart, and the result is kept.
>
> Until then, only the subagent lists and counts are approximate. Lane states, Needs you, the
> quota and the costs don't depend on it.
>
> To settle it now, click the field, then Verify now. It runs one short Claude Code session on
> Haiku (about 5 cents at list price, or part of your 5-hour limit on a subscription) and never
> runs by itself. If the check finds the details changed, a warning bar says so, and subagent
> lists stay approximate until the panel is updated.

## Refusals

| Situation | What happens instead |
|---|---|
| Verify now while the version is verified, unread or not yet read | The field isn't shown, so the button isn't offered. A direct POST gets 409 `not-checking`: "Claude Code <v> needs no check." |
| Verify now while a probe is running | 409 `running`: "Verify now is already running (started 20 s ago)." The button reads "Verifying…" and is disabled. |
| Claude Code is refreshing its login (`~/.claude/.oauth_refresh.lock` exists) | 409 `login-refresh`: "Claude Code is refreshing its login. Try again in a minute; starting a session now could fail, or make another session's refresh fail." Nothing is started (D4). |
| The 5-hour quota is at or over the guard (`quotaGuard`) | 409 `quota`: the guard's own sentence, as for a new lane. No override: the check clears by itself anyway. |
| The panel can't start a lane (`startBlocked`, e.g. an API key in tmux's environment) | 409 with the same sentence the New lane button shows. |
| The running version changes while the probe runs | The probe is stopped (interrupt, then kill). The result reads "Stopped: Claude Code changed to <v> during the check." |
| The session exits with no hooks at all | "Not settled: no hook arrived from the session, so this Claude Code may not run hooks in a session like this one. The check carries on from your own sessions." |
| Any request not from the page | Refused by the guards every POST passes (Host, Origin, cookie), as today. |

## Decisions (awaiting the owner)

**D1. Where the unverified version shows.**
- **Recommended:** a status-bar field beside Gate, shown only while there's something to say.
  Clicking it opens the version dialog.
- **Alternatives:**
  - a chip, as #68 put it;
  - a line in the footer;
  - keep the warning bar, but say more.
- **Why:**
  - **Not a chip:** the panel's design rules (`panel.css`, the header) forbid pills and chips, and
    a status-bar field is the panel's existing shape for a quiet figure you can click (Gate).
  - **Not the footer:** the version is already there, under "All counters". Nobody looks there,
    and it can't offer a button.
  - **Not the bar:** it's styled as an alert, and nothing is wrong.

**D2. What still raises the warning bar.**
- **Recommended:** a break, and an unreadable `claude --version`. Checking, unmatched and verifying
  go to the field only.
- **Alternative:** a break only, with an unreadable version in the field too.
- **Why:** both mean something is actually wrong, and the lists are approximate until someone
  acts. An unreadable `claude --version` usually means `claude` isn't on the panel's `PATH`, and
  then lanes can't start either.

**D3. What Verify now runs.**
- **Recommended:** a throwaway `claude -p` session the panel starts directly. It runs in an empty
  folder, with no tools beyond Agent and a tool-less agent, on its own session id, and its hooks
  are recognised by that id and kept out of every project.
- **Alternatives:**
  - start it as a panel lane (a tmux session in the project root) that you can watch;
  - no button: a Help explanation only.
- **Why:**
  - **Not a lane:** a lane in the project root loads the project's CLAUDE.md, skills and hooks.
    It would cost several times more and would run the project's own guards. It would also show
    up as a lane, in Needs you and in the feed, all for a check.
  - **Not just Help:** the owner asked for the button, and a quiet day keeps the check open
    indefinitely.
  - **Kept out of every project:** without recognising the session id, the session's hooks are
    dropped as "other projects", because an empty folder is no project's worktree, and the check
    would never see them.
  - **What isn't verified yet:**
    - that the panel's async command hooks fire in `-p` mode on the running version, and post
      before the process exits;
    - that hooks given with `--settings` work with no settings files loaded;
    - which exact flags give a tool-less agent.

    Task 3.1 checks all three by hand before anything is built on them, and stops for the owner if
    the hooks don't arrive.

**D4. Not racing a login refresh.**
- **Recommended:** Verify now's own guard, ahead of PANEL-28:
  - it refuses to start while `~/.claude/.oauth_refresh.lock` exists;
  - it stops with an interrupt and a 10 s grace before a kill, never a bare kill;
  - one probe at a time, and only when someone presses the button.
- **Alternative:** build the field, Help and the save fix now (groups 1–2), and hold Verify now
  (groups 3–4) until PANEL-28 has studied the race and fixed the panel's other calls.
- **Why:**
  - **The race is real and cited.** On 2026-10-02 the lock was left behind for 11+ minutes, and
    the error killed agents three times (PR #59). The panel's calls today run through
    `exec.CommandContext` (`signals.ExecRunner`), which kills with SIGKILL at the timeout. A
    session killed mid-refresh is the likeliest way to leave the lock behind.
  - **A probe is a full session.** It may refresh the login itself, so it must never be killed
    blind.
  - **The guard is cheap and doesn't wait on PANEL-28.** PANEL-28 can adopt it for `auth status`
    and `agents`.
  - **What the guard doesn't know:** the lock's exact semantics (who creates it, and whether a
    stale one blocks Claude Code). They're inferred from one observation. If the owner would rather
    not ship a session-starting button before PANEL-28 has studied them, the alternative costs
    only the wait.

**D5. How the cost is stated and capped.**
- **Recommended:**
  - **Before:** the line under the button gives an estimate ("About 5 cents at list price").
  - **After:** the result gives the actual, from the session's JSON `total_cost_usd`.
  - **Caps:** $0.25 (`--max-budget-usd`) and 90 s.
  - **Quota guard:** Verify now is refused while the guard is on.
- **Alternative:** a confirmation dialog with the estimate, and no cap.
- **Why:**
  - **No confirmation dialog:** the button and its cost already sit in a dialog you opened on
    purpose, so a second one adds a click and no information.
  - **The caps:** they bound the worst case, a model that loops or a hook that never comes.
  - **The estimate is unmeasured:** Haiku list prices and a short session put it under 5 cents.
    Task 3.1 measures it. If it measures above 5 cents, the builder stops and brings the figure
    back, because this wording is part of the approval.

**D6. "Accept this version" without evidence.**
- **Recommended:** not offered.
- **Alternative:** a second button, "Accept 2.1.288 without checking", worded as a risk.
- **Why:** accepting stops the check, so a real break on that version would then go unnoticed, and
  the subagent lists would be wrong without saying so. Verify now gives the same relief, with
  evidence, for a few cents. Revisit only if Verify now proves unreliable.
