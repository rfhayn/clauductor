package install

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/clauductor/clauductor/internal/brand"
)

// After the rename (OPS-104), the hooks an earlier panel installed carry the legacy name's tag.
// They are still the panel's: found, so install replaces them and uninstall removes them, and
// read at this panel's URL, so driftOf calls them this panel's in an older form, never another
// panel's. A tag of neither name is not the panel's.
func TestHookTargetFindsTheLegacyNamesTag(t *testing.T) {
	tags := hookTags([]string{"newname", "clauductor"})
	re := commandPortRe(tags)
	want := hookURL(4393)
	for _, c := range []struct {
		name string
		hook map[string]any
		want string
	}{
		{"legacy command hook", map[string]any{"type": "command", "command": taggedCommand(4393, "clauductor-panel"), "async": true}, want},
		{"legacy HTTP hook", map[string]any{"type": "http", "url": taggedURL(4393, "clauductor-panel")}, want},
		{"another name's command hook", map[string]any{"type": "command", "command": taggedCommand(4393, "other-panel")}, ""},
		{"another name's HTTP hook", map[string]any{"type": "http", "url": taggedURL(4393, "other-panel")}, ""},
	} {
		raw, _ := json.Marshal(c.hook)
		if got := hookTargetOf(raw, tags, re); got != c.want {
			t.Errorf("%s: hookTargetOf = %q, want %q", c.name, got, c.want)
		}
	}
}

// While the names are one, the hook tag and the label are what they always were.
func TestHookTagAndLabelAreUnchangedWhileTheNameIs(t *testing.T) {
	if hookTag != "clauductor-panel" || LaunchdLabel != "com.clauductor.panel" {
		t.Fatalf("hookTag = %q, LaunchdLabel = %q", hookTag, LaunchdLabel)
	}
	if hookTags(brand.Names())[0] != hookTag {
		t.Fatalf("hookTags = %q", hookTags(brand.Names()))
	}
}

// A login agent a legacy name installed is booted out and its plist deleted, so it never
// runs beside the new one on the same port; with no legacy plist, launchctl is not run.
func TestRemoveLegacyAgentsBootsOutTheLegacyNamesAgent(t *testing.T) {
	home := t.TempDir()
	var calls []string
	run := func(argv ...string) ([]byte, error) {
		calls = append(calls, strings.Join(argv, " "))
		return nil, nil
	}
	names := []string{"newname", "clauductor"}
	if err := removeLegacyAgents(home, names, run); err != nil || len(calls) != 0 {
		t.Fatalf("no legacy agent: err %v, calls %q", err, calls)
	}
	plist := filepath.Join(home, "Library", "LaunchAgents", "com.clauductor.panel.plist")
	if err := os.MkdirAll(filepath.Dir(plist), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(plist, []byte("<plist/>\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := removeLegacyAgents(home, names, run); err != nil {
		t.Fatal(err)
	}
	if want := "/bin/launchctl bootout " + guiDomain() + "/com.clauductor.panel"; len(calls) != 1 || calls[0] != want {
		t.Fatalf("calls = %q, want [%s]", calls, want)
	}
	if _, err := os.Stat(plist); !os.IsNotExist(err) {
		t.Fatalf("the legacy plist is still there: %v", err)
	}
}
