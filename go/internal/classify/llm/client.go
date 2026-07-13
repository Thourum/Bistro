package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	anthropicoption "github.com/anthropics/anthropic-sdk-go/option"
	openai "github.com/openai/openai-go"
	openaioption "github.com/openai/openai-go/option"
	"github.com/openai/openai-go/shared"
)

const (
	maxCommentBodyChars      = 4000
	clientTimeout            = 30 * time.Second
	openAIDefaultBaseURL     = "https://api.openai.com/v1"
	openRouterDefaultBaseURL = "https://openrouter.ai/api/v1"
	ollamaDefaultBaseURL     = "http://localhost:11434/v1"
	anthropicDefaultBaseURL  = "https://api.anthropic.com/v1"
	anthropicMaxTokens       = 64
)

var (
	ErrAuth        = errors.New("llm authentication failed")
	ErrRateLimited = errors.New("llm rate limited")
)

type ClassifierClient interface {
	Classify(ctx context.Context, body string) (severity string, category string, usageTokens int, err error)
}

type OpenAICompatibleClient struct {
	client openai.Client
	model  string
}

type AnthropicClient struct {
	client anthropic.Client
	model  string
}

type classificationReply struct {
	Severity string `json:"severity"`
	Category string `json:"category"`
}

func NewOpenAICompatibleClient(provider string, model string, apiKey string, endpoint string) *OpenAICompatibleClient {
	client := openai.NewClient(
		openaioption.WithAPIKey(apiKey),
		openaioption.WithBaseURL(resolveOpenAICompatibleBaseURL(provider, endpoint)),
	)

	return &OpenAICompatibleClient{
		client: client,
		model:  model,
	}
}

func NewAnthropicClient(model string, apiKey string, endpoint string) *AnthropicClient {
	client := anthropic.NewClient(
		anthropicoption.WithAPIKey(apiKey),
		anthropicoption.WithBaseURL(resolveAnthropicBaseURL(endpoint)),
	)

	return &AnthropicClient{
		client: client,
		model:  model,
	}
}

func (c *OpenAICompatibleClient) Classify(ctx context.Context, body string) (string, string, int, error) {
	requestCtx, cancel := context.WithTimeout(ctx, clientTimeout)
	defer cancel()

	response, err := c.client.Chat.Completions.New(requestCtx, openai.ChatCompletionNewParams{
		Model: shared.ChatModel(c.model),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(systemPrompt()),
			openai.UserMessage(truncateCommentBody(body)),
		},
	})
	if err != nil {
		return "", "", 0, mapOpenAIError(err)
	}
	if len(response.Choices) == 0 {
		return "", "", 0, errors.New("OpenAI-compatible classification response had no choices")
	}

	severity, category, err := parseClassificationReply(response.Choices[0].Message.Content)
	if err != nil {
		return "", "", 0, err
	}

	return severity, category, int(response.Usage.TotalTokens), nil
}

func (c *AnthropicClient) Classify(ctx context.Context, body string) (string, string, int, error) {
	requestCtx, cancel := context.WithTimeout(ctx, clientTimeout)
	defer cancel()

	response, err := c.client.Messages.New(requestCtx, anthropic.MessageNewParams{
		Model:     anthropic.Model(c.model),
		MaxTokens: anthropicMaxTokens,
		System: []anthropic.TextBlockParam{
			{Text: systemPrompt()},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(truncateCommentBody(body))),
		},
	})
	if err != nil {
		return "", "", 0, mapAnthropicError(err)
	}

	content, err := firstAnthropicText(response.Content)
	if err != nil {
		return "", "", 0, err
	}

	severity, category, err := parseClassificationReply(content)
	if err != nil {
		return "", "", 0, err
	}

	return severity, category, int(response.Usage.InputTokens + response.Usage.OutputTokens), nil
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

func resolveAnthropicBaseURL(endpoint string) string {
	baseURL := strings.TrimRight(endpoint, "/")
	if baseURL == "" {
		return anthropicDefaultBaseURL
	}
	return baseURL
}

func mapOpenAIError(err error) error {
	var apiErr *openai.Error
	if errors.As(err, &apiErr) {
		return mapSDKStatusError("OpenAI-compatible", apiErr.StatusCode, err)
	}
	return fmt.Errorf("send OpenAI-compatible classification request: %w", err)
}

func mapAnthropicError(err error) error {
	var apiErr *anthropic.Error
	if errors.As(err, &apiErr) {
		return mapSDKStatusError("Anthropic", apiErr.StatusCode, err)
	}
	return fmt.Errorf("send Anthropic classification request: %w", err)
}

func mapSDKStatusError(provider string, statusCode int, err error) error {
	switch statusCode {
	case http.StatusUnauthorized:
		return fmt.Errorf("%w: %s classification request returned status %d", ErrAuth, provider, statusCode)
	case http.StatusTooManyRequests:
		return fmt.Errorf("%w: %s classification request returned status %d", ErrRateLimited, provider, statusCode)
	default:
		return fmt.Errorf("send %s classification request: %w", provider, err)
	}
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

func firstAnthropicText(content []anthropic.ContentBlockUnion) (string, error) {
	for _, item := range content {
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
