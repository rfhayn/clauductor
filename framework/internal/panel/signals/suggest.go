package signals

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/clauductor/clauductor/internal/panel/config"
)

// Suggestion is one row a template's suggest command offers the Start dialog: what
// could be started next. Name becomes the lane name; Issue, when set, the issue.
type Suggestion struct {
	Name   string `json:"name"`
	Title  string `json:"title,omitempty"`
	Detail string `json:"detail,omitempty"`
	Issue  string `json:"issue,omitempty"`
}

// Suggestions is a suggest command's parsed output.
type Suggestions struct {
	Items []Suggestion `json:"items"`
	// Skipped counts rows with no usable lane name, so a command that names its rows
	// "Add login" rather than "add-login" says why its list is short.
	Skipped int `json:"skipped,omitempty"`
}

// A list is read in the dialog, not scrolled through: the first rows are what's next.
const (
	maxSuggestions  = 50
	maxSuggestTitle = 200
	maxSuggestText  = 400
)

// ParseSuggestions reads a suggest command's stdout. It takes JSON, an array of
// {"name", "title", "detail", "issue"} objects or of names, or else text: one row a
// line, the first word the name and the rest its title ("#" starts a comment).
// A name must be a lane id (config.LaneIDRe); a row without one is skipped. Every
// text is cut to one line of plain text: it reaches the page, never a command.
func ParseSuggestions(out []byte) (Suggestions, error) {
	trimmed := bytes.TrimSpace(out)
	var rows []Suggestion
	if len(trimmed) > 0 && trimmed[0] == '[' {
		var raw []json.RawMessage
		if err := json.Unmarshal(trimmed, &raw); err != nil {
			return Suggestions{}, fmt.Errorf("suggest output: %w", err)
		}
		for _, r := range raw {
			var s Suggestion
			if err := json.Unmarshal(r, &s.Name); err != nil {
				if err := json.Unmarshal(r, &s); err != nil {
					return Suggestions{}, fmt.Errorf("suggest output: each row is a name or an object with a \"name\": %w", err)
				}
			}
			rows = append(rows, s)
		}
	} else {
		for _, l := range strings.Split(string(trimmed), "\n") {
			l = strings.TrimSpace(l)
			if l == "" || strings.HasPrefix(l, "#") {
				continue
			}
			name, title, _ := strings.Cut(l, " ")
			if i := strings.IndexAny(name, "\t"); i >= 0 {
				name, title = name[:i], name[i+1:]+" "+title
			}
			rows = append(rows, Suggestion{Name: name, Title: strings.TrimLeft(strings.TrimSpace(title), "-—–: ")})
		}
	}
	res := Suggestions{Items: []Suggestion{}}
	for _, s := range rows {
		s.Name = strings.TrimSpace(s.Name)
		if !config.ValidLaneID(s.Name) {
			res.Skipped++
			continue
		}
		if len(res.Items) == maxSuggestions {
			break
		}
		s.Title, s.Detail, s.Issue = plainLine(s.Title, maxSuggestTitle), plainLine(s.Detail, maxSuggestText), plainLine(s.Issue, maxSuggestTitle)
		res.Items = append(res.Items, s)
	}
	return res, nil
}

// plainLine folds every control or format character to a space, collapses runs of
// space, and cuts the result to max runes.
func plainLine(s string, max int) string {
	s = strings.Join(strings.FieldsFunc(s, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r) || unicode.Is(unicode.Cf, r)
	}), " ")
	if r := []rune(s); len(r) > max {
		s = string(r[:max-1]) + "…"
	}
	return s
}
