package signals

import (
	"errors"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The session id every fixture under testdata/transcripts uses.
const fixtureSession = "11111111-1111-4111-8111-111111111111"

// transcriptFixture copies testdata/transcripts/<name> into a temporary config
// directory, so a test can append to its files, and returns that directory and the
// path of the session's own transcript.
func transcriptFixture(t *testing.T, name string) (configDir, main string) {
	t.Helper()
	src := filepath.Join("testdata", "transcripts", name)
	configDir = t.TempDir()
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		dst := filepath.Join(configDir, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return configDir, filepath.Join(configDir, "projects", "-fixture-repo", fixtureSession+".jsonl")
}

func appendTo(t *testing.T, path, s string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(s); err != nil {
		t.Fatal(err)
	}
}

// opusLine is one hand-written assistant line on claude-opus-5-5.
func opusLine(id string, in, out, cw, cr int) string {
	return `{"type":"assistant","version":"2.1.302","message":{"id":"` + id + `","model":"claude-opus-5-5","usage":{"input_tokens":` +
		strconv.Itoa(in) + `,"output_tokens":` + strconv.Itoa(out) + `,"cache_creation_input_tokens":` + strconv.Itoa(cw) + `,"cache_read_input_tokens":` + strconv.Itoa(cr) + "}}}\n"
}

func read(t *testing.T, configDir string, cur *TranscriptCursor) TranscriptReading {
	t.Helper()
	r, err := ReadTranscript(configDir, fixtureSession, cur)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func wantCost(t *testing.T, r TranscriptReading, want float64) {
	t.Helper()
	if r.CostUSD == nil {
		t.Fatalf("cost is absent, want $%.6f", want)
	}
	if !near(*r.CostUSD, want) {
		t.Fatalf("cost $%.6f, want $%.6f", *r.CostUSD, want)
	}
}

func wantContext(t *testing.T, r TranscriptReading, want int64) {
	t.Helper()
	if r.Context == nil {
		t.Fatalf("context is absent, want %d tokens", want)
	}
	if got := r.Context.Tokens(); got != want {
		t.Fatalf("context %d tokens, want %d", got, want)
	}
}

// The basic fixture, priced by hand at the published rates (per million tokens:
// Opus 5.5 in 4, out 20, read 0.20; Sonnet 5.5 in 2, out 10, read 0.10; a 5-minute
// write 1.25x input, a 1-hour write 2x):
//   - main msg_A, at its LAST line: 0.004 + 0.010 + 2000*5e-6 + 100000*0.2e-6 = 0.044
//   - main msg_B: 0.008 + 0.020 + 8000*5e-6 + 150000*0.2e-6 = 0.098
//   - subagents/agent-a (Sonnet, 1-hour writes): 0.002 + 0.010 + 4000*4e-6 + 200000*0.1e-6 = 0.048
//   - subagents/workflows/run-1/agent-b: 0.002 + 0.002 + 50000*0.2e-6 = 0.014
const basicCost = 0.204

// The last main-thread record, msg_B: 2000 + 8000 + 150000.
const basicContext = 160000

// The session's own transcript and every subagent transcript under its folder,
// workflow agents' deeper folders included, count toward cost; the context and the
// model come from the main thread alone (D1, D4, D5).
func TestTranscriptReadsTheSessionAndItsSubagents(t *testing.T) {
	t.Parallel()
	configDir, main := transcriptFixture(t, "basic")
	var cur TranscriptCursor
	r := read(t, configDir, &cur)
	if !r.Found || !r.AtEnd || r.Failed != nil {
		t.Fatalf("found %v, at end %v, failed %v", r.Found, r.AtEnd, r.Failed)
	}
	wantCost(t, r, basicCost)
	if r.PricedCalls != 4 {
		t.Errorf("priced calls %d, want 4", r.PricedCalls)
	}
	// agent-a's last record holds 205000 tokens: a subagent must not set the context.
	wantContext(t, r, basicContext)
	if r.Model != "claude-opus-5-5" {
		t.Errorf("model %q, want the main thread's claude-opus-5-5, not the subagent's", r.Model)
	}
	if !r.Answered {
		t.Error("a transcript with assistant records reads as unanswered")
	}

	// A subagent that starts after the first read is found on the next one.
	sub := filepath.Join(filepath.Dir(main), fixtureSession, "subagents", "agent-c.jsonl")
	appendTo(t, sub, opusLine("msg_S9", 1000, 0, 0, 0)) // $0.004
	r = read(t, configDir, &cur)
	wantCost(t, r, basicCost+0.004)
	wantContext(t, r, basicContext)
}

// [MEASURES-1-S5] Only appended bytes are read, and a streamed message counts once,
// at its last line: one line of a message with output 10 is appended and read, then
// a second line of the same message.id with output 400, then a third line without
// its newline.
func TestTranscriptBooksAStreamedMessageOnceAtItsLastLine(t *testing.T) {
	t.Parallel()
	configDir, main := transcriptFixture(t, "basic")
	var cur TranscriptCursor
	wantCost(t, read(t, configDir, &cur), basicCost)

	appendTo(t, main, opusLine("msg_C", 0, 10, 0, 0)) // $0.0002
	wantCost(t, read(t, configDir, &cur), basicCost+0.0002)

	// A line of another type between the streamed lines does not reset the message.
	appendTo(t, main, `{"type":"user","version":"2.1.302","message":{"role":"user","content":"placeholder"}}`+"\n")
	appendTo(t, main, opusLine("msg_C", 0, 400, 0, 0)) // $0.008, replacing the $0.0002
	r := read(t, configDir, &cur)
	wantCost(t, r, basicCost+0.008)
	if r.PricedCalls != 5 {
		t.Errorf("priced calls %d, want 5: msg_C is one call", r.PricedCalls)
	}

	// A partial line waits for its newline, and the reading is still at the end.
	partial := strings.TrimSuffix(opusLine("msg_D", 0, 1000, 0, 0), "\n") // $0.02
	appendTo(t, main, partial)
	r = read(t, configDir, &cur)
	wantCost(t, r, basicCost+0.008)
	if !r.AtEnd || r.Failed != nil {
		t.Fatalf("a partial line: at end %v, failed %v", r.AtEnd, r.Failed)
	}
	appendTo(t, main, "\n")
	wantCost(t, read(t, configDir, &cur), basicCost+0.008+0.02)
}

// [MEASURES-1-S7] A synthetic record moves neither context nor cost, is not
// unpriced, and is set aside before its usage is validated (D2).
func TestTranscriptSetsSyntheticRecordsAside(t *testing.T) {
	t.Parallel()
	configDir, main := transcriptFixture(t, "basic")
	var cur TranscriptCursor
	r := read(t, configDir, &cur)
	wantContext(t, r, basicContext)
	wantCost(t, r, basicCost)

	appendTo(t, main, `{"type":"assistant","version":"2.1.302","message":{"id":"msg_syn1","model":"<synthetic>","usage":{"input_tokens":0,"output_tokens":0,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`+"\n")
	appendTo(t, main, `{"type":"assistant","version":"2.1.302","message":{"id":"msg_syn2","model":"<synthetic>","usage":{"input_tokens":"not a number"}}}`+"\n")
	appendTo(t, main, `{"type":"assistant","version":"2.1.302","message":{"model":"<synthetic>","content":"placeholder"}}`+"\n")
	r = read(t, configDir, &cur)
	if r.Failed != nil {
		t.Fatalf("a synthetic record failed the reading: %v", r.Failed)
	}
	wantContext(t, r, basicContext)
	wantCost(t, r, basicCost)
	if r.Model != "claude-opus-5-5" {
		t.Errorf("model %q, want claude-opus-5-5", r.Model)
	}
	for m := range r.Unpriced {
		if strings.Contains(m, "synthetic") {
			t.Errorf("%q is reported unpriced", m)
		}
	}
	if r.PricedCalls != 4 {
		t.Errorf("priced calls %d, want 4", r.PricedCalls)
	}

	// A synthetic record between a message's streamed lines does not reset it.
	appendTo(t, main, opusLine("msg_E", 0, 10, 0, 0))
	appendTo(t, main, `{"type":"assistant","version":"2.1.302","message":{"id":"msg_syn3","model":"<synthetic>","usage":{"input_tokens":0,"output_tokens":0,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`+"\n")
	appendTo(t, main, opusLine("msg_E", 0, 400, 0, 0))
	wantCost(t, read(t, configDir, &cur), basicCost+0.008)
}

// A synthetic record still counts as an answer for D7's Stop condition, and a
// transcript with no assistant record does not.
func TestTranscriptAnsweredCountsSyntheticRecords(t *testing.T) {
	t.Parallel()
	configDir := t.TempDir()
	main := filepath.Join(configDir, "projects", "-fixture-repo", fixtureSession+".jsonl")
	if err := os.MkdirAll(filepath.Dir(main), 0o755); err != nil {
		t.Fatal(err)
	}
	appendTo(t, main, `{"type":"user","version":"2.1.302","message":{"role":"user","content":"placeholder"}}`+"\n")
	var cur TranscriptCursor
	r := read(t, configDir, &cur)
	if r.Answered || r.CostUSD != nil || r.Context != nil || r.Model != "" {
		t.Fatalf("a transcript with no assistant record: answered %v, cost %v, context %v, model %q", r.Answered, r.CostUSD, r.Context, r.Model)
	}
	appendTo(t, main, `{"type":"assistant","version":"2.1.302","message":{"id":"msg_syn","model":"<synthetic>","usage":{"input_tokens":0,"output_tokens":0,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`+"\n")
	r = read(t, configDir, &cur)
	if !r.Answered {
		t.Error("a synthetic record does not count as an answer")
	}
	if r.CostUSD != nil || r.Context != nil {
		t.Errorf("a synthetic record set a figure: cost %v, context %v", r.CostUSD, r.Context)
	}
}

// [MEASURES-2-S1] An assistant line without usage fails the session, naming the
// line's number and its version, and the figures are absent, neither the last value
// nor 0. A good line appended after it does not bring them back.
func TestTranscriptLineWithoutUsageFailsAndStaysFailed(t *testing.T) {
	t.Parallel()
	configDir, main := transcriptFixture(t, "basic")
	var cur TranscriptCursor
	r := read(t, configDir, &cur)
	wantContext(t, r, basicContext)
	wantCost(t, r, basicCost)

	appendTo(t, main, `{"type":"assistant","version":"2.1.400","message":{"id":"msg_X","model":"claude-opus-5-5","content":"placeholder"}}`+"\n")
	r = read(t, configDir, &cur)
	if r.Failed == nil {
		t.Fatal("an assistant line without message.usage did not fail the reading")
	}
	if r.CostUSD != nil || r.Context != nil || r.Model != "" || r.PricedCalls != 0 {
		t.Errorf("a failed reading carries figures: cost %v, context %v, model %q", r.CostUSD, r.Context, r.Model)
	}
	f := r.Failed
	if f.Session != fixtureSession || f.Line != 8 || f.Version != "2.1.400" || f.Field != "message.usage" {
		t.Errorf("error %+v: want session %s, line 8, version 2.1.400, field message.usage", *f, fixtureSession)
	}
	for _, s := range []string{fixtureSession, "line 8", "2.1.400", "message.usage"} {
		if !strings.Contains(f.Error(), s) {
			t.Errorf("error %q does not name %q", f.Error(), s)
		}
	}

	appendTo(t, main, opusLine("msg_Y", 1000, 0, 0, 0))
	r = read(t, configDir, &cur)
	if r.Failed == nil || r.Failed.Line != 8 {
		t.Fatalf("a good line after the bad one cleared the failure: %v", r.Failed)
	}
	if r.CostUSD != nil || r.Context != nil {
		t.Errorf("figures came back after a failure: cost %v, context %v", r.CostUSD, r.Context)
	}

	// A fresh cursor, as after a restart, reads the files from the start and fails
	// on the same line again.
	var fresh TranscriptCursor
	if r := read(t, configDir, &fresh); r.Failed == nil || r.Failed.Line != 8 {
		t.Errorf("a fresh read: %v, want the failure at line 8", r.Failed)
	}
}

// D7: a complete line that is not valid JSON, an assistant line with no model, and
// an assistant line whose stage-one fields have the wrong type each fail; a
// subagent's bad line names its own file.
func TestTranscriptUnreadableLinesFail(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, line, field string
		sub               bool
	}{
		{"not JSON", `{"type":"assistant","message":` + "\n", "", false},
		{"no model", `{"type":"assistant","version":"2.1.302","message":{"id":"m","usage":{"input_tokens":1,"output_tokens":1,"cache_creation_input_tokens":1,"cache_read_input_tokens":1}}}` + "\n", "message.model", false},
		{"no message", `{"type":"assistant","version":"2.1.302"}` + "\n", "message.model", false},
		{"model not a string", `{"type":"assistant","version":"2.1.302","message":{"id":"m","model":5,"usage":{"input_tokens":1,"output_tokens":1,"cache_creation_input_tokens":1,"cache_read_input_tokens":1}}}` + "\n", "message.model", false},
		{"version not a string", `{"type":"assistant","version":2,"message":{"id":"m","model":"claude-opus-5-5","usage":{"input_tokens":1,"output_tokens":1,"cache_creation_input_tokens":1,"cache_read_input_tokens":1}}}` + "\n", "version", false},
		{"usage not an object", `{"type":"assistant","version":"2.1.302","message":{"id":"m","model":"claude-opus-5-5","usage":"x"}}` + "\n", "message.usage", false},
		{"in a subagent", `{"type":"assistant","version":"2.1.302","message":{"id":"m","model":"claude-opus-5-5"}}` + "\n", "message.usage", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			configDir, main := transcriptFixture(t, "basic")
			path := main
			if tc.sub {
				path = filepath.Join(filepath.Dir(main), fixtureSession, "subagents", "agent-a.jsonl")
			}
			appendTo(t, path, tc.line)
			var cur TranscriptCursor
			r := read(t, configDir, &cur)
			if r.Failed == nil {
				t.Fatalf("%s did not fail the reading", tc.name)
			}
			if r.Failed.Field != tc.field {
				t.Errorf("field %q, want %q (%v)", r.Failed.Field, tc.field, r.Failed)
			}
			if !strings.Contains(r.Failed.Error(), filepath.Base(path)) {
				t.Errorf("error %q does not name the file %s", r.Failed.Error(), filepath.Base(path))
			}
			if r.CostUSD != nil || r.Context != nil {
				t.Errorf("a failed reading carries figures")
			}
		})
	}
}

// [MEASURES-2-S4] Lines of other types are skipped unread: user, attachment,
// system, and a type the reader has never seen whose message is a string and whose
// version is a number. The figures come from the assistant records alone.
func TestTranscriptSkipsLinesOfOtherTypes(t *testing.T) {
	t.Parallel()
	configDir, _ := transcriptFixture(t, "skipped-types")
	var cur TranscriptCursor
	r := read(t, configDir, &cur)
	if r.Failed != nil {
		t.Fatalf("a skipped line failed the reading: %v", r.Failed)
	}
	// msg_A alone: 0.004 + 0.010 + 0.010 + 0.020.
	wantCost(t, r, 0.044)
	wantContext(t, r, 103000)
	if r.PricedCalls != 1 || len(r.Unpriced) != 0 {
		t.Errorf("priced %d, unpriced %v: want the one assistant record", r.PricedCalls, r.Unpriced)
	}
}

// [MEASURES-2-S5] A missing count is "could not read", not 0, and names the count;
// a line with no cache_creation.ephemeral_1h_input_tokens and no speed is read
// without error, as all 5-minute writes at standard speed.
func TestTranscriptMissingCountFails(t *testing.T) {
	t.Parallel()
	configDir, main := transcriptFixture(t, "basic")
	var cur TranscriptCursor
	wantCost(t, read(t, configDir, &cur), basicCost)

	// No 1-hour figure and no speed: 1000 writes at 5 minutes, $0.005.
	appendTo(t, main, opusLine("msg_F", 0, 0, 1000, 0))
	r := read(t, configDir, &cur)
	if r.Failed != nil {
		t.Fatalf("absent optional fields failed the reading: %v", r.Failed)
	}
	wantCost(t, r, basicCost+0.005)

	appendTo(t, main, `{"type":"assistant","version":"2.1.302","message":{"id":"msg_G","model":"claude-opus-5-5","usage":{"input_tokens":1,"output_tokens":1,"cache_creation_input_tokens":1}}}`+"\n")
	r = read(t, configDir, &cur)
	if r.Failed == nil {
		t.Fatal("an absent cache_read_input_tokens did not fail the reading")
	}
	if r.Failed.Field != "message.usage.cache_read_input_tokens" || !strings.Contains(r.Failed.Error(), "cache_read_input_tokens") {
		t.Errorf("error %v does not name cache_read_input_tokens", r.Failed)
	}
	if r.CostUSD != nil || r.Context != nil {
		t.Errorf("figures present after a missing count: cost %v, context %v", r.CostUSD, r.Context)
	}
}

// Every required count, absent or not a number, fails, naming itself.
func TestTranscriptEveryRequiredCountIsChecked(t *testing.T) {
	t.Parallel()
	all := map[string]string{"input_tokens": "1", "output_tokens": "1", "cache_creation_input_tokens": "1", "cache_read_input_tokens": "1"}
	if len(usageCounts) != len(all) {
		t.Fatalf("the counts table holds %d entries, D2 names %d", len(usageCounts), len(all))
	}
	for _, c := range usageCounts {
		if _, ok := all[c.Name]; !ok {
			t.Errorf("%s is not one of D2's four counts", c.Name)
		}
		if c.Optional {
			t.Errorf("%s is optional: task 1.0 has not run, so all four stay required", c.Name)
		}
		for _, bad := range []string{"", `"1"`, "null", "1.5", "-5"} {
			var parts []string
			for k, v := range all {
				if k == c.Name {
					if bad == "" {
						continue
					}
					v = bad
				}
				parts = append(parts, `"`+k+`":`+v)
			}
			line := `{"type":"assistant","version":"2.1.302","message":{"id":"m","model":"claude-opus-5-5","usage":{` + strings.Join(parts, ",") + "}}}\n"
			configDir, main := transcriptFixture(t, "skipped-types")
			appendTo(t, main, line)
			var cur TranscriptCursor
			r := read(t, configDir, &cur)
			if r.Failed == nil || r.Failed.Field != "message.usage."+c.Name {
				t.Errorf("%s as %q: failure %v, want one naming it", c.Name, bad, r.Failed)
			}
		}
	}
}

// [MEASURES-4-S3] An id that is not a UUID opens nothing and builds no path, even
// where a file it would reach exists.
func TestTranscriptRefusesAnIDThatIsNotAUUID(t *testing.T) {
	t.Parallel()
	configDir := t.TempDir()
	good := opusLine("msg_1", 1000, 0, 0, 0)
	for _, p := range []string{
		filepath.Join(configDir, "projects", "p", "abc.jsonl"),
		filepath.Join(configDir, "etc", "x.jsonl"), // projects/p/../../etc/x.jsonl
	} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		appendTo(t, p, good)
	}
	for _, id := range []string{"../../etc/x", "abc", "", fixtureSession + "/../x", strings.ToUpper(fixtureSession) + "x"} {
		var cur TranscriptCursor
		r, err := ReadTranscript(configDir, id, &cur)
		if !errors.Is(err, ErrNotUUID) {
			t.Errorf("id %q: err %v, want ErrNotUUID", id, err)
		}
		if r.Found || r.Searched != "" || r.CostUSD != nil || cur.main != "" || len(cur.files) != 0 {
			t.Errorf("id %q was read: found %v, searched %q, cost %v, cursor %+v", id, r.Found, r.Searched, r.CostUSD, cur)
		}
	}
	// The negative case: a UUID is read.
	configDir, _ = transcriptFixture(t, "basic")
	var cur TranscriptCursor
	if r := read(t, configDir, &cur); !r.Found {
		t.Error("a UUID was not read")
	}
}

// A session with no transcript yet is not an error: the reading says it was not
// found and names the folder searched, and finds it once it appears.
func TestTranscriptNotFoundYet(t *testing.T) {
	t.Parallel()
	configDir := t.TempDir()
	var cur TranscriptCursor
	r := read(t, configDir, &cur)
	if r.Found || r.AtEnd || r.Failed != nil || r.CostUSD != nil {
		t.Fatalf("no projects folder: %+v", r)
	}
	if want := filepath.Join(configDir, "projects"); r.Searched != want {
		t.Errorf("searched %q, want %q", r.Searched, want)
	}
	main := filepath.Join(configDir, "projects", "-late-repo", fixtureSession+".jsonl")
	if err := os.MkdirAll(filepath.Dir(main), 0o755); err != nil {
		t.Fatal(err)
	}
	if r := read(t, configDir, &cur); r.Found {
		t.Fatal("found a transcript in an empty folder")
	}
	appendTo(t, main, opusLine("msg_1", 1000, 0, 0, 0))
	r = read(t, configDir, &cur)
	if !r.Found || !r.AtEnd {
		t.Fatalf("found %v, at end %v", r.Found, r.AtEnd)
	}
	wantCost(t, r, 0.004)
}

// A file that shrank is read again from the start, with its totals reset (D6).
func TestTranscriptRereadsAFileThatShrank(t *testing.T) {
	t.Parallel()
	configDir, main := transcriptFixture(t, "basic")
	var cur TranscriptCursor
	wantCost(t, read(t, configDir, &cur), basicCost)
	if err := os.WriteFile(main, []byte(opusLine("msg_Z", 1000, 0, 0, 0)), 0o644); err != nil {
		t.Fatal(err)
	}
	r := read(t, configDir, &cur)
	// The main thread now holds msg_Z alone; the subagents are unchanged.
	wantCost(t, r, 0.004+0.048+0.014)
	wantContext(t, r, 1000)
}

// At most the budget is read per call; a reading that has not reached the end of
// its files says so, and later reads finish the same total a single read gives.
func TestTranscriptReadsAtMostItsBudgetPerCall(t *testing.T) {
	t.Parallel()
	if maxTranscriptRead != 32<<20 {
		t.Errorf("the budget is %d bytes, D6 sets 32 MiB", maxTranscriptRead)
	}
	configDir, _ := transcriptFixture(t, "basic")
	var cur TranscriptCursor
	var r TranscriptReading
	var err error
	reads := 0
	for {
		r, err = readTranscript(configDir, fixtureSession, &cur, 400)
		if err != nil {
			t.Fatal(err)
		}
		reads++
		if r.Failed != nil {
			t.Fatalf("a budget-cut read failed: %v", r.Failed)
		}
		if r.AtEnd || reads > 50 {
			break
		}
	}
	if reads < 3 {
		t.Errorf("finished in %d reads at 400 bytes each: the budget is not applied", reads)
	}
	wantCost(t, r, basicCost)
	wantContext(t, r, basicContext)

	// A complete line longer than the whole budget can never be read: it fails
	// rather than leaving the session short of the end forever.
	configDir, main := transcriptFixture(t, "basic")
	appendTo(t, main, `{"type":"user","version":"2.1.302","message":{"content":"`+strings.Repeat("x", 600)+`"}}`+"\n")
	cur = TranscriptCursor{}
	for i := 0; i < 50; i++ {
		if r, err = readTranscript(configDir, fixtureSession, &cur, 400); err != nil {
			t.Fatal(err)
		}
		if r.Failed != nil || r.AtEnd {
			break
		}
	}
	if r.Failed == nil {
		t.Fatalf("a line longer than the budget did not fail (at end %v)", r.AtEnd)
	}
}

// [MEASURES-3-S1] An Opus 5.5 call is priced by its parts: 1000 input, 3000 cache
// writes of which 1000 are for an hour, 100000 cache reads and 500 output cost
// $0.052: input $0.004, the 5-minute write $0.010, the 1-hour write $0.008, the read
// $0.020 and the output $0.010.
func TestTranscriptPricesAnOpusCallByItsParts(t *testing.T) {
	t.Parallel()
	u := callUsage{Input: 1000, CacheWrite: 3000, CacheWrite1h: 1000, CacheRead: 100000, Output: 500}
	for _, part := range []struct {
		name string
		u    callUsage
		want float64
	}{
		{"input", callUsage{Input: 1000}, 0.004},
		{"5-minute write", callUsage{CacheWrite: 2000}, 0.010},
		{"1-hour write", callUsage{CacheWrite: 1000, CacheWrite1h: 1000}, 0.008},
		{"read", callUsage{CacheRead: 100000}, 0.020},
		{"output", callUsage{Output: 500}, 0.010},
		{"whole call", u, 0.052},
	} {
		got, ok := priceCall("claude-opus-5-5", part.u)
		if !ok || !near(got, part.want) {
			t.Errorf("%s: $%.6f (priced %v), want $%.3f", part.name, got, ok, part.want)
		}
	}
	// The same call through the reader.
	configDir := t.TempDir()
	main := filepath.Join(configDir, "projects", "-fixture-repo", fixtureSession+".jsonl")
	if err := os.MkdirAll(filepath.Dir(main), 0o755); err != nil {
		t.Fatal(err)
	}
	appendTo(t, main, `{"type":"assistant","version":"2.1.302","message":{"id":"m","model":"claude-opus-5-5","usage":{"input_tokens":1000,"output_tokens":500,"cache_creation_input_tokens":3000,"cache_read_input_tokens":100000,"cache_creation":{"ephemeral_1h_input_tokens":1000}}}}`+"\n")
	var cur TranscriptCursor
	wantCost(t, read(t, configDir, &cur), 0.052)
}

// [MEASURES-3-S2] An unknown model is left out of the figure and counted by name;
// a session with only unknown models shows no cost, not $0.
func TestTranscriptLeavesAnUnknownModelOutAndNamesIt(t *testing.T) {
	t.Parallel()
	configDir, _ := transcriptFixture(t, "unpriced")
	var cur TranscriptCursor
	r := read(t, configDir, &cur)
	// The two Opus 5.5 calls alone: 2 x (0.004 + 0.010).
	wantCost(t, r, 0.028)
	if r.PricedCalls != 2 {
		t.Errorf("priced calls %d, want 2", r.PricedCalls)
	}
	if len(r.Unpriced) != 1 || r.Unpriced["claude-opus-5-6"] != 3 {
		t.Errorf("unpriced %v, want claude-opus-5-6: 3", r.Unpriced)
	}

	configDir, _ = transcriptFixture(t, "only-unpriced")
	cur = TranscriptCursor{}
	r = read(t, configDir, &cur)
	if r.CostUSD != nil {
		t.Errorf("a session with only unpriced calls has cost $%v, want none", *r.CostUSD)
	}
	if r.Unpriced["claude-opus-5-6"] != 3 || r.PricedCalls != 0 {
		t.Errorf("unpriced %v, priced %d", r.Unpriced, r.PricedCalls)
	}
	// Its context still reads: an unpriced model is unpriced, not unreadable.
	wantContext(t, r, 1000)
	if r.Model != "claude-opus-5-6" {
		t.Errorf("model %q", r.Model)
	}
}

// A streamed message re-booked under another model moves its call between priced
// and unpriced, across reads too.
func TestTranscriptRebooksAMessageAcrossPricedAndUnpriced(t *testing.T) {
	t.Parallel()
	configDir, main := transcriptFixture(t, "only-unpriced")
	var cur TranscriptCursor
	read(t, configDir, &cur)
	appendTo(t, main, strings.Replace(opusLine("msg_5", 1000, 0, 0, 0), "claude-opus-5-5", "claude-opus-5-6", 1))
	if r := read(t, configDir, &cur); r.Unpriced["claude-opus-5-6"] != 3 {
		t.Fatalf("a repeated line of msg_5 was counted again: %v", r.Unpriced)
	}
	appendTo(t, main, opusLine("msg_5", 1000, 0, 0, 0))
	r := read(t, configDir, &cur)
	if r.Unpriced["claude-opus-5-6"] != 2 || r.PricedCalls != 1 {
		t.Errorf("unpriced %v, priced %d: msg_5's last line is Opus 5.5", r.Unpriced, r.PricedCalls)
	}
	wantCost(t, r, 0.004)
}

// [MEASURES-3-S3] A dated or tagged id is priced as its version, fast mode at the
// fast multiplier where the table has one, and fast mode on a model without one is
// unpriced.
func TestTranscriptPricesDatedTaggedAndFastIDs(t *testing.T) {
	t.Parallel()
	configDir, _ := transcriptFixture(t, "model-ids")
	var cur TranscriptCursor
	r := read(t, configDir, &cur)
	// 1M input each: the dated id $4, the [1m] id $4, fast Opus 5.5 $8.
	wantCost(t, r, 16)
	if r.PricedCalls != 3 {
		t.Errorf("priced calls %d, want 3", r.PricedCalls)
	}
	if len(r.Unpriced) != 1 || r.Unpriced["claude-sonnet-5-5 (fast)"] != 1 {
		t.Errorf("unpriced %v, want fast claude-sonnet-5-5 alone", r.Unpriced)
	}

	for _, tc := range []struct {
		model string
		fast  bool
		want  float64 // per 1M input; 0 is unpriced
	}{
		{"claude-opus-5-5", false, 4},
		{"claude-opus-5-5-20261001", false, 4},
		{"claude-opus-5-5[1m]", false, 4},
		{"claude-opus-5-5-20261001[1m]", false, 4},
		{"claude-opus-5-5", true, 8},
		{"claude-sonnet-5-5", false, 2},
		{"claude-sonnet-5-5", true, 0},
		{"claude-opus-5-6", false, 0}, // an exact version only
		{"claude-opus-5", false, 0},
		{"claude-opus-5-5-preview", false, 0},
		{"claude-opus-5-5-2026", false, 0},
		{"<synthetic>", false, 0},
	} {
		u := callUsage{Input: 1000000}
		if tc.fast {
			u.Speed = "fast"
		}
		got, ok := priceCall(tc.model, u)
		if ok != (tc.want != 0) || !near(got, tc.want) {
			t.Errorf("%s (fast %v): $%v priced %v, want $%v", tc.model, tc.fast, got, ok, tc.want)
		}
	}
	// Fast mode multiplies every part, caching included.
	if got, _ := priceCall("claude-opus-5-5", callUsage{CacheRead: 1000000, CacheWrite: 1000000, Speed: "fast"}); !near(got, 2*(0.20+5)) {
		t.Errorf("fast cache read and write: $%v, want $%v", got, 2*(0.20+5))
	}
}

// Haiku 5.5's rates change with the prompt's length: over 100,000 tokens, input,
// cache reads and writes all counted, the higher row applies to the whole call.
func TestTranscriptPricesHaikuByPromptLength(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		u    callUsage
		want float64
	}{
		{callUsage{Input: 100000}, 0.01}, // at the threshold: $0.10/MTok
		{callUsage{Input: 1000, CacheRead: 99000, Output: 1000}, 0.0001 + 0.00099 + 0.0005}, // 100,000 in all: the lower row
		{callUsage{Input: 1000, CacheRead: 100000}, 0.0000005*1000 + 0.00000005*100000},     // over: $0.50 and $0.05
		{callUsage{Input: 1, CacheWrite: 100000, CacheWrite1h: 100000}, 0.0000005 + 0.000001*100000},
		{callUsage{Input: 50000, Output: 1000000, Speed: "fast"}, 0}, // no fast rate
	} {
		got, ok := priceCall("claude-haiku-5-5", tc.u)
		if ok != (tc.want != 0) || !near(got, tc.want) {
			t.Errorf("%+v: $%v priced %v, want $%v", tc.u, got, ok, tc.want)
		}
	}
}

// Every row of the price table names its source and the date it was read.
func TestTranscriptPriceRowsCiteTheirSource(t *testing.T) {
	t.Parallel()
	if len(modelPrices) == 0 {
		t.Fatal("the price table is empty")
	}
	for model, p := range modelPrices {
		if !strings.HasPrefix(p.Source, "https://") || !strings.Contains(p.Source, "2026-") {
			t.Errorf("%s: source %q names no URL and date", model, p.Source)
		}
		if p.Input <= 0 || p.Output <= 0 || p.CacheRead <= 0 {
			t.Errorf("%s: a rate is not set: %+v", model, p)
		}
	}
	for model := range fastMultiplier {
		if _, ok := modelPrices[model]; !ok {
			t.Errorf("fast multiplier for %s, which has no price row", model)
		}
	}
}
