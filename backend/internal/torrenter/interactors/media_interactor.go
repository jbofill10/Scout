package interactors

import (
	"context"
	"github.com/jbofill10/scout/backend/internal/torrenter/service"
)

type MediaInteractor struct {
	repo service.Repository
}

func NewMediaInteractor(repo service.Repository) *MediaInteractor {
	return &MediaInteractor{
		repo: repo,
	}
}

// CheckMediaExists checks if media with the given hash exists
func (i *MediaInteractor) CheckMediaExists(ctx context.Context, hash string) (bool, error) {
	return i.repo.MediaExists(ctx, hash)
}
