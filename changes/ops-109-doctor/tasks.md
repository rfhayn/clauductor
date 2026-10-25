# Tasks: ops-109-doctor

## Progress
- 2026-10-24 proposed (session 12); nothing built yet. The build starts from `main` once the owner
  approves. The roadmap row stays `⬜ queued` until its build PR opens.
- 2026-10-24 group 1 built, uncommitted (tasks 1.1-1.5 ticked): `prereqs.sh --rows` and python3,
  the plugin rebuilt, this repo's `.claude/prereqs.sh` synced (`.claude/model-roles.json` drift
  restored), `doctor.go` with the Machine lines (tools, template, gh) and the summary line.
  Left: groups 2 and 3 (the Project half, the panel lines, `CheckAdd`, the docs).
- 2026-10-24 group 1 review round 2's findings fixed, uncommitted: the gh line's causes, the
  `install:` label, the continuation indent, and a test through `rootCmd` with `--template-dir`.
- 2026-10-24 group 1 review round 3's findings fixed, uncommitted: doctor ends on its count line
  in the terminal (`cmd.Main`, `errReported`), and the gh line is back to the design's table.
- 2026-10-24 group 1 (The command and the machine (D1, D2, D7)) built and reviewed: converged in 2 round(s), peak none; grades R1 fail, R2 pass

## Decision log
- 2026-10-24 approved as recommended (D1–D8, Risk low). The proposal review's two open lows
  (PR #236) are carried into tasks 2.2 and 3.2 below, which narrow D8 without changing it:
  `CheckAdd` holds every refusal `AddProject` makes before it saves, `registeredAs` included (L5),
  and a carried `--template-dir` is the resolved absolute `Source.Dir` (L6).

- 2026-10-24 group 1: doctor prints each line as `ok|note|warn|FAIL <subject> <detail>` with a
  following `fix: <command>` line, under a `Machine` heading (a `Project` heading is added only
  when group 2 gives it a line); alternative: a symbol or colour per level. `design.md` fixes the
  levels, not the layout; plain labels survive a pipe and a test can grep them.
- 2026-10-24 group 1 (review fix): class "a test that observes a narrower channel than the
  scenario's THEN": DOCTOR-1-S4 read only the cobra buffer, so a gh child inheriting the process's
  stdout/stderr leaked the token unseen. Closed by capturing the process's own stdout and stderr
  (`captureProcessStreams`) as well as the command's output; verified by setting
  `c.Stdout, c.Stderr = os.Stdout, os.Stderr`, which now fails the test. The `nil, nil` line stays
  as the explicit statement of the rule (exec's nil is /dev/null).
- 2026-10-24 group 1: `prereqs.sh --rows` gives an `ok` row its reason as the fifth field too, so
  every row has five fields; alternative: an empty fifth field for `ok`. A strict reader (D1) is
  simpler when no field is optional.
- 2026-10-24 group 1: the last line pluralises (`1 warning`, `2 warnings`); alternative: the
  literal `warning(s)` of `design.md`. DOCTOR-3-S2 says "2 warnings"; a count of one reads wrong
  as `1 warning(s)`.
- 2026-10-24 group 1: the gh line is printed only when `gh` is on `PATH` (ok when signed in);
  alternative: a note when `gh` is missing. The tools lines already report a missing `gh`.
- 2026-10-24 group 1: `doctorGOOS`, `doctorRunningStamp` and `doctorFileStamp` are declared in
  group 1 as task 1.2 asks, though only group 3's panel lines read them; alternative: declare them
  in group 3. Task 1.2 names them here.
- 2026-10-24 group 1 (review round 3): the gh line follows *The checks* table as written, which
  supersedes the round-2 entry that split the causes by stderr: every non-zero exit of `gh auth
  token` is a **warn** naming `gh auth login`, and both of gh's streams are discarded (a nil
  `Stdout` and `Stderr`). What a check prints and at what level is the owner's decision (D8's table
  and DOCTOR-1-S4); a code-side narrowing was not mine to take. For the owner, if wanted as a
  later amendment: a gh older than 2.17, a locked keychain and a signed-out host all get the same
  fix, which clears only the last.
- 2026-10-24 group 1 (review round 3): class "a scenario test that reads the cobra buffer, not what
  the user's terminal shows" (second instance after DOCTOR-1-S4's process streams): doctor's exit 1
  printed `Error: ...` twice after the count line, through cobra and through main.go, and
  DOCTOR-3-S2's test read only the command's out buffer. Closed at the source: doctor has
  `SilenceErrors`, its error wraps `errReported`, and `cmd.Main` (called by main.go) exits 1 for a
  reported error without printing it; the test runs `Main` with the command's out, its err and
  `Main`'s stderr in ONE buffer, so any line after the count fails it (verified: it fails with
  `SilenceErrors` off). Alternative: `os.Exit(1)` inside RunE, which skips deferred cleanup and
  cannot be tested.
- 2026-10-24 group 1 (review fix): a missing tool's install hint is printed as `install: <hint>`,
  never `fix:`; alternative: split prereqs.sh's hints into a command and its prose (a sixth
  `--rows` field, against D1's five), or guess in Go which hints are commands. prereqs.sh's hints
  are text for people ("see https://...", "(or your distribution's package manager)"), and D8
  reserves `fix:` for a command to paste; `install:` is the label prereqs.sh's own report uses.
- 2026-10-24 group 1 (review fix): a continuation line's indent is derived from the prefix the
  line printed (`hang`), and `fix:`/`install:` sit at `textColumn` (from `levelWidth` and
  `subjectWidth`), under the line's text; alternative: a corrected constant, which drifts the next
  time a width changes.

## 1. The command and the machine (D1, D2, D7)
- [x] 1.1 `template/.claude/prereqs.sh`: the `--rows` mode of D1 (one tab-separated line per tool:
  `ok|missing`, level, tool, path or this OS's hint, reason; no heading, no closing line; exit 0),
  and D7's `python3` row with its Linux hint. The human report is otherwise unchanged. Tested in
  `framework/internal/cmd/prereqs_test.go`: a `--rows` test over a stub `PATH` and `PREREQS_OS`,
  and the existing tests' missing counts and *All present* list brought up to the new row. Then
  `scripts/build-plugin.sh`, and `env -u CLAUDUCTOR_FRAMEWORK framework/clauductor update` from
  the repository root with the binary built from this checkout, its `Template:` line checked to
  name this worktree's `template/`. Commit only `.claude/prereqs.sh` from it: the other tracked
  drift it writes (`.claude/model-roles.json`'s `roles.reviewer.eval.baseline`) is OPS-119's
  (`docs/insights-log.md`, *update follows CLAUDUCTOR_FRAMEWORK*), and is restored with `git
  checkout -- .claude/model-roles.json`.
- [x] 1.2 `framework/internal/cmd/doctor.go` (new), registered on the root command as `diff` is
  (`diff.go`), taking `--template-dir` through `addTemplateFlag`: the *Machine* and *Project*
  groups, one line per check as ok, fail, warn or note with its fix, the last line of *The
  checks* in `design.md`, and exit 1 when any line fails. The operating system and both build
  stamps are read through package variables a test replaces (`runtime.GOOS`,
  `install.RunningStamp`, `install.FileStamp`). Tested in `framework/internal/cmd/doctor_test.go`
  (new). Citing [DOCTOR-3-S2].
- [x] 1.3 The tools lines, from `prereqs.sh --rows` in the resolved template, run with `/bin/sh`
  as `printPrereqs` runs it, and read strictly (D1). Tested in `doctor_test.go` with a stub `PATH`
  (`testbin`), and a `--template-dir` template whose `prereqs.sh` prints only a human report.
  Citing [DOCTOR-1-S1], [DOCTOR-1-S2].
- [x] 1.4 The template line, from `template.Resolve` (its error, or `Source.Warning`), and the
  tools line as a note when no template resolves. Tested in `doctor_test.go` with
  `CLAUDUCTOR_FRAMEWORK` naming a template whose `.template-version` is not the binary's. Citing
  [DOCTOR-1-S3].
- [x] 1.5 The gh line: `gh auth token` with its output discarded, its exit status alone read.
  Tested in `doctor_test.go` with a stand-in `gh` (`testbin`) that exits 1, then one that prints a
  marker string and exits 0, the output checked for the marker's absence. Citing [DOCTOR-1-S4].

## 2. The project (D6)
- [ ] 2.1 The repository line (`git rev-parse --show-toplevel`, and the main checkout as
  `install/candidate.go`'s `mainRoot` finds it: the first entry of `signals.ReadWorktrees`,
  resolved), and the model line with `ownguard.go`'s readings (the plugin marker first, then the
  install marker and the old model's signs, then `ownModelSigns`), the no-model fix `clauductor
  install` as D8 prints it. Tested in `doctor_test.go` with a directory outside any repository, a
  bare `git init` (run from a subdirectory, the fix asserted with its `cd <toplevel> &&`), one
  holding `.claude/skills`, and one holding the plugin marker. Citing [DOCTOR-2-S1], [DOCTOR-2-S2], [DOCTOR-2-S3].
- [ ] 2.2 The drift line, from `template.Survey` at the toplevel: `Report.Converged`, the counts
  of framework files that differ and are missing, whether `settings.json` would change, each
  `Report.Warnings` entry but the template's own warning, and the fix as D8 prints it (a given
  `--template-dir` carried as the resolved `Source.Dir`, quoted, so it survives the `cd`); the last
  line's `sh .claude/checks/run.sh` for Clauductor's model (D4). Tested in `doctor_test.go` with a repository installed by `install`,
  then one framework file edited and one deleted, run from a subdirectory. Citing [DOCTOR-2-S4],
  [DOCTOR-2-S5].
- [ ] 2.3 Doctor writes nothing. Tested in `doctor_test.go` with `internal/treesnap` taken over
  the repository and the home directory before and after a run that finds drift and an untrusted
  config (macOS forced through the test's variable). Citing [DOCTOR-3-S1].

## 3. The panel, and where a user meets doctor
- [ ] 3.1 The panel's machine lines, on macOS only: the registry (`config.LoadProjects`),
  installed (`install.PlistPath`, or a project in the registry), running (`install.RunningPanel`,
  with its `lease.ProcCheck` passed in; the restart named with `install.LaunchdLabel` and
  `install.LogDir`), and the build (`install.FileStamp` of `install.InstalledBinary` against
  `install.RunningStamp`), each fix as D8 prints it (paths through `install.ShellWord`; no
  `--template-dir`, which no panel command takes). Tested in `doctor_test.go` with a temporary home holding
  a plist and no live panel, a registered project with no plist, an installed binary whose stamp
  the test sets apart from the running one, and an empty home. Citing [DOCTOR-1-S5],
  [DOCTOR-1-S6], [DOCTOR-1-S7].
- [ ] 3.2 `framework/internal/panel/install/projects.go`: `CheckAdd`, every read-only check
  `AddProject` runs before it saves (among them: the config loads, the id is free, the repository
  is not registered under another path (`registeredAs`), the socket is no other project's),
  returning its refusal; `AddProject` calls it. Tested in
  `framework/internal/panel/install/projects_test.go`: each refusal from both functions with the
  same text, and `CheckAdd` writing nothing (`internal/treesnap` over the home).
- [ ] 3.3 The panel's project lines, on macOS with the panel installed, read at the main
  checkout: the registry (`Find` on the resolved path), then for a served project only the
  config (`install.ConfigPathOf`, `config.LoadConfigRaw`) and trust (`install.ConfigHash` of the
  bytes loaded, `install.TrustedNow`), each fix carrying `--project <main checkout>`; the
  not-served and not-installed fixes picked through `CheckAdd` (D8). Tested in `doctor_test.go`
  with a temporary home, the project registered and its config trusted through the `install`
  package's own functions; then the config edited, the entry left out while another project is
  registered, the same with this project's `panel.json` naming the served project's socket, a
  repository with no `panel.json`, an unknown field, and a linked worktree made with `git worktree
  add`. Citing [DOCTOR-1-S7], [DOCTOR-2-S6], [DOCTOR-2-S7], [DOCTOR-2-S8], [DOCTOR-2-S9].
- [ ] 3.4 `framework/internal/cmd/prereqs.go`: `printPrereqs` ends with the line naming
  `clauductor doctor`. Tested in `prereqs_test.go`. Docs: a `clauductor doctor` row in
  `README.md`'s command table; `docs/QUICKSTART.md` *Prerequisites* and *Troubleshooting*; and
  `docs/panel.md` *Troubleshooting*.

- [ ] Slice: a user can run clauductor doctor in any directory and see what on their machine and in their project is missing or broken, each with what to do about it
