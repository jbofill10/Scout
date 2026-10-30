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
	"github.com/jbofill10/scout/backend/internal/torrenter/searchgate"
	"github.com/jbofill10/scout/backend/internal/torrenter/service"
	torrsettings "github.com/jbofill10/scout/backend/internal/torrenter/settings"
	"github.com/jbofill10/scout/backend/pkg/dbmigrate"
	"github.com/jbofill10/scout/backend/pkg/telemetry"
	"golift.io/starr"
	"golift.io/starr/prowlarr"

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

	if err := dbmigrate.RunWithConnStr(context.Background(), connStr, logger); err != nil {
		logger.Error("Failed to run database migrations", "error", err)
		os.Exit(1)
	}

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

	// Subtitle fetching is opt-in: it needs an OpenSubtitles API key.
	var subs service.SubtitleFetcher
	if cfg.Subtitles.APIKey != "" {
		provider := clients.NewOpenSubtitlesClient(&cfg.Subtitles, logger)
		refresher := service.NewPlexRefresher(&cfg.Plex, logger)
		subs = service.NewSubtitleSvc(&cfg.Subtitles, provider, fs, refresher, logger)
		logger.Info("Subtitle fetching enabled", "languages", cfg.Subtitles.Languages,
			"authenticated", cfg.Subtitles.Username != "")
	} else {
		logger.Info("Subtitle fetching disabled: OPENSUBTITLES_API_KEY not set")
	}

	// Runtime settings: the environment supplies defaults, the Settings page
	// overrides them, and the services read the effective value as they go.
	settingsResolver := torrsettings.NewResolver(torrsettings.NewStore(repo.DB()), torrsettings.Defaults{
		MinQuality:           cfg.Downloads.MinQuality,
		MinFreeSpaceGB:       cfg.Downloads.MinFreeSpaceGB,
		SubtitleLanguages:    cfg.Subtitles.Languages,
		SubtitlePreferSDH:    cfg.Subtitles.PreferSDH,
		PlexSyncDelay:        cfg.Plex.SyncDelay,
		PlexSyncInterval:     cfg.Plex.SyncInterval,
		SubtitlesEnabled:     cfg.Subtitles.APIKey != "",
		AutoRotateRelay:      true,
		RelayAgentConfigured: cfg.Relay.AgentURL != "",
		UpgradesEnabled:      cfg.Upgrades.Enabled,
		UpgradeCutoff:        cfg.Upgrades.Cutoff,
		UpgradeBatch:         cfg.Upgrades.Batch,
		SearchRatePerMinute:  cfg.Downloads.SearchRatePerMinute,
		SearchBurst:          cfg.Downloads.SearchBurst,
		BulkRequestBatch:     cfg.Downloads.BulkRequestBatch,
	}, logger)
	settingsResolver.FreeSpace = service.DownloadsFreeSpace
	qbitt.SetSettings(settingsResolver)

	// Every indexer search is paced (per-indexer rate, one at a time) and
	// ordered (people before automation) by one gate; limits follow the
	// Settings page without a restart.
	gate := searchgate.New(func(id int64) searchgate.Limits {
		perMinute, burst := settingsResolver.SearchLimits(id)
		return searchgate.Limits{PerMinute: float64(perMinute), Burst: burst}
	})
	qbitt.SetGate(gate)
	logger.Info("Search gate enabled", "rate_per_minute", cfg.Downloads.SearchRatePerMinute, "burst", cfg.Downloads.SearchBurst)
	if subSvc, ok := subs.(*service.SubtitleSvc); ok && subSvc != nil {
		subSvc.SetSettings(settingsResolver)
	}

	mp := service.NewMediaProcessSvc(logger, repo, fs, subs)
	plex := service.NewPlexHandler(repo, logger, &cfg.Plex, mp)
	// Keeps the database mirror of Plex current: at start, after downloads
	// land, and on a timer.
	plexSync := service.NewPlexSyncScheduler(plex, cfg.Plex.SyncDelay, cfg.Plex.SyncInterval, logger)
	plexSync.SetSettings(settingsResolver)

	// Indexer health: watches Prowlarr for disabled indexers and, with the
	// host relay agent, rotates the Mullvad relay when the exit IP is blocked.
	relayAgent := clients.NewRelayAgentClient(cfg.Relay.AgentURL, cfg.Relay.AgentToken)
	healthMonitor := service.NewIndexerHealthMonitor(
		prowlarr.New(starr.New(cfg.Prowlarr.Key, cfg.Prowlarr.Host, 60*time.Hour)).APIer,
		relayAgent, settingsResolver, cfg.Relay.RotateCooldown, cfg.Relay.HealthInterval, logger)
	healthMonitor.SetPauser(gate)
	healthMonitor.Start(context.Background())
	logger.Info("Indexer health monitor started", "relay_agent", relayAgent.Configured(),
		"rotate_cooldown", cfg.Relay.RotateCooldown.String(), "interval", cfg.Relay.HealthInterval.String())

	// Initialize interactors
	downloadInteractor := interactors.NewDownloadInteractor(qbitt, mp, repo, plexSync, logger)
	mediaInteractor := interactors.NewMediaInteractor(repo)
	// Upgrade pass: replaces library copies below the cutoff when enabled.
	upgradeInteractor := interactors.NewUpgradeInteractor(qbitt, repo, settingsResolver, downloadInteractor,
		cfg.Upgrades.CheckInterval, logger)
	upgradeInteractor.Start(context.Background())
	statusInteractor := interactors.NewStatusInteractor(repo, logger)
	libraryInteractor := interactors.NewLibraryInteractor(repo, logger)

	// Initialize handlers
	downloadHandler := handlers.NewDownloadHandler(downloadInteractor, logger)
	controlHandler := handlers.NewControlHandler(downloadInteractor, logger)
	settingsHandler := handlers.NewSettingsHandler(settingsResolver, repo, logger)
	indexerHandler := handlers.NewIndexerHandler(healthMonitor, gate, logger)
	upgradeHandler := handlers.NewUpgradeHandler(upgradeInteractor, logger)
	mediaHandler := handlers.NewMediaHandler(mediaInteractor, logger)
	statusHandler := handlers.NewStatusHandler(statusInteractor, logger)
	libraryHandler := handlers.NewLibraryHandler(libraryInteractor, logger)
	plexProxyHandler := handlers.NewPlexProxyHandler(&cfg.Plex, logger)

	// Setup routes
	r := gin.Default()
	// Only the compose network sits in front of this service, so only it may
	// set X-Forwarded-For. 172.16.0.0/12 covers Docker's default bridge pools.
	if err := r.SetTrustedProxies([]string{"172.16.0.0/12"}); err != nil {
		logger.Error("Failed to set trusted proxies", "error", err)
	}
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
	// Download controls: stop an item, see what the indexers have for it, grab
	// one release by hand.
	r.POST("/downloads/cancel", controlHandler.Cancel)
	r.POST("/releases", controlHandler.ListReleases)
	r.POST("/downloads/manual", controlHandler.ManualDownload)

	// Runtime settings and the picker's preferred uploaders
	r.GET("/settings", settingsHandler.Get)
	r.PUT("/settings", settingsHandler.Update)
	r.GET("/settings/uploaders", settingsHandler.ListUploaders)
	r.POST("/settings/uploaders", settingsHandler.AddUploader)
	r.DELETE("/settings/uploaders/:id", settingsHandler.DeleteUploader)

	// Indexer health and relay rotation
	r.GET("/indexers/health", indexerHandler.Health)
	r.POST("/indexers/rotate", indexerHandler.Rotate)
	r.POST("/indexers/:id/test", indexerHandler.Test)

	// Quality upgrades
	r.GET("/upgrades/candidates", upgradeHandler.Candidates)
	r.GET("/upgrades/attempts", upgradeHandler.Attempts)
	r.POST("/upgrades/run", upgradeHandler.Run)
	r.GET("/media/:hash", mediaHandler.MediaExists)
	r.POST("/media/exists", mediaHandler.MediaExistsBatch)
	r.POST("/status/batch", statusHandler.BatchStatus)

	// Library routes
	r.GET("/library/shows", libraryHandler.GetShows)
	r.GET("/library/missing", libraryHandler.GetMissingEpisodes)
	r.GET("/library/shows/:tvdbId/episodes", libraryHandler.GetShowEpisodes)
	r.GET("/library/shows/:tvdbId/metadata-status", libraryHandler.GetShowMetadataStatus)
	r.GET("/library/movies", libraryHandler.GetMovies)
	r.POST("/sync-episodes", libraryHandler.SyncEpisodes)

	// Plex proxy routes
	r.GET("/plex/thumb", plexProxyHandler.ProxyThumb)

	// Mirror the Plex library into the database (non-blocking): once now, then
	// after each download lands and periodically.
	logger.Info("Starting Plex library sync scheduler",
		"sync_delay", cfg.Plex.SyncDelay.String(), "sync_interval", cfg.Plex.SyncInterval.String())
	go plexSync.Run(context.Background())

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
