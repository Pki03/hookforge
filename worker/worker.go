package worker

import (
	"context"
	"log"
	"time"

	"github.com/prateekkhurmi/hookforge/internal/config"
	"github.com/prateekkhurmi/hookforge/internal/database"
	"github.com/redis/go-redis/v9"
)

type Worker struct {
	db  *database.DB
	rdb *redis.Client
	cfg *config.Config
}

func New(db *database.DB, rdb *redis.Client, cfg *config.Config) *Worker {
	return &Worker{db: db, rdb: rdb, cfg: cfg}
}

func (w *Worker) Start(ctx context.Context) {
	go w.deliveryLoop(ctx)
	go w.retryLoop(ctx)
}

func (w *Worker) deliveryLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			result, err := w.rdb.BLPop(ctx, 1*time.Second, "events:queue").Result()
			if err != nil {
				continue
			}
			if len(result) < 2 {
				continue
			}
			eventID := result[1]
			log.Printf("delivering event %s", eventID)
		}
	}
}

func (w *Worker) retryLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
