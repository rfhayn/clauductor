package install

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSettings(t *testing.T, home, body string) string {
	t.Helper()
	p := SettingsPath(home)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

const settingsFixture = `{
  "model": "opus",
  "hooks": {
    "Stop": [ { "hooks": [ { "type": "command", "command": "echo mine" } ] } ]
  },
  "permissions": { "allow": ["Bash(ls:*)"] },
  "statusLine": { "type": "command", "command": "~/.claude/statusline.sh" }
}
`

func choose(t *testing.T, home, flag, input string, tty bool) (string, string) {
	t.Helper()
	var out bytes.Buffer
	mode, err := ChooseRemoteControl(RemoteOptions{Home: home, Flag: flag, In: strings.NewReader(input), Out: &out, Interactive: tty})
	if err != nil {
		t.Fatal(err)
	}
	return mode, out.String()
}

// PANEL-19: the question comes only when settings.json has no remoteControlAtStartup,
// nothing was chosen before, no flag was given, and stdin is a terminal. It is asked
// once; 2 records lanes and leaves settings.json alone.
func TestRemoteControlAskedOnlyWhenAbsent(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	p := writeSettings(t, home, settingsFixture)
	mode, out := choose(t, home, "", "2\n", true)
	if mode != RemoteLanes || !strings.Contains(out, "Choose 1, 2 or 3") || !strings.Contains(out, "2) Only the panel's lanes") {
		t.Fatalf("asked: %q %q", mode, out)
	}
	if b, _ := os.ReadFile(p); string(b) != settingsFixture {
		t.Fatal("lanes mode changed settings.json")
	}
	if !LanesRemoteControl(home) || RemoteControlSummary(home) != RemoteLanes {
		t.Fatal("lanes mode was not recorded")
	}
	if mode, out := choose(t, home, "", "1\n", true); mode != RemoteLanes || strings.Contains(out, "Choose") {
		t.Fatalf("asked twice: %q %q", mode, out)
	}
	// No terminal, no flag: off, never asked.
	home2 := t.TempDir()
	writeSettings(t, home2, settingsFixture)
	if mode, out := choose(t, home2, "", "1\n", false); mode != RemoteOff || strings.Contains(out, "Choose") {
		t.Fatalf("no terminal: %q %q", mode, out)
	}
	// End of input, or Enter: the safe answer.
	home3 := t.TempDir()
	if mode, _ := choose(t, home3, "", "", true); mode != RemoteOff {
		t.Fatalf("end of input: %q", mode)
	}
}

// An explicit true or false is the user's: never asked about, never changed.
func TestRemoteControlExplicitSettingRespected(t *testing.T) {
	t.Parallel()
	for _, v := range []string{"true", "false"} {
		home := t.TempDir()
		body := `{"remoteControlAtStartup": ` + v + `, "model": "opus"}` + "\n"
		p := writeSettings(t, home, body)
		_, out := choose(t, home, "", "1\n", true)
		if strings.Contains(out, "Choose") || !strings.Contains(out, "left as it is") {
			t.Fatalf("%s: asked anyway: %q", v, out)
		}
		if b, _ := os.ReadFile(p); string(b) != body {
			t.Fatalf("%s: settings.json changed", v)
		}
		want := RemoteOff
		if v == "true" {
			want = RemoteAll
		}
		if RemoteControlSummary(home) != want {
			t.Fatalf("%s: summary %q", v, RemoteControlSummary(home))
		}
	}
}

// "all" merges the one key into settings.json through the hooks' read-merge-write:
// every other key keeps its value and its place; the change and the undo are printed.
func TestRemoteControlAllMergesOneKey(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	p := writeSettings(t, home, settingsFixture)
	mode, out := choose(t, home, "all", "", false)
	if mode != RemoteAll || !strings.Contains(out, `set "remoteControlAtStartup": true`) || !strings.Contains(out, "To undo") || !strings.Contains(out, "(was absent)") {
		t.Fatalf("all: %q %q", mode, out)
	}
	b, _ := os.ReadFile(p)
	var got, want map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal([]byte(settingsFixture), &want)
	want["remoteControlAtStartup"] = true
	gj, _ := json.Marshal(got)
	wj, _ := json.Marshal(want)
	if string(gj) != string(wj) {
		t.Fatalf("settings after all:\n%s\nwant\n%s", gj, wj)
	}
	// Order kept: the new key goes last, the others where they were.
	s := string(b)
	if !(strings.Index(s, `"model"`) < strings.Index(s, `"hooks"`) && strings.Index(s, `"hooks"`) < strings.Index(s, `"permissions"`) &&
		strings.Index(s, `"statusLine"`) < strings.Index(s, `"remoteControlAtStartup"`)) {
		t.Fatalf("order changed:\n%s", s)
	}
	if bak, err := os.ReadFile(p + ".clauductor-panel.bak"); err != nil || string(bak) != settingsFixture {
		t.Fatalf("no backup of the file as it was: %v", err)
	}
	if mode, out := choose(t, home, "all", "", false); mode != RemoteAll || !strings.Contains(out, "already has") {
		t.Fatalf("again: %q", out)
	}
}

// A flag skips the question, whatever is recorded; a bad one is refused.
func TestRemoteControlFlagOverrides(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	writeSettings(t, home, settingsFixture)
	if mode, out := choose(t, home, "lanes", "1\n", true); mode != RemoteLanes || strings.Contains(out, "Choose") {
		t.Fatalf("flag lanes: %q %q", mode, out)
	}
	if mode, _ := choose(t, home, "off", "", true); mode != RemoteOff || LanesRemoteControl(home) {
		t.Fatalf("flag off: %q", mode)
	}
	if _, err := ChooseRemoteControl(RemoteOptions{Home: home, Flag: "everything"}); err == nil {
		t.Fatal("a bad flag was accepted")
	}
	// Off over an explicit true says the setting still applies.
	home2 := t.TempDir()
	writeSettings(t, home2, `{"remoteControlAtStartup": true}`)
	if _, out := choose(t, home2, "off", "", false); !strings.Contains(out, "still sets") {
		t.Fatalf("off over true: %q", out)
	}
}
