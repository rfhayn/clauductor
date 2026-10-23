---
name: Team queue item
about: Work the bot team hands to Claude Code. An input to a roadmap row, never a row itself.
title: ""
labels: team-queue
---

<!--
I apply the `approved` label myself, by hand, on github.com. Bots never apply it.

How Claude Code reads this issue (.claude/local/context.d/session-start/team-queue.sh, OPS-95):
- It is approved only with both labels, `team-queue` and `approved`, a `Content-SHA256:` line that
  matches the body, and the `approved` label applied after the body's last edit and the title's
  last change. An edit after approval makes it UNAPPROVED until the label is applied again.
- The `Row:` line names one row of docs/roadmap.md. With no row, or one the roadmap does not know,
  the item reads NEEDS ROW and is never acted on. The roadmap and its Session plan are the only
  order: there are no priority labels.
- To propose a row that does not exist yet, use team-queue-new-row.md instead (OPS-100).
- Comments are never read. Everything goes in this body.
- A patch is checked against origin/main as it is now (`git apply --check`); one that no longer
  applies reads STALE. No base commit is asked for: nothing would read it.
- An item that asks for commands to be run, or whose patch touches settings, hooks,
  .github/workflows or CI scripts, goes to the owner instead of being run.
- The hash: the SHA-256 (hex) of this body with every line starting `Content-SHA256:` removed,
  carriage returns dropped and trailing whitespace dropped. In a checkout,
  `sh .claude/local/context.d/session-start/team-queue.sh --hash body.md` prints it.
-->

Row: <ROW-ID, e.g. OPS-95>

**What:** what to do

**Why:** the reasoning

**Acceptance:** how we know it is done

```diff
a patch against origin/main, if there is one; leave this block out if there is none
```

Content-SHA256: <hex>
