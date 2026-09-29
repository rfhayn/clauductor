package lease

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/clock"
)

// lock-run's writes to owner.json after it acquires (the command's pid, renewals)
// are a read, a change and a rename. They happen under the reclaim mutex and only
// while the record is still lock-run's, so a reclaim that replaced the record in
// between is never overwritten with the old holder's.
func TestUpdateOwnerIsAtomicWithReclaim(t *testing.T) {
	t.Parallel()
	lock := filepath.Join(t.TempDir(), "gate.lock")
	os.Mkdir(lock, 0o755)
	owner := filepath.Join(lock, ownerFileName)
	write := func(nonce string) { writeLeaseFile(owner, LeaseOwner{V: 1, Nonce: nonce, PID: 1, Host: "h"}) }
	read := func() LeaseOwner { o, _ := readLeaseFile(owner); return o }
	clk := clock.Func(time.Now)

	write("00000000000000aa")
	if ours, err := updateOwner(lock, "00000000000000aa", clk, func(o *LeaseOwner) { o.ChildPID = 42 }); !ours || err != nil || read().ChildPID != 42 {
		t.Fatalf("its own record: ours=%v err=%v %+v", ours, err, read())
	}

	// A reclaimer holds the mutex: no write until it lets go.
	os.Mkdir(reclaimDir(lock), 0o755)
	if _, err := updateOwner(lock, "00000000000000aa", clk, func(o *LeaseOwner) { o.Renewed = 7 }); err != errReclaimBusy || read().Renewed == 7 {
		t.Fatalf("wrote while a reclaimer held the mutex: err=%v %+v", err, read())
	}
	// ...and meanwhile replaced the record with the next holder's.
	write("00000000000000bb")
	os.Remove(reclaimDir(lock))
	if ours, err := updateOwner(lock, "00000000000000aa", clk, func(o *LeaseOwner) { o.Renewed = 7 }); ours || err != nil {
		t.Fatalf("ours=%v err=%v", ours, err)
	}
	if o := read(); o.Nonce != "00000000000000bb" || o.Renewed == 7 {
		t.Fatalf("overwrote the next holder's record: %+v", o)
	}
	if _, err := os.Stat(reclaimDir(lock)); !os.IsNotExist(err) {
		t.Fatal("left the reclaim mutex behind")
	}
}
