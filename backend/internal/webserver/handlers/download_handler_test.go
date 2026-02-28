package handlers

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jbofill10/scout/backend/internal/webserver/interactors"
	"github.com/stretchr/testify/assert"
)

func setupDownloadHandlerForValidationTests() *DownloadHandler {
	gin.SetMode(gin.TestMode)
	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil))
	return NewDownloadHandler(&interactors.DownloadInteractor{}, logger)
}

func TestDownloadShow_RejectsMissingRequiredFields(t *testing.T) {
	handler := setupDownloadHandlerForValidationTests()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/shows", bytes.NewBufferString(`{}`))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.DownloadShow(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Invalid media type")
}

func TestDownloadShow_RejectsMissingEpisodes(t *testing.T) {
	handler := setupDownloadHandlerForValidationTests()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(
		http.MethodPost,
		"/shows",
		bytes.NewBufferString(`{"id":"123","mediaName":"Test Show","type":"series","metadata":{"episodes":[]}}`),
	)
	c.Request.Header.Set("Content-Type", "application/json")

	handler.DownloadShow(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "metadata.episodes")
}

func TestDownloadMovie_RejectsMissingIDAndName(t *testing.T) {
	handler := setupDownloadHandlerForValidationTests()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/movies", bytes.NewBufferString(`{"type":"movie"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.DownloadMovie(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "id and mediaName")
}
