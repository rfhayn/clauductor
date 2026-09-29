package panel

import (
	"encoding/json"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/lanes"
	"github.com/clauductor/clauductor/internal/panel/lease"
)

func waitUntil(t *testing.T, what string, d time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// writeLeaseRecord writes an owner.json or waiter file as the lease protocol
// (docs/panel.md) lays it out on disk.
func writeLeaseRecord(t *testing.T, path string, o lease.LeaseOwner) {
	t.Helper()
	b, err := json.Marshal(o)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

// The lane registry's file, as the tests read and write it (lanes.RegistryPath).
type registryFile struct {
	Version int                `json:"version"`
	Project string             `json:"project"`
	Lanes   []lanes.LaneRecord `json:"lanes"`
}

var uuidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

const sid = "0f8fad5b-d9cb-469f-a165-70867728950e"

func testLaneManager(t *testing.T) *lanes.LaneManager {
	t.Helper()
	cfg, err := config.ParseConfig([]byte(`{"name":"T","lanes":{"main":"orchestrator","fix/":"fix","change/":"build","change/propose-*":"propose"},
		"tmux_socket":"sock","lane_types":{"build":{"model":"opus","effort":"high"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	reg, err := lanes.OpenRegistry(t.TempDir(), "/repo")
	if err != nil {
		t.Fatal(err)
	}
	return &lanes.LaneManager{TmuxPath: "/opt/homebrew/bin/tmux", Socket: cfg.Socket(), Root: "/repo", Cfg: cfg, Registry: reg,
		Program: []string{"/Users/me/.local/bin/claude"}, LookupEnv: func(string) (string, bool) { return "", false }}
}
