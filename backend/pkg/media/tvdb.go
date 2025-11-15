package media

import (
	"encoding/json"
	"fmt"
)

// Status represents the TVDB API v4 status object
type Status struct {
	Id          int    `json:"id"`
	KeepUpdated bool   `json:"keepUpdated"`
	Name        string `json:"name"`
	RecordType  string `json:"recordType"`
}

// UnmarshalJSON custom unmarshaler to handle both string and object formats
// Search endpoint returns string: "status": "Continuing"
// Extended endpoint returns object: "status": {"id": 1, "name": "Continuing", ...}
func (s *Status) UnmarshalJSON(data []byte) error {
	// Try to unmarshal as string first (search endpoint format)
	var statusString string
	if err := json.Unmarshal(data, &statusString); err == nil {
		s.Name = statusString
		return nil
	}

	// Try to unmarshal as object (extended endpoint format)
	type statusAlias Status
	var statusObj statusAlias
	if err := json.Unmarshal(data, &statusObj); err != nil {
		return fmt.Errorf("status field is neither string nor object: %w", err)
	}

	*s = Status(statusObj)
	return nil
}

type Media struct {
	// TVDB Id for show
	Id string `json:"id"`
	// Show name
	Name string `json:"mediaName"`
	// Either series or movie
	Category string `json:"type"`
	// Whether show is an anime or not
	Anime bool `json:"anime"`
	// Fuzzy popularity of show. Used to sort return result
	Score float64 `json:"score"`
	// slug for show
	Slug string `json:"slug"`
	// ImageUrl to serve thumbnail on frontend
	ImageUrl string `json:"image_url"`
	// Original name.. could be japanese, french, etc
	OriginalName string `json:"name"`
	// Whether the show is "Continuing" or "Ended" (TVDB API v4 returns object)
	Status Status `json:"status"`
	// Show description
	Overview string `json:"overview"`
	// Year show was made
	Year string `json:"year"`
	// Alternative names/aliases for the show
	Aliases []string `json:"aliases"`
	// Information about series, such as episodes, date aired, etc.
	Metadata TVDBSeriesMetadata `json:"metadata"`
}

type Overview struct {
	Eng string `json:"eng"`
}

type Translations struct {
	Eng string `json:"eng"`
}

type TVDBSearchResponse struct {
	Data []TVDBSearchItem `json:"data"`
}

type TVDBSearchItem struct {
	// TVDB Id for show
	Id string `json:"id"`
	// Original name from TVDB (not used - we get English name from Translations.Eng)
	Name string `json:"-"`
	// Either series or movie
	Category string `json:"type"`
	// ImageUrl to serve thumbnail on frontend
	ImageUrl string `json:"image_url"`
	// Original name.. could be japanese, french, etc
	OriginalName string `json:"name"`
	// media slug
	Slug string `json:"slug"`
	// Whether the show is "Continuing" or "Ended" (TVDB API v4 returns object)
	Status Status `json:"status"`
	// Show description
	Overview string `json:"overview"`
	// Year show was made
	Year string `json:"year"`
	// Translations of the name, used to get name in english
	Translations Translations `json:"translations"`
	// Translations to the show description
	Overviews Overview `json:"overviews"`
}

type Episode struct {
	Id                   int      `json:"id"`
	Name                 string   `json:"name"`
	Aired                string   `json:"aired"`
	Image                string   `json:"image"`
	Number               int      `json:"number"`
	SeasonNumber         int      `json:"seasonNumber"`
	AbsoluteNumber       int      `json:"absoluteNumber"`
	Runtime              int      `json:"runtime"`              // Episode runtime in minutes (nullable in API)
	Year                 string   `json:"year"`                 // Year the episode aired
	SeriesId             int      `json:"seriesId"`             // Link back to parent series
	SeasonName           string   `json:"seasonName"`           // Name of the season (e.g., "Season 1")
	FinaleType           string   `json:"finaleType"`           // "season", "midseason", or "series"
	LastUpdated          string   `json:"lastUpdated"`          // For cache invalidation
	ImageType            int      `json:"imageType"`            // Image type identifier (nullable in API)
	Overview             string   `json:"overview"`             // Episode description
	NameTranslations     []string `json:"nameTranslations"`     // Available name translation languages
	OverviewTranslations []string `json:"overviewTranslations"` // Available overview translation languages
	AirsAfterSeason      int      `json:"airsAfterSeason"`      // For special episodes that air after a season
	AirsBeforeSeason     int      `json:"airsBeforeSeason"`     // For special episodes that air before a season
	AirsBeforeEpisode    int      `json:"airsBeforeEpisode"`    // For special episodes that air before an episode
	IsMovie              int      `json:"isMovie"`              // Whether this episode is actually a movie
	LinkedMovie          int      `json:"linkedMovie"`          // ID of linked movie if applicable
}

type TVDBSeriesMetadata struct {
	FirstAired string    `json:"firstAired"`
	LastAired  string    `json:"lastAired"`
	Number     int       `json:"number"`
	Episodes   []Episode `json:"episodes"`
	Score      float64   `json:"score"`
}

type TVDBSeriesResponse struct {
	Data TVDBSeriesMetadata `json:"data"`
}

type TVDBSeriesExtendedResponse struct {
	Data TVDBSeriesExtendedData `json:"data"`
}

type TVDBSeriesExtendedData struct {
	Id               int       `json:"id"`               // Series ID
	Name             string    `json:"name"`             // Series name
	Slug             string    `json:"slug"`             // Series slug
	Image            string    `json:"image"`            // Image URL
	Overview         string    `json:"overview"`         // Series overview/description
	Year             string    `json:"year"`             // Year
	Status           Status    `json:"status"`           // Status object (Continuing, Ended, etc.)
	Genres           []Genres  `json:"genres"`           // Critical: Used for anime detection
	Aliases          []Alias   `json:"aliases"`          // Critical: Used for torrent search
	FirstAired       string    `json:"firstAired"`       // Critical: Used for movie scheduling
	LastAired        string    `json:"lastAired"`        // Last aired date
	OriginalCountry  string    `json:"originalCountry"`  // Country of origin
	OriginalLanguage string    `json:"originalLanguage"` // Original language
	Episodes         []Episode `json:"episodes"`         // Episodes now included in extended response
	AverageRuntime   int       `json:"averageRuntime"`   // Average episode runtime
	Score            float64   `json:"score"`            // Popularity score
}

// MovieExtendedData represents movie-specific extended data from TVDB API v4
type MovieExtendedData struct {
	Id               int      `json:"id"`               // Movie ID
	Name             string   `json:"name"`             // Movie name
	Slug             string   `json:"slug"`             // Movie slug
	Image            string   `json:"image"`            // Image URL
	Overview         string   `json:"overview"`         // Movie overview/description
	Year             string   `json:"year"`             // Year
	Status           Status   `json:"status"`           // Status object
	Genres           []Genres `json:"genres"`           // Critical: Used for anime detection
	Aliases          []Alias  `json:"aliases"`          // Critical: Used for torrent search
	FirstAired       string   `json:"firstAired"`       // Critical: Movie release date for scheduling
	OriginalCountry  string   `json:"originalCountry"`  // Country of origin
	OriginalLanguage string   `json:"originalLanguage"` // Original language
	Runtime          int      `json:"runtime"`          // Movie runtime in minutes (nullable in API)
	BoxOffice        string   `json:"boxOffice"`        // Box office earnings
	Budget           string   `json:"budget"`           // Production budget
	Score            float64  `json:"score"`            // Popularity score
}

type Genres struct {
	Id   int    `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type Alias struct {
	Language string `json:"language"`
	Name     string `json:"name"`
}

type TVDBTranslationResponse struct {
	Status string              `json:"status"`
	Data   TVDBTranslationData `json:"data"`
}

type TVDBTranslationData struct {
	Name     string   `json:"name"`
	Overview string   `json:"overview"`
	Language string   `json:"language"`
	Aliases  []string `json:"aliases"`
}

// ContentRating represents content/parental rating information from TVDB API v4
type ContentRating struct {
	Id          int    `json:"id"`
	Name        string `json:"name"`        // e.g., "TV-MA", "PG-13"
	Description string `json:"description"` // Rating description
	Country     string `json:"country"`     // Country code (e.g., "usa")
	ContentType string `json:"contentType"` // Type of content rating
	Order       int    `json:"order"`       // Display order
	FullName    string `json:"fullName"`    // Full name of the rating
}

// RemoteID represents external service IDs (IMDB, TMDB, etc.) from TVDB API v4
type RemoteID struct {
	Id         int    `json:"id"`
	Type       int    `json:"type"`       // Type identifier for the service
	RemoteId   string `json:"remoteId"`   // The actual ID on the external service
	SourceName string `json:"sourceName"` // Name of the service (e.g., "IMDB", "TheMovieDB.com")
}
