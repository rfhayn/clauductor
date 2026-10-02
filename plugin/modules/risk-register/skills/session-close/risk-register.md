**Review the risk register** (`RISK_REGISTER` in `.claude/project.conf`, `docs/risk-register.md`
by default), with the records, before the journal: did today change a risk's likelihood, its
mitigation, its gate or its owner? Edit the row. A new risk gets the next `R<n>` (never reuse a
retired number). Close one by putting ✅ in its bold title (`**✅ MITIGATED — <title>**`) and keep the
row: record how the mitigation became real **and how the old text misled**, which is the half a
future session needs. session-start lists every row whose title has no ✅, so an edit here is what
the next session reads. `clauductor-model checks/run.sh risk-register:register` holds the rows' shape.
