package config

import (
	"os"
	"strconv"

	"github.com/prateekkhurmi/hookforge/internal/notifier"
)

type Config struct {
	Port          string
	DatabaseURL   string
	RedisURL      string
	SigningSecret string
	WorkerCount   int
	SMTPHost      string
	SMTPPort      string
	SMTPUser      string
	SMTPPassword  string
	SMTPFrom      string
	AdminAPIKey   string
	AllowedOrigins string
	MaxBodyBytes  int64
}

func Load() *Config {
	return &Config{
		Port:           getEnv("PORT", "8080"),
		DatabaseURL:    getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/hookforge?sslmode=disable"),
		RedisURL:       getEnv("REDIS_URL", "redis://localhost:6379/0"),
		SigningSecret:  getEnv("SIGNING_SECRET", "hookforge-dev-secret"),
		WorkerCount:    getEnvInt("WORKER_COUNT", 5),
		SMTPHost:       getEnv("SMTP_HOST", ""),
		SMTPPort:       getEnv("SMTP_PORT", "587"),
		SMTPUser:       getEnv("SMTP_USER", ""),
		SMTPPassword:   getEnv("SMTP_PASSWORD", ""),
		SMTPFrom:       getEnv("SMTP_FROM", "hookforge@localhost"),
		AdminAPIKey:    getEnv("ADMIN_API_KEY", ""),
		AllowedOrigins: getEnv("ALLOWED_ORIGINS", ""),
		MaxBodyBytes:   getEnvInt64("MAX_BODY_BYTES", 1048576),
	}
}

func (c *Config) EmailConfig() notifier.EmailConfig {
	return notifier.EmailConfig{
		SMTPHost:     c.SMTPHost,
		SMTPPort:     c.SMTPPort,
		SMTPUser:     c.SMTPUser,
		SMTPPassword: c.SMTPPassword,
		FromAddr:     c.SMTPFrom,
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

func getEnvInt64(key string, fallback int64) int64 {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.ParseInt(val, 10, 64); err == nil {
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
