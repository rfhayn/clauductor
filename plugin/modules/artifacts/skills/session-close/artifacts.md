**Bring every core artifact current, then record the shared copies (the artifacts module).** The
module's guard rule refuses this close's PR (`ops/session-<N>-close`) while any artifact in the
registry is not `OK` at its head, so this is not optional.

1. **After step 3's roadmap edits** (a stamp taken before its sources move goes BEHIND again), run
   `clauductor-model modules/artifacts/bin/currency.sh --worktree 2>&1` and work every line that is not
   `OK`. For each `BEHIND` line, either:
   - **refresh** the artifact (its own skill, or edit the page; a walkthrough: Artifact `read` of its
     url, edit the saved copy, publish it back to the same `url` with no `capabilities`), then
     stamp it: `clauductor-model modules/artifacts/bin/currency.sh --stamp <key> --note "refreshed: <what changed>"`;
   - or **read it against the change and stamp it unchanged**, when the moved source changes nothing
     the artifact says: `--note "reviewed, no change: <why>"`. The reason is recorded in the
     registry. It is a claim about the page, so read the page first.

   Not an owner of a walkthrough: add an owner-queue line naming what it needs, and stamp it with a
   note citing that line. `CANNOT CHECK` is a declaration problem (a glob matching no file, a row
   that is gone, an untracked file): fix the entry's `authorities` (or `git add` the file), then stamp.
2. **Only with `ARTIFACT_PUBLISH="claude.ai"`: record, never publish, the shared copies.**
   `clauductor-model modules/artifacts/bin/publish.sh --status --worktree 2>&1`; for each `STALE` page
   you own (the ownership test in the module's session-start step),
   `clauductor-model modules/artifacts/bin/publish.sh --record <page>` writes the hash of the copy merge-pr
   will publish. Commit the registry to the close branch. `UNREGISTERED`: in the owner's session,
   publish it once WITHOUT `url`, add its URL to the registry, and `--record` it. `MISSING`: drop its
   entry. Not an owner: record nothing for that page, and name it in the **Done** notification.
3. **The close's LAST currency check**, in step 6 after `git merge origin/main` and before the gate:
   `clauductor-model modules/artifacts/bin/currency.sh --worktree --check 2>&1` must print no line that is
   not `OK`: what merged in can move a source. Clear each as in 1, commit, re-run it. Then the gate.
4. `merge-pr` includes the module's publish step: the pages this merge recorded are published from
   the merge commit. If it is blocked, name every page the close branch recorded
   (`git diff origin/main -- docs/artifacts.json`) in the **Blocked** notification.
