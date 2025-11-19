package media

// StatusRequest represents a request to check the download status of a media item
type StatusRequest struct {
	TvdbId    string `json:"tvdbId"`
	MediaType string `json:"mediaType"` // "show" or "movie"
}

// MovieStatus represents the download status of a movie
type MovieStatus struct {
	TvdbId    string `json:"tvdbId"`
	InLibrary bool   `json:"inLibrary"`
}

// EpisodeStatus represents the download status of a single episode
type EpisodeStatus struct {
	EpisodeNum int  `json:"episodeNum"`
	Downloaded bool `json:"downloaded"`
}

// SeasonStatus represents the download status of all episodes in a season
type SeasonStatus struct {
	SeasonNum int             `json:"seasonNum"`
	Episodes  []EpisodeStatus `json:"episodes"`
}

// ShowStatus represents the download status of all seasons/episodes in a TV show
type ShowStatus struct {
	TvdbId  string         `json:"tvdbId"`
	Seasons []SeasonStatus `json:"seasons"`
}

// StatusBatchResponse represents the response from a batch status check
type StatusBatchResponse struct {
	Shows  []ShowStatus  `json:"shows,omitempty"`
	Movies []MovieStatus `json:"movies,omitempty"`
}
