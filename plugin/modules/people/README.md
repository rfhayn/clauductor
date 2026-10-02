# Optional module: people

For a project where more than one person works, each with their own Claude. It adds a **people
registry** (the one place a GitHub login maps to a person), a **who-is-on-what** block in
session-start, a **lane table** (who owns which part of the repository), a **roadmap owner rule**
(every `**Owner:**` must be a person in the registry), and rows for session-close's shared-file
conflict table. The core already marks another person's open PRs (`★`) and the files they share
with your branch (`OVERLAP`); this module adds who those people are and what each is on.

**Off by default.** Nothing here runs until `MODULES` names it. It needs `gh` and `jq`, no Node.

## Switching it on

1. Copy `people.example.json` from this directory to `docs/people.json` (or wherever `PEOPLE`
   points) and fill it: one entry per person with their `name` (exactly what the roadmap's
   `**Owner:**` lines say), their GitHub `login` under `github`, and their `role` (the owner's is
   the project's `OWNER_ROLE`). `lanes` is optional: one `{"lane", "owns"}` per lane, in the order
   the table should show them.
2. Add `people` to `MODULES` in `.claude/project.conf`.
3. Run `sh .claude/checks/run.sh people:registry`: the registry is sound and every roadmap owner is
   a person in it.

## What it adds

| Part | Runs from | What it does |
|---|---|---|
| `people.sh` | everything below | The one reader of the registry. `--check`, `--names`, `--logins`, `--lanes`, `--owners`, and `--activity` (who is on what, from GitHub reads handed over as files; no network of its own). |
| `context.d/session-start/who.sh` | session-start's context | Reads the open PRs, the remote branches with no PR (and the GitHub account of each one's last commit), and each person's merged PRs, then prints per person **Last** (latest merged PR and the row it names), **Now** (open PRs and PR-less branches) and **Next** (their first unfinished roadmap row nobody has a PR or branch for). Work by a login nobody maps to is listed, never dropped. A read that failed says `CANNOT CHECK`. |
| `context.d/session-start/lanes.sh` | session-start's context | The lane table, from the registry's `lanes`. |
| `skills/session-start/people.md` | session-start's *Project steps* | Repeat the who block in the summary; stay in your lane; never merge another person's PR. |
| `roadmap.d/owners.sh` | `roadmap_queue`, on every read of the queue | A row whose owner is not a person in the registry makes the queue UNKNOWN, naming the row. `PEOPLE_OWNERS="off"` turns this rule off. |
| `conflicts.tsv` | session-close's shared-file table | The registry and the roadmap's owner lines. |
| `checks/registry.sh` | `checks/run.sh` as `people:registry` | The registry is sound; the roadmap passes with the owner rule. |

A row names a person through the roadmap's `**Owner:**` line over it (the phase's, or its `###`
section's). A PR or branch is tied to a row by the row's change id in the branch name or title,
else by its row id as a whole word (an all-digit row id is not matched that way: a bare number in a
title is more often a count). A project with its own roadmap parser (`ROADMAP_PARSER`) may emit
the optional 14th `--tsv` column, the row's raw status (`docs/roadmap.md`, the `--tsv` contract);
a status that reads `⬜ deferred` is then never
anyone's **Next**, since a deferred row waits on an event, not a person.

## Keys

| Key | Default | Meaning |
|---|---|---|
| `PEOPLE` | `docs/people.json` | the registry |
| `PEOPLE_OWNERS` | `required` | `off` stops the roadmap owner rule |
