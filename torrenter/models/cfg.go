package models

type ProwlarrCfg struct {
	Host string `toml:"host"`
	Key  string `toml:"key"`
}

type QbittCfg struct {
	Host     string `toml:"host"`
	User     string `toml:"user"`
	Password string `toml:"password"`
}

type UiCfg struct {
	Endpoint string `toml:"endpoint"`
}

type PlexCfg struct {
	MovieSections int    `toml:"movieSections"`
	ShowSections  int    `toml:"showSections"`
	Host          string `toml:"host"`
	Key           string `toml:"key"`
}

type TorrenterConf struct {
	Prowlarr ProwlarrCfg
	Qbitt    QbittCfg
	Ui       UiCfg
	Plex     PlexCfg
}
