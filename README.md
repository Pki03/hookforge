# HookForge

> Self-hosted webhook delivery engine. Reliable, observable, zero cost.

HookForge ingests webhook events via API, queues them in Redis, and delivers them to your targets with exponential backoff retries, circuit breaker protection, SSRF defense, and a live monitoring dashboard. No cloud dependency — one binary, two dependencies (Postgres, Redis), one `docker compose up`.

[![Go Version](https://img.shields.io/badge/Go-1.26-blue)](https://golang.org)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![CI](https://github.com/Pki03/hookforge/actions/workflows/ci.yml/badge.svg)](https://github.com/Pki03/hookforge/actions/workflows/ci.yml)

**Stack:** Go 1.26 · Gin · pgx/v5 · go-redis/v9 · gorilla/websocket · Prometheus · HTMX · Docker · Helm

---

## Table of Contents

- [Features](#features)
- [Architecture](#architecture)
- [Quick Start](#quick-start)
- [Usage](#usage)
- [API Reference](#api-reference)
- [Configuration](#configuration)
- [Production Deploy](#production-deploy)
- [Monitoring](#monitoring)
- [Development](#development)
- [Contributing](#contributing)
- [License](#license)

---

## Features

- **Redis-backed job queue** — events are buffered in Redis streams; the API returns immediately regardless of delivery latency
- **Exponential backoff retry** — 1s → 2s → 4s → 8s → 16s with randomized jitter; configurable max retries
- **Dead Letter Queue** — events that exhaust retries are marked `dead` and available for one-click replay
- **Circuit breaker** — per-endpoint: 5 consecutive failures open the breaker; 30s auto-reset with half-open probe
- **SSRF protection** — blocks delivery to RFC1918, loopback, and link-local IPs at delivery time (not creation time)
- **HMAC-SHA256 signing** — every delivery includes `X-HookForge-Signature` header for payload verification
- **Per-endpoint rate limiting** — Redis token bucket with configurable `rate_limit_per_second` and `rate_limit_burst`
- **Event type filtering** — endpoints subscribe to specific event types; unmatched events are rejected at ingestion
- **Failure alerts** — Slack webhook and/or email (SMTP) notification on dead letter escalation
- **Live dashboard** — HTMX-polled stats panel with per-event delivery attempt logs, clickable event detail, and replay
- **Prometheus metrics** — `/metrics` endpoint exposes event counters, delivery latency histograms, retry gauge, circuit breaker trips
- **Raw SQL** — all database operations via pgx/v5 with no ORM abstraction
- **Graceful shutdown** — drains in-flight deliveries on SIGTERM with a 10s timeout

---

## Architecture

```
                  ┌─────────────┐
                  │  Your App   │
                  └──────┬──────┘
                         │ POST /api/v1/events
                         ▼
               ┌─────────────────┐
               │  Gin HTTP Server │
               │  (handlers,     │
               │   auth, ratelimit)│
               └────────┬────────┘
                        │ enqueue event ID
                        ▼
                 ┌──────────────┐
                 │    Redis     │
                 │  (job queue) │
                 └──────────────┘
                        │ worker pool pulls
                        ▼
               ┌─────────────────┐
               │  Worker Pool    │
               │  (N goroutines) │
               │                 │
               │  ┌───────────┐  │
               │  │ Circuit   │  │
               │  │ Breaker   │  │
               │  └───────────┘  │
               │  ┌───────────┐  │
               │  │ SSRF      │  │
               │  │ Check     │  │
               │  └───────────┘  │
               └────────┬────────┘
                        │ HTTP POST with HMAC sig
                        ▼
               ┌─────────────────┐
               │  Target Service │
               │  (your webhook) │
               └─────────────────┘
```

### Project structure

```
hookforge/
├── api/                    # OpenAPI 3.0 spec
├── cmd/
│   ├── migrate/            # Standalone migration binary
│   └── server/             # Main server entrypoint
├── db/
│   └── migrations/         # 005 incremental SQL migrations
├── deploy/
│   └── helm/hookforge/     # Kubernetes Helm chart (HPA, PDB, NetworkPolicy)
├── internal/
│   ├── circuitbreaker/     # Per-endpoint state machine (closed/open/half-open)
│   ├── config/             # Env-based configuration with sensible defaults
│   ├── dashboard/          # HTMX templates and WebSocket handler
│   ├── database/           # Raw SQL layer via pgx (endpoints, events, attempts, stats)
│   ├── handler/            # Gin HTTP handlers + fuzz tests + integration tests
│   ├── metrics/            # Prometheus metric definitions
│   ├── middleware/         # Auth, CORS, security headers, rate limit, body size
│   ├── notifier/           # Slack and email alert dispatchers
│   ├── ratelimit/          # Redis token bucket
│   └── redis/              # Redis client + event queue operations
├── templates/              # Go HTML templates (dashboard, swagger, event detail)
├── worker/                 # Delivery worker (poll queue → SSRF check → deliver → log)
├── Caddyfile               # Caddy reverse proxy config for prod HTTPS
├── Dockerfile              # Multi-stage build (non-root user, ca-certificates)
├── docker-compose.yml      # Dev stack (Postgres + Redis + app)
└── docker-compose.prod.yml # Prod stack (adds Caddy HTTPS, persistent volumes)
```

### Data model

| Table | Purpose |
|-------|---------|
| `endpoints` | Target URLs, secrets, rate limits, allowed event types, Slack/email alert config |
| `events` | Ingested payloads with status machine: `pending → delivered|retrying → dead` |
| `delivery_attempts` | Per-attempt log: HTTP status code, response body, error message, latency, timestamp |

### Delivery lifecycle

```
pending ──► retrying ──► delivered
                │
                ▼ (5 failures)
              dead ◄── replay resets to pending
```

---

## Quick Start

### Prerequisites

- Docker & Docker Compose
- Go 1.26+ (only for development)

### Run

```bash
git clone https://github.com/Pki03/hookforge
cd hookforge
docker compose up -d
```

Open **http://localhost:8080/dashboard** — you'll see the live monitoring dashboard.

### First event

```bash
# Create an endpoint (target URL that receives webhooks)
curl -s -X POST http://localhost:8080/api/v1/endpoints \
  -H "Content-Type: application/json" \
  -d '{"url": "https://webhook.site/your-test-url"}' | jq .
```

Save the returned `id` — you'll use it to fire events.

```bash
# Fire an event
curl -s -X POST http://localhost:8080/api/v1/events \
  -H "Content-Type: application/json" \
  -d '{
    "endpoint_id": "<endpoint-id>",
    "event_type": "user.signup",
    "payload": {"email": "user@example.com", "plan": "pro"}
  }' | jq .
```

Check the dashboard — the event appears as `pending` then `delivered` (or `retrying` if the target is down).

---

## Usage

### Endpoint management

```bash
# List all endpoints
curl http://localhost:8080/api/v1/endpoints | jq .

# Get a specific endpoint
curl http://localhost:8080/api/v1/endpoints/<id> | jq .

# Rotate signing secret
curl -X POST http://localhost:8080/api/v1/endpoints/<id>/rotate-secret | jq .
```

### Event management

```bash
# List events (filter by status: pending, delivered, retrying, dead)
curl "http://localhost:8080/api/v1/events?status=dead" | jq .

# Get event detail with full delivery attempt history
curl http://localhost:8080/api/v1/events/<event-id> | jq .

# Replay a dead letter event (resets to pending + re-enqueues)
curl -X POST http://localhost:8080/api/v1/events/<event-id>/replay | jq .
```

### Stats

```bash
curl http://localhost:8080/api/v1/stats | jq .
```

Response:
```json
{
  "total_sent": 142,
  "delivered": 138,
  "failed": 0,
  "dead": 4,
  "pending": 0,
  "delivery_rate_percent": 97.18,
  "avg_latency_ms": 234
}
```

### Event type filtering

Create an endpoint that only accepts specific event types:

```bash
curl -X POST http://localhost:8080/api/v1/endpoints \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://api.example.com/webhook",
    "allowed_event_types": ["order.placed", "order.shipped"]
  }' | jq .
```

Events with `event_type: "user.signup"` will be rejected with HTTP 422.

### Authentication (production)

Set `ADMIN_API_KEY` in your environment. All `/api/v1/*` endpoints then require:

```bash
curl -H "X-API-Key: your-secret-key" http://localhost:8080/api/v1/stats
```

---

## API Reference

Interactive Swagger UI at **http://localhost:8080/api/docs** (when running).

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | App liveness (no deps checked) |
| `GET` | `/ready` | Readiness (checks DB + Redis) |
| `GET` | `/metrics` | Prometheus metrics |
| `GET` | `/dashboard` | Live HTMX dashboard |
| `POST` | `/api/v1/endpoints` | Create endpoint |
| `GET` | `/api/v1/endpoints` | List endpoints |
| `GET` | `/api/v1/endpoints/:id` | Get endpoint |
| `POST` | `/api/v1/endpoints/:id/rotate-secret` | Rotate signing secret |
| `POST` | `/api/v1/events` | Fire an event |
| `GET` | `/api/v1/events` | List events (?status=&event_type=) |
| `GET` | `/api/v1/events/:id` | Get event + delivery attempts |
| `POST` | `/api/v1/events/:id/replay` | Replay dead letter event |
| `GET` | `/api/v1/stats` | Delivery statistics |

---

## Configuration

All configuration is via environment variables. The dev `docker-compose.yml` provides defaults that work out of the box.

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | HTTP listen port |
| `DATABASE_URL` | `postgres://postgres:postgres@localhost:5432/hookforge?sslmode=disable` | Postgres connection string |
| `REDIS_URL` | `redis://localhost:6379/0` | Redis connection string |
| `SIGNING_SECRET` | `hookforge-dev-secret` | HMAC-SHA256 signing key |
| `WORKER_COUNT` | `5` | Concurrent delivery goroutines |
| `ADMIN_API_KEY` | (none) | Enables API key auth on all endpoints |
| `ALLOWED_ORIGINS` | (none) | CORS allowed origins (comma-separated) |
| `MAX_BODY_BYTES` | `1048576` | Maximum request body size (bytes) |
| `SMTP_HOST` | (none) | SMTP server for email alerts |
| `SMTP_PORT` | `587` | SMTP port |
| `SMTP_USER` | (none) | SMTP username |
| `SMTP_PASSWORD` | (none) | SMTP password |
| `SMTP_FROM` | `hookforge@localhost` | From address for alert emails |

---

## Production Deploy

### Docker Compose (single VM with HTTPS)

```bash
export DOMAIN="hookforge.yourdomain.com"
export POSTGRES_PASSWORD="$(openssl rand -hex 32)"
export SIGNING_SECRET="$(openssl rand -hex 32)"
export ADMIN_API_KEY="$(openssl rand -hex 16)"

docker compose -f docker-compose.prod.yml up -d
```

This starts Postgres, Redis, the app (on port 8080, non-root user, resource-limited), and Caddy which auto-proxies with Let's Encrypt TLS.

### Kubernetes (Helm)

```bash
helm upgrade --install hookforge ./deploy/helm/hookforge \
  --set config.adminApiKey="your-key" \
  --set config.signingSecret="$(openssl rand -hex 32)"
```

The chart includes: Deployment (with securityContext), Service, ConfigMap, Secret, Ingress, HorizontalPodAutoscaler (CPU >70%, max 10), PodDisruptionBudget (min 1), NetworkPolicy, migration Job, and rolling update (maxUnavailable=0).

### Zero-cost public URL (development)

```bash
cloudflared tunnel --url http://localhost:8080
```

Gives you a temporary `https://random.trycloudflare.com` URL for testing.

---

## Monitoring

| Endpoint | Purpose |
|----------|---------|
| `/health` | Returns `{"status":"ok"}` — no dependency check |
| `/ready` | Returns `{"status":"ok","db":true,"redis":true}` — checks Postgres ping and Redis ping |
| `/metrics` | Prometheus metrics (default port) |

### Prometheus metrics

| Metric | Type | Labels |
|--------|------|--------|
| `hookforge_events_total` | Counter | `status` (pending / delivered / failed / dead) |
| `hookforge_delivery_latency_seconds` | Histogram | — |
| `hookforge_retry_events_current` | Gauge | — |
| `hookforge_delivery_attempts_total` | Counter | — |
| `hookforge_circuit_breaker_trips_total` | Counter | `endpoint_id` |

### Failure alerts

When an event enters the dead letter queue, HookForge sends alerts via:

- **Slack** — set `slack_webhook_url` on the endpoint
- **Email** — configure SMTP env vars and set `email` on the endpoint

---

## Development

### Prerequisites

- Go 1.26+
- Docker (for integration tests and local stack)
- Docker Compose

### Running tests

```bash
# Unit tests (no Docker needed)
go test ./internal/circuitbreaker/... -v -count=1 -race

# Integration tests (requires Docker — spins up Postgres + Redis containers)
docker compose up -d postgres redis
DATABASE_URL="postgres://postgres:postgres@localhost:5432/hookforge?sslmode=disable" \
REDIS_URL="redis://localhost:6379/0" \
INTEGRATION=true \
go test ./... -v -count=1 -race -shuffle=on -coverprofile=coverage.out -covermode=atomic

# Fuzz tests (no Docker needed)
go test ./internal/handler/... -fuzz=FuzzCreateEndpoint -fuzztime=15s
go test ./worker/... -fuzz=FuzzSSRFCheck -fuzztime=15s
```

### Linting

```bash
go vet ./...
staticcheck ./...
```

### CI pipeline

Every push to `main` or pull request triggers:

1. `go vet` + `staticcheck` — static analysis
2. `go test -race -shuffle=on -coverprofile=coverage.out` — test suite with race detection
3. `trivy fs` — vulnerability scan (results uploaded as SARIF to GitHub)
4. Docker build + push to `ghcr.io/Pki03/hookforge` (on `main` only)

Releases are automatically built and published on `v*` tags.

---

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

- Fork the repo and create a feature branch
- Run tests: `go test ./... -count=1 -race`
- Use conventional commits (`feat:`, `fix:`, `docs:`, `chore:`)
- Open a PR

---

## License

MIT. See [LICENSE](LICENSE).
