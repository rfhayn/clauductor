package panel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/clauductor/clauductor/internal/panel/state"
)

// The push command (PANEL-51 D7): the machine's push.json, {"argv": ["/abs/path",
// "arg", ...]}, run for each notice that carries a stalled alert with two more
// arguments, the title and the body, with no shell and under pushTimeout. It is the
// machine's, not a project's: a project's config is committed, and a push topic is
// personal. The machine reads it once for every project (pushWatch); each project's
// pollNotify takes the current command, or its refusal, at each tick.

// pushTimeout bounds one push.
const pushTimeout = 10 * time.Second

// pushCmd is push.json as last read: the argv to run, or why it was refused (a
// warning bar on every project's page), or neither when there is no file.
type pushCmd struct {
	argv    []string
	refused string
}

// pushPath is push.json's path: the machine's panel directory, beside notifier.json.
func pushPath(home string) string { return filepath.Join(config.PanelDir(home), "push.json") }

// readPush reads and judges push.json by D7's rules: refused when the file is not the
// user's, is writable by group or others, or sits in a folder that is (the folder of
// the path and of every link on the way to the file, pushChain); when its argv[0] is
// not an absolute path; and when it is anything but one JSON object with a non-empty
// argv of strings. No file: no command and no refusal. owner is the user the file must
// belong to (os.Getuid; a test stands in for a file another user owns, which it
// cannot create).
func readPush(path string, owner int) pushCmd {
	refuse := func(why string) pushCmd {
		return pushCmd{refused: "Push command " + path + " refused, so stalls are not pushed: " + why + "."}
	}
	fi, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return pushCmd{}
	}
	if err != nil {
		return refuse(err.Error())
	}
	if why := unsafeFile(fi, owner); why != "" {
		return refuse(why)
	}
	hops, err := pushChain(path)
	if err != nil {
		return refuse(err.Error())
	}
	for _, d := range pushDirs(hops) {
		di, err := os.Stat(d)
		if err != nil {
			return refuse(err.Error())
		}
		if di.Mode().Perm()&0o022 != 0 {
			return refuse("its folder " + d + " is writable by group or others")
		}
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return refuse(err.Error())
	}
	var raw map[string]json.RawMessage
	dec := json.NewDecoder(bytes.NewReader(b))
	if err := dec.Decode(&raw); err != nil || raw == nil {
		return refuse(`it is not one JSON object {"argv": ["/abs/path", ...]}`)
	}
	if _, err := dec.Token(); err != io.EOF {
		return refuse("it holds more than one JSON value")
	}
	var argv []string
	if err := json.Unmarshal(raw["argv"], &argv); err != nil || len(argv) == 0 {
		return refuse(`its "argv" is not a non-empty list of strings`)
	}
	if !filepath.IsAbs(argv[0]) {
		return refuse("its program (argv[0]) is not an absolute path")
	}
	return pushCmd{argv: argv}
}

// pushMaxHops bounds a chain of symbolic links at push.json's path.
const pushMaxHops = 8

// pushChain is every path on the way from push.json's path to the file it names: the
// path itself, then each link's target in turn, one hop at a time (os.Readlink), so a
// link in the middle of a chain is seen; EvalSymlinks would name only the end. An error
// when a hop cannot be read or the chain runs past pushMaxHops links.
func pushChain(path string) ([]string, error) {
	hops := []string{path}
	for p := path; ; {
		fi, err := os.Lstat(p)
		if err != nil {
			return hops, err
		}
		if fi.Mode()&fs.ModeSymlink == 0 {
			return hops, nil
		}
		if len(hops) > pushMaxHops {
			return hops, fmt.Errorf("it is a chain of more than %d symbolic links", pushMaxHops)
		}
		t, err := os.Readlink(p)
		if err != nil {
			return hops, err
		}
		if !filepath.IsAbs(t) {
			t = filepath.Join(filepath.Dir(p), t)
		}
		p = t
		hops = append(hops, p)
	}
}

// pushDirs are the folders a push file is judged by: the folder of every path on its
// chain (pushChain), each once. Anyone who can write one of them can point the chain
// at a file of theirs. readPush judges them and the push source's signature watches
// them, so the two never disagree on which they are.
func pushDirs(hops []string) []string {
	var dirs []string
	seen := map[string]bool{}
	for _, h := range hops {
		if d := filepath.Dir(h); !seen[d] {
			seen[d] = true
			dirs = append(dirs, d)
		}
	}
	return dirs
}

// unsafeFile says why a push file may not be trusted to name a program: not the
// user's, or writable by group or others. Empty: it may.
func unsafeFile(fi os.FileInfo, owner int) string {
	if !fi.Mode().IsRegular() {
		return "it is not a regular file"
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); !ok || int(st.Uid) != owner {
		return "it is not owned by you"
	}
	if fi.Mode().Perm()&0o022 != 0 {
		return "it is writable by group or others"
	}
	return ""
}

// pushWatch is the machine's reading of push.json: read at start, and again whenever
// its signature changes (a machine source checks it every Ticks.Push).
type pushWatch struct {
	path string
	sig  string // the push source's goroutine only, after start
	cur  atomic.Pointer[pushCmd]
}

func newPushWatch(home string) *pushWatch {
	p := &pushWatch{path: pushPath(home)}
	p.sig = p.signature()
	c := readPush(p.path, os.Getuid())
	p.cur.Store(&c)
	return p
}

// signature is the file's pathSignature, with the file's and each folder's it is judged
// by (pushDirs) mode and owner: a chmod changes neither size nor mtime, and it can
// make the file safe or unsafe. Every hop of a link chain is in it too, so a link
// pointed at another file is read again.
func (p *pushWatch) signature() string {
	sig := pathSignature(p.path)
	hops, _ := pushChain(p.path)
	sig += "|" + strings.Join(hops, ">")
	for _, path := range append([]string{p.path}, pushDirs(hops)...) {
		if fi, err := os.Stat(path); err == nil {
			sig += fmt.Sprintf("|%o", fi.Mode())
			if st, ok := fi.Sys().(*syscall.Stat_t); ok {
				sig += fmt.Sprintf(":%d", st.Uid)
			}
		}
	}
	return sig
}

// poll re-reads push.json when its signature has changed.
func (p *pushWatch) poll(context.Context, time.Time) (update, time.Duration) {
	if sig := p.signature(); sig != p.sig {
		p.sig = sig
		c := readPush(p.path, os.Getuid())
		p.cur.Store(&c)
	}
	return nil, 0
}

// current is the command as last read.
func (p *pushWatch) current() pushCmd {
	if p == nil {
		return pushCmd{} // a machine built without newMachine (a test's)
	}
	if c := p.cur.Load(); c != nil {
		return *c
	}
	return pushCmd{}
}

// runPush runs the push command with the title and body as two more arguments, with
// no shell, under timeout (pushTimeout; a test's shorter). Its error names the last
// line the program printed (lastLine), never its arguments: an argv can hold a token.
func runPush(ctx context.Context, timeout time.Duration, argv []string, title, body string) error {
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	args := append(append([]string(nil), argv[1:]...), title, body)
	cmd := exec.CommandContext(cctx, argv[0], args...)
	// A program killed at the timeout may leave a child holding its output open; past
	// this the pipes are closed and the wait returns.
	cmd.WaitDelay = time.Second
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	if errors.Is(cctx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("push: no answer within %s", timeout)
	}
	return fmt.Errorf("push: %v: %s", err, lastLine(out, 200))
}

// lastLine is the last line of out that is not blank, clipped to n runes: why a
// program failed, without what it printed before (a verbose request, a token in it),
// for the panel's log and the page's notifier error.
func lastLine(out []byte, n int) string {
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	return signals.Clip(strings.TrimSpace(lines[len(lines)-1]), n)
}

// pushNotice pushes a notice that carries a stalled alert (D7), its title and body
// built from that alert's own fields (state.PushText), never from the notice's Body.
// A notice with no stall, or no command, pushes nothing.
func pushNotice(ctx context.Context, c pushCmd, project string, n state.Notice, now time.Time) (bool, error) {
	if n.Stall == nil || len(c.argv) == 0 {
		return false, nil
	}
	title, body := state.PushText(project, *n.Stall, now)
	return true, runPush(ctx, pushTimeout, c.argv, signals.Clip(title, 80), signals.Clip(body, 240))
}
