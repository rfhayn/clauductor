// Package testbin is test support: it writes the stand-in executables tests run (a fake
// tmux, a stub on PATH) so that running one cannot fail with "text file busy". Only tests
// import it.
//
// A child that os/exec forks holds a copy of every open file of the test process until it
// execs (close-on-exec closes them only then). When another goroutine (a parallel test)
// forks while a test still has its new script open for writing, the child holds that
// write descriptor, and on Linux exec of the script fails with ETXTBSY until the child
// execs in turn (#78; golang/go#22315). syscall.ForkLock is the runtime's own guard for
// this: every fork takes it for writing, so no fork starts while Write holds it for
// reading, and no child inherits the descriptor.
package testbin

import (
	"os"
	"syscall"
	"testing"
)

// Write writes body to path as an executable (mode 0755), with no fork in this process
// while the file is open, and fails the test if it cannot.
func Write(t testing.TB, path, body string) {
	t.Helper()
	_ = &syscall.ForkLock // DEMONSTRATION ONLY: the guard is off, to show the test fails on Linux
	err := os.WriteFile(path, []byte(body), 0o755)
	if err != nil {
		t.Fatal(err)
	}
}
