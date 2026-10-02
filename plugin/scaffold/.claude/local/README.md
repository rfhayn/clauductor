# The local layer

`.claude/local/` is this project's own extension of the operating model. **`clauductor install`
and `clauductor update` never write a file here** (the doc tier: this README is created once, if
missing), and the plugin's `/clauductor:init` only scaffolds it. Put here what is yours and would
otherwise be an edit to a framework file that the next update overwrites.

It is always on. It offers the same points as a module (`${CLAUDE_PLUGIN_ROOT}/modules/README.md`), and its parts
run after the enabled modules'. Each point's contract:

| Point | Run by | Contract |
|---|---|---|
| `guard.d/*.sh` | `pr-merge-guard.sh`, after its own rules, on a merge of this repo it would otherwise allow | Its own process, per its `#!` line, payload on stdin, `GUARD_PR`, `GUARD_HEAD` (sha), `GUARD_BRANCH`, `GUARD_BASE` (merge base), `GUARD_REPO`, `GUARD_COMMAND`, `GUARD_PAYLOAD` (a file), `ROOT`, `MAIN_BRANCH` in its environment. **Exit 0** allows, each stdout line an advisory that reaches Claude; **exit 2** blocks, stderr the reason. Anything else blocks (fail closed): a rule that does not parse, cannot start, exits 1, or runs past `GUARD_RULE_TIMEOUT` seconds (60). |
| `context.d/session-start/*.sh`, `context.d/session-close/*.sh` | the skill's `context.sh`, after its own sections | Print the section's lines; the heading is the file name. Honour `CONTEXT_OFFLINE=1` (no network: print `CANNOT CHECK — offline`). A section that exits non-zero or prints nothing is shown as `CANNOT CHECK`, never as nothing. |
| `health/*.sh` | session-start, after `.claude/health/` | `.claude/health/README.md`: one verdict line per subject (`OK`, `FAILED`, `STALE`, `STUCK`, `RUNNING`, `NEVER RAN`, `CANNOT CHECK — <why>`), exit 0. |
| `checks/*.sh` | `checks/run.sh`, as `local:<name>` | Start with `. "${CHECKS_LIB:?}"` for `ok`, `fail`, `finish`, `scratch`; print `ok`/`FAIL` lines; exit non-zero on a failure. A check that cannot run what it checks fails. |
| `skills/<skill>/*.md` | the skill's `## Project steps` include line | Markdown appended to that template skill's instructions, in file-name order. |
| `conflicts.tsv` | session-close's shared-file table | One row per shared file: the file, a TAB, what to do on a conflict. `#` comments allowed. |
| `roadmap.d/*.sh` | `roadmap_queue` (`lib/conf.sh`), on every read of the change queue in any mode | stdin: the `--tsv` rows; `ROOT` and `ROADMAP` in its environment; run in the project root. **Exit 0** accepts; anything else makes the queue UNKNOWN for every reader, each stdout line naming a refused row. No network: it runs on every read. |

Every script runs per its own `#!` line (`#!/usr/bin/env bash` runs under bash, never forced
through `sh`), and with no `#!` line under `sh`. `clauductor-model extensions.sh list` shows every part.

`checks/modules.sh` holds this to its contract: the local layer's parts load, a guard rule that
does not parse blocks, and a disabled module's parts never run.
