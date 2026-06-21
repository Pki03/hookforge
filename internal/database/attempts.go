package database

import (
	"context"
	"fmt"
)

func (db *DB) CreateAttempt(ctx context.Context, eventID string, attemptNum int, statusCode *int, responseBody *string, errorMessage *string, durationMs int) error {
	_, err := db.Pool.Exec(ctx,
		`INSERT INTO delivery_attempts (event_id, attempt_num, status_code, response_body, error_message, duration_ms)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		eventID, attemptNum, statusCode, responseBody, errorMessage, durationMs,
	)
	if err != nil {
		return fmt.Errorf("creating attempt: %w", err)
	}
	return nil
}

func (db *DB) ListAttempts(ctx context.Context, eventID string) ([]DeliveryAttempt, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT id, event_id, attempt_num, status_code, response_body, error_message, duration_ms, attempted_at
		 FROM delivery_attempts WHERE event_id = $1
		 ORDER BY attempted_at ASC`, eventID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing attempts: %w", err)
	}
	defer rows.Close()

	var attempts []DeliveryAttempt
	for rows.Next() {
		var a DeliveryAttempt
		if err := rows.Scan(&a.ID, &a.EventID, &a.AttemptNum, &a.StatusCode, &a.ResponseBody, &a.ErrorMessage, &a.DurationMs, &a.AttemptedAt); err != nil {
			return nil, fmt.Errorf("scanning attempt: %w", err)
		}
		attempts = append(attempts, a)
	}
	return attempts, nil
}
