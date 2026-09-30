package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jsjg1/resilient-universal-ai-gateway/config"
	"github.com/jsjg1/resilient-universal-ai-gateway/internal/domain"
	"github.com/jsjg1/resilient-universal-ai-gateway/internal/infrastructure/ai"
	gatewayHTTP "github.com/jsjg1/resilient-universal-ai-gateway/internal/infrastructure/http"
	"github.com/jsjg1/resilient-universal-ai-gateway/internal/infrastructure/ratelimit"
	"github.com/jsjg1/resilient-universal-ai-gateway/internal/infrastructure/telemetry"
	"github.com/jsjg1/resilient-universal-ai-gateway/internal/usecase"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
)

func main() {
	// 1. Load configuration
	cfg := config.LoadFromEnv()

	// 2. Setup structured logging
	var logLevel slog.Level
	switch cfg.LogLevel {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}
	logger := telemetry.NewLogger(cfg.LogJSON, logLevel)
	slog.SetDefault(logger)

	logger.Info("bootstrapping resilient universal ai gateway",
		slog.Int("port", cfg.Port),
		slog.String("default_provider", cfg.DefaultProvider),
	)

	// 3. Setup Prometheus Metrics
	metrics := telemetry.NewMetrics(prometheus.DefaultRegisterer)

	// 4. Connect to Redis for Distributed Rate Limiting
	redisClient := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})

	pingCtx, pingCancel := context.WithTimeout(context.Background(), 2*time.Second)
	if err := redisClient.Ping(pingCtx).Err(); err != nil {
		logger.Warn("redis connection not available; rate limiter running in fail-open mode", slog.String("error", err.Error()))
	} else {
		logger.Info("connected to redis successfully", slog.String("addr", cfg.RedisAddr))
	}
	pingCancel()

	rateLimiter := ratelimit.NewRedisRateLimiter(redisClient)

	// 5. Setup AI Providers and Factory (Factory Pattern & Strategy Pattern)
	httpClient := &http.Client{
		Timeout: cfg.ProviderTimeout,
	}

	openaiAdapter := ai.NewOpenAIAdapter(cfg.OpenAIAPIKey, cfg.OpenAIBaseURL, httpClient)
	geminiAdapter := ai.NewGeminiAdapter(cfg.GeminiAPIKey, cfg.GeminiBaseURL, httpClient)

	providerFactory := ai.NewProviderFactory()
	providerFactory.Register(openaiAdapter)
	providerFactory.Register(geminiAdapter)

	// 6. Setup Gateway Service with Functional Options
	defaultProvider := domain.ProviderType(cfg.DefaultProvider)
	gatewayService := usecase.NewGatewayService(
		providerFactory,
		usecase.WithDefaultProvider(defaultProvider),
		usecase.WithFallbackOrder(domain.ProviderOpenAI, domain.ProviderGemini),
		usecase.WithTimeout(cfg.ProviderTimeout),
		usecase.WithMaxRetries(cfg.ProviderMaxRetries),
		usecase.WithLogger(logger),
		usecase.WithMetrics(metrics),
	)

	// 7. Setup HTTP Handlers and Middleware Pipeline
	handler := gatewayHTTP.NewHandler(gatewayService, redisClient)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// Apply Middlewares: Rate Limiting -> Logging & Metrics -> Request ID
	var chainedHandler http.Handler = mux
	chainedHandler = gatewayHTTP.RateLimitMiddleware(rateLimiter, cfg.RateLimitRPS, cfg.RateLimitBurst, metrics, logger)(chainedHandler)
	chainedHandler = gatewayHTTP.LoggingAndMetricsMiddleware(logger, metrics)(chainedHandler)
	chainedHandler = gatewayHTTP.RequestIDMiddleware(chainedHandler)

	// 8. Start Server with Graceful Shutdown
	server := gatewayHTTP.NewServer(cfg.Port, chainedHandler, logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := server.Start(ctx); err != nil {
		logger.Error("server fatal error", slog.String("error", err.Error()))
		os.Exit(1)
	}

	logger.Info("gateway server terminated cleanly")
}
