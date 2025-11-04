package models

import (
	"fmt"
	tvdb "shared/media"

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
}

type TorrentMatch struct {
	Strategy *SearchStrategy
	Torrent  *prowlarr.Search
}
