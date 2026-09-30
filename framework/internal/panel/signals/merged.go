package signals

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

// MergedPR is a merged pull request, as `gh pr list --state merged` reports it
// (PANEL-19: the Metrics view's merge frequency and PR cycle time).
type MergedPR struct {
	Number    int
	Title     string
	Branch    string
	CreatedAt time.Time
	MergedAt  time.Time
}

// MergedLimit is how many merged pull requests one read asks gh for.
const MergedLimit = 300

// MergedArgv is the one read of merged pull requests: those merged since `since`
// (2006-01-02, 90 days back), at most MergedLimit.
func MergedArgv(since string) []string {
	return []string{"gh", "pr", "list", "--state", "merged", "--limit", strconv.Itoa(MergedLimit),
		"--search", "merged:>=" + since, "--json", "number,title,headRefName,createdAt,mergedAt"}
}

// ParseMergedPRs parses MergedArgv's output. A row without a merge time is skipped.
func ParseMergedPRs(out []byte) ([]MergedPR, error) {
	var raw []struct {
		Number      int       `json:"number"`
		Title       string    `json:"title"`
		HeadRefName string    `json:"headRefName"`
		CreatedAt   time.Time `json:"createdAt"`
		MergedAt    time.Time `json:"mergedAt"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(out), &raw); err != nil {
		return nil, fmt.Errorf("gh pr list --state merged: %w", err)
	}
	prs := make([]MergedPR, 0, len(raw))
	for _, r := range raw {
		if r.MergedAt.IsZero() {
			continue
		}
		prs = append(prs, MergedPR{Number: r.Number, Title: r.Title, Branch: r.HeadRefName, CreatedAt: r.CreatedAt, MergedAt: r.MergedAt})
	}
	return prs, nil
}
