package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/prateekkhurmi/hookforge/internal/config"
	"github.com/prateekkhurmi/hookforge/internal/database"
	"github.com/prateekkhurmi/hookforge/internal/redis"
	"github.com/prateekkhurmi/hookforge/internal/router"
	"github.com/prateekkhurmi/hookforge/worker"
)

func main() {
	cfg := config.Load()

	db, err := database.Connect(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer db.Close()

	runMigrations(cfg.DatabaseURL)

	rdb, err := redis.Connect(cfg.RedisURL)
	if err != nil {
		log.Fatalf("failed to connect to redis: %v", err)
	}
	defer rdb.Close()

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
		log.Printf("HookForge server starting on :%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down gracefully...")
	cancel()
}

func runMigrations(databaseURL string) {
	m, err := migrate.New("file://db/migrations", databaseURL)
	if err != nil {
		log.Fatalf("migration init: %v", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		log.Fatalf("migration up: %v", err)
	}
	log.Println("migrations applied")
}
