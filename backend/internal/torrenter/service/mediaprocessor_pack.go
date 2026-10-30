package service

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"

	"github.com/jbofill10/scout/backend/internal/torrenter/models"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
)

// packFileEpRe matches "E05", " - 05 ", "ep05" style numbering in a file name.
var packFileEpRe = regexp.MustCompile(`(?i)(?:\bE|\bep\.? ?|- )0?(\d{1,3})\b`)

// processPack links every video file of a season pack that matches a wanted
// episode, and records per episode what happened in event.PackResults. The
// call fails only when nothing at all could be linked.
func (mp *MediaProcessSvc) processPack(ctx context.Context, event *models.TorrentCompleteEvent) error {
	pack := event.Req.Pack
	event.PackResults = map[string]string{}
	for _, ep := range pack.Episodes {
		event.PackResults[strconv.Itoa(ep.Id)] = "not in the season pack"
	}

	files, err := mp.fs.WalkFiles(event.SavePath)
	if err != nil {
		return fmt.Errorf("failed to read pack directory %s: %w", event.SavePath, err)
	}

	// Match each video file to a wanted episode by its own numbering.
	byKey := map[string]tvdb.Episode{}
	for _, ep := range pack.Episodes {
		byKey[fmt.Sprintf("s%de%d", ep.SeasonNumber, ep.Number)] = ep
		if ep.AbsoluteNumber > 0 {
			byKey[fmt.Sprintf("abs%d", ep.AbsoluteNumber)] = ep
		}
	}
	parser := NewTorrentParser()
	linked := 0
	for _, f := range files {
		if !isVideoFile(f.Path) {
			continue
		}
		name := filepath.Base(f.Path)
		parsed := parser.Parse(name)
		if parsed == nil {
			// Bare "E05" / "05" names inside a season folder.
			parsed = parsePackFileFallback(name, pack.Season)
		}
		if parsed == nil || parsed.IsMultiEpisode() {
			mp.Logger.InfoContext(ctx, "Pack file not matched to an episode", "file", f.Path)
			continue
		}
		var ep tvdb.Episode
		var ok bool
		if parsed.IsAbsolute {
			ep, ok = byKey[fmt.Sprintf("abs%d", parsed.Episode)]
			if !ok {
				ep, ok = byKey[fmt.Sprintf("s%de%d", pack.Season, parsed.Episode)]
			}
		} else {
			season := parsed.Season
			if season == 0 {
				season = pack.Season
			}
			ep, ok = byKey[fmt.Sprintf("s%de%d", season, parsed.Episode)]
		}
		if !ok {
			continue
		}
		id := strconv.Itoa(ep.Id)
		if event.PackResults[id] == "linked" {
			continue // a second copy of the same episode (sample, alt audio)
		}
		single := *event
		strategy := *event.Req
		epCopy := ep
		strategy.EpisodeMeta = &epCopy
		strategy.EpisodeTvdbID = id
		strategy.Season = ep.SeasonNumber
		strategy.Episode = ep.Number
		strategy.Pack = nil
		single.Req = &strategy
		if err := mp.linkFile(ctx, &single, f.Path); err != nil {
			mp.Logger.ErrorContext(ctx, "Failed to link pack episode", "file", f.Path, "episode", ep.Number, "error", err)
			event.PackResults[id] = "failed to link: " + err.Error()
			continue
		}
		event.PackResults[id] = "linked"
		linked++
	}

	mp.Logger.InfoContext(ctx, "Season pack processed", "show", event.Req.MediaName, "season", pack.Season,
		"wanted", len(pack.Episodes), "linked", linked)
	if linked == 0 {
		return fmt.Errorf("no wanted episode found in the season pack at %s", event.SavePath)
	}
	return nil
}

// parsePackFileFallback handles file names the main parser rejects but that
// are unambiguous inside a season pack: a lone episode number such as
// "Show - 05.mkv" or "E05.mkv".
func parsePackFileFallback(name string, season int) *ParsedTorrent {
	m := packFileEpRe.FindStringSubmatch(name)
	if m == nil {
		return nil
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n == 0 {
		return nil
	}
	return &ParsedTorrent{Season: season, Episode: n, IsSeasonal: true, MatchConfidence: 0.5, Raw: name}
}
