package signals

import (
	"fmt"
	"strconv"
	"strings"
)

// PANEL-11: two cheap reads for the lane dashboard, each one process spawn.

// Proc is one process as `ps -o pid=,pcpu=,rss=` reports it.
type Proc struct {
	CPU   float64 // percent of one core, averaged over the process's life (BSD and procps both)
	RSSKB int64
}

// ParsePS parses `ps -o pid=,pcpu=,rss= -p <pids>` output. A pid ps does not list
// has exited; a line that does not parse is skipped.
func ParsePS(out []byte) map[int]Proc {
	procs := map[int]Proc{}
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) != 3 {
			continue
		}
		pid, err1 := strconv.Atoi(f[0])
		cpu, err2 := strconv.ParseFloat(strings.Replace(f[1], ",", ".", 1), 64)
		rss, err3 := strconv.ParseInt(f[2], 10, 64)
		if err1 != nil || err2 != nil || err3 != nil {
			continue
		}
		procs[pid] = Proc{CPU: cpu, RSSKB: rss}
	}
	return procs
}

// GitStat is a worktree's state from `git status --porcelain=v2 --branch`, and,
// when it is dirty, `git diff HEAD --shortstat`.
type GitStat struct {
	Head        string `json:"head,omitempty"`
	Upstream    string `json:"upstream,omitempty"`
	Ahead       int    `json:"ahead"`
	Behind      int    `json:"behind"`
	HasUpstream bool   `json:"hasUpstream"`
	Changed     int    `json:"changed"`   // tracked files with a change, staged or not
	Untracked   int    `json:"untracked"` // untracked files
	Conflicts   int    `json:"conflicts,omitempty"`
	Files       int    `json:"files,omitempty"` // --shortstat against HEAD
	Insertions  int    `json:"insertions,omitempty"`
	Deletions   int    `json:"deletions,omitempty"`
	// LastCommitAt is HEAD's committer time, unix ms (read again only when HEAD moves).
	LastCommitAt int64 `json:"lastCommitAt,omitempty"`
}

// Dirty is how many paths differ from HEAD, untracked ones included.
func (g GitStat) Dirty() int { return g.Changed + g.Untracked + g.Conflicts }

// ParseGitStatusV2 parses `git status --porcelain=v2 --branch` output.
func ParseGitStatusV2(out []byte) (GitStat, error) {
	var g GitStat
	sawBranch := false
	for _, line := range strings.Split(string(out), "\n") {
		switch {
		case strings.HasPrefix(line, "# branch.oid "):
			g.Head, sawBranch = strings.TrimPrefix(line, "# branch.oid "), true
		case strings.HasPrefix(line, "# branch.upstream "):
			g.Upstream, g.HasUpstream = strings.TrimPrefix(line, "# branch.upstream "), true
		case strings.HasPrefix(line, "# branch.ab "):
			if _, err := fmt.Sscanf(strings.TrimPrefix(line, "# branch.ab "), "+%d -%d", &g.Ahead, &g.Behind); err != nil {
				return g, fmt.Errorf("git status: bad branch.ab line %q", line)
			}
		case strings.HasPrefix(line, "1 "), strings.HasPrefix(line, "2 "):
			g.Changed++
		case strings.HasPrefix(line, "u "):
			g.Conflicts++
		case strings.HasPrefix(line, "? "):
			g.Untracked++
		}
	}
	if !sawBranch {
		return g, fmt.Errorf("git status: no branch header in the output")
	}
	return g, nil
}

// ParseShortstat parses `git diff --shortstat` (" 3 files changed, 10 insertions(+),
// 2 deletions(-)"); empty output is no change.
func ParseShortstat(out []byte) (files, ins, del int) {
	for _, part := range strings.Split(strings.TrimSpace(string(out)), ",") {
		f := strings.Fields(part)
		if len(f) < 2 {
			continue
		}
		n, err := strconv.Atoi(f[0])
		if err != nil {
			continue
		}
		switch {
		case strings.HasPrefix(f[1], "file"):
			files = n
		case strings.HasPrefix(f[1], "insertion"):
			ins = n
		case strings.HasPrefix(f[1], "deletion"):
			del = n
		}
	}
	return
}
