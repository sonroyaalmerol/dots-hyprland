package hyprparse

import "testing"

func TestParseSimpleKeyValue(t *testing.T) {
	doc := Parse([]byte("gaps_in = 4\n"))
	m := TopLevelKeyValue(doc)
	if m["gaps_in"] != "4" {
		t.Errorf("expected '4', got %q", m["gaps_in"])
	}
}

func TestParseSection(t *testing.T) {
	doc := Parse([]byte("general {\n\tgaps_in = 4\n\tborder_size = 2\n}\n"))

	sec := FindSection(doc, "general")
	if sec == nil {
		t.Fatal("expected 'general' section")
	}
	m := SectionKeyValueMap(sec)
	if m["gaps_in"] != "4" {
		t.Errorf("expected '4', got %q", m["gaps_in"])
	}
	if m["border_size"] != "2" {
		t.Errorf("expected '2', got %q", m["border_size"])
	}
}

func TestParseNestedSection(t *testing.T) {
	doc := Parse([]byte("outer {\n\tinner {\n\t\tkey = val\n\t}\n}\n"))

	outer := FindSection(doc, "outer")
	if outer == nil {
		t.Fatal("expected 'outer' section")
	}
	// Find nested 'inner' within outer's children
	var inner *Section
	for _, child := range outer.Children {
		if s, ok := child.(*Section); ok && s.Name == "inner" {
			inner = s
			break
		}
	}
	if inner == nil {
		t.Fatal("expected 'inner' nested section")
	}
	m := SectionKeyValueMap(inner)
	if m["key"] != "val" {
		t.Errorf("expected 'val', got %q", m["key"])
	}
}

func TestParseBind(t *testing.T) {
	doc := Parse([]byte("bind = Super, T, exec, kitty\n"))
	binds := TopLevelBinds(doc)
	if len(binds) != 1 {
		t.Fatalf("expected 1 bind, got %d", len(binds))
	}
	if binds[0].Prefix != "bind" {
		t.Errorf("expected prefix 'bind', got %q", binds[0].Prefix)
	}
}

func TestParseBindl(t *testing.T) {
	doc := Parse([]byte("bindl = , XF86PowerOff, exec, command\n"))
	binds := TopLevelBinds(doc)
	if len(binds) != 1 {
		t.Fatalf("expected 1 bind, got %d", len(binds))
	}
	if binds[0].Prefix != "bindl" {
		t.Errorf("expected prefix 'bindl', got %q", binds[0].Prefix)
	}
}

func TestParseExecOnce(t *testing.T) {
	doc := Parse([]byte("exec-once = waybar &\n"))
	execs := TopLevelExecs(doc)
	if len(execs) != 1 {
		t.Fatalf("expected 1 exec, got %d", len(execs))
	}
	if !execs[0].IsOnce {
		t.Error("expected IsOnce=true")
	}
}

func TestParseSource(t *testing.T) {
	doc := Parse([]byte("source = path/to/file.conf\n"))
	srcs := Sources(doc)
	if len(srcs) != 1 {
		t.Fatalf("expected 1 source, got %d", len(srcs))
	}
	if srcs[0].Path != "path/to/file.conf" {
		t.Errorf("expected path 'path/to/file.conf', got %q", srcs[0].Path)
	}
}

func TestParseComments(t *testing.T) {
	doc := Parse([]byte("# This is a comment\ngaps_in = 4\n"))

	found := false
	for _, child := range doc.Children {
		if c, ok := child.(*Comment); ok && c.Text == "# This is a comment" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected comment node")
	}
}

func TestParseBlankLines(t *testing.T) {
	doc := Parse([]byte("gaps_in = 4\n\nborder_size = 2\n"))

	hasBlank := false
	for _, child := range doc.Children {
		if _, ok := child.(*BlankLine); ok {
			hasBlank = true
			break
		}
	}
	if !hasBlank {
		t.Error("expected blank line node")
	}
}

func TestSerializeRoundTrip(t *testing.T) {
	// Round-trip: parse then serialize, then verify key content is preserved
	input := []byte("# config\nbind = Super, T, exec, kitty\n\ngeneral {\n\tgaps_in = 4\n}\n")
	doc := Parse(input)
	output := Serialize(doc)

	parsed := Parse(output)

	// Same top-level KVs
	origKV := TopLevelKeyValue(doc)
	parsedKV := TopLevelKeyValue(parsed)
	for k, v := range origKV {
		if parsedKV[k] != v {
			t.Errorf("key %q: expected %q, got %q", k, v, parsedKV[k])
		}
	}

	// Same binds
	origBinds := TopLevelBinds(doc)
	parsedBinds := TopLevelBinds(parsed)
	if len(origBinds) != len(parsedBinds) {
		t.Errorf("bind count mismatch: orig=%d parsed=%d", len(origBinds), len(parsedBinds))
	}

	// Same section content
	origSec := FindSection(doc, "general")
	parsedSec := FindSection(parsed, "general")
	if origSec == nil || parsedSec == nil {
		t.Fatal("general section missing after round-trip")
	}
	origSecKV := SectionKeyValueMap(origSec)
	parsedSecKV := SectionKeyValueMap(parsedSec)
	for k, v := range origSecKV {
		if parsedSecKV[k] != v {
			t.Errorf("section key %q: expected %q, got %q", k, v, parsedSecKV[k])
		}
	}
}
