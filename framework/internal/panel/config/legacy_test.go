package config

import (
	"os"
	"path/filepath"
	"testing"
)

// After the rename (OPS-104), a machine and a project set up under the legacy name keep their
// files where they are until they migrate. Each reader here reads the legacy name's file while
// only that one exists, and the new name's once it does.
var renamed = []string{"newname", "clauductor"}

func TestPanelDirReadsTheLegacyNameFirst(t *testing.T) {
	home := t.TempDir()
	legacyThenNew(t, panelDir(home, renamed),
		filepath.Join(home, ".newname", "panel"), filepath.Join(home, ".clauductor", "panel"),
		func() string { return panelDir(home, renamed) })
}

func TestSidecarPathReadsTheLegacyNameFirst(t *testing.T) {
	home := t.TempDir()
	legacyThenNew(t, sidecarPath(home, "app", renamed),
		filepath.Join(home, ".newname", "projects", "app", "panel.json"),
		filepath.Join(home, ".clauductor", "projects", "app", "panel.json"),
		func() string { return sidecarPath(home, "app", renamed) })
}

func TestOwnConfigPathReadsTheLegacyNameFirst(t *testing.T) {
	root := t.TempDir()
	legacyThenNew(t, ownConfigPath(root, renamed),
		filepath.Join(root, ".newname", "panel.json"), filepath.Join(root, ".clauductor", "panel.json"),
		func() string { return ownConfigPath(root, renamed) })
}

// While the names are one, each reader is the path it always was.
func TestReadersAreUnchangedWhileTheNameIs(t *testing.T) {
	home := t.TempDir()
	for _, c := range []struct{ name, got, want string }{
		{"PanelDir", PanelDir(home), filepath.Join(home, ".clauductor", "panel")},
		{"SidecarPath", SidecarPath(home, "app"), filepath.Join(home, ".clauductor", "projects", "app", "panel.json")},
		{"OwnConfigPath", OwnConfigPath(home), filepath.Join(home, DefaultConfigRel)},
		{"ConfigPath", ProjectEntry{Root: home}.ConfigPath(), filepath.Join(home, DefaultConfigRel)},
	} {
		if c.got != c.want {
			t.Errorf("%s = %s, want %s", c.name, c.got, c.want)
		}
	}
}

// legacyThenNew: with nothing on disk the reader names the new path; with only the legacy
// path it reads that; with both, the new one.
func legacyThenNew(t *testing.T, first, newP, legacy string, read func() string) {
	t.Helper()
	if first != newP {
		t.Fatalf("nothing there: got %s, want the new %s", first, newP)
	}
	touch(t, legacy)
	if got := read(); got != legacy {
		t.Fatalf("only the legacy one: got %s, want %s", got, legacy)
	}
	touch(t, newP)
	if got := read(); got != newP {
		t.Fatalf("both: got %s, want the new %s", got, newP)
	}
}

func touch(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if filepath.Ext(p) == "" {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
		return
	}
	if err := os.WriteFile(p, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}
