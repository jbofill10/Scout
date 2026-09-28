package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jbofill10/scout/backend/internal/webserver/repository/mocks"
	"github.com/jbofill10/scout/backend/pkg/notifications"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func setupNotificationHandler(t *testing.T) (*NotificationHandler, *mocks.NotificationRepository) {
	gin.SetMode(gin.TestMode)
	mockRepo := mocks.NewNotificationRepository(t)
	buf := new(bytes.Buffer)
	logger := slog.New(slog.NewTextHandler(buf, nil))
	handler := NewNotificationHandler(mockRepo, logger)
	return handler, mockRepo
}

func TestGetNotifications_Success(t *testing.T) {
	handler, mockRepo := setupNotificationHandler(t)

	season := 1
	episode := 2
	notifs := []notifications.Notification{
		{
			ID:         1,
			TvdbID:     "123456",
			MediaTitle: "Test Show",
			Category:   "series",
			Season:     &season,
			Episode:    &episode,
			Status:     "completed",
			CreatedAt:  time.Now(),
		},
	}

	mockRepo.On("GetNotifications", mock.Anything, 100, "", "").Return(notifs, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/notifications", nil)

	handler.GetNotifications(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var response []notifications.Notification
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Len(t, response, 1)
	assert.Equal(t, "Test Show", response[0].MediaTitle)
}

func TestGetNotifications_WithFilters(t *testing.T) {
	handler, mockRepo := setupNotificationHandler(t)

	notifs := []notifications.Notification{
		{
			ID:         2,
			TvdbID:     "789012",
			MediaTitle: "Test Movie",
			Category:   "movie",
			Status:     "scheduled",
			CreatedAt:  time.Now(),
		},
	}

	mockRepo.On("GetNotifications", mock.Anything, 50, "movie", "scheduled").Return(notifs, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/notifications?limit=50&category=movie&status=scheduled", nil)

	handler.GetNotifications(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var response []notifications.Notification
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Len(t, response, 1)
	assert.Equal(t, "movie", response[0].Category)
}

func TestGetNotifications_Error(t *testing.T) {
	handler, mockRepo := setupNotificationHandler(t)

	mockRepo.On("GetNotifications", mock.Anything, 100, "", "").Return(nil, errors.New("database error"))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/notifications", nil)

	handler.GetNotifications(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestGetGroupedNotifications_Success(t *testing.T) {
	handler, mockRepo := setupNotificationHandler(t)

	season := 1
	episode := 2
	grouped := []notifications.GroupedNotification{
		{
			TvdbID:          "123456",
			MediaTitle:      "Test Show",
			Category:        "series",
			PosterURL:       "https://example.com/poster.jpg",
			LatestTimestamp: time.Now(),
			Notifications: []notifications.Notification{
				{
					ID:         1,
					TvdbID:     "123456",
					MediaTitle: "Test Show",
					Category:   "series",
					Season:     &season,
					Episode:    &episode,
					Status:     "completed",
				},
			},
		},
	}

	mockRepo.On("GetGrouped", mock.Anything).Return(grouped, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/notifications/grouped", nil)

	handler.GetGroupedNotifications(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var response []notifications.GroupedNotification
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Len(t, response, 1)
	assert.Equal(t, "Test Show", response[0].MediaTitle)
	assert.Len(t, response[0].Notifications, 1)
}

func TestGetGroupedNotifications_Error(t *testing.T) {
	handler, mockRepo := setupNotificationHandler(t)

	mockRepo.On("GetGrouped", mock.Anything).Return(nil, errors.New("database error"))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/notifications/grouped", nil)

	handler.GetGroupedNotifications(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestGetUnreadCount_Success(t *testing.T) {
	handler, mockRepo := setupNotificationHandler(t)

	mockRepo.On("GetUnreadCount", mock.Anything).Return(5, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/notifications/unread/count", nil)

	handler.GetUnreadCount(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var response map[string]int
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, 5, response["count"])
}

func TestGetUnreadCount_Error(t *testing.T) {
	handler, mockRepo := setupNotificationHandler(t)

	mockRepo.On("GetUnreadCount", mock.Anything).Return(0, errors.New("database error"))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/notifications/unread/count", nil)

	handler.GetUnreadCount(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestMarkAsRead_Success(t *testing.T) {
	handler, mockRepo := setupNotificationHandler(t)

	mockRepo.On("MarkAsRead", mock.Anything, 1).Return(nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("PATCH", "/notifications/1/read", nil)
	c.Params = gin.Params{{Key: "id", Value: "1"}}

	handler.MarkAsRead(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var response map[string]string
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "Notification marked as read", response["message"])
}

func TestMarkAsRead_InvalidID(t *testing.T) {
	handler, _ := setupNotificationHandler(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("PATCH", "/notifications/invalid/read", nil)
	c.Params = gin.Params{{Key: "id", Value: "invalid"}}

	handler.MarkAsRead(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestMarkAsRead_NotFound(t *testing.T) {
	handler, mockRepo := setupNotificationHandler(t)

	mockRepo.On("MarkAsRead", mock.Anything, 999).Return(errors.New("notification not found: id=999"))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("PATCH", "/notifications/999/read", nil)
	c.Params = gin.Params{{Key: "id", Value: "999"}}

	handler.MarkAsRead(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestDismissNotification_Success(t *testing.T) {
	handler, mockRepo := setupNotificationHandler(t)

	mockRepo.On("Dismiss", mock.Anything, 1).Return(nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("DELETE", "/notifications/1", nil)
	c.Params = gin.Params{{Key: "id", Value: "1"}}

	handler.DismissNotification(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var response map[string]string
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "Notification dismissed", response["message"])
}

func TestDismissNotification_InvalidID(t *testing.T) {
	handler, _ := setupNotificationHandler(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("DELETE", "/notifications/invalid", nil)
	c.Params = gin.Params{{Key: "id", Value: "invalid"}}

	handler.DismissNotification(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestDismissNotification_NotFound(t *testing.T) {
	handler, mockRepo := setupNotificationHandler(t)

	mockRepo.On("Dismiss", mock.Anything, 999).Return(errors.New("notification not found: id=999"))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("DELETE", "/notifications/999", nil)
	c.Params = gin.Params{{Key: "id", Value: "999"}}

	handler.DismissNotification(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}
