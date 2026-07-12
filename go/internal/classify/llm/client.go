package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	maxCommentBodyChars      = 4000
	clientTimeout            = 30 * time.Second
	anthropicAPIVersion      = "2023-06-01"
	openAIDefaultBaseURL     = "https://api.openai.com/v1"
	openRouterDefaultBaseURL = "https://openrouter.ai/api/v1"
	ollamaDefaultBaseURL     = "http://localhost:11434/v1"
	anthropicDefaultBaseURL  = "https://api.anthropic.com/v1"
)

var (
	ErrAuth        = errors.New("llm authentication failed")
	ErrRateLimited = errors.New("llm rate limited")
)

type ClassifierClient interface {
	Classify(ctx context.Context, body string) (severity string, category string, usageTokens int, err error)
}

type OpenAICompatibleClient struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
	model      string
}

type AnthropicClient struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
	model      string
}

type openAIChatRequest struct {
	Model    string              `json:"model"`
	Messages []openAIChatMessage `json:"messages"`
}

type openAIChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIChatResponse struct {
	Choices []struct {
		Message openAIChatMessage `json:"message"`
	} `json:"choices"`
	Usage struct {
		TotalTokens int `json:"total_tokens"`
	} `json:"usage"`
}

type anthropicMessagesRequest struct {
	Model     string                    `json:"model"`
	MaxTokens int                       `json:"max_tokens"`
	System    string                    `json:"system"`
	Messages  []anthropicMessageRequest `json:"messages"`
}

type anthropicMessageRequest struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicMessagesResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

type classificationReply struct {
	Severity string `json:"severity"`
	Category string `json:"category"`
}

func NewOpenAICompatibleClient(provider string, model string, apiKey string, endpoint string) *OpenAICompatibleClient {
	return &OpenAICompatibleClient{
		apiKey:     apiKey,
		baseURL:    resolveOpenAICompatibleBaseURL(provider, endpoint),
		httpClient: http.DefaultClient,
		model:      model,
	}
}

func NewAnthropicClient(model string, apiKey string, endpoint string) *AnthropicClient {
	baseURL := strings.TrimRight(endpoint, "/")
	if baseURL == "" {
		baseURL = anthropicDefaultBaseURL
	}

	return &AnthropicClient{
		apiKey:     apiKey,
		baseURL:    baseURL,
		httpClient: http.DefaultClient,
		model:      model,
	}
}

func (c *OpenAICompatibleClient) Classify(ctx context.Context, body string) (string, string, int, error) {
	requestBody := openAIChatRequest{
		Model: c.model,
		Messages: []openAIChatMessage{
			{Role: "system", Content: systemPrompt()},
			{Role: "user", Content: truncateCommentBody(body)},
		},
	}

	payload, err := json.Marshal(requestBody)
	if err != nil {
		return "", "", 0, fmt.Errorf("marshal OpenAI-compatible classification request: %w", err)
	}

	requestCtx, cancel := context.WithTimeout(ctx, clientTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", "", 0, fmt.Errorf("build OpenAI-compatible classification request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", "", 0, fmt.Errorf("send OpenAI-compatible classification request: %w", err)
	}
	defer resp.Body.Close()

	if err := classifyHTTPStatus(resp.StatusCode); err != nil {
		return "", "", 0, err
	}

	var responseBody openAIChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&responseBody); err != nil {
		return "", "", 0, fmt.Errorf("decode OpenAI-compatible classification response: %w", err)
	}
	if len(responseBody.Choices) == 0 {
		return "", "", 0, errors.New("OpenAI-compatible classification response had no choices")
	}

	severity, category, err := parseClassificationReply(responseBody.Choices[0].Message.Content)
	if err != nil {
		return "", "", 0, err
	}

	return severity, category, responseBody.Usage.TotalTokens, nil
}

func (c *AnthropicClient) Classify(ctx context.Context, body string) (string, string, int, error) {
	requestBody := anthropicMessagesRequest{
		Model:     c.model,
		MaxTokens: 64,
		System:    systemPrompt(),
		Messages: []anthropicMessageRequest{
			{Role: "user", Content: truncateCommentBody(body)},
		},
	}

	payload, err := json.Marshal(requestBody)
	if err != nil {
		return "", "", 0, fmt.Errorf("marshal Anthropic classification request: %w", err)
	}

	requestCtx, cancel := context.WithTimeout(ctx, clientTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, c.baseURL+"/messages", bytes.NewReader(payload))
	if err != nil {
		return "", "", 0, fmt.Errorf("build Anthropic classification request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", anthropicAPIVersion)
	if c.apiKey != "" {
		req.Header.Set("x-api-key", c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", "", 0, fmt.Errorf("send Anthropic classification request: %w", err)
	}
	defer resp.Body.Close()

	if err := classifyHTTPStatus(resp.StatusCode); err != nil {
		return "", "", 0, err
	}

	var responseBody anthropicMessagesResponse
	if err := json.NewDecoder(resp.Body).Decode(&responseBody); err != nil {
		return "", "", 0, fmt.Errorf("decode Anthropic classification response: %w", err)
	}
	content, err := firstAnthropicText(responseBody)
	if err != nil {
		return "", "", 0, err
	}

	severity, category, err := parseClassificationReply(content)
	if err != nil {
		return "", "", 0, err
	}

	return severity, category, responseBody.Usage.InputTokens + responseBody.Usage.OutputTokens, nil
}

func resolveOpenAICompatibleBaseURL(provider string, endpoint string) string {
	if strings.EqualFold(provider, "custom") && endpoint != "" {
		return strings.TrimRight(endpoint, "/")
	}

	switch strings.ToLower(provider) {
	case "openrouter":
		return openRouterDefaultBaseURL
	case "ollama":
		return ollamaDefaultBaseURL
	case "custom":
		if endpoint != "" {
			return strings.TrimRight(endpoint, "/")
		}
		return openAIDefaultBaseURL
	default:
		return openAIDefaultBaseURL
	}
}

func classifyHTTPStatus(statusCode int) error {
	switch statusCode {
	case http.StatusUnauthorized:
		return ErrAuth
	case http.StatusTooManyRequests:
		return ErrRateLimited
	}
	if statusCode < 200 || statusCode >= 300 {
		return fmt.Errorf("llm classification request failed with status %d", statusCode)
	}
	return nil
}

func parseClassificationReply(raw string) (string, string, error) {
	var reply classificationReply
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &reply); err != nil {
		return "", "", fmt.Errorf("decode classification JSON reply: %w", err)
	}
	if !isValidSeverity(reply.Severity) {
		return "", "", fmt.Errorf("invalid classification severity %q", reply.Severity)
	}
	if !isValidCategory(reply.Category) {
		return "", "", fmt.Errorf("invalid classification category %q", reply.Category)
	}
	return reply.Severity, reply.Category, nil
}

func firstAnthropicText(responseBody anthropicMessagesResponse) (string, error) {
	for _, item := range responseBody.Content {
		if item.Type == "text" && item.Text != "" {
			return item.Text, nil
		}
	}
	return "", errors.New("Anthropic classification response had no text content")
}

func systemPrompt() string {
	return "Classify this code review comment. Valid severities: critical|warning|suggestion|nit|info. Valid categories: security|correctness|performance|style|readability|docs. Respond with ONLY a JSON object {\"severity\":\"...\",\"category\":\"...\"}."
}

func truncateCommentBody(body string) string {
	runes := []rune(body)
	if len(runes) <= maxCommentBodyChars {
		return body
	}
	return string(runes[:maxCommentBodyChars])
}

func isValidSeverity(severity string) bool {
	switch severity {
	case "critical", "warning", "suggestion", "nit", "info":
		return true
	default:
		return false
	}
}

func isValidCategory(category string) bool {
	switch category {
	case "security", "correctness", "performance", "style", "readability", "docs":
		return true
	default:
		return false
	}
}

func discardResponseBody(body io.ReadCloser) {
	_, _ = io.Copy(io.Discard, body)
}
