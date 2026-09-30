# `clauductor panel` — a local web panel over your Claude sessions

`clauductor panel` serves a live dashboard of every Claude Code session working in your projects:
which lanes (worktrees) have a session, whether each is busy, waiting or idle, its context %,
its running subagents, what needs you, the account quota, open PRs, and any cards the project
defines. One panel serves every project you register (PANEL-16); the project's name at the top
of the page switches between them.

It also **runs lanes**: each lane is an interactive `claude` in its own tmux session, with a
terminal embedded in the page. You start, stop, interrupt, restart and resume lanes from the
browser, and a launchd login agent keeps the panel running with no terminal open. You can work
from the browser alone. On top of lanes it offers lane templates, a queue for a shared gate,
alerts with macOS notifications, a quota guard, and restoring every lane after a reboot.

It is **standalone**. It does not need `clauductor install`, the template, the skills, the
SQLite database or file locks. It reads only Claude Code's own signals, plus git and `gh`:

| Source | How | Gives |
|---|---|---|
| HTTP hooks | pushed to `POST /hook` | prompt submitted, turn stopped, subagent start/stop, notifications, session end |
| Status line | the status-line script copies its stdin to `POST /status` | context %, est. cost, and the account's quota windows (from any session, in any project) |
| `claude agents --json [--cwd <dir>]` | polled every 2 s, every 5 s while hooks flow, every 15 s with no lane and no hook for 5 min | which sessions exist, busy / waiting / idle |
| `claude --version` | at start, then every 10 min | whether the version-pinned heuristics apply |
| `claude auth status --json` | at start, then every 10 min, with a lane's environment (no API key) | how the account signs in and its plan: what the quota's place shows (see *The account and its quota*) |
| queue leases | read every 1 s from the git common dir; `ps` once per process (and every 30 s), else `kill -0` | who holds the gate, who waits |
| `git worktree list --porcelain` | polled every 10 s, and within ~2 s of a worktree being added or removed | lanes, and the branch of each |
| `gh pr list` | polled every 60 s | open PRs and their checks |
| project cards | per card: on a file change or an interval | anything the project prints |
| the project's metrics command | on its `metrics.refresh` (default every 15 min), at start and on **Refresh**, only while the config is trusted | the Metrics view's figures (see *Metrics*) |
| `gh pr list --state merged` | at most every 10 min, on the PR source's cadence, and only while a page is in view | merge frequency and PR cycle time for the Metrics view |
| `gh api graphql` (review threads) | per lane with an open pull request, at most every 2 min, only while a page is in view | a lane's unresolved review threads (merge readiness, PANEL-20) |
| `tmux -L <socket> list-panes -a` | one call for every lane: polled every 2 s while lanes run, every 10 s with none, and right after a lane action | which lanes run, and whether their program exited |
| `tmux -L <socket> show-environment -g` | when the lane set changes, every 30 s, and before every lane start | whether an API key there blocks lanes |
| the lane registry | in memory, re-read from disk every 30 s | which lane owns which Claude session id, where, as which type |

Keeping the panel current costs **no model tokens**. It never reads transcripts or screens; the
only text it types into a lane on its own is a template's first prompt, once.

New to the panel? [guide.md](guide.md) is a short how-to for the page; **?** in its header shows
the keyboard shortcuts.

**Contents:** [Quick start](#quick-start) · [Configuration reference](#configuration-reference) ·
[The page](#the-page) · [Lanes](#lanes) · [Queue and the gate lock protocol](#queue-and-the-gate-lock-protocol) ·
[Alerts](#alerts) · [Metrics](#metrics) · [Appearance](#appearance) · [Signals: hooks and the status line](#signals-hooks-and-the-status-line) ·
[Security model](#security-model) · [Operations](#operations) · [Troubleshooting](#troubleshooting) ·
[Not yet](#not-yet)

## Quick start

1. **Install** the `clauductor` binary: run `./install.sh` in the clauductor repository.
2. **Init** the project: in its checkout, `clauductor panel init` writes a starter
   `.clauductor/panel.json` from what the repository already says, and prints why it chose each
   value (see [`panel init`](#panel-init)). Review it and commit it.
3. **Trust** it: after reviewing it, run `clauductor panel trust`. Until then the panel starts but
   runs none of its commands or templates, and it asks again whenever the file changes (a pull,
   say). See [Config trust](#config-trust).
4. **Open** it: `clauductor panel` starts the panel and opens the browser. To keep it running
   with no terminal, `clauductor panel install --project <path> --app` installs a login agent;
   then `clauductor panel open` (or the app) opens the page.
5. **More projects**: in each other repository, `clauductor panel init`, review, `clauductor
   panel trust`, then `clauductor panel add`, and restart the panel. See [Projects](#projects).
6. Optional: send the panel a copy of your status line ([The status line](#the-status-line)) for
   context % and quota, and put your gate script through the queue
   ([In a project's gate script](#in-a-projects-gate-script)).

Every command:

```bash
clauductor panel init [--project p]           # write a starter .clauductor/panel.json (never overwrites)
clauductor panel                              # serve every project; this repository is added if it is not, and opened on
clauductor panel --project ~/Development/app  # or name it
clauductor panel --project p --only           # serve that one project alone
clauductor panel --config /tmp/panel.json     # use a config outside the repo
clauductor panel --port 4393 --no-open        # print the URL instead of opening a browser
clauductor panel --uninstall-hooks            # remove the panel's hooks and exit
clauductor panel --trust-config               # trust panel.json as it is now, then run
clauductor panel trust [--project p]          # trust panel.json as it is now (a running panel follows)
clauductor panel add [--project p] [--config f] [--id x] [--default]   # register a project (untrusted until `trust`)
clauductor panel remove <id|path> [--force]   # unregister one; its lanes keep running
clauductor panel list                         # id, name, root, socket, trust and lanes of each
clauductor lock-run [--lane id] [--ttl 10m] <lockdir> -- <cmd…>   # run a command through a queue

clauductor panel install [--project ~/Development/app] [--config <file>] [--port 4393] [--app] [--remote-control=all|lanes|off]
clauductor panel open [--project id]          # open the installed panel in the browser (on that project)
clauductor panel rotate-token                 # replace the installed panel's token
clauductor panel uninstall                    # stop and remove the login agent
```

One panel serves every registered project. Stop a hand-started panel with Ctrl-C. Stopping the
panel never stops a lane: lanes belong to tmux.

### Projects

The projects a panel serves are listed in `~/.clauductor/panel/projects.json` (0600, written
atomically). It is the machine's file, not a repository's: nothing in a repository can add a
project, choose its socket, or make it the default.

```json
{ "version": 1, "default": "standingt",
  "projects": [ { "id": "standingt", "root": "/Users/me/Development/StandingT",
                  "config": "", "tmux_socket": "clauductor", "added": 1790000000 } ] }
```

- **A project is one repository**, named by its main worktree. `panel add` refuses a linked
  worktree (add its main one) and a path already registered, through a symlink too. Its `id`
  (`[a-z0-9][a-z0-9-]{0,40}`) comes from the config's `name` (`StandingT` is `standingt`), or
  `--id`; it names the project in routes and in the page. The files of a project stay where they
  were, keyed by a hash of its path (the lane registry, trust, notifications).
- **Adding is not trusting.** `panel add` loads and checks the config, and prints whether it is
  trusted; its cards, queue commands and templates stay off until `clauductor panel trust`.
- **Each project has its own tmux server.** A project's socket is its config's `tmux_socket`,
  else the one recorded when it was added: the first project gets the historical `clauductor`,
  so lanes started before PANEL-16 carry on, and each project after it gets `clauductor-<id>`.
  Two projects on one socket are refused at `add`, and a project whose socket another has is not
  loaded. `panel list` names each project's socket. Each lane is tagged with its project
  (`@clauductor_project`); a session tagged for another project is never shown as a stray, and one
  with no tag (started before PANEL-16) belongs to the socket's project.
- **The default project** is the one a page with no `?p=` opens on, and the one the routes before
  PANEL-16 reach. `panel --project p` (the login agent's plist from before PANEL-16 does this) and
  a bare `panel` inside a repository register it if needed and make it the default. `panel add
  --default` does too; removing the default makes the first remaining project the default.
- **`remove`** never stops a lane and keeps the lane registry, so adding the project again brings
  its lanes back; while lanes are registered it needs `--force`.
- **Restart the panel after `add` or `remove`**: it reads `projects.json` at start (the commands
  print the `launchctl kickstart` line).
- **A project that cannot load** (its config, its worktree list, its socket) is shown in the
  project menu with the reason, and the others serve. The project named on the command line must
  load, as before.

The hooks and the status line are the machine's: every session posts to the one panel. The panel
places each post in its project by its session id first (a lane's own session, or a session it
has placed before, which stays with its project after a `cd`), then by the deepest worktree of
any project that holds its `cwd` (one repository can sit inside another's checkout). A post that
is no project's is counted as **other projects** in each project's footer. The quota, Claude
Code's version and the account are the machine's too: every status post moves every project's
quota, and the quota alert notifies once, not once per project (against the default project's
`alerts`).

## Configuration reference

The project keeps its config at `<project>/.clauductor/panel.json` (or pass `--config`). Unknown
keys are an error, so a misspelt key fails loudly.

### `panel init`

`clauductor panel init [--project <path>]` writes `<git toplevel>/.clauductor/panel.json`. It
**refuses to overwrite** anything already at that path, a dangling symlink included. What it
writes, it reads from the repository:

| Key | From |
|---|---|
| `$schema`, `version` | the published schema, and the latest version |
| `name` | the directory name of the git toplevel |
| `base` | origin's default branch (`origin/HEAD`); else `origin/main` or `origin/master`; else, with no `origin`, the branch the project root has checked out |
| `lanes` | the default branch → `orchestrator`, and each prefix your local branches use (`feature/x` → `"feature/": "feature"`, up to five); with none, `feature/` and `fix/` as a starting point |
| `worktree_dir` | the directory every linked worktree already shares; else the default |
| `queues` | one `gate` queue, only if the project itself names a gate: a `package.json` script (run with the package manager its lockfile names) or a `Makefile` target called `gate`, `ci`, `check`, `verify` or `test`, best name first |

It invents **no card** (a card's command runs by itself on every refresh) and no template. The
only command it writes of its own is the gate queue's, which runs only when you press **RUN**, and
it prints it. JSON has no comments, so `init` prints the reason for each value instead.

**A repository that runs Clauductor's operating model** (PANEL-18: the template `clauductor
install` copies, recognised by `.claude/owner-queue.sh` or `.claude/roadmap-queue.sh`) also gets
that model's own panel setup, the same as the template's preset, for the files it has:

| File | Adds |
|---|---|
| `.claude/owner-queue.sh` | the pinned card **Owner queue**: `sh -c "sh .claude/owner-queue.sh 2>&1"`, refreshed on `watch:docs/owner-queue.md` |
| `.claude/roadmap-queue.sh` | the pinned card **Change queue**: `sh -c "sh .claude/roadmap-queue.sh --text 2>&1"`, refreshed on `watch:docs/roadmap.md` |
| either of those | the four lane templates **build**, **propose**, **fix** and **ops**, and the lanes they use (`change/` → `build`, `fix/` → `fix`, `ops/` → `ops`; a detected prefix mapped otherwise is changed, and said so) |
| `.claude/panel-suggest.sh` | each template's **Up next** (`sh .claude/panel-suggest.sh <template>`); without it, the templates have none |
| `scripts/ci/run-local.sh` | the `gate` queue on it, instead of a gate found in `package.json` or a `Makefile` (those are printed as "also found") |

Each addition is printed with its reason, like every other value, and none of them runs before
`clauductor panel trust`. Without those files, `init` prints one line more: cards and lane
templates come with `clauductor install` (the operating model), or can be added by hand (see
[Keys](#keys), [Pinned cards](#pinned-cards) and [Lane templates](#lane-templates)).

For a repository with a `pnpm` project whose `Makefile` has a `ci` target, it prints:

```text
Wrote /Users/me/Development/acme-web/.clauductor/panel.json:

{
  "$schema": "https://raw.githubusercontent.com/rfhayn/clauductor/main/docs/panel.schema.json",
  "name": "acme-web",
  "version": 2,
  "lanes": {
    "feature/": "feature",
    "fix/": "fix",
    "main": "orchestrator"
  },
  "worktree_dir": ".worktrees",
  "base": "origin/main",
  "queues": [
    {
      "id": "gate",
      "title": "Gate: make ci",
      "lock": "clauductor/gate.lock",
      "command": [
        "make",
        "ci"
      ]
    }
  ]
}

  name          "acme-web", shown in the status bar
  base          origin/main: new lanes branch from it (origin's default branch)
  lanes         feature/ → feature, fix/ → fix, main → orchestrator (from the prefixes of your local branches)
  worktree_dir  .worktrees (where your 1 linked worktree(s) already are)
  queues        gate runs `make ci` (Makefile target "ci"), and only when you press RUN on the page; also found `pnpm run check`, `pnpm run test`
  cards and lane templates: none; they come with `clauductor install` (Clauductor's operating model), or add them by hand (docs/panel.md, "Keys": cards, templates)
```

### Versions

A config declares the version it is written for, and may use only the keys of that version or an
earlier one. The **Since** column of the key table says which is which:

| Version | Keys |
|---|---|
| 1 | `name`, `lanes`, `cards`, and the keys of lanes the panel starts: `tmux_socket`, `worktree_dir`, `base`, `lane_types` |
| 2 | orchestration: `templates`, `queues`, `alerts`, `quota_guard`, `host_names` |
| 3 | what's next: `templates[].suggest` (see *Suggestions*) and `cards[].pin` (see *Pinned cards*) |
| 4 | metrics (PANEL-19): `metrics` (see *Metrics*), `alerts.approval_wait_hours`, `alerts.stale_days` and `quota_economy` |
| 5 | the lane lifecycle (PANEL-20): `lanes_auto_close`, `lane_types.<key>.auto_close`, `quota_auto_resume`, `quota_resume_line`, `worktree_setup`, `worktree_teardown`, `ports` |

- A key from a later version than the file declares is refused, with an error that names the key
  and the version it needs: `panel config: "templates" needs "version": 2 or later (the file
  declares version 1); raise the version, or remove the key`.
- A `version` outside 1–5 (0 included) is refused.
- A key can be newer than the key it sits in (`templates[].suggest` is version 3 inside version 2's
  `templates`). The error names it the same way, and the schema bans it where it sits.
- A file with **no** `version` is read as the latest version, so no existing config breaks. The
  panel says so once at start and suggests adding it: a later panel will read an undeclared config
  as its own latest version.

### The JSON Schema

[`docs/panel.schema.json`](panel.schema.json) is the config as a JSON Schema (draft 2020-12):
every key, its type, the validators' own patterns, the defaults, and the version gate. `init`
writes it as `"$schema"`, so an editor that reads `$schema` (VS Code does) validates the file as
you type. The panel ignores the key. The URL names the `main` branch, so it is the schema of the
newest panel: the repository has no release tags yet to pin it to. A key your editor flags that
your panel accepts (or the reverse) means the two differ in version.

The schema and the key table below are **generated** from the Go config types
(`framework/internal/panel/config`, `Fields`): `go generate ./internal/panel/config` in
`framework/` rewrites both, and a test fails when either differs from what the types generate, or
when a config key has no entry (so no version) in `Fields`.

### An example

```json
{
  "$schema": "https://raw.githubusercontent.com/rfhayn/clauductor/main/docs/panel.schema.json",
  "name": "My Project",
  "version": 2,
  "lanes": { "feature/": "build", "fix/": "fix", "ops/": "ops", "main": "orchestrator" },
  "lane_types": { "build": { "model": "opus", "effort": "high" } },
  "cards": [
    { "id": "todo", "title": "TODOs", "command": ["sh", "-c", "grep -rn TODO src | head -20"],
      "refresh": "watch:src" },
    { "id": "releases", "title": "Recent releases", "command": ["gh", "release", "list", "--limit", "5"],
      "refresh": "interval:300" }
  ],
  "templates": [
    { "id": "build", "title": "Build a feature", "lane_type": "build",
      "branch_pattern": "feature/{name}", "first_prompt": "Build the feature {name} described in docs/specs/{name}.md" },
    { "id": "fix", "title": "Fix an issue", "lane_type": "fix",
      "first_prompt": "Fix GitHub issue {issue}. Start from the code as built." }
  ],
  "queues": [
    { "id": "gate", "title": "Full gate", "lock": "clauductor/gate.lock", "command": ["make", "ci"] }
  ],
  "alerts": { "idle_minutes": 20, "context_pct": 80, "five_hour_pct": 90, "waiting_seconds": 120 },
  "quota_guard": { "five_hour_pct": 95 }
}
```

A smaller one is in `framework/internal/panel/config/testdata/panel.json`.

### Keys

<!-- config-reference begin: generated by `go generate ./internal/panel/config`; do not edit -->
| Key | Type | Default | Since | Meaning |
|---|---|---|---|---|
| `$schema` | string |  | 1 | The JSON Schema the file follows, for editors: `https://raw.githubusercontent.com/rfhayn/clauductor/main/docs/panel.schema.json`. The panel ignores it. `clauductor panel init` writes it. |
| `version` | integer: 1 to 5 |  | 1 | The config version the file is written for. It may use only the keys of that version or an earlier one; a key from a later version is an error that names the key and the version it needs. Without it the file is read as the latest version, and the panel says so once at start. |
| `name` | string, **required** |  | 1 | Shown in the status bar and in notification titles. One line of plain text, at most 80 characters, not blank and not starting with `-`. Matches `^ *[^ \t\n\f\r\v-]`. |
| `lanes` | object: branch rule → lane type |  | 1 | A rule ending in `/` is a prefix (`"feature/"` matches `feature/add-x`, shown as `add-x`). A rule ending in `*` is a prefix without the star (`"feature/spike-*"`). Any other rule matches one branch exactly (`"main"`). The longest matching rule wins. An unmatched branch is `other`; a detached HEAD is `detached`. |
| `cards` | array |  | 1 | Commands whose output renders as a card in the Activity drawer (see *Card output*). |
| `cards[].id` | string, **required** |  | 1 | Unique among the cards. Matches `^[a-z0-9][a-z0-9_-]{0,63}$`. |
| `cards[].title` | string |  | 1 | The card's heading. |
| `cards[].command` | array of strings, **required** |  | 1 | argv, run in the project root **without a shell**. Use `["sh", "-c", "..."]` if you want one. 30-second timeout. |
| `cards[].refresh` | string, **required** |  | 1 | `"watch:<relpath>"`: re-run when that file (or a direct entry of that directory) changes; the path must stay inside the project. `"interval:<seconds>"`: re-run on a timer (minimum 5 s). Every card also runs at start and on ↻ REFRESH. Matches `^(watch:.+|interval:0*[1-9][0-9]*)$`. |
| `cards[].pin` | boolean | `false` | 3 | Also show the card in the side panel, as a tab of the pinned cards' box: below the selected lane's details, or alone while no lane is selected. Each output line is a title that opens to the rest of the line (see *Pinned cards*). |
| `tmux_socket` | string | `"clauductor"` | 1 | The panel's own tmux server (`tmux -L <name>`). Lanes never mix with your own tmux sessions. Matches `^[A-Za-z0-9_-]{1,64}$`. |
| `worktree_dir` | string | `".claude/worktrees"` | 1 | Where a new lane's worktree is created: relative to the project root and inside it, or absolute. |
| `base` | string | `"origin/main"` | 1 | What a new lane's branch starts from. `git fetch` runs first; if it fails, the lane still starts and the page says so. Matches `^[A-Za-z0-9][A-Za-z0-9._/@{}^~-]{0,199}$`. |
| `lane_types` | object: lane type → options |  | 1 | Launch options per lane type, passed as `claude --model <m> --effort <e>`. |
| `lane_types.<key>.model` | string |  | 1 | One argv element: `--model <value>`. Matches `^[A-Za-z0-9][A-Za-z0-9._\[\]-]{0,63}$`. |
| `lane_types.<key>.effort` | string |  | 1 | One argv element: `--effort <value>`. Matches `^[A-Za-z0-9][A-Za-z0-9._\[\]-]{0,63}$`. |
| `lane_types.<key>.auto_close` | string: "off" or "on_merge" |  | 5 | Overrides `lanes_auto_close` for this lane type. |
| `templates` | array |  | 2 | Lane recipes offered by **New lane** (see *Lane templates*). |
| `templates[].id` | string, **required** |  | 2 | Unique among the templates. Matches `^[a-z0-9][a-z0-9_-]{0,63}$`. |
| `templates[].title` | string |  | 2 | Shown in the dialog. |
| `templates[].lane_type` | string, **required** |  | 2 | One of the config's lane types (a value of `lanes`, or a key of `lane_types`). |
| `templates[].branch_pattern` | string |  | 2 | The new branch, with `{name}` (required) and `{issue}`, e.g. `"feature/{name}"`. Empty means the lane type's prefix + `{name}`; a lane type with no prefix rule needs one. |
| `templates[].first_prompt` | string, **required** |  | 2 | ONE line typed into claude once it is ready. `{name}` and `{issue}` only; no newline or control character; at most 4000 characters. |
| `templates[].model` | string |  | 2 | Overrides the lane type's model. Matches `^[A-Za-z0-9][A-Za-z0-9._\[\]-]{0,63}$`. |
| `templates[].effort` | string |  | 2 | Overrides the lane type's effort. Matches `^[A-Za-z0-9][A-Za-z0-9._\[\]-]{0,63}$`. |
| `templates[].suggest` | object |  | 3 | What this template could start next: a command whose output **New lane** lists under the template, each row filling in the lane name (see *Suggestions*). |
| `templates[].suggest.command` | array of strings, **required** |  | 3 | argv, run in the project root **without a shell**, like a card's, only while the config is trusted. 30-second timeout. |
| `templates[].suggest.refresh` | string, **required** |  | 3 | When to re-run it, as a card's `refresh`: `"watch:<relpath>"` or `"interval:<seconds>"`. Matches `^(watch:.+|interval:0*[1-9][0-9]*)$`. |
| `queues` | array |  | 2 | Shared resources held as a lease on disk (see *Queue and the gate lock protocol*). |
| `queues[].id` | string, **required** |  | 2 | Unique among the queues. Matches `^[a-z0-9][a-z0-9_-]{0,63}$`. |
| `queues[].title` | string |  | 2 | Shown on the queue card. |
| `queues[].lock` | string, **required** |  | 2 | The lease directory, relative to the **git common dir** (so every worktree agrees), e.g. `"clauductor/gate.lock"`. No `..`, not absolute. |
| `queues[].command` | array of strings |  | 2 | Optional argv that **Run in `<lane>`** (a lane's Gate tab) starts through `lock-run` in that lane's worktree. It runs only when you press it. |
| `alerts` | object |  | 2 | Alert thresholds (see *Alerts*). A missing key takes the default; `0` turns that alert off. |
| `alerts.idle_minutes` | number | `30` | 2 | A live session idle longer than this raises an idle alert. |
| `alerts.context_pct` | number | `85` | 2 | A context window at or above this percentage raises a context alert. |
| `alerts.five_hour_pct` | number | `90` | 2 | The 5-hour quota at or above this percentage raises a quota alert (block at 100%). |
| `alerts.waiting_seconds` | number | `120` | 2 | A permission prompt, MCP elicitation or input request older than this raises a waiting alert. |
| `alerts.notify` | boolean | `true` | 2 | Send macOS notifications for the alerts that interrupt. |
| `alerts.min_interval_seconds` | number | `300` | 2 | At most one notification per lane per interval. |
| `alerts.approval_wait_hours` | number | `24` | 4 | A change's proposal with no `**Approved:**` line, waiting longer than this since it was last written, raises an approval alert (see *Needs you from the metrics*). |
| `alerts.stale_days` | number | `3` | 4 | A lane on a branch of its own with no commit for this many days (counted from its start while it has none of its own) raises a stale alert. |
| `quota_guard` | object |  | 2 | Refuses to start or restore a lane at or above a 5-hour quota (see *Quota guard*). |
| `quota_guard.five_hour_pct` | number | `95` | 2 | Refuse at or above this 5-hour quota, unless the dialog's override is ticked. `0` turns it off. |
| `host_names` | array of strings |  | 2 | Extra names the panel answers to, each `<label>.localhost` in lower case (for example `"myproject.localhost"`). `clauductor.localhost` always works. No wildcards. |
| `quota_economy` | object |  | 4 | Economy mode (see *Economy mode*): off unless set. Read from the default project's config, since the quota is the machine's. |
| `quota_economy.five_hour_pct` | number |  | 4 | At or above this 5-hour quota the panel writes `~/.clauductor/panel/economy.json` with `"economy": true` and shows an **economy** badge by the quota; it turns off once the quota is 3 points below. `0` is off. |
| `lanes_auto_close` | string: "off" or "on_merge" | `"off"` | 5 | `"on_merge"` closes a lane once its branch's pull request merges, as **Close lane** would, and only when claude is idle, the worktree clean and the pull request merged at the branch's tip; otherwise Needs you asks "PR merged: close lane?" (see *Close a lane when its PR merges*). |
| `quota_auto_resume` | boolean | `false` | 5 | Once the 5-hour window resets, type `quota_resume_line` into each lane the usage limit stopped, once per reset, only while claude is idle and waits on no permission (see *Resume after the 5-hour reset*). |
| `quota_resume_line` | string | `"continue"` | 5 | The line `quota_auto_resume` types. One line of plain text, at most 200 characters. |
| `worktree_setup` | object |  | 5 | A command run in a new lane's new worktree before claude starts (see *Worktree setup, teardown and ports*). |
| `worktree_setup.command` | array of strings, **required** |  | 5 | argv, run **without a shell** in the worktree, only while the config is trusted, with `CLAUDUCTOR_LANE` and `CLAUDUCTOR_PORT` set. 5-minute timeout. |
| `worktree_teardown` | object |  | 5 | A command run in a lane's worktree before **Close lane** removes it. |
| `worktree_teardown.command` | array of strings, **required** |  | 5 | argv, as `worktree_setup.command`. If it fails, or leaves the worktree changed, the worktree stays. |
| `ports` | object |  | 5 | Gives each lane a stable port of its own: `base`, `base + per_lane`, … kept in the lane registry, exported to the lane as `CLAUDUCTOR_PORT` and shown in its header. |
| `ports.base` | integer, **required** |  | 5 | The first lane's port, 1024 to 65000. |
| `ports.per_lane` | integer, **required** |  | 5 | The step between two lanes' ports, 1 to 100 (a lane may use the ports up to the next one). |
| `metrics` | object |  | 4 | The project's metrics for the **Metrics** view and the Flow card (see *Metrics*). Without it the panel still shows what it computes itself: merge frequency and PR cycle time from `gh`, and spend from the status line. |
| `metrics.command` | array of strings |  | 4 | argv, run in the project root **without a shell**, like a card's, only while the config is trusted. 30-second timeout, 1 MB of output. Its stdout is the metrics JSON (see *Metrics*); a payload that breaks the contract shows its error in the view. |
| `metrics.refresh` | string | `"interval:900"` | 4 | When to re-run the command, as a card's `refresh`. It also runs at start and on **Refresh**. Needs `metrics.command`. Matches `^(watch:.+|interval:0*[1-9][0-9]*)$`. |
| `metrics.card` | boolean | `true` | 4 | Show the **Flow** card in the side panel while there are metrics to show; `false` keeps them in the Metrics view alone. |
<!-- config-reference end -->

### Card output

If stdout parses as JSON, it renders as JSON: an array becomes a list (for objects,
`title`/`name`/`text`/`summary`/`id` is the main line and other scalar fields are shown dimmed),
and an object becomes key/value rows. Otherwise each non-empty line is a list item, with a leading
markdown bullet (`-`, `*`, `1.`) removed. A failing command shows "cannot read: …", never an empty
card.

### Pinned cards

A card with `"pin": true` (version 3) also shows in the side panel, so where the project stands
stays on the dashboard rather than in the drawer. The pinned cards share a box of their own, a
tab per card (a tablist, as the lane's is), below the selected lane's tabs, or alone while no lane
is selected; **▾** folds the box. Each card is drawn for reading at a glance: each output line is
a title that opens to the rest of it.

- A line's title is its bold lead (`**Box cleanup** (queued …)`), else what comes before its
  first ` — ` (`2C.10 add-score-photo — …`), else its first sentence. The rest opens under it,
  one line per ` — ` part.
- A line ending in `:`, or wrapped in parentheses, is a caption, not a row.
- `**bold**` and `` `code` `` are drawn; nothing else is read as markup.
- The card shown, whether the box is folded, and which rows are open are kept per browser.

So a card that prints one item per line, with a short lead, reads best. A JSON card is drawn as
in the drawer.

A project with **no card at all** shows one line in the box's place instead of nothing (PANEL-18):
"No cards yet. Add them in .clauductor/panel.json.", with **How cards work**, a link to the
guide's [Cards](guide.md#cards) section (opened in a new tab, from the same address as the Help
dialog's guide link). A project whose cards are all unpinned shows nothing there: its cards are in
**Activity**.

### When the cards may be stale

Every card runs its command in the project's main checkout and watches its file there, so it
shows what that checkout's branch has. When the branch is behind its upstream (someone merged,
you have not pulled), the cards show old data without anything looking wrong. So while it is,
one line above the cards, in the side panel's pinned box and in **Activity**, says so (PANEL-18):

> main is 3 commits behind origin/main (as of last fetch) — cards may be stale

It comes from the dashboard's own `git status --porcelain=v2 --branch` of the main checkout (see
*What the panel reads, and when*), read like a lane's worktree while a page is in view, and only
for a project with cards. The panel adds no `git fetch` for it: behind is counted against the
local remote-tracking branch, as of the last fetch (yours, or the panel's when a lane starts or
Close lane or Remove plans), hence "as of last fetch". There is no line when the branch is up to
date or only ahead, when it has no upstream or HEAD is detached, or when git cannot be read.
Pull in the main checkout, then **Refresh** (it re-reads git and every card at once).

## The page

The page is built around lanes (PANEL-11). From the top: the status bar, the Needs-you rows (only
when something needs you), then a rail on the left (the lane table and the worktree tree) and the
workspace of the lane you selected (its terminal tabs, its header, its terminal, and a side panel
with everything about it).

It is drawn the way control rooms and trading desks are: grey at rest, and colour only when
something is abnormal. Normal work (working, idle, finished) is plain text in three tones. Amber
means it needs you, red means it failed or is stale, and the one link colour marks what you can
click or type into. Every state is a word and a small square, never a colour alone. Numbers sit in
aligned columns with their units. The one motion on the page is a brief flash on a figure whose
value just changed.

- **Status bar.** The project, **Live** or **Disconnected**, and the figures that hold across every
  lane. Totals live here and nowhere else.
  - **The project's name is the project menu** (PANEL-16). It lists every project the panel
    serves, each with its lanes (working, waiting, idle), how many need you there (amber, red when
    one blocks), how many lanes it has to restore, and whether its config is untrusted; a project
    that could not load says why. Beside the name, "N need you elsewhere" counts the other
    projects'. The keyboard works as in **Appearance** (Down or Enter opens it, the arrows move,
    Enter picks, Escape closes). Picking one switches the whole page: its lanes, its selected
    lane (kept per project), and `?p=<id>` in the address, so a bookmark opens on that project.
    Open terminals close on a switch; their lanes run on in tmux. The tab title adds ", +N
    elsewhere" while other projects need you.
  - **The quota**: one bar per window the account's plan reports (usually the **5-hour** and
    **7-day**), each with its reset countdown, and the plan named on the first ("5-hour quota
    (Max)"). On the burn window's bar a magenta mark shows where it lands at its reset at the
    current burn rate. The quota comes only with a status-line post, from any session on the
    machine, and the panel keeps the last one across a restart (`~/.clauductor/panel/quota.json`),
    so a reading older than 10 minutes says "as of … ago"; with none, hovering says where one
    comes from. What stands here depends on the account: see *The account and its quota*.
  - **Burn rate**: the shortest window's change per hour over the last 30 minutes, when there are
    at least 5 minutes of it, and when it runs out at that rate. That time turns amber when it
    comes before the reset.
  - **est. $ (list price)**: the sum of the status line's `total_cost_usd` over the sessions the
    panel tracks now (live ones, and ones heard from in the last 30 minutes), then today's cost and
    the cost per hour over the last hour, with a sparkline. It is a list-price estimate, not a bill.
    Today counts a session already running when the panel started only from that start on.
  - **Lanes**: a one-line bar of working, needing you and idle, with the counts.
  - **Needs you**: how many, and how long the oldest has waited.
  - **Gate** (the first queue): who holds it, how many wait and the oldest wait. Clicking it shows
    the lane that holds it.
  - **Claude processes**: CPU and memory of the lanes' claude processes, with a sparkline (see
    *What the panel reads, and when*).
  - **Interruptions today** (OS notifications sent) and, from 1600 px, the **Hooks** counts.
  - At the right: **New lane**, **Activity** (the drawer), **Metrics**, **Refresh**, **Appearance** and **?**
    (Help, PANEL-17): a dialog with the page's keyboard shortcuts, a few one-line how-tos and a
    link to [the guide](guide.md), which opens in a new tab. The `?` key opens it too, except in
    the terminal (where `?` is claude's) and in a text field; Escape or **Close** returns focus. **New
    lane** is disabled, with the reason on hover, while lanes cannot start (*Subscription only*).
  - The bar keeps to one line above 1180 px: when its figures would wrap, the secondary ones
    (resets, cost today and per hour, sparklines, the Hooks counts) go first, as they do from 140%.
    A quota's age stays.
- **Lost the panel.** The page hears from the panel at once when what the view says changes; the
  polls' own bookkeeping arrives with the next 5 s tick, and a heartbeat comes every 5 s. After
  three missed beats, or a dropped stream, it says so everywhere: **Disconnected** in the status
  bar, a red bar under it with the reconnect status and **Retry now**, "⚠ Disconnected" in the tab
  title, the page dimmed, headings marked "as of HH:MM", every age frozen at the last word from the
  panel, and every action that would reach the panel disabled. It reconnects on its own (a check
  that takes over 5 s counts as failed) and restores all of it. If the panel restarted, the bar
  says so: the new launch has a new token.
- **Needs you (rows under the status bar, only when something needs you).** First in the page at
  every width, across every lane, each row tinted amber (needs you) or red (blocking): sessions
  blocked on you (a permission, elicitation or input notification, or `claude agents` reporting
  them waiting), quota auto-resume warnings, and stuck or restored template lanes. A row names the
  lane, the specific ask when `claude agents` names one (`Permission: Bash(npm run test:e2e)`), and
  how long it has waited. **Open terminal** selects the lane and takes focus to its terminal's
  frame, never inside; clicking the row selects the lane. **Alerts** follow (every alert that
  belongs to no lane, and every blocking alert of any lane), then **Your move** (finished turns,
  not blocked). The tab title reads `(N) <project>` while N items need you, the favicon carries
  the count, and a new item is announced to screen readers (a polite live region).
- **Banners (under that).** Each says what it is. **No hooks**: a lane is busy per `claude agents`
  and no hook has come from it since it went busy, for 60 s; usually the session never loaded the
  hooks, so restart it. **Cannot read**: `claude agents`, `git worktree list` or the panel's tmux
  server cannot be read. **Config changed**, **Events dropped**, **Hooks** and **Lane registry**
  say what their name says. Lanes that lost their tmux session are announced once, in the
  **Restore** bar, with **Restore all**.
- **The rail: Lanes.** A two-hour timeline of every lane's state, one line each, then the lane
  table: one row per lane (a tmux lane the panel started, or a worktree with a claude session
  started elsewhere). A row needing you is tinted amber; one exited, orphaned, stale (busy with no
  hook for 60 s) or with a failure is tinted red. A status that is not a current `claude agents`
  reading is marked `≈`, with how old the last good reading is. Click a column heading to sort by
  it (again to reverse). **Columns** chooses which columns show; the default is state, time in
  state, context, cache and cost per hour, and the others are cost, model, busy ratio, lines,
  lines per hour, turns per hour, asks per hour, compactions, last failure, running agents, git,
  pull request, gate and CPU. The choice and the sort are kept per browser. The cache column
  reads "92%" (its hit ratio) while the cache is warm, "cold in 1:52" in amber in the last two
  minutes, and "cold" once it has gone cold. A table wider than the rail scrolls inside it,
  never the page, and while the rail runs past its foot a line there says "More below". A lane has one name
  everywhere: a lane with a terminal is called what you named it when you started it. Each lane the
  panel started ends its row with **⋯**, its actions (see [From the lists](#from-the-lists)).
- **The rail: Worktrees.** A tree, as the old control room's topology had it: the project, every
  worktree (its lane type, branch and path, and once read, ahead/behind and how many files
  changed), each lane's claude session (state, uptime, context), and its agents, nested by which
  agent started which, with finished ones folded under "N finished". A worktree with no lane has
  **New lane here**, which opens the Start dialog on that worktree, to start a new claude session
  there, and (except the main checkout) **Remove** ([Remove a worktree](#remove-a-worktree)); a
  lane has **⋯**, its actions. The rail's edge drags (or, focused,
  moves with ←/→; Home and End go to the limits, Escape or a double-click restores the theme's
  width, Enter hides the rail), and **Lanes** at the left of the tabs hides or shows it. The width
  and whether it shows are kept per browser; below 900 px it starts hidden.
- **Terminal tabs.** One tab per lane, with its state square and name, then **+** (the Start
  dialog). A tablist: Tab reaches the selected tab only, and ←, →, Home and End move and select at
  once. Selecting a lane (a tab, a row, a tree node, a Needs-you row) never enters its terminal.
- **The lane's header.** Its name and state, branch and worktree, model (as the status line
  reports it) with effort (its template's, else its lane type's), thinking and fast mode, uptime,
  context as a bar with a mark where Claude Code compacts on its own (95%, inferred, not
  documented), and cost with cost per hour; when the lane builds a change whose proposal has a
  budget, a **Budget** bar beside it (PANEL-19, see *Needs you from the metrics*). **Hide
  details** folds the side panel; kept.
- **The terminal.** The selected lane's live terminal, taking the space the workspace leaves.
  Under it: **Attach in Terminal.app**, **Interrupt (Esc)**, **Restart**, **Stop lane** and **Close lane**. Stop
  and restart ask in the page, in words built from the lane's state: idle gets `/exit`, busy or
  waiting gets Escape (and what that interrupts: its subagents, an open question), and whether it
  holds or waits in a queue. **Close lane** also removes the lane's worktree and branch when that
  loses nothing; its confirmation lists what goes and what stays ([Close lane](#close-lane)). An
  orphaned lane has **Resume**, **Forget** and **Close lane** instead of a terminal.
  A lane started outside the panel says it has no terminal here.
- **The side panel: a tab per family of figures.** The choice of tab is kept.
  - **Agents**: the lane's sessions (pid, state and for how long, compaction, last failure), then a
    Gantt of its agents over the lane's window (at most two hours): one row each, nested under
    the agent that started it, running ones reaching now, finished ones grey, with their duration.
  - **Figures**: time in state; context with tokens in and out and the window; the prompt cache's
    hit ratio (a sparkline), its cold-in countdown (amber under two minutes), the last miss cause
    and the tokens a cold cache would rebuild; cost and cost per hour (a sparkline); the busy ratio
    (API time over wall time); wall and API time; lines added and removed, and per hour; turns and
    asks, and per hour; compactions; the last failure; running and finished subagents; the claude
    process's CPU and memory; thinking, fast mode and output style.
  - **Git**: branch, HEAD, path, upstream with ahead and behind, changed and untracked files, the
    diff stat against HEAD, the last commit's age, and the template's first-prompt state; then the
    branch's pull request, its checks and review decision.
  - **Checks** (PANEL-20): the lane's merge readiness, see *Merge readiness*.
  - **Gate**: for each queue, whether this lane holds it, waits in it and where, or is not in it;
    the holder and the line; **Cancel wait** for this lane's own wait; **Run in `<lane>`**.
  - **Alerts**: this lane's only. **Activity**: this lane's events, newest first.
  The tabs wrap onto a second line rather than scroll. Below them, the pinned cards' own box
  (*Pinned cards*); with no lane selected, it is the side panel. Under that, the **Flow (30d)**
  card (PANEL-19): median cycle time, merges a week, change-fail rate and spend a week, each with
  its sparkline, "—" (and why, on hover) where there is none. It shows once any of the four has a
  value, whether the project's command or the panel gave it, unless `metrics.card` is `false`;
  the whole card is one button that opens **Metrics** on Flow at 30d. **Hide details** folds all of them.
  The side panel's left edge drags like the rail's (or, focused, ← widens and → narrows it; Home
  and End go to the limits, Escape or a double-click restores the theme's width), up to half
  the window; the width is kept per browser.
  Below 1180 px the side panel moves under the terminal.
- **Activity (the drawer).** Every queue, the open pull requests (from `gh`, "cannot read" on
  failure, never an empty list), the project's cards, and every lane's last events, grouped by
  lane. Escape or Close closes it and returns focus.
- **Metrics (PANEL-19).** A wider drawer with four tabs, **Flow** (DORA and flow: cycle, lead
  and approval time, merge frequency, change-fail rate, aging work in progress), **Cost** (spend,
  per week, and by role, model, change and project), **Quality** (review rounds, the reviewer's
  eval recall by model, escaped defects) and **Outcomes** (each change's hypothesis, when it is
  due, whether it was checked; an unchecked one past its date is amber). **7d**, **30d** and
  **90d** pick the range; **This project** or **All projects** the scope. The tabs are a tablist
  (←, →, Home, End), the range and scope are pressed buttons, and the choices are kept per
  browser. Each figure is its value with its unit, its series as columns drawn in the page (no
  chart library; an empty bucket is a mark on the baseline, and the series is read out to a
  screen reader), how many items it summarises, and **project** or **built in**; a figure with
  none is "—" and the reason under it. A change over its budget is amber in **By change**. The
  view is fetched when it opens and every minute while it stays open; see [Metrics](#metrics).
- **The keyboard and the terminal.** Nothing moves focus into a terminal by itself: not loading
  the page, not picking a lane, not **Open terminal** (which takes focus to the terminal's frame).
  The terminal is one stop in the Tab order; **Enter** there, or a click, enters it. Inside, every
  key is claude's, Tab, Shift+Tab and Escape included, but the page's size keys (Ctrl+Alt+=, −,
  0), which resize the page and never reach claude. **Ctrl+]**
  leaves, back to the lane's tab; a line above the terminal says so while you are in it. A double
  Escape does not leave, because claude uses Esc Esc itself (to go back to an earlier message).
  The cursor does not blink.
- **The mouse and the terminal.** The wheel scrolls the lane's history: the panel's tmux has
  `mouse on`, so the first wheel-up enters tmux's copy mode, and the line above the terminal says
  "Scrolled back". It ends when you scroll back to the bottom, or with the first key that is not a
  scroll key (arrows, Page Up/Down, Home, End): that key leaves copy mode and then reaches claude,
  so nothing you type is lost. Escape only leaves, since claude would read it as an interrupt. The
  panel asks tmux about copy mode only after a wheel, never per keystroke. tmux binds no clicks
  here, and an unbound click would go to claude if it asked for the mouse (its fullscreen TUI,
  `"tui": "fullscreen"`, does), so the page keeps presses for itself: a plain drag selects text
  in the browser (the page turns a plain press into xterm's Option-press), and the selection
  stays until you copy it with ⌘C or start another. It used to vanish as soon as the pointer
  moved: claude's fullscreen TUI asks for every mouse motion (mode 1003), tmux relays that to
  the page, and xterm.js clears its selection whenever it sends a mouse report, as it does for
  a key. The page declines that one request; the wheel and the other modes still work, and
  claude gets no hover reports (nothing here used them). tmux's status bar is off; the tab
  names the lane.
- **Links in the terminal.** ⌘-click a URL (Ctrl-click off a Mac) to open it in a new tab, as in
  Ghostty and iTerm2. A plain click stays a selection, so a drag or a double-click on a URL
  selects it and never opens it by accident. Hovering over a link underlines it and names its
  target. Both kinds are links: plain-text `http(s)` URLs, which the page finds itself (a URL
  that wraps onto the next row is one link), and OSC 8 hyperlinks, whose visible text can be
  anything. claude prints URLs as OSC 8 inside tmux 3.4 or later, and its fullscreen TUI breaks
  long ones across rows itself, so OSC 8 is what keeps those whole. A link whose text is its
  own address opens directly. One whose text says something else (a link reading "PR 12", or
  one row of a URL broken across rows) shows its real address above the terminal with **Open
  link** and **Cancel** first. Only `http` and `https` ever open; the tab opens with
  `noopener` and no referrer. A ⌘-press never reaches claude as a click. The browser test
  `testdata/browser/terminal-links-selection.cjs` checks the links and the selection through a
  real panel and tmux, with a lane that asks for the mouse as claude's fullscreen TUI does.
- **Images in the terminal.** Drop an image file on a lane's terminal, or paste one (⌘V with an
  image on the clipboard), and claude gets it as it would from a native terminal: its path is
  typed at the cursor, as a bracketed paste followed by a space, and never Enter, so you go on
  typing the prompt around it. A browser never tells a page where a file lives, so the page
  sends the image to the panel, which keeps it in its own state directory (see [Security
  model](#security-model)) and types that path. The terminal is outlined while an image is
  dragged over it. PNG, JPEG, GIF and WebP only, up to 20 MB; what is refused says why under
  the terminal.
- **Warnings** (the amber bars: an unverified Claude Code, ignored events) close with their **×**.
  A closed warning stays closed in this browser while it is about the same thing, even as its
  text changes ("2 of 3 confirmed"); a new Claude Code version, or a break, shows again.
- **Footer.** One line: hook events, status posts, **other projects** (events from sessions
  in no registered project: the hooks are the machine's, so these are expected and set aside), what
  was really **dropped** (overflow, malformed, unknown event; amber when any), and notifications.
  **All counters** opens the rest (the choice is remembered): drops by cause (overflow, malformed,
  unknown event name), unknown notification types, the last, mean and worst `claude agents` poll
  latency and its current interval, the filter in use and why, the Claude Code version against
  the one the heuristics were verified on, and notifications sent or failed.

### Which agent started which

A `SubagentStart` hook names only the new agent, never the one that started it (verified on Claude
Code 2.1.284). The call that starts it does: a `PreToolUse` for the Agent (or Task) tool fired
inside agent A carries A's `agent_id` (on the main thread it has none), and the new agent's
`SubagentStart` follows it within tens of milliseconds. That call's `PostToolUse` names both: the
caller's `agent_id` and the new agent's `tool_response.agentId`.

So a starting subagent is placed under the caller of the oldest Agent call in its session, made in
the last 2 s, that asked for the same subagent type, and marked `≈` until the call's
`PostToolUse` confirms or corrects it; that pair is exact. It arrives at once for a background
agent, and when the agent finishes for a foreground one. Workflow agents start as
`workflow-subagent` with nothing naming their run; the run's id and name come in the Workflow
call's `PostToolUse`. They are grouped under their session's newest run, always marked `≈`,
because two runs overlapping in one session cannot be told apart. Subagents nest at most three
levels below the session. The panel reads only the Agent call's `subagent_type` and
`description`, and the response's `agentId`, `runId` and `workflowName`; the prompt and every
other field are never decoded, and no transcript is read (not even its path).

- Workflow agents stop under a different `agent_id` and `agent_type` (`workflow-subagent`) than
  they started with in some versions, so a `workflow-subagent` stop with an unknown id retires the
  oldest running agent of any type. Any other typed stop with an unknown id retires the oldest
  agent of its own type. An unknown id with an empty type is an internal agent that never sent a
  start, and retires nothing. A session that `claude agents` reports idle for 10 s, or gone, has
  its running list cleared. The hook `Stop` clears nothing, because background agents outlive the
  turn. A retired agent moves to the lane's finished list (the last 20 per session).

### What the panel reads, and when

Every figure on the page comes from what the panel already reads: the status-line posts, hooks,
`claude agents`, `git worktree list`, `gh pr list` (now also asked for the review decision) and
the gate's lease on disk. Nothing costs a model token and nothing reads a transcript. Two reads
were added for the dashboard, and both run only while a page is in view: an open page that is
visible says so once a minute (`POST /api/seen`), and the reads stop 90 s after the last word.

- **ps**, every 10 s: one `ps -o pid=,pcpu=,rss=` for the claude processes `claude agents` names.
  CPU is ps's figure (the process's average since it started, on macOS and Linux alike).
- **git**, every 30 s (and on **Refresh**), for each worktree a lane runs in and, for a project
  with cards, the main checkout the cards run in: one `git status --porcelain=v2 --branch`,
  plus `git diff HEAD --shortstat` only when the tree has changes, and `git log -1` only when
  HEAD moved. No `git fetch`: ahead and behind are as of the last fetch.

The trends (quota, cost, CPU and memory, each lane's cache hit ratio and cost) are sampled once a
minute and kept for two hours, and each lane's state timeline is extended every 5 s; neither
spawns anything. With no page open the panel spawns exactly what it did before PANEL-11.

### The account and its quota

The quota is the account's, not the project's: any session's status-line post moves it, whatever
project it runs in (PANEL-15; before, a post from another project was set aside with the quota in
it). Nothing in it is specific to a plan. The status line's `rate_limits` holds whatever windows
the account's plan has, each a percentage of the plan's own limit, and the panel shows each one it
receives:

- `five_hour`, `seven_day`, `seven_day_opus` and `seven_day_sonnet` are labelled "5-hour",
  "7-day", "7-day Opus" and "7-day Sonnet". A key shaped like them reads the same way
  (`two_hour` is "2-hour"); any other is labelled from its words (`nimbus_quill` is "Nimbus
  quill"). The bars go shortest window first, and a window whose span cannot be read goes last.
- The burn rate and its projection follow the shortest window that has a number. When the
  shortest window changes, the rate starts again.
- A post carrying some windows keeps the others' last values.
- `/api/state` carries `quota.windows` (`key`, `label`, `pct`, `resetsAt`, `expired`). The fields
  before PANEL-15 (`fiveHour`, `sevenDay` and their resets) stay for one release, so a page left
  open across an upgrade still draws.
- The quota alert and the quota guard still read the 5-hour window, as their `five_hour_pct`
  keys say. A plan with no 5-hour window never trips either.

To know what kind of account it is, the panel runs `claude auth status --json` at start and
every 10 minutes (it costs no token), through `/usr/bin/env -u ANTHROPIC_API_KEY -u
ANTHROPIC_AUTH_TOKEN …`, the environment a lane gets, so it names the login lanes use. It keeps
only `loggedIn`, `authMethod`, `apiProvider` and `subscriptionType`. The command also prints your
email, your organisation's name and its id: the first two are never decoded, and the id is kept
only as a short one-way hash, stored with the quota in `quota.json` so a reading saved for one
account is dropped once the panel reads another. What stands in the quota's place:

| Account | `claude auth status` | Shows |
|---|---|---|
| A subscription (Pro, Max, Team, Enterprise) whose status line has windows | `authMethod` `claude.ai` (or `oauth_token`) | the bars, the plan on the first ("(Max)") |
| A subscription that reports no windows | the same, and 3 status posts in a row with no `rate_limits` | "Quota: none reported", and no burn rate |
| An API key, or a cloud provider (Bedrock, Vertex, Foundry, a gateway) | `authMethod` `api_key` or `api_key_helper`, or an `apiProvider` other than `firstParty` | no quota: **API spend (est.)** comes first, and **Spend rate** is its dollars per hour |
| Not read yet, not logged in, or a method the panel does not know | | the 5-hour and 7-day fields with no reading, and where one comes from |

Lanes still start only on a subscription login (*Subscription only*).

## Lanes

A **lane** is one interactive `claude` in its own tmux session, on the panel's own tmux server
(`tmux -L clauductor`, `clauductor-<id>`, or `tmux_socket`: one per project, see [Projects](#projects)). tmux owns the process, not the panel, so:

- closing the browser, or restarting or upgrading the panel, leaves every lane running;
- several viewers can share a lane: browser tabs, and a Terminal.app window.

Lanes are interactive sessions, never `claude --bg`: at a usage limit an interactive session
pauses, while a workflow in a background session fails.

### Starting a lane

**New lane** asks for a template (or none), a lane type (from `lanes` and `lane_types`), a lane
name, and where it runs. Under each choice a line says what it is: a **template** is a recipe for
one kind of work (it sets the lane type, names the branch and types a first prompt); a **lane
type** is only how claude runs (its branch prefix, model and effort). **New lane here** on a
worktree picks that worktree's own lane type (`main` → `orchestrator`). When a template has a
`suggest` command, **Up next** comes first (see *Suggestions*). Where it runs:

- **New branch and worktree.** The server runs `git fetch`, then
  `git worktree add -b <prefix><name> <worktree_dir>/<name> <base>`. The prefix is the type's
  prefix rule in `lanes` (`fix/` for `fix`). A type with only exact rules (such as `main` →
  `orchestrator`) has no prefix and cannot start a new branch.
- **An existing worktree.** Any worktree that `git worktree list` reports.
- **The project root.** Use this for the orchestrator.

A lane is refused in a directory where another lane is already running, registered or not: two
sessions in one checkout would edit the same files.

Then it runs, as an argv list with no shell:

```
tmux -L <socket> -f /dev/null new-session -d -s <name> -c <dir> -x 200 -y 50 \
     -e PATH=… -e HOME=… -e LANG=… \
     /usr/bin/env -u ANTHROPIC_API_KEY … claude [--model m] [--effort e] -n <name> --session-id <uuid>
```

- First, `tmux -L <socket> -f /dev/null start-server ; set-option -g exit-empty off`, run in
  your home directory, so the server (which keeps its starting command line for life) names no
  worktree; `exit-empty` goes back `on` right after the `new-session`, so the server still ends
  with its last lane (see [Never kill the panel's tmux server from a
  script](#never-kill-the-panels-tmux-server-from-a-script)).
- The lane name is the tmux session name and the worktree directory name. It must match
  `[a-z0-9][a-z0-9-]{0,40}`, so it is safe in a tmux target and in a shell command.
- The session id is a UUID that the panel generates, so the lane is bound to its Claude session
  from its first second. The panel never discovers sessions by directory.
- `-e` gives the session an explicit PATH (the directories of `claude`, `tmux`, `git`, `gh` and
  `node`, then the panel's own PATH), HOME and LANG. It does not depend on whichever process
  happened to start the tmux server.
- `/usr/bin/env -u` removes the API-key variables. It also removes the variables a parent Claude
  session sets for its children, so a panel started from inside Claude cannot make its lanes look
  nested.
- `remain-on-exit` keeps a lane's last screen after `claude` exits. A lane that dies at start
  shows why, instead of vanishing. The tab's dot turns red.

**A new directory shows Claude's workspace-trust dialog.** In Claude Code 2.1.284 it defaults
to **No, exit**. Press ↓, then Enter, in the lane's terminal. If you press Enter first, claude
exits and the lane shows a dead pane; STOP it and start it again.

### Merge readiness

PANEL-20. A lane's **Checks** tab says in one line whether its branch is ready to merge, and
every reason it is not ("Not ready: 1 check(s) failed; 1 unresolved review thread(s); no gate
receipt for HEAD"), then a line per check. It is read-only: the panel never merges.

| Check | From | Not ready when |
|---|---|---|
| Pull request | `gh pr list` (already polled) | none is open for the branch, or it is a draft |
| Checks | its status checks | one failed or is pending |
| Review | its review decision | changes requested, or a review required |
| Review threads | `gh api graphql` (`reviewThreads`, the first 100), at most every 2 min per pull request | one is unresolved |
| Tasks | the change's `tasks.md` in the lane's own worktree (the branch's last segment names the change) | a `- [ ]` box is unticked |
| Gate receipt | `<the worktree's git dir>/ci-receipt` (OPS-7's `run-local.sh`: `<sha> TAB full TAB clean\|dirty TAB all`) | it is for another commit than HEAD, or for a dirty tree; with none, only when the project keeps receipts (it has `scripts/ci/run-local.sh` or a queue) |

A check the panel cannot make yet (nothing read, or gh failed) is shown as such, in the dim tier,
and counts as not ready. The reads run with the dashboard's, only while a page is in view. A lane
on the project root or the base branch has nothing to merge, and says so.

### Worktree setup, teardown and ports

PANEL-20 (config version 5). A worktree is a fresh checkout, so a new lane may need its
gitignored files, its dependencies and a port of its own before claude starts in it.

- **`.worktreeinclude`** in the project root, as Claude Code reads it: `.gitignore` syntax, and a
  file is copied from the project root into a lane's **new** worktree only when it matches a
  pattern **and** is ignored, so a tracked file is never copied. git applies both sets of
  patterns (two `git ls-files --others --ignored` lists, one with `--exclude-standard`, one
  with `--exclude-from=.worktreeinclude`; the copy is what both list). Only regular files are
  copied (never a symlink), nothing already in the worktree is overwritten, and at most 2,000
  files or 200 MB; the start says how many it copied.
- **`worktree_setup.command`** runs in the new worktree after that and before claude starts;
  **`worktree_teardown.command`** runs in a lane's worktree when **Close lane** is about to
  remove it. Both are argv run without a shell, with `CLAUDUCTOR_LANE` and `CLAUDUCTOR_PORT` set
  (through `/usr/bin/env`), a 5-minute timeout, and only while the config is trusted: `trust`
  and `install` print them. A failed setup is a note on the start, and the lane starts anyway.
  A failed teardown, or one that leaves the worktree changed (the clean check runs again after
  it), keeps the worktree, and Close says why; the confirmation says the teardown runs first.
  A lane on an existing worktree or the project root runs neither. The lane lock is held
  while they run, so a slow setup delays the other lanes' actions.
- **`ports: {base, per_lane}`** gives each lane a port of its own: `base`, `base + per_lane`,
  and so on, the lowest one no other registered lane holds. It is kept in the lane's registry
  record (so a restart or a restore keeps it, and Forget frees it), exported to the lane's tmux
  session as `CLAUDUCTOR_PORT`, and shown as **Port** in the lane's header. The panel does not
  check that nothing else listens there.

### Lane templates

**New lane** offers the config's `templates`. Pick one, give the lane a name (and an issue if the
template uses `{issue}`), and START:

1. The server validates every value: the name is a lane id; the issue is one line of plain text
   (no newline, no control character, no invisible format character such as a bidi override, at
   most 200 characters), and must also make a valid branch name if the pattern uses it. The
   dialog previews the branch and the exact prompt.
2. The lane starts as any new-branch lane does, with the template's model and effort. The
   rendered prompt is written into the lane registry with state `pending` **before** the lane
   starts.
3. The panel types the prompt **once claude is ready**, and it decides "ready" from structured
   signals only: `claude agents --json` lists the lane's own session id as `idle` with no
   `waitingFor`, no hook says the session waits (a permission prompt, an elicitation), and that
   poll succeeded within the last two poll intervals. Right before the first keystroke, under
   the lane lock, it reads `claude agents` once more and checks again. It never scrapes the
   screen. A session held at the workspace-trust dialog is not listed at all (verified on
   2.1.284), so the prompt can never land in that dialog. The text goes first
   (`send-keys -l -- <text>`, so text starting with `-` stays text), then Enter as a separate
   write, 400 ms later.
4. The state goes `pending` → `typing` → `sent` → `delivered`. `typing` is written before the
   first keystroke, so a panel that dies mid-typing never types the prompt twice. `delivered`
   needs proof: the session's `UserPromptSubmit` hook, or `claude agents` showing it busy.

"Needs you" says so when it is stuck: claude not listed as idle after 15 s ("answer any dialog in
its terminal"), claude exited, the panel stopped mid-typing, or the prompt was typed but not
submitted after 30 s. If you type into the lane first, the prompt is skipped. While it waits on
claude, the panel asks `claude agents` for a fresh reading at most every 2 s. A pending lane whose
tmux session is gone (a reboot, a killed tmux server) waits 30 s, then shows "RESTORE the lane"
and stops asking: no poll can bring it back. Once restored, the prompt is typed as usual.

### Suggestions

A template's `suggest` (version 3) is a command that says what the template could start next:
the project's roadmap, its issue list, whatever it keeps. **New lane** lists every suggesting
template's rows under **Up next**; picking one selects the template and fills in the lane name
(and the issue), so the next piece of work is a click away rather than a trip to the roadmap.

```json
{ "id": "propose", "title": "Propose a roadmap row", "lane_type": "build",
  "branch_pattern": "change/{name}", "first_prompt": "/openspec-propose {name}",
  "suggest": { "command": ["node", "scripts/next.mjs", "propose"], "refresh": "watch:docs/roadmap.md" } }
```

- It runs like a card: argv with no shell, in the project root, 30 s timeout, on its `refresh`
  rule, at start and on **Refresh**, and only while the config is trusted (`panel trust` lists
  it). A failed run keeps the last list and says why.
- Its stdout is JSON, an array of `{"name", "title", "detail", "issue"}` objects (only `name` is
  required) or of names; or text, one row a line, the first word the name and the rest its title
  (`#` starts a comment). A row's `name` must be a lane name (`[a-z0-9][a-z0-9-]{0,40}`); a row
  without one is skipped, and the dialog says how many were. At most 50 rows; every text is one
  line of plain text, cut to length.
- The rows are the command's word, and are only offered: nothing starts until you press **Start
  lane**.
- A template that takes `{issue}` can list the open issues: a row's `issue` (`"#412"`) fills in
  the dialog's Issue, and its name can carry the number (`412-fix-the-week-row`). A script around
  `gh issue list --json number,title,labels` does it; exit non-zero when `gh` fails, so the dialog
  says "cannot read" and keeps the last list rather than showing none.

### The lane registry

`~/.clauductor/panel/<hash of the project path>/lanes.json` (0600, in a 0700 directory) records
each lane: its id, session id, directory, type, branch, and its last action. The intent is
written **before** each action and marked done after it, so a panel that crashes mid-action
finds the half-done action when it restarts. The file is written atomically.

The registry is never trusted on its own. Every 2 s while lanes run (every 10 s with none, and at
once after a lane action) the panel compares it with the tmux socket,
`claude agents --json` (matched by session id) and the worktree list, and it re-reads the file
every 30 s. Anything that does not add up is shown as an **orphan**, never hidden:

| What | Shown as | What you can do |
|---|---|---|
| registered, tmux session gone (a reboot, or tmux ended) | orphaned | **Resume**, **Forget**, or **Close lane** |
| registered, the panel stopped during an action | orphaned, with the action | **Resume**, or **Forget** |
| a tmux session on the socket that the registry does not know | running, "not in the lane registry" | terminal and **Stop lane** only; without a session id it cannot be restarted |
| registered, its directory no longer a worktree | the reason is added | **Forget** |
| a record that fails validation on load (session id not a UUID, relative path, unknown mode…) | "corrupt registry record" | **Stop lane**, **Forget**; it is never launched |
| a record with an invalid lane id | a banner | edit or delete the file |

### Controls

| Button | What it does |
|---|---|
| **Interrupt (Esc)** | `tmux send-keys Escape`, which is claude's interrupt. |
| **Stop lane** | If `claude agents` reports the lane's session **idle**, sends `C-u` (clearing any unsent text), types `/exit`, checks that the session is **still** idle, then presses Enter as a separate write and waits up to 10 s. If it stopped being idle, it presses Escape instead. In any other case (busy, waiting on a permission or dialog, or unknown), it presses **Escape only**, never Enter: an Enter would confirm whatever default the dialog has focused. Then `kill-session`. The lane leaves the registry, and its dropped images go. **The worktree is never removed**; **Close lane** is the control that removes it. |
| **Restart** | Stops the lane, then starts its **own** session again in the same directory: `claude --resume <session id>`. If the session never had a prompt, it uses `--session-id <same id>` instead, because `--resume` refuses an empty session. The panel marks a session as having a conversation when a `UserPromptSubmit` or `Stop` hook arrives from it, or when `claude agents` shows it busy. Hooks can be dropped, so the mark can be wrong. If claude then exits non-zero within 3 s, the panel retries once with the other flag. It judges by the exit status alone and never reads the screen. In Claude Code 2.1.284, both wrong flags exit 1 at once. If both attempts fail, the dead pane shows claude's message. It **never** uses `--continue`, which picks the directory's most recent conversation, whoever's it is. |
| **Resume** (orphans) | The same resume, for a lane whose tmux session is gone. It is refused while `claude agents` shows another process on that session id, or cannot be read. Two processes on one session would interleave its transcript. |
| **Forget** (orphans) | Drops the registry record. The worktree and the conversation stay. |
| **Close lane** (running lanes and orphans) | **Stop lane** exactly as above (for an orphan, **Forget**), then removes the lane's worktree and branch **when that loses nothing**. See [Close lane](#close-lane). |
| **Remove** (a worktree with no lane, in the tree) | Close lane's cleanup without a lane: removes the worktree, and its branch when merged, **when that loses nothing** and no lane or claude session is in it. See [Remove a worktree](#remove-a-worktree). |
| **New lane here** (a worktree with no lane, in the tree) | Opens the Start dialog on that worktree: a new lane, a new claude session there. |
| **Attach in Terminal.app** | Runs `osascript` to open a Terminal window with `exec tmux -u -L <socket> attach-session -t =<name>`. The command reaches AppleScript as an argument and is never spliced into the script, and every part of it is single-quoted. The first time, macOS asks whether the panel may control Terminal. |

Text that the panel types into a lane (`/exit`) goes as the text first, then Enter 400 ms later.
Sent together, a long line can sit in claude's input box unsubmitted.

#### From the lists

Every lane the panel started (a lane with a terminal, running or orphaned) also has a **⋯**
button (PANEL-18) at the end of its row in the **Lanes** table and beside its node in the
**Worktrees** tree, so a lane can be stopped without selecting it first. Its menu has **Interrupt
(Esc)**, **Restart** (registered lanes), **Stop lane** and **Close lane** for a running lane, and
**Resume**, **Forget** and **Close lane** for an orphan. Picking one selects the lane and opens the
same confirmation under its terminal that the button there opens, in words built from the lane's
state at that moment; nothing is sent to the panel until **Confirm …** is pressed, and **Cancel**
sends nothing. From the menu, **Interrupt** and **Resume** ask too, although their buttons under
the terminal act at once: one stray click in a list must never act. A lane started outside the
panel has no **⋯**: the panel has no terminal on it to stop.

The **⋯** is a menu button in the WAI-ARIA pattern the project and Appearance menus follow:
Enter, Space or ↓ opens it on its first item and ↑ on its last; ↑/↓/Home/End move; Enter or Space
picks; Escape closes it back to its button; Tab or a click elsewhere closes it. A click on it
neither selects its row nor reaches the row: the row still selects on its own click, Enter or Space.

### Close lane

**Close lane** (PANEL-17) is for a lane whose work is done: it stops the lane, then cleans up
after it. Before it asks, the panel reads git's own state and the confirmation lists exactly what
will be removed and what will be kept, and why. Confirming sends back what the confirmation
offered to remove; the panel checks everything again once claude has exited (exiting can write
files) and removes no more than that. The result (removed, kept, and why) shows above the lanes.

In order:

1. **The lane stops** exactly as **Stop lane** stops it (idle gets `/exit`, anything else Escape,
   then `kill-session`), and leaves the registry. An orphan is forgotten instead. Its dropped
   images go.
2. **The worktree** is removed with `git worktree remove` (never `--force`), and only if every
   one of these holds. Otherwise it stays, and the page says which failed:
   - it is in `git worktree list`, and it is **not the main worktree**;
   - it is **inside the project's `worktree_dir`**: the panel removes only what it would create;
   - it is **not locked** (`git worktree lock`);
   - no other lane runs in it;
   - it is **clean**: `git status --porcelain --untracked-files=all` prints nothing. An untracked
     file counts; an ignored one does not.
3. **The local branch** is deleted only after its worktree is gone, and only if nothing is lost:
   - its tip is in the configured `base` (`git for-each-ref --merged=<base>`); the confirmation
     runs `git fetch` first, and a failed fetch is shown and only makes the panel keep more; or
   - `gh pr list --head <branch> --state merged` has a pull request whose head is the branch's
     tip now. A squash merge puts none of the branch's commits in the base, so git alone would
     keep it; a merged pull request with commits on the branch since then keeps it.

   It is deleted with `git update-ref -d refs/heads/<branch> <the tip it checked>`, which does
   nothing if the branch moved in the meantime; its `branch.<name>` settings go with it. A
   branch that is not merged stays, and the page says so.

The conversation is never removed: `claude --resume <session id>` still opens it.

### Close a lane when its PR merges

PANEL-20, opt-in: `"lanes_auto_close": "on_merge"` (config version 5), or per lane type
`lane_types.<type>.auto_close`. The panel then closes a registered lane on a branch of its own
once the branch's pull request merges, **exactly as Close lane would**: it asks Close for its
plan (with the fetch the page's confirmation does), and acts only when that plan removes both
the worktree (clean: no change, no untracked file) and the branch (merged: every commit in the
base, or a merged pull request whose head is the branch's tip), and claude is idle, exited or
gone (a current `claude agents` reading; an approximate one does not count). Nothing is forced.
The close goes in the lane's **Activity** ("Lane closed: PR #12 merged; …").

Otherwise the lane stays and **Needs you** asks **PR merged: close lane?**, with why (claude is
working; the worktree has 2 uncommitted files; the branch has commits since the merge): **Close
lane** in its **⋯** menu shows the plan and asks first. While it asks, the panel looks again
every 5 minutes and closes it once nothing holds it back. Each close and each ask raises one OS
notification (once per lane and pull request for the panel's run, when `alerts.notify` is on).

When it looks: when the panel first sees the lane, when the branch's pull request leaves the
open list the panel already polls, and while the lane asks; each look is one `gh pr list --head
<branch> --state merged`. The mode is the config's, so an untrusted config closes nothing.

### Remove a worktree

A worktree with no lane, such as a clean detached worktree a closed session left behind, has
**Remove** under it in the **Worktrees** tree, beside **New lane here** (PANEL-18). It is Close
lane's cleanup without a lane to stop, with the same rules, the same plan first and the same
check again when it acts: the confirmation, under the worktree, lists what **Removes** and what
**Keeps**, and why, after a `git fetch`; **Confirm remove** sends back only what it offered, and
the result shows above the lanes. The main checkout has no **Remove**.

The worktree is removed with `git worktree remove` (never `--force`) only if everything Close
lane checks holds (listed in `git worktree list`, not the main worktree, inside `worktree_dir`,
not locked, clean with untracked files counted), and also:

- **no lane is registered in it**, running or orphaned: that lane's **Close lane** is the control
  for it;
- **no claude session runs in it**: no entry of `claude agents --json` has its `cwd` in this
  worktree (the deepest worktree containing the `cwd`, as the panel matches sessions everywhere),
  which covers a session started in a terminal of your own. If `claude agents` cannot be read, it
  stays: such a session could not be ruled out.

Its local branch then goes only under Close lane's rules (merged into `base`, or the head of a
merged pull request, deleted at the tip checked). A detached worktree has no branch, and the plan
says so: "no branch: the worktree is detached (HEAD at …), so there is no branch to delete".


### Window size: the latest client wins

Each browser viewer gets its own PTY and its own tmux client. The lane's window uses tmux's
`window-size latest`, set explicitly on each lane because `~/.tmux.conf` might change it. The
window takes the size of whichever client last typed or resized. When you type in the browser,
the lane fits the browser. When you type in Terminal.app, it fits that window, and the browser
shows the same screen, clipped or padded, until you type there again.

We chose this over the alternatives:

- `attach -f ignore-size` for the browser would leave a browser-only lane stuck at the detached
  size.
- A grouped session per viewer would share one window size anyway, and add sessions to clean up.

### Subscription only

The panel refuses to start, restart or resume a lane while `ANTHROPIC_API_KEY` or
`ANTHROPIC_AUTH_TOKEN` is set, in the panel's own environment or in the tmux server's global
environment (`tmux -L <socket> show-environment -g`), which every lane inherits. Either key
outranks the subscription login. The page shows the reason and disables **New lane**; it re-reads
the tmux environment when the lane set changes and every 30 s, and every start reads it again. If
the tmux environment cannot be read, the panel refuses too: unknown is not "no key". As a second
layer, the lane command unsets both variables.

### Quota guard

At or above `quota_guard.five_hour_pct`, **New lane** and **Restore all** refuse, and the dialog
offers an override checkbox. An expired window (past its `resets_at`) or an unknown one never
blocks: the guard acts only on a number it has.

### Restore after a reboot

tmux lanes do not survive a reboot. When the panel starts, every registered lane whose tmux
session is gone is **restorable**, and a banner offers **Restore all** (each lane also keeps its
own **Resume**). A restore:

- runs `claude --resume <the lane's own session id>` in its worktree, or `--session-id <id>` for
  a session that never had a prompt. It **never** uses `--continue`;
- never resumes a session twice. A lane is skipped if `claude agents` shows its session id in a
  running process, if a second lane carries the same session id, or if `claude agents` cannot be
  read (then nothing is restored). Its worktree must still exist;
- types nothing into the lane.

**The resume dialog.** A session that was idle for more than an hour and holds more than 100k
tokens makes claude ask, before the first message, whether to resume from a summary. The panel
does not answer it for you. Every lane restored on a conversation shows in **Needs you**
("Restored lane") until you prompt it or it goes busy. Open its terminal, answer the dialog if it
is there, and continue.

## Queue and the gate lock protocol

Two lanes that both run a full gate that binds a fixed port (a dev server, a test database)
collide. A queue serialises them. The lease lives entirely on disk, so it survives a panel
restart, and it works when the panel is not running at all: the gate script takes it itself,
through `clauductor lock-run`.

macOS has no `flock(1)`, so the lock is a **directory**, because `mkdir` is atomic everywhere.
The protocol is plain files, so a shell script can honour it with no clauductor at all
([below](#the-protocol-in-plain-shell)), and a [conformance suite](#the-conformance-suite) checks
any implementation.

### The lease on disk

| Path | Meaning |
|---|---|
| `<lock>/` | Held while it exists. `mkdir` either creates it or fails with `EEXIST`. |
| `<lock>/owner.json` | The holder: `{v, nonce, pid, pstart, child_pid, child_pstart, host, lane, cmd, started, renewed, ttl}`, written atomically (temp file + `mv`). |
| `<lock>.waiters/<arrival>-<nonce>.json` | One per waiter, same fields. The numeric arrival (unix ns, or unix s followed by nine zeros) orders the queue. |
| `<lock>.waiters/<nonce>.cancel` | Asks that waiter to give up (the panel's **Cancel wait**). |
| `<lock>.reclaim/` | A short mutex, taken only to remove a stale holder. |

`pstart` is the holder's process start time exactly as `LC_ALL=C ps -o lstart= -p <pid>`
prints it, with runs of whitespace collapsed to one space (for example
`Mon Sep 28 23:10:17 2026`), or `proc:<field 22 of /proc/<pid>/stat>` where there is no `ps`.
It is what tells a live holder from a reused pid. `child_pid` and `child_pstart` name the
holder's command the same way. The panel's read-only queue view runs `ps` once per process, and
again every 30 s: a start time never changes, and a pid is reused only after its process is gone,
so while `kill -0` answers, the pid is almost surely the process it read; the 30 s re-read covers
a reuse between two checks. `lock-run`'s waiters, which reclaim, read it every time.

### When a holder is stale

Only a stale holder may be removed.

- **Same host, pid gone** (`ps -p <pid>` finds nothing): stale.
- **Same host, pid alive, start time equals `pstart`: never stale**, however long it has been
  silent. A holder stopped with `SIGSTOP`, or on a laptop that slept, is still the holder.
  Expiring it would run two gates at once.
- **Same host, pid alive, another start time:** the pid was reused. Stale.
- **Same host, pid alive, start time unverifiable** (none recorded, none readable, or one from
  `ps` and one from `/proc`): live. Missing data never removes somebody else's lease or waiter
  file. A lease stuck this way (a reused pid, no start time) is removed by hand (see
  [Troubleshooting](#a-lease-never-frees)).
- **The command counts too.** `lock-run` records its command as `child_pid` / `child_pstart`.
  On the same host a record is stale only when the holder **and** the command are both dead. A
  `lock-run` killed with `SIGKILL` leaves its gate running, and the gate still holds the lease.
- **Another host, or no `pid`:** its processes mean nothing here, so the TTL applies: stale once
  `renewed + ttl` has passed. `lock-run` renews `renewed` every ttl/3 (default ttl 10 min). A
  shell holder writes `ttl: 0` (no expiry), and `ttl: 0` never expires.
- **Where there is no `ps`,** liveness is `kill -0` (an `EPERM` answer still means alive) and the
  start time is `proc:` + field 22 of `/proc/<pid>/stat`.
- **A record is valid** when it is one flat JSON object whose values are strings, integers,
  `true`, `false` or `null` (nothing nested, no fraction or exponent); `v`, `pid`, `child_pid`,
  `started`, `renewed` and `ttl` are integers; `nonce`, `pstart`, `child_pstart`, `host`, `lane`
  and `cmd` are strings; and `nonce` is 16 lower-case hex digits. Go (`checkRecord`) and
  `lease.sh` (`lease_valid`) apply exactly this, whitespace and newlines allowed. A lock
  directory whose `owner.json` is missing or invalid is stale once the directory is 10 s old
  (its holder died between `mkdir` and a complete write); until then its holder is starting,
  and waiters wait. Never write `owner.json` in place; write a temp file and `mv` it.

**Removing a stale holder.** Only the first live waiter does it, under the reclaim mutex, after
re-reading `owner.json` and checking it is still the same stale holder (same nonce). **Nothing
ever signals or removes a live holder**, including the panel.

**FIFO.** Only the first live waiter tries `mkdir`. A waiter is dead, and its file removed, by
the same rule as a holder, except that its TTL is 60 s whatever its file says. A waiter file
that is not a valid record is not a waiter: it holds no place in the queue, and nothing removes
it.

### `lock-run`

- **Exit status.** The command's status (128+n if a signal ended it), 75 if its wait was
  cancelled, 70 if its lease was taken away, and 130 if it was interrupted while waiting. It
  releases the lease only while `owner.json` still carries its own nonce.
- **Signals and the terminal.** The command runs in a process group of its own, and `lock-run`
  passes `SIGINT`, `SIGTERM` or `SIGHUP` it receives on to that group **once**. On a terminal
  (when `lock-run` is in the foreground), the command's group becomes the terminal's foreground
  group, so it can use the terminal (`stty`, a prompt) and a Ctrl-C reaches it once, straight
  from the terminal. `lock-run` takes the terminal back when the command ends.
- **Re-entry.** The command runs with `CLAUDUCTOR_LOCK_HELD=<lock>`. A `lock-run` on the same
  lock inside it runs the command directly, so a script can wrap itself.
- **Defence in depth.** It holds `flock(2)` on the lease directory while it runs. The flock
  **never decides liveness**: that is the holder's and the command's pid and start time, the same
  rule the shell applies, so Go and shell waiters always agree. The command does not inherit the
  flock (a daemon the gate leaves behind would hold it long after the gate ended). The panel only
  mentions a flock still held on a stale lease. Once a second it checks `owner.json` still
  carries its nonce. If the lease was taken away, it stops the command's whole process group
  (`TERM`, then `KILL` after 5 s) and exits **70**, rather than let two gates finish.

`lock-run` prints `waiting for gate.lock (held by lane add-x (pid 4242) since 14:02:11)` to
stderr while it waits, so an agent reading the output knows why the gate is slow.

### On the page

The page shows each queue: the holder (lane, pid, age, TTL, and "stale" if it is), then the
waiters in order, each with **Cancel wait** (the drawer lists every queue; a lane's Gate tab shows where that lane stands). **Run in `<lane>`** (the lane's Gate tab) starts the queue's `command` through
`lock-run` in that lane's worktree, detached, with its output in
`~/.clauductor/panel/<project hash>/queue-logs/`.

### In a project's gate script

Put this at the top of the gate script (here `gate.sh`). It needs git 2.5 or later (worktrees)
and no `--path-format` (git 2.31). It re-runs the script through the queue with
`clauductor lock-run` when clauductor is installed, and otherwise uses the plain-shell
implementation below (paste it into the script, or keep it beside it as `lease.sh`). A failure
to find the git directory runs the gate unqueued with a note, rather than aborting under
`set -e`:

```bash
#!/usr/bin/env bash
set -euo pipefail
# Serialise the full gate across every worktree of this repo.
lock=""
if common=$(git rev-parse --git-common-dir 2>/dev/null); then
  case $common in /*) ;; *) common="$PWD/$common" ;; esac
  lock="$common/clauductor/gate.lock"
fi
lane="${CLAUDUCTOR_LANE:-$(basename "$PWD")}"
if [ -z "$lock" ]; then
  echo "gate: not in a git checkout; running without the gate queue" >&2
elif [ "${CLAUDUCTOR_LOCK_HELD:-}" != "$lock" ]; then
  if command -v clauductor >/dev/null 2>&1; then
    # TERM=dumb skips a terminal query at clauductor's startup (up to 5 s on a
    # pty that does not answer); lock-run gives the gate the real TERM back, or
    # leaves it unset if it was unset. Ask the environment, not the shell: bash
    # sets an unexported TERM=dumb of its own when TERM is unset.
    term_set="" term_val=""
    if printenv TERM >/dev/null 2>&1; then term_set=1 term_val=$(printenv TERM); fi
    CLAUDUCTOR_TERM="$term_val" CLAUDUCTOR_TERM_SET="$term_set" TERM=dumb \
      exec clauductor lock-run --lane "$lane" "$lock" -- bash "$0" "$@"
  fi
  if [ -f "$(dirname "$0")/lease.sh" ]; then
    . "$(dirname "$0")/lease.sh"        # the plain-shell protocol below
    lease_run "$lock" "$lane" bash "$0" "$@" && exit 0 || exit $?
  fi
  echo "gate: neither clauductor nor lease.sh found; running without the gate queue" >&2
fi
# ...the gate itself...
```

### The protocol in plain shell

`lease.sh` interoperates with `lock-run` in both directions: a test extracts this block from this
page and runs it against `lock-run`, and through the conformance suite. It sets an `EXIT` trap
while it waits and while it holds the lease.

<!-- lease.sh begin -->
```sh
# clauductor lease protocol v1 in plain POSIX shell: interoperates with
# `clauductor lock-run`. Usage: lease_run <lockdir> <lane> <command> [args...]
# Exit status: the command's; 75 if the wait was cancelled from the panel.
# Liveness and start time need ps; without it, kill -0 (EPERM still means alive)
# and /proc/<pid>/stat field 22. A start time from one source is never compared
# with one from the other, and an alive pid that cannot be verified is live.
lease_alive() {
  if command -v ps >/dev/null 2>&1; then [ -n "$(ps -o pid= -p "$1" 2>/dev/null || true)" ]; return; fi
  _e=$(kill -0 "$1" 2>&1) && return 0
  case $_e in *ermitted*) return 0 ;; esac
  return 1
}
lease_pstart() {
  _v=""
  if command -v ps >/dev/null 2>&1; then _v=$(LC_ALL=C ps -o lstart= -p "$1" 2>/dev/null | awk '{$1=$1; print}' || true); fi
  if [ -z "$_v" ] && [ -r "/proc/$1/stat" ]; then _v="proc:$(sed 's/.*) //' "/proc/$1/stat" | awk '{print $20}')"; fi
  printf '%s\n' "$_v"
}
# lease_proc_dead PID RECORDED_START: 0 (true) only when the pid is gone, or was
# reused (a start time from the same source that differs).
lease_proc_dead() {
  lease_alive "$1" || return 0
  _n=$(lease_pstart "$1")
  { [ -n "$2" ] && [ -n "$_n" ]; } || return 1
  _a=${2%%:*} _b=${_n%%:*}
  if { [ "$_a" = proc ] && [ "$_b" = proc ]; } || { [ "$_a" != proc ] && [ "$_b" != proc ]; }; then
    [ "$_n" != "$2" ]; return
  fi
  return 1
}
lease_get() { LC_ALL=C sed -n "s/.*\"$2\"[[:space:]]*:[[:space:]]*\"\{0,1\}\([^\",}]*\).*/\1/p" "$1" 2>/dev/null | head -n 1 | LC_ALL=C sed 's/[[:space:]]*$//' || true; }
# lease_mtime PATH: its modification time in unix seconds: GNU stat, then BSD stat.
# Unknown reads as now (young), so missing data never makes a lock look abandoned.
lease_mtime() {
  _m=$(stat -c %Y "$1" 2>/dev/null) || _m=$(stat -f %m "$1" 2>/dev/null) || _m=""
  case $_m in ''|*[!0-9]*) date +%s ;; *) printf '%s\n' "$_m" ;; esac
}
# lease_valid FILE: 0 (true) for a valid record, the rule Go applies: one flat JSON
# object whose values are strings, integers, true, false or null; v, pid, child_pid,
# started, renewed and ttl integers; nonce, pstart, child_pstart, host, lane and cmd
# strings; and a nonce of 16 lower-case hex digits. An invalid owner.json counts as
# missing; an invalid waiter file holds no place in the queue and is never removed.
# Byte for byte (LC_ALL=C), as Go decodes: a string may hold any byte but \000-\037,
# so DEL (0x7f, which json.Marshal writes unescaped) and bytes that are not UTF-8 are
# allowed whatever the user's locale.
lease_valid() {
  _j=$(LC_ALL=C awk '{ s = s $0 " " } END { print s }' "$1" 2>/dev/null) || return 1
  _c=$(printf '\001-\037')
  _S='"([^"\\'"$_c"']|\\(["\\/bfnrt]|u[0-9a-fA-F]{4}))*"'
  _V="($_S|-?(0|[1-9][0-9]*)|true|false|null)"
  _P="[[:space:]]*$_S[[:space:]]*:[[:space:]]*$_V[[:space:]]*"
  printf '%s\n' "$_j" | LC_ALL=C grep -Eq "^[[:space:]]*\\{($_P(,$_P)*)?\\}[[:space:]]*\$" || return 1
  if printf '%s\n' "$_j" | LC_ALL=C grep -Eq '"(v|pid|child_pid|started|renewed|ttl)"[[:space:]]*:[[:space:]]*[^-0-9[:space:]]'; then return 1; fi
  if printf '%s\n' "$_j" | LC_ALL=C grep -Eq '"(nonce|pstart|child_pstart|host|lane|cmd)"[[:space:]]*:[[:space:]]*[^"[:space:]]'; then return 1; fi
  lease_get "$1" nonce | grep -Eq '^[0-9a-f]{16}$'
}
# lease_dead FILE WAITER_TTL: 0 (true) when the record can be removed: on this host
# only when the holder AND its command (child_pid, written by lock-run) are dead.
lease_dead() {
  _p=$(lease_get "$1" pid); _s=$(lease_get "$1" pstart); _h=$(lease_get "$1" host)
  _cp=$(lease_get "$1" child_pid); _cs=$(lease_get "$1" child_pstart)
  _r=$(lease_get "$1" renewed); _t=$(lease_get "$1" ttl); [ -n "$2" ] && _t=$2
  if [ -n "$_p" ] && [ "$_h" = "$(hostname)" ]; then
    lease_proc_dead "$_p" "$_s" || return 1
    if [ -n "$_cp" ] && ! lease_proc_dead "$_cp" "$_cs"; then return 1; fi
    return 0
  fi
  [ "${_t:-0}" -gt 0 ] && [ "$(date +%s)" -gt $(( ${_r:-0} + _t )) ]   # another host, or no pid: TTL
}
lease_holder_stale() {
  if ! lease_valid "$1/owner.json"; then [ $(( $(date +%s) - $(lease_mtime "$1") )) -ge 10 ]; return; fi
  lease_dead "$1/owner.json" ""
}
lease_run() {
  # A quote or backslash would break owner.json, which readers then judge stale.
  _lock=$1 _lane=$(printf %s "$2" | tr -d '"\\'); shift 2
  _cmd=$(printf %s "$1" | tr -d '"\\')
  _w="$_lock.waiters" _nonce=$(od -An -N8 -tx1 /dev/urandom | tr -d ' \n')
  mkdir -p "$_w"
  _rec="{\"v\":1,\"nonce\":\"$_nonce\",\"pid\":$$,\"pstart\":\"$(lease_pstart $$)\",\"host\":\"$(hostname)\",\"lane\":\"$_lane\",\"cmd\":\"$_cmd\",\"started\":$(date +%s),\"renewed\":$(date +%s),\"ttl\":0}"
  _me="$_w/$(date +%s)000000000-$_nonce.json"
  printf '%s\n' "$_rec" > "$_me.tmp" && mv "$_me.tmp" "$_me"
  trap 'rm -f "$_me" "$_w/$_nonce.cancel"' EXIT
  _said=""
  while :; do
    if [ -e "$_w/$_nonce.cancel" ]; then echo "lease: wait cancelled from the panel" >&2; return 75; fi
    _first=""
    for _f in $(ls "$_w" 2>/dev/null | grep '\.json$' | sort -t- -k1,1n -k2); do
      lease_valid "$_w/$_f" || continue
      if lease_dead "$_w/$_f" 60; then rm -f "$_w/$_f"; continue; fi
      _first=$_f; break
    done
    if [ "$_first" = "${_me##*/}" ] && mkdir "$_lock" 2>/dev/null; then
      printf '%s\n' "$_rec" > "$_lock/.owner.tmp" && mv "$_lock/.owner.tmp" "$_lock/owner.json"
      rm -f "$_me"
      trap 'if [ "$(lease_get "$_lock/owner.json" nonce)" = "$_nonce" ]; then rm -rf "$_lock"; fi' EXIT
      CLAUDUCTOR_LOCK_HELD=$_lock "$@" && _rc=0 || _rc=$?
      if [ "$(lease_get "$_lock/owner.json" nonce)" = "$_nonce" ]; then rm -rf "$_lock"; fi
      trap - EXIT
      return "$_rc"
    fi
    if [ "$_first" = "${_me##*/}" ] && [ -d "$_lock" ] && lease_holder_stale "$_lock"; then
      _judged=$(lease_get "$_lock/owner.json" nonce)
      if mkdir "$_lock.reclaim" 2>/dev/null; then
        if [ "$(lease_get "$_lock/owner.json" nonce)" = "$_judged" ] && lease_holder_stale "$_lock"; then
          echo "lease: reclaiming $_lock from a dead holder" >&2; rm -rf "$_lock"
        fi
        rmdir "$_lock.reclaim"; continue
      elif [ $(( $(date +%s) - $(lease_mtime "$_lock.reclaim") )) -ge 30 ]; then rmdir "$_lock.reclaim" 2>/dev/null || true
      fi
    fi
    [ -n "$_said" ] || { echo "lease: waiting for $_lock ($(lease_get "$_lock/owner.json" lane))" >&2; _said=1; }
    sleep 1
  done
}
```
<!-- lease.sh end -->

### The conformance suite

The protocol has more than one implementation: `lock-run`, the `lease.sh` above, and whatever a
project writes for itself. `framework/internal/panel/lease/testdata/lease-conformance/` holds the
suite they must all pass: golden `owner.json` and waiter records (`cases/<case>/`, with
`{{PLACEHOLDERS}}` filled in from real processes; one left unfilled fails its case) and a
driver, `conformance.sh`, which prints TAP and exits 1 on a failure.

**The interface.** An implementation waits its turn for a lease, runs a command holding it,
releases it, and exits with the command's status (75 if its wait was cancelled). The driver hands
it the lock one of two ways:

```bash
conformance.sh <impl> [impl-args...]
#   runs  <impl> [impl-args...] <lockdir> <lane> <command> [args...]
conformance.sh --lock-env VAR <impl> [impl-args...]
#   runs  VAR=<lockdir> CLAUDUCTOR_LANE=<lane> <impl> [impl-args...] <command> [args...]
CASES="dead-pid cancel" conformance.sh …     # some cases;  conformance.sh --list  names them all
CONFORMANCE_WAIT=3 CONFORMANCE_TIMEOUT=20    # how long a waiter must wait, and a runnable one may take
```

The first form needs a two-line adapter (the driver's header has the ones for `lock-run` and
`lease.sh`). The second needs none: a project's own gate wrapper, whose lock is normally
`<git common dir>/…`, runs the suite as it is once that path can be overridden by `VAR`.

The command must see `CLAUDUCTOR_LOCK_HELD` equal to the lock path **exactly as it was given**:
cleaned, never symlink-resolved. A gate script compares the two to detect re-entry, and on a
path through a symlink (macOS's `/tmp` is one, and a git common dir can be) a resolved value never
matches, so the script would queue behind itself. `symlinked-lock` checks it.

Each case checks only what every implementation must do: whether and when the command runs, its
exit status, and the files left behind.

| Case | The implementation must |
|---|---|
| `live-holder` | wait behind a live holder on this host (start time matches), leave its record alone, write its own `<arrival>-<nonce>.json`; run once the holder's process ends |
| `dead-pid` | reclaim a holder whose pid is gone, run, and release |
| `pid-reuse` | reclaim a holder whose pid is alive with another start time |
| `proc-format` | treat a `proc:` start time as unverifiable against `ps`: wait |
| `no-ps` | with no `ps` on PATH and no recorded start time, judge by `kill -0`: wait while alive, reclaim once gone |
| `pstart-no-ps` | with no `ps` on PATH, never read a recorded `ps` start time as a reused pid: wait while alive |
| `no-ps-foreign-pid` | with no `ps`, read `kill -0`'s `EPERM` (another user's process) as alive (skipped as root) |
| `other-host-expired`, `other-host-live`, `other-host-no-ttl` | judge another host's holder by `renewed + ttl` only; `ttl: 0` never expires |
| `missing-pid`, `missing-pid-expired` | judge a record with no `pid` by its TTL alone |
| `missing-host` | judge a record with no `host` as another host's: its dead pid means nothing |
| `child-alive`, `child-dead`, `child-reused` | keep a lease whose holder died while its command (`child_pid`) runs; reclaim when both are gone, a reused `child_pid` included |
| `ownerless-old`, `ownerless-young` | reclaim a lock directory with no `owner.json` once it is 10 s old, and wait until then |
| `truncated-owner-old`, `truncated-owner-young`, `bad-nonce-owner-old`, `garbage-owner-old`, `string-pid-owner-old` | treat an invalid `owner.json` (truncated, no 16-hex nonce, not a flat object, a field of the wrong type) as missing, even with a live pid in it |
| `spaced-owner-live` | read a valid record written with spaces, newlines, escapes and an extra `null` field as the live holder it names |
| `del-owner-live` | read a live holder whose `cmd` holds DEL (0x7f, which `json.Marshal` writes unescaped) as live: wait |
| `high-bytes-owner-live` | read a live holder whose `cmd` holds bytes that are not UTF-8 (0xff 0xfe) as live, whatever the waiter's locale: wait |
| `live-waiter-ahead`, `dead-waiter-ahead` | never jump a live waiter that arrived first; skip and remove a dead one |
| `other-host-waiter-stale`, `other-host-waiter-fresh` | judge another host's waiter by a 60 s TTL, whatever `ttl` its file names |
| `malformed-waiters` | give invalid waiter files no place in the queue, and never remove them |
| `cancel` | give up on `<nonce>.cancel`: exit 75, run nothing, remove its files, leave the holder |
| `reclaim-race` | with three waiters meeting one dead holder, run each command exactly once, never two at a time |
| `owner-record` | write every field of `owner.json` while it holds, with `pstart` as `ps` prints it; set `CLAUDUCTOR_LOCK_HELD`; exit with the command's status and release |
| `symlinked-lock` | set `CLAUDUCTOR_LOCK_HELD` to a lock path through a symlink exactly as given |

`TestLeaseConformance` runs it against `lock-run` and against the `lease.sh` block extracted from
this page. The suite is falsified in the same run: two controls, one that ignores the lease and
one that always takes it, must fail every case that depends on the rule they break, and each of
17 mutants of `lease.sh` (`leaseShMutants`: DEL rejected as a control character, the old
`[:cntrl:]` rule in the user's locale (run in a UTF-8 locale), EPERM read as dead, start times compared across
sources, an unverifiable pid read as dead, pid reuse ignored, the command ignored, a waiter's own
`ttl` used, `ttl: 0` expiring, no grace for a starting holder, a truncated `owner.json` read as a
record, the record's shape or its integer fields unchecked, invalid waiter files removed or queued,
LIFO order, cancel ignored) must fail at least one case. The cases that need an old lock
directory set its mtime 60 s back and check that it took; the cases with a young one judge the
grace by timestamps (the command records when it ran, against the lock's mtime), not by how long
the driver watched, so a loaded machine cannot stretch or shrink the window. The driver runs
at most one case per CPU (at least 4) at a time. `TestLeaseConformanceLockEnvMode` runs the adapter-free form.

## Alerts

Alerts are derived from the state, never stored, against the `alerts` thresholds:

| Alert | When | Severity |
|---|---|---|
| waiting | a permission prompt, MCP elicitation or input request older than `waiting_seconds` | block |
| rate_limit | `StopFailure` with `error_type: rate_limit` | block |
| stop_failure | any other `StopFailure` | warn |
| no_auto_resume | `quota_auto_resume_stale` or `_disabled`: the lane will not continue by itself | warn |
| context | `context_window.used_percentage` ≥ `context_pct` | warn |
| idle | a live session idle longer than `idle_minutes` | info |
| quota | the 5-hour quota window ≥ `five_hour_pct` (block at 100%); from any session's status line | warn |
| approval_wait | a change's proposal with no `**Approved:**` line, last written longer than `approval_wait_hours` ago (PANEL-19) | warn |
| budget | a change whose branches have spent more than its proposal's `**Budget:** $N` (PANEL-19) | warn |
| stale | a lane on a branch of its own with no commit for `stale_days` (PANEL-19) | warn |

### Current or stale

Whether a session is blocked on you is decided by ONE predicate, which *Needs you*, the waiting
alert, the lane chip and the first-prompt decision all read, so they cannot disagree. A `claude
agents` entry counts as a **current reading** only while the last poll succeeded within two poll
intervals. A prompt answered in the terminal fires no hook, so only a current reading can say it
was answered. "Recently" allows for the poll itself: two intervals plus the slowest recent poll
(the filter cross-check included), so a `claude agents` slower than its interval never reads as
stale between two good polls. An item is marked **stale/approx** (in its label, and as `approx` in
`/api/state`) when:

- the last reading said waiting, but the poll has since failed or stopped arriving;
- a hook says the session waits and no current reading confirms it (never polled, not listed,
  or listed as idle).

An approximate item is shown and never raises a macOS notification. It also keeps the mark of a
notification already sent, so a poll that flickers stale and back does not notify twice. The lane
card, the lane's status chip, the sessions table and the terminal tab show an approximate status
with a leading `≈`. A failing poll does not keep sessions forever: one silent for 30 minutes (no
hook, no status line, no current reading) is forgotten either way. The exception is a session
with an open permission, elicitation or input prompt: while polls fail, nothing can say it was
answered, so it stays in *Needs you*, marked approximate, until a poll works again, or until its
lane's pane is dead (or its tmux session gone), or 24 hours pass without a word from it.

### Notifications

Each alert shows in the **Alerts** panel. Only what blocks you **interrupts**: a macOS
notification goes out for a new `block` alert (waiting, rate_limit, quota at 100%) and for
`no_auto_resume`. Idle, context, quota and stop-failure alerts stay on the page, and so does any
alert marked approximate. A notification goes out:

- once per stretch: an alert that clears and comes back notifies again. What was notified, and
  when each lane last was, is saved in `~/.clauductor/panel/<project hash>/notifier.json`, so a
  panel restart never re-notifies an alert that is still active, and the daily count survives it;
- grouped: the new alerts of one lane make one notification;
- rate-limited: at most one notification per lane per `min_interval_seconds`; later ones wait and
  go out when the interval ends, if still active;
- suppressed while that lane's terminal has keyboard focus in a visible page (the page reports
  focus over the terminal's WebSocket). A suppressed alert counts as seen.

The Alerts heading shows **interruptions today**. Notifications run
`/usr/bin/osascript -e 'on run argv' -e 'display notification (item 2 of argv) with title (item 1 of argv)' -e 'end run' -- <title> <text>`:
the text arrives as an argument and is never spliced into AppleScript source, so a quote in a lane
name cannot run `do shell script`. The `--` matters: osascript keeps parsing options among its
arguments, so without it a title starting with `-e` would be read as more script. The title is
the config's `name` only while the config is trusted; `name` must be one line of plain text that
does not start with `-`. The first one may make macOS ask whether the panel may send
notifications.

## Metrics

PANEL-19. How the work flows, what it costs, how good it is, and whether it did what it meant
to. The figures come from two places, and the page marks every one with which:

- **The project's metrics command** (`metrics.command`, config version 4): a project command,
  like a card's, whose stdout is the JSON below. Clauductor's operating model ships one
  (`.claude/metrics.sh`, OPS-9); any project can write its own.
- **The panel's own** (built in), for any repository, with or without the command: merge
  frequency and PR cycle time from merged pull requests (`gh`), spend from the status line's
  posts, and the work in flight from the lanes. It costs no model token and runs nothing new but
  one `gh pr list --state merged`, at most every 10 minutes while a page is in view.

Where both have a figure, the project's is shown. A figure neither has shows "—" and why ("Only
a project's metrics command reports this", "No pull request was merged in the last 7d", "The
metrics command failed: …"), never a zero.

### The metrics JSON (contract version 1)

```json
{
  "version": 1,
  "generated_at": 1790000000,
  "windows": {
    "30d": {
      "flow": {
        "lead_time":        { "value": 44,  "series": [50, 40, 46, 38, 42, 44, 45, 41, 43, 44], "n": 12 },
        "cycle_time":       { "value": 7.5, "n": 12 },
        "approval_wait":    { "value": 5.5, "n": 12, "note": "proposal written to Approved line" },
        "merge_frequency":  { "value": 2.8, "series": [2.3, 4.7, 2.3, 0, 2.3, 4.7, 2.3, 2.3, 4.7, 2.3] },
        "change_fail_rate": { "value": 8.3, "n": 12 },
        "aging_wip": [ { "id": "add-score-photo", "title": "Photograph a scorecard", "age_days": 4.5, "stage": "build" } ]
      },
      "cost": {
        "total_usd": 162.4,
        "per_week":   { "value": 37.9, "series": [30, 35, 41, 38, 40, 36, 39, 37, 42, 38] },
        "by_role":    [ { "name": "builder", "usd": 90.1 } ],
        "by_model":   [ { "name": "opus", "usd": 140.4 } ],
        "by_change":  [ { "name": "add-score-photo", "usd": 61.5, "budget_usd": 50 } ],
        "by_project": [ { "name": "My Project", "usd": 162.4 } ]
      },
      "quality": {
        "review_rounds":   { "value": 1.5, "n": 12 },
        "reviewer_recall": [ { "model": "opus", "pct": 92, "n": 40 } ],
        "escaped_defects": { "value": 1 }
      },
      "outcomes": {
        "hypotheses": [ { "change": "add-score-photo", "hypothesis": "Half of new cards start from a photo",
                          "due": "2026-10-20", "checked": false, "result": "" } ]
      }
    }
  }
}
```

- `windows` has any of `7d`, `30d` and `90d`; every section and every figure is optional.
- A figure is `{value, series?, n?, note?}`. `value` may be `null` (with a `note` saying why);
  `series` is the same figure over equal buckets of the window, oldest first, at most 120
  points, a `null` point being a bucket with no data; `n` is how many items it summarises.
- The units are fixed, so a payload carries numbers only: `lead_time`, `cycle_time` and
  `approval_wait` are median **hours**; `merge_frequency` is **per week**; `change_fail_rate`
  and `pct` are **percent** (0–100); money is **US dollars**; `age_days` is days;
  `review_rounds` is a median count per change; `escaped_defects` a count.
- `due` is `YYYY-MM-DD`. `generated_at` is unix seconds; the view shows its age.
- It is read strictly, and drawn whole or not at all: an unknown key, another `version`, a window
  other than those three, a negative or non-finite number, a percent over 100, text with a
  control character or over 300 characters, a list over 500 entries, or output of 1 MB or more is
  refused, and the view shows the error with the path of what is wrong
  (`windows.30d.flow.change_fail_rate.value: must be at most 100`). A failed run shows its error,
  not the last good payload's figures. The panel's own figures still draw.
- `framework/internal/panel/metrics/testdata/metrics.sh` is a fixture command that prints a
  valid payload (`metrics.json` beside it), or with `METRICS_FIXTURE=bad` one that breaks it.

### The panel's own figures

| Figure | From | How |
|---|---|---|
| Merge frequency | `gh pr list --state merged --search merged:>=<90 days ago> --limit 300` | merges in the window, per week; the series per bucket |
| Cycle time | the same | median hours from a pull request's creation to its merge |
| Cost: total, per week, by lane type, by model, by branch, by project | the spend ledger | the status line's `total_cost_usd`, per session, added up a day at a time |
| Aging work in progress | the lanes | each registered lane on a branch of its own, by how long it has run |
| Approval wait | the change directory (see *Needs you from the metrics*) | how long each proposal waiting for approval has waited so far |

Buckets: 7 of a day for 7d, 10 of 3 days for 30d, 15 of 6 days for 90d. At gh's limit of 300 the
figures say the oldest part of the window may lack merges. The ledger (see *Security model*,
what the panel writes) counts a session the first time it sees it in full, and after that only
what it added, so a panel restart counts nothing twice; spend from before the panel kept a
ledger is not in it, and the figures say "Since <day>" until the ledger is as old as the window.
By lane type, because the panel knows a lane's type, not the role a skill switched to: a
project's command can report by role.

### The command runs as a card does

The metrics command is config like any other project command (see *Config trust*): it runs
only while `panel.json` is trusted as it is, in the project's main checkout, without a shell, with
a 30-second timeout and 1 MB of output; `trust` and `install` print it among what they trust.
Untrusted, it does not run and the view says so. The JSON is data: every string reaches the page
through `textContent`, never markup.

### Needs you from the metrics

Three signals act where you already look: the **Alerts** rows under **Needs you** (they show
there whether or not a lane has them), and the lane's own **Alerts** tab. They are warnings, so,
as every alert that does not block (see *Notifications*), they never raise an OS notification.

- **An approval waiting too long.** A change's `proposal.md` with no `**Approved:** <date> by
  <owner>` line (OPS-7's D8), last written (it or its `design.md`) more than
  `alerts.approval_wait_hours` ago (default 24; 0 turns it off).
- **A change over its budget.** A proposal's `**Budget:** $N` line (OPS-7, a cost budget per
  change), against what every lane on the change's branches has spent, from the spend ledger. A
  branch is the change's when its last path segment is the change's id (`change/add-x` builds
  `add-x`). The lane's header shows a **Budget** bar next to **Cost**: what the change has spent
  of its budget, amber from 80%, red past it.
- **Work in flight gone quiet.** A registered lane on a branch of its own with no commit for
  `alerts.stale_days` (default 3; 0 turns it off), counted from the lane's start while it has no
  commit of its own. The commit time is the git read the dashboard takes while a page is in view,
  so a lane not read yet is never called stale.

The panel reads the changes from files alone, every minute and on **Refresh**, and runs no
command for them: `<worktree>/<CHANGES_DIR>/<id>/proposal.md` in the main checkout and in every
worktree (a proposal is drafted on a lane's branch before it lands; the copy written last
counts), where `CHANGES_DIR` comes from `.claude/project.conf` when it names a directory inside
the repository, else `changes`; and `openspec/changes/<id>/`. `archive/` is not read. The same
reading gives the built-in **Approval wait** (the proposals waiting now) and the budgets beside
**By change**.

### Economy mode

Off unless `quota_economy.five_hour_pct` is set (config version 4; the default project's, since
the quota is the machine's). While the account's 5-hour quota is at or above it, the panel is in
economy mode, and it leaves only once the quota is **3 points below** (so a quota hovering at the
line does not flap). A reset window, or no reading, keeps the mode as it is.

**The contract: `~/.clauductor/panel/economy.json`** (0600, written atomically, and only when
the mode switches):

```json
{ "economy": true, "since": 1790000000, "reason": "5-hour quota 87% ≥ 85%" }
```

`since` is the unix time of the switch; `reason` the reading that switched it (off reads
`"5-hour quota 81% < 82% (on at 85%)"`). A missing file means off. With `quota_economy` unset
the panel writes nothing, except to turn a file an earlier run left on to off. Clauductor's
operating model's `build-change` reads it and moves the roles that are not critical down one
tier; which roles, and to what, is the project's `.claude/model-roles.json`:

```
"economy": { "_why": "…", "scribe": { "model": "sonnet", "effort": "low" }, "mechanic": "haiku/low" }
```

Each key not starting with `_` is a role; its value is the tier it drops to, as
`{model, effort}` or `"model/effort"` (the same object may sit under `economy.roles`). The
reviewer and planner are simply not listed. While economy mode is on, an **Economy** field by
the quota says **economy** and names those roles ("scribe to sonnet, low"); hovering says why it
is on and since when. The panel only reads that file, every minute, and runs nothing.

`GET /api/p/<project>/metrics` is the view (`?scope=all` combines every project); like every
route it needs the cookie, and it runs nothing: it reports what the sources last read. For all
projects, merges, spend and escaped defects add up; a median cannot, so it is the projects'
values weighted by how many each summarises, and says so; lists name their project.

## Appearance

**Appearance**, at the right of the status bar, holds three independent choices, each kept per
browser (without storage the page shows the defaults and the menu still works for the visit). A
small script, `static/theme.js`, loads first and applies all three before the first paint, so a
stored choice never flashes the default.

- **Theme**: colour, surfaces, separators and density, in a **Mode**: **System** (the default:
  follows the OS light or dark setting), **Light** or **Dark**. Each theme's light and dark are
  designed apart, not inverted.
- **Type**: the UI face, the data face (every aligned value: ids, SHAs, durations, figures) and the
  terminal face. **Theme's choice** (the default) takes the type system the theme suggests; any
  theme works with any type system.
- **Size**: the page's text, 85% to 175% in 5% steps (with nothing chosen, 110% on a window at
  least 1440 px wide, 100% below), and the terminal's, 11 to 24 px, apart from the page's (with
  nothing chosen, the type system's size times the page's). Each is a slider with A− and A+
  beside it, and the menu stays open while you change them. **Ctrl+Alt+=** and **Ctrl+Alt+−**
  step the page's size and **Ctrl+Alt+0** resets it, inside a terminal too: the terminal takes
  them and never sends them to claude. The browser's own zoom keys stay the browser's.
- **At a large size the layout folds.** When the rail and the side panel would leave the
  terminal fewer than 85 columns of its face, the side panel folds, then the rail; they come back
  when there is room. **Show details** and **Lanes** show them anyway for the visit. From 140% the
  status bar puts each figure on one line and drops the secondary ones (cost today and per hour,
  CPU and memory, interruptions, hooks). The page never scrolls sideways; if the window is short,
  it scrolls down to the terminal rather than squeezing it.

The menu works from the keyboard: Down or Enter opens it, the arrow keys, Home and End move, Enter
picks, Escape closes it and returns focus to the button. On a Tuned slider, Left and Right change
its value. Changing the theme re-colours the open terminals at once; changing the type system
loads its terminal face first, then refits every terminal to the new cell. Every theme's terminal
is dark: only a dark background lets each ANSI colour read as text and also carry a label in
another ANSI colour.

| Theme | After | Light | Dark | Suggests |
|---|---|---|---|---|
| **Grey HMI** (default) | ISA-101 control rooms | A mid-light grey ground, darker grey rules; colour only when abnormal | A neutral charcoal, never navy; desaturated amber and red | Highway |
| **Terminal Amber** | The Bloomberg terminal | Black ink on cool white; black-on-amber blocks for what is abnormal or selected | Amber on true black, white secondary text | Cockpit |
| **Glass Cockpit** | Airbus-style displays | Light grey with a dark instrument bezel for the status bar | Black: green nominal, amber caution, red warning, cyan only for what you can act on, magenta for projections | Cockpit |
| **Tuned** | Linear's generated themes | Generated in CIE LCh from a base hue, an accent hue and a contrast (the Tuned sliders); separators 6 L from the ground | The same inputs; contrast at its top holds all text, and the terminal, to AAA (7:1) | Civic |
| **TUI** | btop, k9s, lazygit | ANSI colours, box-drawn panes with their titles in the border, braille sparklines, a key-hint bar | The same, dark; the focused pane shown by its border | Engineer |
| **System Native** | macOS Activity Monitor | System greys, zebra rows, a source-list sidebar; the accent only for the selection | macOS dark greys, flat fills | Hyperlegible |

| Type system | UI | Data | Terminal |
|---|---|---|---|
| **Cockpit** | B612 | B612 Mono | Iosevka Term (B612 Mono's round brackets read as square ones at terminal sizes) |
| **Highway** (default) | Overpass | Overpass Mono | Overpass Mono |
| **Civic** | Public Sans | Commit Mono | Commit Mono |
| **Hyperlegible** | Atkinson Hyperlegible Next | Atkinson Hyperlegible Mono | Atkinson Hyperlegible Mono |
| **Engineer** | Iosevka Aile | Iosevka | Iosevka Term (half a pixel larger: Iosevka reads small) |
| **Variable** | Mona Sans, condensed in tables | Monaspace Neon | Monaspace Argon |

Rules that hold for every theme × type: weights 400 and 700 only; x-heights evened with
`font-size-adjust: ex-height .52`; every figure in tabular, lining, slashed-zero digits; nothing
under 11.5 px at 100%; sentence-case headings; no chips, pills, cards, gradients, glows or
shadows except on what floats (menus, the drawer, the dialog). All faces are self-hosted Latin
subsets under the SIL OFL 1.1 (B612 is also offered under EPL 2.0 and EDL 1.0; the panel uses the
OFL). Overpass Mono, Commit Mono, the Iosevka faces, Mona Sans and Monaspace were subset from
upstream to keep the box-drawing and prompt glyphs a terminal needs, where the font has them.
Mona Sans and Monaspace carry Reserved Font Names, so their subsets are renamed Panel Sans, Panel
Data Mono and Panel Term Mono, as the OFL requires of a modified font. Before PANEL-11 every
terminal drew in Menlo whatever the theme (the face was fixed in `panel.js`), so the largest
thing on the page looked the same in every theme.

### Adding a theme or a type system: the token contract

A theme and a type system are only tokens. `web/static/panel.css` references tokens and never a
colour or a face of its own, so neither adds component CSS. The root size is the theme's
`--fs-root` times your `--ui-scale`, and every size in `panel.css` is in rem.

To add a theme, add `{ id, name, note, type }` to `THEMES` in `web/static/theme.js` (`type` is the
type system it suggests; `tuned: true` marks a generated one), then three blocks to
`web/static/themes.css`:

- `[data-theme="<id>"]`: density and shape, shared by both modes: `--fs-root`, `--ui-scale` (1;
  theme.js overrides it), `--row-h`, `--cell-x`, `--pane-p`, `--r-ctl` (controls only; panes are
  square), `--bw`, `--rail-w`, `--side-w`, `--term-min-contrast`, and the TUI switches `--tui` (1
  boxes panes with their titles in the border), `--keyhints` (`flex` shows the key-hint bar),
  `--spark` (`"line"` or `"braille"`) and `--title-case`.
- `[data-theme="<id>"][data-mode="light"]` and `…[data-mode="dark"]`: every colour. Surfaces
  `--ground`, `--pane`, `--pane-2`, `--zebra`, `--rail`, `--bar`; rules `--rule`,
  `--rule-strong`; text `--text-1`, `--text-2`, `--text-3`, `--bar-text`, `--bar-text-2`; states
  `--ok` (a text tier unless the theme's convention colours nominal), `--warn`, `--crit` and
  their rows `--warn-bg`, `--crit-bg` with inks `--warn-ink`, `--crit-ink`; `--act` (only what you
  can click or type into) and `--act-ink`; `--info` (projections); `--sel`, `--sel-text`,
  `--focus`, `--scrim`; and the terminal's `--term-bg`, `--term-fg`, `--term-cursor`,
  `--term-selection` and `--ansi-0` to `--ansi-15`.

To add a type system, add `{ id, name, note }` to `TYPES` in `theme.js` and a
`[data-type="<id>"]` block to `web/static/types.css` with `--font-ui`, `--font-data`,
`--font-term`, `--num` (the figures' `font-feature-settings`), `--table-stretch`, `--term-size` and
`--data-bump`. Put its faces in `web/static/fonts/` with an `OFL-<family>.txt` each, and add their
`@font-face`s to `types.css`.

What checks them:

- `themes_test.go`, for every theme × mode: the text tiers, `--act`, `--ok`, `--warn` and `--crit`
  reach 4.5:1 on every surface; `--bar-text` and `--bar-text-2` on the bar, `--act-ink` on
  `--act`, each ink on its row (its text, and any control on it, which takes the ink) and `--sel-text` on the selection reach 4.5:1; `--info`,
  `--focus` and `--rule-strong` reach 3:1; warn and crit differ by ΔE 20; the terminal foreground
  reaches 7:1, each ANSI colour 3:1, and the label pairs a TUI draws 4.5:1 (black on green, yellow
  and cyan; white on red, blue and magenta; and the bright variants). It fails if a theme × mode
  lacks a token another defines or that `panel.css` or `panel.js` uses and no type system
  defines, if `theme.js` and `themes.css` disagree, if a theme block sits inside a conditional
  rule, if `panel.css` fades anything with `opacity` but a disabled control, or on a font file no
  type system loads, one without a licence, or more than 700 KB of fonts.
- `tells_test.go`: `panel.css` has no uppercase, letter-spacing, gradient, animation, weight other
  than 400 and 700, glow, rounded pane, or shadow on anything that does not float; it evens
  x-heights and sets the figure features; nothing is under 11.5 px; `panel.js` joins no words
  with " · " and puts no arrow on a button, and the cursor does not blink. It also runs the Tuned
  generator over a grid of hues, accents and contrasts, in both modes, and holds every output to
  the same floors, and to AAA (7:1, with the High contrast terminal palette) at full contrast.
- `types_test.go`: `theme.js` and `types.css` agree, every type system sets every type token and
  nothing else, the faces it names have an `@font-face`, every theme suggests a type system that
  exists, no token is set in both layers, and each type system's files stay under 125 KB (the
  brief asked for about 120; the largest, Variable, is 117 KB with Mona Sans's width axis).
- The browser test (`testdata/browser/run.sh`) loads every type system's faces through
  `document.fonts`, checks that each terminal face draws `(` curved enough not to read as `[`,
  that a type change refits the terminal to the new cell, that the size keys pressed inside a
  terminal resize the page and send claude nothing (a fake claude records its input), and that
  at 175% on a 1440 px window the page does not scroll sideways and every main control is
  reachable.

## Signals: hooks and the status line

### The status line

Hooks carry no cost or context data; only the status line's stdin does. A project that wants
quota, context % and est. $ on the panel adds this to its status-line script.

`~/.clauductor/panel/port` holds the port and nothing else, because scripts read it as digits.
The panel's PID is in `~/.clauductor/panel/pid` beside it, and `owner.json` records its start
time, project and port. All three are removed on a clean stop, but only while `pid` still names
that panel: a panel never deletes another panel's files. A `pid` naming a process that is not
running (or one with another start time) means the panel was killed and the files are stale.

So the snippet reads `pid`, takes it only as a positive integer (`kill -0 -1` and `kill -0 0`
succeed whatever runs, since they signal every process you own or your process group), and
checks that the process is alive (`kill -0`) before it posts: a
panel killed with `SIGKILL` leaves `port` behind, and another program may hold that port by now.
It posts only then, never waits (background, 0.5 s cap), and prints nothing, so the status line
is unaffected on a machine that has never run the panel, or whose panel is down:

<!-- statusline begin -->
```bash
input=$(cat)
panel="$HOME/.clauductor/panel"
pid=$(cat "$panel/pid" 2>/dev/null)
case "$pid" in ''|0*|*[!0-9]*) pid= ;; esac   # digits only, not 0: kill -0 -1 and kill -0 0 always succeed
if [ -f "$panel/port" ] && [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
  port=$(cat "$panel/port")
  printf '%s' "$input" | curl -s --max-time 0.5 -X POST -H 'Content-Type: application/json' \
    --data-binary @- "http://127.0.0.1:$port/status" >/dev/null 2>&1 &
fi
# ...the script's existing output, reading "$input" instead of stdin...
```
<!-- statusline end -->

`kill -0` is the cheap check a status line can afford on every refresh. The full one also compares
the process's start time with `owner.json`'s, which is what `clauductor panel open` does before it
sends the token (see [The launchd agent](#the-launchd-agent)).

### Hooks

On every start, and again every 30 s while it runs, the panel merges one `type: "http"` hook per
event into the **running user's** `~/.claude/settings.json`, and nowhere else:

```json
{ "type": "http", "url": "http://127.0.0.1:4393/hook?src=clauductor-panel", "timeout": 1 }
```

for `UserPromptSubmit`, `Stop`, `SubagentStart`, `SubagentStop`, `Notification`, `SessionEnd`,
`StopFailure`, `PermissionRequest` (observed only, never answered), `PreCompact`, `PostCompact`,
`CwdChanged`, and from PANEL-11 `PreToolUse` and `PostToolUse` with the matcher
`Agent|Task|Workflow`, so the panel hears them only for the calls that start agents (see [Which
agent started which](#which-agent-started-which)); it answers them 204, no decision.
`SessionStart` is left out because HTTP hooks do not fire for it (Claude Code 2.1.284); new
sessions are found through `claude agents --json`.

- The panel's entries are recognised by the `src=clauductor-panel` query parameter, because Claude
  Code documents no free-form key for ownership. Only tagged entries are replaced or removed;
  every other key and hook is kept, in order. The panel's current entry stays where it is, even if
  a user hook follows it.
- The install is idempotent: a start that would change nothing does not rewrite the file.
- A file that is not exactly one JSON object (invalid, or with trailing data) is refused and not
  touched.
- Before the first write, the file is copied to `settings.json.clauductor-panel.bak`. That backup
  is never overwritten, so it keeps the file as it was before the panel first touched it. Writes
  are atomic (temp file + rename, in the same directory). The install re-reads the file
  immediately before its rename, and redoes the edit if another writer changed it meanwhile (up
  to 5 times, then it refuses).
- A symlinked `settings.json` (for example, one managed by a dotfiles repo) is followed: the
  panel edits the file it points at, and the link stays a link.
- **Drift is repaired.** Every 30 s the running panel checks that its hooks are still there and
  still point at its own port. If they were removed, or point at a port where no panel answers,
  it reinstalls them and a warning bar says what it found, for 10 minutes. If they point at
  another **live** panel (one its lock could not refuse, such as an older binary), it leaves them
  alone and shows a red banner naming that panel's pid and port, so the two never fight over the
  hooks. It takes them back at the first check after that panel stops. `--uninstall-hooks` while
  a panel runs says the panel will put them back.
- **A failed install is not fatal.** If the install fails (for example, `settings.json` is not
  valid JSON), the panel still serves, shows a red banner with the error, and retries after 1 s,
  doubling up to 30 s. Under launchd a fatal error would restart the panel every 30 s instead.
- The hooks stay installed when the panel stops. While it is down, the connection is refused
  at once and the session is never blocked. `clauductor panel --uninstall-hooks` removes them.

### How signals are read

- **Notifications.** Each of the 12 documented `notification_type` values maps to what it does
  to the session, whether it goes in **Needs you**, and how severe it is:

  | Type | Effect | Needs you | Severity |
  |---|---|---|---|
  | `permission_prompt` | waiting | yes | block |
  | `elicitation_dialog`, `elicitation_url_dialog` | waiting | yes | block |
  | `agent_needs_input` | waiting | yes | block |
  | `quota_auto_resume_stale`, `quota_auto_resume_disabled` | none | yes | warn |
  | `idle_prompt` | idle: your move | no: **Done** | info |
  | `agent_completed` | none | no: **Done** once the turn is over | info |
  | `elicitation_complete`, `elicitation_response`, `quota_auto_resume_fired` | answers a waiting note | no | info |
  | `auth_success` | none | no | info |
  | anything else | none; shown on the session, counted | **never** | unknown |

  **Needs you** holds blocking states only. **Done · your move** lists finished turns apart.
- **Hook events.** `StopFailure` (its `error_type`), `PermissionRequest`, `PreCompact` and
  `PostCompact` (shown as "compacting"), and `CwdChanged` (recorded in the feed). The panel only
  **observes** `PermissionRequest`: `/hook` answers `204` with an empty body, which Claude Code
  documents as "no decision", so the permission dialog proceeds as usual. `/hook` takes no token,
  so it must never answer. Only subscribed event names are applied; others are counted.
- **Binding.** A session is bound to its lane once. A lane the panel started is bound by the
  session id it assigned (`--session-id`), whatever the event's `cwd`. Any other session is bound
  by its `cwd` at first sight, and a later `cd` does not move it.
- **Quota.** A window whose `resets_at` has passed is dropped, and its gauge says "reset". Every
  status post updates it, whatever project it comes from; a window that does not decode is
  skipped without costing the post anything else (see *The account and its quota*).
- **`claude agents`.** `id`, `state` and the `waitingFor` enum (permission prompt, input needed,
  sandbox request, worker request, dialog open) are decoded. The poll passes `--cwd <the
  deepest directory holding every worktree>`, but only after a cross-check: every 5 minutes it
  also runs unfiltered, and if the filter drops any session of the project, it polls unfiltered.
  One poll measured 93–103 ms wall (p50 98 ms), about 105 ms CPU and 148 MB peak RSS on
  2.1.284; at 2 s that is about 5% of a core. So it backs off to 5 s while hooks are flowing (a
  hook in the last 30 s), and to 15 s while the panel has no lane and has heard no hook for 5
  minutes; a kick (a lane action, a refresh, a hook arriving during the 15 s wait) still polls
  at once. A reading counts as current for two of whichever interval the loop is on.
- **Overflow.** A hook body dropped because the panel fell behind is counted apart from foreign
  drops, and raises a banner.
- **Version pinning.** The subagent pairing, the missing `SessionStart` HTTP hook and the recorded
  fixtures were verified on Claude Code **2.1.284**. The panel reads `claude --version`; on any
  other version the subagent list says "approximate" and a warning bar says why.
- **Re-verifying a new version, by itself (PANEL-13).** On a version it has not verified, the
  panel checks the pairing against the hooks the project's own sessions send: no model token, no
  session of its own. Each Agent call's `PostToolUse` names the agent it started
  (`tool_response.agentId`); when a `SubagentStart` announced that same agent (`agent_id`),
  before or up to 10 s after, it is a confirmation. Three confirmations verify the version: the
  warning goes, the lists stop saying "approximate", the panel logs it, and
  `~/.clauductor/panel/verified.json` keeps it across restarts. Until then the warning says how
  many of three it has. Two agents a `PostToolUse` names but no `SubagentStart` announces, or
  three `PostToolUse` that name no agent and no confirmation, mean the shape changed: the warning
  says what broke and the lists stay approximate until the panel is updated. It checks the
  pairing only; the recorded fixtures and the other 2.1.284 observations (the missing
  `SessionStart` HTTP hook, the trust dialog's absence from `claude agents`) are re-recorded by
  hand when one of them breaks.

## Security model

A dashboard of your sessions is private, and a browser terminal is a shell, so the page is
locked down even on loopback. Loopback is not a trust boundary: any web page you open can
send requests to `127.0.0.1`.

- **Loopback only.** The server binds `127.0.0.1:<port>` and `[::1]:<port>` (one server, one
  state) and refuses to run if either bound address is not loopback. If the port is taken on
  either address, it **exits with an error** and never falls back to another port (the hooks
  post to a fixed URL, and `clauductor.localhost` would reach whatever holds `[::1]`). A machine
  with no IPv6 loopback at all is served on `127.0.0.1` only, and the log says so. A refused
  start (port taken, or another live panel on the machine) does not touch `settings.json` or the
  marker.
- **The address is `http://clauductor.localhost:<port>`.** macOS and every current browser
  resolve `*.localhost` to loopback with no system change (no `/etc/hosts` entry, no port 80).
  Cookies are per host, so the token exchange happens at that name.
- **Per-launch token.** Each start makes 32 random bytes and opens
  `http://clauductor.localhost:<port>/?t=<token>`. The server swaps the token for an `HttpOnly;
  SameSite=Strict` cookie and redirects to `/`, so the token leaves the address bar. Every route
  except `/hook`, `/status` and `/healthz` needs the cookie (401 otherwise). `/healthz` answers
  only `ok` and the panel's PID. Under launchd, the token persists in a 0600 file instead (see
  [The launchd agent](#the-launchd-agent)).
- **DNS rebinding and cross-site requests.** `Host` must be exactly one of `127.0.0.1:<port>`,
  `localhost:<port>`, `[::1]:<port>` and `clauductor.localhost:<port>`, plus any name in the
  config's `host_names` (each one lower-case label followed by `.localhost`), on every route. There
  are no wildcards: `evil.localhost` and `clauductor.localhost.evil.com` are refused. Names compare
  case-insensitively (RFC 9110), so `CLAUDUCTOR.localhost` is accepted. A `*.localhost` name
  cannot be an attacker's DNS name, because browsers never ask DNS for it. Every state-changing
  request (every `POST`) must carry an `Origin` equal to `http://` + the `Host` of that same
  request, so a page on `localhost` cannot drive the panel at `clauductor.localhost`, or the
  reverse. No CORS headers are sent. The page is served with
  `X-Frame-Options: DENY` and `Referrer-Policy: no-referrer`.
- **Content Security Policy: this origin only.** `script-src 'self'`, `font-src 'self'`,
  `connect-src 'self' ws://<host>`, and no `'unsafe-inline'` anywhere. The page's JS and CSS are
  files embedded in the binary. So are xterm.js and the themes' fonts. The page loads nothing from
  a CDN or Google Fonts. xterm.js creates `<style>` elements at run time, so `style-src` allows
  one per-response nonce as well; `panel.js` stamps it on those elements. xterm's renderer also
  colours cells through a `<span>`'s `style` attribute (truecolor, and colours lifted to
  `--term-min-contrast`), which the CSP blocks. `static/xterm-style.js` routes exactly those
  writes through CSSOM, which the CSP does not govern: only on a `<span>` that is detached or
  inside `.xterm`, and only for a value made of `color` / `background-color` declarations with a
  hex or `rgb()` value. Any other style attribute still meets the CSP.
  `TestXtermStyleRouteIsNarrow` runs it in node (skipped where node is absent).
- **The terminal endpoint** (`GET /ws/term?project=<id>&lane=<id>`) is a shell into a lane, and it is the most
  guarded route. It needs all of the following:
  - the Host check;
  - the cookie;
  - an `Origin` exactly equal to `http://<the Host>`, meaning scheme, host and port;
  - **a single-use ticket**. The page gets one from `POST /api/p/<project>/lanes/<id>/ticket`, which checks
    `Origin`. A ticket is valid for 30 s, for that one lane of that one project (a ticket for a/x never opens b/x), and is sent in the
    `Sec-WebSocket-Protocol` header, never in the URL. The cookie alone is not enough, because
    cookies are not isolated by port (RFC 6265 §8.5). A page on another loopback port, such as a
    dev server on `:3000`, is same-site, and the browser sends it the panel's cookie. So is a page
    on another `*.localhost` name, such as `evil.localhost`. Only the panel's own page can read a
    ticket. A test checks that a request from another loopback port cannot open a terminal.
  - a valid lane id that names a running lane.

  **A lane's terminal is a shell.** Claude runs `!` commands, and anyone who can type into the
  terminal can do what the lane's user can. Everything above exists so that only you can type
  into it. On the panel's own tmux socket the server never loads `~/.tmux.conf` (`-f
  /dev/null`), and every lane start sets `prefix None`, `prefix2 None` and unbinds the prefix
  and root tables. One root binding comes back: `WheelUpPane`, as tmux ships it (into copy
  mode, or to the program if it asked for the mouse), so the wheel scrolls history. Clicks and
  the right-click menu (which offers kill-pane and respawn-pane) stay unbound. The same pass sets
  `mouse on`, `status off` and `terminal-features[99]` to `xterm-256color:hyperlinks`: tmux
  strips OSC 8 hyperlinks unless the client's terminal has that feature, and no default entry
  gives it to the viewers' `xterm-256color`. It lets links through, never a command; the page
  decides what a link may do (see [The page](#the-page), links in the terminal). `-f` only
  applies when the panel starts the server, so the same settings are applied again whenever the panel finds its socket's lane set changed, every 30 s
  while lanes run, and before every viewer attaches. A server someone else started there, with
  their `~/.tmux.conf` bindings, is stripped too. A lane's viewer therefore cannot use tmux keys
  to switch to another lane or reach tmux's command prompt and `run-shell`.

  After the upgrade, the browser may send only `{"type":"input","data":…}`,
  `{"type":"resize","cols":…,"rows":…}`, `{"type":"alive"}` and `{"type":"focus","focused":…}`
  (which silences that lane's notifications); anything else closes the connection. The browser
  never sends a command. The server runs one fixed argv per viewer: `tmux -u -L <socket>
  attach-session -t =<id>`, where `=` makes the match exact. Stopping a lane closes its viewers.
  Closing a viewer only detaches its tmux client.
  - **Idle pages lose their terminals.** While the page is visible it sends `alive` once a
    minute. A terminal that hears nothing for 5 minutes (the page is hidden, asleep or gone) is
    closed with code 4000. When the page is back in view it reopens each terminal with a fresh
    ticket.
  - **Token rotation.** `clauductor panel rotate-token` writes a new token file; `install`
    does too. The running agent notices within 2 s. At once, every cookie for the old token
    gets 401, every terminal closes with code 4001, every event stream ends, and every ticket
    not yet used is dropped. A terminal whose upgrade passed the cookie check just before the
    rotation is closed the moment it registers. Then `clauductor panel open` opens the page with
    the new token.
- **Terminal output is untrusted.** xterm.js renders it to its own DOM, and the page never passes
  it to `innerHTML`. A link that a lane prints opens only on ⌘-click (Ctrl-click off a Mac), and
  only for `http`/`https`. A plain-text URL, or an OSC 8 link whose text is its own address,
  opens directly: what you clicked is where it goes. Any other OSC 8 link opens only after an
  in-page confirmation that shows its real address, since its text can say anything. The
  plain-text matcher is `static/term-links.js`; `TestTermLinks` runs it in node against URLs a
  lane may print, other schemes and look-alike link text. Title escapes are ignored.
- **Remote control** (PANEL-19) reaches past this page's guards: see [Remote control](#remote-control).
- **A dropped image** (`POST /api/p/<project>/lanes/<id>/image`, PANEL-15b) is a lane action like the
  others: the cookie, this page's `Origin`, a valid lane id naming a running lane. The body is
  the image's bytes. A page elsewhere cannot send it at all: a cross-origin request with an image
  body needs a CORS preflight, which the panel never answers. The bytes decide what it is (PNG,
  JPEG, GIF or WebP by their magic numbers; the name and `Content-Type` are ignored), and it is
  refused over 20 MB. The name the page sends only names the file: letters, digits, `_` and `-`,
  60 characters at most, with the extension the bytes are. The file is written 0600 to
  `~/.clauductor/panel/<project hash>/uploads/<lane>/<ms>-<name>` (directories 0700), **never
  into the worktree**, where it would dirty git. Its path is typed with `tmux set-buffer` and
  `paste-buffer -p` (bracketed when claude asked for it), then a space; no Enter. A lane's images
  are removed when it is stopped or forgotten, and any image older than 24 hours when the next
  one is dropped and when the panel starts. The panel never reads an image back.
- **Lane control is fixed verbs on validated ids.** start, stop, close, interrupt, restart, resume,
  forget, terminal-app, remote-control (PANEL-19), restore-all, queue cancel and queue run (a queue id and a worktree from
  `git worktree list`; the command comes from the trusted config, never the browser). A start
  names a lane type (checked against the config), a mode, a lane name, and for "existing" a path,
  which must be one of `git worktree list`'s. Unknown JSON fields are refused.
- **Close lane** (`POST /api/p/<project>/lanes/<id>/close`, PANEL-17) passes the same guards as
  stop: the cookie, this page's `Origin`, a valid lane id. Its body is `{"dryRun": true}` (the
  plan the confirmation lists) or two flags, `worktree` and `branch`: what the confirmation
  offered to remove. It carries no path, no branch name and no command; the worktree and branch
  are the lane's own, from the registry and `git worktree list`, and each is removed only if a
  check made at that moment allows it (see [Close lane](#close-lane)). Nothing is forced.
- **Remove a worktree** (`POST /api/p/<project>/worktrees/remove`, PANEL-18) passes the same
  guards: the cookie, this page's `Origin`, a strict body (unknown fields are refused). It is
  served under `/api/p/<project>/` only. The body is `{"worktree": <key>, "dryRun": true}` for the
  plan, or `{"worktree": <key>, "remove": …, "branch": …}` with what the confirmation offered.
  The key is the one the page's state gave the worktree (its resolved path), and it must be an
  **exact** entry of that project's `git worktree list` at that moment: anything else (a relative
  path, a path inside a worktree, the same path spelled otherwise, a path the list lacks) is
  refused before any command runs in it. It carries no branch name and no command; see
  [Remove a worktree](#remove-a-worktree) for what is checked.
- **The ingest endpoints** (`/hook`, `/status`) take no token, since a session cannot know it.
  They accept `POST` from a loopback peer only, refuse any request carrying `Origin` or
  `Sec-Fetch-Site` (Claude Code sends neither; a browser always does), cap the body at 256 KB,
  answer `204` before processing, and never execute anything. The worst a local process can do
  is post fake lane events.
- **Every route is a project's** (PANEL-16). The actions are served under
  `/api/p/<project>/…` (lanes, a lane's actions, close, image and ticket, restore-all, the queues, refresh,
  and since PANEL-18 worktrees/remove, which has no path without the project), and
  the streams take `?project=<id>` (`/events`, `/api/state`, `/ws/term`). Each passes the same
  guards as before: the Host check, the cookie, and for a POST this page's `Origin`; a test checks
  every one of them. A project the panel does not serve is 404. The paths before PANEL-16
  (`/api/lanes/…`, `/api/queues/…`, `/api/refresh`) reach the default project for one release, so
  a page left open across the upgrade keeps working. `host_names` is the union of the default
  project's and every trusted project's, so an untrusted config cannot add a name.
- **Events of no project are set aside.** An event counts only if its session is one of a
  project's, or its `cwd` is inside one of the project's worktrees, as `git worktree list
  --porcelain` reports them. That list is the authority, never a hand-kept list. It is re-read
  every 10 s, and early when a worktree is added or removed or when an event arrives from an
  unknown `cwd`.
- **What the panel writes to disk:**
  - the marker `~/.clauductor/panel/port`, `pid` and `owner.json`, removed on SIGINT/SIGTERM
    while they are still its own;
  - the hook install;
  - the lane registry;
  - the trusted config hash, and the logs of queue RUNs;
  - the spend ledger, `~/.clauductor/panel/<project hash>/spend.json` (0600, PANEL-19): dollars
    a day by lane type, model and branch for 120 days, and each session's last total for 14 days;
  - images dropped on a lane's terminal, for at most 24 hours (`uploads/`, above);
  - `projects.json`, when a project is added (`panel add`, `install --project`, or a panel
    started on a project it does not list) or removed; a refused start writes nothing;
  - `economy.json`, economy mode's switch, only when `quota_economy` is set (see *Economy mode*);
  - the last quota (`quota.json`, with a one-way hash of the account's organisation id, never
    the email or the organisation's name), and a Claude Code version verified from live hooks;
  - under launchd, the token, the logs, the copied binary and a browser-opened timestamp.

  Hook bodies include prompt text. The panel keeps only a short one-line summary per event, in a
  200-event in-memory ring buffer. It never reads transcript files; a test fails if any panel
  source mentions one.

### Config trust

`panel.json` is in the repository, and it names commands the panel runs (cards, queue RUN, the
metrics command) and
prompts it types (templates). Anyone who can change the repository can change them, so the panel
runs them only for the exact bytes you trusted. Trusting records the file's SHA-256 under
`~/.clauductor/panel/<project hash>/trusted-config.json`, and the panel logs the hash at every
start. A config the panel has never seen (a fresh clone, or the file `panel init` just wrote) is
**not** trusted by running it, and neither is one that changed (a pull, say): the panel still
starts, but its cards, queue RUN, metrics command and templates stay **off**, under a red **Config untrusted**
banner that names the hash (and the trusted one it replaces), until you review the file and run
`clauductor panel trust` (a running panel follows within 5 s) or start with `--trust-config`.
`clauductor panel install` trusts the config it installs. Both commands print the hash they
record and everything it trusts: each card's command, each queue's RUN command, each
template's first prompt, and the metrics command. There is no trust button in the page:
trusting is a command you run after reading the file.

### Remote control

PANEL-19. Remote Control is Claude Code's own feature, and it reaches past everything above: a
session connected to it can be driven from claude.ai/code or the Claude app on **any device
signed in to your account**, which can send it prompts and answer its permission prompts. So
whoever holds your account's session on a phone holds a shell on this Mac through that lane.
Traffic goes out over TLS through Anthropic's API (no port is opened here), and while connected
the transcript is kept on Anthropic's servers. It needs a claude.ai subscription login, and an
organisation can turn it off (`disableRemoteControl`).

The panel only chooses where it is on, once, at `panel install` (see *The launchd agent*):

- **all** sets `remoteControlAtStartup` to `true` in your user `~/.claude/settings.json` (a
  project's `.claude/settings.json` cannot turn it on, only off), so every interactive session
  on the Mac connects;
- **lanes** adds `--remote-control` to each lane's argv, before `-n` (never with its optional
  name argument; `-n` names the session), read at every start and restart;
- **off**, or an explicit `remoteControlAtStartup` of your own, leaves Claude Code as it is.

The lane's header says **Remote: on, the panel's lanes** (or **every session**), and `panel list`
names the mode. In lanes mode a running lane's **⋯** has **Remote control**: after an in-page
confirmation, and only while `claude agents` reports the lane idle (read again just before the
Enter, as Stop's `/exit` is), `POST /api/p/<project>/lanes/<id>/remote-control` types
`/remote-control` and Enter. That connects a lane started before the choice, or shows a connected
one's status. It passes the same guards as the other lane actions and takes no body. The first
time on a machine claude asks, in the terminal, to confirm Remote Control.

## Operations

### One panel per machine

**One panel per machine, enforced.** The hook URL in `~/.claude/settings.json`, the files in
`~/.clauductor/panel/` (`port`, `pid`, `owner.json`, `token`) and the launchd label are all
per-machine, so a second panel (say `--port 4394` for another project) would re-point every
session's hooks at itself. A running panel therefore holds `flock(2)` on
`~/.clauductor/panel/lock` for its whole life; the kernel releases it on any exit, `SIGKILL`
included, so it can never go stale. A panel refuses to start, and exits non-zero, while another
process holds that lock, or while `~/.clauductor/panel/pid` names another **live** panel from
before the lock. Of two panels started at the same instant, exactly one runs. The message names
the running panel's project, pid and port. Live means: the pid is running, and its start time
equals the one recorded in `owner.json` (a different start time is a reused pid, so the old record
is stale and is taken over). A panel from before `owner.json` counts as live only if its port
answers `/healthz` as that pid. The login agent is not refused: when a hand-started panel holds
the machine, it logs "waiting for the running panel to exit" once, blocks on the lock, and takes
over as soon as that panel stops. (Only a live panel from before the lock, which it cannot wait
on, makes it log why and exit 0; start it again with `launchctl kickstart
gui/<uid>/com.clauductor.panel` once that panel has stopped.) A panel started by hand is still
refused at once. One panel serves every project (see [Projects](#projects)), so there is no
reason for a second.

### The launchd agent

```bash
clauductor panel install --project ~/Development/app          # add --app for a Dock/Spotlight launcher
clauductor panel install                                      # once projects.json holds a project
```

`install`:

1. With `--project`, adds the project (if it is not) as the default, and trusts its config as it
   is now. Without it, `projects.json` must hold a project. Either way it loads the default
   project's config and refuses if it is missing or invalid, rather than crash-looping later.
2. Copies the running binary to `~/.clauductor/panel/bin/clauductor`. The agent never runs from
   a build directory or a worktree that may disappear. Re-run `install` after upgrading
   clauductor.
3. Writes a new persistent token to `~/.clauductor/panel/token` (0600, directory 0700). Every
   install rotates it, so reinstalling also revokes the old one.
4. Writes `~/Library/LaunchAgents/com.clauductor.panel.plist` and checks it with `plutil -lint`.
   The plist sets:
   - `RunAtLoad`;
   - `KeepAlive` with `SuccessfulExit = false`, so a crash restarts the panel, throttled to once
     every 30 s, and `launchctl bootout` stops it for good;
   - `LimitLoadToSessionType = Aqua`;
   - logs in `~/.clauductor/panel/logs/`;
   - `LANG=en_US.UTF-8`;
   - a PATH made of `/opt/homebrew/bin`, `~/.local/bin`, the directories where `claude`,
     `tmux`, `git`, `gh`, `node` and `jq` were found at install time, and the system
     directories. launchd's default PATH has none of these.
5. Replaces any loaded copy (`launchctl bootout`), then runs `launchctl bootstrap gui/$UID`.

Then it settles **remote control** (PANEL-19, see [Remote control](#remote-control)):
where Claude Code's Remote Control is on. It asks once, on a terminal, and only when
`~/.claude/settings.json` has no `remoteControlAtStartup` (an explicit `true` or `false` is
your own choice, and is never asked about):

```text
  1) Every Claude session on this Mac (sets remoteControlAtStartup in ~/.claude/settings.json)
  2) Only the panel's lanes (lanes start with --remote-control)
  3) Not now
Choose 1, 2 or 3 [3]:
```

`--remote-control=all|lanes|off` answers without asking; with no terminal and no flag it is
off, unasked. The answer is kept in `~/.clauductor/panel/remote-control.json` (0600), so a
later `install` does not ask again (the flag changes it). **all** merges that one key into
`settings.json` through the same read-merge-atomic-write the hooks use (every other key keeps
its value and place; a backup from before the panel's first change is
`settings.json.clauductor-panel.bak`), and prints the change and how to undo it. **lanes** leaves
`settings.json` alone: every lane the panel starts or restarts runs `claude --remote-control`.
`panel list` names the mode.

Since PANEL-16 the plist runs `clauductor panel --port <port> --launchd`, from your home
directory: the agent serves `projects.json` as it is. A plist written before names `--project
<path>`; it keeps working, and at its first start the panel adds that project (if it is not) and
makes it the default. Re-run `install` to write the new form.

Under launchd, the panel runs with `--launchd`:

- It uses the persistent token, and the cookie lasts 30 days.
- The token never goes to stdout or stderr, which are the log files. The log names the token
  file instead.
- At each start it opens the browser once, with the token. Another start within 5 minutes does
  not open it again, so a crash loop cannot fill the browser with tabs.

`--app` also builds `~/Applications/Clauductor Panel.app`, an AppleScript applet made by
`osacompile` that runs `clauductor panel open`. Put it in the Dock, or find it with Spotlight.

```bash
clauductor panel open         # checks /healthz on both loopbacks, then opens http://clauductor.localhost:4393/?t=<token>
clauductor panel uninstall    # bootout, then remove everything the agent owns
```

`open` sends the token only to the panel it expects. `/healthz` answers `ok pid=<pid>`, and
`open` compares that with `~/.clauductor/panel/pid` on **both** `127.0.0.1` and `[::1]`, because
the browser resolves `clauductor.localhost` to `::1` first. On a mismatch at either address it
refuses rather than hand the token to whatever holds the port. If nothing listens on `[::1]`, it
opens `http://127.0.0.1:<port>/` instead.

### Uninstall

`clauductor panel uninstall` removes:

- the plist, the token and the copied binary;
- the logs, `pid`, `port`, `owner.json` and the browser-opened stamp;
- the app, but only if `install --app` made it;
- every lane registry that lists no lanes.

A registry that still lists lanes is kept, and `uninstall` says so: those lanes may still run in
tmux, and Resume needs their session ids. It never touches lanes. The panel's hooks stay in
`~/.claude/settings.json`; `clauductor panel --uninstall-hooks` removes them.

### Dependencies

| What | Version | Licence | Why |
|---|---|---|---|
| `github.com/coder/websocket` | v1.8.15 | ISC | WebSocket server. It is the maintained successor of nhooyr.io/websocket, with no dependencies of its own and a `context`-based API that fits the panel's shutdown. It negotiates subprotocols, which carry the terminal ticket, and it re-checks `Origin` itself as a second layer. gorilla/websocket was the alternative; it was archived for a time, and its API predates `context`. |
| `github.com/creack/pty` | v1.1.24 | MIT | One PTY per viewer's `tmux attach`. This is why the panel needs no node-pty. |
| `@xterm/xterm` | 6.0.0 | MIT | The terminal in the page. It is vendored as `web/vendor/xterm/xterm.js`, `xterm.css` and `LICENSE`, and embedded with `go:embed`. |
| `@xterm/addon-fit` | 0.11.0 | MIT | Fits the terminal to its box. Vendored the same way. |
| Chakra Petch, IBM Plex Sans, JetBrains Mono, Newsreader, Public Sans, DM Mono, Barlow, Barlow Semi Condensed, Red Hat Mono, Courier Prime, Atkinson Hyperlegible Next, Atkinson Hyperlegible Mono, Big Shoulders Stencil, Archivo, Martian Mono | fontsource 5.3.0, latin | SIL OFL 1.1 | The themes' fonts, in `web/static/fonts/` with their licences. About 470 KB in all; a face downloads only when the active theme uses it. |

To update a vendored file, download it with `npm pack <package>@<version>`, copy the file from
`lib/` (or `files/` for fonts) together with its `LICENSE`, and update this table.

## Troubleshooting

### The panel is down

The lanes are not affected: they run in tmux whether or not the panel is up.

1. `clauductor panel open` tells you whether the panel answers on its port.
2. Restart it with `launchctl kickstart -k gui/$(id -u)/com.clauductor.panel`. See its state with
   `launchctl print gui/$(id -u)/com.clauductor.panel`.
3. Read `~/.clauductor/panel/logs/panel.err.log`. The usual causes are a port another process
   holds, or a config that moved. In both cases the log says which.
4. To reach a lane with no panel, run `tmux -L <socket> ls` (the project's socket: `clauductor
   panel list` names it; `clauductor` for the first project), then
   `tmux -L <socket> attach -t '=<lane>'`. Quote the target, because zsh expands a bare
   `=word`. The panel's socket has no prefix key (see [Security model](#security-model)), so close
   the window to detach; the lane keeps running.
5. If the config file moved (for example, `--config` pointed into a worktree that was removed),
   run `install` again with the new path.

### The panel will not start

- **"no panel config at …"**: the project has no `.clauductor/panel.json`; `clauductor panel
  init` writes one.
- **"unknown field"**: a misspelt or unsupported key. Check it against the [key table](#keys), or
  let an editor check it against `$schema`.
- **"… needs "version": 2 or later"**: the file declares an older version than a key it uses.
  Raise `version`, or remove the key (see [Versions](#versions)).
- **Another panel is running**: see [One panel per machine](#one-panel-per-machine). The message
  names that panel's project, pid and port.
- **The port is taken**: the panel never falls back to another port; free it, or pass `--port`
  (and `install --port`).

### Cards, RUN and templates do nothing

The config is not trusted as it is now (never trusted, or changed since): a red **CONFIG
UNTRUSTED** banner says which. Review it and run `clauductor panel trust` (see
[Config trust](#config-trust)).

### A lane dies at start

Usually Claude's workspace-trust dialog in a new directory: it defaults to **No, exit**. Press ↓,
then Enter, in the lane's terminal; if claude already exited, STOP the lane and start it again. If
**New lane** is disabled, hover it for the reason (an API key, see
[Subscription only](#subscription-only); or the [quota guard](#quota-guard)).

### A lane shows No hooks

The session is busy per `claude agents` but no hook has come from it for 60 s: usually it never
loaded the hooks. Restart it.

### The page shows no context % or quota

Only the status line carries them: add the [status-line snippet](#the-status-line). It posts
nothing while `~/.clauductor/panel/pid` names no live process. Context % comes only from a
session in the project; the quota from any session on the machine. With none running since the
panel started, the quota is the last one saved (with its age), or none on a first start. An API
key or cloud account has no quota: the page shows its spend instead (see *The account and its
quota*).

### Never kill the panel's tmux server from a script

Every lane lives in one tmux server per socket (`tmux -L clauductor`). The server outlives the
panel on purpose, as an orphan (its parent is pid 1), and killing it kills every lane at once.
A cleanup script that kills orphaned processes by what their command line names did exactly
that: until PANEL-15 the server was started by the first lane's `new-session`, and a tmux server
keeps, for life, the command line of the command that started it, which named that lane's
`.claude/worktrees/<lane>`. Since PANEL-15 the panel starts the server on its own with `tmux -L
<socket> -f /dev/null start-server`, in your home directory, so its command line names no
worktree (a test checks it). A server started by an older panel keeps its old command line until
it next starts. Still: never kill tmux servers, or any process whose command line names `tmux`,
from a script; stop a lane from the page (**Stop lane**), or with `tmux -L clauductor
kill-session -t '=<lane>'` for that lane alone.

### A lease never frees

A waiter that keeps waiting behind a holder is doing its job while the holder lives: the page
shows the holder's lane, pid and age. A holder whose pid was reused and that recorded no start
time cannot be told from a live one, so nothing removes it by itself. Check the holder with
`ps -p <pid>`; if it is not the gate, remove the lock directory (`rm -rf <git common
dir>/clauductor/gate.lock`) and the next waiter goes.

## Testing

```sh
cd framework
go test -short ./...    # the fast suite: about 3.5 s once built
go test -race ./...     # everything, under the race detector: about 15 s once built
```

`-short` skips the tests that drive something real and slow: a tmux server on a
throwaway socket, `lock-run` and `lease.sh` as separate processes, panel processes started
side by side, `node` (the xterm style guard and the terminal's URL matcher), `osascript` and
`plutil`. It still runs the security tests, tmux or not: a token rotation closes terminals
and cookies (twice, once during an upgrade), an idle terminal closes, an untrusted config runs
no command, and a gate on a terminal can use it and gets one Ctrl-C. They take `SecurityTmuxSocket`.

**CI enforces the full suite.** `.github/workflows/test.yml` runs `gofmt -l`, `go vet
./...` and `go test -race ./...` on macOS and Ubuntu, with tmux, on every push to `main` and
every pull request into it. `-short` is for working; CI is the gate. The clock check
(`TestOnlyPackageClockReadsTheTime`) is part of the suite.

The rules the suite keeps, and a new test must too:

- **Never sleep to show that nothing happened.** Drive the loop and assert after N
  iterations of it. A sleep proves only that the machine was fast that day.
  - Code on the panel's clock takes a `clock.Fake`: it moves only when the test calls
    `Advance`, and `BlockUntil(n)` returns once n waits are pending on it, so a loop that makes
    a new timer each iteration has finished that iteration (`framework/internal/panel/clock/fake.go`).
  - A running panel reports each source's polls through `Options.OnPoll`. The integration
    tests count them (`pollCounter`) and run at `fastTicks()`, a tenth of a second or less.
  - `lock-run` helper processes append one line per waiter-loop iteration to
    `LOCKRUN_HELPER_TRACE`, and the shell tests put a counting `sleep` first on `lease.sh`'s
    `PATH`. A waiter that must keep waiting is checked after N of its own looks, and
    `LOCKRUN_HELPER_SKEW` puts its clock an hour ahead instead of waiting out a TTL.
- **Order by events, not by timing.** A test that needs B queued before C starts C once B's
  waiter file exists. A test that rewrites `owner.json` waits for lock-run's own last write to
  it.
- **Run in parallel.** Every test that has its own temp `HOME`, tmux socket and port (`:0`)
  calls `t.Parallel()`. A test that sets the environment (`t.Setenv`) or a package hook
  cannot, and runs first, alone. A panel that runs no lane gets a socket no server runs on,
  never the machine's own panel socket.
- **Leave nothing running.** Every tmux socket comes from `leakcheck.TmuxSocket` (or
  `SecurityTmuxSocket`), which kills its server and removes its file at cleanup, pass or
  fail. Each package that starts tmux or helper processes runs `leakcheck.Main` from its
  `TestMain`: the run fails if a socket of this process is still there afterwards, or a
  child, a copy of the test binary, or a command naming the run's temp directory is still
  alive, and it kills them. A timeout or Ctrl-C kills the run's tmux servers first. Only
  sockets named with this process's pid are touched: a suite someone else runs at the same
  time is left alone.
- A helper process the test binary starts gets `GORACE=atexit_sleep_ms=0`. A `-race` binary
  otherwise sleeps a second at exit.

To show that a fix for a flaky test holds, run it 200 times under the race detector:

```sh
go test -race -run '^TestLockRunTwoProcessesQueue$' -count=200 ./internal/panel/lease/
```

## Not yet

- **Close lane** removes only a clean worktree and a merged branch; there is no way to force it
  from the page, by design. Discard or commit the work first, or remove it with git.
- Adding or removing a project needs a panel restart; so does fixing a project that could not
  load. The Needs-you rows show the project on view only (the menu and the tab title count the
  others). One `claude agents` poll runs per project. Quota thresholds are the 5-hour window's,
  and lanes start only on a subscription login.
- No remote access; the panel is loopback only.
- The panel never answers a permission request. Doing it from the browser would need a
  token-carrying HTTP hook (`headers` plus `allowedEnvVars`), and is not planned.
