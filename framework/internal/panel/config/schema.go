package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

//go:generate go test -run TestGeneratedFilesAreCurrent -update .

// jsonSchema generates docs/panel.schema.json: the shape of Config by reflection,
// and each key's text, constraints and version from Fields.
func jsonSchema() ([]byte, error) {
	fields := fieldByPath()
	root, err := schemaFor(reflect.TypeOf(Config{}), "", fields)
	if err != nil {
		return nil, err
	}
	out := map[string]any{
		"$schema":     "https://json-schema.org/draft/2020-12/schema",
		"$id":         SchemaURL,
		"title":       "clauductor panel config (.clauductor/panel.json)",
		"description": "Generated from the Go config types by `go generate ./internal/panel/config`; do not edit. The reference is docs/panel.md.",
	}
	for k, v := range root {
		out[k] = v
	}
	// The version gate: a file that declares version v may not set a key from a
	// later version. A nested key newer than its parent (templates[].suggest) is
	// banned where it sits, inside its parent's items or properties; its own
	// children go with it.
	var gates []any
	for v := MinVersion; v < LatestVersion; v++ {
		banned := map[string]any{}
		for _, f := range Fields {
			if f.Version > v && parentVersion(fields, f.Path) <= v {
				mergeSchema(banned, banAt(f.Path))
			}
		}
		if len(banned) == 0 {
			continue
		}
		gates = append(gates, map[string]any{
			"if":   map[string]any{"properties": map[string]any{"version": map[string]any{"const": v}}, "required": []string{"version"}},
			"then": banned,
		})
	}
	if len(gates) > 0 {
		out["allOf"] = gates
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// parentPath is the key a nested key sits in ("templates" for "templates[].suggest",
// "lane_types" for "lane_types.*.model"); "" for a top-level key.
func parentPath(p string) string {
	i := strings.LastIndexAny(p, ".[")
	if i <= 0 {
		return ""
	}
	return strings.TrimSuffix(strings.TrimSuffix(p[:i], ".*"), "[]")
}

// parentVersion is the version of the key p sits in (0 at the top level).
func parentVersion(fields map[string]Field, p string) int {
	if f, ok := fields[parentPath(p)]; ok {
		return f.Version
	}
	return 0
}

// banAt is a schema fragment refusing the key at path p: "templates[].suggest"
// becomes {"properties": {"templates": {"items": {"properties": {"suggest": false}}}}}.
func banAt(p string) map[string]any {
	var cur any = false
	segs := strings.Split(p, ".")
	for i := len(segs) - 1; i >= 0; i-- {
		if segs[i] == "*" {
			cur = map[string]any{"additionalProperties": cur}
			continue
		}
		name := segs[i]
		for strings.HasSuffix(name, "[]") {
			name = strings.TrimSuffix(name, "[]")
			cur = map[string]any{"items": cur}
		}
		cur = map[string]any{"properties": map[string]any{name: cur}}
	}
	return cur.(map[string]any)
}

func schemaFor(t reflect.Type, path string, fields map[string]Field) (map[string]any, error) {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Struct:
		props := map[string]any{}
		var required []string
		for i := 0; i < t.NumField(); i++ {
			sf := t.Field(i)
			name := jsonName(sf)
			if name == "" {
				continue
			}
			p := joinPath(path, name)
			f, ok := fields[p]
			if !ok {
				return nil, fmt.Errorf("config key %q has no entry in Fields", p)
			}
			s, err := schemaFor(sf.Type, p, fields)
			if err != nil {
				return nil, err
			}
			mergeSchema(s, f.Schema)
			if f.Match != nil {
				s["pattern"] = f.Match.String()
			}
			s["description"] = f.Doc
			if f.Default != nil {
				s["default"] = f.Default
			}
			props[name] = s
			if f.Required {
				required = append(required, name)
			}
		}
		s := map[string]any{"type": "object", "additionalProperties": false, "properties": props}
		if len(required) > 0 {
			s["required"] = required
		}
		return s, nil
	case reflect.Map:
		elem, err := schemaFor(t.Elem(), path+".*", fields)
		if err != nil {
			return nil, err
		}
		return map[string]any{"type": "object", "additionalProperties": elem}, nil
	case reflect.Slice:
		elem, err := schemaFor(t.Elem(), path+"[]", fields)
		if err != nil {
			return nil, err
		}
		return map[string]any{"type": "array", "items": elem}, nil
	case reflect.String:
		return map[string]any{"type": "string"}, nil
	case reflect.Int, reflect.Int64:
		return map[string]any{"type": "integer"}, nil
	case reflect.Float64:
		return map[string]any{"type": "number"}, nil
	case reflect.Bool:
		return map[string]any{"type": "boolean"}, nil
	}
	return nil, fmt.Errorf("config key %q: no schema for Go kind %s", path, t.Kind())
}

// mergeSchema adds extra keywords to s, merging into a keyword that is itself a
// schema (items, additionalProperties) rather than replacing it.
func mergeSchema(s, extra map[string]any) {
	for k, v := range extra {
		if em, ok := v.(map[string]any); ok {
			if sm, ok := s[k].(map[string]any); ok {
				mergeSchema(sm, em)
				continue
			}
		}
		s[k] = v
	}
}

// referenceTable generates the configuration reference table in docs/panel.md.
func referenceTable() string {
	var b strings.Builder
	b.WriteString("| Key | Type | Default | Since | Meaning |\n|---|---|---|---|---|\n")
	for _, f := range Fields {
		typ := f.Type
		if f.Required {
			typ += ", **required**"
		}
		def := ""
		if f.Default != nil {
			j, _ := json.Marshal(f.Default)
			def = "`" + string(j) + "`"
		}
		doc := f.Doc
		if f.Match != nil {
			doc += " Matches `" + f.Match.String() + "`."
		}
		fmt.Fprintf(&b, "| `%s` | %s | %s | %d | %s |\n", displayPath(f.Path), typ, def, f.Version, doc)
	}
	return b.String()
}
