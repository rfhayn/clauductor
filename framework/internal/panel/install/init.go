package install

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/clauductor/clauductor/internal/panel/types"
)

// `clauductor panel init` writes a starter .clauductor/panel.json from what the
// repository already says: its root, its default branch, where its worktrees live,
// the prefixes its branches use, and a gate script it defines. It writes no card,
// since a card's command runs by itself on every refresh; the only command it may
// write is a queue's, which runs only when someone presses RUN, and it prints it.

// InitResult is what InitConfig wrote, and a line of explanation per choice.
type InitResult struct {
	Path  string
	Body  []byte
	Notes []string
}

// ErrConfigExists is returned when the project already has a panel config.
var ErrConfigExists = errors.New("already exists")

// starter is the file init writes, in the order a reader wants it.
type starter struct {
	Schema      string              `json:"$schema"`
	Name        string              `json:"name"`
	Version     int                 `json:"version"`
	Lanes       map[string]string   `json:"lanes"`
	WorktreeDir string              `json:"worktree_dir"`
	Base        string              `json:"base"`
	Queues      []types.QueueConfig `json:"queues,omitempty"`
}

// InitConfig writes <git toplevel of dir>/.clauductor/panel.json. It refuses to
// overwrite any file already there.
func InitConfig(ctx context.Context, run signals.Runner, dir string) (InitResult, error) {
	git := func(args ...string) (string, error) {
		out, err := run(ctx, dir, append([]string{"git"}, args...))
		return strings.TrimSpace(string(out)), err
	}
	root, err := git("rev-parse", "--show-toplevel")
	if err != nil {
		return InitResult{}, fmt.Errorf("not inside a git repository (%v); pass --project <path>", err)
	}
	root = signals.ResolvePath(root)
	dir = root
	path := filepath.Join(root, config.DefaultConfigRel)
	if _, err := os.Lstat(path); err == nil {
		return InitResult{Path: path}, fmt.Errorf("%s %w; edit it, or remove it to start again", path, ErrConfigExists)
	}
	var notes []string
	s := starter{Schema: config.SchemaURL, Version: config.LatestVersion}

	s.Name = filepath.Base(root)
	if config.TypableText(s.Name, 80) != nil || strings.HasPrefix(s.Name, "-") {
		s.Name = "My Project"
	}
	notes = append(notes, fmt.Sprintf("name          %q, shown in the top bar", s.Name))

	defBranch, base, why := detectBase(git)
	s.Base = base
	notes = append(notes, fmt.Sprintf("base          %s: new lanes branch from it (%s)", base, why))

	s.Lanes, why = detectLanes(git, defBranch)
	notes = append(notes, "lanes         "+describeLanes(s.Lanes)+" ("+why+")")

	s.WorktreeDir, why = detectWorktreeDir(ctx, run, root)
	notes = append(notes, fmt.Sprintf("worktree_dir  %s (%s)", s.WorktreeDir, why))

	gates := detectGates(root)
	if len(gates) == 0 {
		notes = append(notes, "queues        none: no gate script found in package.json or a Makefile; add one by hand if two lanes must never run it at once")
	} else {
		g := gates[0]
		s.Queues = []types.QueueConfig{{ID: "gate", Title: "Gate: " + strings.Join(g.argv, " "), Lock: "clauductor/gate.lock", Command: g.argv}}
		n := fmt.Sprintf("queues        gate runs `%s` (%s), and only when you press RUN on the page", strings.Join(g.argv, " "), g.from)
		if len(gates) > 1 {
			var alt []string
			for _, o := range gates[1:] {
				alt = append(alt, "`"+strings.Join(o.argv, " ")+"`")
			}
			n += "; also found " + strings.Join(alt, ", ")
		}
		notes = append(notes, n)
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(s); err != nil {
		return InitResult{}, err
	}
	// What init writes, the panel must read: check it before it reaches the disk.
	if _, err := config.ParseConfig(buf.Bytes()); err != nil {
		return InitResult{}, fmt.Errorf("the starter config does not validate (a bug): %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return InitResult{}, err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return InitResult{Path: path}, fmt.Errorf("%s %w; edit it, or remove it to start again", path, ErrConfigExists)
		}
		return InitResult{}, err
	}
	if _, err := f.Write(buf.Bytes()); err != nil {
		f.Close()
		return InitResult{}, err
	}
	if err := f.Close(); err != nil {
		return InitResult{}, err
	}
	return InitResult{Path: path, Body: buf.Bytes(), Notes: notes}, nil
}

var baseOK = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,199}$`)

// detectBase finds the default branch and what new lanes start from: origin's HEAD,
// else origin/main or origin/master, else the root checkout's own branch.
func detectBase(git func(...string) (string, error)) (branch, base, why string) {
	if ref, err := git("symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD"); err == nil && baseOK.MatchString(ref) {
		return strings.TrimPrefix(ref, "origin/"), ref, "origin's default branch"
	}
	for _, b := range []string{"main", "master"} {
		if _, err := git("rev-parse", "--verify", "--quiet", "refs/remotes/origin/"+b); err == nil {
			return b, "origin/" + b, "origin has it; origin/HEAD is not set"
		}
	}
	if b, err := git("symbolic-ref", "--quiet", "--short", "HEAD"); err == nil && baseOK.MatchString(b) {
		return b, b, "no origin remote, so the local branch the project root has checked out"
	}
	return "main", config.DefaultBase, "nothing to detect it from; check it"
}

var prefixOK = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,30}$`)

// detectLanes maps the default branch to the orchestrator and each branch prefix the
// repository already uses ("feature/x" → "feature/") to a lane type of that name.
func detectLanes(git func(...string) (string, error), defBranch string) (map[string]string, string) {
	lanes := map[string]string{defBranch: "orchestrator"}
	out, _ := git("for-each-ref", "--format=%(refname:short)", "refs/heads")
	count := map[string]int{}
	for _, b := range strings.Fields(out) {
		if p, _, ok := strings.Cut(b, "/"); ok && prefixOK.MatchString(p) {
			count[p]++
		}
	}
	prefixes := make([]string, 0, len(count))
	for p := range count {
		prefixes = append(prefixes, p)
	}
	sort.Slice(prefixes, func(i, j int) bool {
		if count[prefixes[i]] != count[prefixes[j]] {
			return count[prefixes[i]] > count[prefixes[j]]
		}
		return prefixes[i] < prefixes[j]
	})
	if len(prefixes) > 5 {
		prefixes = prefixes[:5]
	}
	if len(prefixes) == 0 {
		lanes["feature/"], lanes["fix/"] = "feature", "fix"
		return lanes, "a starting point: no local branch uses a prefix yet"
	}
	for _, p := range prefixes {
		lanes[p+"/"] = p
	}
	return lanes, "from the prefixes of your local branches"
}

func describeLanes(l map[string]string) string {
	keys := make([]string, 0, len(l))
	for k := range l {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+" → "+l[k])
	}
	return strings.Join(parts, ", ")
}

// detectWorktreeDir is the directory every linked worktree already shares, if they
// share one; else the default.
func detectWorktreeDir(ctx context.Context, run signals.Runner, root string) (string, string) {
	wts, err := signals.ReadWorktrees(ctx, run, root)
	parent := ""
	n := 0
	if err == nil {
		for _, w := range wts {
			if w.Bare || w.Path == root {
				continue
			}
			n++
			d := filepath.Dir(w.Path)
			if parent == "" {
				parent = d
			} else if parent != d {
				return config.DefaultWorktreeDir, "the default; your worktrees are in more than one directory"
			}
		}
	}
	if n == 0 {
		return config.DefaultWorktreeDir, "the default; no linked worktrees yet"
	}
	if rel, err := filepath.Rel(root, parent); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
		return rel, fmt.Sprintf("where your %d linked worktree(s) already are", n)
	}
	return parent, fmt.Sprintf("where your %d linked worktree(s) already are; an absolute path is this machine's, so make it relative to the project if others share the file", n)
}

// gateNames are the script and target names read as "the project's gate", best first.
var gateNames = []string{"gate", "ci", "check", "verify", "test"}

type gateCandidate struct {
	argv []string
	from string
}

// detectGates lists the gate commands the project defines itself, best first: a
// package.json script or a Makefile target with one of gateNames. It never invents
// a command the project does not name.
func detectGates(root string) []gateCandidate {
	var out []gateCandidate
	if b, err := os.ReadFile(filepath.Join(root, "package.json")); err == nil {
		var pkg struct {
			Scripts map[string]string `json:"scripts"`
		}
		if json.Unmarshal(b, &pkg) == nil {
			pm := "npm" // the lockfile names the package manager
			for _, l := range [][2]string{{"pnpm-lock.yaml", "pnpm"}, {"yarn.lock", "yarn"}, {"bun.lock", "bun"}, {"bun.lockb", "bun"}} {
				if _, err := os.Stat(filepath.Join(root, l[0])); err == nil {
					pm = l[1]
					break
				}
			}
			for _, n := range gateNames {
				if _, ok := pkg.Scripts[n]; ok {
					out = append(out, gateCandidate{argv: []string{pm, "run", n}, from: fmt.Sprintf("package.json script %q", n)})
				}
			}
		}
	}
	for _, mf := range []string{"GNUmakefile", "Makefile", "makefile"} {
		b, err := os.ReadFile(filepath.Join(root, mf))
		if err != nil {
			continue
		}
		targets := makeTargets(string(b))
		for _, n := range gateNames {
			if targets[n] {
				out = append(out, gateCandidate{argv: []string{"make", n}, from: fmt.Sprintf("%s target %q", mf, n)})
			}
		}
		break // make reads the first of these it finds
	}
	// The best name wins whichever file names it ("make ci" before "npm run test").
	rank := map[string]int{}
	for i, n := range gateNames {
		rank[n] = i
	}
	sort.SliceStable(out, func(i, j int) bool {
		return rank[out[i].argv[len(out[i].argv)-1]] < rank[out[j].argv[len(out[j].argv)-1]]
	})
	return out
}

var makeTargetRe = regexp.MustCompile(`(?m)^([A-Za-z0-9][A-Za-z0-9_.-]*)\s*:([^=]|$)`)

func makeTargets(s string) map[string]bool {
	t := map[string]bool{}
	for _, m := range makeTargetRe.FindAllStringSubmatch(s, -1) {
		t[m[1]] = true
	}
	return t
}
