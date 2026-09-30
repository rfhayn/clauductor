#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# PreToolUse hook (matcher: Agent|Task). Refuses to spawn a WORKTREE agent while the hooks that
# will guard it lag the hooks on origin/<main>.
#
# WHY. Every hook is registered as `"$CLAUDE_PROJECT_DIR"/.claude/hooks/…`, and that directory is
# the MAIN checkout. A worktree agent is cut from origin/<main>, but every Bash call it makes is
# judged by the hook code on whatever branch the main checkout has checked out. With the main
# checkout on an old branch, an agent's commit was blocked by a hook version its own branch had
# already replaced. The silent half is worse: a guard rule merged to main is simply NOT IN FORCE
# for the agent, and nothing says so.
#
# THE AUTHORITY is the copy that is actually executing: this file's own checkout. Its
# `.claude/hooks/` and `.claude/settings.json` are compared, working tree included, with
# origin/<main>. Both halves of the predicate are needed:
#   1. the trees DIFFER (a checkout that cherry-picked the fix is guarded correctly), and
#   2. origin/<main> has hook COMMITS this checkout lacks (a checkout only AHEAD of main is
#      someone developing a hook there on purpose: noted, not blocked).
#
# WHO RECEIVES IT: the session spawning the agent, at the moment it spawns it, the one place
# both the cause and the remedy are in reach. Everything it cannot evaluate (no git, no remote
# ref) ALLOWS with a note: this is a consistency check on the guards, not a guard itself.
#
# LIMIT, by construction: this file is itself read from the main checkout. A main checkout on a
# branch cut before this hook existed runs no check at all.
#
# Protocol: exit 2 = block (stderr reaches Claude); exit 0 = allow, with
# hookSpecificOutput.additionalContext on stdout as the only exit-0 channel that reaches Claude.
# Checked by .claude/checks/hooks.sh.

payload=$(cat)

# A worktree spawn only: a non-isolated agent works in this checkout, on the same code its guards
# come from. Matched on the raw JSON: a prompt that merely MENTIONS the key arrives with its quotes
# escaped (\"isolation\"), which this pattern cannot match.
printf '%s' "$payload" | grep -Eq '"isolation"[[:space:]]*:[[:space:]]*"worktree"' || exit 0

root=$(cd "$(dirname "$0")/../.." 2>/dev/null && pwd) || root=""
ROOT=$root
# shellcheck disable=SC1091
. "$(dirname "$0")/../lib/conf.sh"

note() { hook_note PreToolUse "worktree-hook-drift: $1"; exit 0; }

base="origin/${MAIN_BRANCH:-main}"
paths=".claude/hooks .claude/settings.json"
if [ -z "$root" ] || ! git -C "$root" rev-parse -q --verify "$base" >/dev/null 2>&1; then
  note "CANNOT CHECK whether the hooks guarding this worktree agent match $base (no git or no $base at ${root:-?}). Allowed; UNKNOWN, not verified."
fi

# $paths unquoted on purpose: two pathspecs, neither containing a space.
# shellcheck disable=SC2086
git -C "$root" diff --quiet "$base" -- $paths 2>/dev/null
rc=$?
[ "$rc" -eq 0 ] && exit 0
[ "$rc" -eq 1 ] || note "CANNOT CHECK: git diff failed (exit $rc) in $root. Allowed; UNKNOWN, not verified."

branch=$(git -C "$root" branch --show-current 2>/dev/null)
# shellcheck disable=SC2086
missing=$(git -C "$root" log --format='    %h %s' "HEAD..$base" -- $paths 2>/dev/null)
[ -n "$missing" ] || note "the hooks that will guard this worktree agent are read from $root on '${branch:-detached HEAD}', whose .claude/hooks or settings.json differ from $base without lacking any merged commit: that checkout's OWN unmerged hook code will judge the agent's commands. Allowed."

# shellcheck disable=SC2086
files=$(git -C "$root" diff --name-only "$base" -- $paths 2>/dev/null | sed 's/^/    /')
{
  echo "BLOCKED by worktree-hook-drift: a worktree agent is cut from $base, but every hook that guards it is"
  echo "read from $root, which is on '${branch:-detached HEAD}' and lacks these hook commits from $base:"
  echo "$missing"
  echo "Differing files:"
  echo "$files"
  echo "Until that checkout carries them, the agent is guarded by the OLD hook code: a merged guard rule is"
  echo "not in force for it. Either bring that checkout up to date (git -C \"$root\" merge $base, or check"
  echo "out ${MAIN_BRANCH:-main} there), or spawn the agent without worktree isolation."
} >&2
exit 2
