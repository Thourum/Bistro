package ws

import (
	"net/http"
	"testing"
)

// TestIsLocalOrigin covers the Origin allowlist for the localhost-only
// WebSocket API. Substring matching is not sufficient: an attacker-controlled
// host may embed "localhost" or "127.0.0.1" anywhere in its name.
func TestIsLocalOrigin(t *testing.T) {
	tests := []struct {
		name   string
		origin string
		want   bool
	}{
		// Absent Origin: non-browser clients (Electron main `ws` client, tests).
		{name: "empty origin allowed", origin: "", want: true},

		// Packaged Electron renderer loads over file://.
		{name: "file origin allowed", origin: "file://", want: true},

		// Dev server and renderer WebSocket origins.
		{name: "http localhost with port", origin: "http://localhost:5173", want: true},
		{name: "http localhost no port", origin: "http://localhost", want: true},
		{name: "http 127.0.0.1", origin: "http://127.0.0.1:8080", want: true},
		{name: "ipv6 loopback", origin: "http://[::1]:8080", want: true},
		{name: "https localhost", origin: "https://localhost:3000", want: true},
		{name: "ws scheme localhost", origin: "ws://localhost:8080", want: true},

		// Spoofing attempts that substring matching would accept.
		{name: "localhost as subdomain suffix", origin: "http://localhost.evil.com", want: false},
		{name: "localhost as subdomain prefix", origin: "http://evil.localhost.com", want: false},
		{name: "loopback ip embedded in host", origin: "http://127.0.0.1.evil.com", want: false},
		{name: "loopback ip in path", origin: "http://evil.com/127.0.0.1", want: false},
		{name: "localhost in userinfo", origin: "http://localhost@evil.com", want: false},
		{name: "localhost in query", origin: "http://evil.com?x=localhost", want: false},
		{name: "decimal loopback form", origin: "http://2130706433", want: false},

		// Non-local origins and malformed values.
		{name: "remote host", origin: "http://example.com", want: false},
		{name: "null origin rejected", origin: "null", want: false},
		{name: "unparseable origin", origin: "http://[::1", want: false},
		{name: "non-http scheme on localhost", origin: "javascript://localhost", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:8080/ws", nil)
			if err != nil {
				t.Fatalf("NewRequest: %v", err)
			}
			if tt.origin != "" {
				r.Header.Set("Origin", tt.origin)
			}
			if got := isLocalOrigin(r); got != tt.want {
				t.Errorf("isLocalOrigin(%q) = %v, want %v", tt.origin, got, tt.want)
			}
		})
	}
}
