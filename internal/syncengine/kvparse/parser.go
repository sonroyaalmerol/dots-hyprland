package kvparse

import (
	"bytes"
	"strings"
)

type KVDocument struct {
	Sections []KVSection
	Comments []string // leading comments before first section
}

type KVSection struct {
	Name    string
	Entries []KVEntry
}

type KVEntry struct {
	Key     string
	Value   string
	Comment string // inline comment
	RawLine string // original line for round-trip
}

func Parse(data []byte) *KVDocument {
	doc := &KVDocument{}
	lines := bytes.Split(data, []byte("\n"))

	var currentSection *KVSection
	sectionMap := make(map[string]int)

	for _, line := range lines {
		trimmed := string(bytes.TrimSpace(line))

		if trimmed == "" {
			if currentSection == nil {
				doc.Comments = append(doc.Comments, "")
			} else {
				currentSection.Entries = append(currentSection.Entries, KVEntry{
					RawLine: string(line),
				})
			}
			continue
		}

		if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") {
			if currentSection == nil {
				doc.Comments = append(doc.Comments, string(line))
			} else {
				currentSection.Entries = append(currentSection.Entries, KVEntry{
					RawLine: string(line),
				})
			}
			continue
		}

		if strings.HasPrefix(trimmed, "[") && strings.Contains(trimmed, "]") {
			endIdx := strings.Index(trimmed, "]")
			sectionName := trimmed[1:endIdx]
			if idx, ok := sectionMap[sectionName]; ok {
				currentSection = &doc.Sections[idx]
			} else {
				doc.Sections = append(doc.Sections, KVSection{Name: sectionName})
				sectionMap[sectionName] = len(doc.Sections) - 1
				currentSection = &doc.Sections[len(doc.Sections)-1]
			}
			currentSection.Entries = append(currentSection.Entries, KVEntry{
				RawLine: string(line),
			})
			continue
		}

		eqIdx := strings.Index(trimmed, "=")
		if eqIdx > 0 {
			key := strings.TrimSpace(trimmed[:eqIdx])
			rest := trimmed[eqIdx+1:]

			value := rest
			var comment string
			if commentIdx := strings.Index(rest, "#"); commentIdx >= 0 {
				value = strings.TrimSpace(rest[:commentIdx])
				comment = rest[commentIdx:]
			} else if commentIdx := strings.Index(rest, ";"); commentIdx >= 0 {
				value = strings.TrimSpace(rest[:commentIdx])
				comment = rest[commentIdx:]
			}

			entry := KVEntry{
				Key:     key,
				Value:   value,
				Comment: comment,
				RawLine: string(line),
			}

			if currentSection == nil {
				doc.Sections = append(doc.Sections, KVSection{Name: ""})
				sectionMap[""] = len(doc.Sections) - 1
				currentSection = &doc.Sections[len(doc.Sections)-1]
			}
			currentSection.Entries = append(currentSection.Entries, entry)
			continue
		}

		if currentSection == nil {
			doc.Comments = append(doc.Comments, string(line))
		} else {
			currentSection.Entries = append(currentSection.Entries, KVEntry{
				RawLine: string(line),
			})
		}
	}

	return doc
}

func Serialize(doc *KVDocument) []byte {
	var buf bytes.Buffer

	for _, comment := range doc.Comments {
		buf.WriteString(comment)
		buf.WriteByte('\n')
	}

	for i, section := range doc.Sections {
		if section.Name != "" {
			if i > 0 || len(doc.Comments) > 0 {
				if buf.Len() > 0 && buf.Bytes()[buf.Len()-1] != '\n' {
					buf.WriteByte('\n')
				}
			}
		}

		for _, entry := range section.Entries {
			if entry.Key != "" {
				buf.WriteString(entry.Key)
				buf.WriteString(" = ")
				buf.WriteString(entry.Value)
				if entry.Comment != "" {
					buf.WriteString(" ")
					buf.WriteString(entry.Comment)
				}
				buf.WriteByte('\n')
			} else if entry.RawLine != "" {
				buf.WriteString(entry.RawLine)
				buf.WriteByte('\n')
			}
		}
	}

	result := buf.Bytes()
	if len(result) > 0 && result[len(result)-1] == '\n' {
		return result
	}
	return append(result, '\n')
}

func (d *KVDocument) KeyValueMap(section string) map[string]string {
	m := make(map[string]string)
	for _, s := range d.Sections {
		if s.Name == section {
			for _, e := range s.Entries {
				if e.Key != "" {
					m[e.Key] = e.Value
				}
			}
			break
		}
	}
	return m
}

func (d *KVDocument) AllKeyValueMap() map[string]string {
	m := make(map[string]string)
	for _, s := range d.Sections {
		for _, e := range s.Entries {
			if e.Key != "" {
				m[e.Key] = e.Value
			}
		}
	}
	return m
}

func (d *KVDocument) SetKeyValue(section, key, value string) {
	for i, s := range d.Sections {
		if s.Name == section {
			for j, e := range s.Entries {
				if e.Key == key {
					d.Sections[i].Entries[j].Value = value
					return
				}
			}
			d.Sections[i].Entries = append(s.Entries, KVEntry{
				Key:   key,
				Value: value,
			})
			return
		}
	}

	d.Sections = append(d.Sections, KVSection{
		Name:    section,
		Entries: []KVEntry{{Key: key, Value: value}},
	})
}

func IsKVContent(data []byte) bool {
	if len(data) == 0 {
		return true
	}
	lines := bytes.Split(data, []byte("\n"))
	kvLines := 0
	totalLines := 0
	for _, line := range lines {
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) == 0 {
			continue
		}
		totalLines++
		if bytes.HasPrefix(trimmed, []byte("#")) || bytes.HasPrefix(trimmed, []byte(";")) {
			continue
		}
		if bytes.Contains(trimmed, []byte("=")) {
			kvLines++
		}
	}
	return totalLines == 0 || kvLines > 0
}

func KeyValueMapFromEntries(entries []KVEntry) map[string]string {
	m := make(map[string]string)
	for _, e := range entries {
		if e.Key != "" {
			m[e.Key] = e.Value
		}
	}
	return m
}
