package usecase

import (
	"log/slog"
	"time"

	"github.com/jsjg1/resilient-universal-ai-gateway/internal/domain"
	"github.com/jsjg1/resilient-universal-ai-gateway/internal/infrastructure/circuitbreaker"
	"github.com/jsjg1/resilient-universal-ai-gateway/internal/infrastructure/telemetry"
)

// Option represents a functional option for configuring the GatewayService.
type Option func(*GatewayService)

// WithTimeout configures the maximum per-attempt timeout for provider calls.
func WithTimeout(timeout time.Duration) Option {
	return func(s *GatewayService) {
		if timeout > 0 {
			s.timeout = timeout
		}
	}
}

// WithMaxRetries configures the maximum retry attempts per provider before triggering fallback.
func WithMaxRetries(retries int) Option {
	return func(s *GatewayService) {
		if retries >= 0 {
			s.maxRetries = retries
		}
	}
}

// WithRetryBackoff configures the initial backoff delay between retries.
func WithRetryBackoff(backoff time.Duration) Option {
	return func(s *GatewayService) {
		if backoff > 0 {
			s.retryBackoff = backoff
		}
	}
}

// WithDefaultProvider configures the primary default AI provider.
func WithDefaultProvider(p domain.ProviderType) Option {
	return func(s *GatewayService) {
		s.defaultProvider = p
	}
}

// WithFallbackOrder defines the fallback sequence of providers when the primary fails.
func WithFallbackOrder(providers ...domain.ProviderType) Option {
	return func(s *GatewayService) {
		s.fallbackOrder = providers
	}
}

// WithCircuitBreakerConfig sets custom configuration for all provider circuit breakers.
func WithCircuitBreakerConfig(cfg circuitbreaker.Config) Option {
	return func(s *GatewayService) {
		s.cbConfig = cfg
	}
}

// WithLogger injects structured slog logger.
func WithLogger(logger *slog.Logger) Option {
	return func(s *GatewayService) {
		if logger != nil {
			s.logger = logger
		}
	}
}

// WithMetrics injects Prometheus metrics collector.
func WithMetrics(metrics *telemetry.Metrics) Option {
	return func(s *GatewayService) {
		s.metrics = metrics
	}
}
