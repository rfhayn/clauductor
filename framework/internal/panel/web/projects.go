package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"sort"
	"sync"

	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/lanes"
	"github.com/clauductor/clauductor/internal/panel/state"
)

// One panel, many projects (PANEL-16). Each project has its own hub, lanes and
// orchestration; the server finds a request's project by its route
// (/api/p/{project}/…) or its ?project= query, and a request that names none (the
// routes before PANEL-16, kept one release) goes to the default project.

// Project is one project the server serves.
type Project struct {
	ID      string
	Name    string
	Hub     *Hub
	Lanes   *lanes.LaneManager // nil when lanes are unavailable
	Orch    *Orchestration
	Refresh func() // re-poll every source of the project now
}

// projects is the server's project table. With none configured it is the one
// project the fields before PANEL-16 describe (Hub, Lanes, Orch, Refresh), id "".
func (s *Server) projects() []*Project {
	if len(s.Projects) > 0 {
		return s.Projects
	}
	s.legacyOnce.Do(func() {
		s.legacy = &Project{Hub: s.Hub, Lanes: s.Lanes, Orch: s.Orch, Refresh: s.Refresh}
	})
	return []*Project{s.legacy}
}

// project returns the project with this id; "" is the default project.
func (s *Server) project(id string) *Project {
	ps := s.projects()
	if id == "" {
		for _, p := range ps {
			if p.ID == s.Default {
				return p
			}
		}
		return ps[0]
	}
	for _, p := range ps {
		if p.ID == id {
			return p
		}
	}
	return nil
}

// withProject resolves {project} in the route (or ?project=, or the default), and
// answers 404 for one the panel does not serve.
func (s *Server) withProject(next func(http.ResponseWriter, *http.Request, *Project)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("project")
		if id == "" {
			id = r.URL.Query().Get("project")
		}
		if id != "" && !config.ProjectIDRe.MatchString(id) {
			http.Error(w, "invalid project id", http.StatusBadRequest)
			return
		}
		p := s.project(id)
		if p == nil {
			writeLaneErr(w, laneErr(http.StatusNotFound, "no-project", "no such project: %s", id))
			return
		}
		next(w, r, p)
	}
}

// ProjectSummary is one project as the page's project menu shows it.
type ProjectSummary struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Default bool   `json:"default,omitempty"`
	OK      bool   `json:"ok"`
	Error   string `json:"error,omitempty"` // why it is not served (its config does not load, …)
	Trusted bool   `json:"trusted"`
	Working int    `json:"working"`
	Waiting int    `json:"waiting"`
	Idle    int    `json:"idle"`
	// NeedsYou counts what needs you; Blocking how many of those block.
	NeedsYou   int   `json:"needsYou"`
	Blocking   int   `json:"blocking"`
	OldestWait int64 `json:"oldestWait,omitempty"` // unix ms
	Restorable int   `json:"restorable"`
}

// Summarize derives a project's menu entry from its view.
func Summarize(id string, v state.View) ProjectSummary {
	s := ProjectSummary{ID: id, Name: v.Name, OK: true, Trusted: v.Trust.Trusted, Restorable: len(v.Restorable)}
	for _, l := range v.Lanes {
		switch l.Status {
		case "busy":
			s.Working++
		case "waiting":
			s.Waiting++
		default:
			s.Idle++
		}
	}
	for _, n := range v.NeedsYou {
		s.NeedsYou++
		if n.Severity == "block" {
			s.Blocking++
		}
		if n.At > 0 && (s.OldestWait == 0 || n.At < s.OldestWait) {
			s.OldestWait = n.At
		}
	}
	return s
}

// Summaries fans the projects' menu entries out to every open page. It publishes
// only when the list says something new, so a hub's routine push costs a page
// nothing unless a count moved.
type Summaries struct {
	mu     sync.Mutex
	order  []string
	byID   map[string]ProjectSummary
	last   []byte
	subs   map[chan []byte]struct{}
	pushes int
}

// NewSummaries starts a list with these projects, in the menu's order.
func NewSummaries(initial []ProjectSummary) *Summaries {
	s := &Summaries{byID: map[string]ProjectSummary{}, subs: map[chan []byte]struct{}{}}
	for _, p := range initial {
		s.order = append(s.order, p.ID)
		s.byID[p.ID] = p
	}
	s.last = s.encode()
	return s
}

func (s *Summaries) encode() []byte {
	list := make([]ProjectSummary, 0, len(s.order))
	for _, id := range s.order {
		list = append(list, s.byID[id])
	}
	b, _ := json.Marshal(list)
	return b
}

// Set replaces one project's entry, and publishes the list if it changed.
func (s *Summaries) Set(p ProjectSummary) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if old, ok := s.byID[p.ID]; ok {
		p.Default = old.Default
	} else {
		s.order = append(s.order, p.ID)
	}
	s.byID[p.ID] = p
	b := s.encode()
	if bytes.Equal(b, s.last) {
		return
	}
	s.last = b
	s.pushes++
	for ch := range s.subs {
		select { // keep only the latest list per slow page
		case <-ch:
		default:
		}
		ch <- b
	}
}

// JSON is the current list.
func (s *Summaries) JSON() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.last
}

// Pushes is how many times the list was published.
func (s *Summaries) Pushes() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pushes
}

func (s *Summaries) subscribe() (chan []byte, func()) {
	ch := make(chan []byte, 1)
	s.mu.Lock()
	s.subs[ch] = struct{}{}
	s.mu.Unlock()
	return ch, func() {
		s.mu.Lock()
		delete(s.subs, ch)
		s.mu.Unlock()
	}
}

// SortSummaries orders a menu: the default first, then by name.
func SortSummaries(list []ProjectSummary) {
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].Default != list[j].Default {
			return list[i].Default
		}
		return list[i].Name < list[j].Name
	})
}

// termKey names a lane's terminals and tickets: a ticket for one project's lane
// never opens another project's lane of the same name.
// The one unnamed project (a server set up the way it was before PANEL-16) keys by
// lane alone.
func termKey(project, lane string) string {
	if project == "" {
		return lane
	}
	return project + "/" + lane
}
