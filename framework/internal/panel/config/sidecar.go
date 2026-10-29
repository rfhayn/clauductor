package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// The sidecar rule (PANEL-42, design.md D5). A config outside its repository is a file
// nobody reviewed with the repository, so its commands may run only the repository's
// own scripts and programs on PATH, as argv with literal words. The rule judges argv
// words only, and parses no script: every command is exec'd with no shell, so what is
// checked is exactly what runs. It is not a sandbox: what a script of the repository's
// does is the repository's own content.

// SidecarRemedy ends every refusal: what the owner does instead.
const SidecarRemedy = "move it into a script in the repository"

// SidecarWord is one word of a command that a config outside its repository may not
// hold, and why.
type SidecarWord struct {
	Word string `json:"word"`
	Why  string `json:"why"`
}

// SidecarLine is one RunList line whose command holds words the rule refuses.
type SidecarLine struct {
	Line  string        `json:"line"`
	Words []SidecarWord `json:"words"`
}

// Marked is the line as the trust report prints it: NOT ALLOWED IN A SIDECAR (<word>:
// <why>; move it into a script in the repository): <line>. Each word is quoted, as
// RunList quotes argv, so a word holding a space or a newline reads as one word.
func (l SidecarLine) Marked() string {
	var b strings.Builder
	b.WriteString("NOT ALLOWED IN A SIDECAR (")
	for _, w := range l.Words {
		fmt.Fprintf(&b, "%q: %s; ", w.Word, w.Why)
	}
	b.WriteString(SidecarRemedy + "): " + l.Line)
	return b.String()
}

// SidecarRefusals lists every command of the config, with every word in it, that a
// config outside the repository at root may not hold. The commands judged are cards,
// queues, suggest, metrics, worktree_setup and worktree_teardown; a first prompt and
// the resume line are text typed into Claude, and are not.
func (c *Config) SidecarRefusals(root string) []SidecarLine {
	var out []SidecarLine
	for _, e := range c.runEntries() {
		if e.command == nil {
			continue
		}
		if ws := SidecarCheck(root, e.command); len(ws) > 0 {
			out = append(out, SidecarLine{Line: e.line, Words: ws})
		}
	}
	return out
}

// SidecarRunList is RunList with each refused line marked: the trust report of a
// config outside the repository at root.
func (c *Config) SidecarRunList(root string) []string {
	marked := map[string]string{}
	for _, l := range c.SidecarRefusals(root) {
		marked[l.Line] = l.Marked()
	}
	var out []string
	for _, line := range c.RunList() {
		if m, ok := marked[line]; ok {
			line = m
		}
		out = append(out, line)
	}
	return out
}

// SidecarCheck lists the words of argv that a config outside the repository at root
// may not hold, in argv order, one reason per word (the first rule that refuses it).
func SidecarCheck(root string, argv []string) []SidecarWord {
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	root = ResolveExisting(root)
	why := map[int]string{}
	refuse := func(i int, format string, a ...any) {
		if _, ok := why[i]; !ok && i < len(argv) {
			why[i] = fmt.Sprintf(format, a...)
		}
	}
	for i, w := range argv {
		if c := shellChar(w); c != "" {
			refuse(i, "%s: with no shell it reaches the program as it is", c)
		}
	}
	codeFlags(argv, refuse)
	for i, w := range argv {
		if p := outsidePiece(root, w); p != "" {
			refuse(i, "%s resolves outside the repository", p)
		}
	}
	programPosition(root, argv, refuse)
	envAnywhere(argv, refuse)

	idx := make([]int, 0, len(why))
	for i := range why {
		idx = append(idx, i)
	}
	sort.Ints(idx)
	out := make([]SidecarWord, 0, len(idx))
	for _, i := range idx {
		out = append(out, SidecarWord{Word: argv[i], Why: why[i]})
	}
	return out
}

// shellChar names the first character in w that a literal word never needs: with no
// shell to read it, a glob, a variable, a quote, a redirection or whitespace reaches
// the program as it is, so it is a mistake or a trick.
func shellChar(w string) string {
	for _, r := range w {
		switch r {
		case ' ':
			return "a space"
		case '\t':
			return "a tab"
		case '\n', '\r':
			return "a newline"
		case '`':
			return "a backtick"
		case '*', '?', '[', ']', '{', '}', '~', '$', '(', ')', '<', '>', '|', '&', ';', '\'', '"', '\\', '!':
			return "the character `" + string(r) + "`"
		}
	}
	return ""
}

// interpreters maps each interpreter's exact name to its kind. Names are matched
// exactly, never on a prefix (`pythonic` is not Python), with case folded
// (programName).
var interpreters = map[string]string{
	"sh": "shell", "bash": "shell", "zsh": "shell", "dash": "shell", "ksh": "shell", "fish": "fish",
	"python": "python", "python3": "python",
	"node": "node", "deno": "deno", "bun": "bun",
	"perl": "perl", "ruby": "ruby", "php": "php", "osascript": "osascript",
}

var python3Minor = regexp.MustCompile(`^python3\.[0-9]+$`)

// programName is a name in program position as the tables hold it: case folded,
// because macOS's case-insensitive filesystem execs `BASH` as bash and `NPX` as npx.
// Folding refuses more than an exact match, never less.
func programName(w string) string { return strings.ToLower(w) }

func interpreterKind(w string) string {
	w = programName(w)
	if k, ok := interpreters[w]; ok {
		return k
	}
	if python3Minor.MatchString(w) {
		return "python"
	}
	return ""
}

// codeFlag is an interpreter's flags whose argument is code, not a path: single
// letters (matched in a single-dash cluster too: `-lc`, `-Bc`, `-mhttp.server`) and
// long flags (matched alone or as `--flag=…`). abbrev: the interpreter's parser takes
// any unambiguous prefix of a long flag (getopt_long), so a prefix is matched too; an
// ambiguous one is refused as well, which costs nothing, since it would not run.
type codeFlag struct {
	letters string
	long    []string
	abbrev  bool
}

var codeFlagsOf = map[string]codeFlag{
	"shell": {letters: "c"},
	// --command is fish's long form of -c; -C (--init-command) runs code before the
	// script, so a script file would not make it safe (the owner, 2026-10-17). fish
	// abbreviates long flags; probed, bash, zsh, python3, perl, ruby, node and bun do
	// not (they refuse a `--vers`-style prefix, or read the code as a file). deno was
	// not probed; php and osascript list no long code flag, so a prefix has nothing to
	// match (PANEL-47).
	"fish":      {letters: "cC", long: []string{"--command", "--init-command"}, abbrev: true},
	"python":    {letters: "cm"},
	"node":      {letters: "epr", long: []string{"--eval", "--print", "--import", "--require", "--loader", "--experimental-loader"}},
	"deno":      {letters: "ep", long: []string{"--eval", "--print"}},
	"bun":       {letters: "ep", long: []string{"--eval", "--print"}},
	"perl":      {letters: "eE"},
	"ruby":      {letters: "e"},
	"php":       {letters: "r"},
	"osascript": {letters: "e"},
}

// codeFlags refuses an interpreter's code flag anywhere after the interpreter's name,
// at any position: a launcher in front of it (env, sudo, caffeinate) needs no list.
func codeFlags(argv []string, refuse func(int, string, ...any)) {
	for i, w := range argv {
		kind := interpreterKind(w)
		if kind == "" {
			continue
		}
		f := codeFlagsOf[kind]
		for j := i + 1; j < len(argv); j++ {
			if f.matches(argv[j]) {
				refuse(j, "`%s %s` runs code given in the command, not a script file in the repository", w, argv[j])
			}
		}
	}
}

func (f codeFlag) matches(w string) bool {
	for _, l := range f.long {
		if w == l || strings.HasPrefix(w, l+"=") {
			return true
		}
		// `--` and at least one letter of the name: `--` alone ends the options.
		if name, _, _ := strings.Cut(w, "="); f.abbrev && len(name) > 2 && strings.HasPrefix(l, name) {
			return true
		}
	}
	if len(w) < 2 || w[0] != '-' || w[1] == '-' {
		return false
	}
	// A single-dash cluster: the letters up to the first character that is not one
	// (a value attached to the cluster's last flag is not judged as letters).
	for _, r := range w[1:] {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') {
			break
		}
		if strings.ContainsRune(f.letters, r) {
			return true
		}
	}
	return false
}

// outsidePiece returns the first piece of w that resolves outside root, or "". A word
// holding `/` or `..` is split at every `=`, `:` and `,`. A piece starting with a
// single `-` is judged from its first `/` or `.` (so a flag cluster of any length is
// caught: `-sC..`, `-isystem/x`), a leading `@` is stripped, and a piece that is then
// absolute, holds a `/`, or is or has a `..` segment must resolve inside the root,
// joined to it and resolved through symlinks as far as it exists. There is no flag
// list: every flag's value is judged the same way.
func outsidePiece(root, w string) string {
	if !strings.Contains(w, "/") && !strings.Contains(w, "..") {
		return ""
	}
	for _, piece := range strings.FieldsFunc(w, func(r rune) bool { return r == '=' || r == ':' || r == ',' }) {
		p := piece
		if strings.HasPrefix(p, "-") && !strings.HasPrefix(p, "--") {
			i := strings.IndexAny(p, "/.")
			if i < 0 {
				continue
			}
			p = p[i:]
		}
		p = strings.TrimPrefix(p, "@")
		if !filepath.IsAbs(p) && !strings.Contains(p, "/") && !hasDotDot(p) {
			continue
		}
		if !insideRoot(root, p) {
			return p
		}
	}
	return ""
}

func hasDotDot(p string) bool {
	for _, s := range strings.Split(p, "/") {
		if s == ".." {
			return true
		}
	}
	return false
}

// insideRoot: p, joined to root when relative, resolves inside it.
func insideRoot(root, p string) bool {
	return Within(ResolveExisting(underRoot(root, p)), root)
}

// underRoot is p joined to root when relative, NOT cleaned: filepath.Join would apply
// a `..` lexically, before ResolveExisting follows the symlink under it.
func underRoot(root, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return root + string(filepath.Separator) + p
}

// launcherValueFlags are each launcher's own flags that take the next word as their
// value. Any other flag of a launcher is refused: the program cannot be found past it.
var launcherValueFlags = map[string][]string{
	"env":     {"-u", "-C", "-P"},
	"nice":    {"-n"},
	"nohup":   nil,
	"time":    {"-o", "-f"},
	"timeout": {"-s", "-k"},
}

// fetchSubcommands are each program's subcommands that fetch a package and run it,
// each matched as any later word. A two-word entry (`tool run`) matches its first word
// with its second anywhere after it.
var fetchSubcommands = map[string][]string{
	"pipx": {"run"},
	"npm":  {"exec", "x", "create"},
	"pnpm": {"dlx", "create"},
	"yarn": {"dlx", "create"},
	"bun":  {"x", "create"},
	"uv":   {"tool run"}, // uv's long form of uvx
}

// assignment: a word that sets the environment (design.md D5, "No environment is
// set"). env takes any word holding `=` as NAME=value, and with no shell a word in
// program position holding one is no program, so any `=` counts. Many variables load
// code into whatever runs next (NODE_OPTIONS, BASH_ENV, LD_PRELOAD), so none is allowed.
func assignment(w string) bool { return strings.Contains(w, "=") }

const assignmentWhy = "`%s` sets the environment, which can load code into the program it runs"

// programPosition applies the rules judged only in program position: argv[0], or the
// first word after a launcher's own words (its flags and their values, timeout's
// duration), through chained launchers. A NAME=value word there, or among env's words,
// is refused and passed over, so the program after it is still judged. Every name is
// compared case-folded (programName).
func programPosition(root string, argv []string, refuse func(int, string, ...any)) {
	p := 0
	for p < len(argv) {
		if assignment(argv[p]) {
			refuse(p, assignmentWhy, argv[p])
			p++
			continue
		}
		l := programName(argv[p])
		if _, ok := launcherValueFlags[l]; !ok {
			break
		}
		j, ok := launcherWords(l, argv, p+1, refuse)
		if !ok {
			return
		}
		p = j
	}
	if p >= len(argv) {
		return
	}
	prog, rest := argv[p], argv[p+1:]
	at := func(k int) int { return p + 1 + k }
	name := programName(prog)
	switch name {
	case ".", "eval", "source":
		refuse(p, "`%s` runs text as shell code", prog)
		return
	case "awk", "gawk", "mawk", "nawk", "sed":
		if !programFromFile(rest) {
			refuse(p, "`%s` without `-f <file>` runs a program given in the command", prog)
		}
		return
	case "xargs":
		refuse(p, "`xargs` builds commands from its input, which the check never sees")
		return
	case "find":
		for k, w := range rest {
			switch w {
			case "-exec", "-execdir", "-ok", "-okdir":
				refuse(at(k), "`find %s` builds commands from what it finds, which the check never sees", w)
			}
		}
		return
	case "npx", "pnpx", "bunx", "uvx": // pnpx: pnpm's alias of `pnpm dlx`
		refuse(p, "`%s` fetches a package and runs it", prog)
		return
	case "go":
		if contains(rest, "run") {
			for k, w := range rest {
				if s := strings.Index(w, "/"); s >= 0 && strings.Contains(w[s:], "@") {
					refuse(at(k), "`go run module@version` fetches a module and runs it")
				}
			}
		}
		return
	}
	for k, w := range rest {
		for _, sub := range fetchSubcommands[name] {
			first, second, two := strings.Cut(sub, " ")
			if w == first && (!two || contains(rest[k+1:], second)) {
				refuse(at(k), "`%s %s` fetches a package and runs it", prog, sub)
			}
		}
		// `npm init` alone writes a package.json; `npm init <package>` runs
		// create-<package>. `innit` is npm's alias of init.
		if name == "npm" && (w == "init" || w == "innit") && hasOperand(rest[k+1:]) {
			refuse(at(k), "`npm %s <package>` fetches a package and runs it", w)
		}
	}
	switch interpreterKind(prog) {
	case "", "osascript":
		return
	}
	k := firstOperand(rest)
	if k < 0 {
		refuse(p, "`%s` with no script file runs code that is not a file in the repository", prog)
		return
	}
	if name == "bun" && rest[k] == "run" && k+1 < len(rest) && bareWord(rest[k+1]) {
		return // a package.json script, as `npm run` runs it
	}
	script := rest[k]
	// Uncleaned, so os.Stat walks it as the kernel does when the shell opens it.
	full := underRoot(root, script)
	if !Within(ResolveExisting(full), root) {
		refuse(at(k), "the script file %s resolves outside the repository", script)
		return
	}
	if fi, err := os.Stat(full); err != nil || !fi.Mode().IsRegular() {
		refuse(at(k), "`%s` runs %s, which is not a script file in the repository", prog, script)
	}
}

// launcherWords walks launcher l's own words from argv[j]: its listed flags and their
// values, env's NAME=value words (refused, and passed over) and timeout's duration. It
// returns the index of the first word past them, or false when it refused a word the
// walk cannot see past (`env -S`, a flag not listed).
func launcherWords(l string, argv []string, j int, refuse func(int, string, ...any)) (int, bool) {
	vals, duration := launcherValueFlags[l], l == "timeout"
	for j < len(argv) {
		w := argv[j]
		switch {
		case l == "env" && (strings.HasPrefix(w, "-S") || w == "--split-string" || strings.HasPrefix(w, "--split-string=")):
			refuse(j, "`env -S` splits one word into a command line")
			return j, false
		case strings.HasPrefix(w, "-"):
			if !contains(vals, w) {
				refuse(j, "`%s %s` is not a launcher flag the check knows, so the program after it cannot be found", l, w)
				return j, false
			}
			j += 2
		case l == "env" && assignment(w):
			refuse(j, assignmentWhy, w)
			j++
		case duration:
			duration = false
			j++
		default:
			return j, true
		}
	}
	return j, true
}

// envAnywhere applies env's own-word rules to every `env` in argv, wherever it stands
// (design.md D5: an env is judged wherever it stands, as code flags are), so a launcher
// on no list in front of it (`caffeinate env FOO=1 make`) hides none of its NAME=value
// words. An env flag D5 does not list is refused there too: past it the scan cannot
// tell env's words from its program's.
func envAnywhere(argv []string, refuse func(int, string, ...any)) {
	for i, w := range argv {
		if programName(w) == "env" {
			launcherWords("env", argv, i+1, refuse)
		}
	}
}

// programFromFile: an awk or sed program read from a file (`-f <file>`), with no
// program given in the command beside it (`-e`, `--expression`, gawk's `--source`).
func programFromFile(rest []string) bool {
	file := false
	for k, w := range rest {
		if w == "-f" && k+1 < len(rest) {
			file = true
		}
		if w == "--expression" || strings.HasPrefix(w, "--expression=") || w == "--source" || strings.HasPrefix(w, "--source=") ||
			(codeFlag{letters: "e"}).matches(w) {
			return false
		}
	}
	return file
}

// firstOperand is the index of the first word that is not a flag, or -1.
func firstOperand(rest []string) int {
	for k, w := range rest {
		if !strings.HasPrefix(w, "-") {
			return k
		}
	}
	return -1
}

func hasOperand(rest []string) bool { return firstOperand(rest) >= 0 }

// bareWord is a name with no path in it: a package.json script's.
func bareWord(w string) bool {
	return w != "" && !strings.HasPrefix(w, "-") && !strings.Contains(w, "/") && !strings.Contains(w, "..")
}

func contains(list []string, w string) bool {
	for _, x := range list {
		if x == w {
			return true
		}
	}
	return false
}
