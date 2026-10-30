package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jbofill10/scout/backend/internal/tvdbproxy/config"
	"github.com/jbofill10/scout/backend/internal/tvdbproxy/genres"
	"github.com/jbofill10/scout/backend/internal/tvdbproxy/handlers"
	"github.com/jbofill10/scout/backend/internal/tvdbproxy/search"
	"github.com/jbofill10/scout/backend/internal/tvdbproxy/tvdb"
	"github.com/jbofill10/scout/backend/pkg/telemetry"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
)

const (
	// loginTimeout bounds the startup login to TVDB.
	loginTimeout = 30 * time.Second
	// shutdownTimeout bounds how long in-flight requests get to finish on shutdown.
	shutdownTimeout = 10 * time.Second
)

func main() {
	// Initialize OpenTelemetry
	otlpEndpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if otlpEndpoint == "" {
		otlpEndpoint = "otel-collector-service:4317"
	}

	// Initialize tracing
	tracerCleanup, err := telemetry.InitTracer("tvdb-proxy", "1.0.0", otlpEndpoint)
	if err != nil {
		slog.Warn("Failed to initialize tracer", "error", err)
	} else {
		defer tracerCleanup()
		slog.Info("OpenTelemetry tracing initialized")
	}

	// Initialize logging with trace correlation
	logger, loggerCleanup, err := telemetry.InitLogger("tvdb-proxy", "1.0.0", otlpEndpoint)
	if err != nil {
		slog.Warn("Failed to initialize logger", "error", err)
		logger = slog.Default()
	} else {
		defer loggerCleanup()
		logger.Info("OpenTelemetry logging initialized")
	}

	// Load configuration (optional api.toml, then environment)
	cfg, err := config.Load()
	if err != nil {
		logger.Error("Error unmarshalling config", "error", err)
		os.Exit(1)
	}

	// Log in to TVDB before serving; /ready reports unavailable until a token is held
	tvdbClient := tvdb.NewClient(cfg.TVDB, tvdb.NewHTTPClient(), logger)
	loginCtx, loginCancel := context.WithTimeout(context.Background(), loginTimeout)
	err = tvdbClient.Login(loginCtx)
	loginCancel()
	if err != nil {
		logger.Error("Unable to get API Key, exiting")
		os.Exit(1)
	}
	// Required fields are checked after the login attempt, the order the service
	// has always used: a missing host or key normally surfaces as a failed login.
	if err := cfg.Validate(); err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}

	refreshCtx, stopRefresher := context.WithCancel(context.Background())
	defer stopRefresher()
	tvdbClient.StartTokenRefresher(refreshCtx, cfg.RefreshInterval(logger))

	// Initialize genre cache on startup
	genreCache := genres.NewCache(tvdbClient, logger)
	if err := genreCache.Refresh(context.Background()); err != nil {
		logger.Warn("Failed to initialize genre cache on startup", "error", err)
	} else {
		logger.Info("Genre cache initialized", "genres_count", len(genreCache.Genres()))
	}

	searchService := search.NewService(tvdbClient, logger)

	// Initialize handlers
	healthHandler := handlers.NewHealthHandler(tvdbClient)
	searchHandler := handlers.NewSearchHandler(searchService, logger)
	extendedHandler := handlers.NewExtendedHandler(tvdbClient, logger)
	episodesHandler := handlers.NewEpisodesHandler(tvdbClient, logger)
	genreHandler := handlers.NewGenreHandler(genreCache, logger)
	popularHandler := handlers.NewPopularHandler(tvdbClient, genreCache, logger)

	router := gin.Default()
	// Only the compose network sits in front of this service, so only it may
	// set X-Forwarded-For. 172.16.0.0/12 covers Docker's default bridge pools.
	if err := router.SetTrustedProxies([]string{"172.16.0.0/12"}); err != nil {
		logger.Error("Failed to set trusted proxies", "error", err)
	}
	router.Use(otelgin.Middleware("tvdb-proxy"))

	router.GET("/health", healthHandler.Health)
	router.GET("/ready", healthHandler.Ready)

	router.GET("/series", searchHandler.GetSeries)
	router.GET("/series/:id/extended", extendedHandler.GetExtended)
	router.POST("/series/batch/extended", extendedHandler.GetBatchExtended)
	router.POST("/series/batch/episodes", episodesHandler.GetBatchEpisodes)
	router.GET("/genres", genreHandler.GetGenres)
	router.GET("/series/popular", popularHandler.GetPopularSeries)
	router.GET("/movies/popular", popularHandler.GetPopularMovies)

	// Setup graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	srv := &http.Server{Addr: cfg.BindAddress, Handler: router.Handler()}
	logger.Info("Starting tvdb-proxy", "address", cfg.BindAddress)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("Server failed", "error", err)
			os.Exit(1)
		}
	}()

	// Block until signal
	sig := <-sigChan
	logger.Info("Received shutdown signal", "signal", sig)

	// Stop the token refresher
	stopRefresher()

	// Gracefully drain in-flight requests
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("Server shutdown error", "error", err)
	} else {
		logger.Info("Server shut down cleanly")
	}
}
