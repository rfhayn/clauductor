---
name: session-close
model: opus
effort: medium
description: "Close out a working session hands-off: land this session's PRs through merge-pr, archive finished changes, update the roadmap, journal, insights and owner queue, land the close PR itself, quiet the machine, and notify the owner once. TRIGGER when the user says 'session close', 'wrap up', 'close out the session', 'let's finish for today', or asks whether everything is cleaned up."
---

# Session close: make session-start's context block TRUE

**The contract in one line: `session-close` exists to make the next `session-start` true.** That
skill reads git state, the change queue, the journal, the insights, the ADRs, the owner queue and
the lanes. Every one is a claim about the present; a session that ends without updating them hands
the next one a confident, specific, wrong picture, and the failure is silent because a stale
document reads exactly like a current one. So the checks below **run** rather than **ask**.

## Context: the state, computed
!`sh ${CLAUDE_PLUGIN_ROOT}/skills/session-close/context.sh`

If the line above shows as literal text instead of output, run
`clauductor-model skills/session-close/context.sh` yourself and read the result before step 1.

## Hands-off, and who hears about it

This skill runs to the end **without waiting for the owner**, merges included (AGENTS.md, *Who
decides*). The owner is told at most twice, both with **`PushNotification`** (a deferred tool: load
it with ToolSearch `select:PushNotification`):
- **Blocked**: only an owner decision (a design issue, an ADR, a deploy, anything irreversible or
  outward), an environment fault, a gate that stays red, or review that does not converge. One
  line: what stopped, where, what you need. Then stop.
- **Done**: once, as the last action. One line: what landed and what is next.

If `PushNotification` is not in your tool list, say so in the final message; do not substitute
another channel (an issue comment, email).

## Several people, one set of shared files

When more than one person runs sessions here, every close edits the same files, so the second
close to merge conflicts, deliberately (it forces step 6's merge of `origin/main`):

| file | on conflict |
|---|---|
| the journal | keep **both** entries, yours on top, renumbered to one past `origin/main`'s top (`pr-merge-guard` rule 7 blocks a duplicate) |
| the insights log | keep both rows |
| the roadmap | keep both sides' status edits; `clauductor-model roadmap-queue.sh --check` must pass |
| `docs/adr/` | keep both index rows; take the next free number for **yours** (file, title, index, every citation) |
| the owner queue | keep both sides' items |

The enabled modules and the local layer add their own shared files (`conflicts.tsv`: the file, a
TAB, what to do). Their rows, if any:

!`sh ${CLAUDE_PLUGIN_ROOT}/extensions.sh conflicts`

## Steps (in order: each one's output changes what the later ones say)

### 1. Land the code
- Uncommitted changes: commit them, or say plainly why they are being left.
- Each open PR of this session's that is yours: **`merge-pr`**, now, without asking. A PR that is
  not yours is never merged here.
- Then `git fetch --prune`, and confirm the remote branch list against GitHub, not local refs.

### 2. Archive what finished, check what is proposed
- A change whose tasks are all `[x]` and whose PR merged: **`/clauductor:archive-change`**. Archiving promotes
  its spec deltas into the living specs; skipping it leaves the spec tier one change behind.
- **At most ONE change may remain proposed.** Two in the Context block is the finding.

### 3. Update the written record
- **The roadmap** (`ROADMAP`), statuses first, because session-start reads the queue off it: a
  merged change `✅ merged (#N)`, an open PR's row `⬜ in flight (#N)`, a dropped one
  `❌ cancelled — <why>`. Nothing else parses. Then `clauductor-model roadmap-queue.sh --check`.
- **`/clauductor:log-insight`** for anything non-obvious found today. If the Context block counts `0` rows
  today after a substantive session, that is the finding. Then the promotion check: a topic at 3+
  Raw rows is the trigger for `/clauductor:new-adr`.
- **The owner queue**: anything left that needs the owner at the computer, as a `- [ ]` line with
  the date and what it needs first; tick what this session did.
- **`/clauductor:dev-journal`** last, once the facts are settled: number and author from the Context block's
  `Journal:` line. Put the Context block's **Flow** line and this session's cost total (its
  `usage-report.sh` table, by role and model) in the entry: the model chosen for each role is a
  hypothesis until its cost sits next to what the role caught. A `CANNOT CHECK` there is said as
  such, never as $0.

### 3b. Compound: feed the lessons back into the rules
The Context block's **Compound** section lists every insight still `Raw`, oldest first, with the
number of sessions it has waited (`NEW` = logged since the last close). A lesson that is only
logged teaches nothing; this step decides each one, deliberately:
- **Promote** it: a new ADR or an amendment (`/clauductor:new-adr`), or a check or hook that now executes it
  (AGENTS.md rule 4). Status `Promoted → ADR-NNNN` (or `→ checks/<name>.sh`).
- **Mark it an instance** of a rule or ADR that already covers it: find the covering sentence
  first. Status `Instance of ADR-NNNN (…)`.
- **Record why not**: `Not promoted — <why>` (a one-off; no mechanism would have caught it).

Leaving a row Raw is allowed while it is young, but `checks/compound.sh` fails the gate once a row
has waited `COMPOUND_MAX_SESSIONS` sessions. Say in the journal entry which rows you promoted.

### 4. Set the forward-looking status line
`clauductor-model status-write.sh "[main] <what just landed>; next: <the actual next action>"`: written
for the person opening the next session.

### 5. Verify by existence, never by this skill's report
Re-run the Context block and read it (AGENTS.md rule 2). The end state: clean tree, nothing ahead
of `origin`, no unaccounted open PR of yours, no unarchived finished change, at most one proposed,
journal and insights carrying today's date, `clauductor-model checks/run.sh` green, a status line
pointing forward. Then state what is still outstanding **and the change that owns it** (AGENTS.md
rule 1); if no change owns it, creating one (a roadmap row, an issue) is part of closing.

### 6. Land the close itself, quiet the machine, notify
1. The records from steps 2–4 are a PR like any other: branch `ops/session-<N>-close`, commit,
   push, `gh pr create`.
2. `git fetch origin main && git merge origin/main`, **every time**, even with no conflict shown:
   someone may have closed while you wrote. Resolve per the table above.
3. The gate (`scripts/ci/gate.sh`, no flags): the merge commit is a new head and needs its own
   receipt.
4. **`merge-pr`**, all of it. For a docs-only diff its review step takes the `clauductor:reviewer-docs` lane.
   If rule 7 blocks, renumber and repeat from 2. Anything else that blocks and is not mechanical is
   a **Blocked** notification.
5. **Leave the machine quiet**: from the main checkout, after the last gate,
   `clauductor-model machine-quiet.sh` (try `--dry-run` first if unsure). Put its output in your final
   message; a `KEEP` line is a finding to name. It never kills tmux (the panel's lanes), removes
   no dirty or locked worktree, and removes none while the live sessions cannot be read; run it
   with no subagent of yours in flight.
6. **Done** notification, one line: e.g. `Session 12 closed: #41 merged; next: 1.3 build`.

## Rules
- **Run the checks; do not narrate them.** Every claim in the closing summary traces to a
  command's output in this session.
- **A check that could not run learned nothing.** `gh` failing on expired auth is not "no open PRs".
- **Close the lanes**: every build, proposal and fix lane is landed, handed off in the journal, or
  stopped, and the main checkout is back on `main`.
- **Do not manufacture work to look thorough.** If the record is current, say so and stop.
- **Never force the gate green by weakening a test.** A red gate at close is the session's real
  finding, and it goes in the journal.

## Project steps

What this project's enabled modules and its local layer (`.claude/local/skills/session-close/`) add to this
skill. Follow them as part of the steps above:

!`sh ${CLAUDE_PLUGIN_ROOT}/extensions.sh fragments session-close`
