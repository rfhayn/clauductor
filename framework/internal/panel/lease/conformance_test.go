package lease

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The lease protocol has more than one implementation: lock-run, the plain-shell
// lease.sh in docs/panel.md, and whatever a project writes for itself. The
// conformance suite in testdata/lease-conformance is what they must all pass. This
// runs it against the first two, checks that it can fail (controls that break the
// protocol, and mutants of lease.sh, must each fail a case), and runs its
// adapter-free --lock-env mode.

func conformanceDriver(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("runs real processes for a minute or two")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash")
	}
	driver, err := filepath.Abs(filepath.Join("testdata", "lease-conformance", "conformance.sh"))
	if err != nil {
		t.Fatal(err)
	}
	return driver
}

func conformanceCases(t *testing.T, driver string) []string {
	t.Helper()
	out, err := exec.Command("bash", driver, "--list").Output()
	if err != nil {
		t.Fatal(err)
	}
	cases := strings.Fields(string(out))
	if len(cases) < 30 {
		t.Fatalf("the driver lists %d cases: %q", len(cases), cases)
	}
	return cases
}

// runConformance runs the driver on some cases and returns its TAP output.
func runConformance(driver string, env []string, cases []string, impl ...string) (string, error) {
	cmd := exec.Command("bash", append([]string{driver}, impl...)...)
	cmd.Env = append(append(os.Environ(), "CASES="+strings.Join(cases, " ")), env...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// conformanceSlots caps the driver runs in flight across these tests. Each run
// mostly sleeps, so it is far above -parallel; the cap keeps a small CI machine's
// ps and fork load from bending the timing a case allows.
var conformanceSlots = make(chan struct{}, 24)

// startConformance starts one driver run now and returns a function that waits for
// its output. Runs start at once, not when -parallel frees a slot.
func startConformance(driver string, env []string, cases []string, impl ...string) func() (string, error) {
	type result struct {
		out string
		err error
	}
	done := make(chan result, 1)
	go func() {
		conformanceSlots <- struct{}{}
		defer func() { <-conformanceSlots }()
		out, err := runConformanceSteady(driver, env, cases, impl...)
		done <- result{out, err}
	}()
	var r *result
	return func() (string, error) {
		if r == nil {
			v := <-done
			r = &v
		}
		return r.out, r.err
	}
}

// runConformanceSteady is runConformance, run again (at most twice more) when the
// wall clock stepped during the run. The protocol judges a lock's age by its mtime
// against the wall clock, so a step of seconds (a CI VM's clock being corrected)
// makes a young lock old, or an old one young, and the run's verdict meaningless
// either way: a pass as much as a failure. A step is measured, not guessed: the
// wall-clock and monotonic elapsed times of the run disagree.
func runConformanceSteady(driver string, env []string, cases []string, impl ...string) (string, error) {
	var notes string
	for attempt := 1; ; attempt++ {
		t0 := time.Now()
		out, err := runConformance(driver, env, cases, impl...)
		t1 := time.Now()
		step := t1.Round(0).Sub(t0.Round(0)) - t1.Sub(t0)
		if step < 0 {
			step = -step
		}
		if step < 2*time.Second || attempt == 3 {
			return notes + out, err
		}
		notes += fmt.Sprintf("# the wall clock stepped %v during this run; running it again\n", step.Round(time.Second))
	}
}

// writeImpl writes an executable implementation script.
func writeImpl(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

// goImpl is lock-run: this test binary acts as `clauductor lock-run` (TestMain). The
// adapter hands it the argv as JSON.
func goImpl(t *testing.T, dir string) string {
	testBin, err := filepath.Abs(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	return writeImpl(t, dir, "lock-run-impl", `#!/bin/sh
lock=$1 lane=$2; shift 2
argv='[' sep=''
for a in "$@"; do
  a=$(printf '%s' "$a" | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g')
  argv="$argv$sep\"$a\"" sep=','
done
GORACE=atexit_sleep_ms=0 LOCKRUN_HELPER_LOCK=$lock LOCKRUN_HELPER_LANE=$lane LOCKRUN_HELPER_ARGV="$argv]" exec '`+testBin+`' -test.run='^$'
`)
}

// shImpl sources a lease.sh text, as a gate script does.
func shImpl(t *testing.T, dir, name, leaseSh string) string {
	t.Helper()
	src := filepath.Join(dir, name+".lease.sh")
	if err := os.WriteFile(src, []byte(leaseSh), 0o644); err != nil {
		t.Fatal(err)
	}
	return writeImpl(t, dir, name, "#!/usr/bin/env bash\nset -euo pipefail\n. '"+src+"'\nlease_run \"$@\"\n")
}

func docLeaseText(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(docLeaseSh(t))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestLeaseConformance(t *testing.T) {
	t.Parallel()
	driver := conformanceDriver(t)
	cases := conformanceCases(t, driver)
	dir := t.TempDir()
	for _, impl := range []struct{ name, path string }{
		{"lock-run", goImpl(t, dir)},
		{"lease.sh", shImpl(t, dir, "lease-sh-impl", docLeaseText(t))},
	} {
		runs := map[string]func() (string, error){}
		for _, c := range cases {
			runs[c] = startConformance(driver, nil, []string{c}, impl.path)
		}
		t.Run(impl.name, func(t *testing.T) {
			for _, c := range cases {
				t.Run(c, func(t *testing.T) {
					out, err := runs[c]()
					if err != nil || !strings.Contains(out, "ok 1 - "+c) || strings.Contains(out, "not ok") {
						t.Fatalf("%v\n%s", err, out)
					}
					if strings.Contains(out, "# SKIP") {
						t.Skip(strings.TrimSpace(out))
					}
				})
			}
		})
	}
}

// The suite must be able to fail. Two implementations that break the protocol
// outright must fail every case that depends on the rule they break: one that
// ignores the lease fails every case where a waiter must wait, and one that always
// takes it fails every case with a live holder.
func TestLeaseConformanceControlsFail(t *testing.T) {
	t.Parallel()
	driver := conformanceDriver(t)
	known := map[string]bool{}
	for _, c := range conformanceCases(t, driver) {
		known[c] = true
	}
	dir := t.TempDir()
	fast := []string{"CONFORMANCE_WAIT=2", "CONFORMANCE_TIMEOUT=12"}
	controls := []struct {
		name, body string
		mustFail   []string
	}{
		{"ignores the lease", "#!/bin/sh\nshift 2\nexec \"$@\"\n", []string{"live-holder", "proc-format", "no-ps",
			"pstart-no-ps", "other-host-live", "other-host-no-ttl", "missing-pid", "missing-host", "child-alive",
			"ownerless-young", "truncated-owner-young", "spaced-owner-live", "live-waiter-ahead", "other-host-waiter-fresh", "cancel"}},
		{"always takes the lease", `#!/bin/sh
lock=$1; shift 2
rm -rf "$lock"; mkdir -p "$lock"
CLAUDUCTOR_LOCK_HELD=$lock "$@"; rc=$?
rm -rf "$lock"; exit $rc
`, []string{"live-holder", "proc-format", "no-ps", "pstart-no-ps", "other-host-live", "other-host-no-ttl",
			"missing-pid", "missing-host", "child-alive", "spaced-owner-live"}},
	}
	runs := map[string]func() (string, error){}
	for i, ctl := range controls {
		impl := writeImpl(t, dir, "control-"+string(rune('a'+i)), ctl.body)
		for _, c := range ctl.mustFail {
			if !known[c] {
				t.Fatalf("control %q names unknown case %q", ctl.name, c)
			}
			runs[ctl.name+"/"+c] = startConformance(driver, fast, []string{c}, impl)
		}
	}
	for _, ctl := range controls {
		for _, c := range ctl.mustFail {
			t.Run(ctl.name+"/"+c, func(t *testing.T) {
				if out, _ := runs[ctl.name+"/"+c](); !strings.Contains(out, "not ok") {
					t.Errorf("a control that %s passed %s:\n%s", ctl.name, c, out)
				}
			})
		}
	}
}

// Each mutant breaks one rule of lease.sh; the suite must fail it. A mutant whose
// substitution does not apply fails the test too: a falsification that fails to
// apply looks exactly like a suite that fails to catch.
var leaseShMutants = []struct {
	name, old, new string
	cases          []string // the cases expected to catch it
}{
	{"EPERM read as dead", "  case $_e in *ermitted*) return 0 ;; esac\n", "", []string{"no-ps-foreign-pid"}},
	{"start times compared across ps and /proc",
		"  if { [ \"$_a\" = proc ] && [ \"$_b\" = proc ]; } || { [ \"$_a\" != proc ] && [ \"$_b\" != proc ]; }; then",
		"  if :; then", []string{"proc-format"}},
	{"an unverifiable pid read as dead", "  { [ -n \"$2\" ] && [ -n \"$_n\" ]; } || return 1",
		"  { [ -n \"$2\" ] && [ -n \"$_n\" ]; } || return 0", []string{"no-ps", "pstart-no-ps"}},
	{"a reused pid read as live", "    [ \"$_n\" != \"$2\" ]; return\n", "    return 1\n", []string{"pid-reuse"}},
	{"the command (child_pid) ignored", "    if [ -n \"$_cp\" ] && ! lease_proc_dead \"$_cp\" \"$_cs\"; then return 1; fi\n", "",
		[]string{"child-alive"}},
	{"a waiter judged by its own ttl", "[ -n \"$2\" ] && _t=$2", ":", []string{"other-host-waiter-stale"}},
	{"ttl 0 expires", "  [ \"${_t:-0}\" -gt 0 ] && [ \"$(date +%s)\"", "  [ \"$(date +%s)\"", []string{"other-host-no-ttl", "missing-pid"}},
	{"no grace for a starting holder", "-ge 10 ]; return; fi", "-ge 0 ]; return; fi",
		[]string{"ownerless-young", "truncated-owner-young"}},
	{"a truncated owner.json read as a record", "  if ! lease_valid \"$1/owner.json\"; then",
		"  if [ ! -f \"$1/owner.json\" ]; then", []string{"truncated-owner-old", "bad-nonce-owner-old"}},
	{"invalid waiter files removed", "      lease_valid \"$_w/$_f\" || continue\n", "      lease_valid \"$_w/$_f\" || { rm -f \"$_w/$_f\"; continue; }\n",
		[]string{"malformed-waiters"}},
	{"invalid waiter files queued", "      lease_valid \"$_w/$_f\" || continue\n", "", []string{"malformed-waiters"}},
	{"the record's shape unchecked", "  printf '%s\\n' \"$_j\" | grep -Eq \"^[[:space:]]*\\\\{($_P(,$_P)*)?\\\\}[[:space:]]*\\$\" || return 1\n", "",
		[]string{"garbage-owner-old"}},
	{"the integer fields' type unchecked", "  if printf '%s\\n' \"$_j\" | grep -Eq '\"(v|pid|child_pid|started|renewed|ttl)\"[[:space:]]*:[[:space:]]*[^-0-9[:space:]]'; then return 1; fi\n", "",
		[]string{"string-pid-owner-old"}},
	{"LIFO", "sort -t- -k1,1n -k2", "sort -t- -k1,1nr -k2", []string{"live-waiter-ahead"}},
	{"cancel ignored", "    if [ -e \"$_w/$_nonce.cancel\" ]; then echo \"lease: wait cancelled from the panel\" >&2; return 75; fi\n", "",
		[]string{"cancel"}},
}

func TestLeaseConformanceCatchesEveryMutant(t *testing.T) {
	t.Parallel()
	driver := conformanceDriver(t)
	orig := docLeaseText(t)
	dir := t.TempDir()
	fast := []string{"CONFORMANCE_WAIT=2", "CONFORMANCE_TIMEOUT=12"}
	mutantRuns := make([][]func() (string, error), len(leaseShMutants))
	for i, m := range leaseShMutants {
		if strings.Count(orig, m.old) != 1 {
			t.Errorf("mutant %q: its text occurs %d times in lease.sh, want once", m.name, strings.Count(orig, m.old))
			continue
		}
		impl := shImpl(t, dir, "mutant-"+string(rune('a'+i)), strings.Replace(orig, m.old, m.new, 1))
		runs := make([]func() (string, error), len(m.cases))
		for j, c := range m.cases {
			runs[j] = startConformance(driver, fast, []string{c}, impl)
		}
		mutantRuns[i] = runs
	}
	for i, m := range leaseShMutants {
		if mutantRuns[i] == nil {
			continue
		}
		t.Run(m.name, func(t *testing.T) {
			for _, run := range mutantRuns[i] {
				if out, _ := run(); strings.Contains(out, "not ok") {
					return
				}
			}
			t.Errorf("the suite passed a lease.sh with %s (cases %v)", m.name, m.cases)
		})
	}
}

// --lock-env: an implementation that takes its lock from the environment runs the
// suite with no adapter.
func TestLeaseConformanceLockEnvMode(t *testing.T) {
	t.Parallel()
	driver := conformanceDriver(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "lease.sh")
	os.WriteFile(src, []byte(docLeaseText(t)), 0o644)
	gate := writeImpl(t, dir, "gate-wrapper", "#!/usr/bin/env bash\nset -euo pipefail\n. '"+src+"'\n"+
		"lease_run \"${GATE_LOCK:?}\" \"${CLAUDUCTOR_LANE:-gate}\" \"$@\"\n")
	cases := []string{"live-holder", "dead-pid", "cancel", "owner-record", "symlinked-lock"}
	out, err := runConformance(driver, nil, cases, "--lock-env", "GATE_LOCK", gate)
	if err != nil || strings.Contains(out, "not ok") || strings.Count(out, "\nok ") != len(cases) {
		t.Fatalf("%v\n%s", err, out)
	}
}

// A golden record with a placeholder the driver does not fill fails its case, rather
// than handing an implementation a record like "pid":{{PID}} to be judged on.
func TestLeaseConformanceRefusesUnfilledPlaceholders(t *testing.T) {
	t.Parallel()
	driver := conformanceDriver(t)
	suite := filepath.Join(t.TempDir(), "suite")
	if out, err := exec.Command("cp", "-R", filepath.Dir(driver), suite).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	owner := filepath.Join(suite, "cases", "dead-pid", "owner.json")
	b, err := os.ReadFile(owner)
	if err != nil || !strings.Contains(string(b), `"lane":"holder"`) {
		t.Fatalf("the dead-pid record changed: %v %s", err, b)
	}
	os.WriteFile(owner, []byte(strings.Replace(string(b), `"lane":"holder"`, `"lane":"{{NOPE}}"`, 1)), 0o644)
	// dead-pid passes an implementation that always takes the lease, so only the
	// placeholder can fail it.
	impl := writeImpl(t, t.TempDir(), "takes", "#!/bin/sh\nlock=$1; shift 2\nrm -rf \"$lock\"; mkdir -p \"$lock\"\n"+
		"CLAUDUCTOR_LOCK_HELD=$lock \"$@\"; rc=$?\nrm -rf \"$lock\"; exit $rc\n")
	out, _ := runConformance(driver, nil, []string{"dead-pid"}, impl)
	if ok, _ := runConformance(filepath.Join(suite, "conformance.sh"), nil, []string{"dead-pid"}, impl); !strings.Contains(ok, "unfilled placeholder") {
		t.Fatalf("an unfilled placeholder did not fail the case:\n%s", ok)
	}
	if strings.Contains(out, "not ok") {
		t.Fatalf("the unedited dead-pid case fails an implementation that always takes the lease, so it cannot isolate the check:\n%s", out)
	}
}
