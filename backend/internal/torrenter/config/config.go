package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jbofill10/scout/backend/internal/torrenter/models"
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

	// Load Refresh config
	cfg.Refresh.WebserverHost = getEnv("WEBSERVER_HOST", "http://localhost:22920")

	refreshIntervalStr := getEnv("TVDB_REFRESH_INTERVAL", "6h")
	refreshInterval, err := time.ParseDuration(refreshIntervalStr)
	if err != nil {
		return cfg, fmt.Errorf("invalid TVDB_REFRESH_INTERVAL value: %s", refreshIntervalStr)
	}
	cfg.Refresh.RefreshInterval = refreshInterval

	// Subtitle fetching is optional: no API key, no fetching.
	cfg.Subtitles.APIKey = getEnv("OPENSUBTITLES_API_KEY", "")
	cfg.Subtitles.Username = getEnv("OPENSUBTITLES_USERNAME", "")
	cfg.Subtitles.Password = getEnv("OPENSUBTITLES_PASSWORD", "")
	cfg.Subtitles.Languages = splitList(getEnv("SUBTITLE_LANGUAGES", "en"))
	cfg.Subtitles.PreferSDH = getEnv("SUBTITLE_PREFER_SDH", "false") == "true"

	maxDownloads := getEnv("SUBTITLE_MAX_DOWNLOADS_PER_ITEM", "3")
	if val, err := strconv.Atoi(maxDownloads); err == nil && val > 0 {
		cfg.Subtitles.MaxDownloadsPerItem = val
	} else {
		return cfg, fmt.Errorf("invalid SUBTITLE_MAX_DOWNLOADS_PER_ITEM value: %s", maxDownloads)
	}

	subtitleTimeoutStr := getEnv("SUBTITLE_TIMEOUT", "2m")
	subtitleTimeout, err := time.ParseDuration(subtitleTimeoutStr)
	if err != nil {
		return cfg, fmt.Errorf("invalid SUBTITLE_TIMEOUT value: %s", subtitleTimeoutStr)
	}
	cfg.Subtitles.Timeout = subtitleTimeout

	return cfg, nil
}

// splitList parses a comma-separated list, dropping blanks and surrounding
// whitespace, so "en, es," becomes ["en", "es"].
func splitList(value string) []string {
	var out []string
	for _, part := range strings.Split(value, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

// getEnv retrieves an environment variable or returns a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
