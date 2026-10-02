# Modules

A module is an optional capability with declared contribution points: the operating model's hosts
(the merge guard, the session context scripts, the health lines, the checks, the skills and
session-close's conflict table) find its parts by where they sit, and run them only while it is on.

```
.claude/modules/<name>/
  module.conf                       name, requires, enables; UPPERCASE default keys
  README.md                         what it is and how to switch it on
  guard.d/*.sh                      extra pr-merge-guard rules
  context.d/session-start/*.sh      extra sections in session-start's context
  context.d/session-close/*.sh      extra sections in session-close's context
  health/*.sh                       extra session-start health lines
  checks/*.sh                       extra process checks (checks/run.sh names them <name>:<check>)
  skills/<skill>/*.md               fragments appended to a template skill ("## Project steps")
  conflicts.tsv                     rows for session-close's shared-file conflict table
  roadmap.d/*.sh                    rules over the parsed change queue (roadmap_queue runs them)
```

**On and off.** `MODULES="a b"` in `.claude/project.conf` is the list; a module not named there
contributes nothing, whatever it contains. The two older switches still work:
`PROPOSALS="openspec"` turns `openspec` on and `REVIEW_PAGE="artifact"` turns `review-page` on
(and turning the module on sets the switch).

**module.conf** is plain `key="value"` lines:

| Key | Meaning |
|---|---|
| `name` | the module's name; must equal its directory |
| `requires` | other modules that must be on too (space-separated); one that is not fails `checks/modules.sh` and the merge guard |
| `enables` | the points it contributes, from `guard.d context.d health checks skills conflicts.tsv roadmap.d`; a part on disk it does not declare, or a declared one that is missing, fails `checks/modules.sh` |
| `UPPERCASE="value"` | a default for that project.conf key, applied only where the project sets nothing (no `$`, backtick or backslash) |

**The contract of each point** is the local layer's (`.claude/local/README.md`): the same points,
the same rules. A module's parts run before the local layer's, in `MODULES` order, and within a
directory by file name. `sh .claude/extensions.sh list` shows what is on and what each layer adds.

**Where modules live.** The modules here are the framework's: `clauductor update` refreshes them.
A project may vendor its own module in this directory under another name; a shipped module of the
same name wins. Every module must work without clauductor (D10), and one that needs a claude.ai
tool says `CANNOT CHECK — no Artifact tool` rather than erroring.

## Shipped

| Module | Contributes |
|---|---|
| `openspec` | `checks/project.sh` (this project's links and `openspec validate`), an `archive-change` fragment; `enable.sh` makes the links |
| `review-page` | a `propose` fragment: the owner's claude.ai review page |
| `artifacts` | shared pages and walkthroughs held current with the sources they declare (`ARTIFACT_REGISTRY`): health lines, context sections, a guard rule blocking a session close while one is BEHIND, session and merge fragments, a conflict row, `checks/registry.sh`, and `bin/currency.sh --stamp`; sh, git and jq, plus Node only for the optional claude.ai copies (`ARTIFACT_PUBLISH`) |
| `people` | a people registry (`PEOPLE`); session-start's who-is-on-what and lane table; a roadmap rule that every `**Owner:**` is a person; conflict rows; `checks/registry.sh` |
