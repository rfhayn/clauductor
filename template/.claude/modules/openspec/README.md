# Optional module: OpenSpec

The core change format (`changes/<id>/{proposal,design,tasks}.md` and `specs/`) is plain Markdown
in OpenSpec's format, written and archived by the `propose`, `apply-change` and `archive-change`
skills with nothing installed. This module lets the [OpenSpec](https://github.com/Fission-AI/OpenSpec)
CLI (`openspec validate`, `openspec archive`, `openspec list`) work on the same files. **Nothing is
migrated**: `openspec/changes` and `openspec/specs` are symlinks to `CHANGES_DIR` and `SPECS_DIR`.

**Off by default.** Nothing here runs until you switch it on.

## Switching it on

1. Install the CLI, **1.13 or later**: `npm install -g @fission-ai/openspec@latest` (or
   `brew upgrade openspec`). 1.2.0 passes a MODIFIED block that leaves out a current scenario and
   then deletes that scenario from the living spec on archive, and it ignores `skip_specs`; the
   module refuses it by name.
2. `sh .claude/modules/openspec/enable.sh`: checks the CLI's version, creates
   `openspec/changes -> ../changes` and `openspec/specs -> ../specs`, and copies `config.yaml` from
   this directory to `openspec/config.yaml` (adjust its rules). Commit the links.
3. Set `PROPOSALS="openspec"` in `.claude/project.conf`. From then on `.claude/checks/openspec.sh`
   (a gate step, through `checks/run.sh`) runs `openspec validate --all --strict` on the project,
   and fails if the links point anywhere else.
4. Allow the read-only CLI calls in `.claude/settings.json` `permissions.allow` if you want them
   without a prompt: `Bash(openspec list *)`, `Bash(openspec status *)`, `Bash(openspec validate *)`.

An adopting project whose records already live under `openspec/` sets
`CHANGES_DIR="openspec/changes"` and `SPECS_DIR="openspec/specs"` instead; `enable.sh` then makes no
links.

## What changes when it is on

| Step | Core (Markdown) | With OpenSpec |
|---|---|---|
| Validate | `sh .claude/checks/run.sh changes` | the same, **plus** `openspec validate --all --strict` (`checks/openspec.sh`) |
| Archive (`archive-change`) | apply the deltas, `git mv` to `changes/archive/` | `openspec archive <id> -y` may do both steps, after the same preconditions |

**clauductor's own checks still run, and still decide.** The CLI leaves several things unguarded,
so `checks/changes.sh`, `archive-change` and `pr-merge-guard` (rules 9–11) hold them whatever
version is installed (tested on 1.2.0 and 1.13.2):

- `openspec archive -y` archives a change with **no spec delta**, and one with **an unchecked
  task**, printing only a warning. clauductor refuses both: a change with no delta must say
  `skip_specs: true` in `changes/<id>/.openspec.yaml`, which `propose` writes.
- OpenSpec matches a MODIFIED block's scenarios by their **whole header**, not by ID. clauductor
  requires the MODIFIED block to copy every current scenario header word for word first.
- A new capability's delta needs a **`## Purpose` of 50 characters or more**; without one, archive
  writes "TBD", which fails strict validation.
- Scenario IDs (`[CAP-n-Sn]`), their uniqueness and their tests (`scenario-trace.sh`) are
  clauductor's; OpenSpec keeps the IDs in the headers through archive.

The example change in `.claude/examples/` is validated through these links by
`checks/openspec.sh` wherever the CLI (1.13+) is installed, and on every push in clauductor's CI.

## Two silent-failure traps in `openspec/config.yaml`

- **`rules` values must be LISTS.** A plain string is accepted and ignored.
- **Rule keys must be valid artifact ids** (`proposal`, `design`, `specs`, `tasks`). A misspelt
  key is accepted and ignored. Read `openspec instructions <artifact> --change <id> --json` once
  after editing and confirm your rule appears.
