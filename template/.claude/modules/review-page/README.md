# Optional module: the owner's review page (claude.ai Artifact)

With `REVIEW_PAGE="none"` (the default) the owner reviews a proposal as a PR: the files and a body
listing every decision awaiting them. This module replaces that with a **published review page**, a
claude.ai Artifact built for reading one decision at a time, which the owner can open on any device
and share.

**Needs** the Artifact tool (a Claude Code session signed in to claude.ai). A session without it
falls back to the PR form, and says so.

## Switching it on

Set `REVIEW_PAGE="artifact"` in `.claude/project.conf`. From then on `propose` step 3 builds the
page, and `.claude/checks/changes.sh` requires every open change's `proposal.md` to open with:

```markdown
**Review page:** https://claude.ai/artifact/<id>
```

(one URL form, so every proposal names its page the same way). The check can tell the line is
there; it cannot tell the page exists or was approved. The owner's approval is the control.

## Building the page (`propose` step 3)

- **Start with `Artifact` `action: "quickstart"`, `intent: "other"`**, and write the page to the
  session scratchpad as `proposal-<id>.html`. Use your design system's tokens if the project has
  one (light and dark); never invent brand values.
- **Sections, in this order:**
  1. a header naming the roadmap row, the change id and the revision, with status chips (awaiting
     approval, task-group count, anything irreversible such as a data migration);
  2. the `Slice:` line;
  3. *what changed from the previous revision* (from revision 2 on);
  4. *why now*;
  5. *what the existing specs already guarantee* (`propose` step 0b);
  6. the flow as numbered steps, each naming who acts and where;
  7. mockups, where a screen changes;
  8. **decisions**: a settled one reads `Decided · <owner>`; an open one carries a recommendation
     AND the alternative it beat;
  9. a refusals table: each situation where the change does not do its thing, and what happens
     instead;
  10. spec deltas beside what is out of scope;
  11. the task groups;
  12. a **Your call** footer: exactly what the owner is asked to decide, and what happens on
      "approve".
- **Publish** (`Artifact` with the file path and an `icon`), send the link with
  `PushNotification`, and **stop**.
- **Revise IN PLACE**: edit the same file and publish the same path again, so the URL never
  changes; bump the revision chip and the *what changed* section. A new URL per revision leaves
  the owner comparing two links, and the old one reads as current.
- **On approval**, `proposal.md` line 1 is the page's link, line 2 the `**Approved:**` line, and
  `design.md` records every decision as the owner decided it, including an adjustment they made in
  conversation (revise the page first, so the page and `design.md` agree).
