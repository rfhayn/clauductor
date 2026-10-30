**Approved:** 2026-10-24 by Rich · design 92ec82fb58f7
**Roadmap row:** OPS-109
**Risk:** low

## Why
When something in a Stridal setup is missing or broken, nothing says so in one place. Each
piece reports, if at all, where it lives:
- `stridal install` and `init` run `.claude/prereqs.sh`, which lists missing tools with an
  install hint. It runs once, at install, and always exits 0 (REL-2).
- A template that does not match the binary is found only when `install`, `update` or `diff`
  refuses to run (`template.Resolve`).
- A project that has drifted from the template is found only by running `stridal diff`.
- The panel's problems (not running, running an older build than the CLI, a project it does not
  serve, a config it does not trust) are spread over three troubleshooting lists:
  `docs/QUICKSTART.md`, `docs/guide.md` and `docs/panel.md`. The user has to know which one to
  read.

For v0.1.0, a stranger installs Stridal on a machine nobody on the team has seen (the team's
joint launch cut of 2026-10-19). The owner's decision of 2026-10-21 puts one command in the
release that checks the machine and the project and says what to do about each problem.

## What changes
`stridal doctor`, run in any directory, prints one line per check, grouped *Machine* and
*Project*. Every problem it finds comes with what to do about it. It changes nothing (D3). It
exits 1 when a check fails and 0 otherwise (D2).

- **Machine:**
  - **Tools.** Every tool `prereqs.sh` lists, read from a new `--rows` mode of the same script
    (D1). `python3` joins the list (D7).
  - **Template.** Whether the binary finds a template it can use, and which one.
  - **gh sign-in.** Whether `gh` is signed in, checked offline.
  - **Panel** (macOS only). Whether it is installed and running, and whether it runs the same
    build as this `stridal`.
- **Project** (inside a git repository only):
  - **Model.** Whether the repository holds Stridal's operating model, a model of its own, the
    plugin's, or none.
  - **Drift.** Whether its framework matches the template. This is `stridal diff`'s verdict.
  - **Panel config** (macOS only). Whether the project's panel config loads, whether the panel
    serves the project, and whether it trusts the config as it is now. These are read from the
    main checkout even when doctor runs in a lane worktree (D6).
- **Every fix works where it is printed** (D8): a repository fix starts `cd <toplevel> &&` when
  doctor runs below it, and a panel fix names `--project <main checkout>`.
- **Where a user meets it:** install's and init's prerequisites report ends with a line naming
  doctor. The README's command table, `docs/QUICKSTART.md` (*Prerequisites* and
  *Troubleshooting*) and `docs/panel.md`'s *Troubleshooting* name it too.
- **Beyond the row's scope column** (`framework/internal/cmd`, `docs/`):
  `template/.claude/prereqs.sh` (D1, D7), the `plugin/` built from it, this repository's own
  `.claude/prereqs.sh`, `README.md`, and `framework/internal/panel/install/projects.go`
  (`CheckAdd`, D8).

## What the existing specs already guarantee
None of the living specs covers the CLI's install-side commands. All seven are the panel's
(`panel-attach`, `panel-catch-up`, `panel-claude-calls`, `panel-lane-context`,
`panel-lane-measures`, `panel-new-lane`, `panel-stall`). Doctor reads the panel's state and
changes none of it, so it modifies no requirement:
- **ATTACH-3:** a config outside its repository is judged by the sidecar rule. Doctor's trust
  line asks the same question `install.TrustedNow` answers, sidecar rule included, so it never
  calls a config trusted that the panel would refuse.
- **CATCHUP-1 to CATCHUP-4:** they keep the owner's main checkout and installed panel current on
  this Mac. Doctor's *panel build* line is a note naming both builds (D2), since a panel catch-up
  rebuilt can be ahead of the `stridal` running doctor.

This change adds a new capability, `doctor` (DOCTOR-1 to DOCTOR-3).

## Out of scope
- **Claude Code's own version, and the payload shapes the framework reads:** OPS-107, the compat
  probe.
- **The project's process checks** (`sh .claude/checks/run.sh`): they are the gate's step
  (`scripts/ci/steps.sh`, *process checks*). Doctor names the command in its last line and does
  not run it (D4). This is a refusal, not a deferral.
- **Fixing what it finds** (`--fix`) and **`--json`:** refusals (D3, D5).
- **The panel on Linux:** the panel runs on macOS (`prereqs.sh`'s `tmux` row), so the panel lines
  run there only.
- **Tool versions** (QUICKSTART's `git` 2.31+): `prereqs.sh` checks that a tool is present, not
  which version, and doctor reads what it reports. No row asks for a version floor.
- **The status-line snippet, and an API key in the environment:** the panel already shows both
  where the user is looking. A missing quota or context % shows on the page, and the **New lane**
  button gives its reason on hover (`docs/panel.md`, *Subscription only*). PANEL-52 may change the
  API-key rule. This is a refusal.
- **A lite project** (OPS-92b): doctor's drift line is `stridal diff`'s verdict
  (`template.Survey`). `install`, `update` and `diff` already share one file classification,
  `template.Classify` in `tier.go`, and OPS-92b's PR b1 adds its profile column there. Doctor
  adds no reading of its own.

## How we'll know
- **Signal:** in the 14 days after it merges, the owner runs `stridal doctor` in three places:
  the main checkout, a lane worktree and a scratch clone on a second machine (or a second user
  account) that has never run Stridal. Every problem it names in those runs is real, and the
  fix it prints clears it on a re-run. It names no problem that is not there. One false report, or
  one fix that does not clear its problem, is a miss, and gets a fix row.
- **Check after:** 14 days
