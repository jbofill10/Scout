// Package config loads tvdb-proxy's configuration: TVDB credentials from an
// optional api.toml (local development) with environment variables taking
// precedence, the bind address and the token refresh interval.
package config

import (
	"errors"
	"log/slog"
	"os"
	"time"

	"github.com/pelletier/go-toml"
)

const (
	// tomlPath is the optional local config file, relative to the working directory.
	tomlPath = "api.toml"

	defaultBindAddress = "localhost:22000"

	defaultRefreshInterval = 24 * time.Hour
	// minRefreshInterval keeps the proxy polite to TVDB's login endpoint.
	minRefreshInterval = time.Hour
)

// TVDB holds the TVDB API v4 base URL and credentials.
type TVDB struct {
	Host   string `toml:"host"`
	APIKey string `toml:"api-key"`
	Pin    string `toml:"pin"`
}

// Config is tvdb-proxy's configuration.
type Config struct {
	TVDB TVDB
	// BindAddress is where the HTTP server listens (BIND_ADDRESS).
	BindAddress string
	// TokenRefreshInterval is the raw TVDB_TOKEN_REFRESH_INTERVAL value; see RefreshInterval.
	TokenRefreshInterval string
}

// Load reads api.toml when it exists, then applies TVDB_HOST, TVDB_API_KEY and
// TVDB_PIN on top (environment wins). The only error is a malformed api.toml.
// Required fields are checked separately by Validate.
func Load() (Config, error) {
	var cfg Config

	// Try to load from api.toml first (optional for local dev)
	conf, err := toml.LoadFile(tomlPath)
	if err == nil {
		if err := conf.Unmarshal(&cfg.TVDB); err != nil {
			return Config{}, err
		}
	}

	// Override with environment variables (highest priority)
	if host := os.Getenv("TVDB_HOST"); host != "" {
		cfg.TVDB.Host = host
	}
	if apiKey := os.Getenv("TVDB_API_KEY"); apiKey != "" {
		cfg.TVDB.APIKey = apiKey
	}
	if pin := os.Getenv("TVDB_PIN"); pin != "" {
		cfg.TVDB.Pin = pin
	}

	cfg.BindAddress = os.Getenv("BIND_ADDRESS")
	if cfg.BindAddress == "" {
		cfg.BindAddress = defaultBindAddress
	}
	cfg.TokenRefreshInterval = os.Getenv("TVDB_TOKEN_REFRESH_INTERVAL")

	return cfg, nil
}

// Validate reports the first required TVDB setting that is missing.
func (c Config) Validate() error {
	if c.TVDB.Host == "" {
		return errors.New("TVDB_HOST is required")
	}
	if c.TVDB.APIKey == "" {
		return errors.New("TVDB_API_KEY is required")
	}
	return nil
}

// RefreshInterval is how often the TVDB bearer token is re-issued:
// TVDB_TOKEN_REFRESH_INTERVAL when it parses, 24h otherwise, with a 1h floor.
func (c Config) RefreshInterval(logger *slog.Logger) time.Duration {
	interval := defaultRefreshInterval
	raw := c.TokenRefreshInterval
	if raw == "" {
		return interval
	}
	parsed, err := time.ParseDuration(raw)
	switch {
	case err != nil:
		logger.Warn("Invalid TVDB_TOKEN_REFRESH_INTERVAL, using default", "value", raw, "default", interval.String())
	case parsed < minRefreshInterval:
		logger.Warn("TVDB_TOKEN_REFRESH_INTERVAL below 1h, clamping to 1h", "value", raw)
		interval = minRefreshInterval
	default:
		interval = parsed
	}
	return interval
}
