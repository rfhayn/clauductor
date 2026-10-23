**Status:** awaiting approval
**Roadmap row:** OPS-118
**Risk:** high
**Budget:** $120

## Why
After every merge the owner does by hand what no lane can: pulls the main checkout, runs
`./install.sh` and `clauductor panel install` when the merge changed `framework/`, and
`clauductor panel trust` when it changed `.clauductor/panel.json` (the session plan's *After each
merge the owner* bullet). A lane cannot do it: worktree isolation refuses `git -C` on the main
checkout, the sandbox protects its `.claude/` files, and trust is bound to the bytes the owner read
(`install/trust.go`). Each time the step waits, something is quietly not in force:
- **Session 5 (2026-10-18), #164:** a merged hook was not in force until the owner fast-forwarded
  the main checkout by hand, four times in one session, and `worktree-hook-drift` blocked worktree
  agents meanwhile.
- **Session 8 (2026-10-22):** #205 turned on `lanes_auto_close` and changed what Up next and the ops
  template run; none of it ran until the owner re-trusted the config the same day.
- **Today:** the installed panel (`go version -m ~/.clauductor/panel/bin/clauductor`) was built at
  `f9a5f396` (#201, session 6's close). #205 changed `framework/internal/panel/install/init.go`
  since, and the installed panel does not have it. Nothing says so.

The owner's ruling of 2026-10-21 slots this as OPS-118, and session 9 is its proposal.

## What changes
- **A machine turns catch-up on per project:** `clauductor panel catch-up --project <path> on`,
  kept in a file of its own, `~/.clauductor/panel/catch-up.json`. It is off until then, it is
  never read from the repository, and an attached project cannot turn it on.
- **The panel fast-forwards the main checkout** when a pull request leaves its open list, and at
  its start, while the checkout is clean, on the base's branch and has no commit of its own, and
  only over commits that are each a pull request merged from the owner's own GitHub account (the
  login the panel runs as, through which every lane's merge-pr also merges). Otherwise it leaves
  the checkout alone and says why, once.
- **A changed `panel.json` that arrived that way is trusted on arrival:** the loaded config was
  trusted and is the file the fast-forward started from, and every commit it moved over is a pull
  request merged into `main` from that account. The panel serves the new config at once and sends one notice naming the pull
  request and each command line added and removed. Any other arrival is served untrusted, with
  its commands off, as a pull followed by a restart is today, and the notice says it waits for the
  owner. **This is what replaces the owner's review at re-trust** (design D1).
- **When the merge changed `framework/`,** the panel builds itself from the main checkout into its
  own folder and checks the new binary runs. It keeps the old one as `clauductor.prev`, replaces
  the installed binary and the `clauductor` on its PATH, says so, and restarts under launchd. The
  token and the login agent stay as they are.
- **`merge-pr` stops trying to pull the main checkout** from a lane, and leaves it to the panel
  while catch-up is on; from the main checkout with catch-up off it still pulls. Its report names
  the owner's step only when catch-up is off.

## What the existing specs already guarantee
- **`panel-attach`:** ATTACH-2 (*Attaching and serving write nothing in the repository*) holds
  for an attached project, so catch-up refuses one (D2) and the requirement is untouched. ATTACH-3
  holds a config outside its repository to the sidecar rule; ATTACH-3-S4 keeps today's trust rules
  for an in-repository config, and trust on arrival adds a second way to trust one, the owner's
  merged pull request, without changing the first.
- **`panel-stall`:** STALL-3 sends a `stalled` alert as a macOS notification and through
  `push.json`, and says the push carries no other alert. **Modified:** a catch-up notice may also
  be pushed (D6), under the same file, argv and timeout rules. `alerts.notify` withholds only the
  notice that a config waits for the owner (the page's untrusted banner says the same); every
  other catch-up notice goes whatever it says. Only the trust-on-arrival notice is titled with the
  project's name; the rest are titled "clauductor panel" (D6, *Titles*). STALL-1's restart
  rules already cover a panel that restarts itself: a stall of a lane not stalled before is
  approximate, and not notified, for one stall time after the start.
- **New capability `panel-catch-up`:** the switch, the fast-forward, trust on arrival, and the
  rebuild (CATCHUP-1 to CATCHUP-4).

## Out of scope
- **#164's second item** (the sandbox's `allowLocalBinding`, and whether lane worktree checkouts
  may write `.claude/`): it stays in #164, which stays open. The build PR references #164 and does
  not close it.
- **An attached repository's main checkout**: ATTACH-2 forbids writing it, so its owner pulls it
  as today. This is a refusal, not a deferral.
- **A panel started by hand**, and a panel not built from this repository (a release build): no
  rebuild. The notice says how to update it (D4).
- **Rolling back a new binary on its own:** the notice before the restart names the rollback, and
  the owner runs it (D4).
- **OPS-108's item (6)** (the owner's step at the end of merge-pr's report): D5 writes it here, in
  the form catch-up leaves it. OPS-108 is built after this (S10) and keeps that wording.

## How we'll know
- **Signal:** in the 7 days after it merges, no merge needs the owner's pull, trust or install.
  The panel log has a catch-up line for each merge, and `go version -m
  ~/.clauductor/panel/bin/clauductor` names the last commit on `main` that changed `framework/`.
- **Check after:** 7 days
