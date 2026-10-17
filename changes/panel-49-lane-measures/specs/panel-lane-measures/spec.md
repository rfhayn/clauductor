# panel-lane-measures Specification

## Purpose
The panel measures the lanes of a repository it attaches to from each session's own transcript: their context against the window the lane was launched with, and their cost priced per model. It does this because an attached repository's status line does not post to the panel. It reads transcripts in one place, decodes only the fields that measuring needs, and says when it cannot read one rather than showing a number it does not have.

## ADDED Requirements
### Requirement: An attached project's sessions are measured from their transcripts
WHERE a project's config resolves outside its repository THE SYSTEM SHALL take the context and cost of each session it holds in that project from the session's own transcript and its subagents' transcripts, and SHALL NOT take them from a status post; WHERE a project's config is in its repository THE SYSTEM SHALL read no transcript for it.

#### Scenario: [MEASURES-1-S1] An attached lane shows its context, cost and alert
- **GIVEN** an attached project, `alerts.context_pct` 85, and a lane launched in it with a 200000 window
- **AND** the session's transcript, whose last `assistant` record's input, cache-creation and cache-read tokens sum to 160000, and one subagent transcript under the session's `subagents/` folder
- **WHEN** the reader has read both files to their end
- **THEN** the lane's context reads 80%, its alert threshold is 78.5, and the context alert fires for it
- **AND** the session's est. $ is the priced sum of every call in both files, each message counted once

#### Scenario: [MEASURES-1-S2] A project with its config in its repository reads no transcript
- **GIVEN** a project whose config is in its repository, and a transcript for one of its sessions under Claude Code's config directory
- **WHEN** the panel serves the project, and the session's status line posts `used_percentage` 42
- **THEN** the panel opens no file under the config directory's `projects/`
- **AND** the session's context reads 42%, as today

#### Scenario: [MEASURES-1-S3] A status post in an attached project moves the quota, not the context or cost
- **GIVEN** an attached project whose session has been measured from its transcript at 80% and $1.20
- **WHEN** that session's status line posts `used_percentage` 10, `total_cost_usd` 9, and a `five_hour` rate limit of 30%
- **THEN** the quota's 5-hour window reads 30%
- **AND** the session's context still reads 80%, and its est. $ $1.20

#### Scenario: [MEASURES-1-S4] A lane without a recorded window shows cost, no percentage, and a warning
- **GIVEN** an attached project whose config sets no `launch.context_window`, and a lane launched in it
- **WHEN** the reader has read the session's transcript
- **THEN** the session's est. $ is shown, and its context percentage is absent, not 0
- **AND** the page carries one warning for the project naming `launch.context_window`

#### Scenario: [MEASURES-1-S5] Only appended bytes are read, and each message is counted once
- **GIVEN** a transcript the reader has read to its end
- **WHEN** two lines of one streamed message, with the same `message.id`, are appended, and then a third line is appended without its newline
- **THEN** the next read adds that message's cost once
- **AND** the partial line is not read until its newline arrives

### Requirement: A transcript the reader does not understand reads as "could not read", never 0
WHEN a transcript holds a complete line that is not JSON, or an `assistant` line without `message.model`, without `message.usage` or with a count that is not a number, or a session that has finished a turn has a transcript with no usage record, or none is found a minute after the session's first prompt, THE SYSTEM SHALL show that session's context and cost as unknown and SHALL report the project's `transcripts` source as failed, naming the session, and the line and its Claude Code version where there is one.

#### Scenario: [MEASURES-2-S1] An assistant line without usage
- **GIVEN** an attached project whose session's transcript has been read at 80% and $1.20
- **WHEN** an `assistant` line with `"version": "2.1.400"` and no `message.usage` is appended
- **THEN** the session's context and est. $ are absent, neither 80% and $1.20 nor 0
- **AND** the `transcripts` source has failed with an error naming the session, the line's number and 2.1.400, which the page shows as a "Cannot read" banner

#### Scenario: [MEASURES-2-S2] A finished turn with no usage record
- **GIVEN** an attached project's session that has sent a `Stop` hook
- **WHEN** its transcript holds lines but no `assistant` record with usage
- **THEN** the `transcripts` source has failed, naming the session and "no usage record"

#### Scenario: [MEASURES-2-S3] No transcript a minute after the first prompt
- **GIVEN** an attached project's session whose first prompt was submitted 61 s ago
- **WHEN** no `<id>.jsonl` exists under any folder of the config directory's `projects/`
- **THEN** the `transcripts` source has failed, naming the session and the folder searched
- **AND** at 59 s it had not failed, and the session's figures were absent

#### Scenario: [MEASURES-2-S4] Lines of other types are skipped
- **GIVEN** a transcript whose lines include `user`, `attachment`, `system` and a type the reader has never seen, none containing `"usage"`
- **WHEN** the reader reads it
- **THEN** it reports no error, and the figures come from its `assistant` records alone

### Requirement: Cost is priced per model, and an unknown model is unpriced, never $0
THE SYSTEM SHALL price each call of a transcript by its exact model version and speed from its price table, SHALL leave a call whose model or speed the table does not price out of the figure, SHALL report each such model with its count of calls left out, and SHALL show no cost for a session none of whose calls is priced.

#### Scenario: [MEASURES-3-S1] An Opus 5.5 call is priced by its parts
- **GIVEN** one `claude-opus-5-5` call with 1000 input tokens, 3000 cache-creation tokens of which 1000 are for one hour, 100000 cache-read tokens and 500 output tokens
- **WHEN** it is priced
- **THEN** its cost is $0.052: input $0.004, the 5-minute write $0.010, the 1-hour write $0.008, the read $0.020 and the output $0.010

#### Scenario: [MEASURES-3-S2] An unknown model is left out and named
- **GIVEN** a session with two `claude-opus-5-5` calls and three `claude-opus-5-6` calls
- **WHEN** it is priced
- **THEN** its est. $ is the two Opus 5.5 calls' cost alone
- **AND** the page carries a warning naming the lane, `claude-opus-5-6` and 3 calls left out
- **AND** a session with only the three `claude-opus-5-6` calls shows no est. $, not $0

#### Scenario: [MEASURES-3-S3] A dated or tagged id is its version, and fast mode is priced only where its rate is known
- **GIVEN** calls on `claude-opus-5-5-20261001`, on `claude-opus-5-5[1m]`, on `claude-opus-5-5` with `speed` `fast`, and on `claude-sonnet-5-5` with `speed` `fast`
- **WHEN** they are priced
- **THEN** the first two are priced as `claude-opus-5-5`, and the third at twice its rates
- **AND** the fourth is unpriced

#### Scenario: [MEASURES-3-S4] A transcript's cost reaches the ledger as a status post's does
- **GIVEN** an attached project's session read at $1.20
- **WHEN** $0.30 more is appended and read
- **THEN** the project's est. $ and cost today rise by $0.30
- **AND** the spend ledger books $0.30 more for the session's lane type, model and branch

### Requirement: The panel reads transcripts only through its one reader, and decodes only what measuring needs
THE SYSTEM SHALL read transcripts in one source file, read-only, by a session id that is a UUID, under Claude Code's config directory, decoding only each line's type, timestamp and version and an assistant message's id, model and token counts; and every other shipped source of the panel SHALL reach no transcript, as before this change.

#### Scenario: [MEASURES-4-S1] A second source that reaches a transcript fails the build
- **GIVEN** the panel's shipped sources
- **WHEN** a file other than the reader mentions `.jsonl` or `transcript_path`, or joins `.claude` and `projects` into a path
- **THEN** `TestNoSourceReadsTranscripts` fails, naming that file
- **AND** it fails as well when the reader's file is missing or no longer reaches a transcript

#### Scenario: [MEASURES-4-S2] A field added to the reader's decoding fails the build
- **GIVEN** the reader's declared decode type
- **WHEN** a field outside the allowlist, such as `message.content`, is added to it, or a line is decoded into a map
- **THEN** the allowlist test fails, naming the field or the decode

#### Scenario: [MEASURES-4-S3] An id that is not a UUID opens nothing
- **GIVEN** a session id `../../etc/x`, and one `abc`
- **WHEN** the reader is asked to read either
- **THEN** it opens no file and builds no path, and the session is never measured

### Requirement: The quota's note says a transcript brings no quota
WHEN the quota has no reading THE SYSTEM SHALL say, where it says the quota comes from a status-line post, that a lane measured from its transcript brings none.

#### Scenario: [MEASURES-5-S1] A machine whose lanes are all measured from transcripts
- **GIVEN** a panel serving only attached projects, whose sessions post no status line
- **WHEN** the page draws the quota
- **THEN** its fields show no reading
- **AND** their note says the quota comes only with a status-line post, and that a lane measured from its transcript brings none
