package lock

import (
	"fmt"
	"os"
	"syscall"
)

// Acquire tries to exclusively lock the file at path using flock(2).
// Returns the file (must be kept open) or an error if another instance holds the lock.
func Acquire(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("open lock file %s: %w", path, err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("another instance of snry-daemon is already running")
	}
	return f, nil
}
