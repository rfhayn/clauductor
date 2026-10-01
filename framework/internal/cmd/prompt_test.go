package cmd

import (
	"bufio"
	"strings"
	"testing"
)

// update re-prompts on any answer it does not know; a closed stdin must end the reviews rather
// than read as an endless run of empty answers.
func TestReadChoiceCancelsAtEndOfInput(t *testing.T) {
	r := bufio.NewReader(strings.NewReader("y\nS\n"))
	for _, want := range []string{"y", "s", "c", "c"} {
		if got := readChoiceFrom(r); got != want {
			t.Fatalf("readChoiceFrom = %q, want %q", got, want)
		}
	}
	// A last answer with no newline still counts.
	if got := readChoiceFrom(bufio.NewReader(strings.NewReader("d"))); got != "d" {
		t.Fatalf("an unterminated answer read as %q, want d", got)
	}
	// An empty line is not the end of input: update re-prompts on it.
	if got := readChoiceFrom(bufio.NewReader(strings.NewReader("\ny\n"))); got != "" {
		t.Fatalf("an empty line read as %q, want empty", got)
	}
}
