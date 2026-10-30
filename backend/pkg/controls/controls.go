// Package controls holds the request and response shapes the webserver and the
// torrenter share for acting on a requested item after the fact: cancelling it,
// listing the releases the indexers have for it, and downloading one by hand.
//
// Every call is keyed by the item's notification tvdb id: the TVDB episode id
// for series and the media id for movies. That is the key the UI already has
// on every activity row and the key the torrenter stores on each strategy.
package controls

import (
	"time"

	tvdb "github.com/jbofill10/scout/backend/pkg/media"
)

// CancelRequest asks the torrenter to stop everything it is doing for an item.
type CancelRequest struct {
	ItemID string `json:"item_id" binding:"required"`
}

// CancelResponse reports what the cancel found to stop.
type CancelResponse struct {
	ItemID              string `json:"item_id"`
	SearchCancelled     bool   `json:"search_cancelled"`
	TorrentsRemoved     int    `json:"torrents_removed"`
	NotificationUpdated bool   `json:"notification_updated"`
}

// Found reports whether the cancel had anything to act on.
func (r CancelResponse) Found() bool {
	return r.SearchCancelled || r.TorrentsRemoved > 0 || r.NotificationUpdated
}

// ReleasesRequest asks for the candidate releases of one item. Media is the
// single-episode (or movie) payload when the caller has it; when it is nil the
// torrenter falls back to the request stored with the item's in-flight torrent.
type ReleasesRequest struct {
	ItemID string      `json:"item_id" binding:"required"`
	Media  *tvdb.Media `json:"media,omitempty"`
}

// ReleaseCandidate is one torrent an indexer returned for the item, with the
// verdict the automatic picker would reach on it.
type ReleaseCandidate struct {
	Title       string    `json:"title"`
	Indexer     string    `json:"indexer"`
	IndexerID   int64     `json:"indexer_id"`
	GUID        string    `json:"guid"`
	DownloadURL string    `json:"download_url,omitempty"`
	InfoHash    string    `json:"info_hash,omitempty"`
	Size        int64     `json:"size"`
	Seeders     int       `json:"seeders"`
	Leechers    int       `json:"leechers"`
	PublishDate time.Time `json:"publish_date"`
	// Parsed from the title; empty when the parser could not tell.
	Quality      string `json:"quality,omitempty"`
	Source       string `json:"source,omitempty"`
	Codec        string `json:"codec,omitempty"`
	ReleaseGroup string `json:"release_group,omitempty"`
	// Accepted is whether Scout would pick this release on its own. Reason
	// explains a rejection; Score ranks accepted candidates.
	Accepted bool    `json:"accepted"`
	Reason   string  `json:"reason,omitempty"`
	Score    float64 `json:"score"`
	// PreviouslyFailed marks a release Scout already tried for this item and
	// gave up on (dead swarm, timeout, removed).
	PreviouslyFailed bool `json:"previously_failed"`
	// Query is the search string that surfaced the release.
	Query string `json:"query"`
}

// ReleasesResponse is the candidate list for one item.
type ReleasesResponse struct {
	ItemID     string             `json:"item_id"`
	MediaTitle string             `json:"media_title"`
	Queries    int                `json:"queries"`
	Candidates []ReleaseCandidate `json:"candidates"`
}

// ReleaseSelection identifies the candidate the user chose. GUID or
// DownloadURL is what qBittorrent is given; the rest is bookkeeping.
type ReleaseSelection struct {
	GUID        string `json:"guid"`
	DownloadURL string `json:"download_url"`
	InfoHash    string `json:"info_hash"`
	Title       string `json:"title"`
	Indexer     string `json:"indexer"`
	IndexerID   int64  `json:"indexer_id"`
	Size        int64  `json:"size"`
	Seeders     int    `json:"seeders"`
	Leechers    int    `json:"leechers"`
}

// Valid reports whether the selection carries enough to hand to qBittorrent.
func (s ReleaseSelection) Valid() bool {
	return s.Title != "" && (s.GUID != "" || s.DownloadURL != "")
}

// ManualDownloadRequest asks the torrenter to download exactly one release for
// an item, replacing whatever it had in flight for it.
type ManualDownloadRequest struct {
	ItemID  string           `json:"item_id" binding:"required"`
	Media   *tvdb.Media      `json:"media,omitempty"`
	Release ReleaseSelection `json:"release"`
}
