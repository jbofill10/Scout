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

type TorrenterConf struct {
	Prowlarr  ProwlarrCfg
	Qbitt     QbittCfg
	Ui        UiCfg
	Plex      PlexCfg
	Refresh   RefreshCfg
	Subtitles SubtitlesCfg
}
