package main

import (
	"fmt"
	"log"
	"os"
	"torrenter/internal/config"
	"torrenter/internal/handlers"
	"torrenter/internal/interactors"
	"torrenter/internal/repository"
	"torrenter/internal/service"

	"github.com/gin-gonic/gin"
)

func main() {
	logger := log.New(os.Stdout, "", log.Ldate|log.Ltime|log.Lmicroseconds|log.Lshortfile)

	cfg, err := config.Load()
	if err != nil {
		logger.Fatal(err)
	}

	// Database connection
	dbHost := os.Getenv("DB_HOST")
	dbUser := os.Getenv("DB_USER")
	dbPass := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")
	connStr := fmt.Sprintf("host=%s port=5432 user=%s password=%s dbname=%s sslmode=disable", dbHost, dbUser, dbPass, dbName)

	repo, err := repository.NewRepo(logger, connStr)
	if err != nil {
		logger.Fatal(err)
	}

	qbitt, err := service.NewQbittHandler(&cfg.Qbitt, &cfg.Prowlarr, repo, logger)
	if err != nil {
		logger.Fatal("Failed to create qBittorrent handler: ", err)
	}

	fs := service.NewFsSvc(logger)
	plex := service.NewPlexHandler(repo, logger, &cfg.Plex)
	mp := service.NewMediaProcessSvc(logger, repo, fs)

	// Initialize interactors
	downloadInteractor := interactors.NewDownloadInteractor(qbitt, mp, repo, logger)
	mediaInteractor := interactors.NewMediaInteractor(repo)

	// Initialize handlers
	downloadHandler := handlers.NewDownloadHandler(downloadInteractor, logger)
	mediaHandler := handlers.NewMediaHandler(mediaInteractor, logger)

	// Setup routes
	r := gin.Default()
	r.POST("/download", downloadHandler.DownloadTorrent)
	r.GET("/media/:hash", mediaHandler.MediaExists)

	// Sync Plex library on startup
	plex.SyncPlexLibrary()

	addr := os.Getenv("BIND_ADDRESS")
	if addr == "" {
		addr = "localhost:22001"
	}
	logger.Printf("Starting server on %s", addr)
	if err := r.Run(addr); err != nil {
		logger.Fatal(err)
	}
}
