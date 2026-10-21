package lanes

import (
	"testing"
)

// While the names are one, the list format is byte for byte the one before the brand package.
func TestListFormatIsUnchangedWhileTheNameIs(t *testing.T) {
	const want = "#{session_name}\t#{pane_current_path}\t#{session_path}\t#{pane_dead}\t#{pane_dead_status}\t#{session_created}\t#{session_attached}\t#{@clauductor_type}\t#{@clauductor_project}"
	if tmuxListFormat != want {
		t.Fatalf("tmuxListFormat = %q, want %q", tmuxListFormat, want)
	}
}

// After the rename (OPS-104), a lane an earlier panel started carries the legacy name's
// options: it is still read as its type and project. A lane with the new name's options is
// read from those.
func TestParseTmuxPanesReadsTheLegacyNamesOptions(t *testing.T) {
	const want = "#{session_name}\t#{pane_current_path}\t#{session_path}\t#{pane_dead}\t#{pane_dead_status}\t#{session_created}\t#{session_attached}" +
		"\t#{@newname_type}\t#{@newname_project}\t#{@clauductor_type}\t#{@clauductor_project}"
	if got := listFormat([]string{"newname", "clauductor"}); got != want {
		t.Fatalf("listFormat = %q, want %q", got, want)
	}
	out := []byte("old\t/p\t/p\t0\t\t100\t0\t\t\tclaude\tapp\n" +
		"new\t/p\t/p\t0\t\t100\t0\tshell\tweb\t\t\n")
	got := parseTmuxPanesOf(out, 2)
	if len(got) != 2 {
		t.Fatalf("lanes = %+v", got)
	}
	if got[1].ID != "old" || got[1].Type != "claude" || got[1].Project != "app" {
		t.Errorf("the legacy lane = %+v, want type claude, project app", got[1])
	}
	if got[0].ID != "new" || got[0].Type != "shell" || got[0].Project != "web" {
		t.Errorf("the new lane = %+v, want type shell, project web", got[0])
	}
}
