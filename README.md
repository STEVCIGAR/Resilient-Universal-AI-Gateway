# Resilient-Universal-AI-Gateway

Quickstart con Docker Compose

1. Clonar y configurar en el ambiente:
```bash
export OPENAI_API_KEY="key-Openai"
export GEMINI_API_KEY="key-Gemini"
```

2. Iniciar el contenedor:
```bash
docker-compose up --build -d
```

Services exposed:
- **AI Gateway API**: `http://localhost:8080`
- **Prometheus UI**: `http://localhost:9090`
- **Redis**: `localhost:6379`

---

## API Endpoints

### Chat (`POST /v1/chat/completions`)

**Request**:
```json
{
  "model": "gpt-4o-mini",
  "messages": [
    {"role": "system", "content": "Eres una asistente útil."},
    {"role": "user", "content": "Explique los disyuntores en sistemas distribuidos."}
  ],
  "temperature": 0.7,
  "max_tokens": 500
}
```

**Response**:
```json
{
  "id": "resp_07d4b47c-...",
  "provider": "openai",
  "model": "gpt-4o-mini",
  "content": "Un disyuntor es un patrón de diseño utilizado en el desarrollo de software...",
  "finish_reason": "stop",
  "usage": {
    "prompt_tokens": 24,
    "completion_tokens": 120,
    "total_tokens": 144
  },
  "latency_ms": 340
}
```

### 2. Health Check (`GET /health`)
```json
{
  "status": "healthy",
  "redis": "connected",
  "timestamp": "2026-09-13T21:40:00Z"
}
```

### 3. Prometheus Metrics (`GET /metrics`)
Exposes:
- `ai_gateway_http_requests_total{method, path, status, status_class}`
- `ai_gateway_http_request_duration_seconds{method, path, status_class}` (P50/P95/P99 latency histogram)
- `ai_gateway_provider_requests_total{provider, model, status}`
- `ai_gateway_provider_duration_seconds{provider, model, status}`
- `ai_gateway_provider_tokens_total{provider, model, type}`
- `ai_gateway_circuit_breaker_state{provider}` (0=Closed, 1=HalfOpen, 2=Open)
- `ai_gateway_circuit_breaker_trips_total{provider}`
- `ai_gateway_rate_limiter_blocks_total{key_type}`

---

## Testing

Ejecutar todas las pruebas unitarias con validaciones de resiliencia y Backup:
```bash
go test -v ./...
```
