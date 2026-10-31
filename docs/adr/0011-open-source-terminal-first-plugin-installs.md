# ADR 0011: Stridal is open source, terminal-first, and installed through the plugin

- **Status**: Accepted
- **Date**: 2026-10-27
- **Tags**: release, distribution, panel, plugin, licensing
- **Source**: the owner's decisions of 2026-10-27, made in a cloud session (OPS-144);
  `docs/plans/direction-shift.md`

## Context
Until 2026-10-27, Stridal was headed for a paid launch:
- a licence bought through Polar and checked offline by the binary (OPS-112, OPS-116, PANEL-54);
- a rebuilt browser panel as its face (PANEL-46, 3–5 sessions);
- two ways to install the operating model, the binary's `stridal install` and the plugin, which refuse each other in one
  repository (ADR-0004, ADR-0007).

Three things moved the owner off that path.

**The audience.** The tool is for a solo builder or a small team, and the owner wants others to adopt it. A licence
gate and a checkout are friction for exactly those users.

**The face.** Comparable tools put the session list where the work happens: Ghostty Dock is a sidebar of Claude Code
sessions inside the terminal, showing each session's state, context and cost and raising the one that needs you. A
browser tab is one more place to look. Standing Tee also moved its own orchestration back to plain terminal sessions
(its ADR-0039), opened as tabs by an orchestrator session.

**The installs.** Two channels for one operating model double what is built and tested, and ADR-0004 exists only to
keep them apart.

## Decision
1. **Open source under MIT.** Nothing is sold. The licensing rows are cancelled (OPS-112, OPS-116, PANEL-54), and the
   site keeps only its docs (OPS-117).
2. **The terminal is the local face.** The panel's Go engine stays. Its face becomes a full-screen terminal UI in the
   same binary, built with compiled-in Go libraries (Bubble Tea and Lip Gloss), so users install nothing extra
   (Phase 4, PRD-tui). The browser page is frozen (fixes only, no new features) until the terminal UI covers what
   the owner uses, and is then removed (PANEL-64). A desktop app may follow after v0.1.0 (PANEL-65).
3. **The plugin is the one way to install the operating model.** `/stridal:init` writes the lite profile by
   default, with the product module on, and `--full` writes the full profile. The binary stops installing the model
   and becomes the terminal UI and its engine only (OPS-92b, OPS-145). This repository moves to the plugin with
   `--full`. When OPS-145 merges, ADR-0004 is superseded; until then it holds.
4. **The product module is on by default, in lite and in full.** Its screens are claude.ai artifacts, shareable and
   usable from a phone, as Standing Tee runs them, plus a read-only summary in the terminal UI (Phase 5, PRD-product).
5. **One version number** covers the plugin, the template and the binary.
6. **v0.1.0 ships only under the new name.** The fallback of shipping under the old name and renaming at v0.2 is dropped:
   after a public release, a rename breaks the Go module path, the plugin id, links and people's scripts.
7. **What Standing Tee's operating model does that the template does not is ported into the template** (Phase 2),
   reversing OPS-71's cancellation of the Standing Tee ports. Practices still flow one way.

## Consequences
### Positive
- One install story for a stranger: add the plugin, and add the binary for the terminal UI.
- No licence code, signer, checkout or EULA to build, secure or support: about 3–4 sessions freed (S24–S26 as
  planned before).
- A public repository's GitHub Actions minutes are free, macOS included, which bears on OPS-98, OPS-28 and OPS-115.
- The terminal UI works on macOS, Linux, WSL and over SSH, from one binary.

### Negative / trade-offs
- The browser panel's investment is written off: PANEL-46's approved design (#149) and the rows built on it are
  cancelled. ADR-0009's colour rules carry over to the terminal UI.
- A terminal UI draws a character grid: no real charts, nothing shared with a teammate. That is why the product
  module's screens stay in claude.ai artifacts.
- Plugin skill names carry a prefix (the old name's `:session-start`, and `/stridal:session-start` after the rename).
- The launch depends on the new name clearing, with no fallback.
- The project that adopts it is no longer paying for it: support and contributions become the owner's cost.

## Enforcement
- The roadmap carries every decision as rows: Phases 4–7, the cancelled rows, and the session plan from S13, read
  through `.claude/roadmap-queue.sh` and `.claude/roadmap-plan.sh`, which `checks/roadmap.sh` holds.
- Plugin-only install: enforced once OPS-145 lands, by what it removes; until then, **Nothing. You.**
- The browser freeze (no new browser features before PANEL-64): **Nothing. You.** No check refuses a change to
  `framework/internal/panel/web`.
- MIT: the `LICENSE` file REL-1 adds. Nothing checks that no row reintroduces a paid gate.

## Amendment — 2026-10-31: the binary is the engine of every control, shipped in the plugin release
- **Source**: ADR-0016, ADR-0017 and ADR-0018 (accepted by the owner, 2026-10-31); OPS-182 (S19).

Decisions 1, 2, 4, 6 and 7 stand. Decisions 3 and 5 change, and so does a line of *Related*:
- **Decision 3.** The binary still stops installing the model, but it no longer becomes "the terminal
  UI and its engine only". It is the engine of every control (ADR-0017): the hooks, the gate, the
  lease and the merge path run through it, and the plugin's MCP server is the same binary. A
  plugin release carries it (ADR-0018), so a stranger adds the plugin and gets the engine with it;
  the terminal UI is that binary's `stridal ui`. This repository moves to the plugin earlier than
  planned, in Phase 2 (OPS-145, before any port).
- **Decision 5.** "One version number" becomes one release: `hooks.json`, the skills and agents and
  the engine ship together, and a repository pins that release in `.claude/stridal.lock`, read from
  `origin/<main>` (ADR-0018).
- **ADR-0006 is no longer "unchanged".** ADR-0016 supersedes it: a guarded repository needs the
  engine, so the promise that a repository works without stridal is withdrawn.
- **The phases above are renumbered** by the re-plan of 2026-10-31 (`docs/roadmap.md`): the
  terminal UI is Phase 6, the product module Phase 7, the launch Phase 8.

**Enforcement** (in addition to the above): OPS-183 and OPS-184 (the lock, the fail-closed
dispatcher and the release that carries the engine) and OPS-145 (plugin-only install). Until they
merge, **Nothing. You.**

## Related
- ADR-0003 (one panel for many repositories: still true of the engine and its terminal UI)
- ADR-0004 (superseded when OPS-145 merges)
- ADR-0006 (a repository works without stridal: superseded by ADR-0016, 2026-10-31; see the amendment above)
- ADR-0016, ADR-0017, ADR-0018 (the engine and its one version authority)
- ADR-0007 (the plugin: now the only install channel)
- ADR-0009 (the panel's colour rules: carried into PRD-tui)
