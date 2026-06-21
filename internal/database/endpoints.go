package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func (db *DB) CreateEndpoint(ctx context.Context, url string) (*Endpoint, error) {
	e := &Endpoint{}
	err := db.Pool.QueryRow(ctx,
		`INSERT INTO endpoints (url) VALUES ($1)
		 RETURNING id, url, created_at, updated_at`,
		url,
	).Scan(&e.ID, &e.URL, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("creating endpoint: %w", err)
	}
	return e, nil
}

func (db *DB) GetEndpoint(ctx context.Context, id string) (*Endpoint, error) {
	e := &Endpoint{}
	err := db.Pool.QueryRow(ctx,
		`SELECT id, url, created_at, updated_at FROM endpoints WHERE id = $1`,
		id,
	).Scan(&e.ID, &e.URL, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("getting endpoint: %w", err)
	}
	return e, nil
}

func (db *DB) GetEndpointURL(ctx context.Context, id string) (string, error) {
	var url string
	err := db.Pool.QueryRow(ctx,
		`SELECT url FROM endpoints WHERE id = $1`, id,
	).Scan(&url)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", nil
		}
		return "", fmt.Errorf("getting endpoint url: %w", err)
	}
	return url, nil
}
