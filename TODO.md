# HookForge — Remaining Features

Items from original roadmap not yet implemented:

1. **Webhook filtering / event types**
   - Add `event_type` field to events
   - Allow endpoints to filter by accepted event types
   - Reject events with unaccepted types at ingestion

2. **Email failure alerts**
   - SMTP-based email notifications when events hit dead letter queue
   - Configurable via env vars (SMTP_HOST, SMTP_PORT, etc.)
   - Works alongside existing Slack alerts

3. **Helm chart for Kubernetes deploy**
   - Helm chart with Deployment, Service, ConfigMap, Secrets
   - Postgres + Redis as sub-charts or external dependencies
   - Ingress + TLS support
