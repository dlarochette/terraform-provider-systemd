// Package unitfile serializes structured systemd unit/network sections to INI text.
package unitfile

import (
	"fmt"
	"sort"
	"strings"
)

// Section is a systemd INI section ([Unit], [Service], [Network], …).
type Section struct {
	Name    string
	Entries []Entry
}

// Entry is a key=value line. Duplicate keys are allowed (systemd multi-value).
type Entry struct {
	Key   string
	Value string
}

// File is an ordered list of sections.
type File struct {
	Sections []Section
}

// Render writes a systemd unit/network file. Empty values are skipped.
func (f File) Render() string {
	var b strings.Builder
	for i, sec := range f.Sections {
		if sec.Name == "" {
			continue
		}
		if i > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "[%s]\n", sec.Name)
		for _, e := range sec.Entries {
			if e.Key == "" {
				continue
			}
			fmt.Fprintf(&b, "%s=%s\n", e.Key, e.Value)
		}
	}
	return b.String()
}

// Parse reads a systemd INI-ish file into Sections. Comments (#, ;) and blank lines are dropped.
func Parse(content string) (File, error) {
	var f File
	var cur *Section
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	for n, line := range lines {
		line = strings.TrimRight(line, " \t")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") {
			continue
		}
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			name := trimmed[1 : len(trimmed)-1]
			f.Sections = append(f.Sections, Section{Name: name})
			cur = &f.Sections[len(f.Sections)-1]
			continue
		}
		if cur == nil {
			return File{}, fmt.Errorf("line %d: key outside section: %q", n+1, line)
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			return File{}, fmt.Errorf("line %d: expected key=value: %q", n+1, line)
		}
		cur.Entries = append(cur.Entries, Entry{Key: strings.TrimSpace(key), Value: val})
	}
	return f, nil
}

// MapSections builds a File from map[section]map[key][]values preserving sorted keys.
func MapSections(sections map[string]map[string][]string, order []string) File {
	var f File
	names := order
	if len(names) == 0 {
		names = make([]string, 0, len(sections))
		for n := range sections {
			names = append(names, n)
		}
		sort.Strings(names)
	}
	for _, name := range names {
		kv, ok := sections[name]
		if !ok {
			continue
		}
		sec := Section{Name: name}
		keys := make([]string, 0, len(kv))
		for k := range kv {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			for _, v := range kv[k] {
				sec.Entries = append(sec.Entries, Entry{Key: k, Value: v})
			}
		}
		f.Sections = append(f.Sections, sec)
	}
	return f
}

// ChooseContent returns raw content if set, otherwise renders structured sections.
// Both set → error. Neither set → error.
func ChooseContent(raw string, structured File) (string, error) {
	hasRaw := strings.TrimSpace(raw) != ""
	hasStruct := len(structured.Sections) > 0
	switch {
	case hasRaw && hasStruct:
		return "", fmt.Errorf("content and structured sections are mutually exclusive")
	case hasRaw:
		return raw, nil
	case hasStruct:
		return structured.Render(), nil
	default:
		return "", fmt.Errorf("either content or structured sections must be set")
	}
}
