package template

import (
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/clauductor/clauductor/internal/testgit"
)

// raceOnDarwin is true in a -race test binary built for darwin (racetoolchain_race_darwin_test.go).
var raceOnDarwin bool

// #90 pinned `toolchain go1.26.9` in go.mod, but only GOTOOLCHAIN=auto honours the pin: under
// GOTOOLCHAIN=local or path with an older Go, or an explicit affected version, the race step runs
// on a Go whose forked children crash or spin (golang/go#79804, #84). That fails intermittently,
// and a retry hides it. The guarantee here is that such a race run FAILS, every time, naming the
// cause (#91). It lives in this package because the race step always races it (RACE_BASELINE in
// scripts/ci/steps.sh), and in TestMain because this package's own tests fork
// (gitattributes_test.go): a child that spins before exec would hang the run before any test could
// report. It cannot make the message come FIRST: go test prints packages in order, so in a
// multi-package run it can appear only after an earlier package's own spin has hit its timeout.
func TestMain(m *testing.M) {
	testgit.Strip() // a git hook's GIT_DIR reaches no process these tests start (#302)
	if v := runtime.Version(); raceOnDarwin && raceForkBroken(v) {
		fmt.Fprintf(os.Stderr, "%s's race runtime crashes or hangs a forked child on darwin (golang/go#79804, #84): "+
			"run the race tests on the toolchain framework/go.mod pins: unset GOTOOLCHAIN (and `go env -u GOTOOLCHAIN`) "+
			"or set GOTOOLCHAIN=auto\n", v)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

// raceForkBroken reports whether runtime.Version() names a Go whose race runtime breaks a forked
// child on darwin: from the 1.26 release candidates (the regression merged before rc1) through
// 1.26.4, syscall.rawSyscall is instrumented, so the child crashes (tsan CHECK failed, exit 66) or
// spins before exec (golang/go#79804, fixed in 1.26.5). Anything it cannot read as one of those (a
// devel build, another minor) is not broken: the guard must never fail a gate on a version it
// does not know.
func raceForkBroken(version string) bool {
	f := strings.Fields(version) // "go1.26.3 X:nocoverageredesign" carries experiments after a space
	if len(f) == 0 {
		return false
	}
	if rc, ok := strings.CutPrefix(f[0], "go1.26rc"); ok {
		return rc != "" && strings.Trim(rc, "0123456789") == ""
	}
	patch, ok := strings.CutPrefix(f[0], "go1.26.")
	if !ok || len(patch) != 1 {
		return false
	}
	return patch[0] >= '0' && patch[0] <= '4'
}

func TestRaceForkBrokenReadsTheVersion(t *testing.T) {
	for _, c := range []struct {
		version string
		want    bool
	}{
		{"go1.26.0", true},
		{"go1.26.1", true},
		{"go1.26.4", true},
		{"go1.26.3 X:nocoverageredesign", true},
		{"go1.26rc1", true},
		{"go1.26rc2", true},
		{"go1.26rc", false},
		{"go1.26rcx", false},
		{"go1.26.5", false},
		{"go1.26.9", false},
		{"go1.26.10", false},
		{"go1.26.40", false},
		{"go1.26", false},
		{"go1.260.1", false},
		{"go1.25.4", false},
		{"go1.27.0", false},
		{"devel go1.27-0123abc Mon Oct 12 00:00:00 2026 +0000", false},
		{"go1.26.x", false},
		{"", false},
	} {
		if got := raceForkBroken(c.version); got != c.want {
			t.Errorf("raceForkBroken(%q) = %v, want %v", c.version, got, c.want)
		}
	}
}

// The guard is wired: the binary's build info is a second witness to -race, so deleting
// racetoolchain_race_darwin_test.go or breaking its build tag fails here instead of turning the
// guard off in silence.
func TestRaceOnDarwinIsWired(t *testing.T) {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		t.Skip("no build info")
	}
	race := false
	for _, s := range bi.Settings {
		if s.Key == "-race" && s.Value == "true" {
			race = true
		}
	}
	if want := race && runtime.GOOS == "darwin"; raceOnDarwin != want {
		t.Fatalf("raceOnDarwin = %v, but the build info says -race=%v on %s: racetoolchain_race_darwin_test.go no longer sets it",
			raceOnDarwin, race, runtime.GOOS)
	}
}
