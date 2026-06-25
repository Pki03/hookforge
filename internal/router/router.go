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

	r.Use(
		gin.Logger(),
		gin.Recovery(),
		middleware.SecurityHeaders(),
		middleware.CORS(cfg.AllowedOrigins),
		middleware.BodySizeLimit(cfg.MaxBodyBytes),
	)

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
		if err := db.Ping(c.Request.Context()); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unhealthy", "error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	r.GET("/ready", func(c *gin.Context) {
		dbErr := db.Ping(c.Request.Context())
		redisPing := rdb.Ping(c.Request.Context())
		redisErr := redisPing.Err()
		if dbErr != nil || redisErr != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status":  "unhealthy",
				"db":      dbErr == nil,
				"redis":   redisErr == nil,
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok", "db": true, "redis": true})
	})

	ep := handler.NewEndpointHandler(db)
	ev := handler.NewEventHandler(db, rdb)
	st := handler.NewStatsHandler(db)

	rl := ratelimit.New(rdb)

	api := r.Group("/api/v1", middleware.AdminAuth(cfg.AdminAPIKey))
	{
		api.GET("/endpoints", ep.List)
		api.POST("/endpoints", ep.Create)
		api.GET("/endpoints/:id", ep.Get)
		api.POST("/endpoints/:id/rotate-secret", ep.RotateSecret)

		events := api.Group("/events")
		events.Use(middleware.RateLimit(db, rl))
		{
			api.GET("/events/:id", ev.Get)
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

	ws := dashboard.NewWSHandler(db, cfg.AllowedOrigins)
	r.GET("/api/v1/ws", gin.WrapH(http.HandlerFunc(ws.Serve)))
	r.GET("/dashboard/events/:id", dh.EventDetail)

	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	r.StaticFile("/api/v1/openapi.json", "api/openapi.json")
	r.GET("/api/docs", func(c *gin.Context) {
		c.HTML(http.StatusOK, "swagger.html", nil)
	})

	return r
}
