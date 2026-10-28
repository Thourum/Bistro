package engine

import (
	"net"
	"testing"
)

// TestWSListenAddr pins the sync engine's WebSocket listen address to
// loopback. Binding to ":<port>" (the wildcard address) would expose every
// WS handler — including the CHECKOUT_PR_HEAD worktree mutation — to any host
// that can route to this machine. Bistro is a local-first desktop app with no
// server component, so the listener must never leave the loopback interface.
func TestWSListenAddr(t *testing.T) {
	tests := []struct {
		name string
		port int
		want string
	}{
		{name: "fixed port", port: 8080, want: "127.0.0.1:8080"},
		{name: "ephemeral port", port: 0, want: "127.0.0.1:0"},
		{name: "high port", port: 64568, want: "127.0.0.1:64568"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := wsListenAddr(tt.port); got != tt.want {
				t.Errorf("wsListenAddr(%d) = %q, want %q", tt.port, got, tt.want)
			}
		})
	}
}

// TestWSListenAddrBindsLoopbackOnly proves the computed address actually binds
// a loopback-only socket rather than the wildcard address.
func TestWSListenAddrBindsLoopbackOnly(t *testing.T) {
	listener, err := net.Listen("tcp", wsListenAddr(0))
	if err != nil {
		t.Fatalf("Listen(%q): %v", wsListenAddr(0), err)
	}
	defer listener.Close() //nolint:errcheck

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("listener addr %T, want *net.TCPAddr", listener.Addr())
	}
	if addr.IP.IsUnspecified() {
		t.Errorf("listener bound to wildcard address %s — WS API is reachable from other hosts", addr)
	}
	if !addr.IP.IsLoopback() {
		t.Errorf("listener IP %s is not loopback", addr.IP)
	}
}
