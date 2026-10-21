package config

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"testing"

	"github.com/clauductor/clauductor/internal/brand"
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

// The config the panel loads, not only the path ownConfigPath (R1a) names: a project set up
// under the legacy name keeps its panel.json at that name's location. With only that file
// there, LoadConfig reads it, rather than reporting it missing at the new name's; once the
// new name's exists, that one is read.
func TestProjectConfigIsReadFromTheLegacyNameFirst(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, ".clauductor", "panel.json")
	newP := filepath.Join(root, ".newname", "panel.json")
	write := func(p, name string) {
		t.Helper()
		touch(t, p)
		if err := os.WriteFile(p, []byte(`{"name":"`+name+`","version":7,"lanes":{"main":"orchestrator"}}`), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(legacy, "Legacy")
	cfg, err := LoadConfig(ownConfigPath(root, renamed))
	if err != nil {
		t.Fatalf("only the legacy one: %v", err)
	}
	if cfg.Name != "Legacy" {
		t.Fatalf("only the legacy one: read %q, want the legacy file's", cfg.Name)
	}
	write(newP, "New")
	if cfg, err = LoadConfig(ownConfigPath(root, renamed)); err != nil || cfg.Name != "New" {
		t.Fatalf("both: read %v (%v), want the new file's", cfg, err)
	}
}

// DefaultConfigRel and DefaultHostName are spelled from brand.Name in config.go's source, so
// the sweep that changes brand.Name moves them with it. While the names are equal a literal
// has the same value, so the source is what is checked. The new location is also what the
// reader tries first.
func TestDefaultsAreSpelledFromTheName(t *testing.T) {
	f, err := parser.ParseFile(token.NewFileSet(), "config.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	spelled := map[string]bool{}
	for _, d := range f.Decls {
		g, ok := d.(*ast.GenDecl)
		if !ok || g.Tok != token.CONST {
			continue
		}
		for _, s := range g.Specs {
			v := s.(*ast.ValueSpec)
			for i, n := range v.Names {
				if i < len(v.Values) {
					spelled[n.Name] = usesBrandName(v.Values[i])
				}
			}
		}
	}
	for _, c := range []string{"DefaultConfigRel", "DefaultHostName"} {
		if !spelled[c] {
			t.Errorf("config.go's %s is not spelled from brand.Name", c)
		}
	}
	if want := filepath.ToSlash(filepath.Join(brand.Dir(brand.Name), "panel.json")); DefaultConfigRel != want {
		t.Errorf("DefaultConfigRel = %q, want %q", DefaultConfigRel, want)
	}
	root := t.TempDir()
	if got, want := ownConfigPath(root, brand.Names()), filepath.Join(root, filepath.FromSlash(DefaultConfigRel)); got != want {
		t.Errorf("nothing there: the reader names %s, want DefaultConfigRel's %s", got, want)
	}
	if DefaultConfigRel != ".clauductor/panel.json" || DefaultHostName != "clauductor.localhost" {
		t.Errorf("while the names are one, the values change: %q, %q", DefaultConfigRel, DefaultHostName)
	}
}

// usesBrandName reports whether the expression names brand.Name.
func usesBrandName(e ast.Expr) bool {
	found := false
	ast.Inspect(e, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok && sel.Sel.Name == "Name" {
			if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "brand" {
				found = true
			}
		}
		return !found
	})
	return found
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
