package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/clauductor/clauductor/internal/panel/signals"
)

// PANEL-29: the page sees each open change's approval, keyed by change id, from the
// changes the metrics already read: approved, not approved, or (absent) no proposal.
// The New lane dialog's start plan warns before a build of an unapproved change.
func TestApprovalsInView(t *testing.T) {
	wt := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(wt, "changes", rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("add-score-photo/proposal.md", "# Proposal\n\n- **Approved:** 2026-10-02 by owner\n")
	write("add-group-card-entry/proposal.md", "# Proposal\n\nNot yet.\n")
	write("no-proposal-yet/design.md", "# Design\n")

	m := alertModel(t, "")
	if v := m.Snapshot(t0); v.Approvals == nil || len(v.Approvals) != 0 {
		t.Fatalf("before the changes are read: %#v, want an empty map", v.Approvals)
	}
	m.ApplyChanges(signals.ReadChanges([]string{wt}, []string{"changes"}), nil)
	v := m.Snapshot(t0)
	want := map[string]bool{"add-score-photo": true, "add-group-card-entry": false}
	if !reflect.DeepEqual(v.Approvals, want) {
		t.Fatalf("approvals %#v, want %#v", v.Approvals, want)
	}
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var page struct {
		Approvals map[string]bool `json:"approvals"`
	}
	if err := json.Unmarshal(b, &page); err != nil {
		t.Fatal(err)
	}
	if _, ok := page.Approvals["no-proposal-yet"]; ok || !reflect.DeepEqual(page.Approvals, want) {
		t.Errorf("the page reads approvals %#v, want %#v (no-proposal-yet absent)", page.Approvals, want)
	}
}
