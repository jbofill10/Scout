package main

import (
	"testing"

	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSearchItemToMedia_Series(t *testing.T) {
	item := tvdb.TVDBSearchItem{
		Id:           "series-78857",
		Category:     "series",
		ImageUrl:     "/banners/v4/series/78857/posters/6069f97b9bad8.jpg",
		OriginalName: "ナルト",
		Slug:         "naruto",
		Year:         "2002",
		Translations: tvdb.Translations{Eng: "Naruto"},
		Overviews:    tvdb.Overview{Eng: "A young ninja."},
		Aliases:      []string{"Naruto Shippuden"},
	}

	media, ok := searchItemToMedia(item)

	require.True(t, ok)
	assert.Equal(t, "78857", media.Id)
	assert.Equal(t, "Naruto", media.Name)
	assert.Equal(t, "ナルト", media.OriginalName)
	assert.Equal(t, "series", media.Category)
	assert.Equal(t, "https://artworks.thetvdb.com/banners/v4/series/78857/posters/6069f97b9bad8.jpg", media.ImageUrl)
	assert.Equal(t, "A young ninja.", media.Overview)
	assert.Equal(t, []string{"Naruto Shippuden"}, media.Aliases)
	assert.False(t, media.Anime, "series hits carry no genres, so anime is not inferred here")
}

func TestSearchItemToMedia_KeepsAbsoluteImageUrl(t *testing.T) {
	media, ok := searchItemToMedia(tvdb.TVDBSearchItem{
		Id:       "movie-1",
		Category: "movie",
		ImageUrl: "https://artworks.thetvdb.com/banners/movies/1/posters/a.jpg",
	})

	require.True(t, ok)
	assert.Equal(t, "https://artworks.thetvdb.com/banners/movies/1/posters/a.jpg", media.ImageUrl)
}

func TestSearchItemToMedia_AnimeFromMovieGenres(t *testing.T) {
	media, ok := searchItemToMedia(tvdb.TVDBSearchItem{
		Id:       "movie-187028",
		Category: "movie",
		Genres:   []string{"Animation", "Fantasy"},
	})

	require.True(t, ok)
	assert.True(t, media.Anime, "Animation marks a movie as anime")
	assert.Equal(t, []string{}, media.Aliases, "missing aliases serialise as an empty list, not null")
}

func TestSearchItemToMedia_RejectsMalformedId(t *testing.T) {
	_, ok := searchItemToMedia(tvdb.TVDBSearchItem{Id: "78857", Category: "series"})

	assert.False(t, ok)
}

func TestIsAnimeGenre(t *testing.T) {
	tests := []struct {
		name      string
		genres    []string
		mediaType string
		want      bool
	}{
		{"anime series", []string{"Action", "Anime"}, "series", true},
		{"animation series is not anime", []string{"Animation"}, "series", false},
		{"animation movie is anime", []string{"Animation"}, "movie", true},
		{"anime movie", []string{"Anime"}, "movie", true},
		{"no genres", nil, "series", false},
		{"unrelated genres", []string{"Drama", "Comedy"}, "movie", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isAnimeGenre(tc.genres, tc.mediaType))
		})
	}
}

// TVDB does not send genres for series hits today; if it starts to, the same
// rule applies so anime series get flagged without an extended lookup.
func TestSearchItemToMedia_SeriesGenresWhenPresent(t *testing.T) {
	anime, ok := searchItemToMedia(tvdb.TVDBSearchItem{
		Id: "series-1", Category: "series", Genres: []string{"Anime", "Action"},
	})
	require.True(t, ok)
	assert.True(t, anime.Anime)

	cartoon, ok := searchItemToMedia(tvdb.TVDBSearchItem{
		Id: "series-2", Category: "series", Genres: []string{"Animation"},
	})
	require.True(t, ok)
	assert.False(t, cartoon.Anime, "Animation alone only marks movies")
}
