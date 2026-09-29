package panel

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// The launchd login agent makes the panel "just there": it starts at login, restarts
// after a crash, and needs no terminal. Only this file knows about launchd.

// LaunchdLabel names the login agent.
const LaunchdLabel = "com.clauductor.panel"

// AppName is the optional Dock/Spotlight launcher `install --app` creates.
const AppName = "Clauductor Panel.app"

// appMarker is written inside the launcher so uninstall removes only an app it made.
const appMarker = "clauductor-panel-launcher"

func panelDir(home string) string { return filepath.Join(home, ".clauductor", "panel") }

// TokenPath holds the persistent token in launchd mode.
func TokenPath(home string) string { return filepath.Join(panelDir(home), "token") }

// LogDir holds the login agent's stdout and stderr.
func LogDir(home string) string { return filepath.Join(panelDir(home), "logs") }

// PlistPath is the login agent's definition.
func PlistPath(home string) string {
	return filepath.Join(home, "Library", "LaunchAgents", LaunchdLabel+".plist")
}

// InstalledBinary is where install copies the clauductor binary the agent runs, so
// the agent never depends on a build directory or a worktree that may disappear.
func InstalledBinary(home string) string { return filepath.Join(panelDir(home), "bin", "clauductor") }

// AppPath is the launcher app.
func AppPath(home string) string { return filepath.Join(home, "Applications", AppName) }

var tokenRe = regexp.MustCompile(`^[0-9a-f]{64}$`)

// ensurePrivateDir makes dir (0700), and tightens it if it already exists looser.
func ensurePrivateDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return os.Chmod(dir, 0o700)
}

// LoadOrCreateToken returns the persistent token, creating it on first use. The
// file is 0600 in a 0700 directory; a file with any other content is replaced.
func LoadOrCreateToken(home string) (string, error) {
	path := TokenPath(home)
	if err := ensurePrivateDir(filepath.Dir(path)); err != nil {
		return "", err
	}
	if b, err := os.ReadFile(path); err == nil {
		if t := strings.TrimSpace(string(b)); tokenRe.MatchString(t) {
			return t, os.Chmod(path, 0o600)
		}
	}
	return writeNewToken(path)
}

// RotateToken replaces the persistent token. A running panel under launchd sees the
// new file within 2 s and rotates: old cookies, terminals and event streams die.
func RotateToken(home string) (string, error) {
	path := TokenPath(home)
	if err := ensurePrivateDir(filepath.Dir(path)); err != nil {
		return "", err
	}
	return writeNewToken(path)
}

// readToken returns the token file's token, or "" if it is missing or malformed.
func readToken(home string) string {
	b, err := os.ReadFile(TokenPath(home))
	if err != nil {
		return ""
	}
	if t := strings.TrimSpace(string(b)); tokenRe.MatchString(t) {
		return t
	}
	return ""
}

func writeNewToken(path string) (string, error) {
	t, err := NewToken()
	if err != nil {
		return "", err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(t+"\n"), 0o600); err != nil {
		return "", err
	}
	if err := os.Chmod(tmp, 0o600); err != nil { // WriteFile's mode is filtered by umask
		return "", err
	}
	return t, os.Rename(tmp, path)
}

// loginOpenGap is how long after one automatic browser open the next is skipped, so
// a crash loop under KeepAlive cannot open a tab every few seconds.
const loginOpenGap = 5 * time.Minute

// shouldOpenAtLogin decides whether a launchd start opens the browser, and records
// the open when it does.
func shouldOpenAtLogin(home string, now time.Time) bool {
	path := filepath.Join(panelDir(home), "browser-opened")
	if b, err := os.ReadFile(path); err == nil {
		if sec, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64); err == nil &&
			now.Sub(time.Unix(sec, 0)) < loginOpenGap {
			return false
		}
	}
	_ = ensurePrivateDir(filepath.Dir(path))
	_ = os.WriteFile(path, []byte(strconv.FormatInt(now.Unix(), 10)+"\n"), 0o600)
	return true
}

// PlistSpec is what the login agent runs.
type PlistSpec struct {
	Home    string
	Binary  string // absolute
	Project string // absolute
	Config  string // absolute, or "" for the project's default
	Port    int
	Path    string // PATH for the agent
}

// PathForAgent builds the agent's PATH: Homebrew, ~/.local/bin, the directories of
// the tools the panel and its lanes run (as found on the installing user's PATH),
// then the system directories. launchd's own default PATH has none of these.
func PathForAgent(home string, lookPath func(string) (string, error)) string {
	var dirs []string
	seen := map[string]bool{}
	add := func(d string) {
		if d != "" && !seen[d] {
			seen[d] = true
			dirs = append(dirs, d)
		}
	}
	add("/opt/homebrew/bin")
	add(filepath.Join(home, ".local", "bin"))
	for _, tool := range []string{"claude", "tmux", "git", "gh", "node", "jq"} {
		if p, err := lookPath(tool); err == nil {
			add(filepath.Dir(p))
		}
	}
	for _, d := range []string{"/usr/local/bin", "/usr/bin", "/bin", "/usr/sbin", "/sbin"} {
		add(d)
	}
	return strings.Join(dirs, ":")
}

func xmlText(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// RenderPlist renders the login agent. KeepAlive restarts it only after an unclean
// exit, so `launchctl bootout` (SIGTERM, clean exit) stops it for good.
func RenderPlist(s PlistSpec) []byte {
	args := []string{s.Binary, "panel", "--project", s.Project}
	if s.Config != "" {
		args = append(args, "--config", s.Config)
	}
	args = append(args, "--port", strconv.Itoa(s.Port), "--launchd")
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>` + LaunchdLabel + `</string>
	<key>ProgramArguments</key>
	<array>
`)
	for _, a := range args {
		b.WriteString("\t\t<string>" + xmlText(a) + "</string>\n")
	}
	b.WriteString(`	</array>
	<key>WorkingDirectory</key>
	<string>` + xmlText(s.Project) + `</string>
	<key>EnvironmentVariables</key>
	<dict>
		<key>PATH</key>
		<string>` + xmlText(s.Path) + `</string>
		<key>LANG</key>
		<string>en_US.UTF-8</string>
	</dict>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<dict>
		<key>SuccessfulExit</key>
		<false/>
	</dict>
	<key>ThrottleInterval</key>
	<integer>30</integer>
	<key>LimitLoadToSessionType</key>
	<string>Aqua</string>
	<key>ProcessType</key>
	<string>Interactive</string>
	<key>StandardOutPath</key>
	<string>` + xmlText(filepath.Join(LogDir(s.Home), "panel.log")) + `</string>
	<key>StandardErrorPath</key>
	<string>` + xmlText(filepath.Join(LogDir(s.Home), "panel.err.log")) + `</string>
</dict>
</plist>
`)
	return []byte(b.String())
}

// InstallOptions configures `clauductor panel install`.
type InstallOptions struct {
	Home    string
	Project string
	Config  string
	Port    int
	App     bool
	Out     io.Writer
	// Exec runs launchctl, osacompile and plutil (injectable for tests).
	Exec func(argv ...string) ([]byte, error)
	// Self is the running binary to copy (default os.Executable).
	Self string
}

func execCombined(argv ...string) ([]byte, error) {
	return exec.Command(argv[0], argv[1:]...).CombinedOutput()
}

func guiDomain() string { return "gui/" + strconv.Itoa(os.Getuid()) }

// copyFileAtomic copies src to dst (mode 0755) via a temp file and rename, so a
// running agent's binary is replaced, never truncated under it.
func copyFileAtomic(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := ensurePrivateDir(filepath.Dir(dst)); err != nil {
		return err
	}
	tmp := dst + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

// Install writes and loads the login agent (and optionally the launcher app).
func Install(o InstallOptions) error {
	if o.Exec == nil {
		o.Exec = execCombined
	}
	if o.Out == nil {
		o.Out = io.Discard
	}
	if o.Self == "" {
		self, err := os.Executable()
		if err != nil {
			return err
		}
		o.Self, _ = filepath.EvalSymlinks(self)
	}
	project := ResolvePath(o.Project)
	if fi, err := os.Stat(project); err != nil || !fi.IsDir() {
		return fmt.Errorf("--project %s is not a directory", o.Project)
	}
	cfg := ""
	if o.Config != "" {
		cfg = ResolvePath(o.Config)
	} else {
		cfg = filepath.Join(project, DefaultConfigRel)
	}
	// Fail now rather than in a crash loop under launchd.
	if _, err := LoadConfig(cfg); err != nil {
		return err
	}
	if o.Config == "" {
		cfg = "" // let the agent follow the project's default path
	}
	bin := InstalledBinary(o.Home)
	if err := copyFileAtomic(o.Self, bin); err != nil {
		return fmt.Errorf("copying the binary to %s: %w", bin, err)
	}
	if err := ensurePrivateDir(LogDir(o.Home)); err != nil {
		return err
	}
	// Every install rotates the token, so reinstalling is also how to revoke it.
	if _, err := RotateToken(o.Home); err != nil {
		return err
	}
	plist := PlistPath(o.Home)
	if err := os.MkdirAll(filepath.Dir(plist), 0o755); err != nil {
		return err
	}
	spec := PlistSpec{Home: o.Home, Binary: bin, Project: project, Config: cfg, Port: o.Port,
		Path: PathForAgent(o.Home, exec.LookPath)}
	if err := os.WriteFile(plist, RenderPlist(spec), 0o644); err != nil {
		return err
	}
	if out, err := o.Exec("/usr/bin/plutil", "-lint", plist); err != nil {
		return fmt.Errorf("plutil rejected %s: %v: %s", plist, err, strings.TrimSpace(string(out)))
	}
	// Replace a loaded copy; "not loaded" is the normal first-install answer.
	_, _ = o.Exec("/bin/launchctl", "bootout", guiDomain()+"/"+LaunchdLabel)
	if out, err := o.Exec("/bin/launchctl", "bootstrap", guiDomain(), plist); err != nil {
		return fmt.Errorf("launchctl bootstrap %s %s: %v: %s", guiDomain(), plist, err, strings.TrimSpace(string(out)))
	}
	fmt.Fprintf(o.Out, "Installed login agent %s (%s)\n  runs %s panel --project %s on 127.0.0.1:%d\n  logs: %s\n",
		LaunchdLabel, plist, bin, project, o.Port, LogDir(o.Home))
	if o.App {
		if err := installApp(o, bin); err != nil {
			return err
		}
		fmt.Fprintf(o.Out, "Created %s (runs `clauductor panel open`)\n", AppPath(o.Home))
	}
	return nil
}

// appleScriptString quotes s as an AppleScript string literal.
func appleScriptString(s string) string {
	return `"` + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `"`, `\"`) + `"`
}

// AppScript is the launcher's AppleScript source.
func AppScript(bin string) string {
	return "do shell script " + appleScriptString(shq(bin)+" panel open")
}

func installApp(o InstallOptions, bin string) error {
	app := AppPath(o.Home)
	if _, err := os.Stat(app); err == nil {
		if _, err := os.Stat(filepath.Join(app, "Contents", "Resources", appMarker)); err != nil {
			return fmt.Errorf("%s exists and was not made by clauductor; not replacing it", app)
		}
		if err := os.RemoveAll(app); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(app), 0o755); err != nil {
		return err
	}
	if out, err := o.Exec("/usr/bin/osacompile", "-o", app, "-e", AppScript(bin)); err != nil {
		return fmt.Errorf("osacompile: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return os.WriteFile(filepath.Join(app, "Contents", "Resources", appMarker), []byte("made by clauductor panel install --app\n"), 0o644)
}

// Uninstall unloads and removes the login agent, the launcher app (only if it made
// it), the installed binary and the token. The hooks stay; --uninstall-hooks removes
// them.
func Uninstall(home string, out io.Writer, run func(argv ...string) ([]byte, error)) error {
	if run == nil {
		run = execCombined
	}
	if out == nil {
		out = io.Discard
	}
	if b, err := run("/bin/launchctl", "bootout", guiDomain()+"/"+LaunchdLabel); err != nil {
		fmt.Fprintf(out, "launchctl bootout: %s (fine if it was not loaded)\n", strings.TrimSpace(string(b)))
	}
	dir := panelDir(home)
	// Everything the agent owns goes. port and pid belong to a running panel, which
	// bootout has just stopped.
	for _, p := range []string{PlistPath(home), TokenPath(home), filepath.Join(dir, "browser-opened"),
		filepath.Join(dir, "pid"), MarkerPath(home), filepath.Join(dir, "bin"), LogDir(home)} {
		if err := os.RemoveAll(p); err != nil {
			return err
		}
	}
	// A lane registry is kept only while it still lists lanes: they may be running in
	// tmux, and RESUME needs their session ids.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		reg := filepath.Join(dir, e.Name(), "lanes.json")
		if !e.IsDir() {
			continue
		}
		b, err := os.ReadFile(reg)
		if err != nil {
			continue
		}
		var f registryFile
		if json.Unmarshal(b, &f) == nil && len(f.Lanes) == 0 {
			if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
				return err
			}
			continue
		}
		fmt.Fprintf(out, "Kept %s: it lists %d lane(s) of %s, which may still run in tmux. Delete it once they are stopped.\n",
			reg, len(f.Lanes), f.Project)
	}
	_ = os.Remove(dir) // only if nothing is left
	app := AppPath(home)
	if _, err := os.Stat(filepath.Join(app, "Contents", "Resources", appMarker)); err == nil {
		if err := os.RemoveAll(app); err != nil {
			return err
		}
		fmt.Fprintf(out, "Removed %s\n", app)
	}
	fmt.Fprintf(out, "Removed login agent %s. Panel hooks stay in %s; `clauductor panel --uninstall-hooks` removes them.\n", LaunchdLabel, SettingsPath(home))
	return nil
}

// OpenURL returns the tokened URL of the installed panel, after checking it answers.
func OpenURL(ctx context.Context, home string, port int) (string, error) {
	b, err := os.ReadFile(TokenPath(home))
	if err != nil {
		return "", fmt.Errorf("no token at %s: the panel is not installed as a login agent (`clauductor panel install --project <path>`); a panel started by hand prints its own URL", TokenPath(home))
	}
	token := strings.TrimSpace(string(b))
	base := fmt.Sprintf("http://%s:%d", LoopbackHost, port)
	cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(cctx, http.MethodGet, base+"/healthz", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("the panel is not answering on %s (%v). Restart it: launchctl kickstart -k %s/%s, and read %s",
			base, err, guiDomain(), LaunchdLabel, LogDir(home))
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
	resp.Body.Close()
	// Send the token only to THIS panel: whatever holds the port must report the PID
	// the panel wrote beside its marker.
	pidFile := filepath.Join(panelDir(home), "pid")
	want, err := os.ReadFile(pidFile)
	if err != nil {
		return "", fmt.Errorf("no %s: the panel is not running (launchctl kickstart -k %s/%s)", pidFile, guiDomain(), LaunchdLabel)
	}
	if got := strings.TrimSpace(string(body)); got != "ok pid="+strings.TrimSpace(string(want)) {
		return "", fmt.Errorf("%s answers /healthz with %q, not as the panel whose pid is %s; not sending it the token",
			base, got, strings.TrimSpace(string(want)))
	}
	return base + "/?t=" + token, nil
}

// OpenBrowser opens url in the default browser.
func OpenBrowser(url string) { openBrowser(url) }
