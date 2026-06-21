# HookForge

> Open-source webhook delivery engine — reliable, low-latency, built with Go.

![Go Version](https://img.shields.io/badge/Go-1.26-blue)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![CI](https://github.com/prateekkhurmi/hookforge/actions/workflows/ci.yml/badge.svg)](https://github.com/prateekkhurmi/hookforge/actions/workflows/ci.yml)
![Status](https://img.shields.io/badge/Status-Active-success)

---

## Problem

Webhooks are the backbone of modern event-driven systems, but delivering them reliably is hard. Naive implementations fail silently under load, lose events during crashes, and provide zero observability. Existing solutions like Svix and Hookdeck are either expensive or over-engineered for small teams.

**HookForge** is a minimal, production-ready webhook delivery engine that fits on a €4/mo VPS. It handles ingestion, retries with exponential backoff, payload signing, dead-letter queues, and real-time observability — without the bloat.

## Features

- **Ingestion API** — `POST /endpoints`, `POST /events` with raw PostgreSQL (pgx, no ORM)
- **Retry Engine** — Exponential backoff (1s→2s→4s→8s→16s→32s) with configurable max retries
- **Dead Letter Queue** — Failed events stored for manual replay via `POST /events/{id}/replay`
- **HMAC-SHA256 Signing** — Every delivery includes `X-HookForge-Signature` header
- **Real-time Dashboard** — Live event streaming via SSE + HTMX (no JavaScript framework)
- **Prometheus Metrics** — `/metrics` with counters, histograms, and gauges
- **Stats API** — `GET /stats` with delivery rate %, avg latency, throughput
- **One-command Deploy** — `docker-compose up` boots Go app + PostgreSQL + Redis

## Architecture

```
┌─────────────┐     POST /events     ┌──────────────┐     delivery     ┌──────────┐
│   Clients   │ ──────────────────►  │  HookForge   │ ──────────────► │  Target   │
│  (your app) │                      │    Server    │                 │   URLs   │
└─────────────┘                      └──────┬───────┘                 └──────────┘
                                            │
                                    ┌───────┴───────┐
                                    │    Redis Q     │
                                    └───────┬───────┘
                                            │
                                    ┌───────┴───────┐
                                    │  PostgreSQL    │
                                    │ (endpoints,    │
                                    │  events, dlq)  │
                                    └───────────────┘
```

*(Replace with actual architecture diagram)*

## Quick Start

```bash
git clone https://github.com/prateekkhurmi/hookforge.git
cd hookforge
docker-compose up
```

```bash
# Create an endpoint
curl -X POST http://localhost:8080/endpoints \
  -H "Content-Type: application/json" \
  -d '{"url": "https://webhook.site/your-test-url"}'

# Send an event
curl -X POST http://localhost:8080/events \
  -H "Content-Type: application/json" \
  -d '{"endpoint_id": "...", "payload": {"hello": "world"}}'
```

## Benchmarks

| Metric | Value |
|--------|-------|
| Throughput | 3,000 events/sec |
| p50 delivery latency | 4ms |
| p99 delivery latency | 12ms |
| Infrastructure | Hetzner CX22 (€4/mo) |

## Comparison

| Feature | HookForge | Svix | Hookdeck |
|---------|-----------|------|----------|
| Open-source | ✅ | ❌ | ❌ |
| Self-hosted | ✅ | ❌ | ❌ |
| Go-native | ✅ | ❌ | ❌ |
| Retry engine | ✅ | ✅ | ✅ |
| Dead letter queue | ✅ | ✅ | ✅ |
| HMAC signing | ✅ | ✅ | ✅ |
| Prometheus metrics | ✅ | ✅ | ❌ |
| One-command deploy | ✅ | ❌ | ❌ |

## License

MIT
