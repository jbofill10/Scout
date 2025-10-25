package interactors

import (
	"torrenter/internal/service"
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
func (i *MediaInteractor) CheckMediaExists(hash string) (bool, error) {
	return i.repo.MediaExists(hash)
}
