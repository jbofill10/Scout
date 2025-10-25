package main

import (
	"fmt"
	"log"
	"os"

	tvdb "shared/media"
	"webserver/internal/clients"
	"webserver/internal/config"
	"webserver/internal/handlers"
	"webserver/internal/interactors"
	"webserver/internal/repository"
	"webserver/internal/scheduler"

	"github.com/gin-gonic/gin"
)

func main() {
	logger := log.New(os.Stdout, "", log.LstdFlags|log.Lshortfile)

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		logger.Fatalf("failed to load config: %v", err)
	}

	// Initialize repository
	repo, err := repository.NewSchedulerRepo(logger, cfg.Database.ConnStr)
	if err != nil {
		logger.Fatalf("failed to init repo: %v", err)
	}

	// Initialize media queue for scheduled downloads
	queue := make(chan tvdb.Media, 100)

	// Initialize scheduler
	sched := scheduler.NewScheduler(repo, logger)

	// Initialize clients
	tvdbClient := clients.NewTVDBProxyClient(cfg.TVDBProxyHost)
	torrenterClient := clients.NewTorrenterClient(cfg.TorrenterHost)

	// Initialize interactors
	searchInteractor := interactors.NewSearchInteractor(tvdbClient)
	downloadInteractor := interactors.NewDownloadInteractor(
		repo,
		sched,
		queue,
		logger,
		tvdbClient,
		torrenterClient,
	)

	// Start watching for due media
	downloadInteractor.WatchForDueMedia()

	// Initialize handlers
	searchHandler := handlers.NewSearchHandler(searchInteractor, logger)
	downloadHandler := handlers.NewDownloadHandler(downloadInteractor, logger)

	// Setup routes
	r := gin.Default()
	r.Use(handlers.CorsMiddleware())
	r.GET("/search", searchHandler.HandleSearch)
	r.POST("/shows", downloadHandler.DownloadShow)
	r.POST("/movies", downloadHandler.DownloadMovie)

	// Start server
	fmt.Printf("Listening on %s\n", cfg.BindAddress)
	log.Fatal(r.Run(cfg.BindAddress))
}
