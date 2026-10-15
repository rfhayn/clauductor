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
// when it is dirty, `git diff-index --shortstat HEAD`.
type GitStat struct {
	Head string `json:"head,omitempty"`
	// Branch is the checked-out branch ("(detached)" when HEAD is detached), from
	// branch.head (PANEL-18: the cards' stale note names it).
	Branch      string `json:"branch,omitempty"`
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
		case strings.HasPrefix(line, "# branch.head "):
			g.Branch = strings.TrimPrefix(line, "# branch.head ")
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

// TrackedChangedPaths is every tracked path `git status --porcelain=v2` lists as
// changed, a rename's old path included, and conflicted; untracked paths are not.
// Status re-hashed each file whose stat alone changed, so these are the paths whose
// content differs (PANEL-42: the diff stat reads only them). ok is false when a quoted
// path does not unquote: the caller then does not know the whole list.
func TrackedChangedPaths(out []byte) (paths []string, ok bool) {
	ok = true
	add := func(p string) {
		if strings.HasPrefix(p, `"`) {
			u, uok := unquoteC(p)
			if !uok {
				ok = false
				return
			}
			p = u
		}
		paths = append(paths, p)
	}
	for _, line := range strings.Split(string(out), "\n") {
		// The path follows a fixed count of space-separated fields, and may hold
		// spaces itself: "1 XY sub mH mI mW hH hI <path>", "2 ... X<score>
		// <path>\t<origPath>", "u XY sub m1 m2 m3 mW h1 h2 h3 <path>".
		n := 0
		switch {
		case strings.HasPrefix(line, "1 "):
			n = 9
		case strings.HasPrefix(line, "2 "):
			n = 10
		case strings.HasPrefix(line, "u "):
			n = 11
		default:
			continue
		}
		f := strings.SplitN(line, " ", n)
		if len(f) != n {
			ok = false
			continue
		}
		last := f[len(f)-1]
		if line[0] == '2' {
			np, op, found := strings.Cut(last, "\t")
			if !found {
				ok = false
				continue
			}
			add(np)
			add(op)
			continue
		}
		add(last)
	}
	return paths, ok
}

// unquoteC reads a path git C-quoted (quote.c, unquote_c_style): it quotes one holding
// a control byte, a quote, a backslash or (core.quotePath) a byte over 0x7f, escaping
// \a \b \t \n \v \f \r \" \\ and \ooo. Every other byte is the path's own, copied as it
// is: with core.quotePath=false a byte over 0x7f stays raw, and strconv.Unquote would
// turn one that is not UTF-8 into U+FFFD, a path git does not have.
func unquoteC(q string) (string, bool) {
	if len(q) < 2 || q[0] != '"' || q[len(q)-1] != '"' {
		return "", false
	}
	s := q[1 : len(q)-1]
	b := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '"':
			return "", false // a quote inside the path is escaped, so this one ended it early
		case '\\':
		default:
			b = append(b, c)
			continue
		}
		if i++; i >= len(s) {
			return "", false
		}
		switch c = s[i]; c {
		case 'a':
			b = append(b, '\a')
		case 'b':
			b = append(b, '\b')
		case 't':
			b = append(b, '\t')
		case 'n':
			b = append(b, '\n')
		case 'v':
			b = append(b, '\v')
		case 'f':
			b = append(b, '\f')
		case 'r':
			b = append(b, '\r')
		case '"', '\\':
			b = append(b, c)
		case '0', '1', '2', '3':
			if i+2 >= len(s) || s[i+1] < '0' || s[i+1] > '7' || s[i+2] < '0' || s[i+2] > '7' {
				return "", false
			}
			b = append(b, (c-'0')<<6|(s[i+1]-'0')<<3|(s[i+2]-'0'))
			i += 2
		default:
			return "", false
		}
	}
	return string(b), true
}

// ParseShortstat parses `git diff-index --shortstat` (" 3 files changed, 10
// insertions(+), 2 deletions(-)"); empty output is no change.
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
