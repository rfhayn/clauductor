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
- 2026-10-24 group 2 (The project, D6) built, uncommitted (tasks 2.1-2.3 ticked): the repository,
  model and drift lines in `framework/internal/cmd/doctor_project.go`, tests in `doctor_test.go`.
  Left: group 3 (the panel's lines, `CheckAdd`, the docs and the prereqs line), then the slice.
- 2026-10-24 group 2 review findings (after the 2-round cap) fixed, uncommitted: the count line is
  last in every report (the checks pointer appended to it), `checkDrift`'s `Report.Warnings` loop
  and its filter tested by mutation, and no `install`/`diff` fix printed without a template.
- 2026-10-24 group 2 (The project (D6)) built and reviewed: converged in 1 round(s), peak low; grades R1 pass
- 2026-10-24 group 3 (The panel, and where a user meets doctor) built, uncommitted (tasks 3.1-3.4
  ticked): `install.CheckAdd`, the panel lines in `doctor_panel.go`, the prereqs line, and the
  docs (README, QUICKSTART, panel.md). Panel-line guards were each mutated and seen to fail.
  Left: the full gate and review. Limit: the sandbox cannot bind ports, so the port-binding tests
  in `panel/install` (e.g. `TestStatusLineSnippetPostsOnlyToALivePanel`) did not run here.
- 2026-10-24 group 3 (The panel, and where a user meets doctor) built and reviewed: converged in 2 round(s), peak low; grades R1 fail, R2 pass

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

- 2026-10-24 group 2 (review fix, replacing the footer entry that stood here): the model's own
  checks are appended to the count line itself, `1 failed, 2 warnings. The model's own checks: sh
  .claude/checks/run.sh` (and `Nothing missing or broken. The model's own checks: …`), only when
  the repository holds Clauductor's model. The last line is then the count in every report
  (DOCTOR-3-S2) and names the checks (DOCTOR-2-S5); *The checks* says the count line "adds" it.
  Alternatives: a footer after the count line, which made a model repository's last line the
  pointer and broke S2; a line just before the count, which breaks S5's "the last line names".
  Below the toplevel it starts `cd <toplevel> &&` like every fix (D8), since
  `.claude/checks/run.sh` is relative.
- 2026-10-24 group 2: the project checks live in `doctor_project.go` (the repository, the model,
  the drift), not `doctor.go`; alternative: one file. doctor.go was already ~300 lines and group
  3 adds the panel's. `checkProject` returns the `doctorRepo` (toplevel, main checkout, carried
  `--template-dir`) for group 3's panel lines to read.
- 2026-10-24 group 2: the `--template-dir` a fix carries is `Source.Dir` of a fresh `Resolve` (the
  override's resolved directory, which `templateIn` makes absolute); flag appended last
  (`clauductor update --dry-run --template-dir '<dir>'`); alternative: the flag as typed. A
  relative path would not survive the `cd`.
- 2026-10-24 group 2 (review fix, superseding the fallback above and round 1's "relative
  `--template-dir` that is no template" test): when no template resolves (a `--template-dir` that
  is none, or a found template of another version), the model line prints NO repository fix,
  neither `install` nor `diff`: both refuse without a template, and D8 prints no fix the command
  would refuse. The note still names the command, adding "once the template line is ok", so
  DOCTOR-2-S2/S3's "names `clauductor install`/`diff`" holds; the template line's FAIL says why.
  The Abs fallback for an unresolvable override is deleted, not kept: no fix carries the flag
  then. `diff` is included though the finding named only `install`, since it is the same rule at
  the same source (`doctorRepo.fixes`). Alternative: print the fix and let the command refuse.
- 2026-10-24 group 2: the own-model fix is `clauductor diff` (with the `cd` below the toplevel),
  printed as a `fix:` line although `diff` changes nothing; alternative: prose only. Spec
  DOCTOR-2-S3 says the note names `clauductor diff`, and `diff` reads the current directory (E2)
  like `update`, so it needs the same `cd`.
- 2026-10-24 group 2: the drift line's detail words the counts from `Report.Files`' framework tier
  ("1 framework file differs, 1 missing, settings.json would change") and every `Report.Warnings`
  entry but the template's own `Source.Warning` is its own warn line; a Survey error is a note
  (D2). `settings.json` is named for `missing`, `invalid` or `merge` alike, as `Converged` treats
  every non-identical status.
- 2026-10-24 group 2: test 2.3's untrusted-config setup (a plist and a registered project in a
  temporary home, `doctorGOOS` forced to darwin) is in place, but the panel's lines are group 3's,
  so until 3.1/3.3 the home half of that snapshot proves only that nothing else writes there
  (checked by adding a write under HOME: the test fails). Group 3 must keep the test running
  through its lines.
- 2026-10-24 group 2 (review round 1, superseding the entry that stood here): DOCTOR-2-S4's
  "clear the warning on a re-run" IS tested end to end. `update`'s prompts read the package var
  `stdin`, which a test sets (`bufioReader("y\n...")`, as `prereqs_test.go` does), so the test runs
  the printed `update` from the toplevel and doctor again from the subdirectory; with the prompts
  answered "n" it fails. The earlier claim that a test cannot answer the prompts was wrong.
- 2026-10-24 group 2 (review round 1): class "a test that passes without the code it claims to
  guard" (three instances: the S4 re-run, `mainCheckout`'s bare-first-worktree case, and
  `--template-dir`'s Resolve and Abs). Closed by mutation, not by one more test: each guard was
  disabled in turn and its test seen to fail (the bare case, the Resolve of a checkout to its
  template/, the Abs of a relative directory). The tests are the linked worktree of a bare
  repository (a bare repository itself never reaches `mainCheckout`: `rev-parse --show-toplevel`
  fails there), a relative `--template-dir` naming a checkout from a subdirectory, and a relative
  `--template-dir` that is no template (where Resolve fails and only the Abs keeps the fix
  absolute). Group 3 keeps the discipline: a new guard in doctor is mutated before its test is
  trusted.
- 2026-10-24 group 2 (review round 3): class "a printed fix that its command would refuse" (D8;
  three instances: no template, a settings.json that does not parse, no `.claude/skills/`). Closed
  at one source, not by a guard per case: update's checks and readings that precede its
  `--dry-run` line moved out of its `RunE` into `planUpdate` (`update.go`), which `update` runs and
  doctor's drift line runs too (writing to `io.Discard`). Doctor prints the `update` fixes only
  when it returns no error; otherwise the detail says `update refuses here, so no fix is printed:`
  and the refusal itself. A refusal added to `planUpdate` reaches doctor with no change here.
  Alternatives: a doctor-side copy of each precondition (the per-case guard the findings warned
  about); printing `clauductor install` for the missing skills (needs install's own checks shared
  too, and the refusal text already names it). Group 3's `CheckAdd` should share
  `AddProject`'s checks the same way. Tested by mutation (doctor ignoring the error fails both
  cases of `TestDoctorDriftPrintsNoUpdateFixUpdateWouldRefuse`).
- 2026-10-24 group 2 (review fix, after the cap): the same class for `install` (a repository with
  no sign `modelOf` reads, but a framework-tier file the template ships already there with other
  content, e.g. `scripts/ci/gate.sh`, or a `project.conf` install cannot place: doctor printed
  `fix: clauductor install`, which refused). Closed as `planUpdate` closed it for `update`:
  install's read-only checks before it prints its plan (the template, `refusePluginModel`,
  `foreignModel`/`refuseForeign`, `planInstall`, `planExtras`) moved out of its `RunE` into
  `prepareInstall` (`install.go`), which `install` runs and doctor's model line runs (writing to
  `io.Discard`); its error text and order are unchanged, and the foreign refusal is marked
  (`foreignRefusal`) so `install --dry-run` still prints it and exits 0. Doctor prints the install
  fix only when it returns no error; otherwise the note still says "has no operating model" and
  names `clauductor install` (DOCTOR-2-S2's GIVEN holds in that state, so its THEN does too) and
  adds `install refuses here, so no fix is printed:` with the refusal, as the drift line does
  (one helper, `refusesHere`). Alternatives: widen `modelOf`'s signs to every framework-tier file
  (a second reading of install's rule, which drifts); call such a repository "its own model" (S2's
  GIVEN names it a no-model repository). The sweep found one more fix of the class: `clauductor
  diff` was guarded only by `Resolve`, while diff's RunE is `template.Survey`, which also refuses
  when the template's defaults cannot be read; the own-model line now runs `Survey` and prints the
  diff fix only when it succeeds. Tested by `TestDoctorNoModelPrintsNoInstallFixInstallWouldRefuse`
  and `TestDoctorOwnModelPrintsNoDiffFixDiffWouldRefuse`, each running the printed command first to
  see it refuse; seen failing on the code before the fix, and by two mutations each (doctor
  ignoring the error; printing the fix beside the refusal).

- 2026-10-24 group 3: `CheckAdd` returns `(AddPlan, error)` and `AddProject` calls it, then saves
  `plan` when it changed; alternative: an error-only wrapper over an unexported `planAdd`.
  `design.md` says `AddProject` calls `CheckAdd`; this makes that literally true, so the two
  cannot drift. Tested by the same refusal text from both and `CheckAdd` writing nothing.
- 2026-10-24 group 3: doctor tells "the project has no config, so `panel init` then `panel add`
  would work" from other refusals by comparing `CheckAdd`'s error with `config.LoadConfigRaw`'s
  own error for `config.OwnConfigPath(main)` plus a not-exist stat; alternative: a typed error from
  the config package (touches a package the design does not name). Any other refusal prints its
  text (`refusesHere`) and no fix. A registered project with a missing config gets `panel init`
  only when the registry reads the project's own file.
- 2026-10-24 group 3: the panel lines live in `doctor_panel.go`; their subjects are registry,
  panel, running, build, serves, config, trust (all within `subjectWidth`, 10). Paths print
  absolute, not `~`; the kickstart fix keeps a literal `$(id -u)` (a paste-ready command).
  `RunningPanel` is passed `lease.LiveProc` through the `doctorLiveProc` variable so a test can
  stand in a live pid. When `projects.json` does not load, the registry line FAILs, there is no
  "not installed" note, and the project's panel lines say they did not run. The `panel install`
  sentence in the build note and a bare `panel install` fix are printed only when the registry's
  default project's config loads (else `panel install` would refuse); `CheckAdd` runs with a zero
  `AddOptions.Now` (it only sets `Added`).
- 2026-10-24 group 3: the existing doctor tests read the machine's real `~/.clauductor` once the
  panel lines ran on macOS; they now take an empty HOME through `ownHome`. Alternative: leave them
  reading the owner's home (passes or fails by whose machine it is).
- 2026-10-24 group 3: `printPrereqs` prints the doctor line in Go (a `defer`, so the "no script"
  path ends with it too), not from `prereqs.sh`: the plugin's `/clauductor:init` and `install.sh`
  run the same script where there may be no binary. Alternative: a line in the script (names a
  command a plugin user may not have).
- 2026-10-25 group 3 (review fix): class "a line built from a stand-in value as if it were read":
  a registry that did not load was replaced by an empty one, so doctor said "0 projects are
  registered" as ok and told the user to start a panel that cannot start from that registry.
  Closed at the source: `loadRegistry` returns nil on error and every reader of the registry
  (`installedLine`, `checkRunning`, `checkBuild`) says only what the plist or the live panel gives,
  or says "not known"; nothing is built from an empty stand-in. Grepped doctor for other
  stand-ins: none. Tests assert the absence of the ok and running lines.
- 2026-10-25 group 3 (review fix): second instance of "a cited test weaker than its scenario's
  THEN" (first: DOCTOR-1-S4). DOCTOR-2-S7's test compared only the printed fix, not "which then
  registers it". Class closed by auditing every THEN in the spec that says a printed command then
  does something (DOCTOR-1-S7, DOCTOR-2-S4, DOCTOR-2-S7): each test now runs the command's effect
  (S7 registers the project after the note and finds it in the registry, then doctor reports it
  served).

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
- [x] 1.3 The tools lines in `framework/internal/cmd/doctor.go`, from `prereqs.sh --rows` in the resolved template, run with `/bin/sh`
  as `printPrereqs` runs it, and read strictly (D1). Tested in `doctor_test.go` with a stub `PATH`
  (`testbin`), and a `--template-dir` template whose `prereqs.sh` prints only a human report.
  Citing [DOCTOR-1-S1], [DOCTOR-1-S2].
- [x] 1.4 The template line in `framework/internal/cmd/doctor.go`, from `template.Resolve` (its error, or `Source.Warning`), and the
  tools line as a note when no template resolves. Tested in `doctor_test.go` with
  `CLAUDUCTOR_FRAMEWORK` naming a template whose `.template-version` is not the binary's. Citing
  [DOCTOR-1-S3].
- [x] 1.5 The gh line: `gh auth token` with its output discarded, its exit status alone read.
  Tested in `doctor_test.go` with a stand-in `gh` (`testbin`) that exits 1, then one that prints a
  marker string and exits 0, the output checked for the marker's absence. Citing [DOCTOR-1-S4].

## 2. The project (D6)
- [x] 2.1 The repository line (`git rev-parse --show-toplevel`, and the main checkout as
  `install/candidate.go`'s `mainRoot` finds it: the first entry of `signals.ReadWorktrees`,
  resolved), and the model line with `ownguard.go`'s readings (the plugin marker first, then the
  install marker and the old model's signs, then `ownModelSigns`), the no-model fix `clauductor
  install` as D8 prints it. Tested in `doctor_test.go` with a directory outside any repository, a
  bare `git init` (run from a subdirectory, the fix asserted with its `cd <toplevel> &&`), one
  holding `.claude/skills`, and one holding the plugin marker. Citing [DOCTOR-2-S1], [DOCTOR-2-S2], [DOCTOR-2-S3].
- [x] 2.2 The drift line in `framework/internal/cmd/doctor_project.go`, from `template.Survey` at the toplevel: `Report.Converged`, the counts
  of framework files that differ and are missing, whether `settings.json` would change, each
  `Report.Warnings` entry but the template's own warning, and the fix as D8 prints it (a given
  `--template-dir` carried as the resolved `Source.Dir`, quoted, so it survives the `cd`); the last
  line's `sh .claude/checks/run.sh` for Clauductor's model (D4). Tested in `doctor_test.go` with a repository installed by `install`,
  then one framework file edited and one deleted, run from a subdirectory. Citing [DOCTOR-2-S4],
  [DOCTOR-2-S5].
- [x] 2.3 Doctor writes nothing. Tested in `framework/internal/cmd/doctor_test.go` with `internal/treesnap` taken over
  the repository and the home directory before and after a run that finds drift and an untrusted
  config (macOS forced through the test's variable). Citing [DOCTOR-3-S1].

## 3. The panel, and where a user meets doctor
- [x] 3.1 The panel's machine lines, on macOS only, in `framework/internal/cmd/doctor_panel.go`: the registry (`config.LoadProjects`),
  installed (`install.PlistPath`, or a project in the registry), running (`install.RunningPanel`,
  with its `lease.ProcCheck` passed in; the restart named with `install.LaunchdLabel` and
  `install.LogDir`), and the build (`install.FileStamp` of `install.InstalledBinary` against
  `install.RunningStamp`), each fix as D8 prints it (paths through `install.ShellWord`; no
  `--template-dir`, which no panel command takes). Tested in `doctor_test.go` with a temporary home holding
  a plist and no live panel, a registered project with no plist, an installed binary whose stamp
  the test sets apart from the running one, and an empty home. Citing [DOCTOR-1-S5],
  [DOCTOR-1-S6], [DOCTOR-1-S7].
- [x] 3.2 `framework/internal/panel/install/projects.go`: `CheckAdd`, every read-only check
  `AddProject` runs before it saves (among them: the config loads, the id is free, the repository
  is not registered under another path (`registeredAs`), the socket is no other project's),
  returning its refusal; `AddProject` calls it. Tested in
  `framework/internal/panel/install/projects_test.go`: each refusal from both functions with the
  same text, and `CheckAdd` writing nothing (`internal/treesnap` over the home).
- [x] 3.3 The panel's project lines, on macOS with the panel installed, read at the main
  checkout: the registry (`Find` on the resolved path), then for a served project only the
  config (`install.ConfigPathOf`, `config.LoadConfigRaw`) and trust (`install.ConfigHash` of the
  bytes loaded, `install.TrustedNow`), each fix carrying `--project <main checkout>`; the
  not-served and not-installed fixes picked through `CheckAdd` (D8). Tested in `doctor_test.go`
  with a temporary home, the project registered and its config trusted through the `install`
  package's own functions; then the config edited, the entry left out while another project is
  registered, the same with this project's `panel.json` naming the served project's socket, a
  repository with no `panel.json`, an unknown field, and a linked worktree made with `git worktree
  add`. Citing [DOCTOR-1-S7], [DOCTOR-2-S6], [DOCTOR-2-S7], [DOCTOR-2-S8], [DOCTOR-2-S9].
- [x] 3.4 `framework/internal/cmd/prereqs.go`: `printPrereqs` ends with the line naming
  `clauductor doctor`. Tested in `prereqs_test.go`. Docs: a `clauductor doctor` row in
  `README.md`'s command table; `docs/QUICKSTART.md` *Prerequisites* and *Troubleshooting*; and
  `docs/panel.md` *Troubleshooting*.

- [ ] Slice: a user can run clauductor doctor in any directory and see what on their machine and in their project is missing or broken, each with what to do about it
