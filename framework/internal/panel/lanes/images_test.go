package lanes

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/leakcheck"
	"github.com/clauductor/clauductor/internal/panel/types"
)

// PANEL-15b: a dropped file is an image by its bytes, whatever it is called, and its
// name is only ever a plain file name.
func TestDroppedImageKindAndName(t *testing.T) {
	t.Parallel()
	for body, want := range map[string]string{
		"\x89PNG\r\n\x1a\nxxxx": "png", "\xFF\xD8\xFF\xE0xx": "jpg", "GIF89a..": "gif", "GIF87a..": "gif",
		"RIFF\x00\x00\x00\x00WEBPVP8 ": "webp", "RIFF\x00\x00\x00\x00WAVE": "", "<svg xmlns=": "", "%PDF-1.7": "", "": "",
	} {
		if got := ImageKind([]byte(body)); got != want {
			t.Errorf("ImageKind(%q) = %q, want %q", body, got, want)
		}
	}
	for in, want := range map[string]string{
		"Screen Shot 2026-09-30 at 10.12.01.png": "Screen-Shot-2026-09-30-at-10-12-01.png",
		"../../etc/passwd":                       "passwd.png",
		`..\..\win.jpg`:                          "win.png",
		"$(rm -rf ~).png":                        "rm--rf.png",
		"":                                       "image.png",
		"ünïcødé.gif":                            "ncd.png",
	} {
		if got := ImageName(in, "png"); got != want {
			t.Errorf("ImageName(%q) = %q, want %q", in, got, want)
		}
	}
	if got := dropText("/Users/a b/it's $x.png"); got != `/Users/a\ b/it\'s\ \$x.png` {
		t.Errorf("dropText: %s", got)
	}
}

// A lane's images go when it is forgotten (or stopped), and anyone's after a day.
func TestDroppedImagesAreCleanedUp(t *testing.T) {
	t.Parallel()
	m := testLaneManager(t)
	m.Socket = leakcheck.NoServerSocket()
	m.UploadDir = filepath.Join(t.TempDir(), "uploads")
	write := func(lane, name string, age time.Duration) string {
		p := filepath.Join(m.UploadDir, lane, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		at := time.Now().Add(-age)
		if err := os.Chtimes(p, at, at); err != nil {
			t.Fatal(err)
		}
		return p
	}
	old := write("lane-a", "1-old.png", ImageRetention+time.Hour)
	fresh := write("lane-a", "2-new.png", time.Minute)
	gone := write("lane-b", "1-old.png", ImageRetention+time.Hour)
	m.PruneImages()
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatal("an image past the retention was kept")
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Fatal("a fresh image was pruned")
	}
	if _, err := os.Stat(filepath.Dir(gone)); !os.IsNotExist(err) {
		t.Fatal("a lane's emptied image directory was kept")
	}
	rec := types.LaneRecord{ID: "lane-a", SessionID: sid, Path: "/repo", Type: "build", Mode: "root"}
	rec, err := m.Registry.Begin(rec, "start", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Registry.Done(rec); err != nil {
		t.Fatal(err)
	}
	if lerr := m.Forget(context.Background(), "lane-a"); lerr != nil {
		t.Fatal(lerr)
	}
	if _, err := os.Stat(filepath.Join(m.UploadDir, "lane-a")); !os.IsNotExist(err) {
		t.Fatal("forgetting a lane kept its images")
	}
}
