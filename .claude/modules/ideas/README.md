# Optional module: ideas (a shared queue of ideas, rendered into the repository)

People have ideas away from the keyboard. This module gives them one place to put them: a claude.ai
page (the Ideas page) whose database is the queue, writable from the phone, the desktop app or the
web, and `/ideas` from Claude Code. Every session-start counts it; every session-close renders it
into the repository as `IDEAS_FILE`, so the queue sits beside the roadmap as planning context. The
owner decides which ideas become roadmap rows.

**Off by default, and claude.ai-only.** It requires the `artifacts` module (the page's url lives in
its registry). The queue is read and written only through the `ArtifactData` tool, so a session
without it (not signed in to claude.ai) says `CANNOT CHECK — no Artifact tool` and leaves the file
as it is. Everything in the repository is sh and jq: no Node.

## Switching it on

1. `sh .claude/modules/ideas/enable.sh` makes, where they do not exist: `.claude/skills/ideas` (the
   `/ideas` skill), `IDEAS_PAGE` (default `docs/ideas.html`, from `page/ideas.html` with the
   project's name and owner filled in) and `IDEAS_FILE` (default `docs/ideas.md`, rendered from an
   empty queue). Map the skill in `.claude/model-roles.json` (`"ideas": "orient"`) and add its row
   to the playbook's skills table.
2. Publish the page once from the owner's session with the Artifact tool, with no `url` and
   `capabilities: {"db": {}, "user": {"scopes": ["profile"]}}` (`db` holds the queue; `user` names
   who added each idea). Every later republish omits `capabilities`, which keeps them; `{}` would
   revoke both. Invite the people who add ideas by email as Editors.
3. Register it in the artifacts registry under `"pages"`, with its url, authorities (say
   `[".claude/modules/ideas/skill/SKILL.md", "docs/ideas.html"]`) and a stamp
   (`currency.sh --stamp docs/ideas.html --note "seed: the Ideas page"`).
4. Add `ideas` to `MODULES` (after `artifacts`) in `.claude/project.conf`.

## What it adds when on

| Point | Part | Does |
|---|---|---|
| `context.d/session-start` | `ideas.sh` | the page's link, and the counts and `data-through` stamp of what the last close rendered |
| `skills/` | `session-start/ideas.md` | the live count: dump the collection, `render.sh --summary --against IDEAS_FILE`, one line verbatim |
| | `session-close/ideas.md` | set the status of each idea the session discussed, dump, resolve names, `render.sh --out IDEAS_FILE`, commit |
| `conflicts.tsv` | `IDEAS_FILE` | never hand-merged: rendered again on the merged tree |
| `checks/` | `queue.sh` | `ideas:queue`: `IDEAS_FILE` is the renderer's output, unedited; the page has a url and a source; the skill is the module's |

`.claude/checks/ideas.sh` tests the parts themselves whether the module is on or not.

**The tool** (`bin/render.sh`; from the plugin, `clauductor-model modules/ideas/bin/render.sh`):

| Call | Does |
|---|---|
| `render.sh <dump> [--names f] [--out f]` | render the dump (`<dump>/ideas/<doc_id>.json`, as `ArtifactData` `list` with `out_dir` writes it) |
| `render.sh <dump> --summary [--against f]` | the count line, and how many ideas changed after the file's `data-through` |
| `render.sh --check f` | exit 0 only for a renderer's output, unedited (the header's `body-sha256`) |
| `render.sh --url` | the page's url, from the registry |
| `render.sh --mint` | a new idea's doc id and timestamp, for `/ideas` |

A document it cannot represent (not an object, no text, a status outside `not-touched`,
`discussed`, `roadmap`, `parked`) is refused, naming the file: a skipped document is a shorter,
plausible queue. A missing dump directory is refused too: an empty collection writes nothing, so
the session `mkdir`s it only after a list that succeeded, and "empty" is never guessed.

## Standing Tee compatibility, and the deliberate differences

`render.sh` is Standing Tee's `infra/ideas-render.mjs` in sh and jq, `skill/SKILL.md` its `ideas`
skill, the fragments its session-start step 2c and session-close step 4, `page/ideas.html` its
`docs/ideas.html` with the branding taken out, and `checks/queue.sh` with `.claude/checks/ideas.sh`
its `ideas-render.test.ts`. Measured against the reference on its vitest fixtures, adversarial
documents (Markdown and HTML in text, every JavaScript whitespace kind, non-ASCII, duplicate keys,
odd field types, prototype-named ids, a BOM) and Standing Tee's real queue (its live page's
collection, dumped on 2026-10-01, when it was empty, and its committed `docs/ideas.md`, which this
renders byte for byte below the header): the same bytes below the header, so the same
`body-sha256`; the same summary lines; the same refusals, naming the same file. Where they differ,
on purpose:

- **The header names its renderer**, `.claude/modules/ideas/bin/render.sh`. `--check` accepts any
  renderer's header, so a file Standing Tee rendered passes here; the reference's `--check` matches
  its own name literally and refuses this one's.
- **The owner sentence** reads `OWNER_NAME` (else "The `OWNER_ROLE`"), and the registry path
  `ARTIFACT_REGISTRY`, where the reference typed "Rich" and `docs/artifacts.json`; with those set to
  Standing Tee's values the body is byte-identical. The summary names `IDEAS_FILE`.
- **A JSON parse error** gives jq's reason in the parentheses, where Node gave its own; both say
  `not JSON` and name the file. The usage line names this tool and its two extra modes.
- **Stricter where the reference trusted the data**: a `createdAt` or `updatedAt` that is not an
  ISO time (or bare date) is refused, naming the file, where the reference printed its first ten
  characters raw into the Markdown; and `--check` also holds the header's `data-through` to the
  body (a `none` stamp means every listed idea renders undated, an ISO time otherwise), which the
  hash does not cover.
- **`enable.sh` never overwrites a project's own `/ideas` skill**: the module's copy carries an
  "installed by the ideas module" line, and a skill without it is refused, not replaced.
- **Two modes the reference did not need**: `--url` (the skill read the registry with `node -p`) and
  `--mint` (it minted ids with `node -e`), so the skill and the steps need no Node.
- **The page** has neutral colour tokens and system fonts (Standing Tee's design tokens and Google
  fonts are its own), says "the page's owner" in its script's messages (a name in a script string is
  a quoting hazard), and names the project and owner from `project.conf` when `enable.sh` makes it.
- **The check runs in the gate** (`ideas:queue` in `checks/run.sh`), where the reference's vitest
  case ran in CI; the rule-3 wiring (the sessions and `/ideas` call the renderer, and read the url
  from the registry) is held by `.claude/checks/ideas.sh` against the shipped fragments.
