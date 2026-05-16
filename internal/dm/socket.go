package dm

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	maxLineLen    = 4096 // generous for a password; reject anything longer
	maxConns      = 4    // only the greeter should connect
	maxAuthPerSec = 3    // rate limit auth attempts
)

// Socket is the IPC server that the greeter connects to for authentication.
type Socket struct {
	path       string
	greeterUID uint32
	greeterGID uint32
	ln         net.Listener
	authCh     chan *Credentials
	mu         sync.Mutex
	conns      map[net.Conn]struct{}
	authTimes  []time.Time // for rate limiting
}

// NewSocket creates a new IPC socket server. The socket is restricted to
// the greeter user (owner read/write only).
func NewSocket(path string, greeterUID, greeterGID uint32) *Socket {
	return &Socket{
		path:       path,
		greeterUID: greeterUID,
		greeterGID: greeterGID,
		authCh:     make(chan *Credentials, 8),
		conns:      make(map[net.Conn]struct{}),
	}
}

// AuthCh returns the channel that receives credentials from the greeter.
func (s *Socket) AuthCh() <-chan *Credentials {
	return s.authCh
}

// Run starts listening and accepting connections. Blocks until ctx is cancelled.
func (s *Socket) Run(ctx context.Context) error {
	// Remove stale socket only if it's a socket (not a symlink).
	if fi, err := os.Lstat(s.path); err == nil {
		if fi.Mode()&os.ModeType == os.ModeSocket {
			os.Remove(s.path)
		} else {
			return fmt.Errorf("socket path %s exists and is not a socket (mode %s)", s.path, fi.Mode())
		}
	}

	// Bind before chown so we know the path is safe.
	ln, err := net.Listen("unix", s.path)
	if err != nil {
		return err
	}
	s.ln = ln

	// Restrict to greeter user only (0600 → owner = snry-dm).
	if err := os.Chmod(s.path, 0600); err != nil {
		ln.Close()
		os.Remove(s.path)
		return fmt.Errorf("chmod socket: %w", err)
	}
	if err := os.Chown(s.path, int(s.greeterUID), int(s.greeterGID)); err != nil {
		ln.Close()
		os.Remove(s.path)
		return fmt.Errorf("chown socket: %w", err)
	}

	log.Printf("[dm/socket] listening on %s (uid=%d)", s.path, s.greeterUID)

	go func() {
		<-ctx.Done()
		ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				log.Printf("[dm/socket] accept error: %v", err)
				continue
			}
		}

		s.mu.Lock()
		if len(s.conns) >= maxConns {
			s.mu.Unlock()
			conn.Close()
			log.Printf("[dm/socket] rejected connection: too many clients")
			continue
		}
		s.conns[conn] = struct{}{}
		s.mu.Unlock()

		go s.handleConn(conn)
	}
}

func (s *Socket) handleConn(conn net.Conn) {
	defer func() {
		conn.Close()
		s.mu.Lock()
		delete(s.conns, conn)
		s.mu.Unlock()
	}()

	log.Printf("[dm/socket] client connected")

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, maxLineLen), maxLineLen)
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) > maxLineLen {
			log.Printf("[dm/socket] oversized line rejected")
			return
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		s.dispatch(conn, line)
	}
}

func (s *Socket) dispatch(conn net.Conn, line string) {
	// Try JSON first.
	var cmd struct {
		Command  string `json:"command"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.Unmarshal([]byte(line), &cmd); err == nil && cmd.Command != "" {
		switch cmd.Command {
		case "auth":
			if !s.checkRateLimit() {
				s.emit(conn, map[string]any{
					"event": "auth_result",
					"data": map[string]any{
						"success":   false,
						"message":   "Too many attempts. Please wait.",
						"remaining": 0,
						"lockedOut": true,
					},
				})
				return
			}
			username := cmd.Username
			if username == "" {
				username = resolveDefaultUser()
			}
			if username == "" {
				s.emit(conn, map[string]any{
					"event": "auth_result",
					"data": map[string]any{
						"success":   false,
						"message":   "Authentication failed.",
						"remaining": 3,
						"lockedOut": false,
					},
				})
				return
			}
			s.authCh <- &Credentials{Username: username, Password: cmd.Password}
		case "lock-startup":
			s.emit(conn, map[string]any{
				"event": "lock_state",
				"data":  map[string]any{"locked": true},
			})
		}
		return
	}

	// Fallback: text protocol (same as snry-daemon).
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return
	}

	switch fields[0] {
	case "auth":
		if len(fields) < 2 {
			return
		}
		if !s.checkRateLimit() {
			s.emit(conn, map[string]any{
				"event": "auth_result",
				"data": map[string]any{
					"success":   false,
					"message":   "Too many attempts. Please wait.",
					"remaining": 0,
					"lockedOut": true,
				},
			})
			return
		}
		password := strings.Join(fields[1:], " ")
		username := resolveDefaultUser()
		if username == "" {
			s.emit(conn, map[string]any{
				"event": "auth_result",
				"data": map[string]any{
					"success":   false,
					"message":   "Authentication failed.",
					"remaining": 3,
					"lockedOut": false,
				},
			})
			return
		}
		s.authCh <- &Credentials{Username: username, Password: password}
	case "lock-startup":
		s.emit(conn, map[string]any{
			"event": "lock_state",
			"data":  map[string]any{"locked": true},
		})
	case "lock":
		s.emit(conn, map[string]any{
			"event": "lock_state",
			"data":  map[string]any{"locked": true},
		})
	}
}

// checkRateLimit enforces maxAuthPerSec. Returns false if rate exceeded.
func (s *Socket) checkRateLimit() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	windowStart := now.Add(-time.Second)

	// Trim old entries.
	valid := s.authTimes[:0]
	for _, t := range s.authTimes {
		if t.After(windowStart) {
			valid = append(valid, t)
		}
	}
	s.authTimes = valid

	if len(s.authTimes) >= maxAuthPerSec {
		return false
	}
	s.authTimes = append(s.authTimes, now)
	return true
}

// SendAuthResult sends an auth result event to the requesting connection only.
func (s *Socket) SendAuthResult(conn net.Conn, success bool, message string) {
	s.emit(conn, map[string]any{
		"event": "auth_result",
		"data": map[string]any{
			"success":   success,
			"message":   message,
			"remaining": 3,
			"lockedOut": false,
		},
	})
}

// SendAuthResultAll sends an auth result to all connected clients.
// Used for backwards compatibility when we don't know which conn asked.
func (s *Socket) SendAuthResultAll(success bool, message string) {
	s.emitAll(map[string]any{
		"event": "auth_result",
		"data": map[string]any{
			"success":   success,
			"message":   message,
			"remaining": 3,
			"lockedOut": false,
		},
	})
}

func (s *Socket) emit(conn net.Conn, m map[string]any) {
	data, _ := json.Marshal(m)
	conn.Write(append(data, '\n'))
}

func (s *Socket) emitAll(m map[string]any) {
	data, err := json.Marshal(m)
	if err != nil {
		return
	}
	data = append(data, '\n')

	s.mu.Lock()
	defer s.mu.Unlock()
	for conn := range s.conns {
		conn.Write(data)
	}
}

// resolveDefaultUser finds the primary user on the system.
// Returns the first regular user (UID >= 1000) from /etc/passwd,
// or empty string if none found (never returns "root").
func resolveDefaultUser() string {
	data, err := os.ReadFile("/etc/passwd")
	if err != nil {
		return ""
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		fields := strings.Split(line, ":")
		if len(fields) < 7 {
			continue
		}
		uid, err := strconv.Atoi(fields[2])
		if err != nil || uid < 1000 || uid >= 65534 {
			continue
		}
		shell := fields[6]
		if strings.Contains(shell, "nologin") || strings.Contains(shell, "false") {
			continue
		}
		return fields[0]
	}
	return ""
}
