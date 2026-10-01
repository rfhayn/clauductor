# Optional module: risk-register

A living register of the project's top risks, read aloud at every session start and reviewed at
every session close, so a live risk is in front of you each session instead of sitting in a file
nothing opens.

**Off by default.** Nothing here runs until `MODULES` names it.

## Switching it on

1. `sh .claude/modules/risk-register/enable.sh`: makes `RISK_REGISTER` (default
   `docs/risk-register.md`) from this module's stub, `risk-register.md`, when it does not exist. An
   existing register is never touched.
2. Add `risk-register` to `MODULES` in `.claude/project.conf`. Set `RISK_REGISTER` there if the
   register lives elsewhere.

## What it adds

| Point | Part | What it does |
|---|---|---|
| `context.d/session-start/live-risks.sh` | session-start's context | one line per live risk: `R<n> <title> — gate <Gate> (<Issue>)` |
| `skills/session-close/risk-register.md` | session-close's *Project steps* | the review: did today change a risk's likelihood, mitigation, gate or owner? |
| `checks/register.sh` | `checks/run.sh` as `risk-register:register` | the register exists and every row is in the grammar below |

`.claude/checks/risk-register.sh` tests the parts themselves whether the module is on or not.

## The row grammar

```
| # | Risk | L | I | Mitigation | Gate | Issue |
|---|------|---|---|-----------|------|-------|
| R1 | **Title** — what could go wrong | M | H | what makes it smaller | Phase 1 | #10 |
```

- The first cell is `R<n>`, unique, and never reused once retired.
- The Risk cell opens with a **bold title**; the listing shows the title, not the whole cell.
- The **last two cells are Gate and Issue**, whatever the table has between (a `|` inside a cell
  shifts the middle columns, never these two). Neither is empty; write `—` for none.
- **A risk is closed by putting ✅ in its bold title** (`**✅ MITIGATED — Title**`). A ✅ elsewhere
  in the row, say on one half of a split mitigation, leaves it live. Keep closed rows: how the
  mitigation became real, and how the old text misled, is what a future session needs.

The section prints `none: …` for a register with no live risk and `CANNOT CHECK — …` for one it
cannot find, never nothing.
