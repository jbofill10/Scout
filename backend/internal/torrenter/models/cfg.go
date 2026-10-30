package models

import "time"

type ProwlarrCfg struct {
	Host string
	Key  string
}

type QbittCfg struct {
	Host     string
	User     string
	Password string
}

type UiCfg struct {
	Endpoint string
}

type PlexCfg struct {
	MovieSections int
	ShowSections  int
	Host          string
	Key           string
	// SyncDelay is the quiet period after a download lands before the library
	// mirror is re-read from Plex; SyncInterval is the periodic re-read (0 = off).
	SyncDelay    time.Duration
	SyncInterval time.Duration
}

type RefreshCfg struct {
	WebserverHost   string
	RefreshInterval time.Duration
}

// DownloadsCfg holds the environment defaults the Settings page can override.
type DownloadsCfg struct {
	// MinQuality is the resolution floor label (480p, 720p, 1080p, 2160p).
	MinQuality string
	// MinFreeSpaceGB is kept free on the downloads volume.
	MinFreeSpaceGB int
}

// RelayCfg points at the host relay agent (tools/relay-agent); an empty URL
// disables relay rotation.
type RelayCfg struct {
	AgentURL       string
	AgentToken     string
	RotateCooldown time.Duration
	HealthInterval time.Duration
}

type TorrenterConf struct {
	Downloads DownloadsCfg
	Relay     RelayCfg
	Prowlarr  ProwlarrCfg
	Qbitt     QbittCfg
	Ui        UiCfg
	Plex      PlexCfg
	Refresh   RefreshCfg
	Subtitles SubtitlesCfg
}
