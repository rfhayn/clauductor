package panel

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// This repository is public. Recorded fixtures keep their payload shapes, but no
// user name, machine path or real session id (review round 3, 2026-09-29). A new
// recording must be scrubbed before it is committed: this test fails until it is.

// syntheticIDs are the only UUIDs a fixture may contain.
var syntheticIDs = map[string]bool{}

func init() {
	for i := 1; i <= 16; i++ {
		syntheticIDs[syntheticID(i)] = true
	}
}

func syntheticID(i int) string {
	s := "000000000000" + strconv.Itoa(i)
	return "00000000-0000-4000-8000-" + s[len(s)-12:]
}

var uuidAnyRe = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)

// fixtureLeaks returns what in b must not be in a public fixture.
func fixtureLeaks(b []byte) []string {
	var bad []string
	s := string(b)
	for _, f := range []string{"/Users/", "/private/tmp/claude-"} {
		if strings.Contains(s, f) {
			bad = append(bad, "contains "+f)
		}
	}
	for _, u := range uuidAnyRe.FindAllString(s, -1) {
		if !syntheticIDs[strings.ToLower(u)] {
			bad = append(bad, "real-looking id "+u)
		}
	}
	return bad
}

func TestFixturesAreScrubbed(t *testing.T) {
	n := 0
	// Every testdata directory of the panel and its packages: the tree is the
	// authority, so a package that adds fixtures is scanned without a list to update.
	err := filepath.WalkDir(".", func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if !strings.Contains(string(filepath.Separator)+p, string(filepath.Separator)+"testdata"+string(filepath.Separator)) {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		n++
		for _, leak := range fixtureLeaks(b) {
			t.Errorf("%s: %s: scrub it (synthetic paths, ids from syntheticID)", p, leak)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if n < 5 {
		t.Fatalf("scanned %d fixtures; the walk is not seeing testdata", n)
	}
	// The guard catches what it is for.
	for _, leak := range []string{`{"cwd":"/Users/someone/x"}`, `/private/tmp/claude-501/x`, `"session_id":"7ffb1122-75ee-4e10-9570-3f84b1d64288"`} {
		if len(fixtureLeaks([]byte(leak))) == 0 {
			t.Errorf("the guard misses %s", leak)
		}
	}
}
