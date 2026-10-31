# ADR 0007: The model also ships as a Claude Code plugin, generated from template/

- **Status**: Accepted
- **Date**: 2026-09-30
- **Tags**: plugin, template, distribution
- **Source**: OPS-11 (#24); docs/plugin.md

## Context
`stridal install` needs the Go binary. A contributor who only has Claude Code (Standing Tee's
designer) could not get the model's skills and hooks without it. Claude Code plugins distribute
skills, agents and hooks through a marketplace, read-only and versioned.

## Decision
- `plugin/` is built from `template/` by `stridal plugin build` (`scripts/build-plugin.sh`) and
  committed; `.claude-plugin/marketplace.json` lists it. `template/` stays the one source.
- Framework files live in the plugin cache, paths rewritten to `${CLAUDE_PLUGIN_ROOT}`; project
  files are scaffolded once by `/stridal:init`, which never overwrites.
- One way per repository: the binary install and the plugin each detect the other's marker and
  refuse (ADR-0004); the plugin's hooks stand aside in a repository it did not set up.

## Consequences
### Positive
- The model reaches Claude Code users without the binary; updates arrive through `/plugin update`.

### Negative / trade-offs
- Every template edit must be followed by a rebuild, or the plugin ships stale.
- Skill names differ (`/stridal:session-start` vs `/session-start`), and the rewriter must place
  every new template file (a file it cannot place fails the build).

## Enforcement
`TestCommittedPluginIsCurrent` (CI) fails when `plugin/` differs from a fresh build; this repo's
gate runs `stridal plugin check` from source (`scripts/ci/steps.sh`); the plugin tests run the
checks in a scaffolded repository on macOS and Linux.

## Amendment — 2026-10-31: a plugin release also carries the engine, and the repo pins it
- **Source**: ADR-0018 (accepted by the owner, 2026-10-31); OPS-182 (S19).

The plugin stays the generated copy of `template/`. What a release holds, and what a repository
runs, change:
- **A release carries the engine.** Under ADR-0017 the controls move into the `stridal` binary, so
  a plugin release carries `hooks.json`, the skills and agents, and the engine for that same
  version: the binaries under `engine/` (never a top-level `bin/`), or their manifest with sha256s
  (where they live is OPS-184's choice). The plugin's hooks and its MCP server call one bootstrap
  launcher, which picks the binary for the running platform.
- **The repo pins the release.** `.claude/stridal.lock`, read from `origin/<main>`, names the
  release and its sha256s, so every worktree is guarded by main's release. A mismatch refuses
  every guarded decision, and allows the commands that repair it.
- **The first consequence above no longer holds.** "The model reaches Claude Code users without the
  binary" was ADR-0006's promise, which ADR-0016 withdraws: a guarded repository needs the engine,
  and the plugin brings it.

**Enforcement** (in addition to the above): built by OPS-183 (the lock and the fail-closed
dispatcher) and OPS-184 (the release that carries the engine, and a check that rebuilds a tagged
release to reproduce its sha256s). Until they merge, **Nothing. You.**: the plugin ships shell
hooks, as before.

## Related
- ADR-0004, ADR-0006 (superseded by ADR-0016), ADR-0017, ADR-0018
