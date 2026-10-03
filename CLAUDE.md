# CLAUDE.md — Clauductor framework repo

This repo builds Clauductor (an operating model for Claude Code, its local panel and its plugin)
and runs that same model on itself. The working conventions every session applies are in
`AGENTS.md`, imported below. This file adds only what is specific to the framework repo.

**Two AGENTS.md files.** The root one governs work in this repo. `template/AGENTS.md` is the
product: what every project receives. Change the template with care; it ships.

## Architecture

```
clauductor/
├── AGENTS.md, .claude/       ← this repo's own operating model (installed from template/)
├── framework/                ← Go source: the CLI and the panel
│   ├── cmd/clauductor/       ← CLI entrypoint
│   └── internal/
│       ├── cmd/              ← commands: install, update, plugin, panel, lock-run, ...
│       ├── panel/            ← the local panel (server, lanes, web UI; docs/panel.md)
│       ├── plugin/           ← builds plugin/ from template/ (docs/plugin.md)
│       ├── template/         ← reads template/ for install and update
│       ├── leakcheck/        ← test support: fails a run that leaves tmux or a helper behind
│       ├── testbin/          ← test support: writes a stand-in executable no parallel fork holds open
│       └── testwait/         ← test support: waits with deadlines scaled by CLAUDUCTOR_TEST_SLOW
├── template/                 ← the operating model projects receive (`clauductor install`)
│   ├── AGENTS.md, CLAUDE.md  ← project-level instructions
│   ├── .claude/              ← skills, agents, hooks, checks, workflows, project.conf
│   ├── scripts/ci/           ← the gate: run-local.sh, gate.sh, lease.sh, steps.sh
│   └── docs/, changes/, specs/  ← record templates
├── plugin/                   ← GENERATED from template/ (scripts/build-plugin.sh): the plugin
├── .claude-plugin/           ← marketplace.json listing plugin/ (docs/plugin.md)
├── docs/                     ← this repo's records (roadmap, journal, insights, ADRs) and PRDs
└── install.sh                ← build + install script
```

## Build, test, gate

```bash
cd framework && go build -o clauductor ./cmd/clauductor
cd framework && go test -short ./...          # seconds, while working
scripts/ci/gate.sh                             # the full gate (steps: scripts/ci/steps.sh)
sh .claude/checks/run.sh                       # this repo's process checks
sh template/.claude/checks/run.sh              # the template's checks
scripts/build-plugin.sh                        # after ANY template/ change (TestCommittedPluginIsCurrent)
```

CI (`.github/workflows/test.yml`) runs gofmt, vet, `go test -race` and the process checks on Ubuntu
for every PR, and on macOS and Ubuntu for every push to `main` (OPS-26, temporary until OPS-28;
the local gate's short suite and scoped race run on the owner's Mac for a PR).

After a template change, bring this repo's own copy along: `framework/clauductor update` from the
repo root, with the binary built from this checkout (it reads the template beside it, and prints
which). Attribution and provenance are off in `.claude/model-roles.json` alone: build-change reads
them at run time, so no framework file differs on purpose.

## Tech stack

- **Go**: the CLI, the panel server, the plugin builder
- **POSIX sh + git + jq**: the operating model's hooks, checks and gate (no binary needed: ADR-0006)
- **tmux**: the panel's lanes (never kill a tmux server from a script: ADR-0005)
- **HTML/JS** (vendored xterm): the panel's web UI

## Git workflow

- Branches: the model's lanes, `change/<id>`, `fix/<n>-<slug>`, `ops/<name>`.
- Commits and PR titles: `PREFIX-N:` in the imperative mood. **No Co-Authored-By.**
- Milestone prefixes: PANEL (the panel), OPS (the operating model and this repo's process), REL
  (releases), ST (Standing Tee convergence). Older history uses M1–M7 and LIFE-n.

## Code standards

```go
// Comments explain WHY, not WHAT
// TODOs must include milestone context: TODO (PANEL-19): description
```

@AGENTS.md
