package cmd

import (
	"testing"

	"github.com/clauductor/clauductor/internal/brand"
)

// After the rename (OPS-104), a repository install or the plugin marked under the legacy name
// is still recognised: install and update still own it, and still refuse a plugin repository.
func TestMarkerInReadsTheLegacyNamesMarker(t *testing.T) {
	names := []string{"newname", "clauductor"}
	for _, c := range []struct {
		what   string
		marker func(string) string
	}{{"install", brand.InstallMarker}, {"plugin", brand.PluginMarker}} {
		dir := t.TempDir()
		if got := markerIn(dir, c.marker, names); got != "" {
			t.Fatalf("%s: no marker, got %q", c.what, got)
		}
		write(t, dir, c.marker("clauductor"), "x\n")
		if got := markerIn(dir, c.marker, names); got != c.marker("clauductor") {
			t.Fatalf("%s: only the legacy marker, got %q", c.what, got)
		}
		write(t, dir, c.marker("newname"), "x\n")
		if got := markerIn(dir, c.marker, names); got != c.marker("newname") {
			t.Fatalf("%s: both markers, got %q, want the new one", c.what, got)
		}
	}
}

// While the names are one, the markers are the files they always were.
func TestMarkersAreUnchangedWhileTheNameIs(t *testing.T) {
	if installMarker != ".claude/clauductor-template" || pluginMarker != ".claude/clauductor-plugin" {
		t.Fatalf("installMarker = %q, pluginMarker = %q", installMarker, pluginMarker)
	}
}
