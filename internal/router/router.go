package router

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/prateekkhurmi/hookforge/internal/config"
	"github.com/prateekkhurmi/hookforge/internal/database"
	"github.com/prateekkhurmi/hookforge/internal/handler"
	"github.com/prateekkhurmi/hookforge/internal/redis"
)

func Setup(db *database.DB, rdb *redis.Client, cfg *config.Config) http.Handler {
	r := gin.Default()

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	ep := handler.NewEndpointHandler(db)
	ev := handler.NewEventHandler(db, rdb)
	st := handler.NewStatsHandler(db)

	api := r.Group("/api/v1")
	{
		api.POST("/endpoints", ep.Create)
		api.POST("/events", ev.Create)
		api.GET("/events", ev.List)
		api.POST("/events/:id/replay", ev.Replay)
		api.GET("/stats", st.Get)
	}

	return r
}
