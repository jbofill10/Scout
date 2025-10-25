package handlers

import (
	"log"
	"webserver/internal/interactors"

	"github.com/gin-gonic/gin"
)

type SearchHandler struct {
	interactor *interactors.SearchInteractor
	logger     *log.Logger
}

func NewSearchHandler(interactor *interactors.SearchInteractor, logger *log.Logger) *SearchHandler {
	return &SearchHandler{
		interactor: interactor,
		logger:     logger,
	}
}

func (h *SearchHandler) HandleSearch(c *gin.Context) {
	query := c.Query("query")
	mediaType := c.Query("media_type")
	h.logger.Printf("SearchRequest: query=%s, media_type=%s", query, mediaType)

	searchResults, err := h.interactor.Search(mediaType, query)
	if err != nil {
		h.logger.Printf("Failed to search via TVDB client: %v", err)
		c.JSON(500, gin.H{"error": "Failed to search media"})
		return
	}

	h.logger.Printf("Returning %d search results", len(searchResults))
	c.JSON(200, searchResults)
}
