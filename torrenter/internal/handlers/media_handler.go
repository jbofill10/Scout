package handlers

import (
	"log"
	"net/http"
	"torrenter/internal/interactors"

	"github.com/gin-gonic/gin"
)

type MediaHandler struct {
	interactor *interactors.MediaInteractor
	logger     *log.Logger
}

func NewMediaHandler(interactor *interactors.MediaInteractor, logger *log.Logger) *MediaHandler {
	return &MediaHandler{
		interactor: interactor,
		logger:     logger,
	}
}

func (h *MediaHandler) MediaExists(c *gin.Context) {
	hash := c.Param("hash")
	if hash == "" {
		h.logger.Println("Missing hash parameter")
		c.JSON(http.StatusBadRequest, gin.H{"error": "hash parameter is required"})
		return
	}

	exists, err := h.interactor.CheckMediaExists(hash)
	if err != nil {
		h.logger.Printf("Error checking media existence: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if exists {
		c.JSON(http.StatusOK, gin.H{})
	} else {
		c.JSON(http.StatusNotFound, gin.H{})
	}
}
