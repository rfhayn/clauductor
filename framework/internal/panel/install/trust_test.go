package install

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/signals"
)

func TestConfigTrust(t *testing.T) {
	t.Parallel()
	home, root := t.TempDir(), signals.ResolvePath(t.TempDir())
	cfg := filepath.Join(root, "panel.json")
	// A config never seen is not trusted by being run, and running it records nothing.
	tv, err := CheckTrust(home, root, cfg, "h1", false)
	if err != nil || tv.Trusted || tv.Prev != "" || TrustedNow(home, root, cfg, "h1") {
		t.Fatalf("an unseen config must not be trusted by running it: %+v %v", tv, err)
	}
	if tv, _ := CheckTrust(home, root, cfg, "h1", true); !tv.Trusted || tv.Note == "" {
		t.Fatalf("trusting it (`panel trust`) records it: %+v", tv)
	}
	if tv, _ := CheckTrust(home, root, cfg, "h1", false); !tv.Trusted {
		t.Fatal("unchanged config untrusted")
	}
	tv, _ = CheckTrust(home, root, cfg, "h2", false)
	if tv.Trusted || tv.Prev != "h1" {
		t.Fatalf("a changed config must not be trusted silently: %+v", tv)
	}
	if TrustedNow(home, root, cfg, "h2") {
		t.Fatal("an untrusted check recorded the new hash")
	}
	if tv, _ := CheckTrust(home, root, cfg, "h2", true); !tv.Trusted || !TrustedNow(home, root, cfg, "h2") {
		t.Fatalf("--trust-config: %+v", tv)
	}
	fi, err := os.Stat(TrustPath(home, root))
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("trust file mode: %v %v", fi, err)
	}
	// The hash covers the exact bytes.
	if ConfigHash([]byte("a")) == ConfigHash([]byte("a ")) {
		t.Fatal("hash ignores bytes")
	}
}

// sidecarProject is a repository root holding the scripts the commands name, a
// directory `sub`, and a symlink scripts/link.sh to a script outside it; and a home
// whose sidecar for it (not yet written) is returned.
func sidecarProject(t *testing.T) (home, root, sidecar string) {
	t.Helper()
	home, root = signals.ResolvePath(t.TempDir()), signals.ResolvePath(t.TempDir())
	for _, f := range []string{".claude/x.sh", ".claude/owner-queue.sh", ".claude/roadmap-queue.sh", ".claude/panel-suggest.sh",
		"scripts/ci/run-local.sh", "infra/roadmap-queue.mjs", "sub/.keep"} {
		os.MkdirAll(filepath.Join(root, filepath.Dir(f)), 0o755)
		// Not executable: the rule judges a script's place, not its mode.
		os.WriteFile(filepath.Join(root, f), []byte("#!/bin/sh\n"), 0o644)
	}
	out := filepath.Join(signals.ResolvePath(t.TempDir()), "gate.sh")
	os.WriteFile(out, []byte("#!/bin/sh\n"), 0o644)
	os.Symlink(out, filepath.Join(root, "scripts", "link.sh"))
	return home, root, config.SidecarPath(home, "acme")
}

// cardsConfig is a config with one card per command.
func cardsConfig(t *testing.T, cmds ...[]string) string {
	t.Helper()
	var cards []string
	for i, c := range cmds {
		b, _ := json.Marshal(c)
		cards = append(cards, fmt.Sprintf(`{"id":"c%d","title":"C","command":%s,"refresh":"interval:60"}`, i, b))
	}
	return `{"name":"acme","lanes":{"main":"orchestrator"},"cards":[` + strings.Join(cards, ",") + `]}`
}

func writeConfig(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// refusedTrust trusts the sidecar as `panel trust` does and asserts the D5 refusal:
// an error naming it, the report with the one card's line marked with want (the word
// and its reason, quoted as the mark prints them) and the remedy, and nothing
// recorded.
func refusedTrust(t *testing.T, home, root, sidecar string, argv []string, want string) {
	t.Helper()
	h, runs, err := TrustConfigReport(home, root, sidecar)
	var refused *SidecarRefusedError
	if !errors.As(err, &refused) || len(refused.Lines) != 1 {
		t.Errorf("%q: trusted, or refused for another reason: %v", argv, err)
		return
	}
	if len(runs) != 1 || !strings.HasPrefix(runs[0], "NOT ALLOWED IN A SIDECAR (") || !strings.Contains(runs[0], want) ||
		!strings.Contains(runs[0], "; move it into a script in the repository): card c0 runs ") {
		t.Errorf("%q: the report %q, want its line marked with %q", argv, runs, want)
	}
	if _, err := os.Stat(TrustPath(home, root)); !os.IsNotExist(err) || TrustedNow(home, root, sidecar, h) {
		t.Errorf("%q: a refused trust recorded something", argv)
	}
}

// [ATTACH-3-S3] An attached project whose sidecar runs each argv command `panel init`
// writes for an attached repository, and the other commands the scenario names, with
// every script file in the repository and `sub` a directory inside it: trusting it
// trusts and records it, and no line is marked.
func TestSidecarOfTheRepositorysCommandsIsTrusted(t *testing.T) {
	t.Parallel()
	home, root, sidecar := sidecarProject(t)
	writeConfig(t, sidecar, cardsConfig(t,
		[]string{"sh", ".claude/owner-queue.sh"}, []string{"sh", ".claude/roadmap-queue.sh", "--text"},
		[]string{"sh", ".claude/panel-suggest.sh", "build"}, []string{"scripts/ci/run-local.sh"},
		[]string{"npm", "run", "gate"}, []string{"pnpm", "run", "gate"}, []string{"yarn", "run", "gate"},
		[]string{"bun", "run", "gate"}, []string{"make", "gate"},
		[]string{"sh", ".claude/x.sh", "arg"}, []string{"node", "infra/roadmap-queue.mjs", "--text"},
		[]string{"git", "log", "--format=%h"}, []string{"git", "log", "main..HEAD"}, []string{"go", "test", "."},
		[]string{"go", "test", "./..."}, []string{"git", "add", "."}, []string{"make", "-j4", "gate"},
		[]string{"make", "-C", "sub", "gate"}))
	h, runs, err := TrustConfigReport(home, root, sidecar)
	if err != nil {
		t.Fatalf("refused: %v\n%s", err, strings.Join(runs, "\n"))
	}
	if !TrustedNow(home, root, sidecar, h) {
		t.Fatal("trusted, but not recorded")
	}
	if len(runs) != 18 || strings.Contains(strings.Join(runs, "\n"), "NOT ALLOWED") {
		t.Errorf("the report:\n%s", strings.Join(runs, "\n"))
	}
}

// [ATTACH-3-S4] A project whose config is its own .clauductor/panel.json, running
// /usr/local/bin/report and sh -c "sh .claude/owner-queue.sh 2>&1": trusting it
// trusts and records it, as before this change.
func TestInRepositoryConfigKeepsTodaysTrust(t *testing.T) {
	t.Parallel()
	home, root, _ := sidecarProject(t)
	own := filepath.Join(root, config.DefaultConfigRel)
	writeConfig(t, own, cardsConfig(t, []string{"/usr/local/bin/report"}, []string{"sh", "-c", "sh .claude/owner-queue.sh 2>&1"}))
	h, runs, err := TrustConfigReport(home, root, own)
	if err != nil || !TrustedNow(home, root, own, h) || strings.Contains(strings.Join(runs, "\n"), "NOT ALLOWED") {
		t.Fatalf("an in-repository config: %v\n%s", err, strings.Join(runs, "\n"))
	}
}

// [ATTACH-3-S5] An attached project whose sidecar runs scripts/link.sh, a symlink in
// the repository to a script outside it: trust is refused, and the line is marked.
func TestSidecarSymlinkOutIsRefused(t *testing.T) {
	t.Parallel()
	home, root, sidecar := sidecarProject(t)
	argv := []string{"scripts/link.sh"}
	writeConfig(t, sidecar, cardsConfig(t, argv))
	refusedTrust(t, home, root, sidecar, argv, `"scripts/link.sh": scripts/link.sh resolves outside the repository`)
}

// [ATTACH-3-S7] Attached projects whose sidecars each have one card running shell or
// interpreter code, through any launcher: each trust is refused, its line marked with
// the word, why, and the remedy; env -S's reason is env -S itself.
func TestSidecarShellAndInterpreterCodeIsRefused(t *testing.T) {
	t.Parallel()
	for _, r := range []struct {
		argv []string
		want string
	}{
		{[]string{"sh", "-c", "sh .claude/x.sh"}, `"-c": ` + "`sh -c` runs code"},
		{[]string{"bash", "-lc", "make gate"}, `"-lc": ` + "`bash -lc` runs code"},
		{[]string{"env", "sh", "-c", "make"}, `"-c": ` + "`sh -c` runs code"},
		{[]string{"eval", "make"}, `"eval": ` + "`eval` runs text as shell code"},
		{[]string{"xargs", "make"}, `"xargs": ` + "`xargs` builds commands"},
		{[]string{"find", ".", "-exec", "make", ";"}, `"-exec": ` + "`find -exec` builds commands"},
		{[]string{"python3", "-c", "print(1)"}, `"-c": ` + "`python3 -c` runs code"},
		{[]string{"python3", "-m", "http.server"}, `"-m": ` + "`python3 -m` runs code"},
		{[]string{"node", "-e", "1"}, `"-e": ` + "`node -e` runs code"},
		{[]string{"node", "--import", "x.mjs", "a.js"}, `"--import": ` + "`node --import` runs code"},
		{[]string{"caffeinate", "-i", "bash", "-c", "make"}, `"-c": ` + "`bash -c` runs code"},
		{[]string{"env", "-S", "make"}, `"-S": ` + "`env -S` splits one word"},
		{[]string{"npx", "tool"}, `"npx": ` + "`npx` fetches a package"},
		{[]string{"npm", "exec", "tool"}, `"exec": ` + "`npm exec` fetches a package"},
		{[]string{"npm", "-y", "exec", "tool"}, `"exec": ` + "`npm exec` fetches a package"},
		{[]string{"pnpm", "-C", "sub", "dlx", "tool"}, `"dlx": ` + "`pnpm dlx` fetches a package"},
		{[]string{"npm", "create", "app"}, `"create": ` + "`npm create` fetches a package"},
		{[]string{"go", "run", "example.com/tool@v1"}, `"example.com/tool@v1": ` + "`go run module@version` fetches"},
		{[]string{"timeout", "9", "npx", "tool"}, `"npx": ` + "`npx` fetches a package"},
		{[]string{"env", "FOO=1", "npx", "tool"}, `"npx": ` + "`npx` fetches a package"},
		// Only the no-environment rule refuses this: npm names no node, and the word's
		// pieces are no path outside (design.md D5, "No environment is set").
		{[]string{"env", "NODE_OPTIONS=--import=data:text/javascript,x", "npm", "run", "gate"},
			`"NODE_OPTIONS=--import=data:text/javascript,x": ` + "`NODE_OPTIONS=--import=data:text/javascript,x` sets the environment"},
		{[]string{"nice", "-n", "5", "xargs", "make"}, `"xargs": ` + "`xargs` builds commands"},
		{[]string{"env", "nice", "npx", "tool"}, `"npx": ` + "`npx` fetches a package"},
		{[]string{"sh"}, `"sh": ` + "`sh` with no script file"},
		{[]string{"sh", ".claude/missing.sh"}, `".claude/missing.sh": ` + "`sh` runs .claude/missing.sh, which is not a script file in the repository"},
	} {
		home, root, sidecar := sidecarProject(t)
		writeConfig(t, sidecar, cardsConfig(t, r.argv))
		refusedTrust(t, home, root, sidecar, r.argv, r.want)
	}
}

// [ATTACH-3-S8] Attached projects whose sidecars each have one card holding a word
// that is not literal, or a redirection: each trust is refused, its line marked; no
// /dev/null exception applies.
func TestSidecarWordThatIsNotLiteralIsRefused(t *testing.T) {
	t.Parallel()
	for _, r := range []struct {
		argv []string
		want string
	}{
		{[]string{"ls", "$HOME"}, `"$HOME": the character ` + "`$`"},
		{[]string{"ls", "*.go"}, `"*.go": the character ` + "`*`"},
		{[]string{"ls", "~"}, `"~": the character ` + "`~`"},
		{[]string{"git", "log", "--format=%h %s"}, `"--format=%h %s": a space`},
		{[]string{"sh", ".claude/x.sh", "2>/dev/null"}, `"2>/dev/null": the character ` + "`>`"},
		{[]string{"sh", ".claude/x.sh", ">", "out.txt"}, `">": the character ` + "`>`"},
		{[]string{"sh", ".claude/x.sh", "'a'"}, `"'a'": the character ` + "`'`"},
		{[]string{"sh", ".claude/x.sh", `a\b`}, `"a\\b": the character ` + "`\\`"},
		{[]string{"git", "-c", "alias.x=!gate", "x"}, `"alias.x=!gate": the character ` + "`!`"},
	} {
		home, root, sidecar := sidecarProject(t)
		writeConfig(t, sidecar, cardsConfig(t, r.argv))
		refusedTrust(t, home, root, sidecar, r.argv, r.want)
	}
}

// [ATTACH-3-S9] Attached projects whose sidecars each have one card reaching a path
// outside the repository through `..`, a flag or an assignment: each trust is
// refused, its line marked with the piece that resolves outside; main..HEAD, -j4,
// ./... and -C sub are trusted.
func TestSidecarPathOutsideIsRefused(t *testing.T) {
	t.Parallel()
	const out = " resolves outside the repository"
	for _, r := range []struct {
		argv []string
		want string
	}{
		{[]string{"make", "-C", "..", "gate"}, `"..": ..` + out},
		{[]string{"make", "-C..", "gate"}, `"-C..": ..` + out},
		{[]string{"npm", "--prefix=..", "run", "gate"}, `"--prefix=..": ..` + out},
		{[]string{"git", "--git-dir=/tmp/x", "log"}, `"--git-dir=/tmp/x": /tmp/x` + out},
		{[]string{"git", "--exec-path=..", "log"}, `"--exec-path=..": ..` + out},
		{[]string{"env", "PATH=x:..", "gate"}, `"PATH=x:..": ..` + out},
		{[]string{"env", "NODE_OPTIONS=--require=../x.js", "make"}, `"NODE_OPTIONS=--require=../x.js": ../x.js` + out},
		{[]string{"cc", "-I../lib", "-o/tmp/x", "a.c"}, `"-I../lib": ../lib` + out + `; "-o/tmp/x": /tmp/x` + out},
		{[]string{"cargo", "build", "--manifest-path=../o/Cargo.toml"}, `"--manifest-path=../o/Cargo.toml": ../o/Cargo.toml` + out},
		{[]string{"curl", "-d", "@/etc/hosts"}, `"@/etc/hosts": /etc/hosts` + out},
		{[]string{"make", "-p.."}, `"-p..": ..` + out},
		{[]string{"make", "-sC..", "gate"}, `"-sC..": ..` + out},
		{[]string{"make", "-sC/tmp", "gate"}, `"-sC/tmp": /tmp` + out},
		{[]string{"cat", "docs/../../x"}, `"docs/../../x": docs/../../x` + out},
	} {
		home, root, sidecar := sidecarProject(t)
		writeConfig(t, sidecar, cardsConfig(t, r.argv))
		refusedTrust(t, home, root, sidecar, r.argv, r.want)
	}
	home, root, sidecar := sidecarProject(t)
	writeConfig(t, sidecar, cardsConfig(t, []string{"git", "log", "main..HEAD"}, []string{"make", "-j4", "gate"},
		[]string{"go", "test", "./..."}, []string{"make", "-C", "sub", "gate"}))
	if _, runs, err := TrustConfigReport(home, root, sidecar); err != nil {
		t.Errorf("refused: %v\n%s", err, strings.Join(runs, "\n"))
	}
}

// ATTACH-3's requirement, a trust entry recorded earlier does not count: CheckTrust
// and TrustedNow judge a sidecar's bytes at its path on every call, so an entry for
// its exact bytes, written by hand, leaves it untrusted; and bytes that do not hash to
// the hash given, or do not parse, are untrusted too.
func TestSidecarRecordedEarlierIsNotTrusted(t *testing.T) {
	t.Parallel()
	home, root, sidecar := sidecarProject(t)
	body := cardsConfig(t, []string{"~/bin/report"})
	writeConfig(t, sidecar, body)
	h := ConfigHash([]byte(body))
	if err := writeTrust(TrustPath(home, root), trustFile{Configs: map[string]string{sidecar: h}}); err != nil {
		t.Fatal(err)
	}
	tv, err := CheckTrust(home, root, sidecar, h, false)
	if err != nil || tv.Trusted || !strings.Contains(tv.Note, "NOT ALLOWED IN A SIDECAR") || TrustedNow(home, root, sidecar, h) {
		t.Fatalf("a hand-written entry counted: %+v %v", tv, err)
	}
	if _, err := CheckTrust(home, root, sidecar, h, true); !errors.As(err, new(*SidecarRefusedError)) {
		t.Fatalf("trusting it: %v", err)
	}
	// Allowed bytes, recorded; then the file is not the bytes the hash names.
	good := cardsConfig(t, []string{"make", "gate"})
	writeConfig(t, sidecar, good)
	hg := ConfigHash([]byte(good))
	if tv, err := CheckTrust(home, root, sidecar, hg, true); err != nil || !tv.Trusted || !TrustedNow(home, root, sidecar, hg) {
		t.Fatalf("an allowed sidecar: %+v %v", tv, err)
	}
	writeConfig(t, sidecar, good+" ")
	if TrustedNow(home, root, sidecar, hg) {
		t.Error("the bytes at the path no longer hash to the trusted hash, but it is trusted")
	}
	if tv, err := CheckTrust(home, root, sidecar, hg, true); err == nil || !errors.Is(err, ErrConfigChanged) || tv.Trusted {
		t.Errorf("trusting a hash the file does not have: %+v %v", tv, err)
	}
	writeConfig(t, sidecar, `{"name":`)
	hb := ConfigHash([]byte(`{"name":`))
	if tv, _ := CheckTrust(home, root, sidecar, hb, true); tv.Trusted || TrustedNow(home, root, sidecar, hb) {
		t.Error("a sidecar that does not parse is trusted")
	}
}
