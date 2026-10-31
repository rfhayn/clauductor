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
2. When one is a real decision or a lesson with a mechanism, promote it with `/new-adr`: copy
   [`TEMPLATE.md`](TEMPLATE.md) to `NNNN-<kebab-title>.md`, or amend the ADR that owns the
   principle. Then retag **every** insights row it cites.
3. Reference the ADR where it is enforced (a hook, a check, `AGENTS.md`'s table) so the decision
   and its guard stay in step.

## Numbering
Zero-padded and monotonic (`0001`, `0002`, …). Never renumber an ADR on `main`; supersede it (new
ADR `Accepted`, old one `Superseded by ADR-NNNN`). The next number comes from `origin/main` and the
open PRs, never from your branch (`new-adr` prints it). `.claude/checks/adr-numbering.sh` fails on
two files with one number and on an ADR missing from the index below. On a collision, the branch
that merges second takes the next free number.

## Index
| ADR | Title | Status |
|-----|-------|--------|
| [0001](0001-openspec-compatible-change-format.md) | Changes and specs are OpenSpec-compatible Markdown, written by our own skills | Accepted |
| [0002](0002-scenario-to-test-traceability.md) | Every scenario a change adds or modifies is cited by a test | Accepted |
| [0003](0003-one-panel-for-many-repositories.md) | One panel serves every repository on the machine | Accepted |
| [0004](0004-install-refuses-a-model-it-did-not-install.md) | install and update refuse a model stridal did not install | Accepted |
| [0005](0005-never-kill-a-tmux-server-from-a-script.md) | No script kills a tmux server, or a claude or stridal process | Accepted |
| [0006](0006-a-repository-works-without-stridal.md) | A repository works without stridal installed | Superseded by ADR-0016 |
| [0007](0007-plugin-distribution.md) | The model also ships as a Claude Code plugin, generated from template/ | Accepted (amended 2026-10-31) |
| [0008](0008-provenance-and-attribution-policy.md) | Provenance and attribution trailers: on in the template, off here | Accepted |
| [0009](0009-the-panel-carries-state-in-colour-at-rest.md) | The panel's look is the hybrid: state in colour at rest, one theme, one type system | Accepted |
| [0011](0011-open-source-terminal-first-plugin-installs.md) | Stridal is open source, terminal-first, and installed through the plugin | Accepted (amended 2026-10-31) |
| [0012](0012-one-merge-path-refused-everywhere-else.md) | One merge path: `merge.sh` merges, and `gh` refuses every other merge | Accepted (amended 2026-10-31) |
| [0016](0016-controls-queries-judgments.md) | Each part of the backend is one of five kinds, by what executes it | Accepted |
| [0017](0017-the-engine-is-the-stridal-binary.md) | The controls and queries live in the engine, the `stridal` binary | Accepted |
| [0018](0018-one-version-authority.md) | One version authority: the plugin release carries the engine, and the repo pins it | Accepted |
| [0019](0019-parity-first-ports.md) | A control is ported parity-first, and the shell version goes only with a parity receipt | Accepted |
| [0020](0020-judgment-gates.md) | Judgment gates: a model checks at an event, beside a control and never instead of one | Accepted |
| [0021](0021-the-roadmap-is-data.md) | The roadmap is data that the engine owns; the markdown and the owner's doc are views of it | Accepted |
| [0022](0022-panel-daemon-or-feed.md) | Live session state comes from a feed in files, or from the panel daemon | Proposed |
