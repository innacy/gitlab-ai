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

const nvidiaAPI = "https://integrate.api.nvidia.com/v1/chat/completions"

// NvidiaClient calls the NVIDIA NIM API (OpenAI-compatible endpoint).
type NvidiaClient struct {
	apiKey     string
	model      string
	maxTokens  int
	httpClient *http.Client
}

// NewNvidiaClient creates a new NVIDIA NIM API client.
func NewNvidiaClient(apiKey, model string, maxTokens int, timeout time.Duration) *NvidiaClient {
	if timeout <= 0 {
		timeout = 3 * time.Minute
	}
	return &NvidiaClient{
		apiKey:    apiKey,
		model:     model,
		maxTokens: maxTokens,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// ProviderName returns "NVIDIA".
func (c *NvidiaClient) ProviderName() string { return "NVIDIA" }

// ─── Request / Response types (OpenAI-compatible) ────────────────────────────

type openAIChatRequest struct {
	Model     string          `json:"model"`
	Messages  []openAIMessage `json:"messages"`
	MaxTokens int             `json:"max_tokens"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIChatResponse struct {
	Choices []openAIChoice  `json:"choices"`
	Error   *openAIAPIError `json:"error,omitempty"`
}

type openAIChoice struct {
	Message openAIMessage `json:"message"`
}

type openAIAPIError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

// ─── Public API ──────────────────────────────────────────────────────────────

// Chat sends a message with an optional system prompt and returns the text response.
func (c *NvidiaClient) Chat(ctx context.Context, systemPrompt, userMessage string) (string, error) {
	messages := make([]openAIMessage, 0, 2)
	if systemPrompt != "" {
		messages = append(messages, openAIMessage{Role: "system", Content: systemPrompt})
	}
	messages = append(messages, openAIMessage{Role: "user", Content: userMessage})

	reqBody := openAIChatRequest{
		Model:     c.model,
		Messages:  messages,
		MaxTokens: c.maxTokens,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", nvidiaAPI, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

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
		var errResp openAIChatResponse
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error != nil {
			return "", fmt.Errorf("NVIDIA API error (%d): %s", resp.StatusCode, errResp.Error.Message)
		}
		return "", fmt.Errorf("NVIDIA API error (%d): %s", resp.StatusCode, string(respBody))
	}

	var chatResp openAIChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if chatResp.Error != nil {
		return "", fmt.Errorf("NVIDIA API error: %s", chatResp.Error.Message)
	}

	var result strings.Builder
	for _, choice := range chatResp.Choices {
		result.WriteString(choice.Message.Content)
	}

	text := result.String()
	if text == "" {
		return "", fmt.Errorf("empty response from NVIDIA API")
	}

	return text, nil
}
