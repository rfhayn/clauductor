package template

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// projectDirs are the template's .claude/ directories a project owns: created when missing,
// never overwritten. evals/ is the project's on purpose (OPS-10): it adds its own cases.
var projectDirs = []string{".claude/agents/", ".claude/health/", ".claude/evals/"}

// Every directory under the template's .claude/ is placed on purpose, as the model's code or as
// the project's, so a new one cannot fall through to "create once, never update" unnoticed: in
// the OPS-8 rehearsal, a new directory's tier was a question nobody was asked, because the doc
// tier is the silent default. The set is the template directory itself, not a list.
func TestEveryTemplateClaudeDirIsPlaced(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(root, "template", ".claude"))
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		n++
		d := ".claude/" + e.Name() + "/"
		if !slices.Contains(FrameworkDirs, d) && !slices.Contains(projectDirs, d) {
			t.Errorf("template/%s is neither in FrameworkDirs nor a project directory: decide which (tier.go, and projectDirs here)", d)
		}
	}
	if n == 0 {
		t.Fatal("no directories under template/.claude: the test learned nothing")
	}
}
