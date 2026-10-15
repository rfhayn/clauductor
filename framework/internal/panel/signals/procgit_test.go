package signals

import (
	"strings"
	"testing"
)

func TestParseGitStatusV2(t *testing.T) {
	t.Parallel()
	out := "# branch.oid 1234abcd\n# branch.head change/x\n# branch.upstream origin/change/x\n# branch.ab +3 -1\n" +
		"1 .M N... 100644 100644 100644 a b src/a.ts\n2 R. N... 100644 100644 100644 a b R100 new.ts\told.ts\nu UU N... 1 2 3 4 a b c x.ts\n? notes.md\n? tmp/\n"
	g, err := ParseGitStatusV2([]byte(out))
	if err != nil {
		t.Fatal(err)
	}
	if g.Head != "1234abcd" || g.Branch != "change/x" || !g.HasUpstream || g.Ahead != 3 || g.Behind != 1 || g.Changed != 2 || g.Conflicts != 1 || g.Untracked != 2 || g.Dirty() != 5 {
		t.Fatalf("%+v", g)
	}
	// The diff stat's pathspec: every tracked changed path, a rename's old one too,
	// and no untracked one (PANEL-42).
	if p, ok := TrackedChangedPaths([]byte(out)); !ok || strings.Join(p, "|") != "src/a.ts|new.ts|old.ts|x.ts" {
		t.Fatalf("paths %q %v", p, ok)
	}
	quoted := "1 .M N... 100644 100644 100644 a b \"sp\\303\\251cial\\tname\"\n1 .M N... 100644 100644 100644 a b with space.go\n"
	if p, ok := TrackedChangedPaths([]byte(quoted)); !ok || strings.Join(p, "|") != "spécial\tname|with space.go" {
		t.Fatalf("quoted paths %q %v", p, ok)
	}
	// With core.quotePath=false git leaves a byte over 0x7f raw inside the quotes of a
	// path it quotes for another reason (a tab): the path is those bytes, not U+FFFD.
	// An escaped one (\377) is the byte too, and \a \b \f \v are git's escapes as well.
	raw := "1 .M N... 100644 100644 100644 a b \"r\xffw\\tname\"\n1 .M N... 100644 100644 100644 a b \"\\377\\a\\b\\f\\v\\\"\\\\\"\n"
	if p, ok := TrackedChangedPaths([]byte(raw)); !ok || strings.Join(p, "|") != "r\xffw\tname|\xff\a\b\f\v\"\\" {
		t.Fatalf("raw-byte paths %q %v", p, ok)
	}
	for _, bad := range []string{"1 .M N... 100644 100644 100644 a b \"unterminated\n", "1 .M N... 100644 100644 100644 a b \"bad\\qescape\"\n", "1 .M N... 100644 100644 100644 a b \"short\\37\"\n", "2 R. N... 100644 100644 100644 a b R100 no-tab\n", "1 .M short\n"} {
		if p, ok := TrackedChangedPaths([]byte(bad)); ok {
			t.Errorf("%q read as the whole list: %q", bad, p)
		}
	}
	g, err = ParseGitStatusV2([]byte("# branch.oid (initial)\n# branch.head main\n"))
	if err != nil || g.HasUpstream || g.Dirty() != 0 {
		t.Fatalf("no upstream, clean: %+v %v", g, err)
	}
	if _, err := ParseGitStatusV2([]byte("fatal: not a git repository\n")); err == nil {
		t.Fatal("output without a branch header parsed")
	}
	if f, i, d := ParseShortstat([]byte(" 3 files changed, 10 insertions(+), 2 deletions(-)\n")); f != 3 || i != 10 || d != 2 {
		t.Fatalf("shortstat %d %d %d", f, i, d)
	}
	if f, i, d := ParseShortstat([]byte(" 1 file changed, 1 deletion(-)\n")); f != 1 || i != 0 || d != 1 {
		t.Fatalf("shortstat %d %d %d", f, i, d)
	}
}

func TestParsePS(t *testing.T) {
	t.Parallel()
	p := ParsePS([]byte("  4101  12.5 204800\n 4102   0,3  1024\nnot a line\n"))
	if len(p) != 2 || p[4101].CPU != 12.5 || p[4101].RSSKB != 204800 || p[4102].CPU != 0.3 {
		t.Fatalf("%+v", p)
	}
}
