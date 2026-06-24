package database

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

var endpointColumns = `id, url, secret, slack_webhook_url, email, allowed_event_types, rate_limit_per_second, rate_limit_burst, created_at, updated_at`

func parseAllowedEventTypes(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func joinAllowedEventTypes(types []string) string {
	return strings.Join(types, ",")
}

func scanEndpoint(row pgx.Row) (*Endpoint, error) {
	var allowedStr string
	e := &Endpoint{}
	err := row.Scan(&e.ID, &e.URL, &e.Secret, &e.SlackWebhookURL, &e.Email, &allowedStr, &e.RateLimitPerSecond, &e.RateLimitBurst, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		return nil, err
	}
	e.AllowedEventTypes = parseAllowedEventTypes(allowedStr)
	return e, nil
}

func generateSecret() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (db *DB) CreateEndpoint(ctx context.Context, url string, slackWebhookURL string, email string, allowedEventTypes []string) (*Endpoint, string, error) {
	secret := generateSecret()
	allowedStr := joinAllowedEventTypes(allowedEventTypes)
	e := &Endpoint{}
	err := db.Pool.QueryRow(ctx,
		`INSERT INTO endpoints (url, secret, slack_webhook_url, email, allowed_event_types) VALUES ($1, $2, $3, $4, $5)
		 RETURNING `+endpointColumns,
		url, secret, slackWebhookURL, email, allowedStr,
	).Scan(&e.ID, &e.URL, &e.Secret, &e.SlackWebhookURL, &e.Email, &allowedStr, &e.RateLimitPerSecond, &e.RateLimitBurst, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		return nil, "", fmt.Errorf("creating endpoint: %w", err)
	}
	e.AllowedEventTypes = parseAllowedEventTypes(allowedStr)
	return e, secret, nil
}

func (db *DB) GetEndpoint(ctx context.Context, id string) (*Endpoint, error) {
	row := db.Pool.QueryRow(ctx,
		`SELECT `+endpointColumns+` FROM endpoints WHERE id = $1`, id,
	)
	e, err := scanEndpoint(row)
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
	err := db.Pool.QueryRow(ctx, `SELECT url FROM endpoints WHERE id = $1`, id).Scan(&url)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", nil
		}
		return "", fmt.Errorf("getting endpoint url: %w", err)
	}
	return url, nil
}

func (db *DB) ListEndpoints(ctx context.Context) ([]Endpoint, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT `+endpointColumns+` FROM endpoints ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("listing endpoints: %w", err)
	}
	defer rows.Close()

	var endpoints []Endpoint
	for rows.Next() {
		e, err := scanEndpoint(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning endpoint: %w", err)
		}
		endpoints = append(endpoints, *e)
	}
	return endpoints, nil
}

func (db *DB) RotateEndpointSecret(ctx context.Context, id string) (string, error) {
	secret := generateSecret()
	_, err := db.Pool.Exec(ctx,
		`UPDATE endpoints SET secret = $1, updated_at = NOW() WHERE id = $2`,
		secret, id,
	)
	if err != nil {
		return "", fmt.Errorf("rotate secret: %w", err)
	}
	return secret, nil
}
