package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
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
