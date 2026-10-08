package config

import (
	"os"

	"github.com/joho/godotenv"
)

// Config holds all runtime configuration, sourced from environment variables.
type Config struct {
	Port          string
	DatabaseURL   string
	RedisAddr     string
	RedisPassword string
	RedisDB       int
	JWTSecret     string
	EncryptionKey string // 32-byte hex key for AES-256-GCM (SMTP creds)
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// Load reads configuration from the environment with sane local-dev defaults.
// A .env file in the working directory is loaded if present.
func Load() *Config {
	_ = godotenv.Load()
	return &Config{
		Port:          getenv("PORT", "8080"),
		DatabaseURL:   getenv("DATABASE_URL", "postgres://emailsender:emailsender@localhost:5432/emailsender?sslmode=disable"),
		RedisAddr:     getenv("REDIS_ADDR", "localhost:6379"),
		RedisPassword: getenv("REDIS_PASSWORD", ""),
		RedisDB:       0,
		JWTSecret:     getenv("JWT_SECRET", "dev-secret-change-me"),
		EncryptionKey: getenv("ENCRYPTION_KEY", ""),
	}
}
