package models

type PlexLibrariesResponse struct {
	Size        int         `xml:"size,attr" json:"size"`
	AllowSync   int         `xml:"allowSync,attr" json:"allowSync"`
	Title1      string      `xml:"title1,attr" json:"title1"`
	Directories []Directory `xml:"Directory" json:"directories"`
}

type Directory struct {
	AllowSync        int        `xml:"allowSync,attr" json:"allowSync"`
	Art              string     `xml:"art,attr" json:"art"`
	Composite        string     `xml:"composite,attr" json:"composite"`
	Filters          int        `xml:"filters,attr" json:"filters"`
	Refreshing       int        `xml:"refreshing,attr" json:"refreshing"`
	Thumb            string     `xml:"thumb,attr" json:"thumb"`
	Key              string     `xml:"key,attr" json:"key"`
	Type             string     `xml:"type,attr" json:"type"`
	Title            string     `xml:"title,attr" json:"title"`
	Agent            string     `xml:"agent,attr" json:"agent"`
	Scanner          string     `xml:"scanner,attr" json:"scanner"`
	Language         string     `xml:"language,attr" json:"language"`
	UUID             string     `xml:"uuid,attr" json:"uuid"`
	UpdatedAt        int64      `xml:"updatedAt,attr" json:"updatedAt"`
	CreatedAt        int64      `xml:"createdAt,attr" json:"createdAt"`
	ScannedAt        int64      `xml:"scannedAt,attr" json:"scannedAt"`
	Content          int        `xml:"content,attr" json:"content"`
	Directory        int        `xml:"directory,attr" json:"directory"`
	ContentChangedAt int64      `xml:"contentChangedAt,attr" json:"contentChangedAt"`
	Hidden           int        `xml:"hidden,attr" json:"hidden"`
	Locations        []Location `xml:"Location" json:"locations"`
}

type Location struct {
	ID   int    `xml:"id,attr" json:"id"`
	Path string `xml:"path,attr" json:"path"`
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
