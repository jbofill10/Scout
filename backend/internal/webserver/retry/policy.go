// Package retry defines the backoff policy used by the webserver's download
// retry engine. Transient download failures are rescheduled with an increasing
// delay until a maximum number of attempts is reached.
package retry

import (
	"time"

	"github.com/jbofill10/scout/backend/pkg/dlstatus"
)

// backoff holds the delay applied before each retry, indexed by the number of
// attempts already made. attemptsMade >= len(backoff) clamps to the last entry.
var backoff = []time.Duration{
	1 * time.Hour,
	4 * time.Hour,
	12 * time.Hour,
	24 * time.Hour,
}

// MaxAttempts is the total number of download attempts allowed (including the
// first). Once this many attempts have been made, no further retries occur.
const MaxAttempts = 5

// FirstAttempt returns the time of the first retry given an initial failure
// (zero prior attempts). It is the single source of truth for the initial retry
// delay used when scheduling a retry row for an immediate-download failure.
func FirstAttempt(now time.Time) time.Time {
	next, _ := NextAttempt(now, 0)
	return next
}

// NextAttempt returns the time of the next retry given how many attempts have
// already been made (attemptsMade is the count BEFORE the failure being
// handled; 0 on the first failure). The bool is false when retries are
// exhausted, in which case the returned time is the zero value.
func NextAttempt(now time.Time, attemptsMade int) (time.Time, bool) {
	return NextAttemptWithin(now, attemptsMade, MaxAttempts)
}

// OldContentAge is how long after airing a no-result search stops being retried on the full ladder.
const OldContentAge = 30 * 24 * time.Hour

// OldContentAttempts is the ladder for no_torrent_found on content older than OldContentAge: the first try and one retry.
const OldContentAttempts = 2

// MaxAttemptsFor is the attempt ladder for a failure: OldContentAttempts for no_torrent_found on content that aired
// more than OldContentAge before now, MaxAttempts otherwise (unknown air date counts as recent).
func MaxAttemptsFor(code dlstatus.FailureCode, aired, now time.Time) int {
	if code == dlstatus.CodeNoTorrentFound && !aired.IsZero() && now.Sub(aired) > OldContentAge {
		return OldContentAttempts
	}
	return MaxAttempts
}

// NextAttemptWithin is NextAttempt with the ladder capped at maxAttempts.
func NextAttemptWithin(now time.Time, attemptsMade, maxAttempts int) (time.Time, bool) {
	// attemptsMade counts attempts already performed. After this failure there
	// have been attemptsMade+1 attempts; if that reaches maxAttempts, stop.
	if attemptsMade >= maxAttempts-1 {
		return time.Time{}, false
	}

	idx := attemptsMade
	if idx >= len(backoff) {
		idx = len(backoff) - 1
	}
	return now.Add(backoff[idx]), true
}
