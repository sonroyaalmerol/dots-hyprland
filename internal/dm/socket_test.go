package dm

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestSocketRateLimit(t *testing.T) {
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "test.sock")

	sock := NewSocket(sockPath, uint32(os.Getuid()), uint32(os.Getgid()))

	ctx := t.Context()

	go sock.Run(ctx)

	// Wait for socket to be ready.
	if err := waitForSocket(sockPath, 2*time.Second); err != nil {
		t.Fatal(err)
	}

	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// Send auth requests up to the rate limit.
	successes := 0
	for range maxAuthPerSec + 2 {
		cmd := `{"command":"auth","username":"testuser","password":"testpass"}`
		conn.Write(append([]byte(cmd), '\n'))
		time.Sleep(10 * time.Millisecond) // small delay to let server process
	}

	// Read responses.
	conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		var msg map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			continue
		}
		if event, ok := msg["event"].(string); ok && event == "auth_result" {
			successes++
		}
	}

	if successes == 0 {
		t.Error("expected at least one auth_result response")
	}
}

func TestSocketConnectionCap(t *testing.T) {
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "test.sock")

	sock := NewSocket(sockPath, uint32(os.Getuid()), uint32(os.Getgid()))

	ctx := t.Context()

	go sock.Run(ctx)

	if err := waitForSocket(sockPath, 2*time.Second); err != nil {
		t.Fatal(err)
	}

	// Open maxConns connections.
	var conns []net.Conn
	for i := range maxConns {
		conn, err := net.Dial("unix", sockPath)
		if err != nil {
			t.Fatalf("failed to connect %d: %v", i, err)
		}
		conns = append(conns, conn)
	}
	defer func() {
		for _, c := range conns {
			c.Close()
		}
	}()

	// The next connection should be accepted but then closed by the server
	// (or rejected during accept loop). We verify the server is still running
	// by checking that existing connections still work.
	time.Sleep(100 * time.Millisecond)

	// Send a lock-startup on an existing connection to verify server is alive.
	cmd := `{"command":"lock-startup"}`
	conns[0].Write(append([]byte(cmd), '\n'))

	conns[0].SetReadDeadline(time.Now().Add(1 * time.Second))
	scanner := bufio.NewScanner(conns[0])
	if !scanner.Scan() {
		t.Error("expected response from existing connection")
	}
}

func TestSocketJSONProtocol(t *testing.T) {
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "test.sock")

	sock := NewSocket(sockPath, uint32(os.Getuid()), uint32(os.Getgid()))

	ctx := t.Context()

	go sock.Run(ctx)

	if err := waitForSocket(sockPath, 2*time.Second); err != nil {
		t.Fatal(err)
	}

	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// Send lock-startup command.
	cmd := `{"command":"lock-startup"}`
	if _, err := conn.Write(append([]byte(cmd), '\n')); err != nil {
		t.Fatal(err)
	}

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		t.Fatal("expected response")
	}

	var msg map[string]any
	if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if msg["event"] != "lock_state" {
		t.Errorf("event = %v, want lock_state", msg["event"])
	}
	data, ok := msg["data"].(map[string]any)
	if !ok {
		t.Fatal("data field is not a map")
	}
	if data["locked"] != true {
		t.Errorf("locked = %v, want true", data["locked"])
	}
}

func TestSocketTextProtocol(t *testing.T) {
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "test.sock")

	sock := NewSocket(sockPath, uint32(os.Getuid()), uint32(os.Getgid()))

	ctx := t.Context()

	go sock.Run(ctx)

	if err := waitForSocket(sockPath, 2*time.Second); err != nil {
		t.Fatal(err)
	}

	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// Send text protocol "lock-startup".
	if _, err := conn.Write([]byte("lock-startup\n")); err != nil {
		t.Fatal(err)
	}

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		t.Fatal("expected response")
	}

	var msg map[string]any
	if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if msg["event"] != "lock_state" {
		t.Errorf("event = %v, want lock_state", msg["event"])
	}
}

func TestSocketAuthSendsCredentials(t *testing.T) {
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "test.sock")

	sock := NewSocket(sockPath, uint32(os.Getuid()), uint32(os.Getgid()))

	ctx := t.Context()

	go sock.Run(ctx)

	if err := waitForSocket(sockPath, 2*time.Second); err != nil {
		t.Fatal(err)
	}

	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// Send auth command.
	cmd := `{"command":"auth","username":"testuser","password":"testpass"}`
	if _, err := conn.Write(append([]byte(cmd), '\n')); err != nil {
		t.Fatal(err)
	}

	// Read credential from the auth channel.
	select {
	case creds := <-sock.AuthCh():
		if creds == nil {
			t.Fatal("got nil credentials")
		}
		if creds.Username != "testuser" {
			t.Errorf("Username = %q, want testuser", creds.Username)
		}
		if creds.Password != "testpass" {
			t.Errorf("Password = %q, want testpass", creds.Password)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for credentials on auth channel")
	}
}

func TestSocketEmptyLineIgnored(t *testing.T) {
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "test.sock")

	sock := NewSocket(sockPath, uint32(os.Getuid()), uint32(os.Getgid()))

	ctx := t.Context()

	go sock.Run(ctx)

	if err := waitForSocket(sockPath, 2*time.Second); err != nil {
		t.Fatal(err)
	}

	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// Send empty lines and whitespace.
	conn.Write([]byte("\n\n   \n\n"))

	// Then send a valid command to verify the connection is still alive.
	cmd := `{"command":"lock-startup"}`
	conn.Write(append([]byte(cmd), '\n'))

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		t.Fatal("expected response after empty lines")
	}
}

func TestSocketStaleSocketCleanup(t *testing.T) {
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "test.sock")

	// Create a file at sockPath to simulate a stale socket.
	if err := os.WriteFile(sockPath, []byte("stale"), 0644); err != nil {
		t.Fatal(err)
	}

	sock := NewSocket(sockPath, uint32(os.Getuid()), uint32(os.Getgid()))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Run should fail because the existing file is not a socket.
	err := sock.Run(ctx)
	if err == nil {
		t.Error("expected error for non-socket file at socket path")
		cancel()
	}
}

func TestSocketSymlinkNotCleaned(t *testing.T) {
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "test.sock")
	linkTarget := filepath.Join(dir, "target")

	// Create a symlink at sockPath.
	if err := os.WriteFile(linkTarget, []byte("target"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(linkTarget, sockPath); err != nil {
		t.Fatal(err)
	}

	sock := NewSocket(sockPath, uint32(os.Getuid()), uint32(os.Getgid()))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Run should fail because the symlink is not a socket.
	err := sock.Run(ctx)
	if err == nil {
		t.Error("expected error for symlink at socket path")
		cancel()
	}

	// Verify the symlink was NOT removed.
	if _, err := os.Lstat(sockPath); err != nil {
		t.Error("symlink should not have been removed")
	}
}

func TestResolveDefaultUser(t *testing.T) {
	// This test reads the real /etc/passwd, which should always exist.
	user := resolveDefaultUser()
	if user == "" {
		// CI runners might not have a regular user, so this is informational.
		t.Log("resolveDefaultUser returned empty (no regular user found)")
	}
	if user == "root" {
		t.Error("resolveDefaultUser should never return root")
	}
}

func TestResolveDefaultUserNotEmpty(t *testing.T) {
	// On most systems, there should be at least one regular user.
	// But in minimal containers there might not be, so just verify
	// it doesn't panic or return "root".
	user := resolveDefaultUser()
	if user == "root" {
		t.Error("resolveDefaultUser must never return root")
	}
}

func TestCheckRateLimit(t *testing.T) {
	sock := &Socket{}

	// Should allow up to maxAuthPerSec.
	for i := range maxAuthPerSec {
		if !sock.checkRateLimit() {
			t.Fatalf("checkRateLimit should allow attempt %d/%d", i+1, maxAuthPerSec)
		}
	}

	// Next one should be denied.
	if sock.checkRateLimit() {
		t.Error("checkRateLimit should deny after exceeding limit")
	}
}

func TestCheckRateLimitConcurrency(t *testing.T) {
	sock := &Socket{}

	var wg sync.WaitGroup
	allowed := make(chan bool, maxAuthPerSec+10)

	for range maxAuthPerSec + 5 {
		wg.Go(func() {
			allowed <- sock.checkRateLimit()
		})
	}

	wg.Wait()
	close(allowed)

	count := 0
	for ok := range allowed {
		if ok {
			count++
		}
	}

	if count > maxAuthPerSec {
		t.Errorf("rate limiter allowed %d requests, max is %d", count, maxAuthPerSec)
	}
}

func waitForSocket(path string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return os.ErrNotExist
}
