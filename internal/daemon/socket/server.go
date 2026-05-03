package socket

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
)

type Dispatcher interface {
	DispatchCommand(line string)
}

type SnapshotProvider interface {
	EmitSnapshot(func(map[string]any))
}

type Server struct {
	path     string
	listener net.Listener
	emitter  *Emitter
	snapshot func() []SnapshotProvider
}

type Emitter struct {
	clients sync.Map
}

func New(path string) *Server {
	return &Server{path: path, emitter: &Emitter{}}
}

func (s *Server) Emitter() *Emitter {
	return s.emitter
}

func (e *Emitter) AddClient(conn net.Conn) {
	e.clients.Store(conn, struct{}{})
}

func (e *Emitter) RemoveClient(conn net.Conn) {
	e.clients.Delete(conn)
}

func (e *Emitter) Emit(m map[string]any) {
	data, err := json.Marshal(m)
	if err != nil {
		log.Printf("socket emitter: marshal error: %v", err)
		return
	}
	e.EmitRaw(append(data, '\n'))
}

func (e *Emitter) EmitRaw(data []byte) {
	var dead []net.Conn
	e.clients.Range(func(key, _ any) bool {
		conn := key.(net.Conn)
		if _, werr := conn.Write(data); werr != nil {
			dead = append(dead, conn)
		}
		return true
	})
	for _, conn := range dead {
		conn.Close()
		e.clients.Delete(conn)
	}
}

func (s *Server) Run(ctx context.Context, dispatch Dispatcher, snapshot func() []SnapshotProvider) error {
	s.snapshot = snapshot
	os.Remove(s.path)
	listener, err := net.Listen("unix", s.path)
	if err != nil {
		return fmt.Errorf("listen %s: %w", s.path, err)
	}
	s.listener = listener
	defer listener.Close()
	log.Printf("socket: listening on %s", s.path)

	go func() {
		<-ctx.Done()
		listener.Close()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
			}
			log.Printf("socket: accept error: %v", err)
			continue
		}
		go s.handleClient(conn, dispatch)
	}
}

func (s *Server) handleClient(conn net.Conn, dispatch Dispatcher) {
	defer conn.Close()
	s.emitter.AddClient(conn)
	defer s.emitter.RemoveClient(conn)

	log.Printf("socket: client connected: %s", conn.RemoteAddr())

	// Read commands in background so snapshot writes can't block processing.
	cmdCh := make(chan string, 16)
	go func() {
		scanner := bufio.NewScanner(conn)
		for scanner.Scan() {
			cmdCh <- scanner.Text()
		}
		close(cmdCh)
	}()

	// Process commands in background so we don't block on snapshot writes.
	doneCh := make(chan struct{})
	go func() {
		defer close(doneCh)
		for line := range cmdCh {
			if dispatch != nil {
				dispatch.DispatchCommand(line)
			}
		}
	}()

	// Emit snapshots (writes to conn; reader goroutine runs concurrently).
	if s.snapshot != nil {
		for _, snap := range s.snapshot() {
			snap.EmitSnapshot(s.emitter.Emit)
		}
	}

	// Wait for command processing to finish (client disconnected).
	<-doneCh
	log.Printf("socket: client disconnected: %s", conn.RemoteAddr())
}
