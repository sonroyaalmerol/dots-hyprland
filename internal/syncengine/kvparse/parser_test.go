package kvparse

import (
	"slices"
	"strings"
	"testing"
)

func trim(s string) string {
	return strings.TrimLeft(s, " ")
}

func TestParseSimpleKV(t *testing.T) {
	input := []byte("key=value\nfoo=bar\n")
	doc := Parse(input)

	m := doc.AllKeyValueMap()
	if m["key"] != "value" {
		t.Errorf("expected 'value', got %q", m["key"])
	}
	if m["foo"] != "bar" {
		t.Errorf("expected 'bar', got %q", m["foo"])
	}
}

func TestParseKVNoSpaces(t *testing.T) {
	input := []byte("key=value\nfoo=bar\n")
	doc := Parse(input)

	m := doc.AllKeyValueMap()
	if m["key"] != "value" {
		t.Errorf("expected 'value', got %q", m["key"])
	}
	if m["foo"] != "bar" {
		t.Errorf("expected 'bar', got %q", m["foo"])
	}
}

func TestParseSections(t *testing.T) {
	input := []byte("[section1]\nkey1=val1\nkey2=val2\n[section2]\nkey3=val3\n")
	doc := Parse(input)

	if len(doc.Sections) != 2 {
		t.Fatalf("expected 2 sections, got %d", len(doc.Sections))
	}
	if doc.Sections[0].Name != "section1" {
		t.Errorf("expected section 'section1', got %q", doc.Sections[0].Name)
	}
	if doc.Sections[1].Name != "section2" {
		t.Errorf("expected section 'section2', got %q", doc.Sections[1].Name)
	}

	m := doc.KeyValueMap("section1")
	if m["key1"] != "val1" || m["key2"] != "val2" {
		t.Error("section1 key-values mismatch")
	}
}

func TestParseComments(t *testing.T) {
	input := []byte("# this is a comment\nkey=val\n; semicolon comment\n")
	doc := Parse(input)

	found := false
	for _, c := range doc.Comments {
		if strings.Contains(c, "this is a comment") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected comment to be preserved")
	}
}

func TestParseBlankLines(t *testing.T) {
	input := []byte("key1=val1\n\nkey2=val2\n")
	doc := Parse(input)

	hasBlank := slices.Contains(doc.Comments, "")
	if !hasBlank {
		for _, s := range doc.Sections {
			for _, e := range s.Entries {
				if e.RawLine == "" {
					hasBlank = true
					break
				}
			}
		}
	}
	if !hasBlank {
		t.Error("expected blank line to be preserved")
	}
}

func TestSerializeRoundTrip(t *testing.T) {
	// Serializer normalizes to "key = value" format (with spaces around =)
	input := []byte("key=value\nother=test\n")
	doc := Parse(input)
	output := Serialize(doc)

	// Re-parse and compare values
	reparsed := Parse(output)
	origMap := doc.AllKeyValueMap()
	reparsedMap := reparsed.AllKeyValueMap()

	for k, v := range origMap {
		if trim(reparsedMap[k]) != v {
			t.Errorf("key %q: expected %q, got (trimmed) %q, raw %q", k, v, trim(reparsedMap[k]), reparsedMap[k])
		}
	}
}

func TestKeyValueMap(t *testing.T) {
	input := []byte("[main]\na=1\nb=2\n[other]\nc=3\n")
	doc := Parse(input)

	main := doc.KeyValueMap("main")
	if len(main) != 2 {
		t.Fatalf("expected 2 keys in main, got %d", len(main))
	}
	if main["a"] != "1" {
		t.Errorf("expected '1', got %q", main["a"])
	}

	all := doc.AllKeyValueMap()
	if len(all) != 3 {
		t.Errorf("expected 3 total keys, got %d", len(all))
	}
}

func TestSetKeyValue(t *testing.T) {
	doc := Parse([]byte("[main]\na=1\n"))

	doc.SetKeyValue("main", "b", "2")
	m := doc.KeyValueMap("main")
	if m["b"] != "2" {
		t.Errorf("expected '2', got %q", m["b"])
	}

	// Update existing
	doc.SetKeyValue("main", "a", "99")
	m = doc.KeyValueMap("main")
	if m["a"] != "99" {
		t.Errorf("expected '99', got %q", m["a"])
	}

	// New section
	doc.SetKeyValue("news", "x", "y")
	m2 := doc.KeyValueMap("news")
	if m2["x"] != "y" {
		t.Errorf("expected 'y', got %q", m2["x"])
	}
}
