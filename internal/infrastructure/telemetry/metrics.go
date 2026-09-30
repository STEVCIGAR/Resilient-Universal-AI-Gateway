package telemetry

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds Prometheus metric collectors for the gateway.
type Metrics struct {
	// HTTP Metrics
	HTTPRequestsTotal   *prometheus.CounterVec
	HTTPRequestDuration *prometheus.HistogramVec

	// AI Provider Metrics
	ProviderRequestsTotal   *prometheus.CounterVec
	ProviderRequestDuration *prometheus.HistogramVec
	ProviderTokensTotal     *prometheus.CounterVec

	// Circuit Breaker Metrics
	CircuitBreakerState *prometheus.GaugeVec
	CircuitBreakerTrips *prometheus.CounterVec

	// Rate Limiting Metrics
	RateLimitBlocksTotal *prometheus.CounterVec
}

// NewMetrics initializes and registers all Prometheus metric collectors with the registry.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}

	factory := promauto.With(reg)

	return &Metrics{
		HTTPRequestsTotal: factory.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "ai_gateway",
				Subsystem: "http",
				Name:      "requests_total",
				Help:      "Total number of HTTP requests processed, labeled by method, path, and status code.",
			},
			[]string{"method", "path", "status", "status_class"},
		),

		HTTPRequestDuration: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "ai_gateway",
				Subsystem: "http",
				Name:      "request_duration_seconds",
				Help:      "Latency distribution of HTTP requests.",
				Buckets:   []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30},
			},
			[]string{"method", "path", "status_class"},
		),

		ProviderRequestsTotal: factory.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "ai_gateway",
				Subsystem: "provider",
				Name:      "requests_total",
				Help:      "Total number of requests dispatched to upstream AI providers.",
			},
			[]string{"provider", "model", "status"},
		),

		ProviderRequestDuration: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "ai_gateway",
				Subsystem: "provider",
				Name:      "request_duration_seconds",
				Help:      "Latency distribution of upstream AI provider requests for P95 and P99 monitoring.",
				Buckets:   []float64{0.1, 0.25, 0.5, 1, 2, 3, 5, 8, 12, 20, 30, 60},
			},
			[]string{"provider", "model", "status"},
		),

		ProviderTokensTotal: factory.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "ai_gateway",
				Subsystem: "provider",
				Name:      "tokens_total",
				Help:      "Total number of tokens consumed by upstream AI provider.",
			},
			[]string{"provider", "model", "type"}, // prompt vs completion
		),

		CircuitBreakerState: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "ai_gateway",
				Subsystem: "circuit_breaker",
				Name:      "state",
				Help:      "Current state of provider circuit breaker (0=Closed, 1=HalfOpen, 2=Open).",
			},
			[]string{"provider"},
		),

		CircuitBreakerTrips: factory.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "ai_gateway",
				Subsystem: "circuit_breaker",
				Name:      "trips_total",
				Help:      "Total count of circuit breaker state transitions to Open.",
			},
			[]string{"provider"},
		),

		RateLimitBlocksTotal: factory.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "ai_gateway",
				Subsystem: "rate_limiter",
				Name:      "blocks_total",
				Help:      "Total number of requests rejected due to rate limits.",
			},
			[]string{"key_type"},
		),
	}
}
