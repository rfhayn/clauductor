package signals

import (
	"strings"
	"testing"
)

func TestParseSuggestions(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		in      string
		want    []Suggestion
		skipped int
	}{
		"json objects": {
			in:   `[{"name":"add-login","title":"Sign in","detail":"needs 2C.14"},{"name":"Bad Name","title":"x"},{"name":"fix-12","issue":"#12"}]`,
			want: []Suggestion{{Name: "add-login", Title: "Sign in", Detail: "needs 2C.14"}, {Name: "fix-12", Issue: "#12"}}, skipped: 1,
		},
		"json names": {in: `["a-one", "b-two"]`, want: []Suggestion{{Name: "a-one"}, {Name: "b-two"}}},
		"text": {
			in:   "# up next\nadd-login — Sign in with Google\n\nadd-teams\tTeams across rounds\nNot A Name at all\n",
			want: []Suggestion{{Name: "add-login", Title: "Sign in with Google"}, {Name: "add-teams", Title: "Teams across rounds"}}, skipped: 1,
		},
		"control characters fold": {
			in:   `[{"name":"a","title":"one\u001b[31m\ntwo‮"}]`,
			want: []Suggestion{{Name: "a", Title: "one [31m two"}},
		},
		"empty": {in: "  \n", want: []Suggestion{}},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := ParseSuggestions([]byte(tc.in))
			if err != nil {
				t.Fatal(err)
			}
			if got.Skipped != tc.skipped || len(got.Items) != len(tc.want) {
				t.Fatalf("got %+v", got)
			}
			for i := range tc.want {
				if got.Items[i] != tc.want[i] {
					t.Errorf("row %d: %+v, want %+v", i, got.Items[i], tc.want[i])
				}
			}
		})
	}
}

func TestParseSuggestionsLimits(t *testing.T) {
	t.Parallel()
	var b strings.Builder
	for i := 0; i < 80; i++ {
		b.WriteString("row-" + strings.Repeat("x", i%5) + " " + strings.Repeat("t", 300) + "\n")
	}
	got, err := ParseSuggestions([]byte(b.String()))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != maxSuggestions || len([]rune(got.Items[0].Title)) != maxSuggestTitle {
		t.Fatalf("%d rows, title %d runes", len(got.Items), len([]rune(got.Items[0].Title)))
	}
	if _, err := ParseSuggestions([]byte(`[{"name": 3}]`)); err == nil {
		t.Error("a row that is neither a name nor an object was accepted")
	}
}
