package handler

import (
	"context"
	"testing"
)

type mockCurrentUserReader struct {
	login string
	found bool
}

func (m *mockCurrentUserReader) Get(context.Context, string) (string, bool, error) {
	return m.login, m.found, nil
}
func extractData(t *testing.T, resp interface{}) map[string]interface{} {
	t.Helper()
	env, ok := resp.(map[string]interface{})
	if !ok {
		t.Fatalf("response is not a map, got %T", resp)
	}
	data, ok := env["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("data is not a map, got %T", env["data"])
	}
	return data
}

type spyBroadcaster struct{ messages []interface{} }

func (s *spyBroadcaster) Broadcast(message interface{}) { s.messages = append(s.messages, message) }
