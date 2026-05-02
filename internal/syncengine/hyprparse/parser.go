package hyprparse

import (
	"bytes"
	"strings"
)

type Node interface {
	nodeMarker()
}

type Document struct {
	Children []Node
}

func (d *Document) nodeMarker() {}

type Section struct {
	Name     string
	Children []Node
}

func (s *Section) nodeMarker() {}

type KeyValue struct {
	Key     string
	Value   string
	Comment string
}

func (kv *KeyValue) nodeMarker() {}

type BindEntry struct {
	Prefix     string
	RawLine    string
	Mods       string
	Key        string
	Dispatcher string
	Param      string
}

func (b *BindEntry) nodeMarker() {}

type ExecEntry struct {
	IsOnce  bool
	Command string
	RawLine string
}

func (e *ExecEntry) nodeMarker() {}

type SourceDirective struct {
	Path    string
	RawLine string
}

func (s *SourceDirective) nodeMarker() {}

type HyprlangDirective struct {
	Kind    string
	Value   string
	RawLine string
}

func (h *HyprlangDirective) nodeMarker() {}

type Comment struct {
	Text string
}

func (c *Comment) nodeMarker() {}

type BlankLine struct{}

func (b *BlankLine) nodeMarker() {}

type UnknownLine struct {
	RawLine string
}

func (u *UnknownLine) nodeMarker() {}

func Parse(data []byte) *Document {
	doc := &Document{}
	lines := bytes.Split(data, []byte("\n"))

	var sectionStack []*Section

	for _, line := range lines {
		lineStr := string(line)
		trimmed := strings.TrimSpace(lineStr)

		if trimmed == "" {
			node := &BlankLine{}
			appendTo(doc, sectionStack, node)
			continue
		}

		if strings.HasPrefix(trimmed, "#") {
			if strings.HasPrefix(trimmed, "# hyprlang") {
				parts := strings.Fields(trimmed)
				kind := ""
				value := ""
				if len(parts) >= 3 {
					kind = parts[2]
					if len(parts) > 3 {
						value = strings.Join(parts[3:], " ")
					}
				}
				node := &HyprlangDirective{Kind: kind, Value: value, RawLine: lineStr}
				appendTo(doc, sectionStack, node)
			} else {
				node := &Comment{Text: lineStr}
				appendTo(doc, sectionStack, node)
			}
			continue
		}

		if trimmed == "}" {
			if len(sectionStack) > 0 {
				sectionStack = sectionStack[:len(sectionStack)-1]
			}
			continue
		}

		lower := strings.ToLower(trimmed)

		if strings.HasPrefix(lower, "source=") || strings.HasPrefix(lower, "source =") {
			val := extractAfterEqual(trimmed)
			node := &SourceDirective{Path: val, RawLine: lineStr}
			appendTo(doc, sectionStack, node)
			continue
		}

		if strings.HasPrefix(lower, "exec-once=") || strings.HasPrefix(lower, "exec-once =") {
			val := extractAfterEqual(trimmed)
			node := &ExecEntry{IsOnce: true, Command: val, RawLine: lineStr}
			appendTo(doc, sectionStack, node)
			continue
		}

		if strings.HasPrefix(lower, "exec=") || strings.HasPrefix(lower, "exec =") {
			val := extractAfterEqual(trimmed)
			node := &ExecEntry{IsOnce: false, Command: val, RawLine: lineStr}
			appendTo(doc, sectionStack, node)
			continue
		}

		bindPrefixes := []string{"bindid", "bindit", "binditn", "bindl", "binde", "bindm", "bind"}
		isBind := false
		for _, prefix := range bindPrefixes {
			if strings.HasPrefix(lower, prefix) {
				rest := trimmed[len(prefix):]
				if len(rest) > 0 && (rest[0] == '=' || rest[0] == ' ' || strings.HasPrefix(rest, "d=") || strings.HasPrefix(rest, "m=") || strings.HasPrefix(rest, "t=") || strings.HasPrefix(rest, "n=") || strings.HasPrefix(rest, "e=") || strings.HasPrefix(rest, "i=") || strings.HasPrefix(rest, "s=")) {
					node := &BindEntry{Prefix: prefix, RawLine: lineStr}
					appendTo(doc, sectionStack, node)
					isBind = true
					break
				}
			}
		}
		if isBind {
			continue
		}

		// Check for section open: name { ... }
		if before, ok := strings.CutSuffix(trimmed, "{"); ok {
			name := strings.TrimSpace(before)
			section := &Section{Name: name}
			appendTo(doc, sectionStack, section)
			sectionStack = append(sectionStack, section)
			continue
		}

		// Check for key = value
		if idx := strings.Index(trimmed, "="); idx > 0 {
			key := strings.TrimSpace(trimmed[:idx])
			val := trimmed[idx+1:]
			var comment string
			if commentIdx := strings.Index(val, "#"); commentIdx >= 0 {
				comment = val[commentIdx:]
				val = strings.TrimSpace(val[:commentIdx])
			}
			node := &KeyValue{Key: key, Value: strings.TrimSpace(val), Comment: comment}
			appendTo(doc, sectionStack, node)
			continue
		}

		node := &UnknownLine{RawLine: lineStr}
		appendTo(doc, sectionStack, node)
		continue
	}

	return doc
}

func extractAfterEqual(s string) string {
	_, after, ok := strings.Cut(s, "=")
	if !ok {
		return ""
	}
	return strings.TrimSpace(after)
}

func appendTo(doc *Document, stack []*Section, node Node) {
	if len(stack) > 0 {
		parent := stack[len(stack)-1]
		parent.Children = append(parent.Children, node)
	} else {
		doc.Children = append(doc.Children, node)
	}
}

func Serialize(doc *Document) []byte {
	var buf bytes.Buffer
	serializeNodes(doc.Children, &buf, 0)
	result := buf.Bytes()
	if len(result) > 0 && result[len(result)-1] != '\n' {
		result = append(result, '\n')
	}
	return result
}

func serializeNodes(nodes []Node, buf *bytes.Buffer, indent int) {
	indentStr := strings.Repeat("\t", indent)

	for _, node := range nodes {
		switch n := node.(type) {
		case *Section:
			buf.WriteString(indentStr)
			buf.WriteString(n.Name)
			buf.WriteString(" {\n")
			serializeNodes(n.Children, buf, indent+1)
			buf.WriteString(indentStr)
			buf.WriteString("}\n")
		case *KeyValue:
			buf.WriteString(indentStr)
			if n.Comment != "" {
				buf.WriteString(n.Key)
				buf.WriteString(" = ")
				buf.WriteString(n.Value)
				buf.WriteString(" ")
				buf.WriteString(n.Comment)
				buf.WriteByte('\n')
			} else {
				buf.WriteString(n.Key)
				buf.WriteString(" = ")
				buf.WriteString(n.Value)
				buf.WriteByte('\n')
			}
		case *BindEntry:
			buf.WriteString(n.RawLine)
			buf.WriteByte('\n')
		case *ExecEntry:
			buf.WriteString(n.RawLine)
			buf.WriteByte('\n')
		case *SourceDirective:
			buf.WriteString(n.RawLine)
			buf.WriteByte('\n')
		case *HyprlangDirective:
			buf.WriteString(n.RawLine)
			buf.WriteByte('\n')
		case *Comment:
			buf.WriteString(n.Text)
			buf.WriteByte('\n')
		case *BlankLine:
			buf.WriteByte('\n')
		case *UnknownLine:
			buf.WriteString(n.RawLine)
			buf.WriteByte('\n')
		}
	}
}

func FindSection(doc *Document, name string) *Section {
	for _, child := range doc.Children {
		if s, ok := child.(*Section); ok && s.Name == name {
			return s
		}
	}
	return nil
}

func SectionKeyValueMap(s *Section) map[string]string {
	m := make(map[string]string)
	for _, child := range s.Children {
		if kv, ok := child.(*KeyValue); ok {
			m[kv.Key] = kv.Value
		}
	}
	return m
}

func TopLevelKeyValue(doc *Document) map[string]string {
	m := make(map[string]string)
	for _, child := range doc.Children {
		if kv, ok := child.(*KeyValue); ok {
			m[kv.Key] = kv.Value
		}
	}
	return m
}

func TopLevelBinds(doc *Document) []*BindEntry {
	var binds []*BindEntry
	for _, child := range doc.Children {
		if b, ok := child.(*BindEntry); ok {
			binds = append(binds, b)
		}
	}
	return binds
}

func TopLevelExecs(doc *Document) []*ExecEntry {
	var execs []*ExecEntry
	for _, child := range doc.Children {
		if e, ok := child.(*ExecEntry); ok {
			execs = append(execs, e)
		}
	}
	return execs
}

func Sources(doc *Document) []*SourceDirective {
	var sources []*SourceDirective
	for _, child := range doc.Children {
		if s, ok := child.(*SourceDirective); ok {
			sources = append(sources, s)
		}
	}
	return sources
}

func AllSectionNames(doc *Document) []string {
	var names []string
	for _, child := range doc.Children {
		if s, ok := child.(*Section); ok {
			names = append(names, s.Name)
		}
	}
	return names
}
