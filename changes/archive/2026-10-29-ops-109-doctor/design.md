# Design: ops-109-doctor

## Evidence
Read on 2026-10-24, at `a864a680`.

- **E1. The tool list.** `template/.claude/prereqs.sh` is the one list of tools the model uses,
  with a level (`required`, `recommended`, `optional`), a reason and an install hint for each
  OS (`row`, line 52). It is plain POSIX `sh`, so it runs on a machine that lacks every tool it
  reports. Four callers run it, each printing its report as it is: `install` and `init`
  (`printPrereqs`, `framework/internal/cmd/prereqs.go`; `install.go:156`, `init.go:59`), the
  plugin's `/clauductor:init` (`framework/internal/plugin/assets/scaffold.sh:238`) and
  `install.sh:134`. It prints lines for people and always exits 0, so a caller cannot tell from
  the exit code whether anything is missing. `prereqs.go`'s comment explains why it is a script:
  "one list of tools and hints, and every caller runs it". `prereqs_test.go` drives it through a
  stub `PATH` and `PREREQS_OS`.
- **E2. The template and drift.** `template.Resolve` (`framework/internal/template/resolve.go:63`)
  finds the template and refuses one whose `.template-version` is not the binary's. Its error
  says what to do: install the matching release, set `CLAUDUCTOR_FRAMEWORK`, or pass
  `--template-dir`. A `--template-dir` mismatch comes back as `Source.Warning`, not an error.
  `template.Survey` (`survey.go:72`) is `clauductor diff`'s report. `Report.Converged`
  (`survey.go:61`) is true when no framework file differs or is missing and a merge would not
  change `settings.json`. A `Source.Warning` is copied into `Report.Warnings`. `diff`, `update`
  and `install` all act on the current directory (`diff.go`; `update.go:66`; `install.go:77`), not
  the git toplevel: from a subdirectory `update` refuses ("no .claude/skills/ found") and `install`
  installs into the subdirectory.
- **E3. Whose model a repository runs.** `ownguard.go` decides it for `install` and `update`:
  - the plugin marker means the plugin's (`refusePluginModel`, line 172). `install` and `update`
    check it first and refuse (`install.go:103`, `update.go:72`), so it wins over the install
    marker;
  - the install marker `.claude/clauductor-template`, or signs of the old model, means
    Clauductor's (`ownedByClauductor`, line 50);
  - any of `ownModelSigns` (`AGENTS.md`, `.claude/skills`, `.claude/hooks`,
    `.claude/settings.json`) without either marker means a model of the repository's own, which
    `install` and `update` refuse.
- **E4. The panel's state, readable without changing it:**
  - **Running:** `install.RunningPanel` (`install/singleton.go:163`) returns the live panel's pid
    and port, or nil.
  - **Installed:** the login agent's plist (`install.PlistPath`) and the binary it runs
    (`install.InstalledBinary`, `install/launchd.go:64`, `:70`). Its label and log directory are
    `install.LaunchdLabel` and `install.LogDir`, which follow the brand's names.
  - **Builds:** each binary's build stamp, `install.FileStamp` for a file and
    `install.RunningStamp` for the running process (`install/selfupdate.go:69`, `:78`), giving
    module, revision and modified.
  - **Registry:** `config.LoadProjects` (an error when `projects.json` does not parse) and
    `Projects.Find`, which matches an ID or an exact `Root`, so the path is resolved first
    (`signals.ResolvePath`), as `ConfigPathOf` does.
  - **Config path:** `install.ConfigPathOf` (`install/projects.go:135`), the path every panel
    command reads, a sidecar included.
  - **Config load:** `config.LoadConfigRaw` (`config/config.go:445`), which returns the bytes it
    parsed, "so the trust hash covers what runs rather than a second read".
  - **Trust:** `install.ConfigHash` of those bytes and `install.TrustedNow`
    (`install/trust.go:304`). Trust is the exact bytes, and the sidecar rule applies to a config
    outside its repository. The panel reads trust only for a project it serves.
- **E5. The panel keys everything by the main checkout.** A registry entry's `Root` is "the main
  worktree, resolved" (`config/projects.go:30`). `panel add` finds it as `mainRoot` does
  (`install/candidate.go:111`): the toplevel, resolved, must be the first entry of `git worktree
  list` (`signals.ReadWorktrees`), or it refuses with code `linked-worktree`. A trust record lives under
  `config.ProjectDir(home, project)`. `panel trust` with no `--project` takes `git rev-parse
  --show-toplevel` (`cmd/panel.go:577`), which is the worktree's path when it runs in a lane
  worktree. `panel install` without `--project` installs the registry's default project, and
  with an empty registry refuses: "no project registered" (`install/launchd.go:340`), or when the
  default's config does not load (`:353`). `panel add` and `panel install --project` both
  register through `install.AddProject` (`install/projects.go:41`), which refuses a project with
  no config that loads (`:100`, naming `clauductor panel init`) and one whose tmux socket another
  project holds (`:114`, `socketOwner`). The template's `panel.json` names `"tmux_socket":
  "myproject"`, so a second project installed from it, unedited, is refused. `panel init` takes
  `--project`. `install.ShellWord` quotes a path "for a command line a person copies and runs".
- **E6. The documented failures doctor turns into lines:** `docs/QUICKSTART.md` *Troubleshooting*,
  `docs/guide.md` *When something goes wrong*, and `docs/panel.md` *Troubleshooting* (*The panel
  is down*: `launchctl kickstart -k gui/$(id -u)/com.clauductor.panel` and
  `~/.clauductor/panel/logs/panel.err.log`; *The panel will not start*; *Cards, RUN and templates
  do nothing*: `clauductor panel trust`).
- **E7. A tool the docs name and the list lacks.** `docs/QUICKSTART.md` *Prerequisites* names
  `python3` as optional ("machine-quiet's live-session check; without it, it removes no
  worktree"). `template/.claude/machine-quiet.sh:108` runs it. `prereqs.sh` does not list it.

## Decisions

- **D1. One list of tools: doctor reads `prereqs.sh`, through a new `--rows` mode.**
  **Recommended.** With `--rows`, `prereqs.sh` prints one tab-separated line per tool:
  `ok|missing`, the level, the tool, its path (when present) or this OS's install hint (when
  missing), and the reason, and no heading or closing line. Without the flag it prints the
  report it prints today, D7's row included, so its four callers (E1) need no change. Doctor runs the copy in the template the binary resolves
  (`/bin/sh <template>/.claude/prereqs.sh --rows`, as `printPrereqs` runs it today), so the
  machine half needs no project. When no template resolves, the template check has already failed
  (D2), and the tools line is a note that it could not run. Doctor reads the rows strictly: every
  line must hold five fields with a known status and level, and there must be at least one. A
  report that does not parse (a `--template-dir` template older than `--rows` prints its human
  report) is a note that the tools could not be read, naming the script, never an empty list.
  **Beat (b):** parse the report's human lines (`MISSING  gitleaks  recommended: …`). This needs
  no template edit, but doctor would then depend on wording written for people, and a reword
  would break it.
  **Beat (c):** a Go list of tools in doctor. That makes two lists that drift, which is what
  `prereqs.go`'s comment rules out.
  **Cost of (a):** a template edit ships to every project, so the plugin is rebuilt
  (`scripts/build-plugin.sh`) and this repo's copy is synced (`clauductor update`).

- **D2. What fails, what warns, what is a note.** Doctor exits 1 when any line fails and 0
  otherwise. Its last line counts failures and warnings. **Recommended:**
  - **fail** means Clauductor cannot work here until it is fixed: a required tool missing, no
    template the binary can use, a panel registry that does not load, or a served project's
    panel config that does not load;
  - **warn** means something is degraded: a recommended tool missing, `gh` signed out, the
    panel's login agent installed but the panel down, framework drift, or a config the panel serves and does not trust as it is
    now;
  - **note** is a fact, not a problem: an optional tool missing, no panel installed, projects
    registered with no login agent and no panel running (the user runs it by hand), a panel
    binary of another revision, a project the panel does not serve, no model, a model of the
    repository's own or the plugin's, or a check that could not run, with why.
  **Beat:** exit 0 always, as `prereqs.sh` does. That is a report a setup script cannot act on,
  and a doctor's exit code is its point.
  **Beat:** a `--exit-code` flag, as `diff` has. `diff` reports differences, which are not
  failures. A doctor reports failures, so exiting non-zero on one is its default, not an option.
  An optional tool is a note rather than a warning so that a warning always means something a
  user should act on. For the same reason:
  - **A project the panel does not serve is a note.** A user may not want every repository on the
    panel. The config and trust lines run only for a project it serves, since the panel reads
    neither for one it does not.
  - **A panel binary of another revision is a note.** Doctor cannot tell which build is newer:
    the panel may be behind this `clauductor`, or ahead of it (catch-up rebuilt it at main while
    the user runs a worktree build). The note names both revisions and both paths, and says that
    `clauductor panel install` would copy this binary over the panel's and rotate the page's
    token, to run only when this one is the newer.

- **D3. Doctor changes nothing.** It reads files and runs read-only commands (`git rev-parse`,
  `git worktree list`, `gh auth token`, `prereqs.sh`), reads build information from binaries, and
  asks a live panel's `/healthz` (as `RunningPanel` does). It writes no file in the repository or in
  `~/.clauductor`, and starts no process that outlives it. **Recommended.**
  **Beat:** `--fix`, running the safe fixes. Every fix doctor prints is a command whose user
  should see what it does:
  - `panel trust` shows what a config runs before trusting it, and that review is what trust is
    for;
  - `update --dry-run` shows its diff;
  - `panel install` rotates the page's token.
  Running them unseen would remove the review each one exists to give.

- **D4. Doctor does not run the project's process checks.** When the repository holds
  Clauductor's model, the last line names `sh .claude/checks/run.sh` for the model's own
  consistency. **Recommended.** The checks are the gate's *process checks* step
  (`scripts/ci/steps.sh`). A doctor that runs them is a second, slower gate whose output is not a
  receipt.
  **Beat:** run them, or a `--checks` flag.

- **D5. No `--json`.** **Recommended.** Rule 3 asks which production process calls a capability,
  and nothing would call this one: no panel card, CI job or skill reads doctor's output. Add it
  with its first caller.
  **Beat:** `--json` now, as `diff` has, "for scripts".

- **D6. Where the project half looks.** **Recommended:**
  - **Model and drift:** read at the git toplevel of the current directory. That is the files
    being worked on, so doctor gives the same answer from any subdirectory. `diff` reads the
    current directory (E2), which differs only below the toplevel.
  - **The panel's lines:** read at the main checkout, found as `mainRoot` finds it: the first
    entry of `git worktree list`, resolved, and that path is the one looked up in the registry.
    The registry and trust are keyed by the main checkout (E5). When that first entry is a bare
    repository, the panel cannot serve it, and the panel's project lines are one note saying so.
  **Beat:** the toplevel for both. From a lane worktree that reports the project unregistered and
  untrusted when it is neither, and the fix it prints would be refused.
  **Beat:** the parent of `git rev-parse --git-common-dir`. That is wrong for a submodule
  (`.git/modules/<name>`) and a `--separate-git-dir` repository, where `panel add` would refuse
  the path doctor printed.

- **D7. `python3` joins `prereqs.sh`'s list as optional,** with QUICKSTART's reason (E7), and
  joins the Linux hint `sudo apt install …` (`hint` has no Linux fallback, so without it a Linux
  report would print an empty hint; macOS takes the `brew install` fallback). **Recommended.** D1
  makes that list everything doctor knows about tools, so a tool the docs name and the list lacks
  is a tool doctor never reports. The line goes in the file group 1 edits anyway.
  **Beat:** leave it to a fix lane. Until then doctor and `install` would say nothing about it.

- **D8. Every fix doctor prints works where it is printed, in the state doctor found.**
  **Recommended.** A fix is a command the user can paste as it stands, every path in it quoted
  with `install.ShellWord`:
  - **Repository fixes** (`update --dry-run`, `update`, `install`) act on the current directory
    (E2), so when doctor runs below the toplevel they are printed as `cd <toplevel> && clauductor
    …`. When doctor was given `--template-dir`, they carry it too, so they compare against the
    template doctor judged.
  - **Panel fixes** carry `--project <main checkout>`. `panel add` refuses a linked worktree,
    `panel trust` without `--project` acts on the worktree, and `panel install` without it
    refuses an empty registry (E5).
  - **Registering is asked of `AddProject`'s own checks.** The `install` package gains a
    read-only `CheckAdd`, the checks `AddProject` runs before it saves (the config loads, the id
    is free, no other project holds the socket), and `AddProject` calls it, so the two cannot
    drift apart. Doctor prints `panel add` or `panel install --project` only when `CheckAdd`
    passes. Otherwise it prints what clears the refusal: `clauductor panel init --project <main
    checkout>` when there is no config, or the refusal itself (a config that does not load, or a
    socket another project holds: "give this project its own tmux_socket in <path>"). The bare
    `panel install` is printed only when the registry's default config loads.
  - **No fix is printed that the command would refuse.** A plugin model gets no `update`, and an
    unknown direction gets no unconditional `panel install` (D2).
  - **One placeholder.** Outside a repository there is no project path to name, so the
    not-installed note tells the user to run `clauductor panel install --project` with a
    repository's path.
  Each fix scenario asserts the exact command.
  **Beat:** copy `AddProject`'s refusals into doctor. Two readings of one rule drift apart, and
  rounds 1 and 2 of this proposal's review each found a refusal the list had missed.
  **Beat:** the bare commands, as the troubleshooting lists give them. Run from a subdirectory or
  a lane worktree, they fail or act on the wrong directory, and the signal in `proposal.md` (the
  fix clears the problem on a re-run) would miss.

## The checks

Doctor accepts `--template-dir`, as `install`, `update` and `diff` do (`addTemplateFlag`), so it
judges the template those commands would use. It takes no other flag.

| Group | Line | When | ok | Problem → level and fix printed |
|---|---|---|---|---|
| Machine | one per tool | always | present, with its path | missing: required **fail**, recommended **warn**, optional **note**. Fix: the row's install hint |
| Machine | template | always | dir, how found, version (`Source.String`) | `Resolve` error → **fail**, its message (which names the fix); `Source.Warning` → **warn** |
| Machine | gh sign-in | `gh` present | signed in | `gh auth token` exits non-zero → **warn**: `gh auth login`. Its output is discarded, never printed |
| Machine | panel registry | macOS, `projects.json` exists | loads | `LoadProjects` error → **fail**: its error (the panel cannot start), and the panel's project lines are a note that they could not run |
| Machine | panel | macOS | — | no plist and no registered project → **note**: not installed (optional). In a repository, the fix D8 picks through `CheckAdd`: `clauductor panel install --project <main checkout>`, or `panel init --project` first, or the refusal. Outside one, run `clauductor panel install --project` with a repository's path |
| Machine | panel running | macOS, installed | pid and port | login agent installed and not running → **warn**: `launchctl kickstart -k gui/$(id -u)/<install.LaunchdLabel>`, and `<install.LogDir>/panel.err.log`. No login agent and not running → **note**: the panel runs by hand; `clauductor panel install` adds the login agent, printed only when the registry's default config loads (D8) |
| Machine | panel build | macOS, login agent's binary present | same module and revision | different → **note** (D2): both short revisions and both paths, `modified` named when set, and `clauductor panel install` as the way to put this binary in the panel when it is the newer (it rotates the page's token). Either stamp without a revision → **note**: cannot compare |
| Project | repository | always | the toplevel | not in a git repository → **note**: the project checks did not run |
| Project | model | in a repository | Clauductor's (E3) | the plugin's (its marker wins, E3) → **note** (update it with the plugin, `docs/plugin.md`); its own → **note** (`install` and `update` refuse here; `clauductor diff` compares it); none → **note**: `clauductor install` adopts one (D8: from the toplevel). Drift is checked only for Clauductor's |
| Project | drift | Clauductor's model | matches template `<version>` | not `Converged` → **warn**: counts of framework files that differ and are missing, whether `settings.json` would change, then `clauductor update --dry-run` to see each change and `clauductor update` to apply them (D8: from the toplevel). Each `Report.Warnings` entry other than the template line's own `Source.Warning` → **warn**. Survey error → **note**: see the template line |
| Project | panel serves it | macOS, panel installed | registered | not in the registry → **note**, with the fix D8 picks through `CheckAdd`: `clauductor panel add --project <main checkout>`, or `clauductor panel init --project <main checkout>` first, or `AddProject`'s refusal |
| Project | panel config | macOS, served | loads | `LoadConfigRaw` error → **fail**: its error, and `docs/panel.md` *The panel will not start*. No config at `ConfigPathOf` → **note**: `clauductor panel init` writes one |
| Project | config trusted | macOS, served, config loads | trusted as it is now | `TrustedNow` false → **warn**: cards, Run and templates do nothing until it is reviewed and trusted, `clauductor panel trust --project <main checkout>` |

The last line is `<n> failed, <m> warning(s)`, or `Nothing missing or broken.` When the
repository holds Clauductor's model, it adds `The model's own checks: sh .claude/checks/run.sh`
(D4).

## Where a user meets doctor

- **`install` and `init`:** `printPrereqs` prints one line after the report, naming `clauductor
  doctor` for checking the machine and the project again. The line is printed by the Go caller,
  not added to `prereqs.sh`, because the plugin's `/clauductor:init` and `install.sh` run the
  same script, and a plugin user may have no `clauductor` binary.
- **Docs:** the README's command table gets a row. `docs/QUICKSTART.md` names doctor under
  *Prerequisites* and at the top of *Troubleshooting*, and `docs/panel.md`'s *Troubleshooting*
  names it as the first thing to run. The troubleshooting entries stay: doctor finds a problem,
  and the entry explains it.

## Refusals

| Situation | What happens instead |
|---|---|
| Run outside a git repository | The machine half runs. The project half is one note saying it did not run |
| No template the binary can use | The template line fails with `Resolve`'s message. The tools line and the drift line are notes saying they could not run |
| A repository running its own model, or the plugin's | A note saying so. No drift line, because `install` and `update` refuse such a repository and their fix would not apply |
| A project the panel does not serve | One note with the `panel add` fix. No config or trust line: the panel reads neither for it |
| A repository whose first worktree is bare | One note: the panel serves only a main worktree. No panel project lines |
| Linux, or WSL2 | No panel lines: the panel runs on macOS |
| A problem doctor could fix itself | It prints the fix and runs nothing (D3) |
