package service

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/jbofill10/scout/backend/internal/torrenter/models"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"

	"github.com/stretchr/testify/assert"
	"golift.io/starr/prowlarr"
)

func validateHandler() *QbittHandler {
	return &QbittHandler{parser: NewTorrentParser(), logger: slog.New(slog.NewTextHandler(new(bytes.Buffer), nil))}
}

// Cyberpunk: Edgerunners S02E05, 2026-11-04: the fallback indexer returned
// "Cyberpunk Edgerunners S02E05 1080p WEB H264-SUGOI" on every query and the
// torrenter rejected it every time, once because the query was built under
// the alias "Cyberpunk: Edgerunners 2" and once because the absolute-number
// query compared S02E05 against absolute episode 15.
func TestValidateShowTorrent_ReleaseNamingTheShowPassesUnderAnyAlias(t *testing.T) {
	q := validateHandler()
	ep := &tvdb.Episode{Id: 11229821, SeasonNumber: 2, Number: 5, AbsoluteNumber: 15}
	names := []string{"Cyberpunk: Edgerunners", "Cyberpunk: Edgerunners 2", "Cyberpunk: Edgerunners II"}
	release := &prowlarr.Search{Title: "Cyberpunk Edgerunners S02E05 1080p WEB H264-SUGOI"}

	alias := &models.SearchStrategy{Query: "Cyberpunk: Edgerunners 2 S02E05", MediaName: "Cyberpunk: Edgerunners 2",
		Names: names, Season: 2, Episode: 5, EpisodeMeta: ep}
	result := q.validateShowTorrent(release, alias)
	assert.True(t, result.IsValid, "found under an alias, names the show: %s", result.Reason)

	absolute := &models.SearchStrategy{Query: "Cyberpunk: Edgerunners 15", MediaName: "Cyberpunk: Edgerunners",
		Names: names, Season: 2, Episode: 15, EpisodeMeta: ep, Exclude: []string{"season", "episode"}}
	result = q.validateShowTorrent(release, absolute)
	assert.True(t, result.IsValid, "an absolute-number query still wants the S02E05 release: %s", result.Reason)

	wrongEpisode := &prowlarr.Search{Title: "Cyberpunk Edgerunners S02E04 1080p WEB H264-SUGOI"}
	result = q.validateShowTorrent(wrongEpisode, absolute)
	assert.False(t, result.IsValid)
	assert.Equal(t, "episode mismatch: expected 5, got 4", result.Reason, "the episode's own numbers are the yardstick")

	otherShow := &prowlarr.Search{Title: "Cyberpunk 2077 Chronicles S02E05 1080p WEB H264-GRP"}
	result = q.validateShowTorrent(otherShow, alias)
	assert.False(t, result.IsValid)
	assert.Contains(t, result.Reason, "show name mismatch")
}

// A strategy from before Names existed (an ActiveTorrents row written by an
// older build) validates against MediaName alone, as it always did.
func TestValidateShowTorrent_NoNamesFallsBackToMediaName(t *testing.T) {
	q := validateHandler()
	ep := &tvdb.Episode{Id: 1, SeasonNumber: 2, Number: 5}
	release := &prowlarr.Search{Title: "Cyberpunk Edgerunners S02E05 1080p WEB H264-SUGOI"}

	result := q.validateShowTorrent(release, &models.SearchStrategy{MediaName: "Cyberpunk: Edgerunners", Season: 2, Episode: 5, EpisodeMeta: ep})
	assert.True(t, result.IsValid, result.Reason)

	result = q.validateShowTorrent(release, &models.SearchStrategy{MediaName: "Cyberpunk: Edgerunners 2", Season: 2, Episode: 5, EpisodeMeta: ep})
	assert.False(t, result.IsValid, "without Names the alias alone does not match the title")
}

// Every strategy the ladder builds carries the show's names.
func TestCreateSearchStrategy_CarriesEveryName(t *testing.T) {
	q := validateHandler()
	req := &tvdb.Media{Id: "384541", Name: "Cyberpunk: Edgerunners", Anime: true, Aliases: []string{"Cyberpunk: Edgerunners 2"}}
	ep := &tvdb.Episode{Id: 11229821, SeasonNumber: 2, Number: 5, AbsoluteNumber: 15}

	for _, s := range q.createSearchStrategy(req, ep) {
		assert.Equal(t, []string{"Cyberpunk: Edgerunners", "Cyberpunk: Edgerunners 2"}, s.Names, s.Query)
	}
}
