# Risk register (living)

The project's top risks. **Living doc**: session-close reviews it, and session-start lists every
risk still live, so a live risk is in front of you every session instead of sitting in a file
nothing opens. Each risk names its mitigation, the **gate** by which that mitigation must be real
(a phase, a milestone, a roadmap row), and its tracking issue(s).

Scale: Likelihood / Impact each **L / M / H** (`M→L` when a mitigation has moved it). The owner is
the project's owner unless the row says otherwise.

**The row grammar** (`.claude/modules/risk-register/README.md`): the first cell is `R<n>`, never
reused; the Risk cell opens with a **bold title**; the last two cells are Gate and Issue. A risk is
closed by putting ✅ in its bold title, `**✅ MITIGATED — <title>**`, and the row stays: record how
the mitigation became real and how the old text misled.

A row, for the shape (indented here so it is not read as a risk; add yours to the table below
without the indent):

    | R1 | **No delivery pipeline** — merged code reaches production by hand | M | H | A scripted deploy with a health check and a rollback | Phase 1 | #10 |

| # | Risk | L | I | Mitigation | Gate | Issue |
|---|------|---|---|-----------|------|-------|
