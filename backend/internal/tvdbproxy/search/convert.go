package search

import (
	"strings"

	tvdb "github.com/jbofill10/scout/backend/pkg/media"
)

// searchItemToMedia maps one TVDB search hit onto the Media shape the rest of
// Scout consumes. It reports false when the hit's id is not in the expected
// "<type>-<id>" form.
//
// Search hits already carry aliases (series) and genres (movies), the two
// things the extended record used to be fetched for on every search result,
// so both are mapped here.
func searchItemToMedia(item tvdb.TVDBSearchItem) (tvdb.Media, bool) {
	parts := strings.SplitN(item.Id, "-", 2)
	if len(parts) != 2 {
		return tvdb.Media{}, false
	}

	aliases := item.Aliases
	if aliases == nil {
		aliases = []string{}
	}

	// TVDB only translates some names; fall back to the original so a hit
	// never shows up without a title.
	name := item.Translations.Eng
	if name == "" {
		name = item.OriginalName
	}

	return tvdb.Media{
		Id:               parts[1],
		Name:             name,
		Category:         item.Category,
		ImageUrl:         absoluteArtworkURL(item.ImageUrl),
		OriginalName:     item.OriginalName,
		Slug:             item.Slug,
		Status:           item.Status,
		Overview:         item.Overviews.Eng,
		Year:             item.Year,
		Aliases:          aliases,
		Genres:           item.Genres,
		Anime:            isAnimeGenre(item.Genres, item.Category),
		Director:         item.Director,
		Studios:          item.Studios,
		Network:          item.Network,
		OriginalCountry:  item.Country,
		OriginalLanguage: item.PrimaryLanguage,
		Metadata:         tvdb.TVDBSeriesMetadata{Episodes: []tvdb.Episode{}},
	}, true
}

// isAnimeGenre applies Scout's anime rule to a list of genre names: "Anime"
// marks anything, and "Animation" additionally marks movies (Ghibli, Shinkai).
func isAnimeGenre(genres []string, mediaType string) bool {
	for _, genre := range genres {
		if genre == "Anime" || (mediaType == "movie" && genre == "Animation") {
			return true
		}
	}
	return false
}
