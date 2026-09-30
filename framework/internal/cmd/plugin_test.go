package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The marketplace installs plugin/ as committed, so a template change that is not rebuilt into it
// never reaches a plugin user. The committed plugin must be exactly what template/ builds to.
func TestCommittedPluginIsCurrent(t *testing.T) {
	repo, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	diffs, err := PluginDrift(repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(diffs) > 0 {
		t.Fatalf("plugin/ is stale against template/; run `cd framework && go run ./cmd/clauductor plugin build`:\n  %s", strings.Join(diffs, "\n  "))
	}
}

// install and update refuse a repository the plugin set up: both at once register every hook twice.
func TestInstallRefusesPluginRepo(t *testing.T) {
	dir := t.TempDir()
	if err := refusePluginModel("install", dir); err != nil {
		t.Fatalf("no marker: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, pluginMarker), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := refusePluginModel("install", dir)
	if err == nil || !strings.Contains(err.Error(), "plugin") {
		t.Fatalf("marker present: err = %v, want a refusal naming the plugin", err)
	}
}
