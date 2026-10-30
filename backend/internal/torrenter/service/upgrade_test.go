package service

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/jbofill10/scout/backend/internal/torrenter/models"
	repomocks "github.com/jbofill10/scout/backend/internal/torrenter/repository/mocks"
	"github.com/jbofill10/scout/backend/internal/torrenter/service/mocks"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"golift.io/starr/prowlarr"
)

// A strategy's quality window overrides the global floor upwards and caps
// what an upgrade may take, so a 720p copy is replaced by 1080p, not 4K.
func TestScoreMatch_QualityWindow(t *testing.T) {
	h := newResumeTestHandler(t, repomocks.NewRepository(t))
	h.minQualityTier = 0
	ss := &models.SearchStrategy{MediaName: "Some Show", Season: 2, Episode: 13, MinTier: QualityTier("1080p"), MaxTier: QualityTier("1080p")}
	cases := map[string]bool{
		"Some.Show.S02E13.720p.WEB.x264-GRP":  false,
		"Some.Show.S02E13.1080p.WEB.x264-GRP": true,
		"Some.Show.S02E13.2160p.WEB.x265-GRP": false,
	}
	for title, accepted := range cases {
		match := &models.TorrentMatch{Strategy: ss, Torrent: &prowlarr.Search{Title: title, Seeders: 50}}
		vr := h.validateTorrent(match.Torrent, ss)
		if !vr.IsValid {
			t.Fatalf("%s should parse as a valid match: %s", title, vr.Reason)
		}
		verdict := h.scoreMatch(context.Background(), match, vr, nil)
		assert.Equal(t, accepted, verdict.rejection == "", "%s: %s", title, verdict.rejection)
	}
}

// An upgrade's old copies are removed before the new file is linked, and a
// file already at the destination is cleared too; missing ones are skipped.
func TestRemoveOldCopies(t *testing.T) {
	fs := mocks.NewFileSystem(t)
	fs.On("Exists", "/lib/old-720.mkv").Return(true)
	fs.On("Remove", "/lib/old-720.mkv").Return(nil)
	fs.On("Exists", "/lib/gone.mkv").Return(false)
	fs.On("Exists", "/lib/new.mkv").Return(true)
	fs.On("Remove", "/lib/new.mkv").Return(nil)
	mp := &MediaProcessSvc{fs: fs, Logger: slog.New(slog.NewTextHandler(new(bytes.Buffer), nil))}

	mp.removeOldCopies(context.Background(), []string{"/lib/old-720.mkv", "/lib/gone.mkv", "/lib/old-720.mkv"}, "/lib/new.mkv")

	fs.AssertNumberOfCalls(t, "Remove", 2)
	fs.AssertNotCalled(t, "Remove", "/lib/gone.mkv")
	_ = mock.Anything
}
