# Design: panel-29-lane-dialog

## Where the behaviour lives today

- **Page** (`framework/internal/panel/web/static/panel.js`):
  - `openStart(opts)` builds the dialog. With `opts.worktree` ("New lane here") it selects that
    worktree and "An existing worktree", and clears the template.
  - `stUpdate()` disables the lane type and every "Where it runs" radio when a template is chosen,
    and forces "new".
  - `pickNext()` (Up next) sets the template, the name and the issue, never where the lane runs.
  - The submit handler sends `{template, name}` for a template and `{type, mode, name, worktree}`
    without one.
- **Server** (`framework/internal/panel/lanes/lanes.go`):
  - `StartLane` renders the template, then refuses any mode but "new".
  - `Start` "new" checks that the worktree path is free, then runs `git worktree add -b <branch>
    <path> <base>`. A branch that already exists fails there, and the git error is passed through.

## The shape of the change

1. **Server: a template may start in an existing worktree, or on its existing branch in a new
   worktree.**
   - `StartLane` accepts `mode` "new", "existing" or "branch" with a template; "root" stays refused.
   - "existing" requires the worktree's branch to equal the template's rendered branch (D1).
   - "branch" runs `git worktree add <path> <branch>` for a local branch. For a branch that exists
     only on `origin` it runs `git worktree add --track -b <branch> <path> origin/<branch>`.
   - "new" checks the branch first. If it exists, the server answers 409 `branch-exists` with a
     plain sentence ("A branch named change/add-score-photo already exists. Start on its worktree,
     or on the branch in a new worktree.") and the facts the page needs (`branch`, `worktree` if one
     has it, `remote` if only origin has it).
   - The first prompt and the registry record are the same in every mode, so the lane looks the same
     to the rest of the panel.
   - "branch" is a lane mode like the others: the registry knows it, and it runs the same fetch,
     `.worktreeinclude` and `worktree_setup` as "new". The branch checks in "new" and "branch" run
     after the fetch, so an unfetched origin branch is seen. A refusal's facts reach the page in
     the HTTP reply, not only in the server's error.
2. **What the page needs to decide, without new polling.**
   - **Worktrees and their branches:** already in the state.
   - **Approval:** each open change's approval (approved, not approved, or no proposal) is added to
     the state, keyed by change id, from the changes the panel already reads for its metrics.
   - **A branch with no worktree:** the state has no branch list. When a template and a name are
     chosen, the dialog asks the server once, with a read-only call (`GET /api/lanes/branch?name=…`),
     whether that branch exists locally or on origin and which worktree has it. It asks again only
     when the template or name changes (D6).
3. **Page: the decision is one pure function, tested in node** (D4). `start-plan.js` exports
   `planStart(state, intent)`:
   - **Intents:** `{from: "next", template, item}` (an Up next row), `{from: "worktree", path}`
     ("New lane here"), `{from: "template", template, name}` (the template or the name changed),
     `{from: "branch", facts}` (the server's answer about the branch), `{from: "conflict", error}`
     (a 409 at Start, the fallback if the branch changed in between).
   - **It returns** `{template, name, mode, worktree, choices, warnings}`. `choices` says which
     "Where it runs" radios are enabled; `warnings` lists what's in the build's way.
   - panel.js calls it from `openStart`, `pickNext`, the template and name handlers, and the submit
     error path, then renders the result. This mirrors `term-links.js`, which is tested the same
     way.
4. **The rules `planStart` applies:**
   - **A template's branch with a worktree:** mode "existing", that worktree selected.
   - **A template's branch with no worktree:** mode "branch".
   - **Otherwise:** mode "new".
   - **"New lane here" on a worktree whose branch matches a template's pattern** (`change/{name}`
     → build, `fix/{name}` → fix, …): that template is selected, and the name is taken from the
     branch. When several templates share a pattern (Standing Tee's build and propose both use
     `change/{name}`), the one whose Up next lists that name wins. If none lists it, the first in
     the config's order wins (D5).
   - **A build template on a change whose proposal has no Approved line:** a warning, "add-score-photo
     has no Approved line in its proposal; /build-change will stop for it", and Start reads "Start
     anyway". A build template is one whose first prompt runs `/build-change`. A propose template, or
     a change with no proposal yet, gets no approval warning.
   - **The empty template option** reads "No template: a plain Claude session".
5. **Docs**: `docs/panel.md`'s New lane section and the in-page Help say what each "Where it runs"
   choice does with a template, and what the warnings mean.

## Refusals

| Situation | What happens instead |
|---|---|
| A template with mode "root" | 400: "A template names a branch, so it runs in a worktree, not the project root." |
| A template with "existing" on a worktree whose branch isn't the template's | 400, naming both branches. The page never offers it (D1). |
| "new" for a branch that exists locally or on origin | 409 `branch-exists`, a plain sentence plus the facts. The page switches to "existing" or "branch" and says why. |
| "branch" for a branch that exists nowhere | 400: "No branch named X; choose New branch and worktree." |
| "branch" when the target folder already exists | 409 `exists`, as today. |
| "branch" without a template | 400: the existing-branch choice belongs to a template's named branch; a plain lane uses "An existing worktree" or a new branch. |
| Templates while panel.json is untrusted | Unchanged: 409 `untrusted-config`. |

## Decisions (the owner decided all six as recommended, 2026-10-02, after the proposal's review)

D1, D2, D4 and D5 are as first approved. D3 is narrowed and D6 is new: the review found that
"unmet dependencies" has no structured source and that the page couldn't see a branch without a
worktree before Start.

**D1. Which existing worktree a template may run in.**
- **Recommended:** only the worktree whose branch is the template's own (`change/add-score-photo`
  for build `add-score-photo`).
- **Alternative:** any worktree.
- **Why:** a template types a prompt about one change. Running `/build-change add-score-photo` in
  another change's worktree would build in the wrong place, and the page has no honest way to warn
  about every such mismatch.

**D2. A branch that exists with no worktree.**
- **Recommended:** a separate, visible choice, "Its existing branch, in a new worktree", selected
  automatically and explained in one line.
- **Alternative:** "New branch and worktree" silently reuses the existing branch.
- **Why:** the label "New branch" would be false. Reusing a branch carries its commits, which the
  person should see before the lane starts on them.

**D3. Blockers shown before Start.**
- **Recommended:** one structured fact only: a build template on a change whose proposal has no
  Approved line, as a warning with "Start anyway". Never a hard block.
- **Alternatives:**
  - a hard block on a missing Approved line;
  - also warning on dependencies, by reading "needs …" out of an Up next row's detail text;
  - also matching founder-queue prose to the change.
- **Why:**
  - **Not a block:** Standing Tee records approval in prose until its swap, so a block would refuse
    a genuinely approved change. `build-change` already stops on its own for what it needs.
  - **No dependency warning:** an Up next row's detail is free text. This repo's never lists
    dependencies, and Standing Tee's lists them whether they're met or not, so the warning would be
    wrong either way. A structured "unmet dependencies" field in the suggest schema would fix that,
    but it isn't part of this change.
  - **No prose matching:** it would be a guess, and a wrong warning teaches people to ignore them.

**D4. Where the dialog's decision lives.**
- **Recommended:** a pure `start-plan.js`, unit-tested in node, with panel.js only rendering it.
- **Alternative:** keep the logic inline in panel.js, covered only by "is wired" string tests and a
  manual pass.
- **Why:** the rules above have a dozen branches. Today none of them is tested, which is how the
  forced "new" survived. The same pattern already tests the terminal's link matcher.

**D5. Two templates share a branch pattern** (Standing Tee: build and propose both name
`change/{name}`).
- **Recommended:** the template whose Up next lists the name. If none lists it, the first in the
  config's order.
- **Alternative:** don't pre-select a template when the pattern is ambiguous.
- **Why:** Up next already says which kind of work a name is waiting for (propose versus build), so
  it's the best evidence the panel has. A guess the person can change beats an empty field they must
  fill.

**D6. How the page learns that a branch exists with no worktree, before Start.**
- **Recommended:** one read-only server call when a template and a name are chosen (`GET
  /api/lanes/branch?name=…`), answering whether the branch exists locally or on origin and which
  worktree has it.
- **Alternatives:**
  - a list of branches in the polled state, refreshed every 10 seconds;
  - learning it only from a refused Start (a 409), then letting the person start again.
- **Why:**
  - **Not polled:** a branch list adds a `git for-each-ref` to every poll, which is the idle cost
    PANEL-25 removes.
  - **Not only after a refusal:** "one click" would really be "refused, then pick again". The 409
    path stays as the fallback for a branch created between the check and Start.
