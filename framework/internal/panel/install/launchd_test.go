package install

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/clock"
	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/lanes"
	"github.com/clauductor/clauductor/internal/panel/types"
)

func TestTokenFileIsPrivateAndPersistent(t *testing.T) {
	home := t.TempDir()
	// A pre-existing, too-open directory is tightened, not trusted.
	if err := os.MkdirAll(config.PanelDir(home), 0o755); err != nil {
		t.Fatal(err)
	}
	tok, err := LoadOrCreateToken(home)
	if err != nil {
		t.Fatal(err)
	}
	if !tokenRe.MatchString(tok) {
		t.Fatalf("token %q", tok)
	}
	fi, err := os.Stat(TokenPath(home))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("token file mode %o, want 600", fi.Mode().Perm())
	}
	di, _ := os.Stat(config.PanelDir(home))
	if di.Mode().Perm() != 0o700 {
		t.Fatalf("token dir mode %o, want 700", di.Mode().Perm())
	}
	again, err := LoadOrCreateToken(home)
	if err != nil || again != tok {
		t.Fatalf("token did not persist: %q vs %q (%v)", again, tok, err)
	}
	// A loosened file is tightened again on load.
	os.Chmod(TokenPath(home), 0o644)
	if _, err := LoadOrCreateToken(home); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(TokenPath(home)); fi.Mode().Perm() != 0o600 {
		t.Fatalf("loosened token file left at %o", fi.Mode().Perm())
	}
	// Garbage is replaced, never served as a token.
	os.WriteFile(TokenPath(home), []byte("short"), 0o600)
	if tok2, _ := LoadOrCreateToken(home); tok2 == "short" || !tokenRe.MatchString(tok2) {
		t.Fatalf("garbage token kept: %q", tok2)
	}
}

func TestPlistContent(t *testing.T) {
	home := "/Users/me"
	path := pathForAgent(home, func(tool string) (string, error) {
		switch tool {
		case "claude":
			return "/Users/me/.claude/local/claude", nil
		case "node":
			return "/opt/homebrew/opt/node@22/bin/node", nil
		}
		return "", errors.New("not found")
	})
	for _, want := range []string{"/opt/homebrew/bin", "/Users/me/.local/bin", "/Users/me/.claude/local", "/opt/homebrew/opt/node@22/bin", "/usr/bin"} {
		if !strings.Contains(":"+path+":", ":"+want+":") {
			t.Fatalf("PATH %q lacks %s", path, want)
		}
	}
	p := string(renderPlist(plistSpec{Home: home, Binary: "/Users/me/.clauductor/panel/bin/clauductor",
		Project: "/Users/me/dev/a&b", Config: "/Users/me/cfg.json", Port: 4393, Path: path}))
	for _, want := range []string{
		"<key>Label</key>\n\t<string>com.clauductor.panel</string>",
		"<string>/Users/me/.clauductor/panel/bin/clauductor</string>\n\t\t<string>panel</string>\n\t\t<string>--project</string>\n\t\t<string>/Users/me/dev/a&amp;b</string>\n\t\t<string>--config</string>\n\t\t<string>/Users/me/cfg.json</string>\n\t\t<string>--port</string>\n\t\t<string>4393</string>\n\t\t<string>--launchd</string>",
		"<key>RunAtLoad</key>\n\t<true/>",
		"<key>KeepAlive</key>\n\t<dict>\n\t\t<key>SuccessfulExit</key>\n\t\t<false/>",
		"<key>PATH</key>\n\t\t<string>" + path + "</string>",
		"<key>LANG</key>\n\t\t<string>en_US.UTF-8</string>",
		"<string>/Users/me/.clauductor/panel/logs/panel.log</string>",
		"<string>/Users/me/.clauductor/panel/logs/panel.err.log</string>",
		"<key>LimitLoadToSessionType</key>\n\t<string>Aqua</string>",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("plist lacks %q\n%s", want, p)
		}
	}
	if strings.Contains(p, "ANTHROPIC") {
		t.Fatal("plist carries an API key variable")
	}
	if runtime.GOOS == "darwin" {
		f := filepath.Join(t.TempDir(), "a.plist")
		os.WriteFile(f, []byte(p), 0o644)
		if out, err := exec.Command("/usr/bin/plutil", "-lint", f).CombinedOutput(); err != nil {
			t.Fatalf("plutil: %v %s", err, out)
		}
	}
}

func TestInstallAndUninstallWithATempHome(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	writeFile(t, filepath.Join(project, config.DefaultConfigRel), `{"name":"P"}`)
	self := filepath.Join(t.TempDir(), "clauductor")
	os.WriteFile(self, []byte("#!/bin/sh\n"), 0o755)
	var calls []string
	fake := func(argv ...string) ([]byte, error) {
		calls = append(calls, strings.Join(argv, " "))
		if argv[0] == "/usr/bin/osacompile" { // make the bundle osacompile would
			os.MkdirAll(filepath.Join(argv[2], "Contents", "Resources"), 0o755)
		}
		return nil, nil
	}
	var out bytes.Buffer
	old, _ := LoadOrCreateToken(home)
	if err := Install(InstallOptions{Clock: clock.System, Home: home, Project: project, Port: 4393, App: true, Out: &out, Exec: fake, Self: self}); err != nil {
		t.Fatal(err)
	}
	if tok, _ := LoadOrCreateToken(home); tok == old || !tokenRe.MatchString(tok) {
		t.Fatal("install did not rotate the token")
	}
	plist, err := os.ReadFile(PlistPath(home))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(plist), "<string>"+InstalledBinary(home)+"</string>") || strings.Contains(string(plist), "--config") {
		t.Fatalf("plist should run the installed copy with the project's default config:\n%s", plist)
	}
	if b, _ := os.ReadFile(InstalledBinary(home)); string(b) != "#!/bin/sh\n" {
		t.Fatal("binary not copied")
	}
	if fi, _ := os.Stat(TokenPath(home)); fi == nil || fi.Mode().Perm() != 0o600 {
		t.Fatal("install did not create a private token")
	}
	joined := strings.Join(calls, "\n")
	for _, want := range []string{"/usr/bin/plutil -lint " + PlistPath(home), "/bin/launchctl bootstrap " + guiDomain() + " " + PlistPath(home),
		"/usr/bin/osacompile -o " + AppPath(home) + " -e " + appScript(InstalledBinary(home))} {
		if !strings.Contains(joined, want) {
			t.Fatalf("install did not run %q; ran:\n%s", want, joined)
		}
	}
	if appScript("/a b/clauductor") != `do shell script "'/a b/clauductor' panel open"` {
		t.Fatalf("AppScript: %s", appScript("/a b/clauductor"))
	}

	calls = nil
	if err := Uninstall(home, &out, fake); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{PlistPath(home), InstalledBinary(home), TokenPath(home), AppPath(home)} {
		if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s survived uninstall", p)
		}
	}
	if !strings.Contains(strings.Join(calls, "\n"), "/bin/launchctl bootout "+guiDomain()+"/"+LaunchdLabel) {
		t.Fatalf("uninstall did not boot the agent out: %v", calls)
	}
}

func TestInstallRefusesAMissingConfigAndAForeignApp(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	fake := func(argv ...string) ([]byte, error) { t.Fatalf("ran %v", argv); return nil, nil }
	if err := Install(InstallOptions{Clock: clock.System, Home: home, Project: project, Port: 4393, Exec: fake, Self: "/bin/sh"}); err == nil {
		t.Fatal("installed an agent that would crash-loop on a missing config")
	}
	if _, err := os.Stat(PlistPath(home)); err == nil {
		t.Fatal("a refused install wrote the plist")
	}
	writeFile(t, filepath.Join(project, config.DefaultConfigRel), `{"name":"P"}`)
	os.MkdirAll(filepath.Join(AppPath(home), "Contents"), 0o755) // someone else's app
	ok := func(argv ...string) ([]byte, error) { return nil, nil }
	if err := Install(InstallOptions{Clock: clock.System, Home: home, Project: project, Port: 4393, App: true, Exec: ok, Self: "/bin/sh"}); err == nil ||
		!strings.Contains(err.Error(), "not made by clauductor") {
		t.Fatalf("replaced a foreign app: %v", err)
	}
	if err := Uninstall(home, nil, ok); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(AppPath(home)); err != nil {
		t.Fatal("uninstall removed an app it did not make")
	}
}

func TestBrowserOpensOncePerLogin(t *testing.T) {
	home := t.TempDir()
	now := time.Unix(1_790_000_000, 0)
	if !ShouldOpenAtLogin(home, now) {
		t.Fatal("first start did not open the browser")
	}
	if ShouldOpenAtLogin(home, now.Add(30*time.Second)) {
		t.Fatal("a crash restart 30 s later opened another tab")
	}
	if !ShouldOpenAtLogin(home, now.Add(2*time.Hour)) {
		t.Fatal("a later login did not open the browser")
	}
}

// A reinstall waits for bootout to finish before bootstrapping, and retries a
// bootstrap that fails once (launchd's error 5 when the old job is still tearing
// down; seen live).
func TestReinstallWaitsForBootoutAndRetriesBootstrap(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	writeFile(t, filepath.Join(project, config.DefaultConfigRel), `{"name":"P"}`)
	self := filepath.Join(t.TempDir(), "clauductor")
	os.WriteFile(self, []byte("#!/bin/sh\n"), 0o755)
	var calls []string
	prints, bootstraps := 0, 0
	fake := func(argv ...string) ([]byte, error) {
		verb := ""
		if len(argv) > 1 {
			verb = argv[1]
		}
		calls = append(calls, verb)
		switch verb {
		case "print": // the old job lingers for two polls
			prints++
			if prints <= 2 {
				return nil, nil
			}
			return []byte("Could not find service"), errors.New("exit status 113")
		case "bootstrap":
			bootstraps++
			if bootstraps == 1 {
				return []byte("Bootstrap failed: 5: Input/output error"), errors.New("exit status 5")
			}
		}
		return nil, nil
	}
	if err := Install(InstallOptions{Clock: clock.System, Home: home, Project: project, Port: 4393, Exec: fake, Self: self, PollDelay: time.Millisecond}); err != nil {
		t.Fatalf("install: %v (calls %v)", err, calls)
	}
	got := strings.Join(calls, ",")
	if !strings.Contains(got, "bootout,print,print,print,bootstrap,bootstrap") {
		t.Fatalf("launchctl sequence %s", got)
	}
}

// F7: uninstall removes what the agent owns, and keeps a lane registry only while it
// still lists lanes.
func TestUninstallRemovesPanelFilesButKeepsLiveRegistries(t *testing.T) {
	home := t.TempDir()
	RotateToken(home)
	dir := config.PanelDir(home)
	for _, f := range []string{"browser-opened", "pid", "port", "bin/clauductor", "logs/panel.log"} {
		os.MkdirAll(filepath.Dir(filepath.Join(dir, f)), 0o700)
		os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o600)
	}
	empty, _ := lanes.OpenRegistry(home, "/empty")
	empty.Put(types.LaneRecord{ID: "a", SessionID: sid, Path: "/empty", Mode: "root", Action: "start"})
	empty.Delete("a")
	live, _ := lanes.OpenRegistry(home, "/live")
	live.Put(types.LaneRecord{ID: "b", SessionID: sid, Path: "/live", Mode: "root", Action: "start"})
	var out strings.Builder
	if err := Uninstall(home, &out, func(...string) ([]byte, error) { return nil, nil }); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"token", "browser-opened", "pid", "port", "bin", "logs", filepath.Dir(lanes.RegistryPath(home, "/empty"))} {
		p := f
		if !filepath.IsAbs(p) {
			p = filepath.Join(dir, f)
		}
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s survived uninstall", p)
		}
	}
	if _, err := os.Stat(lanes.RegistryPath(home, "/live")); err != nil || !strings.Contains(out.String(), "Kept "+lanes.RegistryPath(home, "/live")) {
		t.Fatalf("a registry with lanes was not kept and reported: %v\n%s", err, out.String())
	}
}
