package router

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/prateekkhurmi/hookforge/internal/config"
	"github.com/prateekkhurmi/hookforge/internal/database"
)

func Setup(db *database.DB, cfg *config.Config) http.Handler {
	r := gin.Default()

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	api := r.Group("/api/v1")
	{
		api.POST("/endpoints", createEndpoint(db))
		api.POST("/events", createEvent(db))
		api.GET("/stats", getStats(db))
		api.GET("/events", listEvents(db))
		api.POST("/events/:id/replay", replayEvent(db))
		api.GET("/metrics", getMetrics())
	}

	return r
}

func createEndpoint(db *database.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusNotImplemented, gin.H{"message": "not implemented"})
	}
}

func createEvent(db *database.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusNotImplemented, gin.H{"message": "not implemented"})
	}
}

func getStats(db *database.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusNotImplemented, gin.H{"message": "not implemented"})
	}
}

func listEvents(db *database.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusNotImplemented, gin.H{"message": "not implemented"})
	}
}

func replayEvent(db *database.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusNotImplemented, gin.H{"message": "not implemented"})
	}
}

func getMetrics() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusNotImplemented, gin.H{"message": "not implemented"})
	}
}
