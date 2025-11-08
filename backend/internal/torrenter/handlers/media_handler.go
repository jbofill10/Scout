package handlers

import (
	"log/slog"
	"net/http"
	"github.com/jbofill10/scout/backend/internal/torrenter/interactors"

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
