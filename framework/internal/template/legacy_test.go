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

// Install, update and survey place the panel file where the project holds it: a project with
// only the legacy name's panel.json has one, and is not given a second the panel would read.
// Every other file stays where the mapping put it.
func TestProjectDestIsTheLegacyPanelFileTheProjectHolds(t *testing.T) {
	dir := t.TempDir()
	names := []string{"newname", "clauductor"}
	p := filepath.Join(dir, ".clauductor", "panel.json")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := projectDest(dir, PanelPath, PanelPath, names); got != ".clauductor/panel.json" {
		t.Fatalf("panel file: got %s, want the legacy .clauductor/panel.json", got)
	}
	if got := projectDest(dir, "AGENTS.md", "docs/AGENTS.md", names); got != "docs/AGENTS.md" {
		t.Fatalf("another file: got %s, want where the mapping put it", got)
	}
}

// After the rename (OPS-104), PanelPath is the new name's, and so is what is read when
// nothing is there.
func TestPanelPathIsTheNewNames(t *testing.T) {
	if PanelPath != ".stridal/panel.json" || ProjectPanelPath(t.TempDir()) != PanelPath {
		t.Fatalf("PanelPath = %q", PanelPath)
	}
}
