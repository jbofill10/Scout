package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"

	tvdb "shared/media"

	"github.com/gin-gonic/gin"
	"github.com/pelletier/go-toml"
)

type TvDbConfig struct {
	Host   string `toml:"host"`
	ApiKey string `toml:"api-key"`
	Token  string `toml:"token"`
}

var tvDbConfig TvDbConfig

var logger *log.Logger

func main() {

	logger = log.New(os.Stdout, "", log.Ldate|log.Ltime|log.Lmicroseconds|log.Lshortfile)

	if err := loadConfig(); err != nil {
		logger.Fatalf("Failed to load config: %v", err)
	}

	router := gin.Default()

	router.GET("/series", getSeries)
	router.GET("/series/:id/extended", getExtendedInformation)

	addr := os.Getenv("BIND_ADDRESS")
	if addr == "" {
		addr = "localhost:22000"
	}
	logger.Printf("Starting server on %s", addr)
	if err := router.Run(addr); err != nil {
		logger.Fatalf("Failed to run server: %v", err)
	}
}

func getSeries(c *gin.Context) {
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

	response, err := queryShow(mediaName, mediaType)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintln("Internal Server Error: Unable to query show", mediaName)})
		return
	}
	logger.Println("response", response)
	c.JSON(http.StatusOK, response)

}

// Queries TVDB for media
func queryShow(showName, mediaType string) ([]tvdb.Media, error) {

	results := []tvdb.Media{}

	uriQuery := fmt.Sprintf("/search?query=%s&type=%s", showName, mediaType)

	var searchResponse = &tvdb.TVDBSearchResponse{}
	result, err := tvDbGet(uriQuery)

	if err != nil {
		logger.Println("Unable to get show information:", err)
	}

	logger.Println("TVDB response:", string(result))

	// Attempt to marshal response
	if err := json.Unmarshal(result, &searchResponse); err != nil {
		logger.Println("Unable to unmarshal json response", err)
		return nil, err
	}

	ch := make(chan struct{}, 20)
	var wg sync.WaitGroup
	logger.Println(len(searchResponse.Data))
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
		logger.Println("Processing series data...")

		go func() {
			defer wg.Done()
			seriesResponse, err := querySeriesMetadata(mediaData.Id)
			if err != nil {
				logger.Println("Unable to process media", err)
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
		}()

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
			logger.Fatalf("Error unmarshalling config: %v", err)
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
		logger.Fatal("TVDB_HOST is required")
	}
	if tvDbConfig.ApiKey == "" {
		logger.Fatal("TVDB_API_KEY is required")
	}
	if tvDbConfig.Token == "" {
		logger.Fatal("TVDB_TOKEN is required")
	}

	return nil
}

func tvDbGet(uri string) ([]byte, error) {
	// Create a new HTTP request
	logger.Println("config: ", tvDbConfig.Host)
	req, err := http.NewRequest("GET", tvDbConfig.Host+uri, nil)
	if err != nil {
		logger.Println("Error making request")
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tvDbConfig.Token))

	client := &http.Client{}

	resp, err := client.Do(req)
	if err != nil {
		logger.Println("Error making request to TVDB")
		return nil, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Println("Error closing response body:", err)
		}
	}()

	// Read and print the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Println("Error reading response body:", err)
		return nil, err
	}

	return body, nil
}

func querySeriesMetadata(seriesId string) (tvdb.TVDBSeriesResponse, error) {

	queryUri := fmt.Sprintf("/series/%s/episodes/official/eng", seriesId)
	var seriesResponse = tvdb.TVDBSeriesResponse{}
	res, err := tvDbGet(queryUri)

	if err != nil {
		logger.Println("Unable to get show information:", err)
		return seriesResponse, err
	}

	if err := json.Unmarshal(res, &seriesResponse); err != nil {
		logger.Println("Unable to unmarshal json response", err)
		return seriesResponse, err
	}

	return seriesResponse, nil

}

func getExtendedInformation(c *gin.Context) {
	mediaId := c.Param("id")
	fmt.Printf("Fetching extended information for media ID: %s\n", mediaId)
	url := tvDbConfig.Host + fmt.Sprintf("/series/%s/extended", mediaId)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Internal Server Error: Unable to create request for media ID %s", mediaId)})
		return
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tvDbConfig.Token))

	client := &http.Client{}

	resp, err := client.Do(req)
	if err != nil {
		logger.Println("Error making request to TVDB")
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Internal Server Error: Unable to get extended information for media ID %s", mediaId)})
		return
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			logger.Printf("warning: failed to close response body: %v", cerr)
		}
	}()

	var info tvdb.TVDBSeriesExtendedResponse

	// unmarshal and print string body for debugging
	var debugBody string
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Printf("Failed to read response body for media ID %s: %v", mediaId, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Internal Server Error: Unable to read extended information for media ID %s", mediaId)})
		return
	}
	fmt.Println(string(bodyBytes))

	if err := json.Unmarshal(bodyBytes, &info); err != nil {
		// Try to unmarshal into a string for debugging
		_ = json.Unmarshal(bodyBytes, &debugBody)
		logger.Printf("Failed to decode response for media ID %s. Raw body as string: %s", mediaId, debugBody)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Internal Server Error: Unable to decode extended information for media ID %s", mediaId)})
		return
	}

	c.JSON(http.StatusOK, info)
}
