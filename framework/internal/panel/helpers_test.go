package panel

import (
	"encoding/json"
	"os"
	"testing"
	"time"

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
