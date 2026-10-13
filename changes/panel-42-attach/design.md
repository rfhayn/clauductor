# Design: panel-42-attach

## Where the behaviour lives today

- **The registry** (`framework/internal/panel/config/projects.go`): `ProjectEntry.Config` is the
  panel.json path, absolute, or `""` for `<root>/.clauductor/panel.json` (`ConfigPath()`).
  `install.AddProject` (`install/projects.go`) takes `AddOptions.Config` and records it. For a root
  already registered, it switches the entry's `config` to the one passed. It loads the config
  before it registers anything, so a project with no config that loads cannot be added.
- **Every config path is relative to the repository, never to the file.** Commands run with the
  project root as their directory (`signals/run.go`). `watch:` paths and a relative
  `worktree_dir` are joined to the root (`runtime.go`, `config.WorktreeRoot`). So a config file
  can sit anywhere and still mean the same repository.
- **`panel init`** (`install/init.go`): `PlanInit` reads the repository and returns the bytes and
  a note per choice. It reads the root, the default branch, branch prefixes, where the worktrees
  live, a gate script, and the operating model's scripts if they exist. It runs git's read-only
  plumbing and reads files, nothing the repository names. It fixes the target at
  `<root>/.clauductor/panel.json` and refuses when that exists. `WriteInit` creates the file with
  `O_EXCL`.
- **Trust** (`install/trust.go`): `CheckTrust` records and checks a config's SHA-256 in
  `~/.clauductor/panel/<hash of the root>/trusted-config.json`, keyed by the config's resolved
  path. Every trust path goes through it: `panel trust` (`TrustConfigReport`), `--trust-config`,
  `panel install`, and the page (`TrustExact`). `TrustedNow` re-reads the file. The report is
  `Config.RunList()`: one line per card, queue, template, suggest command, metrics command,
  worktree hook and resume line.
- **Readers that skip the registry**: `configPathOf` (`cmd/panel.go`, for `panel trust`) and
  `panel.Run`'s up-front check of `--project` (`run.go`) both use `<root>/.clauductor/panel.json`
  when `--config` is not given. `panel list` prints no config path.
- **`panel remove`** (`install.RemoveProject`, and the page's **Remove from panel…**) deletes
  nothing on disk. It keeps the repository, its config, its worktrees and its lane registry.
- **The git poll** (`runtime.go`, `pollGit`) runs `git -C <path> status --porcelain=v2 --branch`
  on each lane's worktree and on the checkout the cards run in (`CardsCheckout`), while a page is
  visible. Without `--no-optional-locks`, git may take `index.lock` and write a refreshed
  `.git/index`.
- **The plugin** (`plugin/hooks/if-project.sh`): its guard hooks already stand aside, exiting 0
  without reading the payload, in a repository without `.claude/clauductor-plugin`. Its
  `/clauductor:*` skills and its SessionStart hook (`plugin-session.sh`) load in every repository
  in the plugin's scope.

## The shape of the change

1. **`install.PlanInit` takes its target.** It gets a target path, plus whether a config already in
   the repository should refuse the plan (for `panel init`) or add a note (for attach). The reading
   of the repository is unchanged, byte for byte. `panel init` and the page's **Create this config**
   pass `<root>/.clauductor/panel.json` as today.
2. **`install.Attach`** resolves the repository the way `panel add` does: its main worktree, a
   linked one refused, the same common-dir check, the same socket check. It picks the id (the
   config name's slug, or `--id`). It plans the config with the target
   `~/.clauductor/projects/<id>/panel.json` and writes it with `O_EXCL` (folder 0700, file 0600).
   Then it registers the repository through `AddProject` with that config. A failed registration
   removes the file it just wrote, so a refused attach leaves nothing behind. A registered project
   in D4's state is attached in place.
3. **`clauductor panel attach [<repo>]`** (`cmd/panel.go`): the repository defaults to the current
   directory's git toplevel. `--id` and `--default` work as in `panel add`. It prints what it
   wrote, init's notes, and the line to trust it: `clauductor panel trust --project <repo>`.
4. **The registry's config, everywhere.** For a project the registry holds, `configPathOf` and
   `panel.Run` use the entry's `ConfigPath()` when `--config` is not given. `panel list` prints
   each project's config path. A config outside the repository is marked so, plus a line when the
   repository also has its own `.clauductor/panel.json`. The page's project menu row shows the
   same line (a field on `ProjectSummary`).
5. **Commands stay in the repository** (D5). A pure function in the config package lists every
   command word in a config that resolves outside a root. `CheckTrust` and `TrustedNow` refuse a
   config outside its repository whose list is not empty, and the trust report marks each line.
6. **Serving writes nothing.** The panel's git reads of a served checkout pass
   `--no-optional-locks`.

## Refusals

| Situation | What happens instead |
|---|---|
| `panel attach` on a path that is not a git work tree, a linked worktree, or a repository registered under another path (the same common dir) | Refused with `panel add`'s message. Nothing is written. |
| `~/.clauductor/projects/<id>/panel.json` exists already | Refused, never overwritten (D3). The message names the file, how to serve it as it is (`clauductor panel add --project <repo> --config <file>`), and `--id` for a new one. |
| The repository is registered, and its config loads | Refused: "registered already as <id>, reading <path>". `panel remove` first, or `panel add --config` to switch on purpose (D4). |
| The repository is registered with `config: ""` and no `.clauductor/panel.json` (Standing Tee today) | Attached in place: the entry's `config` is set, and its id, socket and lane registry are kept (D4). |
| The repository has its own `.clauductor/panel.json` | Attached anyway. The output says the panel will read the sidecar and not the repository's file (D7). |
| The tmux socket the project would get is another project's | Refused, as `panel add` refuses it. The sidecar just written is removed. |
| Registering fails after the sidecar is written | The sidecar is removed. A refused attach leaves no file and no entry. |
| `panel trust` (or the page's **Trust config…**, `--trust-config` or `panel install`) on a sidecar that names a command outside the repository | Refused. Each offending line is printed (on the page, shown) with the word that resolves outside. Nothing is recorded (D5). |
| A trust entry was recorded for such a sidecar earlier (by hand, or before this change) | It does not count: the project is served with its commands off, under the **Config untrusted** banner, whose report shows the marks (D5). |
| `panel remove` (or the page's **Remove from panel…**) of an attached project | Unregisters it. `~/.clauductor/projects/<id>/` stays. `panel remove` says where it is and how to serve it again (D6). |

## Decisions (open: each awaits the owner)

**D1. The command, and the page.**
- **Recommended:** this change ships `clauductor panel attach [<repo>]` with `--id` and `--default`.
  It registers the project untrusted and prints the `panel trust` line, as `panel add` does. The
  page's **+ Add a project…** → **Attach** moves to a new row, **PANEL-43**
  (`panel-43-attach-page`), queued right after PANEL-42. That row covers a preview route, an attach
  route, the dialog's button beside **Create this config**, and their tests. Until then, the page
  serves, trusts (**Trust config…**) and removes an attached project like any other.
- **Alternatives:**
  - keep the page's Attach in PANEL-42, as the row and PRD-attach name it, which adds a fifth task
    group over `live.go`, `web/admin.go`, `panel.js` and `index.html`;
  - give `panel attach` a `--trust` flag that trusts once it has printed the report.
- **Why:**
  - **The command is all that "the panel serves a repository it never writes to" needs.** ST-7
    (Standing Tee's attach) needs only the command.
  - **The page's choice is this change's largest surface.** It needs two routes, the liveSet's
    locking around a write and a register, and the dialog's state machine. Without it, four groups
    converge in a review round each. With it, the change spans six write surfaces (step 0c's split
    threshold is about three).
  - **No `--trust`:** adding is not trusting anywhere else (`panel add`, the page's **Add without
    trusting**). Attach's whole point is a config nobody reviewed in the repository, so the owner
    reads it before trusting it.

**D2. Where the sidecar lives, and how it is made.**
- **Recommended:** `~/.clauductor/projects/<id>/panel.json`, folder 0700, file 0600, created with
  `O_EXCL`. Its bytes are `panel init`'s reading of the repository, unchanged, with only the target
  moved. They are the same bytes `panel init` would write there, with init's notes printed. It is
  the owner's file to edit (ST-7 will). Trust follows its hash as for any config.
- **Alternatives:**
  - the panel's existing per-project folder, `~/.clauductor/panel/<hash of the root>/panel.json`;
  - a generator of its own for attach. For example, one that skips the operating model's cards,
    since an attached repository usually runs a model of its own.
- **Why:**
  - **A readable folder:** the owner edits this file by hand. A hash names nothing a person can
    find, and MAPS' routines registry goes beside it (PRD-attach decision 5, PRD-maps). The id is
    already the project's name in routes, the page and its socket (`clauductor-<id>`).
  - **One reading of a repository:** two generators drift. `panel init` already adds the operating
    model's cards only when the template's scripts are in the repository, so a repository with a
    model of its own gets none (Standing Tee would get its gate script, if any, as a queue, and
    no cards). ST-7 then edits the sidecar by hand.

**D3. A sidecar already at the path.**
- **Recommended:** refuse, and never overwrite. The message names the file and
  `clauductor panel add --project <repo> --config <file>`, which serves it as it is (the registry
  already takes a config anywhere). It also names `--id <other>`, which generates a new one.
- **Alternative:** use the existing file as it is, show its trust report, and register it.
- **Why:** the folder may belong to another repository. Ids come from names, two repositories can
  share a name, and a removed project's folder stays (D6). Reusing it silently would register
  another repository's commands under this one. Trust would still hold them off, but the trust
  report is the only place the owner would notice. `panel add --config` is already the deliberate
  way to do it.

**D4. A repository that is registered already.**
- **Recommended:** attach accepts a registered project only when its registry `config` is empty
  and `<root>/.clauductor/panel.json` does not exist. That is Standing Tee's state today, a project
  that cannot load. Attach then sets the entry's `config` and keeps its id, tmux socket and lane
  registry. Any other registered project is refused with its id and the config it reads.
- **Alternative:** refuse every registered project (`panel remove`, then attach).
- **Why:** the state exists on the owner's machine, and it is the one PRD-attach says attach is
  for. Remove-then-attach would need `--force` while lanes are registered, and could hand out a new
  socket. A narrow rule keeps every working project as it is. Attaching here changes nothing until
  the owner runs it, and ST-7 waits for the owner's go.

**D5. Commands that resolve outside the repository.**
- **Recommended:** a config outside its repository (its resolved path is not under the root)
  cannot be trusted while any command it names reaches outside the repository. The commands
  checked are cards, queues, suggest, metrics, `worktree_setup` and `worktree_teardown`. A
  template's first prompt and the resume line are text typed into Claude, not commands, and are
  not checked. The report lists them as today.
  - **What is checked:** every argv word. When a command is `sh`, `bash`, `zsh` or `dash` with
    `-c`, each word of its script counts as well (split on whitespace and `;` `&&` `||` `|` `(`
    `)`, quotes removed, a leading redirection such as `2>` or `>` dropped). For a word with `=`,
    the part after it is checked too.
  - **What is refused:**
    - a word that starts with `~`;
    - a word with `$` or a backtick, because where it resolves cannot be told before it runs;
    - a word with a `/` that is absolute, or that lands outside the root once joined to it and
      resolved through symlinks as far as it exists.
  - **What is allowed:** a bare word (a program found on `PATH`, such as `sh`, `node`, `npm` or
    `make`) and a path inside the repository.
  - **How it shows:** the trust report (`panel trust`, the page's **Trust config…**, the
    **Config untrusted** banner) marks each line: `OUTSIDE THE REPOSITORY (<word>): card x runs
    …`. Every way of trusting refuses with those lines, and nothing is recorded.
  - **Where it is enforced:** in `CheckTrust` and `TrustedNow`, the two places every trust
    decision passes. A trust entry recorded earlier therefore does not turn the commands on.
  - **What it does not do:** it is not a sandbox. A script inside the repository can still run
    anything. The check holds the config to the repository's own scripts (PRD-attach decision 3:
    "its commands still run in the repository's root, against the repository's own scripts").
  - **In-repository configs:** unchanged.
- **Alternatives:**
  - mark the lines in the report but let the owner trust them anyway;
  - apply the rule to every config, in-repository ones included;
  - refuse only absolute paths, and let `~`, `$` and `..` through.
- **Why:**
  - **Not marks alone:** a sidecar is a file nobody reviewed in the repository, on a machine with
    the owner's other projects. The only commands the premise allows are the repository's own.
    A warning the owner clicks past is not a control.
  - **Not every config:** an in-repository config is reviewed with the repository, and changing
    its rules is a change to every installed project. That is out of this row's scope.
  - **Not absolute paths alone:** `~/bin/x`, `$HOME/bin/x` and `../other-repo/x` reach out just as
    far.
  - **The cost:** a sidecar cannot run a personal script from `~/bin`. Its logic belongs in the
    repository, or in an in-repository config.

**D6. `panel remove` and the sidecar.**
- **Recommended:** keep `~/.clauductor/projects/<id>/`, as remove keeps the lane registry today.
  `panel remove` names the folder and how to serve it again
  (`clauductor panel add --project <repo> --config <file>`). The page's confirmation already
  lists the config's path, and its "nothing on disk is deleted" stays true.
- **Alternatives:**
  - delete the folder on remove;
  - offer `--delete-config`.
- **Why:** remove deletes nothing on disk today, and the page's confirmation promises exactly that.
  A sidecar may hold the owner's edits (ST-7), and deleting it cannot be undone. Deleting is the
  owner's to do by hand.

**D7. A repository with its own `.clauductor/panel.json` and a sidecar.**
- **Recommended:** the registry's `config` wins, as `ConfigPath()` already decides, and every
  reader goes through the registry:
  - `panel trust` and a bare `panel` / `panel --project` without `--config` use the registered
    project's config;
  - `panel list` prints `config <path>`, marked `(outside the repository)`, with a line when the
    repository's own `.clauductor/panel.json` exists and is not read;
  - the project menu row shows the same one line.
- **Alternative:** the repository's own file wins once it exists (the in-repository path first).
- **Why:** `projects.json` is the machine's file so that "nothing in a repository can add a project,
  choose its socket, or make it the default" (`docs/panel.md`, Projects). A pull that adds a
  `panel.json` would otherwise switch what the panel reads without the owner choosing it. The new
  file would be untrusted, but the panel would stop serving the trusted sidecar. Today `panel
  trust` and a bare `panel` in an attached repository read the wrong file, or none.

**D8. The proof that the repository's files are untouched.**
- **Recommended:** two tests on a real git repository in a temporary directory, both using one
  snapshot function. It records every file and directory under the root, `.git` included: path,
  mode, size, modification time and SHA-256.
  - **Attach:** run `clauductor panel attach` through the cobra command, then compare. They must
    be identical.
  - **Serving:** load the project as the panel does, make one tracked file's stat cache stale (same
    bytes, a newer mtime), run one git poll of the cards' checkout, then compare. They must be
    identical. The serving test is first run against today's `git status` to see `.git/index`
    change. The panel's git reads then pass `--no-optional-locks` (git's documented switch for
    background readers).
- **Alternatives:**
  - test attach only;
  - compare `git status --porcelain --ignored` before and after.
- **Why:**
  - **Not attach alone:** the one write serving makes would go unproven. The PRD's claim is that
    the panel *serves* a repository it never writes to.
  - **Not git status:** it misses `.git` itself and a file rewritten with the same bytes.
  - **What neither test covers:** lanes, RUN and Close lane, which write as in any repository
    (proposal, Out of scope).

**D9. The plugin at user scope, and its per-repository opt-out.**
- **Recommended:** `docs/plugin.md` gets a section on installing at user scope.
  - **What that loads in a repository that does not use the model:** the `/clauductor:*` skills
    (namespaced, so they never collide with the repository's own) and the SessionStart hook,
    which records the plugin's root and otherwise says nothing there.
  - **What it does not load:** the guard hooks, which already stand aside without
    `.claude/clauductor-plugin` (`if-project.sh`).
  - **The opt-out:** a repository with a model of its own turns the plugin off with
    `"enabledPlugins": {"clauductor@clauductor": false}` in its uncommitted
    `.claude/settings.local.json`. The skills are then not offered there at all.
  - `docs/panel.md`'s attach section links to it. Nothing enforces it (PRD-attach decision 4).
- **Alternative:** document it as PRD-attach decision 4 words it: "its hooks would run beside a
  repository's own guards, which is what the opt-out prevents".
- **Why:** the code says otherwise. The guard hooks already stand aside in a repository without
  the plugin's marker. A doc that names the wrong reason would send a reader looking for a double
  guard that never fires. The opt-out still matters: it keeps the plugin's skills, and its
  session-start hook, out of a repository whose own skills and hooks run the work.
