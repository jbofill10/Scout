package handlers

import (
	"github.com/jbofill10/scout/backend/internal/torrenter/interactors"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
)

type MediaHandler struct {
	interactor *interactors.MediaInteractor
	logger     *slog.Logger
}

func NewMediaHandler(interactor *interactors.MediaInteractor, logger *slog.Logger) *MediaHandler {
	return &MediaHandler{
		interactor: interactor,
		logger:     logger,
	}
}

func (h *MediaHandler) MediaExists(c *gin.Context) {
	hash := c.Param("hash")
	// Extract context for trace propagation and logging
	ctx := c.Request.Context()

	if hash == "" {
		h.logger.WarnContext(ctx, "Missing hash parameter")
		c.JSON(http.StatusBadRequest, gin.H{"error": "hash parameter is required"})
		return
	}

	exists, err := h.interactor.CheckMediaExists(ctx, hash)
	if err != nil {
		h.logger.ErrorContext(ctx, "Error checking media existence", "hash", hash, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if exists {
		h.logger.InfoContext(ctx, "Media exists", "hash", hash)
		c.JSON(http.StatusOK, gin.H{})
	} else {
		h.logger.InfoContext(ctx, "Media not found", "hash", hash)
		c.JSON(http.StatusNotFound, gin.H{})
	}
}

// MediaExistsBatch checks existence for multiple media IDs and returns map-based response.
func (h *MediaHandler) MediaExistsBatch(c *gin.Context) {
	ctx := c.Request.Context()

	var items []tvdb.Media
	if err := c.ShouldBindJSON(&items); err != nil {
		h.logger.ErrorContext(ctx, "Failed to bind media exists batch request", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	exists, err := h.interactor.CheckMediaExistsBatch(ctx, items)
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed batch media existence check", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check media existence"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"exists":      exists,
		"in_progress": map[string]bool{},
	})
}
