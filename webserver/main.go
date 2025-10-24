package main

import (
	"fmt"
	"log"
	"os"
	"time"

	tvdb "shared/media"
	status "shared/status"

	"github.com/gin-gonic/gin"
)

type SearchRequest struct {
	Query     string `json:"query"`
	MediaType string `json:"media_type"`
}

type DownloadRequest struct {
	Query string `json:"query"`
}

type Interactor struct {
	s               *Scheduler
	repo            SchedulerRepository
	logger          *log.Logger
	mediaQueue      chan tvdb.Media
	tvdbClient      *TVDBProxyClient
	torrenterClient *TorrenterClient
}

func NewInteractor(repo SchedulerRepository, s *Scheduler, mediaQueue chan tvdb.Media, logger *log.Logger) *Interactor {
	tvdbHost := os.Getenv("TVDB_PROXY_HOST")
	if tvdbHost == "" {
		tvdbHost = "localhost:22000"
	}
	torrenterHost := os.Getenv("TORRENTER_HOST")
	if torrenterHost == "" {
		torrenterHost = "localhost:22001"
	}
	tvdbClient := NewTVDBProxyClient(tvdbHost)
	torrenterClient := NewTorrenterClient(torrenterHost)
	return &Interactor{
		repo:            repo,
		s:               s,
		mediaQueue:      mediaQueue,
		logger:          logger,
		tvdbClient:      tvdbClient,
		torrenterClient: torrenterClient,
	}
}

func main() {
	logger := log.New(os.Stdout, "", log.LstdFlags|log.Lshortfile)

	// Database connection
	dbHost := os.Getenv("DB_HOST")
	if dbHost == "" {
		dbHost = "postgres-service"
	}
	dbUser := os.Getenv("DB_USER")
	if dbUser == "" {
		dbUser = "scoutuser"
	}
	dbPass := os.Getenv("DB_PASSWORD")
	if dbPass == "" {
		dbPass = "scoutpass"
	}
	dbName := os.Getenv("DB_NAME")
	if dbName == "" {
		dbName = "scoutdb"
	}
	connStr := fmt.Sprintf("host=%s port=5432 user=%s password=%s dbname=%s sslmode=disable", dbHost, dbUser, dbPass, dbName)

	repo, err := NewSchedulerRepo(logger, connStr)
	if err != nil {
		logger.Fatalf("failed to init repo: %v", err)
	}

	queue := make(chan tvdb.Media, 100)

	s := NewScheduler(repo, logger)
	i := NewInteractor(repo, s, queue, logger)

	i.WatchForDueMedia()

	r := gin.Default()
	r.Use(corsMiddleware())
	r.GET("/search", i.handleSearch)
	r.POST("/shows", i.downloadShow)
	r.POST("/movies", i.downloadMovie)

	addr := os.Getenv("BIND_ADDRESS")
	if addr == "" {
		addr = "0.0.0.0:22920" // Bind to all interfaces by default
	}
	fmt.Printf("Listening on %s\n", addr)
	log.Fatal(r.Run(addr))
}

func (i *Interactor) WatchForDueMedia() {
	i.s.Start(i.mediaQueue)
	go func() {
		for media := range i.mediaQueue {
			i.logger.Printf("Processing due media: %s", media.Name)

			// Media is already complete and ready for Download()
			err := i.torrenterClient.Download(media)
			if err != nil {
				i.logger.Printf("Failed to download media: %v", err)
				// Log failure for each episode in the media
				for _, ep := range media.Metadata.Episodes {
					_ = i.repo.InsertDownloadHistory(media.Name, ep.SeasonNumber, ep.Number, ep.AbsoluteNumber,
						status.Failure, "[Scheduled Download Error]: "+err.Error())
				}
			}
		}
	}()
}

func (i *Interactor) handleSearch(c *gin.Context) {
	query := c.Query("query")
	mediaType := c.Query("media_type")
	i.logger.Printf("SearchRequest: query=%s, media_type=%s", query, mediaType)

	// Use the TVDBProxyClient for service discovery
	searchResults, err := i.tvdbClient.Search(mediaType, query)
	if err != nil {
		i.logger.Printf("Failed to search via TVDB client: %v", err)
		c.JSON(500, gin.H{"error": "Failed to search media"})
		return
	}

	i.logger.Printf("Returning %d search results", len(searchResults))
	c.JSON(200, searchResults)
}

func (i *Interactor) downloadShow(c *gin.Context) {

	var req tvdb.Media
	if err := c.ShouldBindJSON(&req); err != nil {
		i.logger.Printf("Bad request: %v", err)
		c.JSON(400, gin.H{"error": "Bad Request"})
		return
	}

	extendedInfo, err := i.getExtendedInformation(req.Id)
	if err != nil {
		i.logger.Printf("Failed to get extended information: %v", err)
		c.JSON(500, gin.H{"error": "Failed to get extended information"})
		return
	}

	isAnime := isMediaAnime(extendedInfo)

	today := time.Now()
	i.logger.Printf("Download request for: %+v", req)
	mediaToDownload := []tvdb.Episode{}
	absoluteNumber := 1

	for _, episode := range req.Metadata.Episodes {
		episodeAired, err := time.Parse("2006-01-02", episode.Aired)
		if err != nil {
			i.logger.Printf("Failed to parse episode aired date: %v", err)
			continue
		}

		if isAnime {
			episode.AbsoluteNumber = absoluteNumber
			absoluteNumber++
		}

		// Has the episode aired yet?
		if today.After(episodeAired) {
			// Episode has aired - add to immediate download batch
			mediaToDownload = append(mediaToDownload, episode)
			err = i.repo.InsertDownloadHistory(req.Name, episode.SeasonNumber, episode.Number, episode.AbsoluteNumber,
				status.Searching, "")
			if err != nil {
				i.logger.Printf("Failed to insert download history for episode %d: %v", episode.Number, err)
			}
		} else {
			// Episode hasn't aired - schedule it for future download
			scheduledMedia := tvdb.Media{
				Id:           req.Id,
				Name:         req.Name,
				Category:     req.Category,
				Anime:        isAnime,
				Score:        req.Score,
				Slug:         req.Slug,
				ImageUrl:     req.ImageUrl,
				OriginalName: req.OriginalName,
				Status:       req.Status,
				Overview:     req.Overview,
				Year:         req.Year,
			}
			scheduledMedia.Metadata.Episodes = []tvdb.Episode{episode}

			err := i.repo.Schedule(scheduledMedia, episodeAired)
			if err != nil {
				// Check if it's a duplicate (already scheduled)
				if err == ErrDuplicateScheduled {
					i.logger.Printf("Episode S%02dE%02d already scheduled, skipping", episode.SeasonNumber, episode.Number)
					continue
				}
				// Other scheduling error - log failure
				i.logger.Printf("Failed to schedule episode: %v", err)
				err = i.repo.InsertDownloadHistory(req.Name, episode.SeasonNumber, episode.Number, episode.AbsoluteNumber,
					status.Failure, "[Scheduling Error]: "+err.Error())
				if err != nil {
					i.logger.Printf("Failed to insert download history: %v", err)
				}
			} else {
				i.logger.Printf("Scheduled S%02dE%02d for %s", episode.SeasonNumber, episode.Number, episodeAired.Format("2006-01-02"))
			}
		}
	}

	// Only send download request if there are aired episodes
	if len(mediaToDownload) > 0 {
		downloadPayload := tvdb.Media{
			Id:       req.Id,
			Name:     req.Name,
			Category: req.Category,
			Anime:    isAnime,
			Slug:     req.Slug,
			Year:     req.Year,
			Metadata: req.Metadata,
		}
		downloadPayload.Metadata.Episodes = mediaToDownload

		err = i.torrenterClient.Download(downloadPayload)
		if err != nil {
			i.logger.Printf("Failed to download show: %v", err)
			c.JSON(500, gin.H{"error": "Failed to download show"})
			return
		}
		i.logger.Printf("Sent %d aired episodes to torrenter for download", len(mediaToDownload))
	} else {
		i.logger.Printf("No aired episodes to download immediately")
	}

	i.logger.Printf("Successfully processed show: %s", req.Name)
	c.JSON(200, gin.H{})
}

func (i *Interactor) downloadMovie(c *gin.Context) {
	// TODO
}

func (i *Interactor) getExtendedInformation(mediaId string) (tvdb.TVDBSeriesExtendedResponse, error) {
	// Use the TVDBProxyClient to get extended info
	i.logger.Printf("Requesting extended info for mediaId: %s", mediaId)
	seriesInfo, err := i.tvdbClient.GetExtendedInfo(mediaId)
	if err != nil {
		i.logger.Printf("Failed to get extended info from TVDBProxyClient: %v", err)
		return tvdb.TVDBSeriesExtendedResponse{}, err
	}
	return seriesInfo, nil
}

func isMediaAnime(req tvdb.TVDBSeriesExtendedResponse) bool {
	for _, genre := range req.Data.Genres {
		if genre.Name == "Anime" {
			return true
		}
	}
	return false
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(200)
			return
		}
		c.Next()
	}
}
