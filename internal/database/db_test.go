package database

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	tcRedis "github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go/wait"
)

var testDB *DB
var testRDB *goredis.Client

func connectPostgres(ctx context.Context, dsn string) (string, error) {
	if dsn != "" {
		return dsn, nil
	}
	c, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("hookforge_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2),
		),
	)
	if err != nil {
		return "", fmt.Errorf("start postgres container: %w", err)
	}
	url, err := c.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return "", fmt.Errorf("pg connection string: %w", err)
	}
	return url, nil
}

func connectRedis(ctx context.Context, url string) (string, error) {
	if url != "" {
		return url, nil
	}
	c, err := tcRedis.Run(ctx, "redis:7-alpine")
	if err != nil {
		return "", fmt.Errorf("start redis container: %w", err)
	}
	return c.ConnectionString(ctx)
}

func runMigrations(dsn string) error {
	m, err := migrate.New("file://../../db/migrations", dsn)
	if err != nil {
		return fmt.Errorf("migration init: %w", err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migration up: %w", err)
	}
	return nil
}

func TestMain(m *testing.M) {
	ctx := context.Background()

	pgURL := os.Getenv("DATABASE_URL")
	redisURL := os.Getenv("REDIS_URL")

	pgURL, err := connectPostgres(ctx, pgURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "postgres setup: %v\n", err)
		os.Exit(1)
	}

	pool, err := pgxpool.New(ctx, pgURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect to pg: %v\n", err)
		os.Exit(1)
	}
	testDB = &DB{Pool: pool}

	if err := runMigrations(pgURL); err != nil {
		fmt.Fprintf(os.Stderr, "migrations: %v\n", err)
		os.Exit(1)
	}

	redisURL, err = connectRedis(ctx, redisURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "redis setup: %v\n", err)
		os.Exit(1)
	}

	opts, err := goredis.ParseURL(redisURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse redis url: %v\n", err)
		os.Exit(1)
	}
	testRDB = goredis.NewClient(opts)

	code := m.Run()

	pool.Close()
	testRDB.Close()
	os.Exit(code)
}

func TestCreateAndGetEvent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	endpoint, _, err := testDB.CreateEndpoint(ctx, "https://example.com/webhook", "", "", nil)
	if err != nil {
		t.Fatalf("create endpoint: %v", err)
	}

	event, err := testDB.CreateEvent(ctx, CreateEventParams{
		EndpointID: endpoint.ID,
		Payload:    []byte(`{"msg":"hello"}`),
	})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}

	if event.ID == "" {
		t.Fatal("expected non-empty event id")
	}
	if event.Status != "pending" {
		t.Fatalf("expected status pending, got %s", event.Status)
	}

	got, err := testDB.GetEvent(ctx, event.ID)
	if err != nil {
		t.Fatalf("get event: %v", err)
	}
	if got.ID != event.ID {
		t.Fatalf("event id mismatch: %s vs %s", got.ID, event.ID)
	}
}

func TestRetryAndDeadLetter(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	endpoint, _, err := testDB.CreateEndpoint(ctx, "https://example.com/dlq-test", "", "", nil)
	if err != nil {
		t.Fatalf("create endpoint: %v", err)
	}

	event, err := testDB.CreateEvent(ctx, CreateEventParams{
		EndpointID: endpoint.ID,
		Payload:    []byte(`{"test":"dlq"}`),
		MaxRetries: 2,
	})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}

	for i := 0; i < 3; i++ {
		attempts := i + 1
		if attempts >= event.MaxRetries {
			testDB.UpdateEventStatus(ctx, event.ID, "dead")
		} else {
			nextRetry := futureTime(1 * i)
			testDB.RecordAttempt(ctx, event.ID, attempts, "retrying", &nextRetry)
		}
	}

	got, err := testDB.GetEvent(ctx, event.ID)
	if err != nil {
		t.Fatalf("get event: %v", err)
	}
	if got.Status != "dead" {
		t.Fatalf("expected status dead, got %s", got.Status)
	}
}

func TestDeliveryAttemptLog(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	endpoint, _, err := testDB.CreateEndpoint(ctx, "https://example.com/attempt-test", "", "", nil)
	if err != nil {
		t.Fatalf("create endpoint: %v", err)
	}

	event, err := testDB.CreateEvent(ctx, CreateEventParams{
		EndpointID: endpoint.ID,
		Payload:    []byte(`{"test":"attempt-log"}`),
	})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}

	statusCode := 200
	err = testDB.CreateAttempt(ctx, event.ID, 1, &statusCode, nil, nil, 45)
	if err != nil {
		t.Fatalf("create attempt: %v", err)
	}

	attempts, err := testDB.ListAttempts(ctx, event.ID)
	if err != nil {
		t.Fatalf("list attempts: %v", err)
	}
	if len(attempts) != 1 {
		t.Fatalf("expected 1 attempt, got %d", len(attempts))
	}
	if *attempts[0].StatusCode != 200 {
		t.Fatalf("expected status 200, got %d", *attempts[0].StatusCode)
	}
	if attempts[0].DurationMs != 45 {
		t.Fatalf("expected duration 45, got %d", attempts[0].DurationMs)
	}
}

func TestListEndpoints(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	urls := []string{
		"https://first.example.com/webhook",
		"https://second.example.com/webhook",
		"https://third.example.com/webhook",
	}
	for _, url := range urls {
		_, _, err := testDB.CreateEndpoint(ctx, url, "", "", nil)
		if err != nil {
			t.Fatalf("create endpoint %s: %v", url, err)
		}
	}

	endpoints, err := testDB.ListEndpoints(ctx)
	if err != nil {
		t.Fatalf("list endpoints: %v", err)
	}
	if len(endpoints) < 3 {
		t.Fatalf("expected at least 3 endpoints, got %d", len(endpoints))
	}

	found := false
	for _, ep := range endpoints {
		if ep.URL == "https://first.example.com/webhook" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected first endpoint url in list")
	}
}

func futureTime(seconds int) time.Time {
	return time.Now().Add(time.Duration(seconds) * time.Second)
}
