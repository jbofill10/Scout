package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"torrenter/models"

	tvdb "shared/media"

	"github.com/gin-gonic/gin"
	"github.com/pelletier/go-toml"
)

var (
	baseSavePath = "/data/Downloads"
	scoutTag     = "scout"
)

type Interactor struct {
	mp     MediaProcessor
	plex   *PlexHandler
	repo   Repository
	qbitt  *QbittHandler
	logger *log.Logger
}

func main() {
	logger := log.New(os.Stdout, "", log.Ldate|log.Ltime|log.Lmicroseconds|log.Lshortfile)

	cfg, err := loadConfig()
	if err != nil {
		logger.Fatal(err)
	}

	// Database connection
	dbHost := os.Getenv("DB_HOST")
	dbUser := os.Getenv("DB_USER")
	dbPass := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")
	connStr := fmt.Sprintf("host=%s port=5432 user=%s password=%s dbname=%s sslmode=disable", dbHost, dbUser, dbPass, dbName)

	repo, err := NewRepo(logger, connStr)
	if err != nil {
		logger.Fatal(err)
	}

	qbitt, err := NewQbittHandler(&cfg.Qbitt, &cfg.Prowlarr, repo, logger)
	if err != nil {
		logger.Fatal("Failed to create qBittorrent handler: ", err)
	}

	fs := NewFsSvc(logger)

	interactor := &Interactor{
		repo:   repo,
		qbitt:  qbitt,
		mp:     NewMediaProcessSvc(logger, repo, fs),
		plex:   NewPlexHandler(repo, logger, &cfg.Plex),
		logger: logger,
	}

	r := gin.Default()
	r.POST("/download", interactor.DownloadTorrent)
	r.GET("/media/:hash", interactor.mediaExists)

	interactor.plex.syncPlexLibrary()

	addr := os.Getenv("BIND_ADDRESS")
	if addr == "" {
		addr = "localhost:22001"
	}
	logger.Printf("Starting server on %s", addr)
	if err := r.Run(addr); err != nil {
		logger.Fatal(err)
	}
}

func (i *Interactor) DownloadTorrent(c *gin.Context) {

	dlComplete := make(chan models.TorrentCompleteEvent, 1)

	var req tvdb.Media
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Filter out episodes that already exist in Plex
	if req.Category == "series" && len(req.Metadata.Episodes) > 0 {
		episodesToDownload := []tvdb.Episode{}
		for _, episode := range req.Metadata.Episodes {
			var exists bool
			var err error

			// Try TVDB ID matching first (more reliable)
			if req.Id != "" {
				exists, err = i.repo.EpisodeExistsByTvdbId(req.Id, episode.SeasonNumber, episode.Number)
				if err != nil {
					i.logger.Printf("Error checking episode existence by TVDB ID for S%02dE%02d: %v", episode.SeasonNumber, episode.Number, err)
					// Fall back to title matching
					exists, err = i.repo.EpisodeExists(req.Name, episode.SeasonNumber, episode.Number)
				} else {
					i.logger.Printf("Checked episode S%02dE%02d by TVDB ID %s: exists=%v", episode.SeasonNumber, episode.Number, req.Id, exists)
				}
			} else {
				// No TVDB ID available, use title matching
				exists, err = i.repo.EpisodeExists(req.Name, episode.SeasonNumber, episode.Number)
			}

			if err != nil {
				i.logger.Printf("Error checking episode existence for S%02dE%02d: %v", episode.SeasonNumber, episode.Number, err)
				// On error, proceed with download to be safe
				episodesToDownload = append(episodesToDownload, episode)
				continue
			}
			if exists {
				i.logger.Printf("Episode S%02dE%02d of %s already exists in Plex, skipping", episode.SeasonNumber, episode.Number, req.Name)
				continue
			}
			episodesToDownload = append(episodesToDownload, episode)
		}

		// If all episodes already exist, return success without downloading
		if len(episodesToDownload) == 0 {
			i.logger.Printf("All episodes of %s already exist in Plex", req.Name)
			c.JSON(http.StatusOK, gin.H{"message": "All episodes already exist"})
			return
		}

		// Update request with filtered episodes
		req.Metadata.Episodes = episodesToDownload
		i.logger.Printf("Downloading %d new episodes (filtered from %d total)", len(episodesToDownload), len(req.Metadata.Episodes))
	}

	err := i.qbitt.handleDownload(&req, dlComplete)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Torrent download initiated successfully"})

	go func() {
		event := <-dlComplete
		if err := i.mp.ProcessDownloadedTorrent(&event); err != nil {
			i.logger.Printf("Error processing downloaded torrent: %v", err)
		} else {
			i.logger.Printf("Successfully processed downloaded torrent: %s", event.Hash)
		}
	}()

}

func (i *Interactor) mediaExists(c *gin.Context) {
	id := c.Param("hash")
	exists, err := i.repo.MediaExists(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if exists {
		c.JSON(http.StatusOK, gin.H{})
	} else {
		c.JSON(http.StatusNotFound, gin.H{})
	}
}

func loadConfig() (models.TorrenterConf, error) {
	var cfg models.TorrenterConf

	// Try to load from config.toml first (optional for local dev)
	conf, err := toml.LoadFile("config.toml")
	if err == nil {
		if err := conf.Unmarshal(&cfg); err != nil {
			return models.TorrenterConf{}, err
		}
	}

	// Override with environment variables (takes precedence)
	if host := os.Getenv("PROWLARR_HOST"); host != "" {
		cfg.Prowlarr.Host = host
	}
	if key := os.Getenv("PROWLARR_KEY"); key != "" {
		cfg.Prowlarr.Key = key
	}
	if host := os.Getenv("QBITT_HOST"); host != "" {
		cfg.Qbitt.Host = host
	}
	if user := os.Getenv("QBITT_USER"); user != "" {
		cfg.Qbitt.User = user
	}
	if password := os.Getenv("QBITT_PASSWORD"); password != "" {
		cfg.Qbitt.Password = password
	}
	if endpoint := os.Getenv("UI_ENDPOINT"); endpoint != "" {
		cfg.Ui.Endpoint = endpoint
	}
	if host := os.Getenv("PLEX_HOST"); host != "" {
		cfg.Plex.Host = host
	}
	if key := os.Getenv("PLEX_KEY"); key != "" {
		cfg.Plex.Key = key
	}
	if sections := os.Getenv("PLEX_MOVIE_SECTIONS"); sections != "" {
		if val, err := strconv.Atoi(sections); err == nil {
			cfg.Plex.MovieSections = val
		}
	}
	if sections := os.Getenv("PLEX_SHOW_SECTIONS"); sections != "" {
		if val, err := strconv.Atoi(sections); err == nil {
			cfg.Plex.ShowSections = val
		}
	}

	// Load sensitive data from secrets files (K8s mounted secrets)
	if prowlarrKey, err := os.ReadFile("secrets/prowlarr-key"); err == nil {
		cfg.Prowlarr.Key = strings.TrimSpace(string(prowlarrKey))
	}
	if qbittUser, err := os.ReadFile("secrets/qbitt-user"); err == nil {
		cfg.Qbitt.User = strings.TrimSpace(string(qbittUser))
	}
	if qbittPassword, err := os.ReadFile("secrets/qbitt-password"); err == nil {
		cfg.Qbitt.Password = strings.TrimSpace(string(qbittPassword))
	}
	if plexKey, err := os.ReadFile("secrets/plex-key"); err == nil {
		cfg.Plex.Key = strings.TrimSpace(string(plexKey))
	}

	// Validate required fields
	if cfg.Prowlarr.Host == "" {
		return models.TorrenterConf{}, fmt.Errorf("PROWLARR_HOST is required")
	}
	if cfg.Qbitt.Host == "" {
		return models.TorrenterConf{}, fmt.Errorf("QBITT_HOST is required")
	}
	if cfg.Plex.Host == "" {
		return models.TorrenterConf{}, fmt.Errorf("PLEX_HOST is required")
	}

	return cfg, nil
}
