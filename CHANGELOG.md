# Changelog

All notable changes to clauductor are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow
[Semantic Versioning](https://semver.org/spec/v2.0.0.html); until 1.0.0 a minor version may break
things.

`scripts/changelog.sh` regenerates **[Unreleased]** from merged pull requests that no released
section cites yet. When cutting a release, sort that list into the categories below and move it
under the new version's header ([docs/release.md](docs/release.md)).

## [Unreleased]

Nothing merged since the last release.

## [0.1.0] - Unreleased

The first public release: the local panel over Claude Code sessions, and the operating model a
repository installs, as files or as a Claude Code plugin.

### Added

- `clauductor panel`, a loopback live dashboard for Claude Code sessions (#2)
- Embedded lane terminals, lane control and launch at login (#4)
- Orchestration: lane templates, the gate lease, alerts, restore after a reboot, hardening (#5)
- Six themes with light and dark modes (#6)
- Adoption by a second project: config versions, `panel init`, the JSON Schema, explicit trust,
  lease conformance (#12)
- Up next in New lane, pinned cards in the side panel (#14)
- A generic operating model in the template, with an install guard (#19)
- The change process: OpenSpec-compatible records, scenario tracing, and adopted practices (#22)
- The operating model as a Claude Code plugin, through a marketplace in this repository (#24)

### Changed

- UI refinement: safe terminals, in-place updates, Needs you first (#8)
- Idle process spawns cut from 97 to 17 a minute (#9)
- The panel split by subject, with one runtime and one clock (#10)
- A lane-centric panel, restyled as a tool (#13)
- "A repository works without clauductor" made an enforced invariant; designer onboarding
  planned (#23)

### Fixed

- One truth for "blocked", and one panel per machine (#7)
- Deterministic tests in 12 s, CI on macOS and Ubuntu, and the `lock-run` signal fix (#11)
- Smoke-test fixes for Up next and pinned cards (#14)
- Re-verify a new Claude Code from live hooks, and close warnings (#15)

Before the panel, clauductor was a tmux HUD with SQLite-backed workers, file locks and
orchestration skills (milestones M1 to M7 and LIFE-1 to LIFE-2, March to April 2026). That work
is still in the binary (`clauductor start`, `watch`, `lock`, ...) and is not itemised here.

[Unreleased]: https://github.com/rfhayn/clauductor/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/rfhayn/clauductor/releases/tag/v0.1.0
