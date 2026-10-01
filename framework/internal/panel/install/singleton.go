package install

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/clauductor/clauductor/internal/panel/clock"
	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/lease"
	"github.com/clauductor/clauductor/internal/panel/signals"
)

// PANEL-5: one panel per machine, enforced.
//
// The hook URL in ~/.claude/settings.json, ~/.clauductor/panel/{port,pid,token} and
// the launchd label are all per-machine. A second panel on another port would
// silently re-point every session's hooks at itself, and its exit would delete the
// first panel's marker. So a panel holds flock(2) on ~/.clauductor/panel/lock for
// its whole life (the kernel drops it on any exit, SIGKILL included), refuses to
// start while another process holds it or while the pid file names another LIVE
// panel (one from before the lock), and removes the marker files at exit only while
// they are still its own. Running several projects from one panel (a multi-project daemon) is future
// work; until then, one project per machine at a time.

// PanelOwner is ~/.clauductor/panel/owner.json: who holds the machine's panel files.
// The pid file stays digits only (`clauductor panel open` and scripts read it); the
// start time beside it is what tells a live panel from a reused pid.
type PanelOwner struct {
	PID     int    `json:"pid"`
	PStart  string `json:"pstart"`
	Project string `json:"project"` // the default project, for readers from before PANEL-16
	Name    string `json:"name,omitempty"`
	Port    int    `json:"port"`
	Started int64  `json:"started"`
	// Projects are the roots of every project it serves (PANEL-16).
	Projects []string `json:"projects,omitempty"`
}

// ownerPath is the running panel's owner record.
func ownerPath(home string) string { return filepath.Join(config.PanelDir(home), "owner.json") }

// LockPath is the machine lock a running panel holds with flock(2). The file itself
// is never removed: a new inode would let a second panel lock it too.
func LockPath(home string) string { return filepath.Join(config.PanelDir(home), "lock") }

// OtherPanelError is a start refused because another panel holds the machine.
type OtherPanelError struct{ Owner PanelOwner }

func (e *OtherPanelError) Error() string { return errOtherPanel(&e.Owner).Error() }

// LockMachine takes the machine lock without waiting. It fails with an
// *OtherPanelError naming the holder (as far as owner.json says) when another
// panel holds it. The lock lasts until the returned file is closed or the process
// exits, however it exits.
func LockMachine(home string) (*os.File, error) {
	if err := config.EnsurePrivateDir(config.PanelDir(home)); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(LockPath(home), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, fmt.Errorf("locking %s: %w", LockPath(home), err)
		}
		var o PanelOwner
		if b, rerr := os.ReadFile(ownerPath(home)); rerr == nil {
			_ = json.Unmarshal(b, &o)
		}
		if o.PID == 0 {
			o.PID = readPIDFile(home)
		}
		return nil, &OtherPanelError{Owner: o}
	}
	return f, nil
}

// WaitMachineLock takes the machine lock, waiting (flock LOCK_EX, blocking) for
// the panel that holds it to exit (PANEL-7: the login agent takes over from a
// panel started by hand). It gives up when ctx ends; a lock taken after that is
// released at once.
func WaitMachineLock(ctx context.Context, home string) (*os.File, error) {
	if err := config.EnsurePrivateDir(config.PanelDir(home)); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(LockPath(home), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	done := make(chan error, 1)
	go func() {
		for {
			err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
			if !errors.Is(err, syscall.EINTR) {
				done <- err
				return
			}
		}
	}()
	select {
	case err := <-done:
		if err != nil {
			f.Close()
			return nil, fmt.Errorf("locking %s: %w", LockPath(home), err)
		}
		return f, nil
	case <-ctx.Done():
		// The flock call cannot be interrupted; close the file (releasing the lock)
		// once it returns.
		go func() { <-done; f.Close() }()
		return nil, ctx.Err()
	}
}

// Describe names a panel owner in one line.
func (o PanelOwner) Describe() string {
	s := "pid unknown"
	if o.PID > 0 {
		s = fmt.Sprintf("pid %d", o.PID)
	}
	if o.Project != "" {
		s += ", " + o.Project
	}
	if n := len(o.Projects); n > 1 {
		s += fmt.Sprintf(" and %d other project(s)", n-1)
	}
	return s
}

func pidPath(home string) string { return filepath.Join(config.PanelDir(home), "pid") }

func readPIDFile(home string) int {
	b, err := os.ReadFile(pidPath(home))
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		return 0
	}
	return pid
}

// RunningPanel returns the other panel that is live on this machine, or nil. The
// pid file is the authority on who holds the panel files:
//   - its pid dead, or this process: none;
//   - owner.json names that pid with a start time from the same source (ps or
//     /proc) as the one read now: live exactly when they match (else a reused pid);
//   - otherwise (a panel from before owner.json, or start times that cannot be
//     compared): live only if the marker's port answers /healthz as that pid.
func RunningPanel(ctx context.Context, home string, self int, proc lease.ProcCheck) *PanelOwner {
	pid := readPIDFile(home)
	if pid <= 0 || pid == self {
		return nil
	}
	alive, start := proc(pid)
	if !alive {
		return nil
	}
	var o PanelOwner
	if b, err := os.ReadFile(ownerPath(home)); err == nil && json.Unmarshal(b, &o) == nil && o.PID == pid &&
		o.PStart != "" && start != "" && lease.SameSource(o.PStart, start) {
		if o.PStart != start {
			return nil
		}
		return &o
	}
	b, err := os.ReadFile(MarkerPath(home))
	if err != nil {
		return nil
	}
	port, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || port <= 0 {
		return nil
	}
	got, err := healthz(ctx, "http://127.0.0.1:"+strconv.Itoa(port))
	if err != nil || got != "ok pid="+strconv.Itoa(pid) {
		return nil
	}
	return &PanelOwner{PID: pid, Port: port}
}

// errOtherPanel is the refusal: it names the running panel's project and pid.
func errOtherPanel(o *PanelOwner) error {
	what := "a panel whose project is not recorded (an older panel, or one still starting)"
	if o.Project != "" {
		what = o.Project
		if o.Name != "" {
			what = o.Name + " (" + o.Project + ")"
		}
	}
	port := ""
	if o.Port > 0 {
		port = fmt.Sprintf(" on port %d", o.Port)
	}
	pid := "pid unknown"
	if o.PID > 0 {
		pid = fmt.Sprintf("pid %d", o.PID)
	}
	return fmt.Errorf("another clauductor panel is running on this machine: %s, %s%s. "+
		"There is one panel per machine: its hooks in ~/.claude/settings.json and the files in ~/.clauductor/panel/ "+
		"are per-machine, so a second panel would re-point every session's hooks at itself. Stop that one first "+
		"(Ctrl-C in its terminal, or `launchctl bootout %s/%s` for the login agent), or open it with `clauductor panel open`",
		what, pid, port, guiDomain(), LaunchdLabel)
}

// ClaimPanelFiles records this process as the machine's panel: owner.json and pid,
// then the port marker last (status-line scripts post once it exists).
func ClaimPanelFiles(home string, o PanelOwner) error {
	if err := config.EnsurePrivateDir(config.PanelDir(home)); err != nil {
		return err
	}
	b, _ := json.Marshal(o)
	if err := config.WriteAtomic(ownerPath(home), append(b, '\n'), 0o600); err != nil {
		return err
	}
	if err := config.WriteAtomic(pidPath(home), []byte(strconv.Itoa(o.PID)+"\n"), 0o600); err != nil {
		return err
	}
	return config.WriteAtomic(MarkerPath(home), []byte(strconv.Itoa(o.Port)+"\n"), 0o600)
}

// UpdatePanelOwner rewrites owner.json while it is still this process's (PANEL-22: a
// project added or removed live changes Projects, a new default Project and Name).
// `panel add` and `panel remove` read Projects to tell whether the running panel
// took their change.
func UpdatePanelOwner(home string, o PanelOwner) error {
	if readPIDFile(home) != o.PID {
		return nil
	}
	b, _ := json.Marshal(o)
	return config.WriteAtomic(ownerPath(home), append(b, '\n'), 0o600)
}

// WaitPanelServes waits until the running panel's owner record lists root (serves
// true) or does not (false), polling until ctx ends. It reports whether it did: a
// panel from before PANEL-22 never follows projects.json, so the caller then says to
// restart it.
func WaitPanelServes(ctx context.Context, clk clock.Clock, home string, pid int, root string, serves bool) bool {
	for {
		var o PanelOwner
		if b, err := os.ReadFile(ownerPath(home)); err == nil && json.Unmarshal(b, &o) == nil && o.PID == pid {
			has := false
			for _, p := range o.Projects {
				has = has || p == root
			}
			if has == serves {
				return true
			}
		}
		select {
		case <-ctx.Done():
			return false
		case <-clk.After(100 * time.Millisecond):
		}
	}
}

// ReleasePanelFiles removes the marker, pid and owner files only while the pid file
// still names this process. If another panel claimed them since, they are its.
func ReleasePanelFiles(home string, self int) {
	if readPIDFile(home) != self {
		return
	}
	_ = os.Remove(MarkerPath(home))
	_ = os.Remove(ownerPath(home))
	_ = os.Remove(pidPath(home))
}

// ---- hook health ----

// HookDrift is how the hooks in settings.json differ from this panel's.
type HookDrift struct {
	Text  string // "" when every subscribed event carries this panel's hook and no other's
	Ports []int  // the loopback ports other panel-tagged hooks point at
}

// ReadHookDrift compares the hooks in settings.json with this panel's.
func ReadHookDrift(home string, port int) (HookDrift, error) {
	b, err := os.ReadFile(SettingsPath(home))
	if os.IsNotExist(err) {
		return HookDrift{Text: "they had been removed"}, nil
	}
	if err != nil {
		return HookDrift{}, err
	}
	var doc struct {
		Hooks map[string][]struct {
			Hooks []json.RawMessage `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return HookDrift{}, fmt.Errorf("%s: %w", SettingsPath(home), err)
	}
	want := hookURL(port)
	current, _ := marshalRaw(hookEntry(port))
	foreign := map[string]bool{}
	var missing, outdated []string
	for _, ev := range signals.HookEvents {
		found, stale := false, false
		for _, g := range doc.Hooks[ev] {
			for _, h := range g.Hooks {
				target := hookTarget(h)
				switch {
				case target == "":
				case target != want:
					foreign[target] = true
				case sameJSON(h, current):
					found = true
				default: // this panel's address in an older form (the HTTP hook before PANEL-23)
					stale = true
				}
			}
		}
		switch {
		case found:
		case stale:
			outdated = append(outdated, ev)
		default:
			missing = append(missing, ev)
		}
	}
	var d HookDrift
	switch {
	case len(foreign) > 0:
		urls := make([]string, 0, len(foreign))
		for u := range foreign {
			urls = append(urls, u)
			if pu, err := url.Parse(u); err == nil {
				if p, err := strconv.Atoi(pu.Port()); err == nil && p > 0 {
					d.Ports = append(d.Ports, p)
				}
			}
		}
		sort.Strings(urls)
		sort.Ints(d.Ports)
		d.Text = "they pointed at " + strings.Join(urls, ", ") + ", another panel's address"
	case len(missing) == len(signals.HookEvents):
		d.Text = "they had been removed"
	case len(missing) > 0:
		d.Text = "missing for " + strings.Join(missing, ", ")
	case len(outdated) > 0:
		d.Text = "they were an older form (a synchronous HTTP hook)"
	}
	return d, nil
}

// LivePanelAt returns the pid of the panel answering /healthz on a loopback port,
// or 0 when nothing (or something else) answers.
func LivePanelAt(ctx context.Context, port int) int {
	got, err := healthz(ctx, "http://127.0.0.1:"+strconv.Itoa(port))
	if err != nil || !strings.HasPrefix(got, "ok pid=") {
		return 0
	}
	pid, _ := strconv.Atoi(strings.TrimPrefix(got, "ok pid="))
	return pid
}
