package config

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
)

// Config versions. A config declares the version it is written for, and may use
// only the keys of that version or an earlier one:
//
//	1  name, lanes, cards, and the keys of lanes the panel starts itself
//	   (tmux_socket, worktree_dir, base, lane_types)
//	2  orchestration: templates, queues, alerts, quota_guard, host_names
//
// A config with no "version" is read as LatestVersion, and the panel says once that
// it should declare one.
const (
	MinVersion    = 1
	LatestVersion = 2
)

// SchemaURL is the published JSON Schema of panel.json (docs/panel.schema.json);
// `clauductor panel init` writes it as "$schema" so editors validate the file.
// The repository has no release tags yet, so it names main: the schema of the
// newest panel. Pin it to a tag's path once releases are tagged.
const SchemaURL = "https://raw.githubusercontent.com/rfhayn/clauductor/main/docs/panel.schema.json"

// Field documents one key of panel.json. Fields is the ONE table of them: the version
// gate, the JSON Schema (docs/panel.schema.json) and the reference table in
// docs/panel.md are all generated from it, and version_test checks that it names
// every JSON key of Config (found by reflection) and nothing else.
type Field struct {
	// Path is the key's place in the file: "name", "cards[].id",
	// "lane_types.*.model" ("*" is any key of a map, "[]" any element of a list).
	Path string
	// Version is the lowest config version that may use the key.
	Version int
	// Type is the type as the reference table shows it.
	Type     string
	Required bool
	// Default is the value an absent key takes (nil: none, or nothing to show).
	Default any
	// Doc is the reference text: Markdown, one paragraph.
	Doc string
	// Match is the validator's own regexp for a string key: the schema's "pattern"
	// and the reference's "Matches", so neither can drift from what Validate enforces.
	Match *regexp.Regexp
	// Schema holds extra JSON Schema keywords (minimum, items, …).
	Schema map[string]any
}

// pattern turns a validator's own regexp into a schema "pattern".
func pattern(re *regexp.Regexp) map[string]any { return map[string]any{"pattern": re.String()} }

// Fields is every key of panel.json, in the order the reference shows them.
var Fields = []Field{
	{Path: "$schema", Version: 1, Type: "string",
		Doc: "The JSON Schema the file follows, for editors: `" + SchemaURL + "`. The panel ignores it. `clauductor panel init` writes it."},
	{Path: "version", Version: 1, Type: "integer: 1 or 2",
		Doc:    "The config version the file is written for. It may use only the keys of that version or an earlier one; a key from a later version is an error that names the key and the version it needs. Without it the file is read as the latest version, and the panel says so once at start.",
		Schema: map[string]any{"enum": []int{1, 2}}},
	{Path: "name", Version: 1, Type: "string", Required: true,
		Doc:   "Shown in the status bar and in notification titles. One line of plain text, at most 80 characters, not blank and not starting with `-`.",
		Match: nameRe, Schema: map[string]any{"minLength": 1, "maxLength": 80}},
	{Path: "lanes", Version: 1, Type: "object: branch rule → lane type",
		Doc:    "A rule ending in `/` is a prefix (`\"feature/\"` matches `feature/add-x`, shown as `add-x`). A rule ending in `*` is a prefix without the star (`\"feature/spike-*\"`). Any other rule matches one branch exactly (`\"main\"`). The longest matching rule wins. An unmatched branch is `other`; a detached HEAD is `detached`.",
		Schema: map[string]any{"propertyNames": map[string]any{"minLength": 1, "pattern": `\S`}, "additionalProperties": map[string]any{"type": "string", "minLength": 1, "pattern": `\S`}}},
	{Path: "cards", Version: 1, Type: "array",
		Doc: "Commands whose output renders as a card in the Activity drawer (see *Card output*)."},
	{Path: "cards[].id", Version: 1, Type: "string", Required: true,
		Doc: "Unique among the cards.", Match: cardIDRe},
	{Path: "cards[].title", Version: 1, Type: "string",
		Doc: "The card's heading."},
	{Path: "cards[].command", Version: 1, Type: "array of strings", Required: true,
		Doc:    "argv, run in the project root **without a shell**. Use `[\"sh\", \"-c\", \"...\"]` if you want one. 30-second timeout.",
		Schema: map[string]any{"minItems": 1}},
	{Path: "cards[].refresh", Version: 1, Type: "string", Required: true,
		Doc:   "`\"watch:<relpath>\"`: re-run when that file (or a direct entry of that directory) changes; the path must stay inside the project. `\"interval:<seconds>\"`: re-run on a timer (minimum 5 s). Every card also runs at start and on ↻ REFRESH.",
		Match: refreshRe},
	{Path: "tmux_socket", Version: 1, Type: "string", Default: DefaultTmuxSocket,
		Doc: "The panel's own tmux server (`tmux -L <name>`). Lanes never mix with your own tmux sessions.", Match: SocketNameRe},
	{Path: "worktree_dir", Version: 1, Type: "string", Default: DefaultWorktreeDir,
		Doc: "Where a new lane's worktree is created: relative to the project root and inside it, or absolute."},
	{Path: "base", Version: 1, Type: "string", Default: DefaultBase,
		Doc: "What a new lane's branch starts from. `git fetch` runs first; if it fails, the lane still starts and the page says so.", Match: baseRe},
	{Path: "lane_types", Version: 1, Type: "object: lane type → options",
		Doc:    "Launch options per lane type, passed as `claude --model <m> --effort <e>`.",
		Schema: map[string]any{"propertyNames": map[string]any{"minLength": 1, "pattern": `\S`}}},
	{Path: "lane_types.*.model", Version: 1, Type: "string",
		Doc: "One argv element: `--model <value>`.", Match: LaunchOptRe},
	{Path: "lane_types.*.effort", Version: 1, Type: "string",
		Doc: "One argv element: `--effort <value>`.", Match: LaunchOptRe},
	{Path: "templates", Version: 2, Type: "array",
		Doc: "Lane recipes offered by **New lane** (see *Lane templates*)."},
	{Path: "templates[].id", Version: 2, Type: "string", Required: true,
		Doc: "Unique among the templates.", Match: cardIDRe},
	{Path: "templates[].title", Version: 2, Type: "string",
		Doc: "Shown in the dialog."},
	{Path: "templates[].lane_type", Version: 2, Type: "string", Required: true,
		Doc: "One of the config's lane types (a value of `lanes`, or a key of `lane_types`)."},
	{Path: "templates[].branch_pattern", Version: 2, Type: "string",
		Doc: "The new branch, with `{name}` (required) and `{issue}`, e.g. `\"feature/{name}\"`. Empty means the lane type's prefix + `{name}`; a lane type with no prefix rule needs one."},
	{Path: "templates[].first_prompt", Version: 2, Type: "string", Required: true,
		Doc:    "ONE line typed into claude once it is ready. `{name}` and `{issue}` only; no newline or control character; at most 4000 characters.",
		Schema: map[string]any{"minLength": 1, "maxLength": MaxFirstPrompt}},
	{Path: "templates[].model", Version: 2, Type: "string",
		Doc: "Overrides the lane type's model.", Match: LaunchOptRe},
	{Path: "templates[].effort", Version: 2, Type: "string",
		Doc: "Overrides the lane type's effort.", Match: LaunchOptRe},
	{Path: "queues", Version: 2, Type: "array",
		Doc: "Shared resources held as a lease on disk (see *Queue and the gate lock protocol*)."},
	{Path: "queues[].id", Version: 2, Type: "string", Required: true,
		Doc: "Unique among the queues.", Match: cardIDRe},
	{Path: "queues[].title", Version: 2, Type: "string",
		Doc: "Shown on the queue card."},
	{Path: "queues[].lock", Version: 2, Type: "string", Required: true,
		Doc:    "The lease directory, relative to the **git common dir** (so every worktree agrees), e.g. `\"clauductor/gate.lock\"`. No `..`, not absolute.",
		Schema: map[string]any{"pattern": lockPathRe.String(), "not": map[string]any{"pattern": `\.\.`}}},
	{Path: "queues[].command", Version: 2, Type: "array of strings",
		Doc:    "Optional argv that **Run in `<lane>`** (a lane's Gate tab) starts through `lock-run` in that lane's worktree. It runs only when you press it.",
		Schema: map[string]any{"minItems": 1}},
	{Path: "alerts", Version: 2, Type: "object",
		Doc: "Alert thresholds (see *Alerts*). A missing key takes the default; `0` turns that alert off."},
	{Path: "alerts.idle_minutes", Version: 2, Type: "number", Default: DefaultIdleMinutes,
		Doc: "A live session idle longer than this raises an idle alert.", Schema: map[string]any{"minimum": 0}},
	{Path: "alerts.context_pct", Version: 2, Type: "number", Default: DefaultContextPct,
		Doc: "A context window at or above this percentage raises a context alert.", Schema: map[string]any{"minimum": 0}},
	{Path: "alerts.five_hour_pct", Version: 2, Type: "number", Default: DefaultFiveHourPct,
		Doc: "The 5-hour quota at or above this percentage raises a quota alert (block at 100%).", Schema: map[string]any{"minimum": 0}},
	{Path: "alerts.waiting_seconds", Version: 2, Type: "number", Default: DefaultWaitingSeconds,
		Doc: "A permission prompt, MCP elicitation or input request older than this raises a waiting alert.", Schema: map[string]any{"minimum": 0}},
	{Path: "alerts.notify", Version: 2, Type: "boolean", Default: true,
		Doc: "Send macOS notifications for the alerts that interrupt."},
	{Path: "alerts.min_interval_seconds", Version: 2, Type: "number", Default: DefaultNotifyInterval,
		Doc: "At most one notification per lane per interval.", Schema: map[string]any{"minimum": 0}},
	{Path: "quota_guard", Version: 2, Type: "object",
		Doc: "Refuses to start or restore a lane at or above a 5-hour quota (see *Quota guard*)."},
	{Path: "quota_guard.five_hour_pct", Version: 2, Type: "number", Default: DefaultQuotaGuardPct,
		Doc: "Refuse at or above this 5-hour quota, unless the dialog's override is ticked. `0` turns it off.", Schema: map[string]any{"minimum": 0, "maximum": 100}},
	{Path: "host_names", Version: 2, Type: "array of strings",
		Doc:    "Extra names the panel answers to, each `<label>.localhost` in lower case (for example `\"myproject.localhost\"`). `clauductor.localhost` always works. No wildcards.",
		Schema: map[string]any{"items": pattern(localhostNameRe)}},
}

// fieldByPath indexes Fields.
func fieldByPath() map[string]Field {
	m := make(map[string]Field, len(Fields))
	for _, f := range Fields {
		m[f.Path] = f
	}
	return m
}

// jsonName is a struct field's JSON key, or "" if it has none.
func jsonName(sf reflect.StructField) string {
	if !sf.IsExported() {
		return ""
	}
	tag := sf.Tag.Get("json")
	if tag == "-" {
		return ""
	}
	name, _, _ := strings.Cut(tag, ",")
	if name == "" {
		return sf.Name
	}
	return name
}

func joinPath(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}

// configPaths walks a type the way encoding/json reads it and reports every key
// path, parents first. It is the authority Fields is checked against.
func configPaths(t reflect.Type, prefix string, fn func(path string, t reflect.Type)) {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			sf := t.Field(i)
			name := jsonName(sf)
			if name == "" {
				continue
			}
			p := joinPath(prefix, name)
			fn(p, sf.Type)
			configPaths(sf.Type, p, fn)
		}
	case reflect.Map:
		configPaths(t.Elem(), prefix+".*", fn)
	case reflect.Slice:
		configPaths(t.Elem(), prefix+"[]", fn)
	}
}

// keysUsed walks decoded JSON beside the type it decodes into and reports the key
// path of every key the file sets, in a stable order.
func keysUsed(v any, t reflect.Type, prefix string, fn func(path string)) {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Struct:
		obj, ok := v.(map[string]any)
		if !ok {
			return
		}
		for i := 0; i < t.NumField(); i++ {
			sf := t.Field(i)
			name := jsonName(sf)
			if name == "" {
				continue
			}
			if sub, ok := obj[name]; ok {
				p := joinPath(prefix, name)
				fn(p)
				keysUsed(sub, sf.Type, p, fn)
			}
		}
	case reflect.Map:
		obj, ok := v.(map[string]any)
		if !ok {
			return
		}
		keys := make([]string, 0, len(obj))
		for k := range obj {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			keysUsed(obj[k], t.Elem(), prefix+".*", fn)
		}
	case reflect.Slice:
		arr, ok := v.([]any)
		if !ok {
			return
		}
		for _, e := range arr {
			keysUsed(e, t.Elem(), prefix+"[]", fn)
		}
	}
}

// versionNotice is what the panel says once when a config declares no version.
func versionNotice() string {
	return fmt.Sprintf("panel config: no \"version\" key, so it is read as version %d (the latest). Add \"version\": %d to the file: "+
		"a later panel will read an undeclared config as its own latest version.", LatestVersion, LatestVersion)
}

// checkVersion enforces the version gate on the raw file: the declared version is
// supported, and every key the file sets belongs to that version or an earlier one.
// It returns the notices to print once (a missing version).
func checkVersion(raw []byte, c *Config) ([]string, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		return nil, fmt.Errorf("panel config: %w", err)
	}
	var notices []string
	declared := c.Version
	if _, ok := top["version"]; !ok {
		declared = LatestVersion
		notices = append(notices, versionNotice())
	} else if c.Version < MinVersion || c.Version > LatestVersion {
		return nil, fmt.Errorf("panel config: version %d is not supported (this panel reads %d to %d)", c.Version, MinVersion, LatestVersion)
	}
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("panel config: %w", err)
	}
	fields := fieldByPath()
	var gateErr error
	keysUsed(doc, reflect.TypeOf(Config{}), "", func(path string) {
		f, ok := fields[path]
		if gateErr != nil || !ok || f.Version <= declared {
			return
		}
		gateErr = fmt.Errorf("panel config: %q needs \"version\": %d or later (the file declares version %d); raise the version, or remove the key",
			displayPath(path), f.Version, declared)
	})
	return notices, gateErr
}

// displayPath shows a map wildcard as the reference does: lane_types.<key>.model.
func displayPath(p string) string { return strings.ReplaceAll(p, ".*", ".<key>") }
