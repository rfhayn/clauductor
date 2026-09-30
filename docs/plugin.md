# Clauductor as a Claude Code plugin

The operating model in `template/` ships two ways:

| | `clauductor install` | The Claude Code plugin |
|---|---|---|
| Needs | the `clauductor` binary (Go) | Claude Code only |
| Framework files (skills, agents, hooks, checks, workflow) | copied into the repo's `.claude/`, editable, updated by `clauductor update` | live in Claude Code's plugin cache, read-only, updated by `/plugin update` |
| Project files (AGENTS.md, project.conf, docs/, scripts/ci/, ...) | copied into the repo | copied into the repo by `/clauductor:init` |
| Skill names | `/session-start` | `/clauductor:session-start` |
| The panel (`clauductor panel`) | works | works (it needs the binary either way) |

Pick one per repository. Both install the same model from the same `template/`.

## Install with the plugin

```
/plugin marketplace add rfhayn/clauductor
/plugin install clauductor@clauductor
```

Then, in the repository (a git repository):

```
/clauductor:init            # scaffolds the project's own files; never overwrites one
/clauductor:start-project   # fills project.conf, the gate steps, AGENTS.md's essentials
```

From a shell, the same: `claude plugin marketplace add rfhayn/clauductor` and
`claude plugin install clauductor@clauductor --scope project`. Project scope is the better
choice: an installed plugin's skills load in every repository in its scope. `/clauductor:init`
also writes `enabledPlugins` and `extraKnownMarketplaces` into the repository's
`.claude/settings.json`, so a teammate who clones it is offered the plugin.

## Install with the binary

```bash
./install.sh                 # in a clauductor checkout: builds and installs the binary
cd your-repo && clauductor install
```

See the repository README.

## Running both

Don't. Both ways register the same hooks and the same skills, so every Bash call would be judged
twice and every skill would exist in two copies that drift apart. Each side detects the other:

- `/clauductor:init` refuses a repository with `.claude/clauductor-template` (the
  `clauductor install` marker), even with `--force`.
- `clauductor install` and `clauductor update` refuse a repository with
  `.claude/clauductor-plugin` (the plugin's marker) unless `--force`.
- The plugin's hooks stand aside (exit 0, payload unread) in any repository without
  `.claude/clauductor-plugin`, and in one that also has `.claude/clauductor-template`. An enabled
  plugin's hooks fire in every repository you open, so this is also what keeps them out of
  repositories that don't use the model.
- At session start, the plugin says so in context when a repository has both.

To switch a repository from one to the other: remove the one you're leaving (its marker, and for
`clauductor install` its framework files under `.claude/`), then install the other.

The same guard as `clauductor install` also applies: `/clauductor:init` refuses a repository
that runs an operating model of its own (AGENTS.md, `.claude/skills`, `.claude/hooks` or
`.claude/settings.json`, and no marker) unless the owner says `--force`, and lists what it would
add. It never overwrites a file, even with `--force`.

## What the plugin contains

Built from `template/` by `clauductor plugin build` (`scripts/build-plugin.sh`), committed at
`plugin/`, listed by `.claude-plugin/marketplace.json` at the repository root.

| Plugin path | From the template | Adapted how |
|---|---|---|
| `skills/<name>/` | `.claude/skills/`, except `architecture-audit` and `release-prep` | paths rewritten (below); agent names namespaced; start-project gets a note pointing to init |
| `skills/init/` | new | scaffolds the project files (`scaffold.sh`) |
| `agents/*.md` | `.claude/agents/` | paths in the body rewritten |
| `hooks/*.sh`, `hooks/lib/` | `.claude/hooks/` | paths rewritten |
| `hooks/hooks.json` | the `hooks` of `.claude/settings.json` | each handler runs through `hooks/if-project.sh`; `worktree-hook-drift` is not registered (below); a `SessionStart` hook added |
| `workflows/build-change.js` | `.claude/workflows/` | `agentType` names namespaced (`clauductor:builder`) |
| `checks/`, `lib/`, `modules/`, `*.sh` | `.claude/checks/`, `lib/`, `modules/`, the model's scripts | paths rewritten |
| `bin/clauductor-model` | new | on the Bash tool's PATH: `clauductor-model checks/run.sh`, `clauductor-model status-write.sh "<focus>"` |
| `scaffold/` | everything the project owns (next section) | not a plugin component; `/clauductor:init` copies it |

The path rewrites, all mechanical and all in `framework/internal/plugin`:

- **Skill and agent bodies**: a context line `` !`sh .claude/x` `` becomes
  `` !`sh ${CLAUDE_PLUGIN_ROOT}/x` `` (Claude Code substitutes it when it loads the skill); a
  command Claude runs itself, `sh .claude/x`, becomes `clauductor-model x`, one stable name the
  project's allow rules can match (the cache path changes with every version); other mentions
  become `${CLAUDE_PLUGIN_ROOT}/x`. Frontmatter is not substituted by Claude Code, so a path there
  loses its `.claude/` prefix instead.
- **Scripts**: each executed script sets `CLAUDUCTOR_FW` (the plugin root) from its own location,
  and finds the model's files through it; the project root comes from git
  (`git rev-parse --show-toplevel`), because the script's own location is now the plugin cache,
  not the repository. Sourced files (`lib/conf.sh`, `checks/lib.sh`, `hooks/lib/*`) use their
  caller's. Three scripts need a specific edit, listed in `shellPatches` with the reason; the
  build fails when a patch's anchor leaves the template.
- **Project paths are never rewritten**: `.claude/project.conf`, `model-roles.json`,
  `settings.json`, `.claude/worktrees/`, `.claude/health/`, and the project's own skills.

A template file the packager cannot place (a new top-level file or directory under `.claude/`)
fails the build rather than being left out.

## What stays in the repository

`/clauductor:init` copies these from `plugin/scaffold/`, creating what is missing and keeping
what exists:

- `AGENTS.md`, `CLAUDE.md` (merged: `@AGENTS.md` appended when absent), `README.md`,
  `.gitignore` (merged: missing lines appended)
- `.claude/project.conf`, `.claude/model-roles.json` (with the plugin's `init` skill mapped to
  start-project's role), `.clauductor/panel.json` (its commands run the model's scripts through
  `scripts/ci/clauductor-model.sh`)
- `.claude/settings.json` (merged with jq when it exists): the template's model, effort, env and
  skill overrides; the status line through the plugin's shim; allow rules for
  `clauductor-model ...` and the gate; the plugin enabled for the repository. No hooks: the plugin
  registers them.
- `.claude/skills/architecture-audit/`, `.claude/skills/release-prep/`: CONFIGURE FIRST skills
  the project edits, so they are the project's, as with `clauductor install`
- `.claude/health/`: the project's health lines, as with `clauductor install`
- `docs/`, `changes/`, `specs/`, with framework paths in prose named as the plugin's
- `scripts/ci/`: the gate (`steps.sh`, `run-local.sh`, `gate.sh`, `lease.sh`) and
  `clauductor-model.sh`, and `.claude/lib/conf.sh`: the gate runs in CI where no plugin is
  installed, and sources this one library
- `.claude/clauductor-plugin`: the marker

## The gate and CI

`scripts/ci/steps.sh`'s process-checks step runs the plugin's checks through
`scripts/ci/clauductor-model.sh`, which finds the plugin at `$CLAUDUCTOR_PLUGIN_ROOT`, else
`clauductor-model` on the PATH (inside a session), else the root the plugin recorded at its last
session start (`${CLAUDE_PLUGIN_DATA}/root`). Finding none fails the step: a check that did not
run must not read as one that passed. In CI, clone this repository at the release you use and set
`CLAUDUCTOR_PLUGIN_ROOT=<clone>/plugin`.

## What does not carry over, and why

- **`worktree-hook-drift`** is not registered. It blocks a worktree agent when the main
  checkout's `.claude/hooks/` lag `origin/main`, because hooks are read from the main checkout.
  A plugin's hooks are read from the plugin cache, one version for every worktree, so that drift
  cannot happen. The script still ships (the `hooks` check exercises it). AGENTS.md still lists it.
- **The status line** cannot come from a plugin (a plugin's `settings.json` applies only `agent`
  and `subagentStatusLine`) and cannot name `${CLAUDE_PLUGIN_ROOT}`. The plugin's SessionStart
  hook writes a shim to its data directory, which is stable across versions, and the project's
  `statusLine` runs it. It shows nothing until the first session with the plugin enabled.
- **Editing the framework.** The plugin's skills, agents, hooks and workflow are read-only. A
  project that wants to change one copies it into its own `.claude/` (a project skill of the same
  name is `/name`, the plugin's stays `/clauductor:name`), or uses `clauductor install`.
- **Model choice in one place, partly.** `model-roles.json` is the project's, but the skill and
  agent frontmatter and the workflow's ROLES table it is checked against are the plugin's. A
  project that changes a role there fails `checks/model-roles.sh` and cannot fix the plugin's
  side; that needs `clauductor install`, or a change to the template.
- **The commit trailer default** in `build-change.js` (`ATTRIBUTION_DEFAULT`) is the plugin's;
  start-project's step for it applies to `clauductor install` only. Pass `attribution` to the
  workflow instead.

## Versions

The plugin's version is the framework's `Version` (`framework/internal/cmd/root.go`), written
into `plugin/.claude-plugin/plugin.json`; the marketplace entry carries none. Claude Code keeps an
installed user on a version until the string changes, so a template change reaches plugin users
only with a `Version` bump.

## Maintaining it

```bash
scripts/build-plugin.sh          # or: cd framework && go run ./cmd/clauductor plugin build
cd framework && go run ./cmd/clauductor plugin check   # fails if plugin/ is stale
claude plugin validate --strict plugin && claude plugin validate --strict .
```

Tests (`framework/internal/plugin`, `framework/internal/cmd`):

- `TestCommittedPluginIsCurrent`: the committed `plugin/` and marketplace are exactly what
  `template/` builds to. After any template change, rebuild.
- Layout, manifest, hooks, rewritten paths, the scaffold's settings and panel config, the scaffold
  guards, and the hooks standing aside outside a plugin project.
- `TestPluginChecksPassInScaffoldedProject` (skipped with `-short`): scaffolds a repository and
  runs every process check from the plugin against it. macOS only for now: on Linux the
  template's `focus-staleness.sh` tries BSD `stat -f %m` first, which GNU stat misreads, and the
  template's own `hooks` check fails with or without the plugin.
- `TestClaudePluginValidate`: `claude plugin validate --strict` on the plugin and the
  marketplace; skipped where the `claude` CLI is absent (CI runners).
- `TestClaudePluginInstall` (opt-in, `CLAUDUCTOR_PLUGIN_INSTALL_TEST=1`): adds the marketplace
  and installs the plugin with the `claude` CLI into a temporary `HOME` and `CLAUDE_CONFIG_DIR`.

## References

- Plugin manifest: <https://code.claude.com/docs/en/plugins-reference>
- Components (skills, agents, hooks, `bin/`, default settings):
  <https://code.claude.com/docs/en/plugins/components>
- Marketplaces: <https://code.claude.com/docs/en/plugin-marketplaces>
- Loading, cache and versions: <https://code.claude.com/docs/en/plugins/loading>
