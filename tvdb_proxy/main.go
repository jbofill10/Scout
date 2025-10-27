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
	"strings"
	"sync"

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

	router := gin.Default()
	router.Use(otelgin.Middleware("tvdb-proxy"))

	router.GET("/series", getSeries)
	router.GET("/series/:id/extended", getExtendedInformation)

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
		logger.InfoContext(ctx, "Processing series data...")

		go func(ctx context.Context, mediaData tvdb.Media) {
			defer wg.Done()
			seriesResponse, err := querySeriesMetadata(ctx, mediaData.Id)
			if err != nil {
				logger.InfoContext(ctx, "Unable to process media", "value", err)
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
			logger.Error("Error unmarshalling config", "error", err); os.Exit(1)
		}
	}

	// Override with environment variables (takes precedence)
	if host := os.Getenv("TVDB_HOST"); host != "" {
		tvDbConfig.Host = host
	}
	if apiKey := os.Getenv("TVDB_API_KEY"); apiKey != "" {
		tvDbConfig.ApiKey = apiKey
	}
	if token := os.Getenv("TVDB_TOKEN"); token != "" {
		tvDbConfig.Token = token
	}

	// Load sensitive data from secrets files (K8s mounted secrets)
	if apiKey, err := os.ReadFile("secrets/api-key"); err == nil {
		tvDbConfig.ApiKey = strings.TrimSpace(string(apiKey))
	}
	if token, err := os.ReadFile("secrets/token"); err == nil {
		tvDbConfig.Token = strings.TrimSpace(string(token))
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
		logger.InfoContext(ctx, "Error making request to TVDB")
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

func getExtendedInformation(c *gin.Context) {
	mediaId := c.Param("id")
	fmt.Printf("Fetching extended information for media ID: %s\n", mediaId)
	url := tvDbConfig.Host + fmt.Sprintf("/series/%s/extended", mediaId)

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
		logger.InfoContext(ctx, "Error making request to TVDB")
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

	c.JSON(http.StatusOK, info)
}
