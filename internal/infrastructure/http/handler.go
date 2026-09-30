package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/jsjg1/resilient-universal-ai-gateway/internal/domain"
	"github.com/jsjg1/resilient-universal-ai-gateway/internal/usecase"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
)

// Handler handles inbound HTTP requests for the gateway.
type Handler struct {
	gateway *usecase.GatewayService
	redis   *redis.Client
}

// NewHandler constructs a new Handler.
func NewHandler(gateway *usecase.GatewayService, redis *redis.Client) *Handler {
	return &Handler{
		gateway: gateway,
		redis:   redis,
	}
}

// RegisterRoutes registers all gateway routes to the provided http.ServeMux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/chat/completions", h.handleChatCompletion)
	mux.HandleFunc("GET /health", h.handleHealth)
	mux.Handle("GET /metrics", promhttp.Handler())
}

type errorResponse struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    int    `json:"code"`
}

func (h *Handler) handleChatCompletion(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Content-Type") != "" && r.Header.Get("Content-Type") != "application/json" {
		h.writeJSONError(w, "Content-Type must be application/json", "invalid_request_error", http.StatusBadRequest)
		return
	}

	var req domain.CompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeJSONError(w, "Failed to parse JSON body: "+err.Error(), "invalid_request_error", http.StatusBadRequest)
		return
	}

	if len(req.Messages) == 0 {
		h.writeJSONError(w, "Field 'messages' cannot be empty", "invalid_request_error", http.StatusBadRequest)
		return
	}

	resp, err := h.gateway.ExecuteCompletion(r.Context(), req)
	if err != nil {
		if errors.Is(err, domain.ErrInvalidRequest) {
			h.writeJSONError(w, err.Error(), "invalid_request_error", http.StatusBadRequest)
			return
		}
		if errors.Is(err, domain.ErrCircuitBreakerOpen) || errors.Is(err, domain.ErrAllProvidersFailed) {
			h.writeJSONError(w, "Service Temporarily Unavailable: "+err.Error(), "service_unavailable_error", http.StatusServiceUnavailable)
			return
		}
		h.writeJSONError(w, "Internal Gateway Error: "+err.Error(), "api_error", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	redisStatus := "disabled"
	if h.redis != nil {
		if err := h.redis.Ping(ctx).Err(); err != nil {
			redisStatus = "degraded: " + err.Error()
		} else {
			redisStatus = "connected"
		}
	}

	health := map[string]any{
		"status":    "healthy",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"redis":     redisStatus,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(health)
}

func (h *Handler) writeJSONError(w http.ResponseWriter, message, errType string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(errorResponse{
		Error: errorDetail{
			Message: message,
			Type:    errType,
			Code:    statusCode,
		},
	})
}
