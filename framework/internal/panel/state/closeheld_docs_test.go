package state

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// closeForms are the reasons the verdict builds rather than names, each by its source
// expression, with the words docs/panel.md's refusals table must show for it.
var closeForms = map[string][]string{
	`"it holds or waits for the " + q + " lease"`:      {"it holds or waits for the <queue> lease"},
	`"claude is " + statusWords(tv.Status, tv.Approx)`: {"claude is <status>", "claude is starting", "(not a current reading)"},
	`CloseWorkWords + ": " + w`:                        {CloseWorkWords + ": <tasks>"},
	`LaneGoneWords(id)`:                                {"the panel no longer lists lane <id>"},
	`tv.CloseHeld`:                                     nil, // LaneCloseHeld passing on LaneView's
	`m.closeHeld(tv, queue, now)`:                      nil, // LaneView passing on closeHeld's, walked too
	`""`:                                               nil, // may close
}

// TestEveryCloseWordIsDocumented holds docs/panel.md's refusals table for `stridal
// lane close` to the verdict's words. It walks the authority: every reason closeHeld
// and LaneCloseHeld return, and every one LaneView sets, parsed from closeheld.go. A
// reason must be a Close*Words constant, a string literal, or one of closeForms, and
// its words must appear in the docs; a new reason with no row fails, and so does a new
// form until closeForms names it.
func TestEveryCloseWordIsDocumented(t *testing.T) {
	md, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "docs", "panel.md"))
	if err != nil {
		t.Fatal(err)
	}
	// A table row is one line, but a reason in prose may wrap.
	norm := func(s string) string { return strings.Join(strings.Fields(s), " ") }
	docs := norm(string(md))
	documented := func(what, words string) {
		t.Helper()
		if !strings.Contains(docs, norm(words)) {
			t.Errorf("docs/panel.md does not name %s's words: %q", what, words)
		}
	}

	consts := closeWordConsts(t)
	// The parse found the authority, not nothing: a vacuous pass would hide every word.
	if consts["CloseEndedWords"] != CloseEndedWords || consts["CloseWorkWords"] != CloseWorkWords {
		t.Fatalf("parsed %d Close*Words constants, missing ones known to exist: %v", len(consts), consts)
	}
	for n, v := range consts {
		documented(n, v)
	}

	reasons := closeReasons(t)
	if len(reasons) < 15 {
		t.Fatalf("found %d reasons in closeHeld, LaneView and LaneCloseHeld, fewer than they hold", len(reasons))
	}
	for _, r := range reasons {
		src := types.ExprString(r)
		switch e := r.(type) {
		case *ast.Ident:
			if v, ok := consts[e.Name]; ok {
				documented(e.Name, v)
				continue
			}
		case *ast.BasicLit:
			if e.Kind == token.STRING && e.Value != `""` {
				v, err := strconv.Unquote(e.Value)
				if err != nil {
					t.Fatal(err)
				}
				documented(src, v)
				continue
			}
		}
		words, ok := closeForms[src]
		if !ok {
			t.Errorf("closeheld.go gives the reason %s, which is neither a Close*Words constant, a string, nor a form closeForms documents", src)
			continue
		}
		for _, w := range words {
			documented(src, w)
		}
	}
}

// closeWordConsts is every constant named Close*Words in the package's non-test source.
func closeWordConsts(t *testing.T) map[string]string {
	t.Helper()
	name := regexp.MustCompile(`^Close.*Words$`)
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]string{}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), f, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.CONST {
				continue
			}
			for _, spec := range gd.Specs {
				vs := spec.(*ast.ValueSpec)
				for i, id := range vs.Names {
					if !name.MatchString(id.Name) {
						continue
					}
					lit, ok := vs.Values[i].(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						t.Fatalf("%s: %s is not a string literal, so its words cannot be checked", f, id.Name)
					}
					v, err := strconv.Unquote(lit.Value)
					if err != nil {
						t.Fatal(err)
					}
					found[id.Name] = v
				}
			}
		}
	}
	return found
}

// closeReasons is every reason the verdict gives: closeHeld's and LaneCloseHeld's
// returned strings, and what LaneView assigns to CloseHeld.
func closeReasons(t *testing.T) []ast.Expr {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "closeheld.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var out []ast.Expr
	seen := map[string]bool{}
	for _, decl := range file.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Body == nil {
			continue
		}
		switch fd.Name.Name {
		case "closeHeld", "LaneCloseHeld":
			seen[fd.Name.Name] = true
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				if _, ok := n.(*ast.FuncLit); ok {
					return false
				}
				if r, ok := n.(*ast.ReturnStmt); ok && len(r.Results) == 1 {
					out = append(out, r.Results[0])
				}
				return true
			})
		case "LaneView":
			seen[fd.Name.Name] = true
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				a, ok := n.(*ast.AssignStmt)
				if !ok || len(a.Lhs) != 1 {
					return true
				}
				if sel, ok := a.Lhs[0].(*ast.SelectorExpr); ok && sel.Sel.Name == "CloseHeld" {
					out = append(out, a.Rhs[0])
				}
				return true
			})
		}
	}
	for _, f := range []string{"closeHeld", "LaneView", "LaneCloseHeld"} {
		if !seen[f] {
			t.Fatalf("closeheld.go has no func %s: the reasons it gives are not walked", f)
		}
	}
	return out
}
