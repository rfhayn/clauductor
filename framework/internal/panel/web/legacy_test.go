package web

import (
	"slices"
	"testing"

	"github.com/rfhayn/stridal/internal/brand"
	"github.com/rfhayn/stridal/internal/panel/config"
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

// After the rename (OPS-104), the allow-list holds the panel's host, then the legacy name's,
// and the new one is the host the panel is opened at.
func TestBrandHostsHoldTheNewHostFirst(t *testing.T) {
	got := brandHosts(brand.Names())
	if want := []string{config.DefaultHostName, "clauductor.localhost"}; !slices.Equal(got, want) {
		t.Fatalf("brandHosts = %q, want %q", got, want)
	}
	if config.DefaultHostName != "stridal.localhost" || PanelHost(true) != config.DefaultHostName {
		t.Fatalf("DefaultHostName = %q, PanelHost = %q", config.DefaultHostName, PanelHost(true))
	}
}
