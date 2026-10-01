package cmd

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runUpdate(t *testing.T, dir string, dry bool) error {
	t.Helper()
	updateDryRun = dry
	t.Cleanup(func() { updateDryRun, forceUpdate = false, false })
	t.Chdir(dir)
	return updateCmd.RunE(updateCmd, nil)
}

// stdoutOf runs f with os.Stdout captured (update prints with fmt.Printf).
func stdoutOf(t *testing.T, f func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	done := make(chan string)
	go func() { b, _ := io.ReadAll(r); done <- string(b) }()
	defer func() { os.Stdout = old }()
	f()
	w.Close()
	os.Stdout = old
	return <-done
}

// OPS-20 review: update never writes doc-tier files, so it NAMES the guidance docs that differ
// (or are missing), and diff notes each; a project's own records (the roadmap) are never named.
func TestUpdateNamesChangedGuidanceDocs(t *testing.T) {
	tmplDir(t)
	dir := ownedRepo(t)
	if out, err := runInstall(t, dir, false); err != nil {
		t.Fatalf("install: %v\n%s", err, out)
	}
	quiet := stdoutOf(t, func() {
		if err := runUpdate(t, dir, true); err != nil {
			t.Fatal(err)
		}
	})
	if strings.Contains(quiet, "Docs —") {
		t.Errorf("a project whose docs match the template got a docs notice:\n%s", quiet)
	}

	write(t, dir, "docs/playbook.md", "# Our playbook, edited before the machine-setup section\n")
	write(t, dir, "docs/roadmap.md", "# Our roadmap\n")
	if err := os.Remove(filepath.Join(dir, "docs", "principles.md")); err != nil {
		t.Fatal(err)
	}
	out := stdoutOf(t, func() {
		if err := runUpdate(t, dir, true); err != nil {
			t.Fatal(err)
		}
	})
	for _, want := range []string{"Docs —", "~ docs/playbook.md", "- docs/principles.md (missing here)", "clauductor diff"} {
		if !strings.Contains(out, want) {
			t.Errorf("update's notice lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "docs/roadmap.md") {
		t.Errorf("update named the project's own roadmap as drift:\n%s", out)
	}

	dout, _ := runDiff(t, dir)
	if !strings.Contains(dout, "docs/playbook.md") || !strings.Contains(dout, "differs from the template's copy") {
		t.Errorf("diff does not note the changed playbook:\n%s", dout)
	}
}

// OPS-20: the line-ending rules reach every project: a fresh install creates .gitattributes, an
// install over one merges the rules in ahead of the project's own lines, and update, which copies
// only framework files, merges them into a project installed before they shipped.
func TestGitattributesReachEveryProject(t *testing.T) {
	tmpl := tmplDir(t)
	want, err := os.ReadFile(filepath.Join(tmpl, ".gitattributes"))
	if err != nil {
		t.Fatal(err)
	}

	fresh := ownedRepo(t)
	if out, err := runInstall(t, fresh, false); err != nil {
		t.Fatalf("install: %v\n%s", err, out)
	}
	if got, _ := os.ReadFile(filepath.Join(fresh, ".gitattributes")); string(got) != string(want) {
		t.Errorf("a fresh install should create the template's .gitattributes; got:\n%s", got)
	}

	own := ownedRepo(t)
	write(t, own, ".gitattributes", "*.bat text eol=crlf\n")
	if out, err := runInstall(t, own, false); err != nil {
		t.Fatalf("install: %v\n%s", err, out)
	}
	got, _ := os.ReadFile(filepath.Join(own, ".gitattributes"))
	rule, mine := strings.Index(string(got), "* text=auto eol=lf"), strings.Index(string(got), "*.bat text eol=crlf")
	if rule < 0 || mine < 0 || rule > mine {
		t.Errorf("install should merge the LF rule in AHEAD of the project's own line:\n%s", got)
	}

	// A project installed before the rules shipped: everything current but .gitattributes.
	old := ownedRepo(t)
	if out, err := runInstall(t, old, false); err != nil {
		t.Fatalf("install: %v\n%s", err, out)
	}
	write(t, old, ".gitattributes", "*.bat text eol=crlf\n")
	if err := runUpdate(t, old, true); err != nil {
		t.Fatalf("update --dry-run: %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(old, ".gitattributes")); string(got) != "*.bat text eol=crlf\n" {
		t.Errorf("update --dry-run wrote .gitattributes:\n%s", got)
	}
	if err := runUpdate(t, old, false); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ = os.ReadFile(filepath.Join(old, ".gitattributes"))
	rule, mine = strings.Index(string(got), "* text=auto eol=lf"), strings.Index(string(got), "*.bat text eol=crlf")
	if rule < 0 || mine < 0 || rule > mine {
		t.Errorf("update should merge the LF rule in ahead of the project's own line:\n%s", got)
	}
	if err := runUpdate(t, old, false); err != nil {
		t.Fatalf("second update: %v", err)
	}
	if again, _ := os.ReadFile(filepath.Join(old, ".gitattributes")); string(again) != string(got) {
		t.Errorf("a second update changed .gitattributes again:\n%s", again)
	}

	// Removed outright: update creates it again.
	if err := os.Remove(filepath.Join(old, ".gitattributes")); err != nil {
		t.Fatal(err)
	}
	if err := runUpdate(t, old, false); err != nil {
		t.Fatalf("update: %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(old, ".gitattributes")); string(got) != string(want) {
		t.Errorf("update should create a missing .gitattributes; got:\n%s", got)
	}
}
