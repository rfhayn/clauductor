# Tasks: panel-49-lane-measures

## Progress
- 2026-10-17 proposed; nothing built yet. The build starts from `main` once PANEL-48 has merged.
- 2026-10-17: 1.1–1.2 built in the cloud, reviewed in 2 rounds, peak medium (round 1: an unowned price check, now task 1.3; round 2: low); 1.0 pending on the Mac. Left open: a count of 2^63 or more wraps negative in `count()` (low). Groups 2–4 and the full gate run on the Mac after PANEL-48 merges.
- 2026-10-18: 1.0 and 1.3 done on the Mac, reviewed in 2 rounds, peak medium (round 1: the tool's `driver.mjs` kept Sonnet 5.5's $0.20, and the measurement did not count every line the reader fails on; round 2: low). Group 1 is complete; groups 2–4 next, through build-change.

## Decision log
- *(Settled 2026-10-18 by task 1.0's entry below.)* 2026-10-17, group 1: task 1.0 is pending on the owner's Mac; the cloud build cannot reach the transcripts. Its result sets `usageCounts` in `signals/transcript.go`, the one table that marks a count `Optional` (absent reads 0). All four counts stay required until then, and `TestTranscriptEveryRequiredCountIsChecked` holds them there.
- *(Superseded 2026-10-18 by task 1.3's entry below.)* 2026-10-17, group 1: the evidence tool is not in the repository (PANEL-48 task 1.5 has not committed `evidence/tool/`), so `PRICES` and `FAST_MULTIPLIER` could not be copied. The table (`signals/prices.go`) is taken from PANEL-48 `evidence/README.md` and the pricing page (platform.claude.com/docs/en/about-claude/pricing, read 2026-10-17), and each row cites the page. Task 1.3 owns the comparison, and `/verify-change` holds the merge until it is ticked.
- *(Settled 2026-10-18 by task 1.3's entry below: the tool had $0.20, and is corrected.)* 2026-10-17, group 1: **Sonnet 5.5's cache read is $0.10/MTok on the pricing page (0.05× input), where PANEL-48 `evidence/README.md` lists $0.20.** The table uses the page's $0.10. If the tool also has $0.20, its subagent figures overstate cache reads, which bears on the outcome's 10% comparison.
- 2026-10-17, group 1: Haiku 5.5 joins the table, at the pricing page's rates read 2026-10-17. Above a 100,000-token prompt (input, cache writes and cache reads all counted) the page's higher row applies to the whole call, so the row carries both. The alternative, leaving it unpriced, would warn on every Haiku subagent.
- *(Superseded 2026-10-18 by task 1.3's entry below: Opus 5 and 4.8 now have rows and fast rates.)* 2026-10-17, group 1: fast mode is priced for Opus 5.5 alone (the page's $8/$40, 2×, applied to every part of a call, caching included). The page also lists fast mode for Opus 5 and 4.8, which have no row. A `speed` other than absent, `standard` or `fast` is unpriced rather than priced at standard. A fast call on a priced model with no fast rate is counted as `<model> (fast)`.
- 2026-10-17, group 1: a complete line longer than a whole call's 32 MiB budget is a D7 error. D7 does not list it, but the alternative is a session that never reaches the end of its files and never shows an error. A blank line is skipped: it holds no field that could be misread.
- 2026-10-17, group 1: the price table lives in `signals/prices.go`, beside the reader, so `transcript.go` stays the one file that reads a transcript and the table holds no file access.
- 2026-10-18, group 1, task 1.0: **no line the reader would fail on, so all four counts stay required** and `usageCounts` is unchanged; D2 and D7 stand.
  - **Where:** Standing Tee's baseline week (2026-10-08..14: 116 transcript folders, 2,403 files, 39,271 non-synthetic `assistant` lines, Claude Code 2.1.292 to 2.1.300) and this repository's 2026-10-08..18 (497 files, 27,689 lines, 2.1.296 to 2.1.302).
  - **What failed, of what `readLine` fails on:** 0 `assistant` lines with no `message` or no `message.model`; 0 with no `message.usage`; 0 lacking any of `input_tokens`, `output_tokens`, `cache_creation_input_tokens` and `cache_read_input_tokens`; 0 with one of those that is not a non-negative integer; 0 lines of any type that are not valid JSON (up to each file's last newline, as the reader reads); 0 `assistant` lines with no timestamp, so the window missed none.
  - **How:** the evidence tool could not count this, because its `readTranscript` skips a line with no usage and reads an absent count as 0 (`?? 0`). A one-off script in the session's scratchpad read only `type`, `timestamp`, `message.model` and `message.usage` and printed counts, nothing else.
- 2026-10-18, group 1, task 1.3: the tool's `PRICES` and `FAST_MULTIPLIER` (now in `changes/archive/2026-10-17-panel-48-lane-context-policy/evidence/tool/usage-report.mjs`) compared row by row with `signals/prices.go` and the pricing page read 2026-10-18.
  - **Sonnet 5.5's cache read is $0.10**: the page's footnote 2 prices cache hits on Opus 5.5 and Sonnet 5.5 at 0.05× input. `prices.go` was right. The tool's $0.20 was wrong in two places, `usage-report.mjs`'s `PRICES` and `driver.mjs`'s Sonnet repricing (`SON`, and its note that the two models' cache reads are equal), and both are corrected. The tool's README says a re-run over the baseline now prints about $1,207.24 and lever 4 about $255.92, as `evidence/README.md`'s correction computes.
  - Opus 5.5 matches. The tool has no Haiku 5.5 row; `driver.mjs`'s Haiku repricing matches the page, and `PRICES` keeps none, so the tool stays the baseline's.
  - **`prices.go` lacked 16 rows the tool has** (Fable 5.1 and 5, Opus 5 to 4, Sonnet 5 to 4, Haiku 4.5) **and the fast rates of Opus 5 and Opus 4.8.** D5 copies the tool's table, so they are added, each matching the page. Without them a lane on one of them would show no cost: across both repositories' transcripts, Sonnet 5, Opus 5, Fable 5.1, Opus 4.7, Haiku 4.5 and Opus 4.8 all appear.
  - Neither table prices the long-context premium some models before 4.6 carried above 200k tokens. The page prices 4.6 and later at one rate across 1M, and no Sonnet 4.x, Opus 4.5 or earlier appears in those transcripts.
- 2026-10-17, group 1: until 3.2 rewrites the invariant, `TestNoSourceReadsTranscripts` (`.jsonl` in `signals/transcript.go`) and `TestNoSourceReachesTranscriptsByJoinOrNewReadSite` (its unlisted `os.Open`) fail in `internal/panel`. That is the old rule catching the new reader, as it should. Groups 1 and 3 must land together.

## 1. The reader (D1, D2, D5, D7)
- [x] 1.0 Before the counts are made required (D2): with the evidence tool, count how many of the baseline week's calls lack each of the four counts, reading no content, and record the result in the Decision log. A count some calls lack becomes optional, defaulting to 0. (manual: a one-off measurement over the owner's transcripts, not shipped code)
- [x] 1.1 `framework/internal/panel/signals/transcript.go`: find a session's transcript (D1), and walk its `subagents/` folder on every read. Decode every line, with no pre-filter, in D2's two stages, each into a declared type that holds D2's allowlist.
  - Stage one decodes `type` and `version`, and tolerates a type mismatch on any line that is not `assistant`. Stage two decodes `message` for an `assistant` line, where a mismatch is a D7 error.
  - A `<synthetic>` record is set aside before its usage is validated.
  - The id must be a UUID before any path is built.
  - Read from a cursor the caller keeps: per file, the offset and running totals, plus the last `assistant` message id and what was booked for it, a cost or an unpriced call.
  - Read only appended bytes up to the last newline, at most 32 MiB per call. A file that shrank is read from the start. Report whether the reader reached the end.
  - A message's streamed lines are booked once, at the last line read, across reads too.
  - `<synthetic>` records count toward neither cost nor context. The context and the model come from the last non-synthetic main-thread record; subagent records count toward cost only.
  - Report D7's errors, with the line's number, its `version` and the missing field. A required count that is absent is an error, and an optional one that is absent takes its default.
  - Tested in `framework/internal/panel/signals/transcript_test.go` over hand-written fixtures in `signals/testdata/transcripts/`, which hold no real transcript content. Citing [MEASURES-1-S5], [MEASURES-1-S7], [MEASURES-2-S1], [MEASURES-2-S4], [MEASURES-2-S5] and [MEASURES-4-S3].
- [x] 1.2 The price table in `signals` (D5): the evidence tool's `PRICES` and `FAST_MULTIPLIER`, each row with its source and date.
  - Exact version match, after a date suffix or a `[1m]` tag is stripped. Cache writes at 1.25× input for 5 minutes and 2× for an hour.
  - Unpriced calls are counted per model, never priced at $0.
  - Haiku 5.5 joins only with its rate from the published pricing page, cited with the date.
  - Tested in `signals/transcript_test.go`, citing [MEASURES-3-S1], [MEASURES-3-S2] and [MEASURES-3-S3].
- [x] 1.3 Once PANEL-48 has committed `evidence/tool/` (its task 1.5), compare `signals/prices.go` row by row with the tool's `PRICES` and `FAST_MULTIPLIER`, Sonnet 5.5's cache read above all ($0.10 here, $0.20 in PANEL-48 `evidence/README.md`). Record the result in the Decision log, and correct whichever is wrong before the outcome check compares the ledger with the tool. (manual: the tool is not in the repository yet)

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
