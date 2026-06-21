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
	"github.com/prateekkhurmi/hookforge/internal/middleware"
	"github.com/prateekkhurmi/hookforge/internal/ratelimit"
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
				return fmt.Sprintf("%d minutes ago", int(d.Minutes()))
			}
			if d < 24*time.Hour {
				return fmt.Sprintf("%d hours ago", int(d.Hours()))
			}
			return fmt.Sprintf("%d days ago", int(d.Hours()/24))
		},
		"json": func(v interface{}) template.JS {
			return template.JS(fmt.Sprintf("%v", v))
		},
	})
	r.LoadHTMLGlob("templates/*.html")

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	ep := handler.NewEndpointHandler(db)
	ev := handler.NewEventHandler(db, rdb)
	st := handler.NewStatsHandler(db)

	rl := ratelimit.New(rdb)

	api := r.Group("/api/v1")
	{
		api.POST("/endpoints", ep.Create)
		api.GET("/endpoints/:id", ep.Get)
		api.POST("/endpoints/:id/rotate-secret", ep.RotateSecret)

		events := api.Group("/events")
		events.Use(middleware.RateLimit(db, rl))
		{
			events.POST("", ev.Create)
			events.GET("", ev.List)
			events.POST("/:id/replay", ev.Replay)
		}

		api.GET("/stats", st.Get)
	}

	dh := dashboard.NewHandler(db)
	r.GET("/dashboard", dh.Page)
	r.GET("/api/v1/dashboard/stats", dh.StatsPanel)
	r.GET("/api/v1/dashboard/events", dh.EventsPanel)

	ws := dashboard.NewWSHandler(db)
	r.GET("/api/v1/ws", gin.WrapH(http.HandlerFunc(ws.Serve)))

	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	return r
}
