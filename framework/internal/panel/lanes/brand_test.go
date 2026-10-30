package lanes

import "testing"

// The list format reads the lane type and project options spelled from brand.Name.
func TestListFormatIsSpelledFromTheName(t *testing.T) {
	const want = "#{session_name}\t#{pane_current_path}\t#{session_path}\t#{pane_dead}\t#{pane_dead_status}\t#{session_created}\t#{session_attached}" +
		"\t#{@stridal_type}\t#{@stridal_project}"
	if tmuxListFormat != want {
		t.Fatalf("tmuxListFormat = %q, want %q", tmuxListFormat, want)
	}
}
