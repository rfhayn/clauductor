package web

import (
	"slices"
	"testing"

	"github.com/clauductor/clauductor/internal/brand"
	"github.com/clauductor/clauductor/internal/panel/config"
)

// After the rename (OPS-104), the panel is opened at the new name's host, and a bookmark at
// the legacy name's host still opens it. Both come from the names alone, so the new name's
// host is first even though config.DefaultHostName is still the legacy one here.
func TestBrandHostsAcceptTheLegacyName(t *testing.T) {
	got := brandHosts([]string{"newname", "clauductor"})
	if want := []string{"newname.localhost", "clauductor.localhost"}; !slices.Equal(got, want) {
		t.Fatalf("brandHosts = %q, want %q", got, want)
	}
}

// The allow-list is built from the names, not from config.DefaultHostName, so it must still
// hold the host install/open.go and PanelHost send the browser (and the token) to.
func TestAllowedHostsHoldTheHostThePanelIsOpenedAt(t *testing.T) {
	s := &Server{Port: 4321}
	if want := config.DefaultHostName + ":4321"; !slices.Contains(s.allowedHosts(), want) || !s.hostAllowed(want) {
		t.Fatalf("allowedHosts = %q, want it to hold %q", s.allowedHosts(), want)
	}
}

// While the names are one, the allow-list holds the panel's host once, as it always did, and
// it is the host the panel is opened at.
func TestBrandHostsAreUnchangedWhileTheNameIs(t *testing.T) {
	got := brandHosts(brand.Names())
	if want := []string{config.DefaultHostName}; !slices.Equal(got, want) {
		t.Fatalf("brandHosts = %q, want %q", got, want)
	}
	if config.DefaultHostName != "clauductor.localhost" || PanelHost(true) != config.DefaultHostName {
		t.Fatalf("DefaultHostName = %q, PanelHost = %q", config.DefaultHostName, PanelHost(true))
	}
}
