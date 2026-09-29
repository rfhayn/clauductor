package state

import (
	"fmt"
	"time"
)

// hookRepairShown is how long the page keeps saying the hooks were repaired.
const hookRepairShown = 10 * time.Minute

// hookHealth is the hook install's state, as the model holds it.
type hookHealth struct {
	err        string
	repairedAt time.Time
	drift      string
	otherPID   int // the hooks point at this other live panel, and are left to it
	otherPort  int
}

// ApplyHookConflict records that the hooks point at another live panel.
func (m *Model) ApplyHookConflict(pid, port int, now time.Time) {
	m.hooks.otherPID, m.hooks.otherPort = pid, port
}

// ApplyHookHealth records one hook check: an install error, or success (with what
// had drifted, when the check had to repair it).
func (m *Model) ApplyHookHealth(err error, repaired string, now time.Time) {
	if err != nil {
		m.hooks.err = err.Error()
		return
	}
	m.hooks.err = ""
	m.hooks.otherPID, m.hooks.otherPort = 0, 0
	if repaired != "" {
		m.hooks.repairedAt, m.hooks.drift = now, repaired
	}
}

// hookBanners adds the hook install's banner (it failed, so lanes are blind) or its
// warning (it drifted and was repaired).
func (m *Model) hookBanners(v *View, now time.Time) {
	if m.hooks.otherPID > 0 {
		v.banner(BannerHooks, fmt.Sprintf("The hooks in ~/.claude/settings.json point at another live panel (pid %d, port %d), "+
			"so this panel gets no hook events. It leaves them alone rather than fight over them: there is one panel per machine. "+
			"Stop one of the two; this panel takes the hooks back within 30 s of the other stopping.", m.hooks.otherPID, m.hooks.otherPort))
		return
	}
	if m.hooks.err != "" {
		v.banner(BannerHooks, "The panel's hooks could not be installed in ~/.claude/settings.json ("+m.hooks.err+
			"). The panel keeps retrying; until it succeeds, lanes update only from `claude agents` polls.")
		return
	}
	if !m.hooks.repairedAt.IsZero() && now.Sub(m.hooks.repairedAt) < hookRepairShown {
		v.Warnings = append(v.Warnings, "The panel's hooks in ~/.claude/settings.json had drifted ("+m.hooks.drift+
			") and were reinstalled at "+m.hooks.repairedAt.Local().Format("15:04:05")+
			". If another clauductor panel keeps re-pointing them, stop it: there is one panel per machine.")
	}
}
