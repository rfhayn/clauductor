# Tasks: ops-109-doctor

## Progress
- 2026-10-24 proposed (session 12); nothing built yet. The build starts from `main` once the owner
  approves. The roadmap row stays `⬜ queued` until its build PR opens.

## Decision log
- (none yet)

## 1. The command and the machine (D1, D2, D7)
- [ ] 1.1 `template/.claude/prereqs.sh`: the `--rows` mode of D1 (one tab-separated line per tool:
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
- [ ] 1.2 `framework/internal/cmd/doctor.go` (new), registered on the root command as `diff` is
  (`diff.go`), taking `--template-dir` through `addTemplateFlag`: the *Machine* and *Project*
  groups, one line per check as ok, fail, warn or note with its fix, the last line of *The
  checks* in `design.md`, and exit 1 when any line fails. The operating system and both build
  stamps are read through package variables a test replaces (`runtime.GOOS`,
  `install.RunningStamp`, `install.FileStamp`). Tested in `framework/internal/cmd/doctor_test.go`
  (new). Citing [DOCTOR-3-S2].
- [ ] 1.3 The tools lines, from `prereqs.sh --rows` in the resolved template, run with `/bin/sh`
  as `printPrereqs` runs it, and read strictly (D1). Tested in `doctor_test.go` with a stub `PATH`
  (`testbin`), and a `--template-dir` template whose `prereqs.sh` prints only a human report.
  Citing [DOCTOR-1-S1], [DOCTOR-1-S2].
- [ ] 1.4 The template line, from `template.Resolve` (its error, or `Source.Warning`), and the
  tools line as a note when no template resolves. Tested in `doctor_test.go` with
  `CLAUDUCTOR_FRAMEWORK` naming a template whose `.template-version` is not the binary's. Citing
  [DOCTOR-1-S3].
- [ ] 1.5 The gh line: `gh auth token` with its output discarded, its exit status alone read.
  Tested in `doctor_test.go` with a stand-in `gh` (`testbin`) that exits 1, then one that prints a
  marker string and exits 0, the output checked for the marker's absence. Citing [DOCTOR-1-S4].

## 2. The project (D6)
- [ ] 2.1 The repository line (`git rev-parse --show-toplevel`, and the main checkout as
  `install/candidate.go`'s `mainRoot` finds it: the first entry of `signals.ReadWorktrees`,
  resolved), and the model line with `ownguard.go`'s readings (the plugin marker first, then the
  install marker and the old model's signs, then `ownModelSigns`). Tested in `doctor_test.go` with a
  directory outside any repository, a bare `git init`, one holding `.claude/skills`, and one
  holding the plugin marker. Citing [DOCTOR-2-S1], [DOCTOR-2-S2], [DOCTOR-2-S3].
- [ ] 2.2 The drift line, from `template.Survey` at the toplevel: `Report.Converged`, the counts
  of framework files that differ and are missing, whether `settings.json` would change, each
  `Report.Warnings` entry but the template's own warning, and the fix as D8 prints it; the last
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
  `install.RunningStamp`), each fix as D8 prints it (paths through `install.ShellWord`,
  `--template-dir` carried when given). Tested in `doctor_test.go` with a temporary home holding
  a plist and no live panel, a registered project with no plist, an installed binary whose stamp
  the test sets apart from the running one, and an empty home. Citing [DOCTOR-1-S5],
  [DOCTOR-1-S6], [DOCTOR-1-S7].
- [ ] 3.2 `framework/internal/panel/install/projects.go`: `CheckAdd`, the read-only checks
  `AddProject` runs before it saves (the config loads, the id is free, the socket is no other
  project's), returning its refusal; `AddProject` calls it. Tested in
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
