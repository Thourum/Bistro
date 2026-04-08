package ws

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
)

// Server manages a WebSocket HTTP server and its lifecycle.
type Server struct {
	listener   net.Listener
	upgrader   websocket.Upgrader
	mu         sync.Mutex
	stopChan   chan struct{}
	httpServer *http.Server
	conns      map[*websocket.Conn]struct{}
}

// isLocalOrigin reports whether the request origin is localhost, 127.0.0.1,
// or absent (non-browser client). This keeps the server safe for a local
// desktop app while still allowing development tooling.
func isLocalOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true // non-browser client (e.g. Electron IPC, tests)
	}
	return strings.Contains(origin, "localhost") || strings.Contains(origin, "127.0.0.1")
}

// NewServer constructs a Server that only accepts connections from localhost origins.
func NewServer() *Server {
	return &Server{
		upgrader: websocket.Upgrader{
			CheckOrigin: isLocalOrigin,
		},
		stopChan: make(chan struct{}),
		conns:    make(map[*websocket.Conn]struct{}),
	}
}

// Start begins listening on addr and serves WebSocket connections.
// Passing ":0" selects a random available port.
func (s *Server) Start(addr string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var err error
	s.listener, err = net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.handleWebSocket)

	s.httpServer = &http.Server{Handler: mux}

	go func() {
		s.httpServer.Serve(s.listener) //nolint:errcheck
	}()

	return nil
}

// Stop signals all WebSocket handlers to return, closes all active connections,
// and shuts down the HTTP server.
func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Signal all handlers to stop before closing the server.
	select {
	case <-s.stopChan:
		// Already closed.
	default:
		close(s.stopChan)
	}

	// Close every tracked connection so goroutines blocked on ReadJSON unblock.
	for conn := range s.conns {
		conn.Close()
	}

	if s.httpServer != nil {
		return s.httpServer.Close()
	}
	return nil
}

// registerConn adds conn to the tracked set. Must be called with s.mu held.
func (s *Server) registerConn(conn *websocket.Conn) {
	s.conns[conn] = struct{}{}
}

// deregisterConn removes conn from the tracked set. Must be called with s.mu held.
func (s *Server) deregisterConn(conn *websocket.Conn) {
	delete(s.conns, conn)
}

// handleWebSocket upgrades an HTTP connection to WebSocket and keeps it open
// until the client disconnects or the server stops.
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	s.mu.Lock()
	s.registerConn(conn)
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.deregisterConn(conn)
		s.mu.Unlock()
		conn.Close()
	}()

	for {
		select {
		case <-s.stopChan:
			return
		default:
			var msg map[string]interface{}
			if err := conn.ReadJSON(&msg); err != nil {
				return
			}
		}
	}
}
