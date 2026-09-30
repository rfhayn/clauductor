# Optional module: OpenSpec

The core proposal format (`changes/<id>/{proposal,design,tasks}.md` and `specs/`) is plain
Markdown, written and archived by the `propose`, `apply-change` and `archive-change` skills. Its
spec format is OpenSpec's, so you can switch to the [OpenSpec](https://github.com/Fission-AI/OpenSpec)
CLI for scaffolding, strict validation and archiving without rewriting anything.

**Off by default.** Nothing here runs until you switch it on.

## Switching it on

1. Install the CLI (`npm install -g @fission-ai/openspec`) and run `openspec init` in the repo.
2. Move the records under OpenSpec's layout, or point the config at it, in
   `.claude/project.conf`:
   ```sh
   PROPOSALS="openspec"
   CHANGES_DIR="openspec/changes"
   SPECS_DIR="openspec/specs"
   ```
   (`git mv changes openspec/changes && git mv specs openspec/specs` if you already have some.)
3. Copy `config.yaml` from this directory to `openspec/config.yaml` and adjust its rules. It
   carries the operating model's rules into every artifact OpenSpec generates.
4. Allow the read-only CLI calls in `.claude/settings.json` `permissions.allow`:
   `Bash(openspec instructions *)`, `Bash(openspec list *)`, `Bash(openspec status *)`,
   `Bash(openspec validate *)`.
5. Point the panel's build template at the new directory: in `.clauductor/panel.json`, the
   build template's `suggest.refresh` becomes `watch:openspec/changes`.

## What changes when it is on

| Step | Core (Markdown) | With OpenSpec |
|---|---|---|
| Scaffold (`propose` step 2) | write the three files by hand | `openspec new change <id>`, then `openspec instructions <artifact> --change <id> --json` per artifact, in dependency order |
| Validate | `sh .claude/checks/run.sh changes` | `openspec validate <id> --strict` (the `changes` check defers to it) |
| Archive (`archive-change`) | apply deltas by hand, `git mv` to `changes/archive/` | `openspec archive <id>` |

Everything else stays: propose just in time and at most one ahead (check branches too, since a
proposal on an unmerged branch is invisible to `openspec list`), read the specs first, the owner
approves before anything is built, the slice line, per-group build and review, and `merge-pr`.

## Two silent-failure traps in `openspec/config.yaml`

- **`rules` values must be LISTS.** A plain string is accepted and ignored.
- **Rule keys must be valid artifact ids** (`proposal`, `design`, `specs`, `tasks`). A misspelt
  key is accepted and ignored. Read `openspec instructions <artifact> --change <id> --json` once
  after editing and confirm your rule appears.
