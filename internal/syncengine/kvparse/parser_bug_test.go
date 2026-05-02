package kvparse_test

import (
	"testing"

	"github.com/sonroyaalmerol/snry-shell-qs/internal/syncengine/kvparse"
)

// Regression: unquoted # and ; in values are treated as comments.
func TestParseCommentStripping(t *testing.T) {
	data := []byte("key = value # this is a comment\n")
	doc := kvparse.Parse(data)
	m := doc.KeyValueMap("")
	if m["key"] != "value" {
		t.Errorf("expected 'value', got %q", m["key"])
	}
}

// Regression: # inside URLs is NOT stripped when the config format
// doesn't use # as comment. Since our parser does strip unquoted #,
// this test confirms the CURRENT behavior so any change is caught.
func TestParseInlineHashInValue(t *testing.T) {
	data := []byte("key = https://example.com/path#anchor\n")
	doc := kvparse.Parse(data)
	m := doc.KeyValueMap("")
	// The parser treats # as a comment delimiter, so #anchor is stripped.
	// This is the expected behavior for INI-style configs.
	if m["key"] != "https://example.com/path" {
		t.Errorf("expected 'https://example.com/path', got %q", m["key"])
	}
}

// Regression: quoted values preserve # and ; characters.
func TestParseQuotedValuePreservesHash(t *testing.T) {
	data := []byte(`key = "value # not a comment"` + "\n")
	doc := kvparse.Parse(data)
	m := doc.KeyValueMap("")
	// Quotes are preserved (INI-style), but # is NOT stripped as a comment
	expected := `"value # not a comment"`
	if m["key"] != expected {
		t.Errorf("expected %q, got %q", expected, m["key"])
	}
	// Verify # was NOT treated as a comment (value contains #)
	if !contains(m["key"], '#') {
		t.Error("# was incorrectly stripped from quoted value")
	}
}

func contains(s string, r rune) bool {
	for _, c := range s {
		if c == r {
			return true
		}
	}
	return false
}

// Regression: quoted values preserve ; characters.
func TestParseQuotedValuePreservesSemicolon(t *testing.T) {
	data := []byte(`key = "value ; not a comment"` + "\n")
	doc := kvparse.Parse(data)
	m := doc.KeyValueMap("")
	expected := `"value ; not a comment"`
	if m["key"] != expected {
		t.Errorf("expected %q, got %q", expected, m["key"])
	}
	if !contains(m["key"], ';') {
		t.Error("; was incorrectly stripped from quoted value")
	}
}

// Regression: findCommentPos respects quoting (used indirectly through Parse).
func TestParseNoCommentInQuotedURL(t *testing.T) {
	data := []byte(`url = "https://example.com#section"` + "\n")
	doc := kvparse.Parse(data)
	m := doc.KeyValueMap("")
	expected := `"https://example.com#section"`
	if m["url"] != expected {
		t.Errorf("expected %q, got %q", expected, m["url"])
	}
	if !contains(m["url"], '#') {
		t.Error("# was incorrectly stripped from quoted URL")
	}
}
