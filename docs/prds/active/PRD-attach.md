# PRD: Attach: the panel serves a repository with nothing installed in it, and Standing Tee stays independent

**Status:** decided 2026-10-12 by the owner. The rows are OPS-49, PANEL-42 (Phase 2, *Attach*, before MAPS), ST-7 and
ST-6 (Phase 5, renamed "Standing Tee attaches"). PANEL-42 is a capability change: it is proposed just in time and
approved on its own `design.md`. This PRD gives direction; it does not approve a design.

## Why

Until now, Clauductor reached a repository by installing into it:

- `clauductor install` or the plugin's `/clauductor:init` writes `AGENTS.md`, `.claude/` and `scripts/ci/`;
- `panel init` writes `.clauductor/panel.json`.

Phase 5 planned the same for Standing Tee: converge its operating model on the template file by file (ST-1 to ST-4),
then keep it current with `clauductor update`.

The owner asked whether Clauductor could be used without integrating it into a repository. **Most of the machinery
for it already exists outside every repository; the one missing piece is the panel's config.**

| Part | Where it lives today |
|---|---|
| The panel's hooks | The user's `~/.claude/settings.json` (`framework/internal/panel/install/hooks.go`) |
| The project registry | `~/.clauductor/panel/projects.json` |
| Trust | Recorded per project (under a hash of its path), against the config file's path and content hash (`install.TrustedNow`) |
| The registry's `config` field | Can name a config file outside the repository |

**A project with no config does not load today.** An empty `config` falls back to `<root>/.clauductor/panel.json`
(`ProjectEntry.ConfigPath()`), and `config.LoadConfigRaw` fails when that file is missing. Standing Tee shows this now:
it is registered with `"config": ""` and has no `.clauductor/`, and the installed panel's `panel list` reports
`config does not load: no panel config at …/standingtee/.clauductor/panel.json` and 0 lanes. So attach mode is what
makes the panel serve Standing Tee at all, not a cleanup of something that half works. The pieces it builds on already
exist: the `config` field that can name a file outside the repository, `panel init`'s reading of a repository, and
trust recorded against the config's path and content hash, wherever that file lives.

## Decisions (the owner, 2026-10-12)

1. **Standing Tee stays independent, permanently.**
   - It keeps its own model: its `infra/` scripts, skills and merge guard.
   - ST-0 to ST-4 are cancelled, and so is OPS-24, which existed for the swap.
   - P2.15b is cancelled too: only a converged Standing Tee would run the template's parser.
2. **Ideas flow one way.** A practice Standing Tee proves is ported into the template as a row of its own; the MAPS
   rows are the first. Standing Tee never receives the template back, so nothing has to
   keep the two in sync. The cost is that the two models drift, and a fix in one is ported to the other by hand.
3. **Attach mode (PANEL-42) is how the panel serves a repository that does not install Clauductor.**
   - The config is generated into `~/.clauductor/projects/<id>/panel.json` and registered through the existing
     `config` field.
   - Its commands still run in the repository's root, against the repository's own scripts.
   - It is trusted like any config: by its hash, and only on the owner's say.
   - A test proves the repository's files are untouched.
4. **The plugin can be installed at user scope** (PANEL-42 documents this; `docs/plugin.md` recommends project scope
   today), with a per-repository opt-out in the uncommitted
   `.claude/settings.local.json` (`"enabledPlugins": {"clauductor@clauductor": false}`).
   - Its skills are namespaced (`/clauductor:*`), so they never collide with a repository's own.
   - Its hooks would run beside a repository's own guards, which is what the opt-out prevents.
   - PANEL-42 documents this; nothing enforces it.
5. **The records stay in the repository.** The roadmap, changes, `AGENTS.md` and the gate are the repository's.
   - Attach adds no sidecar for records, because the `no-clauductor` check holds that a repository works without
     Clauductor, and its people and CI read the rules from the repository.
   - Only the panel's own config, and MAPS' routines registry (PRD-maps), may live beside it.
6. **Order:** PANEL-42 comes after the in-flight rows and before MAPS phase 1. The owner's standing answer to
   `/propose` step 0a (PRD-maps) names PANEL-42 too: it goes ahead of the approved PANEL-25, PANEL-30 and PANEL-31
   proposals, which keep their approval and wait.
7. **Standing Tee's own attach (ST-7)** is worked out in the open in `docs/panel.md`, as the example of a repository
   with a model of its own:
   - cards run `infra/roadmap-queue.mjs`, `.claude/founder-queue.sh` and its health readers;
   - templates match its kinds of session.

   **Applying it** to the installed panel, which runs the owner's live Standing Tee lanes, waits for the owner's go.
   **ST-6**, Damian's optional onboarding section, is a change in Standing Tee's repository and waits for the owner
   too.

## What PANEL-42's proposal settles

- **The command and the page:** `clauductor panel attach <repo>`, and an Attach choice in **+ Add a project…** beside
  today's init, which writes into the repository.
- **The generator:** the sidecar config comes from `panel init`'s reading of the repository, unchanged, with a target
  outside it.
- **Commands that resolve outside the repository:** how the trust prompt shows them, and refuses them.
- **`panel remove`:** whether it keeps or deletes the sidecar folder. Recommendation: keep it, as the lane registry
  is kept today.
- **A repository with both:** an in-repository `.clauductor/panel.json` and a sidecar. Recommendation: the registry's
  `config` wins, and the page says which file it reads.

**As approved (2026-10-12, `changes/panel-42-attach/design.md`).** Three points differ from the decisions above:
- **The page's Attach choice is its own row, PANEL-43** (D1), queued after PANEL-42. PANEL-42 ships the command.
- **Decision 3's trust is stricter for a sidecar** (D5). A config outside its repository cannot be trusted while any
  command it names reaches outside the repository, and an earlier trust entry does not count. An in-repository config
  keeps today's rules. From 2026-10-13, by the owner's decision, a sidecar's commands are argv lists with no shell text
  at all: no inline code (`sh -c`, `eval`, `xargs`), no glob, variable, redirection or whitespace in a word, and every
  path inside the repository. Anything more belongs in a script in the repository.
- **Decision 4's reason is superseded** (D9). The plugin's guard hooks already stand aside in a repository without
  `.claude/clauductor-plugin` (`plugin/hooks/if-project.sh`). The opt-out keeps the plugin's `/clauductor:*` skills
  and its SessionStart hook out of the repository.

## What does not change

- `clauductor install`, `/clauductor:init` and `clauductor update` stay the way to adopt the whole model.
- The template stays the product.
- Phase 2's other rows, the gate and the merge guard are untouched.
