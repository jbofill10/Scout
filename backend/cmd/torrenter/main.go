package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"runtime/debug"
	"time"

	"github.com/jbofill10/scout/backend/internal/torrenter/clients"
	"github.com/jbofill10/scout/backend/internal/torrenter/config"
	"github.com/jbofill10/scout/backend/internal/torrenter/handlers"
	"github.com/jbofill10/scout/backend/internal/torrenter/interactors"
	"github.com/jbofill10/scout/backend/internal/torrenter/repository"
	"github.com/jbofill10/scout/backend/internal/torrenter/service"
	"github.com/jbofill10/scout/backend/pkg/telemetry"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
)

// RecoveryMiddleware catches panics and logs them with full context to structured logger
func RecoveryMiddleware(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				// Get context for trace correlation
				ctx := c.Request.Context()

				// Get stack trace
				stack := debug.Stack()

				// Log panic with full context including trace_id and span_id
				logger.ErrorContext(ctx, "PANIC RECOVERED",
					"error", err,
					"method", c.Request.Method,
					"path", c.Request.URL.Path,
					"query", c.Request.URL.RawQuery,
					"stack_trace", string(stack))

				// Return 500 error
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"error": "Internal server error",
				})
			}
		}()

		c.Next()
	}
}

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
	mp := service.NewMediaProcessSvc(logger, repo, fs)
	plex := service.NewPlexHandler(repo, logger, &cfg.Plex, mp)

	// Initialize interactors
	downloadInteractor := interactors.NewDownloadInteractor(qbitt, mp, repo, logger)
	mediaInteractor := interactors.NewMediaInteractor(repo)
	statusInteractor := interactors.NewStatusInteractor(repo, logger)
	libraryInteractor := interactors.NewLibraryInteractor(repo, logger)

	// Initialize handlers
	downloadHandler := handlers.NewDownloadHandler(downloadInteractor, logger)
	mediaHandler := handlers.NewMediaHandler(mediaInteractor, logger)
	statusHandler := handlers.NewStatusHandler(statusInteractor, logger)
	libraryHandler := handlers.NewLibraryHandler(libraryInteractor, logger)
	plexProxyHandler := handlers.NewPlexProxyHandler(&cfg.Plex, logger)

	// Setup routes
	r := gin.Default()
	r.Use(otelgin.Middleware("torrenter"))
	r.Use(RecoveryMiddleware(logger)) // Custom recovery middleware with trace logging
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.GET("/ready", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if err := repo.Ping(ctx); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.POST("/download", downloadHandler.DownloadTorrent)
	r.GET("/media/:hash", mediaHandler.MediaExists)
	r.POST("/media/exists", mediaHandler.MediaExistsBatch)
	r.POST("/status/batch", statusHandler.BatchStatus)

	// Library routes
	r.GET("/library/shows", libraryHandler.GetShows)
	r.GET("/library/shows/:tvdbId/episodes", libraryHandler.GetShowEpisodes)
	r.GET("/library/shows/:tvdbId/metadata-status", libraryHandler.GetShowMetadataStatus)
	r.GET("/library/movies", libraryHandler.GetMovies)
	r.POST("/sync-episodes", libraryHandler.SyncEpisodes)

	// Plex proxy routes
	r.GET("/plex/thumb", plexProxyHandler.ProxyThumb)

	// Sync Plex library on startup (non-blocking, retries once on failure)
	logger.Info("Starting Plex library sync in background")
	go func() {
		if err := plex.SyncPlexLibrary(context.Background()); err != nil {
			logger.Error("Plex library sync failed, retrying in 1 minute", "error", err)
			time.Sleep(1 * time.Minute)
			if err := plex.SyncPlexLibrary(context.Background()); err != nil {
				logger.Error("Plex library sync retry failed", "error", err)
			}
		}
	}()

	// Resume monitors for torrents that were still downloading when this process
	// last stopped. Anything that finished during the downtime is picked up on
	// the first poll; without this a restart orphans in-flight downloads.
	logger.Info("Resuming in-flight torrent monitors")
	go func() {
		if err := downloadInteractor.ResumeInFlightDownloads(context.Background()); err != nil {
			logger.Error("Failed to resume in-flight torrent monitors", "error", err)
		}
	}()

	// Initialize webserver client for TVDB refresh
	webserverClient := clients.NewWebserverClient(cfg.Refresh.WebserverHost, logger)

	// Start TVDB refresh service (non-blocking)
	refreshService := service.NewRefreshService(repo, webserverClient, cfg.Refresh.RefreshInterval, logger)
	go refreshService.Start(context.Background())

	// Start cache cleanup goroutine
	ctx := context.Background()
	mp.StartCacheCleanup(ctx)

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
