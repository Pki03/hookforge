package database

import (
	"context"
	"fmt"
	"time"
)

var eventColumns = `id, endpoint_id, event_type, payload, status, attempts, max_retries, next_retry_at, created_at, updated_at`

type CreateEventParams struct {
	EndpointID string
	EventType  string
	Payload    []byte
	MaxRetries int
}

func (db *DB) CreateEvent(ctx context.Context, p CreateEventParams) (*Event, error) {
	if p.MaxRetries == 0 {
		p.MaxRetries = 5
	}

	e := &Event{}
	err := db.Pool.QueryRow(ctx,
		`INSERT INTO events (endpoint_id, event_type, payload, max_retries, status)
		 VALUES ($1, $2, $3, $4, 'pending')
		 RETURNING `+eventColumns,
		p.EndpointID, p.EventType, p.Payload, p.MaxRetries,
	).Scan(&e.ID, &e.EndpointID, &e.EventType, &e.Payload, &e.Status, &e.Attempts, &e.MaxRetries, &e.NextRetryAt, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("creating event: %w", err)
	}
	return e, nil
}

func scanEvent(row interface{ Scan(...interface{}) error }) (*Event, error) {
	e := &Event{}
	err := row.Scan(&e.ID, &e.EndpointID, &e.EventType, &e.Payload, &e.Status, &e.Attempts, &e.MaxRetries, &e.NextRetryAt, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return e, nil
}

func (db *DB) GetEvent(ctx context.Context, id string) (*Event, error) {
	row := db.Pool.QueryRow(ctx,
		`SELECT `+eventColumns+` FROM events WHERE id = $1`, id,
	)
	e, err := scanEvent(row)
	if err != nil {
		return nil, fmt.Errorf("getting event: %w", err)
	}
	return e, nil
}

func (db *DB) UpdateEventStatus(ctx context.Context, id string, status string) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE events SET status = $1, updated_at = NOW() WHERE id = $2`,
		status, id,
	)
	return err
}

func (db *DB) RecordAttempt(ctx context.Context, id string, attempts int, status string, nextRetryAt *time.Time) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE events SET attempts = $1, status = $2, next_retry_at = $3, updated_at = NOW() WHERE id = $4`,
		attempts, status, nextRetryAt, id,
	)
	return err
}

func (db *DB) IncrementAttempts(ctx context.Context, id string, nextRetryAt *time.Time) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE events SET attempts = attempts + 1, status = 'retrying', next_retry_at = $1, updated_at = NOW() WHERE id = $2`,
		nextRetryAt, id,
	)
	return err
}

func (db *DB) ListEvents(ctx context.Context, status string, limit int) ([]Event, error) {
	if limit == 0 {
		limit = 50
	}

	rows, err := db.Pool.Query(ctx,
		`SELECT `+eventColumns+` FROM events WHERE ($1 = '' OR status = $1)
		 ORDER BY created_at DESC LIMIT $2`,
		status, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("listing events: %w", err)
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning event: %w", err)
		}
		events = append(events, *e)
	}
	return events, nil
}
