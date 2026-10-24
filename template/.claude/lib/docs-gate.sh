# .claude/lib/docs-gate.sh: which diffs the docs gate may vouch for (OPS-89). Sourced, never run:
# by scripts/ci/run-local.sh --docs, which refuses any other diff before a step runs, and by
# .claude/hooks/lib/ci-receipt.sh, which accepts a `docs` receipt only for a PR whose own diff
# passes here. POSIX sh and git. The caller has sourced conf.sh (SPECS_DIR, CHANGES_DIR, MAIN_BRANCH).
#
# WHY. A full gate is about twenty minutes, and a PR whose diff is prose (a record, a roadmap row, a
# session close) gives its tests nothing new to run. The docs gate runs the model's steps and the
# project's `gate_steps docs` (the process checks, which read those records) and writes a receipt
# whose stage is `docs`, never `all`, so it can never stand in for the full gate on any other diff.
#
# PROSE is `*.md` and nothing else, EXCEPT what changes what runs or what the system is said to do:
#   - anything under a `.claude/` directory, at any depth (a SKILL.md or agent file is config);
#   - AGENTS.md, CLAUDE.md and CLAUDE.local.md, at any depth (every agent loads them);
#   - a SKILL.md, and anything under `agents/` or `commands/`, at any depth (a plugin's layout);
#   - specs: SPECS_DIR/**, CHANGES_DIR/*/specs/** (archived ones included) and openspec/**.
# Stricter than merge-pr's review-lane.sh (which reads root .claude/ and root AGENTS.md only): a
# gate skipped is worse than a review lightened. Prose the project's own TESTS read (a doc a test
# parses) is the project's to refuse, in its `gate_steps docs`. An empty diff is not docs. Every
# name matches in any case: a case-insensitive file system reads `Claude.md` as CLAUDE.md, and
# writes `Specs/x.md` into specs/.

# docs_gate_verdict: reads changed paths on stdin, one per line. Prints `docs (<n> path(s))` and
# returns 0 when every one is prose; else prints `full:` and the paths that are not, and returns 1.
docs_gate_verdict() {
  _dg_n=0 _dg_full=""
  _dg_sd=$(printf '%s' "${SPECS_DIR:-specs}" | tr 'A-Z' 'a-z')
  _dg_cd=$(printf '%s' "${CHANGES_DIR:-changes}" | tr 'A-Z' 'a-z')
  # `|| [ -n "$_dg_p" ]`: a last line with no newline is still a path.
  while IFS= read -r _dg_p || [ -n "$_dg_p" ]; do
    [ -n "$_dg_p" ] || continue
    _dg_n=$((_dg_n + 1))
    _dg_l=$(printf '%s' "$_dg_p" | tr 'A-Z' 'a-z')
    case $_dg_l in
      "$_dg_sd"/* | "$_dg_cd"/*/specs/* | openspec/*) _dg_full="$_dg_full $_dg_p" ;;
      .claude/* | */.claude/* | agents.md | */agents.md | claude.md | */claude.md | claude.local.md | */claude.local.md) _dg_full="$_dg_full $_dg_p" ;;
      skill.md | */skill.md | agents/* | */agents/* | commands/* | */commands/*) _dg_full="$_dg_full $_dg_p" ;;
      *.md) ;;
      *) _dg_full="$_dg_full $_dg_p" ;;
    esac
  done
  if [ "$_dg_n" -eq 0 ]; then echo "full: no changed path (an empty diff is not evidence of a docs-only one)"; return 1; fi
  if [ -n "$_dg_full" ]; then echo "full:$_dg_full"; return 1; fi
  echo "docs ($_dg_n path(s))"
}

# docs_gate_paths REV [worktree]: the paths REV changes against its merge-base with
# origin/MAIN_BRANCH, without renames (a rename would list only its destination), with every
# submodule bump (a `.gitmodules` `ignore = all` would hide one), and from the root whatever the cwd
# or `diff.relative` says. A caller deciding a merge fetches origin/MAIN_BRANCH first: against a
# stale one, a PR that reverts main's newer code lists no code. With `worktree`,
# also what the working tree changes against HEAD and every untracked file. Fails when it cannot
# compute them (no origin/MAIN_BRANCH, a shallow clone, a git older than 2.28, which has no
# --no-relative): never an empty list.
docs_gate_paths() {
  _dg_ref=origin/${MAIN_BRANCH:-main}
  [ "$(git rev-parse --is-shallow-repository 2>/dev/null)" = false ] || return 1
  _dg_base=$(git merge-base "$_dg_ref" "$1" 2>/dev/null) || return 1
  _dg_out=$(git diff --name-only --no-renames --ignore-submodules=none --no-relative "$_dg_base" "$1" 2>/dev/null) || return 1
  if [ "${2:-}" = worktree ]; then
    _dg_wt=$(git diff --name-only --no-renames --ignore-submodules=none --no-relative HEAD 2>/dev/null) || return 1
    _dg_un=$(git -C "$(git rev-parse --show-toplevel)" ls-files --others --exclude-standard 2>/dev/null) || return 1
    _dg_out=$(printf '%s\n%s\n%s\n' "$_dg_out" "$_dg_wt" "$_dg_un")
  fi
  printf '%s\n' "$_dg_out" | sed '/^$/d' | sort -u
}
