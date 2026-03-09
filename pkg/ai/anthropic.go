package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	anthropicAPI     = "https://api.anthropic.com/v1/messages"
	anthropicVersion = "2023-06-01"
)

// AnthropicClient calls the Anthropic Messages API directly via HTTP.
type AnthropicClient struct {
	apiKey     string
	model      string
	maxTokens  int
	httpClient *http.Client
}

// NewAnthropicClient creates a new Anthropic API client.
func NewAnthropicClient(apiKey, model string, maxTokens int) *AnthropicClient {
	return &AnthropicClient{
		apiKey:    apiKey,
		model:     model,
		maxTokens: maxTokens,
		httpClient: &http.Client{
			Timeout: 3 * time.Minute, // generous timeout for large code reviews
		},
	}
}

// ProviderName returns "Claude".
func (c *AnthropicClient) ProviderName() string { return "Claude" }

// ─── Request / Response types ────────────────────────────────────────────────

type messagesRequest struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens"`
	System    string    `json:"system,omitempty"`
	Messages  []message `json:"messages"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type messagesResponse struct {
	ID      string         `json:"id"`
	Type    string         `json:"type"`
	Role    string         `json:"role"`
	Content []contentBlock `json:"content"`
	Model   string         `json:"model"`
	Usage   usageInfo      `json:"usage"`
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type usageInfo struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type apiErrorResponse struct {
	Type  string `json:"type"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// ─── Public API ──────────────────────────────────────────────────────────────

// Chat sends a message with an optional system prompt and returns the text response.
func (c *AnthropicClient) Chat(ctx context.Context, systemPrompt, userMessage string) (string, error) {
	reqBody := messagesRequest{
		Model:     c.model,
		MaxTokens: c.maxTokens,
		System:    systemPrompt,
		Messages: []message{
			{Role: "user", Content: userMessage},
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", anthropicAPI, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", anthropicVersion)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp apiErrorResponse
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error != nil {
			return "", fmt.Errorf("Anthropic API error (%d): %s", resp.StatusCode, errResp.Error.Message)
		}
		return "", fmt.Errorf("Anthropic API error (%d): %s", resp.StatusCode, string(respBody))
	}

	var msgResp messagesResponse
	if err := json.Unmarshal(respBody, &msgResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	// Extract text from content blocks
	var result strings.Builder
	for _, block := range msgResp.Content {
		if block.Type == "text" {
			result.WriteString(block.Text)
		}
	}

	text := result.String()
	if text == "" {
		return "", fmt.Errorf("empty response from Anthropic API")
	}

	return text, nil
}
