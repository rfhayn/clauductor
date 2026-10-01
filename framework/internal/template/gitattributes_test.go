package template

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const wantAttrs = "# rules\n* text=auto eol=lf\n*.png binary\n"

// The merge keeps the project's lines AFTER the template's, so git still applies the project's
// override (the last matching line wins); asserted through git itself, not by reading the text.
func TestMergeGitattributesKeepsTheProjectsOverrides(t *testing.T) {
	have := "*.bat text eol=crlf\n*.png binary\n"
	merged, added := MergeGitattributes(have, wantAttrs)
	if strings.Join(added, "|") != "* text=auto eol=lf" {
		t.Fatalf("added %q, want only the missing rule", added)
	}
	if !strings.HasSuffix(merged, have) {
		t.Errorf("the project's lines must stay intact at the end:\n%s", merged)
	}
	if again, more := MergeGitattributes(merged, wantAttrs); again != merged || len(more) != 0 {
		t.Errorf("a second merge changed the file (added %q)", more)
	}

	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Skipf("git init: %v %s", err, out)
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitattributes"), []byte(merged), 0o644); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{"run.sh": "lf", "a/b/hook.sh": "lf", "win.bat": "crlf"} {
		out, err := exec.Command("git", "-C", dir, "check-attr", "eol", "--", path).Output()
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.TrimSpace(string(out)); !strings.HasSuffix(got, "eol: "+want) {
			t.Errorf("%s: git reads %q, want eol %s", path, got, want)
		}
	}
}

func TestMergeGitattributesEmptyAndComplete(t *testing.T) {
	if merged, added := MergeGitattributes("", wantAttrs); merged != wantAttrs || len(added) != 2 {
		t.Errorf("an empty file should become the template's whole; got %q, added %q", merged, added)
	}
	if merged, added := MergeGitattributes(wantAttrs, wantAttrs); merged != wantAttrs || added != nil {
		t.Errorf("a complete file should be left alone; added %q", added)
	}
}

// The shipped file carries the LF rule, and the tier merges it (install and update both carry it).
func TestTemplateShipsTheLFRule(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "template", GitattributesPath))
	if err != nil {
		t.Fatal(err)
	}
	if !hasLine(string(b), "* text=auto eol=lf") {
		t.Errorf("template/%s lacks '* text=auto eol=lf':\n%s", GitattributesPath, b)
	}
	if Classify(GitattributesPath) != TierConfig {
		t.Errorf("%s is tier %s, want config (merged)", GitattributesPath, TierLabel(Classify(GitattributesPath)))
	}
}
