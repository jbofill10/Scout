package service

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"sort"
	"strings"
	"time"

	tvdb "shared/media"
	"shared/telemetry"
	"torrenter/internal/models"

	"github.com/superturkey650/go-qbittorrent/qbt"
	"golift.io/starr"
	"golift.io/starr/prowlarr"
)

var (
	baseSavePath = "/data/Downloads"
	scoutTag     = "scout"
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
		for _, episode := range req.Metadata.Episodes {
			q.logger.InfoContext(ctx, "Processing episode", telemetry.WithTraceContext(ctx, "name", req.Name, "season", episode.SeasonNumber, "episode", episode.Number)...)

			searchStrategies = append(searchStrategies, q.createSearchStrategy(req, &episode))
		}

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
					traceID, spanID := telemetry.GetTraceSpanIDs(ctx)
					err := q.repo.InsertDownloadHistory(req.Name, ss.Season, ss.Episode, ss.EpisodeMeta.AbsoluteNumber, "", "failure", "no matching torrents found", traceID, spanID)
					if err != nil {
						q.logger.ErrorContext(ctx, "Failed to insert download history", "error", err)
					}
				} else {
					bestMatches = append(bestMatches, possibleTorrents...)
				}

			}
			q.sortTorrentsByQuality(bestMatches)
			match := q.pickBestTorrent(bestMatches, req.Category, req.Anime)
			if match == nil {
				q.logger.WarnContext(ctx, "No suitable torrent found", "show", req.Name)
				ss := strategies[0]
				traceID, spanID := telemetry.GetTraceSpanIDs(ctx)
				err := q.repo.InsertDownloadHistory(req.Name, ss.Season, ss.Episode, ss.EpisodeMeta.AbsoluteNumber, "", "failure", "no suitable torrent found", traceID, spanID)
				if err != nil {
					q.logger.ErrorContext(ctx, "Failed to insert download history", "error", err)
				}
				continue
			}
			ss := match.Strategy
			traceID, spanID := telemetry.GetTraceSpanIDs(ctx)
			err := q.repo.InsertDownloadHistory(req.Name, ss.Season, ss.Episode, ss.EpisodeMeta.AbsoluteNumber, match.Torrent.InfoHash, "downloading", "", traceID, spanID)
			if err != nil {
				q.logger.ErrorContext(ctx, "Failed to insert download history", "error", err)
			}
			q.downloadTorrent(match.Torrent)

			// watch torrent to complete
			go func(ctx context.Context) {
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
	}
	return nil
}

func (q *QbittHandler) createSearchStrategy(req *tvdb.Media, episode *tvdb.Episode) []*models.SearchStrategy {
	var ss []*models.SearchStrategy

	showNames := []string{req.Name, strings.ReplaceAll(req.Name, " ", "")}

	if req.Anime {
		for _, showName := range showNames {
			ss = append(ss, &models.SearchStrategy{
				Query:       fmt.Sprint(showName, " ", standardizeNumber(episode.AbsoluteNumber)),
				MediaName:   showName,
				Season:      episode.SeasonNumber,
				Episode:     episode.AbsoluteNumber,
				EpisodeMeta: episode,
				Exclude:     []string{"season", "episode"},
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
	if strings.Contains(strings.ToLower(torrent.SortTitle), "batch") {
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
	q.logger.Info("Downloading torrent", "filename", torrent.FileName)

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

func (q *QbittHandler) pickBestTorrent(matches []*models.TorrentMatch, mediaType string, isAnime bool) *models.TorrentMatch {
	// TODO: Discard torrents with low seeds, etc.
	preferred, err := q.repo.GetPreferredUploaders(mediaType, isAnime)
	if err != nil {
		q.logger.Error("Error querying uploader preferences", "error", err)
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
