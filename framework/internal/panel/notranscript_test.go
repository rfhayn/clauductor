package panel

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// The panel must never read transcripts: they are documented as unstable between
// Claude Code versions, and reading them live is what made the old control room
// expensive. This scans every shipped source file of the package (tests excluded,
// since they must name what they forbid) for the ways a transcript would be reached.
func TestNoSourceReadsTranscripts(t *testing.T) {
	forbidden := []string{".jsonl", "transcript_path", "TranscriptPath", "/.claude/projects"}
	var scanned int
	err := filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, "_test.go") || !(strings.HasSuffix(path, ".go") || strings.HasSuffix(path, ".html")) {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		scanned++
		for _, f := range forbidden {
			if strings.Contains(string(b), f) {
				t.Errorf("%s mentions %q: the panel must not read transcripts", path, f)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if scanned < 6 {
		t.Fatalf("scanned only %d files; the walk is not seeing the package", scanned)
	}
	// And the decoded payload types cannot carry a transcript path in memory.
	for _, typ := range []reflect.Type{reflect.TypeOf(HookEvent{}), reflect.TypeOf(StatusPayload{})} {
		for i := 0; i < typ.NumField(); i++ {
			if strings.Contains(strings.ToLower(typ.Field(i).Tag.Get("json")), "transcript") {
				t.Errorf("%s.%s decodes a transcript field", typ.Name(), typ.Field(i).Name)
			}
		}
	}
}
