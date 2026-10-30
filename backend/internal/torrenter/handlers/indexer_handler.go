package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/jbofill10/scout/backend/internal/torrenter/clients"
	"github.com/jbofill10/scout/backend/internal/torrenter/searchgate"
	"github.com/jbofill10/scout/backend/internal/torrenter/service"
	"github.com/jbofill10/scout/backend/pkg/telemetry"
)

type indexerHealth interface {
	Snapshot() service.IndexerHealthSnapshot
	RotateNow(ctx context.Context, reason string) (service.HealthEvent, error)
	TestIndexer(ctx context.Context, id int64) error
}

// searchQueue is the search gate's per-indexer view: what is running, what
// is waiting and at what rate.
type searchQueue interface {
	Snapshot() []searchgate.IndexerStats
}

// IndexerHandler serves indexer health, the search queue and relay rotation.
type IndexerHandler struct {
	monitor indexerHealth
	queue   searchQueue
	logger  *slog.Logger
}

func NewIndexerHandler(monitor indexerHealth, queue searchQueue, logger *slog.Logger) *IndexerHandler {
	return &IndexerHandler{monitor: monitor, queue: queue, logger: logger}
}

// healthResponse is the health snapshot plus the search queue per indexer.
type healthResponse struct {
	service.IndexerHealthSnapshot
	Queue []searchgate.IndexerStats `json:"queue"`
}

func (h *IndexerHandler) Health(c *gin.Context) {
	resp := healthResponse{IndexerHealthSnapshot: h.monitor.Snapshot(), Queue: []searchgate.IndexerStats{}}
	if h.queue != nil {
		resp.Queue = h.queue.Snapshot()
	}
	c.JSON(http.StatusOK, resp)
}

// Rotate: 200 with the event; 409 when the agent refuses (cooldown, nothing
// to pick); 503 when no agent is configured.
func (h *IndexerHandler) Rotate(c *gin.Context) {
	ctx := c.Request.Context()
	var body struct {
		Reason string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&body)
	ev, err := h.monitor.RotateNow(ctx, body.Reason)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrNoAgent):
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "No relay agent is configured"})
		case errors.Is(err, clients.ErrRotateRefused):
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		default:
			h.logger.ErrorContext(ctx, "Relay rotation failed", telemetry.WithTraceContext(ctx, "error", err)...)
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusOK, ev)
}

func (h *IndexerHandler) Test(c *gin.Context) {
	ctx := c.Request.Context()
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id must be a number"})
		return
	}
	if err := h.monitor.TestIndexer(ctx, id); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
