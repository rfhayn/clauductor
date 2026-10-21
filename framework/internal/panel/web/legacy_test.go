package web

import (
	"slices"
	"testing"

	"github.com/clauductor/clauductor/internal/brand"
	"github.com/clauductor/clauductor/internal/panel/config"
)

// After the rename (OPS-104), a bookmark at the legacy name's host still opens the panel.
func TestBrandHostsAcceptTheLegacyName(t *testing.T) {
	got := brandHosts("newname.localhost", []string{"newname", "clauductor"})
	if want := []string{"newname.localhost", "clauductor.localhost"}; !slices.Equal(got, want) {
		t.Fatalf("brandHosts = %q, want %q", got, want)
	}
}

// While the names are one, the allow-list holds the panel's host once, as it always did.
func TestBrandHostsAreUnchangedWhileTheNameIs(t *testing.T) {
	got := brandHosts(config.DefaultHostName, brand.Names())
	if want := []string{config.DefaultHostName}; !slices.Equal(got, want) {
		t.Fatalf("brandHosts = %q, want %q", got, want)
	}
}
