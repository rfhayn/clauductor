package plugin

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
)

var (
	onlyExpansion = regexp.MustCompile(`^"\$(?:\{[^}]*\}|[A-Za-z_][A-Za-z0-9_]*|[0-9@*])"$`)
	shAssignment  = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)
)

// writtenScript is one script a check writes: a quoted heredoc's body, or a printf's words (format
// and arguments) whose output goes to a file; open is the line that starts it.
type writtenScript struct {
	open, text string
	heredoc    bool
}

// writtenScripts reads the scripts a shell file writes, heredocs first and then printfs, each in
// scan order, so a template check's and its plugin copy's pair one to one. It returns what it could
// not read.
//
// It reads the shell as the shell does, not line by line: quotes, backticks, $(…), $((…)) and ${…}
// are consumed whole, so a `<<'T'` or a `printf … >` inside a string (a payload a check hands a
// hook, an awk program) is text, never an opener. What it reads:
//   - a heredoc whose tag is quoted (`<<'T'`, `<<"T"`, `<<\T`, with or without `-`), wherever it
//     sits in the command (`cat <<'T' > f`, `fixture NAME <<'T'`, a comment after it). Its body runs
//     to the tag line, past leading tabs only for `<<-`.
//   - a printf or echo whose output reaches a file: its own redirect, before or after the command
//     (`printf -- '…' > f`, `> f printf …`, `>>`), or the redirect of a group, loop, if or case
//     around it (`{ printf …; cat g; } > f`), at any depth, inside $(…) too. Its text is every word
//     of its command, a redirect's target excepted, so what %s writes is compared too.
//
// Out of scope, by design: an unquoted heredoc (`<<T`: its body expands, so it is the check's own
// text, as a double-quoted string is); text held in a variable (`printf '%s' "$x" > f` compares
// "$x", not what x holds, which was set elsewhere); a printf or echo into a pipe, a $(…) capture or
// the terminal (its text is the check's own output); one in a function whose callers redirect it
// (calls are not followed); and the body of a backtick substitution, which is skipped as a string.
//
// It fails closed: a quote, $(, $((, ${ or heredoc that never closes, a ) that closes nothing, a
// printf to a file whose format is not a literal string (or is only an expansion, "$fmt"), and a
// redirect it does not follow (`&>`, `exec > f`) are errors, never a skip.
func writtenScripts(s string) ([]writtenScript, []string) {
	sc := &shScanner{s: s, line: 1, lines: strings.Split(s, "\n")}
	sc.lists = append(sc.lists, sc.code(0))
	var prints []writtenScript
	errs := sc.errs
	for _, l := range sc.lists {
		p, e := printfScripts(l, sc.lines)
		prints = append(prints, p...)
		errs = append(errs, e...)
	}
	return append(sc.docs, prints...), errs
}

// shTok is one token: a word, an operator (`;` `;;` `&&` `||` `|` `&` a newline `{` `}` `(` `)`),
// or a redirect (text: its target).
type shTok struct {
	kind   byte // 'w', 'o', 'r'
	text   string
	fd     string // a redirect's descriptor, "" when none is written
	dup    bool   // >&N
	output bool   // > or >>, not <
	line   int
}

type shScanner struct {
	s     string
	i     int
	line  int
	lines []string
	lists [][]shTok // each code level's tokens: the file's, and each $(…)'s
	docs  []writtenScript
	errs  []string
	pend  []pendingDoc
}

type pendingDoc struct {
	tag          string
	quoted, dash bool
	line         int
}

func (sc *shScanner) errf(format string, a ...any) {
	sc.errs = append(sc.errs, fmt.Sprintf(format, a...))
}

func (sc *shScanner) peek(k int) byte {
	if sc.i+k < len(sc.s) {
		return sc.s[sc.i+k]
	}
	return 0
}

// at reports whether the text at sc.i starts with p.
func (sc *shScanner) at(p string) bool { return strings.HasPrefix(sc.s[sc.i:], p) }

// isExp: a substitution starts at sc.i (`…`, $(…), $((…), ${…}): what a double quote still expands.
func (sc *shScanner) isExp() bool { return sc.at("`") || sc.at("$(") || sc.at("${") }

// isSub: a quote or a substitution starts at sc.i, to be consumed whole.
func (sc *shScanner) isSub() bool { return sc.at("'") || sc.at(`"`) || sc.isExp() }

// shKeepsCmd: the keywords after which the next word is still a command.
var shKeepsCmd = map[string]bool{"then": true, "do": true, "else": true, "elif": true, "!": true, "if": true, "while": true, "until": true}

// code reads shell code until EOF, or (depth > 0) the `)` that closes the $( it was called for, and
// returns its tokens. A $(…) inside it is read by its own call, whose tokens are their own list.
func (sc *shScanner) code(depth int) []shTok {
	var toks []shTok
	var word strings.Builder
	wordLine, paren := 0, 0
	cmd := true      // at a command's first word
	afterCase := 0   // words left before a case's first pattern: its subject, then `in`
	pattern := false // reading a case pattern, up to its `)`
	emit := func(kind byte, text string, line int) {
		toks = append(toks, shTok{kind: kind, text: text, line: line})
	}
	op := func(text string, line int) {
		emit('o', text, line)
		cmd = true
		if text == ";;" { // only a case item ends in ;;, and a pattern or esac follows
			pattern = true
		}
	}
	flush := func() {
		if word.Len() == 0 {
			return
		}
		w := word.String()
		word.Reset()
		switch {
		case w == "esac": // a case's end, where a pattern could start
			pattern = false
		case pattern:
		case cmd && w == "case":
			afterCase = 2
		case afterCase > 0:
			afterCase--
			pattern = afterCase == 0
		}
		if cmd && (w == "{" || w == "}") { // a group's brace, not a `}` argument
			emit('o', w, wordLine)
			return
		}
		emit('w', w, wordLine)
		// A keyword keeps the next word at command position; any other word ends it. (The `{` and
		// `}` that can follow an assignment are not groups, so an assignment ends it too.)
		cmd = cmd && shKeepsCmd[w]
	}
	start := func() {
		if word.Len() == 0 {
			wordLine = sc.line
		}
	}
	for sc.i < len(sc.s) {
		c := sc.s[sc.i]
		switch {
		case c == '\n':
			flush()
			if !pattern {
				op("\n", sc.line)
			}
			sc.i++
			sc.line++
			sc.readHeredocs()
		case c == ' ' || c == '\t' || c == '\r':
			flush()
			sc.i++
		case c == '#' && word.Len() == 0:
			for sc.i < len(sc.s) && sc.s[sc.i] != '\n' {
				sc.i++
			}
		case c == '\\':
			if sc.peek(1) == '\n' { // a continued line
				sc.i += 2
				sc.line++
				continue
			}
			start()
			word.WriteString(sc.s[sc.i:min(sc.i+2, len(sc.s))])
			sc.i += 2
		case sc.isSub():
			start()
			from := sc.i
			sc.quoted(depth)
			word.WriteString(sc.s[from:min(sc.i, len(sc.s))])
		case sc.at("<<<"): // a here-string: its word follows
			flush()
			sc.i += 3
		case sc.at("<<"):
			flush()
			sc.i += 2
			sc.heredocOpener()
		case sc.at("&>"): // refused, then read on as the > it also is
			flush()
			sc.errf("line %d: &> (bash) sends output where the reader does not follow it: spell it > f 2>&1", sc.line)
			sc.i++
		case c == '<' || c == '>':
			fd := ""
			if w := word.String(); strings.Trim(w, "0123456789") == "" { // "" is no descriptor, as ever
				fd = w
				word.Reset()
			} else {
				flush()
			}
			t := shTok{kind: 'r', fd: fd, output: c == '>', line: sc.line}
			sc.i++
			switch sc.peek(0) {
			case '>', '|':
				sc.i++
			case '&':
				t.dup = true
				sc.i++
			}
			for sc.peek(0) == ' ' || sc.peek(0) == '\t' {
				sc.i++
			}
			t.text = sc.word(depth)
			toks = append(toks, t)
		case c == ';' || c == '&' || c == '|':
			flush()
			o := string(c)
			if sc.peek(1) == c {
				o += string(c)
				sc.i++
			}
			sc.i++
			op(o, sc.line)
		case sc.at("()"): // a function's `()`
			flush()
			sc.i += 2
			cmd = true
		case c == '(' && pattern: // a pattern's leading `(`
			flush()
			sc.i++
		// Not mid-word: `a(b` is a syntax error in sh, so the word test is equivalent on valid shell.
		case c == '(' && word.Len() == 0:
			paren++
			sc.i++
			op("(", sc.line)
		case c == ')':
			flush()
			sc.i++
			switch {
			case pattern: // a case pattern's close
				pattern = false
				op(";", sc.line)
			case paren > 0:
				paren--
				emit('o', ")", sc.line)
				cmd = false
			case depth > 0:
				return toks
			default:
				sc.errf("line %d: a ) that closes nothing", sc.line)
			}
		default:
			start()
			word.WriteByte(c)
			sc.i++
		}
	}
	flush()
	if depth > 0 {
		sc.errf("a $( never closes (read to the end at line %d)", sc.line)
	}
	return toks
}

// quoted consumes one quoted or substituted span at sc.i: '…', "…", `…`, $(…), $((…)) or ${…}.
func (sc *shScanner) quoted(depth int) {
	open := sc.line
	switch c := sc.s[sc.i]; {
	case c == '\'':
		sc.i++
		for sc.i < len(sc.s) && sc.s[sc.i] != '\'' {
			sc.newline()
			sc.i++
		}
		if sc.i >= len(sc.s) {
			sc.errf("a ' opened at line %d never closes", open)
			return
		}
		sc.i++
	case c == '"':
		sc.i++
		for sc.i < len(sc.s) && sc.s[sc.i] != '"' {
			switch {
			case sc.s[sc.i] == '\\':
				sc.i++
				sc.newline()
				sc.i++
			case sc.isExp():
				sc.quoted(depth)
			default:
				sc.newline()
				sc.i++
			}
		}
		if sc.i >= len(sc.s) {
			sc.errf("a \" opened at line %d never closes", open)
			return
		}
		sc.i++
	case c == '`':
		sc.i++
		for sc.i < len(sc.s) && sc.s[sc.i] != '`' {
			if sc.s[sc.i] == '\\' {
				sc.i++
			}
			sc.newline()
			sc.i++
		}
		if sc.i >= len(sc.s) {
			sc.errf("a ` opened at line %d never closes", open)
			return
		}
		sc.i++
	case sc.at("$(("): // arithmetic, where << is a shift
		sc.i += 3
		n := 2
		for sc.i < len(sc.s) && n > 0 {
			switch sc.s[sc.i] {
			case '(':
				n++
			case ')':
				n--
			}
			sc.newline()
			sc.i++
		}
		if n > 0 {
			sc.errf("a $(( opened at line %d never closes", open)
		}
	case sc.at("$("):
		sc.i += 2
		sc.lists = append(sc.lists, sc.code(depth+1))
	default: // ${…}: a quote or substitution inside it is consumed whole
		sc.i += 2
		n := 1
		for sc.i < len(sc.s) && n > 0 {
			if sc.isSub() {
				sc.quoted(depth)
				continue
			}
			switch sc.s[sc.i] {
			case '{':
				n++
			case '}':
				n--
			case '\\':
				sc.i++
			}
			sc.newline()
			sc.i++
		}
		if n > 0 {
			sc.errf("a ${ opened at line %d never closes", open)
		}
	}
}

// newline counts a newline at sc.i inside a quoted span (no heredoc starts there).
func (sc *shScanner) newline() {
	if sc.i < len(sc.s) && sc.s[sc.i] == '\n' {
		sc.line++
	}
}

// word consumes one word at sc.i (a redirect's target, a heredoc's tag), quotes included, and
// returns it raw.
func (sc *shScanner) word(depth int) string {
	from := sc.i
	for sc.i < len(sc.s) {
		c := sc.s[sc.i]
		if strings.IndexByte(" \t\r\n;&|<>()", c) >= 0 {
			break
		}
		switch {
		case sc.isSub():
			sc.quoted(depth)
		case c == '\\':
			sc.i++
			sc.newline()
			sc.i++
		default:
			sc.i++
		}
	}
	return sc.s[from:min(sc.i, len(sc.s))]
}

// heredocOpener reads `<<` 's tag at sc.i; its body is read after the line ends (readHeredocs).
func (sc *shScanner) heredocOpener() {
	p := pendingDoc{line: sc.line}
	if sc.peek(0) == '-' {
		p.dash = true
		sc.i++
	}
	for sc.peek(0) == ' ' || sc.peek(0) == '\t' {
		sc.i++
	}
	raw := sc.word(1)
	p.quoted = strings.ContainsAny(raw, `'"\`)
	p.tag = strings.NewReplacer(`'`, "", `"`, "", `\`, "").Replace(raw)
	if p.tag == "" {
		sc.errf("line %d: a << with no tag", sc.line)
		return
	}
	sc.pend = append(sc.pend, p)
}

// readHeredocs reads the bodies of the heredocs opened on the line that just ended.
func (sc *shScanner) readHeredocs() {
	for _, p := range sc.pend {
		from, closed := sc.i, false
		for sc.i < len(sc.s) {
			eol := strings.IndexByte(sc.s[sc.i:], '\n')
			if eol < 0 {
				eol = len(sc.s) - sc.i
			}
			l := sc.s[sc.i : sc.i+eol]
			if p.dash {
				l = strings.TrimLeft(l, "\t")
			}
			next := min(sc.i+eol+1, len(sc.s))
			if l == p.tag {
				if p.quoted {
					sc.docs = append(sc.docs, writtenScript{open: sc.lines[p.line-1], text: strings.TrimSuffix(sc.s[from:sc.i], "\n"), heredoc: true})
				}
				sc.i = next
				sc.line++
				closed = true
				break
			}
			sc.i = next
			sc.line++
		}
		if !closed {
			sc.errf("the heredoc opened at line %d (%s) never closes", p.line, strings.TrimSpace(sc.lines[p.line-1]))
		}
	}
	sc.pend = nil
}

// printfScripts finds, in one code level's tokens, the printfs whose output reaches a file. A frame
// is a group, loop, if or case: a printf with no redirect of its own writes where its frame's
// output goes. A printf with no redirect and no frame writes to the level's own output: the
// terminal, or a $(…)'s capture.
func printfScripts(toks []shTok, lines []string) ([]writtenScript, []string) {
	var out []writtenScript
	var errs []string
	type pending struct {
		fmt, text string
		line      int
		echo      bool // echo has no format: its words are its text
	}
	var frames [][]pending
	opens := map[string]bool{"do": true, "if": true, "case": true}
	closes := map[string]bool{"done": true, "fi": true, "esac": true}
	// Only a redirect token is an output, so neither test needs its kind.
	isOut := func(t shTok) bool { return t.output && (t.fd == "" || t.fd == "1") }
	isFile := func(t shTok) bool { return isOut(t) && !t.dup && !strings.Contains(t.text, "/dev/null") }
	resolve := func(ps []pending, to string) {
		for _, p := range ps {
			// A format that is only an expansion ("$fmt") holds text set elsewhere, unread here.
			if !p.echo && (p.fmt == "" || (p.fmt[0] != '\'' && p.fmt[0] != '"') || onlyExpansion.MatchString(p.fmt)) {
				errs = append(errs, fmt.Sprintf("line %d (%s): a printf writes to %s with a format that is not a literal string: read it, or say why not", p.line, strings.TrimSpace(lines[p.line-1]), to))
				continue
			}
			out = append(out, writtenScript{open: lines[p.line-1], text: p.text})
		}
	}
	// redirect reads the command from toks[k] to its end: the file its output goes to, whether it
	// sends its output somewhere of its own (a file, >&2, /dev/null), and whether it is piped.
	redirect := func(k int) (to string, own, piped bool) {
		for ; k < len(toks); k++ {
			t := toks[k]
			if t.kind == 'o' {
				return to, own, t.text == "|"
			}
			own = own || isOut(t)
			if to == "" && isFile(t) {
				to = t.text
			}
		}
		return to, own, false
	}
	// A redirect written before the command word: its file, and whether it sends output anywhere.
	// One written after the command's words sets them too, but nothing reads them before the next
	// word or operator clears them.
	cmd, pre, preOwn := true, "", false
	for k, t := range toks {
		switch {
		case t.kind == 'r':
			if isOut(t) {
				preOwn = true
				if pre == "" && isFile(t) {
					pre = t.text
				}
			}
			continue
		// An operator's text is never a keyword, so a keyword test needs no kind. The kind test on
		// `{`/`(` is equivalent on valid shell: an argument `{` is never closed by a group's `}`.
		case (t.kind == 'o' && (t.text == "{" || t.text == "(")) || (cmd && opens[t.text]):
			frames = append(frames, nil)
		case (t.kind == 'o' && (t.text == "}" || t.text == ")")) || (cmd && closes[t.text]):
			if len(frames) == 0 {
				break
			}
			ps := frames[len(frames)-1]
			frames = frames[:len(frames)-1]
			if to, own, piped := redirect(k + 1); to != "" {
				resolve(ps, to)
			} else if !own && !piped && len(frames) > 0 {
				frames[len(frames)-1] = append(frames[len(frames)-1], ps...)
			}
		case cmd && t.text == "exec":
			if to, _, _ := redirect(k + 1); to != "" {
				errs = append(errs, fmt.Sprintf("line %d (%s): exec sends the rest of the output to %s, which the reader does not follow: read it, or say why not", t.line, strings.TrimSpace(lines[t.line-1]), to))
			}
		case cmd && (t.text == "printf" || t.text == "echo"):
			// Every word to the end of the command, a redirect between them skipped (`> f` is not
			// text, and the words after it are); a printf's format is the first, past `--`.
			p := pending{line: t.line, echo: t.text == "echo"}
			var words []string
			for j := k + 1; j < len(toks) && toks[j].kind != 'o'; j++ {
				if toks[j].kind == 'w' {
					words = append(words, toks[j].text)
				}
			}
			if !p.echo && len(words) > 0 && words[0] == "--" {
				words = words[1:]
			}
			if len(words) > 0 {
				p.fmt = words[0]
			}
			p.text = strings.Join(words, " ")
			to, own, piped := redirect(k + 1)
			if to == "" {
				to, own = pre, own || preOwn
			}
			switch {
			case to != "":
				resolve([]pending{p}, to)
			case !own && !piped && len(frames) > 0:
				frames[len(frames)-1] = append(frames[len(frames)-1], p)
			}
		}
		// An operator starts a command. A keyword or an assignment leaves the position as it was
		// (a command still to come, or none); any other word ends it.
		if t.kind == 'o' {
			cmd, pre, preOwn = true, "", false
		} else if !shKeepsCmd[t.text] && !shAssignment.MatchString(t.text) {
			cmd, pre, preOwn = false, "", false
		}
	}
	return out, errs
}

// TestWrittenScripts holds the reader to what it says it reads, skips and refuses (OPS-169): each
// case is a spelling a check could use, and what the reader must make of it.
func TestWrittenScripts(t *testing.T) {
	type want struct {
		heredoc bool
		text    string
	}
	cases := []struct {
		name  string
		src   string
		want  []want
		err   string // a substring of the one error expected, or "" for none
		open2 string // when set, the second script's opening line
	}{
		// What it reads: round 7's spellings.
		{"opener, then its redirect", "cat <<'EOF' > f\nX\nEOF\n", []want{{true, "X"}}, "", ""},
		{"opener, then a comment", "cat > f <<'EOF'  # why\nX\nEOF\n", []want{{true, "X"}}, "", ""},
		{"a double-quoted tag, and <<-", "cat > f <<\"EOF\"\nX\nEOF\ncat > g <<-'EOF'\n\tY\n\tEOF\n", []want{{true, "X"}, {true, "\tY"}}, "", ""},
		{"a helper's heredoc", "fixture f <<'EOF'\nX\nEOF\n", []want{{true, "X"}}, "", ""},
		{"printf --", "printf -- 'X\\n' > f\n", []want{{false, `'X\n'`}}, "", ""},
		{"a double-quoted format", "printf \"X\\n\" > f\n", []want{{false, `"X\n"`}}, "", ""},
		{"a brace group's redirect", "{ printf 'X\\n'; cat g; } > f\n", []want{{false, `'X\n'`}}, "", ""},
		{"two printfs on a line", "printf 'a\\n' > f1; printf 'X\\n' > f\n", []want{{false, `'a\n'`}, {false, `'X\n'`}}, "", ""},
		{"a marker in a string, then a heredoc whose body holds EOF\"", "x=\"python3 <<'PY'\nPY\"\ncat > g <<'EOF'\nEOF\"\nX\nEOF\n", []want{{true, "EOF\"\nX"}}, "", ""},
		{"a loop's redirect", "for x in a; do printf 'X\\n'; done > f\n", []want{{false, `'X\n'`}}, "", ""},
		{"an if's redirect", "if true; then printf 'X\\n'; fi > f\n", []want{{false, `'X\n'`}}, "", ""},
		// Round 8's mediums.
		{"M1: a printf with its own redirect inside $(…)", "y=$(printf 'X\\n' > f; echo ok)\n", []want{{false, `'X\n'`}}, "", ""},
		{"M2: a quoted brace inside ${…}", "x=${a:-\"{\"}\nprintf 'X\\n' > f\n", []want{{false, `'X\n'`}}, "", ""},
		{"M3: the arguments %s writes", "printf '%s\\n' 'X' > f\n", []want{{false, `'%s\n' 'X'`}}, "", ""},
		// Round 8's lows.
		{"a case pattern's parens", "case $x in\n(a) printf 'X\\n' ;;\nesac > f\n", []want{{false, `'X\n'`}}, "", ""},
		{"case as an argument, inside $(…)", "y=$(echo case in)\nprintf 'X\\n' > f\n", []want{{false, `'X\n'`}}, "", ""},
		{"a pattern's leading ( inside $(…)", "y=$(case $x in (a) echo;; esac)\nprintf 'X\\n' > f\n", []want{{false, `'X\n'`}}, "", ""},
		{"a case inside $(…)", "y=$(case $x in a) echo;; esac)\nprintf 'X\\n' > f\n", []want{{false, `'X\n'`}}, "", ""},
		{"<< in $((…)) is a shift", "n=$((1 << 2))\nprintf 'X\\n' > f\n", []want{{false, `'X\n'`}}, "", ""},
		{"a function inside $(…)", "y=$(f() { echo; }; f)\nprintf 'X\\n' > f\n", []want{{false, `'X\n'`}}, "", ""},
		{"a redirect before the command", "> f printf 'X\\n'\n", []want{{false, `'X\n'`}}, "", ""},
		{"fi as an argument", "printf '%s\\n' fi > f\n", []want{{false, `'%s\n' fi`}}, "", ""},
		{"lines after a continued redirect target", "printf 'A\\n' > a\\\nb\nprintf 'B\\n' > g\n", []want{{false, `'A\n'`}, {false, `'B\n'`}}, "", "printf 'B\\n' > g"},
		// Round 9's.
		{"echo to a file", "echo 'X' > f\n", []want{{false, "'X'"}}, "", ""},
		{"echo of an expansion to a file", "echo \"$x\" > f\n", []want{{false, `"$x"`}}, "", ""},
		{"an else branch's redirect", "if x; then :; else printf 'X\\n'; fi > f\n", []want{{false, `'X\n'`}}, "", ""},
		{"an elif branch's redirect", "if a; then :; elif b; then printf 'X\\n'; fi > f\n", []want{{false, `'X\n'`}}, "", ""},
		{"a subshell's redirect", "( printf 'X\\n'; cat g ) > f\n", []want{{false, `'X\n'`}}, "", ""},
		{"after a test and &&", "[ $# -gt 0 ] && printf 'X\\n' > f\n", []want{{false, `'X\n'`}}, "", ""},
		{"a backslash-quoted tag", "cat > f <<\\EOF\nX\nEOF\n", []want{{true, "X"}}, "", ""},
		{"an assignment before printf", "X=1 printf 'X\\n' > f\n", []want{{false, `'X\n'`}}, "", ""},
		{"words after the redirect", "printf '%s\\n' a > f b\n", []want{{false, `'%s\n' a b`}}, "", ""},
		{"a } argument", "printf '%s\\n' } > f\n", []want{{false, `'%s\n' }`}}, "", ""},
		{"a printf's own >&2 inside a redirected group", "{ printf \"$m\" >&2; printf 'Y\\n'; } > f\n", []want{{false, `'Y\n'`}}, "", ""},
		// Round 10's.
		{"2>&1 inside a redirected group keeps stdout", "{ printf 'X\\n' 2>&1; printf 'Y\\n'; } > f\n", []want{{false, `'X\n'`}, {false, `'Y\n'`}}, "", ""},
		{"a redirect before the command, to stderr, inside a redirected group", "{ >&2 printf \"$m\"; printf 'Y\\n'; } > f\n", []want{{false, `'Y\n'`}}, "", ""},
		{"a nested group's own >&2", "{ { printf 'X\\n'; } >&2; printf 'Y\\n'; } > f\n", []want{{false, `'Y\n'`}}, "", ""},
		{"then as an argument", "printf '%s\\n' then } > f\n", []want{{false, `'%s\n' then }`}}, "", ""},
		{"echo keeps --", "echo -- 'X' > f\n", []want{{false, `-- 'X'`}}, "", ""},
		// Round 11's.
		{"an inner group hands its printfs to the outer", "{ { printf 'X\\n'; }; printf 'Y\\n'; } > f\n", []want{{false, `'X\n'`}, {false, `'Y\n'`}}, "", ""},
		{"an if inside a loop hands its printf to the loop", "for x in a; do if true; then printf 'X\\n'; fi; done > f\n", []want{{false, `'X\n'`}}, "", ""},
		{"do and fi as arguments open and close nothing", "{ printf '%s\\n' do fi; printf 'Y\\n'; } > f\n", []want{{false, `'%s\n' do fi`}, {false, `'Y\n'`}}, "", ""},
		{">>", "printf 'X\\n' >> f\n", []want{{false, `'X\n'`}}, "", ""},
		{"1> f", "printf 'X\\n' 1> f\n", []want{{false, `'X\n'`}}, "", ""},
		{"! printf", "! printf 'X\\n' > f\n", []want{{false, `'X\n'`}}, "", ""},
		{"a while condition", "while printf 'X\\n' > f; do :; done\n", []want{{false, `'X\n'`}}, "", ""},
		{"an until condition", "until printf 'X\\n' > f; do :; done\n", []want{{false, `'X\n'`}}, "", ""},
		{"a group as an if condition", "if { printf 'X\\n'; } > f; then :; fi\n", []want{{false, `'X\n'`}}, "", ""},
		// The mutant sweep's: one row per branch term no row held. A keyword keeps command position.
		{"a group after then", "if true; then { printf 'X\\n'; } > f; fi\n", []want{{false, `'X\n'`}}, "", ""},
		{"a group after do", "for x in a; do { printf 'X\\n'; } > f; done\n", []want{{false, `'X\n'`}}, "", ""},
		{"a group after else", "if false; then :; else { printf 'X\\n'; } > f; fi\n", []want{{false, `'X\n'`}}, "", ""},
		{"a group after elif", "if false; then :; elif { printf 'X\\n'; } > f; then :; fi\n", []want{{false, `'X\n'`}}, "", ""},
		{"a group after !", "! { printf 'X\\n'; } > f\n", []want{{false, `'X\n'`}}, "", ""},
		{"a group after while", "while { printf 'X\\n'; } > f; do :; done\n", []want{{false, `'X\n'`}}, "", ""},
		{"a group after until", "until { printf 'X\\n'; } > f; do :; done\n", []want{{false, `'X\n'`}}, "", ""},
		{"a printf as an elif condition", "if false; then :; elif printf 'X\\n' > f; then :; fi\n", []want{{false, `'X\n'`}}, "", ""},
		{"exec as an argument", "echo exec > f\n", []want{{false, "exec"}}, "", ""},
		{"printf as an argument", "echo printf > f\n", []want{{false, "printf"}}, "", ""},
		{"printf as a case's subject", "case printf in *) :;; esac > f\n", nil, "", ""},
		// Case patterns.
		{"a pattern named case, then a subshell in a loop", "case $x in case) :;; esac\nfor y in a; do ( printf 'X\\n' ) > f; done\n", []want{{false, `'X\n'`}}, "", ""},
		{"a second pattern named case, then a subshell in a loop", "case $x in a) :;; case) :;; esac\nfor y in a; do ( printf 'X\\n' ) > f; done\n", []want{{false, `'X\n'`}}, "", ""},
		{"case as an argument, then a subshell", "echo case\n( printf 'X\\n' ) > f\n", []want{{false, `'X\n'`}}, "", ""},
		{"a subshell after ; in a case item", "case x in a) :; ( printf 'X\\n' ) > f;; esac\n", []want{{false, `'X\n'`}}, "", ""},
		{"after esac, ( is a subshell again", "case x in a) :;; esac\n( printf 'X\\n' ) > f\n", []want{{false, `'X\n'`}}, "", ""},
		// Lexing.
		{"a comment holding a quote", "# don't\nprintf 'X\\n' > f\n", []want{{false, `'X\n'`}}, "", ""},
		{"a comment at the end, no newline", "printf 'X\\n' > f # end", []want{{false, `'X\n'`}}, "", ""},
		{"an escaped quote", "echo \\' x > f\nprintf 'X\\n' > g\n", []want{{false, `\' x`}, {false, `'X\n'`}}, "", ""},
		{"a continued line keeps the count", "printf 'A\\n' \\\n  > f\nprintf 'B\\n' > g\n", []want{{false, `'A\n'`}, {false, `'B\n'`}}, "", "printf 'B\\n' > g"},
		{"a here-string", "cat <<< 'x' > f\nprintf 'X\\n' > g\n", []want{{false, `'X\n'`}}, "", ""},
		{"tabs", "printf\t'X\\n'\t>\tf\n", []want{{false, `'X\n'`}}, "", ""},
		{"a CRLF line", "printf 'X\\n' > f\r\n", []want{{false, `'X\n'`}}, "", ""},
		{"a word ending in {", "printf '%s\\n' a{ > f\n", []want{{false, `'%s\n' a{`}}, "", ""},
		{"an input redirect", "printf 'X\\n' < in > f\n", []want{{false, `'X\n'`}}, "", ""},
		{"an input redirect alone", "printf 'X\\n' < in\n", nil, "", ""},
		{"a word glued to <<", "cat x<<'EOF'\nX\nEOF\n", []want{{true, "X"}}, "", ""},
		{"a redirect glued to the format", "printf 'X\\n'>f\n", []want{{false, `'X\n'`}}, "", ""},
		{"a pipe inside a redirected group", "{ printf 'X\\n' | cat; } > f\n", nil, "", ""},
		{"a piped group inside a redirected group", "{ { printf 'X\\n'; } | cat; } > f\n", nil, "", ""},
		{"a ' inside ${…}", "x=${a:-'}'}\nprintf 'X\\n' > f\n", []want{{false, `'X\n'`}}, "", ""},
		{"a $(…) inside ${…}", "x=${a:-$(echo })}\nprintf 'X\\n' > f\n", []want{{false, `'X\n'`}}, "", ""},
		{"a backtick inside ${…}", "x=${a:-`echo }`}\nprintf 'X\\n' > f\n", []want{{false, `'X\n'`}}, "", ""},
		{"a ( inside a double quote", "printf \"a(b\\n\" > f\n", []want{{false, `"a(b\n"`}}, "", ""},
		{"a $(…) holding a quote, inside a double quote", "printf \"$(echo \")\")X\\n\" > f\n", []want{{false, `"$(echo ")")X\n"`}}, "", ""},
		{"a ${…} holding a quote, inside a double quote", "printf \"${x:-\"}\"}X\\n\" > f\n", []want{{false, `"${x:-"}"}X\n"`}}, "", ""},
		{"an escaped \" inside a double quote", "printf \"a\\\"b\\n\" > f\n", []want{{false, `"a\"b\n"`}}, "", ""},
		{"a ' inside a double quote", "printf \"it's\\n\" > f\n", []want{{false, `"it's\n"`}}, "", ""},
		{"a backtick substitution", "x=`pwd`\nprintf 'X\\n' > f\n", []want{{false, `'X\n'`}}, "", ""},
		{"an unclosed \" ending in a backslash", "echo \"x\\", nil, "a \" opened at line 1 never closes", ""},
		// Redirect targets.
		{"a single-quoted target", "printf 'X\\n' > 'a b'\n", []want{{false, `'X\n'`}}, "", ""},
		{"a double-quoted target", "printf 'X\\n' > \"a b\"\n", []want{{false, `'X\n'`}}, "", ""},
		{"a backtick target", "printf 'X\\n' > `echo a b`\n", []want{{false, `'X\n'`}}, "", ""},
		{"a $(…) target", "printf 'X\\n' > $(echo a b)\n", []want{{false, `'X\n'`}}, "", ""},
		{"a ${…} target", "printf 'X\\n' > ${x:-a b}\n", []want{{false, `'X\n'`}}, "", ""},
		{"an escaped space in a target", "printf 'X\\n' > a\\ b\n", []want{{false, `'X\n'`}}, "", ""},
		{"a space before a heredoc's tag", "cat > f << 'EOF'\nX\nEOF\n", []want{{true, "X"}}, "", ""},
		{"a tab before a heredoc's tag", "cat > f <<\t'EOF'\nX\nEOF\n", []want{{true, "X"}}, "", ""},
		// Own redirects and the target named.
		{"1>&2 inside a redirected group", "{ printf 'X\\n' 1>&2; printf 'Y\\n'; } > f\n", []want{{false, `'Y\n'`}}, "", ""},
		{"an own >&2, then a word", "{ printf \"$m\" >&2 x; printf 'Y\\n'; } > f\n", []want{{false, `'Y\n'`}}, "", ""},
		{"an input redirect before printf, in a redirected group", "{ < in printf 'X\\n'; } > f\n", []want{{false, `'X\n'`}}, "", ""},
		{"a } argument inside a group", "{ printf '%s\\n' } > g; printf 'Y\\n'; } > f\n", []want{{false, `'%s\n' }`}, {false, `'Y\n'`}}, "", ""},
		{"an unquoted format", "printf X > f\n", nil, "not a literal string", ""},
		{"the first of two targets", "printf \"$m\" > f > g\n", nil, "writes to f with", ""},
		{"the first of two targets before the command", "> f > g printf \"$m\"\n", nil, "writes to f with", ""},
		// What it skips, by design.
		{"a printf to stderr by fd", "printf 'X\\n' 2> f\n", nil, "", ""},
		{"an unquoted heredoc", "cat > f <<EOF\n$X\nEOF\n", nil, "", ""},
		{"a printf into a pipe", "printf 'X\\n' | sed s/a/b/ > f\n", nil, "", ""},
		{"a printf captured by $(…)", "y=$(printf 'X\\n')\n", nil, "", ""},
		{"a printf to /dev/null", "printf 'X\\n' > /dev/null\n", nil, "", ""},
		{"a printf to stderr", "printf 'X\\n' >&2\n", nil, "", ""},
		// What it refuses.
		{"an unclosed '", "echo 'x\n", nil, "a ' opened at line 1 never closes", ""},
		{"an unclosed \"", "echo \"x\n", nil, "a \" opened at line 1 never closes", ""},
		{"an unclosed `", "echo `x\n", nil, "a ` opened at line 1 never closes", ""},
		{"an unclosed $(", "y=$(echo a\n", nil, "a $( never closes", ""},
		{"an unclosed ${", "y=${a\n", nil, "a ${ opened at line 1 never closes", ""},
		{"an unclosed $((", "y=$((1 + 2\n", nil, "a $(( opened at line 1 never closes", ""},
		{"an unclosed heredoc", "cat > f <<'NEVER'\nx\n", nil, "the heredoc opened at line 1", ""},
		{"a format that is only an expansion", "printf \"$fmt\" > f\n", nil, "not a literal string", ""},
		{"a redirect before the format", "printf > f 'x'\n", []want{{false, "'x'"}}, "", ""},
		{"a printf with no format", "printf > f\n", nil, "not a literal string", ""},
		{"&> is refused, and read as the > it also is", "printf 'X\\n' &> f\n", []want{{false, `'X\n'`}}, "&> (bash)", ""},
		{"exec > f", "exec > f\nprintf 'X\\n'\n", nil, "exec sends the rest of the output", ""},
		{"a ) that closes nothing", "echo a )\n", nil, "a ) that closes nothing", ""},
	}
	for _, c := range cases {
		got, errs := writtenScripts(c.src)
		var gw []want
		for _, g := range got {
			gw = append(gw, want{g.heredoc, g.text})
		}
		if fmt.Sprint(gw) != fmt.Sprint(c.want) {
			t.Errorf("%s: read %+v, want %+v", c.name, gw, c.want)
		}
		switch {
		case c.err == "" && len(errs) > 0:
			t.Errorf("%s: unexpected error(s): %q", c.name, errs)
		case c.err != "" && (len(errs) != 1 || !strings.Contains(errs[0], c.err)):
			t.Errorf("%s: errors %q, want one holding %q", c.name, errs, c.err)
		}
		if c.open2 != "" && (len(got) < 2 || got[1].open != c.open2) {
			t.Errorf("%s: the second script opens %q, want %q", c.name, func() string {
				if len(got) < 2 {
					return ""
				}
				return got[1].open
			}(), c.open2)
		}
	}
	// A script's opening line is the line its command word starts on, even when the word goes on
	// past a continued line.
	if got, _ := writtenScripts("p\\\nrintf 'X\\n' > f\n"); len(got) != 1 || got[0].open != `p\` {
		t.Errorf("a command word split by a continued line: read %+v, want one script opening at line 1", got)
	}
}
