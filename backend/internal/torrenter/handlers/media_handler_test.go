package handlers

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jbofill10/scout/backend/internal/torrenter/interactors"
	serviceMocks "github.com/jbofill10/scout/backend/internal/torrenter/service/mocks"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func setupMediaHandler(t *testing.T) (*MediaHandler, *serviceMocks.Repository) {
	gin.SetMode(gin.TestMode)
	repo := serviceMocks.NewRepository(t)
	interactor := interactors.NewMediaInteractor(repo)
	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil))
	return NewMediaHandler(interactor, logger), repo
}

func TestMediaExistsBatch_Success(t *testing.T) {
	handler, repo := setupMediaHandler(t)

	repo.On("MediaExists", mock.Anything, "series-1").Return(true, nil).Once()
	repo.On("MediaExists", mock.Anything, "movie-1").Return(false, nil).Once()

	items := []tvdb.Media{
		{Id: "series-1"},
		{Id: "movie-1"},
	}
	body, err := json.Marshal(items)
	assert.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/media/exists", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.MediaExistsBatch(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Exists     map[string]bool `json:"exists"`
		InProgress map[string]bool `json:"in_progress"`
	}
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.Equal(t, map[string]bool{"series-1": true, "movie-1": false}, resp.Exists)
	assert.NotNil(t, resp.InProgress)
	assert.Empty(t, resp.InProgress)
}

func TestMediaExistsBatch_InvalidBody(t *testing.T) {
	handler, _ := setupMediaHandler(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/media/exists", bytes.NewBufferString(`{"id":"not-an-array"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.MediaExistsBatch(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "invalid request body")
}
