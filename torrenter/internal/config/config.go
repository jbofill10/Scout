package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"torrenter/internal/models"
)

// Load loads the torrenter configuration from environment variables and Kubernetes secrets
func Load() (models.TorrenterConf, error) {
	var cfg models.TorrenterConf

	// Load Prowlarr config
	cfg.Prowlarr.Host = getEnv("PROWLARR_HOST", "")
	if cfg.Prowlarr.Host == "" {
		return cfg, fmt.Errorf("PROWLARR_HOST environment variable is required")
	}

	// Load Prowlarr key from secret file (mounted by k8s)
	if prowlarrKey, err := os.ReadFile("/root/secrets/prowlarr-key"); err == nil {
		cfg.Prowlarr.Key = strings.TrimSpace(string(prowlarrKey))
	} else {
		return cfg, fmt.Errorf("failed to read prowlarr-key: %w", err)
	}

	// Load qBittorrent config
	cfg.Qbitt.Host = getEnv("QBITT_HOST", "")
	if cfg.Qbitt.Host == "" {
		return cfg, fmt.Errorf("QBITT_HOST environment variable is required")
	}

	if qbittUser, err := os.ReadFile("/root/secrets/qbitt-user"); err == nil {
		cfg.Qbitt.User = strings.TrimSpace(string(qbittUser))
	} else {
		return cfg, fmt.Errorf("failed to read qbitt-user: %w", err)
	}

	if qbittPassword, err := os.ReadFile("/root/secrets/qbitt-password"); err == nil {
		cfg.Qbitt.Password = strings.TrimSpace(string(qbittPassword))
	} else {
		return cfg, fmt.Errorf("failed to read qbitt-password: %w", err)
	}

	// Load Plex config
	cfg.Plex.Host = getEnv("PLEX_HOST", "")
	if cfg.Plex.Host == "" {
		return cfg, fmt.Errorf("PLEX_HOST environment variable is required")
	}

	if plexKey, err := os.ReadFile("/root/secrets/plex-key"); err == nil {
		cfg.Plex.Key = strings.TrimSpace(string(plexKey))
	} else {
		return cfg, fmt.Errorf("failed to read plex-key: %w", err)
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

	// Load UI config
	cfg.Ui.Endpoint = getEnv("UI_ENDPOINT", "ws://localhost:22920/status")

	return cfg, nil
}

// getEnv retrieves an environment variable or returns a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
