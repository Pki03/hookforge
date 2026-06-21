package router

import (
	"fmt"
	"html/template"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prateekkhurmi/hookforge/internal/config"
	"github.com/prateekkhurmi/hookforge/internal/dashboard"
	"github.com/prateekkhurmi/hookforge/internal/database"
	"github.com/prateekkhurmi/hookforge/internal/handler"
	"github.com/prateekkhurmi/hookforge/internal/redis"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func Setup(db *database.DB, rdb *redis.Client, cfg *config.Config) http.Handler {
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	r.SetFuncMap(template.FuncMap{
		"shortID": func(s string) string {
			if len(s) > 8 {
				return s[:8]
			}
			return s
		},
		"since": func(t time.Time) string {
			d := time.Since(t)
			if d < time.Minute {
				return "just now"
			}
			if d < time.Hour {
				m := int(d.Minutes())
				return formatDuration(m, "minute")
			}
			if d < 24*time.Hour {
				h := int(d.Hours())
				return formatDuration(h, "hour")
			}
			d2 := int(d.Hours() / 24)
			return formatDuration(d2, "day")
		},
	})
	r.LoadHTMLGlob("templates/*.html")

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

	dh := dashboard.NewHandler(db)
	r.GET("/dashboard", dh.Page)
	r.GET("/api/v1/dashboard/stats", dh.StatsPanel)
	r.GET("/api/v1/dashboard/events", dh.EventsPanel)

	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	return r
}

func formatDuration(n int, unit string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s ago", unit)
	}
	return fmt.Sprintf("%d %ss ago", n, unit)
}
