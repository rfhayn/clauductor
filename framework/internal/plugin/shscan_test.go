package plugin

import (
	"fmt"
	"regexp"
	"strings"
)

var onlyExpansion = regexp.MustCompile(`^"\$(?:\{[^}]*\}|[A-Za-z_][A-Za-z0-9_]*|[0-9@*])"$`)

// writtenScript is one script a check writes: a quoted heredoc's body, or a printf format whose
// output goes to a file; open is the line that starts it.
type writtenScript struct {
	open, text string
	heredoc    bool
}

// writtenScripts reads the scripts a shell file writes, in order (heredocs, then printfs), so a
// template check's and its plugin copy's pair one to one, and returns what it could not read.
//
// It reads the shell as the shell does, not line by line: quotes, $(…), backticks and ${…} are
// consumed whole, so a `<<'T'` or a `printf … >` inside a string (a payload a check hands a hook,
// an awk program) is text, never an opener. What it reads:
//   - a heredoc whose tag is quoted (`<<'T'`, `<<"T"`, `<<\T`, with or without `-`), wherever it
//     sits in the command (`cat <<'T' > f`, `fixture NAME <<'T'`, a comment after it). Its body
//     runs to the tag line, past leading tabs only for `<<-`. An unquoted tag (`<<T`) is skipped
//     by design: its body expands, so it is the check's own text, as a double-quoted string is.
//   - a printf whose output reaches a file: its own redirect (`printf -- '…' > f`, `>>`), or the
//     redirect of a group, loop, if or case around it (`{ printf …; cat g; } > f`). A printf into a
//     pipe, a $(…) or the terminal is not read: its text is the check's own output. Nor is one in a
//     function body whose callers redirect it (`f() { printf …; }; f > g`): the reader does not
//     follow calls. No check writes a script that way today.
//
// It fails closed: a quote, $( or heredoc that never closes, or a printf whose output reaches a file
// with a format that is not a literal string (or is only an expansion, "$fmt"), is an error, never
// a skip.
func writtenScripts(s string) ([]writtenScript, []string) {
	sc := &shScanner{s: s, line: 1, lines: strings.Split(s, "\n")}
	sc.code(0)
	prints, errs := printfScripts(sc.toks, sc.lines)
	return append(sc.docs, prints...), append(sc.errs, errs...)
}

// shTok is one token at the top level: a word, an operator (`;` `&&` `||` `|` `&` a newline `{`
// `}` `(` `)`), or a redirect to (or from) a target.
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
	toks  []shTok
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

// code reads shell code until EOF, or (depth > 0) the `)` that closes the $( it was called for.
// Only the top level (depth 0) emits tokens: what a $(…) prints is captured, not written.
func (sc *shScanner) code(depth int) {
	var word strings.Builder
	wordLine, paren, cases := 0, 0, 0
	emit := func(kind byte, text string, line int) {
		if depth == 0 {
			sc.toks = append(sc.toks, shTok{kind: kind, text: text, line: line})
		}
	}
	flush := func() {
		if word.Len() == 0 {
			return
		}
		w := word.String()
		word.Reset()
		switch w {
		case "case":
			cases++
		case "esac":
			cases--
		}
		if w == "{" || w == "}" {
			emit('o', w, wordLine)
		} else {
			emit('w', w, wordLine)
		}
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
			emit('o', "\n", sc.line)
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
		case c == '\'' || c == '"' || c == '`' || (c == '$' && (sc.peek(1) == '(' || sc.peek(1) == '{')):
			start()
			from := sc.i
			sc.quoted(depth)
			word.WriteString(sc.s[from:min(sc.i, len(sc.s))])
		case c == '<' && sc.peek(1) == '<' && sc.peek(2) == '<': // a here-string: its word follows
			flush()
			sc.i += 3
		case c == '<' && sc.peek(1) == '<':
			flush()
			sc.i += 2
			sc.heredocOpener()
		case c == '<' || c == '>':
			fd := ""
			if w := word.String(); w != "" && strings.Trim(w, "0123456789") == "" {
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
			if depth == 0 {
				sc.toks = append(sc.toks, t)
			}
		case c == ';' || c == '&' || c == '|':
			flush()
			op := string(c)
			if sc.peek(1) == c {
				op += string(c)
				sc.i++
			}
			sc.i++
			emit('o', op, sc.line)
		case c == '(' && word.Len() == 0:
			paren++
			sc.i++
			emit('o', "(", sc.line)
		case c == ')':
			flush()
			sc.i++
			switch {
			case paren > 0:
				paren--
				emit('o', ")", sc.line)
			case cases > 0: // a case pattern's close
				emit('o', ";", sc.line)
			case depth > 0:
				return
			default:
				emit('o', ";", sc.line)
			}
		default:
			start()
			word.WriteByte(c)
			sc.i++
		}
	}
	flush()
	if depth > 0 {
		sc.errf("a $( opened inside line %d never closes", sc.line)
	}
}

// quoted consumes one quoted or substituted span at sc.i: '…', "…", `…`, $(…) or ${…}.
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
			case sc.s[sc.i] == '`' || (sc.s[sc.i] == '$' && (sc.peek(1) == '(' || sc.peek(1) == '{')):
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
	case sc.peek(1) == '(':
		sc.i += 2
		sc.code(depth + 1)
	default: // ${…}
		sc.i += 2
		for n := 1; sc.i < len(sc.s) && n > 0; {
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
	}
}

// newline counts a newline at sc.i inside a quoted span (no heredoc starts there).
func (sc *shScanner) newline() {
	if sc.i < len(sc.s) && sc.s[sc.i] == '\n' {
		sc.line++
	}
}

// word consumes one word at sc.i (a redirect's target), quotes included, and returns it raw.
func (sc *shScanner) word(depth int) string {
	from := sc.i
	for sc.i < len(sc.s) {
		c := sc.s[sc.i]
		if strings.IndexByte(" \t\r\n;&|<>()", c) >= 0 {
			break
		}
		switch {
		case c == '\'' || c == '"' || c == '`' || (c == '$' && (sc.peek(1) == '(' || sc.peek(1) == '{')):
			sc.quoted(depth)
		case c == '\\':
			sc.i += 2
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
		for sc.i <= len(sc.s) {
			eol := strings.IndexByte(sc.s[sc.i:], '\n')
			if eol < 0 {
				eol = len(sc.s) - sc.i
			}
			l := sc.s[sc.i : sc.i+eol]
			if p.dash {
				l = strings.TrimLeft(l, "\t")
			}
			if l == p.tag {
				if p.quoted {
					sc.docs = append(sc.docs, writtenScript{open: sc.lines[p.line-1], text: strings.TrimSuffix(sc.s[from:sc.i], "\n"), heredoc: true})
				}
				sc.i += eol + 1
				sc.line++
				closed = true
				break
			}
			if sc.i+eol >= len(sc.s) {
				sc.i = len(sc.s)
				break
			}
			sc.i += eol + 1
			sc.line++
		}
		if !closed {
			sc.errf("the heredoc opened at line %d (%s) never closes", p.line, strings.TrimSpace(sc.lines[p.line-1]))
		}
	}
	sc.pend = nil
}

// printfScripts finds the printfs whose output reaches a file. A frame is a group, loop, if or
// case: a printf with no redirect of its own writes where its frame's output goes.
func printfScripts(toks []shTok, lines []string) ([]writtenScript, []string) {
	var out []writtenScript
	var errs []string
	type pending struct {
		fmt  string
		line int
	}
	var frames [][]pending
	closes := map[string]bool{"}": true, ")": true, "done": true, "fi": true, "esac": true}
	opens := map[string]bool{"{": true, "(": true, "do": true, "if": true, "case": true}
	resolve := func(ps []pending, to string) {
		for _, p := range ps {
			// A format that is only an expansion ("$fmt") holds text set elsewhere, unread here.
			if p.fmt == "" || (p.fmt[0] != '\'' && p.fmt[0] != '"') || onlyExpansion.MatchString(p.fmt) {
				errs = append(errs, fmt.Sprintf("line %d (%s): a printf writes to %s with a format that is not a literal string: read it, or say why not", p.line, strings.TrimSpace(lines[p.line-1]), to))
				continue
			}
			out = append(out, writtenScript{open: lines[p.line-1], text: p.fmt})
		}
	}
	// redirect reads the command from toks[k] to its end: the file its output goes to, or piped.
	redirect := func(k int) (to string, piped bool) {
		for ; k < len(toks); k++ {
			t := toks[k]
			if t.kind == 'o' {
				if t.text == "|" {
					return to, true
				}
				if t.text != "(" && t.text != "{" {
					return to, false
				}
			}
			if t.kind == 'w' && closes[t.text] {
				return to, false
			}
			if t.kind == 'r' && t.output && !t.dup && (t.fd == "" || t.fd == "1") && to == "" && !strings.Contains(t.text, "/dev/null") {
				to = t.text
			}
		}
		return to, false
	}
	cmd := true // at a command's first word
	for k, t := range toks {
		switch {
		case t.kind == 'o' && (t.text == "{" || t.text == "("), t.kind == 'w' && cmd && opens[t.text]:
			frames = append(frames, nil)
			cmd = true
		case (t.kind == 'o' && (t.text == "}" || t.text == ")")) || (t.kind == 'w' && cmd && closes[t.text]):
			if len(frames) == 0 {
				cmd = false
				continue
			}
			ps := frames[len(frames)-1]
			frames = frames[:len(frames)-1]
			if to, piped := redirect(k + 1); to != "" {
				resolve(ps, to)
			} else if !piped && len(frames) > 0 {
				frames[len(frames)-1] = append(frames[len(frames)-1], ps...)
			}
			cmd = false
		case t.kind == 'o':
			cmd = true
		case t.kind == 'w' && cmd && (t.text == "then" || t.text == "else" || t.text == "elif" || t.text == "!" || t.text == "in"):
			cmd = true
		case t.kind == 'w' && cmd && strings.Contains(t.text, "=") && !strings.HasPrefix(t.text, "="):
			// an assignment before the command word
		case t.kind == 'w' && cmd && t.text == "printf":
			p := pending{line: t.line}
			j := k + 1
			if j < len(toks) && toks[j].kind == 'w' && toks[j].text == "--" {
				j++
			}
			if j < len(toks) && toks[j].kind == 'w' {
				p.fmt = toks[j].text
			}
			to, piped := redirect(k + 1)
			switch {
			case to != "":
				resolve([]pending{p}, to)
			case !piped && len(frames) > 0:
				frames[len(frames)-1] = append(frames[len(frames)-1], p)
			}
			cmd = false
		case t.kind == 'w':
			cmd = false
		}
	}
	return out, errs
}
