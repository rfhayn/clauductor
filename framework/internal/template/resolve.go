package template

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Which template a command copies from used to be a guess: CLAUDUCTOR_FRAMEWORK, else the first of
// ~/clauductor, ~/Development/clauductor, ... that existed, whatever branch that checkout was on
// (OPS-8 rehearsal: a binary built from one worktree installed another branch's template, and said
// nothing). Now the template is found in a fixed order, its version marker must match the binary,
// and every command that copies from it says which one it used.

// VersionFile sits at the template's root and holds the clauductor version the template belongs
// to. It is not a template file: listDir and the plugin packager leave it out, so a project never
// receives it. TestTemplateVersionMatchesBinary holds it equal to cmd.Version.
const VersionFile = ".template-version"

// BinaryVersion is the running binary's version (cmd sets it from cmd.Version). Empty skips the
// version check, which only a test that builds no binary does.
var BinaryVersion string

// OverrideDir is --template-dir: a template directory (or a clauductor checkout or release
// directory holding template/) named by the user. It wins over everything, and a version mismatch
// under it is a warning, not a refusal: the user chose it.
var OverrideDir string

// Source is the template a command uses, how it was found, and its version.
type Source struct {
	Dir     string `json:"dir"`
	How     string `json:"how"`
	Version string `json:"version"`
	Warning string `json:"warning,omitempty"`
}

// String is the line install, update and diff print.
func (s *Source) String() string {
	v := s.Version
	if v == "" {
		v = "no version marker"
	} else {
		v = "version " + v
	}
	return fmt.Sprintf("%s (%s, %s)", s.Dir, s.How, v)
}

// Resolve finds the template, in this order, and checks its version against the binary's:
//
//  1. --template-dir (OverrideDir)
//  2. $CLAUDUCTOR_FRAMEWORK/template
//  3. next to the binary: <bin dir>/template (an unpacked release archive), <bin dir>/../template
//     (a source build in framework/), and the release share,
//     ${CLAUDUCTOR_SHARE:-~/.local/share/clauductor}/<version>/template (install.sh --release)
//  4. the source checkout the binary was compiled from (a `go build` or `go install` from a clone;
//     a -trimpath release build has no such path)
//
// The first that exists is the answer; a later one is never consulted because an earlier one is
// stale. A version mismatch (or a missing marker) refuses, unless the template came from
// --template-dir, where it is a warning.
func Resolve() (*Source, error) {
	if OverrideDir != "" {
		dir, err := templateIn(OverrideDir)
		if err != nil {
			return nil, fmt.Errorf("--template-dir %s: %w", OverrideDir, err)
		}
		s := &Source{Dir: dir, How: "--template-dir", Version: readVersion(dir)}
		if msg := versionMismatch(s); msg != "" {
			s.Warning = msg + "; using it because --template-dir names it"
		}
		return s, nil
	}
	for _, c := range candidates() {
		if !isTemplate(c.Dir) {
			continue
		}
		c.Version = readVersion(c.Dir)
		if msg := versionMismatch(c); msg != "" {
			return nil, fmt.Errorf("%s.\nRefusing to copy from a template that is not this binary's: install the matching release (it ships its template), point CLAUDUCTOR_FRAMEWORK at a checkout of v%s, or pass --template-dir <dir> to use this one anyway", msg, norm(BinaryVersion))
		}
		return c, nil
	}
	return nil, fmt.Errorf("could not find the Clauductor template: no template next to the binary, in the release share, or in the source checkout it was built from. " +
		"Install a release (install.sh unpacks the binary with its template), set CLAUDUCTOR_FRAMEWORK to a clauductor checkout, or pass --template-dir")
}

func candidates() []*Source {
	var out []*Source
	if env := os.Getenv("CLAUDUCTOR_FRAMEWORK"); env != "" {
		out = append(out, &Source{Dir: filepath.Join(env, "template"), How: "CLAUDUCTOR_FRAMEWORK"})
	}
	if exe, err := os.Executable(); err == nil {
		if real, err := filepath.EvalSymlinks(exe); err == nil {
			exe = real
		}
		bin := filepath.Dir(exe)
		out = append(out,
			&Source{Dir: filepath.Join(bin, "template"), How: "shipped next to the binary"},
			&Source{Dir: filepath.Join(filepath.Dir(bin), "template"), How: "the checkout the binary sits in"})
	}
	if v := norm(BinaryVersion); v != "" {
		share := os.Getenv("CLAUDUCTOR_SHARE")
		if share == "" {
			if home, err := os.UserHomeDir(); err == nil {
				share = filepath.Join(home, ".local", "share", "clauductor")
			}
		}
		if share != "" {
			for _, name := range []string{"v" + v, v} {
				out = append(out, &Source{Dir: filepath.Join(share, name, "template"), How: "the release share"})
			}
		}
	}
	if src := sourceTemplate(); src != "" {
		out = append(out, &Source{Dir: src, How: "the source checkout this binary was built from"})
	}
	return out
}

// sourceTemplate is <checkout>/template for the checkout this file was compiled in, or "" when
// the build recorded no absolute path (-trimpath, as release builds use).
func sourceTemplate() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok || !filepath.IsAbs(file) {
		return ""
	}
	// file is <checkout>/framework/internal/template/resolve.go
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "template")
}

// templateIn accepts a template directory, or a directory holding one as template/.
func templateIn(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	if isTemplate(abs) {
		return abs, nil
	}
	if t := filepath.Join(abs, "template"); isTemplate(t) {
		return t, nil
	}
	return "", fmt.Errorf("not a Clauductor template (no .claude/lib/conf.sh there or in its template/)")
}

func isTemplate(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".claude", "lib", "conf.sh"))
	return err == nil
}

func readVersion(dir string) string {
	b, err := os.ReadFile(filepath.Join(dir, VersionFile))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func norm(v string) string { return strings.TrimPrefix(strings.TrimSpace(v), "v") }

// versionMismatch describes why s is not this binary's template, or "".
func versionMismatch(s *Source) string {
	want := norm(BinaryVersion)
	if want == "" {
		return ""
	}
	if s.Version == "" {
		return fmt.Sprintf("the template at %s (%s) has no %s marker, so it predates version checks; this binary is v%s", s.Dir, s.How, VersionFile, want)
	}
	if norm(s.Version) != want {
		return fmt.Sprintf("the template at %s (%s) is version %s, but this binary is v%s", s.Dir, s.How, s.Version, want)
	}
	return ""
}

// TemplatePath returns the template directory (Resolve's).
func TemplatePath() (string, error) {
	s, err := Resolve()
	if err != nil {
		return "", err
	}
	return s.Dir, nil
}
