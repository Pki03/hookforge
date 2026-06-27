# Benchmarks

Real load test results from k6 against the local Docker Compose stack (Postgres 16 + Redis 7 + 1 app replica, Go 1.26).

## Test 1: Mixed Workload (Endpoints + Events + Stats)

50 VUs ramp-up/steady/ramp-down over 2 minutes. Each iteration: 1 POST endpoint → 5 POST events → 1 GET stats.

| Metric | Value | Threshold | Status |
|--------|-------|-----------|--------|
| Total HTTP requests | 58,807 | — | — |
| Sustained throughput | 490 req/s | — | — |
| Failure rate | 0.00% | < 1% | ✅ |
| p(99) latency | 30.23 ms | < 200 ms | ✅ |
| p(95) latency | 18.37 ms | — | — |
| p(90) latency | 12.52 ms | — | — |
| Avg latency | 5.29 ms | — | — |

## Test 2: Pure Event Ingestion (Optimized)

100 VUs, 50s (10s ramp-up, 30s steady, 10s ramp-down). Single pre-created endpoint, each VU fires events at max rate.

| Metric | Value | Threshold | Status |
|--------|-------|-----------|--------|
| Total events ingested | 122,047 | — | — |
| Sustained throughput | **2,440 events/sec** | — | — |
| Failure rate | **0.00%** | < 1% | ✅ |
| p(99) latency | **91.51 ms** | < 100 ms | ✅ |
| p(95) latency | 86.19 ms | — | — |
| p(90) latency | 82.51 ms | — | — |
| Avg latency | 32.77 ms | — | — |
| Checks passed | 122,048 / 122,048 (100%) | — | ✅ |

## Test Scripts

| Script | Purpose | Command |
|--------|---------|---------|
| `load-test/k6_load_test.js` | End-to-end mixed workload (creates endpoints, fires events, checks stats) | `k6 run load-test/k6_load_test.js` |
| `load-test/k6_ingestion.js` | Pure event ingestion benchmark (pre-created endpoint, max throughput) | `k6 run load-test/k6_ingestion.js` |

## Results Files

- `load-test/k6_results.json` — mixed workload raw output
- `load-test/k6_ingestion_results.json` — ingestion benchmark raw output

## Methodology

- **Tool**: k6
- **App config**: Gin release mode, `DB_POOL_SIZE=20`, `WORKER_COUNT=5`
- **Target**: `http://localhost:8080` (Docker Compose)
- **Delivery target**: `https://httpbin.org/post` (external)
- **Test date**: 2026-06-27

## Interpretation

### Ingestion throughput (2,440 events/sec)
The event ingestion pipeline (`POST /api/v1/events`) handles ~2,440 events/sec on a single node with 100 concurrent clients. Each event is parsed, validated against the endpoint config, INSERTed into PostgreSQL, and LPUSHed to Redis — all in a single request-response cycle. The bottleneck is per-event DB round-trips.

### Mixed workload (490 req/s)
The mixed workload test includes endpoint creation (DB INSERT) and stats queries (DB aggregate query), which consume additional database connections. For pure event ingestion, throughput is 5x higher.

### Delivery throughput
Delivery to external targets depends on target responsiveness. For production use, delivery scales with target server capacity, worker pool size (`WORKER_COUNT`), and circuit breaker state.

## Recommendations for Higher Throughput

To push beyond 2,440 events/sec:
- **Batch DB writes**: Use `pgx` batch API or COPY for bulk INSERT
- **Async write**: Defer DB INSERT to background worker, return 201 immediately after Redis enqueue
- **Prefork**: Run multiple OS processes via Gin's `--prefork` or behind a load balancer
- **Endpoint cache**: Avoid per-request DB query for endpoint lookup (in-memory cache with TTL)
