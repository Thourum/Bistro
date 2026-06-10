package ws

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
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
	dispatcher *Dispatcher
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
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	return &Server{
		upgrader: websocket.Upgrader{
			CheckOrigin: isLocalOrigin,
		},
		stopChan:   make(chan struct{}),
		conns:      make(map[*websocket.Conn]struct{}),
		dispatcher: NewDispatcher(logger),
	}
}

// NewServerWithLogger constructs a Server with a custom logger.
func NewServerWithLogger(logger *slog.Logger) *Server {
	return &Server{
		upgrader: websocket.Upgrader{
			CheckOrigin: isLocalOrigin,
		},
		stopChan:   make(chan struct{}),
		conns:      make(map[*websocket.Conn]struct{}),
		dispatcher: NewDispatcher(logger),
	}
}

// RegisterHandler associates a message type with a handler on the dispatcher.
func (s *Server) RegisterHandler(messageType string, handler MessageHandler) {
	s.dispatcher.Register(messageType, handler)
}

// Broadcast sends a message to all connected WebSocket clients.
func (s *Server) Broadcast(msg interface{}) {
	data, err := json.Marshal(msg)
	if err != nil {
		s.dispatcher.logger.Error("broadcast marshal failed", "error", err)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	clientCount := len(s.conns)
	s.dispatcher.logger.Debug("broadcast start",
		"clients", clientCount,
		"payload_bytes", len(data),
		"payload", truncateForLog(data, 4096),
	)
	var writeErrs int
	for conn := range s.conns {
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			writeErrs++
			s.dispatcher.logger.Warn("broadcast write failed",
				"error", err,
				"remote_addr", conn.RemoteAddr().String(),
			)
		}
	}
	if writeErrs > 0 {
		s.dispatcher.logger.Error("broadcast had write errors",
			"clients", clientCount,
			"failed", writeErrs,
		)
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
	remoteAddr := r.RemoteAddr
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.dispatcher.logger.Warn("ws upgrade failed",
			"remote_addr", remoteAddr,
			"origin", r.Header.Get("Origin"),
			"error", err,
		)
		return
	}
	s.dispatcher.logger.Info("ws connect",
		"remote_addr", remoteAddr,
		"origin", r.Header.Get("Origin"),
	)

	s.mu.Lock()
	s.registerConn(conn)
	clientCount := len(s.conns)
	s.mu.Unlock()
	s.dispatcher.logger.Debug("ws client registered",
		"remote_addr", remoteAddr,
		"total_clients", clientCount,
	)

	defer func() {
		s.mu.Lock()
		s.deregisterConn(conn)
		remaining := len(s.conns)
		s.mu.Unlock()
		conn.Close()
		s.dispatcher.logger.Info("ws disconnect",
			"remote_addr", remoteAddr,
			"total_clients", remaining,
		)
	}()

	baseCtx := r.Context()
	for {
		select {
		case <-s.stopChan:
			s.dispatcher.logger.Debug("ws stop signal received",
				"remote_addr", remoteAddr,
			)
			return
		default:
			_, raw, err := conn.ReadMessage()
			if err != nil {
				s.dispatcher.logger.Debug("ws read ended",
					"remote_addr", remoteAddr,
					"error", err,
				)
				return
			}
			response, dispatchErr := s.dispatcher.Dispatch(baseCtx, raw)
			if dispatchErr != nil {
				s.dispatcher.logger.Error("dispatch error",
					"remote_addr", remoteAddr,
					"error", dispatchErr,
				)
				continue
			}
			if response != nil {
				responseBytes, marshalErr := json.Marshal(response)
				if marshalErr != nil {
					s.dispatcher.logger.Error("marshal response error",
						"remote_addr", remoteAddr,
						"error", marshalErr,
					)
					continue
				}
				if writeErr := conn.WriteMessage(websocket.TextMessage, responseBytes); writeErr != nil {
					s.dispatcher.logger.Warn("ws write failed",
						"remote_addr", remoteAddr,
						"error", writeErr,
					)
					return
				}
				s.dispatcher.logger.Debug("ws response sent",
					"remote_addr", remoteAddr,
					"bytes", len(responseBytes),
				)
			}
		}
	}
}
