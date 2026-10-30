package models

import (
	"fmt"
	"strconv"
	"time"

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
	// PackResults is filled by the media processor for a season pack:
	// episode tvdb id -> "linked" or the reason it was not.
	PackResults map[string]string
}

type MediaExistsRequest struct {
	ID string `json:"id"`
}

// ActiveTorrent is a torrent that is downloading and not yet accounted for.
// It is persisted so the completion monitor can be resumed after a restart —
// the monitor is otherwise pure in-memory state and dies with the process.
type ActiveTorrent struct {
	InfoHash     string
	TrackingUUID string
	TorrentTitle string
	Strategy     *SearchStrategy
	StartedAt    time.Time
	// Request is what is needed to search for this item again from scratch,
	// so a download whose swarm turns out to be dead can be replaced, even
	// after a restart. Nil for rows written before it was recorded.
	Request *SearchRequest
}

// SearchRequest is the media identity and aliases the search strategies for
// one episode (or movie) are built from, plus how many dead torrents have
// already been swapped out for it.
type SearchRequest struct {
	Media    tvdb.Media    // Id, Name, Category, Anime, Aliases; Metadata.Episodes is empty
	Episode  *tvdb.Episode // nil for movies
	Replaced int
}

// FailedTorrent records a torrent that was tried for an item and gave up on:
// no seeds, monitor timeout, or removed from qBittorrent. Later searches for
// the same item skip it, by info hash when the indexer supplies one and by
// exact title otherwise.
type FailedTorrent struct {
	TvdbID   string // show id for series, media id for movies
	Season   int
	Episode  int
	InfoHash string
	Title    string
	Reason   string
}

// FileEntry is a single file discovered underneath a download directory.
// Path is relative to the directory that was walked, so it can contain the
// torrent's own root folder (qBittorrent nests multi-file torrents inside one).
type FileEntry struct {
	Path string
	Size int64
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
	// EpisodeTvdbID is the TVDB identity used for per-episode notifications and
	// EpisodeResult — episode id for series, media id for movies. Distinct from
	// TvdbId, which is the show/media id used for library base-directory lookups.
	EpisodeTvdbID string
	IsMovie       bool // True if this is a movie search strategy
	// ImdbID of the movie, or of the parent show for an episode, carried so
	// subtitles can be matched once the download lands. Empty when unknown.
	ImdbID string
	// RuntimeMinutes of the movie or episode when TVDB knows it; 0 otherwise.
	RuntimeMinutes int
	// RelaxLevel is the query-relaxation tier this strategy belongs to.
	// 0 = strict (exact S##E## / padded absolute), 1 = relaxed (alternate
	// episode formats, unpadded absolute, punctuation-stripped names),
	// 2 = broad (loose tokens / name-only). Higher tiers are only tried when
	// lower ones yield no confident match, and matches from tier >= 2 are
	// gated behind a higher minimum confidence in pickBestTorrent.
	RelaxLevel int
	// MinTier and MaxTier bound the acceptable quality for this search
	// (QualityTier values); zero means no bound beyond the global floor. An
	// upgrade sets both: above the copy it replaces, at most the cutoff.
	MinTier int
	MaxTier int
	// Upgrade is set when this download replaces an existing library copy.
	Upgrade *UpgradeInfo
	// Pack is set when this torrent is a season pack covering several
	// episodes; EpisodeTvdbID/EpisodeMeta then refer to the first of them.
	Pack *PackInfo
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

// UpgradeInfo rides on an upgrade's strategy so the completion path knows
// what to remove once the better copy has landed.
type UpgradeInfo struct {
	FromQuality string   `json:"from_quality"`
	OldFiles    []string `json:"old_files"`
}

// PackInfo describes the episodes a season-pack torrent is expected to hold.
type PackInfo struct {
	Season   int            `json:"season"`
	Episodes []tvdb.Episode `json:"episodes"`
}

// EpisodeIDs lists the pack's episode tvdb ids as strings.
func (p *PackInfo) EpisodeIDs() []string {
	if p == nil {
		return nil
	}
	out := make([]string, 0, len(p.Episodes))
	for _, ep := range p.Episodes {
		out = append(out, strconv.Itoa(ep.Id))
	}
	return out
}
