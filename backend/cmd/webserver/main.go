package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jbofill10/scout/backend/internal/webserver/cache"
	"github.com/jbofill10/scout/backend/internal/webserver/clients"
	"github.com/jbofill10/scout/backend/internal/webserver/config"
	"github.com/jbofill10/scout/backend/internal/webserver/handlers"
	"github.com/jbofill10/scout/backend/internal/webserver/interactors"
	"github.com/jbofill10/scout/backend/internal/webserver/repository"
	"github.com/jbofill10/scout/backend/internal/webserver/scheduler"
	"github.com/jbofill10/scout/backend/pkg/telemetry"

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
	tracerCleanup, err := telemetry.InitTracer("webserver", "1.0.0", otlpEndpoint)
	if err != nil {
		slog.Warn("Failed to initialize tracer", "error", err)
	} else {
		defer tracerCleanup()
		slog.Info("OpenTelemetry tracing initialized")
	}

	// Initialize logging with trace correlation
	logger, loggerCleanup, err := telemetry.InitLogger("webserver", "1.0.0", otlpEndpoint)
	if err != nil {
		slog.Warn("Failed to initialize logger", "error", err)
		logger = slog.Default()
	} else {
		defer loggerCleanup()
		logger.Info("OpenTelemetry logging initialized")
	}

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		logger.Error("Failed to load config", "error", err)
		os.Exit(1)
	}

	// Initialize repositories
	schedulerRepo, err := repository.NewSchedulerRepo(logger, cfg.Database.ConnStr)
	if err != nil {
		logger.Error("Failed to init scheduler repo", "error", err)
		os.Exit(1)
	}

	notificationRepo, err := repository.NewNotificationRepo(logger, cfg.Database.ConnStr)
	if err != nil {
		logger.Error("Failed to init notification repo", "error", err)
		os.Exit(1)
	}

	// Initialize media queue for scheduled downloads
	queue := make(chan repository.DueItem, 100)

	// Initialize scheduler
	sched := scheduler.NewScheduler(schedulerRepo, logger)

	// Initialize clients
	tvdbClient := clients.NewTVDBProxyClient(cfg.TVDBProxyHost)
	torrenterClient := clients.NewTorrenterClient(cfg.TorrenterHost)

	// Initialize interactors
	searchInteractor := interactors.NewSearchInteractor(tvdbClient, logger)
	enrichedSearchInteractor := interactors.NewEnrichedSearchInteractor(
		searchInteractor,
		tvdbClient,
		torrenterClient,
		logger,
	)
	popularEnrichedInteractor := interactors.NewPopularEnrichedInteractor(
		tvdbClient,
		torrenterClient,
		logger,
	)
	downloadInteractor := interactors.NewDownloadInteractor(
		schedulerRepo,
		notificationRepo,
		sched,
		queue,
		logger,
		tvdbClient,
		torrenterClient,
	)

	// Serve popular content from a background-refreshed cache so the home page
	// does not wait on TVDB for every load.
	popularCache := cache.NewPopularCache()
	popularRefresher := cache.NewPopularRefresher(
		popularCache,
		tvdbClient,
		torrenterClient,
		popularEnrichedInteractor,
		logger,
	)
	logger.Info("Starting popular content cache refresher")
	popularRefresher.Start()

	// Start watching for due media
	downloadInteractor.WatchForDueMedia()

	// Setup graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start notification cleanup job
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()

		// Run cleanup immediately on startup
		ctx := context.Background()
		logger.Info("Running initial notification cleanup")
		if err := notificationRepo.CleanupOld(ctx, 30); err != nil {
			logger.Error("Failed to cleanup old notifications", "error", err)
		}

		// Run cleanup daily
		for range ticker.C {
			logger.Info("Running scheduled notification cleanup")
			if err := notificationRepo.CleanupOld(ctx, 30); err != nil {
				logger.Error("Failed to cleanup old notifications", "error", err)
			}
		}
	}()

	// Initialize handlers
	searchHandler := handlers.NewSearchHandler(searchInteractor, logger)
	enrichedSearchHandler := handlers.NewEnrichedSearchHandler(enrichedSearchInteractor, logger)
	downloadHandler := handlers.NewDownloadHandler(downloadInteractor, logger)
	popularHandler := handlers.NewPopularHandler(tvdbClient, popularCache, logger)
	popularEnrichedHandler := handlers.NewPopularEnrichedHandler(popularEnrichedInteractor, popularCache, logger)
	scheduleHandler := handlers.NewScheduleHandler(schedulerRepo, logger)
	notificationHandler := handlers.NewNotificationHandler(notificationRepo, logger)
	statusHandler := handlers.NewStatusHandler(torrenterClient, logger)
	mediaExtendedHandler := handlers.NewMediaExtendedHandler(tvdbClient, logger)
	libraryHandler := handlers.NewLibraryHandler(torrenterClient, logger)
	tvdbHandler := handlers.NewTVDBHandler(tvdbClient, logger)
	activityInteractor := interactors.NewActivityInteractor(notificationRepo, schedulerRepo, logger)
	activityHandler := handlers.NewActivityHandler(activityInteractor, logger)

	// Setup routes
	r := gin.Default()
	r.Use(otelgin.Middleware("webserver"))
	r.Use(handlers.CorsMiddleware())
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.GET("/ready", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if err := schedulerRepo.Ping(ctx); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.GET("/search", searchHandler.HandleSearch)
	r.GET("/search/enriched", enrichedSearchHandler.HandleEnrichedSearch)
	r.POST("/shows", downloadHandler.DownloadShow)
	r.POST("/movies", downloadHandler.DownloadMovie)
	r.GET("/popular/shows", popularHandler.GetPopularShows)
	r.GET("/popular/movies", popularHandler.GetPopularMovies)
	r.GET("/popular/shows/enriched", popularEnrichedHandler.HandleEnrichedPopularShows)
	r.GET("/popular/movies/enriched", popularEnrichedHandler.HandleEnrichedPopularMovies)
	r.GET("/genres", popularHandler.GetGenres)
	r.POST("/status/batch", statusHandler.GetBatchStatus)
	r.POST("/media/batch-extended", mediaExtendedHandler.GetBatchExtended)
	r.GET("/schedule/weekly", scheduleHandler.GetWeeklySchedule)

	// TVDB routes
	r.POST("/tvdb/batch/episodes", tvdbHandler.GetEpisodesBatch)

	// Library routes
	r.GET("/library/shows", libraryHandler.GetShows)
	r.GET("/library/shows/:tvdbId", libraryHandler.GetShowDetails)
	r.GET("/library/shows/:tvdbId/metadata-status", libraryHandler.GetShowMetadataStatus)
	r.GET("/library/movies", libraryHandler.GetMovies)

	// Plex proxy routes
	r.GET("/plex/thumb", libraryHandler.ProxyPlexThumb)
	// In-flight and recently finished downloads, with stage and retry details
	r.GET("/activity", activityHandler.GetActivity)

	// Notification routes
	r.GET("/notifications", notificationHandler.GetNotifications)
	r.GET("/notifications/grouped", notificationHandler.GetGroupedNotifications)
	r.GET("/notifications/unread/count", notificationHandler.GetUnreadCount)
	r.PATCH("/notifications/:id/read", notificationHandler.MarkAsRead)
	r.DELETE("/notifications/:id", notificationHandler.DismissNotification)

	// Start server
	srv := &http.Server{Addr: cfg.BindAddress, Handler: r}
	logger.Info("Starting webserver", "address", cfg.BindAddress)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("Server failed", "error", err)
			os.Exit(1)
		}
	}()

	// Block until signal
	sig := <-sigChan
	logger.Info("Received shutdown signal", "signal", sig)

	// Stop scheduler
	sched.Stop()

	// Stop the popular content cache refresher
	popularRefresher.Stop()

	// Gracefully drain in-flight requests
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("Server shutdown error", "error", err)
	} else {
		logger.Info("Server shut down cleanly")
	}
}
