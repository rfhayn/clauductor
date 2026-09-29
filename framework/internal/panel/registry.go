package panel

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
	"time"

	"github.com/clauductor/clauductor/internal/panel/config"
)

// The lane registry is the panel's durable memory of the lanes it started: which
// lane id owns which Claude session id, in which directory, as which type. It lives
// at ~/.clauductor/panel/<project-hash>/lanes.json.
//
// An intent is written BEFORE each action and marked done after it, so a panel that
// crashes mid-action finds the half-done action on restart. The registry is never
// trusted on its own: the reducer compares it with tmux, `claude agents` and the
// worktree list on every poll, and a record with nothing behind it is shown as an
// orphan, never hidden (see Model.terminalViews).

// LaneRecord is one registered lane.
type LaneRecord struct {
	ID        string `json:"id"`
	SessionID string `json:"sessionId"` // the id the panel passed to claude --session-id
	Path      string `json:"path"`
	Type      string `json:"type"`
	Branch    string `json:"branch,omitempty"`
	Mode      string `json:"mode"`
	Created   int64  `json:"created"` // unix ms
	// Action is the last action begun on the lane: start | restart | resume | stop.
	Action     string `json:"action"`
	ActionAt   int64  `json:"actionAt"`
	ActionDone bool   `json:"actionDone"`
	// Conversation is set once a hook reports a prompt submitted in this session:
	// only then does `claude --resume <id>` have a conversation to resume. Until
	// then a restart reuses --session-id <id>.
	Conversation bool `json:"conversation,omitempty"`
	// Corrupt, when set, says why this record failed validation on load. A corrupt
	// record is shown, can be stopped or forgotten, and is never launched.
	Corrupt string `json:"-"`

	// v2: a template lane's launch options and first prompt, and restores.
	Template    string `json:"template,omitempty"`
	Model       string `json:"model,omitempty"`
	Effort      string `json:"effort,omitempty"`
	FirstPrompt string `json:"firstPrompt,omitempty"`
	// PromptState is pending → typing → sent → delivered, or skipped. "typing" is
	// written before the first keystroke, so a panel that dies mid-typing never
	// types the prompt a second time.
	PromptState string `json:"promptState,omitempty"`
	PromptAt    int64  `json:"promptAt,omitempty"` // unix ms the prompt was typed
	Restored    int64  `json:"restored,omitempty"` // unix ms of the last restore
}

var (
	laneTypeRe  = regexp.MustCompile(`^[A-Za-z0-9._-]{0,64}$`)
	laneModes   = map[string]bool{"root": true, "existing": true, "new": true}
	laneActions = map[string]bool{"start": true, "restart": true, "resume": true, "stop": true, "restore": true}
	// v2 prompt states (lanes_v2.go).
	promptStates = map[string]bool{"": true, "pending": true, "typing": true, "sent": true, "delivered": true, "skipped": true}
)

// validate checks a record read from disk before any field of it can become argv.
func (l LaneRecord) validate() string {
	switch {
	case !uuidRe.MatchString(l.SessionID):
		return "session id is not a UUID"
	case !filepath.IsAbs(l.Path) || filepath.Clean(l.Path) != l.Path:
		return "path is not a clean absolute path"
	case !laneTypeRe.MatchString(l.Type):
		return "lane type has characters a lane type never has"
	case l.Branch != "" && !config.BranchRe.MatchString(l.Branch):
		return "branch is not a branch the panel would create"
	case !laneModes[l.Mode]:
		return "unknown mode"
	case !laneActions[l.Action]:
		return "unknown action"
	case l.Model != "" && !config.LaunchOptRe.MatchString(l.Model), l.Effort != "" && !config.LaunchOptRe.MatchString(l.Effort):
		return "model or effort is not a launch option the panel would pass"
	case !promptStates[l.PromptState]:
		return "unknown first-prompt state"
	case l.FirstPrompt != "" && config.TypableText(l.FirstPrompt, config.MaxFirstPrompt) != nil:
		return "first prompt is not one line of plain text"
	}
	return ""
}

type registryFile struct {
	Version int          `json:"version"`
	Project string       `json:"project"`
	Lanes   []LaneRecord `json:"lanes"`
}

// Registry is the lane registry of one project.
type Registry struct {
	path     string
	project  string
	mu       sync.Mutex
	lanes    map[string]LaneRecord
	problems []string // records dropped on load, for a banner
	// afterRead, if set, runs in Reload between reading the file and applying it.
	// Tests use it to force the interleaving of a reload with a concurrent write.
	afterRead func()
}

// RegistryPath is where a project's registry lives.
func RegistryPath(home, project string) string {
	return filepath.Join(config.ProjectDir(home, project), "lanes.json")
}

// OpenRegistry loads (or starts) a project's registry.
func OpenRegistry(home, project string) (*Registry, error) {
	r := &Registry{path: RegistryPath(home, project), project: project, lanes: map[string]LaneRecord{}}
	return r, r.Reload()
}

// Reload re-reads the file, so a registry edited or restored outside the panel is
// picked up. It reads under the lock: a read taken before a concurrent write and
// applied after it would resurrect a lane that was just stopped.
func (r *Registry) Reload() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, err := os.ReadFile(r.path)
	if os.IsNotExist(err) {
		r.lanes = map[string]LaneRecord{}
		return nil
	}
	if err != nil {
		return err
	}
	if r.afterRead != nil {
		r.afterRead()
	}
	var f registryFile
	if err := json.Unmarshal(b, &f); err != nil {
		return fmt.Errorf("lane registry %s is not valid JSON: %w", r.path, err)
	}
	lanes := map[string]LaneRecord{}
	r.problems = nil
	for i, l := range f.Lanes {
		if !config.ValidLaneID(l.ID) {
			r.problems = append(r.problems, fmt.Sprintf("lane registry %s: record %d has an invalid lane id and is ignored", r.path, i))
			continue
		}
		l.Corrupt = l.validate()
		lanes[l.ID] = l
	}
	r.lanes = lanes
	return nil
}

// Problems lists records that could not even be shown (an invalid lane id).
func (r *Registry) Problems() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.problems...)
}

// List returns the records, sorted by id.
func (r *Registry) List() []LaneRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]LaneRecord, 0, len(r.lanes))
	for _, l := range r.lanes {
		out = append(out, l)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Get returns one record.
func (r *Registry) Get(id string) (LaneRecord, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	l, ok := r.lanes[id]
	return l, ok
}

// Put stores a record and writes the file before returning.
func (r *Registry) Put(l LaneRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	prev, had := r.lanes[l.ID]
	r.lanes[l.ID] = l
	if err := r.writeLocked(); err != nil {
		if had {
			r.lanes[l.ID] = prev
		} else {
			delete(r.lanes, l.ID)
		}
		return err
	}
	return nil
}

// Begin records that an action is starting on a lane.
func (r *Registry) Begin(l LaneRecord, action string, now time.Time) (LaneRecord, error) {
	l.Action, l.ActionAt, l.ActionDone = action, now.UnixMilli(), false
	return l, r.Put(l)
}

// Done marks a lane's last action finished.
func (r *Registry) Done(l LaneRecord) error {
	l.ActionDone = true
	return r.Put(l)
}

// MarkConversation records that a lane's session has a conversation. It returns
// whether the session belongs to a registered lane.
func (r *Registry) MarkConversation(sessionID string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, l := range r.lanes {
		if l.SessionID != sessionID || sessionID == "" {
			continue
		}
		if l.Conversation {
			return true, nil
		}
		l.Conversation = true
		r.lanes[id] = l
		if err := r.writeLocked(); err != nil {
			l.Conversation = false
			r.lanes[id] = l
			return true, err
		}
		return true, nil
	}
	return false, nil
}

// Delete removes a record and writes the file.
func (r *Registry) Delete(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	prev, had := r.lanes[id]
	if !had {
		return nil
	}
	delete(r.lanes, id)
	if err := r.writeLocked(); err != nil {
		r.lanes[id] = prev
		return err
	}
	return nil
}

// writeLocked writes atomically: a temp file in the same private directory, then a
// rename, so a crash leaves either the old registry or the new one.
func (r *Registry) writeLocked() error {
	if err := config.EnsurePrivateDir(filepath.Dir(r.path)); err != nil {
		return err
	}
	f := registryFile{Version: 1, Project: r.project, Lanes: []LaneRecord{}}
	for _, l := range r.lanes {
		f.Lanes = append(f.Lanes, l)
	}
	sort.Slice(f.Lanes, func(i, j int) bool { return f.Lanes[i].ID < f.Lanes[j].ID })
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(r.path), ".lanes-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(append(b, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), r.path)
}

var uuidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// NewSessionID returns a random (version 4) UUID for claude --session-id.
func NewSessionID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	h := hex.EncodeToString(b)
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32], nil
}
