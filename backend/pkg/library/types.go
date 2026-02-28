package library

// LibraryShow represents a show in the user's library
type LibraryShow struct {
	TvdbId string `json:"tvdbId"`
	Title  string `json:"title"`
	Thumb  string `json:"thumb"`
}

// LibraryMovie represents a movie in the user's library
type LibraryMovie struct {
	TvdbId string `json:"tvdbId"`
	Title  string `json:"title"`
	Thumb  string `json:"thumb"`
	Year   int    `json:"year"`
}

// EpisodeWithStatus represents an episode with download status
type EpisodeWithStatus struct {
	TvdbId         string `json:"tvdbId"`
	SeasonNumber   int    `json:"seasonNumber"`
	EpisodeNumber  int    `json:"episodeNumber"`
	AbsoluteNumber int    `json:"absoluteNumber,omitempty"`
	Name           string `json:"name"`
	Aired          string `json:"aired"`
	Downloaded     bool   `json:"downloaded"`
}

// TvdbEpisode represents TVDB episode metadata for library sync
type TvdbEpisode struct {
	TvdbId         string
	SeriesTvdbId   string
	SeasonNumber   int
	EpisodeNumber  int
	AbsoluteNumber int
	Name           string
	Aired          string // YYYY-MM-DD format
}

// MetadataStatus represents the TVDB metadata sync status for a show
type MetadataStatus struct {
	HasTvdbData      bool `json:"hasTvdbData"`
	TvdbEpisodeCount int  `json:"tvdbEpisodeCount"`
	PlexEpisodeCount int  `json:"plexEpisodeCount"`
	MissingCount     int  `json:"missingCount"`
}
