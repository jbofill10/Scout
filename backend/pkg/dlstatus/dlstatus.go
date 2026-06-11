// Package dlstatus defines the failure taxonomy and structured response
// contract for the torrenter's /download endpoint. The webserver's retry
// engine (Phase 2) consumes the per-episode outcomes produced here to decide
// whether a download should be retried.
package dlstatus

// FailureCategory classifies whether a failure is worth retrying.
type FailureCategory string

const (
	// CategoryTransient failures may succeed on a later attempt (e.g. an
	// indexer being temporarily unreachable, or no torrent existing yet).
	CategoryTransient FailureCategory = "transient"
	// CategoryPermanent failures will not be resolved by retrying.
	CategoryPermanent FailureCategory = "permanent"
)

// FailureCode is a stable, machine-readable identifier for a failure reason.
type FailureCode string

const (
	CodeIndexerUnreachable   FailureCode = "indexer_unreachable"   // transient
	CodeNoTorrentFound       FailureCode = "no_torrent_found"      // transient
	CodeTorrentClientError   FailureCode = "torrent_client_error"  // transient
	CodeMonitorLost          FailureCode = "monitor_lost"          // transient
	CodeTorrenterUnreachable FailureCode = "torrenter_unreachable" // transient (set by webserver client)
	CodeProcessingError      FailureCode = "processing_error"      // permanent
	CodeInvalidMedia         FailureCode = "invalid_media"         // permanent
	CodeMaxRetries           FailureCode = "max_retries_exceeded"  // permanent
)

// Outcome values describe the result of a per-episode (or per-movie) download attempt.
const (
	OutcomeDownloading = "downloading"
	OutcomeExists      = "exists"
	OutcomeFailed      = "failed"
)

// Category reports whether a failure code is transient (retryable) or permanent.
// Unknown codes are treated as permanent to avoid retrying indefinitely.
func (c FailureCode) Category() FailureCategory {
	switch c {
	case CodeIndexerUnreachable,
		CodeNoTorrentFound,
		CodeTorrentClientError,
		CodeMonitorLost,
		CodeTorrenterUnreachable:
		return CategoryTransient
	case CodeProcessingError,
		CodeInvalidMedia,
		CodeMaxRetries:
		return CategoryPermanent
	default:
		return CategoryPermanent
	}
}

// HumanReason returns a user-facing explanation for a failure code.
func (c FailureCode) HumanReason() string {
	switch c {
	case CodeIndexerUnreachable:
		return "Indexer unreachable — will retry"
	case CodeNoTorrentFound:
		return "No torrent found yet — will retry"
	case CodeTorrentClientError:
		return "Torrent client error — will retry"
	case CodeMonitorLost:
		return "Lost track of the download — will retry"
	case CodeTorrenterUnreachable:
		return "Download service unreachable — will retry"
	case CodeProcessingError:
		return "Failed to process the downloaded files"
	case CodeInvalidMedia:
		return "Media request was invalid"
	case CodeMaxRetries:
		return "Gave up after repeated attempts"
	default:
		return "Unknown failure"
	}
}

// EpisodeResult is the outcome of attempting to download a single episode
// (for series) or a single movie. For movies, TvdbID is the media id and
// Season/Episode are 0.
type EpisodeResult struct {
	TvdbID  string      `json:"tvdb_id"` // episode id for shows; media id for movies
	Season  int         `json:"season"`
	Episode int         `json:"episode"`
	Outcome string      `json:"outcome"` // "downloading" | "exists" | "failed"
	Code    FailureCode `json:"code,omitempty"`
	Reason  string      `json:"reason,omitempty"`
}

// DownloadResponse is the structured body returned by the torrenter's
// /download endpoint and decoded by the webserver client.
type DownloadResponse struct {
	Results []EpisodeResult `json:"results"`
}
