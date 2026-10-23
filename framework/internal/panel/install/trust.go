package install

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
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
	// Notes are the notes records were made with, by config path, each read only while
	// its hash is the one recorded (OPS-118: "trusted on arrival from #7"), so a runtime
	// started on the record later, and the page's trust report, say how it was trusted.
	// An older binary skips the field and drops it when it writes: the note goes, never
	// the trust.
	Notes map[string]trustNote `json:"notes,omitempty"`
}

// trustNote is the note a record was made with, and the hash it was made for.
type trustNote struct {
	Hash string `json:"hash"`
	Note string `json:"note"`
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
//
// A config outside its repository (a sidecar, design.md D5) is also judged by the
// sidecar rule, on the bytes at cfgPath that hash to hash: while any command holds a
// word the rule refuses, it is not trusted, nothing is recorded, and a record made
// earlier (by hand, or before the rule) does not count. With trustNow that is an
// error, a *SidecarRefusedError.
func CheckTrust(home, project, cfgPath, hash string, trustNow bool) (config.TrustView, error) {
	return checkTrust(home, project, cfgPath, hash, trustNow, "")
}

// checkTrust is CheckTrust, a record it makes carrying note ("" for none, which drops
// the note an earlier record of the path carried). A record already holding hash
// returns its own note, and is not rewritten.
func checkTrust(home, project, cfgPath, hash string, trustNow bool, note string) (config.TrustView, error) {
	project, cfgPath = signals.ResolvePath(project), signals.ResolvePath(cfgPath)
	tv := config.TrustView{Hash: hash, Path: cfgPath}
	if err := sidecarCheck(project, cfgPath, hash); err != nil {
		tv.Note = err.Error()
		if trustNow {
			return tv, err
		}
		return tv, nil
	}
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
		if n, ok := t.Notes[cfgPath]; ok && n.Hash == hash {
			tv.Note = n.Note
		}
		return tv, nil
	case trustNow && note != "":
		tv.Note = note
	case trustNow && !seen:
		tv.Note = "trusted and recorded"
	case trustNow:
		tv.Note = "changed config trusted"
	default:
		return tv, nil
	}
	t.Configs[cfgPath] = hash
	delete(t.Notes, cfgPath)
	if note != "" {
		if t.Notes == nil {
			t.Notes = map[string]trustNote{}
		}
		t.Notes[cfgPath] = trustNote{Hash: hash, Note: note}
	}
	if err := writeTrust(path, t); err != nil {
		return tv, err
	}
	tv.Trusted = true
	return tv, nil
}

// TrustArrived records the config at cfgPath as trusted with note ("trusted on arrival
// from #7", OPS-118 D1) only if its bytes still hash to want, as TrustExact does, but
// without parsing them: when the merge that brought them also rebuilds the panel, the
// binary that will serve them read them (`panel config-check`), and the running one may
// not. So it trusts only a config inside its repository, which the sidecar rule, a
// parse, never judges; catch-up never reaches another.
func TrustArrived(home, project, cfgPath, want, note string) (config.TrustView, error) {
	project = signals.ResolvePath(project)
	if configOutside(project, cfgPath) {
		return config.TrustView{}, fmt.Errorf("%s is outside its repository, so it is not trusted on arrival", cfgPath)
	}
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		return config.TrustView{}, err
	}
	if h := ConfigHash(raw); h != want {
		return config.TrustView{Hash: h}, ErrConfigChanged
	}
	return checkTrust(home, project, cfgPath, want, true, note)
}

// UntrustArrived withdraws a trust TrustArrived recorded: the record of cfgPath goes
// back to prev, the hash recorded before it ("" for none, which removes it), while the
// record is still hash with note, so a trust made since (the owner's, or a later
// arrival's) stands. It says whether it withdrew. A rebuild's new binary judged the
// bytes, and when that binary is not put in place after all (a stop before the swap,
// a swap that failed) the binary that next serves them never did (OPS-118 D1). prev's
// own note, if it had one, is not restored: its trust stands, unannotated.
func UntrustArrived(home, project, cfgPath, hash, note, prev string) (bool, error) {
	project, cfgPath = signals.ResolvePath(project), signals.ResolvePath(cfgPath)
	path := TrustPath(home, project)
	t, err := readTrust(path)
	if err != nil {
		return false, err
	}
	if t.Configs[cfgPath] != hash || t.Notes[cfgPath] != (trustNote{Hash: hash, Note: note}) {
		return false, nil
	}
	if prev == "" {
		delete(t.Configs, cfgPath)
	} else {
		t.Configs[cfgPath] = prev
	}
	delete(t.Notes, cfgPath)
	if err := writeTrust(path, t); err != nil {
		return false, err
	}
	return true, nil
}

// TrustConfig records the config at cfgPath as trusted (`clauductor panel trust`).
func TrustConfig(home, project, cfgPath string) (string, error) {
	h, _, err := TrustConfigReport(home, project, cfgPath)
	return h, err
}

// TrustConfigReport trusts the config and also returns what it runs (TrustRuns), for
// the command to print. A refused sidecar returns its marked lines with the error, so
// the command can print them (PrintRefused).
func TrustConfigReport(home, project, cfgPath string) (string, []string, error) {
	project = signals.ResolvePath(project)
	if cfgPath == "" {
		cfgPath = config.OwnConfigPath(project)
	}
	cfg, raw, err := config.LoadConfigRaw(cfgPath)
	if err != nil {
		return "", nil, err
	}
	h := ConfigHash(raw)
	runs := TrustRuns(project, cfgPath, cfg)
	if _, err := CheckTrust(home, project, cfgPath, h, true); err != nil {
		var refused *SidecarRefusedError
		if errors.As(err, &refused) {
			return h, runs, err
		}
		return "", nil, err
	}
	return h, runs, nil
}

// TrustRuns is a config's trust report: RunList, with each line the sidecar rule
// refuses marked NOT ALLOWED IN A SIDECAR when the config is outside its repository
// (design.md D5). Every report reads it: `panel trust`, the page's Trust config… and
// the add dialog.
func TrustRuns(project, cfgPath string, cfg *config.Config) []string {
	if !configOutside(project, cfgPath) {
		return cfg.RunList()
	}
	return cfg.SidecarRunList(signals.ResolvePath(project))
}

// SidecarRefusedError: a config outside its repository is not trusted, because its
// commands hold words the sidecar rule refuses (Lines), or because the bytes at its
// path cannot be judged: they do not read, do not hash to the hash given, or do not
// parse (Err).
type SidecarRefusedError struct {
	Path  string
	Lines []config.SidecarLine
	Err   error
}

func (e *SidecarRefusedError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s is outside its repository, and its commands cannot be judged: %v", e.Path, e.Err)
	}
	return fmt.Sprintf("%s is outside its repository, and %d of its commands are NOT ALLOWED IN A SIDECAR (each is marked in its trust report): %s",
		e.Path, len(e.Lines), config.SidecarRemedy)
}

func (e *SidecarRefusedError) Unwrap() error { return e.Err }

// sidecarCheck is nil for a config inside its repository (resolved), and for one
// outside it whose bytes at cfgPath hash to hash and hold only commands the sidecar
// rule allows. project is resolved.
func sidecarCheck(project, cfgPath, hash string) error {
	if !configOutside(project, cfgPath) {
		return nil
	}
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		return &SidecarRefusedError{Path: cfgPath, Err: err}
	}
	if ConfigHash(raw) != hash {
		return &SidecarRefusedError{Path: cfgPath, Err: ErrConfigChanged}
	}
	cfg, err := config.ParseConfig(raw)
	if err != nil {
		return &SidecarRefusedError{Path: cfgPath, Err: err}
	}
	if lines := cfg.SidecarRefusals(project); len(lines) > 0 {
		return &SidecarRefusedError{Path: cfgPath, Lines: lines}
	}
	return nil
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

// PrintRefused says why a config outside its repository was not trusted, with its
// report: each line the sidecar rule refuses is marked.
func PrintRefused(w io.Writer, cfgPath, hash string, runs []string) {
	fmt.Fprintf(w, "NOT trusted: panel config %s (sha256 %s) is outside its repository, and the lines marked below hold commands a sidecar may not. It would run:\n",
		cfgPath, config.ShortHash(hash))
	for _, r := range runs {
		fmt.Fprintln(w, "  "+r)
	}
}

// TrustedNow re-reads the trust file: has the loaded config been trusted since? A
// config outside its repository must also pass the sidecar rule (CheckTrust).
func TrustedNow(home, project, cfgPath, hash string) bool {
	project, cfgPath = signals.ResolvePath(project), signals.ResolvePath(cfgPath)
	t, err := readTrust(TrustPath(home, project))
	return err == nil && t.Configs[cfgPath] == hash && sidecarCheck(project, cfgPath, hash) == nil
}
