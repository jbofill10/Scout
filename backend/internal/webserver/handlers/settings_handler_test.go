package handlers

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

type stubProxy struct {
	status int
	body   string
	err    error
	method string
	path   string
	sent   string
}

func (s *stubProxy) Proxy(_ context.Context, method, path string, body []byte) (int, []byte, error) {
	s.method, s.path, s.sent = method, path, string(body)
	return s.status, []byte(s.body), s.err
}

func settingsRelayRouter(p *stubProxy) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := NewSettingsHandler(p, slog.New(slog.NewTextHandler(new(bytes.Buffer), nil)))
	r := gin.New()
	r.GET("/settings", h.Get)
	r.PUT("/settings", h.Update)
	r.DELETE("/settings/uploaders/:id", h.DeleteUploader)
	return r
}

func TestSettingsHandler_RelaysStatusAndBody(t *testing.T) {
	p := &stubProxy{status: http.StatusBadRequest, body: `{"error":"nope","field":"min_quality"}`}
	r := settingsRelayRouter(p)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/settings", strings.NewReader(`{"min_quality":"900p"}`))
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, "the torrenter's validation status passes through")
	assert.Equal(t, `{"error":"nope","field":"min_quality"}`, w.Body.String())
	assert.Equal(t, "PUT", p.method)
	assert.Equal(t, "/settings", p.path)
	assert.Equal(t, `{"min_quality":"900p"}`, p.sent)
}

func TestSettingsHandler_DeleteAndUnreachable(t *testing.T) {
	p := &stubProxy{status: http.StatusNoContent}
	w := httptest.NewRecorder()
	settingsRelayRouter(p).ServeHTTP(w, httptest.NewRequest("DELETE", "/settings/uploaders/7", nil))
	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, "/settings/uploaders/7", p.path)

	w = httptest.NewRecorder()
	settingsRelayRouter(&stubProxy{err: errors.New("down")}).ServeHTTP(w, httptest.NewRequest("GET", "/settings", nil))
	assert.Equal(t, http.StatusBadGateway, w.Code)
}
