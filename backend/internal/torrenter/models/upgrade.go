package models

import "time"

// UpgradeMediaRow is one library file with its Plex resolution, as the
// upgrade pass reads it from the mirror.
type UpgradeMediaRow struct {
	Kind          string // "episode" or "movie"
	ShowTvdbID    string
	ShowTitle     string
	EpisodeTvdbID string
	Season        int
	Episode       int
	MovieTvdbID   string
	MovieTitle    string
	Year          int
	Resolution    string // Plex videoResolution: sd, 480, 720, 1080, 4k
	FilePath      string
}

// UpgradeAttempt is the pass's memory of one item.
type UpgradeAttempt struct {
	ItemKey     string    `json:"item_key"`
	Kind        string    `json:"kind"`
	Title       string    `json:"title"`
	Season      int       `json:"season,omitempty"`
	Episode     int       `json:"episode,omitempty"`
	FromQuality string    `json:"from_quality"`
	ToQuality   string    `json:"to_quality"`
	Status      string    `json:"status"` // searching, downloading, completed, no_release, failed
	Reason      string    `json:"reason,omitempty"`
	Attempts    int       `json:"attempts"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// UpgradeCandidate is a library item below the cutoff.
type UpgradeCandidate struct {
	ItemKey string `json:"item_key"`
	Kind    string `json:"kind"`
	Title   string `json:"title"`
	// ShowTvdbID is set for episodes (the request path needs the show id).
	ShowTvdbID     string          `json:"show_tvdb_id,omitempty"`
	Season         int             `json:"season,omitempty"`
	Episode        int             `json:"episode,omitempty"`
	Year           int             `json:"year,omitempty"`
	CurrentQuality string          `json:"current_quality"`
	TargetQuality  string          `json:"target_quality"`
	Files          []string        `json:"files"`
	LastAttempt    *UpgradeAttempt `json:"last_attempt,omitempty"`
}

// UpgradeCandidatesView is GET /upgrades/candidates.
type UpgradeCandidatesView struct {
	Enabled    bool               `json:"enabled"`
	Cutoff     string             `json:"cutoff"`
	Batch      int                `json:"batch"`
	Total      int                `json:"total"`
	Candidates []UpgradeCandidate `json:"candidates"`
	// Deferred counts items below the cutoff that are waiting out a retry
	// delay or were given up on.
	Deferred int `json:"deferred"`
}

// UpgradeRunResult is POST /upgrades/run.
type UpgradeRunResult struct {
	Started int      `json:"started"`
	Items   []string `json:"items"`
	Message string   `json:"message,omitempty"`
}
