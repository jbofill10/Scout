package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/pelletier/go-toml"
)

type TvDbConfig struct {
	Host   string `toml:"host"`
	ApiKey string `toml:"api-key"`
}

var tvDbConfig TvDbConfig

var logger *log.Logger

func main() {

	logger = log.New(os.Stdout, "", log.Ldate|log.Ltime|log.Lmicroseconds|log.Lshortfile)

	loadConfig()

	router := gin.Default()

	router.GET("/series", getSeries)
	router.Run("localhost:22000")
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

	c.JSON(http.StatusOK, response)

}

// Queries TVDB for media
func queryShow(showName, mediaType string) ([]Media, error) {

	results := []Media{}

	uriQuery := fmt.Sprintf("/search?query=%s&type=%s", showName, mediaType)

	var searchResponse = &TVDBSearchResponse{}
	result, err := tvDbGet(uriQuery)

	if err != nil {
		logger.Println("Unable to get show information:", err)
	}

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
		mediaData := Media{
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

	conf, err := toml.LoadFile("api.toml")
	if err != nil {
		logger.Fatalf("Unable to load api config %v", err)
	}

	err = conf.Unmarshal(&tvDbConfig)

	if err != nil {
		logger.Fatalf("Error unmarshalling %v", err)
	}

	return nil
}

func tvDbGet(uri string) ([]byte, error) {
	// Create a new HTTP request
	req, err := http.NewRequest("GET", tvDbConfig.Host+uri, nil)
	if err != nil {
		logger.Println("Error making request")
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tvDbConfig.ApiKey))

	client := &http.Client{}

	resp, err := client.Do(req)
	if err != nil {
		logger.Println("Error making request to TVDB")
		return nil, err
	}
	defer resp.Body.Close()

	// Read and print the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Println("Error reading response body:", err)
		return nil, err
	}

	return body, nil
}

func querySeriesMetadata(seriesId string) (TVDBSeriesResponse, error) {

	queryUri := fmt.Sprintf("/series/%s/episodes/official/eng", seriesId)
	var seriesResponse = TVDBSeriesResponse{}
	res, err := tvDbGet(queryUri)

	if err != nil {
		logger.Println("Unable to get show information:", err)
		return seriesResponse, err
	}

	if err := json.Unmarshal(res, &seriesResponse); err != nil {
		logger.Println("Unable to unmarshal json response", err)
		return seriesResponse, err
	}

	// logger.Println(seriesResponse)
	return seriesResponse, nil

}
