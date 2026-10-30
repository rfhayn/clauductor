// An external test package, as main_test.go is: testgit imports treesnap, so the internal form
// could not import testgit, and its package-level code could run before testgit's strip.
package treesnap_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rfhayn/stridal/internal/treesnap"
)

// Each kind of write the D8 proof must catch shows in a diff: a file rewritten with
// the same bytes (only its mtime moves), a new file, a removed one, a mode change and
// a changed symlink; and an untouched tree diffs empty.
func TestDiffNamesEveryChange(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("same.txt", "same")
	write("mode.sh", "#!/bin/sh\n")
	write("gone.txt", "bye")
	write(".git/index", "idx")
	if err := os.Symlink("same.txt", filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	before := treesnap.Take(t, root)
	if d := treesnap.Diff(before, treesnap.Take(t, root)); len(d) != 0 {
		t.Fatalf("an untouched tree diffs: %v", d)
	}
	if before[".git/index"].SHA256 == "" || before["."].Mode&os.ModeDir == 0 {
		t.Fatalf(".git or the root is not recorded: %+v", before)
	}

	later := time.Now().Add(2 * time.Second)
	write(".git/index", "idx") // the same bytes, rewritten
	if err := os.Chtimes(filepath.Join(root, ".git", "index"), later, later); err != nil {
		t.Fatal(err)
	}
	write("new.txt", "hi")
	if err := os.Remove(filepath.Join(root, "gone.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(root, "mode.sh"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("new.txt", filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	d := strings.Join(treesnap.Diff(before, treesnap.Take(t, root)), "\n")
	for _, want := range []string{"changed .git/index: [mtime]", "added new.txt", "removed gone.txt", "changed mode.sh: [mode", "changed link:"} {
		if !strings.Contains(d, want) {
			t.Errorf("the diff does not name %q:\n%s", want, d)
		}
	}
	if strings.Contains(d, "same.txt") {
		t.Errorf("an untouched file shows:\n%s", d)
	}
}
