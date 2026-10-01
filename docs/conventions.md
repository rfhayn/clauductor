# Conventions

The detail `AGENTS.md` points to. `AGENTS.md` is loaded by every agent, so it keeps only the
rules; the reasoning and the edge cases live here, read by range when a task touches them. The
sections marked **(yours)** are for your project to fill; the rest describe the operating model
and hold for any project that adopts it.

## Branching and PRs

The unit is **one change = one branch = one PR = one squash commit to `main`**.

- **Branch names.** `change/<id>` for a capability change: one approved proposal in
  `changes/<id>/` (the test: *does it change what a user can do?* If not, it is not a change,
  however much work it is). `fix/<issue>-<slug>` for a defect, `ops/<name>` for tooling, docs and
  process, including the session-close PR. The prefixes are `BRANCH_*` in `.claude/project.conf`
  and the panel's `lanes` map; change them in both.
- **One PR carries the whole slice**: the change's artifacts, the implementation, and the spec
  promotion (archive), reviewed together.
- **Squash-merge only.** Rebase-merging rewrites SHAs and auto-closes stacked PRs.
- **No stacked PRs.** If B depends on A, merge A first, then branch B from `main`.
- **Merge only through `merge-pr`**, which wants gate evidence for the head commit and converged
  review before it runs `gh pr merge --squash`. `pr-merge-guard` refuses `--auto`, `--admin`
  and a merge without evidence. A merge clicked on the website runs no hook: that is an accepted
  residual, not a gap to paper over with branch protection rules the agents cannot satisfy.
- **The main checkout stays on `main`.** Every hook is read from it; work happens in worktrees
  under `.claude/worktrees/<lane>`, one per lane, each cut from `origin/main`.
- **Propose just in time, at most ONE change proposed ahead** (`docs/principles.md`). To capture
  scope for later, write a roadmap row, not a change directory.
- **Every change states its slice**: `tasks.md` ends with a `Slice:` line.

## Commits

Imperative mood, one idea per commit. Commits written by the build-change workflow read
`<change>: task group N — <title>`. When `attribution.enabled` is true in
`.claude/model-roles.json`, every commit Claude writes ends with `attribution.trailer`, so the
history says which model wrote what; set it to false if your project keeps commits unattributed.

## The gate

`GATE_RUN` (`.claude/project.conf`, default `scripts/ci/run-local.sh`) is the full gate. It takes
the machine-wide gate lease first, so two worktrees never run it at once; runs every step in
`GATE_STEPS`; and only after a complete, clean run writes `<git-dir>/ci-receipt` naming the
commit. Agents run it through `GATE`, which keeps the full log in a file and prints only stage
markers, failure lines and a tail. `pr-merge-guard` accepts that receipt, or a named remote
workflow's success for the same SHA, as evidence. See `scripts/ci/README.md`.

## Review per task group, not per PR

Review when a task group is done, not when the PR is (*A review's blind spot is set by its
framing*). The build-change workflow does exactly that: it gates, reviews with a reviewer that is
not told what the builder intended, fixes, and re-reviews until no finding is medium or worse; it
stops for the owner if peak severity RISES between rounds or after three rounds without
converging. A change built by hand gets the same loop from the `apply-change` skill. A PR whose
diff is Markdown prose only gets the lighter `reviewer-docs` instead (`review-lane.sh`).

## Who decides, and what runs unattended

**The owner decides:** a change's proposal and every `design.md` decision; ADRs; a production
deploy and anything else irreversible; anything outward beyond this repo. **Everything else runs
without asking, merging included:** build, gate, review, fixes, commits, push, PRs, issues and
comments on this repo, and the squash-merge through `merge-pr`. A session runs `session-start` to
`session-close` hands-off and reaches the owner only by `PushNotification` (or your equivalent):
at an owner decision, an environment fault, a gate that stays red, or review not converging, and
once at the end, one line. Work that needs the owner at the computer goes in the owner queue.

**Several people.** Each person has their own Claude and merges only their own PRs. Claude's
memory is per machine: what another person needs goes in the repo. Shared files are resolved as
`session-close` describes (the journal renumbers, the insights log keeps both rows, ADRs take the
next number).

## Testing (yours)

What a test must prove in this project, what runs against real services, and what a reviewer
grades on. The principles that apply whatever the language are in `docs/principles.md`: a test is
evidence only once it has failed, and a falsification can fail at any link.

## Architecture rules (yours)

The load-bearing constraints of your code base: layering, the data layer, error handling, what
must stay pure. Keep each rule checkable, and give it a row in AGENTS.md's table once something
executes it.
