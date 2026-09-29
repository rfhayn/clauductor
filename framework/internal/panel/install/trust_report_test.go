package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/clauductor/clauductor/internal/panel/signals"
)

// Trusting a config (`panel trust`, and `panel install`, which trusts what it
// installs) prints the hash it recorded and every command and prompt that trusts.
func TestTrustSaysWhatItTrusts(t *testing.T) {
	t.Parallel()
	home, root := t.TempDir(), signals.ResolvePath(t.TempDir())
	cfg := filepath.Join(root, "panel.json")
	os.WriteFile(cfg, []byte(`{"name":"T","version":2,"lanes":{"fix/":"fix"},
		"cards":[{"id":"todo","command":["sh","-c","grep -rn TODO src"],"refresh":"interval:60"}],
		"queues":[{"id":"gate","lock":"clauductor/gate.lock","command":["make","ci"]},{"id":"bare","lock":"x.lock"}],
		"templates":[{"id":"fix","lane_type":"fix","first_prompt":"Fix issue {issue}"}]}`), 0o644)
	h, runs, err := TrustConfigReport(home, root, cfg)
	if err != nil || !TrustedNow(home, root, cfg, h) {
		t.Fatalf("not trusted: %v", err)
	}
	var b strings.Builder
	PrintTrusted(&b, cfg, h, runs)
	out := b.String()
	for _, want := range []string{h[:12], `card todo runs ["sh" "-c" "grep -rn TODO src"]`, `queue gate runs ["make" "ci"] on RUN`,
		`template fix types "Fix issue {issue}"`} {
		if !strings.Contains(out, want) {
			t.Errorf("the trust report lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "queue bare") {
		t.Errorf("a queue with no command runs nothing:\n%s", out)
	}
}
