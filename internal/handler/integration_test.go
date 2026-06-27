package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/prateekkhurmi/hookforge/internal/database"
	"github.com/prateekkhurmi/hookforge/internal/handler"
	"github.com/prateekkhurmi/hookforge/internal/redis"
)

func skipIfNoIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("DATABASE_URL") == "" && os.Getenv("INTEGRATION") != "true" {
		t.Skip("set INTEGRATION=true or DATABASE_URL to run integration tests")
	}
}

func integrationDB(t *testing.T) *database.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://postgres:postgres@localhost:5432/hookforge?sslmode=disable"
	}
	db, err := database.Connect(dsn)
	if err != nil {
		t.Skipf("database not available: %v", err)
		return nil
	}
	t.Cleanup(func() { db.Close() })

	m, err := migrate.New("file://../../db/migrations", dsn)
	if err != nil {
		t.Skipf("migration init: %v", err)
		return nil
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Skipf("migration up: %v", err)
		return nil
	}
	return db
}

func integrationRedis(t *testing.T) *redis.Client {
	t.Helper()
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:6379/0"
	}
	rdb, err := redis.Connect(redisURL)
	if err != nil {
		t.Skipf("redis not available: %v", err)
		return nil
	}
	t.Cleanup(func() { rdb.Close() })
	return rdb
}

func TestCreateEndpoint(t *testing.T) {
	skipIfNoIntegration(t)
	db := integrationDB(t)

	h := handler.NewEndpointHandler(db)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	body := bytes.NewReader([]byte(`{"url": "https://example.com/webhook"}`))
	c.Request = httptest.NewRequest(http.MethodPost, "/", body)
	c.Request.Header.Set("Content-Type", "application/json")

	h.Create(c)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["url"] != "https://example.com/webhook" {
		t.Errorf("expected url, got %v", resp["url"])
	}
	if resp["id"] == "" {
		t.Errorf("expected id, got empty")
	}
}

func TestCreateEvent(t *testing.T) {
	skipIfNoIntegration(t)
	db := integrationDB(t)
	rdb := integrationRedis(t)

	eh := handler.NewEndpointHandler(db)
	ev := handler.NewEventHandler(db, rdb)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(`{"url": "https://example.com/webhook"}`)))
	c.Request.Header.Set("Content-Type", "application/json")
	eh.Create(c)

	var endpoint map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &endpoint); err != nil {
		t.Fatalf("unmarshal endpoint: %v", err)
	}

	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	payload := `{"endpoint_id": "` + endpoint["id"].(string) + `", "payload": {"event": "test"}}`
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(payload)))
	c.Request.Header.Set("Content-Type", "application/json")
	ev.Create(c)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var eventResp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &eventResp); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}
	if eventResp["status"] != "pending" {
		t.Errorf("expected pending status, got %v", eventResp["status"])
	}
}

func TestStats(t *testing.T) {
	skipIfNoIntegration(t)
	db := integrationDB(t)

	h := handler.NewStatsHandler(db)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	h.Get(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var stats map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &stats); err != nil {
		t.Fatalf("unmarshal stats: %v", err)
	}
	if _, ok := stats["total_sent"]; !ok {
		t.Errorf("expected total_sent in stats")
	}
}

func TestDBOperations(t *testing.T) {
	skipIfNoIntegration(t)
	db := integrationDB(t)
	ctx := context.Background()

	endpoint, _, err := db.CreateEndpoint(ctx, "https://httpbin.org/post", "", "", nil, 10, 20)
	if err != nil {
		t.Fatalf("create endpoint: %v", err)
	}
	if endpoint.URL != "https://httpbin.org/post" {
		t.Errorf("expected url, got %s", endpoint.URL)
	}

	event, err := db.CreateEvent(ctx, database.CreateEventParams{
		EndpointID: endpoint.ID,
		Payload:    []byte(`{"msg": "hello"}`),
		MaxRetries: 3,
	})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	if event.Status != "pending" {
		t.Errorf("expected pending, got %s", event.Status)
	}

	got, err := db.GetEvent(ctx, event.ID)
	if err != nil {
		t.Fatalf("get event: %v", err)
	}
	if got.ID != event.ID {
		t.Errorf("event id mismatch")
	}

	events, err := db.ListEvents(ctx, "", 10)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	found := false
	for _, e := range events {
		if e.ID == event.ID {
			found = true
			break
		}
	}
	if !found {
		t.Error("our event not found in list")
	}

	stats, err := db.GetStats(ctx)
	if err != nil {
		t.Fatalf("get stats: %v", err)
	}
	if stats.TotalSent < 1 {
		t.Errorf("expected at least 1 total, got %d", stats.TotalSent)
	}
}
