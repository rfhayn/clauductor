package lanes

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/clauductor/clauductor/internal/panel/config"
)

// loadConfig reads panel.json bytes the way the panel does: from a file.
func loadConfig(t *testing.T, b []byte) (*config.Config, error) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "panel.json")
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return config.LoadConfig(p)
}
