package handler

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/prateekkhurmi/hookforge/internal/database"
	"github.com/prateekkhurmi/hookforge/internal/redis"
)

type EndpointHandler struct {
	db *database.DB
}

func NewEndpointHandler(db *database.DB) *EndpointHandler {
	return &EndpointHandler{db: db}
}

type createEndpointReq struct {
	URL             string `json:"url" binding:"required"`
	SlackWebhookURL string `json:"slack_webhook_url"`
}

func (h *EndpointHandler) Create(c *gin.Context) {
	var req createEndpointReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	endpoint, secret, err := h.db.CreateEndpoint(c.Request.Context(), req.URL, req.SlackWebhookURL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, database.EndpointResponse{
		ID:                 endpoint.ID,
		URL:                endpoint.URL,
		Secret:             secret,
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
	Payload    json.RawMessage `json:"payload" binding:"required"`
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

	event, err := h.db.CreateEvent(c.Request.Context(), database.CreateEventParams{
		EndpointID: req.EndpointID,
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
	events, err := h.db.ListEvents(c.Request.Context(), status, 50)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if events == nil {
		events = []database.Event{}
	}
	c.JSON(http.StatusOK, events)
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
