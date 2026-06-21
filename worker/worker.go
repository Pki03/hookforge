package worker

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/prateekkhurmi/hookforge/internal/config"
	"github.com/prateekkhurmi/hookforge/internal/database"
	"github.com/prateekkhurmi/hookforge/internal/redis"
)

type Worker struct {
	db     *database.DB
	rdb    *redis.Client
	cfg    *config.Config
	client *http.Client
}

func New(db *database.DB, rdb *redis.Client, cfg *config.Config) *Worker {
	return &Worker{
		db:  db,
		rdb: rdb,
		cfg: cfg,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (w *Worker) Start(ctx context.Context) {
	go w.deliveryLoop(ctx)
	go w.retryLoop(ctx)
	log.Println("worker started")
}

func (w *Worker) deliveryLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			log.Println("delivery loop stopped")
			return
		default:
			eventID, err := w.rdb.DequeueEvent(ctx)
			if err != nil {
				log.Printf("dequeue error: %v", err)
				time.Sleep(100 * time.Millisecond)
				continue
			}

			if err := w.deliver(ctx, eventID); err != nil {
				log.Printf("delivery failed for event %s: %v", eventID, err)
			}
		}
	}
}

func (w *Worker) deliver(ctx context.Context, eventID string) error {
	event, err := w.db.GetEvent(ctx, eventID)
	if err != nil {
		return fmt.Errorf("get event: %w", err)
	}

	endpointURL, err := w.db.GetEndpointURL(ctx, event.EndpointID)
	if err != nil {
		return fmt.Errorf("get endpoint: %w", err)
	}
	if endpointURL == "" {
		return fmt.Errorf("endpoint %s not found", event.EndpointID)
	}

	payloadBytes := event.Payload
	signature := signPayload(payloadBytes, w.cfg.SigningSecret)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpointURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-HookForge-Signature", signature)
	req.Header.Set("X-HookForge-Event-ID", eventID)
	req.Header.Set("User-Agent", "HookForge/1.0")

	start := time.Now()
	resp, err := w.client.Do(req)
	latency := time.Since(start)

	if err != nil {
		if err := w.handleFailure(ctx, event); err != nil {
			log.Printf("handle failure error: %v", err)
		}
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if err := w.db.UpdateEventStatus(ctx, eventID, "delivered"); err != nil {
			return fmt.Errorf("update status: %w", err)
		}
		log.Printf("delivered event %s to %s (status=%d, latency=%v)", eventID, endpointURL, resp.StatusCode, latency)
		return nil
	}

	if err := w.handleFailure(ctx, event); err != nil {
		log.Printf("handle failure error: %v", err)
	}
	return fmt.Errorf("bad status %d from %s", resp.StatusCode, endpointURL)
}

func (w *Worker) handleFailure(ctx context.Context, event *database.Event) error {
	event.Attempts++

	if event.Attempts >= event.MaxRetries {
		if err := w.db.UpdateEventStatus(ctx, event.ID, "dead"); err != nil {
			return fmt.Errorf("mark dead: %w", err)
		}
		log.Printf("event %s moved to dead letter queue after %d attempts", event.ID, event.Attempts)
		return nil
	}

	backoff := calculateBackoff(event.Attempts)
	nextRetryAt := time.Now().Add(backoff)

	if err := w.db.IncrementAttempts(ctx, event.ID, &nextRetryAt); err != nil {
		return fmt.Errorf("increment attempts: %w", err)
	}

	log.Printf("event %s failed (attempt %d/%d), retrying in %v", event.ID, event.Attempts, event.MaxRetries, backoff)
	return nil
}

func (w *Worker) retryLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("retry loop stopped")
			return
		case <-ticker.C:
			w.processRetries(ctx)
		}
	}
}

func (w *Worker) processRetries(ctx context.Context) {
	events, err := w.db.ListEvents(ctx, "retrying", 100)
	if err != nil {
		log.Printf("list retry events error: %v", err)
		return
	}

	now := time.Now()
	for _, event := range events {
		if event.NextRetryAt != nil && event.NextRetryAt.Before(now) {
			if err := w.rdb.EnqueueEvent(ctx, event.ID); err != nil {
				log.Printf("retry enqueue error for event %s: %v", event.ID, err)
				continue
			}
			log.Printf("re-enqueued event %s for retry", event.ID)
		}
	}
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
