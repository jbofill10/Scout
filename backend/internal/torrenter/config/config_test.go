package config

import (
	"testing"
	"time"
)

func setRequiredEnv(t *testing.T) {
	t.Helper()
	for k, v := range map[string]string{
		"PROWLARR_HOST": "http://p", "PROWLARR_KEY": "k", "QBITT_HOST": "http://q", "QBITT_USER": "u", "QBITT_PASSWORD": "p",
		"PLEX_HOST": "http://plex", "PLEX_KEY": "pk",
	} {
		t.Setenv(k, v)
	}
}

func TestLoadSubtitleDefaults(t *testing.T) {
	setRequiredEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Subtitles.APIKey != "" || len(cfg.Subtitles.Languages) != 1 || cfg.Subtitles.Languages[0] != "en" {
		t.Fatalf("unexpected defaults: %+v", cfg.Subtitles)
	}
	if cfg.Subtitles.MaxDownloadsPerItem != 3 || cfg.Subtitles.Timeout != 2*time.Minute || cfg.Subtitles.PreferSDH {
		t.Fatalf("unexpected defaults: %+v", cfg.Subtitles)
	}
}

func TestLoadSubtitleOverrides(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("OPENSUBTITLES_API_KEY", "abc")
	t.Setenv("OPENSUBTITLES_USERNAME", "me")
	t.Setenv("OPENSUBTITLES_PASSWORD", "pw")
	t.Setenv("SUBTITLE_LANGUAGES", " en, es ,,")
	t.Setenv("SUBTITLE_PREFER_SDH", "true")
	t.Setenv("SUBTITLE_MAX_DOWNLOADS_PER_ITEM", "1")
	t.Setenv("SUBTITLE_TIMEOUT", "45s")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	s := cfg.Subtitles
	if s.APIKey != "abc" || s.Username != "me" || s.Password != "pw" || !s.PreferSDH || s.MaxDownloadsPerItem != 1 || s.Timeout != 45*time.Second {
		t.Fatalf("unexpected: %+v", s)
	}
	if len(s.Languages) != 2 || s.Languages[0] != "en" || s.Languages[1] != "es" {
		t.Fatalf("languages: %v", s.Languages)
	}
}

func TestLoadRejectsBadSubtitleValues(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("SUBTITLE_MAX_DOWNLOADS_PER_ITEM", "0")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for zero download cap")
	}
	t.Setenv("SUBTITLE_MAX_DOWNLOADS_PER_ITEM", "3")
	t.Setenv("SUBTITLE_TIMEOUT", "soon")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for bad timeout")
	}
}
