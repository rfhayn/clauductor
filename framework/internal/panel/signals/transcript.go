package signals

// The transcript reader (PANEL-49). An attached repository's status line does not
// post to the panel, so its lanes are measured from each session's own transcript:
// the context of its last main-thread call, and the cost of every call it and its
// subagents made, priced per model.
//
// This is the one file of the panel that reads a transcript, and it reads as little
// as it can (design D1, D2, D7, D8):
//   - a session is found by its id, which must be a UUID before any path is built,
//     under Claude Code's config directory; no path comes from a payload;
//   - files are opened read-only, with os.Open alone;
//   - every line is decoded, with no pre-filter, in two stages, each into a declared
//     type that holds only D2's fields. encoding/json keeps nothing a type does not
//     declare, so prompts, tool input and output, paths and assistant text are
//     scanned and dropped, never stored;
//   - a shape the reader does not know is "could not read", never 0, and the session
//     stays failed until a fresh cursor (a panel restart) reads its files again.
//
// signals keeps no state: the caller keeps each session's TranscriptCursor.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// maxTranscriptRead is the most one session's reading reads per call (D6), so a
// long transcript is read over several polls rather than in one.
const maxTranscriptRead = 32 << 20

// syntheticModel marks a record Claude Code writes itself, for an API error or a
// rate-limit stop. It is no API call: it is set aside before its usage is validated.
const syntheticModel = "<synthetic>"

// ErrNotUUID is returned, before any path is built, for a session id that is not a
// UUID, the shape --session-id takes.
var ErrNotUUID = errors.New("session id is not a UUID")

var sessionUUID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// transcriptHead is stage one, decoded from every line. A line that is not an
// assistant line stops here, and a field of the wrong JSON type in it is no error.
type transcriptHead struct {
	Type    string `json:"type"`
	Version string `json:"version"`
}

// transcriptAssistant is stage two, decoded from an assistant line only. Here a
// field of the wrong type is an error, except in a synthetic record.
type transcriptAssistant struct {
	Message *transcriptMessage `json:"message"`
}

type transcriptMessage struct {
	ID    string           `json:"id"`
	Model string           `json:"model"`
	Usage *transcriptUsage `json:"usage"`
}

// The counts are pointers so that an absent one is told apart from 0 (D2), and
// unsigned so that a negative one is a type error, "could not read" (D7).
type transcriptUsage struct {
	InputTokens              *uint64                 `json:"input_tokens"`
	OutputTokens             *uint64                 `json:"output_tokens"`
	CacheCreationInputTokens *uint64                 `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     *uint64                 `json:"cache_read_input_tokens"`
	CacheCreation            transcriptCacheCreation `json:"cache_creation"` // optional: absent, every write is a 5-minute write
	Speed                    string                  `json:"speed"`          // optional: absent, standard
}

type transcriptCacheCreation struct {
	Ephemeral1hInputTokens uint64 `json:"ephemeral_1h_input_tokens"`
}

// usageCount is one of the four counts D2 names.
type usageCount struct {
	Name     string
	Optional bool // absent reads as 0 instead of failing the session
	get      func(*transcriptUsage) *uint64
}

// usageCounts decides which counts an assistant line must carry. Task 1.0 counts,
// over a real week of the owner's transcripts, how many calls lack each one; a count
// some calls lack is marked Optional here, defaulting to 0, and that is a change to
// design.md D2 for the owner. Until then all four are required: a renamed count must
// not read as an empty context.
var usageCounts = []usageCount{
	{Name: "input_tokens", get: func(u *transcriptUsage) *uint64 { return u.InputTokens }},
	{Name: "output_tokens", get: func(u *transcriptUsage) *uint64 { return u.OutputTokens }},
	{Name: "cache_creation_input_tokens", get: func(u *transcriptUsage) *uint64 { return u.CacheCreationInputTokens }},
	{Name: "cache_read_input_tokens", get: func(u *transcriptUsage) *uint64 { return u.CacheReadInputTokens }},
}

// TranscriptCursor is one session's place in its transcripts, kept by the caller
// between reads and dropped when the session is forgotten. Its zero value starts a
// reading from the beginning.
type TranscriptCursor struct {
	main   string                     // the session's own transcript, once found
	files  map[string]*transcriptFile // by path: the main transcript and each subagent's
	failed *TranscriptError           // set once a line fails; the session stays failed
}

// transcriptFile is the reader's place in one file, and what it has booked from it.
type transcriptFile struct {
	offset int64 // bytes read, always just past a newline
	lines  int   // complete lines read, for an error's line number

	// Every message before the last, booked for good.
	cost     float64
	priced   int
	unpriced map[string]int

	// The last assistant message and what was booked for it. A later line with the
	// same id replaces the booking, in this read or a later one (D5).
	lastID  string
	last    booking
	hasLast bool

	// Main thread only: the last non-synthetic record, and whether any assistant
	// record (a synthetic one included) has been seen.
	ctx      *TranscriptContext
	model    string
	answered bool
}

// booking is what one message was booked as: a cost, or an unpriced call against
// unpriced (its model).
type booking struct {
	cost     float64
	unpriced string
}

func (f *transcriptFile) settle() {
	if !f.hasLast {
		return
	}
	if f.last.unpriced != "" {
		if f.unpriced == nil {
			f.unpriced = map[string]int{}
		}
		f.unpriced[f.last.unpriced]++
	} else {
		f.cost += f.last.cost
		f.priced++
	}
	f.hasLast = false
}

func (f *transcriptFile) book(id, model string, u callUsage) {
	b := booking{unpriced: unpricedKey(model, u.Speed)}
	if cost, ok := priceCall(model, u); ok {
		b = booking{cost: cost}
	}
	if f.hasLast && id != "" && id == f.lastID {
		f.last = b
		return
	}
	f.settle()
	f.lastID, f.last, f.hasLast = id, b, true
}

// TranscriptContext is the three counts of the last main-thread call that make up
// the session's context, the same three PANEL-48 sums from a status line's
// current_usage.
type TranscriptContext struct {
	InputTokens         int64
	CacheCreationTokens int64
	CacheReadTokens     int64
}

// Tokens is the context's size, to divide by the lane's recorded window.
func (c TranscriptContext) Tokens() int64 {
	return c.InputTokens + c.CacheCreationTokens + c.CacheReadTokens
}

// TranscriptError is a line the reader could not read (D7).
type TranscriptError struct {
	Session string
	File    string // relative to the config directory's projects/
	Line    int
	Version string // the line's Claude Code version, where it could be read
	Field   string // the field missing or of the wrong type; empty for a line that is not JSON
	Reason  string
}

func (e *TranscriptError) Error() string {
	version := ""
	if e.Version != "" {
		version = " (Claude Code " + e.Version + ")"
	}
	what := e.Reason
	if e.Field != "" {
		what = e.Field + " " + e.Reason
	}
	return fmt.Sprintf("session %s: %s line %d%s: %s", e.Session, e.File, e.Line, version, what)
}

// TranscriptReading is what one read of a session's transcripts found.
type TranscriptReading struct {
	// Found is whether the session's own transcript exists; Searched is the folder
	// it was looked for under, for an error that it is missing.
	Found    bool
	Searched string
	// AtEnd is whether every file was read to its last complete line. A reading that
	// is not at the end holds a partial total, which must not be shown (D6).
	AtEnd bool
	// Answered is whether the main thread holds any assistant record, a synthetic one
	// included: D7's Stop condition.
	Answered bool

	// Model and Context come from the last non-synthetic main-thread record; nil
	// Context means there is none yet.
	Model   string
	Context *TranscriptContext
	// CostUSD is the priced sum over the session and its subagents, each message
	// counted once. It is nil, not 0, while no call is priced (D5).
	CostUSD     *float64
	PricedCalls int
	// Unpriced counts the calls left out of CostUSD, by model.
	Unpriced map[string]int

	// Failed is set once a line could not be read. The reading then carries no
	// figure, and every later read with the same cursor returns the same error.
	Failed *TranscriptError
}

// ReadTranscript reads what was appended to a session's transcript and its
// subagents' since the cursor's last read, under configDir (claudecall.ConfigDir).
// The error is for an id that is not a UUID or a file that could not be read; a line
// that could not be read is the reading's Failed.
func ReadTranscript(configDir, sessionID string, cur *TranscriptCursor) (TranscriptReading, error) {
	return readTranscript(configDir, sessionID, cur, maxTranscriptRead)
}

func readTranscript(configDir, sessionID string, cur *TranscriptCursor, limit int64) (TranscriptReading, error) {
	if !sessionUUID.MatchString(sessionID) {
		return TranscriptReading{}, ErrNotUUID
	}
	projects := filepath.Join(configDir, "projects")
	r := TranscriptReading{Searched: projects}
	if cur.failed != nil {
		r.Found, r.Failed = true, cur.failed
		return r, nil
	}
	if cur.main == "" {
		main, err := findTranscript(projects, sessionID)
		if err != nil || main == "" {
			return r, err
		}
		cur.main = main
	}
	if _, err := os.Stat(cur.main); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// The file is gone: start again, and look for it on the next read.
			*cur = TranscriptCursor{}
			return r, nil
		}
		return r, err
	}
	r.Found = true

	paths := []string{cur.main}
	subagents := filepath.Join(filepath.Dir(cur.main), sessionID, "subagents")
	// The folder is walked on every read, because a new subagent adds a file (D1).
	err := filepath.WalkDir(subagents, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == subagents && errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if d.Type().IsRegular() && strings.HasSuffix(path, ".jsonl") {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return r, err
	}
	if cur.files == nil {
		cur.files = map[string]*transcriptFile{}
	}
	budget := limit
	r.AtEnd = true
	for _, path := range paths {
		f := cur.files[path]
		if f == nil {
			f = &transcriptFile{}
			cur.files[path] = f
		}
		rel, _ := filepath.Rel(projects, path)
		atEnd, lineErr, err := readTranscriptFile(path, f, path == cur.main, &budget, limit)
		if err != nil {
			return TranscriptReading{Found: true, Searched: projects}, err
		}
		if lineErr != nil {
			lineErr.Session, lineErr.File = sessionID, rel
			cur.failed = lineErr
			return TranscriptReading{Found: true, Searched: projects, Failed: lineErr}, nil
		}
		r.AtEnd = r.AtEnd && atEnd
	}

	// A file booked earlier and since removed (a subagent's) keeps its calls: they
	// were made.
	var cost float64
	for _, f := range cur.files {
		cost += f.cost
		r.PricedCalls += f.priced
		for m, n := range f.unpriced {
			addUnpriced(&r, m, n)
		}
		if f.hasLast {
			if f.last.unpriced != "" {
				addUnpriced(&r, f.last.unpriced, 1)
			} else {
				cost += f.last.cost
				r.PricedCalls++
			}
		}
	}
	if r.PricedCalls > 0 {
		r.CostUSD = &cost
	}
	main := cur.files[cur.main]
	r.Answered, r.Model = main.answered, main.model
	if main.ctx != nil {
		c := *main.ctx
		r.Context = &c
	}
	return r, nil
}

func addUnpriced(r *TranscriptReading, model string, n int) {
	if r.Unpriced == nil {
		r.Unpriced = map[string]int{}
	}
	r.Unpriced[model] += n
}

// findTranscript lists projects/ once for <id>.jsonl in any of its folders. Claude
// Code's rule for naming a folder after a path is undocumented, so the folder is not
// built from the lane's path (D1). It returns "" while there is none.
func findTranscript(projects, sessionID string) (string, error) {
	entries, err := os.ReadDir(projects)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		path := filepath.Join(projects, e.Name(), sessionID+".jsonl")
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			return path, nil
		}
	}
	return "", nil
}

// readTranscriptFile reads one file's appended bytes up to its last newline, within
// what is left of the call's budget. atEnd is whether nothing but a partial line
// remains unread.
func readTranscriptFile(path string, f *transcriptFile, main bool, budget *int64, limit int64) (atEnd bool, lineErr *TranscriptError, err error) {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) && !main {
			return true, nil, nil
		}
		return false, nil, err
	}
	if info.Size() < f.offset {
		// The file shrank: read it again from the start, its totals reset (D6).
		*f = transcriptFile{}
	}
	avail := info.Size() - f.offset
	if avail == 0 {
		return true, nil, nil
	}
	n := min(avail, *budget)
	if n == 0 {
		return false, nil, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return false, nil, err
	}
	defer file.Close()
	buf := make([]byte, n)
	got, err := io.ReadFull(io.NewSectionReader(file, f.offset, n), buf)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return false, nil, err
	}
	buf = buf[:got]
	*budget -= int64(got)
	cut := int64(got) < avail
	end := bytes.LastIndexByte(buf, '\n')
	if end < 0 {
		if cut && int64(got) == limit {
			// A whole call's budget holds no newline: the line can never be read, and
			// waiting would leave the session short of the end with no error.
			return false, &TranscriptError{Line: f.lines + 1, Reason: fmt.Sprintf("a line longer than %d bytes", limit)}, nil
		}
		return !cut, nil, nil
	}
	for _, line := range bytes.Split(buf[:end], []byte{'\n'}) {
		f.lines++
		if lineErr := f.readLine(line, main); lineErr != nil {
			lineErr.Line = f.lines
			return false, lineErr, nil
		}
	}
	f.offset += int64(end) + 1
	return !cut, nil, nil
}

// readLine decodes one complete line in D2's two stages and books it.
func (f *transcriptFile) readLine(line []byte, main bool) *TranscriptError {
	if len(bytes.TrimSpace(line)) == 0 {
		return nil // a blank line carries no field to misread
	}
	if !json.Valid(line) {
		return &TranscriptError{Reason: "is not valid JSON"}
	}
	var head transcriptHead
	if err := json.Unmarshal(line, &head); head.Type != "assistant" {
		return nil
	} else if err != nil {
		return decodeError(head.Version, err)
	}
	var a transcriptAssistant
	err := json.Unmarshal(line, &a)
	// The model is read first: a synthetic record is set aside before anything else
	// in it is validated, so a malformed one is no error (D2).
	if a.Message != nil && a.Message.Model == syntheticModel {
		f.answered = f.answered || main
		return nil
	}
	if err != nil {
		return decodeError(head.Version, err)
	}
	if a.Message == nil || a.Message.Model == "" {
		return &TranscriptError{Version: head.Version, Field: "message.model", Reason: "missing"}
	}
	u := a.Message.Usage
	if u == nil {
		return &TranscriptError{Version: head.Version, Field: "message.usage", Reason: "missing"}
	}
	for _, c := range usageCounts {
		if c.get(u) == nil && !c.Optional {
			return &TranscriptError{Version: head.Version, Field: "message.usage." + c.Name, Reason: "missing"}
		}
	}
	call := callUsage{
		Input:        count(u.InputTokens),
		Output:       count(u.OutputTokens),
		CacheWrite:   count(u.CacheCreationInputTokens),
		CacheWrite1h: int64(u.CacheCreation.Ephemeral1hInputTokens),
		CacheRead:    count(u.CacheReadInputTokens),
		Speed:        u.Speed,
	}
	f.book(a.Message.ID, a.Message.Model, call)
	if main {
		f.answered = true
		f.model = a.Message.Model
		f.ctx = &TranscriptContext{InputTokens: call.Input, CacheCreationTokens: call.CacheWrite, CacheReadTokens: call.CacheRead}
	}
	return nil
}

// count is an optional count's value: absent reads as 0.
func count(p *uint64) int64 {
	if p == nil {
		return 0
	}
	return int64(*p)
}

func decodeError(version string, err error) *TranscriptError {
	var te *json.UnmarshalTypeError
	if errors.As(err, &te) {
		return &TranscriptError{Version: version, Field: te.Field, Reason: "is a " + te.Value + ", not the type expected"}
	}
	return &TranscriptError{Version: version, Reason: err.Error()}
}
