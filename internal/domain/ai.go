package domain

import "context"

// ProviderType represents the type identifier for an AI provider.
type ProviderType string

const (
	ProviderOpenAI ProviderType = "openai"
	ProviderGemini ProviderType = "gemini"
)

// Role represents the author of a message in a conversation.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// ChatMessage represents a single message in a chat completion prompt.
type ChatMessage struct {
	Role    Role   `json:"role"`
	Content string `json:"content"`
}

// CompletionRequest represents the canonical unified request schema across all AI providers.
type CompletionRequest struct {
	Model            string            `json:"model,omitempty"`
	Messages         []ChatMessage     `json:"messages"`
	Temperature      *float64          `json:"temperature,omitempty"`
	MaxTokens        *int              `json:"max_tokens,omitempty"`
	ProviderOverride *ProviderType     `json:"provider_override,omitempty"`
	Metadata         map[string]string `json:"metadata,omitempty"`
}

// TokenUsage holds token consumption statistics for an inference request.
type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// CompletionResponse represents the canonical unified response returned by the gateway.
type CompletionResponse struct {
	ID           string       `json:"id"`
	Provider     ProviderType `json:"provider"`
	Model        string       `json:"model"`
	Content      string       `json:"content"`
	FinishReason string       `json:"finish_reason,omitempty"`
	Usage        TokenUsage   `json:"usage"`
	LatencyMs    int64        `json:"latency_ms"`
}

// AIProvider defines the Strategy contract that all external AI model adapters must implement.
type AIProvider interface {
	// Type returns the provider identifier.
	Type() ProviderType

	// GenerateCompletion sends the request to the upstream AI provider and returns a normalized response.
	GenerateCompletion(ctx context.Context, req CompletionRequest) (*CompletionResponse, error)
}
