package install

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/clauductor/clauductor/internal/panel/signals"
)

func TestConfigTrust(t *testing.T) {
	t.Parallel()
	home, root := t.TempDir(), signals.ResolvePath(t.TempDir())
	cfg := filepath.Join(root, "panel.json")
	// A config never seen is not trusted by being run, and running it records nothing.
	tv, err := CheckTrust(home, root, cfg, "h1", false)
	if err != nil || tv.Trusted || tv.Prev != "" || TrustedNow(home, root, cfg, "h1") {
		t.Fatalf("an unseen config must not be trusted by running it: %+v %v", tv, err)
	}
	if tv, _ := CheckTrust(home, root, cfg, "h1", true); !tv.Trusted || tv.Note == "" {
		t.Fatalf("trusting it (`panel trust`) records it: %+v", tv)
	}
	if tv, _ := CheckTrust(home, root, cfg, "h1", false); !tv.Trusted {
		t.Fatal("unchanged config untrusted")
	}
	tv, _ = CheckTrust(home, root, cfg, "h2", false)
	if tv.Trusted || tv.Prev != "h1" {
		t.Fatalf("a changed config must not be trusted silently: %+v", tv)
	}
	if TrustedNow(home, root, cfg, "h2") {
		t.Fatal("an untrusted check recorded the new hash")
	}
	if tv, _ := CheckTrust(home, root, cfg, "h2", true); !tv.Trusted || !TrustedNow(home, root, cfg, "h2") {
		t.Fatalf("--trust-config: %+v", tv)
	}
	fi, err := os.Stat(TrustPath(home, root))
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("trust file mode: %v %v", fi, err)
	}
	// The hash covers the exact bytes.
	if ConfigHash([]byte("a")) == ConfigHash([]byte("a ")) {
		t.Fatal("hash ignores bytes")
	}
}
