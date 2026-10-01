package panel

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/clauductor/clauductor/internal/panel/install"
	"github.com/clauductor/clauductor/internal/panel/state"
)

// hookKeeper keeps this panel's hooks installed: at start, then every interval.
// A failed install is a banner and a retry with backoff, never a fatal error (under
// launchd a fatal error restarts the panel every 30 s).
type hookKeeper struct {
	home       string
	port       int
	apply      func(update) // applies an update to every project's model
	out        io.Writer
	interval   time.Duration
	retryBase  time.Duration
	installed  bool          // an install has succeeded at least once
	conflicted bool          // the last check found the hooks held by another live panel
	delay      time.Duration // the next retry delay after a failure (backoff)
}

// check installs (or verifies) the hooks once, and reports success. Hooks that
// point at another LIVE panel (one the machine lock could not refuse: an older
// binary) are left alone and a banner says so; fighting over them every interval
// would flap every session between two panels. Once that panel stops answering,
// the next check takes them back.
func (k *hookKeeper) check() bool {
	d, _ := install.ReadHookDrift(k.home, k.port)
	for _, p := range d.Ports {
		if pid := install.LivePanelAt(context.Background(), p); pid > 0 && pid != os.Getpid() {
			k.apply(func(m *state.Model, now time.Time) { m.ApplyHookConflict(pid, p, now) })
			if !k.conflicted {
				fmt.Fprintf(k.out, "the hooks point at another live panel (pid %d, port %d); leaving them alone until it stops\n", pid, p)
			}
			k.conflicted = true
			return true
		}
	}
	k.conflicted = false
	drift := ""
	if k.installed {
		drift = d.Text
	}
	changed, err := install.InstallHooks(k.home, k.port)
	if err != nil {
		k.apply(func(m *state.Model, now time.Time) { m.ApplyHookHealth(err, "", now) })
		fmt.Fprintf(k.out, "installing hooks failed (the panel keeps running and retries): %v\n", err)
		return false
	}
	switch {
	case !k.installed && changed:
		fmt.Fprintf(k.out, "Installed panel hooks in %s (pre-panel backup: settings.json.clauductor-panel.bak). Running sessions pick them up live (Claude Code 2.1.284); restart any that do not.\n", install.SettingsPath(k.home))
	case !k.installed:
		fmt.Fprintf(k.out, "Panel hooks already present in %s.\n", install.SettingsPath(k.home))
	case changed:
		if drift == "" {
			drift = "they had been changed"
		}
		fmt.Fprintf(k.out, "panel hooks in %s had drifted (%s); reinstalled\n", install.SettingsPath(k.home), drift)
	}
	repaired := ""
	if k.installed && changed {
		repaired = drift
	}
	k.installed = true
	k.apply(func(m *state.Model, now time.Time) { m.ApplyHookHealth(nil, repaired, now) })
	return true
}

// nextWait is how long to wait before the next check, given how the last one went:
// the interval after a success; after a failure a delay that doubles from retryBase
// up to the interval.
func (k *hookKeeper) nextWait(ok bool) time.Duration {
	if k.delay == 0 || ok {
		k.delay = k.retryBase
	}
	if ok {
		return k.interval
	}
	wait := k.delay
	if k.delay *= 2; k.delay > k.interval {
		k.delay = k.interval
	}
	return wait
}
