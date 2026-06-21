package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/prateekkhurmi/hookforge/internal/database"
	"github.com/prateekkhurmi/hookforge/internal/ratelimit"
)

func RateLimit(db *database.DB, rl *ratelimit.Limiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.Next()
			return
		}
		c.Request.Body = io.NopCloser(bytes.NewReader(body))

		var payload struct {
			EndpointID string `json:"endpoint_id"`
		}
		if err := json.Unmarshal(body, &payload); err != nil || payload.EndpointID == "" {
			c.Next()
			return
		}

		ep, err := db.GetEndpoint(c.Request.Context(), payload.EndpointID)
		if err != nil || ep == nil {
			c.Next()
			return
		}

		allowed, err := rl.Allow(c.Request.Context(), payload.EndpointID, ep.RateLimitPerSecond, ep.RateLimitBurst)
		if err != nil || !allowed {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":       "rate limit exceeded",
				"retry_after": "1s",
			})
			return
		}

		c.Next()
	}
}
