package config

import (
	"os"
	"strconv"
	"time"
)

// Config encapsulates all environment and runtime configurations.
type Config struct {
	Port               int
	LogJSON            bool
	LogLevel           string
	RedisAddr          string
	RedisPassword      string
	RedisDB            int
	RateLimitRPS       float64
	RateLimitBurst     int64
	OpenAIAPIKey       string
	OpenAIBaseURL      string
	GeminiAPIKey       string
	GeminiBaseURL      string
	DefaultProvider    string
	ProviderTimeout    time.Duration
	ProviderMaxRetries int
}

// LoadFromEnv loads configuration values from environment variables or sensible defaults.
func LoadFromEnv() *Config {
	return &Config{
		Port:               getEnvInt("PORT", 8080),
		LogJSON:            getEnvBool("LOG_JSON", true),
		LogLevel:           getEnvString("LOG_LEVEL", "info"),
		RedisAddr:          getEnvString("REDIS_ADDR", "localhost:6379"),
		RedisPassword:      getEnvString("REDIS_PASSWORD", ""),
		RedisDB:            getEnvInt("REDIS_DB", 0),
		RateLimitRPS:       getEnvFloat("RATE_LIMIT_RPS", 20.0),
		RateLimitBurst:     getEnvInt64("RATE_LIMIT_BURST", 40),
		OpenAIAPIKey:       getEnvString("OPENAI_API_KEY", ""),
		OpenAIBaseURL:      getEnvString("OPENAI_BASE_URL", "https://api.openai.com"),
		GeminiAPIKey:       getEnvString("GEMINI_API_KEY", ""),
		GeminiBaseURL:      getEnvString("GEMINI_BASE_URL", "https://generativelanguage.googleapis.com"),
		DefaultProvider:    getEnvString("DEFAULT_PROVIDER", "openai"),
		ProviderTimeout:    time.Duration(getEnvInt("PROVIDER_TIMEOUT_SEC", 30)) * time.Second,
		ProviderMaxRetries: getEnvInt("PROVIDER_MAX_RETRIES", 1),
	}
}

func getEnvString(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if val := os.Getenv(key); val != "" {
		if n, err := strconv.Atoi(val); err == nil {
			return n
		}
	}
	return fallback
}

func getEnvInt64(key string, fallback int64) int64 {
	if val := os.Getenv(key); val != "" {
		if n, err := strconv.ParseInt(val, 10, 64); err == nil {
			return n
		}
	}
	return fallback
}

func getEnvFloat(key string, fallback float64) float64 {
	if val := os.Getenv(key); val != "" {
		if n, err := strconv.ParseFloat(val, 64); err == nil {
			return n
		}
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	if val := os.Getenv(key); val != "" {
		if b, err := strconv.ParseBool(val); err == nil {
			return b
		}
	}
	return fallback
}
