package handlers

import (
	"context"
	"io"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/jbofill10/scout/backend/pkg/telemetry"
)

// torrenterProxy is the one call the settings handler needs from the client.
type torrenterProxy interface {
	Proxy(ctx context.Context, method, path string, body []byte) (int, []byte, error)
}

// SettingsHandler relays the runtime settings endpoints to the torrenter,
// which owns their storage and meaning. Statuses and bodies pass through
// unchanged, so a validation error reaches the UI with its field name.
type SettingsHandler struct {
	torrenter torrenterProxy
	logger    *slog.Logger
}

func NewSettingsHandler(torrenter torrenterProxy, logger *slog.Logger) *SettingsHandler {
	return &SettingsHandler{torrenter: torrenter, logger: logger}
}

func (h *SettingsHandler) Get(c *gin.Context)    { h.relay(c, http.MethodGet, "/settings") }
func (h *SettingsHandler) Update(c *gin.Context) { h.relay(c, http.MethodPut, "/settings") }
func (h *SettingsHandler) ListUploaders(c *gin.Context) {
	h.relay(c, http.MethodGet, "/settings/uploaders")
}
func (h *SettingsHandler) AddUploader(c *gin.Context) {
	h.relay(c, http.MethodPost, "/settings/uploaders")
}
func (h *SettingsHandler) DeleteUploader(c *gin.Context) {
	h.relay(c, http.MethodDelete, "/settings/uploaders/"+c.Param("id"))
}

func (h *SettingsHandler) relay(c *gin.Context, method, path string) {
	ctx := c.Request.Context()
	var body []byte
	if c.Request.Body != nil && (method == http.MethodPut || method == http.MethodPost) {
		raw, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Could not read request"})
			return
		}
		body = raw
	}

	status, out, err := h.torrenter.Proxy(ctx, method, path, body)
	if err != nil {
		h.logger.ErrorContext(ctx, "Settings relay failed", telemetry.WithTraceContext(ctx, "error", err, "path", path)...)
		c.JSON(http.StatusBadGateway, gin.H{"error": "Download service unreachable"})
		return
	}
	if status == http.StatusNoContent || len(out) == 0 {
		c.Status(status)
		return
	}
	c.Data(status, "application/json", out)
}
