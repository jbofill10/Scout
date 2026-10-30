package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jbofill10/scout/backend/internal/webserver/interactors"
	"github.com/jbofill10/scout/backend/internal/webserver/repository"
	"github.com/jbofill10/scout/backend/pkg/controls"
	"github.com/jbofill10/scout/backend/pkg/dlstatus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubControls struct {
	cancelErr error
	retryErr  error
	grabSel   controls.ReleaseSelection
	grabErr   error
}

func (s *stubControls) Cancel(context.Context, string) (interactors.CancelOutcome, error) {
	return interactors.CancelOutcome{RowsCancelled: 1}, s.cancelErr
}
func (s *stubControls) RetryNow(context.Context, string) (interactors.RetryOutcome, error) {
	return interactors.RetryOutcome{Rearmed: 1}, s.retryErr
}
func (s *stubControls) ListReleases(context.Context, string) (controls.ReleasesResponse, error) {
	return controls.ReleasesResponse{Candidates: []controls.ReleaseCandidate{}}, nil
}
func (s *stubControls) Grab(_ context.Context, _ string, sel controls.ReleaseSelection) (dlstatus.EpisodeResult, error) {
	s.grabSel = sel
	return dlstatus.EpisodeResult{Outcome: dlstatus.OutcomeDownloading}, s.grabErr
}
func (s *stubControls) Upcoming(context.Context) ([]repository.UpcomingItem, error) {
	return nil, nil
}

func controlRouter(stub *stubControls) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := NewDownloadControlHandler(stub, slog.New(slog.NewTextHandler(new(bytes.Buffer), nil)))
	r := gin.New()
	r.POST("/downloads/:id/cancel", h.Cancel)
	r.POST("/downloads/:id/retry", h.RetryNow)
	r.GET("/downloads/:id/releases", h.ListReleases)
	r.POST("/downloads/:id/releases", h.Grab)
	r.GET("/schedule/upcoming", h.Upcoming)
	return r
}

func do(r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

func TestDownloadControlHandler_StatusCodes(t *testing.T) {
	t.Run("cancel ok and not found", func(t *testing.T) {
		assert.Equal(t, http.StatusOK, do(controlRouter(&stubControls{}), "POST", "/downloads/55/cancel", "").Code)
		assert.Equal(t, http.StatusNotFound,
			do(controlRouter(&stubControls{cancelErr: interactors.ErrNothingToControl}), "POST", "/downloads/55/cancel", "").Code)
	})

	t.Run("retry conflict", func(t *testing.T) {
		assert.Equal(t, http.StatusConflict,
			do(controlRouter(&stubControls{retryErr: interactors.ErrDispatchInProgress}), "POST", "/downloads/55/retry", "").Code)
		assert.Equal(t, http.StatusNotFound,
			do(controlRouter(&stubControls{retryErr: interactors.ErrNothingToControl}), "POST", "/downloads/55/retry", "").Code)
	})

	t.Run("grab validates the selection", func(t *testing.T) {
		stub := &stubControls{}
		assert.Equal(t, http.StatusBadRequest, do(controlRouter(stub), "POST", "/downloads/55/releases", `{"title":"x"}`).Code,
			"a title without a guid or url cannot be downloaded")
		w := do(controlRouter(stub), "POST", "/downloads/55/releases", `{"title":"x","guid":"magnet:?x","seeders":9}`)
		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, 9, stub.grabSel.Seeders)
		var result dlstatus.EpisodeResult
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
		assert.Equal(t, dlstatus.OutcomeDownloading, result.Outcome)
	})

	t.Run("upcoming is never null", func(t *testing.T) {
		w := do(controlRouter(&stubControls{}), "GET", "/schedule/upcoming", "")
		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "[]", strings.TrimSpace(w.Body.String()))
	})
}
