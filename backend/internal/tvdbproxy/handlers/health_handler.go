package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// TokenSource exposes the TVDB bearer token currently in use.
type TokenSource interface {
	CurrentToken() string
}

// HealthHandler serves the liveness and readiness probes.
type HealthHandler struct {
	tokens TokenSource
}

// NewHealthHandler returns a HealthHandler that reports ready once tokens holds a token.
func NewHealthHandler(tokens TokenSource) *HealthHandler {
	return &HealthHandler{tokens: tokens}
}

// Health handles GET /health.
func (h *HealthHandler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// Ready handles GET /ready: ready once the proxy holds a TVDB token.
func (h *HealthHandler) Ready(c *gin.Context) {
	if h.tokens.CurrentToken() != "" {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	} else {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable"})
	}
}
