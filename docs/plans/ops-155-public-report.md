# Going public: the report (OPS-155)

**Status:** for the owner, 2026-10-29 (S16). Nothing on GitHub was changed to write it: the
visibility, the settings, the workflows and the history are as they were. Flipping the repository
public is the owner's decision, outward and not undoable (AGENTS.md, Who decides).

**Verdict.** No secret in any commit GitHub holds. Nothing in the records *must* go before the
flip; a handful of items were worth a decision, listed below with a recommendation each. The
owner ruled on 2026-10-29: every recommendation accepted, the email kept, no history rewrite.
This PR applies the rulings (the *Ruling* column); what is left is the owner's settings, section 6.

## 1. Secrets, over the whole history

**What was scanned.** gitleaks 8.30.1, `gitleaks git --log-opts=--all`, three ways:

| Scan | Commits | Rules | Findings |
|---|---|---|---|
| This clone, every local ref | 367 | default + `.gitleaks.toml` | 0 |
| This clone, every local ref | 367 | default only, no allowlist | 4 |
| A `git clone --mirror` of GitHub (every branch, tag and all 233 `refs/pull/*`) | 1220 | default only, no allowlist | 8 |

The mirror matters: GitHub keeps every PR's head under `refs/pull/N/head`, so the pre-squash
commits of every PR (1249 non-merge commits, against 371 reachable here) go public with the
repository, even though no branch names them.

**Every finding is a known non-secret**, the two the repository's allowlist already names:

| Commit | Path:line | Rule | Judgement |
|---|---|---|---|
| `b08294c4`, `edcb3eab` | `framework/internal/panel/terminal_test.go:31` (`:29` in `edcb3eab`) | generic-api-key | Not a secret: RFC 6455's sample `Sec-WebSocket-Key` |
| `b08294c4`, `edcb3eab` | `framework/internal/panel/web/vendor/xterm/xterm.js:1` (3 matches each) | generic-api-key | Not a secret: identifiers in vendored, minified xterm.js |

A filename pass over every path ever committed on GitHub found no `.env`, key, certificate or
credential file; the `token` paths are reviewer-eval fixtures.

**Not a secret, but private data in history.** The first hook fixture,
`framework/internal/panel/testdata/spike-hooks.log` in `8e7fee2f` (PANEL-1, on `main`; also
`6bb37292`, `76b7ac45`), is a raw capture: the Mac's username and home paths, a Standing Tee
working directory and session ids, and the test prompts. It was scrubbed in `d2d56686`. Low:
keep, or it goes only with a rewrite (below).

## 2. The records

Recommendations: **keep** (fine to publish), **redact** (edit the tree), **move** (out of the
repository). Every item stays in history whatever the tree does: see *What a rewrite would and
would not do*.

| # | Where | What | Recommendation | Ruling (the owner, 2026-10-29) |
|---|---|---|---|---|
| 1 | Commit metadata, 1123 of 1373 commits on GitHub's refs (56 on `main`, 2026-03-18 to now) | The owner's personal Gmail address, as author or committer | Owner's call. The tree holds it nowhere. Removing it needs a new repository or GitHub Support (below) | **Accepted by the owner, 2026-10-29; future commits use the GitHub noreply address (the owner's step: `git config user.email`).** No rewrite |
| 2 | `docs/development-journal.md` (140 lines with `$`, e.g. :19, :34, :117), `docs/journal.d/` cost fields, the "actual cost" line of 10 archived changes' `tasks.md`, `docs/insights-log.md` (10), and 136 PR bodies | Spend: per-row and per-session costs, the 30-day flow line's dollars per week | **Keep.** It is what building with this model costs. Redacting it is large and history keeps it. The template writes these fields for every project by design | Keep |
| 3 | `docs/owner-queue.md:15`, `:17`, `:32` | The API credit running out, a promotional credit's expiry, OPS-93's budget overrun | **Keep** (ticked, operational) | Keep |
| 4 | Every tracked file (the places the report first named, and the rest a whole-tree search found, any case): `docs/plans/standing-tee-swap.md` (the people table, ST-3.6, ST-6.1, decisions 2, 9 and 10, the convergence decisions 5 and 9, #49's row, readiness 6, two risks, Appendix A), `docs/prds/active/PRD-attach.md:74`, `docs/roadmap-history.md:278`, `:292`, `:301`; two check fixtures (`ideas.sh`, `living-visuals.sh`) in `template/`, `.claude/` and `plugin/`; 3 PR bodies | A named Standing Tee collaborator, their platform (WSL2) and their onboarding | **Redact** to a role. Low: no contact details | **Redacted in this PR** to "the designer" (a Standing Tee contributor), keeping each sentence's sense; the fixtures use neutral names. A whole-tree search now finds the name in no tracked file. **History keeps the old text** (every earlier commit, and the 3 PR bodies): no rewrite was chosen |
| 5 | `docs/plans/standing-tee-swap.md` (whole file), `docs/plans/direction-shift.md:35` | The private `rfhayn/standingtee` repository's internals: its paths, commit and session numbers, its gate | **Keep** or **move**: a finished plan for another, private project | Keep |
| 6 | `docs/roadmap-history.md:264`, `:269`, `:271`; `docs/adr/0011-open-source-terminal-first-plugin-installs.md:11`; `docs/plans/direction-shift.md:89`, `:96`; `docs/roadmap.md:215` | The abandoned business plan: licences sold through Polar, an Ed25519 signer, pricing and checkout | **Keep.** It is cancelled and it is why ADR-0011 reads as it does | Keep |
| 7 | `docs/owner-queue.md:23` | The LLC and trademark filing (moot since open source) | **Keep** (ticked, moot) | Keep |
| 8 | `docs/owner-queue.md:26`, `:27`, `:28`, `:31`; `docs/development-journal.md:39`, `:271`, `:2369`; `.claude/local/context.d/session-start/team-queue.sh`; `.claude/local/skills/*/team-queue.md` | The retired bot team (a COO and a CLO role) and its queue | **Keep** the records. See section 4 for the live part | Keep; the live part is fixed in this PR (section 4) |
| 9 | `.claude/modules/artifacts/README.md:24` (and its copies in `template/` and `plugin/`) | A claude.ai artifact address with a real-looking id, as the example of a registry entry | **Redact** to a placeholder: it ships with the template | **Done in this PR:** `EXAMPLEEXAMPLEEXAMPLE0` in all three copies (the plugin rebuilt), and `local:roadmap-tracker`'s exemption for the module's sample id follows it. History keeps the old id |
| 10 | 156 of 233 PR bodies and 4 of 88 issue bodies (counted 2026-10-29; no PR body holds an artifact link), 24 commit messages (5 on `main`); 188 PR bodies also end in a "Generated with Claude Code" line | claude.ai session links (the attribution lines) | Edit the PR and issue bodies before the flip (revised 2026-10-29): they become public with the repository, and this repo turns attribution off | **Keep, no scrub** (the owner, 2026-10-29): a link opens only for the owner's signed-in account; GitHub keeps a body's edit history anyway; and the 24 in commit messages cannot be edited. From now on a PR body carries neither line (#323's carries none) |
| 11 | `docs/plans/direction-shift.md:13` | Andrew Baker's Ghostty Dock, credited with its public source | **Keep** | Keep |

Searched and **not found** in the tree: an email address other than placeholders and a font
licence's, a committed tracker address (`local:roadmap-tracker` guards it), a routine's id or
address (`.claude/local/cloud-lanes.tsv` holds names only), and the new name.

**What a rewrite would and would not do.** A `git filter-repo` of `main` and a force-push would
not remove anything from the 233 `refs/pull/*`: GitHub does not let anyone rewrite those, and they
hold every pre-squash commit. So an item truly goes only by (a) asking GitHub Support to purge the
PR refs and cached views after a rewrite, or (b) publishing a **new** repository from a cleaned
snapshot or rewritten `main`, which loses the PR and issue history and changes every SHA the
records cite. For item 1, (b) is the realistic route; for everything else here, the
recommendation is that no rewrite is needed. Either is the owner's call; nothing was rewritten.

## 3. The licence

`LICENSE` is already at the root: MIT, "Copyright (c) 2026 Rich Hayn", added in `86db6441`
(2026-03-18) by Rich Hayn. It matches ADR-0011 (Open source under MIT; line 71 expected REL-1 to
add it, and it is already there) and the commit authorship (Rich Hayn on every human commit).
Nothing to add.

## 4. The workflows, and what acts on public input

| Workflow | Triggers | Permissions, secrets | On GitHub now | Public-repo note |
|---|---|---|---|---|
| `test.yml` | push to `main`, `pull_request` to `main` | `contents: read`; no secrets; actions pinned to SHAs | disabled manually | Runs fork PRs with a read-only token. OPS-98 turns it on |
| `release-build.yml` | push to `main`, `pull_request` to `main` | `contents: read`; no secrets | disabled manually | Build only; never publishes. OPS-98 |
| `checks-double-pass.yml` | weekly cron, dispatch | `contents: read`; no secrets | active | GitHub disables a public repository's schedules after 60 days without activity |
| `dependency-audit.yml` | weekly cron, dispatch | `contents: read`; no secrets | active | Same |
| `release.yml` (only on the stale `feature/REL-1-release`, 2026-09-30) | `v*` tag push, `pull_request` (paths), dispatch | job-level `contents: write` in the `release` job, gated on `ref_type == 'tag'`; no secrets | active (GitHub lists it) | Only a writer can push a tag, so a fork cannot reach the write job. Delete the branch once REL-1 is rebuilt, or keep it knowingly |
| `release-homebrew.yml` (only on `feature/REL-1-release`) | `release: published`, dispatch | `contents: read`; reads `secrets.HOMEBREW_TAP_TOKEN`, runs only when the variable `HOMEBREW_TAP_REPO` is set | not listed | Only a writer publishes a release. The repository has no secret, variable or environment set today, so it cannot run |
| Dependabot | `.github/dependabot.yml` | GitHub's own | active | No change |

**No `pull_request_target`, no `workflow_run`, no self-hosted runner** in any workflow on any
GitHub ref, and the one `secrets.` reference is the Homebrew job above, on a release only a
writer can publish. A fork PR therefore runs with a read-only token and no secrets. It can still
change the workflow file in its own PR and run what it likes on GitHub's runner, so:

- **Require approval for every outside contributor's workflow run** (Settings, Actions, General,
  *Fork pull request workflows*, the strictest option) before the flip. The weaker options hold
  only first-time contributors.
- **The `team-queue` issue templates** (`.github/ISSUE_TEMPLATE/team-queue.md`,
  `team-queue-new-row.md`) applied the `team-queue` label (and `new-row`) for anyone who filed
  through them, so on a public repository a stranger could put an issue's title and body into
  session-start's context (the team-queue listing reads open `team-queue` issues; it never acts
  on one without the `approved` label, which needs triage rights). **Fixed in this PR:** neither
  template applies a label now, so only someone with triage rights can add `team-queue`. The
  listing selects by the label alone (its GraphQL query, `labels: ["team-queue"]`), and neither it
  nor `local:team-queue` reads the templates' front matter, so neither needed a change. **Skipping
  issues from authors without write access** is not a small change: the query, its completeness
  checks and the check's fixtures all change, with a fixture shown failing first. It is queued as
  **OPS-166**. Until then, a triager labelling an outsider's issue is the one path left.
- **Open PR titles from forks** reach the same context through `lib/context.sh`. The approval setting above limits what a fork's PR runs, not its title.
- **The free minutes.** Standard GitHub-hosted runners (Ubuntu and macOS) are free for a public
  repository, which is what lets OPS-98 come before 2026-11-01.

## 5. The name

Publishing now publishes **clauductor**, the old name. The new name has cleared (the owner,
2026-10-28) but the sweep, OPS-104 (R2, rename step 5, session S17), has not run, and nothing in
the repository writes the new name, so the flip does not reveal it early. What publishing first
means:

- **The old name becomes public and permanent.** It is in every file, the plugin id, the
  marketplace (`/plugin marketplace add rfhayn/clauductor`, `README.md:39`), 23 files' links to
  `github.com/rfhayn/clauductor`, the history and every PR. OPS-104 keeps class-C names and does
  not rename the plugin id, and its `name-residue.sh` allow-list keeps the journal and
  `changes/archive/` as they are, so the old name stays visible after the sweep too.
- **The repository's own rename** comes with or after OPS-104. GitHub redirects the old URL for
  clones, links and the raw schema URL (`template/.clauductor/panel.json:2`) until someone
  creates a new repository under the old name. Anyone who forks or stars in the gap sees the
  rename happen.
- **The Go module path** is already `github.com/clauductor/clauductor` (not `rfhayn/`), and OPS-104
  changes it with `go mod edit -module`; nothing imports it from outside yet.
- **No release.** v0.1.0 (REL-1) ships only under the new name, with no fallback (ADR-0011,
  the owner, 2026-10-27), so going public now is a source-visible preview, not a launch.

If the owner wants the old name never to be public at all, the order is OPS-104 first, then the
flip; that costs the free minutes until S17 (or 2026-11-01, whichever is first).

## 6. Settings for the owner

Done in this PR: items 4 and 9 redacted, the issue templates label nothing. The rest is the
owner's, at github.com and on the Mac:

**Before the flip**

1. `git config user.email` to the GitHub noreply address, on each machine that commits (item 1).
2. Merge this PR, so the issue templates stop labelling before strangers can file.

**After the flip**

1. **Actions:** Settings, Actions, General, *Fork pull request workflows*: require approval for
   all outside contributors (the strictest option).
2. **OPS-98:** check Billing (Settings, Billing: Actions usage and any spending limit), then turn
   `test.yml` and `release-build.yml` back on (Actions, each workflow, Enable workflow). The owner
   queue holds this item; OPS-98's row comes right after.
