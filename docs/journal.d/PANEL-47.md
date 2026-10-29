# PANEL-47 — close PANEL-42's group 5 lows, and refuse fish's -C in a sidecar

- **PR**: #321
- **Session**: S16, started 2026-10-29 12:12 ET
- **Review**: 0 rounds, peak none, cap 3
- **Gate**: CANNOT CHECK — the lane stops before the gate; the main session runs it
- **Cost**: CANNOT CHECK — a lane shares the orchestrator's session id, so no reading is the row's own
- **Session start**: 141778173 tokens, 207 min

## What happened

Every low from PANEL-42's group 5 review (#128) was still on main, at moved lines. The command lines attach's refusals and the `panel trust` hints of attach and `panel add` print are shell-quoted with `install.ShellWord`. A card of exactly 200 lines no longer says "… (truncated)": the marker is added only when a line is cut, from stdout or stderr (`appendCardLines`). `servingGit`'s comment names `signals.ReadWorktrees`' bare `git worktree list` as its one exception. `unquoteC` has a `\n`/`\r` row. The misplaced comment in `projects_test.go` moved to `TestListProjectsNamesEachConfig`. `docs/panel.md`'s "What it does not do" sub-list has a heading that fits its items. D5 also refuses fish's `-C`, a cluster holding `C`, `--init-command` and `--init-command=…`.

Closing the class (PRAC-0010): three more printed command lines had the same gap and are quoted too: `clauductor init`'s `cd <dir>`, the self-update rollback's `mv` (`install.Tilde` now returns one shell word, with `~/` left outside the quotes) and a hand-started panel's `go build -o <exe>`. Not changed, because every value is fixed or validated: `launchctl` lines (the GUI domain and the constant label), `clauductor panel remove <id>` (an id matching `ProjectIDRe`, quoted anyway) and leakcheck's `tmux -L <name>` (test support, a name it made).

## Decisions

- fish's `-C` is refused as the owner decided on 2026-10-17: it runs code before any script, so a script file does not make it safe.
- The `\n`/`\r` rows test code that was already right. They were shown red by removing each case from `unquoteC` in turn, then put back.

## Surprises

- Before the fix, `caffeinate -i fish -C x` passed D5 with no refusal at all. Directly at `argv[0]`, the script-file rule caught `x` instead, so the gap showed only behind a launcher.
- The lane's sandbox blocks loopback names, unix sockets and `ps`, so about 80 short tests in `internal/panel`, `install`, `lease`, `claudecall`, `web` and `cmd` fail there, each on a lookup, a bind or a process read. None of them is in a file this row touches. The full gate covers them.

## Insights

None.
