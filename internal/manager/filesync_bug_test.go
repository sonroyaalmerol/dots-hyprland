package manager

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// Regression: copyFile must use atomic write (temp + rename).
// After two writes, the inode should change, proving temp-then-rename.
func TestCopyFileIsAtomic(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")

	if err := os.WriteFile(src, []byte("first"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(src, dst, 0o644); err != nil {
		t.Fatal(err)
	}
	st1, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	inode1 := st1.Sys().(*syscall.Stat_t).Ino

	if err := os.WriteFile(src, []byte("second"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(src, dst, 0o644); err != nil {
		t.Fatal(err)
	}
	st2, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	inode2 := st2.Sys().(*syscall.Stat_t).Ino

	if inode1 == inode2 {
		t.Errorf("inode unchanged (%d): copyFile is not using atomic rename", inode1)
	}
}

// Regression: copyFile content must be correct after atomic write.
func TestCopyFileContentCorrect(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")

	content := "hello world\nline two\n"
	if err := os.WriteFile(src, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(src, dst, 0o644); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != content {
		t.Errorf("content mismatch: got %q, want %q", string(data), content)
	}
}
