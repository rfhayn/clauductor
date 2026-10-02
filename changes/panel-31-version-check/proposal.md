**Status:** awaiting approval
**Roadmap row:** PANEL-31
**Risk:** high

## Why

Every time Claude Code updates (every few days), the panel puts an orange warning bar across the
whole page (issue #68):

> Claude Code 2.1.287 is running; the subagent pairing was verified on 2.1.284. The panel is
> checking it against this project's own hooks (0 of 3 subagent launches confirmed); subagent
> lists are approximate until then.

The owner asked three questions: when does it clear, does it need a restart, and can there be a
button to force it?

- **When it clears.** After three confirmed subagent launches on the new version with none
  unmatched (`verify.go`, `confirm`). Confirmations only come from sessions that launch subagents,
  so on a quiet day the bar stays up.
- **Restart.** Not needed. The verified version is kept in `~/.clauductor/panel/verified.json`
  (`machine.go`, `saveReadings`), so a restart doesn't reset it. Reading the code turned up two
  cases where it never clears, though:
  - **With two projects, a project can stay stuck.** Each project's model keeps the last verified
    version, and a restored version never replaces one already held (`RestoreAutoVerified` only
    fills an empty one). `saveReadings` saves the first project's version it finds. So when the
    owner's first project still holds 2.1.287 and the second verifies 2.1.288, nothing is saved:
    the first project keeps its bar, and a restart loses the second project's result too.
  - **One unmatched launch holds the check open.** A single agent with no matching `SubagentStart`
    is below the break threshold (two), but `confirm` needs zero unmatched. The count then climbs
    past three ("5 of 3 confirmed") and never clears until a restart starts the count over.
- **A button.** None today. The only control is the bar's ×, which hides it in this browser until
  the next version.

The bar is styled as an alert, but nothing is wrong and there is nothing to do. What it says is
approximate, the subagent lists, is a small part of the page and the bar doesn't say which part.

## What changes

- **A new version is a quiet status-bar field, not a bar.** "Claude Code 2.1.288 · checking, 1 of
  3" sits in the status bar beside Gate. It goes away by itself once the version is verified.
  Clicking it opens a short explanation and **Verify now**.
- **Verify now settles it in about a minute, on demand.** It runs one short Claude Code session
  on Haiku in an empty folder. Three subagents each answer "ok", with no tools. The button says what
  that costs before you press it, and the result says what it cost. It never runs by itself.
- **A real break still alerts.** When the check finds the pairing changed, the warning bar shows
  as it does today, and the field says "changed".
- **Help says what's approximate meanwhile.** Only the subagent lists and counts are approximate.
  Lane states, Needs you, the quota and the costs don't depend on the check.
- **A version verified in any project counts for every project and is kept**, and one unmatched
  launch shows as such instead of silently counting past three.

## What the existing specs already guarantee

This repo has no living specs for the panel yet (`specs/` holds only its README; PANEL-29's
`panel-new-lane` is approved but not merged), so nothing here modifies or contradicts a
requirement. The behaviour this change alters is documented, not specified, in `docs/panel.md`
("Version pinning" and "Re-verifying a new version, by itself (PANEL-13)"). This change adds the
capability spec `panel-version-check` and updates those two sections to match.

**Scope note.** The roadmap row names `framework/internal/panel/state` and
`framework/internal/panel/web`. Verify now and the save fix also touch `framework/internal/panel`
itself (`machine.go`, where the version is polled and saved and hooks are routed). The row is
updated to say so when this proposal lands.

## Out of scope

- **The panel's other `claude` calls racing the login refresh** (`auth status`, `agents`): PANEL-28.
  Verify now has its own guard (D4), which PANEL-28 can reuse.
- **Re-checking the other 2.1.284 observations** (HTTP hooks not firing on `SessionStart`, the
  trust dialog missing from `claude agents`, the recorded fixtures). PANEL-13 checks only the
  pairing, and so does this change. They're re-recorded by hand when one breaks. No row owns
  automating them; open one if a break goes unnoticed.
- **Moving `HeuristicsVerifiedOn` forward** (`state/model.go`, "2.1.284"). It moves with a
  clauductor release. REL rows own releases.
- **"Accept this version" without evidence** (#68's optional item 3): not offered (D6). No row:
  revisit if Verify now proves unreliable.

## How we'll know

- **Signal:** after the next Claude Code update, the owner sees the status-bar field and no
  warning bar. Verify now clears it in under a minute at the cost it stated, and no agent fails
  with "another Claude Code process is refreshing it" while it runs.
- **Check after:** 14 days
