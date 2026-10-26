package models

import "time"

// SubtitlesCfg configures post-download subtitle fetching. An empty APIKey
// disables the feature.
type SubtitlesCfg struct {
	// OpenSubtitles REST API key (https://www.opensubtitles.com/consumers).
	APIKey string
	// Optional account credentials. When set, downloads run as that user and
	// count against its quota; when empty, downloads are anonymous.
	Username string
	Password string
	// Languages to fetch, in ISO 639-1 as OpenSubtitles expects ("en", "es").
	// One subtitle file is written per language that has a candidate.
	Languages []string
	// PreferSDH picks hearing-impaired tracks over plain ones when both exist.
	PreferSDH bool
	// MaxDownloadsPerItem caps how many candidates are downloaded for one
	// video when the sanity check rejects the first pick.
	MaxDownloadsPerItem int
	// Timeout bounds the whole fetch for one video.
	Timeout time.Duration
}

// SubtitleRequest identifies one video that just landed in the library and
// what is known about it, so a subtitle can be matched to it.
type SubtitleRequest struct {
	// VideoPath is the file qBittorrent wrote; it is hashed and sized.
	VideoPath string
	// LibraryPath is the Plex-facing path (the link created next to the
	// media); subtitle files are written beside it with its basename.
	LibraryPath string
	// ReleaseName is the release the video came from, used to prefer
	// subtitles cut for the same rip. Usually the original filename.
	ReleaseName string
	// ImdbID of the movie, or of the parent show for an episode. Optional.
	ImdbID  string
	Title   string
	Year    string
	IsMovie bool
	Season  int
	Episode int
	// RuntimeMinutes, when known, lets a mistimed subtitle be rejected.
	RuntimeMinutes int
}

// SubtitleResult describes one subtitle file written for a request.
type SubtitleResult struct {
	Path            string
	Language        string
	HearingImpaired bool
	HashMatch       bool
	Release         string
	FileID          int
	// Remaining is the provider's remaining download quota after this
	// download, or -1 when the provider did not say.
	Remaining int
}

// SubtitleSearchParams is one provider query. Either ImdbID (movie) or
// ParentImdbID + Season + Episode (episode) identifies the title exactly;
// Query + Year is the fallback when no id is known. MovieHash, when set, lets
// the provider flag subtitles cut for this exact file.
type SubtitleSearchParams struct {
	Languages    []string
	IsMovie      bool
	ImdbID       string
	ParentImdbID string
	Season       int
	Episode      int
	Query        string
	Year         string
	MovieHash    string
}

// SubtitleCandidate is one subtitle a provider offers for a search.
type SubtitleCandidate struct {
	FileID            int
	Language          string
	Release           string
	HearingImpaired   bool
	ForeignPartsOnly  bool
	MachineTranslated bool
	AITranslated      bool
	FromTrusted       bool
	HashMatch         bool
	DownloadCount     int
	Ratings           float64
	FPS               float64
	// What the provider thinks the subtitle is for, so an episode search that
	// returned a neighbouring episode can be rejected.
	FeatureType string
	Season      int
	Episode     int
	Year        int
}
