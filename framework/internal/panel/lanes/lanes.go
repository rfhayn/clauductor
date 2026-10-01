// Package lanes starts, stops, restarts and restores the panel's lanes: one `claude`
// per tmux session on the panel's own socket, bound to its session id by the lane
// registry. Every command it runs is an argv list; nothing a browser sends is ever
// parsed by a shell.
package lanes

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/clauductor/clauductor/internal/panel/clock"
	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/clauductor/clauductor/internal/panel/types"
)

// A lane is one interactive `claude` in its own tmux session on the panel's dedicated
// socket. tmux, not the panel, owns the process, so lanes survive a panel restart
// and several viewers (browser tabs, Terminal.app) can attach to one lane.
//
// Every tmux, git and osascript invocation here is an argv list run without a shell.
// The only inputs a browser can supply are a lane id (validated below), a lane type
// (checked against the config), a worktree path (checked against `git worktree
// list`) and a flag. It never supplies a command.

// apiKeyVars outrank the subscription login; a lane never starts while one is set.
var apiKeyVars = []string{"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN"}

// parentSessionVars are set by a running Claude Code session for its own children.
// If the panel was itself launched from inside a session they would leak into every
// lane and make it look nested, so the lane command unsets them.
var parentSessionVars = []string{
	"CLAUDECODE", "CLAUDE_PID", "CLAUDE_EFFORT", "CLAUDE_CODE_ENTRYPOINT", "CLAUDE_CODE_SESSION_ID",
	"CLAUDE_CODE_CHILD_SESSION", "CLAUDE_CODE_SESSION_ATTENDED", "CLAUDE_CODE_MESSAGING_SOCKET",
	"CLAUDE_CODE_MESSAGING_TOKEN", "CLAUDE_CODE_BRIDGE_SESSION_ID", "CLAUDE_CODE_EXECPATH", "CLAUDE_CODE_SSE_PORT",
}

// Initial size of a detached lane; the first attached client resizes it.
const laneCols, laneRows = 200, 50

const tmuxListFormat = "#{session_name}\t#{pane_current_path}\t#{session_path}\t#{pane_dead}\t#{pane_dead_status}\t#{session_created}\t#{session_attached}\t#{@clauductor_type}\t#{@clauductor_project}"

// parseTmuxPanes parses `list-panes -a -F tmuxListFormat`, one lane per session. A
// session whose name is not a valid lane id was not started by the panel and is
// ignored.
func parseTmuxPanes(out []byte) []types.TmuxLane {
	seen := map[string]bool{}
	var lanes []types.TmuxLane
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		f := strings.Split(sc.Text(), "\t")
		if len(f) < 8 || !config.ValidLaneID(f[0]) || seen[f[0]] {
			continue
		}
		seen[f[0]] = true
		path := f[1]
		if path == "" {
			path = f[2]
		}
		created, _ := strconv.ParseInt(f[5], 10, 64)
		attached, _ := strconv.Atoi(f[6])
		l := types.TmuxLane{ID: f[0], Path: signals.ResolvePath(path), Type: f[7], Created: created,
			Attached: attached, Dead: f[3] == "1", DeadStatus: f[4]}
		if len(f) > 8 {
			l.Project = f[8]
		}
		lanes = append(lanes, l)
	}
	sort.Slice(lanes, func(i, j int) bool { return lanes[i].ID < lanes[j].ID })
	return lanes
}

// LaneError is an expected failure of a lane command, with the HTTP status the API
// answers with.
type LaneError struct {
	Status int
	Code   string
	Msg    string
}

func (e *LaneError) Error() string { return e.Msg }

func laneErr(status int, code, format string, a ...any) *LaneError {
	return &LaneError{Status: status, Code: code, Msg: fmt.Sprintf(format, a...)}
}

// LaneManager starts, lists and controls lanes on one tmux socket.
type LaneManager struct {
	TmuxPath string         // absolute path of tmux
	Socket   string         // tmux -L name
	Root     string         // project root (resolved)
	Cfg      *config.Config //
	Registry *Registry      // durable lane ↔ session binding
	Run      signals.Runner // runs git and `claude agents` (injectable for tests)
	Program  []string       // the lane program; default the absolute path of `claude`
	// UploadDir keeps images dropped on a lane's terminal, one directory per lane
	// (images.go): in the panel's state directory, never a worktree. "" refuses them.
	UploadDir string
	// Project is the project id each lane is tagged with (@clauductor_project,
	// PANEL-16). A session tagged for another project is not this manager's: each
	// project has its own socket, and the tag guards against two sharing one.
	Project string
	// LookupEnv reads the panel's own environment (injectable for tests).
	LookupEnv func(string) (string, bool)
	// StopTimeout is how long Stop waits for /exit before killing the session.
	StopTimeout time.Duration
	// EnterDelay separates typed text from its Enter: sent together, a long line
	// can sit in claude's input box unsubmitted.
	EnterDelay time.Duration
	// FastExit is how long a (re)started claude must stay up to count as started;
	// a faster non-zero exit triggers the --resume / --session-id fallback.
	FastExit time.Duration
	// Changed is called after a lane is started or stopped (re-poll now).
	Changed func()
	// Stopped is called once a lane's tmux session is gone, to close its viewers.
	Stopped func(id string)
	// Clock is the time registry stamps and every wait use. Required.
	Clock clock.Clock
	// Exec, if set, runs a tmux argv (after the tmux path) instead of tmux itself
	// (tests count the calls).
	Exec func(ctx context.Context, argv []string) ([]byte, error)

	// RemoteControl reports whether lanes start with `claude --remote-control`, read
	// at each start (PANEL-19: the machine's choice at `panel install`). Nil is no.
	RemoteControl func() bool
	// Trusted reports whether panel.json's commands may run (PANEL-20: worktree_setup
	// and worktree_teardown). Nil is trusted (tests).
	Trusted func() bool

	mu sync.Mutex // serialises lane actions
}

// ConnectRemote types /remote-control and Enter into a lane (PANEL-19), which
// connects a lane started before remote control was chosen, or shows the connected
// one's status. As Stop's /exit, only into a claude that `claude agents` reports idle,
// read twice, before the text and before the Enter: an Enter typed into a dialog
// would confirm whatever it has focused.
func (m *LaneManager) ConnectRemote(ctx context.Context, id string) *LaneError {
	if !config.ValidLaneID(id) {
		return laneErr(400, "invalid", "invalid lane id")
	}
	if m.RemoteControl == nil || !m.RemoteControl() {
		return laneErr(409, "remote-off", "remote control is not on for the panel's lanes (clauductor panel install --remote-control=lanes)")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	rec, ok := m.Registry.Get(id)
	if !ok || rec.SessionID == "" {
		return laneErr(404, "not-found", "no registered lane %q", id)
	}
	lane, ok := m.find(ctx, id)
	if !ok || lane.Dead {
		return laneErr(409, "not-running", "lane %q is not running", id)
	}
	idle := func() bool {
		st, found, err := m.agentStatus(ctx, rec.SessionID)
		return err == nil && found && st == "idle"
	}
	if !idle() {
		return laneErr(409, "not-idle", "claude in %s is not idle (claude agents), so nothing is typed; try again when it is", id)
	}
	target := "=" + id + ":"
	_, _ = m.tmux(ctx, "send-keys", "-t", target, "C-u")
	if _, err := m.tmux(ctx, "send-keys", "-t", target, "-l", "--", "/remote-control"); err != nil {
		return laneErr(500, "tmux", "%v", err)
	}
	m.clock().Sleep(m.EnterDelay)
	if !idle() {
		_, _ = m.tmux(ctx, "send-keys", "-t", target, "C-u")
		return laneErr(409, "not-idle", "claude in %s stopped being idle; the text was cleared, nothing was sent", id)
	}
	if _, err := m.tmux(ctx, "send-keys", "-t", target, "Enter"); err != nil {
		return laneErr(500, "tmux", "%v", err)
	}
	return nil
}

func (m *LaneManager) lookupEnv(k string) (string, bool) {
	if m.LookupEnv != nil {
		return m.LookupEnv(k)
	}
	return os.LookupEnv(k)
}

func (m *LaneManager) changed() {
	if m.Changed != nil {
		m.Changed()
	}
}

// TmuxEnv is the environment of every tmux command the panel runs. The first one
// starts the socket's server, whose global environment every lane inherits.
func TmuxEnv() []string {
	drop := map[string]bool{"TMUX": true, "TMUX_PANE": true}
	for _, k := range parentSessionVars {
		drop[k] = true
	}
	var env []string
	hasLang := false
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if drop[k] {
			continue
		}
		if k == "LANG" || k == "LC_ALL" || k == "LC_CTYPE" {
			hasLang = true
		}
		env = append(env, kv)
	}
	if !hasLang { // without a UTF-8 locale tmux draws claude's box characters as "_"
		env = append(env, "LANG=en_US.UTF-8")
	}
	return env
}

// TmuxArgv prefixes a tmux command with the socket. `-f /dev/null` means a panel
// socket's server never loads ~/.tmux.conf, whose bindings could run commands.
func (m *LaneManager) TmuxArgv(args ...string) []string {
	return append([]string{"-L", m.Socket, "-f", "/dev/null"}, args...)
}

func (m *LaneManager) tmux(ctx context.Context, args ...string) ([]byte, error) {
	return m.tmuxIn(ctx, "", args...)
}

// tmuxIn is tmux run in dir ("" is the panel's own directory).
func (m *LaneManager) tmuxIn(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if m.Exec != nil {
		return m.Exec(cctx, m.TmuxArgv(args...))
	}
	cmd := exec.CommandContext(cctx, m.TmuxPath, m.TmuxArgv(args...)...)
	cmd.Env = TmuxEnv()
	cmd.Dir = dir
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = err.Error()
		}
		return out.Bytes(), errors.New("tmux: " + msg)
	}
	return out.Bytes(), nil
}

// ServerArgv is the command that starts the socket's server when it has none. A
// tmux server is the fork of the client that started it: it keeps that client's
// argv and cwd for life, and runs as an orphan (ppid 1). Started by a lane's
// new-session, it would carry `-c <worktree>` and the lane's claude command line,
// and a cleanup script hunting orphans that name `.claude/worktrees/` killed one,
// and every lane with it. So the server is started by a command that names no
// worktree, in the home directory, and exit-empty is off only until the lane's own
// session exists (newSession turns it back on).
func (m *LaneManager) ServerArgv() []string {
	return m.TmuxArgv("start-server", ";", "set-option", "-g", "exit-empty", "off")
}

// newSession runs a lane's new-session on a server started by ServerArgv.
func (m *LaneManager) newSession(ctx context.Context, argv []string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "/"
	}
	if _, err := m.tmuxIn(ctx, home, m.ServerArgv()[4:]...); err != nil {
		return err
	}
	_, err = m.tmux(ctx, argv[4:]...)
	// Back to tmux's default either way: the server ends with its last session, as
	// before, and a failed start leaves no empty server behind.
	_, _ = m.tmux(ctx, "set-option", "-g", "exit-empty", "on")
	return err
}

// noServer reports whether a tmux error only means the socket has no server yet.
func noServer(err error) bool {
	s := err.Error()
	return strings.Contains(s, "no server running") || strings.Contains(s, "error connecting to")
}

// List returns the lanes on the socket. No server means no lanes, not an error.
func (m *LaneManager) List(ctx context.Context) ([]types.TmuxLane, error) {
	lanes, _, err := m.ListServer(ctx)
	return lanes, err
}

// ListServer is List, and whether the socket has a server at all: one `list-panes
// -a` call covers every lane.
func (m *LaneManager) ListServer(ctx context.Context) (lanes []types.TmuxLane, up bool, err error) {
	out, err := m.tmux(ctx, "list-panes", "-a", "-F", tmuxListFormat)
	if err != nil {
		if noServer(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	all := parseTmuxPanes(out)
	// A session tagged for another project is that project's, never an orphan here;
	// an untagged one (started before PANEL-16) belongs to the socket's owner.
	lanes = all[:0]
	for _, l := range all {
		if l.Project == "" || m.Project == "" || l.Project == m.Project {
			lanes = append(lanes, l)
		}
	}
	return lanes, true, nil
}

// Exists reports whether a lane's tmux session is running. "=" makes the match
// exact; without it tmux would take "lane" to mean "lane-2".
func (m *LaneManager) Exists(ctx context.Context, id string) bool {
	if !config.ValidLaneID(id) {
		return false
	}
	_, err := m.tmux(ctx, "has-session", "-t", "="+id)
	return err == nil
}

func (m *LaneManager) find(ctx context.Context, id string) (types.TmuxLane, bool) {
	lanes, _ := m.List(ctx)
	for _, l := range lanes {
		if l.ID == id {
			return l, true
		}
	}
	return types.TmuxLane{}, false
}

// StartBlocked returns why no lane may start now, or "". An API key in the panel's
// environment, or in the tmux server's global environment that every lane
// inherits, would outrank the subscription login.
func (m *LaneManager) StartBlocked(ctx context.Context) string {
	if why := m.EnvBlocked(); why != "" {
		return why
	}
	return m.TmuxEnvBlocked(ctx)
}

// EnvBlocked is StartBlocked's check of the panel's own environment (no spawn).
func (m *LaneManager) EnvBlocked() string {
	for _, k := range apiKeyVars {
		if _, ok := m.lookupEnv(k); ok {
			return fmt.Sprintf("%s is set in the panel's environment. It outranks your subscription login, so no lane starts until it is removed and the panel restarted.", k)
		}
	}
	return ""
}

// TmuxEnvBlocked is StartBlocked's check of the tmux server's global environment:
// one `show-environment -g`.
func (m *LaneManager) TmuxEnvBlocked(ctx context.Context) string {
	out, err := m.tmux(ctx, "show-environment", "-g")
	if err != nil {
		if noServer(err) {
			return ""
		}
		return "cannot read the tmux server's environment, so an API key there cannot be ruled out: " + err.Error()
	}
	for _, line := range strings.Split(string(out), "\n") {
		k, _, isSet := strings.Cut(strings.TrimSpace(line), "=")
		for _, v := range apiKeyVars {
			if isSet && k == v {
				return fmt.Sprintf("%s is set in the global environment of tmux server -L %s, which every lane inherits. Remove it (tmux -L %s set-environment -g -u %s) before starting a lane.", v, m.Socket, m.Socket, v)
			}
		}
	}
	return ""
}

// ScrubbedArgv runs argv through /usr/bin/env with the API-key and parent-session
// variables unset: the environment a lane's claude sees. `claude auth status` runs
// this way too (PANEL-15), so it reports the login lanes will use.
func ScrubbedArgv(argv ...string) []string {
	out := []string{"/usr/bin/env"}
	for _, k := range append(append([]string{}, apiKeyVars...), parentSessionVars...) {
		out = append(out, "-u", k)
	}
	return append(out, argv...)
}

// LaneCommand is the argv tmux runs for a lane. /usr/bin/env unsets the variables a
// parent Claude session would leak; with two or more arguments tmux execs the
// command directly instead of passing it to a shell.
//
// The session id is the panel's, never discovered: a new lane gets --session-id
// <uuid>, and a restart or restore gets --resume <uuid>. --continue is never used,
// because it picks the directory's most recent conversation, whoever's it is.
func (m *LaneManager) LaneCommand(id, laneType, sessionID string, resume bool) []string {
	argv := ScrubbedArgv(m.Program...)
	lt := m.launchOptions(id, laneType)
	if lt.Model != "" {
		argv = append(argv, "--model", lt.Model)
	}
	if lt.Effort != "" {
		argv = append(argv, "--effort", lt.Effort)
	}
	if m.RemoteControl != nil && m.RemoteControl() {
		// PANEL-19: remote control for the panel's lanes only (`panel install`). Its
		// optional name argument is never given: -n names the session.
		argv = append(argv, "--remote-control")
	}
	argv = append(argv, "-n", id)
	if resume {
		return append(argv, "--resume", sessionID)
	}
	return append(argv, "--session-id", sessionID)
}

// SessionEnv is set explicitly on every lane's tmux session (new-session -e), so a
// lane does not depend on the environment its tmux server happened to start with:
// PATH carries the directories of claude, tmux, git, gh and node, then the panel's.
func (m *LaneManager) SessionEnv() []string {
	var dirs []string
	seen := map[string]bool{}
	add := func(d string) {
		if d != "" && !seen[d] {
			seen[d] = true
			dirs = append(dirs, d)
		}
	}
	if len(m.Program) > 0 && filepath.IsAbs(m.Program[0]) {
		add(filepath.Dir(m.Program[0]))
	}
	add(filepath.Dir(m.TmuxPath))
	for _, tool := range []string{"git", "gh", "node"} {
		if p, err := exec.LookPath(tool); err == nil {
			add(filepath.Dir(p))
		}
	}
	for _, d := range filepath.SplitList(os.Getenv("PATH")) {
		add(d)
	}
	home, _ := os.UserHomeDir()
	lang := os.Getenv("LANG")
	if lang == "" {
		lang = "en_US.UTF-8"
	}
	return []string{"PATH=" + strings.Join(dirs, ":"), "HOME=" + home, "LANG=" + lang}
}

// NewSessionArgv is the tmux argv that starts a lane. The chained set-options run in
// the same server command, before a fast-failing program can exit: remain-on-exit
// keeps its last output on screen, and @clauductor_type records the lane type.
// window-size latest (tmux's default, set explicitly because ~/.tmux.conf may change
// it) sizes the lane to whichever client typed or resized last, so the browser's
// terminal drives the size and a Terminal.app window can still share the lane.
func (m *LaneManager) NewSessionArgv(id, path, laneType, sessionID string, resume bool) []string {
	args := []string{"new-session", "-d", "-s", id, "-c", path,
		"-x", strconv.Itoa(laneCols), "-y", strconv.Itoa(laneRows)}
	for _, kv := range m.SessionEnv() {
		args = append(args, "-e", kv)
	}
	// A gate script run inside the lane names its lane in the queue (lock-run).
	args = append(args, "-e", "CLAUDUCTOR_LANE="+id)
	if port := m.LanePort(id); port > 0 {
		args = append(args, "-e", "CLAUDUCTOR_PORT="+strconv.Itoa(port)) // PANEL-20
	}
	args = append(args, m.LaneCommand(id, laneType, sessionID, resume)...)
	args = append(args,
		";", "set-option", "-t", "="+id+":", "remain-on-exit", "on",
		";", "set-option", "-t", "="+id+":", "@clauductor_type", laneType,
		";", "set-option", "-t", "="+id+":", "window-size", "latest",
		";")
	if m.Project != "" {
		args = append(args, "set-option", "-t", "="+id+":", "@clauductor_project", m.Project, ";")
	}
	args = append(args, hardenArgs...)
	return m.TmuxArgv(args...)
}

// AttachArgv is the argv (after the tmux path) of one viewer's client. -u forces
// UTF-8 whatever the locale.
func (m *LaneManager) AttachArgv(id string) []string {
	return []string{"-u", "-L", m.Socket, "-f", "/dev/null", "attach-session", "-t", "=" + id}
}

// shq single-quotes s for a POSIX shell.
func shq(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// TerminalAppArgv is the osascript argv that opens a Terminal.app window attached to
// a lane. The command reaches AppleScript as an argument, never spliced into the
// script's source, and every part of it is single-quoted for Terminal's shell; the
// lane id and socket are validated as well.
func (m *LaneManager) TerminalAppArgv(id string) []string {
	cmd := "exec " + shq(m.TmuxPath)
	for _, a := range m.AttachArgv(id) {
		cmd += " " + shq(a)
	}
	return []string{"/usr/bin/osascript",
		"-e", "on run argv",
		"-e", `tell application "Terminal"`,
		"-e", "activate",
		"-e", "do script (item 1 of argv)",
		"-e", "end tell",
		"-e", "end run",
		"--", cmd} // "--": osascript parses options among its arguments
}

// StartRequest is the body of POST /api/lanes.
type StartRequest struct {
	Type     string `json:"type"`
	Mode     string `json:"mode"`     // "new" (new branch + worktree) | "existing" (a worktree) | "root" (the project root)
	Name     string `json:"name"`     // the lane id; for "new" also the branch name after the type's prefix
	Worktree string `json:"worktree"` // for "existing": a path from git worktree list
	// v2 (StartLane): a template fills the branch and the first prompt.
	Template      string `json:"template,omitempty"`
	Issue         string `json:"issue,omitempty"`
	OverrideQuota bool   `json:"overrideQuota,omitempty"`

	tpl *config.RenderedTemplate // set by StartLane after validation, never from JSON
}

// StartResult reports a started lane.
type StartResult struct {
	ID        string   `json:"id"`
	SessionID string   `json:"sessionId"`
	Path      string   `json:"path"`
	Branch    string   `json:"branch,omitempty"`
	Notes     []string `json:"notes,omitempty"`
}

func (m *LaneManager) clock() clock.Clock {
	if m.Clock == nil {
		panic("lanes: LaneManager.Clock is not set")
	}
	return m.Clock
}

func (m *LaneManager) now() time.Time { return m.clock().Now() }

// Start starts a lane: a new claude session, with an id the panel assigns, in the
// project root, an existing worktree, or a new branch's new worktree.
func (m *LaneManager) Start(ctx context.Context, req StartRequest) (StartResult, *LaneError) {
	id := req.Name
	if !config.ValidLaneID(id) {
		return StartResult{}, laneErr(400, "invalid", "lane name %q must match %s", id, config.LaneIDRe)
	}
	if !m.Cfg.HasLaneType(req.Type) {
		return StartResult{}, laneErr(400, "invalid", "unknown lane type %q", req.Type)
	}
	if req.Mode != "new" && req.Mode != "existing" && req.Mode != "root" {
		return StartResult{}, laneErr(400, "invalid", "mode must be new, existing or root")
	}
	if why := m.StartBlocked(ctx); why != "" {
		return StartResult{}, laneErr(409, "api-key", "%s", why)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Exists(ctx, id) {
		return StartResult{}, laneErr(409, "exists", "lane %q is already running", id)
	}
	if rec, ok := m.Registry.Get(id); ok {
		return StartResult{}, laneErr(409, "exists", "lane %q is registered (session %s, in %s): resume or forget it first", id, rec.SessionID, rec.Path)
	}
	sid, err := newSessionID()
	if err != nil {
		return StartResult{}, laneErr(500, "fault", "%v", err)
	}
	res := StartResult{ID: id, SessionID: sid}
	switch req.Mode {
	case "root":
		res.Path = m.Root
		if other := m.pathTaken(ctx, res.Path, ""); other != "" {
			return res, laneErr(409, "path-taken", "lane %q already runs in %s; two sessions in one checkout would edit the same files", other, res.Path)
		}
	case "existing":
		wts, err := signals.ReadWorktrees(ctx, m.Run, m.Root)
		if err != nil {
			return res, laneErr(500, "git", "cannot read the worktree list: %v", err)
		}
		want := signals.ResolvePath(req.Worktree)
		for _, w := range wts {
			if !w.Bare && w.Path == want {
				res.Path, res.Branch = w.Path, w.Branch
			}
		}
		if res.Path == "" {
			return res, laneErr(400, "invalid", "%q is not one of this project's worktrees", req.Worktree)
		}
		if other := m.pathTaken(ctx, res.Path, ""); other != "" {
			return res, laneErr(409, "path-taken", "lane %q already runs in %s; two sessions in one checkout would edit the same files", other, res.Path)
		}
	case "new":
		prefix := m.Cfg.BranchPrefix(req.Type)
		if prefix == "" && req.tpl == nil {
			return res, laneErr(400, "invalid", "lane type %q has no branch prefix in lanes; pick an existing worktree or the project root", req.Type)
		}
		res.Branch = prefix + id
		if req.tpl != nil {
			res.Branch = req.tpl.Branch
		}
		if !config.BranchRe.MatchString(res.Branch) {
			return res, laneErr(400, "invalid", "branch %q is not allowed", res.Branch)
		}
		if _, err := m.Run(ctx, m.Root, []string{"git", "check-ref-format", "--branch", res.Branch}); err != nil {
			return res, laneErr(400, "invalid", "branch %q is not a valid branch name", res.Branch)
		}
		res.Path = filepath.Join(m.Cfg.WorktreeRoot(m.Root), id)
		if _, err := os.Stat(res.Path); err == nil {
			return res, laneErr(409, "exists", "%s already exists; start the lane on the existing worktree instead", res.Path)
		}
	}

	// The intent is on disk before anything is created, so a crash from here on
	// leaves a record the next start shows as an orphan.
	rec, err := m.Registry.Begin(req.withTemplate(types.LaneRecord{ID: id, SessionID: sid, Path: res.Path, Type: req.Type,
		Branch: res.Branch, Mode: req.Mode, Created: m.now().UnixMilli(), Port: m.allocatePort(id)}), "start", m.now())
	if err != nil {
		return res, laneErr(500, "registry", "cannot write the lane registry: %v", err)
	}
	fail := func(e *LaneError) (StartResult, *LaneError) {
		_ = m.Registry.Delete(id)
		return res, e
	}
	if req.Mode == "new" {
		fctx, cancel := context.WithTimeout(ctx, 60*time.Second)
		_, ferr := m.Run(fctx, m.Root, []string{"git", "fetch", "--quiet"})
		cancel()
		if ferr != nil {
			res.Notes = append(res.Notes, fmt.Sprintf("git fetch failed (%v); the branch starts from the last fetched %s", ferr, m.Cfg.BaseRef()))
		}
		actx, cancel := context.WithTimeout(ctx, 60*time.Second)
		_, err := m.Run(actx, m.Root, []string{"git", "worktree", "add", "-b", res.Branch, res.Path, m.Cfg.BaseRef()})
		cancel()
		if err != nil {
			return fail(laneErr(409, "git", "git worktree add failed: %v", err))
		}
		res.Path = signals.ResolvePath(res.Path)
		rec.Path = res.Path
		// PANEL-20: the new worktree's gitignored files, then its setup, before claude.
		if n, note := m.copyWorktreeInclude(ctx, res.Path); note != "" || n > 0 {
			if n > 0 {
				res.Notes = append(res.Notes, fmt.Sprintf(".worktreeinclude: copied %d file(s) from the project root", n))
			}
			if note != "" {
				res.Notes = append(res.Notes, note)
			}
		}
		if ran, err := m.runHook(ctx, "worktree_setup", m.Cfg.WorktreeSetup, res.Path, id, rec.Port); err != nil {
			res.Notes = append(res.Notes, err.Error()+"; the lane starts anyway")
		} else if ran {
			res.Notes = append(res.Notes, "worktree_setup ran")
		}
	}
	if err := m.newSession(ctx, m.NewSessionArgv(id, res.Path, req.Type, sid, false)); err != nil {
		return fail(laneErr(500, "tmux", "starting the lane failed: %v", err))
	}
	if err := m.Registry.Done(rec); err != nil {
		res.Notes = append(res.Notes, "the lane started, but the registry could not record it: "+err.Error())
	}
	m.changed()
	return res, nil
}

// hardenArgs make the panel's socket keyless: no prefix, and no prefix or root
// (bind -n) bindings. From a lane's terminal, a prefix would reach every other lane
// and tmux's own command prompt (run-shell). `-f /dev/null` only applies when the
// panel starts the server, so this also runs against a server someone else started
// on the socket.
//
// PANEL-6: the mouse wheel scrolls the lane's history. tmux shows a lane on the
// browser's alternate screen, and xterm.js turns a wheel there into ↑/↓ keypresses
// unless the program asked for mouse reports: in a permission dialog, ↑ moved the
// selection. `mouse on` makes tmux ask, so the wheel reaches tmux as a mouse event.
// Of the root table that unbind-key cleared, only WheelUpPane comes back, as tmux
// 3.x ships it: into copy mode (-e: it ends when you scroll back to the bottom),
// or to the program if it asked for the mouse itself. Clicks and the right-click
// menu (kill-pane, respawn-pane) stay unbound. In copy mode, q or Escape leaves.
// The status bar is off: the tab already names the lane. The condition nests `||`
// two at a time: tmux before 3.5 reads only the first two arguments of one, and
// passed every wheel to the program (seen on Ubuntu 24.04's tmux 3.4).
//
// PANEL-14: tmux strips OSC 8 hyperlinks unless the client's terminal has the
// `hyperlinks` feature, which no default entry gives xterm-256color (the viewers'
// TERM). claude emits them inside tmux 3.4+, so without it no link it prints reached
// the page. A fixed index keeps the per-poll re-run from growing the array; -q keeps
// a tmux without the option (before 3.2) hardening.
var hardenArgs = []string{"set-option", "-g", "prefix", "None",
	";", "set-option", "-g", "prefix2", "None",
	";", "unbind-key", "-q", "-a", "-T", "prefix",
	";", "unbind-key", "-q", "-a", "-T", "root",
	";", "set-option", "-g", "status", "off",
	";", "set-option", "-g", "mouse", "on",
	";", "bind-key", "-T", "root", "WheelUpPane",
	"if-shell", "-F", "#{||:#{alternate_on},#{||:#{pane_in_mode},#{mouse_any_flag}}}", "send-keys -M", "copy-mode -e",
	";", "set-option", "-sq", "terminal-features[99]", "xterm-256color:hyperlinks"}

// Harden applies hardenArgs if the socket has a server. The panel runs it whenever
// it finds the server (every tmux poll) and before every viewer attaches.
func (m *LaneManager) Harden(ctx context.Context) error {
	if _, err := m.tmux(ctx, hardenArgs...); err != nil && !noServer(err) {
		return err
	}
	return nil
}

// sendText types text into a lane, then presses Enter as a separate write.
func (m *LaneManager) sendText(ctx context.Context, id, text string) error {
	// "--": the text may start with "-", which send-keys would read as a flag.
	if _, err := m.tmux(ctx, "send-keys", "-t", "="+id+":", "-l", "--", text); err != nil {
		return err
	}
	m.clock().Sleep(m.EnterDelay)
	_, err := m.tmux(ctx, "send-keys", "-t", "="+id+":", "Enter")
	return err
}

// Interrupt presses Escape in a lane: claude's interrupt.
func (m *LaneManager) Interrupt(ctx context.Context, id string) *LaneError {
	if !m.Exists(ctx, id) {
		return laneErr(404, "not-found", "no lane %q", id)
	}
	if _, err := m.tmux(ctx, "send-keys", "-t", "="+id+":", "Escape"); err != nil {
		return laneErr(500, "tmux", "%v", err)
	}
	return nil
}

// Stop ends a lane and forgets it; the worktree is never removed (Close, in
// close.go, is Stop and then the worktree and branch when that is safe). Only a lane that
// `claude agents` reports idle is asked to /exit. Anything else (busy, waiting on a
// permission or a dialog, or unknown) gets Escape and then kill-session, never a
// typed Enter: an Enter would confirm whatever default the dialog has focused.
func (m *LaneManager) Stop(ctx context.Context, id string) *LaneError {
	if !config.ValidLaneID(id) {
		return laneErr(400, "invalid", "invalid lane id")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stopAndForgetLocked(ctx, id)
}

// stopAndForgetLocked is Stop under the lane lock; Close (PANEL-17) runs it too, so
// the two end a lane in exactly the same way.
func (m *LaneManager) stopAndForgetLocked(ctx context.Context, id string) *LaneError {
	rec, registered := m.Registry.Get(id)
	if registered {
		var err error
		if rec, err = m.Registry.Begin(rec, "stop", m.now()); err != nil {
			return laneErr(500, "registry", "cannot write the lane registry: %v", err)
		}
	}
	if lerr := m.stopLocked(ctx, id, rec.SessionID); lerr != nil {
		if !(lerr.Status == 404 && registered) {
			return lerr
		}
		// Registered but already gone from tmux: stopping it means forgetting it.
	}
	m.dropImages(id)
	if registered {
		if err := m.Registry.Delete(id); err != nil {
			return laneErr(500, "registry", "the lane stopped, but the registry could not forget it: %v", err)
		}
	}
	return nil
}

func (m *LaneManager) waitDead(ctx context.Context, id string, d time.Duration) {
	deadline := m.now().Add(d)
	for m.now().Before(deadline) {
		l, ok := m.find(ctx, id)
		if !ok || l.Dead {
			return
		}
		m.clock().Sleep(200 * time.Millisecond)
	}
}

func (m *LaneManager) stopLocked(ctx context.Context, id, sessionID string) *LaneError {
	lane, ok := m.find(ctx, id)
	if !ok {
		return laneErr(404, "not-found", "no lane %q", id)
	}
	defer m.changed()
	if !lane.Dead {
		status, found, err := m.agentStatus(ctx, sessionID)
		if sessionID != "" && err == nil && found && status == "idle" {
			// C-u first, as its own key: it clears any unsent text in claude's input,
			// which would otherwise turn "/exit" into part of a prompt.
			target := "=" + id + ":"
			_, _ = m.tmux(ctx, "send-keys", "-t", target, "C-u")
			_, _ = m.tmux(ctx, "send-keys", "-t", target, "-l", "/exit")
			m.clock().Sleep(m.EnterDelay)
			// Still idle right before the Enter? A dialog may have opened meanwhile.
			if st, ok, err := m.agentStatus(ctx, sessionID); err == nil && ok && st == "idle" {
				_, _ = m.tmux(ctx, "send-keys", "-t", target, "Enter")
				m.waitDead(ctx, id, m.StopTimeout)
			} else {
				_, _ = m.tmux(ctx, "send-keys", "-t", target, "Escape")
				m.waitDead(ctx, id, time.Second)
			}
		} else {
			_, _ = m.tmux(ctx, "send-keys", "-t", "="+id+":", "Escape")
			m.waitDead(ctx, id, time.Second)
		}
	}
	if _, err := m.tmux(ctx, "kill-session", "-t", "="+id); err != nil && m.Exists(ctx, id) {
		return laneErr(500, "tmux", "%v", err)
	}
	if m.Stopped != nil {
		m.Stopped(id)
	}
	return nil
}

// agentStatus reads one session's status from `claude agents --json`.
func (m *LaneManager) agentStatus(ctx context.Context, sessionID string) (status string, found bool, err error) {
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := m.Run(cctx, m.Root, []string{"claude", "agents", "--json"})
	if err != nil {
		return "", false, err
	}
	agents, err := signals.ParseAgents(out)
	if err != nil {
		return "", false, err
	}
	for _, a := range agents {
		if sessionID != "" && a.SessionID == sessionID {
			return a.Status, true, nil
		}
	}
	return "", false, nil
}

// liveSession reports whether a claude process already runs this session id, per
// `claude agents --json`. Two processes on one session interleave its transcript,
// so a failure to read the list refuses rather than guesses.
func (m *LaneManager) liveSession(ctx context.Context, sessionID string) (bool, error) {
	_, found, err := m.agentStatus(ctx, sessionID)
	return found, err
}

// pathTaken returns the id of a running lane (registered or not) in dir, if any.
// Two claude sessions in one checkout would edit the same files.
func (m *LaneManager) pathTaken(ctx context.Context, dir, except string) string {
	lanes, _ := m.List(ctx)
	for _, l := range lanes {
		if l.ID != except && l.Path == dir && !l.Dead {
			return l.ID
		}
	}
	for _, rec := range m.Registry.List() {
		if rec.ID != except && rec.Path == dir && m.Exists(ctx, rec.ID) {
			return rec.ID
		}
	}
	return ""
}

// launchSession starts a registered lane's claude on its own session id and, when
// claude exits within FastExit with a non-zero status, retries once with the other
// flag. `--resume` exits 1 on a session with no conversation, and `--session-id`
// exits 1 on one that has a conversation; the panel only learns which from hooks,
// which can be dropped. It returns the mode that stayed up.
func (m *LaneManager) launchSession(ctx context.Context, rec types.LaneRecord) (resume bool, lerr *LaneError) {
	resume = rec.Conversation
	for attempt := 0; attempt < 2; attempt++ {
		if err := m.newSession(ctx, m.NewSessionArgv(rec.ID, rec.Path, rec.Type, rec.SessionID, resume)); err != nil {
			return resume, laneErr(500, "tmux", "starting claude failed: %v", err)
		}
		failed := ""
		// The last look is at or after the deadline: a loop that only looked while
		// before it could sleep past it and call a claude that died within FastExit
		// started (one look takes a tmux call, slow on a loaded machine).
		for deadline := m.now().Add(m.FastExit); ; {
			l, ok := m.find(ctx, rec.ID)
			if ok && l.Dead && l.DeadStatus != "" && l.DeadStatus != "0" {
				failed = l.DeadStatus
				break
			}
			if !m.now().Before(deadline) {
				break
			}
			m.clock().Sleep(200 * time.Millisecond)
		}
		if failed == "" {
			return resume, nil
		}
		if attempt == 1 {
			// Leave the dead pane: remain-on-exit keeps claude's own message on screen.
			return resume, laneErr(409, "launch-failed", "claude exited with status %s both with --resume and with --session-id %s; the lane's terminal shows why", failed, rec.SessionID)
		}
		_, _ = m.tmux(ctx, "kill-session", "-t", "="+rec.ID)
		resume = !resume
	}
	return resume, nil
}

// resumeLocked restarts a registered lane whose tmux session is gone on its own
// session id, after checking no process holds that session: `claude --resume <id>`
// once the session has a conversation, else `claude --session-id <id>` again
// (--resume refuses a session with no conversation), with launchSession's fallback.
func (m *LaneManager) resumeLocked(ctx context.Context, rec types.LaneRecord, action string) *LaneError {
	if rec.Corrupt != "" {
		return laneErr(409, "corrupt", "lane %q has a corrupt registry record (%s); forget it", rec.ID, rec.Corrupt)
	}
	if m.Exists(ctx, rec.ID) {
		return laneErr(409, "exists", "lane %q is still running", rec.ID)
	}
	if other := m.pathTaken(ctx, rec.Path, rec.ID); other != "" {
		return laneErr(409, "path-taken", "lane %q already runs in %s", other, rec.Path)
	}
	var live bool
	var err error
	for i := 0; i < 25; i++ { // an exiting claude can linger in the list briefly
		if live, err = m.liveSession(ctx, rec.SessionID); err != nil || !live {
			break
		}
		m.clock().Sleep(200 * time.Millisecond)
	}
	if err != nil {
		return laneErr(409, "unverified", "cannot confirm session %s is not already running (claude agents: %v), so it is not resumed", rec.SessionID, err)
	}
	if live {
		return laneErr(409, "session-live", "session %s is already running in another claude process; resuming it twice would interleave its transcript", rec.SessionID)
	}
	if fi, err := os.Stat(rec.Path); err != nil || !fi.IsDir() {
		return laneErr(409, "no-worktree", "%s no longer exists; forget the lane instead", rec.Path)
	}
	rec, err = m.Registry.Begin(rec, action, m.now())
	if err != nil {
		return laneErr(500, "registry", "cannot write the lane registry: %v", err)
	}
	resumed, lerr := m.launchSession(ctx, rec)
	rec.Conversation = resumed
	if lerr != nil {
		_ = m.Registry.Put(rec)
		m.changed()
		return lerr
	}
	if err := m.Registry.Done(rec); err != nil {
		return laneErr(500, "registry", "the lane resumed, but the registry could not record it: %v", err)
	}
	m.changed()
	return nil
}

// Restart stops a registered lane and resumes its own session in the same place.
func (m *LaneManager) Restart(ctx context.Context, id string) *LaneError {
	if !config.ValidLaneID(id) {
		return laneErr(400, "invalid", "invalid lane id")
	}
	if why := m.StartBlocked(ctx); why != "" {
		return laneErr(409, "api-key", "%s", why)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	rec, ok := m.Registry.Get(id)
	if !ok {
		return laneErr(409, "unregistered", "lane %q was not started by this panel, so its session id is unknown; stop it and start a new lane", id)
	}
	rec, err := m.Registry.Begin(rec, "restart", m.now())
	if err != nil {
		return laneErr(500, "registry", "cannot write the lane registry: %v", err)
	}
	if rec.Corrupt != "" {
		return laneErr(409, "corrupt", "lane %q has a corrupt registry record (%s); stop and forget it", id, rec.Corrupt)
	}
	if lerr := m.stopLocked(ctx, id, rec.SessionID); lerr != nil && lerr.Status != 404 {
		return lerr
	}
	return m.resumeLocked(ctx, rec, "restart")
}

// Resume restarts an orphaned lane (registered, with no tmux session: after a
// reboot, or a tmux server that died) on its own session id.
func (m *LaneManager) Resume(ctx context.Context, id string) *LaneError {
	if !config.ValidLaneID(id) {
		return laneErr(400, "invalid", "invalid lane id")
	}
	if why := m.StartBlocked(ctx); why != "" {
		return laneErr(409, "api-key", "%s", why)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	rec, ok := m.Registry.Get(id)
	if !ok {
		return laneErr(404, "not-found", "no registered lane %q", id)
	}
	return m.resumeLocked(ctx, rec, "resume")
}

// Forget removes an orphan from the registry. A lane that is still running must be
// stopped instead.
func (m *LaneManager) Forget(ctx context.Context, id string) *LaneError {
	if !config.ValidLaneID(id) {
		return laneErr(400, "invalid", "invalid lane id")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Exists(ctx, id) {
		return laneErr(409, "exists", "lane %q is running; stop it instead", id)
	}
	if _, ok := m.Registry.Get(id); !ok {
		return laneErr(404, "not-found", "no registered lane %q", id)
	}
	if err := m.Registry.Delete(id); err != nil {
		return laneErr(500, "registry", "%v", err)
	}
	m.dropImages(id)
	m.changed()
	return nil
}

// OpenInTerminalApp opens a Terminal.app window attached to the lane.
func (m *LaneManager) OpenInTerminalApp(ctx context.Context, id string) *LaneError {
	if !m.Exists(ctx, id) {
		return laneErr(404, "not-found", "no lane %q", id)
	}
	argv := m.TerminalAppArgv(id)
	cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	out, err := exec.CommandContext(cctx, argv[0], argv[1:]...).CombinedOutput()
	if err != nil {
		return laneErr(500, "osascript", "opening Terminal.app failed: %v: %s (macOS may need you to allow the panel to control Terminal, in System Settings → Privacy & Security → Automation)",
			err, strings.TrimSpace(string(out)))
	}
	return nil
}

// withTemplate copies a rendered template's launch options and first prompt into the
// record written before the lane starts, so a restart or a panel restart keeps them.
func (req StartRequest) withTemplate(rec types.LaneRecord) types.LaneRecord {
	if req.tpl == nil {
		return rec
	}
	rec.Template, rec.Model, rec.Effort = req.tpl.Template, req.tpl.Model, req.tpl.Effort
	rec.FirstPrompt, rec.PromptState = req.tpl.FirstPrompt, "pending"
	return rec
}

// launchOptions are a lane's model and effort: its lane type's, overridden by the
// template it was started from (recorded in the registry).
func (m *LaneManager) launchOptions(id, laneType string) config.LaneTypeConfig {
	lt := m.Cfg.LaneTypes[laneType]
	if m.Registry != nil {
		if rec, ok := m.Registry.Get(id); ok {
			if rec.Model != "" {
				lt.Model = rec.Model
			}
			if rec.Effort != "" {
				lt.Effort = rec.Effort
			}
		}
	}
	return lt
}

// StartGate holds what StartLane checks besides the request itself: whether the
// config is trusted (templates are repo-controlled prompts) and the quota guard.
type StartGate struct {
	Trusted    func() bool
	QuotaGuard func() string // why the quota refuses a new lane now, or ""
}

// StartLane is the v2 start: an optional template, then the quota guard, then Start.
func (m *LaneManager) StartLane(ctx context.Context, req StartRequest, g StartGate) (StartResult, *LaneError) {
	if req.Template != "" {
		if g.Trusted != nil && !g.Trusted() {
			return StartResult{}, laneErr(409, "untrusted-config", "panel.json is not trusted as it is now, so its templates are off; review it and run `clauductor panel trust`")
		}
		r, err := m.Cfg.RenderTemplate(req.Template, req.Name, req.Issue)
		if err != nil {
			return StartResult{}, laneErr(400, "invalid", "%v", err)
		}
		if req.Type != "" && req.Type != r.LaneType {
			return StartResult{}, laneErr(400, "invalid", "template %q starts a %s lane, not %s", req.Template, r.LaneType, req.Type)
		}
		if req.Mode != "" && req.Mode != "new" {
			return StartResult{}, laneErr(400, "invalid", "a template lane starts on a new branch and worktree")
		}
		req.Type, req.Mode, req.tpl = r.LaneType, "new", &r
	} else if req.Issue != "" {
		return StartResult{}, laneErr(400, "invalid", "issue is only for templates")
	}
	if g.QuotaGuard != nil && !req.OverrideQuota {
		if why := g.QuotaGuard(); why != "" {
			return StartResult{}, laneErr(409, "quota", "%s. Tick the override to start it anyway.", why)
		}
	}
	return m.Start(ctx, req)
}

// SetPromptState moves a template lane's first prompt to a new state.
func (m *LaneManager) SetPromptState(id, from, to string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.Registry.Update(id, func(r *types.LaneRecord) bool {
		if r.PromptState != from {
			return false
		}
		r.PromptState = to
		return true
	})
}

// DeliverFirstPrompt types a template lane's first prompt: the text, then Enter as a
// separate write. It holds the lane lock, so no start, stop or restart interleaves,
// and it writes "typing" to the registry BEFORE the first keystroke: a panel that
// dies mid-typing leaves "typing", which is never typed again. The caller has
// already decided, from `claude agents`, that claude is idle.
func (m *LaneManager) DeliverFirstPrompt(ctx context.Context, id string, stillReady func() string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec, ok := m.Registry.Get(id)
	if !ok || rec.PromptState != "pending" || rec.FirstPrompt == "" {
		return fmt.Errorf("lane %q has no pending first prompt", id)
	}
	if l, ok := m.find(ctx, id); !ok || l.Dead {
		return fmt.Errorf("lane %q is not running", id)
	}
	// Re-checked under the lane lock, right before the first keystroke: the model's
	// view (a waiting note from a hook), then a fresh `claude agents` read.
	if stillReady != nil {
		if why := stillReady(); why != "" {
			return fmt.Errorf("not typed: %s", why)
		}
	}
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	out, err := m.Run(cctx, m.Root, []string{"claude", "agents", "--json"})
	cancel()
	var agents []signals.Agent
	if err == nil {
		agents, err = signals.ParseAgents(out)
	}
	if err != nil {
		return fmt.Errorf("not typed: cannot read claude agents: %v", err)
	}
	if why := agentReady(agents, rec.SessionID); why != "" {
		return fmt.Errorf("not typed: %s", why)
	}
	// Re-validated at the moment of typing: a registry edited by hand must not be
	// able to smuggle a newline or an escape sequence into the lane.
	if err := config.TypableText(rec.FirstPrompt, config.MaxFirstPrompt); err != nil {
		_ = m.Registry.Update(id, func(r *types.LaneRecord) bool { r.PromptState = "skipped"; return true })
		return fmt.Errorf("first prompt %v; not typed", err)
	}
	if err := m.Registry.Update(id, func(r *types.LaneRecord) bool {
		if r.PromptState != "pending" {
			return false
		}
		r.PromptState, r.PromptAt = "typing", m.now().UnixMilli()
		return true
	}); err != nil {
		return err
	}
	if err := m.sendText(ctx, id, rec.FirstPrompt); err != nil {
		return err // stays "typing": shown in Needs you, never retyped
	}
	return m.Registry.Update(id, func(r *types.LaneRecord) bool {
		r.PromptState, r.PromptAt = "sent", m.now().UnixMilli()
		return true
	})
}

// agentReady says why a session is not ready for typed text, or "" when it is:
// listed, idle, and waiting for nothing.
func agentReady(agents []signals.Agent, sessionID string) string {
	for _, a := range agents {
		if a.SessionID != sessionID {
			continue
		}
		switch {
		case a.Status != "idle":
			return "claude is " + a.Status
		case a.WaitingFor != "":
			return "claude is waiting for " + signals.OneLine(a.WaitingFor)
		}
		return ""
	}
	return "the session is not in claude agents"
}

// RestoreSkip is a lane RestoreAll did not restore, and why.
type RestoreSkip struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

// selectRestorable picks the registered lanes to restore: those whose tmux session
// is gone. It never picks a session twice: not one that a claude process already
// runs (`live`), and not the same session id for two lanes. Two processes on one
// session interleave its transcript.
func selectRestorable(recs []types.LaneRecord, running map[string]bool, live map[string]bool, dirOK func(string) bool) ([]types.LaneRecord, []RestoreSkip) {
	sort.Slice(recs, func(i, j int) bool { return recs[i].ID < recs[j].ID })
	var out []types.LaneRecord
	var skip []RestoreSkip
	seen := map[string]string{}
	for _, r := range recs {
		switch {
		case running[r.ID]:
			continue // not lost
		case r.Corrupt != "":
			skip = append(skip, RestoreSkip{r.ID, "corrupt registry record (" + r.Corrupt + "): never launched"})
		case !uuidRe.MatchString(r.SessionID):
			skip = append(skip, RestoreSkip{r.ID, "no valid session id"})
		case live[r.SessionID]:
			skip = append(skip, RestoreSkip{r.ID, "session " + r.SessionID + " already runs in another claude process"})
		case seen[r.SessionID] != "":
			skip = append(skip, RestoreSkip{r.ID, "session " + r.SessionID + " is also lane " + seen[r.SessionID] + "'s; restored once only"})
		case !dirOK(r.Path):
			skip = append(skip, RestoreSkip{r.ID, r.Path + " no longer exists; forget the lane"})
		default:
			seen[r.SessionID] = r.ID
			out = append(out, r)
		}
	}
	return out, skip
}

// liveSessions reads the session ids claude processes run now.
func (m *LaneManager) liveSessions(ctx context.Context) (map[string]bool, error) {
	out, err := m.Run(ctx, m.Root, []string{"claude", "agents", "--json"})
	if err != nil {
		return nil, err
	}
	agents, err := signals.ParseAgents(out)
	if err != nil {
		return nil, err
	}
	live := map[string]bool{}
	for _, a := range agents {
		live[a.SessionID] = true
	}
	return live, nil
}

// RestoreResult reports a restore.
type RestoreResult struct {
	Restored []string      `json:"restored"`
	Skipped  []RestoreSkip `json:"skipped"`
}

// RestoreAll resumes every lane a reboot (or a dead tmux server) took away, each on
// its own session id: `claude --resume <id>`, never --continue. Nothing is typed into
// a restored lane: claude may first show its resume-from-summary dialog, which "Needs
// you" points at.
func (m *LaneManager) RestoreAll(ctx context.Context, quotaGuard func() string, override bool) (RestoreResult, *LaneError) {
	res := RestoreResult{Restored: []string{}, Skipped: []RestoreSkip{}}
	if why := m.StartBlocked(ctx); why != "" {
		return res, laneErr(409, "api-key", "%s", why)
	}
	if quotaGuard != nil && !override {
		if why := quotaGuard(); why != "" {
			return res, laneErr(409, "quota", "%s. Tick the override to restore anyway.", why)
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	lanes, err := m.List(ctx)
	if err != nil {
		return res, laneErr(500, "tmux", "%v", err)
	}
	running := map[string]bool{}
	for _, l := range lanes {
		running[l.ID] = true
	}
	live, err := m.liveSessions(ctx)
	if err != nil {
		return res, laneErr(409, "unverified", "cannot read claude agents (%v), so no session can be shown not to be running already; nothing restored", err)
	}
	pick, skip := selectRestorable(m.Registry.List(), running, live, func(p string) bool {
		fi, err := os.Stat(p)
		return err == nil && fi.IsDir()
	})
	res.Skipped = append(res.Skipped, skip...)
	for _, rec := range pick {
		if lerr := m.resumeLocked(ctx, rec, "restore"); lerr != nil {
			res.Skipped = append(res.Skipped, RestoreSkip{rec.ID, lerr.Msg})
			continue
		}
		m.MarkRestored(rec.ID)
		res.Restored = append(res.Restored, rec.ID)
	}
	return res, nil
}

// MarkRestored records when a lane was restored, for "Needs you".
func (m *LaneManager) MarkRestored(id string) {
	_ = m.Registry.Update(id, func(r *types.LaneRecord) bool { r.Restored = m.now().UnixMilli(); return true })
}

// InCopyMode reports whether the lane's pane is scrolled back in tmux's copy mode.
func (m *LaneManager) InCopyMode(ctx context.Context, id string) bool {
	out, err := m.tmux(ctx, "display-message", "-p", "-t", "="+id+":", "#{pane_in_mode}")
	return err == nil && strings.TrimSpace(string(out)) == "1"
}

// LeaveCopyMode returns the lane's pane to the live screen.
func (m *LaneManager) LeaveCopyMode(ctx context.Context, id string) {
	_, _ = m.tmux(ctx, "send-keys", "-t", "="+id+":", "-X", "cancel")
}
