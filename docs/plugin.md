# Stridal as a Claude Code plugin

The operating model in `template/` ships two ways:

| | `stridal install` | The Claude Code plugin |
|---|---|---|
| Needs | the `stridal` binary (Go) | Claude Code only |
| Framework files (skills, agents, hooks, checks, workflow) | copied into the repo's `.claude/`, editable, updated by `stridal update` | live in Claude Code's plugin cache, read-only, updated by `/plugin update` |
| Project files (AGENTS.md, project.conf, docs/, scripts/ci/, ...) | copied into the repo | copied into the repo by `/stridal:init` |
| Skill names | `/session-start` | `/stridal:session-start` |
| The panel (`stridal panel`) | works | works (it needs the binary either way) |

Pick one per repository. Both install the same model from the same `template/`.

## Install with the plugin

```
/plugin marketplace add rfhayn/stridal
/plugin install stridal@stridal
```

Then, in the repository (a git repository):

```
/stridal:init            # scaffolds the project's own files; never overwrites one
/stridal:start-project   # fills project.conf, the gate steps, AGENTS.md's essentials
```

From a shell, the same: `claude plugin marketplace add rfhayn/stridal` and
`claude plugin install stridal@stridal --scope project`. Project scope is the better
choice: an installed plugin's skills load in every repository in its scope. `/stridal:init`
also writes `enabledPlugins` and `extraKnownMarketplaces` into the repository's
`.claude/settings.json`, so a teammate who clones it is offered the plugin.

## At user scope

Project scope stays the recommendation for a repository that runs the model. A plugin installed
at user scope (`claude plugin install stridal@stridal --scope user`) is enabled in every
repository you open, the model's or not. In a repository that does not run the model (no
`.claude/stridal-plugin`), that means:

- **What loads:** the `/stridal:*` skills and the plugin's agents, namespaced so they never
  collide with the repository's own; `stridal-model` on the Bash tool's `PATH`; and the
  SessionStart hook (`hooks/plugin-session.sh`). That hook records where the loaded plugin version
  lives, in the plugin's own data directory outside the repository, and says nothing there.
- **What does not:** the guard hooks (`pr-merge-guard`, `no-blind-source-rewrite`,
  `no-background-gate`, `no-silent-wait`, `format`, `focus-staleness`). Each runs through `hooks/if-project.sh`,
  which stands aside (exit 0, payload unread) in a repository without `.claude/stridal-plugin`.
  So they never judge a call there beside the repository's own guards.

**The per-repository opt-out.** A repository with an operating model of its own (one the panel
serves through `stridal panel attach`, say: [docs/panel.md](panel.md#attach-a-repository-without-writing-to-it))
can turn the plugin off, so its skills are not offered there at all. Put this in the repository's
uncommitted `.claude/settings.local.json`:

```json
{ "enabledPlugins": { "stridal@stridal": false } }
```

The file is yours, never committed: where the repository's `.gitignore` does not cover it, list it
in `.git/info/exclude`. Nothing checks that a repository has turned the plugin off.

## Install with the binary

```bash
./install.sh                 # in a stridal checkout: builds and installs the binary
cd your-repo && stridal install
```

See the repository README.

## Running both

Don't. Both ways register the same hooks and the same skills, so every Bash call would be judged
twice and every skill would exist in two copies that drift apart. Each side detects the other:

- `/stridal:init` refuses a repository with `.claude/stridal-template` (the
  `stridal install` marker), even with `--force`.
- `stridal install` and `stridal update` refuse a repository with
  `.claude/stridal-plugin` (the plugin's marker) unless `--force`.
- The plugin's hooks stand aside (exit 0, payload unread) in any repository without
  `.claude/stridal-plugin`, and in one that also has `.claude/stridal-template`. An enabled
  plugin's hooks fire in every repository you open, so this is also what keeps them out of
  repositories that don't use the model.
- At session start, the plugin says so in context when a repository has both.

To switch a repository from one to the other: remove the one you're leaving (its marker, and for
`stridal install` its framework files under `.claude/`), then install the other.

The same guard as `stridal install` also applies: `/stridal:init` refuses a repository
that runs an operating model of its own (AGENTS.md, `.claude/skills`, `.claude/hooks` or
`.claude/settings.json`, and no marker) unless the owner says `--force`, and lists what it would
add. It never overwrites a file, even with `--force`.

A repository that runs stridal's **old** model (three of its lock-based skills, a hook that
calls the binary, settings registering one, or `orchestration/config.json`) is not refused: the
plugin replaces that model. `/stridal:init` lists the old model's skills and hooks under OLD
MODEL, and `scaffold.sh --prune` removes them (only them; never a project file). Either way,
`settings.json` loses the hook registrations that run a `.claude/hooks/` script that does not
exist, and the old model's hooks, and names each.

## What the plugin contains

Built from `template/` by `stridal plugin build` (`scripts/build-plugin.sh`), committed at
`plugin/`, listed by `.claude-plugin/marketplace.json` at the repository root.

| Plugin path | From the template | Adapted how |
|---|---|---|
| `skills/<name>/` | `.claude/skills/`, except `architecture-audit` and `release-prep` | paths rewritten (below); agent names namespaced; start-project gets a note pointing to init |
| `skills/init/` | new | scaffolds the project files (`scaffold.sh`) |
| `agents/*.md` | `.claude/agents/` | paths in the body rewritten |
| `hooks/*.sh`, `hooks/lib/` | `.claude/hooks/` | paths rewritten |
| `hooks/hooks.json` | the `hooks` of `.claude/settings.json` | each handler runs through `hooks/if-project.sh`; `worktree-hook-drift` is not registered (below); a `SessionStart` hook added |
| `workflows/build-change.js` | `.claude/workflows/` | `agentType` names namespaced (`stridal:builder`) |
| `checks/`, `lib/`, `modules/`, `examples/`, `*.sh` | `.claude/checks/`, `lib/`, `modules/`, `examples/` (the example change the checks hold correct), the model's scripts | paths rewritten |
| `bin/stridal-model` | new | on the Bash tool's PATH: `stridal-model checks/run.sh`, `stridal-model status-write.sh "<focus>"` |
| `scaffold/` | everything the project owns (next section) | not a plugin component; `/stridal:init` copies it |

The path rewrites, all mechanical and all in `framework/internal/plugin`:

- **Skill and agent bodies**: a context line `` !`sh .claude/x` `` becomes
  `` !`sh ${CLAUDE_PLUGIN_ROOT}/x` `` (Claude Code substitutes it when it loads the skill); a
  command Claude runs itself, `sh .claude/x`, becomes `stridal-model x`, one stable name the
  project's allow rules can match (the cache path changes with every version); other mentions
  become `${CLAUDE_PLUGIN_ROOT}/x`. Frontmatter is not substituted by Claude Code, so a path there
  loses its `.claude/` prefix instead.
- **Scripts**: each executed script sets `STRIDAL_FW` (the plugin root) from its own location,
  and finds the model's files through it; the project root comes from git
  (`git rev-parse --show-toplevel`), because the script's own location is now the plugin cache,
  not the repository. Sourced files (`lib/conf.sh`, `checks/lib.sh`, `hooks/lib/*`) use their
  caller's. The variable is set, not exported, so a `sh -c '…'` child whose body names it gets
  `STRIDAL_FW="$STRIDAL_FW"` among its assignments, unless they already set it (a check
  pointing it at a fixture). A child after any other word, a wrapper (`with_timeout 60 sh -c`) or
  a reserved word (`then sh -c`), is not rewritten, and `TestShellChildGetsPluginRoot` fails on
  it. A copy a check puts in a scratch repository's `.claude/` takes that repository as its
  root, as in the template; the plugin's own copy takes the `ROOT` its caller names, else the
  repository the command runs in. Inside a check, a bare `sh .claude/x` runs the scratch copy and
  is left alone. A few scripts need a specific edit, listed in `shellPatches` with the reason; the
  build fails when a patch's anchor leaves the template.
- **Skill names**: every plugin skill named as a command (`/session-start`) in skills, agents,
  the scaffolded docs, AGENTS.md and the panel's lane prompts becomes `/stridal:session-start`;
  the project's own skills (`/architecture-audit`, `/release-prep`) keep their names. The
  scaffolded AGENTS.md and playbook lose the table rows for hooks the plugin does not register.
- **Project paths are never rewritten**: `.claude/project.conf`, `model-roles.json`,
  `settings.json`, `.claude/worktrees/`, `.claude/health/`, `.claude/evals/`, and the project's
  own skills.

A template file the packager cannot place (a new top-level file or directory under `.claude/`)
fails the build rather than being left out.

## What stays in the repository

`/stridal:init` copies these from `plugin/scaffold/`, creating what is missing and keeping
what exists:

- `AGENTS.md`, `CLAUDE.md` (merged: `@AGENTS.md` appended when absent), `README.md`,
  `.gitignore` (merged: missing lines appended)
- `.claude/project.conf`, `.claude/model-roles.json` (with the plugin's `init` skill mapped to
  start-project's role), `.stridal/panel.json` (its commands run the model's scripts through
  `scripts/ci/stridal-model.sh`; when created, its lanes and branch patterns follow the
  project's `BRANCH_CHANGE`, `BRANCH_FIX`, `BRANCH_OPS` and `MAIN_BRANCH`, with jq)
- `.claude/settings.json` (merged with jq when it exists, as `stridal install` merges it: the
  project's keys and values kept, the allow and deny lists and the sandbox's excluded commands and
  domains unioned): the template's model, effort, env and skill overrides; the status line
  through the plugin's shim; allow rules for `stridal-model ...` and the gate, at the paths
  `GATE_RUN` and `GATE` give; the plugin enabled for the repository. No hooks: the plugin
  registers them.
- `.claude/skills/architecture-audit/`, `.claude/skills/release-prep/`: CONFIGURE FIRST skills
  the project edits, so they are the project's, as with `stridal install`
- `.claude/health/`: the project's health lines, as with `stridal install`
- `.claude/local/`: the project's own extension layer (guard rules, context sections, health
  lines, checks, skill fragments, conflict rows; its README has the contracts). The plugin's hooks,
  context scripts and checks find it in the repository, exactly as an installed copy does.
  Modules: the plugin carries the shipped ones (`modules/`), turned on by `MODULES` in
  `.claude/project.conf`; a module of the project's own goes in the repository's
  `.claude/modules/<name>/`, which the loader searches after the plugin's.
- `.claude/evals/`: the seeded-defect suites, their runner and the receipts a project's model
  choices rest on (OPS-10). The project adds cases for its own domain and commits its receipts.
  The runner cannot source the plugin's `lib/evals.sh`, so it carries its own copy of the hash
  functions, and `checks/evals.sh` fails if the two copies drift. A plugin project points the
  runner at the plugin's agent with `EVAL_AGENT_FILE`.
- `docs/`, `changes/`, `specs/`, `.github/`, with framework paths in prose named as the plugin's
- `scripts/ci/`: the gate (`steps.sh`, `run-local.sh`, `gate.sh`, `lease.sh`, the model's steps
  in `lib/steps.sh`, which reach the plugin's scripts through the resolver, the run state and
  run log in `lib/gate-state.sh` with `gate-wait.sh`, which reads them, and the lease
  conformance kit in `lease-conformance/`) and
  `stridal-model.sh`, and `.claude/lib/conf.sh`: the gate runs in CI where no plugin is
  installed, and sources this one library
- `.claude/stridal-plugin`: the marker

## The gate and CI

`scripts/ci/steps.sh`'s process-checks step runs the plugin's checks through
`scripts/ci/stridal-model.sh`, which finds the plugin at `$STRIDAL_PLUGIN_ROOT`, else
`stridal-model` on the PATH (inside a session), else the root the plugin recorded at its last
session start (`${CLAUDE_PLUGIN_DATA}/root`), else, in CI (`$CI` set) or with
`STRIDAL_FETCH=1`, a shallow clone of this repository at the plugin's pinned version:

| Variable | Default | |
|---|---|---|
| `STRIDAL_REF` | `v<Version>`, the version `/stridal:init` scaffolded from | the tag or branch cloned; bump it when the project updates the plugin |
| `STRIDAL_REPO_URL` | `https://github.com/rfhayn/stridal.git` | |
| `STRIDAL_CACHE` | `~/.cache/stridal` | one directory per ref; cache it between CI runs to skip the clone |

Finding none fails the step: a check that did not run must not read as one that passed. The
default ref is a release tag, so a project scaffolded from a version that has no tag yet sets
`STRIDAL_REF` (a branch or another tag) until one exists.

## What does not carry over, and why

- **`worktree-hook-drift`** is not registered. It blocks a worktree agent when the main
  checkout's `.claude/hooks/` lag `origin/main`, because hooks are read from the main checkout.
  A plugin's hooks are read from the plugin cache, one version for every worktree, so that drift
  cannot happen. The script still ships (the `hooks` check exercises it); the scaffolded AGENTS.md
  and playbook drop its rows.
- **The status line** cannot come from a plugin (a plugin's `settings.json` applies only `agent`
  and `subagentStatusLine`) and cannot name `${CLAUDE_PLUGIN_ROOT}`. The plugin's SessionStart
  hook writes a shim to its data directory, which is stable across versions, and the project's
  `statusLine` runs it. It shows nothing until the first session with the plugin enabled.
- **Editing the framework.** The plugin's skills, agents, hooks and workflow are read-only. A
  project that wants to change one copies it into its own `.claude/` (a project skill of the same
  name is `/name`, the plugin's stays `/stridal:name`), or uses `stridal install`.
- **Model choice in one place, partly.** `model-roles.json` is the project's, but the skill and
  agent frontmatter and the workflow's ROLES table it is checked against are the plugin's. A
  project that changes a role there fails `checks/model-roles.sh` and cannot fix the plugin's
  side; that needs `stridal install`, or a change to the template.
- **The commit trailers and branch prefixes are the project's.** `build-change.js` reads
  attribution and provenance from the project's `.claude/model-roles.json`, and the change branch,
  changes directory and gate from its `.claude/project.conf`, at run time (`project-config.sh
  --json`, run through `stridal-model`), so neither install path needs an edit to the workflow.

- **Workflows from a plugin are not yet verified in a session.** `workflows/build-change.js` is in
  the plugin's default `workflows/` directory, which the docs say a plugin can carry, but
  `claude plugin details` does not list workflows, so whether it loads (and under which name:
  `/build-change` or namespaced) is to be checked in a real session. The panel's build lane and
  the docs still say `/build-change`. Until then, `/stridal:apply-change` is the same loop by
  hand.

## Versions

Decided: the plugin's version tracks stridal's semver `Version`
(`framework/internal/cmd/root.go`), bumped at release (REL-1), written into
`plugin/.claude-plugin/plugin.json`; the marketplace entry carries none. Claude Code keeps an
installed user on a version until the string changes, so a template change reaches plugin users
with the next release, and a scaffolded project's CI clones the same version's tag.

## Maintaining it

```bash
scripts/build-plugin.sh          # or: cd framework && go run ./cmd/stridal plugin build
cd framework && go run ./cmd/stridal plugin check   # fails if plugin/ is stale
claude plugin validate --strict plugin && claude plugin validate --strict .
```

Tests (`framework/internal/plugin`, `framework/internal/cmd`):

- `TestCommittedPluginIsCurrent`: the committed `plugin/` and marketplace are exactly what
  `template/` builds to. After any template change, rebuild.
- Layout, manifest, hooks, rewritten paths, the scaffold's settings and panel config, the scaffold
  guards, and the hooks standing aside outside a plugin project.
- `TestPluginChecksPassInScaffoldedProject` (skipped with `-short`): scaffolds a repository and
  runs every process check from the plugin against it.
- `TestResolverFetchesPinnedPluginInCI`: in CI the gate's resolver clones the pinned tag (from a
  local repository in the test), serves the second run from its cache, and fails on a ref it
  cannot clone.
- `TestClaudePluginValidate`: `claude plugin validate --strict` on the plugin and the
  marketplace; skipped where the `claude` CLI is absent (CI runners).
- `TestClaudePluginInstall` (opt-in, `STRIDAL_PLUGIN_INSTALL_TEST=1`): adds the marketplace
  and installs the plugin with the `claude` CLI into a temporary `HOME` and `CLAUDE_CONFIG_DIR`.

## References

- Plugin manifest: <https://code.claude.com/docs/en/plugins-reference>
- Components (skills, agents, hooks, `bin/`, default settings):
  <https://code.claude.com/docs/en/plugins/components>
- Marketplaces: <https://code.claude.com/docs/en/plugin-marketplaces>
- Loading, cache and versions: <https://code.claude.com/docs/en/plugins/loading>
