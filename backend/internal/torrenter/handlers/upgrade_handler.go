package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/jbofill10/scout/backend/internal/torrenter/interactors"
	"github.com/jbofill10/scout/backend/internal/torrenter/models"
	"github.com/jbofill10/scout/backend/pkg/telemetry"
)

type upgradeService interface {
	Candidates(ctx context.Context) (models.UpgradeCandidatesView, error)
	Attempts(ctx context.Context, limit int) ([]models.UpgradeAttempt, error)
	Run(ctx context.Context) (models.UpgradeRunResult, error)
}

// UpgradeHandler serves the upgrade pass.
type UpgradeHandler struct {
	interactor upgradeService
	logger     *slog.Logger
}

func NewUpgradeHandler(interactor upgradeService, logger *slog.Logger) *UpgradeHandler {
	return &UpgradeHandler{interactor: interactor, logger: logger}
}

func (h *UpgradeHandler) Candidates(c *gin.Context) {
	ctx := c.Request.Context()
	view, err := h.interactor.Candidates(ctx)
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to list upgrade candidates", telemetry.WithTraceContext(ctx, "error", err)...)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list upgrade candidates"})
		return
	}
	c.JSON(http.StatusOK, view)
}

func (h *UpgradeHandler) Attempts(c *gin.Context) {
	ctx := c.Request.Context()
	attempts, err := h.interactor.Attempts(ctx, 25)
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to list upgrade attempts", telemetry.WithTraceContext(ctx, "error", err)...)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list upgrade attempts"})
		return
	}
	if attempts == nil {
		attempts = []models.UpgradeAttempt{}
	}
	c.JSON(http.StatusOK, attempts)
}

// Run: 202 with the batch; 409 while a pass is running; 412 when upgrades are off.
func (h *UpgradeHandler) Run(c *gin.Context) {
	ctx := c.Request.Context()
	result, err := h.interactor.Run(ctx)
	if err != nil {
		switch {
		case errors.Is(err, interactors.ErrUpgradesOff):
			c.JSON(http.StatusPreconditionFailed, gin.H{"error": "Upgrades are turned off in Settings"})
		case errors.Is(err, interactors.ErrUpgradeRunning):
			c.JSON(http.StatusConflict, gin.H{"error": "An upgrade pass is already running"})
		default:
			h.logger.ErrorContext(ctx, "Upgrade run failed", telemetry.WithTraceContext(ctx, "error", err)...)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start the upgrade pass"})
		}
		return
	}
	c.JSON(http.StatusAccepted, result)
}
