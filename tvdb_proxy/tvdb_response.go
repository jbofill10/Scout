package main

// Finalized json we send out
type Media struct {
	// TVDB Id for show
	Id string `json:"id"`
	// English translated name
	Name string `json:"mediaName"`
	// Either series or movie
	Category string `json:"type"`
	// Fuzzy popularity of show. Used to sort return result
	Score int `json:"score"`
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
	Id           int    `json:"id"`
	Name         string `json:"name"`
	Aired        string `json:"aired"`
	Image        string `json:"image"`
	Number       int    `json:"number"`
	SeasonNumber int    `json:"seasonNumber"`
}

type TVDBSeriesMetadata struct {
	FirstAired string    `json:"firstAired"`
	LastAired  string    `json:"lastAired"`
	Number     int       `json:"number"`
	Episodes   []Episode `json:"episodes"`
	// Found in the metadata, put makes sense to put score in TVDBSearchItem
	Score int `json:"score"`
}

type TVDBSeriesResponse struct {
	Data TVDBSeriesMetadata `json:"data"`
}
