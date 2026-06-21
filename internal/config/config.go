package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port          string
	DatabaseURL   string
	RedisURL      string
	SigningSecret string
	WorkerCount   int
}

func Load() *Config {
	return &Config{
		Port:          getEnv("PORT", "8080"),
		DatabaseURL:   getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/hookforge?sslmode=disable"),
		RedisURL:      getEnv("REDIS_URL", "redis://localhost:6379/0"),
		SigningSecret: getEnv("SIGNING_SECRET", "hookforge-dev-secret"),
		WorkerCount:   getEnvInt("WORKER_COUNT", 5),
	}
}

func getEnvInt(key string, fallback int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return fallback
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
