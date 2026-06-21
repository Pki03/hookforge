package dashboard

import (
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prateekkhurmi/hookforge/internal/database"
)

type Handler struct {
	db *database.DB
}

func NewHandler(db *database.DB) *Handler {
	return &Handler{db: db}
}

func (h *Handler) Page(c *gin.Context) {
	c.HTML(http.StatusOK, "dashboard.html", nil)
}

func (h *Handler) StatsPanel(c *gin.Context) {
	stats, err := h.db.GetStats(c.Request.Context())
	if err != nil {
		c.String(http.StatusInternalServerError, "error loading stats")
		return
	}
	c.HTML(http.StatusOK, "_stats.html", stats)
}

func (h *Handler) EventDetail(c *gin.Context) {
	eventID := c.Param("id")
	event, err := h.db.GetEvent(c.Request.Context(), eventID)
	if err != nil {
		log.Printf("event detail error: %v", err)
		c.String(http.StatusNotFound, "Event not found")
		return
	}

	attempts, err := h.db.ListAttempts(c.Request.Context(), eventID)
	if err != nil {
		log.Printf("list attempts error: %v", err)
		attempts = []database.DeliveryAttempt{}
	}

	if attempts == nil {
		attempts = []database.DeliveryAttempt{}
	}

	c.HTML(http.StatusOK, "_event_detail.html", gin.H{
		"event":    event,
		"attempts": attempts,
	})
}

func (h *Handler) EventsPanel(c *gin.Context) {
	events, err := h.db.ListEvents(c.Request.Context(), "", 20)
	if err != nil {
		c.String(http.StatusInternalServerError, "error loading events")
		return
	}
	c.HTML(http.StatusOK, "_events.html", gin.H{
		"events": events,
		"now":    time.Now(),
	})
}
