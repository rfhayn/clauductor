package config

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// DefaultConfigRel is the new name's location only. A project set up under a legacy name
// keeps its panel.json at that name's until it migrates, so production code reads a project's
// config through OwnConfigPath (or ProjectEntry.ConfigPath), never by joining DefaultConfigRel
// to a root: that join reports the legacy project's config missing. This holds the rule over
// every non-test .go file of the framework. A test may join it, to write a config where a new
// one goes.
func TestNoReadSiteJoinsDefaultConfigRel(t *testing.T) {
	framework := filepath.Join("..", "..", "..")
	if _, err := os.Stat(filepath.Join(framework, "go.mod")); err != nil {
		t.Fatalf("framework root not at %s: %v", framework, err)
	}
	if hits := joinsOfDefaultConfigRel(t, framework); len(hits) > 0 {
		t.Fatalf("DefaultConfigRel joined to a root, which misses a legacy name's panel.json; read it through config.OwnConfigPath:\n  %s",
			strings.Join(hits, "\n  "))
	}
}

// The scan finds each form of the join, and passes a use that is not a path (schema.go's
// title) and a join in a test.
func TestJoinsOfDefaultConfigRelFindsEachJoin(t *testing.T) {
	dir := t.TempDir()
	put := func(name, src string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	put("prod.go", `package p

func a(root string) string { return filepath.Join(root, config.DefaultConfigRel) }
func b(root string) string { return path.Join(root, DefaultConfigRel) }
func c(root string) string { return root + "/" + config.DefaultConfigRel }
func d(root string) string { return root + string(os.PathSeparator) + (DefaultConfigRel) }
func e(root string) string { return "x: " + filepath.Join(root, DefaultConfigRel) }
`)
	put("title.go", `package p

var title = brand.Name + " panel config (" + DefaultConfigRel + ")"
var rel = DefaultConfigRel
`)
	put("prod_test.go", `package p

func w(root string) string { return filepath.Join(root, config.DefaultConfigRel) }
`)
	got := joinsOfDefaultConfigRel(t, dir)
	var want []string
	for _, line := range []int{3, 4, 5, 6, 7} {
		want = append(want, filepath.Join(dir, "prod.go")+":"+strconv.Itoa(line))
	}
	if !slices.Equal(got, want) {
		t.Fatalf("hits:\n  %s\nwant:\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}

// joinsOfDefaultConfigRel lists, as file:line, each place in the non-test .go files under dir
// that joins DefaultConfigRel to something before it: an argument of filepath.Join or
// path.Join, or a + whose left neighbour is not a string literal (a root) or is one ending in
// a path separator. A + after other text (a title, a message) is not a path.
func joinsOfDefaultConfigRel(t *testing.T, dir string) []string {
	t.Helper()
	var hits []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != dir && (d.Name() == "testdata" || d.Name() == "vendor" || strings.HasPrefix(d.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, p, nil, 0)
		if err != nil {
			return err
		}
		hit := func(n ast.Node) { hits = append(hits, p+":"+strconv.Itoa(fset.Position(n.Pos()).Line)) }
		inner := map[ast.Expr]bool{} // the + nodes inside a chain already checked
		ast.Inspect(f, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.CallExpr:
				if sel, ok := x.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Join" {
					if pkg, ok := sel.X.(*ast.Ident); ok && (pkg.Name == "filepath" || pkg.Name == "path") &&
						slices.ContainsFunc(x.Args, isDefaultConfigRel) {
						hit(x)
					}
				}
			case *ast.BinaryExpr:
				if x.Op != token.ADD || inner[x] {
					return true
				}
				ops := concatOperands(x, inner)
				for i := 1; i < len(ops); i++ {
					if isDefaultConfigRel(ops[i]) && joinsAPath(ops[i-1]) {
						hit(x)
						break
					}
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return hits
}

// concatOperands flattens a chain of + into its operands, left to right, marking each inner
// + so the walk does not check it again.
func concatOperands(e ast.Expr, inner map[ast.Expr]bool) []ast.Expr {
	b, ok := e.(*ast.BinaryExpr)
	if !ok || b.Op != token.ADD {
		return []ast.Expr{e}
	}
	for _, s := range []ast.Expr{b.X, b.Y} {
		if sb, ok := s.(*ast.BinaryExpr); ok && sb.Op == token.ADD {
			inner[sb] = true
		}
	}
	return append(concatOperands(b.X, inner), concatOperands(b.Y, inner)...)
}

func isDefaultConfigRel(e ast.Expr) bool {
	for {
		p, ok := e.(*ast.ParenExpr)
		if !ok {
			break
		}
		e = p.X
	}
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name == "DefaultConfigRel"
	case *ast.SelectorExpr:
		return x.Sel.Name == "DefaultConfigRel"
	}
	return false
}

// joinsAPath: the operand before DefaultConfigRel makes it a path under something, not text.
func joinsAPath(left ast.Expr) bool {
	lit, ok := left.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return true // a variable or a call: a root, or a separator
	}
	s, err := strconv.Unquote(lit.Value)
	return err != nil || strings.HasSuffix(s, "/") || strings.HasSuffix(s, `\`)
}
