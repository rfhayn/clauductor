# Tasks: panel-49-lane-measures

## Progress
- 2026-10-17 proposed; nothing built yet. The build starts from `main` once PANEL-48 has merged.

## Decision log

## 1. The reader (D1, D2, D5, D7)
- [ ] 1.0 Before the counts are made required (D2): with the evidence tool, count how many of the baseline week's calls lack each of the four counts, reading no content, and record the result in the Decision log. A count some calls lack becomes optional, defaulting to 0. (manual: a one-off measurement over the owner's transcripts, not shipped code)
- [ ] 1.1 `framework/internal/panel/signals/transcript.go`: find a session's transcript (D1), and walk its `subagents/` folder on every read. Decode every line, with no pre-filter, in D2's two stages, each into a declared type that holds D2's allowlist.
  - Stage one decodes `type` and `version`, and tolerates a type mismatch on any line that is not `assistant`. Stage two decodes `message` for an `assistant` line, where a mismatch is a D7 error.
  - A `<synthetic>` record is set aside before its usage is validated.
  - The id must be a UUID before any path is built.
  - Read from a cursor the caller keeps: per file, the offset and running totals, plus the last `assistant` message id and what was booked for it, a cost or an unpriced call.
  - Read only appended bytes up to the last newline, at most 32 MiB per call. A file that shrank is read from the start. Report whether the reader reached the end.
  - A message's streamed lines are booked once, at the last line read, across reads too.
  - `<synthetic>` records count toward neither cost nor context. The context and the model come from the last non-synthetic main-thread record; subagent records count toward cost only.
  - Report D7's errors, with the line's number, its `version` and the missing field. A required count that is absent is an error, and an optional one that is absent takes its default.
  - Tested in `framework/internal/panel/signals/transcript_test.go` over hand-written fixtures in `signals/testdata/transcripts/`, which hold no real transcript content. Citing [MEASURES-1-S5], [MEASURES-1-S7], [MEASURES-2-S1], [MEASURES-2-S4], [MEASURES-2-S5] and [MEASURES-4-S3].
- [ ] 1.2 The price table in `signals` (D5): the evidence tool's `PRICES` and `FAST_MULTIPLIER`, each row with its source and date.
  - Exact version match, after a date suffix or a `[1m]` tag is stripped. Cache writes at 1.25× input for 5 minutes and 2× for an hour.
  - Unpriced calls are counted per model, never priced at $0.
  - Haiku 5.5 joins only with its rate from the published pricing page, cited with the date.
  - Tested in `signals/transcript_test.go`, citing [MEASURES-3-S1], [MEASURES-3-S2] and [MEASURES-3-S3].

## 2. The state (D3, D4, D5, D7)
- [ ] 2.1 `framework/internal/panel/state`: an `ApplyTranscript` for one session's reading, and a switch that marks the project as measured from transcripts.
  - **Context:** for a session with a recorded window, it goes through PANEL-48's one computation from the three counts and the window, into the session's `CtxPct`, the metrics' `CtxPct` and `CtxWindow`, and its tick and alert threshold (D4). The alert's time is the reading's.
  - **Cost:** it goes into `costByID`, `countCost`, `observeSpend` and the session's `Stats.CostUSD` (D5).
  - **Model:** `s.Model` takes the reading's model id.
  - **A partial reading:** one that has not reached the end of its files sets no figure.
  - **What it leaves alone:** a transcript reading leaves `statusPosts`, `StatusAt` and the quota's no-window count unchanged.
  - Tested in `framework/internal/panel/state/transcript_test.go`, citing [MEASURES-1-S1] and [MEASURES-3-S4].
- [ ] 2.2 In a project measured from transcripts, `ApplyStatus` and `Metrics.fold` still fold the quota and every figure a transcript does not carry.
  - They no longer set cost (`costByID`, `Stats.CostUSD`, `countCost`, `observeSpend`) or the model.
  - They no longer set the context of a session with a recorded window. They still set the context of a session without one (D3).
  - Tested in `state/transcript_test.go`, citing [MEASURES-1-S3] and [MEASURES-1-S6].
- [ ] 2.3 The warnings and the source:
  - **The window warning:** one per project for a session with no recorded window (D4). For a panel lane it names `launch.context_window`; for a session the panel did not launch it is worded as such.
  - **The unpriced warning:** one per lane and unpriced model, with its count of calls left out (D5).
  - **The `transcripts` source:** an entry in the view's `sources`, failed while any session's reading fails, with D7's error.
    - A session that failed on a line keeps its figures absent for the rest of the run.
    - The Stop and first-prompt conditions are judged against readings taken after them, and clear on a reading that finds what they lacked.
  - Tested in `state/transcript_test.go`, citing [MEASURES-1-S4], [MEASURES-2-S1], [MEASURES-2-S2], [MEASURES-2-S3], [MEASURES-2-S6] and [MEASURES-3-S2].

## 3. The runtime and the invariant (D3, D6, D8)
- [ ] 3.1 `framework/internal/panel/runtime.go` and `live.go`: pass whether the config is outside the repository into `newRuntime`, and add a `transcripts` source every 10 s only then.
  - It reads each session the state holds through 1.1's reader, with `claudecall.ConfigDir` as the directory, and keeps each session's cursor until the state forgets the session.
  - Tested in an integration test in `framework/internal/panel/transcripts_integration_test.go`, which builds the runtime through `live.go`'s path:
    - an attached project with a lane and a fixture transcript under a temporary config directory shows the lane's context and cost in `/api/state`;
    - an in-repository project with the same fixture opens nothing.
  - Citing [MEASURES-1-S1] and [MEASURES-1-S2].
- [ ] 3.2 `framework/internal/panel/notranscript_test.go`: rewrite `TestNoSourceReadsTranscripts` and its AST scan to exempt exactly `signals/transcript.go`. It fails when another file reaches a transcript, and when the exempt file is missing or no longer reaches one.
  - First show it failing, on a mention planted in a second file and on the exempt file renamed, and record both in the Decision log.
  - Add `signals/transcript.go` to `fileReadSites`.
  - Keep the `HookEvent` and `StatusPayload` field check.
  - Citing [MEASURES-4-S1].
- [ ] 3.3 A test in `notranscript_test.go` walks both of the reader's decode types by reflection, every nested struct included, and fails on each case D8 lists. It also checks by AST that `transcript.go` decodes JSON only into those two types and opens files only with `os.Open`.
  - It is shown failing first on an added `content` field and on a `json.RawMessage` field.
  - Citing [MEASURES-4-S2].
- [ ] 3.4 The comments of the old rule: `signals/hooks.go` (`HookEvent`'s: the path is not declared because the reader builds it from a validated id) and `state/trends.go:13`.

## 4. The page and the docs (D8, D9)
- [ ] 4.1 `framework/internal/panel/web/static/panel.js`: the quota's "No reading yet" note says the quota comes only with a status-line post, and that a lane measured from its transcript brings none. That is the one string, and nothing else in the file changes.
  - Tested in `framework/internal/panel/web/quota_note_test.go`, which reads the embedded `panel.js` for the sentence, citing [MEASURES-5-S1].
- [ ] 4.2 `docs/panel.md`:
  - Rewrite the old rule's four statements (*Keeping the panel current*, *Which agent started which*, *What the panel reads, and when*, *Security model*) to D8's invariant, including what its scans can and cannot see.
  - Add a subsection under *Signals*. It covers:
    - what the reader opens, decodes and keeps, and its cadence;
    - the price table, UNPRICED and synthetic records;
    - "could not read", and that a failed session stays unknown until a restart;
    - what a status post in an attached project still sets.
  - The alerts table's `context` row, *The page shows no context % or quota*, and the quota table say how an attached project's lanes are measured, and that the quota needs a status line.

- [ ] Slice: an owner can open an attached repository's lane on the panel at http://127.0.0.1:<port> and see its context bar, est. $ and context alert, read from its transcript
