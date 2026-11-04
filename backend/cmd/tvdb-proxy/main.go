package main

import (
	"context"
	"database/sql"
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
	"github.com/jbofill10/scout/backend/internal/tvdbproxy/popular"
	"github.com/jbofill10/scout/backend/internal/tvdbproxy/ratings"
	"github.com/jbofill10/scout/backend/internal/tvdbproxy/search"
	"github.com/jbofill10/scout/backend/internal/tvdbproxy/tvdb"
	"github.com/jbofill10/scout/backend/pkg/dbmigrate"
	"github.com/jbofill10/scout/backend/pkg/telemetry"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
)

const (
	// loginTimeout bounds the startup login to TVDB.
	loginTimeout = 30 * time.Second
	// shutdownTimeout bounds how long in-flight requests get to finish on shutdown.
	shutdownTimeout = 10 * time.Second
	// datasetLoadTimeout bounds the startup download of IMDb's ratings table;
	// past it the proxy serves without it and keeps trying in the background.
	datasetLoadTimeout = 90 * time.Second
	// storeOpenTimeout bounds the startup migration and first query of the
	// ratings store.
	storeOpenTimeout = 30 * time.Second
	// storeKeepExpired is how long an expired rating answer is kept as a
	// stand-in before the startup prune drops it.
	storeKeepExpired = 90 * 24 * time.Hour
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

	// Ratings ride along on every search hit, popular row and extended
	// record: the IMDb rating from IMDb's public table (loaded before serving
	// so the webserver's first rows carry it), the Tomatometer and Metascore
	// Wikidata records (no key), and a current Tomatometer and Metascore
	// from OMDb when a key is configured. Every answer is kept in the shared
	// PostgreSQL when DB_HOST is set, so the library's scores build up
	// across restarts instead of being looked up again.
	resolver := ratings.NewResolver(tvdbClient, logger)
	ratingOpts := ratings.Options{Resolver: resolver}
	if connStr := cfg.Database.ConnStr(); connStr != "" {
		if store := openRatingsStore(refreshCtx, connStr, logger); store != nil {
			defer store.Close()
			ratingOpts.Store = store
			resolver.SetStore(store)
		}
	} else {
		logger.Info("DB_HOST not set; ratings cached in memory only")
	}
	if cfg.IMDb.Enabled {
		dataset := ratings.NewDataset(cfg.IMDb.URL, ratings.NewDatasetHTTPClient(), logger)
		loadCtx, loadCancel := context.WithTimeout(refreshCtx, datasetLoadTimeout)
		if err := dataset.Load(loadCtx); err != nil {
			logger.Warn("IMDb ratings table not loaded, retrying in background", "error", err)
		}
		loadCancel()
		dataset.Start(refreshCtx)
		ratingOpts.IMDb = dataset
	} else {
		logger.Info("RATINGS_IMDB=false; no IMDb ratings table")
	}
	if cfg.Wikidata.Enabled {
		wikidata := ratings.NewWikidata(cfg.Wikidata.URL, cfg.Wikidata.APIURL, ratings.NewWikidataHTTPClient(), logger)
		ratingOpts.Wikidata = wikidata
		ratingOpts.Finder = wikidata
		ratingOpts.Prose = ratings.NewProse(cfg.Wikidata.WikipediaURL, ratings.NewProseHTTPClient(), logger)
		logger.Info("Tomatometer and Metascore read from Wikidata and Wikipedia")
	} else {
		logger.Info("RATINGS_WIKIDATA=false; no Wikidata scores")
	}
	if cfg.OMDb.APIKey != "" {
		ratingOpts.Source = ratings.NewOMDb(cfg.OMDb.Host, cfg.OMDb.APIKey, ratings.NewOMDbHTTPClient(), logger)
		logger.Info("Current Tomatometer and Metascore looked up on OMDb")
	} else {
		logger.Info("OMDB_API_KEY not set; Tomatometer comes from Wikidata and Wikipedia")
	}
	var rater handlers.Rater
	if ratingOpts.IMDb != nil || ratingOpts.Wikidata != nil || ratingOpts.Source != nil {
		rater = ratings.NewService(ratingOpts, logger)
	}

	// Popular rows: Trakt's most-watched list when a client id is configured,
	// a recency-weighted TVDB ranking otherwise (and whenever Trakt fails).
	tvdbPopular := popular.NewTVDBSource(tvdbClient, genreCache, logger)
	if cfg.Wikipedia.Enabled {
		tvdbPopular.SetAttention(popular.NewWikipedia(popular.WikipediaOptions{
			APIHost:  cfg.Wikipedia.APIHost,
			RESTHost: cfg.Wikipedia.RESTHost,
		}, popular.NewWikipediaHTTPClient(), logger))
		logger.Info("TVDB popular ranking ordered by Wikipedia attention")
	}
	if cfg.AniList.Enabled {
		tvdbPopular.SetAnimeTrends(popular.NewAniList(cfg.AniList.Host, popular.NewAniListHTTPClient(), logger))
		logger.Info("Anime rows led by AniList trending")
	}
	var primaryPopular popular.Source
	if cfg.Trakt.ClientID != "" {
		trakt := popular.NewTraktSource(popular.TraktOptions{
			Host:     cfg.Trakt.Host,
			ClientID: cfg.Trakt.ClientID,
			List:     cfg.Trakt.List,
			Period:   cfg.Trakt.Period,
		}, popular.NewTraktHTTPClient(), tvdbClient, logger)
		primaryPopular = trakt
		logger.Info("Popular rows ranked by Trakt", "source", trakt.Name())
	} else {
		logger.Info("TRAKT_CLIENT_ID not set; popular rows ranked by recency-weighted TVDB score")
	}
	popularService := popular.NewService(primaryPopular, tvdbPopular, logger)
	if rater != nil {
		// Rows carry their scores from the moment they are cached, so the
		// webserver's refresh never caches an unrated row.
		popularService.SetRater(rater)
	}
	// Build the UI's rows now so the webserver's first refresh finds them ready.
	popularService.Warm(refreshCtx, popular.DefaultRows())

	// Initialize handlers
	healthHandler := handlers.NewHealthHandler(tvdbClient)
	searchHandler := handlers.NewSearchHandler(searchService, rater, logger)
	extendedHandler := handlers.NewExtendedHandler(tvdbClient, rater, logger)
	episodesHandler := handlers.NewEpisodesHandler(tvdbClient, logger)
	genreHandler := handlers.NewGenreHandler(genreCache, logger)
	popularHandler := handlers.NewPopularHandler(popularService, genreCache, rater, logger)
	ratingsHandler := handlers.NewRatingsHandler(rater, logger)

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
	router.POST("/ratings/batch", ratingsHandler.GetBatch)

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

// openRatingsStore brings the schema up to date and opens the ratings
// store, pruning answers that expired long ago. It returns nil, with a
// warning, when the database is unreachable: the proxy serves without it.
func openRatingsStore(ctx context.Context, connStr string, logger *slog.Logger) *ratings.PostgresStore {
	ctx, cancel := context.WithTimeout(ctx, storeOpenTimeout)
	defer cancel()
	if err := dbmigrate.RunWithConnStr(ctx, connStr, logger); err != nil {
		logger.Warn("Database migrations failed; ratings cached in memory only", "error", err)
		return nil
	}
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		logger.Warn("Ratings store not opened; ratings cached in memory only", "error", err)
		return nil
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(30 * time.Minute)
	if err := db.PingContext(ctx); err != nil {
		logger.Warn("Ratings store unreachable; ratings cached in memory only", "error", err)
		_ = db.Close()
		return nil
	}
	store := ratings.NewPostgresStore(db)
	pruned, err := store.Prune(ctx, time.Now().Add(-storeKeepExpired))
	if err != nil {
		logger.Warn("Ratings store prune failed", "error", err)
	}
	count, err := store.Count(ctx)
	if err != nil {
		logger.Warn("Ratings store count failed", "error", err)
	}
	logger.Info("Ratings store open", "entries", count, "pruned", pruned)
	return store
}
