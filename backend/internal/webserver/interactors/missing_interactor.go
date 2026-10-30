package interactors

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/jbofill10/scout/backend/pkg/library"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
)

type missingTorrenter interface {
	GetMissingEpisodes(ctx context.Context) ([]library.MissingShow, error)
}

// MissingInteractor lists the aired episodes library shows lack, marks the
// ones Scout is already handling, and requests the rest on demand.
type MissingInteractor struct {
	torrenter missingTorrenter
	sched     followScheduler
	notifs    followNotifications
	requester showRequester
	logger    *slog.Logger
}

func NewMissingInteractor(
	torrenter missingTorrenter,
	sched followScheduler,
	notifs followNotifications,
	requester showRequester,
	logger *slog.Logger,
) *MissingInteractor {
	return &MissingInteractor{torrenter: torrenter, sched: sched, notifs: notifs, requester: requester, logger: logger}
}

// List returns every library show with missing episodes, each episode marked
// Requested when a scheduled row or a notification already exists for it.
func (m *MissingInteractor) List(ctx context.Context) ([]library.MissingShow, error) {
	shows, err := m.torrenter.GetMissingEpisodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("missing episodes: %w", err)
	}
	if shows == nil {
		shows = []library.MissingShow{}
	}

	ids := []string{}
	for _, s := range shows {
		for _, ep := range s.Episodes {
			ids = append(ids, ep.TvdbId)
		}
	}
	tracked, err := m.sched.TrackedItemKeys(ctx)
	if err != nil {
		m.logger.ErrorContext(ctx, "Failed to load tracked items for missing list", "error", err)
		tracked = map[string]bool{}
	}
	notified, err := m.notifs.ExistingTvdbIDs(ctx, ids)
	if err != nil {
		m.logger.ErrorContext(ctx, "Failed to load notifications for missing list", "error", err)
		notified = map[string]bool{}
	}
	for si := range shows {
		for ei := range shows[si].Episodes {
			id := shows[si].Episodes[ei].TvdbId
			shows[si].Episodes[ei].Requested = tracked[id] || notified[id]
		}
	}
	return shows, nil
}

// FillMissing requests a show's missing episodes that are not already
// requested, through the normal show request path. ErrNothingToControl when
// the show has nothing missing.
func (m *MissingInteractor) FillMissing(ctx context.Context, tvdbID string) (DownloadSummary, error) {
	shows, err := m.List(ctx)
	if err != nil {
		return DownloadSummary{}, err
	}
	var show *library.MissingShow
	for i := range shows {
		if shows[i].TvdbId == tvdbID {
			show = &shows[i]
			break
		}
	}
	if show == nil {
		return DownloadSummary{}, ErrNothingToControl
	}

	summary := DownloadSummary{MediaTitle: show.Title, Category: "series"}
	episodes := []tvdb.Episode{}
	for _, ep := range show.Episodes {
		if ep.Requested {
			summary.Skipped++
			continue
		}
		id, err := strconv.Atoi(ep.TvdbId)
		if err != nil {
			summary.Skipped++
			continue
		}
		episodes = append(episodes, tvdb.Episode{
			Id:             id,
			SeasonNumber:   ep.SeasonNumber,
			Number:         ep.EpisodeNumber,
			AbsoluteNumber: ep.AbsoluteNumber,
			Name:           ep.Name,
			Aired:          ep.Aired,
		})
	}
	if len(episodes) == 0 {
		m.logger.InfoContext(ctx, "Every missing episode is already requested", "tvdb_id", tvdbID, "title", show.Title)
		return summary, nil
	}

	media := tvdb.Media{Id: tvdbID, Name: show.Title, Category: "series"}
	media.Metadata.Episodes = episodes
	result, err := m.requester.DownloadShow(ctx, media)
	if err != nil {
		return summary, err
	}
	result.Skipped += summary.Skipped
	m.logger.InfoContext(ctx, "Requested missing episodes", "tvdb_id", tvdbID, "title", show.Title,
		"queued_now", result.QueuedNow, "scheduled", result.Scheduled, "skipped", result.Skipped)
	return result, nil
}
