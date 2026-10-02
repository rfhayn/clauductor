# Optional module: artifacts (shared pages held current with their sources)

A project shares pages: a roadmap page, an ERD, a playbook, a welcome page, a go-live walkthrough.
Each describes sources in the repository, and each goes stale a different way, because every
refresh trigger written into a skill fires on something being *built*, never on a source moving
under the page. This module asks the question mechanically, per artifact, and refuses a session's
close until every answer is current.

**Off by default.** Nothing here runs until `MODULES` in `.claude/project.conf` names `artifacts`.
The currency half needs only sh, git and jq. The optional publishing half
(`ARTIFACT_PUBLISH="claude.ai"`) keeps claude.ai copies of `docs/*.html` pages in step with `main`;
it needs Node for the copy and the Artifact tool (a Claude Code session signed in to claude.ai) to
publish, and a session without that tool says `CANNOT CHECK — no Artifact tool`.

## The registry

`ARTIFACT_REGISTRY` (default `docs/artifacts.json`): sections of entries, keys starting with `$` are
comments. Every entry with a `url` is an artifact:

```json
{
  "pages": {
    "docs/erd.html": {
      "url": "https://claude.ai/artifact/XvoZvCgkJJVbiXrWabnVS7",
      "published": "<40-hex: the blob hash of the copy last published (claude.ai only)>",
      "authorities": ["db/migrations/*.sql"],
      "reviewedAt": "<40-hex: the authorities' content hash at the last review>",
      "reviewNote": "2026-10-01: reviewed, no change: the migration adds no column"
    }
  },
  "walkthroughs": {
    "go-live": { "url": "https://claude.ai/artifact/…", "row": "2D.0a", "steps": 50,
                 "authorities": ["row:2D.0a", "docs/runbooks/*.md"], "reviewedAt": "…", "reviewNote": "…" }
  }
}
```

**Authorities** are what the artifact describes: path globs (`*` within a directory, `**` across),
`row:<id>` (a roadmap row: its change, summary, scope, deps and status), `gate:<id>` (every row under
a `## Gate <id>` heading, two to four `#`), and `registry:entries` (which artifacts exist and where,
for a page that lists them). A glob matching no file, a row that is not there, or a missing stamp is
`CANNOT CHECK`, never `OK`.

**Start it** by writing the entries, then stamping each once:
`sh .claude/modules/artifacts/bin/currency.sh --stamp <key>... --note "seed: <why>"`.

## What it adds when on

| Point | Part | Does |
|---|---|---|
| `health/` | `currency.sh` | one `STALE — <key> is BEHIND` line per artifact whose authorities moved at `origin/main` (or one `OK`) |
| | `shared-copies.sh` | claude.ai only: one `STALE` line per page copy that differs from `origin/main` |
| `context.d/session-start` | `artifacts.sh` | every artifact's link; walkthroughs with their row and steps; claude.ai only, each copy's state |
| `context.d/session-close` | `artifacts.sh` | `currency.sh --worktree` on the close's own tree; claude.ai only, each copy's state |
| `guard.d` | `currency.sh` | **blocks** a close PR (`ops/session-<N>-close*`) while any artifact is not `OK` at its head, within `ARTIFACT_RULE_SECONDS`; fails closed |
| `skills/` | `session-start`, `session-close`, `merge-pr` | report BEHIND and walkthrough progress; refresh or stamp before the close; claude.ai only: republish, record, publish what a merge recorded, open |
| `conflicts.tsv` | `docs/artifacts.json` | merged entry by entry, never one side whole |
| `checks/` | `registry.sh` | `artifacts:registry`: the project's registry lints clean, urls well formed and unique; claude.ai only, every `docs/*.html` registered |

**The tools** (`bin/`):

| Tool | Needs | Does |
|---|---|---|
| `currency.sh [--ref <rev> \| --worktree] [--check]` | sh, git, jq | the status lines; `--check` exits 1 when any is not OK |
| `currency.sh --stamp <key>... --note "<line>" [--at <rev>]` | sh, git, jq | record the review: `"refreshed: <what>"` or `"reviewed, no change: <why>"` |
| `currency.sh --lint` | sh, jq | declarations only, on disk |
| `publish.sh --recorded-in <commit>` | sh, git, jq | the pages a merge recorded, for merge-pr |
| `publish.sh --status \| --record <page> \| <page> [--ref]` | Node | the claude.ai copies (`prep-artifact.mjs`) |
| `open.sh [--print]` | sh, jq | open every registered artifact |

From the plugin, each is `clauductor-model modules/artifacts/bin/<tool>`.

## Why a content hash, and why at the close

A stamp is written on a branch, and branches land as squashes: the branch commit is never on `main`,
and a commit cannot contain its own hash. The content a review saw survives the squash, so that is
what `reviewedAt` records; to name *which* commit moved a source, the tool walks the authorities'
history back to the state that matches the stamp (100 commits; 4 s under `--check`, 12 s otherwise).

The guard rule holds only the close: a change PR moves an authority and the page follows at the
close, the one PR per session whose job is to leave the pages true. Blocking every PR would force a
page review into the middle of a build. The price: a session that never closes leaves a page behind,
and the next session-start says so. That a close's head contains `origin/main` (so the head's tree is
what the squash lands) is the core merge guard's rule, not this module's.

## Standing Tee compatibility, and the deliberate differences

`currency.sh` is Standing Tee's `infra/artifact-currency.mjs` in sh and jq, and `prep-artifact.mjs`
is its `infra/prep-artifact.mjs`: the same hashes, so a registry stamped and recorded by either
reads the same in the other. Measured on Standing Tee's own history (every `main` commit since the
check landed, and 25 working-tree, stamp and lint states): the same BEHIND and CANNOT CHECK lists,
the same exit codes and the same registry bytes after every stamp. Where they differ, on purpose:

- **Row ids** follow the template's roadmap grammar, `[A-Za-z0-9._-]+`, not Standing Tee's enumerated
  gate letters. Gate headings take any `## Gate <id>` id. On a roadmap Standing Tee's parser
  accepts, both read the same rows.
- **The roadmap's owner, started and boundary lines are not validated here.** Standing Tee's parser
  throws on a malformed one (which makes every row authority `CANNOT CHECK`); the template's
  `checks/roadmap.sh` owns the roadmap's grammar.
- **Path globs are matched against real paths** (`git ls-tree -z`). Standing Tee's `--ref` mode
  matched git's quoted form of a path with non-ASCII characters, so such a path hashed differently
  between `--ref` and `--worktree` there.
- **The history walk is slower in sh**, so on a run with many BEHIND artifacts it can stop at its
  budget where Node named the commit. The verdict never depends on the walk, only the commit named.
- **Text**: the stamp command named in a BEHIND line is this module's; the copy is written to a temp
  directory, not `.artifact-publish/`; "the founder's session" reads "the owner's session".
- **Stricter where Standing Tee read nothing as green**: a registry with no artifact is
  `CANNOT CHECK` (Node printed "All 0 … current"), and the guard rule blocks a close whose head
  deleted the registry `origin/main` still has. `currency.sh` refuses a `--root` inside a
  repository rather than at its top.
- **Both hash raw bytes** (`hash-object --no-filters`, as Node did): in a repository whose
  `.gitattributes` filter files (LFS, eol conversion), a `--worktree` stamp of such a file never
  matches the committed blob, and the close stays blocked. Stamp with `--at HEAD` there.
- **Characters past U+FFFF** in a review note or a commit subject are counted as one character where
  Node counted two, when a line is shortened.
