package signals

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for p, body := range files {
		full := filepath.Join(root, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// PANEL-19: a proposal's Approved and Budget lines, as OPS-7 writes them.
func TestParseProposal(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		in       string
		approved bool
		budget   float64 // -1: none
	}{
		{"# Add x\n\n**Approved:** 2026-09-30 by owner\n**Budget:** $50\n", true, 50},
		{"# Add x\n\n- **Budget:** $1,250.50\n", false, 1250.5},
		{"**Approved:**\n", false, -1},             // an empty Approved line approves nothing
		{"Approved: 2026-09-30\n", false, -1},      // only the bold form counts
		{"**Budget:** fifty dollars\n", false, -1}, // no number, no budget
		{"text **Approved:** inline is not a line\n", false, -1},
		{"**approved:** 2026-10-01 by rich\n**budget:** US$ 12\n", true, 12},
	} {
		a, b := ParseProposal([]byte(c.in))
		if a != c.approved || (c.budget < 0) != (b == nil) || (b != nil && *b != c.budget) {
			t.Errorf("%q: approved %v budget %v", c.in, a, b)
		}
	}
}

// The change directory is CHANGES_DIR from project.conf when it names one inside the
// repository, and openspec/changes too.
func TestChangesDirs(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if got := ChangesDirs(root); !reflect.DeepEqual(got, []string{"changes", "openspec/changes"}) {
		t.Fatalf("default %v", got)
	}
	writeTree(t, root, map[string]string{".claude/project.conf": "# x\nPROPOSALS=\"markdown\"\nCHANGES_DIR=\"docs/changes\"  # where\n"})
	if got := ChangesDirs(root); !reflect.DeepEqual(got, []string{"docs/changes", "openspec/changes"}) {
		t.Fatalf("configured %v", got)
	}
	writeTree(t, root, map[string]string{".claude/project.conf": "CHANGES_DIR=\"../elsewhere\"\n"})
	if got := ChangesDirs(root); got[0] != "changes" {
		t.Fatalf("a directory outside the repository was taken: %v", got)
	}
}

// Every worktree's changes are read; the archive is not; a change in two worktrees is
// the copy written last.
func TestReadChanges(t *testing.T) {
	t.Parallel()
	main, lane := t.TempDir(), t.TempDir()
	writeTree(t, main, map[string]string{
		"changes/add-x/proposal.md":                  "**Approved:** 2026-09-01 by owner\n**Budget:** $10\n",
		"changes/archive/2026-09-01-old/proposal.md": "old\n",
		"changes/README.md":                          "not a change\n",
		"openspec/changes/os-change/proposal.md":     "no approval\n",
	})
	writeTree(t, lane, map[string]string{"changes/add-x/proposal.md": "rewritten, not approved\n", "changes/add-y/design.md": "no proposal\n"})
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(filepath.Join(main, "changes/add-x/proposal.md"), old, old); err != nil {
		t.Fatal(err)
	}
	cs := ReadChanges([]string{main, lane}, []string{"changes", "openspec/changes"})
	if len(cs) != 2 || cs[0].ID != "add-x" || cs[1].ID != "os-change" {
		t.Fatalf("changes %+v", cs)
	}
	if cs[0].Worktree != lane || cs[0].Approved || cs[0].BudgetUSD != nil {
		t.Fatalf("the newer copy of add-x should win: %+v", cs[0])
	}
	if cs[1].Approved || cs[1].Written.IsZero() {
		t.Fatalf("os-change %+v", cs[1])
	}
}

// PANEL-20: tasks.md checkboxes, the gate's receipt and GitHub's review threads.
func TestReadinessParsers(t *testing.T) {
	t.Parallel()
	if o, d := CountTasks([]byte("## Tasks\n- [x] a\n  - [ ] b\n* [X] c\n- [] not a box\ntext [ ] no\n")); o != 1 || d != 2 {
		t.Errorf("tasks %d open %d done", o, d)
	}
	if r, ok := ParseReceipt([]byte("0581df54c2aa\tfull\tclean\tall\n")); !ok || r.SHA != "0581df54c2aa" || !r.Clean || r.Kind != "full" {
		t.Errorf("receipt %+v %v", r, ok)
	}
	if r, ok := ParseReceipt([]byte("0581df54c2aa\tfull\tdirty\tall")); !ok || r.Clean {
		t.Errorf("a dirty receipt %+v", r)
	}
	for _, bad := range []string{"", "not-a-sha\tfull\tclean", "abc\tfull"} {
		if _, ok := ParseReceipt([]byte(bad)); ok {
			t.Errorf("accepted receipt %q", bad)
		}
	}
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"ci-receipt": "0581df54c2aa\tfull\tclean\tall\n"})
	if r, ok := ReadReceipt(dir); !ok || r.SHA != "0581df54c2aa" {
		t.Errorf("read receipt %+v", r)
	}
	if _, ok := ReadReceipt(t.TempDir()); ok {
		t.Error("a receipt from nowhere")
	}
	u, n, err := ParseReviewThreads([]byte(`{"data":{"repository":{"pullRequest":{"reviewThreads":{"totalCount":3,"nodes":[{"isResolved":true},{"isResolved":false},{"isResolved":false}]}}}}}`))
	if err != nil || u != 2 || n != 3 {
		t.Errorf("threads %d of %d %v", u, n, err)
	}
	if _, _, err := ParseReviewThreads([]byte("oops")); err == nil {
		t.Error("bad graphql accepted")
	}
	if a := ReviewThreadsArgv(7); a[0] != "gh" || a[len(a)-1][:6] != "query=" {
		t.Errorf("argv %q", a)
	}
}

func TestBranchOfChange(t *testing.T) {
	t.Parallel()
	for branch, want := range map[string]bool{"change/add-x": true, "feature/team/add-x": true, "add-x": true, "change/add-xy": false, "change/x-add-x": false, "": false} {
		if got := BranchOfChange(branch, "add-x"); got != want {
			t.Errorf("%q: %v", branch, got)
		}
	}
}
