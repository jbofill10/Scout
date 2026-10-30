package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	tvdb "github.com/jbofill10/scout/backend/pkg/media"

	"github.com/gin-gonic/gin"
)

// TVDBGetter performs an authenticated TVDB GET of a path relative to the TVDB host.
type TVDBGetter interface {
	Get(ctx context.Context, uri string) ([]byte, error)
}

// GenreLookup resolves a genre name to its TVDB id.
type GenreLookup interface {
	LookupID(name string) (int, bool)
}

// PopularHandler serves GET /series/popular and GET /movies/popular.
type PopularHandler struct {
	client TVDBGetter
	genres GenreLookup
	logger *slog.Logger
}

// NewPopularHandler returns a PopularHandler that queries client and resolves genres through genres.
func NewPopularHandler(client TVDBGetter, genres GenreLookup, logger *slog.Logger) *PopularHandler {
	return &PopularHandler{client: client, genres: genres, logger: logger}
}

// GetPopularSeries handles GET /series/popular
func (h *PopularHandler) GetPopularSeries(c *gin.Context) {
	ctx := c.Request.Context()

	// Parse query parameters
	genreName := c.Query("genre")
	limitStr := c.DefaultQuery("limit", "20")

	// Parse and validate limit
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50 // Enforce maximum as per spec
	}

	// Build TVDB filter query
	query := "/series/filter?country=usa&lang=eng&sort=score&sortType=desc"

	// Add genre filter if provided
	if genreName != "" {
		genreID, ok := h.genres.LookupID(genreName)
		if !ok {
			h.logger.WarnContext(ctx, "Genre not found in cache", "genre", genreName)
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Genre '%s' not found", genreName)})
			return
		}
		query += fmt.Sprintf("&genre=%d", genreID)
		h.logger.InfoContext(ctx, "Filtering by genre", "genre_name", genreName, "genre_id", genreID)
	}

	h.logger.InfoContext(ctx, "Fetching popular series", "query", query, "limit", limit)

	// Call TVDB API
	result, err := h.client.Get(ctx, query)
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to fetch popular series from TVDB", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch popular series"})
		return
	}

	// Parse response - TVDB /filter endpoints return SeriesBaseRecord objects
	var filterResponse struct {
		Status string `json:"status"`
		Data   []struct {
			Id     int      `json:"id"`
			Name   string   `json:"name"`
			Image  string   `json:"image"`
			Slug   string   `json:"slug"`
			Year   string   `json:"year"`
			Status struct { // Status is an object with name, id, etc.
				Id   int    `json:"id"`
				Name string `json:"name"`
			} `json:"status"`
			FirstAired           string   `json:"firstAired"`
			LastAired            string   `json:"lastAired"`
			Country              string   `json:"country"`
			OriginalCountry      string   `json:"originalCountry"`
			OriginalLanguage     string   `json:"originalLanguage"`
			NameTranslations     []string `json:"nameTranslations"`
			OverviewTranslations []string `json:"overviewTranslations"`
			Score                float64  `json:"score"`
		} `json:"data"`
	}

	if err := json.Unmarshal(result, &filterResponse); err != nil {
		h.logger.ErrorContext(ctx, "Failed to unmarshal popular series response",
			"error", err,
			"response_body", string(result))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse TVDB response"})
		return
	}

	// Convert to Media objects (NO episode enrichment for performance)
	results := []tvdb.Media{}
	for i, item := range filterResponse.Data {
		if i >= limit {
			break
		}

		// Normalize image URL - prepend domain if relative path
		var imageUrl string
		if item.Image != "" && !strings.Contains(item.Image, "https") {
			imageUrl = "https://artworks.thetvdb.com" + item.Image
		} else {
			imageUrl = item.Image
		}

		media := tvdb.Media{
			Id:           strconv.Itoa(item.Id),
			Name:         item.Name,
			Category:     "series",
			ImageUrl:     imageUrl,
			OriginalName: item.Name, // SeriesBaseRecord doesn't have separate original name
			Status: tvdb.Status{ // Convert inline Status struct to media.Status
				Id:   item.Status.Id,
				Name: item.Status.Name,
			},
			Overview: "", // Filter endpoint doesn't return full overview text
			Year:     item.Year,
			Slug:     item.Slug,
			Metadata: tvdb.TVDBSeriesMetadata{Episodes: []tvdb.Episode{}}, // Empty metadata
		}
		results = append(results, media)
	}

	h.logger.InfoContext(ctx, "Returning popular series", "count", len(results))
	c.JSON(http.StatusOK, results)
}

// GetPopularMovies handles GET /movies/popular
func (h *PopularHandler) GetPopularMovies(c *gin.Context) {
	ctx := c.Request.Context()

	// Parse query parameters
	genreName := c.Query("genre")
	limitStr := c.DefaultQuery("limit", "20")

	// Parse and validate limit
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50 // Enforce maximum as per spec
	}

	// Build TVDB filter query
	query := "/movies/filter?country=usa&lang=eng&sort=score&sortType=desc"

	// Add genre filter if provided
	if genreName != "" {
		genreID, ok := h.genres.LookupID(genreName)
		if !ok {
			h.logger.WarnContext(ctx, "Genre not found in cache", "genre", genreName)
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Genre '%s' not found", genreName)})
			return
		}
		query += fmt.Sprintf("&genre=%d", genreID)
		h.logger.InfoContext(ctx, "Filtering by genre", "genre_name", genreName, "genre_id", genreID)
	}

	h.logger.InfoContext(ctx, "Fetching popular movies", "query", query, "limit", limit)

	// Call TVDB API
	result, err := h.client.Get(ctx, query)
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to fetch popular movies from TVDB", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch popular movies"})
		return
	}

	// Parse response - TVDB /filter endpoints return MovieBaseRecord objects (similar structure to SeriesBaseRecord)
	var filterResponse struct {
		Status string `json:"status"`
		Data   []struct {
			Id     int      `json:"id"`
			Name   string   `json:"name"`
			Image  string   `json:"image"`
			Slug   string   `json:"slug"`
			Year   string   `json:"year"`
			Status struct { // Status is an object with name, id, etc.
				Id   int    `json:"id"`
				Name string `json:"name"`
			} `json:"status"`
			FirstAired           string   `json:"firstAired"`
			LastAired            string   `json:"lastAired"`
			Country              string   `json:"country"`
			OriginalCountry      string   `json:"originalCountry"`
			OriginalLanguage     string   `json:"originalLanguage"`
			NameTranslations     []string `json:"nameTranslations"`
			OverviewTranslations []string `json:"overviewTranslations"`
			Score                float64  `json:"score"`
		} `json:"data"`
	}

	if err := json.Unmarshal(result, &filterResponse); err != nil {
		h.logger.ErrorContext(ctx, "Failed to unmarshal popular movies response",
			"error", err,
			"response_body", string(result))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse TVDB response"})
		return
	}

	// Convert to Media objects
	results := []tvdb.Media{}
	for i, item := range filterResponse.Data {
		if i >= limit {
			break
		}

		var imageUrl string

		if !strings.Contains(item.Image, "https") {
			imageUrl = "https://artworks.thetvdb.com" + item.Image
		} else {
			imageUrl = item.Image
		}

		media := tvdb.Media{
			Id:           strconv.Itoa(item.Id),
			Name:         item.Name,
			Category:     "movie",
			ImageUrl:     imageUrl,
			OriginalName: item.Name, // MovieBaseRecord doesn't have separate original name
			Status: tvdb.Status{ // Convert inline Status struct to media.Status
				Id:   item.Status.Id,
				Name: item.Status.Name,
			},
			Overview: "", // Filter endpoint doesn't return full overview text
			Year:     item.Year,
			Slug:     item.Slug,
			Metadata: tvdb.TVDBSeriesMetadata{Episodes: []tvdb.Episode{}}, // Empty metadata for movies
		}
		results = append(results, media)
	}

	h.logger.InfoContext(ctx, "Returning popular movies", "count", len(results))
	c.JSON(http.StatusOK, results)
}
