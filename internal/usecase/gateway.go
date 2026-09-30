package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jsjg1/resilient-universal-ai-gateway/internal/domain"
	"github.com/jsjg1/resilient-universal-ai-gateway/internal/infrastructure/ai"
	"github.com/jsjg1/resilient-universal-ai-gateway/internal/infrastructure/circuitbreaker"
	"github.com/jsjg1/resilient-universal-ai-gateway/internal/infrastructure/telemetry"
)

// GatewayService coordinates AI provider dispatch, circuit breaking, retries, and fallback.
type GatewayService struct {
	factory         *ai.ProviderFactory
	breakers        map[domain.ProviderType]*circuitbreaker.ProviderBreaker
	mu              sync.RWMutex
	defaultProvider domain.ProviderType
	fallbackOrder   []domain.ProviderType
	timeout         time.Duration
	maxRetries      int
	retryBackoff    time.Duration
	cbConfig        circuitbreaker.Config
	logger          *slog.Logger
	metrics         *telemetry.Metrics
}

// NewGatewayService initializes the service using Factory and Functional Options.
func NewGatewayService(factory *ai.ProviderFactory, opts ...Option) *GatewayService {
	s := &GatewayService{
		factory:         factory,
		breakers:        make(map[domain.ProviderType]*circuitbreaker.ProviderBreaker),
		defaultProvider: domain.ProviderOpenAI,
		fallbackOrder:   []domain.ProviderType{domain.ProviderOpenAI, domain.ProviderGemini},
		timeout:         30 * time.Second,
		maxRetries:      1,
		retryBackoff:    200 * time.Millisecond,
		cbConfig:        circuitbreaker.DefaultConfig(),
		logger:          slog.Default(),
	}

	for _, opt := range opts {
		opt(s)
	}

	// Initialize circuit breakers for all registered providers
	if factory != nil {
		for _, pType := range factory.ListProviders() {
			s.breakers[pType] = circuitbreaker.NewProviderBreaker(pType, s.cbConfig, s.metrics, s.logger)
		}
	}

	return s
}

// RegisterProviderBreaker adds or updates a circuit breaker for a provider.
func (s *GatewayService) RegisterProviderBreaker(pType domain.ProviderType) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.breakers[pType] = circuitbreaker.NewProviderBreaker(pType, s.cbConfig, s.metrics, s.logger)
}

func (s *GatewayService) getBreaker(pType domain.ProviderType) *circuitbreaker.ProviderBreaker {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.breakers[pType]
}

// ExecuteCompletion executes the chat completion with retry, circuit breaker, and automatic fallback.
func (s *GatewayService) ExecuteCompletion(ctx context.Context, req domain.CompletionRequest) (*domain.CompletionResponse, error) {
	if len(req.Messages) == 0 {
		return nil, fmt.Errorf("%w: messages cannot be empty", domain.ErrInvalidRequest)
	}

	// Determine candidate providers sequence
	candidateProviders := s.resolveProviderOrder(req.ProviderOverride)

	var lastErr error
	for _, providerType := range candidateProviders {
		provider, err := s.factory.Get(providerType)
		if err != nil {
			s.logger.WarnContext(ctx, "provider not available in factory",
				slog.String("provider", string(providerType)),
				slog.String("error", err.Error()),
			)
			lastErr = err
			continue
		}

		breaker := s.getBreaker(providerType)
		if breaker == nil {
			s.RegisterProviderBreaker(providerType)
			breaker = s.getBreaker(providerType)
		}

		// Attempt execution with retries through circuit breaker
		resp, attemptErr := s.executeWithBreakerAndRetries(ctx, provider, breaker, req)
		if attemptErr == nil && resp != nil {
			return resp, nil
		}

		s.logger.WarnContext(ctx, "ai provider execution failed, attempting next fallback",
			slog.String("failed_provider", string(providerType)),
			slog.String("error", attemptErr.Error()),
		)
		lastErr = attemptErr
	}

	s.logger.ErrorContext(ctx, "all ai providers failed", slog.String("last_error", fmt.Sprintf("%v", lastErr)))
	return nil, fmt.Errorf("%w: %v", domain.ErrAllProvidersFailed, lastErr)
}

func (s *GatewayService) executeWithBreakerAndRetries(
	ctx context.Context,
	provider domain.AIProvider,
	breaker *circuitbreaker.ProviderBreaker,
	req domain.CompletionRequest,
) (*domain.CompletionResponse, error) {
	var lastErr error

	for attempt := 0; attempt <= s.maxRetries; attempt++ {
		if attempt > 0 {
			backoff := s.retryBackoff * time.Duration(1<<uint(attempt-1))
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		start := time.Now()
		rawResult, err := breaker.Execute(func() (any, error) {
			callCtx, cancel := context.WithTimeout(ctx, s.timeout)
			defer cancel()
			return provider.GenerateCompletion(callCtx, req)
		})

		duration := time.Since(start).Seconds()

		if err != nil {
			lastErr = err
			s.recordMetrics(string(provider.Type()), req.Model, "error", duration, nil)
			s.logger.WarnContext(ctx, "provider attempt failed",
				slog.String("provider", string(provider.Type())),
				slog.Int("attempt", attempt+1),
				slog.String("error", err.Error()),
			)
			continue
		}

		resp, ok := rawResult.(*domain.CompletionResponse)
		if !ok || resp == nil {
			lastErr = fmt.Errorf("provider returned nil or invalid response format")
			s.recordMetrics(string(provider.Type()), req.Model, "error", duration, nil)
			continue
		}

		s.recordMetrics(string(provider.Type()), resp.Model, "success", duration, &resp.Usage)
		return resp, nil
	}

	return nil, lastErr
}

func (s *GatewayService) resolveProviderOrder(override *domain.ProviderType) []domain.ProviderType {
	var order []domain.ProviderType
	visited := make(map[domain.ProviderType]bool)

	if override != nil && *override != "" {
		order = append(order, *override)
		visited[*override] = true
	} else if s.defaultProvider != "" {
		order = append(order, s.defaultProvider)
		visited[s.defaultProvider] = true
	}

	for _, p := range s.fallbackOrder {
		if !visited[p] {
			order = append(order, p)
			visited[p] = true
		}
	}

	return order
}

func (s *GatewayService) recordMetrics(provider, model, status string, duration float64, usage *domain.TokenUsage) {
	if s.metrics == nil {
		return
	}

	s.metrics.ProviderRequestsTotal.WithLabelValues(provider, model, status).Inc()
	s.metrics.ProviderRequestDuration.WithLabelValues(provider, model, status).Observe(duration)

	if usage != nil {
		if usage.PromptTokens > 0 {
			s.metrics.ProviderTokensTotal.WithLabelValues(provider, model, "prompt").Add(float64(usage.PromptTokens))
		}
		if usage.CompletionTokens > 0 {
			s.metrics.ProviderTokensTotal.WithLabelValues(provider, model, "completion").Add(float64(usage.CompletionTokens))
		}
	}
}
