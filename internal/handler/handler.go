package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"
	"github.com/prateekkhurmi/hookforge/internal/database"
	"github.com/prateekkhurmi/hookforge/internal/redis"
)

func validateTargetURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("unsupported scheme %s", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("missing host")
	}
	return nil
}

type EndpointHandler struct {
	db *database.DB
}

func NewEndpointHandler(db *database.DB) *EndpointHandler {
	return &EndpointHandler{db: db}
}

type createEndpointReq struct {
	URL                string   `json:"url" binding:"required"`
	SlackWebhookURL    string   `json:"slack_webhook_url"`
	Email              string   `json:"email"`
	AllowedEventTypes  []string `json:"allowed_event_types"`
	RateLimitPerSecond int      `json:"rate_limit_per_second"`
	RateLimitBurst     int      `json:"rate_limit_burst"`
}

func (h *EndpointHandler) Create(c *gin.Context) {
	var req createEndpointReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := validateTargetURL(req.URL); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "invalid target URL: must be http or https with a valid host"})
		return
	}

	rps := req.RateLimitPerSecond
	if rps <= 0 {
		rps = 10
	}
	burst := req.RateLimitBurst
	if burst <= 0 {
		burst = 20
	}
	endpoint, secret, err := h.db.CreateEndpoint(c.Request.Context(), req.URL, req.SlackWebhookURL, req.Email, req.AllowedEventTypes, rps, burst)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, database.EndpointResponse{
		ID:                 endpoint.ID,
		URL:                endpoint.URL,
		Secret:             secret,
		AllowedEventTypes:  endpoint.AllowedEventTypes,
		RateLimitPerSecond: endpoint.RateLimitPerSecond,
		RateLimitBurst:     endpoint.RateLimitBurst,
		CreatedAt:          endpoint.CreatedAt,
		UpdatedAt:          endpoint.UpdatedAt,
	})
}

func (h *EndpointHandler) RotateSecret(c *gin.Context) {
	id := c.Param("id")
	secret, err := h.db.RotateEndpointSecret(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id, "secret": secret})
}

func (h *EndpointHandler) Get(c *gin.Context) {
	id := c.Param("id")
	endpoint, err := h.db.GetEndpoint(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if endpoint == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "endpoint not found"})
		return
	}
	c.JSON(http.StatusOK, database.EndpointResponse{
		ID:                 endpoint.ID,
		URL:                endpoint.URL,
		AllowedEventTypes:  endpoint.AllowedEventTypes,
		RateLimitPerSecond: endpoint.RateLimitPerSecond,
		RateLimitBurst:     endpoint.RateLimitBurst,
		CreatedAt:          endpoint.CreatedAt,
		UpdatedAt:          endpoint.UpdatedAt,
	})
}

type EventHandler struct {
	db  *database.DB
	rdb *redis.Client
}

func NewEventHandler(db *database.DB, rdb *redis.Client) *EventHandler {
	return &EventHandler{db: db, rdb: rdb}
}

type createEventReq struct {
	EndpointID string          `json:"endpoint_id" binding:"required"`
	EventType  string          `json:"event_type"`
	Payload    json.RawMessage `json:"payload" binding:"required"`
}

func (h *EndpointHandler) List(c *gin.Context) {
	endpoints, err := h.db.ListEndpoints(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var resp []database.EndpointResponse
	for _, ep := range endpoints {
		allowed := ep.AllowedEventTypes
		if allowed == nil {
			allowed = []string{}
		}
		resp = append(resp, database.EndpointResponse{
			ID:                 ep.ID,
			URL:                ep.URL,
			AllowedEventTypes:  allowed,
			RateLimitPerSecond: ep.RateLimitPerSecond,
			RateLimitBurst:     ep.RateLimitBurst,
			CreatedAt:          ep.CreatedAt,
			UpdatedAt:          ep.UpdatedAt,
		})
	}
	if resp == nil {
		resp = []database.EndpointResponse{}
	}
	c.JSON(http.StatusOK, resp)
}

func (h *EventHandler) Create(c *gin.Context) {
	var req createEventReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	endpoint, err := h.db.GetEndpoint(c.Request.Context(), req.EndpointID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if endpoint == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "endpoint not found"})
		return
	}

	if len(endpoint.AllowedEventTypes) > 0 {
		allowed := false
		for _, t := range endpoint.AllowedEventTypes {
			if t == req.EventType {
				allowed = true
				break
			}
		}
		if !allowed {
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"error":              "event_type not allowed for this endpoint",
				"allowed_event_types": endpoint.AllowedEventTypes,
			})
			return
		}
	}

	event, err := h.db.CreateEvent(c.Request.Context(), database.CreateEventParams{
		EndpointID: req.EndpointID,
		EventType:  req.EventType,
		Payload:    req.Payload,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := h.rdb.EnqueueEvent(c.Request.Context(), event.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "event created but failed to enqueue"})
		return
	}

	c.JSON(http.StatusCreated, event)
}

func (h *EventHandler) List(c *gin.Context) {
	status := c.Query("status")
	eventType := c.Query("event_type")
	events, err := h.db.ListEvents(c.Request.Context(), status, 50)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if events == nil {
		events = []database.Event{}
	}

	if eventType != "" {
		var filtered []database.Event
		for _, e := range events {
			if e.EventType == eventType {
				filtered = append(filtered, e)
			}
		}
		events = filtered
	}

	c.JSON(http.StatusOK, events)
}

func (h *EventHandler) Get(c *gin.Context) {
	eventID := c.Param("id")
	event, err := h.db.GetEvent(c.Request.Context(), eventID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "event not found"})
		return
	}

	attempts, err := h.db.ListAttempts(c.Request.Context(), eventID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if attempts == nil {
		attempts = []database.DeliveryAttempt{}
	}

	c.JSON(http.StatusOK, gin.H{
		"event":    event,
		"attempts": attempts,
	})
}

func (h *EventHandler) Replay(c *gin.Context) {
	eventID := c.Param("id")
	event, err := h.db.GetEvent(c.Request.Context(), eventID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := h.db.UpdateEventStatus(c.Request.Context(), eventID, "pending"); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := h.rdb.EnqueueEvent(c.Request.Context(), event.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "replay failed to enqueue"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "event re-enqueued", "id": eventID})
}

type StatsHandler struct {
	db *database.DB
}

func NewStatsHandler(db *database.DB) *StatsHandler {
	return &StatsHandler{db: db}
}

func (h *StatsHandler) Get(c *gin.Context) {
	stats, err := h.db.GetStats(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, stats)
}
