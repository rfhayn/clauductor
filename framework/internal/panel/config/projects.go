package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// The projects one panel serves (PANEL-16): ~/.clauductor/panel/projects.json. It is
// the machine's, not a repository's, so nothing a repository says can add a project,
// pick a socket for one, or make another the default.

// ProjectsVersion is the one version of projects.json this panel reads.
const ProjectsVersion = 1

// ProjectIDRe is a project id: it names the project in routes, the page and a tmux
// socket (clauductor-<id>), so it is kept to what is safe in all three.
var ProjectIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,40}$`)

// ProjectEntry is one registered project.
type ProjectEntry struct {
	ID   string `json:"id"`
	Root string `json:"root"` // the main worktree, resolved
	// Config is the panel.json path, "" for <root>/.clauductor/panel.json.
	Config string `json:"config"`
	// TmuxSocket is the socket its lanes run on unless panel.json names one. The
	// first project keeps the historical "clauductor", so lanes already running
	// there carry on; each project added after gets clauductor-<id>.
	TmuxSocket string `json:"tmux_socket"`
	Added      int64  `json:"added"` // unix seconds
}

// ConfigPath is the entry's panel.json.
func (e ProjectEntry) ConfigPath() string {
	if e.Config != "" {
		return e.Config
	}
	return filepath.Join(e.Root, DefaultConfigRel)
}

// Socket is the tmux socket the project's lanes use: panel.json's tmux_socket when
// it names one, else the one the registry recorded.
func (e ProjectEntry) Socket(cfg *Config) string {
	if cfg != nil && cfg.TmuxSocket != "" {
		return cfg.TmuxSocket
	}
	if e.TmuxSocket != "" {
		return e.TmuxSocket
	}
	return DefaultTmuxSocket
}

// Projects is projects.json.
type Projects struct {
	Version  int            `json:"version"`
	Default  string         `json:"default"`
	Projects []ProjectEntry `json:"projects"`
}

// ProjectsPath is the registry file.
func ProjectsPath(home string) string { return filepath.Join(PanelDir(home), "projects.json") }

// LoadProjects reads the registry. A missing file is an empty registry; anything
// else that does not validate is an error, never a silent reset.
func LoadProjects(home string) (*Projects, error) {
	b, err := os.ReadFile(ProjectsPath(home))
	if errors.Is(err, os.ErrNotExist) {
		return &Projects{Version: ProjectsVersion}, nil
	}
	if err != nil {
		return nil, err
	}
	var p Projects
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("%s: %v", ProjectsPath(home), err)
	}
	if dec.More() {
		return nil, fmt.Errorf("%s: trailing data after the object", ProjectsPath(home))
	}
	if err := p.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %v", ProjectsPath(home), err)
	}
	return &p, nil
}

// Validate checks the registry as a whole: every id, root and socket unique.
func (p *Projects) Validate() error {
	if p.Version != ProjectsVersion {
		return fmt.Errorf("version %d; this panel reads version %d", p.Version, ProjectsVersion)
	}
	ids, roots, socks := map[string]bool{}, map[string]bool{}, map[string]string{}
	for _, e := range p.Projects {
		switch {
		case !ProjectIDRe.MatchString(e.ID):
			return fmt.Errorf("project id %q must match %s", e.ID, ProjectIDRe)
		case ids[e.ID]:
			return fmt.Errorf("project id %q is listed twice", e.ID)
		case !filepath.IsAbs(e.Root) || filepath.Clean(e.Root) != e.Root:
			return fmt.Errorf("project %s: root %q must be a clean absolute path", e.ID, e.Root)
		case roots[e.Root]:
			return fmt.Errorf("project %s: root %s is registered twice", e.ID, e.Root)
		case e.Config != "" && !filepath.IsAbs(e.Config):
			return fmt.Errorf("project %s: config %q must be absolute", e.ID, e.Config)
		case e.TmuxSocket != "" && !SocketNameRe.MatchString(e.TmuxSocket):
			return fmt.Errorf("project %s: tmux_socket %q must match %s", e.ID, e.TmuxSocket, SocketNameRe)
		}
		if other, ok := socks[e.TmuxSocket]; ok && e.TmuxSocket != "" {
			return fmt.Errorf("projects %s and %s share tmux socket %q; each project needs its own", other, e.ID, e.TmuxSocket)
		}
		ids[e.ID], roots[e.Root], socks[e.TmuxSocket] = true, true, e.ID
	}
	if p.Default != "" && !ids[p.Default] {
		return fmt.Errorf("default %q is not a registered project", p.Default)
	}
	if p.Default == "" && len(p.Projects) > 0 {
		return errors.New("no default project")
	}
	return nil
}

// SaveProjects writes the registry atomically, 0600 in the panel's 0700 directory.
func SaveProjects(home string, p *Projects) error {
	if err := p.Validate(); err != nil {
		return err
	}
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	if err := EnsurePrivateDir(PanelDir(home)); err != nil {
		return err
	}
	return WriteAtomic(ProjectsPath(home), append(b, '\n'), 0o600)
}

// Find returns the entry with this id, or whose root is this path, or nil.
func (p *Projects) Find(idOrRoot string) *ProjectEntry {
	for i := range p.Projects {
		if e := &p.Projects[i]; e.ID == idOrRoot || e.Root == idOrRoot {
			return e
		}
	}
	return nil
}

// DefaultEntry is the default project's entry, or nil.
func (p *Projects) DefaultEntry() *ProjectEntry { return p.Find(p.Default) }

// Slug makes a project id from a name: "StandingT" → "standingt", "My App!" →
// "my-app". An id already taken gets -2, -3, …
func (p *Projects) Slug(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case b.Len() > 0 && !dash:
			b.WriteByte('-')
			dash = true
		}
	}
	id := strings.Trim(b.String(), "-")
	if len(id) > 36 {
		id = strings.Trim(id[:36], "-")
	}
	if id == "" {
		id = "project"
	}
	base, n := id, 2
	for p.Find(id) != nil {
		id = fmt.Sprintf("%s-%d", base, n)
		n++
	}
	return id
}

// NewSocket is the socket a newly added project records: the historical
// "clauductor" for the first (lanes started before PANEL-16 run there), and
// clauductor-<id> for every one after, so no two projects share a tmux server.
func (p *Projects) NewSocket(id string) string {
	for _, e := range p.Projects {
		if e.TmuxSocket == DefaultTmuxSocket {
			return DefaultTmuxSocket + "-" + id
		}
	}
	if len(p.Projects) == 0 {
		return DefaultTmuxSocket
	}
	return DefaultTmuxSocket + "-" + id
}
