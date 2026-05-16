package dm

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSanitizeForLog(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain", "alice", "alice"},
		{"newline", "alice\nbob", "alice\\nbob"},
		{"carriage return", "alice\r\nbob", "alice\\r\\nbob"},
		{"tab", "alice\tbob", "alicebob"}, // tab is 0x09, stripped
		{"control chars", "alice\x00\x01\x02bob", "alicebob"},
		{"mixed", "\x1b[31malice\n", "[31malice\\n"},
		{"empty", "", ""},
		{"null byte", "al\x00ice", "alice"},
		{"bell", "ding\x07", "ding"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeForLog(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeForLog(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSanitizeForLogNoNewlines(t *testing.T) {
	result := sanitizeForLog("user\nmalicious\nextra")
	if containsAnyLiteral(result, "\n") {
		t.Error("sanitizeForLog should not contain literal newlines")
	}
}

func containsAnyLiteral(s, chars string) bool {
	for _, c := range chars {
		for _, r := range s {
			if r == c {
				return true
			}
		}
	}
	return false
}

func TestFindBinary(t *testing.T) {
	// Should find a well-known binary.
	if p := findBinary("sh"); p == "sh" {
		t.Error("findBinary(\"sh\") should resolve to an absolute path on most systems")
	}

	// Should return just the name for a nonexistent binary.
	if p := findBinary("nonexistent-binary-xyz-12345"); p != "nonexistent-binary-xyz-12345" {
		t.Errorf("findBinary(nonexistent) = %q, want fallback name", p)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.SocketPath != "/run/snry-dm.sock" {
		t.Errorf("DefaultConfig().SocketPath = %q", cfg.SocketPath)
	}
	if cfg.GreeterUser != "snry-dm" {
		t.Errorf("DefaultConfig().GreeterUser = %q", cfg.GreeterUser)
	}
	if cfg.GreeterVT != 1 {
		t.Errorf("DefaultConfig().GreeterVT = %d", cfg.GreeterVT)
	}
}

func TestVerifyQMLDir(t *testing.T) {
	// Create a temp dir to simulate a QML directory.
	dir := t.TempDir()
	qmlFile := filepath.Join(dir, "test.qml")
	if err := os.WriteFile(qmlFile, []byte("// test"), 0644); err != nil {
		t.Fatal(err)
	}

	// Non-system path (not /usr/) should work regardless of ownership.
	if !verifyQMLDir(dir, "test.qml") {
		t.Error("verifyQMLDir should succeed for temp dir")
	}

	// Nonexistent file should fail.
	if verifyQMLDir(dir, "missing.qml") {
		t.Error("verifyQMLDir should fail for missing file")
	}

	// Nonexistent dir should fail.
	if verifyQMLDir("/nonexistent/path", "test.qml") {
		t.Error("verifyQMLDir should fail for nonexistent dir")
	}
}

func TestVerifyQMLDirSystemPath(t *testing.T) {
	// System paths (/usr/...) require root ownership.
	// In tests (non-root), we can verify the ownership check logic
	// by checking that a temp dir masquerading as /usr fails or passes
	// based on the /usr/ prefix check logic.

	// We can't actually create files in /usr/ in tests,
	// but we can test the non-/usr/ path works.
	dir := t.TempDir()
	qmlFile := filepath.Join(dir, "shell.qml")
	if err := os.WriteFile(qmlFile, []byte("// shell"), 0644); err != nil {
		t.Fatal(err)
	}

	if !verifyQMLDir(dir, "shell.qml") {
		t.Error("verifyQMLDir should succeed for temp dir with shell.qml")
	}
}

func TestResolvePaths(t *testing.T) {
	dm := &DM{cfg: Config{}}

	// With no system install, resolvePaths should try dev fallbacks.
	// Since we're in a temp context, it should gracefully handle missing dirs.
	dm.resolvePaths()

	// The fields should either be set (if running from source tree) or empty.
	// No crash is the main assertion here.
}

func TestResolvePathsWithExplicitConfig(t *testing.T) {
	dir := t.TempDir()
	qmlFile := filepath.Join(dir, "greeter.qml")
	if err := os.WriteFile(qmlFile, []byte("// greeter"), 0644); err != nil {
		t.Fatal(err)
	}

	dm := &DM{cfg: Config{
		GreeterQSConfigDir: dir,
	}}
	dm.resolvePaths()

	if dm.cfg.GreeterQMLPath != dir+"/greeter.qml" {
		t.Errorf("GreeterQMLPath = %q, want %q", dm.cfg.GreeterQMLPath, dir+"/greeter.qml")
	}
}
