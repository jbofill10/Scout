package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/jbofill10/scout/backend/internal/webserver/interactors"
	"github.com/jbofill10/scout/backend/pkg/library"
	"github.com/jbofill10/scout/backend/pkg/telemetry"
)

type missingService interface {
	List(ctx context.Context) ([]library.MissingShow, error)
	FillMissing(ctx context.Context, tvdbID string) (interactors.DownloadSummary, error)
}

// MissingHandler serves the missing-episodes list and the fill action.
type MissingHandler struct {
	interactor missingService
	logger     *slog.Logger
}

func NewMissingHandler(interactor missingService, logger *slog.Logger) *MissingHandler {
	return &MissingHandler{interactor: interactor, logger: logger}
}

// List: every library show with missing episodes, requested ones marked.
func (h *MissingHandler) List(c *gin.Context) {
	ctx := c.Request.Context()
	shows, err := h.interactor.List(ctx)
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to list missing episodes", telemetry.WithTraceContext(ctx, "error", err)...)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load missing episodes"})
		return
	}
	c.JSON(http.StatusOK, shows)
}

// Fill: request a show's missing episodes. 202 with the request summary.
func (h *MissingHandler) Fill(c *gin.Context) {
	ctx := c.Request.Context()
	tvdbID := c.Param("tvdbId")
	summary, err := h.interactor.FillMissing(ctx, tvdbID)
	if err != nil {
		if errors.Is(err, interactors.ErrNothingToControl) {
			c.JSON(http.StatusNotFound, gin.H{"error": "That show has nothing missing"})
			return
		}
		h.logger.ErrorContext(ctx, "Failed to fill missing episodes", telemetry.WithTraceContext(ctx, "error", err, "tvdb_id", tvdbID)...)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to request missing episodes"})
		return
	}
	c.JSON(http.StatusAccepted, summary)
}
