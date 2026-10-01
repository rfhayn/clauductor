package template

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeTemplate is a directory that passes isTemplate, with the given version marker ("" = none).
func fakeTemplate(t *testing.T, dir, version string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".claude", "lib"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".claude", "lib", "conf.sh"), []byte("MAIN_BRANCH=\"main\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if version != "" {
		if err := os.WriteFile(filepath.Join(dir, VersionFile), []byte(version+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func withVersion(t *testing.T, v string) {
	t.Helper()
	old, oldOverride := BinaryVersion, OverrideDir
	BinaryVersion, OverrideDir = v, ""
	t.Cleanup(func() { BinaryVersion, OverrideDir = old, oldOverride })
}

// OPS-8 rehearsal, finding 3: with CLAUDUCTOR_FRAMEWORK unset, the binary took its template from
// ~/Development/clauductor, a checkout on whatever branch. Now that guess is never consulted: a
// HOME holding a decoy checkout there does not decide the template.
func TestResolveNeverGuessesFromHome(t *testing.T) {
	withVersion(t, "0.1.0")
	home := t.TempDir()
	decoy := fakeTemplate(t, filepath.Join(home, "Development", "clauductor", "template"), "0.1.0")
	t.Setenv("HOME", home)
	t.Setenv("CLAUDUCTOR_FRAMEWORK", "")
	t.Setenv("CLAUDUCTOR_SHARE", filepath.Join(home, "share"))
	s, err := Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if s.Dir == decoy {
		t.Fatalf("Resolve took the ambient ~/Development/clauductor template %s", decoy)
	}
	want, _ := filepath.Abs(filepath.Join(repoRoot(t), "template"))
	if got, _ := filepath.Abs(s.Dir); got != want || s.How != "the source checkout this binary was built from" {
		t.Fatalf("want the checkout the binary was built from (%s); got %s (%s)", want, s.Dir, s.How)
	}
}

// The release layout (REL-1): install.sh unpacks a release to <share>/<version>/ with template/
// beside the binary. A share holding this version's template is used.
func TestResolveReleaseShare(t *testing.T) {
	withVersion(t, "v9.8.7")
	share := t.TempDir()
	want := fakeTemplate(t, filepath.Join(share, "v9.8.7", "template"), "9.8.7")
	t.Setenv("CLAUDUCTOR_FRAMEWORK", "")
	t.Setenv("CLAUDUCTOR_SHARE", share)
	s, err := Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if s.Dir != want || s.How != "the release share" {
		t.Fatalf("want the release share's template %s; got %s (%s)", want, s.Dir, s.How)
	}
}

// A template whose version marker is not the binary's (or that has none) is refused, naming both
// versions; --template-dir uses it anyway, with a warning.
func TestResolveRefusesAVersionMismatch(t *testing.T) {
	withVersion(t, "0.2.0")
	root := t.TempDir()
	fakeTemplate(t, filepath.Join(root, "template"), "0.1.0")
	t.Setenv("CLAUDUCTOR_FRAMEWORK", root)
	_, err := Resolve()
	if err == nil || !strings.Contains(err.Error(), "version 0.1.0") || !strings.Contains(err.Error(), "v0.2.0") {
		t.Fatalf("want a refusal naming both versions; got %v", err)
	}

	unmarked := t.TempDir()
	fakeTemplate(t, filepath.Join(unmarked, "template"), "")
	t.Setenv("CLAUDUCTOR_FRAMEWORK", unmarked)
	if _, err := Resolve(); err == nil || !strings.Contains(err.Error(), "no "+VersionFile) {
		t.Fatalf("want a refusal of a template with no marker; got %v", err)
	}

	OverrideDir = root
	s, err := Resolve()
	if err != nil {
		t.Fatalf("--template-dir should be used despite the mismatch: %v", err)
	}
	if s.How != "--template-dir" || !strings.Contains(s.Warning, "version 0.1.0") {
		t.Fatalf("want a warning from --template-dir; got %+v", s)
	}
}

// The marker is not a template file: no project receives it.
func TestVersionFileIsNotATemplateFile(t *testing.T) {
	files, err := listDir(filepath.Join(repoRoot(t), "template"))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if f == VersionFile {
			t.Fatalf("%s is listed as a template file; install would copy it into projects", VersionFile)
		}
	}
	if _, err := os.Stat(filepath.Join(repoRoot(t), "template", VersionFile)); err != nil {
		t.Fatalf("the template has no %s: %v", VersionFile, err)
	}
}
