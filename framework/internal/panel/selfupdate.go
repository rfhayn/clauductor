package panel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"time"

	"github.com/rfhayn/stridal/internal/panel/clock"
	"github.com/rfhayn/stridal/internal/panel/config"
	"github.com/rfhayn/stridal/internal/panel/install"
	"github.com/rfhayn/stridal/internal/panel/signals"
)

// The panel's rebuild and restart of itself (OPS-118 D4), on catch-up's source after a
// fast-forward of the repository that is its own source: install.Rebuild decides,
// builds, checks and swaps; this file sends the notices, judges a changed config with
// the new binary, and restarts. Every notice of it goes whatever the project's
// alerts.notify says, titled "clauductor panel" (D6): it is about the panel, and one of
// them is the only place the rollback is written.

// ExitRestart is the exit status of a panel that put a rebuilt binary in place: not 0,
// so the login agent's KeepAlive starts the new one.
const ExitRestart = 75

// restartQuiesce bounds the restart's wait for the panel's own claude calls.
const restartQuiesce = 10 * time.Minute

// RestartError is Run's return after the restart: the run shut down as on SIGTERM, and
// the process is to exit with Code, which `clauductor panel` does itself, since main
// turns every returned error into 1.
type RestartError struct {
	Code     int
	From, To string
}

func (e *RestartError) Error() string {
	return fmt.Sprintf("the panel replaced its binary (%s, now %s) and exits %d for the login agent to start the new one", short(e.From), short(e.To), e.Code)
}

// rebuildEnv is what the rebuild reads of the panel's process: its build stamp and
// path, the go command, the clauductor its PATH names, and the build's bound. Run reads
// production's when a rebuild first needs it; a test stands its own in (Options.rebuild).
type rebuildEnv struct {
	self    install.BuildStamp
	exe     string
	goCmd   string
	pathBin string
	timeout time.Duration
}

func processRebuildEnv() rebuildEnv {
	exe, _ := os.Executable()
	p, err := exec.LookPath("clauductor")
	if err != nil {
		p = ""
	}
	return rebuildEnv{self: install.RunningStamp(), exe: exe, goCmd: "go", pathBin: p, timeout: install.RebuildTimeout}
}

// selfUpdater is one panel run's rebuild and restart.
type selfUpdater struct {
	home    string
	launchd bool
	env     func() rebuildEnv
	out     io.Writer
	run     signals.Runner
	clk     clock.Clock
	cu      *catchUp // its notices go through catch-up's sender, on its source
	// canServe is what canStart checks of a changed config beyond its parse, on the new
	// binary's reading of it (the liveSet's canServe); nil when catch-up's source runs
	// alone (a test's), which serves nothing to clash with.
	canServe func(root string, chk config.Check) error
	// quiesce waits for the panel's claude calls and keeps any more from starting
	// (the call gate's Quiesce).
	quiesce func(context.Context, time.Duration) bool
	// restart ends the run with the exit status recorded (Run's cancel).
	restart func(*RestartError)
}

// rebuildKey keys a rebuild's notices: one per fast-forward's commit.
func rebuildKey(root, to, what string) string {
	return "catch-up\x00" + root + "\x00rebuild\x00" + to + what
}

// selfUpdate is catch-up's Options.selfUpdate: nil when no rebuild follows (another
// project, framework/ unchanged, or a panel that does not qualify, which says why
// here); else the build's failure, or the new binary's reading of the changed config
// at cfgPath and the swap.
func (s *selfUpdater) selfUpdate(ctx context.Context, t catchUpTarget, res catchUpResult, cfgPath string) *rebuilt {
	env := s.env()
	rb := &install.Rebuild{Home: s.home, Root: t.root, From: res.from, To: res.to, Launchd: s.launchd,
		Self: env.self, Exe: env.exe, Go: env.goCmd, PathBinary: env.pathBin, Timeout: env.timeout, Run: s.run}
	// Decide's git reads run to their answer on a stop too, each under its own bound:
	// the next start finds nothing to arrive, so a rebuild the stop drops is said now,
	// and only a known answer tells one from a merge that left framework/ alone.
	plan := rb.Decide(context.WithoutCancel(ctx))
	if !plan.Ours || !plan.Changed {
		return nil
	}
	if !plan.Rebuilds() {
		fmt.Fprintf(s.out, "catch-up %s: framework/ changed (%s..%s), and the panel does not rebuild itself: %s\n",
			t.root, short(plan.Base), short(res.to), plan.Skip)
		// The reason is clipped, so a long git error cannot push how to update out of
		// the body.
		s.cu.send(ctx, catchUpNotice{root: t.root, key: rebuildKey(t.root, res.to, ""), body: catchUpBody(fmt.Sprintf(
			"The panel did not rebuild itself after catch-up moved %s to %s (%s), which changed framework/: %s. It keeps running as it was. %s",
			t.root, short(res.to), prList(res.prs), oneLine(plan.Skip, 200), plan.How))}, true)
		return nil
	}
	// A stop at any step of the build ends the rebuild as a failure does, and says so
	// (errRebuildStopped): nothing retries it at the next start.
	stopped := func() *rebuilt {
		fmt.Fprintf(s.out, "catch-up %s: stopped before the rebuild at %s was done; the next start runs the binary it ran\n", t.root, short(res.to))
		return &rebuilt{err: errRebuildStopped}
	}
	if ctx.Err() != nil {
		return stopped()
	}
	fmt.Fprintf(s.out, "catch-up %s: framework/ changed (%s..%s): building the panel at %s\n", t.root, short(plan.Base), short(res.to), short(res.to))
	if err := rb.Build(ctx); err != nil {
		if ctx.Err() != nil {
			return stopped()
		}
		return &rebuilt{err: fmt.Errorf("its build failed: %w", err)}
	}
	if err := rb.Preflight(ctx); err != nil {
		if ctx.Err() != nil {
			return stopped()
		}
		return &rebuilt{err: fmt.Errorf("the new binary's preflight failed: %w", err)}
	}
	u := &rebuilt{discard: rb.Discard}
	if cfgPath != "" {
		// D1 (e) in the binary that will serve the config: it loads there, and canStart's
		// other check, its socket, holds among the projects served now.
		chk, err := configCheck(ctx, rb.NewBinary(), cfgPath)
		if err == nil && s.canServe != nil {
			err = s.canServe(t.root, chk)
		}
		if err != nil {
			rb.Discard()
			if ctx.Err() != nil {
				return stopped()
			}
			return &rebuilt{err: fmt.Errorf("the new binary's preflight failed: %w", err)}
		}
		u.check = &chk
	}
	u.swap = func(ctx context.Context) { s.swap(ctx, rb, t, res, plan, u.notInPlace) }
	return u
}

// errRebuildStopped is a rebuild the run's end stopped before its swap. The next start
// runs the binary that ran, and does not build again: its fast-forward is done, so
// nothing arrives. The next fast-forward's rebuild carries this change, as after a
// failed build: it builds from the running binary's commit (Plan.Base).
var errRebuildStopped = errors.New("the panel was stopped before its rebuild was done")

// stoppedRebuildBody is the notice of a rebuild a stop ended, before its build was done
// or before its swap: what the next start runs, and how the change still comes.
func stoppedRebuildBody(lead string) string {
	return lead + " Its next start runs the binary it ran; catch-up's next fast-forward rebuilds it, this change included. " + install.UpdateByHand
}

// swap puts the new binary in place, then restarts once no claude call of the panel's
// runs (or after restartQuiesce): the marker for the new binary's first start, the
// notice with the rollback, a browser open recorded, and the run's end with
// ExitRestart. A stop during the wait still sends the rollback, and ends there: the
// binary has replaced itself either way. Nothing here waits for the shutdown it starts:
// catch-up's source returns, and Run's shutdown waits for it as for every source.
// notInPlace (nil for none) hands a changed config to the running binary when the swap
// fails, as a failed build does (D1): the trust recorded on arrival for the new binary's
// reading withdrawn, and the project served on the file as this binary judges it. It
// returns what became of the config, for the notice.
func (s *selfUpdater) swap(ctx context.Context, rb *install.Rebuild, t catchUpTarget, res catchUpResult, plan install.Plan, notInPlace func(context.Context) string) {
	pathBin, pathErr, err := rb.Swap(ctx)
	if err != nil {
		fmt.Fprintf(s.out, "catch-up %s: the rebuilt panel was not put in place: %v\n", t.root, err)
		// What became of the config before the error, whose two paths can be long: the
		// body is cut at catchUpBodyMax, the log keeps the error whole, and the config's
		// account is what corrects a trust notice already sent.
		body := fmt.Sprintf("The panel built %s after catch-up moved %s there (%s), but could not put it in place, and keeps running as it was.",
			short(res.to), t.root, prList(res.prs))
		if notInPlace != nil {
			body += notInPlace(ctx)
		}
		body += fmt.Sprintf(" Putting it in place failed: %v.", err)
		s.cu.send(ctx, catchUpNotice{root: t.root, key: rebuildKey(t.root, res.to, ""), body: catchUpBody(body)}, true)
		return
	}
	bin := install.InstalledBinary(s.home)
	fmt.Fprintf(s.out, "catch-up %s: %s is now the build of %s (the one of %s is %s.prev)\n", t.root, bin, short(res.to), short(plan.Base), bin)
	if pathBin != "" {
		fmt.Fprintf(s.out, "catch-up %s: %s replaced too (its old one is %s.prev)\n", t.root, pathBin, pathBin)
	}
	if pathErr != nil {
		fmt.Fprintf(s.out, "catch-up %s: %s was not replaced: %v\n", t.root, rb.PathBinary, pathErr)
	}
	// Before the wait: a stop meanwhile still starts the new binary, which says so.
	if err := install.WriteRestartMarker(s.home, install.RestartMarker{Root: t.root, From: plan.Base, To: res.to}); err != nil {
		fmt.Fprintf(s.out, "catch-up %s: the restart marker: %v\n", t.root, err)
	}
	quiet := s.quiesce(ctx, restartQuiesce)
	stopped := ctx.Err() != nil
	if !quiet && !stopped {
		fmt.Fprintf(s.out, "catch-up %s: a claude call of the panel's still runs after %s; restarting anyway\n", t.root, restartQuiesce)
	}
	// The rollback first, then the install step: the body is cut at catchUpBodyMax, and
	// these are the two things nothing else says. A stop during the wait (a logout,
	// `launchctl bootout`, `clauductor panel install`) sends it too: the binary replaced
	// itself all the same, and "now running" comes only from a new binary that starts.
	home := s.home
	lead, back := fmt.Sprintf("The panel rebuilt itself and restarts now, from %s to %s.", short(plan.Base), short(res.to)), "in 2 minutes"
	if stopped {
		lead, back = fmt.Sprintf("The panel rebuilt itself, from %s to %s, and was stopped before its restart, with the new build in place.",
			short(plan.Base), short(res.to)), "within 2 minutes of its next start"
	}
	body := fmt.Sprintf("%s If it is not back %s: `mv %s %s`, then `launchctl kickstart -k gui/$(id -u)/%s`.",
		lead, back, install.Tilde(home, bin+".prev"), install.Tilde(home, bin), install.LaunchdLabel)
	if len(plan.Install) > 0 {
		names := make([]string, len(plan.Install))
		for i, f := range plan.Install {
			names[i] = strings.TrimPrefix(f, "framework/internal/panel/")
		}
		body += fmt.Sprintf(" The update changed what `clauductor panel install` writes (%s): run `clauductor panel install` once.", strings.Join(names, ", "))
	}
	body += fmt.Sprintf(" Catch-up moved %s to %s (%s).", t.root, short(res.to), prList(res.prs))
	if pathErr != nil {
		body += fmt.Sprintf(" %s was not replaced: %v.", rb.PathBinary, pathErr)
	}
	s.cu.send(ctx, catchUpNotice{root: t.root, key: rebuildKey(t.root, res.to, "\x00restart"), body: catchUpBody(body)}, true)
	if stopped {
		fmt.Fprintf(s.out, "catch-up %s: stopped before the restart, with the build of %s in place\n", t.root, short(res.to))
		return
	}
	install.RecordBrowserOpen(home, s.clk.Now())
	fmt.Fprintf(s.out, "catch-up %s: restarting into the build of %s (exit %d)\n", t.root, short(res.to), ExitRestart)
	s.restart(&RestartError{Code: ExitRestart, From: plan.Base, To: res.to})
}

// announce is the first start after a restart: the marker the restart left is taken,
// and one notice names the commit this binary runs. A second start finds no marker.
func (s *selfUpdater) announce(ctx context.Context) {
	m, err := install.TakeRestartMarker(s.home)
	if err != nil {
		fmt.Fprintf(s.out, "catch-up: the restart marker: %v\n", err)
		return
	}
	if m == nil {
		return
	}
	own := s.env().self.Revision
	body := fmt.Sprintf("The panel is now running %s, rebuilt after catch-up moved %s to it.", short(own), m.Root)
	if own != m.To {
		body = fmt.Sprintf("The panel restarted after rebuilding itself at %s for %s, but is now running %s (the rollback, or another build).",
			short(m.To), m.Root, orNone(short(own)))
	}
	fmt.Fprintf(s.out, "catch-up %s: %s\n", m.Root, body)
	s.cu.send(ctx, catchUpNotice{root: m.Root, key: "catch-up\x00" + m.Root + "\x00running\x00" + m.To, body: catchUpBody(body)}, true)
}

// newSelfUpdater is Run's rebuild for the catch-up source c; env nil reads the
// process's when a rebuild first needs it.
func newSelfUpdater(o Options, clk clock.Clock, c *catchUp, quiesce func(context.Context, time.Duration) bool, restart func(*RestartError)) *selfUpdater {
	s := &selfUpdater{home: o.Home, launchd: o.Launchd, out: o.Out, run: o.Runner, clk: clk, cu: c, quiesce: quiesce, restart: restart}
	if o.rebuild != nil {
		env := *o.rebuild
		s.env = func() rebuildEnv { return env }
	} else {
		var once atomic.Pointer[rebuildEnv]
		s.env = func() rebuildEnv {
			if e := once.Load(); e != nil {
				return *e
			}
			e := processRebuildEnv()
			once.Store(&e)
			return e
		}
	}
	return s
}
