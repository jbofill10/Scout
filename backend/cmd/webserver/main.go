package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/jbofill10/scout/backend/pkg/telemetry"
	"github.com/jbofill10/scout/backend/internal/webserver/clients"
	"github.com/jbofill10/scout/backend/internal/webserver/config"
	"github.com/jbofill10/scout/backend/internal/webserver/handlers"
	"github.com/jbofill10/scout/backend/internal/webserver/interactors"
	"github.com/jbofill10/scout/backend/internal/webserver/repository"
	"github.com/jbofill10/scout/backend/internal/webserver/scheduler"

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
	queue := make(chan tvdb.Media, 100)

	// Initialize scheduler
	sched := scheduler.NewScheduler(schedulerRepo, logger)

	// Initialize clients
	tvdbClient := clients.NewTVDBProxyClient(cfg.TVDBProxyHost)
	torrenterClient := clients.NewTorrenterClient(cfg.TorrenterHost)

	// Initialize interactors
	searchInteractor := interactors.NewSearchInteractor(tvdbClient)
	downloadInteractor := interactors.NewDownloadInteractor(
		schedulerRepo,
		notificationRepo,
		sched,
		queue,
		logger,
		tvdbClient,
		torrenterClient,
	)

	// Start watching for due media
	downloadInteractor.WatchForDueMedia()

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
	downloadHandler := handlers.NewDownloadHandler(downloadInteractor, logger)
	popularHandler := handlers.NewPopularHandler(tvdbClient, logger)
	scheduleHandler := handlers.NewScheduleHandler(schedulerRepo, logger)
	notificationHandler := handlers.NewNotificationHandler(notificationRepo, logger)
	statusHandler := handlers.NewStatusHandler(torrenterClient, logger)

	// Setup routes
	r := gin.Default()
	r.Use(otelgin.Middleware("webserver"))
	r.Use(handlers.CorsMiddleware())
	r.GET("/search", searchHandler.HandleSearch)
	r.POST("/shows", downloadHandler.DownloadShow)
	r.POST("/movies", downloadHandler.DownloadMovie)
	r.GET("/popular/shows", popularHandler.GetPopularShows)
	r.GET("/popular/movies", popularHandler.GetPopularMovies)
	r.GET("/genres", popularHandler.GetGenres)
	r.POST("/status/batch", statusHandler.GetBatchStatus)
	r.GET("/schedule/weekly", scheduleHandler.GetWeeklySchedule)

	// Notification routes
	r.GET("/notifications", notificationHandler.GetNotifications)
	r.GET("/notifications/grouped", notificationHandler.GetGroupedNotifications)
	r.GET("/notifications/unread/count", notificationHandler.GetUnreadCount)
	r.PATCH("/notifications/:id/read", notificationHandler.MarkAsRead)
	r.DELETE("/notifications/:id", notificationHandler.DismissNotification)

	// Start server
	logger.Info("Starting webserver", "address", cfg.BindAddress)
	if err := r.Run(cfg.BindAddress); err != nil {
		logger.Error("Server failed", "error", err)
		os.Exit(1)
	}
}
