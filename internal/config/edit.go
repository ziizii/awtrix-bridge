package config

import (
	"fmt"
	"regexp"
	"strings"
)

// Settings changed from the admin UI are written back to the YAML file as
// line edits rather than by re-encoding the document, so the user's
// comments, ordering and formatting survive. The edits understand just
// enough YAML for this file: top-level mappings ("chime:") and block-style
// list entries whose first key is the name ("  - name: bus333").

// Field is one "key: value" line to set. Value is written verbatim.
type Field struct{ Key, Value string }

// SetChime sets keys of the top-level chime mapping, creating it before
// "apps:" if missing. Keys in comment are commented out.
func SetChime(raw []byte, fields []Field, comment ...string) []byte {
	return SetSection(raw, "chime", fields, comment...)
}

// SetSection sets keys of a top-level mapping (chime, timer), creating it
// before "apps:" if missing. Keys in comment are commented out.
func SetSection(raw []byte, section string, fields []Field, comment ...string) []byte {
	l := lines(strings.Split(string(raw), "\n"))
	start := l.topLevel(section)
	if start < 0 {
		at := l.topLevel("apps")
		if at < 0 {
			at = len(l)
		}
		l.insert(at, section+":", "")
		start = at
	}
	l.set(block{start: start, indent: 2}, fields, comment)
	return l.bytes()
}

// SetApp sets keys of the apps entry named app. Keys in comment are
// commented out.
func SetApp(raw []byte, app string, fields []Field, comment ...string) ([]byte, error) {
	l := lines(strings.Split(string(raw), "\n"))
	b, ok := l.listItem(app)
	if !ok {
		return nil, fmt.Errorf("app %q not found as a \"- name: %s\" entry in the config file", app, app)
	}
	l.set(b, fields, comment)
	return l.bytes(), nil
}

// SetThresholdColor sets the color of entry index of an app's colors
// list, in flow style ("- { below: 2, color: x }") or block style.
func SetThresholdColor(raw []byte, app string, index int, color string) ([]byte, error) {
	l := lines(strings.Split(string(raw), "\n"))
	b, ok := l.listItem(app)
	if !ok {
		return nil, fmt.Errorf("app %q not found as a \"- name: %s\" entry in the config file", app, app)
	}
	k := l.find(b, "colors")
	if k < 0 {
		return nil, fmt.Errorf("app %q has no colors list", app)
	}
	entry, base := -1, -1
	for i := k + 1; i < len(l); i++ {
		s, t := l[i], strings.TrimSpace(l[i])
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		ind := indentOf(s)
		if ind < b.indent || (ind == b.indent && !strings.HasPrefix(t, "- ")) {
			break // left the list
		}
		if strings.HasPrefix(t, "- ") && (base < 0 || ind == base) {
			base = ind
			entry++
		}
		if entry > index {
			break
		}
		if m := colorValue.FindStringSubmatchIndex(s); entry == index && m != nil {
			l[i] = s[:m[4]] + quote(color) + s[m[5]:]
			return l.bytes(), nil
		}
	}
	return nil, fmt.Errorf("app %q has no colors entry %d with a color", app, index)
}

// colorValue finds the value of a "color:" key (not "textColor:").
var colorValue = regexp.MustCompile(`(^|[\s{,-])color:\s*("[^"]*"|'[^']*'|[^\s,}#]+)`)

// quote makes a YAML string value; "#" would otherwise start a comment.
func quote(v string) string { return `"` + v + `"` }

type lines []string

// block is a mapping in the file: the line that opens it and the
// indentation of its keys.
type block struct{ start, indent int }

func (l lines) bytes() []byte { return []byte(strings.Join(l, "\n")) }

func (l *lines) insert(at int, add ...string) {
	*l = append((*l)[:at], append(add, (*l)[at:]...)...)
}

func indentOf(s string) int { return len(s) - len(strings.TrimLeft(s, " ")) }

// topLevel returns the line of a top-level "key:" or -1.
func (l lines) topLevel(key string) int {
	for i, s := range l {
		if strings.HasPrefix(s, key+":") {
			return i
		}
	}
	return -1
}

// listItem finds the "- name: <app>" entry of a block-style list.
func (l lines) listItem(app string) (block, bool) {
	for i, s := range l {
		t := strings.TrimLeft(s, " ")
		item, ok := strings.CutPrefix(t, "- ")
		if !ok {
			continue
		}
		k, v, ok := strings.Cut(item, ":")
		if ok && strings.TrimSpace(k) == "name" && unquote(stripComment(v)) == app {
			return block{start: i, indent: indentOf(s) + 2}, true
		}
	}
	return block{}, false
}

// find returns the line of key directly inside b, or -1.
func (l lines) find(b block, key string) int {
	for i := b.start + 1; i < len(l); i++ {
		s, t := l[i], strings.TrimSpace(l[i])
		if t != "" && !strings.HasPrefix(t, "#") && indentOf(s) < b.indent {
			return -1 // left the block
		}
		if indentOf(s) == b.indent && strings.HasPrefix(t, key+":") {
			return i
		}
	}
	return -1
}

// set replaces existing keys in place (keeping their trailing comment),
// inserts missing ones after the previous field, and comments out keys
// listed in comment.
func (l *lines) set(b block, fields []Field, comment []string) {
	pad := strings.Repeat(" ", b.indent)
	prev := b.start
	for _, f := range fields {
		line := pad + f.Key + ": " + f.Value
		if i := l.find(b, f.Key); i >= 0 {
			(*l)[i] = line + trailingComment((*l)[i])
			prev = i
			continue
		}
		l.insert(prev+1, line)
		prev++
	}
	for _, k := range comment {
		if i := l.find(b, k); i >= 0 {
			(*l)[i] = pad + "# " + strings.TrimSpace((*l)[i])
		}
	}
}

// trailingComment returns the "   # ..." tail of a "key: value  # comment"
// line, including the padding, or "" when the value is quoted (it might
// contain "#").
func trailingComment(line string) string {
	if strings.ContainsAny(line, `"'`) {
		return ""
	}
	i := strings.Index(line, " #")
	if i < 0 {
		return ""
	}
	return line[len(strings.TrimRight(line[:i], " ")):]
}

func stripComment(v string) string {
	if i := strings.Index(v, " #"); i >= 0 {
		return v[:i]
	}
	return v
}

func unquote(v string) string { return strings.Trim(strings.TrimSpace(v), `"'`) }
