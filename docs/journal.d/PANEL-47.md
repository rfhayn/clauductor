# PANEL-47 — close PANEL-42's group 5 lows, and refuse fish's -C in a sidecar

- **PR**: #321
- **Session**: S16, started 2026-10-29 12:12 ET
- **Review**: 1 round, peak medium, cap 3
- **Gate**: CANNOT CHECK — the lane stops before the gate; the main session runs it
- **Cost**: CANNOT CHECK — a lane shares the orchestrator's session id, so no reading is the row's own
- **Session start**: 141778173 tokens, 207 min

## What happened

Every low from PANEL-42's group 5 review (#128) was still on main, at moved lines. The command lines attach's refusals and the `panel trust` hints of attach and `panel add` print are shell-quoted with `install.ShellWord`. A card of exactly 200 lines no longer says "… (truncated)": the marker is added only when a line is cut, from stdout or stderr (`appendCardLines`). `servingGit`'s comment names its two exceptions: `signals.ReadWorktrees`' bare `git worktree list`, and catch-up, which is not serving. `unquoteC` has a `\n`/`\r` row. The misplaced comment in `projects_test.go` moved to `TestListProjectsNamesEachConfig`. `docs/panel.md`'s "What it does not do" sub-list has a heading that fits its items. D5 also refuses fish's `-C`, a cluster holding `C`, `--init-command` and `--init-command=…`.

Closing the class (PRAC-0010): three more printed command lines had the same gap and are quoted too: `clauductor init`'s `cd <dir>`, the self-update rollback's `mv` (`install.Tilde` now returns one shell word, with `~/` left outside the quotes) and a hand-started panel's `go build -o <exe>`. Not changed, because every value is fixed or validated: `launchctl` lines (the GUI domain and the constant label), `clauductor panel remove <id>` (an id matching `ProjectIDRe`, quoted anyway) and leakcheck's `tmux -L <name>` (test support, a name it made).

## Decisions

- fish's `-C` is refused as the owner decided on 2026-10-17: it runs code before any script, so a script file does not make it safe.
- Round 1 (medium): fish's getopt takes any unambiguous prefix of a long flag, so `--comm=…` and `--init=…` ran code past D5. Every `--<prefix>` of `--command` and `--init-command` is now refused, an ambiguous one too, since fish would refuse it anyway. Closing the class: bash, zsh, python3, perl and ruby reject a `--vers`-style prefix, node says "bad option", and bun reads `--eva <code>` as an unknown flag and the code as a file, so none of them needs it. deno was not probed (it is not installed here).
- The `\n`/`\r` rows test code that was already right. They were shown red by removing each case from `unquoteC` in turn, then put back.

## Surprises

- Before the fix, `caffeinate -i fish -C x` passed D5 with no refusal at all. Directly at `argv[0]`, the script-file rule caught `x` instead, so the gap showed only behind a launcher.
- The lane's sandbox blocks loopback names, unix sockets and `ps`, so about 80 short tests in `internal/panel`, `install`, `lease`, `claudecall`, `web` and `cmd` fail there, each on a lookup, a bind or a process read. None of them is in a file this row touches. The full gate covers them.

## Insights

None.
