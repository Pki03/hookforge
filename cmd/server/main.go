package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/prateekkhurmi/hookforge/internal/config"
	"github.com/prateekkhurmi/hookforge/internal/database"
	"github.com/prateekkhurmi/hookforge/internal/metrics"
	"github.com/prateekkhurmi/hookforge/internal/redis"
	"github.com/prateekkhurmi/hookforge/internal/router"
	"github.com/prateekkhurmi/hookforge/worker"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	cfg := config.Load()

	db, err := database.Connect(cfg.DatabaseURL)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	runMigrations(cfg.DatabaseURL)

	rdb, err := redis.Connect(cfg.RedisURL)
	if err != nil {
		slog.Error("failed to connect to redis", "error", err)
		os.Exit(1)
	}
	defer rdb.Close()

	initMetrics()
	w := worker.New(db, rdb, cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.Start(ctx)

	r := router.Setup(db, rdb, cfg)

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: r,
	}

	go func() {
		slog.Info("server starting", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("shutting down gracefully...")
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("server shutdown error", "error", err)
	}
	slog.Info("server stopped")
}

func runMigrations(databaseURL string) {
	m, err := migrate.New("file://db/migrations", databaseURL)
	if err != nil {
		slog.Error("migration init", "error", err)
		os.Exit(1)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		slog.Error("migration up", "error", err)
		os.Exit(1)
	}
	slog.Info("migrations applied")
}

func initMetrics() {
	metrics.EventsTotal.WithLabelValues("pending")
	metrics.EventsTotal.WithLabelValues("delivered")
	metrics.EventsTotal.WithLabelValues("failed")
	metrics.EventsTotal.WithLabelValues("dead")
}
