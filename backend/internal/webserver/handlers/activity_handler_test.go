package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jbofill10/scout/backend/internal/webserver/interactors"
	"github.com/stretchr/testify/assert"
)

type stubActivityService struct {
	snapshot interactors.ActivitySnapshot
	cleared  int64
	err      error
}

func (s *stubActivityService) GetActivity(context.Context, int) (interactors.ActivitySnapshot, error) {
	return s.snapshot, s.err
}

func (s *stubActivityService) ClearFinished(context.Context) (int64, error) {
	return s.cleared, s.err
}

func setupActivityHandler(svc *stubActivityService) *ActivityHandler {
	gin.SetMode(gin.TestMode)
	return NewActivityHandler(svc, slog.New(slog.NewTextHandler(new(bytes.Buffer), nil)))
}

func TestClearFinishedNotifications_Success(t *testing.T) {
	handler := setupActivityHandler(&stubActivityService{cleared: 3})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("DELETE", "/notifications", nil)

	handler.ClearFinishedNotifications(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var response struct {
		Message string `json:"message"`
		Count   int64  `json:"count"`
	}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, "Finished notifications cleared", response.Message)
	assert.Equal(t, int64(3), response.Count)
}

func TestClearFinishedNotifications_Error(t *testing.T) {
	handler := setupActivityHandler(&stubActivityService{err: errors.New("db down")})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("DELETE", "/notifications", nil)

	handler.ClearFinishedNotifications(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
