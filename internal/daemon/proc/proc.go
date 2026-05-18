// Package proc provides shared utilities for scanning /proc.
package proc

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Running checks whether a process with the given comm name is currently running.
func Running(name string) bool {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "comm"))
		if err != nil {
			continue
		}
		if strings.TrimSpace(string(data)) == name {
			return true
		}
	}
	return false
}

// ForEachComm iterates over /proc and calls fn for each unique comm name found.
// The fn receives (commName, pid). Iteration stops early if fn returns false.
func ForEachComm(fn func(comm string, pid int) bool) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return
	}
	seen := make(map[string]bool)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if len(name) == 0 || name[0] < '0' || name[0] > '9' {
			continue
		}
		comm, err := os.ReadFile(filepath.Join("/proc", name, "comm"))
		if err != nil {
			continue
		}
		procName := strings.TrimSpace(string(comm))
		if seen[procName] {
			continue
		}
		seen[procName] = true
		pid, _ := strconv.Atoi(name)
		if !fn(procName, pid) {
			return
		}
	}
}

// FindPID returns the PID of a running process with the given comm name, or 0.
func FindPID(name string) int {
	var pid int
	ForEachComm(func(comm string, p int) bool {
		if comm == name {
			pid = p
			return false
		}
		return true
	})
	return pid
}
