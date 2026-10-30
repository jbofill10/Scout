package handlers

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jbofill10/scout/backend/internal/torrenter/repository"
	torrsettings "github.com/jbofill10/scout/backend/internal/torrenter/settings"
	"github.com/jbofill10/scout/backend/pkg/settings"
	"github.com/stretchr/testify/assert"
)

type stubResolver struct{ applyErr error }

func (s *stubResolver) View(context.Context) (settings.View, error) { return settings.View{}, nil }
func (s *stubResolver) Apply(context.Context, settings.Update) (settings.View, error) {
	return settings.View{}, s.applyErr
}

type stubUploaders struct {
	addErr error
	found  bool
}

func (s *stubUploaders) ListUploaderPreferences(context.Context) ([]settings.UploaderPreference, error) {
	return nil, nil
}
func (s *stubUploaders) AddUploaderPreference(_ context.Context, p settings.NewUploaderPreference) (settings.UploaderPreference, error) {
	return settings.UploaderPreference{ID: 1, MediaType: p.MediaType, Uploader: p.Uploader}, s.addErr
}
func (s *stubUploaders) DeleteUploaderPreference(context.Context, int) (bool, error) {
	return s.found, nil
}

func settingsRouter(r *stubResolver, u *stubUploaders) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := NewSettingsHandler(r, u, slog.New(slog.NewTextHandler(new(bytes.Buffer), nil)))
	e := gin.New()
	e.GET("/settings", h.Get)
	e.PUT("/settings", h.Update)
	e.GET("/settings/uploaders", h.ListUploaders)
	e.POST("/settings/uploaders", h.AddUploader)
	e.DELETE("/settings/uploaders/:id", h.DeleteUploader)
	return e
}

func call(e *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	e.ServeHTTP(w, req)
	return w
}

func TestSettingsHandler_StatusCodes(t *testing.T) {
	t.Run("validation error is a 400 naming the field", func(t *testing.T) {
		e := settingsRouter(&stubResolver{applyErr: &torrsettings.ValidationError{Field: "min_quality", Msg: "nope"}}, &stubUploaders{})
		w := call(e, "PUT", "/settings", `{"min_quality":"900p"}`)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), `"field":"min_quality"`)
	})

	t.Run("uploader list is never null", func(t *testing.T) {
		w := call(settingsRouter(&stubResolver{}, &stubUploaders{}), "GET", "/settings/uploaders", "")
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "[]", strings.TrimSpace(w.Body.String()))
	})

	t.Run("add validates and reports duplicates", func(t *testing.T) {
		assert.Equal(t, http.StatusBadRequest,
			call(settingsRouter(&stubResolver{}, &stubUploaders{}), "POST", "/settings/uploaders", `{"media_type":"book","uploader":"x"}`).Code)
		assert.Equal(t, http.StatusCreated,
			call(settingsRouter(&stubResolver{}, &stubUploaders{}), "POST", "/settings/uploaders", `{"media_type":"movie","uploader":"QxR"}`).Code)
		assert.Equal(t, http.StatusConflict,
			call(settingsRouter(&stubResolver{}, &stubUploaders{addErr: repository.ErrDuplicateUploader}), "POST", "/settings/uploaders", `{"media_type":"movie","uploader":"QxR"}`).Code)
	})

	t.Run("delete", func(t *testing.T) {
		assert.Equal(t, http.StatusNoContent, call(settingsRouter(&stubResolver{}, &stubUploaders{found: true}), "DELETE", "/settings/uploaders/3", "").Code)
		assert.Equal(t, http.StatusNotFound, call(settingsRouter(&stubResolver{}, &stubUploaders{}), "DELETE", "/settings/uploaders/3", "").Code)
		assert.Equal(t, http.StatusBadRequest, call(settingsRouter(&stubResolver{}, &stubUploaders{}), "DELETE", "/settings/uploaders/x", "").Code)
	})
}
