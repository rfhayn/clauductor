package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sidecarRoot is a repository's root holding the scripts the rows name, a directory
// `sub`, a symlink that stays inside and one that points out, and three directory
// symlinks a later `..` goes back up through: `outdir` to <outside>/deep, `srclink` to
// src/x (relative) and `inlink` to .claude. A `payload.sh` is both in the root and
// outside it, so a path judged lexically finds the wrong one. It returns the root and
// the folder outside it.
func sidecarRoot(t *testing.T) (root, outside string) {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root, outside = filepath.Join(base, "repo"), filepath.Join(base, "other")
	for _, f := range []string{
		".claude/x.sh", ".claude/owner-queue.sh", ".claude/roadmap-queue.sh", ".claude/panel-suggest.sh",
		"scripts/ci/run-local.sh", "infra/roadmap-queue.mjs", "src/x/a.h", "../other/gate.sh",
		"payload.sh", "../other/payload.sh", "../other/deep/x.sh",
	} {
		p := filepath.Join(root, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		// Not executable: the rule judges a script's place, not its mode.
		if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "gate.sh"), filepath.Join(root, "scripts", "link.sh")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, ".claude", "x.sh"), filepath.Join(root, "scripts", "in.sh")); err != nil {
		t.Fatal(err)
	}
	for link, target := range map[string]string{
		"outdir": filepath.Join(outside, "deep"), "srclink": filepath.Join("src", "x"), "inlink": filepath.Join(root, ".claude"),
	} {
		if err := os.Symlink(target, filepath.Join(root, link)); err != nil {
			t.Fatal(err)
		}
	}
	return root, outside
}

// sidecarRow is one command: allowed (word == ""), or refused with word marked and
// a reason holding why. only: no other word may be refused, so the row proves that
// one rule.
type sidecarRow struct {
	argv []string
	word string
	why  string
	only bool
}

func checkRows(t *testing.T, root string, rows []sidecarRow) {
	t.Helper()
	for _, r := range rows {
		got := SidecarCheck(root, r.argv)
		if r.word == "" {
			if len(got) != 0 {
				t.Errorf("%q refused: %+v", r.argv, got)
			}
			continue
		}
		found := false
		for _, w := range got {
			if w.Word == r.word && strings.Contains(w.Why, r.why) {
				found = true
			}
		}
		if !found || (r.only && len(got) != 1) {
			t.Errorf("%q: want %q refused for %q, got %+v", r.argv, r.word, r.why, got)
		}
	}
}

const codeWhy = "runs code given in the command"

// ATTACH-3's requirement, the sidecar command rule (design.md D5): an interpreter's
// code flag is refused wherever it stands, at argv[0], behind a launcher on no list
// (caffeinate) and behind env; the interpreter's name is matched exactly.
func TestSidecarCodeFlags(t *testing.T) {
	t.Parallel()
	root, _ := sidecarRoot(t)
	var rows []sidecarRow
	for _, c := range [][]string{
		{"sh", "-c"}, {"bash", "-c"}, {"bash", "-lc"}, {"zsh", "-c"}, {"dash", "-c"}, {"ksh", "-c"}, {"fish", "-c"},
		// fish's long form of -c (an alias of a named form; tasks.md's decision log).
		{"fish", "--command"}, {"fish", "--command=make"},
		// fish's -C runs code before the script, as -c does (the owner, 2026-10-17; PANEL-47).
		{"fish", "-C"}, {"fish", "-lC"}, {"fish", "--init-command"}, {"fish", "--init-command=make"},
		// fish's getopt takes any unambiguous prefix of a long flag (PANEL-47's review).
		{"fish", "--init=make"}, {"fish", "--ini"}, {"fish", "--comm=make"}, {"fish", "--c"},
		{"python", "-c"}, {"python3", "-c"}, {"python3", "-m"}, {"python3.12", "-c"}, {"python3", "-Bc"}, {"python3", "-mhttp.server"},
		{"node", "-e"}, {"node", "-p"}, {"node", "--eval"}, {"node", "--print=1"}, {"node", "--import"}, {"node", "-r"},
		{"node", "--require"}, {"node", "--loader"}, {"node", "--experimental-loader"},
		{"deno", "-e"}, {"deno", "--eval"}, {"deno", "-p"}, {"deno", "--print"},
		{"bun", "-e"}, {"bun", "--eval"}, {"bun", "-p"}, {"bun", "--print"},
		{"perl", "-e"}, {"perl", "-E"}, {"ruby", "-e"}, {"php", "-r"}, {"osascript", "-e"},
	} {
		base := []string{c[0], c[1], "x"}
		for _, argv := range [][]string{base, append([]string{"caffeinate", "-i"}, base...), append([]string{"env"}, base...)} {
			rows = append(rows, sidecarRow{argv: argv, word: c[1], why: codeWhy})
		}
	}
	rows = append(rows,
		// After the script too: an accepted false refusal (design.md D5, the cost).
		sidecarRow{argv: []string{"python3", ".claude/x.sh", "-c", "cfg.yml"}, word: "-c", why: codeWhy, only: true},
		// fish's other long flags, and `--` ending its options, are not code flags.
		sidecarRow{argv: []string{"fish", "--private", "--", ".claude/x.sh"}},
		// An exact name only.
		sidecarRow{argv: []string{"pythonic", "-c", "x"}},
		sidecarRow{argv: []string{"shx", "-c", "x"}},
	)
	checkRows(t, root, rows)
}

// ATTACH-3's requirement, the sidecar command rule: the program-position rules, at
// argv[0] and behind launchers' own words, through chained launchers; each row's own
// reason.
func TestSidecarProgramPosition(t *testing.T) {
	t.Parallel()
	root, _ := sidecarRoot(t)
	checkRows(t, root, []sidecarRow{
		{argv: []string{".", ".claude/x.sh"}, word: ".", why: "runs text as shell code", only: true},
		{argv: []string{"eval", "make"}, word: "eval", why: "runs text as shell code", only: true},
		{argv: []string{"source", ".claude/x.sh"}, word: "source", why: "runs text as shell code", only: true},
		{argv: []string{"env", "-S", "make"}, word: "-S", why: "`env -S`", only: true},
		{argv: []string{"env", "--split-string=make"}, word: "--split-string=make", why: "`env -S`", only: true},
		{argv: []string{"awk", "NR", "data"}, word: "awk", why: "without `-f <file>`", only: true},
		{argv: []string{"sed", "p"}, word: "sed", why: "without `-f <file>`", only: true},
		{argv: []string{"mawk", "NR", "data"}, word: "mawk", why: "without `-f <file>`", only: true},
		{argv: []string{"nawk", "NR", "data"}, word: "nawk", why: "without `-f <file>`", only: true},
		{argv: []string{"gawk", "-f", "a.awk", "-e", "x"}, word: "gawk", why: "without `-f <file>`", only: true},
		{argv: []string{"sed", "-f", "a.sed", "--expression=p"}, word: "sed", why: "without `-f <file>`", only: true},
		{argv: []string{"awk", "-f", "a.awk", "data"}},
		{argv: []string{"sed", "-n", "-f", "a.sed", "data"}},
		{argv: []string{"xargs", "make"}, word: "xargs", why: "builds commands", only: true},
		{argv: []string{"find", ".", "-exec", "make"}, word: "-exec", why: "builds commands", only: true},
		{argv: []string{"find", ".", "-execdir", "make"}, word: "-execdir", why: "builds commands", only: true},
		{argv: []string{"find", ".", "-ok", "make"}, word: "-ok", why: "builds commands", only: true},
		{argv: []string{"find", ".", "-okdir", "make"}, word: "-okdir", why: "builds commands", only: true},
		{argv: []string{"find", ".", "-name", "x"}},
		// `.` outside program position is a path, the root.
		{argv: []string{"go", "test", "."}},
		{argv: []string{"git", "add", "."}},
		// Behind launchers.
		{argv: []string{"timeout", "9", "npx", "tool"}, word: "npx", why: "fetches a package", only: true},
		{argv: []string{"timeout", "-s", "KILL", "-k", "5", "9", "npx", "tool"}, word: "npx", why: "fetches a package", only: true},
		{argv: []string{"env", "FOO=1", "npx", "tool"}, word: "npx", why: "fetches a package"},
		{argv: []string{"nice", "-n", "5", "xargs", "make"}, word: "xargs", why: "builds commands", only: true},
		{argv: []string{"env", "nice", "npx", "tool"}, word: "npx", why: "fetches a package", only: true},
		{argv: []string{"env", "-u", "HOME", "eval", "x"}, word: "eval", why: "runs text as shell code", only: true},
		{argv: []string{"nohup", "time", "-o", "t.txt", "eval", "x"}, word: "eval", why: "runs text as shell code", only: true},
		{argv: []string{"env", "-C", "sub", "sh"}, word: "sh", why: "with no script file", only: true},
		{argv: []string{"nice", "--weird", "npx", "tool"}, word: "--weird", why: "not a launcher flag", only: true},
		{argv: []string{"env", "-", "npx", "tool"}, word: "-", why: "not a launcher flag", only: true},
		{argv: []string{"env", "-i", "npx", "tool"}, word: "-i", why: "not a launcher flag", only: true},
		{argv: []string{"env", "-u", "HOME", "make", "gate"}},
		{argv: []string{"timeout", "60", "make", "gate"}},
		// Each launcher flag D5 lists takes its value, and the program after it is found.
		{argv: []string{"env", "-P", "sub", "make", "gate"}},
		{argv: []string{"time", "-f", "%e", "make", "gate"}},
		{argv: []string{"env", "-P", "sub", "npx", "tool"}, word: "npx", why: "fetches a package", only: true},
		{argv: []string{"time", "-f", "%e", "xargs", "make"}, word: "xargs", why: "builds commands", only: true},
	})
}

// ATTACH-3's requirement, the sidecar command rule: no environment is set (design.md
// D5, amended 2026-10-14). A `NAME=value` word among env's words or in program position
// is refused, whatever program follows, since a variable can load code into any of them
// (NODE_OPTIONS with `npm run gate`, which names no node). The walk goes on past it, so
// the program after it is still judged. `env -u NAME` only removes a variable.
func TestSidecarNoEnvironment(t *testing.T) {
	t.Parallel()
	root, _ := sidecarRoot(t)
	const why = "sets the environment"
	checkRows(t, root, []sidecarRow{
		{argv: []string{"env", "NODE_OPTIONS=--import=data:text/javascript,x", "npm", "run", "gate"}, word: "NODE_OPTIONS=--import=data:text/javascript,x", why: why, only: true},
		{argv: []string{"env", "FOO=1", "npx", "tool"}, word: "FOO=1", why: why},
		{argv: []string{"env", "FOO=1", "npx", "tool"}, word: "npx", why: "fetches a package"},
		{argv: []string{"FOO=1", "make"}, word: "FOO=1", why: why, only: true},
		{argv: []string{"FOO=1", "npx", "tool"}, word: "FOO=1", why: why},
		{argv: []string{"FOO=1", "npx", "tool"}, word: "npx", why: "fetches a package"},
		{argv: []string{"nice", "-n", "5", "env", "BASH_ENV=x.sh", "make", "gate"}, word: "BASH_ENV=x.sh", why: why, only: true},
		{argv: []string{"timeout", "9", "LD_PRELOAD=x.so", "make"}, word: "LD_PRELOAD=x.so", why: why, only: true},
		// -i is not a launcher flag D5 lists, so the walk stops at it.
		{argv: []string{"env", "-i", "FOO=1", "make"}, word: "-i", why: "not a launcher flag", only: true},
		// The named accepted false refusal.
		{argv: []string{"env", "CI=1", "make", "test"}, word: "CI=1", why: why, only: true},
		{argv: []string{"env", "-u", "NAME", "make", "gate"}},
		// `=` after program position is the program's own word, not an assignment.
		{argv: []string{"make", "V=1", "gate"}},
		{argv: []string{"go", "test", "-run=TestX", "./..."}},
		// An env is judged wherever it stands, behind a launcher on no list too, as code
		// flags are: its NAME=value words are refused past its listed flags and values.
		{argv: []string{"caffeinate", "env", "NODE_OPTIONS=--import=data:text/javascript,x", "npm", "run", "gate"}, word: "NODE_OPTIONS=--import=data:text/javascript,x", why: why, only: true},
		{argv: []string{"stdbuf", "-o0", "env", "X=1", "make"}, word: "X=1", why: why, only: true},
		{argv: []string{"caffeinate", "-i", "env", "FOO=1", "make"}, word: "FOO=1", why: why, only: true},
		{argv: []string{"caffeinate", "env", "-u", "HOME", "make", "gate"}},
		{argv: []string{"caffeinate", "env", "-C", "sub", "-P", "sub", "make", "gate"}},
		{argv: []string{"caffeinate", "env", "-u", "HOME", "FOO=1", "make"}, word: "FOO=1", why: why, only: true},
		// An unlisted env flag ends what the scan can read of env's words, so it is
		// refused wherever env stands, as in program position (tasks.md's decision log).
		// only: the scan stops at -i, so NODE_OPTIONS=x past it is not judged as env's.
		{argv: []string{"caffeinate", "env", "-i", "NODE_OPTIONS=x", "npm", "run", "gate"}, word: "-i", why: "not a launcher flag", only: true},
		// env -S behind a launcher on no list keeps its own reason, so the -S case
		// cannot regress behind the unknown-flag branch, which would refuse it too.
		{argv: []string{"caffeinate", "env", "-SFOO=1", "make"}, word: "-SFOO=1", why: "`env -S`", only: true},
		{argv: []string{"caffeinate", "env", "--split-string=make"}, word: "--split-string=make", why: "`env -S`", only: true},
		{argv: []string{"caffeinate", "ENV", "FOO=1", "make"}, word: "FOO=1", why: why, only: true},
		// D5's named limits, allowed and pinned so nobody mistakes them for gaps: an
		// environment set by a program's own flag or word rather than through env
		// (`sudo NAME=value`, npm's `--node-options`) is not judged.
		{argv: []string{"sudo", "FOO=1", "make"}},
		{argv: []string{"npm", "--node-options=x", "run", "gate"}},
	})
}

// ATTACH-3's requirement, the sidecar command rule: a program-position name is
// compared with case folded, because macOS's case-insensitive filesystem execs `BASH`
// as bash (tasks.md's decision log). Every table is reached through an upper-case
// name, and an exact name is still not matched on a prefix.
func TestSidecarNamesFoldCase(t *testing.T) {
	t.Parallel()
	root, _ := sidecarRoot(t)
	checkRows(t, root, []sidecarRow{
		{argv: []string{"NODE", "--import=data:text/javascript,x", "infra/roadmap-queue.mjs"}, word: "--import=data:text/javascript,x", why: codeWhy, only: true},
		{argv: []string{"BASH", "-c", "make"}, word: "-c", why: codeWhy},
		{argv: []string{"PYTHON3", "-c", "x"}, word: "-c", why: codeWhy},
		{argv: []string{"Python3.12", "-m", "http.server"}, word: "-m", why: codeWhy},
		{argv: []string{"env", "PERL", "-e", "x"}, word: "-e", why: codeWhy},
		{argv: []string{"SH"}, word: "SH", why: "with no script file", only: true},
		{argv: []string{"NPX", "tool"}, word: "NPX", why: "fetches a package", only: true},
		{argv: []string{"PNPX", "tool"}, word: "PNPX", why: "fetches a package", only: true},
		{argv: []string{"NPM", "exec", "tool"}, word: "exec", why: "fetches a package", only: true},
		{argv: []string{"Pnpm", "dlx", "tool"}, word: "dlx", why: "fetches a package", only: true},
		{argv: []string{"GO", "run", "example.com/tool@v1"}, word: "example.com/tool@v1", why: "fetches a", only: true},
		{argv: []string{"XARGS", "make"}, word: "XARGS", why: "builds commands", only: true},
		{argv: []string{"FIND", ".", "-exec", "make"}, word: "-exec", why: "builds commands", only: true},
		{argv: []string{"AWK", "NR", "data"}, word: "AWK", why: "without `-f <file>`", only: true},
		{argv: []string{"Sed", "p"}, word: "Sed", why: "without `-f <file>`", only: true},
		{argv: []string{"EVAL", "make"}, word: "EVAL", why: "runs text as shell code", only: true},
		{argv: []string{"ENV", "npx", "tool"}, word: "npx", why: "fetches a package", only: true},
		{argv: []string{"Timeout", "9", "NPX", "tool"}, word: "NPX", why: "fetches a package", only: true},
		{argv: []string{"BUN", "run", "gate"}},
		{argv: []string{"PYTHONIC", "-c", "x"}},
	})
}

// ATTACH-3's requirement, the sidecar command rule: every fetch-and-run form, its
// subcommand matched as any later word, so a flag or its value cannot hide it.
func TestSidecarFetchAndRun(t *testing.T) {
	t.Parallel()
	root, _ := sidecarRoot(t)
	const why = "fetches a"
	checkRows(t, root, []sidecarRow{
		{argv: []string{"npx", "tool"}, word: "npx", why: why, only: true},
		{argv: []string{"bunx", "tool"}, word: "bunx", why: why, only: true},
		{argv: []string{"uvx", "tool"}, word: "uvx", why: why, only: true},
		// uv's long form of uvx, its `run` matched as any later word.
		{argv: []string{"uv", "tool", "run", "ruff"}, word: "tool", why: "`uv tool run` fetches a", only: true},
		{argv: []string{"uv", "--offline", "tool", "--quiet", "run", "ruff"}, word: "tool", why: "`uv tool run` fetches a", only: true},
		{argv: []string{"UV", "tool", "run", "ruff"}, word: "tool", why: why, only: true},
		{argv: []string{"uv", "tool", "list"}},
		{argv: []string{"pipx", "run", "tool"}, word: "run", why: why, only: true},
		{argv: []string{"npm", "exec", "tool"}, word: "exec", why: why, only: true},
		{argv: []string{"npm", "-y", "exec", "tool"}, word: "exec", why: why, only: true},
		{argv: []string{"npm", "x", "tool"}, word: "x", why: why, only: true},
		{argv: []string{"npm", "init", "pkg"}, word: "init", why: why, only: true},
		// npm's alias of init, and pnpm's of dlx (aliases of named forms; the decision log).
		{argv: []string{"npm", "innit", "pkg"}, word: "innit", why: why, only: true},
		{argv: []string{"pnpx", "tool"}, word: "pnpx", why: why, only: true},
		{argv: []string{"npm", "innit", "-y"}},
		{argv: []string{"npm", "create", "app"}, word: "create", why: why, only: true},
		{argv: []string{"yarn", "create", "app"}, word: "create", why: why, only: true},
		{argv: []string{"yarn", "dlx", "tool"}, word: "dlx", why: why, only: true},
		{argv: []string{"pnpm", "create", "app"}, word: "create", why: why, only: true},
		{argv: []string{"pnpm", "dlx", "tool"}, word: "dlx", why: why, only: true},
		{argv: []string{"pnpm", "-C", "sub", "dlx", "tool"}, word: "dlx", why: why, only: true},
		{argv: []string{"bun", "x", "tool"}, word: "x", why: why, only: true},
		{argv: []string{"bun", "create", "app"}, word: "create", why: why, only: true},
		{argv: []string{"go", "run", "example.com/tool@v1"}, word: "example.com/tool@v1", why: why, only: true},
		{argv: []string{"go", "run", "-mod=mod", "example.com/tool@latest", "x"}, word: "example.com/tool@latest", why: why, only: true},
		// An accepted false refusal: a script named like a subcommand.
		{argv: []string{"npm", "run", "create"}, word: "create", why: why, only: true},
		{argv: []string{"npm", "init", "-y"}},
		{argv: []string{"go", "run", "./cmd/tool"}},
		{argv: []string{"pipx", "list"}},
	})
}

// ATTACH-3's requirement, the sidecar command rule: every word is literal.
func TestSidecarLiteralWords(t *testing.T) {
	t.Parallel()
	root, _ := sidecarRoot(t)
	var rows []sidecarRow
	for c, name := range map[string]string{
		"*": "`*`", "?": "`?`", "[": "`[`", "]": "`]`", "{": "`{`", "}": "`}`", "~": "`~`", "$": "`$`", "`": "a backtick",
		"(": "`(`", ")": "`)`", "<": "`<`", ">": "`>`", "|": "`|`", "&": "`&`", ";": "`;`",
		"'": "`'`", `"`: "`\"`", `\`: "`\\`", "!": "`!`", " ": "a space", "\t": "a tab", "\n": "a newline",
	} {
		rows = append(rows, sidecarRow{argv: []string{"ls", "a" + c + "b"}, word: "a" + c + "b", why: name, only: true})
	}
	rows = append(rows,
		sidecarRow{argv: []string{"git", "-c", "alias.x=!../gate.sh", "x"}, word: "alias.x=!../gate.sh", why: "the character `!`", only: true},
		sidecarRow{argv: []string{"sh", ".claude/x.sh", "2>/dev/null"}, word: "2>/dev/null", why: "the character `>`", only: true},
		sidecarRow{argv: []string{"sh", ".claude/x.sh", ">", "out.txt"}, word: ">", why: "the character `>`", only: true},
		sidecarRow{argv: []string{"git", "log", "--format=%h %s"}, word: "--format=%h %s", why: "a space", only: true},
		// No placeholder: {{.Dir}}-style text is refused like any other `{`.
		sidecarRow{argv: []string{"go", "list", "-f", "{{.Dir}}"}, word: "{{.Dir}}", why: "the character `{`", only: true},
	)
	checkRows(t, root, rows)
}

// ATTACH-3's requirement, the sidecar command rule: every path stays in the
// repository, judged piece by piece with no flag list.
func TestSidecarPaths(t *testing.T) {
	t.Parallel()
	root, _ := sidecarRoot(t)
	const why = "resolves outside the repository"
	checkRows(t, root, []sidecarRow{
		{argv: []string{"cat", ".."}, word: "..", why: why, only: true},
		{argv: []string{"cat", "docs/../../x"}, word: "docs/../../x", why: why, only: true},
		{argv: []string{"make", "-C..", "gate"}, word: "-C..", why: why, only: true},
		{argv: []string{"make", "-C", "..", "gate"}, word: "..", why: why, only: true},
		{argv: []string{"make", "-sC..", "gate"}, word: "-sC..", why: why, only: true},
		{argv: []string{"make", "-sC/tmp", "gate"}, word: "-sC/tmp", why: "/tmp " + why, only: true},
		{argv: []string{"cc", "-isystem/x", "a.c"}, word: "-isystem/x", why: "/x " + why, only: true},
		{argv: []string{"tar", "-xf/tmp/a.tar"}, word: "-xf/tmp/a.tar", why: "/tmp/a.tar " + why, only: true},
		{argv: []string{"npm", "--prefix=..", "run", "gate"}, word: "--prefix=..", why: why, only: true},
		{argv: []string{"make", "--chdir=..", "gate"}, word: "--chdir=..", why: why, only: true},
		{argv: []string{"make", "-p.."}, word: "-p..", why: why, only: true},
		{argv: []string{"cc", "-I../lib", "-o/tmp/x", "a.c"}, word: "-I../lib", why: "../lib " + why},
		{argv: []string{"cc", "-I../lib", "-o/tmp/x", "a.c"}, word: "-o/tmp/x", why: "/tmp/x " + why},
		{argv: []string{"curl", "-d", "@/abs"}, word: "@/abs", why: "/abs " + why, only: true},
		{argv: []string{"git", "--git-dir=/tmp/x", "log"}, word: "--git-dir=/tmp/x", why: why, only: true},
		{argv: []string{"git", "--exec-path=..", "log"}, word: "--exec-path=..", why: why, only: true},
		{argv: []string{"cargo", "build", "--manifest-path=../o"}, word: "--manifest-path=../o", why: why, only: true},
		{argv: []string{"tool", "--require=../x.js"}, word: "--require=../x.js", why: why, only: true},
		{argv: []string{"env", "NODE_OPTIONS=--require=../x.js", "make"}, word: "NODE_OPTIONS=--require=../x.js", why: why, only: true},
		{argv: []string{"env", "PATH=x:..", "gate"}, word: "PATH=x:..", why: why, only: true},
		{argv: []string{"curl", "https://host/x"}, word: "https://host/x", why: "//host/x " + why, only: true},
		{argv: []string{"/usr/local/bin/report"}, word: "/usr/local/bin/report", why: why, only: true},
		// Split at `,` too; and no /dev/null exception (re-adding one fails here).
		{argv: []string{"cmd", "a,..,b"}, word: "a,..,b", why: ".. " + why, only: true},
		{argv: []string{"cat", "/dev/null"}, word: "/dev/null", why: why, only: true},
		// A directory symlink out of the repository, then `..`: the kernel follows the
		// link before the `..` applies, so the path lands outside (D5: resolved through
		// symlinks), whatever its form.
		{argv: []string{"make", "-C", "outdir/..", "gate"}, word: "outdir/..", why: "outdir/.. " + why, only: true},
		{argv: []string{"cat", "outdir/../payload.sh"}, word: "outdir/../payload.sh", why: why, only: true},
		{argv: []string{"make", "--chdir=outdir/..", "gate"}, word: "--chdir=outdir/..", why: why, only: true},
		{argv: []string{"cat", filepath.Join(root, "outdir") + "/../payload.sh"}, word: filepath.Join(root, "outdir") + "/../payload.sh", why: why, only: true},
		{argv: []string{"cat", "outdir/x.sh"}, word: "outdir/x.sh", why: why, only: true},
		// A `..` that climbs back from a folder not there yet to one that is: the rest
		// is resolved through symlinks again, as the kernel does once `mkdir -p` has
		// made the folder (before, it made <outside>/escape).
		{argv: []string{"mkdir", "-p", "x/../outdir/../escape"}, word: "x/../outdir/../escape", why: why, only: true},
		{argv: []string{"cat", "nope/../outdir/../payload.sh"}, word: "nope/../outdir/../payload.sh", why: why, only: true},
		{argv: []string{"mkdir", "-p", "a/b/../../outdir/x"}, word: "a/b/../../outdir/x", why: why, only: true},
		{argv: []string{"mkdir", "-p", "a/b/../c"}},
		// Symlinks that stay inside, then `..`, and a real folder then `..`.
		{argv: []string{"cat", "inlink/../payload.sh"}},
		{argv: []string{"cat", "srclink/../x/a.h"}},
		{argv: []string{"make", "-C", "sub/..", "gate"}},
		// A symlink out of the repository, and one that stays in.
		{argv: []string{"scripts/link.sh"}, word: "scripts/link.sh", why: why, only: true},
		{argv: []string{"scripts/in.sh"}},
		{argv: []string{"sh", "scripts/in.sh"}},
		// The named false refusal, and how to write it.
		{argv: []string{"cc", "-Isrc/x", "a.c"}, word: "-Isrc/x", why: "/x " + why, only: true},
		{argv: []string{"cc", "-I", "src/x", "a.c"}},
		// Still allowed.
		{argv: []string{"git", "log", "main..HEAD"}},
		{argv: []string{"make", "-j4", "gate"}},
		{argv: []string{"git", "log", "--format=%h"}},
		{argv: []string{"go", "test", "./..."}},
	})
}

// ATTACH-3's requirement, the sidecar command rule: an interpreter in program position
// runs a script file of the repository's that exists, or (bun) a package.json script.
func TestSidecarScriptFiles(t *testing.T) {
	t.Parallel()
	root, outside := sidecarRoot(t)
	checkRows(t, root, []sidecarRow{
		{argv: []string{"sh"}, word: "sh", why: "with no script file", only: true},
		{argv: []string{"bash", "-e"}, word: "bash", why: "with no script file", only: true},
		{argv: []string{"sh", "../other/gate.sh"}, word: "../other/gate.sh", why: "resolves outside the repository", only: true},
		{argv: []string{"sh", filepath.Join(outside, "gate.sh")}, word: filepath.Join(outside, "gate.sh"), why: "resolves outside the repository", only: true},
		{argv: []string{"sh", ".claude/missing.sh"}, word: ".claude/missing.sh", why: "not a script file in the repository", only: true},
		{argv: []string{"sh", "gate.sh"}, word: "gate.sh", why: "not a script file in the repository", only: true},
		{argv: []string{"sh", "sub"}, word: "sub", why: "not a script file in the repository", only: true},
		{argv: []string{"bun", "run", "scripts/x.ts"}, word: "run", why: "not a script file in the repository", only: true},
		{argv: []string{"deno", "run", "x.ts"}, word: "run", why: "not a script file in the repository", only: true},
		{argv: []string{"node", "--test"}, word: "node", why: "with no script file", only: true},
		// Through a directory symlink and `..`: the file judged is the one the kernel
		// opens. outdir/.. is <outside>, so its payload.sh is outside, though the root
		// has one too; srclink/.. is src, which has no payload.sh, though the root has.
		{argv: []string{"sh", "outdir/../payload.sh"}, word: "outdir/../payload.sh", why: "resolves outside the repository", only: true},
		{argv: []string{"sh", "srclink/../payload.sh"}, word: "srclink/../payload.sh", why: "not a script file in the repository", only: true},
		{argv: []string{"sh", "srclink/../x/a.h"}},
		{argv: []string{"sh", "inlink/../payload.sh"}},
		// The repository's own scripts.
		{argv: []string{"sh", ".claude/x.sh", "arg"}},
		{argv: []string{"bash", "-e", ".claude/x.sh"}},
		{argv: []string{"sh", filepath.Join(root, ".claude", "x.sh")}},
		{argv: []string{"node", "infra/roadmap-queue.mjs", "--text"}},
		{argv: []string{"bun", "run", "gate"}},
		{argv: []string{"osascript", "x.scpt"}},
	})
}

// ATTACH-3's requirement, the sidecar command rule: each argv command `panel init`
// writes for an attached repository passes (design.md D5).
func TestSidecarAllowsAttachCommands(t *testing.T) {
	t.Parallel()
	root, _ := sidecarRoot(t)
	var rows []sidecarRow
	for _, argv := range [][]string{
		{"sh", ".claude/owner-queue.sh"}, {"sh", ".claude/roadmap-queue.sh", "--text"},
		{"sh", ".claude/panel-suggest.sh", "build"}, {"sh", ".claude/panel-suggest.sh", "propose"},
		{"sh", ".claude/panel-suggest.sh", "fix"}, {"sh", ".claude/panel-suggest.sh", "ops"},
		{"scripts/ci/run-local.sh"},
		{"npm", "run", "gate"}, {"pnpm", "run", "gate"}, {"yarn", "run", "gate"}, {"bun", "run", "gate"}, {"make", "gate"},
		{"make", "-C", "sub", "gate"},
	} {
		rows = append(rows, sidecarRow{argv: argv})
	}
	checkRows(t, root, rows)
}

// ATTACH-3's requirement, the sidecar command rule, over a whole config: the commands
// judged are cards, queues, suggest, metrics and the worktree hooks; a first prompt and
// the resume line are typed text and are not; each refused line is marked in the run
// list with its words, their reasons and the remedy.
func TestSidecarRefusalsOfAConfig(t *testing.T) {
	t.Parallel()
	root, _ := sidecarRoot(t)
	cfg, err := ParseConfig([]byte(`{"name":"x","lanes":{"main":"orchestrator","change/":"build"},
		"cards":[{"id":"ok","title":"OK","command":["sh",".claude/x.sh"],"refresh":"interval:60"},
		         {"id":"bad","title":"Bad","command":["sh","-c","make gate"],"refresh":"interval:60"}],
		"queues":[{"id":"q","lock":"stridal/gate.lock","command":["npx","tool"]}],
		"templates":[{"id":"t","title":"T","lane_type":"build","branch_pattern":"change/{name}","first_prompt":"run $HOME; rm -rf ~",
		              "suggest":{"command":["eval","x"],"refresh":"interval:60"}}],
		"metrics":{"command":["/usr/bin/report"]},
		"worktree_setup":{"command":["xargs","make"]},"worktree_teardown":{"command":["sh",".claude/missing.sh"]},
		"quota_auto_resume":true,"quota_resume_line":"go on $HOME"}`))
	if err != nil {
		t.Fatal(err)
	}
	refused := cfg.SidecarRefusals(root)
	var lines []string
	for _, l := range refused {
		lines = append(lines, l.Line)
	}
	want := []string{"card bad ", "queue q ", "template t suggests", "metrics ", "worktree_setup ", "worktree_teardown "}
	if len(refused) != len(want) {
		t.Fatalf("refused lines %q, want %q", lines, want)
	}
	for i, w := range want {
		if !strings.HasPrefix(lines[i], w) {
			t.Errorf("line %d is %q, want %q…", i, lines[i], w)
		}
	}
	if len(refused[0].Words) != 2 || refused[0].Words[0].Word != "-c" || refused[0].Words[1].Word != "make gate" {
		t.Errorf("every word of the card: %+v", refused[0].Words)
	}
	runs := cfg.SidecarRunList(root)
	all := strings.Join(runs, "\n")
	if len(runs) != len(cfg.RunList()) {
		t.Fatalf("the marked list drops or adds a line:\n%s", all)
	}
	wantMark := `NOT ALLOWED IN A SIDECAR ("-c": ` + "`sh -c` runs code given in the command, not a script file in the repository; " +
		`"make gate": a space: with no shell it reaches the program as it is; move it into a script in the repository): card bad runs`
	if !strings.Contains(all, wantMark) {
		t.Errorf("the card's mark:\n%s\nwant %s", all, wantMark)
	}
	for _, r := range runs {
		// Typed text is never judged; every command but card ok is refused.
		marked := strings.HasPrefix(r, "NOT ALLOWED IN A SIDECAR (")
		want := !strings.Contains(r, " types ") && !strings.HasPrefix(r, "card ok ")
		if marked != want {
			t.Errorf("marked %v, want %v: %s", marked, want, r)
		}
	}
}
