package handlers

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/jbofill10/scout/backend/internal/webserver/repository"
	"github.com/jbofill10/scout/backend/pkg/telemetry"
)

type targetTester interface {
	Test(ctx context.Context, target repository.NotificationTarget) error
}

// NotificationTargetHandler manages outbound notification targets.
type NotificationTargetHandler struct {
	repo   repository.TargetRepository
	tester targetTester
	logger *slog.Logger
}

func NewNotificationTargetHandler(repo repository.TargetRepository, tester targetTester, logger *slog.Logger) *NotificationTargetHandler {
	return &NotificationTargetHandler{repo: repo, tester: tester, logger: logger}
}

type targetInput struct {
	Name    string   `json:"name"`
	Kind    string   `json:"kind"`
	URL     string   `json:"url"`
	Token   *string  `json:"token"` // nil keeps the stored token on update
	Events  []string `json:"events"`
	Enabled *bool    `json:"enabled"`
}

// validate normalises the input and names the first problem.
func (in *targetInput) validate() string {
	in.Name = strings.TrimSpace(in.Name)
	in.Kind = strings.ToLower(strings.TrimSpace(in.Kind))
	in.URL = strings.TrimSpace(in.URL)
	if in.Name == "" || len(in.Name) > 64 {
		return "name is required (up to 64 characters)"
	}
	switch in.Kind {
	case repository.TargetWebhook, repository.TargetNtfy, repository.TargetDiscord:
	default:
		return "kind must be webhook, ntfy or discord"
	}
	u, err := url.Parse(in.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "url must be an http(s) URL"
	}
	if len(in.Events) == 0 {
		return "pick at least one event"
	}
	seen := map[string]bool{}
	clean := []string{}
	for _, e := range in.Events {
		e = strings.ToLower(strings.TrimSpace(e))
		ok := false
		for _, allowed := range repository.TargetEvents {
			if e == allowed {
				ok = true
			}
		}
		if !ok {
			return "events may only contain completed, failed, cancelled"
		}
		if !seen[e] {
			seen[e] = true
			clean = append(clean, e)
		}
	}
	in.Events = clean
	return ""
}

func (h *NotificationTargetHandler) List(c *gin.Context) {
	ctx := c.Request.Context()
	targets, err := h.repo.List(ctx)
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to list notification targets", telemetry.WithTraceContext(ctx, "error", err)...)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load notification targets"})
		return
	}
	c.JSON(http.StatusOK, targets)
}

func (h *NotificationTargetHandler) Create(c *gin.Context) {
	ctx := c.Request.Context()
	var in targetInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Malformed target"})
		return
	}
	if msg := in.validate(); msg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": msg})
		return
	}
	t := repository.NotificationTarget{Name: in.Name, Kind: in.Kind, URL: in.URL, Events: in.Events, Enabled: true}
	if in.Token != nil {
		t.Token = strings.TrimSpace(*in.Token)
	}
	if in.Enabled != nil {
		t.Enabled = *in.Enabled
	}
	created, err := h.repo.Create(ctx, t)
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to create notification target", telemetry.WithTraceContext(ctx, "error", err)...)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save notification target"})
		return
	}
	c.JSON(http.StatusCreated, created)
}

func (h *NotificationTargetHandler) Update(c *gin.Context) {
	ctx := c.Request.Context()
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id must be a number"})
		return
	}
	var in targetInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Malformed target"})
		return
	}
	if msg := in.validate(); msg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": msg})
		return
	}
	t := repository.NotificationTarget{ID: id, Name: in.Name, Kind: in.Kind, URL: in.URL, Events: in.Events, Enabled: true}
	if in.Enabled != nil {
		t.Enabled = *in.Enabled
	}
	keepToken := in.Token == nil
	if !keepToken {
		t.Token = strings.TrimSpace(*in.Token)
	}
	found, err := h.repo.Update(ctx, t, keepToken)
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to update notification target", telemetry.WithTraceContext(ctx, "error", err, "id", id)...)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save notification target"})
		return
	}
	if !found {
		c.JSON(http.StatusNotFound, gin.H{"error": "No such target"})
		return
	}
	updated, err := h.repo.Get(ctx, id)
	if err != nil || updated == nil {
		c.JSON(http.StatusOK, t)
		return
	}
	c.JSON(http.StatusOK, updated)
}

func (h *NotificationTargetHandler) Delete(c *gin.Context) {
	ctx := c.Request.Context()
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id must be a number"})
		return
	}
	found, err := h.repo.Delete(ctx, id)
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to delete notification target", telemetry.WithTraceContext(ctx, "error", err, "id", id)...)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete notification target"})
		return
	}
	if !found {
		c.JSON(http.StatusNotFound, gin.H{"error": "No such target"})
		return
	}
	c.Status(http.StatusNoContent)
}

// Test sends a sample event; 200 {ok:true} or 502 with the delivery error.
func (h *NotificationTargetHandler) Test(c *gin.Context) {
	ctx := c.Request.Context()
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id must be a number"})
		return
	}
	target, err := h.repo.Get(ctx, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load target"})
		return
	}
	if target == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "No such target"})
		return
	}
	if err := h.tester.Test(ctx, *target); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
