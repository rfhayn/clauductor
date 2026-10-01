package template

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// node is a JSON value that keeps its object keys in file order, so a merged settings.json reads
// like the project's own file plus the template's additions, and a diff of it shows only what
// changed.
type node struct {
	obj    bool
	arr    bool
	keys   []string
	fields map[string]*node
	items  []*node
	raw    json.RawMessage // scalars (string, number, bool, null), compacted
}

func parseNode(data []byte) (*node, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	n, err := decodeNode(dec)
	if err != nil {
		return nil, err
	}
	if dec.More() {
		return nil, fmt.Errorf("trailing data after the JSON value")
	}
	return n, nil
}

func decodeNode(dec *json.Decoder) (*node, error) {
	t, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch v := t.(type) {
	case json.Delim:
		switch v {
		case '{':
			n := &node{obj: true, fields: map[string]*node{}}
			for dec.More() {
				kt, err := dec.Token()
				if err != nil {
					return nil, err
				}
				k := kt.(string)
				c, err := decodeNode(dec)
				if err != nil {
					return nil, err
				}
				if _, dup := n.fields[k]; !dup {
					n.keys = append(n.keys, k)
				}
				n.fields[k] = c
			}
			_, err := dec.Token() // }
			return n, err
		case '[':
			n := &node{arr: true}
			for dec.More() {
				c, err := decodeNode(dec)
				if err != nil {
					return nil, err
				}
				n.items = append(n.items, c)
			}
			_, err := dec.Token() // ]
			return n, err
		}
		return nil, fmt.Errorf("unexpected %v", v)
	default:
		b, err := marshalNoEscape(v)
		if err != nil {
			return nil, err
		}
		return &node{raw: b}, nil
	}
}

func marshalNoEscape(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSpace(b.Bytes()), nil
}

func strNode(s string) *node {
	b, _ := marshalNoEscape(s)
	return &node{raw: b}
}

// str returns a string scalar's value.
func (n *node) str() (string, bool) {
	if n == nil || n.obj || n.arr || len(n.raw) == 0 || n.raw[0] != '"' {
		return "", false
	}
	var s string
	if json.Unmarshal(n.raw, &s) != nil {
		return "", false
	}
	return s, true
}

func (n *node) get(k string) *node {
	if n == nil || !n.obj {
		return nil
	}
	return n.fields[k]
}

func (n *node) set(k string, v *node) {
	if _, ok := n.fields[k]; !ok {
		n.keys = append(n.keys, k)
	}
	n.fields[k] = v
}

func (n *node) clone() *node {
	if n == nil {
		return nil
	}
	c := &node{obj: n.obj, arr: n.arr, raw: n.raw}
	if n.obj {
		c.fields = map[string]*node{}
		c.keys = append([]string(nil), n.keys...)
		for k, v := range n.fields {
			c.fields[k] = v.clone()
		}
	}
	for _, it := range n.items {
		c.items = append(c.items, it.clone())
	}
	return c
}

// compact is the value as one-line JSON, which is also its identity for comparisons.
func (n *node) compact() string {
	var b strings.Builder
	n.write(&b, "", "")
	return b.String()
}

// pretty is the value indented as the template's settings.json is (two spaces), with a final
// newline.
func (n *node) pretty() []byte {
	var b strings.Builder
	n.write(&b, "", "  ")
	b.WriteString("\n")
	return []byte(b.String())
}

func (n *node) write(b *strings.Builder, prefix, indent string) {
	nl, sep := "", ":"
	if indent != "" {
		nl, sep = "\n", ": "
	}
	switch {
	case n.obj:
		if len(n.keys) == 0 {
			b.WriteString("{}")
			return
		}
		b.WriteString("{" + nl)
		for i, k := range n.keys {
			kb, _ := marshalNoEscape(k)
			b.WriteString(prefix + indent + string(kb) + sep)
			n.fields[k].write(b, prefix+indent, indent)
			if i < len(n.keys)-1 {
				b.WriteString(",")
			}
			b.WriteString(nl)
		}
		b.WriteString(prefix + "}")
	case n.arr:
		if len(n.items) == 0 {
			b.WriteString("[]")
			return
		}
		b.WriteString("[" + nl)
		for i, it := range n.items {
			b.WriteString(prefix + indent)
			it.write(b, prefix+indent, indent)
			if i < len(n.items)-1 {
				b.WriteString(",")
			}
			b.WriteString(nl)
		}
		b.WriteString(prefix + "]")
	default:
		b.Write(n.raw)
	}
}
