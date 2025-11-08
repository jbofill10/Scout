package models

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
}

type TorrenterConf struct {
	Prowlarr ProwlarrCfg
	Qbitt    QbittCfg
	Ui       UiCfg
	Plex     PlexCfg
}
