package ws

import (
	"fmt"
	"net"
	"net/http"
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
}

// NewServer constructs a Server with permissive origin checking (localhost only in production).
func NewServer() *Server {
	return &Server{
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins (localhost only in production)
			},
		},
		stopChan: make(chan struct{}),
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

// Stop shuts down the HTTP server, closing all active connections.
func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.httpServer != nil {
		return s.httpServer.Close()
	}
	return nil
}

// handleWebSocket upgrades an HTTP connection to WebSocket and keeps it open
// until the client disconnects or the server stops.
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

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
