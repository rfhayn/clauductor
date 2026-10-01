package install

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/clauductor/clauductor/internal/panel/config"
)

// Remote control (PANEL-19): Claude Code's Remote Control lets a session be driven
// from claude.ai/code or the Claude app, on any device signed in to the account,
// permission prompts included. `panel install` asks once where it should be on:
//
//	all    every Claude session on this Mac: "remoteControlAtStartup": true in
//	       ~/.claude/settings.json (Claude Code's own setting)
//	lanes  only the panel's lanes: each starts with `claude --remote-control`
//	off    neither
//
// The choice is the machine's, kept in ~/.clauductor/panel/remote-control.json (a file
// of its own, so a panel of another version reading projects.json never meets a key
// it does not know). An explicit remoteControlAtStartup, true or false, is the
// user's own choice: install never asks over it and never changes it unless told to
// with --remote-control=all.

// Remote control modes.
const (
	RemoteAll   = "all"
	RemoteLanes = "lanes"
	RemoteOff   = "off"
)

const remoteKey = "remoteControlAtStartup"

// RemoteControlPath is the machine's remote-control choice.
func RemoteControlPath(home string) string {
	return filepath.Join(config.PanelDir(home), "remote-control.json")
}

type remoteFile struct {
	Mode string `json:"mode"`
}

// RemoteControlMode is the recorded choice ("" when none was made).
func RemoteControlMode(home string) string {
	b, err := os.ReadFile(RemoteControlPath(home))
	if err != nil {
		return ""
	}
	var f remoteFile
	if json.Unmarshal(b, &f) != nil {
		return ""
	}
	switch f.Mode {
	case RemoteAll, RemoteLanes, RemoteOff:
		return f.Mode
	}
	return ""
}

func setRemoteControlMode(home, mode string) error {
	b, _ := json.Marshal(remoteFile{Mode: mode})
	if err := config.EnsurePrivateDir(config.PanelDir(home)); err != nil {
		return err
	}
	return config.WriteAtomic(RemoteControlPath(home), b, 0o600)
}

// LanesRemoteControl reports whether the panel's lanes start with --remote-control.
func LanesRemoteControl(home string) bool { return RemoteControlMode(home) == RemoteLanes }

// RemoteAtStartup reads remoteControlAtStartup from ~/.claude/settings.json: its raw
// JSON value, and whether the key is there at all.
func RemoteAtStartup(home string) (string, bool) {
	b, err := os.ReadFile(SettingsPath(home))
	if err != nil {
		return "", false
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(b, &m) != nil {
		return "", false
	}
	v, ok := m[remoteKey]
	return strings.TrimSpace(string(v)), ok
}

// RemoteControlSummary says where remote control is on, for `panel list` and the page.
func RemoteControlSummary(home string) string {
	if v, ok := RemoteAtStartup(home); ok && v == "true" {
		return RemoteAll
	}
	if LanesRemoteControl(home) {
		return RemoteLanes
	}
	return RemoteOff
}

// setRemoteAtStartup merges "remoteControlAtStartup": true into settings.json through
// the hooks' own read-merge-write: every other key keeps its bytes and its order.
func setRemoteAtStartup(home string) (bool, error) {
	return rewriteSettings(home, func(root *orderedObject) error {
		root.set(remoteKey, json.RawMessage("true"))
		return nil
	})
}

// RemoteOptions is how `panel install` chooses.
type RemoteOptions struct {
	Home string
	// Flag is --remote-control: all, lanes or off; "" asks (on a terminal only).
	Flag string
	In   io.Reader
	Out  io.Writer
	// Interactive: stdin is a terminal, so a question can be answered.
	Interactive bool
}

// ChooseRemoteControl runs the install's remote-control step and returns the mode it
// leaves in force. It asks at most once: only when settings.json has no
// remoteControlAtStartup, no choice is recorded, no flag was given, and stdin is a
// terminal. With no terminal and no flag it chooses off, without asking.
func ChooseRemoteControl(o RemoteOptions) (string, error) {
	out := o.Out
	if out == nil {
		out = io.Discard
	}
	settings := SettingsPath(o.Home)
	mode := strings.TrimSpace(strings.ToLower(o.Flag))
	if mode != "" && mode != RemoteAll && mode != RemoteLanes && mode != RemoteOff {
		return "", fmt.Errorf("--remote-control must be all, lanes or off, not %q", o.Flag)
	}
	cur, explicit := RemoteAtStartup(o.Home)
	if mode == "" {
		switch {
		case explicit:
			fmt.Fprintf(out, "Remote control: %s already sets %s to %s; left as it is.\n", settings, remoteKey, cur)
			return RemoteControlSummary(o.Home), nil
		case RemoteControlMode(o.Home) != "":
			m := RemoteControlMode(o.Home)
			fmt.Fprintf(out, "Remote control: %s (chosen before; --remote-control=all|lanes|off changes it).\n", m)
			return m, nil
		case !o.Interactive:
			mode = RemoteOff
			fmt.Fprintf(out, "Remote control: off (no terminal to ask on; --remote-control=all|lanes chooses it).\n")
		default:
			var err error
			if mode, err = askRemoteControl(o.In, out); err != nil {
				return "", err
			}
		}
	}
	switch mode {
	case RemoteAll:
		changed, err := setRemoteAtStartup(o.Home)
		if err != nil {
			return "", err
		}
		if changed {
			fmt.Fprintf(out, "Remote control: set \"%s\": true in %s (was %s); every other key is unchanged.\n", remoteKey, settings, orAbsent(cur, explicit))
		} else {
			fmt.Fprintf(out, "Remote control: %s already has \"%s\": true.\n", settings, remoteKey)
		}
		fmt.Fprintf(out, "  Every new interactive Claude session on this Mac now connects to Remote Control.\n"+
			"  To undo: remove the \"%s\" line from %s, or set it to false (or /config in claude,\n"+
			"  \"Enable Remote Control for all sessions\"). A backup from before the panel's first change is %s.clauductor-panel.bak.\n", remoteKey, settings, settings)
	case RemoteLanes:
		fmt.Fprintf(out, "Remote control: the panel's lanes start with `claude --remote-control` (recorded in %s).\n"+
			"  Lanes started before keep running without it; restart one to connect it. settings.json is unchanged.\n", RemoteControlPath(o.Home))
	case RemoteOff:
		if explicit && cur == "true" {
			fmt.Fprintf(out, "Remote control: off for the panel, but %s still sets %s to true, so every session connects; set it to false to stop that.\n", settings, remoteKey)
		}
	}
	if err := setRemoteControlMode(o.Home, mode); err != nil {
		return "", err
	}
	return mode, nil
}

func orAbsent(v string, ok bool) string {
	if !ok {
		return "absent"
	}
	return v
}

func askRemoteControl(in io.Reader, out io.Writer) (string, error) {
	fmt.Fprint(out, `Remote Control lets you drive a Claude session from claude.ai/code or the Claude app on
any device signed in to your account, answering its permission prompts included. Where should it be on?
  1) Every Claude session on this Mac (sets remoteControlAtStartup in ~/.claude/settings.json)
  2) Only the panel's lanes (lanes start with --remote-control)
  3) Not now
Choose 1, 2 or 3 [3]: `)
	sc := bufio.NewScanner(in)
	for {
		if !sc.Scan() {
			fmt.Fprintln(out)
			return RemoteOff, nil // end of input: the safe answer
		}
		switch strings.TrimSpace(sc.Text()) {
		case "1":
			return RemoteAll, nil
		case "2":
			return RemoteLanes, nil
		case "", "3":
			return RemoteOff, nil
		}
		fmt.Fprint(out, "Choose 1, 2 or 3: ")
	}
}
