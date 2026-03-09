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

const geminiAPI = "https://generativelanguage.googleapis.com/v1beta/models"

// GeminiClient calls the Google Gemini REST API directly via HTTP.
type GeminiClient struct {
	apiKey     string
	model      string
	maxTokens  int
	httpClient *http.Client
}

// NewGeminiClient creates a new Gemini API client.
func NewGeminiClient(apiKey, model string, maxTokens int) *GeminiClient {
	return &GeminiClient{
		apiKey:    apiKey,
		model:     model,
		maxTokens: maxTokens,
		httpClient: &http.Client{
			Timeout: 3 * time.Minute, // generous timeout for large code reviews
		},
	}
}

// ProviderName returns "Gemini".
func (c *GeminiClient) ProviderName() string { return "Gemini" }

// ─── Request / Response types ────────────────────────────────────────────────

type geminiRequest struct {
	SystemInstruction *geminiContent    `json:"system_instruction,omitempty"`
	Contents          []geminiContent   `json:"contents"`
	GenerationConfig  geminiGenConfig   `json:"generationConfig"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiGenConfig struct {
	MaxOutputTokens int     `json:"maxOutputTokens"`
	Temperature     float64 `json:"temperature,omitempty"`
}

type geminiResponse struct {
	Candidates []geminiCandidate `json:"candidates"`
	Error      *geminiError      `json:"error,omitempty"`
}

type geminiCandidate struct {
	Content      geminiContent `json:"content"`
	FinishReason string        `json:"finishReason"`
}

type geminiError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Status  string `json:"status"`
}

// ─── Public API ──────────────────────────────────────────────────────────────

// Chat sends a message with an optional system prompt and returns the text response.
func (c *GeminiClient) Chat(ctx context.Context, systemPrompt, userMessage string) (string, error) {
	reqBody := geminiRequest{
		Contents: []geminiContent{
			{
				Role:  "user",
				Parts: []geminiPart{{Text: userMessage}},
			},
		},
		GenerationConfig: geminiGenConfig{
			MaxOutputTokens: c.maxTokens,
			Temperature:     0.7,
		},
	}

	if systemPrompt != "" {
		reqBody.SystemInstruction = &geminiContent{
			Parts: []geminiPart{{Text: systemPrompt}},
		}
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/%s:generateContent?key=%s", geminiAPI, c.model, c.apiKey)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

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
		var errResp geminiResponse
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error != nil {
			return "", fmt.Errorf("Gemini API error (%d): %s", errResp.Error.Code, errResp.Error.Message)
		}
		return "", fmt.Errorf("Gemini API error (%d): %s", resp.StatusCode, string(respBody))
	}

	var gemResp geminiResponse
	if err := json.Unmarshal(respBody, &gemResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if gemResp.Error != nil {
		return "", fmt.Errorf("Gemini API error: %s", gemResp.Error.Message)
	}

	// Extract text from candidates
	var result strings.Builder
	for _, candidate := range gemResp.Candidates {
		for _, part := range candidate.Content.Parts {
			result.WriteString(part.Text)
		}
	}

	text := result.String()
	if text == "" {
		return "", fmt.Errorf("empty response from Gemini API")
	}

	return text, nil
}
