package service

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jbofill10/scout/backend/internal/torrenter/models"
	"github.com/jbofill10/scout/backend/pkg/dlstatus"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/jbofill10/scout/backend/pkg/notifications"
	"github.com/jbofill10/scout/backend/pkg/telemetry"

	qbittorrent "github.com/autobrr/go-qbittorrent"
	"go.opentelemetry.io/otel/trace"
	"golift.io/starr/prowlarr"
)

// minShowNameScore is the word-level name match below which a title is
// another show; the same threshold the search validators apply.
const minShowNameScore = 0.3

// coverFromFinished looks through the finished torrents Scout already holds in
// qBittorrent for ones that carry the wanted episodes - a pack whose title
// covers their seasons and whose downloaded files match them, or a single
// episode release - and links those episodes from disk through the normal
// completion path instead of searching for them. It returns the per-episode
// results (downloading) and which episode ids were covered.
func (q *QbittHandler) coverFromFinished(ctx context.Context, req *tvdb.Media, episodes []tvdb.Episode,
	done chan<- models.TorrentCompleteEvent, wg *sync.WaitGroup) ([]dlstatus.EpisodeResult, map[int]bool) {
	if q.c == nil || len(episodes) == 0 {
		return nil, nil
	}
	torrents, err := q.c.GetTorrentsCtx(ctx, qbittorrent.TorrentFilterOptions{Category: scoutTag})
	if err != nil {
		q.logger.WarnContext(ctx, "Could not list finished torrents, searching as usual",
			telemetry.WithTraceContext(ctx, "show", req.Name, "error", err.Error())...)
		return nil, nil
	}
	inFlight := map[string]bool{}
	if active, aerr := q.repo.GetActiveTorrents(ctx); aerr == nil {
		for _, at := range active {
			inFlight[strings.ToLower(at.InfoHash)] = true
		}
	}

	var results []dlstatus.EpisodeResult
	covered := map[int]bool{}
	for i := range torrents {
		t := torrents[i]
		if inFlight[strings.ToLower(t.Hash)] {
			continue
		}
		// Most finished torrents are other shows: read the title once and
		// move on quietly before any validator is run or anything logged.
		pt := parsePack(t.Name)
		parsed := q.parser.Parse(t.Name)
		if !q.titleNamesRequest(pt, parsed, req) || !q.didTorrentComplete(&t) {
			continue
		}
		uncovered := withoutEpisodes(episodes, covered)
		if len(uncovered) == 0 {
			break
		}
		ss, matched := q.matchFinished(ctx, &t, pt, parsed, req, uncovered)
		if ss == nil {
			continue
		}
		results = append(results, q.emitFinished(ctx, &t, req, ss, matched, done, wg)...)
		for _, ep := range matched {
			covered[ep.Id] = true
		}
	}
	if len(covered) == 0 {
		return nil, nil
	}
	return results, covered
}

// titleNamesRequest reports whether a torrent title, read as a pack or as a
// single episode, names the requested show or one of its aliases.
func (q *QbittHandler) titleNamesRequest(pt packTitle, parsed *ParsedTorrent, req *tvdb.Media) bool {
	if !pt.ParseError && q.showNameScore(pt.ShowName, req) >= minShowNameScore {
		return true
	}
	return parsed != nil && q.showNameScore(parsed.ShowName, req) >= minShowNameScore
}

// showNameScore is the best word-level match between a parsed show name and
// the request's title or any of its aliases.
func (q *QbittHandler) showNameScore(parsedName string, req *tvdb.Media) float64 {
	best := q.scoreNameMatch(parsedName, req.Name)
	for _, alias := range req.Aliases {
		if s := q.scoreNameMatch(parsedName, alias); s > best {
			best = s
		}
	}
	return best
}

// matchFinished decides whether finished torrent t holds any of the uncovered
// episodes; it returns the strategy to link them under and the episodes.
func (q *QbittHandler) matchFinished(ctx context.Context, t *qbittorrent.Torrent, pt packTitle,
	parsed *ParsedTorrent, req *tvdb.Media, uncovered []tvdb.Episode) (*models.SearchStrategy, []tvdb.Episode) {
	fake := &prowlarr.Search{Title: t.Name, Size: t.Size, Seeders: 1}
	if !pt.ParseError && !pt.SingleEp {
		return q.matchFinishedPack(ctx, t, fake, req, uncovered)
	}
	if parsed == nil {
		return nil, nil
	}
	// A single-episode release: only the episode its numbering names is a
	// candidate, and the strict search strategies decide as they would for
	// an indexer result.
	for i := range uncovered {
		ep := uncovered[i]
		if !parsedNamesEpisode(parsed, ep) {
			continue
		}
		for _, ss := range q.createSearchStrategy(req, &ep) {
			if ss.RelaxLevel != 0 || ss.EpisodeMeta == nil || ss.EpisodeTvdbID == "" {
				continue
			}
			if q.isCorrectTorrent(ctx, fake, ss) {
				return ss, []tvdb.Episode{ep}
			}
		}
	}
	q.logger.DebugContext(ctx, "Finished torrent matches no wanted episode",
		telemetry.WithTraceContext(ctx, "show", req.Name, "torrent_title", t.Name)...)
	return nil, nil
}

// parsedNamesEpisode reports whether a parsed single-episode title numbers
// the given episode, by season and number or by absolute number.
func parsedNamesEpisode(parsed *ParsedTorrent, ep tvdb.Episode) bool {
	if parsed.IsAbsolute {
		return ep.AbsoluteNumber > 0 && parsed.Episode == ep.AbsoluteNumber
	}
	return parsed.Season == ep.SeasonNumber && parsed.Episode == ep.Number
}

func (q *QbittHandler) matchFinishedPack(ctx context.Context, t *qbittorrent.Torrent, fake *prowlarr.Search,
	req *tvdb.Media, uncovered []tvdb.Episode) (*models.SearchStrategy, []tvdb.Episode) {
	bySeason := map[int][]tvdb.Episode{}
	for _, ep := range uncovered {
		bySeason[ep.SeasonNumber] = append(bySeason[ep.SeasonNumber], ep)
	}
	seasons := make([]int, 0, len(bySeason))
	for s := range bySeason {
		seasons = append(seasons, s)
	}
	sort.Ints(seasons)

	accept := func(span []int) (bool, string) {
		wanted := []tvdb.Episode{}
		for _, s := range span {
			wanted = append(wanted, bySeason[s]...)
		}
		ok, why, _, _, _ := q.validatePackSpan(fake, req, span, wanted)
		return ok, why
	}
	span := seasons
	ok, why := accept(span)
	if !ok && len(seasons) > 1 {
		for _, s := range seasons {
			if ok, _ = accept([]int{s}); ok {
				span = []int{s}
				break
			}
		}
	}
	if !ok {
		q.logger.DebugContext(ctx, "Finished pack rejected",
			telemetry.WithTraceContext(ctx, "show", req.Name, "torrent_title", t.Name, "reason", why)...)
		return nil, nil
	}

	files, err := q.c.GetFilesInformationCtx(ctx, t.Hash)
	if err != nil || files == nil {
		q.logger.DebugContext(ctx, "Finished pack rejected: files unavailable",
			telemetry.WithTraceContext(ctx, "show", req.Name, "torrent_title", t.Name)...)
		return nil, nil
	}
	spanEpisodes := []tvdb.Episode{}
	for _, s := range span {
		spanEpisodes = append(spanEpisodes, bySeason[s]...)
	}
	info := &models.PackInfo{Show: req.Name, Season: span[0], Seasons: span, Episodes: spanEpisodes}
	byKey := packEpisodeIndex(info)
	seen := map[int]bool{}
	matched := []tvdb.Episode{}
	for _, f := range *files {
		// A file is usable once fully downloaded, whatever its priority now:
		// a trim after the fact deselects files without removing them.
		if !isVideoFile(f.Name) || f.Progress < 1 {
			continue
		}
		for _, ep := range packFileEpisodes(q.parser, f.Name, info, byKey) {
			if !seen[ep.Id] {
				seen[ep.Id] = true
				matched = append(matched, ep)
			}
		}
	}
	if len(matched) == 0 {
		q.logger.DebugContext(ctx, "Finished pack rejected: no file matches a wanted episode",
			telemetry.WithTraceContext(ctx, "show", req.Name, "torrent_title", t.Name)...)
		return nil, nil
	}
	sort.Slice(matched, func(i, j int) bool {
		if matched[i].SeasonNumber != matched[j].SeasonNumber {
			return matched[i].SeasonNumber < matched[j].SeasonNumber
		}
		return matched[i].Number < matched[j].Number
	})
	return q.packStrategy(req, span, matched, "finished: "+t.Name), matched
}

// emitFinished hands a covered set to the completion path and reports it.
func (q *QbittHandler) emitFinished(ctx context.Context, t *qbittorrent.Torrent, req *tvdb.Media,
	ss *models.SearchStrategy, matched []tvdb.Episode, done chan<- models.TorrentCompleteEvent,
	wg *sync.WaitGroup) []dlstatus.EpisodeResult {
	q.updateNotificationStatus(ctx, ss.EpisodeTvdbID, "", notifications.StatusDownloading)
	for _, id := range ss.Pack.EpisodeIDs() {
		if id != ss.EpisodeTvdbID {
			q.updateNotificationStatus(ctx, id, "", notifications.StatusDownloading)
		}
	}

	// Recorded as in flight so a restart between here and the completion
	// handler resumes a monitor that re-emits the event; the handler clears
	// the row once the files are linked. No tracking tag: the torrent was
	// never added by this request.
	active := &models.ActiveTorrent{
		InfoHash:     t.Hash,
		TorrentTitle: t.Name,
		Strategy:     ss,
		StartedAt:    time.Now(),
		Request:      searchRequestFor(req, ss),
	}
	if err := q.repo.InsertActiveTorrent(ctx, active); err != nil {
		q.logger.ErrorContext(ctx, "Failed to persist finished torrent as in-flight",
			telemetry.WithTraceContext(ctx, "error", err.Error(), "torrent_hash", t.Hash)...)
	}

	event := models.TorrentCompleteEvent{
		SavePath:    t.SavePath,
		Req:         ss,
		Hash:        t.Hash,
		SpanContext: trace.SpanContextFromContext(ctx),
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		done <- event
	}()

	results := make([]dlstatus.EpisodeResult, 0, len(matched))
	for _, ep := range matched {
		results = append(results, dlstatus.EpisodeResult{
			TvdbID:  strconv.Itoa(ep.Id),
			Season:  ep.SeasonNumber,
			Episode: ep.Number,
			Outcome: dlstatus.OutcomeDownloading,
		})
	}
	q.logger.InfoContext(ctx, "Episodes covered by a finished download",
		telemetry.WithTraceContext(ctx, "show", req.Name, "torrent_title", t.Name, "torrent_hash", t.Hash,
			"episodes", len(matched))...)
	return results
}
