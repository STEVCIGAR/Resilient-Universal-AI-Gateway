package http

import (
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jsjg1/resilient-universal-ai-gateway/internal/domain"
	"github.com/jsjg1/resilient-universal-ai-gateway/internal/infrastructure/telemetry"
)

type responseWriterWrapper struct {
	http.ResponseWriter
	statusCode int
	bytesRead  int
}

func (rw *responseWriterWrapper) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriterWrapper) Write(b []byte) (int, error) {
	if rw.statusCode == 0 {
		rw.statusCode = http.StatusOK
	}
	n, err := rw.ResponseWriter.Write(b)
	rw.bytesRead += n
	return n, err
}

// RequestIDMiddleware injects or forwards unique request ID in context and header.
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := r.Header.Get("X-Request-ID")
		if reqID == "" {
			reqID = uuid.New().String()
		}
		w.Header().Set("X-Request-ID", reqID)
		ctx := telemetry.WithRequestID(r.Context(), reqID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// LoggingAndMetricsMiddleware logs requests and measures Prometheus metrics.
func LoggingAndMetricsMiddleware(logger *slog.Logger, metrics *telemetry.Metrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			wrapper := &responseWriterWrapper{ResponseWriter: w, statusCode: http.StatusOK}

			next.ServeHTTP(wrapper, r)

			duration := time.Since(start)
			durationSec := duration.Seconds()
			status := wrapper.statusCode
			statusStr := strconv.Itoa(status)

			statusClass := "2xx"
			if status >= 400 && status < 500 {
				statusClass = "4xx"
			} else if status >= 500 {
				statusClass = "5xx"
			} else if status >= 300 && status < 400 {
				statusClass = "3xx"
			}

			path := r.URL.Path

			if metrics != nil {
				metrics.HTTPRequestsTotal.WithLabelValues(r.Method, path, statusStr, statusClass).Inc()
				metrics.HTTPRequestDuration.WithLabelValues(r.Method, path, statusClass).Observe(durationSec)
			}

			if logger != nil {
				logger.InfoContext(r.Context(), "http request processed",
					slog.String("method", r.Method),
					slog.String("path", path),
					slog.Int("status", status),
					slog.String("status_class", statusClass),
					slog.Float64("duration_ms", float64(duration.Microseconds())/1000.0),
					slog.String("remote_addr", r.RemoteAddr),
					slog.String("user_agent", r.UserAgent()),
				)
			}
		})
	}
}

// RateLimitMiddleware applies distributed Redis token bucket rate limiting.
func RateLimitMiddleware(limiter domain.RateLimiter, rate float64, capacity int64, metrics *telemetry.Metrics, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip rate limiting on /health and /metrics
			if r.URL.Path == "/health" || r.URL.Path == "/metrics" {
				next.ServeHTTP(w, r)
				return
			}

			// Extract rate limit key (API Key or IP)
			clientKey := extractClientKey(r)

			res, err := limiter.Allow(r.Context(), clientKey, rate, capacity)
			if err != nil {
				if logger != nil {
					logger.ErrorContext(r.Context(), "rate limit error, fail open", slog.String("error", err.Error()))
				}
				// Fail-open to avoid total outage
				next.ServeHTTP(w, r)
				return
			}

			w.Header().Set("X-RateLimit-Remaining", strconv.FormatInt(res.Remaining, 10))
			w.Header().Set("X-RateLimit-Reset-Ms", strconv.FormatInt(res.ResetMs, 10))

			if !res.Allowed {
				if metrics != nil {
					metrics.RateLimitBlocksTotal.WithLabelValues("api_key").Inc()
				}
				if logger != nil {
					logger.WarnContext(r.Context(), "rate limit exceeded",
						slog.String("client_key", clientKey),
						slog.Int64("reset_ms", res.ResetMs),
					)
				}
				w.Header().Set("Retry-After", strconv.FormatInt(res.ResetMs/1000+1, 10))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"error":{"message":"Rate limit exceeded. Please retry later.","type":"rate_limit_error","code":429}}`))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func extractClientKey(r *http.Request) string {
	if apiKey := r.Header.Get("X-API-Key"); apiKey != "" {
		return "key:" + apiKey
	}

	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return "bearer:" + strings.TrimPrefix(auth, "Bearer ")
	}

	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	if ip == "" {
		ip = "anonymous"
	}
	return "ip:" + ip
}
