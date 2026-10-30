// Package settings holds the API shapes of Scout's runtime settings: the knobs
// a user can change from the Settings page instead of editing environment
// variables. The torrenter owns their meaning and storage; the webserver only
// proxies them.
package settings

// Setting keys as stored in the Settings table and used in the reset list.
const (
	KeyMinQuality        = "min_quality"
	KeySubtitleLanguages = "subtitle_languages"
	KeySubtitlePreferSDH = "subtitle_prefer_sdh"
	KeyPlexSyncDelay     = "plex_sync_delay"
	KeyPlexSyncInterval  = "plex_sync_interval"
)

// Keys lists every settable key, in display order.
var Keys = []string{KeyMinQuality, KeySubtitleLanguages, KeySubtitlePreferSDH, KeyPlexSyncDelay, KeyPlexSyncInterval}

// Values are the settings as the API presents them. Durations are Go
// duration strings ("2m", "30m"); "0s" disables the periodic sync.
type Values struct {
	MinQuality        string   `json:"min_quality"`
	SubtitleLanguages []string `json:"subtitle_languages"`
	SubtitlePreferSDH bool     `json:"subtitle_prefer_sdh"`
	PlexSyncDelay     string   `json:"plex_sync_delay"`
	PlexSyncInterval  string   `json:"plex_sync_interval"`
}

// Update is the body of PUT /settings: any subset of keys to override, plus
// keys to reset to their environment default. A nil field leaves that key
// alone.
type Update struct {
	MinQuality        *string   `json:"min_quality,omitempty"`
	SubtitleLanguages *[]string `json:"subtitle_languages,omitempty"`
	SubtitlePreferSDH *bool     `json:"subtitle_prefer_sdh,omitempty"`
	PlexSyncDelay     *string   `json:"plex_sync_delay,omitempty"`
	PlexSyncInterval  *string   `json:"plex_sync_interval,omitempty"`
	Reset             []string  `json:"reset,omitempty"`
}

// Options is the context the Settings page needs beyond the values.
type Options struct {
	// MinQuality lists the accepted quality floor labels, lowest first.
	MinQuality []string `json:"min_quality"`
	// SubtitlesEnabled is false when no OpenSubtitles API key is configured;
	// the subtitle settings are then shown but have no effect.
	SubtitlesEnabled bool `json:"subtitles_enabled"`
}

// View is the response of GET /settings and PUT /settings.
type View struct {
	Values     Values          `json:"values"`
	Defaults   Values          `json:"defaults"`
	Overridden map[string]bool `json:"overridden"`
	Options    Options         `json:"options"`
}

// UploaderPreference is one row of the picker's preferred-uploader list.
type UploaderPreference struct {
	ID        int    `json:"id"`
	MediaType string `json:"media_type"` // "series" or "movie"
	IsAnime   bool   `json:"is_anime"`
	Uploader  string `json:"uploader"`
}

// NewUploaderPreference is the body of POST /settings/uploaders.
type NewUploaderPreference struct {
	MediaType string `json:"media_type"`
	IsAnime   bool   `json:"is_anime"`
	Uploader  string `json:"uploader"`
}

// Valid reports whether the preference can be stored.
func (p NewUploaderPreference) Valid() bool {
	return (p.MediaType == "series" || p.MediaType == "movie") && len(p.Uploader) > 0 && len(p.Uploader) <= 64
}
