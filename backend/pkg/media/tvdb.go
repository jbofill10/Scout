package media

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
	Score int `json:"score"`
	// slug for show
	Slug string `json:"slug"`
	// ImageUrl to serve thumbnail on frontend
	ImageUrl string `json:"imageUrl"`
	// Original name.. could be japanese, french, etc
	OriginalName string `json:"name"`
	// Whether the show is "Continuing" or "Ended"
	Status string `json:"status"`
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
	Id string `json:"tvdb_id"`
	// English translated name
	Name string `json:"mediaName"`
	// Either series or movie
	Category string `json:"type"`
	// ImageUrl to serve thumbnail on frontend
	ImageUrl string `json:"image_url"`
	// Original name.. could be japanese, french, etc
	OriginalName string `json:"name"`
	// media slug
	Slug string `json:"slug"`
	// Whether the show is "Continuing" or "Ended"
	Status string `json:"status"`
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
	Id             int    `json:"id"`
	Name           string `json:"name"`
	Aired          string `json:"aired"`
	Image          string `json:"image"`
	Number         int    `json:"number"`
	SeasonNumber   int    `json:"seasonNumber"`
	AbsoluteNumber int    `json:"absoluteNumber"`
}

type TVDBSeriesMetadata struct {
	FirstAired string    `json:"firstAired"`
	LastAired  string    `json:"lastAired"`
	Number     int       `json:"number"`
	Episodes   []Episode `json:"episodes"`
	Score      int       `json:"score"`
}

type TVDBSeriesResponse struct {
	Data TVDBSeriesMetadata `json:"data"`
}

type TVDBSeriesExtendedResponse struct {
	Data TVDBSeriesExtendedData `json:"data"`
}

type TVDBSeriesExtendedData struct {
	Slug    string   `json:"slug"`
	Genres  []Genres `json:"genres"`
	Aliases []Alias  `json:"aliases"`
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
