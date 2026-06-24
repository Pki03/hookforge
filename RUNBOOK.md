# HookForge Runbook

## Health Checks

| Endpoint | Purpose | Expected |
|----------|---------|----------|
| `/health` | Liveness — app is alive | `200 {"status":"ok"}` |
| `/ready` | Readiness — DB + Redis connected | `200 {"status":"ok","db":true,"redis":true}` |

## Restart Services

```bash
# Docker Compose
docker-compose restart app

# Kubernetes
kubectl rollout restart deployment hookforge
```

## Check Logs

```bash
# Docker Compose
docker-compose logs -f app

# Kubernetes
kubectl logs -l app.kubernetes.io/name=hookforge -f
```

## Common Issues

### Events Stuck "pending"
1. Check Redis: `redis-cli LLEN events:queue` — if > 0, workers are processing
2. Check worker logs: `docker-compose logs app | grep deliver`
3. If queue is empty but events are pending, restart app: `docker-compose restart app`

### Circuit Breaker Open
1. Check `/metrics` for `hookforge_circuit_breaker_trips_total`
2. Verify the target endpoint is healthy
3. The breaker auto-resets after 30 seconds
4. For immediate reset: delete the endpoint and recreate

### Dead Letter Queue Growing
1. Check failed attempt details via `GET /api/v1/events/:id`
2. Verify the target URL is reachable from the app container
3. Replay dead events: `POST /api/v1/events/:id/replay`
4. If target is permanently down, create a new endpoint with a working URL

### High Memory Usage
- Set `MAX_BODY_BYTES` env var to cap payload size (default 1MB)
- Check if any large events are stuck with `GET /api/v1/events`
- Reduce worker count with `WORKER_COUNT`

## Database Recovery

### Backup
```bash
docker exec hookforge-postgres-1 pg_dump -U postgres hookforge > backup.sql
```

### Restore
```bash
cat backup.sql | docker exec -i hookforge-postgres-1 psql -U postgres hookforge
```

### Reset & Re-migrate
```bash
docker-compose down -v   # destroys all data
docker-compose up -d     # fresh start, auto-migrates
```

## Redis Recovery

Check AOF persistence is on:
```bash
redis-cli CONFIG GET appendonly
# Should return: appendonly yes
```

Flush queue without data loss:
```bash
redis-cli DEL events:queue
```