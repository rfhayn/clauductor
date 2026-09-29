package lease

import (
	"strings"
	"unicode/utf8"
)

// clip makes s one line of at most n runes. It keeps the panel's own rule
// (whitespace collapsed, cut at 140 runes, then at n), so a record lock-run writes
// reads as it did before the lease had a package of its own.
func clip(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) > 140 {
		s = string([]rune(s)[:140]) + "…"
	}
	r := []rune(s)
	if len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}
