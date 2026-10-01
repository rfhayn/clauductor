package install

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/signals"
)

// PANEL-23. The panel's hooks run in EVERY Claude Code session on the machine, so they
// must never show a message or delay a prompt, whatever is on the port. Claude Code
// runs only a command hook in the background ("Only type: command hooks support
// async: true"; an http hook "always runs synchronously") and "ignores the exit code
// and stdout/stderr from async hooks" (code.claude.com/docs/en/hooks). So the entry
// must be an async command hook, and its command must stay quiet and bounded on its
// own, because Claude Code does not enforce the timeout of an async hook.

// oldHTTPEntry is the hook object panels before PANEL-23 installed.
func oldHTTPEntry(port int) map[string]any {
	return map[string]any{"type": "http", "url": hookURL(port), "timeout": 1}
}

// installedHook returns the panel's hook object for ev as InstallHooks writes it.
func installedHook(t *testing.T, port int, ev string) map[string]any {
	t.Helper()
	home := t.TempDir()
	if _, err := InstallHooks(home, port); err != nil {
		t.Fatal(err)
	}
	hooks := readSettings(t, home)["hooks"].(map[string]any)
	for _, g := range hooks[ev].([]any) {
		for _, h := range g.(map[string]any)["hooks"].([]any) {
			raw, _ := json.Marshal(h)
			if isOurs(raw) {
				return h.(map[string]any)
			}
		}
	}
	t.Fatalf("no panel hook for %s", ev)
	return nil
}

// runHook runs a hook the way Claude Code runs a command hook (the command through
// a shell, the event's JSON on stdin) and returns its combined output, its exit
// error, and how long it ran. It fails the test unless the entry is a command hook
// Claude Code runs in the background: any other kind it waits on, and reports its
// failures, in every session.
func runHook(t *testing.T, h map[string]any, stdin string) (string, time.Duration, error) {
	t.Helper()
	if h["type"] != "command" || h["async"] != true {
		t.Fatalf("the panel hook is type %v, async %v: Claude Code waits on it and reports its failures in every session", h["type"], h["async"])
	}
	// The command is the panel's own (hookCommand); run through a shell as Claude Code
	// runs it. The deadline turns a hook that never ends into a failure, not a hang.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", h["command"].(string))
	cmd.WaitDelay = time.Second
	cmd.Stdin = strings.NewReader(stdin)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	start := time.Now()
	err := cmd.Run()
	return out.String(), time.Since(start), err
}

const hookEvent = `{"session_id":"s1","hook_event_name":"UserPromptSubmit","prompt":"it's \"quoted\" & <odd>"}`

// quiet asserts the hook said nothing, succeeded and ended within curl's bound.
func quiet(t *testing.T, out string, took time.Duration, err error) {
	t.Helper()
	if err != nil || out != "" {
		t.Fatalf("the hook failed or spoke: err=%v output=%q", err, out)
	}
	if took > 5*time.Second {
		t.Fatalf("the hook ran %v; it must end on its own (an async hook's timeout is not enforced)", took)
	}
}

func TestHookStatePanelUp(t *testing.T) {
	t.Parallel()
	got := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		w.WriteHeader(http.StatusNoContent)
		got <- r.Method + " " + r.URL.RequestURI() + " " + string(b)
	}))
	defer srv.Close()
	port := srv.Listener.Addr().(*net.TCPAddr).Port
	out, took, err := runHook(t, installedHook(t, port, "UserPromptSubmit"), hookEvent)
	quiet(t, out, took, err)
	select {
	case req := <-got:
		if want := "POST /hook?src=clauductor-panel " + hookEvent; req != want {
			t.Fatalf("the panel received %q, want %q", req, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the signal never reached the panel")
	}
}

func TestHookStatePanelHung(t *testing.T) {
	t.Parallel()
	// Accepts the connection, reads, and never answers.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() { _, _ = io.Copy(io.Discard, c); c.Close() }()
		}
	}()
	out, took, err := runHook(t, installedHook(t, ln.Addr().(*net.TCPAddr).Port, "Stop"), hookEvent)
	quiet(t, out, took, err)
}

func TestHookStatePanelDown(t *testing.T) {
	t.Parallel()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close() // connection refused from here on
	out, took, err := runHook(t, installedHook(t, port, "SessionEnd"), hookEvent)
	quiet(t, out, took, err)
}

func TestHookStatePortReusedBySomethingElse(t *testing.T) {
	t.Parallel()
	// Something else answers, slowly, with a body that would be a blocking decision
	// had Claude Code read it (for an HTTP hook it does: the prompt is blocked).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"decision":"block","reason":"not the panel","systemMessage":"NOT THE PANEL"}`))
	}))
	defer srv.Close()
	out, took, err := runHook(t, installedHook(t, srv.Listener.Addr().(*net.TCPAddr).Port, "UserPromptSubmit"), hookEvent)
	quiet(t, out, took, err)
}

// Every event gets the same quiet form, matcher events included.
func TestEveryEventGetsTheAsyncCommandForm(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	if _, err := InstallHooks(home, 4393); err != nil {
		t.Fatal(err)
	}
	want, _ := marshalRaw(hookEntry(4393))
	hooks := readSettings(t, home)["hooks"].(map[string]any)
	for _, ev := range signals.HookEvents {
		groups := hooks[ev].([]any)
		h, _ := json.Marshal(groups[0].(map[string]any)["hooks"].([]any)[0])
		if len(groups) != 1 || !sameJSON(h, want) {
			t.Errorf("%s: %s", ev, h)
		}
	}
}

// An install from before PANEL-23 is upgraded in place: the old HTTP entries go, the
// new ones come, the user's hooks and keys stay, and drift detection names it.
func TestOldHTTPHooksAreReplaced(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	old := map[string]any{}
	for _, ev := range signals.HookEvents {
		old[ev] = []any{map[string]any{"hooks": []any{oldHTTPEntry(4393)}}}
	}
	old["Stop"] = append(old["Stop"].([]any), map[string]any{"hooks": []any{map[string]any{"type": "command", "command": "say done"}}})
	b, _ := json.MarshalIndent(map[string]any{"model": "opus", "hooks": old}, "", "  ")
	writeFile(t, SettingsPath(home), string(b))

	d, err := readHookDrift(home, 4393)
	if err != nil || !strings.Contains(d.Text, "older form") || len(d.Ports) != 0 {
		t.Fatalf("the old form is not reported as drift: %+v %v", d, err)
	}
	if changed, err := InstallHooks(home, 4393); err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	after, _ := os.ReadFile(SettingsPath(home))
	if strings.Contains(string(after), `"type": "http"`) || !strings.Contains(string(after), `"say done"`) ||
		!strings.Contains(string(after), `"model": "opus"`) {
		t.Fatalf("not upgraded cleanly:\n%s", after)
	}
	for _, ev := range signals.HookEvents {
		if n := ourHooks(t, home)[ev]; n != 1 {
			t.Fatalf("%s: %d panel hooks after the upgrade", ev, n)
		}
	}
	if d, _ := readHookDrift(home, 4393); d.Text != "" {
		t.Fatalf("drift after the upgrade: %q", d.Text)
	}
	assertStopOrder(t, home)
}

// assertStopOrder fails unless Stop is still [the panel's hook, the user's "say done"]:
// the upgrade replaces the old entry where it was rather than moving it to the end.
func assertStopOrder(t *testing.T, home string) {
	t.Helper()
	stop := readSettings(t, home)["hooks"].(map[string]any)["Stop"].([]any)
	first, _ := json.Marshal(stop[0].(map[string]any)["hooks"].([]any)[0])
	last, _ := json.Marshal(stop[len(stop)-1])
	if len(stop) != 2 || !isOurs(first) || !strings.Contains(string(last), "say done") {
		t.Fatalf("Stop was reordered by the upgrade: %v", stop)
	}
}

// A panel hook that shares a group with a user hook keeps its relative place too: an
// old entry AFTER the user's hook is replaced by one after it, and one BEFORE by one
// before it.
func TestUpgradeKeepsPlaceAroundAMixedGroup(t *testing.T) {
	t.Parallel()
	user := map[string]any{"type": "command", "command": "say done"}
	for name, c := range map[string]struct {
		hooks     []any
		oursFirst bool
	}{
		"panel after":  {[]any{user, oldHTTPEntry(4393)}, false},
		"panel before": {[]any{oldHTTPEntry(4393), user}, true},
	} {
		home := t.TempDir()
		b, _ := json.Marshal(map[string]any{"hooks": map[string]any{"Stop": []any{
			map[string]any{"matcher": "", "hooks": c.hooks}}}})
		writeFile(t, SettingsPath(home), string(b))
		if _, err := InstallHooks(home, 4393); err != nil {
			t.Fatal(err)
		}
		stop := readSettings(t, home)["hooks"].(map[string]any)["Stop"].([]any)
		first, _ := json.Marshal(stop[0].(map[string]any)["hooks"].([]any)[0])
		if len(stop) != 2 || isOurs(first) != c.oursFirst {
			t.Errorf("%s: %v", name, stop)
		}
	}
}

// lookalikes are the user's own command hooks that merely CONTAIN the panel's URL, or
// one like it. Install (every 30 s) and uninstall edit the user's global settings, so
// claiming one of these would silently delete a user's hook.
var lookalikes = []string{
	"echo http://127.0.0.1:4393/hook?src=clauductor-panel-mine",
	"my-logger.sh; curl -d @- http://127.0.0.1:4393/hook?src=clauductor-panel",
	"echo see http://127.0.0.1:4393/hook?src=clauductor-panel-ish",
	hookCommand(4393) + " # mine",
}

func lookalikeHooks() []any {
	out := []any{map[string]any{"type": "http", "url": "http://127.0.0.1:9000/hook?src=someone-else"}}
	for _, c := range lookalikes {
		out = append(out, map[string]any{"type": "command", "command": c})
	}
	return out
}

// assertLookalikesKept fails unless every lookalike hook is still in settings.json.
func assertLookalikesKept(t *testing.T, home, what string) {
	t.Helper()
	hooks := readSettings(t, home)["hooks"].(map[string]any)
	got := map[string]bool{}
	for _, g := range hooks["PreToolUse"].([]any) {
		for _, h := range g.(map[string]any)["hooks"].([]any) {
			if c, ok := h.(map[string]any)["command"].(string); ok {
				got[c] = true
			}
		}
	}
	for _, c := range lookalikes {
		if !got[c] {
			t.Errorf("%s deleted the user's hook %q", what, c)
		}
	}
}

func TestLookalikesAreNotOurs(t *testing.T) {
	t.Parallel()
	for _, c := range lookalikes {
		raw, _ := json.Marshal(map[string]any{"type": "command", "command": c})
		if isOurs(raw) {
			t.Errorf("claimed as the panel's: %q", c)
		}
	}
	for _, h := range []map[string]any{hookEntry(4393), hookEntry(1), oldHTTPEntry(4393)} {
		raw, _ := json.Marshal(h)
		if !isOurs(raw) {
			t.Errorf("not recognised as the panel's: %s", raw)
		}
	}
}

func TestInstallKeepsLookalikeHooks(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	b, _ := json.Marshal(map[string]any{"hooks": map[string]any{"PreToolUse": []any{map[string]any{"hooks": lookalikeHooks()}}}})
	writeFile(t, SettingsPath(home), string(b))
	if _, err := InstallHooks(home, 4393); err != nil {
		t.Fatal(err)
	}
	assertLookalikesKept(t, home, "install")
}

// Uninstall removes the panel's hooks in both forms and nothing else.
func TestUninstallRemovesBothForms(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	settings := map[string]any{"hooks": map[string]any{
		"Stop":             []any{map[string]any{"hooks": []any{oldHTTPEntry(4393), map[string]any{"type": "command", "command": "say done"}}}},
		"UserPromptSubmit": []any{map[string]any{"hooks": []any{hookEntry(4393)}}},
		"Notification":     []any{map[string]any{"hooks": []any{oldHTTPEntry(5000)}}, map[string]any{"hooks": []any{hookEntry(5001)}}},
		// Not ours: another tool's URL, and user commands that merely contain the URL.
		"PreToolUse": []any{map[string]any{"hooks": lookalikeHooks()}},
	}}
	b, _ := json.Marshal(settings)
	writeFile(t, SettingsPath(home), string(b))
	if changed, err := UninstallHooks(home); err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	after, _ := os.ReadFile(SettingsPath(home))
	s := string(after)
	if n := ourHooks(t, home); len(n) != 0 {
		t.Fatalf("panel hooks survived uninstall (%v):\n%s", n, s)
	}
	for _, url := range []string{"127.0.0.1:5000", "127.0.0.1:5001"} {
		if strings.Contains(s, url) {
			t.Fatalf("another panel's hook (%s) survived uninstall:\n%s", url, s)
		}
	}
	for _, want := range []string{`"say done"`, "src=someone-else"} {
		if !strings.Contains(s, want) {
			t.Errorf("uninstall removed a foreign hook (%s):\n%s", want, s)
		}
	}
	assertLookalikesKept(t, home, "uninstall")
	hooks := readSettings(t, home)["hooks"].(map[string]any)
	for _, ev := range []string{"UserPromptSubmit", "Notification"} {
		if _, ok := hooks[ev]; ok {
			t.Errorf("%s left behind", ev)
		}
	}
}

// The drift check reads the address out of either form, so the keeper still finds
// another panel's hooks (and leaves a live one alone) after the change of form.
func TestDriftFindsAnotherPanelInEitherForm(t *testing.T) {
	t.Parallel()
	for name, entry := range map[string]map[string]any{"http": oldHTTPEntry(4400), "command": hookEntry(4400)} {
		home := t.TempDir()
		hooks := map[string]any{}
		for _, ev := range signals.HookEvents {
			hooks[ev] = []any{map[string]any{"hooks": []any{entry}}}
		}
		b, _ := json.Marshal(map[string]any{"hooks": hooks})
		writeFile(t, SettingsPath(home), string(b))
		d, err := readHookDrift(home, 4393)
		if err != nil || len(d.Ports) != 1 || d.Ports[0] != 4400 || !strings.Contains(d.Text, strconv.Itoa(4400)) {
			t.Errorf("%s: %+v %v", name, d, err)
		}
	}
}
