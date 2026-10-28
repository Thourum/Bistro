package engine

import (
	"encoding/json"
	"fmt"
	"io"
)

type readySignal struct {
	Event  string `json:"event"`
	WSPort int    `json:"wsPort"`
}

// emitReady writes a single machine-readable JSON line announcing that
// migrations have completed and the WebSocket server is accepting
// connections on the given port. Electron parses this line from stdout to
// learn the dynamically assigned port and to know it is safe to issue reads.
func emitReady(w io.Writer, port int) error {
	data, err := json.Marshal(readySignal{Event: "ready", WSPort: port})
	if err != nil {
		return fmt.Errorf("marshal ready signal: %w", err)
	}
	if _, err := fmt.Fprintf(w, "%s\n", data); err != nil {
		return fmt.Errorf("write ready signal: %w", err)
	}
	return nil
}
