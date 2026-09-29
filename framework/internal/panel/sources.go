package panel

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

// Worktree is one entry of `git worktree list --porcelain`. It is the authority for
// which cwds belong to the project: an event whose cwd is under none of these is
// dropped.
type Worktree struct {
	Path   string `json:"path"`
	Branch string `json:"branch"` // "" when detached
	Head   string `json:"head"`
	Bare   bool   `json:"bare,omitempty"`
}

// ParseWorktreePorcelain parses `git worktree list --porcelain` output.
func ParseWorktreePorcelain(out []byte) ([]Worktree, error) {
	var wts []Worktree
	var cur *Worktree
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimRight(line, "\r")
		switch {
		case strings.HasPrefix(line, "worktree "):
			wts = append(wts, Worktree{Path: filepath.Clean(strings.TrimPrefix(line, "worktree "))})
			cur = &wts[len(wts)-1]
		case cur == nil:
			continue
		case strings.HasPrefix(line, "HEAD "):
			cur.Head = strings.TrimPrefix(line, "HEAD ")
		case strings.HasPrefix(line, "branch "):
			cur.Branch = strings.TrimPrefix(strings.TrimPrefix(line, "branch "), "refs/heads/")
		case line == "bare":
			cur.Bare = true
		}
	}
	if len(wts) == 0 {
		return nil, fmt.Errorf("git worktree list returned no worktrees")
	}
	return wts, nil
}

// MatchWorktree returns the index of the worktree containing cwd, or -1. Linked
// worktrees often live INSIDE the main checkout (.claude/worktrees/x), so the longest
// containing path wins; a plain prefix test would file every lane under main.
func MatchWorktree(wts []Worktree, cwd string) int {
	if cwd == "" {
		return -1
	}
	cwd = filepath.Clean(cwd)
	best, bestLen := -1, -1
	for i, w := range wts {
		if w.Bare {
			continue
		}
		if cwd == w.Path || strings.HasPrefix(cwd, w.Path+string(filepath.Separator)) {
			if len(w.Path) > bestLen {
				best, bestLen = i, len(w.Path)
			}
		}
	}
	return best
}

// Agent is one entry of `claude agents --json`.
type Agent struct {
	PID        int    `json:"pid"`
	Cwd        string `json:"cwd"`
	Kind       string `json:"kind"`
	StartedAt  int64  `json:"startedAt"`
	SessionID  string `json:"sessionId"`
	Name       string `json:"name"`
	Status     string `json:"status"` // busy | idle | waiting
	WaitingFor string `json:"waitingFor,omitempty"`
	// ID and State are set for background sessions (`claude agents` lists them with
	// a short id and a lifecycle state). Interactive sessions on 2.1.284 omit both.
	ID    string `json:"id,omitempty"`
	State string `json:"state,omitempty"`
}

// ParseAgents parses `claude agents --json` output.
func ParseAgents(out []byte) ([]Agent, error) {
	var agents []Agent
	if err := json.Unmarshal(bytes.TrimSpace(out), &agents); err != nil {
		return nil, fmt.Errorf("claude agents --json: %w", err)
	}
	return agents, nil
}

// PR is a summarised `gh pr list` row.
type PR struct {
	Number     int    `json:"number"`
	Title      string `json:"title"`
	HeadRef    string `json:"headRefName"`
	Author     string `json:"author"`
	IsDraft    bool   `json:"isDraft"`
	ChecksPass int    `json:"checksPass"`
	ChecksFail int    `json:"checksFail"`
	ChecksWait int    `json:"checksPending"`
}

type ghPR struct {
	Number      int    `json:"number"`
	Title       string `json:"title"`
	HeadRefName string `json:"headRefName"`
	Author      struct {
		Login string `json:"login"`
	} `json:"author"`
	IsDraft           bool `json:"isDraft"`
	StatusCheckRollup []struct {
		Status     string `json:"status"`     // CheckRun
		Conclusion string `json:"conclusion"` // CheckRun
		State      string `json:"state"`      // StatusContext
	} `json:"statusCheckRollup"`
}

// ParsePRs parses `gh pr list --json number,title,headRefName,author,isDraft,statusCheckRollup`.
func ParsePRs(out []byte) ([]PR, error) {
	var raw []ghPR
	if err := json.Unmarshal(bytes.TrimSpace(out), &raw); err != nil {
		return nil, fmt.Errorf("gh pr list: %w", err)
	}
	prs := make([]PR, 0, len(raw))
	for _, r := range raw {
		p := PR{Number: r.Number, Title: r.Title, HeadRef: r.HeadRefName, Author: r.Author.Login, IsDraft: r.IsDraft}
		for _, c := range r.StatusCheckRollup {
			switch classifyCheck(c.Status, c.Conclusion, c.State) {
			case "pass":
				p.ChecksPass++
			case "fail":
				p.ChecksFail++
			default:
				p.ChecksWait++
			}
		}
		prs = append(prs, p)
	}
	return prs, nil
}

func classifyCheck(status, conclusion, state string) string {
	if state != "" { // commit status context
		switch state {
		case "SUCCESS":
			return "pass"
		case "FAILURE", "ERROR":
			return "fail"
		}
		return "pending"
	}
	if status != "" && status != "COMPLETED" {
		return "pending"
	}
	switch conclusion {
	case "SUCCESS", "NEUTRAL", "SKIPPED":
		return "pass"
	case "":
		return "pending"
	}
	return "fail"
}

// CardOutput is a card command's rendered stdout: JSON when it parses as JSON,
// otherwise plain text (markdown) lines.
type CardOutput struct {
	Kind  string          `json:"kind"` // "json" | "text"
	JSON  json.RawMessage `json:"json,omitempty"`
	Lines []string        `json:"lines,omitempty"`
}

const maxCardLines = 200

// ParseCardOutput classifies a card command's stdout.
func ParseCardOutput(out []byte) CardOutput {
	trimmed := bytes.TrimSpace(out)
	if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') && json.Valid(trimmed) {
		var compact bytes.Buffer
		if json.Compact(&compact, trimmed) == nil {
			return CardOutput{Kind: "json", JSON: compact.Bytes()}
		}
	}
	lines := []string{}
	for _, l := range strings.Split(string(trimmed), "\n") {
		l = strings.TrimRight(l, " \t\r")
		if l == "" {
			continue
		}
		lines = append(lines, l)
		if len(lines) == maxCardLines {
			lines = append(lines, "… (truncated)")
			break
		}
	}
	return CardOutput{Kind: "text", Lines: lines}
}
