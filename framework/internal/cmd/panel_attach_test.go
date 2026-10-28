package cmd

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/install"
	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/clauductor/clauductor/internal/testbin"
	"github.com/clauductor/clauductor/internal/testgit"
	"github.com/clauductor/clauductor/internal/treesnap"
)

// gitFreeRunner is signals.ExecRunner for the test's own reads, with gitFreeEnv: the
// test process's environment may carry a hook's GIT_DIR.
func gitFreeRunner(ctx context.Context, dir string, argv []string) ([]byte, error) {
	c := exec.CommandContext(ctx, argv[0], argv[1:]...)
	c.Dir = dir
	c.Env = gitFreeEnv()
	return c.Output()
}

// attachRepo is a repository with commits, a second branch, an untracked file, an
// ignored file and the operating model's queue scripts (so init's notes name cards).
func attachRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	root := filepath.Join(t.TempDir(), "acme-web")
	if err := os.MkdirAll(filepath.Join(root, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	root = signals.ResolvePath(root)
	git := func(args ...string) {
		t.Helper()
		if out, err := testgit.Command(root, args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	write := func(rel, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, rel), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-q", "-b", "main")
	testbin.Write(t, filepath.Join(root, ".claude", "owner-queue.sh"), "#!/bin/sh\necho owner\n")
	testbin.Write(t, filepath.Join(root, ".claude", "roadmap-queue.sh"), "#!/bin/sh\necho roadmap\n")
	write(".gitignore", "*.log\n")
	write("README.md", "hello\n")
	git("add", "-A")
	git("commit", "-q", "-m", "one")
	write("README.md", "hello again\n")
	git("commit", "-q", "-am", "two")
	git("branch", "feature/x")
	write("untracked.txt", "not added\n")
	write("debug.log", "ignored\n")
	return root
}

// [ATTACH-1-S1] [ATTACH-1-S2] [ATTACH-2-S1] `clauductor panel attach <repo>` writes the
// sidecar under the home, registers the repository with it, prints what it wrote,
// init's notes one per choice and the trust line, and leaves every file under the
// repository's root, .git included, exactly as it was.
func TestPanelAttachCommand(t *testing.T) {
	t.Parallel()
	home := signals.ResolvePath(t.TempDir())
	root := attachRepo(t)
	before := treesnap.Take(t, root)

	r := cli(t, home, "panel", "attach", root)
	if r.code != 0 {
		t.Fatalf("exit %d: %s %s", r.code, r.stdout, r.stderr)
	}
	if d := treesnap.Diff(before, treesnap.Take(t, root)); len(d) != 0 {
		t.Fatalf("attach wrote into the repository:\n%s", strings.Join(d, "\n"))
	}

	path := config.SidecarPath(home, "acme-web")
	reg, err := config.LoadProjects(home)
	if err != nil {
		t.Fatal(err)
	}
	e := reg.Find(root)
	if e == nil || e.ID != "acme-web" || e.Config != path {
		t.Fatalf("projects.json: %+v", reg.Projects)
	}
	cfg, raw, err := config.LoadConfigRaw(path)
	if err != nil {
		t.Fatalf("the project does not load: %v", err)
	}
	if install.TrustedNow(home, root, path, install.ConfigHash(raw)) {
		t.Fatal("attach trusted the config")
	}
	// The output: what it wrote, init's notes for this repository, and the trust line.
	plan, err := install.PlanInit(context.Background(), gitFreeRunner, root, install.InitTarget{Path: filepath.Join(t.TempDir(), "p.json"), Sidecar: true})
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != string(plan.Body) || len(cfg.Cards) != 2 {
		t.Fatalf("the sidecar is not init's reading:\n%s", raw)
	}
	for _, want := range append([]string{"Wrote " + path, string(raw), "`clauductor panel trust --project " + root + "`", "Nothing was written into " + root},
		plan.Notes...) {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("the output does not say %q:\n%s", want, r.stdout)
		}
	}
	if !strings.Contains(r.stdout, "Owner queue, pinned") || !strings.Contains(r.stdout, "Change queue, pinned") {
		t.Errorf("the notes do not name the cards:\n%s", r.stdout)
	}
}

// ATTACH-1's requirement, the default repository: with no <repo>, attach takes the current directory's git toplevel; a
// second attach of the same repository is refused, and writes nothing more.
func TestPanelAttachDefaultsToTheCurrentRepository(t *testing.T) {
	t.Parallel()
	home := signals.ResolvePath(t.TempDir())
	root := attachRepo(t)
	r := cliIn(t, filepath.Join(root, ".claude"), home, "panel", "attach", "--id", "web")
	if r.code != 0 || !strings.Contains(r.stdout, `Registered `+root+` as "web"`) {
		t.Fatalf("exit %d: %s %s", r.code, r.stdout, r.stderr)
	}
	if _, err := os.Stat(config.SidecarPath(home, "web")); err != nil {
		t.Fatal(err)
	}
	r = cli(t, home, "panel", "attach", root)
	if r.code != 1 || !strings.Contains(r.stderr, `registered already as "web"`) {
		t.Fatalf("a second attach: exit %d: %s %s", r.code, r.stdout, r.stderr)
	}
	if entries, _ := os.ReadDir(filepath.Join(home, ".clauductor", "projects")); len(entries) != 1 {
		t.Fatalf("the second attach wrote a sidecar: %v", entries)
	}
}

// ATTACH-1's requirement, the --default flag: --default makes the attached project the default, as `panel add
// --default` does; without it the first project stays the default.
func TestPanelAttachDefault(t *testing.T) {
	t.Parallel()
	home := signals.ResolvePath(t.TempDir())
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"--id", "one"}, "one"},
		{[]string{"--id", "two"}, "one"},
		{[]string{"--id", "three", "--default"}, "three"},
	} {
		root := attachRepo(t)
		if r := cli(t, home, append([]string{"panel", "attach", root}, c.args...)...); r.code != 0 {
			t.Fatalf("%v: exit %d: %s %s", c.args, r.code, r.stdout, r.stderr)
		}
		if reg, err := config.LoadProjects(home); err != nil || reg.Default != c.want {
			t.Fatalf("after %v the default is %q, want %q (%v)", c.args, reg.Default, c.want, err)
		}
	}
}

// [ATTACH-4-S2] In an attached repository with no .clauductor/panel.json of its own,
// `clauductor panel trust --project <repo>` without --config trusts the registry's
// config, the sidecar, and names its path.
func TestPanelTrustReadsTheRegistrysConfig(t *testing.T) {
	t.Parallel()
	home := signals.ResolvePath(t.TempDir())
	root := attachRepo(t)
	if r := cli(t, home, "panel", "attach", root); r.code != 0 {
		t.Fatalf("attach: exit %d: %s %s", r.code, r.stdout, r.stderr)
	}
	if _, err := os.Stat(filepath.Join(root, config.DefaultConfigRel)); err == nil {
		t.Fatal("the repository has a config of its own")
	}
	path := config.SidecarPath(home, "acme-web")
	r := cli(t, home, "panel", "trust", "--project", root)
	if r.code != 0 {
		t.Fatalf("trust: exit %d: %s %s", r.code, r.stdout, r.stderr)
	}
	if !strings.Contains(r.stdout, "Trusted panel config "+path+" ") {
		t.Errorf("the output does not name the sidecar:\n%s", r.stdout)
	}
	_, raw, err := config.LoadConfigRaw(path)
	if err != nil {
		t.Fatal(err)
	}
	if !install.TrustedNow(home, root, path, install.ConfigHash(raw)) {
		t.Fatal("the sidecar is not recorded as trusted")
	}
}

// [ATTACH-3-S1] An attached project whose sidecar has a card running
// /usr/local/bin/report and a queue running `sh ../other/gate.sh`: `clauductor panel
// trust --project <repo>` refuses (exit non-zero), prints each of the two lines marked
// as not allowed in a sidecar with the word that resolves outside and the remedy, and
// records nothing in the project's trusted-config.json.
func TestPanelTrustRefusesASidecarCommandOutside(t *testing.T) {
	t.Parallel()
	home := signals.ResolvePath(t.TempDir())
	root := attachRepo(t)
	if r := cli(t, home, "panel", "attach", root); r.code != 0 {
		t.Fatalf("attach: exit %d: %s %s", r.code, r.stdout, r.stderr)
	}
	path := config.SidecarPath(home, "acme-web")
	if err := os.WriteFile(path, []byte(`{"name":"acme-web","lanes":{"main":"orchestrator"},
		"cards":[{"id":"report","title":"Report","command":["/usr/local/bin/report"],"refresh":"interval:60"}],
		"queues":[{"id":"gate","lock":"clauductor/gate.lock","command":["sh","../other/gate.sh"]}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	r := cli(t, home, "panel", "trust", "--project", root)
	if r.code == 0 {
		t.Fatalf("trust exited 0:\n%s", r.stdout)
	}
	for _, want := range []string{
		`  NOT ALLOWED IN A SIDECAR ("/usr/local/bin/report": /usr/local/bin/report resolves outside the repository; move it into a script in the repository): card report runs `,
		`  NOT ALLOWED IN A SIDECAR ("../other/gate.sh": ../other/gate.sh resolves outside the repository; move it into a script in the repository): queue gate runs `,
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("the output lacks %q:\n%s", want, r.stdout)
		}
	}
	if strings.Contains(r.stdout, "Trusted panel config") {
		t.Errorf("the output says it trusted:\n%s", r.stdout)
	}
	if _, err := os.Stat(install.TrustPath(home, root)); !os.IsNotExist(err) {
		t.Fatalf("trusted-config.json was written: %v", err)
	}
}

// ATTACH-3's requirement, the trust report's words: `panel trust` prints "the lines
// marked below" only for a sidecar refusal that marks lines. One that judged no line
// (its bytes changed since they were read, or did not read) prints its error alone,
// with no unmarked list under a sentence that names marks.
func TestPrintTrustNamesMarksOnlyWhenThereAreSome(t *testing.T) {
	t.Parallel()
	h := strings.Repeat("a", 64)
	runs := []string{`card report runs ["make","gate"] (interval:60)`}
	changed := &install.SidecarRefusedError{Path: "/s/panel.json", Err: install.ErrConfigChanged}
	var out strings.Builder
	if err := printTrust(&out, "/s/panel.json", h, runs, changed); !errors.Is(err, install.ErrConfigChanged) {
		t.Fatalf("the error: %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("a refusal with no marked line printed:\n%s", out.String())
	}
	marked := &install.SidecarRefusedError{Path: "/s/panel.json", Lines: []config.SidecarLine{{Line: runs[0]}}}
	out.Reset()
	if err := printTrust(&out, "/s/panel.json", h, runs, marked); err != marked {
		t.Fatalf("the error: %v", err)
	}
	if !strings.Contains(out.String(), "the lines marked below") || !strings.Contains(out.String(), runs[0]) {
		t.Errorf("a refusal with a marked line:\n%s", out.String())
	}
}

// shellWords is line as a POSIX shell splits it into words: what a reader who pastes
// it runs.
func shellWords(t *testing.T, line string) []string {
	t.Helper()
	out, err := exec.Command("sh", "-c", `printf '%s\0' `+line).Output()
	if err != nil {
		t.Fatalf("sh reading %q: %v", line, err)
	}
	return strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00")
}

// [ATTACH-5-S1] `clauductor panel remove <id>` of an attached project with no lanes
// keeps its sidecar byte for byte, and says where it is and the `panel add --config`
// line; running that line serves the project again from the same file, still trusted.
// The home holds a space and a quote, so the line runs as printed only if it is
// shell-quoted: a shell splits it into the words that are run.
func TestPanelRemoveKeepsTheSidecar(t *testing.T) {
	t.Parallel()
	home := filepath.Join(signals.ResolvePath(t.TempDir()), "o'k home")
	if err := os.Mkdir(home, 0o755); err != nil {
		t.Fatal(err)
	}
	root := attachRepo(t)
	if r := cli(t, home, "panel", "attach", root); r.code != 0 {
		t.Fatalf("attach: exit %d: %s %s", r.code, r.stdout, r.stderr)
	}
	path := config.SidecarPath(home, "acme-web")
	if _, err := install.TrustConfig(home, root, path); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	r := cli(t, home, "panel", "remove", "acme-web")
	if r.code != 0 {
		t.Fatalf("remove: exit %d: %s %s", r.code, r.stdout, r.stderr)
	}
	if after, err := os.ReadFile(path); err != nil || string(after) != string(before) {
		t.Fatalf("remove changed the sidecar (%v)", err)
	}
	for _, want := range []string{path, filepath.Dir(path), "To serve it again: clauductor panel add "} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("remove does not say %q:\n%s", want, r.stdout)
		}
	}
	if reg, _ := config.LoadProjects(home); reg.Find(root) != nil {
		t.Fatal("the project is still registered")
	}

	// The line it printed, split by a shell, is the add command, and serves it again
	// from the same file, still trusted.
	_, line, _ := strings.Cut(r.stdout, "To serve it again: clauductor ")
	line, _, _ = strings.Cut(line, "\n")
	words := shellWords(t, line)
	if want := []string{"panel", "add", "--project", root, "--config", path}; strings.Join(words, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("a shell reads the printed line %q as %q, want %q", line, words, want)
	}
	r = cli(t, home, words...)
	if r.code != 0 || strings.Contains(r.stdout, "is not trusted") {
		t.Fatalf("add again: exit %d: %s %s", r.code, r.stdout, r.stderr)
	}
	reg, err := config.LoadProjects(home)
	if err != nil {
		t.Fatal(err)
	}
	if e := reg.Find(root); e == nil || e.ConfigPath() != path {
		t.Fatalf("re-added: %+v", reg.Projects)
	}
	if !install.TrustedNow(home, root, path, install.ConfigHash(before)) {
		t.Fatal("the re-added sidecar is no longer trusted")
	}
	if r := cli(t, home, "panel", "list"); r.code != 0 || !strings.Contains(r.stdout, "· trusted ·") || !strings.Contains(r.stdout, "config "+path) {
		t.Fatalf("list after the re-add: exit %d: %s %s", r.code, r.stdout, r.stderr)
	}
}
