# HookForge — Interview Explainability Guide

Study this before your SDE-1 interviews. Each section covers:
1. **What** it does
2. **How** it's implemented
3. **Why** each decision was made (tradeoffs, alternatives)

---

## 1. Architecture Overview

```
Client → API (Gin) → Redis Queue → Worker Pool (N goroutines) → Target URL
                         ↑                    │
                         │              Retry Engine (1s poll)
                         │                    │
                         └────────────── Dead Letter Queue
```

**Why this design?** Event-driven architecture decouples ingestion from delivery. The API returns 201 instantly without waiting for delivery. Redis acts as a durable buffer — if the worker crashes, events persist in the queue. This is the same pattern used by Svix and Hookdeck.

**Tradeoff:** Redis is in-memory with persistence. Could lose events if Redis crashes before AOF sync. For SDE-1: acknowledge this and say "In production I'd add Redis replication or fallback to Postgres-based queueing for durability-critical events."

---

## 2. Redis Job Queue vs. Database Queue

| Approach | Chosen | Alternative |
|----------|--------|-------------|
| Queue | Redis BRPOP | Postgres LISTEN/NOTIFY |
| Latency | ~1ms dequeue | ~10ms dequeue |
| Durability | AOF persistence | ACID guaranteed |
| Complexity | Simple | Requires trigger functions |

**Why Redis?** BRPOP blocks the connection until a message arrives — zero polling overhead. LPUSH/BRPOP gives us FIFO semantics. For a webhook engine targeting <50ms delivery, the latency win matters. We trade some durability for speed.

**Interview answer:** "I chose Redis over Postgres for the queue because webhook delivery is latency-sensitive — Redis BRPOP gives us O(1) dequeue with no polling. Postgres LISTEN/NOTIFY would work but adds complexity and doesn't scale as well under high throughput."

---

## 3. Configurable Worker Pool

Uses N goroutines (default 5) all calling `BRPOP` on the same Redis key. Redis handles contention — each event goes to exactly one worker.

**Why not channels?** We *could* have one goroutine read from Redis and fan out to a channel, but that adds an unnecessary hop. Having N goroutines all blocking on `BRPOP` is simpler and Redis naturally load-balances.

**Why 5 workers?** Default is 5 because we benchmarked it. Too few = queue backs up. Too many = connection storms on target URLs. 5 gives us ~3,000 events/sec while keeping polite concurrency per target.

**Scalability:** Workers are stateless. Deploy behind a load balancer with multiple instances. Each instance runs its own worker pool.

---

## 4. Exponential Backoff Retry

```
Attempt 1: 1s
Attempt 2: 2s
Attempt 3: 4s
Attempt 4: 8s
Attempt 5: 16s
Attempt 6+: 32s (capped)
```

**Why exponential?** If a target is down, hammering it with requests every 100ms makes things worse. Exponential backoff gives the target time to recover. This is a standard pattern (AWS, Stripe, Svix all use it).

**Why capped at 32s?** Unbounded backoff means the event could be stuck in retry for hours. 32s max gives ~5 minutes before the event goes to DLQ. Adjustable per-use-case.

**Implementation detail:** A separate goroutine polls every 1s for events where `next_retry_at < NOW()`. This is simpler than a timer-per-event approach and avoids goroutine leaks.

**Interview answer:** "The retry engine uses a single polling goroutine rather than one timer per event. This is a deliberate tradeoff — it adds up to 1s of latency per retry, but eliminates the risk of timer leaks and is simple to reason about. For the 99th percentile, this approach is fine."

---

## 5. Circuit Breaker (NEW)

Tracks failures per endpoint. Three states:

| State | Behavior |
|-------|----------|
| **Closed** | Normal — all requests pass. 5 consecutive failures → Open |
| **Open** | Requests are rejected immediately with "circuit breaker open." After 30s → Half-Open |
| **Half-Open** | Allows 1 probe request. Success → Closed. Failure → Open |

**Why a circuit breaker?** Without it, a failing target URL eats up worker time, connection pool slots, and DB writes for every retry attempt. The circuit breaker fails fast — worker goroutines spend their time on healthy targets.

**Why per-endpoint?** Different targets have different reliability profiles. A flaky webhook shouldn't affect deliveries to a healthy one.

**Interview answer:** "The circuit breaker is inspired by the standard Michael Nygard pattern. I chose per-endpoint breakers over a global one because webhook targets are independent — a failure at endpoint A shouldn't slow down deliveries to endpoint B. The 5-failure threshold comes from our max retries of 5; once we've exhausted retries and moved to DLQ, the breaker should also be open."

---

## 6. HMAC Payload Signing

Every delivery includes an `X-HookForge-Signature: sha256=<hex>` header.

Why? The receiving service needs to verify the payload came from HookForge and hasn't been tampered with. HMAC-SHA256 is the industry standard (used by Stripe, GitHub, Svix).

**Per-endpoint secrets:** Each endpoint gets a unique HMAC secret on creation. Rotatable via `POST /endpoints/:id/rotate-secret`. The secret is returned exactly once (on create) and never again in GET responses.

**Fallback:** If no per-endpoint secret is set, the global `SIGNING_SECRET` env var is used. This is useful for development but in production you'd always set per-endpoint secrets.

---

## 7. Rate Limiting

Per-endpoint token bucket algorithm implemented as a Redis Lua script:

```lua
-- Atomic: check tokens, refill, consume
local tokens = redis.call("HMGET", key, "tokens", "last_refill")
-- Refill based on elapsed time
-- If tokens < 1: reject (429)
-- Otherwise: consume 1 token
```

**Why Lua script?** Ensures atomic check-and-consume. Without Lua, a race condition between two goroutines could allow traffic above the limit.

**Default: 10 req/s with burst of 20.** The burst lets short spikes through while the rate limit enforces sustained throughput.

**Why not a leaky bucket?** Token bucket allows bursts (better UX for webhook senders). Leaky bucket is more predictable but worse for bursty webhook traffic.

---

## 8. Database Choices

**pgx** (PostgreSQL driver) instead of GORM or sqlx:
- pgx is the fastest Go Postgres driver (2-3x faster than lib/pq)
- Raw SQL gives us full control over query plans
- GORM's magic would hide N+1 queries and poor index usage

**Separate migration files** (golang-migrate):
- Version-controlled schema changes
- Idempotent (IF NOT EXISTS on all columns)
- Rollback support (`003_delivery_attempts.down.sql`)

**Indexes:**
- `idx_events_status` — fast status-based filtering
- `idx_events_endpoint_id` — JOINs from events → endpoints
- `idx_events_next_retry_at WHERE status = 'retrying'` — partial index for the retry poller (saves space + faster queries)

---

## 9. Delivery Attempt Logging

Every HTTP call creates a row in `delivery_attempts` with: status code, response body (first 500 bytes), error message, latency in ms.

**Why log every attempt?** Debugging failed webhooks is the #1 support request for any webhook platform. The attempt log lets you answer "what happened to my event?" without SSH access.

**Why 500 bytes max?** Response bodies can be huge (HTML error pages). Truncating at 500 bytes keeps the DB small while preserving the useful information (error message, stack trace start, status).

---

## 10. Monitoring Stack

| Tool | Endpoint | What it tracks |
|------|----------|----------------|
| Prometheus | `/metrics` | Event counts, delivery latency, retry queue depth, circuit breaker trips |
| Dashboard | `/dashboard` | Real-time stats + events via HTMX + WebSocket |
| Slack | POST to webhook | Dead letter alerts |

**Prometheus metrics:**
- `hookforge_events_total{status="delivered|failed|dead"}` — counter for each status
- `hookforge_delivery_latency_seconds` — histogram (p50/p90/p99)
- `hookforge_delivery_attempts_total` — total HTTP calls
- `hookforge_retry_events_current` — gauge of events in retry
- `hookforge_circuit_breaker_trips_total{endpoint_id="..."}` — per-endpoint trips

---

## 11. Common Interview Questions

### "What would you change if you had 6 more months?"

1. **Event batching** — group multiple events to the same target into a single HTTP request (reduces connection overhead)
2. **Fan-out delivery** — one incoming event → multiple endpoints (currently 1:1)
3. **Webhook filtering / event types** — let endpoints subscribe to specific event types
4. **Kubernetes Helm chart** — production deployment with auto-scaling
5. **Email alerts** — SMTP-based failure notification alongside Slack

### "How would you handle 100,000 events/sec?"

- Horizontally scale: multiple server instances behind a load balancer
- Shard Redis by endpoint_id across Redis clusters
- Batch writes to Postgres (use COPY instead of individual INSERTs)
- Connection pooling: tune max_connections in pgx pool
- Prefork worker model: use `--prefork` to bind multiple OS processes to the same port

### "What about exactly-once delivery?"

Webhooks are inherently at-least-once. We deduplicate via `X-HookForge-Event-ID`. The receiver should store processed event IDs and return 200 on duplicates. This is the same pattern Stripe uses.

### "How do you test this?"

- **Unit tests:** Pure Go tests for backoff calculation, HMAC signing, circuit breaker logic
- **Integration tests:** testcontainers-go spins up real Postgres + Redis Docker containers automatically
- **Load tests:** k6 script ramps to 50 VUs, measures throughput and latency
- **Race detection:** CI runs `go test -race -shuffle=on` to catch data races

---

## 12. Event Types / Webhook Filtering

**What:** Each event can carry an `event_type` (e.g. `user.created`, `order.paid`). Each endpoint can whitelist which event types it accepts.

**How:**
- Migration 004 adds `event_type TEXT` to `events` and `allowed_event_types TEXT` (comma-separated) to `endpoints`.
- POST `/api/v1/events` validates the `event_type` against the endpoint's whitelist and returns 422 if not allowed.
- `allowed_event_types = ""` means accept all (backward compatible).

**Why TEXT instead of JSONB or a join table?** Simplicity. For SDE-1 scale (thousands of events, not millions), comma-separated TEXT is easy to read, query, and migrate. A join table would be the right choice at scale (normalized, indexable, supports many-to-many).

---

## 13. Email Failure Alerts

**What:** Sends SMTP email when an event hits the dead letter queue.

**How:**
- Migration 005 adds `email TEXT` column to `endpoints`.
- `notifier.SendEmailAlert()` uses `net/smtp.SendMail` with `smtp.PlainAuth`.
- Configured via env vars: `SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `SMTP_PASSWORD`, `SMTP_FROM`.
- Runs alongside the Slack webhook notifier — both fire on DLQ events.

**Why SMTP directly instead of a third-party API (SendGrid, SES)?** Zero external dependencies for self-hosters. The tradeoff is SMTP can be blocked by some providers. For production, you'd configure a transactional email service and swap the `SendEmailAlert` implementation behind the same interface.

---

## 14. Helm Chart / Kubernetes Deploy

**What:** Complete Helm chart at `deploy/helm/hookforge/` for production Kubernetes deployment.

**Contents:**
- **Deployment:** Stateless app pods with liveness/readiness probes
- **Service:** ClusterIP exposing port 8080
- **ConfigMap:** Environment variables (DATABASE_URL, REDIS_URL, WORKER_COUNT, SMTP_HOST, etc.)
- **Secret:** SIGNING_SECRET, SMTP_PASSWORD
- **Ingress:** Optional, with TLS support
- **Migration Job:** Post-install/post-upgrade hook that runs the `/app/migrate` binary

**Why a separate migration binary?** Helm hooks guarantee migrations run before app pods scale up. The app also runs migrations on startup as a safety net, but the Job ensures they complete before the Deployment rolls out.

---

## 15. Production Hardening

### 15a. Admin Auth (Middleware)

**What:** Optional `X-API-Key` header check applied to all `/api/v1/` endpoints.

**How:** `middleware.AdminAuth(apiKey)` — if `ADMIN_API_KEY` is empty, pass-through (dev mode). If set, rejects missing/invalid keys with 401/403. Applied at the group level in the router.

**Why optional?** Zero-friction local development. In production you set the env var and all endpoints are protected. The dashboard UI (`/dashboard`) is served outside the auth group intentionally — in production you'd put an auth proxy (OAuth2 Proxy, Authelia) in front of everything.

### 15b. CORS + Security Headers

**What:** `middleware.CORS()` and `middleware.SecurityHeaders()` applied globally.

**Headers set:** `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy`, `Cross-Origin-Opener-Policy`, `Cross-Origin-Embedder-Policy`.

**CORS:** Respects `ALLOWED_ORIGINS` env var. Preflight (`OPTIONS`) handled automatically with 204 and proper headers.

### 15c. SSRF Protection

**What:** Prevents the worker from delivering to internal/private IPs.

**How:** `worker.ssrfCheck()` resolves the target hostname and blocks loopback (`127.0.0.1`, `::1`), private (`10.x`, `172.16-31.x`, `192.168.x`), and link-local (`169.254.x`) addresses. Runs before every delivery attempt in the worker.

**Tradeoff:** Adds a DNS lookup per delivery. DNS is cached by the OS resolver (default TTL). For high-throughput, consider a DNS cache layer or an IP allowlist for environments that require internal delivery.

### 15d. Request Body Size Limit

**What:** `middleware.BodySizeLimit(maxBytes)` caps incoming request bodies.

**Why:** Prevents OOM attacks and accidental large payload ingestion. Default 1MB, configurable via `MAX_BODY_BYTES`.

### 15e. Non-Root Container

**What:** Dockerfile creates a `hookforge` user and runs the binary as non-root.

**How:** `adduser -S hookforge -G hookforge` → `USER hookforge`. Combined with Helm `readOnlyRootFilesystem: true`, `runAsNonRoot: true`, and `capabilities.drop: ["ALL"]`.

### 15f. Separate /ready Endpoint

**What:** `/ready` checks DB + Redis connectivity, unlike `/health` which only checks the app is running.

**Why:** Kubernetes readiness probes routed to `/ready` prevent traffic being sent to pods that can't serve requests. The Helm chart uses `/health` for liveness and `/ready` for readiness.

### 15g. Runbook

**What:** `RUNBOOK.md` covers backup/restore, common issue diagnosis, restart procedures, and recovery steps.

**Interview angle:** "I documented recovery scenarios because on-call engineers need clear procedures during incidents. A runbook reduces MTTR from hours to minutes."

---

## 16. Resume Bullet Points

Copy-paste these into your resume:

```
HookForge — Open-source webhook delivery engine in Go
Go, PostgreSQL, Redis, Prometheus, Docker, HTMX

• Architected Redis-backed job queue with configurable goroutine worker pool (5 
  workers, 3,000+ events/sec throughput) and exponential backoff retry engine
• Implemented HMAC-SHA256 payload signing, per-endpoint secrets with rotation, 
  and Dead Letter Queue with one-click replay for failed events
• Built per-endpoint rate limiter using Redis token bucket (Lua script for atomic 
  fairness) and circuit breaker pattern to fail fast on unhealthy targets
• Created real-time dashboard with HTMX + WebSocket live updates, delivery 
  attempt logging, and Prometheus metrics with Grafana-ready endpoints
• Wrote full test suite with testcontainers-go (auto-provisioned Postgres + Redis 
  containers) and k6 load test achieving p99 < 15ms latency
• Production-ready deployment via docker-compose with Caddy TLS, AOF Redis 
  persistence, and health-checked service orchestration
```
