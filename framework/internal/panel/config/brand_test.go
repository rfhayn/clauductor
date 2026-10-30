package config

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"
)

// DefaultConfigRel and DefaultHostName are spelled from brand.Name in config.go's source, so
// a change of brand.Name moves them with it.
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
	if DefaultConfigRel != ".stridal/panel.json" || DefaultHostName != "stridal.localhost" {
		t.Errorf("DefaultConfigRel = %q, DefaultHostName = %q", DefaultConfigRel, DefaultHostName)
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

// Each reader names the name's path.
func TestReadersNameTheNamesPath(t *testing.T) {
	home := t.TempDir()
	for _, c := range []struct{ name, got, want string }{
		{"PanelDir", PanelDir(home), filepath.Join(home, ".stridal", "panel")},
		{"SidecarPath", SidecarPath(home, "app"), filepath.Join(home, ".stridal", "projects", "app", "panel.json")},
		{"OwnConfigPath", OwnConfigPath(home), filepath.Join(home, ".stridal", "panel.json")},
		{"ConfigPath", ProjectEntry{Root: home}.ConfigPath(), filepath.Join(home, ".stridal", "panel.json")},
	} {
		if c.got != c.want {
			t.Errorf("%s = %s, want %s", c.name, c.got, c.want)
		}
	}
}
