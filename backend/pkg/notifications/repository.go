package notifications

import "context"

// Repository defines the core notification repository interface
// Both webserver and torrenter must implement these methods
type Repository interface {
	// Create creates a new notification
	CreateNotification(ctx context.Context, notification *Notification) error

	// Get retrieves a notification by its TVDB ID
	// For series: tvdbID is the episode ID from TVDB
	// For movies: tvdbID is the movie ID from TVDB
	GetNotification(ctx context.Context, tvdbID string) (*Notification, error)

	// Update updates an existing notification
	UpdateNotification(ctx context.Context, notification *Notification) error
}
