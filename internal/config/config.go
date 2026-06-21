package config

import "os"

type Config struct {
	Port          string
	DatabaseURL   string
	RedisURL      string
	SigningSecret string
}

func Load() *Config {
	return &Config{
		Port:          getEnv("PORT", "8080"),
		DatabaseURL:   getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/hookforge?sslmode=disable"),
		RedisURL:      getEnv("REDIS_URL", "redis://localhost:6379/0"),
		SigningSecret: getEnv("SIGNING_SECRET", "hookforge-dev-secret"),
	}
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
