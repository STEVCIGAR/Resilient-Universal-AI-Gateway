package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jsjg1/resilient-universal-ai-gateway/internal/domain"
)

// OpenAIAdapter implements domain.AIProvider for OpenAI API.
type OpenAIAdapter struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

// NewOpenAIAdapter constructs a new OpenAI adapter.
func NewOpenAIAdapter(apiKey string, baseURL string, httpClient *http.Client) *OpenAIAdapter {
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &OpenAIAdapter{
		apiKey:     apiKey,
		baseURL:    baseURL,
		httpClient: httpClient,
	}
}

func (a *OpenAIAdapter) Type() domain.ProviderType {
	return domain.ProviderOpenAI
}

type openAIChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIChatRequest struct {
	Model       string              `json:"model"`
	Messages    []openAIChatMessage `json:"messages"`
	Temperature *float64            `json:"temperature,omitempty"`
	MaxTokens   *int                `json:"max_tokens,omitempty"`
}

type openAIChoice struct {
	Index        int               `json:"index"`
	Message      openAIChatMessage `json:"message"`
	FinishReason string            `json:"finish_reason"`
}

type openAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type openAIChatResponse struct {
	ID      string         `json:"id"`
	Model   string         `json:"model"`
	Choices []openAIChoice `json:"choices"`
	Usage   openAIUsage    `json:"usage"`
	Error   *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    any    `json:"code"`
	} `json:"error,omitempty"`
}

func (a *OpenAIAdapter) GenerateCompletion(ctx context.Context, req domain.CompletionRequest) (*domain.CompletionResponse, error) {
	start := time.Now()
	model := req.Model
	if model == "" {
		model = "gpt-4o-mini"
	}

	messages := make([]openAIChatMessage, len(req.Messages))
	for i, m := range req.Messages {
		messages[i] = openAIChatMessage{
			Role:    string(m.Role),
			Content: m.Content,
		}
	}

	bodyPayload := openAIChatRequest{
		Model:       model,
		Messages:    messages,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
	}

	payloadBytes, err := json.Marshal(bodyPayload)
	if err != nil {
		return nil, fmt.Errorf("openai: failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/v1/chat/completions", a.baseURL)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("openai: failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if a.apiKey != "" {
		httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", a.apiKey))
	}

	resp, err := a.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai: request error: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("openai: error reading response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("openai api error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var openAIResp openAIChatResponse
	if err := json.Unmarshal(bodyBytes, &openAIResp); err != nil {
		return nil, fmt.Errorf("openai: failed to unmarshal response: %w", err)
	}

	if openAIResp.Error != nil {
		return nil, fmt.Errorf("openai error: %s", openAIResp.Error.Message)
	}

	var content string
	var finishReason string
	if len(openAIResp.Choices) > 0 {
		content = openAIResp.Choices[0].Message.Content
		finishReason = openAIResp.Choices[0].FinishReason
	}

	respID := openAIResp.ID
	if respID == "" {
		respID = fmt.Sprintf("resp_%s", uuid.New().String())
	}

	return &domain.CompletionResponse{
		ID:           respID,
		Provider:     domain.ProviderOpenAI,
		Model:        openAIResp.Model,
		Content:      content,
		FinishReason: finishReason,
		Usage: domain.TokenUsage{
			PromptTokens:     openAIResp.Usage.PromptTokens,
			CompletionTokens: openAIResp.Usage.CompletionTokens,
			TotalTokens:      openAIResp.Usage.TotalTokens,
		},
		LatencyMs: time.Since(start).Milliseconds(),
	}, nil
}
