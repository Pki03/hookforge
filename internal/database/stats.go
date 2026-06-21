package database

import (
	"context"
	"fmt"
)

type Stats struct {
	TotalSent     int64   `json:"total_sent"`
	Delivered     int64   `json:"delivered"`
	Failed        int64   `json:"failed"`
	Dead          int64   `json:"dead"`
	Pending       int64   `json:"pending"`
	DeliveryRate  float64 `json:"delivery_rate_percent"`
	AvgLatencyMs  float64 `json:"avg_latency_ms"`
}

func (db *DB) GetStats(ctx context.Context) (*Stats, error) {
	s := &Stats{}

	err := db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM events`).Scan(&s.TotalSent)
	if err != nil {
		return nil, fmt.Errorf("count total: %w", err)
	}

	err = db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM events WHERE status = 'delivered'`).Scan(&s.Delivered)
	if err != nil {
		return nil, fmt.Errorf("count delivered: %w", err)
	}

	err = db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM events WHERE status = 'failed'`).Scan(&s.Failed)
	if err != nil {
		return nil, fmt.Errorf("count failed: %w", err)
	}

	err = db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM events WHERE status = 'dead'`).Scan(&s.Dead)
	if err != nil {
		return nil, fmt.Errorf("count dead: %w", err)
	}

	err = db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM events WHERE status = 'pending' OR status = 'retrying'`).Scan(&s.Pending)
	if err != nil {
		return nil, fmt.Errorf("count pending: %w", err)
	}

	if s.TotalSent > 0 {
		s.DeliveryRate = float64(s.Delivered) / float64(s.TotalSent) * 100
	}

	return s, nil
}
