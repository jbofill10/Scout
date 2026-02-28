package interactors

import (
	"context"
	"github.com/jbofill10/scout/backend/internal/torrenter/service"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
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

// CheckMediaExistsBatch checks whether each media item exists.
// Result map keys are media IDs from the request payload.
func (i *MediaInteractor) CheckMediaExistsBatch(ctx context.Context, items []tvdb.Media) (map[string]bool, error) {
	result := make(map[string]bool, len(items))

	for _, item := range items {
		if item.Id == "" {
			continue
		}
		exists, err := i.repo.MediaExists(ctx, item.Id)
		if err != nil {
			return nil, err
		}
		result[item.Id] = exists
	}

	return result, nil
}
