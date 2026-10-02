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
- **Reading the version** (`state/model.go`, `ApplyClaudeVersion`): a failed read records the
  error but keeps the last version read. The view's switch puts *checking* before *unread*. So
  when a read works and a later one fails, the page still shows the check in progress, and says
  nothing about the failure.
- **Saving it** (`framework/internal/panel/machine.go`):
  - `pollVersion` reads `claude --version` and logs a warning line when it isn't 2.1.284.
  - `saveReadings` writes the first project's verified version it finds to `verified.json` and
    pushes it to every project with `RestoreAutoVerified`.
  - `RestoreAutoVerified` only fills an empty slot. Once a project holds any verified version, a
    newer one pushed to it is ignored, whichever project verified it.
- **The page** (`state/model.go`, the view, then `web/static/panel.js`):
  - The view adds the check as a *warning* (`v.warn`, key `version:<v>` or `version:<v>:broken`).
  - `renderBanners` draws each warning as a full-width `.warnbar` with a ×. The × closes it for
    this browser by key, so it comes back on a new version.
  - A lane's Agents tab says "Approximate: …" while `heuristicsApprox` is true, which is the
    only use of the check in the view (`lv.SubagentsApprox`).
  - The footer shows the version under "All counters".
- **Help** (`web/index.html`, `#helpdlg`) says nothing about versions.

## The shape of the change

1. **The check's state, made explicit.** The view's `verification` carries one state for the
   field and the dialog, and the page renders it without deciding anything. In order of
   precedence, the first that holds wins (D2):
   - **changed:** a break, with what broke;
   - **unread:** the latest `claude --version` failed, even if an earlier read worked;
   - **checking:** with the count and the confirmations needed;
   - **verified:** nothing to show.

   The bars are separate from that state. The break bar shows whenever the check found a break,
   and the unread bar whenever the latest read failed, so both can show at once. The
   `version:<v>` warning for *checking* goes away.
2. **An unmatched launch costs one more confirmation, not a restart** (D5). The version is
   verified at three confirmations plus one for each orphan, while the orphans stay below the
   break threshold (two). Today one orphan holds the check open until a restart. After this change
   it means the check needs four confirmations, and it still clears by itself.
   - **Overdue launches are aged before every verify decision.** Today `observePost` and
     `observeStart` call `confirm` *before* `ageOrphans`, and ageing happens only when an Agent
     hook arrives. So a launch already overdue, but not yet counted as an orphan, can't stop the
     confirmation that verifies the version. Once the version is verified the check stops, and the
     break that ageing then finds is never shown.
   - Today that can happen with no orphan at all: the third match verifies while an overdue launch
     waits. With D5 it can also happen at the fourth match.
   - After this change both functions age first, so the decision always counts every launch that
     is more than 10 s overdue.
   - It follows that a launch whose own `SubagentStart` arrives more than 10 s after its
     `PostToolUse` always counts as an orphan, never as a confirmation.
3. **A verification counts for every project, and is kept.**
   - `saveReadings` saves the version verified for the running Claude Code, whichever project
     verified it.
   - It pushes that version to every project with a setter that replaces an older one.
     `RestoreAutoVerified` stays fill-only, for the start-up read of `verified.json`.
4. **The status-bar field.** It's a field like Gate: a key, a value, and a click. It's shown only
   while the state is checking or changed. Clicking it opens the version dialog: what the check
   is, what is approximate, the count, and when it clears.
5. **Help** gets a "Claude Code versions" section (the wording is below). `docs/panel.md`'s two
   sections are rewritten to match.

## The wording

**The status-bar field.** The key is "Claude Code". The value is one of:

| State | Value | Ink |
|---|---|---|
| checking | `2.1.288 · checking, 1 of 3` | normal |
| checking, with one unmatched launch | `2.1.288 · checking, 3 of 4` | normal |
| changed | `2.1.288 · changed` | warn |

Its tooltip: "Subagent lists are approximate until Claude Code 2.1.288 is checked. Click for what
that means."

**The version dialog** (title "Claude Code 2.1.288"):

> The panel matches each subagent to the lane that started it using two details of Claude Code's
> hooks that aren't documented. They were checked on 2.1.284. On 2.1.288 the panel is checking
> them against the hooks your own sessions send: **1 of 3** subagent launches confirmed so far.
> It clears by itself after three. Nothing needs a restart, and the result is kept across
> restarts.
>
> Until then, only the subagent lists and counts (a lane's Agents tab, the Agents column) are
> approximate. Lane states, Needs you, the quota and the costs don't depend on it.

With one unmatched launch, the dialog's first paragraph has one sentence replaced: "It clears by
itself after three." becomes "1 launch named an agent Claude Code never announced, so it clears by
itself at four instead of three." The count reads "**n of 4**". Every other sentence stays.

In the *changed* state, the dialog shows the break bar's text instead of both paragraphs.

**The bars** keep today's text:
- **A break:** "Claude Code 2.1.288 changed what the subagent pairing relies on: <what>. Subagent
  lists stay approximate until the panel is updated for it (docs/panel.md, Version pinning)."
- **Unread:** "cannot read `claude --version` (<error>); subagent lists are approximate."

**Help**, a new section after "How to":

> **Claude Code versions**
>
> The panel matches each subagent to the lane that started it using two details of Claude Code's
> hooks that aren't documented. When Claude Code updates, the panel re-checks them from the hooks
> your own sessions send, and the status bar shows "Claude Code · checking, n of 3 (or 4)". Three matched
> subagent launches clear it by itself, in every project, plus one more for each launch that
> didn't match. Nothing needs a restart, and the result is kept.
>
> Until then, only the subagent lists and counts are approximate. Lane states, Needs you, the
> quota and the costs don't depend on it.
>
> If the check finds the details changed, a warning bar says so, and subagent lists stay
> approximate until the panel is updated.

## Refusals

| Situation | What happens instead |
|---|---|
| The version is verified (in the source, or from live hooks in any project) | No field and no bar. |
| `claude --version` has never been read (the first poll is pending) | No field and no bar: nothing is known yet, and the first poll runs at start. |
| A read works, then a later one fails | The *unread* bar, and no field. The version last read isn't treated as current, and the subagent lists say they're approximate, as `heuristicsApprox`'s own comment already intends ("Unknown (not read yet, or unreadable) counts as approximate too") (D2). |
| A break, then a read fails | Both bars. The field stays "changed": a failed read doesn't hide a break (D2). |
| A break while the field's dialog is open | The dialog switches to the break's text. The bar appears as well. |

## Decisions (the owner decided all as recommended, 2026-10-02, including D3: Verify now becomes PANEL-32)

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
  - **Not the footer:** the version is already there, under "All counters". Nobody looks there.
  - **Not the bar:** it's styled as an alert, and nothing is wrong.

**D2. What still raises the warning bar, and which state wins.**
- **Recommended:**
  - **The bar:** a break, and an unreadable `claude --version`, each with its own bar, so both
    can show. Checking goes to the field only.
  - **Precedence:** a failed latest read wins over the check in progress, even when an earlier
    read worked. A break still wins over a failed read, because the break was found on real
    evidence, and a failed read says nothing about the pairing.
- **Alternative:** a break only, with an unreadable version shown in the field; precedence as
  today, where the last version read stays current after a failed read.
- **Why:**
  - **Both mean something is actually wrong**, and the lists stay approximate until someone acts.
    An unreadable `claude --version` usually means `claude` isn't on the panel's `PATH`, and then
    lanes can't start either.
  - **Today a later failure hides behind the check:** `ApplyClaudeVersion` keeps the last version,
    and the view shows *checking* first. With the field quiet, the failure would go unseen.

**D3. Verify now: in this change, or its own row after PANEL-28.**
- **Recommended:** split it out. PANEL-31 ships the field, the alert rules, the fixes, the
  dialog and Help. Verify now becomes **PANEL-32** (`panel-32-verify-now`, queued after PANEL-28),
  and reuses PANEL-28's `claude` gate.
- **Alternative:** keep Verify now in PANEL-31 (risk high) with what review found, briefly:
  - **The lock guard checks freshness, not existence.** Block while
    `<config dir>/.oauth_refresh.lock` was touched in the last 60 s. The config dir is
    `$CLAUDE_CONFIG_DIR`, else `~/.claude`, as in PANEL-28's draft (its D4 and D7). A stale lock
    gets its own sentence. Today's "exists" guard would refuse forever on the stale lock from the
    very incident it cites.
  - **One stop policy and one concurrency slot, shared with PANEL-28's gate**, not a second
    interrupt-then-kill policy outside the Runner.
  - **The owner approves flag invariants, not a flag list:**
    - only Agent executes;
    - nothing waits on a prompt;
    - no settings but the hooks passed on the command line;
    - the hooks arrive before the process exits.

    `.claude/evals/run.sh` (lines 268–272) is prior art: `-p --agents --model --output-format json
    --no-session-persistence --setting-sources project --permission-mode dontAsk --allowedTools
    --max-budget-usd`, with a receipt in
    `.claude/evals/receipts/reviewer-opus-high-2026-10-01.json`. `--setting-sources project` in an
    empty folder loads nothing. `--permission-prompts none` likely doesn't exist. The probe's
    hooks should be synchronous (`async: false`), so they post before exit.
  - **The feasibility spike runs before approval, by the owner,** not as a task after the field is
    built.
  - **Smaller points:**
    - name whose quota guard applies: the default project's thresholds, as `pollQuotaAlert` reads
      them (`machine.go`);
    - remove leftover probe folders at start and at shutdown;
    - make the 90 s and 10 s limits injectable for tests.
- **Why:**
  - **The owner asked for the button, and the split delays it.** That's the cost.
  - **For the split:**
    - **The other parts don't depend on the button.** The field, the alert rules and the fixes
      are safe on their own, and they end the page-wide alert now. With D5, nothing in PANEL-31
      needs a button or a restart to clear.
    - **The button depends on PANEL-28.** A session the panel starts can refresh the shared login
      itself, and PANEL-28 is establishing which `claude` calls are safe and is building one gate
      for them. Building Verify now before that gate exists means a second, separate policy that
      PANEL-28 would then have to reconcile.
    - **Its open questions are spike-sized.** Do hooks fire in `-p`? Which flags give a tool-less
      agent? What does it cost? Those questions belong before an approval, not inside a build.

**D4. "Accept this version" without evidence.**
- **Recommended:** not offered.
- **Alternative:** a button in the version dialog, "Accept 2.1.288 without checking", worded as a
  risk.
- **Why:** accepting stops the check, so a real break on that version would then go unnoticed, and
  the subagent lists would be wrong without saying so. With this change the cost of waiting is
  small: a quiet field, in one place, that says only the subagent lists are approximate. PANEL-32's
  row names "Accept this version" as one to revisit there if the check proves too slow to clear.

**D5. One unmatched launch.**
- **Recommended:** it costs one more confirmation. The version is verified at three confirmations
  plus one per orphan, while the orphans stay below the break threshold (two). With one orphan,
  four confirmations verify the version. Overdue launches are aged into orphans before every
  verify decision, never after it.
- **Alternatives:**
  - verify at three confirmations whenever the orphans are below the break threshold, so one
    orphan is simply ignored;
  - forgive an orphan once it is older than some number of minutes;
  - keep today's rule (any orphan blocks verification), and name a restart as the remedy: Help,
    the dialog and the Help requirement then say so for this case.
- **Why:**
  - **Today one orphan is a trap.** `confirm` needs zero orphans, and only a second orphan makes a
    break. So the count climbs past three ("5 of 3") and only a restart, which forgets the counts,
    gets out of it. Without Verify now, a restart would be the only remedy.
  - **One orphan isn't a break, by the existing threshold's own logic.** A background agent whose
    `SubagentStart` is late by more than 10 s can produce one. Today whether it does depends on
    other traffic, because ageing is lazy (it runs only when an Agent hook arrives). With ageing
    first (shape 2), a launch more than 10 s overdue always counts by the time it could matter.
  - **It is still evidence against the pairing**, which is why it costs a confirmation rather than
    being ignored. Once the version is verified the check stops, so a second orphan after that
    would never be seen. The extra confirmation is a small price against that, and it holds only
    with ageing first. Otherwise an overdue second launch could slip past the fourth match
    unseen.
  - **Not forgiving by age:** an orphan's age says nothing about whether the pairing held, and a
    timer is one more number to explain.
