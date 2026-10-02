**Status:** awaiting approval
**Roadmap row:** PANEL-31
**Risk:** low

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
- **Restart.** Usually not needed today. The verified version is kept in
  `~/.clauductor/panel/verified.json` (`machine.go`, `saveReadings`), so a restart doesn't reset
  it. But reading the code turned up two cases where today it never clears by itself (both are
  fixed by this change, after which nothing needs a restart):
  - **With two projects, the verification doesn't reach the other project.** Each project's model
    keeps the last verified version, and `saveReadings` pushes a new one with
    `RestoreAutoVerified`, which only fills an empty slot. So once any older version has been
    verified, a new verification never reaches the other projects, whichever project made it.
    They keep their bar.
    And `saveReadings` saves the first project's version it finds. When the second project
    verifies 2.1.288 while the first still holds 2.1.287, nothing is saved, and a restart loses
    the second project's result too.
  - **One unmatched launch holds the check open.** A single agent with no matching `SubagentStart`
    is below the break threshold (two), but `confirm` needs zero unmatched. Today the count then
    climbs past three ("5 of 3 confirmed"), and the check clears only when a restart forgets the
    counts.
- **A button.** None today. The only control is the bar's ×, which hides it in this browser until
  the next version. A button that settles the check means the panel starting a Claude Code session
  of its own, which runs into the login-refresh race PANEL-28 is studying. This proposal
  recommends building it next, as PANEL-32, on PANEL-28's guard (D3).

The bar is styled as an alert, but nothing is wrong and there is nothing to do. What it says is
approximate, the subagent lists, is a small part of the page and the bar doesn't say which part.

## What changes

- **A new version is a quiet status-bar field, not a bar.** "Claude Code 2.1.288 · checking, 1 of
  3" sits in the status bar beside Gate. It goes away by itself once the version is verified.
  Clicking it opens a short explanation of what is approximate and when it clears.
- **A real break still alerts**, and so does a `claude --version` that can't be read, even after an
  earlier read worked. The field says "changed" for a break.
- **Help says what's approximate meanwhile.** Only the subagent lists and counts are approximate.
  Lane states, Needs you, the quota and the costs don't depend on the check.
- **A version verified in any project counts for every project and is kept.**
- **One unmatched launch costs one more confirmation instead of holding the check open** ("3 of
  4", D5). So the check always clears by itself, and nothing needs a restart.
- **A break and an unreadable version each get their own bar.** A failed read wins over the check
  in progress, but never hides a break (D2).

## What the existing specs already guarantee

This repo has no living specs for the panel yet (`specs/` holds only its README; PANEL-29's
`panel-new-lane` is approved but not merged), so nothing here modifies or contradicts a
requirement. The behaviour this change alters is documented, not specified, in `docs/panel.md`
("Version pinning" and "Re-verifying a new version, by itself (PANEL-13)"). This change adds the
capability spec `panel-version-check` and updates those two sections to match.

**Scope.** The save fix is in `framework/internal/panel/machine.go`, outside the row's original
scope (`state`, `web`). This proposal's PR updates the row in `docs/roadmap.md`:
- it adds `framework/internal/panel` to the scope;
- it says "status-bar field" instead of "chip" (D1);
- it moves "Verify now" to a new row, PANEL-32.

## Out of scope

- **Verify now**, the button that runs a throwaway session to settle the check: PANEL-32, after
  PANEL-28 (D3). PANEL-32's row carries what this proposal's review found it needs.
- **The panel's other `claude` calls racing the login refresh** (`auth status`, `agents`): PANEL-28.
- **Re-checking the other 2.1.284 observations** (HTTP hooks not firing on `SessionStart`, the
  trust dialog missing from `claude agents`, the recorded fixtures). PANEL-13 checks only the
  pairing, and so does this change. They're re-recorded by hand when one breaks. No row owns
  automating them; open one if a break goes unnoticed.
- **Moving `HeuristicsVerifiedOn` forward** (`state/model.go`, "2.1.284"). It moves with a
  clauductor release. REL rows own releases.
- **"Accept this version" without evidence** (#68's optional item 3): not offered (D4). PANEL-32's
  row names it for a revisit if the check proves too slow to clear.

## How we'll know

- **Signal:** after the next Claude Code update, the owner sees the status-bar field and no
  warning bar, in every project. Once a project's sessions confirm three subagent launches (four
  if one didn't match), the field goes away in all of them, and it stays gone after a panel
  restart.
- **Check after:** 14 days
