package config

import (
	"fmt"
	"os"
)

// Config holds the webserver configuration
type Config struct {
	Database      DatabaseConfig
	BindAddress   string
	TVDBProxyHost string
	TorrenterHost string
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
	cfg.Database.Host = getEnv("DB_HOST", "postgres-service")
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

	// Service discovery
	cfg.TVDBProxyHost = getEnv("TVDB_PROXY_HOST", "localhost:22000")
	cfg.TorrenterHost = getEnv("TORRENTER_HOST", "localhost:22001")

	// Server configuration
	cfg.BindAddress = getEnv("BIND_ADDRESS", "0.0.0.0:22920")

	return cfg, nil
}

// getEnv retrieves an environment variable or returns a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
