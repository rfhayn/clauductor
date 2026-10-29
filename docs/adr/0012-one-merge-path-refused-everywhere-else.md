# ADR 0012: One merge path: `merge.sh` merges, and `gh` refuses every other merge

- **Status**: Proposed
- **Date**: 2026-10-29
- **Tags**: merge, controls, hooks, parsers
- **Source**: roadmap row OPS-86 (S16); insights-log row *shell parser never converges*
  (2026-10-17); the review records of #103, #112, #146, #268 and #313; Standing Tee's PRAC-0041,
  its row `2C.30`, PR #681 and the parked #698 (read in `~/Development/standingtee` and on
  GitHub, 2026-10-29)

## Context

`pr-merge-guard.sh` is the whole merge gate: Claude merges without waiting for the owner, and no
server-side check can see a local gate run. **Its rules are sound. How it finds a merge is not.**
It reads the Bash command's text with `hooks/lib/merge-reader.awk`, a tokenizer that knows enough
bash and zsh to find `gh pr merge` and `gh api …/merge` in any spelling, and refuses what it
cannot read. The detection is most of the code: the hook is 1,178 lines, the reader 625, and
`checks/merge-guard.sh` 1,208 (503 cases at #268).

**It has not converged, and each fix lists one more shell form.** Every October PR that changed how
the guard reads a merge ran past its first round:

| PR | Row | Review |
|---|---|---|
| #103 | OPS-52, non-ASCII words | 3 rounds, peak above medium |
| #112 | OPS-57, a PR word read whole | 4 rounds, peak medium |
| #146 | OPS-66, false refusals | 5 rounds, peak high in rounds 2–5, each a new bypass live on `main` |
| #268 | OPS-132, only an open PR merges | 13 rounds; rounds 6–12 each peaked high |
| #313 | OPS-141, a renamed repository | 3 rounds, peak high |

#146's rounds found redirections placed mid-command (three rounds, more forms each time), a `(`
from process substitution or a zsh glob qualifier, then a regression from narrowing the rule.
#268's rounds found that a text match on `GH_REPO`, `GH_HOST` and `cd` falls to `GIT_DIR=`, `cd;`,
`c\d` and a split `export`. PRAC-0010 records the pattern: a review that refutes one instance at a
time searches a space the fix never bounded.

**Known limits are left,** listed in the hook header's KNOWN LIMITS and in #268's body, and the
owner's call on #146 round 5 was to list them and rethink the guard here rather than close each
one:
- a definition inside quoted program text: `eval 'f() { gh "$@"; }'; f api …`, `sh -c 'alias …'`;
- zsh's parameter tables: `functions[jq]='gh "$@"'` or `commands[jq]=/usr/bin/gh`, then `jq api …`;
- git's own aliases: `git -c alias.y='!gh pr' y merge 5`;
- a command whose every word is computed (`$A $B $C 5`), a program that is computed, piped or in a
  file, and a shell profile's aliases, functions and environment;
- a merge through another client and a click on the website (the header); a merge between the
  guard's reads and the merge itself (#268's KNOWN LIMITS, *time of check*).

**It refuses work that is not a merge.** Agents routed commands around it (#120, fixed by #146 for
the forms found then). While this record was drafted, it refused three commands that merge
nothing, in this row's lane on 2026-10-29, the first two seen twice each:
- `gh pr merge --help` piped into a `grep` for `--auto`: "'--auto' merges before checks settle";
- `gh help pr merge`: "this command may merge in a form this guard cannot read: the merge reads
  as [help pr merge], and what comes before pr is not a plain gh";
- a heredoc holding this record's own edits, refused as an `--auto` merge.

**Its purpose is narrower than its parser.** The owner set it on 2026-10-12 (OPS-58's row): an
accidental merge without evidence, not a determined adversary with the same token on the same
machine. The bypasses the reviews found are adversarial forms; the false refusals are the cost
paid every day.

**Standing Tee met the same wall and moved the judgment.** Its PRAC-0041 (Accepted by its founder,
2026-10-09) puts a wrapper, `.claude/bin/gh`, first on PATH. The wrapper receives the argv the
shell built, with every quote, expansion, alias, `eval` and `sh -c` already resolved, applies the
guard's rules to a merge, pins the merge to the judged head (`--match-head-commit`, or `sha` in a
`gh api` merge's body), and refuses a merge route it cannot classify. What happened next, read on
2026-10-29:
- **PR 1 merged** (#681, 2026-10-11): the wrapper, its route and a per-call route check. Its
  measurement found that `CLAUDE_CODE_SHELL_PREFIX` loses to the shell snapshot's `export PATH`,
  while a `CLAUDE_ENV_FILE` line written on SessionStart holds in a main session, a worktree, after
  a `cd`, and in a subagent.
- **PR 2 is parked** (#698, a draft). It deletes the reader and keeps only refusals of the ways
  past the wrapper a command's text can show. It ran 8 rounds, peak high: round 6 found a
  regression of round 4's fix in `inword()`, a grammar of parameters glued into a gh location. The
  founder chose to refuse any gh location holding a `$` instead, then parked the PR until after
  Standing Tee's go-live. So Standing Tee runs the reader AND the wrapper today.
- **The wrapper has false refusals of its own.** On 2026-10-21 it refused a read,
  `gh -R … pr view 778 --json state,mergeCommit`, because `-R` came before the command and the
  argv held `merge`: a substring, not gh's grammar.

Two lessons come from that record. Moving the judgment to the argv closes every spelling of a
merge through `gh` on PATH. But what the hook must still read, the ways past the wrapper, does not
converge either when it is a grammar; it converges only as a coarse refusal.

**Clauductor already ships the route.** `hooks/pattern-kill-path.sh` writes `.claude/bin` first on
PATH through `CLAUDE_ENV_FILE` on SessionStart and drops any alias or function of the wrapped name,
so `.claude/bin/pkill` and `killall` refuse every call (ST-14). `no-pattern-kill.sh` refuses the
ways past them that a command shows: a path, `command -p`, `env PATH=`. A `gh` wrapper would ride
the same route.

## Options

| Option | What executes it | Closes | Leaves | Cost |
|---|---|---|---|---|
| **A. Keep the parser** | `pr-merge-guard.sh`, `merge-reader.awk` | each form a review finds, one at a time | the limits above; every form not yet found | review rounds per guard PR (above); false refusals; OPS-58 and others keep adding forms |
| **B. One named merge command, read as text** | the hook allows a merge only as `sh .claude/merge.sh <n>`, whole, and refuses any other command whose text holds `gh` and `merg` | the three listed limits, when their text names gh and merge | an all-computed or encoded program, as A | more false refusals than A: no grammar, so `gh pr view 5 --json mergedAt` and `gh help pr merge` are refused |
| **C. Standing Tee's wrapper (PRAC-0041)** | `.claude/bin/gh` applies every rule to the argv; the hook keeps a route check and the ways past the wrapper | every spelling through gh on PATH, aliases and functions included; the moving head | a gh reached without PATH (a computed path, a script that rebuilds PATH); other clients; the web | a PATH route that must hold in every session kind; the rules live in a wrapper on every gh call; ST's PR 2 did not converge on its grammar |
| **D. One merge path, refused everywhere else at the argv** (recommended) | `.claude/merge.sh` holds the rules and is the only thing that merges; `.claude/bin/gh` refuses every merge route, with no exceptions and no rules; the hook keeps a route check and coarse refusals of the ways past the wrapper | as C | as C | as C's route; three PRs at high risk |
| **E. Server-side required check** (a complement) | a ruleset on `main` requiring the remote `test` check, with no bypass actor | a click on the website, curl, another client, a Cursor agent | the local gate receipt and rules 3 and 7–14, which no server check sees | every PR, the owner's included, waits for Actions; possible only once Actions run (OPS-98, or public at REL-1) |

**Why D over C.** Both judge where the argv is final, and both close the listed limits by
construction. D splits C's wrapper in two:
- **The wrapper only refuses.** It answers one question, "is this a merge route?", the shape of
  the pkill wrapper. No rule, no evidence read, no exception for a caller it would have to trust.
- **The rules run in one named command** a person or an agent can run, read and test without a
  PATH route. `merge.sh --explain <n>` takes OPS-92c's `--explain`, which that row puts on
  `pr-merge-guard.sh` and moves to `merge.sh` "if OPS-86 picks B"; D needs the same move (below).
- **A mod should not undo it.** A mod can approve a tool call a `PreToolUse` hook refused (OPS-32),
  but the wrapper refuses inside the command's own process, after any approval. That is expected,
  not tested: D3's measurement includes a session with such a mod loaded.

**Why not B.** B keeps the text as the authority, so it cannot tell a read from a merge without a
grammar, and a grammar is A again.

**Why not A.** Five PRs in a month, peaks of high in at least three (#146, #268, #313; #103's record says only "above medium"), and the limits that remain need a
different design, as the owner said on #146.

**E is not an alternative.** It cannot see the local gate, and Standing Tee's founder rejected it
as a replacement (a `workflow_dispatch` run cannot satisfy a required check, and it gates every
PR). As a complement, it is the only control that covers a click on the website or a Cursor agent,
which runs no Claude Code hook. Whether to add it is a separate question for the owner (below).

## Decision

**Proposed: option D.** The record states each decision and the requirements its build must meet.
Mechanisms belong to the build PRs, where they are tested in real shells: Standing Tee's record
found that mechanism detail in a design record is reviewed as if it were the parser (its
PRAC-0017 amendment of 2026-10-09).

**D1. One merge path.** `.claude/merge.sh <pr>`, with a squash body file, is the only command that
merges this repository's PRs. It runs every rule `pr-merge-guard.sh` and `lib/change-guard.sh` run
today (1–4 and 7–14), reads the PR once, judges that head by SHA, and merges pinned to it
(`--match-head-commit`). It never passes `--auto` or `--admin`. `merge-pr` step 4 calls it; a
refusal names the rule and the fix, on the caller's stderr.

**D2. `gh` never merges by itself.** A wrapper, `.claude/bin/gh`, first on PATH through the route
`pattern-kill-path.sh` already writes, refuses every merge route gh offers: `pr merge`, a `gh api`
merge, a GraphQL `mergePullRequest` or auto-merge mutation, and any route it cannot classify that
could merge (an alias or extension, a computed endpoint, an `--input` body it cannot read). It
reads gh's grammar by position, never a substring: `gh -R o/n pr view 5 --json mergeCommit` passes
(Standing Tee's false refusal is an acceptance case). Every other gh command execs the real gh
unchanged. `merge.sh` reaches the real gh past the wrapper, by a lookup that skips the wrapper's
directory: a computed gh location. D4 reads the command, not the script it runs, so `merge.sh`
passes; nothing else in the model may take that route. An agent that writes the same lookup into
its own command is, subject to open question 3, either refused by D4 (a location holding a `$`) or
left to the first known limit below (a computed path the text does not show).

**D3. The route is measured, and fails closed.** The build measures, through the Bash tool, that
`gh` resolves to the wrapper in each kind of session that can merge here: a main session, a
worktree lane, a panel lane, a subagent, before and after a `cd`, a plugin install, and a session
with a mod loaded that handles `tool.call` (the mod's approval must not reach the wrapper). A kind
where the route is not established refuses any command holding a `gh` word and the letters `merg`,
naming the cause. If no route holds in a main session, the build stops for the owner.

**D4. The hook stops looking for merges.** What is left of `pr-merge-guard.sh` is the route check
(D3) and coarse refusals of the ways past the wrapper a command's text shows: a literal path to a
gh that is not the wrapper, a command that changes how names resolve (a PATH assignment beside a
gh word, `command -p`, `env -i`, `hash`), and a gh location holding a `$`. Each is an over-read with
no grammar, the form Standing Tee's founder chose after #698's round 6. `merge-reader.awk`,
`merge_hint`, the prose allowlist, `foreign_shape` and the raw-JSON fallback are deleted.

**D5. A lane cannot loosen its own path.** An edit to the wrapper or `merge.sh` in a lane's
worktree must not change what judges its merge. No executor does this today:
`worktree-hook-drift.sh` compares only `.claude/hooks/` and `.claude/settings.json` with
`origin/<main>`, and only when a worktree agent is spawned. So D6 PR 1 makes `merge.sh` refuse to
run while its own copy differs from `origin/<main>`'s, and D6 PR 2 does the same for the wrapper
and extends the drift hook to `.claude/bin/`, each refusal seen first on an edited copy.

**D6. Three PRs in sequence, each a roadmap row at Risk high.** None of the rows exists yet. The PR
that records the owner's acceptance (the owner queue's S16 item, *OPS-86: accept or amend its
ADR*) adds all three to the roadmap's merge-guard section, and E's row if question 4 says so:
1. `merge.sh` with the rules moved and the pin; `merge-pr` calls it; the hook still runs.
2. The wrapper and D3's measurement; the hook still runs.
3. D4: the reader deleted. `checks/merge-guard.sh`'s block rows are ported as acceptance cases
   against the wrapper under bash 3.2, bash 5 and zsh, each seen merging a stub gh without the
   control first. Its allow rows, #120's commands and the three refusals above become allow cases.

Each PR moves AGENTS.md's What-executes rows for the executors it moves, in the same PR.

**Known limits, accepted under the guard's purpose:**
- a gh reached without PATH: a computed path the text does not show, a script or make target that
  rebuilds PATH, a sourced file that does (Standing Tee's #617 lists the same);
- a session kind or dotfile D3's measurement did not cover: the route check proves configuration,
  the measurement proves resolution on the machines it ran on;
- any client but gh, a click on the website, and an agent that runs no Claude Code hook (a Cursor
  lane): only E covers these;
- the evidence `merge.sh` reads is self-reported, as the gate receipt is today.

## Consequences

### Positive
- Every spelling of a merge through gh on PATH is refused unless it is `merge.sh`, so the listed
  limits close by construction, and so does a merge a script or child process runs through PATH.
- The shell-reading code goes (the reader's 625 lines and the hook's detection), and with it the
  false refusals of reads and the review surface that did not converge.
- The merge lands only the head that was judged; today the guard judges a head, and the merge it
  allows is not pinned to it.
- The rules become a command: OPS-92c's `--explain`, OPS-77's merged-tree evidence and OPS-78's
  review line are written once, in `merge.sh`, and tested by running it.

### Negative / trade-offs
- The guard depends on a PATH route in every session kind. An unconfigured kind refuses rather
  than passes, but with the coarse letters test until it is fixed.
- Every gh call pays one more exec.
- A merge in another repository from here is refused by the wrapper unless `merge.sh` accepts one
  (an open question below); today the hook allows an allowlisted shape.
- Three high-risk PRs, each with its review rounds. Standing Tee's PR 2 did not converge on a
  grammar of the ways past the wrapper; D4 keeps that part coarse for that reason, and pays in
  false refusals of a computed gh location (`$GH pr list`).

## What this decides for other rows
- **OPS-58** (the `gh api` merge in any command-word form, limits by class, #118's allow case):
  **cancel.** Its forms close by construction at the argv, the reader it extends is deleted, and
  its cases become D6 PR 3's acceptance rows.
- **OPS-32** (a mod can undo a blocking hook): **keep, rescoped.** The merge rules leave
  `PreToolUse` for the wrapper and `merge.sh`, which a mod's approval cannot undo; what the row
  names becomes the guards that are still only a `PreToolUse` refusal (`no-blind-source-rewrite.sh`,
  the merge hook's route check and D4's refusals, and the other blocking hooks). It no longer waits
  on this record.
- **OPS-68** (the rewrite hook also guards `changes/**`, `specs/**` and `.html`): **keep, and drop
  its dependency on OPS-86.** It changes which files `no-blind-source-rewrite.sh` protects, not how
  any reader reads shell, and that hook is not the merge guard. This record does not decide the
  rewrite hook's own parser.
- **OPS-77 and OPS-78** (S21) read D1: merged-tree evidence and the review line are rules of
  `merge.sh`, judged on the one PR read and its pinned SHA, and their refusals reach the merging
  agent on stderr. If either lands before D6 PR 1, it lands in `lib/change-guard.sh`, which moves
  under `merge.sh` whole. OPS-77's own ADR (the cost of merged-tree evidence) is unaffected.
- **OPS-156** (no `Co-authored-by` in a squash, queued in S16) lands in the hook now and moves with
  the rules in D6 PR 1.
- **OPS-92c** (lite's guard): **keep, its PR c1 rescoped.** c1 puts `--explain` on
  `pr-merge-guard.sh`, whose rules D6 PR 1 moves out and whose detection D6 PR 3 deletes. Under D,
  `--explain` is `merge.sh --explain <pr>`: the same verdict from the same rules. If c1 is built
  before D6 PR 1, it moves into `merge.sh` with the rules. c2's lite table is unaffected.

## Open questions for the owner
1. Accept D, or another option.
2. A merge in another repository from a session here: `merge.sh -R` with rule 1 only, as today's
   `foreign_shape`, or refused (merge there, under that repository's own guard).
3. A gh location holding a `$` (D4): refused outright, as Standing Tee chose, or a known limit.
4. E as a complement once Actions run (OPS-98, or public at REL-1): a row now, or not.
5. Order against S21: build D6 before OPS-77 and OPS-78, so each writes its rule into `merge.sh`
   once, or let them land in `change-guard.sh` first.

## Enforcement
**Nothing yet: this record is Proposed, not built.** The PR that records the owner's acceptance
adds D6's three rows; until they merge, the guard keeps the limits its header lists. Once built:
`.claude/merge.sh` and `lib/change-guard.sh` for the rules; `.claude/bin/gh` for the refusal of
every other merge; `pr-merge-guard.sh` for the route check and D4; `checks/merge-guard.sh` for the
acceptance cases; for D5, the drift refusals of `merge.sh` and the wrapper, and the extended
`worktree-hook-drift.sh`.

## Related
- PRAC-0010 (close the class, not the instance): this record applies it to the merge guard.
- `docs/principles.md`, *A check that reads source is a parser*.
- Standing Tee: PRAC-0041, row `2C.30`, #681, #698, #617.
- ST-14 (`pattern-kill-path.sh`, `.claude/bin/pkill`): the route D2 reuses.
- OPS-32, OPS-58, OPS-68, OPS-77, OPS-78, OPS-92c, OPS-156; #120, #146, #268.
