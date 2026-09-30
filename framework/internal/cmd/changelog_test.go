package cmd

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// A Version bump without a CHANGELOG section would ship a release whose notes are empty: the
// release workflow takes its notes from that section (scripts/release-check.sh, docs/release.md).
// Failing here, on every PR, catches it long before a tag does.
func TestChangelogHasCurrentVersion(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "CHANGELOG.md"))
	if err != nil {
		t.Fatalf("read CHANGELOG.md at the repository root: %v", err)
	}
	if !regexp.MustCompile(`(?m)^## \[Unreleased\]`).Match(b) {
		t.Error("CHANGELOG.md has no '## [Unreleased]' section; scripts/changelog.sh needs it")
	}
	header := regexp.MustCompile(`(?m)^## \[` + regexp.QuoteMeta(Version) + `\]( - .+)?$`)
	if !header.Match(b) {
		t.Errorf("CHANGELOG.md has no '## [%s]' section for the current Version (framework/internal/cmd/root.go)", Version)
	}
}
