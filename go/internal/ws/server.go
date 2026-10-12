package ws

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
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
	// conns maps each connection to its write mutex. gorilla/websocket
	// allows only ONE concurrent writer per connection; the dispatch loop
	// (responses) and Broadcast (async pushes: sync events, LLM tier) write
	// from different goroutines, so every WriteMessage must hold the
	// connection's mutex.
	conns      map[*websocket.Conn]*sync.Mutex
	dispatcher *Dispatcher
}

// localOriginSchemes are the URL schemes a local Bistro renderer may present.
// Anything else (javascript:, data:, custom app schemes) is rejected even when
// the host parses as loopback.
var localOriginSchemes = map[string]bool{
	"http":  true,
	"https": true,
	"ws":    true,
	"wss":   true,
}

// isLocalOrigin reports whether the request origin belongs to this machine's
// Bistro UI. The allowlist is: an absent Origin (non-browser clients such as
// the Electron main-process `ws` client and tests), "file://" (the packaged
// renderer loaded via loadFile), and http/https/ws/wss origins whose host is
// "localhost" or a loopback IP.
//
// The origin is parsed as a URL and matched on the host component exactly —
// never by substring. Substring matching is spoofable, because an
// attacker-controlled host can embed the allowed text anywhere in the origin
// ("http://localhost.evil.com", "http://evil.com/127.0.0.1"), and a browser
// on the victim's machine can reach the loopback listener.
func isLocalOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true // non-browser client (e.g. Electron IPC, tests)
	}

	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}

	scheme := strings.ToLower(parsed.Scheme)
	// The packaged renderer is loaded from disk; Chromium reports "file://".
	// A remote page can never make the browser send this origin.
	if scheme == "file" && parsed.Host == "" {
		return true
	}
	if !localOriginSchemes[scheme] {
		return false
	}
	// A serialized origin is scheme://host[:port] — no userinfo, path, query,
	// or fragment. Anything else is malformed and not trusted.
	if parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}

	host := parsed.Hostname()
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// NewServer constructs a Server that only accepts connections from localhost origins.
func NewServer() *Server {
	return NewServerWithLogger(slog.New(slog.NewTextHandler(os.Stderr, nil)))
}

// NewServerWithLogger constructs a Server with a custom logger.
func NewServerWithLogger(logger *slog.Logger) *Server {
	return &Server{
		upgrader: websocket.Upgrader{
			CheckOrigin: isLocalOrigin,
		},
		stopChan:   make(chan struct{}),
		conns:      make(map[*websocket.Conn]*sync.Mutex),
		dispatcher: NewDispatcher(logger),
	}
}

// RegisterHandler associates a message type with a handler on the dispatcher.
func (s *Server) RegisterHandler(messageType string, handler MessageHandler) {
	s.dispatcher.Register(messageType, handler)
}

// Broadcast sends a message to all connected WebSocket clients.
func (s *Server) Broadcast(msg any) {
	data, err := json.Marshal(msg)
	if err != nil {
		s.dispatcher.logger.Error("broadcast marshal failed", "error", err)
		return
	}
	// Snapshot under s.mu, write under each connection's own write mutex —
	// never hold s.mu across network writes, and never write a connection
	// concurrently with its dispatch-loop response writer.
	s.mu.Lock()
	clientCount := len(s.conns)
	type connWriter struct {
		conn *websocket.Conn
		wmu  *sync.Mutex
	}
	writers := make([]connWriter, 0, clientCount)
	for conn, wmu := range s.conns {
		writers = append(writers, connWriter{conn, wmu})
	}
	s.mu.Unlock()

	// Broadcasts may include cached private PR content; never log the body.
	s.dispatcher.logger.Debug("broadcast start",
		"clients", clientCount,
		"payload_bytes", len(data),
	)
	var writeErrs int
	for _, w := range writers {
		w.wmu.Lock()
		err := w.conn.WriteMessage(websocket.TextMessage, data)
		w.wmu.Unlock()
		if err != nil {
			writeErrs++
			s.dispatcher.logger.Warn("broadcast write failed",
				"error", err,
				"remote_addr", w.conn.RemoteAddr().String(),
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

// Port returns the actual TCP port the server is listening on, or 0 if the
// server has not been started. Useful when Start was called with ":0".
func (s *Server) Port() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener == nil {
		return 0
	}
	if addr, ok := s.listener.Addr().(*net.TCPAddr); ok {
		return addr.Port
	}
	return 0
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
		if err := s.httpServer.Serve(s.listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.dispatcher.logger.Error("ws server failed", "error", err)
		}
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

// registerConn adds conn to the tracked set and returns its write mutex.
// Must be called with s.mu held.
func (s *Server) registerConn(conn *websocket.Conn) *sync.Mutex {
	wmu := &sync.Mutex{}
	s.conns[conn] = wmu
	return wmu
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
	wmu := s.registerConn(conn)
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
				wmu.Lock()
				writeErr := conn.WriteMessage(websocket.TextMessage, responseBytes)
				wmu.Unlock()
				if writeErr != nil {
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
