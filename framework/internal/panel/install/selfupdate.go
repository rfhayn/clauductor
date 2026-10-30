package install

import (
	"bufio"
	"bytes"
	"context"
	"debug/buildinfo"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/signals"
)

// The panel's rebuild of itself (OPS-118 D4): after catch-up fast-forwards the
// repository whose framework/ is the panel's own source, the login agent's panel builds
// the new commit beside its installed binary, checks the build, and puts it in place
// with two renames, keeping the old one as clauductor.prev. The token, the plist and
// the user's hooks are never opened for writing: `clauductor panel install` would
// rotate the token (E6). The restart that follows is the panel's (run.go).

// RebuildTimeout bounds the build.
const RebuildTimeout = 5 * time.Minute

// UpdateByHand is how a notice says to update a login agent's panel that did not
// rebuild itself. The notice names the checkout, so the commands say framework/, not
// its path: the body is cut at a length a deep checkout's path would otherwise fill.
const UpdateByHand = "To update it, run `go build -o clauductor ./cmd/clauductor && ./clauductor panel install` in framework/."

// preflightTimeout bounds each of the new binary's preflight commands.
const preflightTimeout = 30 * time.Second

// installFiles are what `clauductor panel install` writes beyond the binary: a rebuild
// whose range changed either says to run it once.
var installFiles = []string{"framework/internal/panel/install/launchd.go", "framework/internal/panel/install/hooks.go"}

// BuildStamp is what a Go binary's build information says of its source: the main
// module, the commit it was built at ("" when the build carries no VCS information),
// and whether the checkout had local changes.
type BuildStamp struct {
	Module   string
	Revision string
	Modified bool
}

func stampOf(bi *debug.BuildInfo) BuildStamp {
	s := BuildStamp{Module: bi.Main.Path}
	for _, kv := range bi.Settings {
		switch kv.Key {
		case "vcs.revision":
			s.Revision = kv.Value
		case "vcs.modified":
			s.Modified = kv.Value == "true"
		}
	}
	return s
}

// RunningStamp is the running binary's build stamp; the zero stamp when it has none.
func RunningStamp() BuildStamp {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return BuildStamp{}
	}
	return stampOf(bi)
}

// FileStamp is the build stamp of the binary at path.
func FileStamp(path string) (BuildStamp, error) {
	bi, err := buildinfo.ReadFile(path)
	if err != nil {
		return BuildStamp{}, err
	}
	return stampOf(bi), nil
}

// ModuleOf is the module path the go.mod at path declares.
func ModuleOf(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		line, _, _ := strings.Cut(sc.Text(), "//")
		f := strings.Fields(line)
		if len(f) == 2 && f[0] == "module" {
			return strings.Trim(f[1], "\"`"), nil
		}
	}
	return "", fmt.Errorf("%s declares no module", path)
}

// Rebuild is one rebuild of the panel from the repository at Root, which catch-up
// moved from From to To.
type Rebuild struct {
	Home, Root string
	From, To   string
	// Launchd: the panel runs as the login agent, which alone is rebuilt and restarted.
	Launchd bool
	// Self is the running binary's stamp, and Exe its path (named in a notice).
	Self BuildStamp
	Exe  string
	// Go is the go command, from the panel's PATH ("go") unless a test stands one in.
	Go string
	// PathBinary is the clauductor the panel's PATH names, "" for none: replaced as the
	// installed one is, when its own stamp names this module and a commit of Root.
	PathBinary string
	// Timeout bounds the build: RebuildTimeout when 0.
	Timeout time.Duration
	// Run runs git in Root.
	Run signals.Runner
}

// Plan is Decide's answer: whether the panel rebuilds after the fast-forward, and if
// not, whether and why it says so.
type Plan struct {
	// Ours: the running binary's module is the one Root/framework/go.mod declares, so
	// Root is the panel's own source. When it is not, nothing more is done or said.
	Ours bool
	// Base is the commit the changed range starts from: R, the running binary's
	// commit, when it is a commit of Root and an ancestor of To; else From.
	Base string
	// Changed: framework/ differs between Base and To. When it does not, there is
	// nothing to rebuild and nothing to say.
	Changed bool
	// Skip is why the panel does not rebuild ("" when it does), and How how to update
	// it instead: one notice says both.
	Skip, How string
	// Install names the files `clauductor panel install` writes that Base..To changed.
	Install []string
}

// Rebuilds says the plan is a rebuild.
func (p Plan) Rebuilds() bool { return p.Ours && p.Changed && p.Skip == "" }

// NewBinary is where the build goes: beside the installed binary, renamed into its
// place only after the preflight.
func (r *Rebuild) NewBinary() string { return InstalledBinary(r.Home) + ".new" }

// git runs one git command in Root under the call's own bound.
func (r *Rebuild) git(ctx context.Context, args ...string) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, preflightTimeout)
	defer cancel()
	out, err := r.Run(cctx, r.Root, append([]string{"git", "--no-optional-locks", "-C", r.Root}, args...))
	return strings.TrimSpace(string(out)), err
}

// isCommit says rev names a commit of Root.
func (r *Rebuild) isCommit(ctx context.Context, rev string) bool {
	_, err := r.git(ctx, "cat-file", "-e", rev+"^{commit}")
	return err == nil
}

// Decide is D4's conditions, in order: the module first, silently; then whether
// framework/ changed (over R..B when R can be the base, else over the fast-forward);
// then the login agent, the VCS stamp, R's place in the history, and a clean checkout,
// each failing with the reason the notice gives.
func (r *Rebuild) Decide(ctx context.Context) Plan {
	mod, err := ModuleOf(filepath.Join(r.Root, "framework", "go.mod"))
	if err != nil || r.Self.Module == "" || mod != r.Self.Module {
		return Plan{}
	}
	p := Plan{Ours: true, Base: r.From}
	rev := r.Self.Revision
	revCommit := rev != "" && r.isCommit(ctx, rev)
	revAncestor := false
	if revCommit {
		_, err := r.git(ctx, "merge-base", "--is-ancestor", rev, r.To)
		revAncestor = err == nil
	}
	if revAncestor {
		p.Base = rev
	}
	changed, err := r.git(ctx, "diff", "--name-only", p.Base, r.To, "--", "framework/")
	if err != nil {
		p.Changed = true // not known: said, never taken for unchanged
		p.Skip = fmt.Sprintf("git could not say whether framework/ changed between %s and %s: %v", short(p.Base), short(r.To), err)
		p.How = UpdateByHand
		return p
	}
	if p.Changed = changed != ""; !p.Changed {
		return p
	}
	switch {
	case !r.Launchd:
		exe := r.Exe
		if exe == "" {
			exe = "clauductor"
		}
		p.Skip = "it was started by hand, not as the login agent, so it does not replace itself"
		p.How = fmt.Sprintf("To update it, run `go build -o %s ./cmd/clauductor` in framework/, then stop this panel and start it again.", ShellWord(exe))
		return p
	case rev == "":
		p.Skip = "its binary carries no VCS build information, so its commit is not known"
	case r.Self.Modified:
		p.Skip = "its binary was built from a checkout with local changes"
	case !revCommit:
		p.Skip = "its binary's commit " + short(rev) + " is not a commit of this repository"
	case !revAncestor:
		p.Skip = "its binary's commit " + short(rev) + " is not an ancestor of " + short(r.To) + ", so a build of " + short(r.To) + " would drop what it holds"
	}
	if p.Skip != "" {
		p.How = UpdateByHand
		return p
	}
	st, err := r.git(ctx, "status", "--porcelain")
	switch {
	case err != nil:
		p.Skip = "git status failed, so whether a build would be stamped as modified is not known: " + err.Error()
		p.How = UpdateByHand
		return p
	case st != "":
		lines := strings.Split(st, "\n")
		what := strings.TrimSpace(lines[0])
		if strings.HasPrefix(what, "?? ") {
			what = strings.TrimPrefix(what, "?? ") + " (untracked)"
		}
		if n := len(lines) - 1; n > 0 {
			what += fmt.Sprintf(" and %d more", n)
		}
		p.Skip = "the checkout holds " + what + ", so a build would be stamped as modified"
		p.How = "Move it out of the checkout, or commit it: the next merge into framework/ then rebuilds. " + UpdateByHand
		return p
	}
	if p.Install, err = r.installChanged(ctx, p.Base); err != nil {
		p.Install = installFiles // not known: the notice names them all, never none
	}
	return p
}

// installChanged is the installFiles that base..To changed.
func (r *Rebuild) installChanged(ctx context.Context, base string) ([]string, error) {
	out, err := r.git(ctx, append([]string{"diff", "--name-only", base, r.To, "--"}, installFiles...)...)
	if err != nil || out == "" {
		return nil, err
	}
	return strings.Split(out, "\n"), nil
}

// Build runs `go build -o <NewBinary> ./cmd/clauductor` in Root/framework under the
// timeout, with the go the panel's PATH names. It writes nothing in the repository: the
// output goes beside the installed binary, and Go's caches are outside it. A failure
// leaves no NewBinary.
func (r *Rebuild) Build(ctx context.Context) error {
	out := r.NewBinary()
	_ = os.Remove(out) // a run that stopped mid-build
	if err := config.EnsurePrivateDir(filepath.Dir(out)); err != nil {
		return err
	}
	goCmd := r.Go
	if goCmd == "" {
		goCmd = "go"
	}
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = RebuildTimeout
	}
	path, err := exec.LookPath(goCmd)
	if err != nil {
		return fmt.Errorf("go is not on the panel's PATH (`clauductor panel install` adds its directory): %v", err)
	}
	bctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(bctx, path, "build", "-o", out, "./cmd/clauductor")
	cmd.Dir = filepath.Join(r.Root, "framework")
	cmd.Env = buildEnv()
	// A group of its own, killed whole at the timeout: the compiler and linker go
	// starts would otherwise hold the output open past it.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 5 * time.Second
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	err = cmd.Run()
	if err == nil {
		return nil
	}
	_ = os.Remove(out)
	if errors.Is(bctx.Err(), context.DeadlineExceeded) && ctx.Err() == nil {
		return fmt.Errorf("the build took over %s", timeout)
	}
	if msg := buildMessage(buf.String()); msg != "" {
		return fmt.Errorf("go build: %v: %s", err, msg)
	}
	return fmt.Errorf("go build: %v", err)
}

// buildEnv is the panel's environment without what would point git, and so Go's VCS
// stamp, at a repository other than the one being built.
func buildEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if k != "GIT_DIR" && k != "GIT_WORK_TREE" && k != "GIT_INDEX_FILE" {
			env = append(env, kv)
		}
	}
	return env
}

// buildMessage is go build's output as one line: its errors, without the "# package"
// headers, at most the first three.
func buildMessage(s string) string {
	var keep []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "#") {
			keep = append(keep, l)
		}
	}
	if len(keep) > 3 {
		keep = append(keep[:3], "…")
	}
	return strings.Join(keep, "; ")
}

// Preflight checks the new binary: its stamp names this module, To and no local
// changes, so the next merge can rebuild again; then its `version` and `panel list`
// (which reads projects.json) each exit 0, run with HOME the panel's. A failure
// removes NewBinary.
func (r *Rebuild) Preflight(ctx context.Context) error {
	err := r.preflight(ctx)
	if err != nil {
		r.Discard()
	}
	return err
}

func (r *Rebuild) preflight(ctx context.Context) error {
	bin := r.NewBinary()
	s, err := FileStamp(bin)
	if err != nil {
		return fmt.Errorf("the new binary's build information: %v", err)
	}
	if s.Module != r.Self.Module || s.Revision != r.To || s.Modified {
		return fmt.Errorf("the new binary is stamped %s at %s (modified: %t), not %s at %s with no local changes",
			s.Module, orNone(short(s.Revision)), s.Modified, r.Self.Module, short(r.To))
	}
	for _, args := range [][]string{{"version"}, {"panel", "list"}} {
		if err := r.runNew(ctx, args...); err != nil {
			return err
		}
	}
	return nil
}

// runNew runs the new binary with args under the preflight's bound.
func (r *Rebuild) runNew(ctx context.Context, args ...string) error {
	cctx, cancel := context.WithTimeout(ctx, preflightTimeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, r.NewBinary(), args...)
	cmd.Dir = r.Home
	cmd.Env = append(buildEnv(), "HOME="+r.Home)
	cmd.WaitDelay = signals.HeldOutputWait
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	if errors.Is(cctx.Err(), context.DeadlineExceeded) && ctx.Err() == nil {
		return fmt.Errorf("the new binary's `%s` gave no answer within %s", strings.Join(args, " "), preflightTimeout)
	}
	if msg := buildMessage(string(out)); msg != "" {
		return fmt.Errorf("the new binary's `%s`: %v: %s", strings.Join(args, " "), err, msg)
	}
	return fmt.Errorf("the new binary's `%s`: %v", strings.Join(args, " "), err)
}

// Discard removes NewBinary: a build that will not be swapped leaves none.
func (r *Rebuild) Discard() { _ = os.Remove(r.NewBinary()) }

// Swap puts NewBinary in place of the installed binary with two renames, the old one
// kept as clauductor.prev, then does the same for PathBinary when its own stamp names
// this module and a commit of Root, copying the new binary beside it first. err is the
// installed binary's swap, which leaves it as it was and no NewBinary; pathErr the
// PATH one's, which leaves that one as it was. replaced is the PATH binary replaced,
// "" for none.
func (r *Rebuild) Swap(ctx context.Context) (replaced string, pathErr, err error) {
	bin, nb := InstalledBinary(r.Home), r.NewBinary()
	prev := bin + ".prev"
	if err := os.Rename(bin, prev); err != nil {
		r.Discard()
		return "", nil, err
	}
	if err := os.Rename(nb, bin); err != nil {
		if rerr := os.Rename(prev, bin); rerr != nil {
			err = fmt.Errorf("%v, and putting the old binary back failed too: %v", err, rerr)
		}
		r.Discard()
		return "", nil, err
	}
	// The installed binary is replaced: the PATH one's check runs to its answer under
	// its own bound (git's), since a stop that read as "not this module's" would leave
	// it behind with no word.
	if !r.pathQualifies(context.WithoutCancel(ctx), bin) {
		return "", nil, nil
	}
	p := r.PathBinary
	staged := p + ".new"
	if err := copyExecutable(bin, staged); err != nil {
		_ = os.Remove(staged)
		return "", err, nil
	}
	if err := os.Rename(p, p+".prev"); err != nil {
		_ = os.Remove(staged)
		return "", err, nil
	}
	if err := os.Rename(staged, p); err != nil {
		_ = os.Rename(p+".prev", p)
		_ = os.Remove(staged)
		return "", err, nil
	}
	return p, nil, nil
}

// copyExecutable copies src to dst, mode 0755. Not copyFileAtomic: that one makes its
// directory private, and the PATH's directory is the owner's, never re-permissioned.
func copyExecutable(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	return err
}

// pathQualifies says PathBinary is to be replaced too: a file of its own (not the
// installed binary) whose stamp names this module and a commit of Root.
func (r *Rebuild) pathQualifies(ctx context.Context, installed string) bool {
	if r.PathBinary == "" {
		return false
	}
	pi, err := os.Stat(r.PathBinary)
	if err != nil {
		return false
	}
	if ii, err := os.Stat(installed); err == nil && os.SameFile(pi, ii) {
		return false
	}
	s, err := FileStamp(r.PathBinary)
	return err == nil && s.Module == r.Self.Module && s.Revision != "" && r.isCommit(ctx, s.Revision)
}

// RestartMarker is what the panel leaves for the binary that starts after its restart:
// that binary's first start says it is running (D4), and removes it.
type RestartMarker struct {
	Root string `json:"root"`
	From string `json:"from"` // R
	To   string `json:"to"`   // B
}

// RestartMarkerPath is the marker's file.
func RestartMarkerPath(home string) string { return filepath.Join(config.PanelDir(home), "restarted") }

// WriteRestartMarker leaves m for the next start.
func WriteRestartMarker(home string, m RestartMarker) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	path := RestartMarkerPath(home)
	if err := config.EnsurePrivateDir(filepath.Dir(path)); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// TakeRestartMarker reads and removes the marker: nil when there is none. It is
// removed before it is acted on, so one restart is said once, even by a start that
// fails while saying it.
func TakeRestartMarker(home string) (*RestartMarker, error) {
	path := RestartMarkerPath(home)
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := os.Remove(path); err != nil {
		return nil, err
	}
	var m RestartMarker
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("%s: %v", path, err)
	}
	return &m, nil
}

// Tilde is p as one shell word with the home directory written as ~, as a notice's
// command names it. The ~/ stays outside the quotes: a shell expands only a bare one.
func Tilde(home, p string) string {
	if home != "" && strings.HasPrefix(p, home+string(filepath.Separator)) {
		return "~/" + ShellWord(p[len(home)+1:])
	}
	return ShellWord(p)
}

// short is a commit's first 12 hex digits.
func short(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}

func orNone(s string) string {
	if s == "" {
		return "no commit"
	}
	return s
}
