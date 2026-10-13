# Tasks: panel-42-attach

## Progress
- 2026-10-12 proposed; nothing built yet
- 2026-10-13 revised after review (#96), design.md untouched: ATTACH-2-S2 needs a card and a dirty checkout, and `diff HEAD` becomes `diff-index` (4.1); D5's word rule covers a bare `..` and a script behind any program's flags (ATTACH-3-S7), and settles `/dev/null` as refused (ATTACH-3-S8); the snapshot helper moves to `internal/treesnap`; remove's output is tested through the command; the menu line and the Remove confirmation come from a pure JS function tested under node (2.3, 2.4, ATTACH-5-S2)
- 2026-10-13 the owner allowed `/dev/null` as a redirection's target (design.md D5's one exception; its approval to be re-recorded by the owner's decision); ATTACH-3-S8 and 3.1 follow it
- 2026-10-13 the owner made a sidecar's commands argv-only, with no shell text (design.md D5 rewritten; its approval to be re-recorded by the owner's decision): ATTACH-3-S1 to S9 are rewritten to the new rule, ATTACH-6-S1 is added (a sidecar's card shows its stderr), 1.1 writes argv-form commands for an attach, and 3.1's tokenizer is gone
- 2026-10-13 review of the argv-only D5, applied by the owner's decision (design.md D5 edited; its approval to be re-recorded after one last review round, in which a program that runs code from a flag of its own counts as a named known limit unless the panel writes that command):
  - paths are judged piece by piece, with no flag list;
  - `'` `"` `\` `!` are refused;
  - code flags are refused at any position, while `.`, `eval`, `source`, fetch-and-run and the script-file rule apply in program position;
  - `bun run <bare word>` is allowed;
  - fetch-and-run gains `npm exec`, `npm x`, `npm init`, the `create` forms and `go run module@version`;
  - stderr is merged at the card's call site only;
  - a script file must exist when trusted;
  - known limits and accepted false refusals are named;
  - 3.1 to 3.3 are renumbered in order (3.2 is the card's stderr, 3.3 trust).
- 2026-10-13 the last review round, by the owner's rule (the approval is recorded on its result):
  - a single-dash piece is judged from its first `/` or `.`, which closes flag clusters (`-sC..`);
  - program position walks past launchers' flags and their values, `env`'s `NAME=value` words and `timeout`'s duration, through chained launchers, and refuses an unknown launcher flag;
  - a fetch-and-run subcommand is matched as any later word;
  - `node` adds `--import`, `-r`/`--require` and the loaders to its code flags;
  - interpreter names are matched exactly, and clusters are single-dash only;
  - the `--name=` and `!` strips are dropped;
  - a card's JSON stdout is parsed alone;
  - the new false refusals and the symlink limit are named.

## Decision log
- 2026-10-13 proposal review (#96): task 3.1's tokenizer is stricter than design.md D5's "What is checked" text, to hold D5's own requirement ("a word whose location cannot be known before it runs"): any argv word holding whitespace or a shell operator is split as a script whatever program precedes it (D5 names only `sh`, `bash`, `zsh` and `dash` with `-c`); `<`, `>` and `&` split too; a backslash is unescaped before a word is judged; a bare `..` is a path; and every shell expansion character (`*` `?` `[` `{` `}` `~` `$` backtick) is refused, as is each `:` entry of a leading assignment. design.md is unchanged, and this line is here so the owner sees the difference when re-approving
- 2026-10-13 the owner's decision, superseding the line above: a sidecar's commands are argv lists with no shell text (design.md D5). No script is parsed, so the tokenizer is dropped, and its rules with it. The owner's `/dev/null` redirection exception of the same day is superseded too: with no shell there are no redirections, and the panel merges a sidecar card's stderr itself (ATTACH-6-S1), so `2>&1` is not needed. Four review rounds each found another shell form the tokenizer missed (nested quoting, `eval`, extglob, backslash-newline, `export PATH=…`, brace expansion, globs), which is why the class is closed by having no shell text at all

## 1. The sidecar and `panel attach`
- [ ] 1.1 `install.PlanInit` takes its target path, and whether a config already in the repository refuses the plan (`panel init`, the page's Create this config) or adds a note saying it will not be read (attach). The reading of the repository is unchanged: `panel init` and `/api/projects/init-preview` produce the same bytes and notes as before. For an attach, every command is written in argv form with no `sh -c` and no `2>&1` (design.md D5): `["sh", ".claude/owner-queue.sh"]`, `["sh", ".claude/roadmap-queue.sh", "--text"]`, `["sh", ".claude/panel-suggest.sh", "<build|propose|fix|ops>"]`, `["scripts/ci/run-local.sh"]`, and the detected gate as it is today (`detectGates`: `["npm", "run", "gate"]`, `["pnpm", "run", "gate"]`, `["yarn", "run", "gate"]`, `["bun", "run", "gate"]`, `["make", "gate"]`, for each gate name). `bun run <bare word>` is allowed by D5's exception, so every one of these passes 3.1. The schema does not change: `command` is already an argv list the panel execs with no shell. Tested in `framework/internal/panel/install/init_test.go`, whose existing cases pass unchanged, plus a case pinning that the plan for an attach equals the in-repository plan except for those commands, and that 3.1's list is empty for it, citing [ATTACH-1-S2]
- [ ] 1.2 `install.Attach` (new `framework/internal/panel/install/attach.go`) resolves the repository as `panel add` does, picks the id (the config name's slug, or `--id`), writes `~/.clauductor/projects/<id>/panel.json` with `O_EXCL` (folder 0700, file 0600), and registers it through `AddProject` with that config. It removes the file it wrote when registering fails. It attaches in place a registered project whose registry `config` is empty and whose `.clauductor/panel.json` does not exist, keeping its id, socket and lane registry (design.md D4). It refuses any other registered project, and a sidecar already at the path, with design.md's messages (D3). Tested in a new `framework/internal/panel/install/attach_test.go` citing [ATTACH-1-S1], [ATTACH-1-S2], [ATTACH-1-S3], [ATTACH-1-S4], [ATTACH-1-S5] and [ATTACH-1-S6]
- [ ] 1.3 `clauductor panel attach [<repo>]` in `framework/internal/cmd/panel.go` (`--id`, `--default`; the repository defaults to the current directory's git toplevel) prints what it wrote, init's notes, and the `clauductor panel trust --project <repo>` line, and follows a running panel as `panel add` does (`followRunningPanel`). Tested in a new `framework/internal/cmd/panel_attach_test.go`, which drives the cobra command against a real git repository in a temporary directory with `HOME` pointed at another, citing [ATTACH-1-S1] and [ATTACH-1-S2] (the output carries init's notes for the repository, one per choice)
- [ ] 1.4 The repository-untouched proof: a snapshot helper in a new test-support package, `framework/internal/treesnap` (beside `internal/testbin`), so the tests in `cmd` and `panel` can both import it. It records every file and directory under a root, `.git` included (path, mode, size, mtime, SHA-256), and a diff of two snapshots names what changed; `treesnap`'s own test pins that a rewritten file with the same bytes, a new file and a mode change each show. The test in `panel_attach_test.go` attaches a repository with commits, a branch, an untracked file and an ignored file, and compares. It cites [ATTACH-2-S1], and is first run against a version that writes into the repository (the in-repository target) to see it fail

## 2. One config per project, read from the registry
- [ ] 2.1 `configPathOf` (`framework/internal/cmd/panel.go`, for `panel trust`) and `panel.Run`'s up-front check of `--project` (`framework/internal/panel/run.go`) use the registered entry's `ConfigPath()` when `--config` is not given, and `<root>/.clauductor/panel.json` only for a project the registry does not hold. `panel trust` resolves that path before it calls `install.TrustConfigReport` (which otherwise defaults an empty path to `<project>/.clauductor/panel.json`, `framework/internal/panel/install/trust.go`), and wherever `res.Entry.Config` is passed it uses the resolved `ConfigPath()`, so the config trusted and the path printed are the same file. Tested in `panel_attach_test.go` citing [ATTACH-4-S2], and in `framework/internal/panel/multiproject_integration_test.go` citing [ATTACH-4-S3], each first run against today's code to see it read the in-repository path and fail
- [ ] 2.2 `install.ListProjects` prints each project's `config <path>`, marks one outside its repository `(outside the repository)`, and adds a line when the repository's own `.clauductor/panel.json` exists and is not read. `install.RemoveProject`'s caller in `cmd/panel.go` names a sidecar folder that stays and the `panel add --project <repo> --config <file>` line (design.md D6). `ListProjects` is tested in `framework/internal/panel/install/projects_test.go` citing [ATTACH-4-S1]. `panel remove`'s printed output is tested through the cobra command in `framework/internal/cmd/panel_attach_test.go` citing [ATTACH-5-S1], which then re-adds the project with `panel add --config` and checks that it is served from the same file, still trusted
- [ ] 2.3 The project menu row says when a project reads a config outside its repository: a `Config` field on `web.ProjectSummary`, set only then. The row's line is built by a pure function in a new `framework/internal/panel/web/static/project-config.js` that `panel.js` calls, loaded as `start-plan.js` is: a `<script src="/static/project-config.js"></script>` tag in `framework/internal/panel/web/index.html` before `panel.js`'s, its order pinned as `framework/internal/panel/web/startdialog_test.go` pins start-plan's, and its path added to `TestPageCSPIsSelfOnly`'s served-file list in `framework/internal/panel/web/terminal_test.go`. The Go side is tested with the loaded runtime in `framework/internal/panel/multiproject_integration_test.go`, an attached project and one with its own config, asserting `Config` is set for the first only, citing [ATTACH-4-S1] and [ATTACH-4-S4]. The line is tested by running `project-config.js` under node in a new `framework/internal/panel/web/project_config_test.go` (the `start_plan_test.go` pattern) for both summaries, citing [ATTACH-4-S4]
- [ ] 2.4 The page's **Remove from panel…** confirmation, for a project whose config is outside its repository, names the sidecar, says it stays, and gives `clauductor panel add --project <repo> --config <file>` as the way back, instead of today's "adding it again brings it all back" (which is false for it: **+ Add a project…** reads only `<root>/.clauductor/panel.json`, `install/candidate.go`, and would offer Create this config inside the repository). The remove plan (`live.go`, `removePlan`) carries whether the config is outside the repository, and the sentence comes from `project-config.js`. Tested in `framework/internal/panel/live_integration_test.go` (the plan's field for both kinds of project) and in `project_config_test.go` (the sentence for both), citing [ATTACH-5-S2]

## 3. Commands stay in the repository
- [ ] 3.1 A pure function in `framework/internal/panel/config` lists every command in a config, and every word in it, that a sidecar may not hold, by design.md D5's rule, with the word, why, and "move it into a script in the repository". It judges argv words only and parses no script. The rule has four parts:
  - **Code flags, anywhere.** An interpreter's name, matched exactly (`sh`, `bash`, `zsh`, `dash`, `ksh`, `fish`, `python`, `python3`, `python3.<n>`, `node`, `deno`, `bun`, `perl`, `ruby`, `php`, `osascript`), followed anywhere later in the argv by its code flag, is refused at any position. So a launcher needs no list (`caffeinate -i bash -c …`, `sudo`, `stdbuf`, `arch`, `xcrun`):
    - a shell with `-c`, or a single-dash cluster holding `c` (`-lc`);
    - Python with `-c` or `-m`;
    - `node`, `deno` and `bun` with `-e`, `-p`, `--eval` or `--print`, and `node` also with `--import`, `-r`, `--require`, `--loader` or `--experimental-loader`;
    - `perl` with `-e` or `-E`, `ruby` with `-e`, `php` with `-r`, `osascript` with `-e`.
  - **Program position only.** Program position is `argv[0]`. Behind a launcher (`env`, `nice`, `nohup`, `time`, `timeout`), it is the first word after the launcher's own words, following chained launchers. The launcher's own words are:
    - its flags and their values: `env -u NAME`, `-C dir`, `-P path`; `nice -n N`; `timeout -s SIG`, `-k DUR`; `time -o file`, `-f fmt`;
    - `env`'s `NAME=value` words;
    - `timeout`'s duration.

    A launcher flag outside that list is refused.
    - `.`, `eval` and `source`. Elsewhere `.` is a path, the root: `go test .` and `git add .` pass.
    - `env -S` and `--split-string`.
    - `awk` (and `gawk`, `mawk`, `nawk`) and `sed` without `-f <file>`.
    - `xargs`, and `find` with `-exec`, `-execdir`, `-ok` or `-okdir`.
    - Fetch-and-run:
      - `npx`, `bunx`, `uvx`, `pipx run`;
      - `npm exec`, `npm x`, `npm init <package>`, `npm create`;
      - `pnpm dlx`, `pnpm create`, `yarn dlx`, `yarn create`;
      - `bun x`, `bun create`;
      - `go run` with a word holding `@` after a `/`.

      The subcommand is matched as any later word of that program, so a flag or its value cannot hide it (`npm -y exec tool`, `pnpm -C sub dlx tool`).
    - A shell, Python, `node`, `deno`, `bun`, `perl`, `ruby` or `php` whose first operand (its first word that is not a flag) is not a script file inside the repository that exists when the config is trusted, or that has none. The exception is `bun run <bare word>`, a `package.json` script as with `npm run`.
  - **Every word is literal.** These are refused in any word:
    - `*` `?` `[` `]` `{` `}` `~` `$`, a backtick;
    - `(` `)` `<` `>` `|` `&` `;`;
    - `'` `"` `\` `!`;
    - a space, a tab, a newline.

    No command key takes a placeholder (`{name}` and `{issue}` belong to `branch_pattern` and `first_prompt`, which are not commands and are not checked), so `{{.Dir}}`-style text is refused like any other `{`.
  - **Paths, piece by piece, with no flag list.**
    - A word holding `/` or `..` is split at every `=`, `:` and `,`.
    - A piece that starts with a single `-` is judged from its first `/` or `.` onward, so a cluster of any length is caught (`-sC..` → `..`, `-sC/tmp/x` → `/tmp/x`, `-isystem/x` → `/x`). A leading `@` is stripped. There is no `!` strip, because `!` is a refused character.
    - A piece that is then absolute, holds a `/`, or is `..` or has a `..` segment must resolve inside the root once joined to it and resolved through symlinks as far as it exists.
    - A separate word (`-C ..`, `..`) is judged the same way, and so is an interpreter's script file.
    - Still allowed: `main..HEAD` (no `..` segment), `-j4` and `--format=%h` (neither `/` nor `..`), and `./...` (inside).
  - **Other bare words are allowed:** a program on `PATH`, or a literal argument (`gate`, `--text`).

  Tested in a new `framework/internal/panel/config/sidecar_test.go` with a table of cases:
  - each code flag, refused, at `argv[0]` and behind `caffeinate -i` and `env`; `node --import x`, `node -r x`, `node --loader x` among them; `pythonic -c x` allowed (an exact name match only);
  - each program-position form, refused, each row asserting its own reason: `env -S make` and `env --split-string=make` (reason: `env -S`, with no space in the row, so only that rule can refuse it); `.` allowed outside program position (`go test .`, `git add .`);
  - behind launchers, refused: `timeout 9 npx tool`, `env FOO=1 npx tool`, `nice -n 5 xargs make`, `env nice npx tool`, `env -u HOME eval x`; an unknown launcher flag (`nice --weird npx tool`), refused;
  - each fetch-and-run form, refused: `npm exec tool`, `npm -y exec tool`, `npm x tool`, `npm init pkg`, `npm create app`, `yarn create app`, `pnpm create app`, `pnpm -C sub dlx tool`, `go run example.com/tool@v1`, and the rest;
  - each forbidden character, refused, `'` `"` `\` `!` included; `git -c alias.x=!../gate.sh x` refused with the reason "the character `!`";
  - each path shape, refused: `..`, `docs/../../x`, `-C..`, `-C ..`, `-sC..` (`make -sC.. gate`), `-sC/tmp` (`make -sC/tmp gate`), `-isystem/x`, `-xf/tmp/a.tar`, `--prefix=..`, `--chdir=..`, `-p..`, `-I../lib`, `-o/tmp/x`, `@/abs`, `--exec-path=..`, `--manifest-path=../o`, `--require=../x.js`, `NODE_OPTIONS=--require=../x.js`, `env PATH=x:..`, `https://host/x`, a symlink out; `-Isrc/x` refused (the named false refusal) while `-I src/x` is allowed;
  - a symlink that stays in, allowed;
  - `sh` with no operand, `sh ../other/gate.sh`, and `sh .claude/missing.sh` (a file that does not exist), refused;
  - allowed rows:
    - each argv command 1.1 writes for an attach, `bun run gate` among the detected gates;
    - `sh .claude/x.sh arg` and `node infra/roadmap-queue.mjs --text`;
    - `git log --format=%h`, `git log main..HEAD` and `git add .`;
    - `go test .` and `go test ./...`;
    - `make -j4 gate` and `make -C sub gate`.

  It cites [ATTACH-1-S2], [ATTACH-3-S3], [ATTACH-3-S5], [ATTACH-3-S7], [ATTACH-3-S8] and [ATTACH-3-S9].
- [ ] 3.2 A sidecar's card shows its stderr with its stdout (design.md D5, Output). The merge is at the card's call site only, `runtime.go:441` (the card's `commandFetch`). It is not in the shared `commandFetch`, which also serves suggest (`runtime.go:469`), metrics (`metrics_source.go:58`), `gh pr list` (`runtime.go:224`) and `claude agents` (`runtime.go:556`), whose output is parsed. It applies to a config outside its repository, within today's output limit. An in-repository config keeps stdout alone. Tested in `framework/internal/panel/multiproject_integration_test.go`, citing [ATTACH-6-S1]:
  - a script that writes one line to each stream, run as a card in an attached project and in one with its own config, first run against today's code to see the sidecar's stderr line missing;
  - the attached project's suggest command writing a line to stderr, its suggestions still parsed;
  - a sidecar card whose stdout is JSON and which writes a line to stderr: still a JSON card. Its stdout is parsed alone first (`signals.ParseCardOutput`), and stderr lines are added, after stdout's, only to a text card.
- [ ] 3.3 `install.CheckTrust` and `install.TrustedNow` refuse a config outside its repository whose list is not empty: no record is written, and an entry recorded earlier does not count. Both take only a path and a hash today, so to enforce this each reads the file at the config path, confirms its bytes hash to the hash it was given (a mismatch is untrusted), parses it, and runs 3.1's list against the resolved root; a file that does not read or parse is untrusted. The trust report (`TrustConfigReport` and `PrintTrusted`, `Inspect`'s `Runs`, the page's `trustReport`) marks each offending line `NOT ALLOWED IN A SIDECAR (<word>: <why>; move it into a script in the repository): …`, and `panel trust` prints them and exits non-zero. Tested in `framework/internal/panel/install/trust_test.go` citing [ATTACH-3-S1], [ATTACH-3-S3], [ATTACH-3-S4], [ATTACH-3-S5], [ATTACH-3-S7], [ATTACH-3-S8] and [ATTACH-3-S9], and in `framework/internal/panel/live_integration_test.go` citing [ATTACH-3-S2] (a hand-written trust entry; the project loads untrusted) and [ATTACH-3-S6] (the `/api/projects/{id}/trust` report and the trust, refused). Each of S1, S2 and S6 is first run against today's code to see the trust recorded or the commands on

## 4. Serving writes nothing
- [ ] 4.1 The panel's git reads of a served checkout write nothing: `pollGit`'s `status` and `log` in `framework/internal/panel/runtime.go`, and the read-only `rev-parse` calls beside them, pass `--no-optional-locks`. The dirty-tree stat `git diff HEAD --shortstat` becomes `git --no-optional-locks diff-index --shortstat HEAD`: on git 2.54 `git diff` rewrites `.git/index` even with `--no-optional-locks` (or `GIT_OPTIONAL_LOCKS=0`), while `diff-index` leaves it alone and does not count a file whose stat alone changed. Tested in `framework/internal/panel/multiproject_integration_test.go` with `treesnap` from 1.4: an attached repository whose config has a card (without one, `CardsCheckout` is empty and the root is never polled), one tracked file touched with its bytes unchanged and one tracked file modified, then one git poll of the cards' checkout. It asserts the snapshot is unchanged and the dirty stat counts the modified file only, citing [ATTACH-2-S2]. The test is first run against today's `diff HEAD`, and against `status` without the flag, to see `.git/index` change. If it cannot be made to fail that way, the builder stops and records why in the decision log, rather than keeping a test that never failed

## 5. Docs, and a look at a real panel
- [ ] 5.1 `docs/panel.md`:
  - Quick start gets an "Attach a repository without writing to it" subsection.
  - "Projects" covers `panel attach`, D3's and D4's refusals, D6 (remove keeps the sidecar), and D7 (the registry's config wins, and `panel list` and the menu say so).
  - "Add, trust and remove from the page" says what Remove from panel… says for an attached project (2.4), and that + Add a project… does not attach until PANEL-43.
  - "`panel init`" says attach uses the same reading.
  - "Config trust" and "Security model" state D5's rule for a sidecar (argv only, no shell text: code forms, non-literal words and paths outside the repository are refused, with "move it into a script in the repository"), that an earlier trust entry does not count, that the `/dev/null` exception is superseded, and that in-repository configs are unchanged.
  - They also state its limits (not a sandbox):
    - what the repository's own content does is not judged: a script file, a Makefile target, a `package.json` script;
    - the environment the panel inherits (`PATH`, `CDPATH`, `OLDPWD`) is outside the check;
    - a program that runs code from a flag of its own;
    - and design.md D5's named limits: `ssh` runs elsewhere; `docker` runs an image's code, and `docker --mount` reaches any host path; an scp-style remote (`git@host:o/r.git`) passes to `git clone`, `rsync` or `scp`; a package install runs the scripts it fetches; two cards can download a file and then run it; `go test -exec <bare word>`; `flock -c <word>`; the program-position rules behind a launcher not on their list (`sudo`, `caffeinate`); and a bare word naming a symlink out of the repository (`make -C outlink gate`), which is the repository's own content.
  - They list the accepted false refusals (D5, "The cost"), each moved into a script in the repository:
    - `git diff HEAD~1` and `git log @{u}..`;
    - `node --test` and `bun test`;
    - `bash -o pipefail x.sh` and `python3 -W error x.py`;
    - `pytest -k "not slow"`;
    - a URL;
    - every `deno` command, and `bun run scripts/x.ts`;
    - a script's own flag that matches a code flag (`python3 x.py -c cfg.yml`);
    - `-Isrc/x`, to be written `-I src/x`;
    - `npm run create`.
  - A sidecar's card shows its stderr with its stdout, and no `2>&1` is needed (ATTACH-6).
  - The attach subsection says what attaching and serving never write (D8), and what a lane, RUN and Close lane do write.
  - The attach subsection links to `docs/plugin.md`'s opt-out.
- [ ] 5.2 `docs/plugin.md`: a section on installing at user scope. It says what loads in a repository that does not use the model (the namespaced skills and the SessionStart hook; not the guard hooks, which stand aside without `.claude/clauductor-plugin`), and gives the per-repository opt-out, `"enabledPlugins": {"clauductor@clauductor": false}` in the uncommitted `.claude/settings.local.json` (design.md D9). Project scope stays the recommendation for a repository that runs the model.
- [ ] 5.3 A throwaway panel on its own port, tmux socket and `HOME`, attaching a scratch repository with no Clauductor files. The check covers: `panel attach`, then `panel trust --project`, then the page shows its cards; the project menu names the sidecar; `git status --porcelain --ignored` in the scratch repository is empty before and after; the installed panel, its socket and its lanes are untouched (manual: it needs a real browser, and the states and routes are tested above)

- [ ] Slice: an owner can run `clauductor panel attach <repo>` and have the panel serve a repository that has no Clauductor files, with its commands trusted from a config under `~/.clauductor/projects/`, and nothing written into the repository
