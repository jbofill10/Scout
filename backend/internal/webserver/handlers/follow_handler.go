package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/jbofill10/scout/backend/internal/webserver/interactors"
	"github.com/jbofill10/scout/backend/internal/webserver/repository"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/jbofill10/scout/backend/pkg/telemetry"
)

type followService interface {
	Follow(ctx context.Context, media tvdb.Media, includeMissing bool) (interactors.FollowResponse, error)
	Unfollow(ctx context.Context, tvdbID string) (bool, error)
	List(ctx context.Context) ([]repository.FollowedShow, error)
	CheckOne(ctx context.Context, tvdbID string) (interactors.CheckResult, error)
	CheckAll(ctx context.Context) (interactors.CheckReport, error)
}

// FollowHandler serves followed shows.
type FollowHandler struct {
	interactor followService
	logger     *slog.Logger
}

func NewFollowHandler(interactor followService, logger *slog.Logger) *FollowHandler {
	return &FollowHandler{interactor: interactor, logger: logger}
}

type followRequest struct {
	Media          tvdb.Media `json:"media"`
	IncludeMissing bool       `json:"include_missing"`
}

// List: every followed show.
func (h *FollowHandler) List(c *gin.Context) {
	ctx := c.Request.Context()
	shows, err := h.interactor.List(ctx)
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to list follows", telemetry.WithTraceContext(ctx, "error", err)...)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load followed shows"})
		return
	}
	if shows == nil {
		shows = []repository.FollowedShow{}
	}
	c.JSON(http.StatusOK, shows)
}

// Follow: 201 with the stored follow and the first check's outcome.
func (h *FollowHandler) Follow(c *gin.Context) {
	ctx := c.Request.Context()
	var req followRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Media.Id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "media with an id is required"})
		return
	}
	resp, err := h.interactor.Follow(ctx, req.Media, req.IncludeMissing)
	if err != nil {
		if errors.Is(err, interactors.ErrNotSeries) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Only shows can be followed"})
			return
		}
		h.logger.ErrorContext(ctx, "Failed to follow show", telemetry.WithTraceContext(ctx, "error", err, "tvdb_id", req.Media.Id)...)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to follow show"})
		return
	}
	c.JSON(http.StatusCreated, resp)
}

// Unfollow: 204, or 404 when the show was not followed.
func (h *FollowHandler) Unfollow(c *gin.Context) {
	ctx := c.Request.Context()
	found, err := h.interactor.Unfollow(ctx, c.Param("tvdbId"))
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to unfollow show", telemetry.WithTraceContext(ctx, "error", err)...)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to unfollow show"})
		return
	}
	if !found {
		c.JSON(http.StatusNotFound, gin.H{"error": "That show is not followed"})
		return
	}
	c.Status(http.StatusNoContent)
}

// CheckOne: re-check one show now.
func (h *FollowHandler) CheckOne(c *gin.Context) {
	ctx := c.Request.Context()
	result, err := h.interactor.CheckOne(ctx, c.Param("tvdbId"))
	if err != nil {
		if errors.Is(err, interactors.ErrNothingToControl) {
			c.JSON(http.StatusNotFound, gin.H{"error": "That show is not followed"})
			return
		}
		h.logger.ErrorContext(ctx, "Follow check failed", telemetry.WithTraceContext(ctx, "error", err)...)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Check failed"})
		return
	}
	c.JSON(http.StatusOK, result)
}

// CheckAll: re-check every followed show now.
func (h *FollowHandler) CheckAll(c *gin.Context) {
	ctx := c.Request.Context()
	report, err := h.interactor.CheckAll(ctx)
	if err != nil {
		h.logger.ErrorContext(ctx, "Follow check failed", telemetry.WithTraceContext(ctx, "error", err)...)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Check failed"})
		return
	}
	c.JSON(http.StatusOK, report)
}
