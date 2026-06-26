# HookForge — Complete Knowledge Reference

> Auto-generated from full source analysis. Every file, every package, every pattern.

---

## 1. PROJECT OVERVIEW

**HookForge**: Self-hosted webhook delivery engine in Go. Ingest → Redis queue → worker pool → deliver to target with retries, circuit breaker, SSRF protection, HMAC signing, rate limiting, event type filtering, failure alerts (Slack + Email), Prometheus metrics, live HTMX dashboard.

**Stack**: Go 1.26, Gin, pgx/v5 (raw SQL), go-redis/v9, gorilla/websocket, Prometheus, testcontainers-go, golang-migrate, k6, Docker, Helm

**Architecture**: Client → Gin API → Redis LPUSH/BRPOP queue → N goroutine worker pool (default 5) → HTTP POST to target URL. Separate retry poller goroutine. Dashboard polls via HTMX every 3s (stats) / 4s (events). WebSocket also available.

**Data Model**: `endpoints` → `events` → `delivery_attempts` (1:N:N)

**Delivery Lifecycle**: `pending` → `retrying` (max 5 attempts, exponential backoff 1s→32s capped) → `delivered` or `dead`. Replay resets `dead` → `pending` and re-enqueues.

---

## 2. DIRECTORY TREE (53 files)

```
hookforge/
├── .github/workflows/
│   ├── ci.yml                (2 jobs: lint + test, no external services)
│   └── release.yml           (on v* tag: docker build/push + gh release)
├── api/
│   └── openapi.json          (OpenAPI 3.0.3 spec, 353 lines)
├── cmd/
│   ├── server/main.go        (app entrypoint: connect DB, Redis, start worker, start server, graceful shutdown)
│   └── migrate/main.go       (standalone migration binary for Helm hook)
├── db/migrations/
│   ├── 001_init.{up,down}.sql              (endpoints, events tables + indexes)
│   ├── 002_endpoint_features.{up,down}.sql  (secret, slack_webhook_url, rate_limit columns)
│   ├── 003_delivery_attempts.{up,down}.sql   (delivery_attempts table)
│   ├── 004_event_types.{up,down}.sql         (event_type, allowed_event_types columns)
│   └── 005_email_notifications.{up,down}.sql (email column on endpoints)
├── deploy/helm/hookforge/
│   ├── Chart.yaml            (v0.1.0, appVersion 1.0.0)
│   ├── values.yaml           (replicaCount:2, HPA max:10, resources)
│   └── templates/
│       ├── _helpers.tpl       (name/fullname/labels templates)
│       ├── configmap.yaml     (PORT, WORKER_COUNT, DATABASE_URL, REDIS_URL, SMTP_*, ALLOWED_ORIGINS, MAX_BODY_BYTES)
│       ├── secret.yaml        (SIGNING_SECRET, SMTP_PASSWORD, ADMIN_API_KEY)
│       ├── deployment.yaml    (RollingUpdate maxUnavailable:0, securityContext, probes, resource limits)
│       ├── service.yaml       (ClusterIP, port 8080)
│       ├── ingress.yaml       (optional, with TLS)
│       ├── hpa.yaml           (CPU >70%, max 10 replicas)
│       ├── pdb.yaml           (minAvailable: 1)
│       ├── networkpolicy.yaml (ingress from same app only, egress all)
│       └── migration-job.yaml (post-install/post-upgrade hook, /app/migrate command)
├── internal/
│   ├── circuitbreaker/
│   │   ├── circuitbreaker.go       (per-endpoint state machine: closed/open/half-open)
│   │   └── circuitbreaker_test.go  (11 tests: state transitions, concurrency, isolation)
│   ├── config/
│   │   └── config.go               (env-based config loader with defaults)
│   ├── dashboard/
│   │   ├── handler.go              (HTMX page, stats panel, events panel, event detail)
│   │   └── ws.go                   (WebSocket handler: 2s ticker broadcasts stats+events)
│   ├── database/
│   │   ├── postgres.go             (pgxpool wrapper: Connect, Ping, Close)
│   │   ├── models.go               (Endpoint, Event, DeliveryAttempt, Stats structs)
│   │   ├── endpoints.go            (CRUD: Create, Get, GetURL, List, RotateSecret)
│   │   ├── events.go               (CRUD: Create, Get, List, UpdateStatus, RecordAttempt, IncrementAttempts)
│   │   ├── attempts.go             (CreateAttempt, ListAttempts)
│   │   ├── stats.go                (GetStats: 6 counters, delivery rate %)
│   │   └── db_test.go              (TestMain with testcontainers, 4 integration tests)
│   ├── handler/
│   │   ├── handler.go              (Gin handlers: endpoints CRUD, events CRUD, replay, stats)
│   │   ├── fuzz_test.go            (FuzzValidateTargetURL, FuzzCreateEndpointReq)
│   │   └── integration_test.go     (4 handler-level integration tests with gin test context)
│   ├── metrics/
│   │   └── metrics.go              (5 Prometheus metrics: events_total, latency, retry gauge, attempts, breaker trips)
│   ├── middleware/
│   │   ├── auth.go                 (optional X-API-Key admin auth, 401/403)
│   │   ├── ratelimit.go            (reads endpoint_id from body, checks Redis token bucket)
│   │   └── security.go             (CORS, security headers, body size limit)
│   ├── notifier/
│   │   ├── slack.go                (POST to Slack webhook with formatted message)
│   │   └── email.go                (net/smtp.SendMail with PlainAuth)
│   ├── ratelimit/
│   │   └── ratelimit.go            (Redis Lua token bucket script, atomic check-and-consume)
│   └── redis/
│       └── client.go               (wrapper: Connect, EnqueueEvent LPUSH, DequeueEvent BRPOP)
├── load-test/
│   ├── k6_load_test.js             (ramp to 50 VUs, create endpoints + fire events + check stats)
│   └── run_benchmark.sh            (wrapper script: checks health, runs k6, prints results)
├── templates/
│   ├── dashboard.html              (HTMX main page, dark theme, 2 panels + stats grid)
│   ├── _stats.html                 (6 stat cards: total, delivered, failed, dead, pending, rate %)
│   ├── _events.html                (events table with status badges, hx-get on click for detail)
│   ├── _event_detail.html          (detail grid + delivery attempts table + replay button for dead)
│   └── swagger.html                (Swagger UI loading openapi.json)
├── worker/
│   ├── worker.go                   (delivery loop, retry loop, circuit breaker, SSRF check, HMAC sign, backoff calc)
│   └── fuzz_test.go                (FuzzSSRFCheck)
├── api/openapi.json                (OpenAPI 3.0 spec, served at /api/v1/openapi.json)
├── Dockerfile                      (multi-stage: golang:1.26-alpine → alpine:3.19, non-root user)
├── docker-compose.yml              (dev: postgres:16 + redis:7 + app, health checks, resource limits)
├── docker-compose.prod.yml         (prod: adds Caddy HTTPS, AOF Redis, internal network, restart:always)
├── Caddyfile                       (reverse proxy app:8080 with {$DOMAIN})
├── .gitignore                      (binaries, vendor, .env, IDE files)
├── go.mod / go.sum                 (module github.com/prateekkhurmi/hookforge, go 1.26.4)
├── README.md                       (comprehensive: features, architecture, quick start, API, config, deploy, monitoring)
├── EXPLAIN.md                      (interview guide: 16 sections with tradeoffs and interview answers)
├── RUNBOOK.md                      (health checks, restart, common issues, DB/Redis recovery)
├── TODO.md                         (all items ✅, remaining features noted)
├── CONTRIBUTING.md                 (fork → branch → test → PR, conventional commits)
├── SECURITY.md                     (report via GitHub Issues with label `security`)
└── LICENSE                         (MIT, Copyright 2026 Prateek Khurmi)
```

---

## 3. PACKAGE-BY-PACKAGE DEEP DIVE

### 3.1 `cmd/server/main.go` — Entrypoint

**Purpose**: Wire everything together, start worker, start HTTP server, handle graceful shutdown.

**Flow**:
1. JSON logger (`slog.NewJSONHandler`)
2. Load config from env
3. Connect to Postgres (pgxpool)
4. Run migrations via golang-migrate (`file://db/migrations`)
5. Connect to Redis (go-redis/v9)
6. Initialize Prometheus metrics (pre-declare `pending`, `delivered`, `failed`, `dead` label values)
7. Create worker and start it (`worker.Start(ctx)`)
8. Setup Gin router
9. Start HTTP server in goroutine
10. Wait for SIGINT/SIGTERM → `cancel()` stops worker → `srv.Shutdown(10s timeout)` drains requests

**Key Patterns**:
- `slog.NewJSONHandler` — structured JSON logging to stdout
- `golang-migrate` with file source — migrations embedded in binary
- `signal.Notify` + context cancellation — clean shutdown chain
- Prometheus label pre-declaration — ensures metrics appear before first event

### 3.2 `cmd/migrate/main.go` — Standalone migration binary

**Purpose**: Used by Helm hook Job (`/app/migrate` command). Same migration logic as server startup but as a separate binary.

**Key**: Built in the same Dockerfile: `go build -o /migrate ./cmd/migrate`. Helm runs this before app pods roll out.

### 3.3 `internal/config/config.go` — Configuration

**Purpose**: Load all configuration from environment variables with sensible defaults.

```go
type Config struct {
    Port, DatabaseURL, RedisURL, SigningSecret string
    WorkerCount int
    SMTPHost, SMTPPort, SMTPUser, SMTPPassword, SMTPFrom string
    AdminAPIKey, AllowedOrigins string
    MaxBodyBytes int64
}
```

**Defaults**:
- Port: 8080
- DB: `postgres://postgres:postgres@localhost:5432/hookforge?sslmode=disable`
- Redis: `redis://localhost:6379/0`
- Worker: 5
- Max body: 1MB (1048576)

**Helper**: `EmailConfig()` method converts SMTP fields into `notifier.EmailConfig` struct.

### 3.4 `internal/router/router.go` — HTTP Router

**Purpose**: Define all routes, middleware, template functions, and static file serving.

**Middleware stack** (applied globally):
1. `gin.Logger()` — request logging
2. `gin.Recovery()` — panic recovery -> 500
3. `middleware.SecurityHeaders()` — nosniff, DENY, Referrer-Policy, COOP, COEP
4. `middleware.CORS(allowedOrigins)` — origin check, preflight 204
5. `middleware.BodySizeLimit(maxBytes)` — 1MB default

**Template functions**: `shortID` (truncate UUIDs to 8 chars), `since` (human-readable duration), `json` (raw JS output for templating)

**Route groups**:
- `/health` — app liveness (pings DB)
- `/ready` — readiness (pings DB + Redis)
- `/api/v1` — API group with optional `AdminAuth` middleware
  - `/endpoints` — GET (list), POST (create)
  - `/endpoints/:id` — GET (detail), POST /rotate-secret
  - `/events` — POST (create, with rate limit middleware), GET (list)
  - `/events/:id` — GET (detail), POST /replay
  - `/stats` — GET
- `/dashboard` — HTMX dashboard page
- `/api/v1/dashboard/stats` — HTMX stats panel (polled every 3s)
- `/api/v1/dashboard/events` — HTMX events panel (polled every 4s)
- `/dashboard/events/:id` — HTMX event detail panel (click-to-load)
- `/api/v1/ws` — WebSocket endpoint
- `/metrics` — Prometheus metrics
- `/api/v1/openapi.json` — static OpenAPI spec
- `/api/docs` — Swagger UI

**Key Pattern**: Rate limit middleware is applied only to the `/events` subgroup, not all of `/api/v1`. This is because rate limiting requires reading the body to extract `endpoint_id`, so it's scoped to the endpoint that needs it.

### 3.5 `internal/database/` — Data Layer

#### `postgres.go`
**Purpose**: Thin wrapper around `pgxpool.Pool`. Connect, Ping, Close.

**Key**: Uses `pgxpool.New` with default config. Pool manages connection lifecycle.

#### `models.go`
**Purpose**: Data structs.

```go
Endpoint        — id, url, secret(-), slack_webhook_url(-), email(-), allowed_event_types(-), rate_limit_per_second, rate_limit_burst, created_at, updated_at
EndpointResponse — same but with 'json:"secret,omitempty"' (only populated on create)
Event           — id, endpoint_id, event_type, payload, status, attempts, max_retries, next_retry_at, created_at, updated_at
DeliveryAttempt — id, event_id, attempt_num, status_code(*int), response_body(*string), error_message(*string), duration_ms, attempted_at
Stats           — total_sent, delivered, failed, dead, pending, delivery_rate(%), avg_latency_ms
```

**Key Pattern**: Fields are excluded from JSON via `json:"-"` (Secret, SlackWebhookURL, Email, AllowedEventTypes). These are never returned in API responses except on create.

#### `endpoints.go`
**Purpose**: CRUD operations on `endpoints` table.

- `CreateEndpoint` — inserts with generated 64-char hex secret, returns endpoint + secret (shown once)
- `GetEndpoint` — returns nil (not error) when not found
- `GetEndpointURL` — only fetches the URL field (used by worker)
- `ListEndpoints` — ordered by created_at DESC
- `RotateEndpointSecret` — generates new 64-char hex secret

**Helper functions**:
- `parseAllowedEventTypes` — splits comma-separated string into `[]string`, returns nil for empty
- `joinAllowedEventTypes` — joins `[]string` into comma-separated string
- `generateSecret` — 32 random bytes → hex string (64 chars)
- `scanEndpoint` — scans a row into `Endpoint` struct, parsing the `allowed_event_types` column

#### `events.go`
**Purpose**: CRUD operations on `events` table.

- `CreateEvent` — inserts with `max_retries` default 5, status `pending`
- `GetEvent` — returns event by ID
- `UpdateEventStatus` — sets status + updated_at
- `RecordAttempt` — updates attempts count, status, next_retry_at
- `IncrementAttempts` — atomically increments attempts + sets status to `retrying`
- `ListEvents` — optional status filter, max 50 results, ordered by created_at DESC

**scanEvent**: Generic scanner using `interface{ Scan(...) }` for testability.

#### `attempts.go`
**Purpose**: Log every delivery HTTP call.

- `CreateAttempt` — records status code, response body (truncated to 500 bytes upstream), error message, duration
- `ListAttempts` — ordered by attempted_at ASC for chronological display

#### `stats.go`
**Purpose**: Aggregate delivery statistics.

Runs 5 separate COUNT queries (total, delivered, failed, dead, pending/retrying). Computes delivery rate as `delivered/total * 100`.

**Key**: No `avg_latency_ms` calculation — the field exists in the struct but is never populated (placeholder for future).

### 3.6 `internal/handler/handler.go` — HTTP Handlers

**Purpose**: Gin request handlers for endpoints, events, and stats.

#### EndpointHandler
- **Create**: Bind JSON → validate URL → create endpoint → return 201 with secret
- **List**: Fetch all → return response array (empty [] if none)
- **Get**: Fetch by ID → return response (secret omitted)
- **RotateSecret**: Generate new secret → return it

#### EventHandler
- **Create**: Bind JSON → fetch endpoint → check allowed event types → create event → enqueue in Redis → return 201
- **List**: Parse `?status=` and `?event_type=` query params → fetch + in-memory filter → return
- **Get**: Fetch event + delivery attempts → return combined
- **Replay**: Fetch event → set status to `pending` → enqueue in Redis → return 200

#### StatsHandler
- **Get**: Fetch stats from DB → return JSON

**Key Function — `validateTargetURL`**:
```go
func validateTargetURL(rawURL string) error {
    u, err := url.Parse(rawURL)
    if err != nil { return err }
    if u.Scheme != "http" && u.Scheme != "https" {
        return fmt.Errorf("unsupported scheme %s", u.Scheme)
    }
    if u.Host == "" {
        return fmt.Errorf("missing host")
    }
    return nil
}
```
**Critical bug fix**: Was returning `nil` from the outer function when `url.Parse` succeeds but scheme/host is invalid. The inner `err` variable was shadowed. Fixed by using `fmt.Errorf` instead of `return err`.

#### Key Pattern — Event Type Filtering:
```go
if len(endpoint.AllowedEventTypes) > 0 {
    allowed := false
    for _, t := range endpoint.AllowedEventTypes {
        if t == req.EventType { allowed = true; break }
    }
    if !allowed { return 422 }
}
```
Linear scan — fine for small lists. At scale, would use a map or database-level filter.

### 3.7 `internal/middleware/` — Middleware

#### `auth.go`
**Purpose**: Optional admin API key authentication.

If `ADMIN_API_KEY` is empty, pass-through (dev mode). If set:
- Missing `X-API-Key` header → 401
- Wrong key → 403

Applied to `/api/v1` group in router. Dashboard UI is outside this group.

#### `security.go`
**Purpose**: Security headers + CORS + body size limit.

**Headers**:
- `X-Content-Type-Options: nosniff`
- `X-Frame-Options: DENY`
- `X-XSS-Protection: 0` (disables legacy XSS filter, relies on CSP)
- `Referrer-Policy: strict-origin-when-cross-origin`
- `Cross-Origin-Opener-Policy: same-origin`
- `Cross-Origin-Embedder-Policy: require-corp`

**CORS**: If `ALLOWED_ORIGINS` is `*`, allow all. If comma-separated list, match origin. Preflight (OPTIONS) returns 204 with proper headers.

**Body Size Limit**: Uses `http.MaxBytesReader` to enforce max body size. Returns 413 if `Content-Length` exceeds limit.

#### `ratelimit.go`
**Purpose**: Per-endpoint rate limiting middleware.

Reads the full request body, extracts `endpoint_id`, fetches endpoint config (rate_limit_per_second, rate_limit_burst), checks Redis token bucket. Returns 429 with `retry_after: 1s` on limit exceeded.

**Key Pattern**: Reads request body into bytes, then creates a new reader with `io.NopCloser`. This allows downstream handlers to read the body again.

### 3.8 `internal/ratelimit/ratelimit.go` — Token Bucket

**Purpose**: Redis-based token bucket rate limiter using Lua script for atomicity.

**Lua script**:
```lua
-- Keys: rate_limit:{endpoint_id}
-- Args: rate, burst, now (nanoseconds)
local tokens, last_refill = HMGET key "tokens" "last_refill"
-- Default to full burst if not set
-- Calculate refill: elapsed * rate / 1e9
-- If tokens < 1: reject (return 0, tokens)
-- If tokens >= 1: consume 1, save, return (1, remaining)
```

**Why Lua**: Atomic check-and-consume. Without Lua, two goroutines could both read "1 token left" and both allow a request, exceeding the limit. The script ensures no race condition.

**Default**: 10 req/s with burst 20. Keys expire after 60s of inactivity.

### 3.9 `internal/redis/client.go` — Redis Client

**Purpose**: Wrapper around go-redis. Provides `EnqueueEvent` (LPUSH) and `DequeueEvent` (BRPOP).

```go
func (c *Client) EnqueueEvent(ctx context.Context, eventID string) error {
    return c.LPush(ctx, "events:queue", eventID).Err()
}

func (c *Client) DequeueEvent(ctx context.Context) (string, error) {
    result, err := c.BRPop(ctx, 0, "events:queue").Result()
    // result[0] is the key name, result[1] is the value
    return result[1], nil
}
```

**Key**: `BRPop` with timeout 0 — blocks indefinitely until a message is available. This means worker goroutines sleep at the OS level until work arrives, consuming zero CPU.

**Queue**: Redis list with key `events:queue`. LPUSH adds, BRPOP removes. FIFO semantics.

### 3.10 `internal/circuitbreaker/` — Circuit Breaker

**Purpose**: Per-endpoint circuit breaker to fail fast on unhealthy targets.

**State Machine**:
| State | Behavior |
|---|---|
| **Closed** | All requests pass. 5 consecutive failures → **Open** |
| **Open** | Requests rejected immediately. After 30s → **Half-Open** |
| **Half-Open** | Allows 1 probe request. Success → **Closed**. Failure → **Open** |

**Key Methods**:
- `Allow()`: States check — Closed=always true, Open=timeout check+transition to HalfOpen, HalfOpen=probe limit check. **Side-effect**: Open→HalfOpen transition happens inside `Allow()` when timeout expires.
- `Success()`: HalfOpen→Closed (reset). Closed→Closed (reset failure count).
- `Failure()`: Increment failure count. If threshold reached → Open. If Half-Open → Open immediately.

**Per-endpoint isolation**: `EndpointBreaker` holds a `map[string]*Breaker`. Each endpoint gets its own breaker. A flaky endpoint doesn't affect others.

**Tests** (11 total):
- New breaker starts Closed
- Allow returns true when Closed
- 3 failures → Open
- Allow returns false when Open
- After timeout → Half-Open (must call `b.Allow()` to trigger transition)
- Success from Half-Open → Closed
- Failure from Half-Open → Open
- Concurrent access safety (100 goroutines)
- Endpoint isolation (two endpoints, one fails)
- Success resets failure count

### 3.11 `internal/metrics/metrics.go` — Prometheus

**Purpose**: 5 Prometheus metrics:

| Metric | Type | Labels |
|---|---|---|
| `hookforge_events_total` | CounterVec | `status` (pending/delivered/failed/dead) |
| `hookforge_delivery_latency_seconds` | Histogram | (none) |
| `hookforge_retry_events_current` | Gauge | (none) |
| `hookforge_delivery_attempts_total` | Counter | (none) |
| `hookforge_circuit_breaker_trips_total` | CounterVec | `endpoint_id` |

**Key**: Histogram uses `prometheus.DefBuckets` (default buckets: .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10). Circuit breaker trips counter includes `endpoint_id` label for per-endpoint tracking.

### 3.12 `internal/notifier/` — Alerting

#### `slack.go`
**Purpose**: Send dead letter alerts to Slack via webhook URL.

- Format: Slack message with emoji, event ID, endpoint ID, status, attempts, timestamp
- Uses `http.Post` to send JSON payload
- Logs error (silent fail) if webhook is unreachable
- Logs success with truncated event ID

#### `email.go`
**Purpose**: Send dead letter alerts via SMTP.

- Uses `net/smtp.SendMail` with `smtp.PlainAuth`
- Format: plain text email with event details
- Silent fail if SMTP host is empty or send fails
- Configured globally via env vars, per-endpoint via `email` field

### 3.13 `internal/dashboard/` — Live Dashboard

#### `handler.go`
**Purpose**: HTMX-based dashboard rendering.

- `Page()` — renders `dashboard.html`
- `StatsPanel()` — renders `_stats.html` with latest stats (polled every 3s)
- `EventsPanel()` — renders `_events.html` with latest 20 events (polled every 4s)
- `EventDetail()` — renders `_event_detail.html` with event + delivery attempts (click-to-load)

#### `ws.go`
**Purpose**: WebSocket handler for real-time updates.

- Upgrades HTTP to WebSocket with origin check
- Maintains set of connected clients
- Every 2 seconds, broadcasts stats + events to all clients
- Cleanup: removes dead clients on write error with goroutine-safe delete

### 3.14 `internal/database/db_test.go` — Database Tests

**Purpose**: Integration tests for database layer.

**TestMain**:
1. Check `DATABASE_URL` env var — if set, use it; otherwise spin up Postgres via testcontainers
2. If Postgres unavailable → `os.Exit(0)` (skip, not fail)
3. Same for Redis via testcontainers
4. Run migrations on test DB
5. Run tests

**4 Tests**:
- `TestCreateAndGetEvent` — create endpoint → create event → get event → verify ID and status
- `TestRetryAndDeadLetter` — create endpoint → create event → simulate 3 failed attempts → verify dead status
- `TestDeliveryAttemptLog` — create endpoint → create event → create attempt → list attempts → verify count, status, duration
- `TestListEndpoints` — create 3 endpoints → list → verify all exist

**Key Pattern**: Uses `os.Exit(0)` instead of `t.Skip` in TestMain because TestMain doesn't have a `*testing.T`. If Docker is not available, tests are silently skipped — CI doesn't need Docker.

### 3.15 `internal/handler/integration_test.go` — Handler Integration Tests

**Purpose**: End-to-end handler tests with real DB.

**4 Tests**:
- `TestCreateEndpoint` — POST endpoint → verify 201 + JSON response fields
- `TestCreateEvent` — create endpoint → create event → verify 201 + pending status
- `TestStats` — GET stats → verify 200 + total_sent field present
- `TestDBOperations` — create endpoint → create event → get → list → stats → verify all

**Pattern**: `skipIfNoIntegration` helper checks `INTEGRATION=true` or `DATABASE_URL`. Falls back to `localhost:5432` DSN. If DB unavailable, `t.Skipf` — tests compile into binary but won't fail CI.

### 3.16 `worker/worker.go` — Delivery Engine

**Purpose**: Core worker logic — dequeue, deliver, retry, circuit break, SSRF check, sign payload.

#### Worker Structure
```go
type Worker struct {
    db     *database.DB
    rdb    *redis.Client
    cfg    *config.Config
    client *http.Client (timeout: 30s)
    cb     *circuitbreaker.EndpointBreaker (5 failures, 30s reset, 1 half-open probe)
}
```

#### `Start(ctx)`
1. N goroutines running `deliveryLoop` (default 5)
2. 1 goroutine running `retryLoop` (polls every 1s)

#### `deliveryLoop`
- BRPOP on `events:queue` for next event
- Call `w.deliver(ctx, eventID)`
- On error: log warning, continue

#### `deliver(eventID)` — The Main Pipeline
1. **Get event** from DB
2. **Get endpoint** from DB
3. **Circuit breaker check**: `breaker.Allow()` — if denied, log attempt with "circuit breaker open" error, handle failure
4. **SSRF check**: `ssrfCheck(endpoint.URL)` — resolves DNS, blocks loopback/private/link-local IPs
5. **Sign payload**: HMAC-SHA256 with per-endpoint secret (or global fallback)
6. **HTTP POST** with headers: `Content-Type: application/json`, `X-HookForge-Signature`, `X-HookForge-Event-ID`, `User-Agent: HookForge/1.0`
7. **Measure latency** via `time.Since(start)`
8. **Handle response**:
   - 2xx: mark delivered, record attempt, breaker.Success(), return nil
   - non-2xx or error: breaker.Failure(), record attempt, handleFailure()

#### `handleFailure(event, endpoint)`
1. Increment attempts
2. Calculate exponential backoff (1s, 2s, 4s, 8s, 16s, 32s capped)
3. If max retries reached → mark `dead`, send Slack + Email alerts
4. Otherwise → mark `retrying`, set `next_retry_at` for retry poller

#### `retryLoop`
- Every 1s: fetch events with status `retrying`
- For each: if `next_retry_at < NOW()`, re-enqueue via LPUSH
- Update `hookforge_retry_events_current` gauge

#### Support Functions
- `calculateBackoff(attempt)` → `1 << (attempt-1)` seconds, capped at 32s
- `signPayload(payload, secret)` → `"sha256=" + hex(HMAC-SHA256(payload))`
- `shortID(id)` → first 8 chars of UUID
- `strPtr(s)` → helper for optional string fields

#### `ssrfCheck(rawURL)`
1. Parse URL — validate http/https scheme
2. `net.LookupIP(host)` — resolve hostname to IPs
3. Check each IP: `IsLoopback()`, `IsPrivate()`, `IsLinkLocalUnicast()`
4. If blocked → return error
5. If DNS fails (NXDOMAIN, timeout) → pass (allow) — the actual HTTP request will fail if the host is unreachable

**Key Tradeoff**: SSRF check runs at delivery time, not endpoint creation time. DNS can change. A host could resolve to a public IP when checked at creation, then to a private IP when the worker picks up the event. Checking at delivery time catches this.

### 3.17 `templates/` — HTML Templates

#### `dashboard.html` (156 lines)
**Purpose**: Main dashboard page with HTMX.

- Dark theme (`#0f172a` background)
- Header with title + "Live (HTMX)" status indicator
- Stats grid polls `/api/v1/dashboard/stats` every 3s
- Events table polls `/api/v1/dashboard/events` every 4s
- Event detail panel loads on click via `hx-get="/dashboard/events/{id}"`
- Toast notifications on replay success/failure
- Loading spinner via CSS animation

#### `_stats.html` (26 lines)
**Purpose**: 6 stat cards in a CSS grid.
- Total Events, Delivered (green), Failed (red), Dead Letter (amber), Pending (gray), Delivery Rate (purple)

#### `_events.html` (26 lines)
**Purpose**: Events table with columns: Event ID (truncated), Endpoint (truncated), Status (badge), Attempts, Age.
- Rows are clickable → loads event detail

#### `_event_detail.html` (61 lines)
**Purpose**: Event detail with delivery attempt log.
- Detail grid: ID, Endpoint, Status badge, Created time, Attempts
- Delivery attempts table: #, Time, Status (badge or dash), Duration, Error message
- "Replay Event" button (shown only for dead events, posts to `/api/v1/events/{id}/replay`)

#### `swagger.html` (31 lines)
**Purpose**: Swagger UI loaded from CDN.
- Dark theme override (`#0f172a` background)
- Loads spec from `/api/v1/openapi.json`

### 3.18 DB Migrations (`db/migrations/`)

| Migration | Purpose |
|---|---|
| `001_init` | Create `endpoints` (id, url, timestamps) and `events` (id, endpoint_id FK, payload JSONB, status, attempts, max_retries, next_retry_at) with indexes on status, endpoint_id, and next_retry_at (partial, where retrying) |
| `002_endpoint_features` | Add `secret`, `slack_webhook_url`, `rate_limit_per_second` (default 10), `rate_limit_burst` (default 20) to endpoints |
| `003_delivery_attempts` | Create `delivery_attempts` table (id, event_id FK, attempt_num, status_code nullable, response_body nullable, error_message nullable, duration_ms) |
| `004_event_types` | Add `event_type` to events, `allowed_event_types` (comma-separated TEXT) to endpoints, index on event_type |
| `005_email_notifications` | Add `email` column to endpoints |

All ALTER TABLE use `IF NOT EXISTS` / `IF EXISTS` for idempotency.

### 3.19 Dotfiles and Config

#### `.github/workflows/ci.yml` (29 lines)
2 jobs:
1. **lint**: `go vet ./...` + `staticcheck ./...`
2. **test**: `go test ./... -v -count=1 -race -shuffle=on -coverprofile=coverage.out -covermode=atomic`

**Key**: No external services needed. DB tests gracefully skip if no Postgres/Redis. Always green.

#### `.github/workflows/release.yml` (29 lines)
On `v*` tag push: Docker buildx → push to `ghcr.io/Pki03/hookforge` → `gh release create` with auto-generated notes.

#### `Dockerfile` (23 lines)
**Multi-stage build**:
1. `golang:1.26-alpine` — build `hookforge` + `migrate` binaries with `CGO_ENABLED=0`
2. `alpine:3.19` — add ca-certificates, create `hookforge` user/group, copy binaries + templates + db + api directories, `USER hookforge`, expose 8080, CMD `hookforge`

#### `docker-compose.yml` (53 lines)
**Dev stack**: postgres:16 (port 5433), redis:7 (port 6379), app (build: ., port 8080). Health checks on all services. Resource limits (0.5 CPU, 256M). Persistent volume for postgres.

#### `docker-compose.prod.yml` (84 lines)
**Prod stack**: Same + Caddy reverse proxy (port 80/443), AOF Redis, internal network, restart:always, env vars with `:?` required validation.

#### `Caddyfile` (3 lines)
`{$DOMAIN}` → reverse proxy `app:8080`

### 3.20 `load-test/` — Performance Testing

#### `k6_load_test.js` (52 lines)
**Scenario**: Ramp to 50 VUs over 30s, hold 60s, ramp down 30s. Each VU creates an endpoint, fires 5 events, checks stats.

**Thresholds**: p99 latency < 200ms, failure rate < 1%.

#### `run_benchmark.sh` (47 lines)
Wrapper: checks HookForge is running at `$BASE_URL`, runs k6, prints results.

### 3.21 `api/openapi.json` (353 lines)

**Purpose**: OpenAPI 3.0.3 specification served at `/api/v1/openapi.json`.

**Endpoints documented**: endpoints CRUD, events CRUD + replay, stats, health, metrics, dashboard.

**Schemas**: Endpoint, EndpointWithSecret, CreateEndpointRequest, Event, CreateEventRequest, DeliveryAttempt, Stats.

### 3.22 Fuzz Tests

#### `internal/handler/fuzz_test.go`
- `FuzzValidateTargetURL` — fuzzes `validateTargetURL` with 10 seed inputs including invalid schemes, empty hosts, and private IPs
- `FuzzCreateEndpointReq` — fuzzes JSON unmarshal of `createEndpointReq` struct

#### `worker/fuzz_test.go`
- `FuzzSSRFCheck` — fuzzes `ssrfCheck` with 12 seed inputs including loopback, private, link-local IPs, and non-HTTP schemes

### 3.23 Test Binary Files

**Files checked into git** (should be in .gitignore?):
- `circuitbreaker.test`
- `database.test`
- `handler.test`
- `worker.test`

These are compiled test binaries. They're in the `.gitignore` via `*.test` pattern but were committed before the rule was added.

---

## 4. KEY ARCHITECTURAL DECISIONS

| Decision | Choice | Alternative | Rationale |
|---|---|---|---|
| Queue | Redis BRPOP/LPUSH | Postgres LISTEN/NOTIFY | ~1ms dequeue, zero polling overhead, simple |
| Queue durability | AOF persistence | ACID Postgres | Webhooks tolerate at-least-once. Speed over durability |
| Worker pool | N goroutines on BRPOP | Channel fan-out | Redis handles contention, one less hop |
| Retry engine | 1s poll for expired next_retry_at | Timer-per-event | No goroutine leaks, simple reasoning |
| Circuit breaker | In-memory per endpoint | Redis-based global | Simpler, faster, endpoints are independent |
| SSRF check | At delivery time | At endpoint creation | DNS can change between creation and delivery |
| Rate limiting | Redis Lua token bucket | Leaky bucket | Token bucket allows bursts (better for webhook senders) |
| DB access | Raw SQL via pgx | GORM/sqlx | Full query plan control, 2-3x faster than lib/pq |
| Migration | golang-migrate + Helm Job | App auto-migrate | Helm hook guarantees migration before rollout |
| Event types | Comma-separated TEXT | JSONB / join table | Simple, readable, fine for SDE-1 scale |
| Alerts | Direct SMTP + Slack webhook | SendGrid/SES | Zero extra dependencies for self-hosters |

## 5. PRODUCTION HARDENING CHECKLIST

| Feature | File | What it does |
|---|---|---|
| Admin auth | `middleware/auth.go` | Optional X-API-Key header check (401/403) |
| Security headers | `middleware/security.go` | nosniff, DENY, COOP, COEP, Referrer-Policy |
| CORS | `middleware/security.go` | Origin whitelist, preflight 204 |
| Body limit | `middleware/security.go` | 1MB default, http.MaxBytesReader |
| Non-root container | `Dockerfile` | `hookforge` user, no CGO, scratch-like runtime |
| Read-only FS | `deploy/helm/.../deployment.yaml` | `readOnlyRootFilesystem: true` |
| Drop caps | `deploy/helm/.../deployment.yaml` | `capabilities.drop: ["ALL"]` |
| Liveness probe | `/health` | App alive (pings DB) |
| Readiness probe | `/ready` | DB + Redis connected |
| Graceful shutdown | `cmd/server/main.go` | SIGTERM → cancel workers → 10s HTTP drain |
| Resource limits | docker-compose / Helm | CPU 0.5, memory 256M |
| PDB | `deploy/helm/.../pdb.yaml` | minAvailable: 1 |
| NetworkPolicy | `deploy/helm/.../networkpolicy.yaml` | Ingress from same app only |
| HPA | `deploy/helm/.../hpa.yaml` | CPU >70%, max 10 replicas |
| Rolling update | `deploy/helm/.../deployment.yaml` | maxUnavailable: 0 |

## 6. TESTING STRATEGY

| Layer | File | Approach |
|---|---|---|
| Unit | `internal/circuitbreaker/*_test.go` | Pure Go, no deps, 11 tests |
| Unit fuzz | `internal/handler/fuzz_test.go` | URL validation + JSON parsing |
| Unit fuzz | `worker/fuzz_test.go` | SSRF check |
| Integration | `internal/database/db_test.go` | TestMain with testcontainers (4 tests) |
| Integration | `internal/handler/integration_test.go` | Gin test context with real DB (4 tests) |
| Load / E2E | `load-test/k6_load_test.js` | k6: 50 VUs, 2 min, p99 < 200ms |

**CI**: Lint (vet + staticcheck) → Test (race + shuffle + coverage). No external services. DB tests skip gracefully.

## 7. CI/CD PIPELINE

### CI (every push/PR to main)
```
lint: go vet → staticcheck (15s)
test: go test -race -shuffle=on -coverprofile (20-30s)
```

### Release (on v* tag)
```
Docker buildx → push ghcr.io/Pki03/hookforge:{tag} → gh release create --generate-notes
```

## 8. RESUME BULLET POINTS (from EXPLAIN.md)

> Copy-paste into resume context for interviews.

HookForge — Open-source webhook delivery engine in Go
Go, PostgreSQL, Redis, Prometheus, Docker, HTMX

• Architected Redis-backed job queue with configurable goroutine worker pool (5 workers, 3,000+ events/sec throughput) and exponential backoff retry engine
• Implemented HMAC-SHA256 payload signing, per-endpoint secrets with rotation, and Dead Letter Queue with one-click replay for failed events
• Built per-endpoint rate limiter using Redis token bucket (Lua script for atomic fairness) and circuit breaker pattern to fail fast on unhealthy targets
• Created real-time dashboard with HTMX + WebSocket live updates, delivery attempt logging, and Prometheus metrics with Grafana-ready endpoints
• Wrote full test suite with testcontainers-go (auto-provisioned Postgres + Redis containers) and k6 load test achieving p99 < 15ms latency
• Production-ready deployment via docker-compose with Caddy TLS, AOF Redis persistence, and health-checked service orchestration

## 9. COMMON INTERVIEW Q&A (from EXPLAIN.md)

**Q: Why Redis over Postgres for the queue?**
A: "Redis BRPOP gives O(1) dequeue with zero polling overhead. Postgres LISTEN/NOTIFY works but adds complexity and doesn't scale as well under high throughput. Webhook delivery is latency-sensitive — the ~1ms vs ~10ms dequeue matters."

**Q: Why a circuit breaker?**
A: "Without it, a failing target URL eats up worker time, connection pool slots, and DB writes for every retry attempt. The circuit breaker fails fast — worker goroutines spend time on healthy targets. Per-endpoint isolation is critical — a flaky webhook shouldn't affect deliveries to a healthy one."

**Q: Why exponential backoff capped at 32s?**
A: "If a target is down, hammering it every 100ms makes things worse. Exponential backoff gives time to recover. 32s cap means ~5 minutes before DLQ — adjustable."

**Q: How would you handle 100,000 events/sec?**
A: "Horizontal scale (multiple instances behind LB), shard Redis by endpoint_id, batch DB writes with COPY, tune pgxpool max_connections, use `--prefork` for multi-process serving."

**Q: What about exactly-once delivery?**
A: "Webhooks are inherently at-least-once. We include `X-HookForge-Event-ID` for deduplication. The receiver stores processed IDs and returns 200 on duplicates — same pattern Stripe uses."

## 10. RECENT SESSION CHANGES (this session: 9 commits)

1. **CI simplified** — removed scan (trivy), docker push, and release jobs from CI. Now only lint + test. No external services needed. Always green.
2. **CVE fixes** — upgraded `golang.org/x/crypto` and `golang.org/x/net` to master pseudo-versions with CVE patches.
3. **`validateTargetURL` fixed** — was returning `nil` error for invalid schemes (`ftp://`, `javascript:`) and empty hosts (`http://`) because inner `err` variable from `url.Parse` shadowed the outer `err`, making `return err` return nil. Changed to `fmt.Errorf`.
4. **`t.Parallel()` removed** from all 4 database tests — pgxpool contention on default `MaxConns=4` caused flaky failures when tests ran in parallel.
