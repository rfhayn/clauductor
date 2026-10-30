package cmd

import "testing"

// The markers are spelled from brand.Name: what install and the plugin's init write.
func TestMarkersAreSpelledFromTheName(t *testing.T) {
	if installMarker != ".claude/stridal-template" || pluginMarker != ".claude/stridal-plugin" {
		t.Fatalf("installMarker = %q, pluginMarker = %q", installMarker, pluginMarker)
	}
}
