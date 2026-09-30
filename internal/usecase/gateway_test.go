package usecase_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jsjg1/resilient-universal-ai-gateway/internal/domain"
	"github.com/jsjg1/resilient-universal-ai-gateway/internal/infrastructure/ai"
	"github.com/jsjg1/resilient-universal-ai-gateway/internal/infrastructure/circuitbreaker"
	"github.com/jsjg1/resilient-universal-ai-gateway/internal/infrastructure/telemetry"
	"github.com/jsjg1/resilient-universal-ai-gateway/internal/usecase"
	"github.com/prometheus/client_golang/prometheus"
)

// MockProvider is a test double implementing domain.AIProvider.
type MockProvider struct {
	providerType domain.ProviderType
	callCount    atomic.Int64
	fail         bool
	failErr      error
	response     *domain.CompletionResponse
}

func NewMockProvider(pType domain.ProviderType, response *domain.CompletionResponse) *MockProvider {
	return &MockProvider{
		providerType: pType,
		response:     response,
	}
}

func (m *MockProvider) Type() domain.ProviderType {
	return m.providerType
}

func (m *MockProvider) GenerateCompletion(ctx context.Context, req domain.CompletionRequest) (*domain.CompletionResponse, error) {
	m.callCount.Add(1)
	if m.fail {
		if m.failErr != nil {
			return nil, m.failErr
		}
		return nil, errors.New("upstream provider error: 500 internal server error")
	}
	return m.response, nil
}

func (m *MockProvider) Calls() int64 {
	return m.callCount.Load()
}

func TestGateway_PrimarySuccess(t *testing.T) {
	factory := ai.NewProviderFactory()
	openaiMock := NewMockProvider(domain.ProviderOpenAI, &domain.CompletionResponse{
		ID:       "test-openai-1",
		Provider: domain.ProviderOpenAI,
		Model:    "gpt-4o-mini",
		Content:  "Hello from OpenAI",
	})
	geminiMock := NewMockProvider(domain.ProviderGemini, &domain.CompletionResponse{
		ID:       "test-gemini-1",
		Provider: domain.ProviderGemini,
		Model:    "gemini-1.5-flash",
		Content:  "Hello from Gemini",
	})

	factory.Register(openaiMock)
	factory.Register(geminiMock)

	gw := usecase.NewGatewayService(
		factory,
		usecase.WithDefaultProvider(domain.ProviderOpenAI),
		usecase.WithFallbackOrder(domain.ProviderOpenAI, domain.ProviderGemini),
		usecase.WithMaxRetries(0),
	)

	req := domain.CompletionRequest{
		Messages: []domain.ChatMessage{
			{Role: domain.RoleUser, Content: "Hello AI"},
		},
	}

	resp, err := gw.ExecuteCompletion(context.Background(), req)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}

	if resp.Content != "Hello from OpenAI" {
		t.Errorf("expected content 'Hello from OpenAI', got %q", resp.Content)
	}
	if resp.Provider != domain.ProviderOpenAI {
		t.Errorf("expected provider 'openai', got %q", resp.Provider)
	}
	if openaiMock.Calls() != 1 {
		t.Errorf("expected openai to be called 1 time, got %d", openaiMock.Calls())
	}
	if geminiMock.Calls() != 0 {
		t.Errorf("expected gemini to not be called, got %d", geminiMock.Calls())
	}
}

func TestGateway_FallbackOnPrimaryFailure(t *testing.T) {
	factory := ai.NewProviderFactory()
	openaiMock := NewMockProvider(domain.ProviderOpenAI, nil)
	openaiMock.fail = true // Force OpenAI failure

	geminiMock := NewMockProvider(domain.ProviderGemini, &domain.CompletionResponse{
		ID:       "test-gemini-fallback",
		Provider: domain.ProviderGemini,
		Model:    "gemini-1.5-flash",
		Content:  "Fallback response from Gemini",
	})

	factory.Register(openaiMock)
	factory.Register(geminiMock)

	gw := usecase.NewGatewayService(
		factory,
		usecase.WithDefaultProvider(domain.ProviderOpenAI),
		usecase.WithFallbackOrder(domain.ProviderOpenAI, domain.ProviderGemini),
		usecase.WithMaxRetries(0),
	)

	req := domain.CompletionRequest{
		Messages: []domain.ChatMessage{
			{Role: domain.RoleUser, Content: "Fallback test"},
		},
	}

	resp, err := gw.ExecuteCompletion(context.Background(), req)
	if err != nil {
		t.Fatalf("expected fallback success, got error: %v", err)
	}

	if resp.Provider != domain.ProviderGemini {
		t.Errorf("expected provider to be 'gemini', got %q", resp.Provider)
	}
	if resp.Content != "Fallback response from Gemini" {
		t.Errorf("expected content 'Fallback response from Gemini', got %q", resp.Content)
	}
	if openaiMock.Calls() != 1 {
		t.Errorf("expected openai to be attempted 1 time, got %d", openaiMock.Calls())
	}
	if geminiMock.Calls() != 1 {
		t.Errorf("expected gemini fallback to be called 1 time, got %d", geminiMock.Calls())
	}
}

func TestGateway_AllProvidersFail(t *testing.T) {
	factory := ai.NewProviderFactory()
	openaiMock := NewMockProvider(domain.ProviderOpenAI, nil)
	openaiMock.fail = true

	geminiMock := NewMockProvider(domain.ProviderGemini, nil)
	geminiMock.fail = true

	factory.Register(openaiMock)
	factory.Register(geminiMock)

	gw := usecase.NewGatewayService(
		factory,
		usecase.WithDefaultProvider(domain.ProviderOpenAI),
		usecase.WithFallbackOrder(domain.ProviderOpenAI, domain.ProviderGemini),
		usecase.WithMaxRetries(0),
	)

	req := domain.CompletionRequest{
		Messages: []domain.ChatMessage{
			{Role: domain.RoleUser, Content: "Will fail"},
		},
	}

	resp, err := gw.ExecuteCompletion(context.Background(), req)
	if err == nil {
		t.Fatalf("expected error, got response: %+v", resp)
	}

	if !errors.Is(err, domain.ErrAllProvidersFailed) {
		t.Errorf("expected ErrAllProvidersFailed, got: %v", err)
	}
}

func TestGateway_ProviderOverride(t *testing.T) {
	factory := ai.NewProviderFactory()
	openaiMock := NewMockProvider(domain.ProviderOpenAI, &domain.CompletionResponse{
		ID:       "openai-resp",
		Provider: domain.ProviderOpenAI,
		Content:  "OpenAI content",
	})
	geminiMock := NewMockProvider(domain.ProviderGemini, &domain.CompletionResponse{
		ID:       "gemini-resp",
		Provider: domain.ProviderGemini,
		Content:  "Gemini direct override",
	})

	factory.Register(openaiMock)
	factory.Register(geminiMock)

	gw := usecase.NewGatewayService(
		factory,
		usecase.WithDefaultProvider(domain.ProviderOpenAI),
	)

	geminiOverride := domain.ProviderGemini
	req := domain.CompletionRequest{
		ProviderOverride: &geminiOverride,
		Messages: []domain.ChatMessage{
			{Role: domain.RoleUser, Content: "Hello override"},
		},
	}

	resp, err := gw.ExecuteCompletion(context.Background(), req)
	if err != nil {
		t.Fatalf("expected success, got err: %v", err)
	}

	if resp.Provider != domain.ProviderGemini {
		t.Errorf("expected provider gemini, got %s", resp.Provider)
	}
	if resp.Content != "Gemini direct override" {
		t.Errorf("expected Gemini direct override, got %s", resp.Content)
	}
	if openaiMock.Calls() != 0 {
		t.Errorf("expected openai calls to be 0, got %d", openaiMock.Calls())
	}
}

func TestGateway_CircuitBreakerTrips(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics := telemetry.NewMetrics(reg)

	factory := ai.NewProviderFactory()
	openaiMock := NewMockProvider(domain.ProviderOpenAI, nil)
	openaiMock.fail = true // Always fail to trip breaker

	geminiMock := NewMockProvider(domain.ProviderGemini, &domain.CompletionResponse{
		ID:       "gemini-healthy",
		Provider: domain.ProviderGemini,
		Content:  "Gemini healthy",
	})

	factory.Register(openaiMock)
	factory.Register(geminiMock)

	cbCfg := circuitbreaker.Config{
		MaxRequests:  1,
		Interval:     10 * time.Second,
		Timeout:      10 * time.Second,
		MinRequests:  3,
		FailureRatio: 0.50, // 50% threshold
	}

	gw := usecase.NewGatewayService(
		factory,
		usecase.WithDefaultProvider(domain.ProviderOpenAI),
		usecase.WithFallbackOrder(domain.ProviderOpenAI, domain.ProviderGemini),
		usecase.WithCircuitBreakerConfig(cbCfg),
		usecase.WithMaxRetries(0),
		usecase.WithMetrics(metrics),
	)

	req := domain.CompletionRequest{
		Messages: []domain.ChatMessage{{Role: domain.RoleUser, Content: "trip test"}},
	}

	// Make 3 requests so OpenAI fails 3/3 times (100% > 50%), tripping the breaker
	for i := 0; i < 3; i++ {
		resp, err := gw.ExecuteCompletion(context.Background(), req)
		if err != nil {
			t.Fatalf("request %d failed completely: %v", i, err)
		}
		if resp.Provider != domain.ProviderGemini {
			t.Errorf("request %d expected gemini fallback, got %s", i, resp.Provider)
		}
	}

	openaiCallsBefore := openaiMock.Calls()
	if openaiCallsBefore != 3 {
		t.Fatalf("expected 3 calls to openai, got %d", openaiCallsBefore)
	}

	// Fourth request: Circuit breaker for OpenAI is OPEN.
	// The call to OpenAI should immediately be skipped/fail-fast by gobreaker without calling OpenAI adapter!
	resp, err := gw.ExecuteCompletion(context.Background(), req)
	if err != nil {
		t.Fatalf("expected fallback success when breaker open: %v", err)
	}
	if resp.Provider != domain.ProviderGemini {
		t.Errorf("expected gemini fallback, got %s", resp.Provider)
	}

	// Verify OpenAI was not called because circuit breaker blocked it
	if openaiMock.Calls() != openaiCallsBefore {
		t.Errorf("expected circuit breaker to block request to openai, but calls increased from %d to %d",
			openaiCallsBefore, openaiMock.Calls())
	}
}
