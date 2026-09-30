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

// GeminiAdapter implements domain.AIProvider for Google Gemini API.
type GeminiAdapter struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

// NewGeminiAdapter constructs a new Gemini adapter.
func NewGeminiAdapter(apiKey string, baseURL string, httpClient *http.Client) *GeminiAdapter {
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com"
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &GeminiAdapter{
		apiKey:     apiKey,
		baseURL:    baseURL,
		httpClient: httpClient,
	}
}

func (a *GeminiAdapter) Type() domain.ProviderType {
	return domain.ProviderGemini
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiSystemInstruction struct {
	Parts []geminiPart `json:"parts"`
}

type geminiGenerationConfig struct {
	Temperature     *float64 `json:"temperature,omitempty"`
	MaxOutputTokens *int     `json:"maxOutputTokens,omitempty"`
}

type geminiGenerateContentRequest struct {
	Contents          []geminiContent          `json:"contents"`
	SystemInstruction *geminiSystemInstruction `json:"system_instruction,omitempty"`
	GenerationConfig  *geminiGenerationConfig  `json:"generationConfig,omitempty"`
}

type geminiCandidate struct {
	Content struct {
		Parts []geminiPart `json:"parts"`
		Role  string       `json:"role"`
	} `json:"content"`
	FinishReason string `json:"finishReason"`
}

type geminiUsageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

type geminiGenerateContentResponse struct {
	Candidates    []geminiCandidate    `json:"candidates"`
	UsageMetadata geminiUsageMetadata  `json:"usageMetadata"`
	Error         *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error,omitempty"`
}

func (a *GeminiAdapter) GenerateCompletion(ctx context.Context, req domain.CompletionRequest) (*domain.CompletionResponse, error) {
	start := time.Now()
	model := req.Model
	if model == "" {
		model = "gemini-1.5-flash"
	}

	var contents []geminiContent
	var systemParts []geminiPart

	for _, msg := range req.Messages {
		if msg.Role == domain.RoleSystem {
			systemParts = append(systemParts, geminiPart{Text: msg.Content})
		} else {
			role := "user"
			if msg.Role == domain.RoleAssistant {
				role = "model"
			}
			contents = append(contents, geminiContent{
				Role:  role,
				Parts: []geminiPart{{Text: msg.Content}},
			})
		}
	}

	payload := geminiGenerateContentRequest{
		Contents: contents,
	}

	if len(systemParts) > 0 {
		payload.SystemInstruction = &geminiSystemInstruction{Parts: systemParts}
	}

	if req.Temperature != nil || req.MaxTokens != nil {
		payload.GenerationConfig = &geminiGenerationConfig{
			Temperature:     req.Temperature,
			MaxOutputTokens: req.MaxTokens,
		}
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("gemini: failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/v1beta/models/%s:generateContent", a.baseURL, model)
	if a.apiKey != "" {
		url = fmt.Sprintf("%s?key=%s", url, a.apiKey)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("gemini: failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := a.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("gemini: request error: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("gemini: error reading response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("gemini api error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var geminiResp geminiGenerateContentResponse
	if err := json.Unmarshal(bodyBytes, &geminiResp); err != nil {
		return nil, fmt.Errorf("gemini: failed to unmarshal response: %w", err)
	}

	if geminiResp.Error != nil {
		return nil, fmt.Errorf("gemini error: %s", geminiResp.Error.Message)
	}

	var content string
	var finishReason string
	if len(geminiResp.Candidates) > 0 {
		cand := geminiResp.Candidates[0]
		finishReason = cand.FinishReason
		if len(cand.Content.Parts) > 0 {
			content = cand.Content.Parts[0].Text
		}
	}

	return &domain.CompletionResponse{
		ID:           fmt.Sprintf("gemini_%s", uuid.New().String()),
		Provider:     domain.ProviderGemini,
		Model:        model,
		Content:      content,
		FinishReason: finishReason,
		Usage: domain.TokenUsage{
			PromptTokens:     geminiResp.UsageMetadata.PromptTokenCount,
			CompletionTokens: geminiResp.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      geminiResp.UsageMetadata.TotalTokenCount,
		},
		LatencyMs: time.Since(start).Milliseconds(),
	}, nil
}
