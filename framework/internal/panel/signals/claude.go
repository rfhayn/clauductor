package signals

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ClaudeStopGrace is how long a `claude` the panel stops has between SIGTERM and
// SIGKILL (PANEL-28): it covers Claude Code's 5 s refresh bound and its 2 s exit wait,
// so a process holding the login-refresh lock can release it. SIGKILL allows no
// release, so it comes last.
const ClaudeStopGrace = 15 * time.Second

// ClaudeArgs is claude's own arguments when argv runs Claude Code: its program (any
// path) is named claude, directly or after `env` (lanes.ScrubbedArgv), GNU's or
// BSD's. env's arguments are read as env reads them (envCommand): short options
// alone or clustered, with their argument attached or the next word (-u NAME,
// -iuNAME); long options, also as an unambiguous prefix (--unset NAME, --uns=NAME);
// NAME=value assignments; `--`; and -S/--split-string, whose string is split into
// words (splitWords) and read as more of env's arguments. ok is false for any other
// command.
func ClaudeArgs(argv []string) (args []string, ok bool) {
	for depth := 0; len(argv) > 0 && filepath.Base(argv[0]) == "env" && depth < 8; depth++ {
		argv = envCommand(argv[1:], 0)
	}
	if len(argv) == 0 || filepath.Base(argv[0]) != "claude" {
		return nil, false
	}
	return argv[1:], true
}

// envShortArg are the short options of GNU and BSD env that take an argument:
// -u NAME, -C DIR, -a ARG0 (GNU); -P PATH, -L/-U USER (BSD); -S STRING (both).
const envShortArg = "uCaPLUS"

// envLong are GNU env's long options, and whether each takes an argument (one that
// takes it only after "=", as --block-signal[=SIG], does not).
var envLong = map[string]bool{
	"ignore-environment": false, "null": false, "unset": true, "chdir": true, "split-string": true, "argv0": true,
	"block-signal": false, "default-signal": false, "ignore-signal": false, "list-signal-handling": false,
	"debug": false, "help": false, "version": false,
}

// envLongOption is the long option name names, exactly or as an unambiguous prefix
// (GNU getopt's rule); "" for none or several.
func envLongOption(name string) string {
	if _, ok := envLong[name]; ok {
		return name
	}
	found := ""
	for opt := range envLong {
		if strings.HasPrefix(opt, name) {
			if found != "" {
				return ""
			}
			found = opt
		}
	}
	return found
}

// envCommand is the command env runs, given env's arguments (nil: none). depth bounds
// nested -S strings.
func envCommand(rest []string, depth int) []string {
	split := func(s string, after []string) []string {
		if depth >= 8 {
			return nil
		}
		return envCommand(append(splitWords(s), after...), depth+1)
	}
	for i := 0; i < len(rest); i++ {
		a := rest[i]
		switch {
		case a == "--":
			return rest[i+1:]
		case a == "-":
			// -i, in its oldest spelling
		case strings.HasPrefix(a, "--"):
			name, val, hasVal := strings.Cut(a[2:], "=")
			opt := envLongOption(name)
			if !envLong[opt] || hasVal {
				if opt == "split-string" {
					return split(val, rest[i+1:])
				}
				continue
			}
			if i+1 >= len(rest) {
				return nil
			}
			i++
			if opt == "split-string" {
				return split(rest[i], rest[i+1:])
			}
		case strings.HasPrefix(a, "-"):
			// A cluster of short options: one that takes an argument takes the rest
			// of the cluster, or the next word.
			for j := 1; j < len(a); j++ {
				c := a[j]
				if strings.IndexByte(envShortArg, c) < 0 {
					continue
				}
				val := a[j+1:]
				if val == "" {
					if i+1 >= len(rest) {
						return nil
					}
					i++
					val = rest[i]
				}
				if c == 'S' {
					return split(val, rest[i+1:])
				}
				break
			}
		case strings.Contains(a, "="):
			// NAME=value
		default:
			return rest[i:]
		}
	}
	return nil
}

// splitWords splits an env -S string into words: blanks separate them, single quotes
// keep their text as it is, double quotes and a backslash outside quotes escape the
// next character. env's ${VAR} expansion and its \c, \_ and #-comment forms are not
// interpreted: a command spelled with them is not recognised as claude.
func splitWords(s string) []string {
	var words []string
	var w strings.Builder
	in, quote := false, byte(0)
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote == '\'':
			if c == '\'' {
				quote = 0
			} else {
				w.WriteByte(c)
			}
		case quote == '"':
			switch {
			case c == '"':
				quote = 0
			case c == '\\' && i+1 < len(s):
				i++
				w.WriteByte(s[i])
			default:
				w.WriteByte(c)
			}
		case c == '\'' || c == '"':
			quote, in = c, true
		case c == '\\' && i+1 < len(s):
			i++
			w.WriteByte(s[i])
			in = true
		case c == ' ' || c == '\t' || c == '\n':
			if in {
				words = append(words, w.String())
				w.Reset()
				in = false
			}
		default:
			w.WriteByte(c)
			in = true
		}
	}
	if in {
		words = append(words, w.String())
	}
	return words
}

// IsClaude reports whether argv runs Claude Code (ClaudeArgs).
func IsClaude(argv []string) bool {
	_, ok := ClaudeArgs(argv)
	return ok
}

type pollKey struct{}

// WithPoll marks ctx as a poll's (PANEL-28): the gate gives a poll a short wait for
// the slot and a quick retry, where a call with no mark (a lane action) waits within
// its own timeout. Runtime.exec and the machine's run set it.
func WithPoll(ctx context.Context) context.Context { return context.WithValue(ctx, pollKey{}, true) }

// IsPoll reports whether ctx carries WithPoll's mark.
func IsPoll(ctx context.Context) bool {
	v, _ := ctx.Value(pollKey{}).(bool)
	return v
}

type processStartKey struct{}

// WithProcessStart asks ExecRunner to report each `claude` it starts under ctx: f
// gets the started process (not a bare pid, so the panel's shutdown can still kill
// it, and never a reused pid). The gate keeps its call history and its shutdown with it.
func WithProcessStart(ctx context.Context, f func(*os.Process)) context.Context {
	return context.WithValue(ctx, processStartKey{}, f)
}

func processStart(ctx context.Context) func(*os.Process) {
	f, _ := ctx.Value(processStartKey{}).(func(*os.Process))
	return f
}

type stopSignalKey struct{}

// WithStopSignal asks ExecRunner to report the stop signal it delivered to a `claude`
// under ctx: f is called only when the signal reached a process still running (not
// for os.ErrProcessDone), so the gate's log names only signals actually sent.
func WithStopSignal(ctx context.Context, f func(os.Signal)) context.Context {
	return context.WithValue(ctx, stopSignalKey{}, f)
}

func stopSignal(ctx context.Context) func(os.Signal) {
	f, _ := ctx.Value(stopSignalKey{}).(func(os.Signal))
	return f
}
