package notifications

import "time"

// NotificationStatus is an enumerated type for Notification.Status
type NotificationStatus string

const (
	// StatusScheduled indicates the notification is scheduled for future download
	StatusScheduled NotificationStatus = "scheduled"
	// StatusSearching indicates the notification is currently searching for torrents
	StatusSearching NotificationStatus = "searching"
	// StatusDownloading indicates the notification is currently downloading
	StatusDownloading NotificationStatus = "downloading"
	// StatusCompleted indicates the notification completed successfully
	StatusCompleted NotificationStatus = "completed"
	// StatusFailed indicates the notification failed
	StatusFailed NotificationStatus = "failed"
)

// Notification represents a download notification in the database
type Notification struct {
	ID              int                `json:"id"`
	TvdbID          string             `json:"tvdb_id"`          // Episode ID for series, Movie ID for movies
	MediaTitle      string             `json:"media_title"`      // Show/Movie name
	Category        string             `json:"category"`         // 'series' or 'movie'
	Season          *int               `json:"season,omitempty"` // Season number (series only)
	Episode         *int               `json:"episode,omitempty"` // Episode number (series only)
	AbsoluteEpisode *int               `json:"absolute_episode,omitempty"` // Absolute episode number (anime only)
	PosterURL       string             `json:"poster_url"`       // Poster image URL
	IsAnime         bool               `json:"is_anime"`         // Whether media is anime
	Status          NotificationStatus `json:"status"`           // 'scheduled', 'searching', 'downloading', 'completed', 'failed'
	Reason          string             `json:"reason,omitempty"` // Error message, failure reason, or success details
	IsRead          bool               `json:"is_read"`          // Whether user has read the notification
	AutoDismissed   bool               `json:"auto_dismissed"`   // Whether notification was auto-dismissed
	TraceID         string             `json:"trace_id"`         // OpenTelemetry trace ID
	SpanID          string             `json:"span_id"`          // OpenTelemetry span ID
	CreatedAt       time.Time          `json:"created_at"`       // Creation timestamp
	UpdatedAt       time.Time          `json:"updated_at"`       // Last update timestamp
}

// GroupedNotification groups notifications by media
type GroupedNotification struct {
	TvdbID          string         `json:"tvdb_id"`
	MediaTitle      string         `json:"media_title"`
	Category        string         `json:"category"`
	PosterURL       string         `json:"poster_url"`
	IsAnime         bool           `json:"is_anime"`
	LatestTimestamp time.Time      `json:"latest_timestamp"`
	Notifications   []Notification `json:"notifications"`
}
