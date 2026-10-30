package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/jbofill10/scout/backend/internal/torrenter/repository"
	torrsettings "github.com/jbofill10/scout/backend/internal/torrenter/settings"
	"github.com/jbofill10/scout/backend/pkg/settings"
	"github.com/jbofill10/scout/backend/pkg/telemetry"
)

// settingsResolver is what the handler needs from the settings resolver.
type settingsResolver interface {
	View(ctx context.Context) (settings.View, error)
	Apply(ctx context.Context, upd settings.Update) (settings.View, error)
}

// uploaderStore is the uploader-preference slice of the repository.
type uploaderStore interface {
	ListUploaderPreferences(ctx context.Context) ([]settings.UploaderPreference, error)
	AddUploaderPreference(ctx context.Context, p settings.NewUploaderPreference) (settings.UploaderPreference, error)
	DeleteUploaderPreference(ctx context.Context, id int) (bool, error)
}

// SettingsHandler serves the runtime settings and the preferred-uploader list.
type SettingsHandler struct {
	resolver  settingsResolver
	uploaders uploaderStore
	logger    *slog.Logger
}

func NewSettingsHandler(resolver settingsResolver, uploaders uploaderStore, logger *slog.Logger) *SettingsHandler {
	return &SettingsHandler{resolver: resolver, uploaders: uploaders, logger: logger}
}

// Get returns the effective settings, their defaults and what is overridden.
func (h *SettingsHandler) Get(c *gin.Context) {
	ctx := c.Request.Context()
	view, err := h.resolver.View(ctx)
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to load settings", telemetry.WithTraceContext(ctx, "error", err)...)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load settings"})
		return
	}
	c.JSON(http.StatusOK, view)
}

// Update applies a partial update; 400 names the invalid field.
func (h *SettingsHandler) Update(c *gin.Context) {
	ctx := c.Request.Context()
	var upd settings.Update
	if err := c.ShouldBindJSON(&upd); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Malformed settings update"})
		return
	}
	view, err := h.resolver.Apply(ctx, upd)
	if err != nil {
		var verr *torrsettings.ValidationError
		if errors.As(err, &verr) {
			c.JSON(http.StatusBadRequest, gin.H{"error": verr.Msg, "field": verr.Field})
			return
		}
		h.logger.ErrorContext(ctx, "Failed to update settings", telemetry.WithTraceContext(ctx, "error", err)...)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save settings"})
		return
	}
	c.JSON(http.StatusOK, view)
}

// ListUploaders returns every preferred uploader.
func (h *SettingsHandler) ListUploaders(c *gin.Context) {
	ctx := c.Request.Context()
	prefs, err := h.uploaders.ListUploaderPreferences(ctx)
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to list uploader preferences", telemetry.WithTraceContext(ctx, "error", err)...)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load uploader preferences"})
		return
	}
	if prefs == nil {
		prefs = []settings.UploaderPreference{}
	}
	c.JSON(http.StatusOK, prefs)
}

// AddUploader stores a preferred uploader; 409 when it is already there.
func (h *SettingsHandler) AddUploader(c *gin.Context) {
	ctx := c.Request.Context()
	var p settings.NewUploaderPreference
	if err := c.ShouldBindJSON(&p); err != nil || !p.Valid() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "media_type must be series or movie and uploader must be 1 to 64 characters"})
		return
	}
	pref, err := h.uploaders.AddUploaderPreference(ctx, p)
	if err != nil {
		if errors.Is(err, repository.ErrDuplicateUploader) {
			c.JSON(http.StatusConflict, gin.H{"error": "That uploader is already preferred for this media type"})
			return
		}
		h.logger.ErrorContext(ctx, "Failed to add uploader preference", telemetry.WithTraceContext(ctx, "error", err)...)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save uploader preference"})
		return
	}
	c.JSON(http.StatusCreated, pref)
}

// DeleteUploader removes a preferred uploader; 404 when the id is unknown.
func (h *SettingsHandler) DeleteUploader(c *gin.Context) {
	ctx := c.Request.Context()
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id must be a number"})
		return
	}
	found, err := h.uploaders.DeleteUploaderPreference(ctx, id)
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to delete uploader preference", telemetry.WithTraceContext(ctx, "error", err, "id", id)...)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete uploader preference"})
		return
	}
	if !found {
		c.JSON(http.StatusNotFound, gin.H{"error": "No such uploader preference"})
		return
	}
	c.Status(http.StatusNoContent)
}
