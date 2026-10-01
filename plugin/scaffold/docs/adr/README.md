# Architecture Decision Records (ADRs)

Durable, promoted records of decisions with lasting consequences: the tier above the raw
[`../insights-log.md`](../insights-log.md). ADRs are the owner's decisions (AGENTS.md, *Who
decides*); a session drafts one `Proposed` and puts it in front of them.

## What this tier holds

Two kinds of record, and both belong here:
- **Architecture decisions**: choices with trade-offs about how the system is built.
- **Lessons that have a mechanism**: rules about testing, review, evidence and process, each with
  something that executes it, or a plain statement that nothing does. `AGENTS.md` does not take
  new lessons (it changes only to add a mechanism or delete something), so this tier is a
  lesson's durable home. The insights log is intake, not a home.

How to route a lesson:
- **A new principle** gets a new ADR.
- **A refinement of an existing ADR's principle**, or a new instance that changes what it says, is
  an **amendment**: a dated `## Amendment — YYYY-MM-DD: …` section inside that ADR, with its own
  source note and Enforcement line. Splitting one principle across two files leaves two documents
  that must agree.
- **A further example that changes nothing** is not written here: its insights-log row takes
  `Instance of ADR-NNNN (check N)`.
- **A technique with no decision and no mechanism** stays in the log as `Technique — no
  mechanism`.

Every ADR and every amendment names what executes it in an **Enforcement** line, or says plainly
that nothing does. `docs/principles.md` holds the principles this operating model starts from;
promote your own versions of them here as your project confirms or amends them.

## Process
1. Capture raw observations in the insights log during the work.
2. When one is a real decision or a lesson with a mechanism, promote it with `/clauductor:new-adr`: copy
   [`TEMPLATE.md`](TEMPLATE.md) to `NNNN-<kebab-title>.md`, or amend the ADR that owns the
   principle. Then retag **every** insights row it cites.
3. Reference the ADR where it is enforced (a hook, a check, `AGENTS.md`'s table) so the decision
   and its guard stay in step.

## Numbering
Zero-padded and monotonic (`0001`, `0002`, …). Never renumber an ADR on `main`; supersede it (new
ADR `Accepted`, old one `Superseded by ADR-NNNN`). The next number comes from `origin/main` and the
open PRs, never from your branch (`new-adr` prints it). `${CLAUDE_PLUGIN_ROOT}/checks/adr-numbering.sh` fails on
two files with one number and on an ADR missing from the index below. On a collision, the branch
that merges second takes the next free number. Each ADR opens `# ADR NNNN: <title>` and has an
`## Enforcement` section; a project that adopted this with ADRs of its own sets `RECORDS_BASELINE`
(`.claude/project.conf`), and the ADRs from before it keep their headings as written.

## Index
| ADR | Title | Status |
|-----|-------|--------|
