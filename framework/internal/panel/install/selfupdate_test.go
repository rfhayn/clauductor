package install

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/clauductor/clauductor/internal/testbin"
	"github.com/clauductor/clauductor/internal/treesnap"
)

// selfMod is the throwaway module standing in for the panel's own source.
const selfMod = "example.com/selfpanel"

// selfModule is that module's framework/: a go.mod, the ./cmd/clauductor the rebuild
// builds (answering the preflight's version, panel list and panel config-check), and
// the files D4 names.
var selfModule = map[string]string{
	"framework/go.mod": "module " + selfMod + "\n\ngo 1.22\n",
	"framework/cmd/clauductor/main.go": `package main

import (
	"fmt"
	"os"
	"strings"

	"example.com/selfpanel/internal/panel"
)

func main() {
	switch a := strings.Join(os.Args[1:], " "); {
	case a == "version":
		fmt.Println("selfpanel", panel.Version)
	case a == "panel list":
		fmt.Println("no projects")
	case strings.HasPrefix(a, "panel config-check --config "):
		fmt.Println(` + "`" + `{"name":"T","notify":true,"runs":[],"tmux_socket":""}` + "`" + `)
	default:
		fmt.Fprintln(os.Stderr, "unexpected:", a)
		os.Exit(2)
	}
}
`,
	"framework/internal/panel/server.go":          "package panel\n\nconst Version = \"one\"\n",
	"framework/internal/panel/install/launchd.go": "package install\n",
}

// realGo skips without go, and under -short: the test runs go build.
func realGo(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("-short: runs go build")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("no go")
	}
}

func gitT(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// selfRepo is a repository holding selfModule, at its first commit r.
func selfRepo(t *testing.T) (root, r string) {
	t.Helper()
	root = newRepo(t, nil)
	for rel, body := range selfModule {
		writeFile(t, filepath.Join(root, rel), body)
	}
	return root, commitAll(t, root, "framework")
}

func commitAll(t *testing.T, root, msg string) string {
	t.Helper()
	gitT(t, root, "add", "-A")
	gitT(t, root, "commit", "-q", "-m", msg)
	return gitT(t, root, "rev-parse", "HEAD")
}

// commitFile commits rel with body in root and returns the commit.
func commitFile(t *testing.T, root, rel, body string) string {
	t.Helper()
	writeFile(t, filepath.Join(root, rel), body)
	return commitAll(t, root, rel)
}

// buildAt builds root's ./cmd/clauductor, as it is checked out, into out.
func buildAt(t *testing.T, root, out string) BuildStamp {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "build", "-o", out, "./cmd/clauductor")
	cmd.Dir = filepath.Join(root, "framework")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v %s", err, b)
	}
	s, err := FileStamp(out)
	if err != nil || s.Module != selfMod || s.Revision == "" || s.Modified {
		t.Fatalf("the build's stamp: %+v %v", s, err)
	}
	return s
}

func readBytes(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func gone(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("%s is there (%v)", path, err)
	}
}

func TestModuleOfReadsTheModuleLine(t *testing.T) {
	t.Parallel()
	p := filepath.Join(t.TempDir(), "go.mod")
	writeFile(t, p, "// the panel\nmodule \"example.com/x\" // quoted\n\ngo 1.22\n")
	if m, err := ModuleOf(p); err != nil || m != "example.com/x" {
		t.Fatalf("ModuleOf: %q %v", m, err)
	}
	writeFile(t, p, "go 1.22\n")
	if m, err := ModuleOf(p); err == nil {
		t.Fatalf("a go.mod with no module line read as %q", m)
	}
}

// Decide's conditions, each in turn: another module is silent; then framework/ must
// differ over R..B; then the login agent, the VCS stamp, R's place in the history and a
// clean checkout, each with the reason the notice gives and how to update.
func TestDecideAsksEachConditionInTurn(t *testing.T) {
	t.Parallel()
	root, r := selfRepo(t)
	gitT(t, root, "checkout", "-q", "-b", "side")
	side := commitFile(t, root, "framework/internal/panel/side.go", "package panel\n")
	gitT(t, root, "checkout", "-q", "main")
	b := commitFile(t, root, "framework/internal/panel/server.go", "package panel\n\nconst Version = \"two\"\n")
	docs := commitFile(t, root, "docs.txt", "docs\n")
	rebuild := func(edit func(*Rebuild)) Plan {
		rb := &Rebuild{Home: t.TempDir(), Root: root, From: r, To: b, Launchd: true, Exe: "/x/clauductor",
			Self: BuildStamp{Module: selfMod, Revision: r}, Run: signals.ExecRunner}
		if edit != nil {
			edit(rb)
		}
		return rb.Decide(context.Background())
	}
	for _, tc := range []struct {
		name          string
		edit          func(*Rebuild)
		ours, changed bool
		skip          string
	}{
		{"another module", func(rb *Rebuild) { rb.Self.Module = "example.com/other" }, false, false, ""},
		{"no module stamp", func(rb *Rebuild) { rb.Self.Module = "" }, false, false, ""},
		{"rebuilds", nil, true, true, ""},
		{"framework unchanged", func(rb *Rebuild) { rb.Self.Revision, rb.From, rb.To = b, b, docs }, true, false, ""},
		{"started by hand", func(rb *Rebuild) { rb.Launchd = false }, true, true, "started by hand"},
		{"no VCS", func(rb *Rebuild) { rb.Self.Revision = "" }, true, true, "no VCS build information"},
		{"modified", func(rb *Rebuild) { rb.Self.Modified = true }, true, true, "local changes"},
		{"not a commit", func(rb *Rebuild) { rb.Self.Revision = strings.Repeat("0123456789", 4) }, true, true, "not a commit of this repository"},
		{"another branch", func(rb *Rebuild) { rb.Self.Revision = side }, true, true, "not an ancestor"},
	} {
		p := rebuild(tc.edit)
		if p.Ours != tc.ours || p.Changed != tc.changed || !strings.Contains(p.Skip, tc.skip) || (tc.skip == "") != (p.Skip == "") {
			t.Errorf("%s: %+v, want ours %v, changed %v, skip %q", tc.name, p, tc.ours, tc.changed, tc.skip)
			continue
		}
		if p.Skip != "" && !strings.Contains(p.How, "go build -o") {
			t.Errorf("%s: how to update is %q", tc.name, p.How)
		}
	}
	if p := rebuild(func(rb *Rebuild) { rb.Launchd = false }); !strings.Contains(p.How, "go build -o /x/clauductor ./cmd/clauductor") {
		t.Errorf("a panel started by hand is told %q, want its own binary rebuilt", p.How)
	}
	// R not an ancestor of B: the range is the fast-forward's, still said.
	if p := rebuild(func(rb *Rebuild) { rb.Self.Revision = side }); p.Base != r {
		t.Errorf("base %s, want the fast-forward's start %s", p.Base, r)
	}
	// An untracked file: no rebuild, and the file named.
	writeFile(t, filepath.Join(root, "notes.txt"), "mine\n")
	p := rebuild(nil)
	if !strings.Contains(p.Skip, "notes.txt (untracked)") || !strings.Contains(p.Skip, "stamped as modified") {
		t.Errorf("an untracked file: %+v", p)
	}
	if err := os.Remove(filepath.Join(root, "notes.txt")); err != nil {
		t.Fatal(err)
	}
	// A git that cannot diff is said, never read as framework/ unchanged.
	if p := rebuild(func(rb *Rebuild) { rb.To = strings.Repeat("f", 40) }); !p.Changed || !strings.Contains(p.Skip, "git could not say") {
		t.Errorf("a failed diff: %+v", p)
	}
}

// The install step is named over R..B, the build's range: a merge that touched
// launchd.go and was not rebuilt is carried by the next rebuild, whose own merge did not.
func TestDecideNamesInstallOverTheBuildsRange(t *testing.T) {
	t.Parallel()
	root, r := selfRepo(t)
	a := commitFile(t, root, "framework/internal/panel/install/launchd.go", "package install\n\n// v2\n")
	b := commitFile(t, root, "framework/internal/panel/server.go", "package panel\n\nconst Version = \"two\"\n")
	rb := &Rebuild{Home: t.TempDir(), Root: root, From: a, To: b, Launchd: true, Self: BuildStamp{Module: selfMod, Revision: r}, Run: signals.ExecRunner}
	p := rb.Decide(context.Background())
	if !p.Rebuilds() || p.Base != r || len(p.Install) != 1 || p.Install[0] != "framework/internal/panel/install/launchd.go" {
		t.Fatalf("plan %+v, want a rebuild over %s..%s naming launchd.go", p, r, b)
	}
	rb.Self.Revision = a
	if p := rb.Decide(context.Background()); !p.Rebuilds() || len(p.Install) != 0 {
		t.Fatalf("plan %+v, want a rebuild naming no install file", p)
	}
}

// A build that fails leaves no new binary, even one the compiler had begun to write,
// and its error is the compiler's, without the package header.
func TestBuildFailureLeavesNoNewBinary(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "framework"), 0o755); err != nil {
		t.Fatal(err)
	}
	goCmd := filepath.Join(t.TempDir(), "go")
	testbin.Write(t, goCmd, `#!/bin/sh
[ "$1 $2" = "build -o" ] || exit 9
printf partial > "$3"
echo '# example.com/selfpanel/internal/panel'
echo 'internal/panel/server.go:3:1: syntax error: non-declaration statement outside function body'
exit 1
`)
	rb := &Rebuild{Home: t.TempDir(), Root: root, Go: goCmd, Run: signals.ExecRunner}
	err := rb.Build(context.Background())
	if err == nil || !strings.Contains(err.Error(), "server.go:3:1: syntax error") || strings.Contains(err.Error(), "# example.com") {
		t.Fatalf("Build: %v", err)
	}
	gone(t, rb.NewBinary())
}

// A build over its bound is stopped, group and all, and leaves no new binary.
func TestBuildTimeoutStopsIt(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "framework"), 0o755); err != nil {
		t.Fatal(err)
	}
	goCmd := filepath.Join(t.TempDir(), "go")
	testbin.Write(t, goCmd, "#!/bin/sh\nprintf partial > \"$3\"\nsleep 30\n")
	rb := &Rebuild{Home: t.TempDir(), Root: root, Go: goCmd, Timeout: 300 * time.Millisecond, Run: signals.ExecRunner}
	start := time.Now()
	err := rb.Build(context.Background())
	if err == nil || !strings.Contains(err.Error(), "took over 300ms") {
		t.Fatalf("Build: %v", err)
	}
	if took := time.Since(start); took > 15*time.Second {
		t.Fatalf("the build returned after %s", took)
	}
	gone(t, rb.NewBinary())
}

// With no go to run, the build says where the PATH comes from.
func TestBuildWithoutGoSaysSo(t *testing.T) {
	t.Parallel()
	rb := &Rebuild{Home: t.TempDir(), Root: t.TempDir(), Go: filepath.Join(t.TempDir(), "go"), Run: signals.ExecRunner}
	if err := rb.Build(context.Background()); err == nil || !strings.Contains(err.Error(), "clauductor panel install") {
		t.Fatalf("Build: %v", err)
	}
	gone(t, rb.NewBinary())
}

// [CATCHUP-4-S5] The build and the preflight of a merge that changed framework/ write
// nothing in the repository: its files are as the fast-forward left them, and its status
// shows no change. The new binary names B, with no local changes.
func TestRebuildWritesNothingInTheRepository(t *testing.T) {
	realGo(t)
	t.Parallel()
	root, r := selfRepo(t)
	b := commitFile(t, root, "framework/internal/panel/server.go", "package panel\n\nconst Version = \"two\"\n")
	before := treesnap.Take(t, root)
	rb := &Rebuild{Home: t.TempDir(), Root: root, From: r, To: b, Launchd: true, Self: BuildStamp{Module: selfMod, Revision: r},
		Timeout: 2 * time.Minute, Run: signals.ExecRunner}
	if err := rb.Build(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := rb.Preflight(context.Background()); err != nil {
		t.Fatal(err)
	}
	after := treesnap.Take(t, root)
	for _, s := range []treesnap.Snapshot{before, after} {
		for p := range s {
			if p == ".git" || strings.HasPrefix(p, ".git/") {
				delete(s, p)
			}
		}
	}
	if d := treesnap.Diff(before, after); len(d) > 0 {
		t.Fatalf("the build wrote in the repository:\n%s", strings.Join(d, "\n"))
	}
	if st := gitT(t, root, "status", "--porcelain"); st != "" {
		t.Fatalf("the repository's status after the build: %q", st)
	}
	if s, err := FileStamp(rb.NewBinary()); err != nil || s != (BuildStamp{Module: selfMod, Revision: b}) {
		t.Fatalf("the new binary's stamp: %+v %v, want %s at %s", s, err, selfMod, b)
	}
	// The same binary is not B's when the fast-forward reached another commit.
	rb.To = r
	if err := rb.Preflight(context.Background()); err == nil || !strings.Contains(err.Error(), "stamped") {
		t.Fatalf("a binary stamped %s passed the preflight for %s: %v", b, r, err)
	}
	gone(t, rb.NewBinary())
}

// A new binary with no Go build information fails the preflight and is removed.
func TestPreflightRefusesABinaryWithNoBuildInformation(t *testing.T) {
	t.Parallel()
	rb := &Rebuild{Home: t.TempDir(), Root: t.TempDir(), To: "b", Self: BuildStamp{Module: selfMod}, Run: signals.ExecRunner}
	if err := config.EnsurePrivateDir(filepath.Dir(rb.NewBinary())); err != nil {
		t.Fatal(err)
	}
	testbin.Write(t, rb.NewBinary(), "#!/bin/sh\nexit 0\n")
	if err := rb.Preflight(context.Background()); err == nil || !strings.Contains(err.Error(), "build information") {
		t.Fatalf("Preflight: %v", err)
	}
	gone(t, rb.NewBinary())
}

// The swap keeps the binary it replaces as clauductor.prev, never opens the token or the
// plist, and replaces the PATH's clauductor the same way only when its build names this
// module and a commit of the repository, leaving its directory's permissions as they were.
// A stop as the swap runs (a logout, `launchctl bootout`) still replaces the PATH's: review
// 2026-10-23 found its check read the stop's ended context as "not a commit", leaving it
// behind with no word.
func TestSwapKeepsThePreviousBinaries(t *testing.T) {
	realGo(t)
	t.Parallel()
	root, r := selfRepo(t)
	built := filepath.Join(t.TempDir(), "clauductor")
	buildAt(t, root, built)
	atR := readBytes(t, built)
	for _, tc := range []struct {
		name, module      string
		replaced, stopped bool
	}{{"this module", selfMod, true, false}, {"another module", "example.com/other", false, false},
		{"this module, stopped", selfMod, true, true}} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			rb := &Rebuild{Home: home, Root: root, From: r, To: r, Launchd: true, Self: BuildStamp{Module: tc.module, Revision: r}, Run: signals.ExecRunner}
			bin := InstalledBinary(home)
			writeFile(t, bin, "the binary built at R")
			writeFile(t, rb.NewBinary(), "the binary built at B")
			writeFile(t, TokenPath(home), "token\n")
			writeFile(t, PlistPath(home), "<plist/>\n")
			pathDir := filepath.Join(t.TempDir(), "bin")
			if err := os.MkdirAll(pathDir, 0o755); err != nil {
				t.Fatal(err)
			}
			rb.PathBinary = filepath.Join(pathDir, "clauductor")
			if err := copyExecutable(built, rb.PathBinary); err != nil {
				t.Fatal(err)
			}
			ctx, stop := context.WithCancel(context.Background())
			defer stop()
			if tc.stopped {
				stop()
			}
			replaced, pathErr, err := rb.Swap(ctx)
			if err != nil || pathErr != nil {
				t.Fatalf("Swap: %v, the PATH's: %v", err, pathErr)
			}
			if readBytes(t, bin) != "the binary built at B" || readBytes(t, bin+".prev") != "the binary built at R" {
				t.Fatalf("installed %q, .prev %q", readBytes(t, bin), readBytes(t, bin+".prev"))
			}
			gone(t, rb.NewBinary())
			if readBytes(t, TokenPath(home)) != "token\n" || readBytes(t, PlistPath(home)) != "<plist/>\n" {
				t.Fatal("the token or the plist changed")
			}
			if tc.replaced {
				if replaced != rb.PathBinary || readBytes(t, rb.PathBinary) != "the binary built at B" || readBytes(t, rb.PathBinary+".prev") != atR {
					t.Fatalf("the PATH's clauductor was not replaced, keeping its .prev (replaced %q)", replaced)
				}
			} else {
				if replaced != "" || readBytes(t, rb.PathBinary) != atR {
					t.Fatalf("another module's clauductor on PATH was replaced (%q)", replaced)
				}
				gone(t, rb.PathBinary+".prev")
			}
			gone(t, rb.PathBinary+".new")
			if fi, err := os.Stat(pathDir); err != nil || fi.Mode().Perm() != 0o755 {
				t.Fatalf("the PATH's directory: %v %v, want 0755 as it was", fi.Mode(), err)
			}
		})
	}
}

// A swap with no installed binary to keep fails, and leaves no new binary.
func TestSwapWithNoInstalledBinaryFails(t *testing.T) {
	t.Parallel()
	rb := &Rebuild{Home: t.TempDir(), Run: signals.ExecRunner}
	writeFile(t, rb.NewBinary(), "new")
	if _, _, err := rb.Swap(context.Background()); err == nil {
		t.Fatal("Swap with no installed binary succeeded")
	}
	gone(t, rb.NewBinary())
	gone(t, InstalledBinary(rb.Home))
}

// The restart marker is said once: the first take reads it, the second finds none.
func TestRestartMarkerIsTakenOnce(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	want := RestartMarker{Root: "/r", From: "a", To: "b"}
	if err := WriteRestartMarker(home, want); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(config.PanelDir(home)); err != nil || fi.Mode().Perm() != 0o700 {
		t.Fatalf("the panel's directory: %v %v", fi, err)
	}
	if m, err := TakeRestartMarker(home); err != nil || m == nil || *m != want {
		t.Fatalf("first take: %+v %v", m, err)
	}
	if m, err := TakeRestartMarker(home); err != nil || m != nil {
		t.Fatalf("second take: %+v %v", m, err)
	}
}

func TestTildeWritesHomeAsTilde(t *testing.T) {
	t.Parallel()
	if got := Tilde("/Users/a", "/Users/a/.clauductor/panel/bin/clauductor"); got != "~/.clauductor/panel/bin/clauductor" {
		t.Fatalf("Tilde: %q", got)
	}
	if got := Tilde("/Users/a", "/Users/ab/x"); got != "/Users/ab/x" {
		t.Fatalf("Tilde of another home: %q", got)
	}
}
