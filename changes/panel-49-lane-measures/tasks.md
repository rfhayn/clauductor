# Tasks: panel-49-lane-measures

## Progress
- 2026-10-17 proposed; nothing built yet. The build starts from `main` once PANEL-48 has merged.

## Decision log

## 1. The reader (D1, D2, D5, D7)
- [ ] 1.1 `framework/internal/panel/signals/transcript.go`: find a session's transcript and its subagents' (D1), and decode D2's allowlist into one declared type.
  - The id must be a UUID before any path is built.
  - Read from a cursor the caller keeps: per file, the offset and running totals. Read only appended bytes up to the last newline, at most 32 MiB per call. A file that shrank is read from the start. Report whether the reader reached the end.
  - Count each `message.id` once. The context is the last main-thread `assistant` record's three input counts; subagent records count toward cost only.
  - Report D7's errors with the line's number and its `version`.
  - Tested in `framework/internal/panel/signals/transcript_test.go` over hand-written fixtures in `signals/testdata/transcripts/`, which hold no real transcript content, citing [MEASURES-1-S5], [MEASURES-2-S1], [MEASURES-2-S4] and [MEASURES-4-S3].
- [ ] 1.2 The price table in `signals` (D5): the evidence tool's `PRICES` and `FAST_MULTIPLIER`, each row with its source and date.
  - Exact version match, after a date suffix or a `[1m]` tag is stripped. Cache writes at 1.25× input for 5 minutes and 2× for an hour.
  - Unpriced calls are counted per model, never priced at $0.
  - Haiku 5.5 joins only with its rate from the published pricing page, cited with the date.
  - Tested in `signals/transcript_test.go`, citing [MEASURES-3-S1], [MEASURES-3-S2] and [MEASURES-3-S3].

## 2. The state (D3, D4, D5, D7)
- [ ] 2.1 `framework/internal/panel/state`: an `ApplyTranscript` for one session's reading, and a switch that marks the project as measured from transcripts.
  - Context goes through PANEL-48's one computation from the three counts and the lane's recorded window, into the session's `CtxPct`, the metrics' `CtxPct` and `CtxWindow`, and its tick and alert threshold (D4).
  - Cost goes into `costByID`, `countCost` and `observeSpend`, as `ApplyStatus` sends a status post's total (D5).
  - A reading that has not reached the end of its files sets no figure.
  - A transcript reading leaves `statusPosts`, `StatusAt` and the quota's no-window count unchanged.
  - Tested in `framework/internal/panel/state/transcript_test.go`, citing [MEASURES-1-S1] and [MEASURES-3-S4].
- [ ] 2.2 In a project measured from transcripts, `ApplyStatus` and `Metrics.fold` still fold the quota and every other figure, but not the session's context or cost (D3).
  - Tested in `state/transcript_test.go`, citing [MEASURES-1-S3].
- [ ] 2.3 The warnings and the source:
  - one warning per project for a session with no recorded window, naming `launch.context_window` (D4);
  - one warning per lane and unpriced model, with its count of calls left out (D5);
  - a `transcripts` entry in the view's `sources`, failed with D7's error. The session's figures become absent, and a later good reading clears both. A session that sent `Stop` with no usage record, or has no transcript a minute after its first prompt, fails it too.
  - Tested in `state/transcript_test.go`, citing [MEASURES-1-S4], [MEASURES-2-S1], [MEASURES-2-S2], [MEASURES-2-S3] and [MEASURES-3-S2].

## 3. The runtime and the invariant (D3, D6, D8)
- [ ] 3.1 `framework/internal/panel/runtime.go`: a `transcripts` source every 10 s, added only when `outsideConfig` is set. It reads each session the state holds through 1.1's reader, with `claudecall.ConfigDir` as the directory, and keeps each session's cursor until the state forgets the session.
  - Tested in an integration test in `framework/internal/panel/transcripts_integration_test.go`: an attached project with a lane and a fixture transcript under a temporary config directory shows the lane's context and est. $ in `/api/state`; an in-repository project with the same fixture opens nothing. It cites [MEASURES-1-S1] and [MEASURES-1-S2].
- [ ] 3.2 `framework/internal/panel/notranscript_test.go`: rewrite `TestNoSourceReadsTranscripts` and its AST scan to exempt exactly `signals/transcript.go`. It fails when another file reaches a transcript, and when the exempt file is missing or no longer reaches one.
  - First show it failing, on a mention planted in a second file and on the exempt file renamed, and record both in the Decision log.
  - Add `signals/transcript.go` to `fileReadSites`.
  - Keep the `HookEvent` and `StatusPayload` field check.
  - Citing [MEASURES-4-S1].
- [ ] 3.3 A test in `notranscript_test.go` walks the reader's decode type by reflection and holds every json tag to D2's allowlist. It checks by AST that `transcript.go` decodes JSON only into that type and opens files only with `os.Open`. It is shown failing on an added `content` field first. Citing [MEASURES-4-S2].
- [ ] 3.4 `signals/hooks.go`: `HookEvent`'s comment says the path is not declared because the reader builds it from a validated id (D1). It no longer says the panel never reads transcripts.

## 4. The page and the docs (D8, D9)
- [ ] 4.1 `framework/internal/panel/web/static/panel.js`: the quota's "No reading yet" note says the quota comes only with a status-line post, and that a lane measured from its transcript brings none. That is the one string, and nothing else in the file changes.
  - Tested in `framework/internal/panel/web/quota_note_test.go`, which reads the embedded `panel.js` for the sentence, citing [MEASURES-5-S1].
- [ ] 4.2 `docs/panel.md`:
  - Rewrite the old rule's four statements (*Keeping the panel current*, *Which agent started which*, *What the panel reads, and when*, *Security model*) to D8's invariant.
  - Add a subsection under *Signals* on what the reader opens, decodes and keeps, its cadence, the price table and UNPRICED, "could not read", and that a status post in an attached project sets neither context nor cost.
  - The alerts table's `context` row, *The page shows no context % or quota*, and the quota table say how an attached project's lanes are measured, and that the quota needs a status line.

- [ ] Slice: an owner can open an attached repository's lane on the panel at http://127.0.0.1:<port> and see its context bar, est. $ and context alert, read from its transcript
