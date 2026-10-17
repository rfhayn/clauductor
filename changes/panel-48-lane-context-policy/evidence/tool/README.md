# The audit tool behind `evidence/README.md`

These are the three scripts that measured Standing Tee's week of 2026-10-08..14 (D12). The outcome
check runs them again on a later week and compares the figures with the baseline. No transcript
data or output is kept here. Write the output to a scratchpad, never into the repository.

| Script | What it does |
|---|---|
| `usage-report.mjs` | The library: it reads transcripts, prices calls and attributes their content. It came from Standing Tee's `infra/usage-report.mjs`, so its header still names that repository's paths. |
| `audit.mjs` | It walks every transcript of one repository in a time window and writes `calls.jsonl`, `items.jsonl` and `subject.json` into an output folder. |
| `driver.mjs` | It reads those three files and prints the driver-decided measures: the caps, startup, wakes, the levers and the combined figure. It also writes `driver.json` and its CSVs beside them. |

## Re-running it

They need Node 18 or later and no packages.

```sh
OUT="$TMPDIR/panel-48-outcome"            # a scratch folder, never inside the repository
mkdir -p "$OUT"
node audit.mjs "$OUT" 2026-10-08 2026-10-15 "$HOME/.claude/projects/<slug>"
node driver.mjs "$OUT" > "$OUT/driver.md"
```

- **`<slug>`** names the repository's main checkout's transcript folder: its absolute path with
  every character that is not a letter or a digit turned into `-`. For a checkout at
  `/Users/<you>/Development/standingtee` that is `-Users-<you>-Development-standingtee`. The
  checkout's worktree folders (`<slug>--claude-worktrees-<name>`) are found beside it.
- **The window:** `since` is inclusive and `until` exclusive, both compared against each call's
  ISO timestamp. The baseline is `2026-10-08 2026-10-15`. The outcome check uses a week after the
  change has merged.
- **`driver.mjs <dir> [capClasses]`:** the optional second argument is a comma-separated list of
  classes for the combined cap, such as `orchestrator,lane:ops`. By default the classes are the
  ones where a 200k cap pays on its own.
- **Comparing:** the baseline's headline is $1,208.28 over 76 sessions, 393 subagents and 18,378
  calls. Each figure in `evidence/README.md` section 3 is a line of `driver.md`.

## Named limits
- **Path normalisation:** `normPath` in `usage-report.mjs` shortens file paths only under a
  checkout at `~/Development/standingtee`. Elsewhere, a file's key keeps its full path. That
  splits the per-file contributor table, but no total changes.
- **Scratchpad sessions:** sessions that ran in a scratchpad copy of the repository are listed in
  `subject.json` under `excluded` and not counted.
- **Running `usage-report.mjs` in this repository:** in a Claude Code session here,
  `no-blind-source-rewrite.sh` blocks `node usage-report.mjs ...`. It reads the script's own
  `.replace(` calls and its mention of `.claude/model-roles.json` as a rewrite, though the script
  writes nothing. `audit.mjs` and `driver.mjs` import it and are not blocked. To run the report on
  its own, run it from a shell.
- **Check after an edit:** `audit.mjs` takes its transcript folder as an argument (task 1.5). On
  2026-10-17 it wrote byte-identical `calls.jsonl`, `items.jsonl` and `subject.json` to the
  version with the hard-coded folder, over the window 2026-10-14..15.
