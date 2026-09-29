package panel

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// PANEL-5: one panel per machine, enforced.
//
// The hook URL in ~/.claude/settings.json, ~/.clauductor/panel/{port,pid,token} and
// the launchd label are all per-machine. A second panel on another port would
// silently re-point every session's hooks at itself, and its exit would delete the
// first panel's marker. So a panel refuses to start while the pid file names another
// LIVE panel, and removes the marker files at exit only while they are still its
// own. Running several projects from one panel (a multi-project daemon) is future
// work; until then, one project per machine at a time.

// PanelOwner is ~/.clauductor/panel/owner.json: who holds the machine's panel files.
// The pid file stays digits only (`clauductor panel open` and scripts read it); the
// start time beside it is what tells a live panel from a reused pid.
type PanelOwner struct {
	PID     int    `json:"pid"`
	PStart  string `json:"pstart"`
	Project string `json:"project"`
	Name    string `json:"name,omitempty"`
	Port    int    `json:"port"`
	Started int64  `json:"started"`
}

// OwnerPath is the running panel's owner record.
func OwnerPath(home string) string { return filepath.Join(panelDir(home), "owner.json") }

func pidPath(home string) string { return filepath.Join(panelDir(home), "pid") }

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
func RunningPanel(ctx context.Context, home string, self int, proc ProcCheck) *PanelOwner {
	pid := readPIDFile(home)
	if pid <= 0 || pid == self {
		return nil
	}
	alive, start := proc(pid)
	if !alive {
		return nil
	}
	var o PanelOwner
	if b, err := os.ReadFile(OwnerPath(home)); err == nil && json.Unmarshal(b, &o) == nil && o.PID == pid &&
		o.PStart != "" && start != "" && sameSource(o.PStart, start) {
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
	what := "an older panel (its project is not recorded)"
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
	return fmt.Errorf("another clauductor panel is running on this machine: %s, pid %d%s. "+
		"There is one panel per machine: its hooks in ~/.claude/settings.json and the files in ~/.clauductor/panel/ "+
		"are per-machine, so a second panel would re-point every session's hooks at itself. Stop that one first "+
		"(Ctrl-C in its terminal, or `launchctl bootout %s/%s` for the login agent), or open it with `clauductor panel open`",
		what, o.PID, port, guiDomain(), LaunchdLabel)
}

// claimPanelFiles records this process as the machine's panel: owner.json and pid,
// then the port marker last (status-line scripts post once it exists).
func claimPanelFiles(home string, o PanelOwner) error {
	if err := ensurePrivateDir(panelDir(home)); err != nil {
		return err
	}
	b, _ := json.Marshal(o)
	if err := writeAtomic(OwnerPath(home), append(b, '\n'), 0o600); err != nil {
		return err
	}
	if err := writeAtomic(pidPath(home), []byte(strconv.Itoa(o.PID)+"\n"), 0o600); err != nil {
		return err
	}
	return writeAtomic(MarkerPath(home), []byte(strconv.Itoa(o.Port)+"\n"), 0o600)
}

// releasePanelFiles removes the marker, pid and owner files only while the pid file
// still names this process. If another panel claimed them since, they are its.
func releasePanelFiles(home string, self int) {
	if readPIDFile(home) != self {
		return
	}
	_ = os.Remove(MarkerPath(home))
	_ = os.Remove(OwnerPath(home))
	_ = os.Remove(pidPath(home))
}

// ---- hook health ----

// HookDrift describes how the hooks in settings.json differ from this panel's, or
// "" when every subscribed event carries this panel's hook and no other panel's.
func HookDrift(home string, port int) (string, error) {
	b, err := os.ReadFile(SettingsPath(home))
	if os.IsNotExist(err) {
		return "they had been removed", nil
	}
	if err != nil {
		return "", err
	}
	var doc struct {
		Hooks map[string][]struct {
			Hooks []json.RawMessage `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return "", fmt.Errorf("%s: %w", SettingsPath(home), err)
	}
	want := HookURL(port)
	foreign := map[string]bool{}
	var missing []string
	for _, ev := range HookEvents {
		found := false
		for _, g := range doc.Hooks[ev] {
			for _, h := range g.Hooks {
				if !isOurs(h) {
					continue
				}
				var u struct {
					URL string `json:"url"`
				}
				_ = json.Unmarshal(h, &u)
				if u.URL == want {
					found = true
				} else {
					foreign[u.URL] = true
				}
			}
		}
		if !found {
			missing = append(missing, ev)
		}
	}
	switch {
	case len(foreign) > 0:
		urls := make([]string, 0, len(foreign))
		for u := range foreign {
			urls = append(urls, u)
		}
		sort.Strings(urls)
		return "they pointed at " + strings.Join(urls, ", ") + ", another panel's address", nil
	case len(missing) == len(HookEvents):
		return "they had been removed", nil
	case len(missing) > 0:
		return "missing for " + strings.Join(missing, ", "), nil
	}
	return "", nil
}

// hookKeeper keeps this panel's hooks installed: at start, then every interval.
// A failed install is a banner and a retry with backoff, never a fatal error (under
// launchd a fatal error restarts the panel every 30 s).
type hookKeeper struct {
	home      string
	port      int
	hub       *Hub
	out       io.Writer
	interval  time.Duration
	retryBase time.Duration
	installed bool // an install has succeeded at least once
}

// check installs (or verifies) the hooks once, and reports success.
func (k *hookKeeper) check() bool {
	drift := ""
	if k.installed {
		drift, _ = HookDrift(k.home, k.port)
	}
	changed, err := InstallHooks(k.home, k.port)
	if err != nil {
		k.hub.Update(func(m *Model, now time.Time) { m.ApplyHookHealth(err, "", now) })
		fmt.Fprintf(k.out, "installing hooks failed (the panel keeps running and retries): %v\n", err)
		return false
	}
	switch {
	case !k.installed && changed:
		fmt.Fprintf(k.out, "Installed panel hooks in %s (pre-panel backup: settings.json.clauductor-panel.bak). Running sessions pick them up live (Claude Code 2.1.284); restart any that do not.\n", SettingsPath(k.home))
	case !k.installed:
		fmt.Fprintf(k.out, "Panel hooks already present in %s.\n", SettingsPath(k.home))
	case changed:
		if drift == "" {
			drift = "they had been changed"
		}
		fmt.Fprintf(k.out, "panel hooks in %s had drifted (%s); reinstalled\n", SettingsPath(k.home), drift)
	}
	repaired := ""
	if k.installed && changed {
		repaired = drift
	}
	k.installed = true
	k.hub.Update(func(m *Model, now time.Time) { m.ApplyHookHealth(nil, repaired, now) })
	return true
}

// loop re-checks every interval after a success; after a failure it retries with a
// delay that doubles from retryBase up to the interval.
func (k *hookKeeper) loop(ctx context.Context, ok bool) {
	delay := k.retryBase
	for {
		wait := k.interval
		if ok {
			delay = k.retryBase
		} else {
			wait = delay
			if delay *= 2; delay > k.interval {
				delay = k.interval
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		ok = k.check()
	}
}

// hookRepairShown is how long the page keeps saying the hooks were repaired.
const hookRepairShown = 10 * time.Minute

// hookHealth is the hook install's state, as the model holds it.
type hookHealth struct {
	err        string
	repairedAt time.Time
	drift      string
}

// ApplyHookHealth records one hook check: an install error, or success (with what
// had drifted, when the check had to repair it).
func (m *Model) ApplyHookHealth(err error, repaired string, now time.Time) {
	if err != nil {
		m.hooks.err = err.Error()
		return
	}
	m.hooks.err = ""
	if repaired != "" {
		m.hooks.repairedAt, m.hooks.drift = now, repaired
	}
}

// hookBanners adds the hook install's banner (it failed, so lanes are blind) or its
// warning (it drifted and was repaired).
func (m *Model) hookBanners(v *View, now time.Time) {
	if m.hooks.err != "" {
		v.Banners = append(v.Banners, "The panel's hooks could not be installed in ~/.claude/settings.json ("+m.hooks.err+
			"). The panel keeps retrying; until it succeeds, lanes update only from `claude agents` polls.")
		return
	}
	if !m.hooks.repairedAt.IsZero() && now.Sub(m.hooks.repairedAt) < hookRepairShown {
		v.Warnings = append(v.Warnings, "The panel's hooks in ~/.claude/settings.json had drifted ("+m.hooks.drift+
			") and were reinstalled at "+m.hooks.repairedAt.Local().Format("15:04:05")+
			". If another clauductor panel keeps re-pointing them, stop it: there is one panel per machine.")
	}
}
