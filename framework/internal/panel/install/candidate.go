package install

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/signals"
)

// PANEL-22: the page's "Add a project…" names a path, and the panel checks it before
// anything is written: what Inspect finds is what the dialog shows (the resolved root,
// git's common dir, the config and what trusting it would run), and every refusal is
// one `panel add` makes too. Inspect runs git's read-only plumbing (rev-parse and
// `worktree list`) and reads files; it never runs anything the repository names.

// Refusal is a path the panel will not register, with the reason the page shows.
type Refusal struct {
	Code string // not-absolute, not-found, not-dir, unreadable, not-git, linked-worktree, registered, socket-taken, bad-config
	Msg  string
}

func (r *Refusal) Error() string { return r.Msg }

func refuse(code, format string, a ...any) error {
	return &Refusal{Code: code, Msg: fmt.Sprintf(format, a...)}
}

// Candidate is a repository the page may add: what Inspect found.
type Candidate struct {
	Input      string `json:"input"`      // the path as typed
	Root       string `json:"root"`       // its main worktree, resolved
	CommonDir  string `json:"commonDir"`  // git's common dir, resolved
	ConfigPath string `json:"configPath"` // <root>/.clauductor/panel.json
	HasConfig  bool   `json:"hasConfig"`
	// ConfigError is why the config does not load; the project cannot be added until
	// it does.
	ConfigError string `json:"configError,omitempty"`
	ID          string `json:"id"`     // the id it would get
	Name        string `json:"name"`   // the config's name ("" without a config)
	Socket      string `json:"socket"` // the tmux socket its lanes would use
	// The trust report (`panel trust` prints the same): the exact bytes' hash and
	// every command and prompt they name. Trusted: those bytes are trusted already.
	Hash    string   `json:"hash,omitempty"`
	Runs    []string `json:"runs"`
	Trusted bool     `json:"trusted"`
	Prev    string   `json:"prev,omitempty"` // the hash trusted before, when it changed
}

// MaxPathLen bounds a path the page sends.
const MaxPathLen = 4096

// ExpandPath turns what someone typed into an absolute, clean path: ~ and ~/… are
// the home directory; anything else must be absolute already. Nothing is resolved.
func ExpandPath(home, p string) (string, error) {
	p = strings.TrimSpace(p)
	switch {
	case p == "":
		return "", refuse("not-absolute", "type the repository's path")
	case len(p) > MaxPathLen || strings.ContainsRune(p, 0):
		return "", refuse("not-absolute", "that is not a path")
	case p == "~":
		p = home
	case strings.HasPrefix(p, "~/"):
		p = filepath.Join(home, p[2:])
	case strings.HasPrefix(p, "~"):
		return "", refuse("not-absolute", "~user is not supported: type the full path")
	}
	if !filepath.IsAbs(p) {
		return "", refuse("not-absolute", "%s is not an absolute path: start it with / or ~/", p)
	}
	return filepath.Clean(p), nil
}

// InspectOptions names the path to check against the machine's registry.
type InspectOptions struct {
	Home string
	Path string
	Run  signals.Runner
}

// gitOut runs one read-only git command in dir.
func gitOut(ctx context.Context, run signals.Runner, dir string, args ...string) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := run(cctx, dir, append([]string{"git"}, args...))
	return strings.TrimSpace(string(out)), err
}

// commonDir is git's common dir for the repository at dir, resolved: two paths with
// one common dir are one repository.
func commonDir(ctx context.Context, run signals.Runner, dir string) (string, error) {
	d, err := gitOut(ctx, run, dir, "rev-parse", "--git-common-dir")
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(d) {
		d = filepath.Join(dir, d)
	}
	return signals.ResolvePath(d), nil
}

// mainRoot resolves a path to the repository's main worktree, or refuses it the way
// `panel add` does: not a directory, not readable, not in git, or a linked worktree.
func mainRoot(ctx context.Context, run signals.Runner, path string) (root string, err error) {
	fi, err := os.Stat(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return "", refuse("not-found", "%s does not exist", path)
	case errors.Is(err, os.ErrPermission):
		return "", refuse("unreadable", "%s cannot be read by the panel's user", path)
	case err != nil:
		return "", refuse("not-found", "%s: %v", path, err)
	case !fi.IsDir():
		return "", refuse("not-dir", "%s is not a directory", path)
	}
	resolved := signals.ResolvePath(path)
	f, err := os.Open(resolved)
	if err != nil {
		return "", refuse("unreadable", "%s cannot be read by the panel's user", path)
	}
	_, rerr := f.Readdirnames(1)
	f.Close()
	if rerr != nil && !errors.Is(rerr, io.EOF) {
		return "", refuse("unreadable", "%s cannot be read by the panel's user", path)
	}
	top, err := gitOut(ctx, run, resolved, "rev-parse", "--show-toplevel")
	if err != nil || top == "" {
		return "", refuse("not-git", "%s is not in a git repository with a working tree", path)
	}
	top = signals.ResolvePath(top)
	wts, err := signals.ReadWorktrees(ctx, run, top)
	if err != nil || len(wts) == 0 {
		return "", refuse("not-git", "git lists no worktree for %s", top)
	}
	if main := wts[0].Path; main != top {
		return "", refuse("linked-worktree", "%s is a linked worktree of %s; add that instead (a project is its main worktree)", top, main)
	}
	return top, nil
}

// registeredAs says which registered project is this repository already: the same
// root, or the same git common dir (a second path to one repository). "" for none.
func registeredAs(ctx context.Context, run signals.Runner, reg *config.Projects, root, common string) string {
	if e := reg.Find(root); e != nil {
		return e.ID
	}
	for _, e := range reg.Projects {
		if c, err := commonDir(ctx, run, e.Root); err == nil && c == common {
			return e.ID
		}
	}
	return ""
}

// socketOwner is the registered project whose lanes already use sock, or "".
func socketOwner(reg *config.Projects, sock, except string) string {
	for _, other := range reg.Projects {
		if other.ID == except {
			continue
		}
		oc, _ := config.LoadConfig(other.ConfigPath())
		if other.Socket(oc) == sock {
			return other.ID
		}
	}
	return ""
}

// Inspect checks a path the page names, against the registry, and says what adding
// it would do. A refusal is a *Refusal. It writes nothing and runs nothing the
// repository names.
func Inspect(ctx context.Context, o InspectOptions) (Candidate, error) {
	if o.Run == nil {
		o.Run = signals.ExecRunner
	}
	c := Candidate{Input: o.Path, Runs: []string{}}
	path, err := ExpandPath(o.Home, o.Path)
	if err != nil {
		return c, err
	}
	if c.Root, err = mainRoot(ctx, o.Run, path); err != nil {
		return c, err
	}
	if c.CommonDir, err = commonDir(ctx, o.Run, c.Root); err != nil {
		return c, refuse("not-git", "git cannot name the repository's common dir: %v", err)
	}
	reg, err := config.LoadProjects(o.Home)
	if err != nil {
		return c, err
	}
	if id := registeredAs(ctx, o.Run, reg, c.Root, c.CommonDir); id != "" {
		return c, refuse("registered", "%s is registered already, as %q", c.Root, id)
	}
	c.ConfigPath = filepath.Join(c.Root, config.DefaultConfigRel)
	var cfg *config.Config
	if _, err := os.Lstat(c.ConfigPath); err == nil {
		c.HasConfig = true
		var raw []byte
		cfg, raw, err = config.LoadConfigRaw(c.ConfigPath)
		if err != nil {
			c.ConfigError = signals.Clip(err.Error(), 400)
			cfg = nil
		} else {
			c.Name = cfg.Name
			c.Hash = ConfigHash(raw)
			c.Runs = append(c.Runs, cfg.RunList()...)
			tv, terr := CheckTrust(o.Home, c.Root, c.ConfigPath, c.Hash, false)
			c.Trusted, c.Prev = terr == nil && tv.Trusted, tv.Prev
		}
	}
	name := c.Name
	if name == "" {
		name = filepath.Base(c.Root)
	}
	c.ID = reg.Slug(name)
	e := config.ProjectEntry{ID: c.ID, Root: c.Root, TmuxSocket: reg.NewSocket(c.ID)}
	c.Socket = e.Socket(cfg)
	if other := socketOwner(reg, c.Socket, ""); other != "" {
		return c, refuse("socket-taken", "tmux socket %q is %s's already; give this project its own tmux_socket in %s", c.Socket, other, c.ConfigPath)
	}
	return c, nil
}

// ErrConfigChanged: the config's bytes are not the ones the page showed.
var ErrConfigChanged = errors.New("the config changed since you read it: read its trust report again")

// TrustExact trusts the config at cfgPath only if its bytes still hash to want: the
// page's "Trust and add" trusts exactly what its report showed, never a file edited
// between the report and the click.
func TrustExact(home, project, cfgPath, want string) (config.TrustView, error) {
	_, raw, err := config.LoadConfigRaw(cfgPath)
	if err != nil {
		return config.TrustView{}, err
	}
	if h := ConfigHash(raw); h != want {
		return config.TrustView{Hash: h}, ErrConfigChanged
	}
	return CheckTrust(home, project, cfgPath, want, true)
}
