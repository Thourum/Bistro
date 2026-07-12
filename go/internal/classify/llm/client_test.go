package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAICompatibleClientClassify(t *testing.T) {
	t.Parallel()

	var capturedPath string
	var capturedAuth string
	var capturedRequest openAIChatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&capturedRequest); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"severity\":\"nit\",\"category\":\"style\"}"}}],"usage":{"total_tokens":17}}`))
	}))
	defer server.Close()

	client := NewOpenAICompatibleClient("custom", "gpt-test", "secret-key", server.URL)
	severity, category, usageTokens, err := client.Classify(context.Background(), strings.Repeat("x", maxCommentBodyChars+25))

	if err != nil {
		t.Fatalf("Classify returned error: %v", err)
	}
	if severity != "nit" || category != "style" {
		t.Fatalf("classification = %q/%q, want nit/style", severity, category)
	}
	if usageTokens != 17 {
		t.Fatalf("usageTokens = %d, want 17", usageTokens)
	}
	if capturedPath != "/chat/completions" {
		t.Fatalf("path = %q, want /chat/completions", capturedPath)
	}
	if capturedAuth != "Bearer secret-key" {
		t.Fatalf("Authorization = %q", capturedAuth)
	}
	if capturedRequest.Model != "gpt-test" {
		t.Fatalf("model = %q, want gpt-test", capturedRequest.Model)
	}
	if len(capturedRequest.Messages) != 2 {
		t.Fatalf("messages len = %d, want 2", len(capturedRequest.Messages))
	}
	if capturedRequest.Messages[0].Role != "system" || !strings.Contains(capturedRequest.Messages[0].Content, "critical|warning|suggestion|nit|info") {
		t.Fatalf("system prompt did not pin taxonomy: %#v", capturedRequest.Messages[0])
	}
	if capturedRequest.Messages[1].Role != "user" || len(capturedRequest.Messages[1].Content) != maxCommentBodyChars {
		t.Fatalf("user message was not truncated to %d chars: %#v", maxCommentBodyChars, capturedRequest.Messages[1])
	}
}

func TestAnthropicClientClassify(t *testing.T) {
	t.Parallel()

	var capturedPath string
	var capturedAPIKey string
	var capturedVersion string
	var capturedRequest anthropicMessagesRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedAPIKey = r.Header.Get("x-api-key")
		capturedVersion = r.Header.Get("anthropic-version")
		if err := json.NewDecoder(r.Body).Decode(&capturedRequest); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"{\"severity\":\"warning\",\"category\":\"security\"}"}],"usage":{"input_tokens":11,"output_tokens":7}}`))
	}))
	defer server.Close()

	client := NewAnthropicClient("claude-test", "anthropic-key", server.URL)
	severity, category, usageTokens, err := client.Classify(context.Background(), "please review auth")

	if err != nil {
		t.Fatalf("Classify returned error: %v", err)
	}
	if severity != "warning" || category != "security" {
		t.Fatalf("classification = %q/%q, want warning/security", severity, category)
	}
	if usageTokens != 18 {
		t.Fatalf("usageTokens = %d, want 18", usageTokens)
	}
	if capturedPath != "/messages" {
		t.Fatalf("path = %q, want /messages", capturedPath)
	}
	if capturedAPIKey != "anthropic-key" {
		t.Fatalf("x-api-key = %q", capturedAPIKey)
	}
	if capturedVersion == "" {
		t.Fatal("anthropic-version header was empty")
	}
	if capturedRequest.Model != "claude-test" {
		t.Fatalf("model = %q, want claude-test", capturedRequest.Model)
	}
	if !strings.Contains(capturedRequest.System, "security|correctness|performance|style|readability|docs") {
		t.Fatalf("system prompt did not pin categories: %q", capturedRequest.System)
	}
	if len(capturedRequest.Messages) != 1 || capturedRequest.Messages[0].Role != "user" {
		t.Fatalf("unexpected messages: %#v", capturedRequest.Messages)
	}
}

func TestClientTypedHTTPStatusErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		statusCode int
		want       error
	}{
		{name: "auth", statusCode: http.StatusUnauthorized, want: ErrAuth},
		{name: "rate limited", statusCode: http.StatusTooManyRequests, want: ErrRateLimited},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "nope", tt.statusCode)
			}))
			defer server.Close()

			client := NewOpenAICompatibleClient("custom", "gpt-test", "secret-key", server.URL)
			_, _, _, err := client.Classify(context.Background(), "body")

			if !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want errors.Is(..., %v)", err, tt.want)
			}
			if strings.Contains(err.Error(), "secret-key") {
				t.Fatal("error leaked API key")
			}
		})
	}
}

func TestClientRejectsInvalidClassification(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"severity\":\"blocker\",\"category\":\"style\"}"}}],"usage":{"total_tokens":4}}`))
	}))
	defer server.Close()

	client := NewOpenAICompatibleClient("custom", "gpt-test", "secret-key", server.URL)
	_, _, _, err := client.Classify(context.Background(), "body")

	if err == nil {
		t.Fatal("expected invalid classification error")
	}
	if strings.Contains(err.Error(), "secret-key") {
		t.Fatal("error leaked API key")
	}
}

func TestClientRespectsContextCancellation(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := NewOpenAICompatibleClient("custom", "gpt-test", "secret-key", server.URL)
	_, _, _, err := client.Classify(ctx, "body")

	if err == nil {
		t.Fatal("expected context cancellation error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}
