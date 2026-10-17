**Approved:** 2026-10-17 by Rich · design c802ba9d5463
**Roadmap row:** PANEL-49
**Risk:** normal
**Budget:** $60

## Why
An attached repository's lanes show no context, no cost and no context alert. The panel gets
those figures only from a status line that posts to it. A repository that is attached, rather than
installed, keeps its own status line, and Standing Tee's does not post. So the lanes the owner's
context review was about show nothing on the panel. That review was the evidence behind PANEL-48:
Standing Tee's week of 2026-10-08..14, $1,208.28 at list price.

PANEL-48 caps those lanes' context at launch, and its bar, tick and alert read against the window
it set (its design D7 and D13). Its proposal names the gap: "an attached lane with no status line
shows no bar". Until this change, the cap's effect is invisible in the repository it was built for.

The panel already knows each lane's session id, because it passes it with `--session-id`. Claude
Code writes that session's transcript, with each API call's model and token counts, under its config
directory. The owner's audit tool already prices those records. Run against PANEL-48's probe
transcript on Claude Code 2.1.302, it gave $0.254 where the status line said $0.25 (PANEL-48
`evidence/README.md`, *Probe results*).

**Where it sits in the queue:** after PANEL-48, by the owner's order of 2026-10-15 (#131). PANEL-50,
the per-lane contributors view, builds on it.

## What changes
In a project the panel serves through `panel attach`, each lane shows its context bar, its est. $
and its context alert, read from the lane's own transcript.
- **Context:** the lane's last API call, measured against the window PANEL-48 recorded for the
  lane. PANEL-48's tick and alert threshold apply unchanged.
- **Cost:** priced per model from a table in the panel. A model the table does not know is
  UNPRICED: its calls are left out of the figure, never counted at $0, and the page says which.
- **A shape it does not know** reads as "could not read", never 0. The lane shows no figure
  until the panel restarts and reads it whole, and the page shows a "Cannot read transcripts"
  banner naming the session, the line and Claude Code's version.
- **The quota:** a transcript carries no rate limits, so it stays unknown on a machine where no
  status line posts. The page's quota note says so. This is the one `panel.js` change.
- **The invariant changes.** "The panel never reads transcripts" becomes "the panel reads
  transcripts in one file, for attached projects only, and decodes only the fields named in
  `design.md` D2". The rewritten test holds every other source to the old rule (D8).

A project whose config is in its repository, like this one, is measured exactly as today.

## What the existing specs already guarantee
- **`specs/panel-attach`:**
  - ATTACH-2 (nothing written under the repository) holds. The reader opens files read-only, and
    only under Claude Code's config directory.
  - ATTACH-3 (a sidecar runs only the repository's argv commands) is untouched. The reader runs no
    command.
- **`specs/panel-claude-calls`:** it covers the panel's own `claude` calls, and the reader makes
  none. Unchanged.
- **`specs/panel-new-lane`:** it covers where a lane starts. Unchanged.
- **`panel-lane-context` (PANEL-48, in build, not yet in `specs/`):**
  - This change relies on LANECTX-2: the context is measured against the recorded window, with the
    tick at the window less 33k and the alert at the lower of `alerts.context_pct` and the tick
    less 5. It feeds that requirement's computation from a second source and changes none of its
    requirements.
  - LANECTX-2 keeps Claude Code's `used_percentage` for a lane launched without a window. It
    holds in an attached project too: a status post still sets such a session's context (D3).
    Where no status line posts, a transcript carries no such figure, so the lane shows no
    percentage (D4).

This change adds one capability, `panel-lane-measures` (MEASURES-1 to MEASURES-5), and modifies
no requirement.

**The row, re-measured:** it writes `signals`, `state`, the panel's runtime (one source in
`runtime.go`, and `live.go` passing it the project's kind, beside `notranscript_test.go`, which
the row names), one string in `panel.js`, and
`docs/panel.md`. That is three code surfaces of substance and no MODIFIED requirement, so it stays
one row.

## Out of scope
- **The per-lane top context contributors:** PANEL-50. They need content attribution, which D2's
  field list forbids. PANEL-50 widens that list by its own decision, and the test makes the
  widening a visible diff.
- **Reading transcripts in a project whose config is in its repository:** not proposed (D3). Its
  status line already gives these figures for free.
- **A model-window table** for a lane launched without a window: not proposed (D4).
- **The quota from transcripts:** impossible, since a transcript carries no rate limits.

## How we'll know
- **Signal:** once the owner's panel runs a build with this change and Standing Tee's sidecar
  carries PANEL-48's `launch` object, measured over one week:
  - Standing Tee's lanes show a context bar and an est. $ on the panel.
  - The week's spend for the attached project in its ledger (`spend.json`) is within 10% of the
    evidence tool's figure for the same week's sessions that ran in lanes (PANEL-48
    `evidence/tool/`, `audit.mjs` then `driver.mjs`).
  - No "Cannot read transcripts" banner shows, except across a Claude Code upgrade that changed
    the format. If one does, the banner names the version, and a fix row is opened.
- **Check after:** 14 days
