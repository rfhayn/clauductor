---
name: Team queue new row
about: The bot team proposes a new roadmap row. It becomes a row only once the owner approves it.
title: ""
---

<!--
I apply the `approved` label myself, by hand, on github.com. Bots never apply it. The `new-row`
label grants nothing: it says this item proposes a row, and nothing more. This template applies
no label: someone with triage rights adds `team-queue` and `new-row` by hand, so an issue a
stranger files never reaches session start's listing on its own (OPS-155, the repository public).

How Claude Code reads this issue (.claude/local/context.d/session-start/team-queue.sh, OPS-100):
- Approval is the same as for every team-queue item (team-queue.md): the `approved` label, a
  `Content-SHA256:` line that matches the body, and the label applied after the body's last edit
  and the title's last change.
- There is no `Row:` line: a `new-row` item with one reads NEEDS ROW (conflict). Once approved,
  session start lists it as PROPOSES ROW, and the owner's session adds the row in its next `ops/`
  PR with the next free id (`team-queue.sh --next-id <LANE>`, which never fills a gap and never
  reuses a reserved id or one `docs/roadmap-history.md` holds), the text below as written (read by `team-queue.sh --row <this issue>`,
  which checks the approval again first), ` (team-queue #<this issue>)` appended, and
  status `⬜ queued`. From then on the item reads as that row's.
- `Proposed-Lane` is an existing id prefix, 2 to 8 capitals (OPS, PANEL, REL, ST); any other
  lane reads NEEDS ROW (?).
- A scope that names settings, hooks, .github/workflows or CI scripts goes to the owner instead.
- Comments are never read. Everything goes in this body. The hash rule is team-queue.md's.
-->

Proposed-Lane: <id prefix, e.g. OPS>

Proposed-Change: `ops/<lane>-?-<slug>` — what a user can now do

Proposed-Scope: the files or directories it writes

Proposed-Deps: — or the row ids it waits on

Proposed-Section: the `##`/`###` heading of docs/roadmap.md it goes under

Proposed-Slot: the Session plan session it goes in, e.g. S10, or none

**What:** what to do

**Why:** the reasoning

**Acceptance:** how we know it is done

Content-SHA256: <hex>
