package service

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	tvdb "shared/media"
	"shared/telemetry"
	"torrenter/internal/models"

	"github.com/superturkey650/go-qbittorrent/qbt"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"golift.io/starr"
	"golift.io/starr/prowlarr"
)

var (
	baseSavePath  = "/data/Downloads"
	scoutTag      = "scout"
	torrentTracer = otel.Tracer("torrenter/torrent")
)

type QbittHandler struct {
	c      *qbt.Client
	p      *prowlarr.Prowlarr
	repo   Repository
	logger *slog.Logger
}

// Indexer IDs
var (
	NYAA_ID    = int64(1) // Anime
	ONE337x_ID = int64(5) // General
)

func NewQbittHandler(qCfg *models.QbittCfg, pCfg *models.ProwlarrCfg, repo Repository, logger *slog.Logger) (*QbittHandler, error) {
	logger.Debug("QBittorrent config", "host", qCfg.Host, "user", qCfg.User)
	qb := qbt.NewClient(qCfg.Host)
	if err := qb.Login(qCfg.User, qCfg.Password); err != nil {
		return nil, fmt.Errorf("failed to login to qBittorrent: %w", err)
	}

	logger.Info("Connected to QBittorrent successfully")

	p := prowlarr.New(starr.New(pCfg.Key, pCfg.Host, 60*time.Hour))
	return &QbittHandler{
		c:      qb,
		logger: logger,
		p:      p,
		repo:   repo,
	}, nil
}

func (q *QbittHandler) searchProwlarr(query, category string, isAnime bool) ([]*prowlarr.Search, error) {
	q.logger.Info("Searching Prowlarr", "query", query, "category", category)

	resp, err := q.p.SearchContext(context.Background(), prowlarr.SearchInput{
		Query:      query,
		IndexerIDs: q.calcIndexerIDs(category, isAnime),
	})

	if err != nil {
		q.logger.Error("Error searching Prowlarr", "error", err)
		return nil, err
	}

	return resp, nil
}

func (q *QbittHandler) HandleDownload(ctx context.Context, req *tvdb.Media, done chan<- models.TorrentCompleteEvent) error {

	searchStrategies := [][]*models.SearchStrategy{}
	if req.Category == "series" {
		// Pre-download filtering: Check if episodes already exist in Plex
		ctx, span := torrentTracer.Start(ctx, "filterExistingEpisodes")
		span.SetAttributes(
			attribute.String("show_name", req.Name),
			attribute.String("tvdb_id", req.Id),
			attribute.Int("total_episodes", len(req.Metadata.Episodes)),
		)

		episodesToDownload := []tvdb.Episode{}
		for _, episode := range req.Metadata.Episodes {
			// First try matching by TVDB ID (most reliable)
			exists, err := q.repo.EpisodeExistsByTvdbId(ctx, req.Id, episode.SeasonNumber, episode.Number)
			if err != nil {
				q.logger.WarnContext(ctx, "Failed to check episode existence by TVDB ID, falling back to title match",
					telemetry.WithTraceContext(ctx, "tvdb_id", req.Id, "season", episode.SeasonNumber, "episode", episode.Number, "error", err.Error())...)
				// Fallback to name-based matching if TVDB ID check fails
			}

			if exists {
				q.logger.InfoContext(ctx, "Episode already exists in Plex, skipping",
					telemetry.WithTraceContext(ctx, "name", req.Name, "season", episode.SeasonNumber, "episode", episode.Number)...)
				continue
			}

			episodesToDownload = append(episodesToDownload, episode)
		}

		episodesFiltered := len(req.Metadata.Episodes) - len(episodesToDownload)

		span.SetAttributes(
			attribute.Int("episodes_filtered", episodesFiltered),
			attribute.Int("episodes_to_download", len(episodesToDownload)),
		)
		span.SetStatus(codes.Ok, "Episode filtering complete")
		span.End()

		if len(episodesToDownload) == 0 {
			q.logger.InfoContext(ctx, "All episodes already exist in Plex, nothing to download", telemetry.WithTraceContext(ctx, "show", req.Name)...)
			return nil
		}

		q.logger.InfoContext(ctx, "Episodes to download after filtering", telemetry.WithTraceContext(ctx, "show", req.Name,
			"total", len(req.Metadata.Episodes), "to_download", len(episodesToDownload), "show_aliases", req.Aliases)...)

		for _, episode := range episodesToDownload {
			q.logger.InfoContext(ctx, "Processing episode", telemetry.WithTraceContext(ctx, "name", req.Name, "season", episode.SeasonNumber,
				"episode", episode.Number)...)

			searchStrategies = append(searchStrategies, q.createSearchStrategy(req, &episode))
		}

		// Use WaitGroup to track all monitoring goroutines
		var wg sync.WaitGroup

		for _, strategies := range searchStrategies {
			bestMatches := []*models.TorrentMatch{}
			for _, ss := range strategies {
				q.logger.InfoContext(ctx, "Search Query", telemetry.WithTraceContext(ctx, "query", ss.Query)...)
				pResp, err := q.searchProwlarr(ss.Query, req.Category, req.Anime)
				if err != nil {
					return fmt.Errorf("failed to search Prowlarr: %w", err)
				}

				if len(pResp) == 0 {
					q.logger.InfoContext(ctx, "no results found", "query", ss.Query)
					// TODO: Store in DB
					continue
				}

				possibleTorrents := []*models.TorrentMatch{}
				for _, torrent := range pResp {
					if q.isCorrectTorrent(torrent, ss) {
						possibleTorrents = append(possibleTorrents, &models.TorrentMatch{Strategy: ss, Torrent: torrent})
					}
				}

				if len(possibleTorrents) == 0 {
					q.logger.InfoContext(ctx, "no matching torrents found", "query", ss.Query)
					err := q.repo.InsertDownloadHistory(ctx, req.Name, ss.Season, ss.Episode, ss.EpisodeMeta.AbsoluteNumber, "", "failure", "no matching torrents found")
					if err != nil {
						q.logger.ErrorContext(ctx, "Failed to insert download history", "error", err)
					}
				} else {
					bestMatches = append(bestMatches, possibleTorrents...)
				}

			}

			q.sortTorrentsByQuality(bestMatches)
			match := q.pickBestTorrent(ctx, bestMatches, req.Category, req.Anime)
			if match == nil {
				q.logger.WarnContext(ctx, "No suitable torrent found", "show", req.Name)
				ss := strategies[0]
				err := q.repo.InsertDownloadHistory(ctx, req.Name, ss.Season, ss.Episode, ss.EpisodeMeta.AbsoluteNumber, "", "failure", "no suitable torrent found")
				if err != nil {
					q.logger.ErrorContext(ctx, "Failed to insert download history", "error", err)
				}
				continue
			}

			ss := match.Strategy
			err := q.repo.InsertDownloadHistory(ctx, req.Name, ss.Season, ss.Episode, ss.EpisodeMeta.AbsoluteNumber, match.Torrent.InfoHash, "downloading", "")
			if err != nil {
				q.logger.ErrorContext(ctx, "Failed to insert download history", "error", err)
			}

			q.downloadTorrent(match.Torrent)
			// watch torrent to complete
			wg.Add(1)
			go func(ctx context.Context) {
				defer wg.Done()
				ranRecheck := false
				retries := 1
				for {
					time.Sleep(10 * time.Second)
					filter := qbt.TorrentsOptions{
						Category: &scoutTag,
						Hashes:   []string{match.Torrent.InfoHash},
					}

					torrents, err := q.c.Torrents(filter)
					if err != nil {
						q.logger.ErrorContext(ctx, "Error fetching torrents", "error", err)
						retries++
						time.Sleep(6 * time.Second)
						if retries > 6 {
							q.logger.WarnContext(ctx, "Max retries reached, stopping watch", "torrent", match.Torrent.Title)
							return
						}
					}
					if len(torrents) == 0 {
						q.logger.InfoContext(ctx, "Torrent not found, continuing watch", "torrent", match.Torrent.Title)
						continue
					}
					torrent := torrents[0]
					if q.didTorrentComplete(&torrent) {

						if !ranRecheck {
							ranRecheck = true
							q.c.Recheck([]string{torrent.Hash})
							continue
						}

						q.logger.InfoContext(ctx, "Torrent completed", "title", match.Torrent.Title)
						done <- models.TorrentCompleteEvent{
							SavePath: torrent.SavePath,
							Req:      match.Strategy,
							Hash:     match.Torrent.InfoHash,
						}
						break
					} else {
						q.logger.InfoContext(ctx, "Torrent downloading", "title", match.Torrent.Title, "progress", torrent.Progress*100)
					}
				}
			}(ctx)

		}

		// Close the channel after all monitoring goroutines complete
		go func() {
			wg.Wait()
			close(done)
			q.logger.InfoContext(ctx, "All torrents processed, channel closed")
		}()
	}
	return nil
}

func (q *QbittHandler) createSearchStrategy(req *tvdb.Media, episode *tvdb.Episode) []*models.SearchStrategy {
	var ss []*models.SearchStrategy

	// Build media names from req.Name + all aliases, with deduplication
	mediaNames := []string{req.Name}
	mediaNames = append(mediaNames, req.Aliases...)

	// Deduplicate
	seen := make(map[string]bool)
	uniqueNames := []string{}
	for _, name := range mediaNames {
		if !seen[name] && name != "" {
			seen[name] = true
			uniqueNames = append(uniqueNames, name)
		}
	}
	showNames := uniqueNames

	if req.Anime {
		for _, showName := range showNames {
			ss = append(ss, &models.SearchStrategy{
				Query:       fmt.Sprint(showName, " ", standardizeNumber(episode.AbsoluteNumber)),
				MediaName:   showName,
				Season:      episode.SeasonNumber,
				Episode:     episode.AbsoluteNumber,
				EpisodeMeta: episode,
				Exclude:     []string{"season", "episode"},
				TvdbId:      req.Id,
			})
		}
	}

	queryFormats := []string{
		"%s S%sE%s",
		"%s Season %s Episode %s",
	}

	for _, format := range queryFormats {
		for _, showName := range showNames {

			ss = append(ss, &models.SearchStrategy{
				Query:       fmt.Sprintf(format, showName, standardizeNumber(episode.SeasonNumber), standardizeNumber(episode.Number)),
				MediaName:   showName,
				Season:      episode.SeasonNumber,
				Episode:     episode.Number,
				EpisodeMeta: episode,
				TvdbId:      req.Id,
			})
		}
	}

	return ss
}

func (q *QbittHandler) didTorrentComplete(torrent *qbt.TorrentInfo) bool {
	fmt.Printf("Torrent State: %s\n", torrent.State)
	if torrent.Completed == torrent.Size && torrent.State == "stalledUP" {
		return true
	}
	return false
}

func (q *QbittHandler) calcIndexerIDs(_ string, isAnime bool) []int64 {
	if isAnime {
		return []int64{NYAA_ID}
	} else {
		return []int64{NYAA_ID, ONE337x_ID}
	}

}

func (q *QbittHandler) isCorrectTorrent(torrent *prowlarr.Search, strategy *models.SearchStrategy) bool {
	if strings.Contains(strings.ToLower(torrent.SortTitle), "batch") || strings.Contains(strings.ToLower(torrent.SortTitle), "cour") {
		return false
	}

	for _, exclude := range strategy.Exclude {
		if strings.Contains(strings.ToLower(torrent.SortTitle), exclude) {
			return false
		}
	}

	foundEpisode := false
	paddedEpisode := standardizeNumber(strategy.Episode)
	unpaddedEpisode := fmt.Sprintf("%d", strategy.Episode)
	titleFields := strings.Fields(torrent.SortTitle)

	if slices.Contains(titleFields, paddedEpisode) || slices.Contains(titleFields, unpaddedEpisode) {
		foundEpisode = true
	}

	torTitleLowerCase := strings.ToLower(torrent.Title)
	mediaNameLower := strings.ToLower(strategy.MediaName)

	// q.logger.Printf("\nTorrent: %s\nMedia: %s\nFound Episode: %v\nIs match: %v\nBreakdown:\n\ttorTitleLowerCase: %s,\n\tmediaNameLower: %s", torTitleLowerCase, mediaNameLower, foundEpisode, foundEpisode &&
	// 	(strings.Contains(torTitleLowerCase, mediaNameLower)), torTitleLowerCase, mediaNameLower)

	return foundEpisode && (strings.Contains(torTitleLowerCase, mediaNameLower))
}

func (q *QbittHandler) downloadTorrent(torrent *prowlarr.Search) error {
	q.logger.Info("Downloading torrent", "filename", torrent.FileName, "hash", torrent.InfoHash, "guid", torrent.GUID)

	torrentSavePath := baseSavePath + "/" + torrent.Title
	q.logger.Info("Torrent save path", "path", torrentSavePath)

	if err := os.Mkdir(torrentSavePath, 0777); err != nil && !os.IsExist(err) {
		q.logger.Error("Error creating directory", "value", torrentSavePath, "error", err)
		return err
	}
	dlOpts := qbt.DownloadOptions{
		Savepath: &torrentSavePath,
		Category: &scoutTag,
	}

	if err := q.c.DownloadLinks([]string{torrent.GUID}, dlOpts); err != nil {
		q.logger.Error("Error downloading torrent", "error", err)
		return err
	}

	return nil
}

func (h *QbittHandler) sortTorrentsByQuality(matches []*models.TorrentMatch) {
	sort.SliceStable(matches, func(i, j int) bool {
		qualityOrder := func(title string) int {
			titleLower := strings.ToLower(title)
			if strings.Contains(titleLower, "4k") {
				return 0
			}
			if strings.Contains(titleLower, "2160p") {
				return 1
			}
			if strings.Contains(titleLower, "1080p") {
				return 2
			}
			if strings.Contains(titleLower, "720p") {
				return 3
			}
			if strings.Contains(titleLower, "480p") {
				return 4
			}
			return 5
		}
		return qualityOrder(matches[i].Torrent.SortTitle) < qualityOrder(matches[j].Torrent.SortTitle)
	})
}

func (q *QbittHandler) pickBestTorrent(ctx context.Context, matches []*models.TorrentMatch, mediaType string, isAnime bool) *models.TorrentMatch {
	// TODO: Discard torrents with low seeds, etc.
	preferred, err := q.repo.GetPreferredUploaders(ctx, mediaType, isAnime)
	if err != nil {
		q.logger.ErrorContext(ctx, "Error querying uploader preferences", "error", err)
		return matches[0]
	}

	preferred = append(preferred, "SubsPlease")

	for _, match := range matches {
		for _, uploader := range preferred {
			if strings.Contains(strings.ToLower(match.Torrent.SortTitle), strings.ToLower(uploader)) {
				return match
			}
		}
	}
	return matches[0]
}
