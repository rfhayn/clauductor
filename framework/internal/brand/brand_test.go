package brand

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/clauductor/clauductor/internal/testgit"
)

// Rename step 3 changes nothing a user sees: the name and the legacy name are one, so every
// reader has exactly one place to look.
func TestNamesAreOneWhileTheNameIsUnchanged(t *testing.T) {
	if Name != LegacyName {
		t.Skip("the rename has flipped the name (OPS-104)")
	}
	if got := Names(); len(got) != 1 || got[0] != "clauductor" {
		t.Fatalf("Names() = %q, want [clauductor]", got)
	}
}

// Existing reads the legacy location while only it exists, the new one once it does, and
// names the new one when neither does: that is where a write goes.
func TestExistingReadsTheLegacyPathFirstThenTheNew(t *testing.T) {
	dir := t.TempDir()
	newP, legacy := filepath.Join(dir, ".newname"), filepath.Join(dir, ".clauductor")
	if got := Existing(newP, legacy); got != newP {
		t.Fatalf("neither exists: got %s, want the new %s", got, newP)
	}
	mkdir(t, legacy)
	if got := Existing(newP, legacy); got != legacy {
		t.Fatalf("only the legacy one exists: got %s, want %s", got, legacy)
	}
	mkdir(t, newP)
	if got := Existing(newP, legacy); got != newP {
		t.Fatalf("both exist: got %s, want the new %s", got, newP)
	}
	if got := Existing(); got != "" {
		t.Fatalf("no paths: got %q", got)
	}
}

// The shell half (template/.claude/lib/conf.sh's _brand_env) holds the same name as this
// package: a script and the binary never disagree on where a file is.
func TestShellBrandMatches(t *testing.T) {
	out := shell(t, t.TempDir(), `printf '%s|%s|%s' "$BRAND_NAME" "$BRAND_LEGACY_NAME" "$BRAND_NAMES"`)
	want := Name + "|" + LegacyName + "|" + strings.Join(Names(), " ")
	if out != want {
		t.Fatalf("conf.sh's _brand_env = %q, want %q (framework/internal/brand)", out, want)
	}
}

// focus_file reads a focus file a legacy name's folder holds while the new name's has none.
func TestShellFocusFileReadsTheLegacyNameFirst(t *testing.T) {
	repo := t.TempDir()
	if b, err := testgit.Command(repo, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, b)
	}
	gitDir, err := filepath.EvalSymlinks(filepath.Join(repo, ".git"))
	if err != nil {
		t.Fatal(err)
	}
	focus := `BRAND_NAMES="newname clauductor"; focus_file change/x "$ROOT"`
	newP := filepath.Join(gitDir, "newname", "focus-change-x.txt")
	legacy := filepath.Join(gitDir, "clauductor", "focus-change-x.txt")
	if got := resolved(t, shell(t, repo, focus)); got != newP {
		t.Fatalf("no focus file yet: got %s, want the new %s", got, newP)
	}
	write(t, legacy)
	if got := resolved(t, shell(t, repo, focus)); got != legacy {
		t.Fatalf("only the legacy one: got %s, want %s", got, legacy)
	}
	write(t, newP)
	if got := resolved(t, shell(t, repo, focus)); got != newP {
		t.Fatalf("both: got %s, want the new %s", got, newP)
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
	cmd.Env = append(testgit.Env(), "ROOT="+root, "CONF="+conf) // the script's git reads root's repository
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

func mkdir(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
}

func write(t *testing.T, p string) {
	t.Helper()
	mkdir(t, filepath.Dir(p))
	if err := os.WriteFile(p, []byte("focus\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}
