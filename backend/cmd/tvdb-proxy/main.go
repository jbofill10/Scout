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

	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/jbofill10/scout/backend/pkg/telemetry"

	"github.com/gin-gonic/gin"
	"github.com/pelletier/go-toml"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type TvDbConfig struct {
	Host   string `toml:"host"`
	ApiKey string `toml:"api-key"`
	Token  string `toml:"token"`
	Pin    string `toml:"pin"`
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

	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	router.GET("/ready", func(c *gin.Context) {
		if tvDbConfig.Token != "" {
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
		} else {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable"})
		}
	})

	router.GET("/series", getSeries)
	router.GET("/series/:id/extended", getExtendedInformation)
	router.POST("/series/batch/extended", getBatchExtendedInformation)
	router.POST("/series/batch/episodes", getBatchEpisodes)
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
		// Normalize image URL - prepend domain if relative path
		var imageUrl string
		if item.ImageUrl != "" && !strings.Contains(item.ImageUrl, "https") {
			imageUrl = "https://artworks.thetvdb.com" + item.ImageUrl
		} else {
			imageUrl = item.ImageUrl
		}

		// TODO: Remove later
		logger.InfoContext(ctx, "Is the URL here?", "image_url", item.ImageUrl)

		mediaData := tvdb.Media{
			Id:           strings.Split(item.Id, "-")[1],
			Name:         item.Translations.Eng,
			Category:     item.Category,
			ImageUrl:     imageUrl,
			OriginalName: item.OriginalName,
			Slug:         item.Slug,
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
				logger.WarnContext(ctx, "Failed to fetch episode metadata, returning partial result",
					"media_id", mediaData.Id,
					"media_name", mediaData.Name,
					"error", err)
				// Still add the media with empty metadata rather than dropping it
				mediaData.Metadata = tvdb.TVDBSeriesMetadata{Episodes: []tvdb.Episode{}}
				results = append(results, mediaData)
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
	if pin := os.Getenv("TVDB_PIN"); pin != "" {
		tvDbConfig.Pin = pin
	}

	tokenCtx, tokenCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer tokenCancel()
	token, err := getApiKey(tokenCtx)
	if err != nil {
		logger.Error("Unable to get API Key, exiting")
		os.Exit(1)
	}

	tvDbConfig.Token = token

	// Validate required fields
	if tvDbConfig.Host == "" {
		logger.Error("TVDB_HOST is required")
		os.Exit(1)
	}
	if tvDbConfig.ApiKey == "" {
		logger.Error("TVDB_API_KEY is required")
		os.Exit(1)
	}

	return nil
}

type tvdbLoginResponse struct {
	Data struct {
		Token string `json:"token"`
	} `json:"data"`
	Status string `json:"status"`
}

func getApiKey(ctx context.Context) (string, error) {
	// Prepare login request body with apikey and pin
	loginReq := map[string]string{
		"apikey": tvDbConfig.ApiKey,
	}
	// Only include pin if it's set (licensed keys don't need PIN)
	if tvDbConfig.Pin != "" {
		loginReq["pin"] = tvDbConfig.Pin
	}

	reqBody, err := json.Marshal(loginReq)
	if err != nil {
		logger.ErrorContext(ctx, "Error marshalling login request", "err", err)
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", tvDbConfig.Host+"/login", strings.NewReader(string(reqBody)))
	if err != nil {
		logger.InfoContext(ctx, "Error creating login request")
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Transport: otelhttp.NewTransport(http.DefaultTransport),
		Timeout:   30 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		logger.ErrorContext(ctx, "Error making request to TVDB", "err", err)
		return "", err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.InfoContext(ctx, "Error closing response body", "err", err)
		}
	}()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.ErrorContext(ctx, "Error reading response body", "err", err)
		return "", err
	}

	// Check for non-2xx status codes
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		logger.ErrorContext(ctx, "TVDB login failed", "status_code", resp.StatusCode, "response", string(body))
		return "", fmt.Errorf("TVDB login failed with status %d: %s", resp.StatusCode, string(body))
	}

	var lr tvdbLoginResponse
	if err := json.Unmarshal(body, &lr); err != nil {
		logger.ErrorContext(ctx, "Unable to parse login response body", "err", err, "body", string(body))
		return "", err
	}

	logger.InfoContext(ctx, "Successfully authenticated with TVDB API")
	return lr.Data.Token, nil

}

func tvDbGet(ctx context.Context, uri string) ([]byte, error) {
	// Create a new HTTP request
	logger.InfoContext(ctx, "Making TVDB API request", "uri", uri)
	req, err := http.NewRequestWithContext(ctx, "GET", tvDbConfig.Host+uri, nil)
	if err != nil {
		logger.InfoContext(ctx, "Error making request")
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tvDbConfig.Token))

	client := &http.Client{
		Transport: otelhttp.NewTransport(http.DefaultTransport),
		Timeout:   30 * time.Second,
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

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.InfoContext(ctx, "Error reading response body", "value", err)
		return nil, err
	}

	// Log response details for debugging
	preview := string(body)
	if len(body) > 500 {
		preview = string(body[:500]) + "..."
	}
	logger.InfoContext(ctx, "TVDB API response",
		"status_code", resp.StatusCode,
		"body_length", len(body),
		"body_preview", preview)

	// Check HTTP status code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		logger.ErrorContext(ctx, "TVDB API returned error",
			"status_code", resp.StatusCode,
			"response_body", string(body))
		return nil, fmt.Errorf("TVDB API error: status %d", resp.StatusCode)
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

	logger.InfoContext(
		ctx,
		"Series metadata querried",
		"seriesId", seriesId)
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
		Timeout:   30 * time.Second,
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

// fetchExtendedInformation fetches extended metadata for a single media item
// Returns the extended response or an error if the fetch fails
func fetchExtendedInformation(ctx context.Context, mediaId string, mediaType string) (*tvdb.TVDBSeriesExtendedResponse, error) {
	// Conditional endpoint logic based on media type
	var url string
	if mediaType == "movie" {
		url = tvDbConfig.Host + fmt.Sprintf("/movies/%s/extended", mediaId)
	} else {
		// Default to series for backwards compatibility
		url = tvDbConfig.Host + fmt.Sprintf("/series/%s/extended", mediaId)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tvDbConfig.Token))

	client := &http.Client{
		Transport: otelhttp.NewTransport(http.DefaultTransport),
		Timeout:   30 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request to TVDB: %w", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			logger.ErrorContext(ctx, "warning: failed to close response body", "error", cerr)
		}
	}()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var info tvdb.TVDBSeriesExtendedResponse
	if err := json.Unmarshal(bodyBytes, &info); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
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

	// Filter aliases by language: always include 'eng', include 'jpn' if anime
	filteredAliases := []tvdb.Alias{}
	for _, alias := range info.Data.Aliases {
		if alias.Language == "eng" || (isAnime && alias.Language == "jpn") {
			filteredAliases = append(filteredAliases, alias)
		}
	}
	info.Data.Aliases = filteredAliases

	// Fetch episode metadata for TV series to enable episode-level status display
	if mediaType == "series" {
		logger.InfoContext(ctx, "Fetching episode metadata for series", "media_id", mediaId)
		seriesMetadata, err := querySeriesMetadata(ctx, mediaId)
		if err == nil && len(seriesMetadata.Data.Episodes) > 0 {
			info.Data.Episodes = seriesMetadata.Data.Episodes
			logger.InfoContext(ctx, "Successfully fetched episode metadata", "media_id", mediaId, "episode_count", len(seriesMetadata.Data.Episodes))
		} else if err != nil {
			logger.ErrorContext(ctx, "Failed to fetch episode metadata", "media_id", mediaId, "error", err)
		} else {
			logger.WarnContext(ctx, "No episodes found for series", "media_id", mediaId)
		}
	}

	logger.InfoContext(ctx, "Fetched extended information", "media_id", mediaId, "is_anime", isAnime)
	return &info, nil
}

func getExtendedInformation(c *gin.Context) {
	mediaId := c.Param("id")
	mediaType := c.Query("mediaType")
	fmt.Printf("Fetching extended information for media ID: %s, type: %s\n", mediaId, mediaType)

	ctx := c.Request.Context()

	info, err := fetchExtendedInformation(ctx, mediaId, mediaType)
	if err != nil {
		logger.ErrorContext(ctx, "Failed to fetch extended information", "media_id", mediaId, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Unable to get extended information for media ID %s: %v", mediaId, err)})
		return
	}

	c.JSON(http.StatusOK, info)
}

// BatchExtendedRequest represents a single request in the batch
type BatchExtendedRequest struct {
	Id        string `json:"id" binding:"required"`
	MediaType string `json:"mediaType" binding:"required"`
}

// BatchExtendedResponse wraps a single response with optional error
type BatchExtendedResponse struct {
	Request *BatchExtendedRequest            `json:"request"`
	Data    *tvdb.TVDBSeriesExtendedResponse `json:"data,omitempty"`
	Error   string                           `json:"error,omitempty"`
}

// BatchEpisodesRequest represents a single request for episode metadata
type BatchEpisodesRequest struct {
	SeriesId string `json:"seriesId" binding:"required"`
}

// BatchEpisodesResponse wraps episode metadata with optional error
type BatchEpisodesResponse struct {
	Request *BatchEpisodesRequest    `json:"request"`
	Data    *tvdb.TVDBSeriesMetadata `json:"data,omitempty"`
	Error   string                   `json:"error,omitempty"`
}

// getBatchExtendedInformation handles batch requests for extended media information
// POST /series/batch/extended
// Input: Array of {id: string, mediaType: string}
// Output: Array of extended responses (with partial failure support)
func getBatchExtendedInformation(c *gin.Context) {
	ctx := c.Request.Context()

	var requests []BatchExtendedRequest
	if err := c.ShouldBindJSON(&requests); err != nil {
		logger.ErrorContext(ctx, "Invalid batch request body", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body: expected array of {id, mediaType}"})
		return
	}

	if len(requests) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Empty batch request"})
		return
	}

	logger.InfoContext(ctx, "Processing batch extended request", "count", len(requests))

	// Use buffered channel to limit concurrent requests (similar to episode enrichment pattern)
	ch := make(chan struct{}, 20)
	var wg sync.WaitGroup

	// Thread-safe result collection
	var mu sync.Mutex
	results := make([]BatchExtendedResponse, 0, len(requests))

	for _, req := range requests {
		wg.Add(1)
		ch <- struct{}{} // Acquire semaphore

		go func(ctx context.Context, request BatchExtendedRequest) {
			defer wg.Done()
			defer func() { <-ch }() // Release semaphore

			logger.InfoContext(ctx, "Fetching extended info in batch", "media_id", request.Id, "media_type", request.MediaType)

			// Fetch extended information
			info, err := fetchExtendedInformation(ctx, request.Id, request.MediaType)

			response := BatchExtendedResponse{
				Request: &request,
			}

			if err != nil {
				// Log error but don't fail the entire batch
				logger.WarnContext(ctx, "Failed to fetch extended info in batch",
					"media_id", request.Id,
					"media_type", request.MediaType,
					"error", err)
				response.Error = err.Error()
			} else {
				response.Data = info
			}

			// Thread-safe append
			mu.Lock()
			results = append(results, response)
			mu.Unlock()
		}(ctx, req)
	}

	wg.Wait()
	close(ch)

	logger.InfoContext(ctx, "Batch extended request completed", "total_requests", len(requests), "responses", len(results))
	c.JSON(http.StatusOK, results)
}

// getBatchEpisodes handles batch requests for series episode metadata
// POST /series/batch/episodes
// Input: Array of {seriesId: string}
// Output: Array of episode metadata responses (with partial failure support)
func getBatchEpisodes(c *gin.Context) {
	ctx := c.Request.Context()

	var requests []BatchEpisodesRequest
	if err := c.ShouldBindJSON(&requests); err != nil {
		logger.ErrorContext(ctx, "Invalid batch episodes request body", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body: expected array of {seriesId}"})
		return
	}

	if len(requests) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Empty batch request"})
		return
	}

	logger.InfoContext(ctx, "Processing batch episodes request", "count", len(requests))

	// Use buffered channel to limit concurrent requests
	ch := make(chan struct{}, 20)
	var wg sync.WaitGroup

	// Thread-safe result collection
	var mu sync.Mutex
	results := make([]BatchEpisodesResponse, 0, len(requests))

	for _, req := range requests {
		wg.Add(1)
		ch <- struct{}{} // Acquire semaphore

		go func(ctx context.Context, request BatchEpisodesRequest) {
			defer wg.Done()
			defer func() { <-ch }() // Release semaphore

			logger.InfoContext(ctx, "Fetching episodes in batch", "series_id", request.SeriesId)

			// Fetch episode metadata using existing querySeriesMetadata function
			seriesResponse, err := querySeriesMetadata(ctx, request.SeriesId)

			response := BatchEpisodesResponse{
				Request: &request,
			}

			if err != nil {
				// Log error but don't fail the entire batch
				logger.WarnContext(ctx, "Failed to fetch episodes in batch",
					"series_id", request.SeriesId,
					"error", err)
				response.Error = err.Error()
			} else {
				response.Data = &seriesResponse.Data
			}

			// Thread-safe append
			mu.Lock()
			results = append(results, response)
			mu.Unlock()
		}(ctx, req)
	}

	wg.Wait()
	close(ch)

	logger.InfoContext(ctx, "Batch episodes request completed", "total_requests", len(requests), "responses", len(results))
	c.JSON(http.StatusOK, results)
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
		logger.ErrorContext(ctx, "Failed to unmarshal popular series response",
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
		logger.ErrorContext(ctx, "Failed to unmarshal popular movies response",
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

	logger.InfoContext(ctx, "Returning popular movies", "count", len(results))
	c.JSON(http.StatusOK, results)
}
