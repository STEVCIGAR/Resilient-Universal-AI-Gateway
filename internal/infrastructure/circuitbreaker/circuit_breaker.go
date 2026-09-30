package circuitbreaker

import (
	"log/slog"
	"time"

	"github.com/jsjg1/resilient-universal-ai-gateway/internal/domain"
	"github.com/jsjg1/resilient-universal-ai-gateway/internal/infrastructure/telemetry"
	"github.com/sony/gobreaker"
)

// Config configures the circuit breaker behavior.
type Config struct {
	MaxRequests  uint32        // Max requests allowed to pass through when Half-Open
	Interval     time.Duration // Cyclic period of the closed state to clear counts
	Timeout      time.Duration // Period of open state before switching to Half-Open
	MinRequests  uint32        // Minimum consecutive or windowed requests before tripping evaluation
	FailureRatio float64       // Error threshold ratio (0.5 = 50%)
}

// DefaultConfig returns production default settings.
func DefaultConfig() Config {
	return Config{
		MaxRequests:  5,
		Interval:     30 * time.Second,
		Timeout:      15 * time.Second,
		MinRequests:  6,
		FailureRatio: 0.50, // 50% failure rate triggers the breaker
	}
}

// ProviderBreaker wraps sony/gobreaker.CircuitBreaker with telemetry and custom error checking.
type ProviderBreaker struct {
	cb       *gobreaker.CircuitBreaker
	provider domain.ProviderType
	metrics  *telemetry.Metrics
	logger   *slog.Logger
}

// NewProviderBreaker creates a circuit breaker for a given AI provider.
func NewProviderBreaker(provider domain.ProviderType, cfg Config, metrics *telemetry.Metrics, logger *slog.Logger) *ProviderBreaker {
	if cfg.FailureRatio <= 0 {
		cfg.FailureRatio = 0.50
	}
	if cfg.MinRequests == 0 {
		cfg.MinRequests = 5
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 15 * time.Second
	}

	pb := &ProviderBreaker{
		provider: provider,
		metrics:  metrics,
		logger:   logger,
	}

	st := gobreaker.Settings{
		Name:        string(provider),
		MaxRequests: cfg.MaxRequests,
		Interval:    cfg.Interval,
		Timeout:     cfg.Timeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			if counts.Requests < cfg.MinRequests {
				return false
			}
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			return failureRatio >= cfg.FailureRatio
		},
		OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
			pb.handleStateChange(name, from, to)
		},
	}

	pb.cb = gobreaker.NewCircuitBreaker(st)

	// Set initial metrics gauge (0 = Closed)
	if metrics != nil {
		metrics.CircuitBreakerState.WithLabelValues(string(provider)).Set(0)
	}

	return pb
}

// Execute runs the given operation through the circuit breaker.
func (pb *ProviderBreaker) Execute(op func() (any, error)) (any, error) {
	result, err := pb.cb.Execute(op)
	if err != nil {
		if err == gobreaker.ErrOpenState || err == gobreaker.ErrTooManyRequests {
			return nil, domain.ErrCircuitBreakerOpen
		}
		return nil, err
	}
	return result, nil
}

// State returns the current gobreaker state.
func (pb *ProviderBreaker) State() gobreaker.State {
	return pb.cb.State()
}

// Name returns the breaker name.
func (pb *ProviderBreaker) Name() string {
	return string(pb.provider)
}

func (pb *ProviderBreaker) handleStateChange(name string, from gobreaker.State, to gobreaker.State) {
	var stateVal float64
	switch to {
	case gobreaker.StateClosed:
		stateVal = 0
	case gobreaker.StateHalfOpen:
		stateVal = 1
	case gobreaker.StateOpen:
		stateVal = 2
	}

	if pb.metrics != nil {
		pb.metrics.CircuitBreakerState.WithLabelValues(name).Set(stateVal)
		if to == gobreaker.StateOpen {
			pb.metrics.CircuitBreakerTrips.WithLabelValues(name).Inc()
		}
	}

	if pb.logger != nil {
		pb.logger.Warn("circuit breaker state changed",
			slog.String("provider", name),
			slog.String("from_state", from.String()),
			slog.String("to_state", to.String()),
		)
	}
}
