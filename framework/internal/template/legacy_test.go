package template

import (
	"os"
	"path/filepath"
	"testing"
)

// After the rename (OPS-104), update checks the panel.json the project holds: the legacy
// name's while only that one exists, the new name's once it does.
func TestProjectPanelPathReadsTheLegacyNameFirst(t *testing.T) {
	dir := t.TempDir()
	names := []string{"newname", "clauductor"}
	if got := projectPanelPath(dir, names); got != ".newname/panel.json" {
		t.Fatalf("nothing there: got %s, want the new .newname/panel.json", got)
	}
	put := func(rel string) {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	put(".clauductor/panel.json")
	if got := projectPanelPath(dir, names); got != ".clauductor/panel.json" {
		t.Fatalf("only the legacy one: got %s", got)
	}
	put(".newname/panel.json")
	if got := projectPanelPath(dir, names); got != ".newname/panel.json" {
		t.Fatalf("both: got %s, want the new one", got)
	}
}

// While the names are one, PanelPath is the path it always was, and so is what is read.
func TestPanelPathIsUnchangedWhileTheNameIs(t *testing.T) {
	if PanelPath != ".clauductor/panel.json" || ProjectPanelPath(t.TempDir()) != PanelPath {
		t.Fatalf("PanelPath = %q", PanelPath)
	}
}
