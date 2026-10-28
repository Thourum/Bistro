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
	"time"

	"github.com/gorilla/websocket"
)

// clientSendBufferSize bounds the messages queued for one client. A client
// whose buffer is full is not draining its socket, so it is dropped rather
// than allowed to stall broadcasts for everyone else.
const clientSendBufferSize = 256

// clientWriteTimeout ends the writer of a client whose peer stopped reading.
// Without it a full TCP send buffer blocks the writer goroutine forever.
const clientWriteTimeout = 10 * time.Second

// Server manages a WebSocket HTTP server and its lifecycle.
type Server struct {
	listener   net.Listener
	upgrader   websocket.Upgrader
	mu         sync.Mutex
	stopChan   chan struct{}
	httpServer *http.Server
	conns      map[*websocket.Conn]*client
	dispatcher *Dispatcher
}

// client is one WebSocket connection. Only its writePump goroutine writes to
// conn, which satisfies gorilla/websocket's one-writer rule without a mutex.
// Everything else enqueues onto send and never blocks.
type client struct {
	conn      *websocket.Conn
	send      chan []byte
	done      chan struct{}
	closeOnce sync.Once
}

func newClient(conn *websocket.Conn) *client {
	return &client{
		conn: conn,
		send: make(chan []byte, clientSendBufferSize),
		done: make(chan struct{}),
	}
}

// enqueue queues msg for the writer without blocking. It reports false when
// the buffer is full, meaning the client is not keeping up.
func (c *client) enqueue(msg []byte) bool {
	select {
	case c.send <- msg:
		return true
	default:
		return false
	}
}

// close stops the writer and closes the socket. Safe to call repeatedly.
// send is deliberately never closed, so a concurrent enqueue cannot panic.
func (c *client) close() {
	c.closeOnce.Do(func() {
		close(c.done)
		c.conn.Close() //nolint:errcheck
	})
}

// writePump is the only goroutine that writes to conn. It exits, and closes
// the client, on shutdown or on a failed or timed-out write.
func (c *client) writePump(logger *slog.Logger, remoteAddr string) {
	defer c.close()
	for {
		select {
		case <-c.done:
			return
		case msg := <-c.send:
			if err := c.conn.SetWriteDeadline(time.Now().Add(clientWriteTimeout)); err != nil {
				logger.Warn("ws set write deadline failed", "remote_addr", remoteAddr, "error", err)
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				logger.Warn("ws write failed", "remote_addr", remoteAddr, "error", err)
				return
			}
		}
	}
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
		conns:      make(map[*websocket.Conn]*client),
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
	// Enqueueing never blocks, so holding s.mu here is safe. A client whose
	// buffer is full is dropped so one stalled peer cannot delay the rest.
	s.mu.Lock()
	clientCount := len(s.conns)
	var dropped int
	for conn, c := range s.conns {
		if !c.enqueue(data) {
			dropped++
			s.deregisterConn(conn)
		}
	}
	s.mu.Unlock()

	// Broadcasts may include cached private PR content; never log the body.
	s.dispatcher.logger.Debug("broadcast start",
		"clients", clientCount,
		"payload_bytes", len(data),
	)
	if dropped > 0 {
		s.dispatcher.logger.Warn("broadcast dropped slow clients",
			"clients", clientCount,
			"dropped", dropped,
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

	// Close every tracked client so writers exit and readers blocked on
	// ReadMessage unblock.
	for _, c := range s.conns {
		c.close()
	}

	if s.httpServer != nil {
		return s.httpServer.Close()
	}
	return nil
}

// registerConn adds conn to the tracked set and returns its client.
// Must be called with s.mu held.
func (s *Server) registerConn(conn *websocket.Conn) *client {
	c := newClient(conn)
	s.conns[conn] = c
	return c
}

// deregisterConn removes conn from the tracked set and closes it. Must be
// called with s.mu held; it is a no-op for connections already removed.
func (s *Server) deregisterConn(conn *websocket.Conn) {
	c, ok := s.conns[conn]
	if !ok {
		return
	}
	delete(s.conns, conn)
	c.close()
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
	c := s.registerConn(conn)
	clientCount := len(s.conns)
	s.mu.Unlock()
	go c.writePump(s.dispatcher.logger, remoteAddr)
	s.dispatcher.logger.Debug("ws client registered",
		"remote_addr", remoteAddr,
		"total_clients", clientCount,
	)

	defer func() {
		s.mu.Lock()
		s.deregisterConn(conn)
		remaining := len(s.conns)
		s.mu.Unlock()
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
				if !c.enqueue(responseBytes) {
					s.dispatcher.logger.Warn("ws client not draining responses, dropping",
						"remote_addr", remoteAddr,
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
