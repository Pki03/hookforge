# HookForge — Remaining Features

All original roadmap items have been implemented:

1. ~~**Webhook filtering / event types**~~ ✅
   - Migration 004: event_type on events, allowed_event_types on endpoints
   - Validates event_type against endpoint whitelist at ingestion (422 if rejected)

2. ~~**Email failure alerts**~~ ✅
   - Migration 005: email column on endpoints
   - SMTP notifier alongside existing Slack alerts
   - Configurable via env vars (SMTP_HOST, SMTP_PORT, SMTP_USER, SMTP_PASSWORD, SMTP_FROM)

3. ~~**Helm chart for Kubernetes deploy**~~ ✅
   - `deploy/helm/hookforge/` — Deployment, Service, ConfigMap, Secrets, Ingress, migration Job
   - Separate `/app/migrate` binary in Docker image for migration job

## Next if desired:
- Webhook replay endpoint (POST /api/v1/events/:id/replay)
- Batch event ingestion (POST /api/v1/events/batch)
- Admin API key auth via header
