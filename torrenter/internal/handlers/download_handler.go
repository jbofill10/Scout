package handlers

import (
	"log"
	"net/http"
	tvdb "shared/media"
	"torrenter/internal/interactors"

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

func (h *DownloadHandler) DownloadTorrent(c *gin.Context) {
	var req tvdb.Media
	if err := c.ShouldBind(&req); err != nil {
		h.logger.Printf("Failed to bind request: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.interactor.InitiateDownload(&req); err != nil {
		h.logger.Printf("Failed to initiate download: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Torrent download initiated successfully"})
}
