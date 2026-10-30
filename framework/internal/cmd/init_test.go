package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rfhayn/stridal/internal/panel/signals"
)

// PANEL-47: the `cd` line init prints as a next step runs as printed. The project's
// path holds a space and a quote, so a shell reads it as one word only if it is quoted.
func TestInitNextStepsCdIsShellQuoted(t *testing.T) {
	t.Parallel()
	framework, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	// init reads the template beside the framework: a missing one would fail init for another reason.
	if _, err := os.Stat(filepath.Join(framework, "template")); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(signals.ResolvePath(t.TempDir()), "o'k proj")
	r := cliEnv(t, "", t.TempDir(), []string{"CLAUDUCTOR_FRAMEWORK=" + framework}, "init", dir)
	if r.code != 0 {
		t.Fatalf("init: exit %d: %s %s", r.code, r.stdout, r.stderr)
	}
	_, line, found := strings.Cut(r.stdout, "\n  cd ")
	if !found {
		t.Fatalf("no cd line:\n%s", r.stdout)
	}
	line, _, _ = strings.Cut(line, "\n")
	if words := shellWords(t, line); len(words) != 1 || words[0] != dir {
		t.Fatalf("a shell reads the printed cd line %q as %q, want %q", line, words, dir)
	}
}
