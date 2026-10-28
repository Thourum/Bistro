package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// TestBroadcastConcurrentWithResponses is the regression test for the
// "panic: concurrent write to websocket connection" crash: the dispatch
// loop's response writes and Broadcast (async pushes from the LLM tier and
// sync events) raced on the same *websocket.Conn. Every write must hold the
// connection's write mutex. Run with -race.
func TestBroadcastConcurrentWithResponses(t *testing.T) {
	server := NewServer()
	server.RegisterHandler("PING", func(_ context.Context, msg InboundMessage) (interface{}, error) {
		return map[string]interface{}{"id": msg.ID, "type": "PONG", "data": map[string]interface{}{}}, nil
	})
	if err := server.Start(":0"); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer server.Stop() //nolint:errcheck

	url := fmt.Sprintf("ws://127.0.0.1:%d/ws", server.Port())
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// Reader drains everything so writes never block on flow control.
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	// Writer goroutine: a stream of PING requests → dispatch-loop responses.
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			msg, _ := json.Marshal(map[string]interface{}{
				"id": fmt.Sprintf("req-%d", i), "type": "PING", "data": map[string]interface{}{},
			})
			if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		}
	}()
	// Broadcast goroutine: async pushes racing the responses.
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			server.Broadcast(map[string]interface{}{
				"type": "COMMENTS_READY",
				"data": map[string]interface{}{"pr_id": "pr-1"},
			})
		}
	}()

	wg.Wait()
	// Give in-flight responses a beat, then close; the test passes if no
	// panic/race fired while requests and broadcasts overlapped.
	time.Sleep(100 * time.Millisecond)
	conn.Close()
	select {
	case <-readerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("reader did not finish")
	}
}

// TestStalledClientDoesNotBlockOtherClients is the regression test for one
// client that stops reading wedging the engine: its full TCP buffers used to
// block the synchronous write in Broadcast, which stalled every other client.
// The stalled client must be dropped while a healthy client keeps receiving
// broadcasts and responses. Run with -race.
func TestStalledClientDoesNotBlockOtherClients(t *testing.T) {
	const (
		broadcastCount = 1000
		payloadBytes   = 32 * 1024
		// Keeps the healthy client's backlog well under clientSendBufferSize,
		// so only the stalled client is ever dropped.
		maxHealthyBacklog = 64
		testTimeout       = 20 * time.Second
	)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	server := NewServer()
	server.RegisterHandler("PING", func(_ context.Context, msg InboundMessage) (interface{}, error) {
		return map[string]interface{}{"id": msg.ID, "type": "PONG", "data": map[string]interface{}{}}, nil
	})
	if err := server.Start(":0"); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer server.Stop() //nolint:errcheck

	url := fmt.Sprintf("ws://127.0.0.1:%d/ws", server.Port())
	stalled, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial stalled: %v", err)
	}
	defer stalled.Close()

	healthy, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial healthy: %v", err)
	}
	defer healthy.Close()

	var received atomic.Int64
	go func() {
		for {
			if _, _, err := healthy.ReadMessage(); err != nil {
				return
			}
			received.Add(1)
		}
	}()

	broadcastsDone := make(chan struct{})
	payload := strings.Repeat("x", payloadBytes)
	go func() {
		defer close(broadcastsDone)
		for i := 0; i < broadcastCount; i++ {
			if !waitFor(ctx, func() bool { return int64(i)-received.Load() <= maxHealthyBacklog }) {
				return
			}
			server.Broadcast(map[string]any{
				"type": "COMMENTS_READY",
				"data": map[string]string{"body": payload},
			})
		}
	}()

	select {
	case <-broadcastsDone:
	case <-ctx.Done():
		t.Fatal("broadcasts blocked by a client that stopped reading")
	}

	ping, err := json.Marshal(map[string]any{"id": "ping-1", "type": "PING", "data": map[string]any{}})
	if err != nil {
		t.Fatalf("marshal ping: %v", err)
	}
	if err := healthy.WriteMessage(websocket.TextMessage, ping); err != nil {
		t.Fatalf("write ping: %v", err)
	}
	wantReceived := int64(broadcastCount + 1) // every broadcast plus the PONG
	if !waitFor(ctx, func() bool { return received.Load() >= wantReceived }) {
		t.Fatalf("healthy client received %d messages, want %d", received.Load(), wantReceived)
	}
}

// waitFor polls cond until it holds, reporting false if ctx ends first.
func waitFor(ctx context.Context, cond func() bool) bool {
	for !cond() {
		if ctx.Err() != nil {
			return false
		}
		time.Sleep(time.Millisecond)
	}
	return true
}
