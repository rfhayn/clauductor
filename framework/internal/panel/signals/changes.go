package signals

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// PANEL-19: the project's changes, as Clauductor's operating model writes them
// (changes/<id>/proposal.md, OPS-7), for three Needs-you signals: a proposal waiting
// for its owner's approval, and a change over its budget. Only files are read: no
// command runs, and a repository without them has no change.

// Change is one change directory's proposal.
type Change struct {
	ID string
	// Dir is where the proposal was read, and Worktree the worktree it is in.
	Dir      string
	Worktree string
	// Approved: the proposal has an `**Approved:** <date> by <owner>` line.
	Approved bool
	// Written is when proposal.md (or design.md beside it) was last written: an
	// unapproved proposal has waited for approval since then.
	Written time.Time
	// BudgetUSD is `**Budget:** $N`, when the proposal has one.
	BudgetUSD *float64
	// Tasks: tasks.md beside the proposal has them (PANEL-20); how many are ticked.
	Tasks     bool
	TasksOpen int
	TasksDone int
}

var (
	taskOpenRe = regexp.MustCompile(`(?m)^\s*[-*]\s+\[ \]\s`)
	taskDoneRe = regexp.MustCompile(`(?m)^\s*[-*]\s+\[[xX]\]\s`)
)

// ReadTasks reads a change's tasks.md (its first 256 KB).
func ReadTasks(path string) ([]byte, error) { return readHead(path, maxProposal) }

// CountTasks counts a tasks.md's Markdown checkboxes: open and ticked.
func CountTasks(b []byte) (open, done int) {
	return len(taskOpenRe.FindAll(b, -1)), len(taskDoneRe.FindAll(b, -1))
}

// Receipt is the gate's receipt, `<git dir>/ci-receipt` (OPS-7's run-local.sh):
// `<sha> TAB full TAB clean|dirty TAB all`, written only after a complete run.
type Receipt struct {
	SHA   string `json:"sha"`
	Kind  string `json:"kind"`
	Clean bool   `json:"clean"`
}

// ParseReceipt reads a receipt's first line; ok is false for anything else.
func ParseReceipt(b []byte) (Receipt, bool) {
	line, _, _ := strings.Cut(string(b), "\n")
	f := strings.Split(strings.TrimSpace(line), "\t")
	if len(f) < 3 || len(f[0]) < 7 {
		return Receipt{}, false
	}
	for _, c := range f[0] {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return Receipt{}, false
		}
	}
	return Receipt{SHA: f[0], Kind: f[1], Clean: f[2] == "clean"}, true
}

// ReadReceipt reads a worktree's receipt from its git dir; ok false when there is none.
func ReadReceipt(gitDir string) (Receipt, bool) {
	b, err := readHead(filepath.Join(gitDir, "ci-receipt"), 4096)
	if err != nil {
		return Receipt{}, false
	}
	return ParseReceipt(b)
}

// ReviewThreadsArgv asks GitHub for a pull request's review threads (gh fills in
// {owner} and {repo} from the checkout's remote).
func ReviewThreadsArgv(number int) []string {
	return []string{"gh", "api", "graphql", "-F", "owner={owner}", "-F", "name={repo}", "-F", "n=" + strconv.Itoa(number), "-f",
		"query=query($owner:String!,$name:String!,$n:Int!){repository(owner:$owner,name:$name){pullRequest(number:$n){reviewThreads(first:100){totalCount nodes{isResolved}}}}}"}
}

// ParseReviewThreads counts a pull request's unresolved review threads (of the first 100).
func ParseReviewThreads(out []byte) (unresolved, total int, err error) {
	var r struct {
		Data struct {
			Repository struct {
				PullRequest struct {
					ReviewThreads struct {
						TotalCount int `json:"totalCount"`
						Nodes      []struct {
							IsResolved bool `json:"isResolved"`
						} `json:"nodes"`
					} `json:"reviewThreads"`
				} `json:"pullRequest"`
			} `json:"repository"`
		} `json:"data"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(out), &r); err != nil {
		return 0, 0, fmt.Errorf("gh api graphql: %w", err)
	}
	t := r.Data.Repository.PullRequest.ReviewThreads
	for _, n := range t.Nodes {
		if !n.IsResolved {
			unresolved++
		}
	}
	return unresolved, t.TotalCount, nil
}

// maxProposal caps what is read of one proposal: its header lines are near the top.
const maxProposal = 256 << 10

var (
	approvedRe = regexp.MustCompile(`(?mi)^\s*[-*]?\s*\*\*Approved:\*\*\s*\S`)
	budgetRe   = regexp.MustCompile(`(?mi)^\s*[-*]?\s*\*\*Budget:\*\*\s*(?:US)?\$\s*([0-9][0-9,]*(?:\.[0-9]+)?)`)
	confDirRe  = regexp.MustCompile(`^\s*CHANGES_DIR\s*=\s*["']?([^"'#\s]+)["']?`)
	changeIDRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,99}$`)
)

// ChangesDirs are the directories, relative to a worktree, that hold changes:
// CHANGES_DIR from the project's .claude/project.conf when it names one inside the
// repository, else changes; and openspec/changes, where the OpenSpec layout keeps them.
func ChangesDirs(root string) []string {
	dir := "changes"
	if b, err := os.ReadFile(filepath.Join(root, ".claude", "project.conf")); err == nil {
		sc := bufio.NewScanner(bytes.NewReader(b))
		for sc.Scan() {
			if m := confDirRe.FindStringSubmatch(sc.Text()); m != nil {
				c := filepath.Clean(m[1])
				if !filepath.IsAbs(c) && c != "." && c != ".." && !strings.HasPrefix(c, "../") {
					dir = c
				}
			}
		}
	}
	out := []string{dir}
	if o := filepath.Join("openspec", "changes"); o != dir {
		out = append(out, o)
	}
	return out
}

// ParseProposal reads what the signals need from a proposal's text.
func ParseProposal(b []byte) (approved bool, budget *float64) {
	approved = approvedRe.Match(b)
	if m := budgetRe.FindSubmatch(b); m != nil {
		if v, err := strconv.ParseFloat(strings.ReplaceAll(string(m[1]), ",", ""), 64); err == nil && v >= 0 {
			budget = &v
		}
	}
	return approved, budget
}

// ReadChanges reads every change's proposal in each worktree's change directories.
// A change found in more than one worktree is the copy written last (a proposal is
// drafted on a lane's branch before it lands). The archive is not read.
func ReadChanges(worktrees []string, dirs []string) []Change {
	byID := map[string]Change{}
	for _, wt := range worktrees {
		for _, d := range dirs {
			base := filepath.Join(wt, d)
			entries, err := os.ReadDir(base)
			if err != nil {
				continue
			}
			for _, e := range entries {
				id := e.Name()
				if !e.IsDir() || id == "archive" || !changeIDRe.MatchString(id) {
					continue
				}
				p := filepath.Join(base, id, "proposal.md")
				fi, err := os.Stat(p)
				if err != nil || !fi.Mode().IsRegular() {
					continue
				}
				b, err := readHead(p, maxProposal)
				if err != nil {
					continue
				}
				c := Change{ID: id, Dir: filepath.Join(base, id), Worktree: wt, Written: fi.ModTime()}
				if di, err := os.Stat(filepath.Join(base, id, "design.md")); err == nil && di.ModTime().After(c.Written) {
					c.Written = di.ModTime()
				}
				c.Approved, c.BudgetUSD = ParseProposal(b)
				if tb, err := readHead(filepath.Join(base, id, "tasks.md"), maxProposal); err == nil {
					c.Tasks = true
					c.TasksOpen, c.TasksDone = CountTasks(tb)
				}
				if old, ok := byID[id]; !ok || c.Written.After(old.Written) {
					byID[id] = c
				}
			}
		}
	}
	out := make([]Change, 0, len(byID))
	for _, c := range byID {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func readHead(path string, n int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, int64(n)))
}

// BranchOfChange reports whether a branch is a change's: its last path segment is the
// change's id (change/add-x, feature/add-x), or the whole branch is.
func BranchOfChange(branch, id string) bool {
	if branch == "" || id == "" {
		return false
	}
	if branch == id {
		return true
	}
	i := strings.LastIndexByte(branch, '/')
	return i >= 0 && branch[i+1:] == id
}
