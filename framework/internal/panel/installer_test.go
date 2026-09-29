package panel

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/clauductor/clauductor/internal/panel/signals"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readSettings(t *testing.T, home string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(SettingsPath(home))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// ourHooks counts tagged hook objects per event.
func ourHooks(t *testing.T, home string) map[string]int {
	t.Helper()
	out := map[string]int{}
	hooks, _ := readSettings(t, home)["hooks"].(map[string]any)
	for ev, groups := range hooks {
		for _, g := range groups.([]any) {
			for _, h := range g.(map[string]any)["hooks"].([]any) {
				raw, _ := json.Marshal(h)
				if isOurs(raw) {
					out[ev]++
				}
			}
		}
	}
	return out
}

// A realistic user file: other keys, a command hook with shell metacharacters, a
// matcher group, and a group mixing a foreign hook with a stale panel hook.
const foreignSettings = `{
  "env": {"FOO": "bar"},
  "permissions": {"allow": ["Bash(git status)"], "deny": []},
  "statusLine": {"type": "command", "command": "~/.claude/statusline.sh && echo <ok>"},
  "hooks": {
    "Stop": [
      {"hooks": [{"type": "command", "command": "say done && afplay /System/Library/Sounds/Glass.aiff"}]}
    ],
    "Notification": [
      {"matcher": "permission_prompt", "hooks": [
        {"type": "command", "command": "terminal-notifier -message hi"},
        {"type": "http", "url": "http://127.0.0.1:1111/hook?src=clauductor-panel", "timeout": 1}
      ]}
    ],
    "PreToolUse": [
      {"matcher": "Edit|Write", "hooks": [{"type": "http", "url": "http://127.0.0.1:9000/other?src=someone-else"}]}
    ]
  },
  "model": "opus"
}
`

// topKeys reads key order with encoding/json's tokenizer, independently of the
// orderedObject under test (reading with it would let a reversal cancel itself out).
func topKeys(t *testing.T, b []byte) []string {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(b))
	if _, err := dec.Token(); err != nil {
		t.Fatal(err)
	}
	var keys []string
	for dec.More() {
		k, err := dec.Token()
		if err != nil {
			t.Fatal(err)
		}
		keys = append(keys, k.(string))
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			t.Fatal(err)
		}
	}
	return keys
}

func TestInstallOnFreshHome(t *testing.T) {
	home := t.TempDir()
	changed, err := InstallHooks(home, 4393)
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	got := ourHooks(t, home)
	for _, ev := range signals.HookEvents {
		if got[ev] != 1 {
			t.Fatalf("%s: %d tagged hooks", ev, got[ev])
		}
	}
	if _, ok := got["SessionStart"]; ok {
		t.Fatal("SessionStart installed; HTTP hooks do not fire for it")
	}
	b, _ := os.ReadFile(SettingsPath(home))
	if !strings.Contains(string(b), `"url": "http://127.0.0.1:4393/hook?src=clauductor-panel"`) || !strings.Contains(string(b), `"timeout": 1`) {
		t.Fatalf("unexpected hook shape:\n%s", b)
	}
}

func TestInstallIsIdempotent(t *testing.T) {
	home := t.TempDir()
	writeFile(t, SettingsPath(home), foreignSettings)
	if _, err := InstallHooks(home, 4393); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(SettingsPath(home))
	fi1, _ := os.Stat(SettingsPath(home))
	changed, err := InstallHooks(home, 4393)
	if err != nil || changed {
		t.Fatalf("second install changed=%v err=%v", changed, err)
	}
	second, _ := os.ReadFile(SettingsPath(home))
	fi2, _ := os.Stat(SettingsPath(home))
	if !bytes.Equal(first, second) || !fi1.ModTime().Equal(fi2.ModTime()) {
		t.Fatal("second install rewrote the file")
	}
	for _, ev := range signals.HookEvents {
		if n := ourHooks(t, home)[ev]; n != 1 {
			t.Fatalf("%s: %d tagged hooks after two installs", ev, n)
		}
	}
}

func TestInstallPreservesForeignHooksAndKeys(t *testing.T) {
	home := t.TempDir()
	writeFile(t, SettingsPath(home), foreignSettings)
	if _, err := InstallHooks(home, 4393); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(SettingsPath(home))
	s := string(after)
	for _, want := range []string{
		`"say done && afplay /System/Library/Sounds/Glass.aiff"`, // no & rewrite
		`"~/.claude/statusline.sh && echo <ok>"`,
		`"terminal-notifier -message hi"`,
		`"matcher": "permission_prompt"`,
		`"http://127.0.0.1:9000/other?src=someone-else"`,
		`"matcher": "Edit|Write"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("lost %s", want)
		}
	}
	if strings.Contains(s, "127.0.0.1:1111") {
		t.Error("stale tagged hook (old port) not replaced")
	}
	if got, want := strings.Join(topKeys(t, after), ","), "env,permissions,statusLine,hooks,model"; got != want {
		t.Errorf("top-level key order %s, want %s", got, want)
	}
	m := readSettings(t, home)
	if m["model"] != "opus" || m["env"].(map[string]any)["FOO"] != "bar" {
		t.Error("other keys not preserved")
	}
	backup, err := os.ReadFile(SettingsPath(home) + ".clauductor-panel.bak")
	if err != nil || string(backup) != foreignSettings {
		t.Error("backup missing or not the pre-install content")
	}
}

func TestUninstallRemovesOnlyOurs(t *testing.T) {
	home := t.TempDir()
	writeFile(t, SettingsPath(home), foreignSettings)
	if _, err := InstallHooks(home, 4393); err != nil {
		t.Fatal(err)
	}
	changed, err := UninstallHooks(home)
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	if n := len(ourHooks(t, home)); n != 0 {
		t.Fatalf("%d events still carry a tagged hook", n)
	}
	// Expected: the original with only its stale tagged hook removed.
	var want bytes.Buffer
	json.Compact(&want, []byte(strings.Replace(foreignSettings,
		`,
        {"type": "http", "url": "http://127.0.0.1:1111/hook?src=clauductor-panel", "timeout": 1}`, "", 1)))
	got, _ := os.ReadFile(SettingsPath(home))
	if !jsonEqual(want.Bytes(), got) {
		t.Fatalf("uninstall changed foreign content:\n%s", got)
	}
	// Events we emptied are gone; events the user owns remain.
	hooks := readSettings(t, home)["hooks"].(map[string]any)
	for _, ev := range []string{"UserPromptSubmit", "SubagentStart", "SubagentStop", "SessionEnd"} {
		if _, ok := hooks[ev]; ok {
			t.Errorf("%s left behind as an empty event", ev)
		}
	}
	if changed, _ := UninstallHooks(home); changed {
		t.Error("second uninstall reported a change")
	}
}

func TestInstallRefusesInvalidSettings(t *testing.T) {
	home := t.TempDir()
	writeFile(t, SettingsPath(home), `{"hooks": [}`)
	if _, err := InstallHooks(home, 4393); err == nil {
		t.Fatal("invalid settings accepted")
	}
	b, _ := os.ReadFile(SettingsPath(home))
	if string(b) != `{"hooks": [}` {
		t.Fatal("invalid settings file was modified")
	}
}

func TestUninstallWithoutSettingsIsANoop(t *testing.T) {
	home := t.TempDir()
	changed, err := UninstallHooks(home)
	if err != nil || changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	if _, err := os.Stat(SettingsPath(home)); !os.IsNotExist(err) {
		t.Fatal("uninstall created a settings file")
	}
}

func TestInstallRefusesTrailingData(t *testing.T) {
	home := t.TempDir()
	const bad = `{"model":"x"} {"env":{"FOO":"bar"}}`
	writeFile(t, SettingsPath(home), bad)
	for _, f := range []func() (bool, error){
		func() (bool, error) { return InstallHooks(home, 4393) },
		func() (bool, error) { return UninstallHooks(home) },
	} {
		if changed, err := f(); err == nil || changed {
			t.Fatalf("accepted trailing data: changed=%v err=%v", changed, err)
		}
	}
	if b, _ := os.ReadFile(SettingsPath(home)); string(b) != bad {
		t.Fatal("file modified")
	}
	o := &orderedObject{}
	if o.UnmarshalJSON([]byte(`{"a":1} {"b":2}`)) == nil {
		t.Fatal("orderedObject accepted trailing data")
	}
}

func TestInstallFollowsASymlinkedSettingsFile(t *testing.T) {
	home := t.TempDir()
	dotfiles := t.TempDir()
	target := filepath.Join(dotfiles, "claude-settings.json")
	writeFile(t, target, `{"model":"opus"}`)
	if err := os.MkdirAll(filepath.Dir(SettingsPath(home)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, SettingsPath(home)); err != nil {
		t.Fatal(err)
	}
	if changed, err := InstallHooks(home, 4393); err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	fi, err := os.Lstat(SettingsPath(home))
	if err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("the symlink was replaced by a regular file")
	}
	b, _ := os.ReadFile(target)
	if !strings.Contains(string(b), HookURL(4393)) || !strings.Contains(string(b), `"model": "opus"`) {
		t.Fatalf("target not updated:\n%s", b)
	}
	if _, err := os.Stat(target + ".clauductor-panel.bak"); err != nil {
		t.Fatal("backup not written next to the target")
	}
}

func TestBackupKeepsThePrePanelFile(t *testing.T) {
	home := t.TempDir()
	writeFile(t, SettingsPath(home), foreignSettings)
	for _, port := range []int{4393, 4394, 4395} {
		if changed, err := InstallHooks(home, port); err != nil || !changed {
			t.Fatalf("port %d: changed=%v err=%v", port, changed, err)
		}
	}
	if _, err := UninstallHooks(home); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(SettingsPath(home) + ".clauductor-panel.bak"); string(b) != foreignSettings {
		t.Fatal("backup overwritten by a later change")
	}
}

func TestInstallIsIdempotentWhenAUserHookFollowsOurs(t *testing.T) {
	home := t.TempDir()
	if _, err := InstallHooks(home, 4393); err != nil {
		t.Fatal(err)
	}
	// The user (or another tool) appends their own Stop hook after ours.
	m := readSettings(t, home)
	stop := m["hooks"].(map[string]any)["Stop"].([]any)
	m["hooks"].(map[string]any)["Stop"] = append(stop, map[string]any{
		"hooks": []any{map[string]any{"type": "command", "command": "say done"}}})
	b, _ := json.MarshalIndent(m, "", "  ")
	writeFile(t, SettingsPath(home), string(b)+"\n")
	before, _ := os.ReadFile(SettingsPath(home))
	changed, err := InstallHooks(home, 4393)
	if err != nil || changed {
		t.Fatalf("second install changed=%v err=%v", changed, err)
	}
	after, _ := os.ReadFile(SettingsPath(home))
	if !bytes.Equal(before, after) {
		t.Fatal("file rewritten")
	}
	stop = readSettings(t, home)["hooks"].(map[string]any)["Stop"].([]any)
	first, _ := json.Marshal(stop[0])
	if len(stop) != 2 || !strings.Contains(string(first), HookTag) {
		t.Fatalf("our entry moved or duplicated: %s", after)
	}
}
