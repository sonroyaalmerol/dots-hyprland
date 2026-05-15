package dm

import (
	"bufio"
	"context"
	"encoding/json"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
)

// Socket is the IPC server that the greeter connects to for authentication.
// It speaks the same JSON protocol as snry-daemon's socket so the greeter's
// DaemonSocket.qml component works unchanged.
type Socket struct {
	path   string
	ln     net.Listener
	authCh chan *Credentials
	mu     sync.Mutex
	conns  map[net.Conn]struct{}
}

// NewSocket creates a new IPC socket server.
func NewSocket(path string) *Socket {
	return &Socket{
		path:   path,
		authCh: make(chan *Credentials, 8),
		conns:  make(map[net.Conn]struct{}),
	}
}

// AuthCh returns the channel that receives credentials from the greeter.
func (s *Socket) AuthCh() <-chan *Credentials {
	return s.authCh
}

// Run starts listening and accepting connections. Blocks until ctx is cancelled.
func (s *Socket) Run(ctx context.Context) error {
	os.Remove(s.path)

	ln, err := net.Listen("unix", s.path)
	if err != nil {
		return err
	}
	s.ln = ln

	// Set socket permissions: readable by greeter user.
	os.Chmod(s.path, 0666)

	log.Printf("[dm/socket] listening on %s", s.path)

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
		s.conns[conn] = struct{}{}
		s.mu.Unlock()

		go s.handleConn(conn)
	}
}

// handleConn reads commands from a greeter connection.
// The protocol is the same as snry-daemon: line-based text commands.
// For auth: "auth <password>" or JSON: {"command":"auth","password":"..."}
func (s *Socket) handleConn(conn net.Conn) {
	defer func() {
		conn.Close()
		s.mu.Lock()
		delete(s.conns, conn)
		s.mu.Unlock()
	}()

	log.Printf("[dm/socket] client connected")

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		s.dispatch(conn, line)
	}
}

// dispatch processes a single command from the greeter.
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
			username := cmd.Username
			if username == "" {
				username = "current" // TODO: get from system
			}
			s.authCh <- &Credentials{Username: username, Password: cmd.Password}
		case "lock-startup":
			// In greeter mode, always acknowledge — the lock screen IS the UI.
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
		password := strings.Join(fields[1:], " ")
		username := resolveDefaultUser()
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

// SendAuthResult sends an auth result event to all connected greeters.
func (s *Socket) SendAuthResult(success bool, message string) {
	s.emitAll(map[string]any{
		"event": "auth_result",
		"data": map[string]any{
			"success":   success,
			"message":   message,
			"remaining": 3, // default attempts remaining
			"lockedOut": false,
		},
	})
}

func (s *Socket) emit(conn net.Conn, m map[string]any) {
	data, _ := json.Marshal(m)
	conn.Write(append(data, '\n'))
}

// resolveDefaultUser finds the primary user on the system.
// Returns the first regular user (UID >= 1000) from /etc/passwd,
// or "root" as a fallback.
func resolveDefaultUser() string {
	data, err := os.ReadFile("/etc/passwd")
	if err != nil {
		return "root"
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		fields := strings.Split(line, ":")
		if len(fields) < 7 {
			continue
		}
		uid, err := strconv.Atoi(fields[2])
		if err != nil || uid < 1000 {
			continue
		}
		shell := fields[6]
		if strings.Contains(shell, "nologin") || strings.Contains(shell, "false") {
			continue
		}
		return fields[0]
	}
	return "root"
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
