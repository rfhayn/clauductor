# Design: panel-49-lane-measures

The figures and the transcript shape below come from PANEL-48's `evidence/` (`README.md` and
`tool/usage-report.mjs`). That tool read Standing Tee's week of 2026-10-08..14 and PANEL-48's probe
transcripts on Claude Code 2.1.302. PANEL-48's D7 and D13 are cited as designed and approved on
2026-10-17. This change is built after PANEL-48 merges, and builds on its code.

## Decisions

- **D1 One reader, `signals/transcript.go`, finds a session's transcript by its id under Claude
  Code's config directory.**
  - **The directory:** `claudecall.ConfigDir`, which is `$CLAUDE_CONFIG_DIR`, else `~/.claude`.
    The panel already uses it for the login locks, and its lanes inherit the same environment.
  - **The files:**
    - the session's own transcript, `projects/*/<id>.jsonl`, found by listing `projects/` once
      and kept for the session once found;
    - its subagents' transcripts, every `*.jsonl` under `projects/<that folder>/<id>/subagents/`,
      workflow agents' deeper folders included. The evidence tool's first version missed those and
      reported half the cost.
  - **The id:** it must be a UUID, the shape `--session-id` takes, before any path is built. A
    session whose id is not one is never read.
  - **No path from a payload:** `HookEvent` still declares no `transcript_path`, and
    `StatusPayload` no transcript field.
  - **Resume:** `--resume` appends to the same file. PANEL-48's P1 session, resumed twice by P5,
    is still one file under one session id.
  - **Beat:**
    - the hook payload's `transcript_path`. It is exact, but it lets any process that can post a
      hook name the file the panel opens;
    - building the folder name from the lane's path. The rule is Claude Code's and undocumented,
      and the folder a long path gets is not its plain spelling.

- **D2 The reader decodes an allowlist of fields, and nothing else.**
  - **What it decodes:** each line's `type`, `timestamp` and `version`. For an `assistant` line,
    also `message.id` and `message.model`, and `message.usage`'s `input_tokens`, `output_tokens`,
    `cache_creation_input_tokens`, `cache_creation.ephemeral_1h_input_tokens`,
    `cache_read_input_tokens` and `speed`.
  - **What it never decodes:** prompt text, tool input, tool results, file paths and assistant
    text are never decoded or stored.
  - **How:** a line that does not contain `"usage"` is skipped before any decoding, as the evidence
    tool does. The rest are decoded into one declared Go type that holds only those fields.
  - **Beat:** decoding each line into a generic map. That would hold content in memory, and the
    allowlist could not be tested. PANEL-50 needs content to attribute context to its sources. It
    widens the type by its own decision, and D8's test makes the widening a reviewed diff.

- **D3 Only an attached project's lanes are read, and there the transcript is the one source of
  context and cost.**
  - **Which projects:** a project whose config resolves outside its repository (PANEL-42's
    `install.ConfigOutside`, already held by the runtime as `outsideConfig`).
  - **Which sessions:** every session the state holds in that project, whether in a lane the
    panel launched or one that hooks place in its worktrees.
  - **A status post there** still moves the quota, and every figure a transcript does not carry,
    such as cache warmth, lines changed and durations. It no longer sets the session's context or
    cost, because one session must not have two sources.
  - **Projects with the config in the repository** read no transcript, and are measured exactly as
    today.
  - **Beat:**
    - a per-session rule: the status line where it posts, the transcript where it is silent.
      After a panel restart an idle session posts nothing, so its source would switch. The
      ledger records one total per session and counts only rises. A switch between two totals
      priced differently would then book the gap as spend, or hide later spend under it;
    - every project, which re-derives this repository's figures from a second source for nothing.

- **D4 Context: the last main-thread call, over the lane's recorded window, through PANEL-48's
  computation.**
  - **The figure:** the last `assistant` record of the session's own transcript. Its
    `input_tokens`, `cache_creation_input_tokens` and `cache_read_input_tokens` are the same three
    counts PANEL-48 D7 sums from the status line's `current_usage`. Their sum is divided by the
    window PANEL-48 recorded on the lane.
  - **One computation:** the transcript and the status post call the same function. So the
    session's and the metrics' context, `CtxWindow`, the tick and the alert threshold are PANEL-48's
    exactly. Subagent calls do not count toward the session's context, as they do not in Claude
    Code's.
  - **A session without a recorded window** shows its cost but no percentage. That covers a lane
    launched before its sidecar had a `launch` object, and a session the panel did not launch.
    - The page warns once per project, naming `launch.context_window`.
    - Standing Tee's sidecar predates PANEL-48, and its outcome step adds the `launch` object by
      hand. Until then, this warning is what the owner sees.
  - **Beat:**
    - a table of each model's default window. It is a second table to keep current, and a wrong
      entry would draw the bar and fire the alert at the wrong point, silently;
    - showing tokens without a percentage, for which the page has no field. That would be a second
      `panel.js` change.

- **D5 Cost: priced per model, summed over the session and its subagents. An unknown model is
  UNPRICED, never $0.**
  - **The table:** a Go table in `signals`, copied from the evidence tool's `PRICES` and
    `FAST_MULTIPLIER`, each row with its source and date.
    - A model id matches its exact version only, after a date suffix (`-20251001`) or a context
      tag (`[1m]`) is stripped. So `claude-opus-5-6` is not priced as Opus 5.5.
    - A cache write costs 1.25× input for 5 minutes and 2× for an hour. The streamed lines of one
      message share an id, and the message is counted once.
    - Haiku 5.5 joins the table only with its rate from the published pricing page, cited with the
      date as the tool's Sonnet 5.5 row is. Without it, it stays unpriced.
  - **UNPRICED:** a model not in the table, or fast mode on a model with no fast rate, is
    unpriced.
    - Its calls are left out of the figure.
    - A page warning names the lane, the model and how many calls were left out, as the evidence
      tool reports UNPRICED beside its total.
    - A session whose every call is unpriced shows no figure, not $0.
  - **Where it goes:** into the same three places as the status line's `total_cost_usd`:
    `costByID`, `countCost` and `observeSpend`. So est. $, cost today, the Metrics view's spend,
    and a change's budget alert count an attached project's lanes.
  - **Beat:**
    - no figure for a session with any unpriced call. One Haiku subagent would blank a lane's cost,
      and Haiku 5.5 was $0.13 of the $1,208 week;
    - Claude Code's own `cost-state` records. PANEL-48's probe transcripts carry lines of that
      type, but they are undocumented, their shape was not read for this proposal, and the row
      prices per model;
    - prices in `panel.json`, which adds a config key per model for a table that changes only
      when a model ships.

- **D6 The reader runs every 10 s, and reads only what was appended.**
  - **Where it runs:** one source in the runtime, added only for an attached project. It reads
    each session the state holds in the project.
  - **What it reads:**
    - It stats each file first, and reads only the bytes appended since its last read, up to the
      last newline. A partly written line waits for the next read.
    - A file that shrank is read again from the start, with its totals reset.
  - **The cursor:** the offsets and running totals are kept per session by the runtime, since
    `signals` keeps no state. They are dropped when the state forgets the session.
  - **A long transcript:** at most 32 MiB is read per session per poll. A session shows no figure
    until the reader has reached the end of its files, so a partial total is never shown.
  - **Beat:**
    - only while a page is in view, as the dashboard's `ps` and `git` reads run. The ledger and the
      budget alert would then miss what a lane spent while nobody looked;
    - on hooks. A session's hooks can be missing, and a busy one sends many.

- **D7 A shape the reader does not know reads as "could not read", never 0.**
  - **What counts:**
    - a complete line that is not JSON;
    - an `assistant` line with no `message.model`, no `message.usage`, or a count that is not a
      number;
    - a session that has finished a turn (a `Stop` hook), whose transcript holds no usage record;
    - a session with no transcript under `projects/` a minute after its first prompt.
  - **What then happens:**
    - The session's context and cost become unknown, as before any reading: neither the last value
      nor 0.
    - The project's `transcripts` source carries the error, naming the session, the line, and the
      line's Claude Code `version`.
    - The page shows it with the banner every failing source already gets, "Cannot read
      transcripts: …", so the page needs no change for it.
    - A later read that succeeds clears it.
  - **Lines of other types** are skipped unread. The 2.1.302 transcripts already carry types the
    evidence tool never needed, among them `attachment`, `api-request` and `cost-state`.
  - **Beat:**
    - skipping a bad line, which undercounts silently;
    - keeping the last good figure, which is stale but plausible.

- **D8 The invariant that replaces "the panel never reads transcripts".**
  - **The new invariant:** the panel reads transcripts in one file, `signals/transcript.go`. It
    reads them read-only, for an attached project's sessions, by a validated session id under
    Claude Code's config directory. It decodes only D2's fields.
  - **What executes it** (AGENTS.md rule 4):
    - **`TestNoSourceReadsTranscripts`, rewritten.** It runs the same string scan (`.jsonl`,
      `transcript_path`, `TranscriptPath`, `/.claude/projects`) and the same AST scan for a path
      joined from `.claude` and `projects`, over every shipped source. Exactly one file is exempt.
      - It fails if any other file reaches a transcript.
      - It fails if the exempt file is missing, or no longer reaches one. A stale exemption fails
        rather than exempting nothing.
      - The build shows it failing on a mention planted in a second file before trusting it.
    - **A new test holds the reader's decode type to D2's allowlist.** It walks every nested
      field's json tag by reflection, so the type itself is the authority: an added field fails.
      It also checks that the reader's only JSON decode target is that type, and that it opens
      files with `os.Open` alone.
    - **`fileReadSites`** lists the reader's read sites, as it lists every other.
    - **The payload check is kept:** `HookEvent` and `StatusPayload` decode no transcript field.
    - **`docs/panel.md`'s four statements** of the old rule are rewritten to the new one (lines 37,
      830, 844 and 2596 today). So is `signals/hooks.go`'s comment.
  - **Beat:**
    - deleting the test, which the row rules out: every other source keeps the old rule;
    - a check at run time of each path opened. The path is built in one function, and the test
      holds the build, where a failure stops a merge rather than a poll.

- **D9 The quota note is one string in `panel.js`.**
  - **The note:** the quota's "No reading yet" note adds that only a status-line post brings the
    quota, and that a lane measured from its transcript brings none.
  - **Why one string:** on a machine with only attached projects the quota stays unknown, and the
    note then says why. It is true on every machine.
  - **Beat:** a state flag and a visible "unknown" value shown only when every project is
    attached. That is a state field and two page changes for a note that is always true. The page
    is rebuilt in PANEL-46 anyway.

- **D10 Four groups. Risk normal. Budget $60, soft, as PANEL-48's (the owner decides).**
  - **Why normal risk:**
    - The reader opens files that hold the owner's prompts. D2 and D8 bound what it keeps.
    - Its cost reaches the spend ledger that budget alerts read.
    - It changes no lane's launch, and nothing in a project whose config is in its repository.
  - **The budget:**
    - PANEL-48 had spent $47.52 of its $40 by the end of its probe group and its amendment
      (`change-cost.sh`, 2026-10-17), with three groups still to build.
    - PANEL-42 cost $70.87 over five groups.
    - This change has four groups and no probes. Its fixtures are hand-written, since no
      transcript content is committed.
    - `change-cost.sh` misses agents spawned from the main checkout (OPS-74), so the build prices
      its agents' transcripts directly as well, as PANEL-48 D10 does.
  - **How the build runs:** through `apply-change` by hand, as PANEL-48's does, because
    `build-change.js`'s gate fails in the sandbox until OPS-72.
  - **Beat:** a hard stop at $60.

## Refusals
| Situation | What happens instead |
|---|---|
| The project's config is in its repository | No transcript is read. The status line measures its lanes, as today (D3) |
| A session id that is not a UUID | It is never read, and no path is built from it (D1) |
| A line the reader does not know (D7) | The session shows no context or cost, and the "Cannot read transcripts" banner names the session, the line and the version |
| A model the price table does not know | Its calls are left out, a warning names them, and a session with no priced call shows no cost (D5) |
| A lane launched without a context window | Cost shows, the percentage does not, and a warning names `launch.context_window` (D4) |
| A machine where no status line posts | The quota stays unknown, and its note says why (D9) |
| A status post from an attached project's session | It moves the quota and the figures transcripts do not carry, but not context or cost (D3) |
| `$CLAUDE_CONFIG_DIR` set for the lanes but not for the panel | The transcript is not found, and after a minute the banner says so, naming the folder searched (D7) |
| The build's cost reaches $60 | The owner is told at once, and the build goes on (soft budget) |
