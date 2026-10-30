package config

import (
	"fmt"
	"os"
	"time"
)

// Config holds the webserver configuration
type Config struct {
	Database      DatabaseConfig
	BindAddress   string
	TVDBProxyHost string
	TorrenterHost string
	// FollowCheckInterval is how often followed shows are re-checked against TVDB.
	FollowCheckInterval time.Duration
}

// DatabaseConfig holds database connection settings
type DatabaseConfig struct {
	Host     string
	User     string
	Password string
	Name     string
	ConnStr  string
}

// Load loads the webserver configuration from environment variables
func Load() (Config, error) {
	var cfg Config

	// Database configuration
	cfg.Database.Host = getEnv("DB_HOST", "postgres")
	cfg.Database.User = getEnv("DB_USER", "scoutuser")
	cfg.Database.Password = getEnv("DB_PASSWORD", "scoutpass")
	cfg.Database.Name = getEnv("DB_NAME", "scoutdb")

	cfg.Database.ConnStr = fmt.Sprintf(
		"host=%s port=5432 user=%s password=%s dbname=%s sslmode=disable",
		cfg.Database.Host,
		cfg.Database.User,
		cfg.Database.Password,
		cfg.Database.Name,
	)

	// Service discovery (defaults for docker-compose)
	cfg.TVDBProxyHost = getEnv("TVDB_PROXY_HOST", "tvdb-proxy:22000")
	cfg.TorrenterHost = getEnv("TORRENTER_HOST", "torrenter:22001")

	// Server configuration
	cfg.BindAddress = getEnv("BIND_ADDRESS", "0.0.0.0:22920")

	// Followed shows
	followInterval := getEnv("FOLLOW_CHECK_INTERVAL", "6h")
	d, err := time.ParseDuration(followInterval)
	if err != nil || d < time.Minute {
		return cfg, fmt.Errorf("invalid FOLLOW_CHECK_INTERVAL value: %s (want a duration of at least 1m)", followInterval)
	}
	cfg.FollowCheckInterval = d

	return cfg, nil
}

// getEnv retrieves an environment variable or returns a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
