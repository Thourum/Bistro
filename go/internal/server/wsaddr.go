package server

import (
	"net"
	"strconv"
)

// loopbackHost is the only interface the sync engine's WebSocket API is served
// on. Bistro is local-first: the engine, the SQLite cache, and the Electron UI
// all live on one machine, and every WS handler is an unauthenticated local
// privilege (reading cached PR data, mutating the user's git worktree via
// CHECKOUT_PR_HEAD). A wildcard bind (":<port>") would make all of that
// reachable from any host that can route to this machine.
const loopbackHost = "127.0.0.1"

// wsListenAddr returns the loopback listen address for the WebSocket server.
// Port 0 asks the kernel for an ephemeral port; the actual port is reported
// back to Electron via the ready event.
func wsListenAddr(port int) string {
	return net.JoinHostPort(loopbackHost, strconv.Itoa(port))
}
