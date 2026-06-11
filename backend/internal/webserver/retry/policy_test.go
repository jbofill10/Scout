package retry

import (
	"testing"
	"time"
)

func TestFirstAttempt(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// FirstAttempt must equal the first backoff step (and NextAttempt at 0).
	if got, want := FirstAttempt(now), now.Add(backoff[0]); !got.Equal(want) {
		t.Fatalf("FirstAttempt = %v, want %v", got, want)
	}
	next, ok := NextAttempt(now, 0)
	if !ok || !FirstAttempt(now).Equal(next) {
		t.Fatalf("FirstAttempt must match NextAttempt(now, 0): %v vs %v (ok=%v)", FirstAttempt(now), next, ok)
	}
}

func TestNextAttempt(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		attemptsMade int
		wantOK      bool
		wantDelay   time.Duration
	}{
		{name: "first failure -> 1h", attemptsMade: 0, wantOK: true, wantDelay: 1 * time.Hour},
		{name: "second failure -> 4h", attemptsMade: 1, wantOK: true, wantDelay: 4 * time.Hour},
		{name: "third failure -> 12h", attemptsMade: 2, wantOK: true, wantDelay: 12 * time.Hour},
		{name: "fourth failure -> 24h", attemptsMade: 3, wantOK: true, wantDelay: 24 * time.Hour},
		{name: "fifth failure -> exhausted", attemptsMade: 4, wantOK: false},
		{name: "beyond max -> exhausted", attemptsMade: 10, wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := NextAttempt(now, tt.attemptsMade)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !tt.wantOK {
				if !got.IsZero() {
					t.Fatalf("expected zero time when exhausted, got %v", got)
				}
				return
			}
			if want := now.Add(tt.wantDelay); !got.Equal(want) {
				t.Fatalf("next attempt = %v, want %v", got, want)
			}
		})
	}
}
