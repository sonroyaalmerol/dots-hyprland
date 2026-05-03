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
	data = append(data, '\n')
	e.clients.Range(func(key, _ any) bool {
		conn := key.(net.Conn)
		if _, werr := conn.Write(data); werr != nil {
			log.Printf("socket emitter: client write error: %v", werr)
			conn.Close()
			e.clients.Delete(conn)
		}
		return true
	})
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

	if s.snapshot != nil {
		for _, snap := range s.snapshot() {
			snap.EmitSnapshot(s.emitter.Emit)
		}
	}

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		if dispatch != nil {
			dispatch.DispatchCommand(scanner.Text())
		}
	}
	if err := scanner.Err(); err != nil {
		log.Printf("socket: client read error: %v", err)
	}
	log.Printf("socket: client disconnected: %s", conn.RemoteAddr())
}
