package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	tvdb "shared/media"
	"shared/telemetry"

	"github.com/gin-gonic/gin"
	"github.com/pelletier/go-toml"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type TvDbConfig struct {
	Host   string `toml:"host"`
	ApiKey string `toml:"api-key"`
	Token  string `toml:"token"`
}

var tvDbConfig TvDbConfig

var logger *slog.Logger

// Genre cache structures
type genreCache struct {
	mu           sync.RWMutex
	genres       []tvdb.Genres
	nameToID     map[string]int
	lastRefresh  time.Time
	refreshEvery time.Duration
}

var cache = &genreCache{
	nameToID:     make(map[string]int),
	refreshEvery: 7 * 24 * time.Hour, // Refresh weekly as per design.md
}

func main() {
	// Initialize OpenTelemetry
	otlpEndpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if otlpEndpoint == "" {
		otlpEndpoint = "otel-collector-service:4317"
	}

	// Initialize tracing
	tracerCleanup, err := telemetry.InitTracer("tvdb-proxy", "1.0.0", otlpEndpoint)
	if err != nil {
		slog.Warn("Failed to initialize tracer", "error", err)
	} else {
		defer tracerCleanup()
		slog.Info("OpenTelemetry tracing initialized")
	}

	// Initialize logging with trace correlation
	var loggerCleanup func()
	logger, loggerCleanup, err = telemetry.InitLogger("tvdb-proxy", "1.0.0", otlpEndpoint)
	if err != nil {
		slog.Warn("Failed to initialize logger", "error", err)
		logger = slog.Default()
	} else {
		defer loggerCleanup()
		logger.Info("OpenTelemetry logging initialized")
	}

	if err := loadConfig(); err != nil {
		logger.Error("Failed to load config", "error", err)
		os.Exit(1)
	}

	// Initialize genre cache on startup
	ctx := context.Background()
	if err := cache.refresh(ctx); err != nil {
		logger.Warn("Failed to initialize genre cache on startup", "error", err)
	} else {
		logger.Info("Genre cache initialized", "genres_count", len(cache.genres))
	}

	router := gin.Default()
	router.Use(otelgin.Middleware("tvdb-proxy"))

	router.GET("/series", getSeries)
	router.GET("/series/:id/extended", getExtendedInformation)
	router.GET("/genres", getGenres)
	router.GET("/series/popular", getPopularSeries)
	router.GET("/movies/popular", getPopularMovies)

	addr := os.Getenv("BIND_ADDRESS")
	if addr == "" {
		addr = "localhost:22000"
	}
	logger.Info("Starting tvdb-proxy", "address", addr)
	if err := router.Run(addr); err != nil {
		logger.Error("Server failed", "error", err)
		os.Exit(1)
	}
}

func getSeries(c *gin.Context) {
	// Extract context for trace propagation
	ctx := c.Request.Context()

	mediaName := c.Query("mediaName")
	mediaType := c.Query("mediaType")

	if mediaName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Bad Request: 'mediaName' parameter is missing"})
		return
	}
	if mediaType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Bad Request: 'mediaType' parameter is missing"})
		return
	}

	response, err := queryShow(ctx, mediaName, mediaType)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintln("Internal Server Error: Unable to query show", mediaName)})
		return
	}
	logger.InfoContext(ctx, "response", telemetry.WithTraceContext(ctx, "value", response)...)
	c.JSON(http.StatusOK, response)

}

// Queries TVDB for media
func queryShow(ctx context.Context, showName, mediaType string) ([]tvdb.Media, error) {

	results := []tvdb.Media{}

	uriQuery := fmt.Sprintf("/search?query=%s&type=%s", showName, mediaType)

	var searchResponse = &tvdb.TVDBSearchResponse{}
	result, err := tvDbGet(ctx, uriQuery)

	if err != nil {
		logger.InfoContext(ctx, "Unable to get show information", telemetry.WithTraceContext(ctx, "value", err)...)
	}

	logger.InfoContext(ctx, "TVDB response", telemetry.WithTraceContext(ctx, "data", string(result))...)

	// Attempt to marshal response
	if err := json.Unmarshal(result, &searchResponse); err != nil {
		logger.InfoContext(ctx, "Unable to unmarshal json response", "value", err)
		return nil, err
	}

	ch := make(chan struct{}, 20)
	var wg sync.WaitGroup
	logger.InfoContext(ctx, "Search results count", "count", len(searchResponse.Data))
	for _, item := range searchResponse.Data {
		wg.Add(1)
		mediaData := tvdb.Media{
			Id:           item.Id,
			Name:         item.Translations.Eng,
			Category:     item.Category,
			ImageUrl:     item.ImageUrl,
			OriginalName: item.OriginalName,
			Status:       item.Status,
			Overview:     item.Overviews.Eng,
			Year:         item.Year,
		}
		ch <- struct{}{}
		logger.InfoContext(ctx, "Processing media data...")

		go func(ctx context.Context, mediaData tvdb.Media) {
			defer wg.Done()

			// Skip episode metadata fetching for movies
			if mediaData.Category == "movie" {
				// Set empty metadata for movies
				mediaData.Metadata = tvdb.TVDBSeriesMetadata{Episodes: []tvdb.Episode{}}
				results = append(results, mediaData)
				<-ch
				return
			}

			// Fetch episode metadata for series
			seriesResponse, err := querySeriesMetadata(ctx, mediaData.Id)
			if err != nil {
				logger.InfoContext(ctx, "Unable to process media", "value", err)
				<-ch
				return
			}

			mediaData.Metadata = seriesResponse.Data
			mediaData.Metadata.Episodes = seriesResponse.Data.Episodes
			mediaData.Metadata.Number = seriesResponse.Data.Number
			mediaData.Metadata.FirstAired = seriesResponse.Data.FirstAired
			mediaData.Metadata.LastAired = seriesResponse.Data.LastAired
			mediaData.Score = seriesResponse.Data.Score

			results = append(results, mediaData)
			<-ch
		}(ctx, mediaData)

	}

	wg.Wait()
	close(ch)

	// Sort by score in descending order
	sort.SliceStable(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	return results, nil
}

func loadConfig() error {
	// Try to load from api.toml first (optional for local dev)
	conf, err := toml.LoadFile("api.toml")
	if err == nil {
		if err := conf.Unmarshal(&tvDbConfig); err != nil {
			logger.Error("Error unmarshalling config", "error", err)
			os.Exit(1)
		}
	}

	// Override with environment variables (highest priority)
	if host := os.Getenv("TVDB_HOST"); host != "" {
		tvDbConfig.Host = host
	}
	if apiKey := os.Getenv("TVDB_API_KEY"); apiKey != "" {
		tvDbConfig.ApiKey = apiKey
	}
	if token := os.Getenv("TVDB_TOKEN"); token != "" {
		tvDbConfig.Token = token
	}

	// Validate required fields
	if tvDbConfig.Host == "" {
		logger.Error("TVDB_HOST is required")
		os.Exit(1)
	}
	if tvDbConfig.ApiKey == "" {
		logger.Error("TVDB_API_KEY is required")
		os.Exit(1)
	}
	if tvDbConfig.Token == "" {
		logger.Error("TVDB_TOKEN is required")
		os.Exit(1)
	}

	return nil
}

func tvDbGet(ctx context.Context, uri string) ([]byte, error) {
	// Create a new HTTP request
	logger.InfoContext(ctx, "Config", "host", tvDbConfig.Host)
	req, err := http.NewRequestWithContext(ctx, "GET", tvDbConfig.Host+uri, nil)
	if err != nil {
		logger.InfoContext(ctx, "Error making request")
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tvDbConfig.Token))

	client := &http.Client{
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}

	resp, err := client.Do(req)
	if err != nil {
		logger.InfoContext(ctx, "Error making request to TVDB", "error", err)
		return nil, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.InfoContext(ctx, "Error closing response body", "value", err)
		}
	}()

	// Read and print the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.InfoContext(ctx, "Error reading response body", "value", err)
		return nil, err
	}

	return body, nil
}

func querySeriesMetadata(ctx context.Context, seriesId string) (tvdb.TVDBSeriesResponse, error) {

	queryUri := fmt.Sprintf("/series/%s/episodes/official/eng", seriesId)
	var seriesResponse = tvdb.TVDBSeriesResponse{}
	res, err := tvDbGet(ctx, queryUri)

	if err != nil {
		logger.InfoContext(ctx, "Unable to get show information", "value", err)
		return seriesResponse, err
	}

	if err := json.Unmarshal(res, &seriesResponse); err != nil {
		logger.InfoContext(ctx, "Unable to unmarshal json response", "value", err)
		return seriesResponse, err
	}

	return seriesResponse, nil

}

// fetchTranslations fetches translation data for a given media ID and language code
// Returns a slice of aliases from the translation endpoint, or an empty slice on error
func fetchTranslations(ctx context.Context, mediaId string, language string, mediaType string) []tvdb.Alias {
	// Conditional translations endpoint based on media type
	var translationURL string
	if mediaType == "movie" {
		translationURL = tvDbConfig.Host + fmt.Sprintf("/movies/%s/translations/%s", mediaId, language)
	} else {
		translationURL = tvDbConfig.Host + fmt.Sprintf("/series/%s/translations/%s", mediaId, language)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", translationURL, nil)
	if err != nil {
		logger.ErrorContext(ctx, "Failed to create translations request", "media_id", mediaId, "language", language, "error", err)
		return []tvdb.Alias{}
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tvDbConfig.Token))

	client := &http.Client{
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}

	resp, err := client.Do(req)
	if err != nil {
		logger.ErrorContext(ctx, "Failed to fetch translations", "media_id", mediaId, "language", language, "error", err)
		return []tvdb.Alias{}
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			logger.ErrorContext(ctx, "warning: failed to close translations response body", "error", cerr)
		}
	}()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.ErrorContext(ctx, "Failed to read translations response body", "media_id", mediaId, "language", language, "error", err)
		return []tvdb.Alias{}
	}

	var translationInfo tvdb.TVDBTranslationResponse
	if err := json.Unmarshal(bodyBytes, &translationInfo); err != nil {
		logger.ErrorContext(ctx, "Failed to decode translations response", "media_id", mediaId, "language", language, "error", err)
		return []tvdb.Alias{}
	}

	// If we got a valid translation name, return it as an alias
	logger.InfoContext(ctx, "Fetched translation", "media_id", mediaId, "language", language, "name", translationInfo.Data.Name, "body", translationInfo)
	var aliases []tvdb.Alias
	for _, alias := range translationInfo.Data.Aliases {
		aliases = append(aliases, tvdb.Alias{
			Language: language,
			Name:     alias,
		})
	}
	return aliases

}

func getExtendedInformation(c *gin.Context) {
	mediaId := c.Param("id")
	mediaType := c.Query("mediaType")
	fmt.Printf("Fetching extended information for media ID: %s, type: %s\n", mediaId, mediaType)

	// Conditional endpoint logic based on media type
	var url string
	if mediaType == "movie" {
		url = tvDbConfig.Host + fmt.Sprintf("/movies/%s/extended", mediaId)
	} else {
		// Default to series for backwards compatibility
		url = tvDbConfig.Host + fmt.Sprintf("/series/%s/extended", mediaId)
	}

	// Extract context for trace propagation
	ctx := c.Request.Context()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Internal Server Error: Unable to create request for media ID %s", mediaId)})
		return
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tvDbConfig.Token))

	client := &http.Client{
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}

	resp, err := client.Do(req)
	if err != nil {
		logger.InfoContext(ctx, "Error making request to TVDB", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Internal Server Error: Unable to get extended information for media ID %s", mediaId)})
		return
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			logger.ErrorContext(ctx, "warning: failed to close response body", "error", cerr)
		}
	}()

	var info tvdb.TVDBSeriesExtendedResponse

	// unmarshal and print string body for debugging
	var debugBody string
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.ErrorContext(ctx, "Failed to read response body for media ID", "value", mediaId, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Internal Server Error: Unable to read extended information for media ID %s", mediaId)})
		return
	}
	fmt.Println(string(bodyBytes))

	if err := json.Unmarshal(bodyBytes, &info); err != nil {
		// Try to unmarshal into a string for debugging
		_ = json.Unmarshal(bodyBytes, &debugBody)
		logger.ErrorContext(ctx, "Failed to decode response", "media_id", mediaId, "body", debugBody)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Internal Server Error: Unable to decode extended information for media ID %s", mediaId)})
		return
	}

	// Determine if the media is anime by checking genres
	// For movies: check both "Anime" and "Animation" genres
	// For series: check only "Anime" genre (existing behavior)
	isAnime := false
	for _, genre := range info.Data.Genres {
		if mediaType == "movie" {
			if genre.Name == "Anime" || genre.Name == "Animation" {
				isAnime = true
				break
			}
		} else {
			if genre.Name == "Anime" {
				isAnime = true
				break
			}
		}
	}

	// Always fetch English translations
	logger.InfoContext(ctx, "Fetching English translations", "media_id", mediaId)
	engAliases := fetchTranslations(ctx, mediaId, "eng", mediaType)
	info.Data.Aliases = append(info.Data.Aliases, engAliases...)

	// If anime, also fetch Japanese translations
	if isAnime {
		logger.InfoContext(ctx, "Media is anime, fetching Japanese translations", "media_id", mediaId)
		jpnAliases := fetchTranslations(ctx, mediaId, "jpn", mediaType)
		info.Data.Aliases = append(info.Data.Aliases, jpnAliases...)
	}

	logger.InfoContext(ctx, "All aliases", "media_id", mediaId, "is_anime", isAnime, "aliases", info.Data.Aliases)

	// Filter aliases by language: always include 'eng', include 'jpn' if anime
	filteredAliases := []tvdb.Alias{}
	for _, alias := range info.Data.Aliases {
		if alias.Language == "eng" || (isAnime && alias.Language == "jpn") {
			filteredAliases = append(filteredAliases, alias)
		}
	}
	info.Data.Aliases = filteredAliases

	logger.InfoContext(ctx, "Filtered aliases", "media_id", mediaId, "is_anime", isAnime, "aliases", info.Data.Aliases)

	c.JSON(http.StatusOK, info)
}

// Genre cache methods
func (gc *genreCache) refresh(ctx context.Context) error {
	logger.InfoContext(ctx, "Refreshing genre cache")

	// Call TVDB API to fetch genres
	result, err := tvDbGet(ctx, "/genres")
	if err != nil {
		logger.ErrorContext(ctx, "Failed to fetch genres from TVDB", "error", err)
		return err
	}

	// Parse response
	var genresResponse struct {
		Status string        `json:"status"`
		Data   []tvdb.Genres `json:"data"`
	}

	if err := json.Unmarshal(result, &genresResponse); err != nil {
		logger.ErrorContext(ctx, "Failed to unmarshal genres response", "error", err)
		return err
	}

	// Update cache with write lock
	gc.mu.Lock()
	defer gc.mu.Unlock()

	gc.genres = genresResponse.Data
	gc.lastRefresh = time.Now()

	// Build name → ID mapping with case-insensitive keys
	gc.nameToID = make(map[string]int)
	for _, genre := range gc.genres {
		// Store by lowercase name for case-insensitive lookup
		gc.nameToID[strings.ToLower(genre.Name)] = genre.Id
		// Also store by slug
		gc.nameToID[strings.ToLower(genre.Slug)] = genre.Id
	}

	// Add "Sci-Fi" → "Science Fiction" alias as per design.md
	if id, ok := gc.nameToID["science fiction"]; ok {
		gc.nameToID["sci-fi"] = id
	}

	logger.InfoContext(ctx, "Genre cache refreshed", "genres_count", len(gc.genres))
	return nil
}

func (gc *genreCache) getGenres() []tvdb.Genres {
	gc.mu.RLock()
	defer gc.mu.RUnlock()
	return gc.genres
}

func (gc *genreCache) needsRefresh() bool {
	gc.mu.RLock()
	defer gc.mu.RUnlock()
	return time.Since(gc.lastRefresh) > gc.refreshEvery
}

func (gc *genreCache) lookupGenreID(name string) (int, bool) {
	gc.mu.RLock()
	defer gc.mu.RUnlock()
	id, ok := gc.nameToID[strings.ToLower(name)]
	return id, ok
}

// Handler: GET /genres
func getGenres(c *gin.Context) {
	ctx := c.Request.Context()

	// Refresh cache if needed
	if cache.needsRefresh() {
		logger.InfoContext(ctx, "Genre cache is stale, refreshing")
		if err := cache.refresh(ctx); err != nil {
			logger.ErrorContext(ctx, "Failed to refresh genre cache, returning cached data", "error", err)
			// Continue with cached data if available
		}
	}

	genres := cache.getGenres()
	if len(genres) == 0 {
		// Try to refresh if cache is empty
		if err := cache.refresh(ctx); err != nil {
			logger.ErrorContext(ctx, "Genre cache is empty and refresh failed", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch genres"})
			return
		}
		genres = cache.getGenres()
	}

	c.JSON(http.StatusOK, gin.H{"data": genres})
}

// Handler: GET /series/popular
func getPopularSeries(c *gin.Context) {
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
		genreID, ok := cache.lookupGenreID(genreName)
		if !ok {
			logger.WarnContext(ctx, "Genre not found in cache", "genre", genreName)
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Genre '%s' not found", genreName)})
			return
		}
		query += fmt.Sprintf("&genre=%d", genreID)
		logger.InfoContext(ctx, "Filtering by genre", "genre_name", genreName, "genre_id", genreID)
	}

	logger.InfoContext(ctx, "Fetching popular series", "query", query, "limit", limit)

	// Call TVDB API
	result, err := tvDbGet(ctx, query)
	if err != nil {
		logger.ErrorContext(ctx, "Failed to fetch popular series from TVDB", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch popular series"})
		return
	}

	// Parse response
	var filterResponse struct {
		Status string                `json:"status"`
		Data   []tvdb.TVDBSearchItem `json:"data"`
	}

	if err := json.Unmarshal(result, &filterResponse); err != nil {
		logger.ErrorContext(ctx, "Failed to unmarshal popular series response", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse TVDB response"})
		return
	}

	// Convert to Media objects (NO episode enrichment for performance)
	results := []tvdb.Media{}
	for i, item := range filterResponse.Data {
		if i >= limit {
			break
		}
		media := tvdb.Media{
			Id:           item.Id,
			Name:         item.Translations.Eng,
			Category:     "series",
			ImageUrl:     item.ImageUrl,
			OriginalName: item.OriginalName,
			Status:       item.Status,
			Overview:     item.Overviews.Eng,
			Year:         item.Year,
			Slug:         item.Slug,
			Metadata:     tvdb.TVDBSeriesMetadata{Episodes: []tvdb.Episode{}}, // Empty metadata
		}
		results = append(results, media)
	}

	logger.InfoContext(ctx, "Returning popular series", "count", len(results))
	c.JSON(http.StatusOK, results)
}

// Handler: GET /movies/popular
func getPopularMovies(c *gin.Context) {
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
		genreID, ok := cache.lookupGenreID(genreName)
		if !ok {
			logger.WarnContext(ctx, "Genre not found in cache", "genre", genreName)
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Genre '%s' not found", genreName)})
			return
		}
		query += fmt.Sprintf("&genre=%d", genreID)
		logger.InfoContext(ctx, "Filtering by genre", "genre_name", genreName, "genre_id", genreID)
	}

	logger.InfoContext(ctx, "Fetching popular movies", "query", query, "limit", limit)

	// Call TVDB API
	result, err := tvDbGet(ctx, query)
	if err != nil {
		logger.ErrorContext(ctx, "Failed to fetch popular movies from TVDB", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch popular movies"})
		return
	}

	// Parse response
	var filterResponse struct {
		Status string                `json:"status"`
		Data   []tvdb.TVDBSearchItem `json:"data"`
	}

	if err := json.Unmarshal(result, &filterResponse); err != nil {
		logger.ErrorContext(ctx, "Failed to unmarshal popular movies response", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse TVDB response"})
		return
	}

	// Convert to Media objects
	results := []tvdb.Media{}
	for i, item := range filterResponse.Data {
		if i >= limit {
			break
		}
		media := tvdb.Media{
			Id:           item.Id,
			Name:         item.Translations.Eng,
			Category:     "movie",
			ImageUrl:     item.ImageUrl,
			OriginalName: item.OriginalName,
			Status:       item.Status,
			Overview:     item.Overviews.Eng,
			Year:         item.Year,
			Slug:         item.Slug,
			Metadata:     tvdb.TVDBSeriesMetadata{Episodes: []tvdb.Episode{}}, // Empty metadata for movies
		}
		results = append(results, media)
	}

	logger.InfoContext(ctx, "Returning popular movies", "count", len(results))
	c.JSON(http.StatusOK, results)
}
