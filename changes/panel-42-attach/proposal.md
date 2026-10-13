**Approved:** 2026-10-12 by Rich · design a72fc5cbcf53
**Roadmap row:** PANEL-42
**Risk:** normal

## Why

Until now the panel reached a repository by writing into it: `panel init` (or `clauductor install`,
or the plugin's `/clauductor:init`) puts `.clauductor/panel.json` in the repository, and the panel
reads it from there. The owner decided on 2026-10-12 that a repository can use the panel without
Clauductor installed in it, and that Standing Tee stays independent for good
(`docs/prds/active/PRD-attach.md`, decisions 1 and 3).

Most of what that needs already lives outside every repository: the panel's hooks (the user's
`~/.claude/settings.json`), the project registry (`~/.clauductor/panel/projects.json`), trust
(recorded per project against the config's resolved path and content hash, `install.CheckTrust`)
and the registry's `config` field, which may already name a file anywhere. The one missing piece is
the config itself:

- **A registered project with no config does not load.** An empty `config` falls back to
  `<root>/.clauductor/panel.json` (`ProjectEntry.ConfigPath()`), and `config.LoadConfigRaw` fails
  when it is missing. Standing Tee is in that state on the owner's machine today: `panel list`
  says `config does not load: no panel config at …/standingtee/.clauductor/panel.json`, with 0
  lanes.
- **Nothing generates a config outside a repository.** `panel init` (and the page's **Create this
  config**) can only write `<root>/.clauductor/panel.json`.
- **Several readers ignore the registry's `config`.** `panel trust --project <repo>` and a bare
  `panel` (or `panel --project <repo>`) read `<root>/.clauductor/panel.json` unless `--config` is
  passed (`configPathOf` in `framework/internal/cmd/panel.go`, `panel.Run`'s first check in
  `framework/internal/panel/run.go`). So a project served from a config outside it could not be
  trusted by `panel trust`, and a bare `panel` inside it would refuse to start.
- **Serving a repository can write into it today.** The panel's git poll runs `git status` in the
  checkout the cards run in (`pollGit`, `runtime.go`). `git status` may rewrite `.git/index` to
  refresh its stat cache unless told not to (`--no-optional-locks`). For an attached repository,
  "the panel never writes to it" has to hold for serving as well as for attaching.

**Step 0a.** `changes/` holds approved proposals for PANEL-25, PANEL-28, PANEL-29, PANEL-30 and
PANEL-31. The owner's standing answer to `/propose` step 0a covers PANEL-42 by name
(`docs/prds/active/PRD-maps.md`, "The standing answer to `/propose` step 0a"; PRD-attach decision
6): PANEL-42 goes ahead of the approved PANEL-25, PANEL-30 and PANEL-31, which keep their
approval and wait.

## What changes

- **`clauductor panel attach [<repo>]`** serves a repository without writing to it. It reads the
  repository exactly as `panel init` does and writes the result to
  `~/.clauductor/projects/<id>/panel.json`. It then registers the repository with `config` naming
  that file, with its commands off until the owner trusts it (`panel trust`, which now finds it).
  Cards, queues and templates run in the repository's root against the repository's own scripts,
  as they do today (D1, D2).
- **Nothing is overwritten.** A sidecar already at that path is refused (D3). A registered project
  whose config does not load and whose registry `config` is empty, Standing Tee's state today, is
  attached in place: its id, socket and lane registry are kept (D4).
- **A config outside its repository runs only the repository's commands.** The trust report marks
  any card, queue, suggest, metrics or worktree-hook command that names a path outside the
  repository (or one whose location cannot be told before it runs). No way of trusting will record
  such a config, and the panel ignores a trust entry recorded for one earlier. A config inside the
  repository keeps today's rules (D5).
- **One config per project, read from the registry.** `panel trust`, a bare `panel`, `panel list`
  and the page all read the config the registry names. `panel list` and the project menu say when
  a project reads a config outside its repository, and when the repository's own
  `.clauductor/panel.json` is therefore not read (D7).
- **`panel remove` keeps the sidecar folder**, says where it is, and says how to serve it again
  (D6).
- **A test proves the repository's files are untouched.** Every file under the repository's root,
  `.git` included, is identical before and after `panel attach`, and after the panel loads the
  project and polls it. The panel's git reads pass `--no-optional-locks` so the poll does not
  rewrite `.git/index` (D8).
- **The plugin's per-repository opt-out is documented.** `docs/plugin.md` says the plugin can be
  installed at user scope. It explains how a repository with a model of its own turns it off with
  `"enabledPlugins": {"clauductor@clauductor": false}` in its uncommitted
  `.claude/settings.local.json`, and what that does and does not change (D9).

## What the existing specs already guarantee

This repo has no living specs yet: `specs/` holds only its README. The approved, unmerged changes
(`panel-new-lane` in PANEL-29, `panel-version-check` in PANEL-31, and the others in `changes/`)
don't touch the registry, `panel init` or trust. So nothing here modifies or contradicts a
requirement.

What this change alters is documented, not specified, in `docs/panel.md`:

- "Projects" (the registry, add and remove);
- "Add, trust and remove from the page";
- "`panel init`";
- "Config trust";
- "Security model".

This change adds the capability spec `panel-attach` and updates those sections to match.

**Scope.** The row names the page's Attach choice. D1 recommends moving that choice to a new row,
PANEL-43, queued right after this one, so this change ships the command and everything serving
needs. This proposal's PR updates the roadmap to match: it narrows the PANEL-42 row, adds
`framework/internal/panel` to its scope (for `run.go` and the git poll), and adds PANEL-43.

## Out of scope

- **The page's Attach choice in + Add a project…** (the preview, the button and its routes):
  PANEL-43, if the owner agrees with D1. Until then the page serves, trusts and removes an attached
  project like any other. It shows the trust report with its marks and names the config in the
  project menu.
- **Standing Tee's own attach**: ST-7. That includes its sidecar's cards (`infra/roadmap-queue.mjs`,
  its founder queue and health readers) and templates, and the worked example in `docs/panel.md`.
  Applying it to the installed panel waits for the owner's go.
- **What a lane, RUN or Close lane writes.** Starting a lane creates a worktree and a branch.
  RUN takes the gate lease under git's common dir. These are the owner's own actions, and they
  write as they would in any repository; `docs/panel.md` says so. The proof in D8 covers
  attaching and serving.
- **What the repository's own scripts write** when a card or queue runs them. They are the
  repository's commands, by D5's rule.
- **Records beside the repository** (roadmap, changes, AGENTS.md): PRD-attach decision 5 keeps them
  in the repository. MAPS' routines registry, which may live beside the sidecar, is OPS-42's.
- **Enforcing the plugin opt-out.** PRD-attach decision 4: documented, not enforced.

## How we'll know

- **Signal:** a repository with nothing Clauductor installed in it is attached on the owner's
  machine and served with its cards on, and running `git status --porcelain --ignored` in it shows
  nothing the panel wrote. `panel list` names its config under `~/.clauductor/projects/`.
- **Check after:** 14 days
