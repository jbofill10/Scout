package handlers

import (
	"log"
	tvdb "shared/media"
	"webserver/internal/interactors"

	"github.com/gin-gonic/gin"
)

type DownloadHandler struct {
	interactor *interactors.DownloadInteractor
	logger     *log.Logger
}

func NewDownloadHandler(interactor *interactors.DownloadInteractor, logger *log.Logger) *DownloadHandler {
	return &DownloadHandler{
		interactor: interactor,
		logger:     logger,
	}
}

func (h *DownloadHandler) DownloadShow(c *gin.Context) {
	var req tvdb.Media
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Printf("Bad request: %v", err)
		c.JSON(400, gin.H{"error": "Bad Request"})
		return
	}

	err := h.interactor.DownloadShow(req)
	if err != nil {
		h.logger.Printf("Failed to download show: %v", err)
		c.JSON(500, gin.H{"error": "Failed to download show"})
		return
	}

	h.logger.Printf("Successfully processed show: %s", req.Name)
	c.JSON(200, gin.H{})
}

func (h *DownloadHandler) DownloadMovie(c *gin.Context) {
	// TODO: Implement movie download
	h.logger.Printf("Movie download not yet implemented")
	c.JSON(501, gin.H{"error": "Not Implemented"})
}
