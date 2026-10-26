package server

import "context"

// wsEventMsg is the envelope sent to WebSocket clients for orchestrator events.
type wsEventMsg struct {
	Type string         `json:"type"`
	Data map[string]any `json:"data"`
}

// startEventBridge subscribes to the current orchestrator's event channel and
// broadcasts each event to all connected WebSocket clients. Any previously
// running bridge goroutine is cancelled first so stale goroutines do not leak
// or deliver events from a superseded orchestrator.
//
// Must be called with a.provMu held for writing (or before the App is shared).
func (a *App) startEventBridge(parent context.Context) {
	if a.bridgeCancel != nil {
		a.bridgeCancel()
	}
	if a.orchestrator == nil || a.wsServer == nil {
		a.bridgeCancel = nil
		return
	}

	ctx, cancel := context.WithCancel(parent)
	a.bridgeCancel = cancel

	o := a.orchestrator
	ch := o.GetEventChannel()
	go func() {
		defer o.UnsubscribeEventChannel(ch)
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-ch:
				if !ok {
					return
				}
				a.wsServer.Broadcast(wsEventMsg{Type: ev.Type, Data: ev.Data})
			}
		}
	}()
}
