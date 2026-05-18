package wallpaper

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// SignalGhostty sends SIGUSR1 to all ghostty processes to trigger config reload.
func SignalGhostty() {
	out, err := exec.Command("pidof", "ghostty").Output()
	if err != nil {
		return
	}
	for pidStr := range strings.SplitSeq(strings.TrimSpace(string(out)), " ") {
		if pid, err := strconv.Atoi(pidStr); err == nil {
			syscall.Kill(pid, syscall.SIGUSR1)
		}
	}
}

// BroadcastSequences reads a sequences file and writes it to all /dev/pts/* entries.
func BroadcastSequences(seqPath string) {
	seqData, err := os.ReadFile(seqPath)
	if err != nil {
		return
	}
	entries, err := os.ReadDir("/dev/pts")
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		// Only write to numeric pts entries
		if _, err := strconv.Atoi(entry.Name()); err != nil {
			continue
		}
		f, err := os.OpenFile(filepath.Join("/dev/pts", entry.Name()), os.O_WRONLY, 0)
		if err != nil {
			continue
		}
		f.Write(seqData)
		f.Close()
	}
}
