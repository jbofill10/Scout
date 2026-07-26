package handlers

import (
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jbofill10/scout/backend/internal/torrenter/models"
)

type PlexProxyHandler struct {
	plexCfg *models.PlexCfg
	logger  *slog.Logger
}

func NewPlexProxyHandler(plexCfg *models.PlexCfg, logger *slog.Logger) *PlexProxyHandler {
	return &PlexProxyHandler{
		plexCfg: plexCfg,
		logger:  logger,
	}
}

// ProxyThumb proxies Plex thumbnail requests with authentication
// GET /plex/thumb?path=/library/metadata/123/thumb/456
func (h *PlexProxyHandler) ProxyThumb(c *gin.Context) {
	path := c.Query("path")
	if path == "" {
		h.logger.Warn("Missing path parameter for Plex thumb proxy")
		c.JSON(http.StatusBadRequest, gin.H{"error": "path parameter is required"})
		return
	}

	// Construct full Plex URL
	plexURL := h.plexCfg.Host + path

	// Create request to Plex
	req, err := http.NewRequestWithContext(c.Request.Context(), "GET", plexURL, nil)
	if err != nil {
		h.logger.Error("Failed to create Plex request", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to proxy thumbnail"})
		return
	}

	// Add Plex authentication
	req.Header.Set("X-Plex-Token", h.plexCfg.Key)

	// Make request to Plex
	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		h.logger.Error("Failed to fetch from Plex", "error", err, "url", plexURL)
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to fetch thumbnail from Plex"})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		h.logger.Warn("Plex returned non-200 status", "status", resp.StatusCode, "url", plexURL)
		c.JSON(resp.StatusCode, gin.H{"error": "Plex server error"})
		return
	}

	// Copy Plex response to client
	c.Header("Content-Type", resp.Header.Get("Content-Type"))
	c.Header("Cache-Control", "public, max-age=86400") // Cache for 24 hours
	c.Status(http.StatusOK)

	if _, err := io.Copy(c.Writer, resp.Body); err != nil {
		h.logger.Error("Failed to stream thumbnail", "error", err)
	}
}
