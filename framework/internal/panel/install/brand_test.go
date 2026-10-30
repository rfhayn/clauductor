package install

import "testing"

// The login agent's label and the hook tag are spelled from brand.Name.
func TestLabelAndHookTagAreSpelledFromTheName(t *testing.T) {
	if LaunchdLabel != "com.stridal.panel" || hookTag != "stridal-panel" {
		t.Fatalf("LaunchdLabel = %q, hookTag = %q", LaunchdLabel, hookTag)
	}
}
