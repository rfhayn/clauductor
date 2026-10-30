package brand

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/rfhayn/stridal/internal/testgit"
)

// The shell half (template/.claude/lib/conf.sh's _brand_env) holds the same name as this
// package: a script and the binary never disagree on where a file is.
func TestShellBrandMatches(t *testing.T) {
	if out := shell(t, t.TempDir(), `printf '%s' "$BRAND_NAME"`); out != Name {
		t.Fatalf("conf.sh's _brand_env = %q, want %q (framework/internal/brand)", out, Name)
	}
}

// focus_file names the focus file under the name's folder in the git directory.
func TestShellFocusFileIsUnderTheName(t *testing.T) {
	repo := t.TempDir()
	if b, err := testgit.Command(repo, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, b)
	}
	gitDir, err := filepath.EvalSymlinks(filepath.Join(repo, ".git"))
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(gitDir, Name, "focus-change-x.txt")
	if got := resolved(t, shell(t, repo, `focus_file change/x "$ROOT"`)); got != want {
		t.Fatalf("focus_file = %s, want %s", got, want)
	}
}

// shell sources the template's conf.sh with ROOT set to root (so no project.conf is read)
// and runs script, a constant: paths reach it only through the environment.
func shell(t *testing.T, root, script string) string {
	t.Helper()
	conf, err := filepath.Abs(filepath.Join("..", "..", "..", "template", ".claude", "lib", "conf.sh"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", "-c", `. "$CONF"; `+script)
	cmd.Env = append(os.Environ(), "ROOT="+root, "CONF="+conf)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("sh: %v: %s", err, out)
	}
	return string(out)
}

// resolved is p with its existing folders' symlinks evaluated (macOS's /var is /private/var).
func resolved(t *testing.T, p string) string {
	t.Helper()
	d, err := filepath.EvalSymlinks(filepath.Dir(filepath.Dir(p)))
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(d, filepath.Base(filepath.Dir(p)), filepath.Base(p))
}
