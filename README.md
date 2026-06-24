# HookForge

> Self-hosted webhook delivery engine. Reliable, observable, free.

[![Go Version](https://img.shields.io/badge/Go-1.26-blue)](https://golang.org)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![CI](https://github.com/Pki03/hookforge/actions/workflows/ci.yml/badge.svg)](https://github.com/Pki03/hookforge/actions/workflows/ci.yml)
[![Docker](https://img.shields.io/badge/Docker-ready-2496ED?logo=docker)](https://hub.docker.com)

HookForge is a production-ready webhook delivery engine that fits on a €4/mo VPS. It handles event ingestion, delivery with exponential backoff retries, HMAC payload signing, dead-letter queues, real-time monitoring, and Prometheus metrics — without the bloat.

**[Live demo](https://hookforge.dev)** · **[Quick start](#quick-start)** · **[API docs](#api-reference)**

---

## The Problem

- Webhooks fail. Networks degrade, services restart, timeouts happen. Without a reliable delivery engine, you lose events — and that means lost orders, missed alerts, broken integrations.
- Most teams either build their own (hours of engineering) or pay for a managed service ($50+/mo). HookForge gives you production-grade delivery in one `docker-compose up`.
- Without retry + dead letter queues, a single transient failure cascades into data loss. HookForge retries with exponential backoff and moves undeliverable events to a DLQ for manual replay.

## Features

- Redis-backed job queue with configurable goroutine worker pool
- Exponential backoff retry: 1s → 2s → 4s → 8s → 16s → 32s
- HMAC-SHA256 payload signing via `X-HookForge-Signature` header
- Dead Letter Queue with one-click replay
- Per-endpoint rate limiting via Redis token bucket
- Real-time HTMX dashboard with WebSocket live updates
- Delivery attempt logging with per-attempt HTTP status, response body, and latency
- Prometheus metrics (`/metrics`) — counters, histograms, gauges
- Single `docker-compose up` deploy: Go + Postgres + Redis
- Full test suite with testcontainers-go (real Postgres + Redis containers)
- Slack + Email failure alerts on dead letter escalation
- Event type filtering: endpoints subscribe to specific event types
- Kubernetes Helm chart for production deploy (`deploy/helm/hookforge/`)
- Circuit breaker per endpoint (5-failure threshold, 30s auto-reset)
- JSON structured logging (log/slog)
- OpenAPI 3.0 spec with Swagger UI at `/api/docs`
- Race-condition tested CI (`go test -race -shuffle=on`)

## Quick Start

```bash
git clone https://github.com/prateekkhurmi/hookforge
cd hookforge
docker-compose up
# Open http://localhost:8080/dashboard
```

```bash
# Create an endpoint
curl -X POST http://localhost:8080/api/v1/endpoints \
  -H "Content-Type: application/json" \
  -d '{"url": "https://webhook.site/your-test-url"}'

# Send an event
curl -X POST http://localhost:8080/api/v1/events \
  -H "Content-Type: application/json" \
  -d '{"endpoint_id": "<id from above>", "payload": {"hello": "world"}}'

# Check stats
curl http://localhost:8080/api/v1/stats
```

## Architecture

```
                         ┌──────────────────────────────────────────────────┐
                         │                 HookForge Server                 │
                         │                                                  │
   POST /endpoints ──────┤  ┌──────────┐    ┌──────────────────────────┐   │
   POST /events    ──────┤  │  Gin API │───▶   Worker Pool (N goroutines)│──┼──▶ Target URL
                         │  └────┬─────┘    │  BRPOP → HTTP POST       │   │    (HMAC signed)
                         │       │          │  + delivery_attempts log │   │
                         │  ┌────▼─────┐    └──────┬───────────────────┘   │
                         │  │  Redis   │           │                       │
                         │  │  Queue   │    ┌──────▼───────────────────┐   │
                         │  └──────────┘    │  Retry Engine            │   │
                         │                  │  (1s poll goroutine)     │   │
                         │  ┌──────────┐    └──────┬───────────────────┘   │
                         │  │PostgreSQL│           │                       │
                         │  │(pgx raw) │◀──────────┘                       │
                         │  └──────────┘                                    │
                         └──────────────────────────────────────────────────┘
```

## API Reference

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/endpoints` | List all endpoints |
| `POST` | `/api/v1/endpoints` | Create an endpoint |
| `GET` | `/api/v1/endpoints/:id` | Get endpoint details |
| `POST` | `/api/v1/endpoints/:id/rotate-secret` | Rotate HMAC signing secret |
| `POST` | `/api/v1/events` | Create and enqueue an event |
| `GET` | `/api/v1/events` | List events (filter by `?status=`) |
| `GET` | `/api/v1/events/:id` | Get event detail with delivery attempts |
| `POST` | `/api/v1/events/:id/replay` | Re-enqueue a dead-letter event |
| `GET` | `/api/v1/stats` | Aggregated delivery statistics |
| `GET` | `/dashboard` | Real-time monitoring dashboard |
| `GET` | `/metrics` | Prometheus metrics |

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | HTTP server port |
| `DATABASE_URL` | `postgres://postgres:postgres@localhost:5432/hookforge?sslmode=disable` | PostgreSQL connection string |
| `REDIS_URL` | `redis://localhost:6379/0` | Redis connection string |
| `SIGNING_SECRET` | `hookforge-dev-secret` | Fallback HMAC signing key |
| `WORKER_COUNT` | `5` | Number of concurrent delivery goroutines |
| `ADMIN_API_KEY` | (empty) | API key for admin endpoints (empty = no auth) |
| `ALLOWED_ORIGINS` | (empty) | CORS allowed origins (comma-separated, `*` for all) |
| `MAX_BODY_BYTES` | `1048576` | Maximum request body size in bytes (1MB) |
| `SMTP_HOST` | (empty) | SMTP server for email alerts |
| `SMTP_PORT` | `587` | SMTP port |
| `SMTP_USER` | (empty) | SMTP username |
| `SMTP_PASSWORD` | (empty) | SMTP password |
| `SMTP_FROM` | `hookforge@localhost` | From address for alert emails |

## Benchmarks

Run the benchmark yourself:

```bash
docker-compose up -d
./load-test/run_benchmark.sh
```

Expected results on a single node (2 CPU, 4GB RAM):

| Metric | Value |
|--------|-------|
| Throughput | 3,000+ events/sec |
| p50 delivery latency | < 8ms |
| p99 delivery latency | < 15ms |
| Delivery success rate | > 99% |
| Retry accuracy | 100% (tested via chaos engineering) |

*Tested with k6, 50 concurrent VUs, 60-second steady state, PostgreSQL + Redis on docker-compose.*

## Self-Hosting

### Docker Compose (single VM)

Deploy on a €4/mo Hetzner CX22 VPS:

```bash
# Install Docker + Caddy
apt install docker.io docker-compose caddy

# Clone and start
git clone https://github.com/prateekkhurmi/hookforge
cd hookforge
docker-compose -f docker-compose.prod.yml up -d
```

### Kubernetes (Helm)

```bash
helm upgrade --install hookforge ./deploy/helm/hookforge \
  --set config.signingSecret="your-secret" \
  --set postgresql.password="your-db-pass" \
  --set ingress.enabled=true \
  --set ingress.host="hookforge.yourdomain.com"
```

Requires Postgres and Redis running in-cluster or externally (set via `postgresql.*` and `redis.*` values).

Caddyfile (`/etc/caddy/Caddyfile`):

```
hookforge.dev {
    reverse_proxy localhost:8080
}
```

## Project Structure

```
├── cmd/server/main.go              # Entry point
├── internal/
│   ├── config/config.go            # Env-based configuration
│   ├── database/                   # PostgreSQL layer (pgx)
│   │   ├── postgres.go             # Connection pool
│   │   ├── endpoints.go            # Endpoint CRUD
│   │   ├── events.go               # Event CRUD
│   │   ├── attempts.go             # Delivery attempt logging
│   │   ├── stats.go                # Aggregation queries
│   │   └── models.go               # Domain types
│   ├── handler/handler.go          # HTTP handlers
│   ├── dashboard/handler.go        # HTMX dashboard handlers
│   ├── dashboard/ws.go             # WebSocket dashboard
│   ├── middleware/ratelimit.go     # Rate limiting middleware
│   ├── notifier/slack.go           # Slack alert integration
│   ├── ratelimit/ratelimit.go      # Redis token bucket
│   ├── metrics/metrics.go          # Prometheus metric definitions
│   ├── redis/client.go             # Redis connection + queue ops
│   └── router/router.go           # Route definitions
├── worker/worker.go                # Delivery + retry goroutines
├── db/migrations/                  # SQL migrations (golang-migrate)
├── templates/                      # Go HTML templates
├── load-test/                      # k6 load test scripts
├── docker-compose.yml              # Dev deploy
├── docker-compose.prod.yml         # Production deploy
├── Dockerfile                      # Multi-stage build
└── .github/workflows/ci.yml        # CI pipeline
```

## License

MIT © [Prateek Khurmi](https://github.com/Pki03)
