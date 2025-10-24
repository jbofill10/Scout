package models

type PlexMovieLibraryData struct {
	XMLName string  `xml:"MediaContainer"`
	Movies  []Movie `xml:"Video"`
}

type PlexShowsResponse struct {
	XMLName string     `xml:"MediaContainer"`
	Shows   []PlexShow `xml:"Directory"`
}

type PlexShow struct {
	ShowKey string     `xml:"ratingKey,attr"`
	Title   string     `xml:"title,attr"`
	Key     string     `xml:"key,attr"`
	Thumb   string     `xml:"thumb,attr"`
	Guids   []PlexGuid `xml:"Guid"`
}

type PlexGuid struct {
	ID string `xml:"id,attr"`
}

type PlexSeasonsResponse struct {
	XMLName string       `xml:"MediaContainer"`
	Seasons []PlexSeason `xml:"Directory"`
}

type PlexSeason struct {
	SeasonKey string `xml:"ratingKey,attr"`
	Key       string `xml:"key,attr"`
	Title     string `xml:"title,attr"`
	Index     int    `xml:"index,attr"`
}

type PlexEpisodesResponse struct {
	XMLName string        `xml:"MediaContainer"`
	Videos  []PlexEpisode `xml:"Video"`
}

type PlexEpisode struct {
	EpisodeKey  string      `xml:"ratingKey,attr"`
	Key         string      `xml:"key,attr"`
	Index       int         `xml:"index,attr"`
	ParentIndex int         `xml:"parentIndex,attr"`
	Media       []MediaMeta `xml:"Media"`
}

type PlexShowLibraryData struct {
	Shows []PlexShowData
}

type PlexShowData struct {
	Id       string
	Title    string
	ShowMeta string
	Thumb    string
	TvdbId   string
	Seasons  []PlexSeasonData
}

type PlexSeasonData struct {
	Id           string
	SeasonMeta   string
	SeasonNumber int
	Episodes     []PlexEpisodeData
}

type PlexEpisodeData struct {
	Id            string
	EpisodeMeta   string
	EpisodeNumber int
	Media         []PlexMediaData
}

type PlexMediaData struct {
	Id              string
	VideoResolution string
	File            string
}

type Movie struct {
	Id        string      `xml:"ratingKey,attr" json:"ratingKey"`
	Title     string      `xml:"title,attr" json:"title"`
	Year      int         `xml:"year,attr" json:"year,omitempty"`
	Thumb     string      `xml:"thumb,attr" json:"thumb,omitempty"`
	Art       string      `xml:"art,attr" json:"art,omitempty"`
	Guids     []PlexGuid  `xml:"Guid"`
	MovieMeta []MediaMeta `xml:"Media" json:"media"`
}

type MediaMeta struct {
	ID              int    `xml:"id,attr" json:"id"`
	VideoResolution string `xml:"videoResolution,attr" json:"videoResolution"`
	Part            []Part `xml:"Part" json:"part"`
}

type Part struct {
	Id   string `xml:"id,attr" json:"id"`
	File string `xml:"file,attr" json:"file"`
}
