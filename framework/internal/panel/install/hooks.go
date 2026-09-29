// Package install puts the panel on the machine and keeps it single: the tagged
// hooks in ~/.claude/settings.json, the launchd login agent and its launcher app,
// the persistent token, config trust, and the machine lock and marker files that
// make it one panel per machine.
package install

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"github.com/clauductor/clauductor/internal/panel/clock"
	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/signals"
)

// HookTag is the query parameter that marks a hook entry as the panel's own. Claude
// Code's hook schema documents no free-form key for ownership, so the tag lives in the
// URL, which Claude Code passes through untouched.
const HookTag = "clauductor-panel"

// HookURL is the URL the panel's hooks post to.
func HookURL(port int) string {
	return fmt.Sprintf("http://127.0.0.1:%d/hook?src=%s", port, HookTag)
}

// SettingsPath is the user settings file the installer edits.
func SettingsPath(home string) string { return filepath.Join(home, ".claude", "settings.json") }

// InstallHooks idempotently makes the user's ~/.claude/settings.json carry exactly one
// tagged panel hook per event in HookEvents. Every other key, and every hook not
// tagged as ours, is preserved byte for byte. It returns whether the file changed.
func InstallHooks(home string, port int) (bool, error) {
	entry, _ := marshalRaw(map[string]any{
		"hooks": []map[string]any{{"type": "http", "url": HookURL(port), "timeout": 1}},
	})
	return rewriteHooks(home, func(h *orderedObject) error {
		want := map[string]bool{}
		for _, ev := range signals.HookEvents {
			want[ev] = true
			if raw, ok := h.get(ev); ok {
				var probe []json.RawMessage
				if err := json.Unmarshal(raw, &probe); err != nil {
					return fmt.Errorf("hooks.%s is not an array: %w", ev, err)
				}
			}
		}
		for _, ev := range append([]string(nil), h.keys...) {
			if !want[ev] {
				stripEvent(h, ev, nil)
			}
		}
		for _, ev := range signals.HookEvents {
			// Our current entry stays where it is (a user hook may follow it), so a
			// repeat install is a no-op; anything else tagged as ours goes.
			if !stripEvent(h, ev, entry) {
				var groups []json.RawMessage
				if raw, ok := h.get(ev); ok {
					_ = json.Unmarshal(raw, &groups)
				}
				groups = append(groups, entry)
				b, _ := marshalRaw(groups)
				h.set(ev, b)
			}
		}
		return nil
	})
}

// UninstallHooks removes only the panel's tagged hooks. It returns whether the file
// changed.
func UninstallHooks(home string) (bool, error) {
	return rewriteHooks(home, stripOurs)
}

// sameJSON compares two JSON values structurally.
func sameJSON(a, b []byte) bool {
	var va, vb any
	return json.Unmarshal(a, &va) == nil && json.Unmarshal(b, &vb) == nil && reflect.DeepEqual(va, vb)
}

// errSettingsChanged means settings.json changed on disk between the panel's read
// and its rename: another writer (Claude Code itself, a dotfile manager) got there
// first, so the edit is redone on the new content rather than overwriting it.
var errSettingsChanged = errors.New("settings.json changed while the panel was editing it")

// settingsRetries is how many times a read-modify-write is redone after a
// concurrent change.
const settingsRetries = 5

// beforeSettingsRename, if set, runs just before the compare-and-rename (tests use
// it to simulate a concurrent writer).
var beforeSettingsRename func(path string)

func rewriteHooks(home string, edit func(*orderedObject) error) (bool, error) {
	var err error
	for i := 0; i < settingsRetries; i++ {
		var changed bool
		changed, err = rewriteHooksOnce(home, edit)
		if !errors.Is(err, errSettingsChanged) {
			return changed, err
		}
		clock.System.Sleep(time.Duration(20*(i+1)) * time.Millisecond)
	}
	return false, fmt.Errorf("%w %d times in a row; not touching it", err, settingsRetries)
}

func rewriteHooksOnce(home string, edit func(*orderedObject) error) (bool, error) {
	path := SettingsPath(home)
	// Edit the file a symlinked settings.json points at (dotfile managers do this);
	// renaming over the link would silently replace it with a regular file.
	if target, err := filepath.EvalSymlinks(path); err == nil {
		path = target
	}
	orig, err := os.ReadFile(path)
	mode := os.FileMode(0o644)
	switch {
	case os.IsNotExist(err):
		orig = nil
	case err != nil:
		return false, err
	default:
		if fi, statErr := os.Stat(path); statErr == nil {
			mode = fi.Mode().Perm()
		}
	}
	root := &orderedObject{}
	if len(bytes.TrimSpace(orig)) > 0 {
		if !json.Valid(orig) {
			return false, fmt.Errorf("%s is not valid JSON (one object expected); not touching it", path)
		}
		if err := root.UnmarshalJSON(orig); err != nil {
			return false, fmt.Errorf("%s is not a JSON object: %w", path, err)
		}
	}
	hooks := &orderedObject{}
	if raw, ok := root.get("hooks"); ok {
		if err := hooks.UnmarshalJSON(raw); err != nil {
			return false, fmt.Errorf("%s: \"hooks\" is not an object: %w", path, err)
		}
	}
	if err := edit(hooks); err != nil {
		return false, fmt.Errorf("%s: %w", path, err)
	}
	if len(hooks.keys) == 0 {
		root.del("hooks")
	} else {
		hb, _ := hooks.MarshalJSON()
		root.set("hooks", hb)
	}
	compact, _ := root.MarshalJSON()
	var out bytes.Buffer
	if err := json.Indent(&out, compact, "", "  "); err != nil {
		return false, err
	}
	out.WriteByte('\n')
	if orig != nil && jsonEqual(orig, out.Bytes()) {
		return false, nil
	}
	if orig == nil && len(hooks.keys) == 0 {
		return false, nil // nothing to remove from a file that does not exist
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	// The backup is written once and never overwritten, so it keeps the file as it was
	// before the panel first touched it.
	if bak := path + ".clauductor-panel.bak"; orig != nil {
		if _, err := os.Lstat(bak); os.IsNotExist(err) {
			if err := os.WriteFile(bak, orig, mode); err != nil {
				return false, fmt.Errorf("backing up %s: %w", path, err)
			}
		}
	}
	// Re-read immediately before the rename: if the file is no longer what this edit
	// was computed from, another writer changed it, and renaming would lose its
	// change. The window left is the rename itself.
	return true, config.WriteAtomicChecked(path, out.Bytes(), mode, func() error {
		if beforeSettingsRename != nil {
			beforeSettingsRename(path)
		}
		now, err := os.ReadFile(path)
		if os.IsNotExist(err) && orig == nil {
			return nil
		}
		if err != nil || !bytes.Equal(now, orig) {
			return errSettingsChanged
		}
		return nil
	})
}

// jsonEqual compares two documents after compaction, so an install that would only
// re-indent the file is treated as no change and never rewrites it.
func jsonEqual(a, b []byte) bool {
	var ca, cb bytes.Buffer
	if json.Compact(&ca, a) != nil || json.Compact(&cb, b) != nil {
		return false
	}
	return bytes.Equal(ca.Bytes(), cb.Bytes())
}

// isOurs reports whether one hook object is tagged as the panel's.
func isOurs(raw json.RawMessage) bool {
	var h struct {
		URL string `json:"url"`
	}
	if json.Unmarshal(raw, &h) != nil || h.URL == "" {
		return false
	}
	u, err := url.Parse(h.URL)
	return err == nil && u.Query().Get("src") == HookTag
}

// stripOurs removes every tagged hook object from every event.
func stripOurs(h *orderedObject) error {
	for _, ev := range append([]string(nil), h.keys...) {
		stripEvent(h, ev, nil)
	}
	return nil
}

// stripEvent removes tagged hook objects from one event. If keep is non-nil, the
// first group equal to keep is left untouched and in place, and stripEvent reports
// whether it found one. A matcher group or event array is dropped only when OUR
// removal emptied it; anything already empty is left as the user wrote it.
func stripEvent(h *orderedObject, ev string, keep json.RawMessage) bool {
	raw, _ := h.get(ev)
	var groups []json.RawMessage
	if err := json.Unmarshal(raw, &groups); err != nil {
		return false // not an array: not ours to judge
	}
	kept := false
	changed := false
	out := make([]json.RawMessage, 0, len(groups))
	for _, g := range groups {
		if keep != nil && !kept && sameJSON(g, keep) {
			kept = true
			out = append(out, g)
			continue
		}
		grp := &orderedObject{}
		if grp.UnmarshalJSON(g) != nil {
			out = append(out, g)
			continue
		}
		hraw, ok := grp.get("hooks")
		var hs []json.RawMessage
		if !ok || json.Unmarshal(hraw, &hs) != nil {
			out = append(out, g)
			continue
		}
		var keepHooks []json.RawMessage
		for _, one := range hs {
			if isOurs(one) {
				changed = true
				continue
			}
			keepHooks = append(keepHooks, one)
		}
		if len(keepHooks) == len(hs) {
			out = append(out, g)
			continue
		}
		if len(keepHooks) == 0 {
			continue
		}
		b, _ := marshalRaw(keepHooks)
		grp.set("hooks", b)
		gb, _ := grp.MarshalJSON()
		out = append(out, gb)
	}
	if !changed {
		return kept
	}
	if len(out) == 0 {
		h.del(ev)
		return kept
	}
	b, _ := marshalRaw(out)
	h.set(ev, b)
	return kept
}

// marshalRaw encodes without HTML escaping. encoding/json's default would rewrite a
// user's `&&` inside a command hook as \u0026\u0026: equal JSON, but a needless edit.
func marshalRaw(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(b.Bytes(), "\n"), nil
}

// orderedObject is a JSON object that remembers key order and keeps each value's raw
// bytes, so rewriting one key does not reorder or re-encode the user's other keys.
type orderedObject struct {
	keys []string
	vals map[string]json.RawMessage
}

func (o *orderedObject) get(k string) (json.RawMessage, bool) {
	v, ok := o.vals[k]
	return v, ok
}

func (o *orderedObject) set(k string, v json.RawMessage) {
	if o.vals == nil {
		o.vals = map[string]json.RawMessage{}
	}
	if _, ok := o.vals[k]; !ok {
		o.keys = append(o.keys, k)
	}
	o.vals[k] = v
}

func (o *orderedObject) del(k string) {
	if _, ok := o.vals[k]; !ok {
		return
	}
	delete(o.vals, k)
	for i, key := range o.keys {
		if key == k {
			o.keys = append(o.keys[:i], o.keys[i+1:]...)
			break
		}
	}
}

func (o *orderedObject) UnmarshalJSON(b []byte) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return fmt.Errorf("expected a JSON object")
	}
	o.keys, o.vals = nil, map[string]json.RawMessage{}
	for dec.More() {
		kt, err := dec.Token()
		if err != nil {
			return err
		}
		k, _ := kt.(string)
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return err
		}
		o.set(k, v)
	}
	if _, err := dec.Token(); err != nil {
		return err
	}
	// Anything after the closing brace (a second object, say) is refused: silently
	// ignoring it would drop the user's settings on the next write.
	if _, err := dec.Token(); err != io.EOF {
		return fmt.Errorf("unexpected data after the JSON object")
	}
	return nil
}

func (o *orderedObject) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, k := range o.keys {
		if i > 0 {
			b.WriteByte(',')
		}
		kb, _ := marshalRaw(k)
		b.Write(kb)
		b.WriteByte(':')
		b.Write(o.vals[k])
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}
