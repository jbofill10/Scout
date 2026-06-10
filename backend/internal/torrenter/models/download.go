package models

import (
	"fmt"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"

	"go.opentelemetry.io/otel/trace"
	"golift.io/starr/prowlarr"
)

type DownloadRequest struct {
	MediaName   string `form:"mediaName" binding:"required"`
	MediaType   string `form:"mediaType" binding:"required"`
	ReleaseYear string `form:"releaseYear" binding:"required"`
	Episode     string `form:"episode" binding:"-"`
	Season      string `form:"season" binding:"-"`
}

func (d *DownloadRequest) IsShow() bool {
	return d.Episode != ""
}

func (d *DownloadRequest) String() string {
	return fmt.Sprintf("[MediaName: %s, MediaType: %s, Episode: %s]", d.MediaName, d.MediaType, d.Episode)
}

type TorrentCompleteEvent struct {
	SavePath    string
	Req         *SearchStrategy
	Hash        string
	UUID        string
	SpanContext trace.SpanContext
}

type MediaExistsRequest struct {
	ID string `json:"id"`
}

type SearchStrategy struct {
	MediaName   string
	EpisodeMeta *tvdb.Episode
	Query       string
	Season      int
	Episode     int
	Exclude     []string
	ReleaseYear string
	TvdbId      string
	IsMovie     bool // True if this is a movie search strategy
	// RelaxLevel is the query-relaxation tier this strategy belongs to.
	// 0 = strict (exact S##E## / padded absolute), 1 = relaxed (alternate
	// episode formats, unpadded absolute, punctuation-stripped names),
	// 2 = broad (loose tokens / name-only). Higher tiers are only tried when
	// lower ones yield no confident match, and matches from tier >= 2 are
	// gated behind a higher minimum confidence in pickBestTorrent.
	RelaxLevel int
}

type TorrentMatch struct {
	Strategy *SearchStrategy
	Torrent  *prowlarr.Search
}

func (tm *TorrentMatch) String() string {
	if tm == nil {
		return "<nil>"
	}
	if tm.Torrent == nil || tm.Strategy == nil {
		return fmt.Sprintf("TorrentMatch{Strategy: %v, Torrent: %v}", tm.Strategy, tm.Torrent)
	}
	return fmt.Sprintf("TorrentMatch{Title: %s, Query: %s, Seeders: %d}", tm.Torrent.Title, tm.Strategy.Query, tm.Torrent.Seeders)
}
