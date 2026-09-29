package lease

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The lease protocol has more than one implementation: lock-run, the plain-shell
// lease.sh in docs/panel.md, and whatever a project writes for itself. The
// conformance suite in testdata/lease-conformance is what they must all pass; this
// runs it against the first two, so the suite is known to hold for the reference
// implementations and a third can trust it.
func TestLeaseConformance(t *testing.T) {
	if testing.Short() {
		t.Skip("runs real processes for about a minute")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash")
	}
	driver, err := filepath.Abs(filepath.Join("testdata", "lease-conformance", "conformance.sh"))
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("bash", driver, "--list").Output()
	if err != nil {
		t.Fatal(err)
	}
	cases := strings.Fields(string(out))
	if len(cases) < 15 {
		t.Fatalf("the driver lists %d cases: %q", len(cases), cases)
	}
	testBin, err := filepath.Abs(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	// lock-run: this test binary acts as `clauductor lock-run` (TestMain). The adapter
	// hands it the argv as JSON.
	goImpl := filepath.Join(dir, "lock-run-impl")
	os.WriteFile(goImpl, []byte(`#!/bin/sh
lock=$1 lane=$2; shift 2
argv='[' sep=''
for a in "$@"; do
  a=$(printf '%s' "$a" | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g')
  argv="$argv$sep\"$a\"" sep=','
done
LOCKRUN_HELPER_LOCK=$lock LOCKRUN_HELPER_LANE=$lane LOCKRUN_HELPER_ARGV="$argv]" exec '`+testBin+`' -test.run='^$'
`), 0o755)
	// lease.sh: the text in docs/panel.md, as a gate script sources it.
	shImpl := filepath.Join(dir, "lease-sh-impl")
	os.WriteFile(shImpl, []byte("#!/usr/bin/env bash\nset -euo pipefail\n. '"+docLeaseSh(t)+"'\nlease_run \"$@\"\n"), 0o755)

	for _, impl := range []struct{ name, path string }{{"lock-run", goImpl}, {"lease.sh", shImpl}} {
		t.Run(impl.name, func(t *testing.T) {
			for _, c := range cases {
				t.Run(c, func(t *testing.T) {
					t.Parallel()
					cmd := exec.Command("bash", driver, impl.path)
					cmd.Env = append(os.Environ(), "CASES="+c)
					out, err := cmd.CombinedOutput()
					if err != nil || !strings.Contains(string(out), "ok 1 - "+c) || strings.Contains(string(out), "not ok") {
						t.Fatalf("%v\n%s", err, out)
					}
					if strings.Contains(string(out), "# SKIP") {
						t.Skip(strings.TrimSpace(string(out)))
					}
				})
			}
		})
	}
}
