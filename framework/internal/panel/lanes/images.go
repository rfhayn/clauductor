package lanes

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/clauductor/clauductor/internal/panel/config"
)

// An image dropped (or pasted) on a lane's terminal (PANEL-15b). In a native terminal
// a dropped file pastes its path and Claude Code attaches the image; a browser never
// exposes a local path, so the page sends the bytes, the panel keeps them in its own
// state directory (never the worktree, where they would dirty git), and types the
// path into the lane the way a terminal's drop does: a bracketed paste, then a
// space, and never Enter, so you keep typing the prompt around it.

// MaxImageBytes caps one dropped image.
const MaxImageBytes = 20 << 20

// ImageRetention is how long a dropped image is kept: long enough for the lane's
// claude to read it (it reads the file when the prompt is sent), then gone. A lane's
// images also go when it is stopped or forgotten.
const ImageRetention = 24 * time.Hour

// imageKinds are the formats claude reads, by their magic bytes: the name and the
// Content-Type a browser sends say nothing a page could not forge.
var imageKinds = []struct {
	ext   string
	magic func([]byte) bool
}{
	{"png", func(b []byte) bool { return bytes.HasPrefix(b, []byte("\x89PNG\r\n\x1a\n")) }},
	{"jpg", func(b []byte) bool { return bytes.HasPrefix(b, []byte{0xFF, 0xD8, 0xFF}) }},
	{"gif", func(b []byte) bool {
		return bytes.HasPrefix(b, []byte("GIF87a")) || bytes.HasPrefix(b, []byte("GIF89a"))
	}},
	{"webp", func(b []byte) bool { return len(b) >= 12 && string(b[:4]) == "RIFF" && string(b[8:12]) == "WEBP" }},
}

// ImageKind is the extension of an image claude reads, from its magic bytes, or "".
func ImageKind(b []byte) string {
	for _, k := range imageKinds {
		if k.magic(b) {
			return k.ext
		}
	}
	return ""
}

// ImageName makes a file name of what the page says the file was called: letters,
// digits, '.', '_' and '-' only, at most 60 characters, and the extension the bytes
// are, not the one the name claims.
func ImageName(name, ext string) string {
	base := filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	base = strings.TrimSuffix(base, filepath.Ext(base))
	var b strings.Builder
	for _, r := range base {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
			b.WriteRune(r)
		case r == ' ' || r == '.':
			b.WriteByte('-')
		}
	}
	s := strings.Trim(b.String(), "-")
	if len(s) > 60 {
		s = s[:60]
	}
	if s == "" {
		s = "image"
	}
	return s + "." + ext
}

// ImageDir is where a lane's dropped images are kept.
func (m *LaneManager) ImageDir(id string) string {
	if m.UploadDir == "" || !config.ValidLaneID(id) {
		return ""
	}
	return filepath.Join(m.UploadDir, id)
}

// PasteImage keeps a dropped image and types its path into the lane. It returns the
// path it typed.
func (m *LaneManager) PasteImage(ctx context.Context, id string, data []byte, name string) (string, *LaneError) {
	if !config.ValidLaneID(id) {
		return "", laneErr(400, "invalid", "invalid lane id")
	}
	if m.UploadDir == "" {
		return "", laneErr(503, "no-uploads", "this panel keeps no dropped images")
	}
	if len(data) > MaxImageBytes {
		return "", laneErr(413, "too-large", "the image is over %d MB", MaxImageBytes>>20)
	}
	ext := ImageKind(data)
	if ext == "" {
		return "", laneErr(415, "not-an-image", "only PNG, JPEG, GIF and WebP images can be dropped on a lane")
	}
	if !m.Exists(ctx, id) {
		return "", laneErr(404, "not-found", "no lane %q", id)
	}
	m.PruneImages()
	dir := m.ImageDir(id)
	if err := config.EnsurePrivateDir(dir); err != nil {
		return "", laneErr(500, "fault", "cannot keep the image: %v", err)
	}
	path := filepath.Join(dir, strconv.FormatInt(m.now().UnixMilli(), 10)+"-"+ImageName(name, ext))
	if err := config.WriteAtomic(path, data, 0o600); err != nil {
		return "", laneErr(500, "fault", "cannot keep the image: %v", err)
	}
	// A paste buffer of the panel's own, pasted bracketed (-p) when the program asked
	// for it, as a terminal pastes a drop; then a space. Never Enter.
	buf := "clauductor-drop-" + id
	if _, err := m.tmux(ctx, "set-buffer", "-b", buf, "--", dropText(path)); err != nil {
		return "", laneErr(500, "tmux", "%v", err)
	}
	if _, err := m.tmux(ctx, "paste-buffer", "-p", "-d", "-b", buf, "-t", "="+id+":"); err != nil {
		return "", laneErr(500, "tmux", "%v", err)
	}
	if _, err := m.tmux(ctx, "send-keys", "-t", "="+id+":", "-l", "--", " "); err != nil {
		return "", laneErr(500, "tmux", "%v", err)
	}
	return path, nil
}

// dropText is a path as a terminal types a dropped file: a backslash before each
// character a shell would split or expand.
func dropText(path string) string {
	var b strings.Builder
	for _, r := range path {
		if strings.ContainsRune(" \t'\"\\$`!&*?()[]{}<>|;#~", r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// dropImages removes a lane's dropped images (it was stopped or forgotten).
func (m *LaneManager) dropImages(id string) {
	if dir := m.ImageDir(id); dir != "" {
		_ = os.RemoveAll(dir)
	}
}

// PruneImages removes dropped images older than ImageRetention, and any lane's
// directory left empty.
func (m *LaneManager) PruneImages() {
	if m.UploadDir == "" {
		return
	}
	cut := m.now().Add(-ImageRetention)
	lanes, _ := os.ReadDir(m.UploadDir)
	for _, l := range lanes {
		if !l.IsDir() {
			continue
		}
		dir := filepath.Join(m.UploadDir, l.Name())
		files, _ := os.ReadDir(dir)
		left := 0
		for _, f := range files {
			if info, err := f.Info(); err == nil && info.ModTime().Before(cut) {
				_ = os.Remove(filepath.Join(dir, f.Name()))
				continue
			}
			left++
		}
		if left == 0 {
			_ = os.Remove(dir)
		}
	}
}
