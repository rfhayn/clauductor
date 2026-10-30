# Practice records (PRAC)

Durable, promoted records of **how we work**: principles about testing, review, evidence and
process, each with something that executes it, and the tooling decisions behind them. The tier
above the raw [`../insights-log.md`](../insights-log.md). Decisions about how the system is built
are ADRs, in [`../adr/`](../adr/README.md).

## What this series holds

- **A lesson that has a mechanism**: a rule about testing, review, evidence or process, promoted
  from incidents, with something that executes it, or a plain statement that nothing does.
- **A tooling decision behind one**: how the work is run (the gate, the review loop, the lanes),
  where there were real alternatives.

A practice churns more than an architecture decision: it is amended in place as its mechanism
changes. Practice records are the owner's decisions like ADRs (AGENTS.md, *Who decides*): a
session drafts one `Proposed` and puts it in front of them.

How to route a lesson:
- **A new principle** gets a new practice record.
- **A refinement of an existing record's principle**, or a new instance that changes what it says,
  is an **amendment**: a dated `## Amendment — YYYY-MM-DD: …` section inside that record, with its
  own source note and Enforcement line.
- **A further example that changes nothing** is not written here: its insights-log row takes
  `Instance of PRAC-NNNN (check N)`.
- **A lesson already filed as an ADR stays one.** A record never moves between the two series;
  a new practice record may supersede it.

## Process
1. Capture raw observations in the insights log during the work.
2. Promote one with `/new-adr`, which asks first whether it is a decision (ADR) or a practice
   (PRAC): copy [`TEMPLATE.md`](TEMPLATE.md) to `NNNN-<kebab-title>.md`, or amend the record that
   owns the principle. Then retag **every** insights row it cites `Promoted → PRAC-NNNN`.
3. Reference the record where it is enforced (a hook, a check, `AGENTS.md`'s table).

## Numbering
**One number space with [`../adr/`](../adr/README.md)**: the next record of either kind takes the
next free number, so `0007` names one record, an ADR or a practice. The number comes from
`origin/main` and the open PRs across both directories, never from your branch (`new-adr` prints
it). `.claude/checks/adr-numbering.sh` fails on a number used twice across both, on a record
missing from its directory's index, and on a record since adoption that does not open
`# PRAC NNNN: <title>` or has no `## Enforcement` section (`RECORDS_BASELINE` keeps a project's
older records as written). Never renumber a record on `main`; supersede it.

## Index
| PRAC | Title | Status |
|------|-------|--------|
| [0010](0010-close-the-class-not-the-instance.md) | Close the class, not the instance, when review keeps finding the next one | Accepted |
| [0013](0013-batch-reviewed-lanes-one-gate-per-batch.md) | Batch reviewed lanes into one integration branch, one gate per batch | Accepted |
| [0015](0015-a-classifier-stop-is-a-park-signal.md) | A classifier stop is a park signal, by a rule agreed before the retry | Accepted |
