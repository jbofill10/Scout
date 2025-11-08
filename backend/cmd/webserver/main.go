package main

import (
	"log/slog"
	"os"

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

	// Initialize repository
	repo, err := repository.NewSchedulerRepo(logger, cfg.Database.ConnStr)
	if err != nil {
		logger.Error("Failed to init repo", "error", err)
		os.Exit(1)
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
	popularHandler := handlers.NewPopularHandler(tvdbClient, logger)
	scheduleHandler := handlers.NewScheduleHandler(repo, logger)

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
	r.GET("/schedule/weekly", scheduleHandler.GetWeeklySchedule)

	// Start server
	logger.Info("Starting webserver", "address", cfg.BindAddress)
	if err := r.Run(cfg.BindAddress); err != nil {
		logger.Error("Server failed", "error", err)
		os.Exit(1)
	}
}
