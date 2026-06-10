// Package retry defines the backoff policy used by the webserver's download
// retry engine. Transient download failures are rescheduled with an increasing
// delay until a maximum number of attempts is reached.
package retry

import "time"

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
	// attemptsMade counts attempts already performed. After this failure there
	// have been attemptsMade+1 attempts; if that reaches MaxAttempts, stop.
	if attemptsMade >= MaxAttempts-1 {
		return time.Time{}, false
	}

	idx := attemptsMade
	if idx >= len(backoff) {
		idx = len(backoff) - 1
	}
	return now.Add(backoff[idx]), true
}
