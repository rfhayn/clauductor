package claudecall

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/leakcheck"
)

func TestMain(m *testing.M) { os.Exit(leakcheck.Main(m)) }

// t0 is a fixed "now" the readings are taken at; lock mtimes are set around it.
var t0 = time.Date(2026, 10, 2, 10, 12, 0, 0, time.UTC)

func envOf(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

// fakeConfig is a temp home and a config dir inside it, as Claude Code lays it out.
func fakeConfig(t *testing.T) (home string, r Reader) {
	t.Helper()
	home = t.TempDir()
	r = NewReader(envOf(nil), home)
	if err := os.MkdirAll(r.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	return home, r
}

// makeLock makes a lock directory (proper-lockfile's kind) modified at mod.
func makeLock(t *testing.T, path string, mod time.Time) {
	t.Helper()
	if err := os.Mkdir(path, 0o755); err != nil && !os.IsExist(err) {
		t.Fatal(err)
	}
	touch(t, path, mod)
}

func touch(t *testing.T, path string, mod time.Time) {
	t.Helper()
	if err := os.Chtimes(path, mod, mod); err != nil {
		t.Fatal(err)
	}
}

func writeRecord(t *testing.T, r Reader, body string) {
	t.Helper()
	if err := os.WriteFile(r.OwnerPath(), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestConfigDirFollowsClaudeConfigDir(t *testing.T) {
	home := t.TempDir()
	if got, want := ConfigDir(envOf(nil), home), filepath.Join(home, ".claude"); got != want {
		t.Errorf("unset: %q, want %q", got, want)
	}
	if got, want := ConfigDir(envOf(map[string]string{"CLAUDE_CONFIG_DIR": "  "}), home), filepath.Join(home, ".claude"); got != want {
		t.Errorf("blank: %q, want %q", got, want)
	}
	if got := ConfigDir(envOf(map[string]string{"CLAUDE_CONFIG_DIR": "/x/cfg/"}), home); got != "/x/cfg" {
		t.Errorf("set: %q, want /x/cfg", got)
	}
	// The panel passes os.Getenv: the process environment is the one read.
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, "other"))
	r := NewReader(os.Getenv, home)
	if r.Dir != filepath.Join(home, "other") {
		t.Errorf("os.Getenv: %q", r.Dir)
	}
	if got, want := r.CurrentPath(), filepath.Join(home, "other", ".oauth_refresh.lock"); got != want {
		t.Errorf("current lock: %q, want %q", got, want)
	}
	if got, want := r.OwnerPath(), filepath.Join(home, "other", ".oauth_refresh.lock.owner"); got != want {
		t.Errorf("owner record: %q, want %q", got, want)
	}
}

func TestStateAtBoundaries(t *testing.T) {
	for _, c := range []struct {
		name string
		mod  time.Time
		want State
	}{
		{"3 s ago", t0.Add(-3 * time.Second), Live},
		{"just under 60 s ago", t0.Add(-StaleAfter + time.Millisecond), Live},
		{"60 s ago", t0.Add(-StaleAfter), Stale},
		{"5 min ago", t0.Add(-5 * time.Minute), Stale},
		{"5 s ahead", t0.Add(FutureSkew), Live},
		{"just over 5 s ahead", t0.Add(FutureSkew + time.Millisecond), Future},
		{"2 h ahead", t0.Add(2 * time.Hour), Future},
	} {
		if got := StateAt(c.mod, t0); got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}
}

func TestReadNoLocks(t *testing.T) {
	_, r := fakeConfig(t)
	got := r.Read(t0)
	for _, s := range got.Locks() {
		if s.State != None || s.Holder != nil || s.RecordUnreadable {
			t.Errorf("%s: %+v, want none", s.Which, s)
		}
	}
	if got.Current.Path != r.CurrentPath() || got.Legacy.Path != r.LegacyPath() {
		t.Errorf("paths: %q %q", got.Current.Path, got.Legacy.Path)
	}
}

func TestReadBothLocksAndTheOwnerRecord(t *testing.T) {
	_, r := fakeConfig(t)
	makeLock(t, r.CurrentPath(), t0.Add(-3*time.Second))
	makeLock(t, r.LegacyPath(), t0.Add(-5*time.Minute))
	// A record as 2.1.288 writes it: fractional birth times and fields the panel ignores.
	writeRecord(t, r, `{"pid":4242,"procStart":"Thu Oct  2 10:11:00 2026","pidDomain":"host","pidSpace":"ns",`+
		`"lockBirthtimeMs":1790935917123.5,"legacyLockBirthtimeMs":1790935917124}`)
	got := r.Read(t0)

	c := got.Current
	if c.State != Live || !c.ModTime.Equal(t0.Add(-3*time.Second)) {
		t.Errorf("current: %s at %v, want live at -3s", c.State, c.ModTime)
	}
	if c.Holder == nil || c.Holder.PID != 4242 || c.Holder.ProcStart != "Thu Oct  2 10:11:00 2026" {
		t.Fatalf("current holder: %+v", c.Holder)
	}
	if want := time.UnixMilli(1790935917123).Add(500 * time.Microsecond); !c.Holder.Birth.Equal(want) {
		t.Errorf("current birth: %v, want %v", c.Holder.Birth, want)
	}
	l := got.Legacy
	if l.State != Stale || l.Holder == nil || !l.Holder.Birth.Equal(time.UnixMilli(1790935917124)) || l.NotInRecord || c.NotInRecord {
		t.Errorf("legacy: %+v holder %+v", l, l.Holder)
	}
	if c.ID() == "" || l.ID() == "" || c.ID() == l.ID() {
		t.Errorf("identities: %q, %q", c.ID(), l.ID())
	}
}

func TestReadLegacyHolderNeedsItsOwnBirthTime(t *testing.T) {
	_, r := fakeConfig(t)
	makeLock(t, r.LegacyPath(), t0.Add(-2*time.Second))
	writeRecord(t, r, `{"pid":4242,"lockBirthtimeMs":1790935917123}`)
	got := r.Read(t0)
	if got.Legacy.State != Live {
		t.Errorf("legacy: %s, want live", got.Legacy.State)
	}
	if got.Legacy.Holder != nil || got.Legacy.ID() != "" {
		t.Errorf("legacy holder from a record that names no legacy lock: %+v", got.Legacy.Holder)
	}
	if !got.Legacy.NotInRecord || got.Legacy.RecordUnreadable {
		t.Errorf("legacy: NotInRecord %v, RecordUnreadable %v; want a readable record that names no legacy lock",
			got.Legacy.NotInRecord, got.Legacy.RecordUnreadable)
	}
	if got.Current.State != None || got.Current.Holder != nil {
		t.Errorf("current: %+v, want none (a record with no lock names no holder)", got.Current)
	}
}

func TestReadFutureDatedLock(t *testing.T) {
	_, r := fakeConfig(t)
	makeLock(t, r.CurrentPath(), t0.Add(2*time.Hour))
	if s := r.Read(t0).Current.State; s != Future {
		t.Errorf("state %s, want future", s)
	}
}

// An owner record the panel cannot parse leaves the lock watched by its mtime, with
// its holder unknown.
func TestReadUnparseableOwnerRecord(t *testing.T) {
	for name, body := range map[string]string{
		"not JSON":       `{"pid":`,
		"no birth time":  `{"pid":4242}`,
		"zero pid":       `{"pid":0,"lockBirthtimeMs":1790935917123}`,
		"string pid":     `{"pid":"4242","lockBirthtimeMs":1790935917123}`,
		"negative birth": `{"pid":4242,"lockBirthtimeMs":-1}`,
		"over 4 KiB":     `{"pid":4242,"lockBirthtimeMs":1790935917123,"pad":"` + strings.Repeat("x", 4096) + `"}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, r := fakeConfig(t)
			makeLock(t, r.CurrentPath(), t0.Add(-3*time.Second))
			writeRecord(t, r, body)
			c := r.Read(t0).Current
			if c.State != Live || c.Holder != nil || !c.RecordUnreadable {
				t.Errorf("%+v, want live, no holder, record unreadable", c)
			}
		})
	}
}

// Claude Code names the legacy lock after the config dir's real path.
func TestLegacyPathResolvesASymlinkedConfigDir(t *testing.T) {
	home := t.TempDir()
	real := filepath.Join(home, "real-claude")
	if err := os.Mkdir(real, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(home, ".claude")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	r := NewReader(envOf(nil), home)
	realResolved, _ := filepath.EvalSymlinks(real)
	if got, want := r.LegacyPath(), realResolved+".lock"; got != want {
		t.Errorf("legacy: %q, want %q", got, want)
	}
	// A config dir that does not exist yet: beside it, as named.
	r = Reader{Dir: filepath.Join(home, "missing") + "/"}
	if got, want := r.LegacyPath(), filepath.Join(home, "missing")+".lock"; got != want {
		t.Errorf("missing dir: %q, want %q", got, want)
	}
}

// The reader only reads: the locks and the record keep their contents and mtimes.
func TestReadWritesNothing(t *testing.T) {
	_, r := fakeConfig(t)
	makeLock(t, r.CurrentPath(), t0.Add(-3*time.Second))
	makeLock(t, r.LegacyPath(), t0.Add(-3*time.Second))
	body := `{"pid":4242,"lockBirthtimeMs":1790935917123}`
	writeRecord(t, r, body)
	recMod := t0.Add(-4 * time.Second)
	touch(t, r.OwnerPath(), recMod)
	for i := 0; i < 3; i++ {
		r.Read(t0)
	}
	for _, p := range []string{r.CurrentPath(), r.LegacyPath()} {
		fi, err := os.Stat(p)
		if err != nil || !fi.ModTime().Equal(t0.Add(-3*time.Second)) || !fi.IsDir() {
			t.Errorf("%s changed: %v %v", p, fi, err)
		}
	}
	fi, err := os.Stat(r.OwnerPath())
	b, _ := os.ReadFile(r.OwnerPath())
	if err != nil || !fi.ModTime().Equal(recMod) || string(b) != body {
		t.Errorf("owner record changed: %v %q", err, b)
	}
}
