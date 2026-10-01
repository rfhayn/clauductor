# Optional module: living-visuals (pages whose stated facts are checked)

A living page is a shared page that states facts about the system as it is now: a roadmap page
with the open queue on it, an ERD naming the latest migration, an API map counting the audited
tables. The artifacts module says when such a page's sources MOVED. It cannot say whether what the
page STATES is still true, and the skill that refreshes a page fires on a structural event, never on
a stated fact going false because the thing it counts grew somewhere else. This module makes a
page's facts checkable, checks them in every gate, and puts the refresh in the session close.

**Off by default.** It requires the `artifacts` module (living pages are entries in its registry).
sh and jq, plus Node only to parse each page's inline scripts (`LIVING_SCRIPT_CHECK="node"`, the
default; `"off"` says on every page that its scripts were not parsed).

## Declaring a living page

An entry in the artifacts registry (`ARTIFACT_REGISTRY`) is a living page when it declares
`refresh`:

```json
"docs/roadmap.html": {
  "url": "…", "authorities": ["docs/roadmap.md"], "reviewedAt": "…", "reviewNote": "…",
  "refresh": "update-roadmap",
  "generated": {
    "queue": { "run": "sh scripts/queue-html.sh",
               "outside": "sh .claude/roadmap-queue.sh --queued" }
  },
  "claims": {
    "migration-range": "sh scripts/claims/migration-range.sh",
    "owner-phase-*": { "run": "sh scripts/claims/owner.sh phase \"$1\"",
                       "each": "sh scripts/claims/phases.sh" }
  }
}
```

| Field | Means | The check |
|---|---|---|
| `refresh` | the skill that refreshes the page (`.claude/skills/<name>`), or `"edit"` | the skill exists: a refresh nobody can run is an instruction that gets interpreted |
| `generated.<name>` | a command, or `{run, outside}` | the page holds exactly one `<!-- generated:<name>:begin … -->` (a comment, then a newline) … `<!-- generated:<name>:end -->`, and its content equals `run`'s output (trimmed). Each line `outside` prints must not appear in the page outside the block (the hand-mirrored copy that drifts); `outside` printing nothing fails, since it would search for nothing. A block on the page that the entry does not declare fails too |
| `claims.<name>` | a command, or `{run, each}` | every `data-claim="<name>">VALUE<` on the page equals `run`'s output: as text, or as the same number (`six`, `Six`, `6`). A claim on the page with no command fails, and so does a declared claim the page no longer carries |
| `claims.<prefix>*<suffix>` | a family | each member's command gets the part `*` matched as `$1` (and the full name as `$CLAIM`). With `each` (a command listing the members the authority defines), the page must claim exactly those: a new phase with no owner shown fails, and so does an owner left on a removed one |

And for every living page: it is read twice and must be identical (a page caught mid-rewrite is
not reported as drift); an HTML page must end with `</html>`; every inline `<script>` must parse,
and the count extracted must equal a plain count of the opening tags; every
`data-days-since="YYYY-MM-DD"` must be a real day, not in the future, filled by a script of the
page's own; and no elapsed duration is typed (`LIVING_TYPED_DURATION`, by default
`\b[0-9]+ (days?|weeks?|months?|years?) (so far|and counting|elapsed|ago)\b`, matched
case-insensitively in the page's text outside scripts, styles and comments; set it empty in
`project.conf` to turn it off).

The commands run with `sh -c` from the project root, each stopped after `LIVING_COMMAND_SECONDS`
(default 60): they are the project's own, as `GATE_RUN` is.

## What it adds when on

| Point | Part | Does |
|---|---|---|
| `checks/` | `pages.sh` | `living-visuals:pages`, in every gate: every rule above, on every living page |
| `context.d/session-close` | `living-visuals.sh` | each living page's currency with how to refresh it, and every page rule failing on the close's tree |
| `skills/` | `session-close/living-visuals.md` | regenerate the blocks, refresh each BEHIND page with its skill, run the check; before the artifacts step stamps |
| | `merge-pr/living-visuals.md` | which living pages the merge moved, and how each is refreshed |
| `conflicts.tsv` | every living page | take `origin/main`'s side, regenerate and refresh on the merged tree; a generated block is never hand-merged |

`.claude/checks/living-visuals.sh` tests the parts themselves whether the module is on or not.

**The tool** (`bin/living.sh`; from the plugin, `clauductor-model modules/living-visuals/bin/living.sh`):

| Call | Does |
|---|---|
| `living.sh --check` | every rule, every living page: `ok`/`FAIL` lines, exit 1 on a FAIL |
| `living.sh --list [--ref <rev> \| --worktree]` | each living page's currency (the artifacts module's `currency.sh`) and how to refresh it |
| `living.sh --regen [<page>...]` | rewrite every generated block in place from its command |

## Standing Tee compatibility, and the deliberate differences

This is the generic half of Standing Tee's `living-visuals.test.ts`, its `update-roadmap` skill's
generated-block contract, its session-close step 3 and its merge-pr post-merge reminders. Measured
against the reference on Standing Tee's real pages (a copy of its repository, its living pages
declared in its registry with its own claim computations as the commands): the same verdict on the
unchanged pages (both green) and on each mutation of the rules ported here (a hand edit inside the
generated block, a queue row moved without regenerating, an open change and the block's own
markup mirrored outside it, a script that no longer parses, a truncated page, each claim family
set wrong, a claim deleted, a claim renamed). Where they differ, on purpose:

- **The domain checks stay Standing Tee's.** The ERD's rendered notes, the API map's routes and
  wiring, the stylesheet pins and the capability strip are about its pages, not a mechanism; they
  stay in its own test. So does one rule tied to its roadmap's shape: an open gate's day counter
  must count from that gate's started date.
- **The authorities are commands in the registry**, where the reference computed each claim in the
  test. A claim is held only when it has a command, and an annotated claim with none FAILS (the
  reference ignored a `data-claim` no test read).
- **Claims compare as text, or as the same number**, for every claim. The reference compared counts
  numerically (`six`, `Six` and `6` alike, words up to eight) and names exactly; here words go to
  twelve.
- **Family coverage is declared** (`each`): the reference wrote each family's both-directions test
  by hand.
- **The outside-the-block leak check** takes its strings from a command and fails when it prints
  none; the reference required more than five open changes.
- **Script extraction is case-insensitive**, and a script the extractor misses is a failure (the
  reference matched `<script>` exactly and parsed what it found). Every living page's scripts are
  parsed, where the reference parsed four pages'.
- **New: typed durations and day-counter dates.** The reference's `update-roadmap` skill said
  "never type an elapsed count"; here `LIVING_TYPED_DURATION` executes it, and a `data-days-since`
  that is not a past real day, or that no script of the page fills, fails.
- **`--regen` writes the block**, where the reference's skill pasted the generator's output by hand.
- **`refresh` is new**: the reference's registry had no field naming a page's refresh, and its
  session-close kept a table of them in prose. A named skill is checked to exist.
