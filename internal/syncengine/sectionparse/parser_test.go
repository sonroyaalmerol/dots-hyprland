package sectionparse

import (
	"bytes"
	"testing"
)

func TestHasMarker(t *testing.T) {
	data := []byte("# >>> snry-shell >>>\nexport FOO=bar\n# <<< snry-shell <<<\n")
	if !HasMarker(data) {
		t.Error("expected markers to be found")
	}
}

func TestHasMarkerNegative(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{"empty", []byte{}},
		{"no markers", []byte("export FOO=bar\n")},
		{"only start marker", []byte("# >>> snry-shell >>>\n")},
		{"only end marker", []byte("# <<< snry-shell <<<\n")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if HasMarker(tt.data) {
				t.Errorf("expected no markers in %q", string(tt.data))
			}
		})
	}
}

func TestExtractBlock(t *testing.T) {
	data := []byte("before\n# >>> snry-shell >>>\nexport FOO=bar\nexport BAZ=qux\n# <<< snry-shell <<<\nafter\n")
	block := ExtractBlock(data)

	expected := []byte("export FOO=bar\nexport BAZ=qux")
	if !bytes.Equal(block, expected) {
		t.Errorf("extracted block mismatch:\n got  %q\n want %q", string(block), string(expected))
	}
}

func TestReplaceBlock(t *testing.T) {
	data := []byte("before\n# >>> snry-shell >>>\nold content\n# <<< snry-shell <<<\nafter\n")
	newBlock := []byte("new content")
	result := ReplaceBlock(data, newBlock)

	expected := []byte("before\n# >>> snry-shell >>>\nnew content\n# <<< snry-shell <<<\nafter\n")
	if string(result) != string(expected) {
		t.Errorf("replace mismatch:\n got  %q\n want %q", string(result), string(expected))
	}
}

func TestWrapInMarkers(t *testing.T) {
	content := []byte("export FOO=bar\n")
	result := WrapInMarkers(content)

	expected := []byte("# >>> snry-shell >>>\nexport FOO=bar\n# <<< snry-shell <<<\n")
	if string(result) != string(expected) {
		t.Errorf("wrap mismatch:\n got  %q\n want %q", string(result), string(expected))
	}
}
