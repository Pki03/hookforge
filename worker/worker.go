package worker

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/prateekkhurmi/hookforge/internal/circuitbreaker"
	"github.com/prateekkhurmi/hookforge/internal/config"
	"github.com/prateekkhurmi/hookforge/internal/database"
	"github.com/prateekkhurmi/hookforge/internal/metrics"
	"github.com/prateekkhurmi/hookforge/internal/notifier"
	"github.com/prateekkhurmi/hookforge/internal/redis"
)

type Worker struct {
	db     *database.DB
	rdb    *redis.Client
	cfg    *config.Config
	client *http.Client
	cb     *circuitbreaker.EndpointBreaker
}

func New(db *database.DB, rdb *redis.Client, cfg *config.Config) *Worker {
	return &Worker{
		db:  db,
		rdb: rdb,
		cfg: cfg,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		cb: circuitbreaker.NewEndpointBreaker(circuitbreaker.Config{
			FailureThreshold: 5,
			ResetTimeout:     30 * time.Second,
			HalfOpenMaxReqs:  1,
		}),
	}
}

func (w *Worker) Start(ctx context.Context) {
	for i := 0; i < w.cfg.WorkerCount; i++ {
		go w.deliveryLoop(ctx)
	}
	go w.retryLoop(ctx)
	slog.Info("worker started", "delivery_goroutines", w.cfg.WorkerCount)
}

func (w *Worker) deliveryLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			slog.Info("delivery loop stopped")
			return
		default:
			eventID, err := w.rdb.DequeueEvent(ctx)
			if err != nil {
				time.Sleep(100 * time.Millisecond)
				continue
			}
			if err := w.deliver(ctx, eventID); err != nil {
				slog.Warn("delivery failed", "event_id", shortID(eventID), "error", err.Error())
			}
		}
	}
}

func (w *Worker) deliver(ctx context.Context, eventID string) error {
	event, err := w.db.GetEvent(ctx, eventID)
	if err != nil {
		return fmt.Errorf("get event: %w", err)
	}

	endpoint, err := w.db.GetEndpoint(ctx, event.EndpointID)
	if err != nil {
		return fmt.Errorf("get endpoint: %w", err)
	}
	if endpoint == nil {
		return fmt.Errorf("endpoint %s not found", event.EndpointID)
	}

	breaker := w.cb.Get(event.EndpointID)
	if !breaker.Allow() {
		metrics.CircuitBreakerTrips.WithLabelValues(endpoint.ID).Inc()
		slog.Warn("circuit breaker open, skipping delivery",
			"event_id", shortID(eventID),
			"endpoint_id", shortID(endpoint.ID),
			"url", endpoint.URL,
		)
		w.db.CreateAttempt(ctx, eventID, event.Attempts+1, nil, nil, strPtr("circuit breaker open — target marked as failing"), 0)
		w.handleFailure(ctx, event, endpoint)
		return fmt.Errorf("circuit breaker open for endpoint %s", endpoint.ID)
	}

	if err := ssrfCheck(endpoint.URL); err != nil {
		slog.Warn("ssrf protection blocked delivery",
			"event_id", shortID(eventID),
			"endpoint_id", shortID(endpoint.ID),
			"url", endpoint.URL,
		)
		w.db.CreateAttempt(ctx, eventID, event.Attempts+1, nil, nil, strPtr("ssrf block: "+err.Error()), 0)
		w.handleFailure(ctx, event, endpoint)
		return fmt.Errorf("ssrf check failed for %s: %w", endpoint.URL, err)
	}

	secret := endpoint.Secret
	if secret == "" {
		secret = w.cfg.SigningSecret
	}

	signature := signPayload(event.Payload, secret)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.URL, bytes.NewReader(event.Payload))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-HookForge-Signature", signature)
	req.Header.Set("X-HookForge-Event-ID", eventID)
	req.Header.Set("User-Agent", "HookForge/1.0")

	start := time.Now()
	resp, err := w.client.Do(req)
	durationMs := int(time.Since(start).Milliseconds())

	metrics.DeliveryAttempts.Inc()

	if err != nil {
		metrics.EventsTotal.WithLabelValues("failed").Inc()
		breaker.Failure()
		w.db.CreateAttempt(ctx, eventID, event.Attempts+1, nil, nil, strPtr(err.Error()), durationMs)
		w.handleFailure(ctx, event, endpoint)
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 500))
	bodyStr := string(bodyBytes)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		metrics.EventsTotal.WithLabelValues("delivered").Inc()
		metrics.DeliveryLatency.Observe(time.Since(start).Seconds())
		breaker.Success()
		if err := w.db.UpdateEventStatus(ctx, eventID, "delivered"); err != nil {
			return fmt.Errorf("update status: %w", err)
		}
		w.db.CreateAttempt(ctx, eventID, event.Attempts+1, &resp.StatusCode, &bodyStr, nil, durationMs)
		slog.Info("delivered",
			"event_id", shortID(eventID),
			"endpoint_id", shortID(endpoint.ID),
			"status", resp.StatusCode,
			"latency_ms", durationMs,
		)
		return nil
	}

	metrics.EventsTotal.WithLabelValues("failed").Inc()
	breaker.Failure()
	statusCode := resp.StatusCode
	w.db.CreateAttempt(ctx, eventID, event.Attempts+1, &statusCode, nil, strPtr(fmt.Sprintf("bad status %d from target", statusCode)), durationMs)
	w.handleFailure(ctx, event, endpoint)
	return fmt.Errorf("bad status %d from %s", statusCode, endpoint.URL)
}

func (w *Worker) handleFailure(ctx context.Context, event *database.Event, endpoint *database.Endpoint) {
	event.Attempts++

	backoff := calculateBackoff(event.Attempts)
	nextRetryAt := time.Now().Add(backoff)

	if event.Attempts >= event.MaxRetries {
		metrics.EventsTotal.WithLabelValues("dead").Inc()
		w.db.UpdateEventStatus(ctx, event.ID, "dead")
		slog.Warn("event moved to dead letter queue",
			"event_id", shortID(event.ID),
			"endpoint_id", shortID(event.EndpointID),
			"attempts", event.Attempts,
			"max_retries", event.MaxRetries,
		)
		notifier.SendSlackAlert(endpoint.SlackWebhookURL, event.ID, event.EndpointID, "dead", event.Attempts, event.MaxRetries)
		emailCfg := w.cfg.EmailConfig()
		notifier.SendEmailAlert(emailCfg, endpoint.Email, event.ID, event.EndpointID, "dead", event.Attempts, event.MaxRetries)
		return
	}

	w.db.UpdateEventStatus(ctx, event.ID, "retrying")
	w.db.IncrementAttempts(ctx, event.ID, &nextRetryAt)
	slog.Info("scheduling retry",
		"event_id", shortID(event.ID),
		"endpoint_id", shortID(event.EndpointID),
		"attempt", event.Attempts,
		"max_retries", event.MaxRetries,
		"backoff", backoff.String(),
	)
}

func strPtr(s string) *string {
	return &s
}

func (w *Worker) retryLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("retry loop stopped")
			return
		case <-ticker.C:
			w.processRetries(ctx)
		}
	}
}

func (w *Worker) processRetries(ctx context.Context) {
	events, err := w.db.ListEvents(ctx, "retrying", 100)
	if err != nil {
		slog.Error("list retry events", "error", err)
		return
	}

	now := time.Now()
	for _, event := range events {
		if event.NextRetryAt != nil && event.NextRetryAt.Before(now) {
			if err := w.rdb.EnqueueEvent(ctx, event.ID); err != nil {
				slog.Error("retry enqueue", "event_id", shortID(event.ID), "error", err)
				continue
			}
			slog.Info("re-enqueued for retry", "event_id", shortID(event.ID))
		}
	}
	metrics.RetryCount.Set(float64(len(events)))
}

func calculateBackoff(attempt int) time.Duration {
	backoff := time.Duration(1<<(attempt-1)) * time.Second
	if backoff > 32*time.Second {
		backoff = 32 * time.Second
	}
	return backoff
}

func signPayload(payload []byte, secret string) string {
	if secret == "" {
		return ""
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func ssrfCheck(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("unsupported scheme %s", u.Scheme)
	}
	host := u.Hostname()
	ips, err := net.LookupIP(host)
	if err != nil {
		return nil
	}
	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
			return fmt.Errorf("blocked private/loopback IP %s for host %s", ip, host)
		}
	}
	return nil
}
