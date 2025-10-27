package main

import (
	"fmt"
	"log/slog"
	"os"

	"shared/telemetry"
	"torrenter/internal/config"
	"torrenter/internal/handlers"
	"torrenter/internal/interactors"
	"torrenter/internal/repository"
	"torrenter/internal/service"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
)

func main() {
	// Initialize OpenTelemetry
	otlpEndpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if otlpEndpoint == "" {
		otlpEndpoint = "otel-collector-service:4317"
	}

	// Initialize tracing
	tracerCleanup, err := telemetry.InitTracer("torrenter", "1.0.0", otlpEndpoint)
	if err != nil {
		slog.Warn("Failed to initialize tracer", "error", err)
	} else {
		defer tracerCleanup()
		slog.Info("OpenTelemetry tracing initialized")
	}

	// Initialize logging with trace correlation
	logger, loggerCleanup, err := telemetry.InitLogger("torrenter", "1.0.0", otlpEndpoint)
	if err != nil {
		slog.Warn("Failed to initialize logger", "error", err)
		logger = slog.Default()
	} else {
		defer loggerCleanup()
		logger.Info("OpenTelemetry logging initialized")
	}

	cfg, err := config.Load()
	if err != nil {
		logger.Error("Failed to load config", "error", err)
		os.Exit(1)
	}

	// Database connection
	dbHost := os.Getenv("DB_HOST")
	dbUser := os.Getenv("DB_USER")
	dbPass := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")
	connStr := fmt.Sprintf("host=%s port=5432 user=%s password=%s dbname=%s sslmode=disable", dbHost, dbUser, dbPass, dbName)

	repo, err := repository.NewRepo(logger, connStr)
	if err != nil {
		logger.Error("Failed to create repo", "error", err)
		os.Exit(1)
	}

	qbitt, err := service.NewQbittHandler(&cfg.Qbitt, &cfg.Prowlarr, repo, logger)
	if err != nil {
		logger.Error("Failed to create qBittorrent handler", "error", err)
		os.Exit(1)
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
	r.Use(otelgin.Middleware("torrenter"))
	r.POST("/download", downloadHandler.DownloadTorrent)
	r.GET("/media/:hash", mediaHandler.MediaExists)

	// Sync Plex library on startup
	plex.SyncPlexLibrary()

	addr := os.Getenv("BIND_ADDRESS")
	if addr == "" {
		addr = "localhost:22001"
	}
	logger.Info("Starting torrenter", "address", addr)
	if err := r.Run(addr); err != nil {
		logger.Error("Server failed", "error", err)
		os.Exit(1)
	}
}
