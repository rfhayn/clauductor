package panel

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// A lane is one interactive `claude` in its own tmux session on the panel's dedicated
// socket. tmux, not the panel, owns the process, so lanes survive a panel restart
// and several viewers (browser tabs, Terminal.app) can attach to one lane.
//
// Every tmux, git and osascript invocation here is an argv list run without a shell.
// The only inputs a browser can supply are a lane id (validated below), a lane type
// (checked against the config), a worktree path (checked against `git worktree
// list`) and a flag. It never supplies a command.

var laneIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,40}$`)

// ValidLaneID reports whether id can name a lane (and so a tmux session and a
// worktree directory). It is also what keeps an id safe inside a tmux target and a
// Terminal.app command.
func ValidLaneID(id string) bool { return laneIDRe.MatchString(id) }

// branchRe is checked before `git check-ref-format`, so nothing that looks like an
// option (a leading "-") ever reaches git.
var branchRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,199}$`)

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

// TmuxLane is one session on the panel's socket.
type TmuxLane struct {
	ID         string
	Path       string // the pane's cwd (resolved), falling back to the session's start dir
	Type       string // the lane type the panel started it as (@clauductor_type)
	Created    int64  // unix seconds
	Attached   int    // attached clients
	Dead       bool   // the lane's program exited (remain-on-exit keeps its output)
	DeadStatus string
}

const tmuxListFormat = "#{session_name}\t#{pane_current_path}\t#{session_path}\t#{pane_dead}\t#{pane_dead_status}\t#{session_created}\t#{session_attached}\t#{@clauductor_type}"

// ParseTmuxPanes parses `list-panes -a -F tmuxListFormat`, one lane per session. A
// session whose name is not a valid lane id was not started by the panel and is
// ignored.
func ParseTmuxPanes(out []byte) []TmuxLane {
	seen := map[string]bool{}
	var lanes []TmuxLane
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		f := strings.Split(sc.Text(), "\t")
		if len(f) < 8 || !ValidLaneID(f[0]) || seen[f[0]] {
			continue
		}
		seen[f[0]] = true
		path := f[1]
		if path == "" {
			path = f[2]
		}
		created, _ := strconv.ParseInt(f[5], 10, 64)
		attached, _ := strconv.Atoi(f[6])
		lanes = append(lanes, TmuxLane{ID: f[0], Path: ResolvePath(path), Type: f[7], Created: created,
			Attached: attached, Dead: f[3] == "1", DeadStatus: f[4]})
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
	TmuxPath string    // absolute path of tmux
	Socket   string    // tmux -L name
	Root     string    // project root (resolved)
	Cfg      *Config   //
	Registry *Registry // durable lane ↔ session binding
	Run      Runner    // runs git and `claude agents` (injectable for tests)
	Program  []string  // the lane program; default the absolute path of `claude`
	// LookupEnv reads the panel's own environment (injectable for tests).
	LookupEnv func(string) (string, bool)
	// StopTimeout is how long Stop waits for /exit before killing the session.
	StopTimeout time.Duration
	// EnterDelay separates typed text from its Enter: sent together, a long line
	// can sit in claude's input box unsubmitted.
	EnterDelay time.Duration
	// Changed is called after a lane is started or stopped (re-poll now).
	Changed func()
	// Stopped is called once a lane's tmux session is gone, to close its viewers.
	Stopped func(id string)
	// Now is the clock for registry timestamps.
	Now func() time.Time

	mu sync.Mutex // serialises lane actions
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

// tmuxEnv is the environment of every tmux command the panel runs. The first one
// starts the socket's server, whose global environment every lane inherits.
func tmuxEnv() []string {
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

// TmuxArgv prefixes a tmux command with the socket.
func (m *LaneManager) TmuxArgv(args ...string) []string {
	return append([]string{"-L", m.Socket}, args...)
}

func (m *LaneManager) tmux(ctx context.Context, args ...string) ([]byte, error) {
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, m.TmuxPath, m.TmuxArgv(args...)...)
	cmd.Env = tmuxEnv()
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

// noServer reports whether a tmux error only means the socket has no server yet.
func noServer(err error) bool {
	s := err.Error()
	return strings.Contains(s, "no server running") || strings.Contains(s, "error connecting to")
}

// List returns the lanes on the socket. No server means no lanes, not an error.
func (m *LaneManager) List(ctx context.Context) ([]TmuxLane, error) {
	out, err := m.tmux(ctx, "list-panes", "-a", "-F", tmuxListFormat)
	if err != nil {
		if noServer(err) {
			return nil, nil
		}
		return nil, err
	}
	return ParseTmuxPanes(out), nil
}

// Exists reports whether a lane's tmux session is running. "=" makes the match
// exact; without it tmux would take "lane" to mean "lane-2".
func (m *LaneManager) Exists(ctx context.Context, id string) bool {
	if !ValidLaneID(id) {
		return false
	}
	_, err := m.tmux(ctx, "has-session", "-t", "="+id)
	return err == nil
}

func (m *LaneManager) find(ctx context.Context, id string) (TmuxLane, bool) {
	lanes, _ := m.List(ctx)
	for _, l := range lanes {
		if l.ID == id {
			return l, true
		}
	}
	return TmuxLane{}, false
}

// StartBlocked returns why no lane may start now, or "". An API key in the panel's
// environment, or in the tmux server's global environment that every lane
// inherits, would outrank the subscription login.
func (m *LaneManager) StartBlocked(ctx context.Context) string {
	for _, k := range apiKeyVars {
		if _, ok := m.lookupEnv(k); ok {
			return fmt.Sprintf("%s is set in the panel's environment. It outranks your subscription login, so no lane starts until it is removed and the panel restarted.", k)
		}
	}
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

// LaneCommand is the argv tmux runs for a lane. /usr/bin/env unsets the variables a
// parent Claude session would leak; with two or more arguments tmux execs the
// command directly instead of passing it to a shell.
//
// The session id is the panel's, never discovered: a new lane gets --session-id
// <uuid>, and a restart or restore gets --resume <uuid>. --continue is never used,
// because it picks the directory's most recent conversation, whoever's it is.
func (m *LaneManager) LaneCommand(id, laneType, sessionID string, resume bool) []string {
	argv := []string{"/usr/bin/env"}
	for _, k := range append(append([]string{}, apiKeyVars...), parentSessionVars...) {
		argv = append(argv, "-u", k)
	}
	argv = append(argv, m.Program...)
	lt := m.Cfg.LaneTypes[laneType]
	if lt.Model != "" {
		argv = append(argv, "--model", lt.Model)
	}
	if lt.Effort != "" {
		argv = append(argv, "--effort", lt.Effort)
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
	args = append(args, m.LaneCommand(id, laneType, sessionID, resume)...)
	args = append(args,
		";", "set-option", "-t", "="+id+":", "remain-on-exit", "on",
		";", "set-option", "-t", "="+id+":", "@clauductor_type", laneType,
		";", "set-option", "-t", "="+id+":", "window-size", "latest")
	return m.TmuxArgv(args...)
}

// AttachArgv is the argv (after the tmux path) of one viewer's client. -u forces
// UTF-8 whatever the locale.
func (m *LaneManager) AttachArgv(id string) []string {
	return []string{"-u", "-L", m.Socket, "attach-session", "-t", "=" + id}
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
		cmd}
}

// StartRequest is the body of POST /api/lanes.
type StartRequest struct {
	Type     string `json:"type"`
	Mode     string `json:"mode"`     // "new" (new branch + worktree) | "existing" (a worktree) | "root" (the project root)
	Name     string `json:"name"`     // the lane id; for "new" also the branch name after the type's prefix
	Worktree string `json:"worktree"` // for "existing": a path from git worktree list
}

// StartResult reports a started lane.
type StartResult struct {
	ID        string   `json:"id"`
	SessionID string   `json:"sessionId"`
	Path      string   `json:"path"`
	Branch    string   `json:"branch,omitempty"`
	Notes     []string `json:"notes,omitempty"`
}

func (m *LaneManager) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now()
}

// Start starts a lane: a new claude session, with an id the panel assigns, in the
// project root, an existing worktree, or a new branch's new worktree.
func (m *LaneManager) Start(ctx context.Context, req StartRequest) (StartResult, *LaneError) {
	id := req.Name
	if !ValidLaneID(id) {
		return StartResult{}, laneErr(400, "invalid", "lane name %q must match %s", id, laneIDRe)
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
	sid, err := NewSessionID()
	if err != nil {
		return StartResult{}, laneErr(500, "fault", "%v", err)
	}
	res := StartResult{ID: id, SessionID: sid}
	switch req.Mode {
	case "root":
		res.Path = m.Root
	case "existing":
		wts, err := readWorktrees(ctx, m.Run, m.Root)
		if err != nil {
			return res, laneErr(500, "git", "cannot read the worktree list: %v", err)
		}
		want := ResolvePath(req.Worktree)
		for _, w := range wts {
			if !w.Bare && w.Path == want {
				res.Path, res.Branch = w.Path, w.Branch
			}
		}
		if res.Path == "" {
			return res, laneErr(400, "invalid", "%q is not one of this project's worktrees", req.Worktree)
		}
	case "new":
		prefix := m.Cfg.BranchPrefix(req.Type)
		if prefix == "" {
			return res, laneErr(400, "invalid", "lane type %q has no branch prefix in lanes; pick an existing worktree or the project root", req.Type)
		}
		res.Branch = prefix + id
		if !branchRe.MatchString(res.Branch) {
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
	rec, err := m.Registry.Begin(LaneRecord{ID: id, SessionID: sid, Path: res.Path, Type: req.Type,
		Branch: res.Branch, Mode: req.Mode, Created: m.now().UnixMilli()}, "start", m.now())
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
		res.Path = ResolvePath(res.Path)
		rec.Path = res.Path
	}
	if _, err := m.tmux(ctx, m.NewSessionArgv(id, res.Path, req.Type, sid, false)[2:]...); err != nil {
		return fail(laneErr(500, "tmux", "starting the lane failed: %v", err))
	}
	if err := m.Registry.Done(rec); err != nil {
		res.Notes = append(res.Notes, "the lane started, but the registry could not record it: "+err.Error())
	}
	m.changed()
	return res, nil
}

// sendText types text into a lane, then presses Enter as a separate write.
func (m *LaneManager) sendText(ctx context.Context, id, text string) error {
	if _, err := m.tmux(ctx, "send-keys", "-t", "="+id+":", "-l", text); err != nil {
		return err
	}
	time.Sleep(m.EnterDelay)
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

// Stop asks the lane's claude to /exit, waits up to StopTimeout, then kills the
// tmux session and forgets the lane. The worktree is never removed.
func (m *LaneManager) Stop(ctx context.Context, id string) *LaneError {
	if !ValidLaneID(id) {
		return laneErr(400, "invalid", "invalid lane id")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	rec, registered := m.Registry.Get(id)
	if registered {
		var err error
		if rec, err = m.Registry.Begin(rec, "stop", m.now()); err != nil {
			return laneErr(500, "registry", "cannot write the lane registry: %v", err)
		}
	}
	if lerr := m.stopLocked(ctx, id); lerr != nil {
		if !(lerr.Status == 404 && registered) {
			return lerr
		}
		// Registered but already gone from tmux: stopping it means forgetting it.
	}
	if registered {
		if err := m.Registry.Delete(id); err != nil {
			return laneErr(500, "registry", "the lane stopped, but the registry could not forget it: %v", err)
		}
	}
	return nil
}

func (m *LaneManager) stopLocked(ctx context.Context, id string) *LaneError {
	lane, ok := m.find(ctx, id)
	if !ok {
		return laneErr(404, "not-found", "no lane %q", id)
	}
	defer m.changed()
	if !lane.Dead {
		_ = m.sendText(ctx, id, "/exit")
		deadline := time.Now().Add(m.StopTimeout)
		for time.Now().Before(deadline) {
			l, ok := m.find(ctx, id)
			if !ok || l.Dead {
				break
			}
			time.Sleep(200 * time.Millisecond)
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

// liveSession reports whether a claude process already runs this session id, per
// `claude agents --json`. Two processes on one session interleave its transcript,
// so a failure to read the list refuses rather than guesses.
func (m *LaneManager) liveSession(ctx context.Context, sessionID string) (bool, error) {
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := m.Run(cctx, m.Root, []string{"claude", "agents", "--json"})
	if err != nil {
		return false, err
	}
	agents, err := ParseAgents(out)
	if err != nil {
		return false, err
	}
	for _, a := range agents {
		if a.SessionID == sessionID {
			return true, nil
		}
	}
	return false, nil
}

// resumeLocked restarts a registered lane whose tmux session is gone on its own
// session id, after checking no process holds that session: `claude --resume <id>`
// once the session has a conversation, else `claude --session-id <id>` again
// (--resume refuses a session with no conversation).
func (m *LaneManager) resumeLocked(ctx context.Context, rec LaneRecord, action string) *LaneError {
	if m.Exists(ctx, rec.ID) {
		return laneErr(409, "exists", "lane %q is still running", rec.ID)
	}
	var live bool
	var err error
	for i := 0; i < 25; i++ { // an exiting claude can linger in the list briefly
		if live, err = m.liveSession(ctx, rec.SessionID); err != nil || !live {
			break
		}
		time.Sleep(200 * time.Millisecond)
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
	if _, err := m.tmux(ctx, m.NewSessionArgv(rec.ID, rec.Path, rec.Type, rec.SessionID, rec.Conversation)[2:]...); err != nil {
		return laneErr(500, "tmux", "starting claude --resume failed: %v", err)
	}
	if err := m.Registry.Done(rec); err != nil {
		return laneErr(500, "registry", "the lane resumed, but the registry could not record it: %v", err)
	}
	m.changed()
	return nil
}

// Restart stops a registered lane and resumes its own session in the same place.
func (m *LaneManager) Restart(ctx context.Context, id string) *LaneError {
	if !ValidLaneID(id) {
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
	if lerr := m.stopLocked(ctx, id); lerr != nil && lerr.Status != 404 {
		return lerr
	}
	return m.resumeLocked(ctx, rec, "restart")
}

// Resume restarts an orphaned lane (registered, with no tmux session: after a
// reboot, or a tmux server that died) on its own session id.
func (m *LaneManager) Resume(ctx context.Context, id string) *LaneError {
	if !ValidLaneID(id) {
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
	if !ValidLaneID(id) {
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
