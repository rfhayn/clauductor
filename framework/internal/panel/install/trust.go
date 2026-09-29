package install

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/signals"
)

// panel.json lives in the repository and names argv the panel runs (cards, queue
// commands) and prompts it types into lanes (templates). A pull can change it. This
// is the panel's equivalent of workspace trust: the config's SHA-256 is recorded
// when you first run it, and a DIFFERENT config runs none of its commands or
// templates until you trust it again (`clauductor panel trust`, or --trust-config).
// Lanes, terminals and every read-only source keep working meanwhile.
//
// Trusted hashes live in ~/.clauductor/panel/<project hash>/trusted-config.json,
// keyed by the config's absolute path.

type trustFile struct {
	Configs map[string]string `json:"configs"` // absolute config path → sha256
}

// ConfigHash is the hex SHA-256 of the config bytes.
func ConfigHash(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// TrustPath is where a project's trusted config hashes live.
func TrustPath(home, project string) string {
	return filepath.Join(config.ProjectDir(home, project), "trusted-config.json")
}

func readTrust(path string) (trustFile, error) {
	t := trustFile{Configs: map[string]string{}}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return t, nil
	}
	if err != nil {
		return t, err
	}
	if err := json.Unmarshal(b, &t); err != nil {
		return t, fmt.Errorf("%s is not valid JSON: %w", path, err)
	}
	if t.Configs == nil {
		t.Configs = map[string]string{}
	}
	return t, nil
}

func writeTrust(path string, t trustFile) error {
	if err := config.EnsurePrivateDir(filepath.Dir(path)); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(t, "", "  ") // map keys marshal sorted
	return config.WriteAtomic(path, append(b, '\n'), 0o600)
}

// CheckTrust decides whether a config may run its commands. Only a config whose
// exact bytes were trusted may: one never seen before is not (a clone, or a pull
// that adds panel.json, names commands nobody reviewed), nor is a changed one. Either
// becomes trusted only when trustNow is set (`clauductor panel trust`,
// --trust-config, `panel install`), which records it.
func CheckTrust(home, project, cfgPath, hash string, trustNow bool) (config.TrustView, error) {
	project, cfgPath = signals.ResolvePath(project), signals.ResolvePath(cfgPath)
	tv := config.TrustView{Hash: hash, Path: cfgPath}
	path := TrustPath(home, project)
	t, err := readTrust(path)
	if err != nil {
		return tv, err
	}
	prev, seen := t.Configs[cfgPath]
	tv.Prev = prev
	switch {
	case seen && prev == hash:
		tv.Trusted = true
		return tv, nil
	case trustNow && !seen:
		tv.Note = "trusted and recorded"
	case trustNow:
		tv.Note = "changed config trusted"
	default:
		return tv, nil
	}
	t.Configs[cfgPath] = hash
	if err := writeTrust(path, t); err != nil {
		return tv, err
	}
	tv.Trusted = true
	return tv, nil
}

// TrustConfig records the config at cfgPath as trusted (`clauductor panel trust`).
func TrustConfig(home, project, cfgPath string) (string, error) {
	h, _, err := TrustConfigReport(home, project, cfgPath)
	return h, err
}

// TrustConfigReport trusts the config and also returns what it runs
// (config.RunList), for the command to print.
func TrustConfigReport(home, project, cfgPath string) (string, []string, error) {
	project = signals.ResolvePath(project)
	if cfgPath == "" {
		cfgPath = filepath.Join(project, config.DefaultConfigRel)
	}
	cfg, raw, err := config.LoadConfigRaw(cfgPath)
	if err != nil {
		return "", nil, err
	}
	h := ConfigHash(raw)
	if _, err := CheckTrust(home, project, cfgPath, h, true); err != nil {
		return "", nil, err
	}
	return h, cfg.RunList(), nil
}

// PrintTrusted says what was trusted: the hash and every command and prompt.
func PrintTrusted(w io.Writer, cfgPath, hash string, runs []string) {
	fmt.Fprintf(w, "Trusted panel config %s (sha256 %s). It runs:\n", cfgPath, config.ShortHash(hash))
	if len(runs) == 0 {
		fmt.Fprintln(w, "  nothing: no cards, queue commands or templates")
	}
	for _, r := range runs {
		fmt.Fprintln(w, "  "+r)
	}
}

// TrustedNow re-reads the trust file: has the loaded config been trusted since?
func TrustedNow(home, project, cfgPath, hash string) bool {
	t, err := readTrust(TrustPath(home, signals.ResolvePath(project)))
	return err == nil && t.Configs[signals.ResolvePath(cfgPath)] == hash
}
