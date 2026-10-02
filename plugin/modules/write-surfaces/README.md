# Optional module: the write-surface advisory

A change should be small enough that its second review round finds nothing inside the first
round's fixes. The number of write surfaces it adds (routes, screens, handlers) predicts that
better than lines or commits: in Standing Tee, where this comes from (merge-guard rule 5), lines and
commits ranked a data corpus as the riskiest change, while added routes and screens picked out
exactly the change that never converged (10 surfaces) and the one that had to be split mid-flight
(4).

**Off by default.** Needs git only. **Advisory: it never blocks a merge.**

## What it does

`guard.d/write-surfaces.sh` runs in the merge guard on a capability change (a branch under
`BRANCH_CHANGE`). It counts the files the PR **adds** (from git, between the merge base and the
head) that match `WRITE_SURFACE_GLOBS`, and at `WRITE_SURFACE_MAX` (default 4) or more tells
Claude, naming them: if this is one capability, carry on; if it is several, split it now, which is
a roadmap edit, not a mid-review one.

Its limits: it counts added files only (a change that rewrites ten existing routes scores 0, and a
rename is not an addition), and it cannot tell a correctly large change from an oversized one.

## Switching it on

1. Set `MODULES="write-surfaces"` (with any others) in `.claude/project.conf`, and say what a write
   surface is in your project: space-separated shell patterns (`case` patterns, so `*` matches `/`).
   Standing Tee's:

   ```sh
   WRITE_SURFACE_GLOBS="*app/api/v1/*route.ts *apps/web/app/*page.tsx"
   WRITE_SURFACE_MAX="4"
   ```

2. `sh .claude/checks/run.sh write-surfaces:project` fails until `WRITE_SURFACE_GLOBS` is set, and
   fails a pattern that matches no file in the tree (a typo counts nothing, silently).

## Tested

`.claude/checks/write-surfaces.sh` (whether the module is on or not) runs the rule through the real
merge guard on the shapes the threshold was calibrated on, and on the ones that must stay quiet.
