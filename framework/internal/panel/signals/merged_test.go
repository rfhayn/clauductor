package signals

import (
	"encoding/json"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"
)

// merged-real.json is recorded from a real `gh pr list --state merged --limit 5
// --search merged:>=2026-07-01 --json number,title,headRefName,createdAt,mergedAt`
// (rfhayn/clauductor, gh 2.x): keys sorted, times RFC 3339 in UTC. The parser reads
// that shape, and MergedArgv asks for exactly the fields the recording has.
func TestParseMergedPRsRealShape(t *testing.T) {
	t.Parallel()
	b := fixture(t, "merged-real.json")
	prs, err := ParseMergedPRs(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(prs) != 5 {
		t.Fatalf("got %d, want 5", len(prs))
	}
	p := prs[0]
	if p.Number != 24 || p.Branch != "feature/OPS-11-plugin" || !strings.HasPrefix(p.Title, "OPS-11:") ||
		!p.MergedAt.Equal(time.Date(2026, 9, 30, 23, 36, 5, 0, time.UTC)) || !p.CreatedAt.Equal(time.Date(2026, 9, 30, 23, 32, 44, 0, time.UTC)) {
		t.Fatalf("first row %+v", p)
	}
	for _, p := range prs {
		if p.MergedAt.Before(p.CreatedAt) || p.CreatedAt.IsZero() {
			t.Fatalf("row %+v", p)
		}
	}

	var rows []map[string]any
	if err := json.Unmarshal(b, &rows); err != nil {
		t.Fatal(err)
	}
	var keys []string
	for k := range rows[0] {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	argv := MergedArgv("2026-07-01")
	i := slices.Index(argv, "--json")
	if i < 0 || i+1 >= len(argv) {
		t.Fatalf("no --json in %v", argv)
	}
	asked := strings.Split(argv[i+1], ",")
	sort.Strings(asked)
	if !slices.Equal(asked, keys) {
		t.Fatalf("MergedArgv asks for %v; the recording has %v", asked, keys)
	}
	if !slices.Contains(argv, "merged") || !slices.Contains(argv, "merged:>=2026-07-01") {
		t.Fatalf("argv %v", argv)
	}
}

// An empty list is no merges, not an error; gh's own error text is not JSON.
func TestParseMergedPRsEdges(t *testing.T) {
	t.Parallel()
	if prs, err := ParseMergedPRs([]byte("[]\n")); err != nil || len(prs) != 0 {
		t.Fatalf("empty: %v %v", prs, err)
	}
	if prs, err := ParseMergedPRs([]byte(`[{"number":1,"createdAt":"2026-09-01T00:00:00Z","mergedAt":null}]`)); err != nil || len(prs) != 0 {
		t.Fatalf("a row without a merge time is skipped: %v %v", prs, err)
	}
	if _, err := ParseMergedPRs([]byte("To get started with GitHub CLI, please run:  gh auth login")); err == nil {
		t.Fatal("non-JSON output parsed")
	}
}
