package config

import (
	"fmt"
	"github.com/jbofill10/scout/backend/internal/torrenter/models"
	"os"
	"strconv"
)

// Load loads the torrenter configuration from environment variables
func Load() (models.TorrenterConf, error) {
	var cfg models.TorrenterConf

	// Load Prowlarr config
	cfg.Prowlarr.Host = getEnv("PROWLARR_HOST", "")
	if cfg.Prowlarr.Host == "" {
		return cfg, fmt.Errorf("PROWLARR_HOST environment variable is required")
	}

	cfg.Prowlarr.Key = getEnv("PROWLARR_KEY", "")
	if cfg.Prowlarr.Key == "" {
		return cfg, fmt.Errorf("PROWLARR_KEY environment variable is required")
	}

	// Load qBittorrent config
	cfg.Qbitt.Host = getEnv("QBITT_HOST", "")
	if cfg.Qbitt.Host == "" {
		return cfg, fmt.Errorf("QBITT_HOST environment variable is required")
	}

	cfg.Qbitt.User = getEnv("QBITT_USER", "")
	if cfg.Qbitt.User == "" {
		return cfg, fmt.Errorf("QBITT_USER environment variable is required")
	}

	cfg.Qbitt.Password = getEnv("QBITT_PASSWORD", "")
	if cfg.Qbitt.Password == "" {
		return cfg, fmt.Errorf("QBITT_PASSWORD environment variable is required")
	}

	// Load Plex config
	cfg.Plex.Host = getEnv("PLEX_HOST", "")
	if cfg.Plex.Host == "" {
		return cfg, fmt.Errorf("PLEX_HOST environment variable is required")
	}

	cfg.Plex.Key = getEnv("PLEX_KEY", "")
	if cfg.Plex.Key == "" {
		return cfg, fmt.Errorf("PLEX_KEY environment variable is required")
	}

	// Parse Plex sections from environment variables
	movieSections := getEnv("PLEX_MOVIE_SECTIONS", "3")
	if val, err := strconv.Atoi(movieSections); err == nil {
		cfg.Plex.MovieSections = val
	} else {
		return cfg, fmt.Errorf("invalid PLEX_MOVIE_SECTIONS value: %s", movieSections)
	}

	showSections := getEnv("PLEX_SHOW_SECTIONS", "4")
	if val, err := strconv.Atoi(showSections); err == nil {
		cfg.Plex.ShowSections = val
	} else {
		return cfg, fmt.Errorf("invalid PLEX_SHOW_SECTIONS value: %s", showSections)
	}

	// Load UI config (default for docker-compose)
	cfg.Ui.Endpoint = getEnv("UI_ENDPOINT", "ws://webserver:22920/status")

	return cfg, nil
}

// getEnv retrieves an environment variable or returns a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
