package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prateekkhurmi/hookforge/internal/database"
	"github.com/prateekkhurmi/hookforge/internal/handler"
	"github.com/prateekkhurmi/hookforge/internal/middleware"
	"github.com/prateekkhurmi/hookforge/internal/ratelimit"
)

func TestConcurrentEventIngestion(t *testing.T) {
	skipIfNoIntegration(t)
	db := integrationDB(t)
	rdb := integrationRedis(t)

	ctx := context.Background()
	endpoint, _, err := db.CreateEndpoint(ctx, "https://httpbin.org/post", "", "", nil, 10000, 20000)
	if err != nil {
		t.Fatalf("create endpoint: %v", err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	eh := handler.NewEventHandler(db, rdb)
	router.POST("/api/v1/events", eh.Create)

	concurrency := 50
	eventsPerGoroutine := 20
	var succeeded, failed atomic.Int32
	var wg sync.WaitGroup

	for i := range concurrency {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < eventsPerGoroutine; j++ {
				body, _ := json.Marshal(map[string]any{
					"endpoint_id": endpoint.ID,
					"payload":     map[string]any{"seq": id*eventsPerGoroutine + j, "ts": "concurrent_test"},
				})
				req := httptest.NewRequest("POST", "/api/v1/events", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)
				if w.Code == http.StatusCreated {
					succeeded.Add(1)
				} else {
					failed.Add(1)
				}
			}
		}(i)
	}
	wg.Wait()

	t.Logf("concurrent events: %d succeeded, %d failed", succeeded.Load(), failed.Load())
	if failed.Load() > 0 {
		t.Errorf("expected 0 failures, got %d", failed.Load())
	}
	if succeeded.Load() == 0 {
		t.Fatal("expected at least 1 success")
	}
}

func TestConcurrentEndpointCreation(t *testing.T) {
	skipIfNoIntegration(t)
	db := integrationDB(t)
	integrationRedis(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	ep := handler.NewEndpointHandler(db)
	router.POST("/api/v1/endpoints", ep.Create)

	concurrency := 30
	var succeeded, failed atomic.Int32
	var wg sync.WaitGroup

	for i := range concurrency {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			body, _ := json.Marshal(map[string]any{
				"url": "https://example.com/hook/" + itoa(id),
			})
			req := httptest.NewRequest("POST", "/api/v1/endpoints", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code == http.StatusCreated {
				succeeded.Add(1)
			} else {
				failed.Add(1)
			}
		}(i)
	}
	wg.Wait()

	t.Logf("concurrent endpoints: %d succeeded, %d failed", succeeded.Load(), failed.Load())
	if failed.Load() > 0 {
		t.Errorf("expected 0 failures, got %d", failed.Load())
	}
}

func TestConcurrentReplayDuringIngestion(t *testing.T) {
	skipIfNoIntegration(t)
	db := integrationDB(t)
	rdb := integrationRedis(t)

	ctx := context.Background()
	endpoint, _, err := db.CreateEndpoint(ctx, "https://httpbin.org/post", "", "", nil, 10000, 20000)
	if err != nil {
		t.Fatalf("create endpoint: %v", err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	eh := handler.NewEventHandler(db, rdb)
	router.POST("/api/v1/events", eh.Create)
	router.POST("/api/v1/events/:id/replay", eh.Replay)

	preCreated := make([]string, 10)
	for i := range preCreated {
		event, err := db.CreateEvent(ctx, database.CreateEventParams{
			EndpointID: endpoint.ID,
			EventType:  "",
			Payload:    json.RawMessage(`{"pre":true,"seq":` + itoa(i) + `}`),
		})
		if err != nil {
			t.Fatalf("pre-create event: %v", err)
		}
		db.UpdateEventStatus(ctx, event.ID, "dead")
		preCreated[i] = event.ID
	}

	var wg sync.WaitGroup
	var replaySucceeded, replayFailed, ingestSucceeded, ingestFailed atomic.Int32

	for _, id := range preCreated {
		wg.Add(1)
		go func(eventID string) {
			defer wg.Done()
			body, _ := json.Marshal(map[string]any{"event_id": eventID})
			req := httptest.NewRequest("POST", "/api/v1/events/"+eventID+"/replay", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code == http.StatusOK {
				replaySucceeded.Add(1)
			} else {
				replayFailed.Add(1)
			}
		}(id)
	}

	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			body, _ := json.Marshal(map[string]any{
				"endpoint_id": endpoint.ID,
				"payload":     map[string]any{"ts": "replay_ingest_concurrent"},
			})
			req := httptest.NewRequest("POST", "/api/v1/events", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code == http.StatusCreated {
				ingestSucceeded.Add(1)
			} else {
				ingestFailed.Add(1)
			}
		}()
	}

	wg.Wait()
	t.Logf("replay: %d ok, %d fail | ingest: %d ok, %d fail",
		replaySucceeded.Load(), replayFailed.Load(),
		ingestSucceeded.Load(), ingestFailed.Load())

	if replayFailed.Load() > 0 {
		t.Errorf("replay failures: %d", replayFailed.Load())
	}
	if ingestFailed.Load() > 0 {
		t.Errorf("ingest failures: %d", ingestFailed.Load())
	}
}

func TestMixedReadWriteWorkload(t *testing.T) {
	skipIfNoIntegration(t)
	db := integrationDB(t)
	rdb := integrationRedis(t)

	ctx := context.Background()
	endpoint, _, err := db.CreateEndpoint(ctx, "https://httpbin.org/post", "", "", nil, 10000, 20000)
	if err != nil {
		t.Fatalf("create endpoint: %v", err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	eh := handler.NewEventHandler(db, rdb)
	sh := handler.NewStatsHandler(db)
	router.POST("/api/v1/events", eh.Create)
	router.GET("/api/v1/stats", sh.Get)
	router.GET("/api/v1/events", eh.List)
	router.GET("/api/v1/events/:id", eh.Get)

	var wg sync.WaitGroup
	var ops atomic.Int32

	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 30 {
				body, _ := json.Marshal(map[string]any{
					"endpoint_id": endpoint.ID,
					"payload":     map[string]any{"ts": "mixed_workload"},
				})
				req := httptest.NewRequest("POST", "/api/v1/events", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)
				if w.Code == http.StatusCreated {
					ops.Add(1)
				}
			}
		}()
	}

	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 10 {
				req := httptest.NewRequest("GET", "/api/v1/stats", nil)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)
				if w.Code == http.StatusOK {
					ops.Add(1)
				}
			}
		}()
	}

	for range 3 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 5 {
				req := httptest.NewRequest("GET", "/api/v1/events", nil)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)
				if w.Code == http.StatusOK {
					ops.Add(1)
				}
			}
		}()
	}

	wg.Wait()
	t.Logf("mixed workload completed: %d ops", ops.Load())
	if ops.Load() == 0 {
		t.Fatal("expected at least 1 op to succeed")
	}
}

func TestChaosMalformedPayloads(t *testing.T) {
	skipIfNoIntegration(t)
	db := integrationDB(t)
	rdb := integrationRedis(t)

	ctx := context.Background()
	endpoint, _, err := db.CreateEndpoint(ctx, "https://httpbin.org/post", "", "", nil, 10, 20)
	if err != nil {
		t.Fatalf("create endpoint: %v", err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	eh := handler.NewEventHandler(db, rdb)
	router.POST("/api/v1/events", eh.Create)

	tests := []struct {
		name       string
		payload    any
		wantStatus int
	}{
		{"empty endpoint_id", map[string]any{"endpoint_id": "", "payload": map[string]any{"x": 1}}, http.StatusBadRequest},
		{"nil payload", map[string]any{"endpoint_id": endpoint.ID, "payload": nil}, http.StatusCreated},
		{"missing fields", map[string]any{}, http.StatusBadRequest},
		{"wrong type for payload", "not an object", http.StatusBadRequest},
		{"unknown endpoint", map[string]any{"endpoint_id": "00000000-0000-0000-0000-000000000000", "payload": map[string]any{"x": 1}}, http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := json.Marshal(tt.payload)
			if err != nil {
				t.Skip("marshal error:", err)
			}
			req := httptest.NewRequest("POST", "/api/v1/events", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != tt.wantStatus {
				t.Errorf("%s: got status %d, want %d", tt.name, w.Code, tt.wantStatus)
			}
		})
	}
}

func TestChaosRateLimiterUnderExtremeLoad(t *testing.T) {
	skipIfNoIntegration(t)
	db := integrationDB(t)
	rdb := integrationRedis(t)

	ctx := context.Background()
	endpoint, _, err := db.CreateEndpoint(ctx, "https://httpbin.org/post", "", "", nil, 5, 10)
	if err != nil {
		t.Fatalf("create endpoint: %v", err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	eh := handler.NewEventHandler(db, rdb)
	rl := ratelimit.New(rdb)
	router.POST("/api/v1/events", middleware.RateLimit(db, rl), eh.Create)

	var accepted, rejected atomic.Int32
	var wg sync.WaitGroup

	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 50 {
				body, _ := json.Marshal(map[string]any{
					"endpoint_id": endpoint.ID,
					"payload":     map[string]any{"x": rand.Int()},
				})
				req := httptest.NewRequest("POST", "/api/v1/events", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)
				if w.Code == http.StatusCreated {
					accepted.Add(1)
				} else if w.Code == http.StatusTooManyRequests {
					rejected.Add(1)
				}
			}
		}()
	}
	wg.Wait()
	t.Logf("rate limited endpoint: %d accepted, %d rejected (limit 5/s burst 10)", accepted.Load(), rejected.Load())
	if accepted.Load() == 0 {
		t.Fatal("expected at least some accepted requests")
	}
	if rejected.Load() == 0 {
		t.Fatal("expected rate limiter to reject some requests")
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}


