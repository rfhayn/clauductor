# Hooks

Registered in `.claude/settings.json`. Each is plain POSIX `sh`; the ones that read a payload
use `jq`. Each is exercised by `.claude/checks/hooks.sh` (and `merge-guard.sh`) as a payload →
exit-code table, in both directions.

| Hook | Event | Blocks? | What it does |
|---|---|---|---|
| `no-blind-source-rewrite.sh` | PreToolUse `Bash` | yes | Refuses `sed -i`, `perl -pi`, python `.replace` read-modify-write and the like against files `git ls-files` tracks. Use the Edit tool. |
| `pr-merge-guard.sh` | PreToolUse `Bash` | yes | Refuses `gh pr merge --auto/--admin`, a merge with no gate evidence for the head SHA, and a duplicate journal session number. |
| `worktree-hook-drift.sh` | PreToolUse `Agent\|Task` | yes | Refuses a worktree agent while the main checkout's hooks lack commits from `origin/main`. |
| `focus-staleness.sh` | UserPromptSubmit | no | Nudges when the status-line focus is missing or stale. |
| `format.sh` | PostToolUse `Write\|Edit` | no | Runs `FORMAT_CMD` from `.claude/project.conf` on the file just written. Off until configured. |

## Rules every hook here follows

1. **Registered through `"$CLAUDE_PROJECT_DIR"`**, never a relative path: a session's cwd can be
   a subdirectory or a worktree, and a relative path then names nothing, silently.
2. **Only two channels reach Claude.** A PreToolUse hook's stderr on **exit 2** (the block), and
   `hookSpecificOutput.additionalContext` JSON on stdout on **exit 0** (a note). Plain stdout or
   stderr on exit 0 reaches only the debug log: a warning printed there is a control that runs,
   is right, and is read by nobody. (UserPromptSubmit is the exception: its plain stdout on exit
   0 is added to Claude's context.) `hook_note` in `.claude/lib/conf.sh` writes the note form.
3. **A guard filters for itself.** Claude Code supports an `"if"` field in permission-rule
   syntax (`"if": "Bash(git commit *)"`), but only on each **handler object**, beside `type` and
   `command`, and only for tool events. Placed on the matcher group, beside `matcher`, it is not
   a schema field and is silently ignored: the hook then runs on every call. Even placed
   correctly it is a pre-filter on how a command is SPELLED, and a guard must also catch
   `cd x && gh pr merge 5`, so the guards here read the command themselves and use no `if`.
4. **A missing dependency fails CLOSED for what the hook polices, and open for everything
   else.** Without `jq`, `no-blind-source-rewrite` blocks commands containing `sed`, `perl`,
   `ruby` or `.replace(` and allows the rest; the message says to install `jq`.
5. **The main checkout's copy is what runs.** Hooks are read from the main checkout, even for a
   worktree agent. Keep the main checkout on `main`; `worktree-hook-drift` refuses the case where
   it is not and it matters.

## Adding a hook

Write it to take the payload on stdin and exit 0 or 2; name what it executes in AGENTS.md's
table; add rows to `.claude/checks/hooks.sh` in both directions; and falsify the rows once
(replace the hook with `exit 0`, watch the BLOCK rows fail, restore).
