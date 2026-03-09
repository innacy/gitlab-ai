package ai

import "context"

// ChatClient is the common interface for all AI providers.
type ChatClient interface {
	// Chat sends a user message with an optional system prompt and returns the text response.
	Chat(ctx context.Context, systemPrompt, userMessage string) (string, error)

	// ProviderName returns a human-readable provider name (e.g. "Claude", "Gemini").
	ProviderName() string
}
