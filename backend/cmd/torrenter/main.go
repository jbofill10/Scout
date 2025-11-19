package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"runtime/debug"

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

	// Initialize handlers
	downloadHandler := handlers.NewDownloadHandler(downloadInteractor, logger)
	mediaHandler := handlers.NewMediaHandler(mediaInteractor, logger)
	statusHandler := handlers.NewStatusHandler(statusInteractor, logger)

	// Setup routes
	r := gin.Default()
	r.Use(otelgin.Middleware("torrenter"))
	r.Use(RecoveryMiddleware(logger)) // Custom recovery middleware with trace logging
	r.POST("/download", downloadHandler.DownloadTorrent)
	r.GET("/media/:hash", mediaHandler.MediaExists)
	r.POST("/status/batch", statusHandler.BatchStatus)

	// Sync Plex library on startup (non-blocking)
	logger.Info("Starting Plex library sync in background")
	go plex.SyncPlexLibrary(context.Background())

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
